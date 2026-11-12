package cli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ArpanSriv/gg/internal/config"
	"github.com/ArpanSriv/gg/internal/git"
	"github.com/ArpanSriv/gg/internal/repo"
)

func prCmd() *cobra.Command {
	pr := &cobra.Command{Use: "pr", Short: "Work with pull requests"}
	pr.AddCommand(&cobra.Command{
		Use:   "checkout <number | owner/repo#number | PR URL>",
		Short: "Check out a pull request in its own worktree",
		Long: `Check out a pull request in its own worktree, named after its head branch.

A bare number uses the repo you are in. gg clones the repo first if needed.
For a PR from a fork, gg adds a remote named after the fork's owner, so
"git pull" and "git push" go to the contributor's branch. If the fork can't
be fetched, or the PR's branch is gone, gg makes a read-only snapshot of the
PR with no upstream.

If the worktree already exists, gg prints its path and does not update it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error { return prCheckout(args[0]) },
	})
	return pr
}

var (
	prURLRe = regexp.MustCompile(`^https?://github\.com/([^/\s]+)/([^/\s]+)/pull/(\d+)(?:[/?#].*)?$`)
	prRefRe = regexp.MustCompile(`^([^/\s#]+)/([^/\s#]+)#(\d+)$`)
	prNumRe = regexp.MustCompile(`^#?(\d+)$`)
)

// parsePRRef parses a PR reference. owner and name are empty for a bare number.
func parsePRRef(s string) (owner, name string, number int, err error) {
	var m []string
	switch {
	case prURLRe.MatchString(s):
		m = prURLRe.FindStringSubmatch(s)
	case prRefRe.MatchString(s):
		m = prRefRe.FindStringSubmatch(s)
	case prNumRe.MatchString(s):
		m = append([]string{s, "", ""}, prNumRe.FindStringSubmatch(s)[1])
	default:
		return "", "", 0, fmt.Errorf("invalid PR %q: want a number, owner/repo#number or a PR URL", s)
	}
	number, err = strconv.Atoi(m[3])
	return m[1], m[2], number, err
}

func prCheckout(ref string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	base, err := requireBase(cfg)
	if err != nil {
		return err
	}
	owner, name, num, err := parsePRRef(ref)
	if err != nil {
		return err
	}

	var r *repo.Repo
	if owner == "" {
		if r, err = currentRepo(); err != nil {
			return fmt.Errorf("%w; use owner/repo#%d", err, num)
		}
		owner, name = r.Owner, r.Name
	}

	pr, err := apiClient(cfg).PullRequest(owner, name, num)
	if err != nil {
		return err
	}
	info("#%d %s (%s)", pr.Number, pr.Title, pr.State)
	if r == nil {
		r = repo.At(base, pr.Base.Repo.Owner.Login, pr.Base.Repo.Name)
	}
	if !r.Exists() {
		info("Cloning %s into %s", pr.Base.Repo.FullName, r.Root)
		if _, err := repo.Clone(r, cfg.CloneURL(pr.Base.Repo.Owner.Login, pr.Base.Repo.Name), cfg.VSCode()); err != nil {
			return err
		}
	}

	headRef := pr.Head.Ref
	isFork := pr.Head.Repo == nil || !strings.EqualFold(pr.Head.Repo.FullName, pr.Base.Repo.FullName)
	localBranch := headRef
	if isFork && headRef == r.DefaultBranch() {
		// Don't collide with the default branch's worktree. Not "owner/ref":
		// that is ambiguous with the remote-tracking ref of the same name.
		localBranch = pr.Head.User.Login + "-" + headRef
	}
	dir := r.WorktreeDir(localBranch)
	if repo.IsDir(dir) {
		info("Already checked out. Run `git pull` there to update it.")
		printCd(dir)
		return nil
	}

	info("Fetching origin...")
	if err := git.Run(r.Bare, "fetch", "--prune", "origin"); err != nil {
		return err
	}

	tracking := ""
	if isFork && pr.Head.Repo != nil {
		remote := pr.Head.Repo.Owner.Login
		if err := ensureRemote(r, remote, cfg.CloneURL(remote, pr.Head.Repo.Name)); err != nil {
			return err
		}
		info("Fetching %s from %s...", headRef, remote)
		if err := git.Run(r.Bare, "fetch", remote, headRef); err == nil && r.HasRemote(remote, headRef) {
			tracking = remote + "/" + headRef
		} else {
			info("Could not fetch %s's fork.", remote)
		}
	} else if !isFork && r.HasRemote("origin", headRef) {
		tracking = "origin/" + headRef
	}

	if tracking == "" && !r.HasLocal(localBranch) {
		info("Making a read-only snapshot of the PR (no upstream).")
		if err := git.Run(r.Bare, "fetch", "origin", fmt.Sprintf("pull/%d/head:refs/heads/%s", num, localBranch)); err != nil {
			return err
		}
	}

	if r.HasLocal(localBranch) {
		if err := git.Run(r.Bare, "worktree", "add", dir, localBranch); err != nil {
			return err
		}
		if tracking != "" {
			if _, err := r.Git("branch", "--set-upstream-to="+tracking, localBranch); err != nil {
				return err
			}
		}
	} else if err := git.Run(r.Bare, "worktree", "add", "--track", "-b", localBranch, dir, tracking); err != nil {
		return err
	}

	if tracking != "" && localBranch != headRef {
		remote, _, _ := strings.Cut(tracking, "/")
		info("The local branch is %s. Push with: git push %s HEAD:%s", localBranch, remote, headRef)
	}
	if cfg.VSCode() {
		repo.WriteVSCodeSettings(r, dir)
	}
	printCd(dir)
	return nil
}

// ensureRemote adds the remote, or fixes its URL.
func ensureRemote(r *repo.Repo, name, url string) error {
	cur, err := r.Git("remote", "get-url", name)
	switch {
	case err != nil:
		_, err = r.Git("remote", "add", name, url)
	case cur != url:
		_, err = r.Git("remote", "set-url", name, url)
	}
	return err
}
