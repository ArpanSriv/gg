// Package repo knows gg's directory layout:
//
//	<base>/<owner>/<repo>/<repo>.git   bare repo
//	<base>/<owner>/<repo>/<branch>     one worktree per branch ("/" -> "_")
package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ArpanSriv/gg/internal/git"
)

// Repo is one gg-managed repo.
type Repo struct {
	Root  string // <base>/<owner>/<name>
	Bare  string // <root>/<name>.git
	Owner string
	Name  string
}

// DirName maps a branch name to its worktree directory name.
func DirName(branch string) string {
	return strings.ReplaceAll(branch, "/", "_")
}

// At returns the repo for <base>/<owner>/<name>. It may not exist yet.
func At(base, owner, name string) *Repo {
	root := filepath.Join(base, owner, name)
	return &Repo{Root: root, Bare: filepath.Join(root, name+".git"), Owner: owner, Name: name}
}

func isBareName(n string) bool { return strings.HasSuffix(n, ".git") && n != ".git" }

func fromRoot(root, bareName string) *Repo {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return &Repo{
		Root:  root,
		Bare:  filepath.Join(root, bareName),
		Name:  filepath.Base(root),
		Owner: filepath.Base(filepath.Dir(root)),
	}
}

// bareIn returns the name of the *.git directory directly inside dir.
func bareIn(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() && isBareName(e.Name()) {
			return e.Name(), true
		}
	}
	return "", false
}

// FromDir finds the gg-managed repo that contains dir: from any worktree,
// from inside the bare repo, or from the repo root.
func FromDir(dir string) (*Repo, error) {
	if common, err := git.Output(dir, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		common = filepath.Clean(filepath.FromSlash(common))
		if base := filepath.Base(common); isBareName(base) {
			return fromRoot(filepath.Dir(common), base), nil
		}
	}
	if name, ok := bareIn(dir); ok {
		return fromRoot(dir, name), nil
	}
	return nil, errors.New("not inside a gg-managed repo (expected <base>/<owner>/<repo>/<repo>.git)")
}

// FindAll returns every repo at <base>/<owner>/<repo>, sorted.
func FindAll(base string) ([]*Repo, error) {
	owners, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var repos []*Repo
	for _, o := range owners {
		if !o.IsDir() {
			continue
		}
		names, err := os.ReadDir(filepath.Join(base, o.Name()))
		if err != nil {
			continue
		}
		for _, n := range names {
			root := filepath.Join(base, o.Name(), n.Name())
			if !n.IsDir() {
				continue
			}
			if bare, ok := bareIn(root); ok {
				repos = append(repos, fromRoot(root, bare))
			}
		}
	}
	sort.Slice(repos, func(i, j int) bool {
		return strings.ToLower(repos[i].Owner+"/"+repos[i].Name) < strings.ToLower(repos[j].Owner+"/"+repos[j].Name)
	})
	return repos, nil
}

// Exists reports whether the bare repo is on disk.
func (r *Repo) Exists() bool { return IsDir(r.Bare) }

// WorktreeDir is where branch is checked out.
func (r *Repo) WorktreeDir(branch string) string { return filepath.Join(r.Root, DirName(branch)) }

// Git runs git in the bare repo and returns its stdout.
func (r *Repo) Git(args ...string) (string, error) { return git.Output(r.Bare, args...) }

// HasLocal reports whether the local branch exists.
func (r *Repo) HasLocal(branch string) bool { return git.RefExists(r.Bare, "refs/heads/"+branch) }

// HasRemote reports whether <remote>/<branch> exists.
func (r *Repo) HasRemote(remote, branch string) bool {
	return git.RefExists(r.Bare, "refs/remotes/"+remote+"/"+branch)
}

// DefaultBranch is the bare repo's HEAD (set from origin at clone time),
// else main, else master.
func (r *Repo) DefaultBranch() string {
	if b, err := r.Git("symbolic-ref", "--short", "HEAD"); err == nil && r.HasRemote("origin", b) {
		return b
	}
	for _, b := range []string{"main", "master"} {
		if r.HasRemote("origin", b) {
			return b
		}
	}
	return "main"
}

