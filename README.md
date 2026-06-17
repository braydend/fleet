# fleet

A terminal UI for running and managing multiple isolated [Claude Code](https://claude.com/claude-code)
sessions at once.

Each session runs in its own **git worktree** inside its own **tmux session**, so
sessions are isolated from each other and from your main checkout — and they
survive restarts of the TUI itself. `fleet` gives you one dashboard to create,
attach to, watch, and tear down sessions across all your projects.

![fleet demo](docs/assets/demo.gif)

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

## Quick start

You need `git`, `tmux`, and `claude` on your `PATH` (and `gh` if you want to open
PRs from the cleanup menu).

Install a prebuilt binary from the
[releases page](https://github.com/braydend/fleet/releases), or build from source:

```bash
git clone https://github.com/braydend/fleet.git
cd fleet
go build -o fleet .   # or: go run .
```

Then run it:

1. Run `fleet`. On first run it prompts for the directory to scan for git
   projects (`scan_root`) and writes `~/.config/fleet/config.yaml` for you.
2. Press `n`, pick a project, name the session, accept or edit the base/branch,
   and submit. A new session appears on the dashboard.
3. Select it and press `Enter` to attach — you're now in a live Claude Code
   session in an isolated worktree. Detach (`<prefix> d`, e.g. `Ctrl-b d`) to
   return to the dashboard.

If a session has exited (shown as `○`), selecting it relaunches Claude Code in
its existing worktree.

When a newer release is published, an update banner appears on the dashboard —
press `u` to update the binary in place (checksum-verified), then restart fleet.

See the [usage guide](docs/usage.md) for full install, configuration, self-update,
and keybinding details.

## Configuration

`fleet` reads `~/.config/fleet/config.yaml`; on first run it prompts you and
writes the file for you. The two fields are `scan_root` (required — the directory
scanned for projects) and `worktree_base_dir` (where session worktrees are
created; defaults to `~/.local/share/fleet/worktrees`). See the
[usage guide](docs/usage.md#configuration) for the full reference.

## Keybindings

The essentials on the dashboard:

| Key | Action |
|-----|--------|
| `n` | new session (opens the project picker) |
| `Enter` | attach to the selected session |
| `d` | cleanup menu for the selected session |
| `r` | refresh now |
| `q` / `Ctrl-c` | quit |

See the [usage guide](docs/usage.md#keybindings) for the project picker,
new-session form, and cleanup menu keys.

## Status

MVP. Create / attach / kill sessions, live dashboard, and in-TUI cleanup work
across multiple projects. Planned future work is tracked in the
[design doc](docs/superpowers/specs/2026-06-16-fleet-tui-design.md).

## Contributing

Build/test commands, the development workflow, and working conventions live in
[`CONTRIBUTING.md`](CONTRIBUTING.md).
