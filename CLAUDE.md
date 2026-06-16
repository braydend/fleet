# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

`fleet` — a terminal UI (TUI) for running and managing multiple isolated
Claude Code instances at once. Each session runs in its own **git worktree**
inside its own **tmux session**, so sessions are isolated from each other and
survive restarts of the TUI itself.

Full design: [`docs/superpowers/specs/2026-06-16-fleet-tui-design.md`](docs/superpowers/specs/2026-06-16-fleet-tui-design.md).

## Status

Pre-implementation. Design approved; implementation plan not yet written.

## Tech stack

- **Language:** Go
- **TUI:** Bubble Tea (+ Lip Gloss styling, Bubbles components) — Charm ecosystem
- **Execution backend:** tmux (one tmux session per Claude Code instance)
- **External CLIs depended on:** `tmux`, `git`, `gh` (for PR creation)

## Core design decisions

- **Session model:** tmux-backed. Each instance = a tmux session named
  `fleet-<project>-<session>` running `claude` in its worktree.
- **Project discovery:** scan a configured root directory for git repos.
- **Worktree location:** central dir per project —
  `<worktreeBaseDir>/<project>/<session>`.
- **Branching:** user chooses per session; default base = repo default branch,
  default branch name = `fleet/<session>`.
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

## MVP scope

Create / attach / kill sessions; live dashboard (run state, git info, project +
path + created-at); in-TUI cleanup (delete / push+PR / leave); correct handling
of N concurrent sessions across projects.

Explicitly **future** (do not build yet): activity/attention detection,
config-registered projects, alternate worktree layouts, session history, bulk
actions, embedded PTY panes.

## Working conventions

- **TDD:** write tests before implementation (project follows the superpowers
  workflow).
- Keep adapters thin and behind interfaces; keep domain logic free of direct
  CLI calls.
- Surface errors in the UI status line; never panic on a malformed worktree or
  meta file.
- Destructive actions (delete with uncommitted/unpushed changes) require
  confirmation.

## Maintenance

Keep this file and the design spec in sync as decisions change. Update the
**Status** section as the project moves from design → implementation.