// Clone clones url as a bare repo and checks out the default branch.
// It returns the default branch's worktree, or the repo root when the
// remote repo is empty.
func Clone(r *Repo, url string, vscode bool) (worktree string, err error) {
	if entries, err := os.ReadDir(r.Root); err == nil && len(entries) > 0 {
		return "", fmt.Errorf("%s already exists and is not empty", r.Root)
	}
	if err := os.MkdirAll(r.Root, 0o755); err != nil {
		return "", err
	}
	// r.Root was empty, so on failure everything in it is ours to remove.
	defer func() {
		if err != nil {
			os.RemoveAll(r.Root)
		}
	}()

	if err := git.Run(r.Root, "clone", "--bare", url, r.Bare); err != nil {
		return "", err
	}
	// Bare clones have no remote-tracking refs. Give them the normal ones.
	if _, err := r.Git("config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"); err != nil {
		return "", err
	}
	// The first `git push` of a new branch creates it on the remote.
	if _, err := r.Git("config", "push.autoSetupRemote", "true"); err != nil {
		return "", err
	}
	if err := git.Run(r.Bare, "fetch", "origin"); err != nil {
		return "", err
	}

	def := r.DefaultBranch()
	// A bare clone copies every remote branch to a local branch. Keep only
	// the default one, so other branches start fresh from origin when you
	// switch to them.
	if out, err := r.Git("for-each-ref", "--format=%(refname)", "refs/heads"); err == nil {
		var del strings.Builder
		for _, ref := range strings.Fields(out) {
			if ref != "refs/heads/"+def {
				fmt.Fprintf(&del, "delete %s\n", ref)
			}
		}
		if del.Len() > 0 {
			if _, err := git.OutputInput(r.Bare, del.String(), "update-ref", "--stdin"); err != nil {
				return "", err
			}
		}
	}

	if !r.HasLocal(def) {
		fmt.Fprintln(os.Stderr, "The repository is empty; no worktree was created.")
		return r.Root, nil
	}
	wt := r.WorktreeDir(def)
	if err := git.Run(r.Bare, "worktree", "add", wt, def); err != nil {
		return "", err
	}
	if _, err := r.Git("branch", "--set-upstream-to=origin/"+def, def); err != nil {
		return "", err
	}
	if vscode {
		WriteVSCodeSettings(r, wt)
	}
	return wt, nil
}

// WriteVSCodeSettings sets "window.title" in <wt>/.vscode/settings.json,
// merging with existing settings. It leaves the file alone when the repo
// tracks it or it is not plain JSON, and adds it to info/exclude so it does
// not make the worktree dirty. Problems are reported as warnings.
func WriteVSCodeSettings(r *Repo, wt string) {
	warn := func(err error) { fmt.Fprintln(os.Stderr, "warning: .vscode/settings.json:", err) }
	if _, err := git.Output(wt, "ls-files", "--error-unmatch", ".vscode/settings.json"); err == nil {
		return
	}
	p := filepath.Join(wt, ".vscode", "settings.json")
	settings := map[string]any{}
	if data, err := os.ReadFile(p); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			warn(errors.New("not plain JSON, leaving it unchanged"))
			return
		}
	}
	settings["window.title"] = r.Name + " ${rootName}"
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		warn(err)
		return
	}
	if err := os.WriteFile(p, append(data, '\n'), 0o644); err != nil {
		warn(err)
		return
	}
	if err := addExclude(r, "/.vscode/settings.json"); err != nil {
		warn(err)
	}
}

// addExclude adds a pattern to the shared info/exclude of the bare repo.
func addExclude(r *Repo, pattern string) error {
	p := filepath.Join(r.Bare, "info", "exclude")
	data, err := os.ReadFile(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return os.WriteFile(p, append(data, pattern+"\n"...), 0o644)
}

// IsDir reports whether p is an existing directory.
func IsDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
