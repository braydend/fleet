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

## Build & run

- Build: `go build ./...` (or `go build -o fleet .`)
- Test: `go test ./...` (git/tmux integration tests skip if those binaries are
  absent). A build-tagged real-CLI smoke test lives in `internal/refresher`:
  `go test -tags smoke -run Smoke ./internal/refresher/`.
- Run: `go run .` — requires `git`, `tmux`, and `claude` on PATH. On first run
  (no config file) it prompts for `scan_root` and writes the config; thereafter
  it loads `~/.config/fleet/config.yaml`.
- Config (`~/.config/fleet/config.yaml`):
  ```yaml
  scan_root: /home/you/code
  worktree_base_dir: /home/you/.local/share/fleet/worktrees
  tmux_socket: fleet   # dedicated tmux server; "" = use the default tmux server
  ```

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

## MVP scope

Create / attach / kill sessions; live dashboard (run state, git info, project +
path + created-at); in-TUI cleanup (delete / push+PR / leave); correct handling
of N concurrent sessions across projects.

Explicitly **future** (do not build yet): activity/attention detection,
config-registered projects, alternate worktree layouts, session history, bulk
actions, embedded PTY panes.

## Development workflow (hard rules)

These rules are **mandatory**, not advisory. They exist so that every change
leaves behind durable design documentation that future agentic workers can
read to understand *why* the code looks the way it does. The artefacts under
`docs/superpowers/` are treated as the authoritative historical record of the
project's design decisions.

### 1. Every feature request follows the superpowers spec → plan flow

No feature work begins without documentation. For **every** feature request,
regardless of size:

1. **Brainstorm** the intent and requirements (superpowers `brainstorming`
   skill) before designing.
2. Write a **design spec** in `docs/superpowers/specs/` named
   `YYYY-MM-DD-<short-name>-design.md`.
3. Write an **implementation plan** in `docs/superpowers/plans/` named
   `YYYY-MM-DD-<short-name>.md`. The plan must reference its spec and the
   CLAUDE.md conventions, and use checkbox (`- [ ]`) task syntax.

Both a spec **and** a plan are required for every feature — never one without
the other. These follow the same format as the existing artefacts in those
directories; match their structure (Goal / Background / Status header, etc.).

### 2. GitHub issues are a valid entrypoint — and must be linked

A GitHub issue may be the entrypoint for a feature **or** a bug fix. When
addressing an issue:

- Still produce the spec + plan artefacts described above and check them in.
- **Link bidirectionally:** the spec/plan header must cite the issue
  (e.g. `**Issue:** #12`), and a comment must be posted back on the issue
  linking to the committed spec/plan paths. Use `gh` for issue interaction.
- Bug fixes that are genuinely trivial (one-line, no design choice) still need
  a plan documenting the fix and the issue link; a full design spec is at your
  discretion only when there is no design decision to record — when in doubt,
  write both.

### 3. Artefacts are checked in with the work

The spec and plan must be committed within the **same PR** as the
implementation (commit order is flexible). A PR that changes behaviour without
its accompanying superpowers documentation is incomplete and should not be
merged.

### 4. Commit messages MUST follow Conventional Commits

Releases and version numbers are automated from commit history via
release-please (see
[`docs/superpowers/specs/2026-06-16-release-binaries-design.md`](docs/superpowers/specs/2026-06-16-release-binaries-design.md)).
Versioning therefore depends on every commit following the
[Conventional Commits](https://www.conventionalcommits.org/) format:

- `feat: ...` → minor bump, `fix: ...` → patch bump.
- A `!` after the type (`feat!:`) or a `BREAKING CHANGE:` footer → major bump.
- Other types (`docs:`, `chore:`, `test:`, `refactor:`, `ci:`, etc.) do not
  trigger a release on their own.
- An optional scope is encouraged: `feat(ui): ...`, `fix(tmux): ...`.

A non-conforming commit message silently breaks version inference, so this is
**mandatory** for every commit.

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
