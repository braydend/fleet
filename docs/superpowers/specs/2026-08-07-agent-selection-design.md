# Fleet — choose the coding agent that drives a session

**Date:** 2026-08-07
**Status:** Design — approved, pending implementation plan
**Issue:** none (feature request raised directly)

## Goal

Let a session be driven by **opencode** as well as Claude Code, with the choice
made by the user when the session is created and remembered for the life of the
session. Everything else fleet does — worktrees, tmux windows, activity
classification, cleanup, PRs — must work identically whichever agent is chosen.

## Background

Fleet hardcodes Claude Code in three places:

- `internal/session/command.go` — `launchFresh` / `launchResume` build `claude …`
  command strings, which `Manager.Create` and `Manager.EnsureRunning` hand to
  tmux as the window command.
- `internal/activity/activity.go` — `promptMarkers` greps the captured pane tail
  for Claude's input prompt to distinguish `waiting` from `idle`. Its package
  doc names itself "the ONLY place that knows what Claude's prompt looks like".
- `internal/meta/meta.go` — `ClaudeSessionID` persists the stable Claude session
  ID that `--resume` reattaches to.

Nothing else in the codebase cares which process runs in the window.

### What the opencode CLI provides

From <https://opencode.ai/docs/cli/>:

- `opencode [project]` — start the TUI, optionally in a given directory.
- `-c, --continue` — continue the **last session for the current project
  directory**.
- `-s, --session <id>` — resume a specific session by ID.
- `--title` — title for the session (defaults to a truncated prompt).

The key asymmetry with Claude Code: opencode session IDs (`ses_…`) are minted by
opencode itself. There is no documented equivalent of `claude --session-id
<uuid>`, which lets fleet *choose* an ID before the process exists. Fleet
therefore cannot own an opencode session's identity the way it owns a Claude
session's, and the resume strategy has to differ per agent.

## Intent (decided during brainstorming)

1. **Per-session choice at creation.** A fourth field in the new-session form,
   not a separate screen and not config-only.
2. **A closed set of two agents in code.** No user-defined agents in
   `config.yaml`; adding a third agent is a small code change.
3. **A configurable default** so single-agent users never think about the field.
4. **Resume via `--continue` for opencode** — no ID bookkeeping.
5. **No guessed prompt markers.** opencode sessions never report `waiting`
   until someone observes its real TUI.
6. **A missing binary is a form-time error**, never a half-created session.

## Approach

### Rejected alternatives

- **Config-driven agent registry** (users declare command templates in
  `config.yaml`). Buys arbitrary agents for free, but fleet would own a
  templating mini-language, its validation, and its failure reporting, for a
  feature nobody has asked for. YAGNI.
- **Capturing opencode's real session ID after launch** (`opencode session
  list`, parse, store in meta, resume with `--session <id>`). Exact, but adds a
  subprocess, JSON parsing, and a race — the ID does not exist until opencode
  has started — and buys nothing over `--continue` given that every fleet
  session owns a unique worktree.
- **An agent-picker screen between the project picker and the form.** More
  prominent, but adds a screen to every single session creation.

### Why `--continue` is right here, when it was rejected for Claude

[The session-resume spec](2026-07-22-claude-session-resume-design.md) rejected
`claude --continue` in favour of stored IDs, on the grounds that `--continue`
"silently follows whatever the latest conversation in the dir happens to be".
That objection is weaker for opencode and, more importantly, the alternative
does not exist:

- opencode will not accept a caller-minted ID, so the stored-ID approach is only
  reachable via the rejected capture-after-launch dance.
- The ambiguity `--continue` was faulted for is bounded by fleet's own design:
  a session's worktree is created by fleet, used by exactly one agent process at
  a time, and destroyed with the session. The "latest conversation in this
  directory" and "this fleet session's conversation" cannot diverge unless the
  user runs a second opencode in the same worktree by hand.

## Design

### New package: `internal/agent`

The single place that knows anything agent-specific.

```go
// Agent describes one coding agent fleet can run in a session window.
type Agent struct {
    ID      string   // "claude" | "opencode" — persisted in meta
    Label   string   // display name in the form and dashboard
    Bin     string   // binary looked up on PATH
    NeedsID bool     // fleet mints and stores a stable session ID for it
    Fresh   func(id, name string) string  // first-launch command
    Resume  func(id, name string) string  // re-launch command
    Markers []string // pane-tail substrings meaning "waiting for input"
}

