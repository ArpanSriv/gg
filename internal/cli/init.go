package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ArpanSriv/gg/internal/config"
	"github.com/ArpanSriv/gg/internal/github"
	"github.com/ArpanSriv/gg/internal/repo"
)

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Set up gg interactively (safe to re-run)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			base := expandHome(prompt("Base path for repos", cfg.Base()))
			if !repo.IsDir(base) {
				if confirm(fmt.Sprintf("%s does not exist. Create it?", base), true) {
					if err := os.MkdirAll(base, 0o755); err != nil {
						return err
					}
				} else {
					info("Create %s before cloning.", base)
				}
			}
			cfg.BasePath = base

			org := prompt("Default owner for `gg clone <repo>` (blank for none, - to clear)", cfg.DefaultOrg)
			if org == "-" {
				org = ""
			}
			cfg.DefaultOrg = org

			for {
				p := prompt("Git protocol (ssh or https)", cfg.GitProtocol())
				if p == "ssh" || p == "https" {
					cfg.Protocol = p
					break
				}
			}

			vs := confirm("Write .vscode/settings.json (window title) into new worktrees?", cfg.VSCode())
			cfg.WriteVSCodeSettings = &vs

			tok := github.ResolveToken(cfg.PAT)
			var askPAT bool
			switch {
			case tok.Value != "" && tok.Source != "gg config":
				info("gg will use the GitHub token from %s.", tok.Source)
				askPAT = confirm("Store a separate PAT in gg's config instead?", false)
			case cfg.PAT != "":
				askPAT = confirm("A PAT is stored. Replace it?", false)
			default:
				info("No GitHub token found. Public repos work without one, but private repos,")
				info("`gg clone <owner>/` and higher API rate limits need one. `gh auth login` also works.")
				askPAT = confirm("Add a PAT now?", false)
			}
			if askPAT {
				pat, login, err := askAndValidatePAT()
				if err != nil {
					return err
				}
				info("Authenticated as %s", login)
				cfg.PAT = pat
			}

			if err := cfg.Save(); err != nil {
				return err
			}
			p, _ := config.Path()
			info("Saved %s", p)
			return nil
		},
	}
}
