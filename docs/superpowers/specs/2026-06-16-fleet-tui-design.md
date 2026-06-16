# Fleet — a TUI for managing multiple Claude Code instances

**Date:** 2026-06-16
**Status:** Design approved, pending implementation plan
**Working name:** `fleet` (rename-able)

## Summary

`fleet` is a terminal UI for running and managing many isolated Claude Code
sessions at once. Each session runs in its own git worktree (isolated from the
others and from the user's main checkout) inside its own tmux session, so
sessions survive restarts of the TUI itself. The user picks a project from an
auto-discovered list, creates a session (choosing base + branch), attaches to a
real interactive Claude Code terminal, and cleans up when done — all from the
dashboard.

## Goals

- Run N isolated Claude Code sessions across multiple projects without them
  interfering with each other.
- Make creating, attaching to, observing, and tearing down sessions fast and
  obvious.
- Survive TUI restarts: the running Claude sessions live in tmux, not in the TUI
  process.
- No drift: live state is always derived from the real world (tmux + git), never
  from a database that can get out of sync.

## Non-goals (v1)

- Activity/attention detection ("Claude is waiting for you") — future.
- Embedded PTY panes — future (we use `tmux attach`).
- Performance tuning for very large session counts — correctness first.
- Config-file-registered projects and multiple/alternate scan roots — future.

## Key decisions

| Decision | Choice |
|----------|--------|
| Language / framework | Go + Bubble Tea (Charm: Lip Gloss, Bubbles) |
| Session execution model | tmux-backed sessions (one tmux session per Claude instance) |
| Project discovery | Scan a configured root directory for git repos |
| Worktree location | Central dir per project: `<worktreeBaseDir>/<project>/<session>` |
| Branching | User chooses per session: default base = repo default branch, default branch name = the (sanitized) session name |
| State management | Approach C — tmux + git authoritative for live state; small per-worktree `.fleet/meta.json` for facts they can't infer |
| Cleanup options | Delete worktree+branch / push+open PR / leave for manual handling |

## Architecture

A single Go binary that is the Bubble Tea TUI. No long-running daemon: the TUI
process is the app, while tmux holds the actual Claude Code sessions so they
outlive it.

```
┌──────────────────────────────────────────────┐
│  fleet (Bubble Tea TUI)                        │
│  ┌──────────┐  ┌───────────┐  ┌────────────┐  │
│  │ UI/views │→ │  app state │← │  refresher │  │
│  └──────────┘  └───────────┘  └────────────┘  │
│        │             │               │         │
│        ▼             ▼               ▼         │
│   ┌─────────┐  ┌──────────┐   ┌────────────┐   │
│   │ tmux    │  │ git/wt   │   │ projects   │   │
│   │ adapter │  │ adapter  │   │ scanner    │   │
│   └─────────┘  └──────────┘   └────────────┘   │
└────────┼────────────┼──────────────┼───────────┘
         ▼            ▼               ▼
       tmux        git CLI       filesystem
```

### Packages

Each package does one thing and is testable in isolation.

- **`config`** — load/validate config from `~/.config/fleet/config.yaml`: scan
  root dir(s), worktree base dir, defaults. Provides sane defaults if absent.
- **`projects`** — scan the root dir, find git repos, return projects
  (name, path, default branch).
- **`tmux`** — thin adapter over the `tmux` CLI: list / create / kill / attach
  sessions, check liveness. Sessions namespaced `fleet-<project>-<session>`.
  Behind an interface for testing.
- **`git`** — worktree + branch ops: add/remove worktree, create branch off a
  chosen base, query status (branch, dirty/clean, ahead/behind, change count),
  push, and a `gh`-backed PR open. Behind an interface for testing.
- **`meta`** — read/write per-worktree `.fleet/meta.json` (base branch,
  created-at, cleanup intent). This is the approach-C metadata that tmux/git
  cannot infer.
- **`session`** — domain model tying a tmux session + worktree + meta together.
  Knows how to create, attach, and tear down a session.
- **`refresher`** — periodically (tick) and on demand rebuilds the live session
  list from tmux + git + meta, emitting a Bubble Tea message to update state.
- **`ui`** — views/components: project picker, session dashboard, new-session
  form, confirm dialogs, status/toast line.

## Data flow

### State derivation (approach C)

On startup and every refresh tick (~2s, plus manual `r`), the `refresher`:

1. Asks `tmux` for all sessions matching `fleet-*` → liveness + names.
2. For each, reads `.fleet/meta.json` from its worktree → base branch,
   created-at, cleanup intent.
3. Runs `git` in each worktree → branch, dirty/clean, ahead/behind, change
   count.
4. Emits a `sessionsUpdated` message; the dashboard re-renders.

Sessions found on disk but with a dead tmux session show as `exited`
(re-attach/restart or clean up). Because meta lives *inside* the worktree, when
the worktree is removed the metadata goes with it — no reconciliation needed.

### Create a session

1. Dashboard → `n` → project picker (from `projects` scan).
2. New-session form: session name; base branch (default = project default
   branch); branch name (default = the sanitized session name).
3. `git` adds a worktree at `<worktreeBaseDir>/<project>/<session>` on the new
   branch off the chosen base.
4. `meta` writes `.fleet/meta.json`.
5. `tmux` creates session `fleet-<project>-<session>`, cwd = worktree, launches
   `claude`.
6. Refresh → it appears in the dashboard.

### Attach

Select a session → `Enter`. The TUI suspends and execs `tmux attach` (via
Bubble Tea `tea.ExecProcess`). On detach (`Ctrl-b d`) the user returns to the
dashboard.

### Cleanup

Select a session → `d` → menu with the three paths:

- **Delete** — kill tmux session, `git worktree remove`, optionally delete
  branch. Confirm if dirty/unpushed.
- **Push / PR** — push branch; if `gh` present, offer to open a PR.
- **Leave** — kill tmux session only; worktree + branch remain.

## Dashboard contents

Per session, at a glance:

- **Run state** — tmux session alive / process running / exited.
- **Git info** — branch, dirty/clean, ahead/behind, # uncommitted changes.
- **Project + path** — owning project, worktree path, created-at.

(Activity/attention is a future addition to this view.)

## Error handling

- Adapters return typed errors; the UI surfaces them in a status/toast line
  instead of crashing.
- Destructive ops (delete with uncommitted/unpushed changes) require
  confirmation.
- Missing `tmux` / `git` / `gh` detected at startup (or at point of use for
  `gh`) with a clear message.
- A worktree whose repo is gone, or an unparseable meta file, degrades
  gracefully — shown as a partial/unknown session, never a panic.

## Testing

- `tmux` and `git` adapters sit behind interfaces; domain logic (`session`,
  `meta`, `refresher`, `projects`) is unit-tested with fakes.
- A thin set of integration tests exercises the real `git`/`tmux` CLIs against a
  temp repo for the create → attach → cleanup happy path.
- TDD: tests come before implementation.

## MVP scope (v1)

- Config: scan a root dir for git repos → project list.
- Create session: pick project → choose base + branch → worktree + meta + tmux
  session running `claude`.
- Attach / detach.
- Kill session.
- Live dashboard: run state, git info, project + path + created-at; refresh on
  tick + manual.
- Cleanup actions in-TUI: delete / push+PR / leave.
- Concurrency: N sessions across projects, correct but not perf-optimized.

## Future features (documented, not built)

- **Activity/attention detection** — "waiting for you" vs. "working"; first
  fast-follow.
- Config-file-registered projects + multiple scan roots.
- Alternate worktree layouts (inside-repo, sibling-to-repo).
- Richer concurrency/scale work.
- Session history / archive; reopen a past session.
- Bulk actions (clean up all merged, kill all in a project).
- Embedded PTY panes as an alternative to `tmux attach`.

## Known follow-ups (from MVP code review)

Tracked, not blocking the MVP:

- **Keep-branch cleanup option.** `Manager.Delete` already takes a
  `deleteBranch bool`, but the UI hardcodes `true`. Add a UI choice to delete
  the worktree while keeping the branch.
- **`git.DefaultBranch` detached-HEAD fallback.** When `origin/HEAD` is absent
  and HEAD is detached, the fallback returns the literal `HEAD`. Guard against
  using that as a base branch (e.g. detect `main`/`master`).
- **Idempotent push/PR.** `Manager.PushPR` re-runs `gh pr create` even when a PR
  already exists; detect "already exists" and treat as a no-op rather than
  surfacing an error.
- **Wire up or remove `tmux.List` / `naming.ParseTmuxName`.** The refresher
  derives state from worktrees-on-disk (approach C), so these are currently
  unused by production paths. Keep as intentional hooks or remove.
