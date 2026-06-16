# Fleet — tmux-native tab switching + richer session info

**Date:** 2026-06-16
**Status:** Design — approved, pending implementation plan

## Goal

Make it fast to switch between live Claude sessions and easy to tell them
apart. Two parts:

1. **Switching:** while attached to a session, jump to another live session
   instantly with a keybind, with a persistent tab strip — like browser tabs.
2. **Information:** a cleaner dashboard that makes each session easy to
   identify and ties it to its tab, plus a per-session activity indicator
   (working / waiting / idle / exited).

## Background and key constraint

Today each fleet session is its own tmux session (`fleet-<project>-<session>`),
and attaching uses `tea.ExecProcess(tmux attach …)`, which **fully suspends the
fleet TUI** and hands the whole terminal to tmux. You return by detaching.

This rules out fleet *rendering* a tab strip over live sessions while attached —
that would require fleet to own the PTYs and embed a terminal emulator (a
VT parser + screen buffer) inside Bubble Tea, route input, reserve a prefix
key, propagate resizes, and throttle high-frequency redraws. That is, in
effect, reimplementing a terminal multiplexer inside fleet — explicitly the
thing the "approach C / tmux is authoritative" decision exists to avoid. It is
out of scope (see *Embedded panes* under Non-goals).

**Instead we lean on tmux**, which already does all of the above. The shift:

- **Before:** N separate tmux sessions, attach to one at a time.
- **After:** one shared `fleet-workspace` tmux session; each fleet session is a
  **window** in it. Switching is tmux's native `select-window` — a keybind,
  rendered by tmux's window-list status bar.

fleet's role becomes: create/destroy windows in the workspace, label them,
configure the switch keybinds and tab strip, and remain the launch/manage
dashboard.

## Two surfaces for session info

| Surface | When seen | Density | Rendered by |
|---|---|---|---|
| **Tab strip** | While attached, working | Concise | tmux status bar |
| **Dashboard** | While managing | Rich | fleet (Bubble Tea) |

### Tab strip (while attached)

Format: `<index> <glyph><project>/<session><dirty?>`, e.g.
`1 ◉web/login   2 ◉api/fix-bug✱   3 ○cli/refactor`. The active window is
highlighted. The leading number **is** the keybind.

Because tmux cannot know git/activity state, **fleet owns each window's name**
and rewrites it on each refresh tick to encode the glyph + label. The tmux
status format is just `#{window_index} #{window_name}` (with
`automatic-rename off`).

**Switch keys (prefix-less):**
- `Alt-1` … `Alt-9` → jump to that tab (`select-window -t N`)
- `Alt-Left` / `Alt-Right` → previous / next window
- existing **prefix-d** → detach back to the dashboard (unchanged)

### Dashboard (while managing)

Grouped-by-project list. Each session is two lines: identity then state. Adds a
**tab number** (matches `Alt-N`), **base branch**, and an **activity glyph**.
A **legend** for the glyphs sits in the footer.

```
fleet — 3 sessions · 2 projects

web
› 1 ◉ login     fleet/login ← main
     working · ✱3 · ↑2 · 2h ago

api
  2 ◉ fix-bug   fleet/fix-bug ← main
     waiting for input · clean · 1d ago
  3 ○ refactor  fleet/refactor ← develop
     exited · clean · 3d ago

◉ working  ◉ waiting  ◉ idle  ○ exited
n new · enter attach · d cleanup · r refresh · q quit
```

The tab number is the tmux **window index**. fleet sets the workspace's
`base-index` to 1 so indices start at 1 and line up with `Alt-1…9`.

## Activity detection

Four states, surfaced by colour + glyph on both surfaces:

| State | Glyph | Meaning | Signal | Robustness |
|---|---|---|---|---|
| working | ◉ green | produced output recently | `window_activity` timestamp | robust |
| waiting | ◉ yellow | quiet **and** a Claude input prompt is showing | `capture-pane` tail match | best-effort |
| idle | ◉ grey | quiet, nothing pending | timestamp + no prompt | robust |
| exited | ○ dim | the Claude process is gone | dead window (`pane_dead`) | robust |

Built in two tiers: **working / idle / exited** derived robustly from tmux's
window-activity timestamp and dead-pane flag; **waiting** as a clearly-isolated
best-effort layer that pattern-matches Claude's prompt in the captured pane
tail. The matcher lives in one place (`internal/activity`) so it is the single
spot to update if Claude's TUI changes. A `capture-pane` failure degrades to
*idle* and never fails the refresh.

Cost: the refresher already ticks every 2s. This adds one `list-windows` call
plus one `capture-pane` per session per tick — fine for the expected handful of
sessions.

## Components

Each unit keeps one clear purpose and stays testable in isolation.

### `internal/tmux` — window-level adapter

Shift from session-level to window-level operations against `fleet-workspace`:

- `EnsureWorkspace()` — create the detached `fleet-workspace` session if absent;
  set `base-index 1`, `automatic-rename off`, `remain-on-exit on` (so an exited
  Claude leaves a dead window we can show as ○ and respawn, rather than the
  window vanishing).
