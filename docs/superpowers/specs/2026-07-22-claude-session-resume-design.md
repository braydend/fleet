# Fleet — resume Claude Code sessions via stable session IDs

**Date:** 2026-07-22
**Status:** Design — approved, pending implementation plan
**Issue:** [#29 — Leverage Claude Code session naming](https://github.com/braydend/fleet/issues/29)

## Goal

When fleet re-launches a session's `claude` process, resume the *same*
conversation instead of starting a fresh one. This should hold both in-fleet
(a session's process exited and gets respawned) and across a fleet or machine
restart (the tmux server is gone and the window is recreated).

## Background and key constraint

Fleet launches a **bare `claude`** in each tmux window
(`internal/session/manager.go`, `claudeCommand = "claude"`; no flags). Because
tmux windows use `remain-on-exit` and the shared workspace survives fleet
restarts, the *running* Claude process persists across detach/reattach and
across fleet restarts. Conversation state is therefore only lost when fleet
**re-launches** the process, which happens in two places:

- `Manager.EnsureRunning` → `RespawnWindow` — the process exited (dead window)
  and gets respawned. Today this runs bare `claude`, starting a blank
  conversation.
- `Manager.EnsureRunning` → `CreateWindow` — the window is missing entirely
  (e.g. the first attach after a fleet/machine restart). Also bare `claude`.

The installed `claude` CLI already provides the primitives this needs:

- `--session-id <uuid>` — start a conversation with a specific (new) session ID.
- `-r, --resume [value]` — resume a conversation by session ID.
- `-n, --name <name>` — set a display name (shown in the prompt box and Claude's
  own `/resume` picker). This is the "session naming" the issue title refers to.
- `--fork-session` — resume into a *new* ID. Not used here (see Non-goals).

Because each fleet session already has a stable identity (`project` + `session`
name, a dedicated worktree, and a `.fleet/meta.json`), fleet can own a stable
Claude **session ID** per fleet-session and resume it deterministically.

## Intent (decided during brainstorming)

1. **Every re-launch resumes** — both in-fleet respawn and re-open after a
   fleet/machine restart pick up the prior conversation.
2. **Conversation-only** — "maintain settings" in the issue was loose wording
   for "resume where I left off". There is no separate settings-carryover work.
3. **Set the display name** — launch with `-n <project/session>` so the
   conversation is labeled with fleet's name.
4. **New sessions only** — sessions created before this ships have no stored ID
   and keep starting fresh on re-launch. No fallback or backfill for them.

## Approach

**Stable stored session ID.** At create time fleet generates a UUID, stores it
in the session's `.fleet/meta.json`, and launches Claude with `--session-id`.
Every subsequent re-launch resumes that exact ID. Deterministic, fleet fully
owns the identity, and it directly leverages Claude Code session naming.

**Rejected alternative — `claude --continue` in the worktree.** Each session
has a unique worktree dir, so "continue the most recent conversation here" is
usually unambiguous and needs no stored ID. But the "new sessions only"
decision removes its only advantage over stored IDs, and it is fragile: it
silently follows whatever the latest conversation in the dir happens to be, and
first-launch needs special handling. Not chosen.

## Design

### Data model

Add a `ClaudeSessionID` field to the persisted metadata and the domain model:

- `meta.Meta` gains `ClaudeSessionID string` with JSON tag
  `claude_session_id,omitempty`. Older `meta.json` files simply omit it, so it
  reads back as `""` — the legacy path.
- `session.Session` gains `ClaudeSessionID string`.
- The refresher (`internal/refresher/refresher.go`), which reconstructs
  `session.Session` from `meta.Read` on every tick and after a restart, copies
  `md.ClaudeSessionID` into the reconstructed session. This is what makes the
  ID available on the restart re-launch path.

### Launch-command construction

Replace the `claudeCommand = "claude"` const with two builders keyed on
`(id, name)`, where `name` is the human `"<project>/<session>"` string:

- **Fresh launch (used by `Manager.Create`):**
  - `id == ""` → `claude` (never happens for new sessions, but keeps the
    builder total)
  - otherwise → `claude --session-id <id> -n '<name>'`
- **Re-launch (used by `Manager.EnsureRunning`, both the respawn and the
  missing-window branches):**
  - `id == ""` → `claude` (legacy sessions)
  - otherwise →
    `claude --resume <id> || claude --session-id <id> -n '<name>' || claude`

The re-launch `||` chain self-heals:

1. `claude --resume <id>` — the normal case; succeeds silently.
2. `claude --session-id <id> -n '<name>'` — fires only if resume fails because
   the session file is missing; recreates the conversation under the *same*
   stable ID so future resumes work.
3. `claude` — last-resort bare launch so the window is always usable even if
   both of the above fail.

Using `--session-id` directly (not the chain) on the **fresh** path avoids a
spurious "no conversation found" error flashing in the pane on the very first
launch, when there is deliberately nothing to resume.

The command string is passed to tmux as a single argument and executed via the
shell (`sh -c`), so the `||` operators work as written. `<name>` is
shell-single-quoted, with any embedded single quotes escaped, so arbitrary
project/session names are safe.

### ID generation

A UUIDv4 generated with `crypto/rand` — no new dependency (the module currently
pulls in only the Charm ecosystem, YAML, and selfupdate). The generator is
injected into `Manager` as a `newID func() string` field, defaulting to the
real generator in production and overridable in tests — mirroring the existing
injectable `clock func() time.Time`.

### Data flow summary

```
Manager.Create
  ├─ newID() → uuid
  ├─ meta.Write(wt, {… ClaudeSessionID: uuid})
  └─ CreateWindow(name, wt, launchFresh(uuid, "<proj>/<sess>"))

fleet restart / tick
  └─ refresher: meta.Read(wt) → Session{… ClaudeSessionID: uuid}

Attach → Manager.EnsureRunning(s)
  ├─ window missing → CreateWindow(name, wt, launchResume(s.ClaudeSessionID, name))
  └─ window dead    → RespawnWindow(target, wt, launchResume(s.ClaudeSessionID, name))
```

## Testing (TDD)

- **Command builders** — table tests: empty id → `claude`; populated id →
  the exact `--session-id …` (fresh) and `--resume … || … || claude`
  (re-launch) strings, including correct shell-quoting of a name containing a
  space and a single quote.
- **meta** — round-trip `Write`/`Read` preserves `ClaudeSessionID`; a legacy
  `meta.json` without the field reads back as `""`.
- **Manager.Create** — stores a non-empty `ClaudeSessionID` in meta and passes
  the fresh command (with that id) to the fake tmux's `CreateWindow`.
- **Manager.EnsureRunning** — for a session *with* an id, the respawn and
  missing-window branches pass the resume chain; for a legacy session
  (`id == ""`), both pass bare `claude`.
- **refresher** — a reconstructed `Session` carries the `ClaudeSessionID` from
  its `meta.json`.

## Non-goals

- **No `--fork-session`.** Re-launch continues the same linear conversation;
  forking would create divergent histories per respawn.
- **No `--continue`.** Superseded by stored IDs (see Rejected alternative).
- **No legacy backfill.** Sessions created before this feature keep the empty
  ID and start fresh on re-launch (decision 4).
- **No settings carryover.** Conversation resume only (decision 2).

## Assumptions

- Claude keys resumable sessions by the working directory, which is the
  session's worktree and is stable for the life of the fleet session, so
  `--resume <id>` in that cwd resolves the intended conversation.
- A given Claude session ID is only ever launched in one window at a time
  (respawn happens only after the previous process exited), so there is no
  concurrent-resume contention.
