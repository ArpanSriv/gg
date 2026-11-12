package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/ArpanSriv/gg/internal/config"
	"github.com/ArpanSriv/gg/internal/github"
)

func authCmd() *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Manage the GitHub token gg uses for API calls",
		Long: `gg uses a token only for GitHub API calls (PR lookup, repo listing).
Git itself uses your normal git credentials or SSH keys.

gg looks for a token in this order: $GH_TOKEN, $GITHUB_TOKEN, the PAT saved
by "gg auth login", then "gh auth token". If you use the gh CLI you do not
need "gg auth login" at all.`,
	}
	auth.AddCommand(
		&cobra.Command{
			Use:   "login",
			Short: "Save a personal access token in gg's config",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				pat, login, err := askAndValidatePAT()
				if err != nil {
					return err
				}
				cfg.PAT = pat
				if err := cfg.Save(); err != nil {
					return err
				}
				info("Authenticated as %s", login)
				return nil
			},
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show which token gg uses and check it",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				tok := github.ResolveToken(cfg.PAT)
				if tok.Value == "" {
					info("No token. Public repos still work (60 API requests/hour).")
					return errors.New("not logged in; run `gh auth login` or `gg auth login`")
				}
				u, scopes, err := github.New(tok.Value).CurrentUser()
				if err != nil {
					return err
				}
				info("Authenticated as %s (token from %s)", u.Login, tok.Source)
				if scopes != "" {
					info("Token scopes: %s", scopes)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "logout",
			Short: "Remove the PAT from gg's config",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				cfg.PAT = ""
				if err := cfg.Save(); err != nil {
					return err
				}
				info("Removed the stored PAT.")
				if tok := github.ResolveToken(""); tok.Value != "" {
					info("gg still uses the token from %s.", tok.Source)
				}
				return nil
			},
		},
	)
	return auth
}

// askAndValidatePAT reads a PAT and checks it against the API.
func askAndValidatePAT() (pat, login string, err error) {
	info("Create a token at https://github.com/settings/tokens")
	info("(classic: `repo` scope; fine-grained: Contents and Pull requests, read access).")
	pat, err = readSecret("Token: ")
	if err != nil {
		return "", "", err
	}
	if pat == "" {
		return "", "", errors.New("no token entered")
	}
	u, _, err := github.New(pat).CurrentUser()
	if err != nil {
		return "", "", err
	}
	return pat, u.Login, nil
}