- `CreateWindow(windowName, workdir, command)` — `new-window` in the workspace.
- `KillWindow(target)` / `RespawnWindow(target, workdir, command)`.
- `ListWindows()` → `[]Window{Index int, Name string, Alive bool, Dead bool,
  LastActivity time.Time}` in a single `list-windows -F` call. Replaces the
  per-session `Has`.
- `CapturePane(target)` → last N lines (for the waiting heuristic).
- `SetWindowName(target, name)` — used each tick to push the tab label.
- `AttachWorkspaceCmd(target)` — attach to the workspace selecting a window
  (for `tea.ExecProcess`).
- `ConfigureTabs()` — window-status formats + the Alt keybinds, scoped to the
  workspace session. Best-effort.

`Decorate` is superseded by `ConfigureTabs` (the status bar now shows the tab
strip; the detach hint can move into the status-right).

### `internal/naming`

Reuse `TmuxName(project, session)` as the deterministic **window** name (the
existing sanitize + round-trip stays useful for mapping windows back to
sessions). Add a `Workspace` constant (`fleet-workspace`). The session's tmux
identity becomes the window target `fleet-workspace:<windowName>`.

### `internal/activity` (new)

Pure classifier, no I/O:

```
Classify(lastActivity, now time.Time, paneTail string, dead bool) State
```

Unit-tested with fixtures of captured pane text + timestamps, including the
waiting-prompt match. Owns the only Claude-UI-specific knowledge in the system.

### `internal/session`

`Session` gains:

- `Activity activity.State`
- `LastActivity time.Time`
- `WindowIndex int`

(`Alive`/`Exited` are retained or folded into `Activity` during
implementation — the plan decides.)

Manager changes:

- `Create` → `EnsureWorkspace` then `CreateWindow`.
- `EnsureRunning` → respawn a dead window; **also create the window if it is
  missing entirely**, so sessions created under the old per-session model join
  the workspace seamlessly on next attach.
- `Leave` → kill the window (not the whole workspace).
- `Delete` / `PushPR` → unchanged except the tmux step kills the window.
- `AttachCmd` → attach to the workspace at the session's window.

### `internal/refresher`

`Build` keeps enumerating worktrees + meta as the source of truth for *which
sessions exist*. For liveness/activity it now:

1. calls `ListWindows()` once and maps each session to its window by name,
2. runs `CapturePane` per live session and `activity.Classify`,
3. pushes the computed label back into the window name (`SetWindowName`) so the
   tab strip reflects current state.

The `liveness` interface widens to expose `ListWindows` / `CapturePane` /
`SetWindowName`. Malformed worktrees are still skipped; capture/label failures
degrade gracefully.

### `internal/ui`

- Rewrite `viewDashboard` to the grouped layout above: project headers,
  two-line rows, tab number, base branch, colored activity glyph, and a legend
  in the footer.
- `enter` attaches to the workspace at the selected session's window index.
- Add Lip Gloss styles for the four activity states.
- Cursor/selection logic adapts to the grouped (project-header) list.

## Data flow

- **Tick (2s):** refresher → scan worktrees → `ListWindows` → per session
  `CapturePane` + `Classify` → `SetWindowName` → return `[]Session` → dashboard
  re-renders. (Unchanged: the periodic refresh must not reset the active
  screen.)
- **Attach (`enter`):** `EnsureWorkspace` / respawn/create window →
  `ConfigureTabs` → `tea.ExecProcess(AttachWorkspaceCmd(index))`. While
  attached, Alt-keys switch windows entirely within tmux; fleet stays suspended
  (as today). On detach, refresh and return to the dashboard.

## Error handling

Per project conventions: surface errors in the status line, never panic on a
malformed worktree/meta. `ConfigureTabs`, `SetWindowName`, `Decorate`-style
status config, and `CapturePane` are all best-effort and must not block attach
or fail a refresh. An empty workspace (all windows gone, tmux auto-kills the
session) is recreated on the next `Create`/attach.

## Testing (TDD)

- `internal/activity` — pure unit tests over fixture pane tails + timestamps,
  including waiting-prompt detection and graceful unknown→idle.
- `internal/tmux` — integration tests gated on a real tmux binary (existing
  skip-if-absent pattern) for workspace/window create, list, kill, capture.
- `internal/refresher` — fake tmux returning window data → asserts liveness,
  index, and activity mapping; asserts capture failure degrades, not errors.
- `internal/ui` — view tests asserting grouped layout, project headers, tab
  numbers, glyphs, and the legend; attach targets the right window index.

## Migration

Existing sessions created under the old per-session model are **not** windows
in `fleet-workspace`. Their worktrees and branches are untouched. They display
as *exited* until attached; `EnsureRunning` then creates the window in the
workspace. No data loss, no manual migration step.

## Non-goals

- **Embedded PTY panes** — fleet rendering live session output in its own
  Bubble Tea screen. Requires building a terminal multiplexer inside fleet
  (PTY ownership, VT emulation, input routing, resize, render throttling);
  duplicates tmux. Deferred (matches the existing spec's future list).
- Bulk actions, session history, config-registered projects — unchanged from
  the original MVP scope.
