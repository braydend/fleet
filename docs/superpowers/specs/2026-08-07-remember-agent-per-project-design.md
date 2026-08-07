# Fleet — remember the agent used per project

**Date:** 2026-08-07
**Status:** Design — approved, pending implementation plan
**Issue:** none (feature request raised directly)

## Goal

When the new-session form opens for a project, its agent field defaults to the
**last agent used in that project**, instead of always starting on the global
`default_agent` config. One choice stays unchanged: the agent for a session is
still picked per session and fixed at creation. This feature only changes the
*default* the form starts on.

## Background

Agent selection landed as per-session choice at creation
([2026-08-07-agent-selection-design.md](2026-08-07-agent-selection-design.md)).
The form seeds its agent field from `config.default_agent`
(`newForm(p, defaultAgent)`), so a user who runs opencode in one project and
claude in another must tab over and re-pick on every single session. The only
record of "which agent did I use here" is the per-session `agent` field in each
worktree's `.fleet/meta.json` — and it is destroyed with the worktree.

## Intent (decided during brainstorming)

1. **Last agent used per project** — the form seeds the agent you last created a
   session with in that project. Most recent wins.
2. **A per-project memory file** next to the worktrees, plain-text agent ID.
3. **Written on every successful create; never deleted by cleanup.** The memory
   outlives the sessions it was learned from, because the moment you need a
   default is when you have *no* sessions left.
4. **Per-project memory beats the global `default_agent`** when both exist; the
   global default is the fallback when there is no memory yet.
5. **Always seed the remembered agent, even if its binary is gone.** The form's
   existing `(not installed)` marker and submit-time refusal handle that.
6. **Silent default.** No UI hint that the default came from history.
7. **No backfill.** Projects with pre-existing sessions but no memory file fall
   back to the global default until a new session is created.
8. **A failed memory write never fails the session.** Remembering is a
   convenience; a warning notice surfaces on the status line instead.

## Approach (chosen)

A tiny `internal/memory` package (plain-text per-project file), the write hooked
into the `Actions.Create` wrapper in `main.go`, and the read hooked into the
form-open path via a new `Actions.RememberedAgent`. `Manager` and its tests are
untouched.

### Rejected alternatives

- **Write inside `Manager.Create`.** Keeps create orchestration in one place,
  but the warning needs a new return channel off `(Session, error)` — an extra
  field on `Session` or a widened signature rippling through many manager tests.
  Too much plumbing for one best-effort write.
- **Derive from existing session metas (no file at all).** Lost the moment the
  last session for a project is cleaned up, needs a directory scan on every
  form-open, and would disagree with the "keep on cleanup" intent.
- **A central project→agent map** in a fleet state dir. Survives worktree
  cleanup, but adds a second state location and a file that can drift from the
  worktrees. YAGNI for one string per project.
- **A JSON memory file.** Consistent with `meta.json`, but more machinery for a
  single line. Plain text wins.

## Design

### New package: `internal/memory`

```go
// Package memory persists the last agent used per project, so the new-session
// form can default to it.
package memory

// Path returns the per-project memory file for project, under base.
func Path(base, project string) string

// Read returns the stored value, or "" when the file is missing or unreadable.
func Read(path string) string

// Write stores value, creating the project directory if needed.
func Write(path, value string) error
```

- **Location:** `<worktreeBaseDir>/<Sanitize(project)>/.fleet-agent`. The
  worktrees for a project already live in `<base>/<Sanitize(project)>/`, so the
  picker's `p.Name` maps to the same directory. The refresher skips
  non-directory entries under a project dir (`refresher.Build`), so the file is
  never mistaken for a session.
- **Format:** one line, the agent ID (`opencode\n`). Unrecognised or garbled
  content reads back as `""` → no memory.
- **Read** swallows errors: a missing file means "no memory", and there is
  nothing actionable in a read failure at form-open time. The call site decides
  whether a non-empty value names a known agent.
- **Write** creates the project dir (`MkdirAll`), mirroring `meta.Write`.

### Data flow

```
main.go: Actions.RememberedAgent = func(p) string {
    return memory.Read(memory.Path(cfg.WorktreeBaseDir, p.Name))
}
ui:      keyProjectPicker on enter
           seed := m.defaultAgent
           if r := m.actions.RememberedAgent(p); r != "" && agent.Known(r) {
               seed = r
           }
           m.form = newForm(p, seed)

main.go: Actions.Create = func(p, name, branch, base, agentID) (string, error) {
    if err := mgr.Create(p, name, branch, base, agent.Lookup(agentID)); err != nil {
        return "", err
    }
    if err := memory.Write(memory.Path(cfg.WorktreeBaseDir, p.Name), agentID); err != nil {
        return fmt.Sprintf("⚠ could not remember agent for %s", p.Name), nil
    }
    return "", nil
}
```

- The seed is computed from `m.defaultAgent` plus the remembered value; the
  remembered value wins only when it is non-empty **and** a known agent ID.
- `Actions.Create` gains a `notice string` return so a failed memory write
  reaches the status line without failing the session. `submitForm` threads the
  notice into `sessionsUpdatedMsg`.
- The read is synchronous: a few-byte local file read when the form opens. No
  async machinery, no flash of the wrong default.

### Interface changes

- `ui.Actions`:
  - `RememberedAgent func(p projects.Project) string` — nil-safe (nil → `""`).
  - `Create func(p projects.Project, name, branch, base, agentID string) (notice string, err error)`.
- `ui.Model.submitForm` passes the returned notice through to
  `sessionsUpdatedMsg{sessions: ss, notice: notice}`.

### No changes

- `meta.Meta` schema — the per-project file is separate state; `meta.json`
  stays per-session.
- `session.Manager` and its tests — nothing there changes.
- Dashboard and form rendering — silent default; the `(not installed)` marker
  and submit-time refusal already cover an uninstalled remembered agent.
- Config schema — `default_agent` remains the global fallback.

## Testing (TDD)

- **memory** — `Path` maps a project under a base dir to the sanitized
  `<base>/<project>/.fleet-agent`; `Read` returns the stored value, `""` for a
  missing file, and `""` for garbled content; `Write` creates the project
  directory when absent and round-trips a value.
- **ui/model** — opening the form for a project with a remembered, known agent
  seeds the form on it (beats the global default); an empty or unknown
  remembered value falls back to the global default; nil `RememberedAgent` is
  safe and falls back; `submitForm` carries the create notice to the status
  line.
- **main** — wiring only; no new tests (existing convention).

## Documentation

- `CLAUDE.md` — add `memory` to the package layout list; note in the session
  model that the new-session form defaults to the last agent used in the
  project.
- `docs/usage.md` — a sentence in the new-session flow: the agent field starts
  on the last agent used for that project, falling back to `default_agent`.

## Non-goals

- **No backfill** from pre-existing sessions.
- **No per-project file deletion on cleanup** — the memory outlives sessions.
- **No UI indication** that the default was remembered.
- **No dashboard change.**
- **No `meta.json` schema change.**
