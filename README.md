# gg

Git workflow CLI for github.com, built around a "bare repo + one worktree per
branch" layout. Many branches can be checked out at once, with no stashing or
branch switching.

```
<base>/<owner>/<repo>/
    <repo>.git/     bare repo (shared object store, remote "origin")
    main/           worktree for the default branch
    fix_thing/      worktree for branch "fix/thing" ("/" -> "_")
```

gg runs your `git` for all repo work and calls the GitHub API only for
metadata (PR lookup, repo listing). It never changes your shell's directory;
it prints the `cd` to run (the only thing it writes to stdout), so in
PowerShell `gg switch foo | iex` takes you there.

## Install

```
go install ./cmd/gg
gg init
```

## Commands

```
gg init                                  interactive setup, safe to re-run
gg auth {login | status | logout}        manage the token gg uses for API calls
gg clone [owner/repo | URL | owner/ | repo | owner]
gg switch [-c [--from <ref>]] <branch>
gg pr checkout {<n> | owner/repo#<n> | <PR URL>}
gg pull all [-j N]
```

- **clone** makes the bare repo, sets up normal `origin/*` tracking refs and
  checks out the default branch. `owner/` (or a bare `owner` when no default
  owner is set) opens a fuzzy picker; no argument picks from your own repos.
- **switch** checks out an existing branch (local or on origin) in its own
  worktree, or with `-c` creates one from origin's default branch. New
  branches have no upstream; the first `git push` creates `origin/<branch>`
  (gg sets `push.autoSetupRemote`). If the worktree exists, gg prints its path.
- **pr checkout** clones the repo if needed. For fork PRs it adds a remote
  named after the fork owner so push/pull go to the contributor's branch; if
  the fork is gone it makes a read-only snapshot from `pull/<n>/head`. A fork
  PR from the fork's default branch gets the local branch `<owner>-<branch>`
  so it doesn't collide with your default-branch worktree.
- **pull all** fetches every repo under the base path and fast-forwards each
  default-branch worktree. It skips dirty, diverged or off-branch worktrees and
  never merges, rebases or resets. Exits 1 if any repo failed.

## Configuration

`%AppData%\gg\config.json` on Windows (`$XDG_CONFIG_HOME/gg/config.json` if
set, or `$GG_CONFIG` to override):

| field                   | meaning                                                   |
|-------------------------|-----------------------------------------------------------|
| `base_path`             | where repos go (default `~/src/github.com`)               |
| `default_org`           | lets `gg clone <repo>` skip the owner                     |
| `protocol`              | `ssh` or `https`; default follows `gh config git_protocol` |
| `write_vscode_settings` | write `window.title` into `.vscode/settings.json` (default true) |
| `pat`                   | optional token, see below                                 |

## Authentication

Public repos need no token. For private repos, the repo picker and higher rate
limits, gg uses the first of: `$GH_TOKEN`, `$GITHUB_TOKEN`, the PAT from
`gg auth login`, `gh auth token`. If you use the gh CLI there's nothing to set
up. The token is only used for API calls; git uses your own credentials or
SSH keys.

## VS Code settings

When enabled, each new worktree gets `"window.title": "<repo> ${rootName}"`
merged into `.vscode/settings.json`. gg skips the file if the repo tracks it
or it isn't plain JSON, and adds it to the repo's `info/exclude` so it never
makes a worktree dirty.

## Not done yet

- No command to remove worktrees: `git worktree remove <dir>` (and
  `git branch -D <branch>`).
- `gg pr checkout` does not update an existing PR worktree; run `git pull` in it.
