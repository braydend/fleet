# fleet

A terminal UI for running and managing multiple isolated [Claude Code](https://claude.com/claude-code)
sessions at once.

Each session runs in its own **git worktree** inside its own **tmux session**, so
sessions are isolated from each other and from your main checkout — and they
survive restarts of the TUI itself. `fleet` gives you one dashboard to create,
attach to, watch, and tear down sessions across all your projects.

## How it works

- Pick a project from a list auto-discovered by scanning a root directory for git repos.
- Create a session: `fleet` makes a git worktree on a new branch (you choose the
  base and branch name) and launches `claude` inside a dedicated tmux session.
- The dashboard shows each session's run state, git info (branch, dirty count,
  ahead/behind), project, worktree path, and creation time — refreshed live.
- Attach to drop into the real interactive Claude Code terminal; detach to come back.
- Clean up from the TUI: delete the worktree + branch, push and open a PR, or just
  leave it for manual handling.

State is always derived from the real world (tmux + git) plus a small
per-worktree `.fleet/meta.json` for the few facts they can't infer — there's no
separate database to drift out of sync.

## Dependencies

Required on your `PATH` at runtime:

| Tool | Purpose |
|------|---------|
| `git` | worktree and branch operations, status queries |
| `tmux` | runs each Claude Code instance in its own session |
| `claude` | the [Claude Code](https://claude.com/claude-code) CLI that each session launches |

Optional:

| Tool | Purpose |
|------|---------|
| `gh` | opening a pull request from the cleanup menu (push still works without it) |

To build from source you also need **Go 1.22+**.

`fleet` checks for `git`, `tmux`, and `claude` at startup and exits with a clear
message if any are missing.

## Install

Clone and build the binary:

```bash
git clone https://github.com/braydend/fleet.git
cd fleet
go build -o fleet .
```

Then put `fleet` somewhere on your `PATH` (e.g. `~/.local/bin`), or run it from
the build directory. During development you can also run it directly with
`go run .`.

## Configuration

`fleet` reads `~/.config/fleet/config.yaml`.

**First run:** if the file doesn't exist, `fleet` prompts you for the directory
to scan for git projects and writes the config for you (using the default
worktree location). You don't have to create it by hand.

To configure it manually instead, create the file yourself:

```yaml
# Directory scanned (one level deep) for git repositories to offer as projects.
scan_root: /home/you/code

# Where session worktrees are created: <worktree_base_dir>/<project>/<session>.
worktree_base_dir: /home/you/.local/share/fleet/worktrees
```

- `scan_root` is **required** (the first-run prompt collects it; it must be an
  existing directory).
- `worktree_base_dir` defaults to `~/.local/share/fleet/worktrees` if omitted.

## Getting started

1. Make sure `git`, `tmux`, and `claude` are installed and on your `PATH`.
2. Run `fleet` (or `go run .`). On first run it prompts for your `scan_root` and
   writes `~/.config/fleet/config.yaml` for you.
3. Press `n`, pick a project, name the session, accept or edit the base/branch,
   and submit. A new session appears on the dashboard.
4. Select it and press `Enter` to attach — you're now in a live Claude Code
   session running in an isolated worktree. Detach with tmux's `Ctrl-b d` to
   return to the dashboard.

### Keybindings

**Dashboard**

| Key | Action |
|-----|--------|
| `n` | new session (opens the project picker) |
| `Enter` | attach to the selected session |
| `d` | cleanup menu for the selected session |
| `r` | refresh now |
| `↑`/`k`, `↓`/`j` | move selection |
| `q` / `Ctrl-c` | quit |

**Project picker / cleanup menu**: `↑`/`↓` to move, `Enter` to choose, `Esc` to cancel.

**New-session form**: `Tab`/`Shift-Tab` (or `↑`/`↓`) to move between fields, type
to edit, `Enter` to advance / submit on the last field, `Esc` to cancel. The
branch defaults to `fleet/<session>` and the base to the project's default branch.

**Cleanup menu** offers three actions for a session:

- **delete worktree + branch** — kills the tmux session and removes the worktree
  (and branch). If the worktree has uncommitted or unpushed changes, you'll be
  asked to confirm.
- **push / open PR** — pushes the branch and, if `gh` is available, opens a PR.
- **leave** — kills only the tmux session, leaving the worktree and branch for
  you to handle manually.

## Development

```bash
go build ./...                    # build everything
go vet ./...                      # vet
go test ./...                     # unit tests (git/tmux integration tests skip if those binaries are absent)
go test -tags smoke -run Smoke ./internal/refresher/   # real-CLI integration smoke (needs git + tmux)
```

Design and implementation notes live in `docs/superpowers/specs/` and
`docs/superpowers/plans/`. See also `CLAUDE.md` for working conventions.

## Status

MVP. Create / attach / kill sessions, live dashboard, and in-TUI cleanup work
across multiple projects. Planned future work (activity/attention detection,
config-registered projects, alternate worktree layouts, session history, bulk
actions, embedded PTY panes) is tracked in the design doc.
