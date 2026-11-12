// Package cli holds gg's commands.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ArpanSriv/gg/internal/config"
	"github.com/ArpanSriv/gg/internal/git"
	"github.com/ArpanSriv/gg/internal/github"
	"github.com/ArpanSriv/gg/internal/repo"
)

// version can be set at build time:
//
//	go build -ldflags "-X github.com/ArpanSriv/gg/internal/cli.version=v1.2.3"
var version string

func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "gg",
		Short: "Git workflow CLI for github.com: one bare repo, one worktree per branch",
		Long: `gg clones each repo as a bare repo and checks out every branch as its
own worktree:

  <base>/<owner>/<repo>/<repo>.git   bare repo
  <base>/<owner>/<repo>/main         default branch
  <base>/<owner>/<repo>/fix_thing    branch "fix/thing"

gg never changes your shell's directory. It prints the cd command to run.`,
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(initCmd(), authCmd(), cloneCmd(), switchCmd(), prCmd(), pullCmd())
	return root
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(devel)"
}

// info prints a message for the user on stderr.
func info(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// printCd prints the command that takes the user to path. It is the only
// thing gg writes to stdout.
func printCd(path string) {
	if strings.ContainsAny(path, " '&()") {
		path = `"` + path + `"`
	}
	fmt.Printf("cd %s\n", path)
}

func requireBase(cfg *config.Config) (string, error) {
	base := cfg.Base()
	if !repo.IsDir(base) {
		return "", fmt.Errorf("base path %s does not exist; run `gg init`", base)
	}
	return base, nil
}

func apiClient(cfg *config.Config) *github.Client {
	return github.New(github.ResolveToken(cfg.PAT).Value)
}

func currentRepo() (*repo.Repo, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return repo.FromDir(cwd)
}

// checkedOutBranch returns the branch checked out in dir, or "".
func checkedOutBranch(dir string) string {
	b, _ := git.Output(dir, "branch", "--show-current")
	return b
}

var repoURLRe = regexp.MustCompile(`^(?:https?://|ssh://git@|git@)?github\.com[/:]([^/\s]+)/([^/\s]+?)(?:\.git)?/?$`)

// parseRepoURL parses a github.com repo URL in https, ssh or scp form.
func parseRepoURL(s string) (owner, name string, ok bool) {
	m := repoURLRe.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

var stdin = bufio.NewReader(os.Stdin)

// prompt asks for a line of input, showing def as the default.
func prompt(question, def string) string {
	if def != "" {
		fmt.Fprintf(os.Stderr, "%s [%s]: ", question, def)
	} else {
		fmt.Fprintf(os.Stderr, "%s: ", question)
	}
	line, _ := stdin.ReadString('\n')
	if line = strings.TrimSpace(line); line == "" {
		return def
	}
	return line
}

func confirm(question string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	for {
		ans := strings.ToLower(prompt(question+" ("+hint+")", ""))
		switch ans {
		case "":
			return def
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
	}
}

// readSecret reads a line without echo when stdin is a terminal.
func readSecret(question string) (string, error) {
	fmt.Fprint(os.Stderr, question)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(string(b)), err
	}
	line, err := stdin.ReadString('\n')
	if strings.TrimSpace(line) != "" {
		err = nil
	}
	return strings.TrimSpace(line), err
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[1:])
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p
}
