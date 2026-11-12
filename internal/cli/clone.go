package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"
	"github.com/spf13/cobra"

	"github.com/ArpanSriv/gg/internal/config"
	"github.com/ArpanSriv/gg/internal/github"
	"github.com/ArpanSriv/gg/internal/repo"
)

func cloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone [owner/repo | owner/ | repo | owner | URL]",
		Short: "Clone a repo as a bare repo plus a worktree for its default branch",
		Long: `Clone a repo as a bare repo plus a worktree for its default branch.

  gg clone owner/repo     clone that repo
  gg clone <URL>          clone from a github.com https or ssh URL
  gg clone owner/         pick one of owner's repos
  gg clone name           clone <default_org>/name, or pick from owner "name"
                          when no default owner is set
  gg clone                pick one of your own repos (needs a token)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			base, err := requireBase(cfg)
			if err != nil {
				return err
			}
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			owner, name, err := resolveCloneTarget(cfg, arg)
			if err != nil {
				return err
			}
			r := repo.At(base, owner, name)
			info("Cloning %s/%s into %s", owner, name, r.Root)
			wt, err := repo.Clone(r, cfg.CloneURL(owner, name), cfg.VSCode())
			if err != nil {
				return err
			}
			printCd(wt)
			return nil
		},
	}
}

func resolveCloneTarget(cfg *config.Config, arg string) (owner, name string, err error) {
	if o, n, ok := parseRepoURL(arg); ok {
		return o, n, nil
	}
	switch {
	case arg == "":
		tok := github.ResolveToken(cfg.PAT)
		if tok.Value == "" {
			return "", "", errors.New("`gg clone` with no argument lists your repos and needs a token; run `gh auth login` or `gg auth login`")
		}
		u, _, err := github.New(tok.Value).CurrentUser()
		if err != nil {
			return "", "", err
		}
		return pickRepo(cfg, u.Login)
	case strings.HasSuffix(arg, "/") && strings.Count(arg, "/") == 1:
		return pickRepo(cfg, strings.TrimSuffix(arg, "/"))
	case strings.Contains(arg, "/"):
		parts := strings.Split(arg, "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("invalid repo %q: want owner/repo", arg)
		}
		return parts[0], strings.TrimSuffix(parts[1], ".git"), nil
	case cfg.DefaultOrg != "":
		return cfg.DefaultOrg, arg, nil
	default:
		return pickRepo(cfg, arg)
	}
}

func pickRepo(cfg *config.Config, owner string) (string, string, error) {
	info("Listing repos of %s...", owner)
	repos, err := apiClient(cfg).ListRepos(owner)
	if err != nil {
		return "", "", err
	}
	if len(repos) == 0 {
		return "", "", fmt.Errorf("%s has no repos you can see", owner)
	}
	sort.Slice(repos, func(i, j int) bool { return strings.ToLower(repos[i].Name) < strings.ToLower(repos[j].Name) })
	idx, err := fuzzyfinder.Find(repos,
		func(i int) string {
			label := repos[i].Name
			if repos[i].Private {
				label += " (private)"
			}
			if repos[i].Archived {
				label += " (archived)"
			}
			return label
		},
		fuzzyfinder.WithPreviewWindow(func(i, _, _ int) string {
			if i < 0 {
				return ""
			}
			return repos[i].FullName + "\n\n" + repos[i].Description
		}),
	)
	if errors.Is(err, fuzzyfinder.ErrAbort) {
		return "", "", errors.New("cancelled")
	}
	if err != nil {
		return "", "", err
	}
	return repos[idx].Owner.Login, repos[idx].Name, nil
}