func All() []Agent              // registry order = form cycle order
func Lookup(id string) Agent    // "" or unrecognised → claude
func (a Agent) Available() bool // binary found on PATH
```

The registry:

| | claude | opencode |
|---|---|---|
| `Label` | `claude` | `opencode` |
| `Bin` | `claude` | `opencode` |
| `NeedsID` | `true` | `false` |
| `Fresh(id, name)` | `claude --session-id <id> -n '<name>'` | `opencode` |
| `Resume(id, name)` | `claude --resume <id> \|\| claude --session-id <id> -n '<name>' \|\| claude` | `opencode --continue \|\| opencode` |
| `Markers` | `❯ 1.`, `Do you want`, `(y/n)` | *(empty)* |

Claude's two builders are the existing `launchFresh` / `launchResume` from
`internal/session/command.go`, moved verbatim along with `shellQuote`; their
`id == ""` legacy branches are preserved. `internal/session/command.go` and its
test go away.

opencode's builders ignore both arguments. `--continue` falls back to a bare
`opencode` via `||` so a first launch — or one where opencode has no session for
this directory — still lands in a usable window, matching the self-healing shape
of the Claude chain.

`Lookup` is total: it never returns an error and never returns a zero `Agent`.
An unrecognised ID in a `meta.json` (a downgrade, a hand-edit, a future agent)
degrades to claude rather than breaking the dashboard for every session.

`Available` wraps `exec.LookPath(a.Bin)`. The lookup function is a package-level
variable so tests can substitute a fake rather than depending on what is
installed on the machine running them.

### Data model

- `meta.Meta` gains `Agent string` with JSON tag `agent,omitempty`. Existing
  `meta.json` files have no `agent` key and read back as `""` → claude, which is
  exactly what they are.
- `session.Session` gains `Agent string`.
- `refresher.Build` copies `md.Agent` into the reconstructed session, which is
  what makes the agent available on the restart re-launch path.
- `ClaudeSessionID` keeps its name and JSON tag. Renaming it to a neutral
  `session_id` would need a back-compat read of the old key for no behavioural
  gain. It is written only when `NeedsID` is true, so opencode sessions carry an
  empty ID — the same shape as a pre-resume legacy session.

### Launch paths

`Manager.Create` takes one new parameter, `ag agent.Agent`, appended to the
existing `(p, name, branch, base)`. A typed parameter rather than a fifth
adjacent string, so there is nothing to transpose at a call site.

```
Manager.Create(p, name, branch, base, ag)
  ├─ id := ""; if ag.NeedsID { id = newID() }
  ├─ meta.Write(wt, {… Agent: ag.ID, ClaudeSessionID: id})
  └─ CreateWindow(wname, wt, ag.Fresh(id, "<proj>/<sess>"))

Manager.EnsureRunning(s)
  ├─ ag := agent.Lookup(s.Agent)
  ├─ window missing → CreateWindow(s.TmuxName, wt, ag.Resume(s.ClaudeSessionID, name))
  └─ window dead    → RespawnWindow(target, wt, ag.Resume(s.ClaudeSessionID, name))
```

`ui.Actions.Create` grows an `agentID string` parameter rather than an
`agent.Agent`, keeping the action boundary to plain data; `main.go` maps it
through `agent.Lookup` before calling the manager.

### Activity classification

`activity.Classify` gains a `markers []string` parameter and the package-level
`promptMarkers` var is deleted; the package doc comment loses its claim to own
Claude's prompt strings, which now live in the registry. `refresher.Build`
passes `agent.Lookup(md.Agent).Markers`.

With an empty marker list, `Classify` can only return `working`, `idle`, or
`exited` — an opencode session never claims to be waiting for input. This is a
deliberate gap, recorded in the plan as a follow-up: filling it needs someone to
watch a live opencode session and read its real permission prompt.

### Configuration

`config.Config` gains `DefaultAgent string` (`default_agent`), defaulting to
`claude` in `config.Default()`. `Config.Validate` rejects an ID that is not in
the registry, naming the valid IDs, so a typo fails loudly at startup instead of
silently launching the wrong agent on every new session.

### New-session form

- `fieldAgent` is added after `fieldBase`; `fieldCount` becomes 4.
- The field holds an index into `agent.All()`, seeded from `cfg.DefaultAgent`.
  `ui.New` gains a `defaultAgent string` parameter to carry it; `newForm` seeds
  each form from it.
- **←/→ cycle** the selection, wrapping in both directions. Arrow keys currently
  do nothing in the form (the default branch requires `msg.Runes`), so nothing
  is displaced. Typed runes and backspace are ignored on this field — it is an
  enum, not a text input.
- `tab` / `shift+tab` wrap through all four fields as they do today. `enter`
  advances while `field < fieldAgent` and submits on `fieldAgent`, so the submit
  key still lives on the last field.
- Availability is computed once when the form opens (a `LookPath` per agent) and
  cached on the form. An unavailable agent renders as `opencode (not installed)`.
- Submitting with an unavailable agent selected does **not** create anything: the
  form stays open showing
  `⚠ opencode is not on PATH — install it or pick another agent`. The message
  belongs to the form (a `submitError` field rendered in `warnStyle` beside the
  existing `fetchWarning`), **not** the dashboard status line — `View` renders
  `m.form.view()` alone for `stateNewSession`, so a status-line error would be
  invisible while the form is up. Every other failure mode here would leave a
  real worktree and branch behind for what is usually a typo-level mistake.

```
✨ new session — fleet ✨

