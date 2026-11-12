package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ArpanSriv/gg/internal/config"
	"github.com/ArpanSriv/gg/internal/git"
	"github.com/ArpanSriv/gg/internal/repo"
)

func switchCmd() *cobra.Command {
	var create bool
	var from string
	cmd := &cobra.Command{
		Use:   "switch [-c [--from <ref>]] <branch>",
		Short: "Check out a branch in its own worktree",
		Long: `Check out a branch in its own worktree, <repo root>/<branch with / -> _>.
Run it from anywhere inside a gg-managed repo.

Without -c the branch must exist locally or on origin. With -c gg creates a
new branch from origin's default branch (or --from <ref>). The new branch
has no upstream; the first "git push" creates origin/<branch>.

If the branch already has a worktree, gg just prints its path.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if from != "" && !create {
				return errors.New("--from needs -c")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			r, err := currentRepo()
			if err != nil {
				return err
			}
			branch := args[0]
			dir := r.WorktreeDir(branch)

			if repo.IsDir(dir) {
				if cur := checkedOutBranch(dir); cur != branch {
					return fmt.Errorf("%s already exists but has %q checked out", dir, cur)
				}
				if create {
					return fmt.Errorf("branch %s already exists at %s", branch, dir)
				}
				info("%s is already checked out.", branch)
				printCd(dir)
				return nil
			}

			info("Fetching origin...")
			if err := git.Run(r.Bare, "fetch", "--prune", "origin"); err != nil {
				return err
			}
			local, remote := r.HasLocal(branch), r.HasRemote("origin", branch)

			switch {
			case create:
				if local || remote {
					return fmt.Errorf("branch %s already exists; run without -c", branch)
				}
				start := from
				if start == "" {
					start = "origin/" + r.DefaultBranch()
				}
				if err := git.Run(r.Bare, "worktree", "add", "--no-track", "-b", branch, dir, start); err != nil {
					return err
				}
				// Clones made by gg have this already; set it for older ones too.
				r.Git("config", "push.autoSetupRemote", "true")
				info("Created %s from %s. The first `git push` creates origin/%s.", branch, start, branch)
			case local:
				if err := git.Run(r.Bare, "worktree", "add", dir, branch); err != nil {
					return err
				}
				if _, err := r.Git("rev-parse", "--abbrev-ref", branch+"@{upstream}"); err != nil && remote {
					r.Git("branch", "--set-upstream-to=origin/"+branch, branch)
				}
			case remote:
				if err := git.Run(r.Bare, "worktree", "add", "--track", "-b", branch, dir, "origin/"+branch); err != nil {
					return err
				}
			default:
				return fmt.Errorf("no branch %q on origin (use -c to create it)", branch)
			}

			if cfg.VSCode() {
				repo.WriteVSCodeSettings(r, dir)
			}
			printCd(dir)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&create, "create", "c", false, "create a new branch")
	cmd.Flags().StringVar(&from, "from", "", "start point for -c (default: origin/<default branch>)")
	return cmd
}
