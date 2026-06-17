# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`fleet` — a terminal UI (TUI) for running and managing multiple isolated
Claude Code instances at once. Each session runs in its own **git worktree**
inside its own **tmux session**, so sessions are isolated from each other and
survive restarts of the TUI itself.

Full design: [`docs/superpowers/specs/2026-06-16-fleet-tui-design.md`](docs/superpowers/specs/2026-06-16-fleet-tui-design.md).

## Status

MVP implemented. Core loop works: scan projects → create session (worktree +
tmux + meta) → attach/detach → live dashboard → cleanup (delete / push+PR /
leave). Run with `go run .` (needs `~/.config/fleet/config.yaml` with
`scan_root` set). Implementation plan:
`docs/superpowers/plans/2026-06-16-fleet-tui.md`.

Self-update is implemented: on startup (and hourly) fleet checks GitHub Releases
for a newer version; when one is found a banner appears on the dashboard and
pressing `u` applies an in-place binary swap (checksum-verified).

## Build & run

Build/test/run commands live in [`CONTRIBUTING.md`](CONTRIBUTING.md#build--run);
the full config reference is in [`docs/usage.md`](docs/usage.md#configuration).
In short: `go build ./...`, `go test ./...`, `go run .` (needs `git`, `tmux`,
`claude` on PATH).

## Tech stack

- **Language:** Go
- **TUI:** Bubble Tea (+ Lip Gloss styling, Bubbles components) — Charm ecosystem
- **Execution backend:** tmux (one tmux session per Claude Code instance)
- **External CLIs depended on:** `tmux`, `git`, `gh` (for PR creation)

## Core design decisions

- **Session model:** tmux-backed. fleet runs on its own **dedicated tmux server**
  (`tmux -L fleet`, configurable via `tmux_socket`) so it is fully isolated from
  the user's personal tmux — fleet's options/keybindings/`kill-session` can't
  touch the default server, and vice versa (this isolation is what fixed #5,
  where the test suite's `kill-session fleet-workspace` on the shared default
  server was destroying live sessions). On that server, all instances share one
  session (`fleet-workspace`); each instance is a *window* named
  `fleet-<project>-<session>` running `claude` in its worktree. Windows act as
  tabs — switch with Alt-1..9 / Alt-←/→ while attached. Per-session activity
  (working / waiting / idle / exited) is derived from tmux's window-activity
  timestamp plus a best-effort capture-pane prompt match (`internal/activity`).
  Tests and the integration smoke test each run on their own private socket
  (`NewWithSocket`) so they never touch a real fleet server.
- **Project discovery:** scan a configured root directory for git repos.
- **Worktree location:** central dir per project —
  `<worktreeBaseDir>/<project>/<session>`.
- **Branching:** user chooses per session; default base = repo default branch,
  default branch name = the (sanitized) session name.
- **State management (approach C):** tmux + git are authoritative for live
  state (liveness, branch, dirty/ahead/behind). A small per-worktree
  `.fleet/meta.json` holds only what they can't infer (base branch, created-at,
  cleanup intent). No separate database → no drift. Because meta lives inside
  the worktree, removing the worktree removes the metadata.

## Planned package layout

Each package does one thing, testable in isolation. Adapters (`tmux`, `git`)
sit behind interfaces so domain logic can be unit-tested with fakes.

- `config` — load/validate `~/.config/fleet/config.yaml`.
- `projects` — scan root dir, discover git repos.
- `tmux` — adapter over the tmux CLI (list/create/kill/attach/liveness).
- `git` — worktree + branch ops, status queries, push, `gh` PR open.
- `meta` — read/write per-worktree `.fleet/meta.json`.
- `session` — domain model (create / attach / tear down).
- `refresher` — rebuild live session list on tick + on demand.
- `ui` — Bubble Tea views (project picker, dashboard, new-session form,
  confirm dialogs, status line).
- `selfupdate` — check GitHub Releases for a newer version, verify asset
  checksum, and swap the running binary in place.

## MVP scope

Create / attach / kill sessions; live dashboard (run state, git info, project +
path + created-at); in-TUI cleanup (delete / push+PR / leave); correct handling
of N concurrent sessions across projects.

Explicitly **future** (do not build yet): activity/attention detection,
config-registered projects, alternate worktree layouts, session history, bulk
actions, embedded PTY panes.

## Development workflow (hard rules)

The mandatory development workflow — the brainstorm → spec → plan flow, GitHub
issue linking, artefacts-checked-in-with-the-PR rule, and Conventional Commits
requirement — is documented in [`CONTRIBUTING.md`](CONTRIBUTING.md#development-workflow-hard-rules).
These rules are mandatory for every change; follow them exactly.

## Working conventions

See [`CONTRIBUTING.md`](CONTRIBUTING.md#working-conventions) for the working
conventions (TDD, thin adapters behind interfaces, errors surfaced in the status
line, confirmation for destructive actions).

## Maintenance

Keep this file and the design spec in sync as decisions change. Update the
**Status** section as the project moves from design → implementation.
