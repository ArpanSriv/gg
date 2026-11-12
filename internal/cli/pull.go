package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/ArpanSriv/gg/internal/config"
	"github.com/ArpanSriv/gg/internal/git"
	"github.com/ArpanSriv/gg/internal/repo"
)

func pullCmd() *cobra.Command {
	pull := &cobra.Command{Use: "pull", Short: "Update repos"}
	var jobs int
	all := &cobra.Command{
		Use:   "all",
		Short: "Fetch every repo and fast-forward its default-branch worktree",
		Long: `Fetch every repo under the base path and fast-forward the worktree of its
default branch. gg only fast-forwards: it skips worktrees that have
uncommitted changes, are on another branch, or have diverged from origin.
It never merges, rebases or resets.

Exits 1 if any repo failed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			base, err := requireBase(cfg)
			if err != nil {
				return err
			}
			repos, err := repo.FindAll(base)
			if err != nil {
				return err
			}
			if len(repos) == 0 {
				info("No repos under %s", base)
				return nil
			}
			// Fetches run unattended; fail instead of waiting for a password.
			os.Setenv("GIT_TERMINAL_PROMPT", "0")

			width := 0
			for _, r := range repos {
				width = max(width, len(r.Owner)+1+len(r.Name))
			}
			if jobs < 1 {
				jobs = 1
			}
			var (
				wg     sync.WaitGroup
				mu     sync.Mutex
				counts = map[string]int{}
				sem    = make(chan struct{}, jobs)
			)
			for _, r := range repos {
				wg.Add(1)
				go func() {
					defer wg.Done()
					sem <- struct{}{}
					status, detail := pullRepo(r)
					<-sem
					mu.Lock()
					defer mu.Unlock()
					counts[status]++
					fmt.Printf("%-10s  %-*s  %s\n", status, width, r.Owner+"/"+r.Name, detail)
				}()
			}
			wg.Wait()
			fmt.Printf("\n%d updated, %d up-to-date, %d skipped, %d errors\n",
				counts["updated"], counts["up-to-date"], counts["skipped"], counts["error"])
			if n := counts["error"]; n > 0 {
				return fmt.Errorf("%d of %d repos failed", n, len(repos))
			}
			return nil
		},
	}
	all.Flags().IntVarP(&jobs, "jobs", "j", 8, "repos to update at the same time")
	pull.AddCommand(all)
	return pull
}

func pullRepo(r *repo.Repo) (status, detail string) {
	if _, err := r.Git("fetch", "--prune", "origin"); err != nil {
		return "error", git.FirstLine(err)
	}
	def := r.DefaultBranch()
	wt := r.WorktreeDir(def)
	if !repo.IsDir(wt) {
		return "skipped", "no " + repo.DirName(def) + " worktree"
	}
	if cur := checkedOutBranch(wt); cur != def {
		return "skipped", fmt.Sprintf("worktree is on %q, not %s", cur, def)
	}
	dirty, err := git.Output(wt, "status", "--porcelain")
	if err != nil {
		return "error", git.FirstLine(err)
	}
	if dirty != "" {
		return "skipped", "uncommitted changes"
	}
	out, err := git.Output(wt, "rev-list", "--left-right", "--count", "HEAD...origin/"+def)
	if err != nil {
		return "error", git.FirstLine(err)
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return "error", "unexpected rev-list output: " + out
	}
	ahead, _ := strconv.Atoi(fields[0])
	behind, _ := strconv.Atoi(fields[1])
	switch {
	case behind == 0 && ahead == 0:
		return "up-to-date", ""
	case behind == 0:
		return "up-to-date", fmt.Sprintf("ahead %d (unpushed)", ahead)
	case ahead > 0:
		return "skipped", fmt.Sprintf("diverged (ahead %d, behind %d)", ahead, behind)
	}
	if _, err := git.Output(wt, "merge", "--ff-only", "--quiet", "origin/"+def); err != nil {
		return "error", git.FirstLine(err)
	}
	return "updated", fmt.Sprintf("+%d", behind)
}