› session:  add-oauth
  branch:   add-oauth
           new branch from main
  base:     main
  agent:    ‹ claude ›

tab next · ←/→ change agent · enter submit on last field · esc cancel
```

### Dashboard

The per-session detail line gains the agent label after the activity word:

```
📂 fleet
  1 ◉ add-oauth    add-oauth ← main
    working · claude · ✱1 · created 2026-08-07 09:12
  2 ◉ refactor-db  refactor-db ← main
    idle · opencode · clean · created 2026-08-07 10:40
```

The tmux tab label is unchanged. It already carries a coloured glyph, the
project, the session, and a dirty count; the agent is one fact too many for a
tab, and it is always visible on the dashboard.

## Testing (TDD)

- **agent** — `Lookup("")`, `Lookup("claude")`, `Lookup("opencode")`, and
  `Lookup("nonsense")` return the expected agents, with unknown and empty both
  falling back to claude; `Fresh`/`Resume` produce the exact command strings for
  both agents, including shell-quoting of a name containing a space and a single
  quote (moved from the existing `command_test.go`); `Available` is true and
  false against a faked `LookPath`.
- **meta** — `Write`/`Read` round-trips `Agent`; a legacy `meta.json` with no
  `agent` key reads back as `""`.
- **session.Manager** — `Create` with claude is unchanged (stores a non-empty
  ID, launches the `--session-id` command); `Create` with opencode stores
  `agent: "opencode"`, leaves `ClaudeSessionID` empty, and launches `opencode`;
  `EnsureRunning` passes the right resume chain per agent on both the
  missing-window and dead-window branches; a session whose meta names an unknown
  agent resumes as claude.
- **config** — `default_agent` defaults to `claude`, is overridable from the
  file, and an unrecognised value fails `Validate` with a message naming the
  valid IDs.
- **ui/newsession** — the form seeds from the configured default; ←/→ cycle and
  wrap; the label shows `(not installed)` for an unavailable agent; `enter` on
  `fieldAgent` submits while earlier fields advance.
- **ui/model** — submitting passes the selected agent ID through to
  `Actions.Create`; submitting an unavailable agent leaves the form open with
  its error rendered and calls `Create` zero times.
- **refresher** — a reconstructed session carries `Agent` from its `meta.json`,
  and a legacy meta yields `""`; an opencode session whose pane tail contains
  Claude's markers still classifies as `idle`, not `waiting`.
- **activity** — `Classify` matches supplied markers and, with an empty marker
  list, never returns `Waiting`.

## Documentation

- `CLAUDE.md` — add `agent` to the package layout list, and note in the session
  model that the driving agent is chosen per session.
- `docs/usage.md` — document `default_agent` in the config reference and the
  agent field in the new-session flow.
- **`AGENTS.md` → `CLAUDE.md` symlink** at the repo root, so an opencode session
  developing fleet picks up the same project instructions Claude Code does.
  opencode already falls back to `CLAUDE.md` when no `AGENTS.md` exists, so this
  changes nothing for opencode specifically — it makes the intent explicit and
  covers every other tool that reads the `AGENTS.md` convention without a
  Claude-Code fallback. A symlink rather than a copy so the two can never drift;
  git stores it as a symlink, and fleet already requires a Unix environment
  (tmux, `git`), so the Windows-checkout caveat does not apply. `CLAUDE.md`'s
  Maintenance section records that `CLAUDE.md` is the file to edit.

## Non-goals

- **No per-agent model or flag selection** (`--model`, `--agent`, `--title`).
- **No changing an existing session's agent.** It is fixed at creation; delete
  and recreate to switch.
- **No user-defined agents in config.** Rejected above.
- **No opencode waiting-state detection.** Deliberately deferred until its real
  prompt text has been observed.
- **No backfill.** Sessions created before this ships have no `agent` key and
  keep running Claude, which is what they are already running.

## Assumptions

- opencode scopes its sessions by project directory, so `--continue` inside a
  fleet worktree resolves that session's conversation and no other.
- `opencode --continue` in a directory with no prior session either starts a new
  one or exits non-zero; either way the `|| opencode` fallback leaves a usable
  window.
- opencode, like Claude, is a long-running foreground TUI process, so tmux
  window liveness and last-activity timestamps classify it the same way.
- The `|| opencode` fallback is a known, accepted risk, not a neutral one:
  because it is position-pinned rather than ID-pinned, it cannot distinguish
  "no prior session" from `--continue` failing for any other reason (a crash,
  a version that doesn't know the flag, a corrupt session store). Any of those
  causes it to silently start a fresh, empty session in the worktree, which
  then becomes "the last session here" and shadows the real conversation on
  every subsequent resume — with nothing surfaced to the user. This is judged
  acceptable because each fleet worktree is used by one opencode process at a
  time and the failure modes that trigger it are rare, not because it is safe.
