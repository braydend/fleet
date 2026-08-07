# Per-session agent selection (claude | opencode) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-08-07-agent-selection-design.md`](../specs/2026-08-07-agent-selection-design.md)

**Goal:** Let the user pick which coding agent (Claude Code or opencode) drives a
session, at creation time, and have every later re-launch use that agent.

**Architecture:** A new `internal/agent` package owns everything agent-specific:
the launch/resume command strings, the binary name, whether fleet mints a
session ID, and the pane-tail markers that mean "waiting for input". Every other
package takes the agent as data — `meta` persists its ID, `session.Manager` asks
it for a command, `refresher` asks it for markers, `ui` lists it in a form field
and on the dashboard. `agent.Lookup` is total, so a session created before this
ships (no `agent` key in its meta) resolves to Claude Code exactly as it behaves
today.

**Tech Stack:** Go, Bubble Tea/Lip Gloss (TUI), tmux + git CLIs, `gopkg.in/yaml.v3`.

## Global Constraints

- **No new module dependencies.** Everything here uses the standard library.
- **TDD.** Write the failing test, watch it fail, then implement. Every task
  below is ordered that way; do not reorder.
- **Conventional Commits**, one per task, as specified in each task's commit step.
- **Format/vet/lint must stay clean:** `gofmt -l .` prints nothing,
  `go vet ./...` passes, and
  `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags=smoke ./...`
  passes. Run the linter at least once before the final task.
- **Agent IDs are a persisted format.** The strings `"claude"` and `"opencode"`
  are written into `.fleet/meta.json`; never rename them.
- **Comment style:** explain *why*, in full sentences, matching the surrounding
  code. Do not add comments that restate the code.
- Full suite for any task: `go test -race ./...`.

---

## File Structure

**Created:**
- `internal/agent/agent.go` — the `Agent` type, the two-entry registry, `Lookup`,
  `Known`, `IDs`, `Available`.
- `internal/agent/command.go` — the launch/resume command builders for both
  agents plus `shellQuote` (moved from `internal/session/command.go`).
- `internal/agent/agent_test.go`, `internal/agent/command_test.go`.

**Deleted:**
- `internal/session/command.go`, `internal/session/command_test.go` — their
  contents move into `internal/agent`.

**Modified:**
- `internal/meta/meta.go` — `Agent` field.
- `internal/session/session.go` — `Agent` field.
- `internal/session/manager.go` — `Create` takes an `agent.Agent`;
  `EnsureRunning` looks one up.
- `internal/activity/activity.go` — `Classify` takes markers; the hardcoded
  `promptMarkers` var is deleted.
- `internal/refresher/refresher.go` — resolves the agent from meta, passes its
  markers to `Classify`, copies the ID onto the session.
- `internal/config/config.go` — `DefaultAgent`.
- `internal/ui/newsession.go` — the `agent` form field.
- `internal/ui/model.go` — `Actions.Create` signature, `New` signature, ←/→ key
  handling, submit gate.
- `internal/ui/views.go` — the agent label on the dashboard detail line.
- `main.go` — wires `cfg.DefaultAgent` into `ui.New` and `agent.Lookup` into the
  `Create` action.
- `CLAUDE.md`, `docs/usage.md` — documentation.

---

### Task 1: The `internal/agent` package

**Files:**
- Create: `internal/agent/agent.go`
- Create: `internal/agent/command.go`
- Create: `internal/agent/agent_test.go`
- Create: `internal/agent/command_test.go`

**Interfaces:**
- Consumes: nothing (leaf package, standard library only).
- Produces:
  - `type Agent struct { ID, Label, Bin string; NeedsID bool; Fresh, Resume func(id, name string) string; Markers []string }`
  - `const IDClaude = "claude"`, `const IDOpencode = "opencode"`
  - `func All() []Agent`
  - `func Lookup(id string) Agent` — total; unknown/empty → the Claude entry
  - `func Known(id string) bool`
  - `func IDs() []string`
  - `func (a Agent) Available() bool`

- [ ] **Step 1: Write the failing registry test**

Create `internal/agent/agent_test.go`:

```go
package agent

import (
	"errors"
	"testing"
)

func TestLookupResolvesKnownAgents(t *testing.T) {
	if got := Lookup(IDClaude); got.ID != IDClaude || !got.NeedsID {
		t.Fatalf("Lookup(%q) = %+v, want the claude entry with NeedsID", IDClaude, got)
	}
	if got := Lookup(IDOpencode); got.ID != IDOpencode || got.NeedsID {
		t.Fatalf("Lookup(%q) = %+v, want the opencode entry without NeedsID", IDOpencode, got)
	}
}

// An empty ID is a session created before agents were selectable, and an
// unrecognised one is a downgrade or a hand-edited meta.json. Both must keep
// working rather than breaking every session on the dashboard.
func TestLookupFallsBackToClaude(t *testing.T) {
	for _, id := range []string{"", "nonsense", "CLAUDE"} {
		if got := Lookup(id); got.ID != IDClaude {
			t.Errorf("Lookup(%q).ID = %q, want %q", id, got.ID, IDClaude)
		}
	}
}

func TestAllIsOrderedClaudeFirst(t *testing.T) {
	all := All()
	if len(all) != 2 || all[0].ID != IDClaude || all[1].ID != IDOpencode {
		t.Fatalf("All() = %+v, want [claude opencode]", all)
	}
}

// opencode's real prompt text has not been observed, so it ships with no
// markers: its sessions report working/idle/exited but never "waiting".
func TestMarkers(t *testing.T) {
	if len(Lookup(IDClaude).Markers) == 0 {
		t.Error("claude must carry its prompt markers")
	}
	if len(Lookup(IDOpencode).Markers) != 0 {
		t.Error("opencode must ship with no markers until its prompt is observed")
	}
}

func TestKnownAndIDs(t *testing.T) {
	if !Known(IDClaude) || !Known(IDOpencode) {
		t.Error("both registered agents must be Known")
	}
	if Known("") || Known("nonsense") {
		t.Error("unregistered IDs must not be Known")
	}
	if got := IDs(); len(got) != 2 || got[0] != IDClaude || got[1] != IDOpencode {
		t.Fatalf("IDs() = %v, want [claude opencode]", got)
	}
}

func TestAvailableUsesLookPath(t *testing.T) {
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })

	lookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	if !Lookup(IDOpencode).Available() {
		t.Error("expected Available when the binary is on PATH")
	}

	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if Lookup(IDOpencode).Available() {
		t.Error("expected unavailable when the binary is not on PATH")
	}
}
```

- [ ] **Step 2: Write the failing command-builder test**

Create `internal/agent/command_test.go`. The claude cases are moved verbatim
from `internal/session/command_test.go` (which Task 2 deletes):

```go
package agent

import "testing"

func TestClaudeFresh(t *testing.T) {
	tests := []struct {
		desc, id, name, want string
	}{
		{"legacy empty id", "", "p/s", "claude"},
		{"id and name", "abc-123", "My App/fix bug",
			`claude --session-id abc-123 -n 'My App/fix bug'`},
		{"name with single quote", "abc-123", "o'brien/x",
			`claude --session-id abc-123 -n 'o'\''brien/x'`},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := claudeFresh(tt.id, tt.name); got != tt.want {
				t.Fatalf("claudeFresh(%q,%q) = %q, want %q", tt.id, tt.name, got, tt.want)
			}
		})
	}
}

func TestClaudeResume(t *testing.T) {
	tests := []struct {
		desc, id, name, want string
	}{
		{"legacy empty id", "", "p/s", "claude"},
		{"id and name", "abc-123", "My App/fix bug",
			`claude --resume abc-123 || claude --session-id abc-123 -n 'My App/fix bug' || claude`},
		{"name with single quote", "abc-123", "o'brien/x",
			`claude --resume abc-123 || claude --session-id abc-123 -n 'o'\''brien/x' || claude`},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := claudeResume(tt.id, tt.name); got != tt.want {
				t.Fatalf("claudeResume(%q,%q) = %q, want %q", tt.id, tt.name, got, tt.want)
			}
		})
	}
}

// opencode mints its own session IDs, so fleet passes it nothing and resumes by
// continuing the last session for the worktree.
func TestOpencodeCommandsIgnoreIDAndName(t *testing.T) {
	if got := opencodeFresh("ignored", "p/s"); got != "opencode" {
		t.Fatalf("opencodeFresh = %q, want %q", got, "opencode")
	}
	want := "opencode --continue || opencode"
	if got := opencodeResume("ignored", "p/s"); got != want {
		t.Fatalf("opencodeResume = %q, want %q", got, want)
	}
}

// The registry must expose the same builders the tests above cover.
func TestRegistryWiresBuilders(t *testing.T) {
	if got := Lookup(IDClaude).Fresh("sid", "p/s"); got != `claude --session-id sid -n 'p/s'` {
		t.Fatalf("claude Fresh = %q", got)
	}
	if got := Lookup(IDOpencode).Resume("", "p/s"); got != "opencode --continue || opencode" {
		t.Fatalf("opencode Resume = %q", got)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/agent/`
Expected: FAIL — the package does not compile (`undefined: Lookup`, `undefined: claudeFresh`, …).

- [ ] **Step 4: Write `internal/agent/agent.go`**

```go
// Package agent describes the coding agents fleet can run in a session window.
// It is the single place that knows anything agent-specific: how to launch and
// resume one, what its binary is called, whether fleet owns its session ID, and
// what its "waiting for input" prompt looks like.
package agent

import "os/exec"

// Agent IDs. These strings are persisted in .fleet/meta.json, so renaming one
// would orphan every session created with it.
const (
	IDClaude   = "claude"
	IDOpencode = "opencode"
)

// Agent describes one coding agent fleet can run in a session window.
type Agent struct {
	ID      string // persisted in meta
	Label   string // shown in the new-session form and on the dashboard
	Bin     string // binary looked up on PATH
	NeedsID bool   // fleet mints and stores a stable session ID for this agent

	// Fresh and Resume build the shell command tmux runs in the window, for a
	// first launch and a re-launch respectively. Agents that mint their own
	// session IDs ignore both arguments.
	Fresh  func(id, name string) string
	Resume func(id, name string) string

	// Markers are pane-tail substrings that mean the agent is waiting for input.
	Markers []string
}

// lookPath is exec.LookPath, indirected so tests can decide what is installed
// rather than depending on the machine running them.
var lookPath = exec.LookPath

// All returns the known agents. The order is the order the new-session form
// cycles through, and the first entry is the fallback for an unknown ID.
func All() []Agent {
	return []Agent{
		{
			ID: IDClaude, Label: "claude", Bin: "claude", NeedsID: true,
			Fresh: claudeFresh, Resume: claudeResume,
			// Best-effort and intentionally centralized; update here if the
			// Claude Code TUI changes.
			Markers: []string{"❯ 1.", "Do you want", "(y/n)"},
		},
		{
			ID: IDOpencode, Label: "opencode", Bin: "opencode", NeedsID: false,
			Fresh: opencodeFresh, Resume: opencodeResume,
			// opencode's prompt text has not been observed yet. An empty list
			// means its sessions never classify as "waiting", which is better
			// than guessed strings producing false "waiting" badges.
			Markers: nil,
		},
	}
}

// Lookup resolves a persisted agent ID. It is total: an empty ID (a session
// created before agents were selectable) or an unrecognised one degrades to
// Claude Code rather than breaking every session on the dashboard.
func Lookup(id string) Agent {
	all := All()
	for _, a := range all {
		if a.ID == id {
			return a
		}
	}
	return all[0]
}

// Known reports whether id names a registered agent. Unlike Lookup it does not
// fall back, so configuration can reject a typo instead of silently launching
// something the user did not ask for.
func Known(id string) bool {
	for _, a := range All() {
		if a.ID == id {
			return true
		}
	}
	return false
}

// IDs returns the registered agent IDs in registry order, for error messages.
func IDs() []string {
	all := All()
	out := make([]string, 0, len(all))
	for _, a := range all {
		out = append(out, a.ID)
	}
	return out
}

// Available reports whether the agent's binary is on PATH.
func (a Agent) Available() bool {
	_, err := lookPath(a.Bin)
	return err == nil
}
```

- [ ] **Step 5: Write `internal/agent/command.go`**

The three claude functions are moved unchanged from
`internal/session/command.go`, renamed from `launchFresh`/`launchResume`:

```go
package agent

import (
	"fmt"
	"strings"
)

// claudeFresh builds the command for a brand-new session: create the
// conversation under a specific session ID and give it a display name. Using
// --session-id directly (not the resume chain) avoids a spurious "no
// conversation found" error flashing in the pane on first launch. A legacy
// session (empty id) launches bare claude.
func claudeFresh(id, name string) string {
	if id == "" {
		return "claude"
	}
	return fmt.Sprintf("claude --session-id %s -n %s", id, shellQuote(name))
}

// claudeResume builds the command for re-launching an existing session. It
// resumes the stored ID; if the session file is gone it recreates it under the
// same stable ID; and it finally falls back to a bare claude so the window is
// always usable. A legacy session (empty id) launches bare claude.
func claudeResume(id, name string) string {
	if id == "" {
		return "claude"
	}
	return fmt.Sprintf(
		"claude --resume %s || claude --session-id %s -n %s || claude",
		id, id, shellQuote(name),
	)
}

// opencodeFresh launches opencode in the session's worktree. opencode mints its
// own session IDs, so there is nothing for fleet to pass.
func opencodeFresh(_, _ string) string { return "opencode" }

// opencodeResume continues the last opencode session for the working directory.
// Every fleet session owns a unique worktree, so "the last session here" can
// only be this session's conversation. The fallback keeps the window usable
// when there is nothing to continue — a first launch, or a session opencode has
// forgotten.
func opencodeResume(_, _ string) string { return "opencode --continue || opencode" }

// shellQuote wraps s in single quotes safe for `sh -c` (tmux runs the window
// command through the shell), escaping any embedded single quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/agent/`
Expected: PASS (all of `TestLookup*`, `TestAll*`, `TestMarkers`, `TestKnownAndIDs`,
`TestAvailableUsesLookPath`, `TestClaude*`, `TestOpencode*`, `TestRegistryWiresBuilders`).

- [ ] **Step 7: Commit**

```bash
gofmt -l internal/agent
git add internal/agent
git commit -m "feat(agent): add registry of runnable coding agents

Introduces internal/agent as the single place that knows agent-specific
details: launch and resume commands, binary name, whether fleet owns the
session ID, and waiting-for-input markers. Lookup is total so an unknown
or absent ID degrades to Claude Code."
```

---

### Task 2: Persist the agent and launch it

**Files:**
- Modify: `internal/meta/meta.go:13-22` (the `Meta` struct)
- Modify: `internal/session/session.go:12-31` (the `Session` struct)
- Modify: `internal/session/manager.go:64-94` (`Create`), `:130-144` (`EnsureRunning`)
- Delete: `internal/session/command.go`, `internal/session/command_test.go`
- Test: `internal/meta/meta_test.go`, `internal/session/manager_test.go`

**Interfaces:**
- Consumes: `agent.Agent`, `agent.Lookup`, `agent.IDClaude`, `agent.IDOpencode` (Task 1).
- Produces:
  - `meta.Meta.Agent string` with JSON tag `agent,omitempty`
  - `session.Session.Agent string`
  - `func (m *Manager) Create(p projects.Project, name, branch, base string, ag agent.Agent) (Session, error)`

- [ ] **Step 1: Write the failing meta test**

Append to `internal/meta/meta_test.go`:

```go
func TestWriteReadRoundTripsAgent(t *testing.T) {
	wt := t.TempDir()
	if err := Write(wt, Meta{Project: "p", Session: "s", Agent: "opencode"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Read(wt)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Agent != "opencode" {
		t.Fatalf("Agent = %q, want %q", got.Agent, "opencode")
	}
}

// Sessions created before agents were selectable have no "agent" key. They must
// read back as empty, which callers resolve to Claude Code.
func TestReadLegacyMetaHasNoAgent(t *testing.T) {
	wt := t.TempDir()
	body := []byte(`{"project":"p","session":"s","branch":"b","base":"main"}`)
	if err := writeRaw(wt, body); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Read(wt)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Agent != "" {
		t.Fatalf("legacy Agent = %q, want empty", got.Agent)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/meta/`
Expected: FAIL — `unknown field Agent in struct literal of type Meta`.

- [ ] **Step 3: Add the meta field**

In `internal/meta/meta.go`, add to the `Meta` struct after `ClaudeSessionID`:

```go
	// Agent is the ID of the coding agent driving this session (see
	// internal/agent). Absent in sessions created before agents were
	// selectable; agent.Lookup resolves the empty string to Claude Code.
	Agent string `json:"agent,omitempty"`
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/meta/`
Expected: PASS

- [ ] **Step 5: Write the failing manager tests**

In `internal/session/manager_test.go`, add the import
`"github.com/bray/fleet/internal/agent"`, then update the three existing
`m.Create(...)` call sites and add the new tests.

Existing call sites gain a final argument (`agent.Lookup(agent.IDClaude)`):

```go
	s, err := m.Create(proj, "fix-bug", "fleet/fix-bug", "main", agent.Lookup(agent.IDClaude))
```

...in `TestCreateAddsWorktreeMetaAndTmux`, and likewise in
`TestCreateUsesExistingLocalBranch`, `TestCreateTracksRemoteBranch`,
`TestCreateNewBranchWhenNeitherExists`, and
`TestCreateExistingBranchCheckedOutElsewhere`.

Then add:

```go
func TestCreateWithClaudeStoresAgentID(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}

	s, err := m.Create(proj, "sess", "b", "main", agent.Lookup(agent.IDClaude))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	md, err := meta.Read(s.WorktreePath)
	if err != nil {
		t.Fatalf("meta read: %v", err)
	}
	if md.Agent != agent.IDClaude || s.Agent != agent.IDClaude {
		t.Fatalf("agent = meta %q session %q, want %q", md.Agent, s.Agent, agent.IDClaude)
	}
	if md.ClaudeSessionID != "test-session-id" {
		t.Fatalf("claude sessions must store a minted ID, got %q", md.ClaudeSessionID)
	}
}

// opencode mints its own session IDs, so fleet stores none and launches it bare.
func TestCreateWithOpencodeStoresNoSessionID(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}

	s, err := m.Create(proj, "sess", "b", "main", agent.Lookup(agent.IDOpencode))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	md, err := meta.Read(s.WorktreePath)
	if err != nil {
		t.Fatalf("meta read: %v", err)
	}
	if md.Agent != agent.IDOpencode || s.Agent != agent.IDOpencode {
		t.Fatalf("agent = meta %q session %q, want %q", md.Agent, s.Agent, agent.IDOpencode)
	}
	if md.ClaudeSessionID != "" || s.ClaudeSessionID != "" {
		t.Fatalf("opencode must store no session ID, got meta %q session %q",
			md.ClaudeSessionID, s.ClaudeSessionID)
	}
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != "opencode" {
		t.Fatalf("create launch cmd = %v, want [opencode]", ft.createdCmds)
	}
}

func TestEnsureRunningUsesOpencodeResumeChain(t *testing.T) {
	ft := &fakeTmux{windows: map[string]tmux.Window{"fleet-p-s": {Index: 1, Name: "fleet-p-s", Dead: true}}}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", Agent: agent.IDOpencode}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	want := "opencode --continue || opencode"
	if len(ft.respawnedCmds) != 1 || ft.respawnedCmds[0] != want {
		t.Fatalf("respawn cmd = %v, want %q", ft.respawnedCmds, want)
	}
}

// A meta naming an agent this build does not know must still produce a usable
// window rather than an empty command.
func TestEnsureRunningUnknownAgentFallsBackToClaude(t *testing.T) {
	ft := &fakeTmux{}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt",
		Agent: "from-the-future", ClaudeSessionID: "sid-1"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	want := "claude --resume sid-1 || claude --session-id sid-1 -n 'p/s' || claude"
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != want {
		t.Fatalf("cmd = %v, want %q", ft.createdCmds, want)
	}
}
```

- [ ] **Step 6: Run them to verify they fail**

Run: `go test ./internal/session/`
Expected: FAIL — `too many arguments in call to m.Create` and `unknown field Agent`.

- [ ] **Step 7: Add the session field**

In `internal/session/session.go`, add to the `Session` struct after
`ClaudeSessionID`:

```go
	// Agent is the ID of the coding agent driving this session (see
	// internal/agent). Empty for sessions created before agents were
	// selectable, which agent.Lookup resolves to Claude Code.
	Agent string
```

- [ ] **Step 8: Rewire the manager**

In `internal/session/manager.go`, add `"github.com/bray/fleet/internal/agent"` to
the imports, then change `Create` and `EnsureRunning`:

```go
// Create makes the worktree, writes meta, and launches the session's window in
// the shared workspace under the chosen agent.
func (m *Manager) Create(p projects.Project, name, branch, base string, ag agent.Agent) (Session, error) {
	wt := naming.WorktreePath(m.cfg.WorktreeBaseDir, p.Name, name)
	if err := m.addWorktreeForBranch(p.Path, wt, branch, base); err != nil {
		return Session{}, err
	}
	// Keep fleet's own .fleet/ bookkeeping out of git status and out of the
	// user's commits.
	if err := m.git.Ignore(wt, ".fleet/"); err != nil {
		return Session{}, err
	}
	now := m.clock()
	// Only agents whose session identity fleet owns get a minted ID; the rest
	// mint their own and are resumed by other means.
	sessionID := ""
	if ag.NeedsID {
		sessionID = m.newID()
	}
	md := meta.Meta{
		Project: p.Name, Session: name, Branch: branch, Base: base,
		RepoPath: p.Path, CreatedAt: now, ClaudeSessionID: sessionID,
		Agent: ag.ID,
	}
	if err := meta.Write(wt, md); err != nil {
		return Session{}, err
	}
	wname := naming.TmuxName(p.Name, name)
	idx, err := m.tmux.CreateWindow(wname, wt, ag.Fresh(sessionID, p.Name+"/"+name))
	if err != nil {
		return Session{}, err
	}
	return Session{
		Project: p.Name, Name: name, Branch: branch, Base: base,
		RepoPath: p.Path, WorktreePath: wt, TmuxName: wname,
		CreatedAt: now, Alive: true, WindowIndex: idx,
		ClaudeSessionID: sessionID, Agent: ag.ID,
	}, nil
}
```

```go
func (m *Manager) EnsureRunning(s Session) error {
	ag := agent.Lookup(s.Agent)
	cmd := ag.Resume(s.ClaudeSessionID, s.Project+"/"+s.Name)
	w, ok := m.tmux.LookupWindow(s.TmuxName)
	if !ok {
		_, err := m.tmux.CreateWindow(s.TmuxName, s.WorktreePath, cmd)
		return err
	}
	if w.Dead {
		return m.tmux.RespawnWindow(naming.WindowTarget(s.Project, s.Name), s.WorktreePath, cmd)
	}
	return nil
}
```

- [ ] **Step 9: Delete the superseded command files**

```bash
git rm internal/session/command.go internal/session/command_test.go
```

- [ ] **Step 10: Keep `main.go` compiling**

`Create` now needs a fifth argument and `main.go` has no agent to pass until
Task 6 builds the form field. Give it the current behaviour explicitly so every
commit on this branch builds and can be bisected. Add
`"github.com/bray/fleet/internal/agent"` to `main.go`'s imports and change the
`Create` action (`main.go:102-105`):

```go
		Create: func(p projects.Project, name, branch, base string) error {
			// Task 6 replaces this with the agent chosen in the form.
			_, err := mgr.Create(p, name, branch, base, agent.Lookup(agent.IDClaude))
			return err
		},
```

- [ ] **Step 11: Run the package tests to verify they pass**

Run: `go build ./... && go test -race ./internal/session/ ./internal/meta/`
Expected: PASS

- [ ] **Step 12: Commit**

```bash
gofmt -l .
git add -A internal/meta internal/session main.go
git commit -m "feat(session): launch and resume the session's chosen agent

Persists the agent ID in .fleet/meta.json, mints a session ID only for
agents whose identity fleet owns, and asks the agent for its launch and
resume commands. The claude builders move to internal/agent."
```

---

### Task 3: Per-agent waiting-for-input detection

**Files:**
- Modify: `internal/activity/activity.go:28-51`
- Modify: `internal/refresher/refresher.go:59-110`
- Test: `internal/activity/activity_test.go`, `internal/refresher/refresher_test.go`

**Interfaces:**
- Consumes: `agent.Lookup`, `Agent.Markers` (Task 1); `meta.Meta.Agent`,
  `session.Session.Agent` (Task 2).
- Produces:
  - `func Classify(lastActivity, now time.Time, paneTail string, markers []string, missing, dead bool) State`

- [ ] **Step 1: Write the failing activity test**

In `internal/activity/activity_test.go`, replace `TestClassify` with a version
that supplies markers, and add the empty-markers case:

```go
func TestClassify(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	claudeMarkers := []string{"❯ 1.", "Do you want", "(y/n)"}

	cases := []struct {
		name         string
		lastActivity time.Time
		paneTail     string
		markers      []string
		missing      bool // no window at all
		dead         bool // window exists but process exited
		want         State
	}{
		{"missing window is exited", time.Time{}, "", claudeMarkers, true, false, Exited},
		{"dead window is exited", now, "anything", claudeMarkers, false, true, Exited},
		{"recent output is working", now.Add(-1 * time.Second), "Running tests...", claudeMarkers, false, false, Working},
		{"quiet with prompt is waiting", now.Add(-30 * time.Second), "Do you want to proceed?\n❯ 1. Yes", claudeMarkers, false, false, Waiting},
		{"quiet without prompt is idle", now.Add(-30 * time.Second), "all done. 4 passed", claudeMarkers, false, false, Idle},
		// An agent with no known prompt text can never be reported as waiting,
		// however suggestive its pane looks.
		{"no markers is never waiting", now.Add(-30 * time.Second), "Do you want to proceed?", nil, false, false, Idle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.lastActivity, now, c.paneTail, c.markers, c.missing, c.dead)
			if got != c.want {
				t.Fatalf("Classify = %v, want %v", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/activity/`
Expected: FAIL — `too many arguments in call to Classify`.

- [ ] **Step 3: Take markers as a parameter**

In `internal/activity/activity.go`: delete the `promptMarkers` var, change the
package doc's second paragraph, and change `Classify`.

Package doc (replace the "This package is the ONLY place…" sentence):

```go
// Callers supply the prompt markers to match, because what a prompt looks like
// is a property of the agent running in the window (see internal/agent), not of
// this package.
```

```go
// Classify decides a session's state. markers are the pane-tail substrings that
// mean the agent is waiting for input; an empty list means the agent's prompt is
// unknown, so it is never reported as waiting. missing means no window exists
// for it; dead means the window exists but its process has exited.
func Classify(lastActivity, now time.Time, paneTail string, markers []string, missing, dead bool) State {
	if missing || dead {
		return Exited
	}
	if now.Sub(lastActivity) <= workingWindow {
		return Working
	}
	for _, mark := range markers {
		if strings.Contains(paneTail, mark) {
			return Waiting
		}
	}
	return Idle
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/activity/`
Expected: PASS

- [ ] **Step 5: Write the failing refresher test**

Append to `internal/refresher/refresher_test.go` (add the imports
`"github.com/bray/fleet/internal/agent"`, `"os"`, and `"path/filepath"` if they
are not already present):

```go
// A session's agent comes from its meta and reaches the dashboard, and an
// agent with no known prompt markers is never reported as waiting even when its
// pane contains another agent's prompt text.
func TestBuildCarriesAgentAndSkipsWaitingWithoutMarkers(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{ScanRoot: "/code", WorktreeBaseDir: base}
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	wt := filepath.Join(base, "proj", "sess")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := meta.Write(wt, meta.Meta{
		Project: "proj", Session: "sess", Branch: "b", Base: "main",
		Agent: agent.IDOpencode,
	}); err != nil {
		t.Fatal(err)
	}

	wname := naming.TmuxName("proj", "sess")
	target := naming.WindowTarget("proj", "sess")
	ft := &fakeTmux{
		windows: []tmux.Window{{Index: 1, Name: wname, LastActivity: now.Add(-30 * time.Second)}},
		tails:   map[string]string{target: "Do you want to proceed?\n❯ 1. Yes"},
	}

	got, err := Build(cfg, ft, fakeGit{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if got[0].Agent != agent.IDOpencode {
		t.Fatalf("Agent = %q, want %q", got[0].Agent, agent.IDOpencode)
	}
	if got[0].Activity != activity.Idle {
		t.Fatalf("Activity = %v, want Idle — opencode has no prompt markers", got[0].Activity)
	}
}
```

- [ ] **Step 6: Run it to verify it fails**

Run: `go test ./internal/refresher/`
Expected: FAIL — `unknown field Agent in struct literal of type meta.Meta` is
already fixed by Task 2, so the failure is `got[0].Agent = "" , want "opencode"`.

- [ ] **Step 7: Wire the agent through the refresher**

In `internal/refresher/refresher.go`, add `"github.com/bray/fleet/internal/agent"`
to the imports. Inside the session loop, resolve the agent before classifying and
copy the ID onto the session:

```go
			ag := agent.Lookup(md.Agent)
			state := activity.Classify(w.LastActivity, now(), tail, ag.Markers, !present, w.Dead)
```

and in the `session.Session{…}` literal, after `ClaudeSessionID`:

```go
				Agent:           md.Agent,
```

- [ ] **Step 8: Run the package tests to verify they pass**

Run: `go test ./internal/refresher/ ./internal/activity/`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
gofmt -l internal
git add internal/activity internal/refresher
git commit -m "feat(activity): match prompt markers supplied by the agent

Classify now takes the waiting-for-input markers instead of owning
Claude's, and the refresher supplies each session's own. An agent with no
known markers is never reported as waiting."
```

---

### Task 4: `default_agent` configuration

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: `agent.Known`, `agent.IDs`, `agent.IDClaude` (Task 1).
- Produces: `config.Config.DefaultAgent string` with YAML key `default_agent`.

- [ ] **Step 1: Write the failing config tests**

Append to `internal/config/config_test.go`:

```go
func TestDefaultAgentDefaultsToClaude(t *testing.T) {
	if got := Default().DefaultAgent; got != "claude" {
		t.Fatalf("Default().DefaultAgent = %q, want \"claude\"", got)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("scan_root: /code\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DefaultAgent != "claude" {
		t.Fatalf("absent default_agent should default to \"claude\", got %q", cfg.DefaultAgent)
	}
}

func TestDefaultAgentOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("scan_root: /code\ndefault_agent: opencode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DefaultAgent != "opencode" {
		t.Fatalf("DefaultAgent = %q, want \"opencode\"", cfg.DefaultAgent)
	}
}

// A typo must fail loudly at startup rather than silently launching the wrong
// agent on every new session.
func TestValidateRejectsUnknownDefaultAgent(t *testing.T) {
	cfg := Config{ScanRoot: "/code", WorktreeBaseDir: "/wt", DefaultAgent: "opencoder"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected an error for an unknown default_agent")
	}
	if !strings.Contains(err.Error(), "opencode") || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("error should name the valid agents, got %v", err)
	}
}

func TestValidateAcceptsEmptyDefaultAgent(t *testing.T) {
	cfg := Config{ScanRoot: "/code", WorktreeBaseDir: "/wt"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("an empty default_agent means \"the built-in default\", got %v", err)
	}
}
```

Add `"strings"` to the test file's imports.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL — `unknown field DefaultAgent in struct literal of type Config`.

- [ ] **Step 3: Add the field, default, and validation**

In `internal/config/config.go`, add the import
`"github.com/bray/fleet/internal/agent"` and:

```go
	// DefaultAgent is the agent ID the new-session form starts on. Empty means
	// the first registered agent.
	DefaultAgent string `yaml:"default_agent"`
```

In `Default()`, add `DefaultAgent: agent.IDClaude,` to the returned struct.

In `Validate()`, before the final `return nil`:

```go
	if c.DefaultAgent != "" && !agent.Known(c.DefaultAgent) {
		return fmt.Errorf("default_agent %q is not a known agent: valid values are %s",
			c.DefaultAgent, strings.Join(agent.IDs(), ", "))
	}
```

Add `"fmt"` and `"strings"` to the file's imports.

- [ ] **Step 4: Run them to verify they pass**

Run: `go test ./internal/config/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/config
git add internal/config
git commit -m "feat(config): add default_agent

Seeds the new-session form's agent field. An unrecognised value fails
validation at startup, naming the valid agent IDs, rather than silently
launching the wrong agent."
```

---

### Task 5: The agent field in the new-session form

**Files:**
- Modify: `internal/ui/newsession.go`
- Test: `internal/ui/newsession_test.go`

**Interfaces:**
- Consumes: `agent.All`, `agent.Lookup`, `agent.IDClaude`, `agent.IDOpencode`,
  `Agent.Available` (Task 1).
- Produces (used by Task 6):
  - `const fieldAgent` (= 3), `fieldCount` (= 4)
  - `func newForm(p projects.Project, defaultAgent string) newSessionForm`
  - `func (f newSessionForm) selectedAgent() agent.Agent`
  - `func (f newSessionForm) agentAvailable() bool`
  - `func (f *newSessionForm) cycleAgent(delta int)`
  - `newSessionForm.submitError string`

- [ ] **Step 1: Write the failing form tests**

Append to `internal/ui/newsession_test.go` (add `"github.com/bray/fleet/internal/agent"`
to its imports):

```go
func TestNewFormSeedsAgentFromDefault(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, agent.IDOpencode)
	if got := f.selectedAgent().ID; got != agent.IDOpencode {
		t.Fatalf("selectedAgent = %q, want %q", got, agent.IDOpencode)
	}
}

// An empty or unrecognised default is not a startup failure here — config
// validation already rejects a bad value — so the form just starts on the first
// registered agent.
func TestNewFormUnknownDefaultStartsOnFirstAgent(t *testing.T) {
	for _, def := range []string{"", "nonsense"} {
		f := newForm(projects.Project{DefaultBranch: "main"}, def)
		if got := f.selectedAgent().ID; got != agent.All()[0].ID {
			t.Errorf("default %q: selectedAgent = %q, want %q", def, got, agent.All()[0].ID)
		}
	}
}

func TestCycleAgentWrapsBothWays(t *testing.T) {
	n := len(agent.All())
	f := newForm(projects.Project{DefaultBranch: "main"}, agent.IDClaude)

	f.cycleAgent(1)
	if f.agentIndex != 1%n {
		t.Fatalf("after +1 agentIndex = %d, want %d", f.agentIndex, 1%n)
	}
	f.cycleAgent(1) // wraps past the end
	if f.agentIndex != 0 {
		t.Fatalf("after wrapping forward agentIndex = %d, want 0", f.agentIndex)
	}
	f.cycleAgent(-1) // wraps past the start
	if f.agentIndex != n-1 {
		t.Fatalf("after wrapping backward agentIndex = %d, want %d", f.agentIndex, n-1)
	}
}

func TestAgentAvailability(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, agent.IDClaude)
	f.available = []bool{true, false}

	f.agentIndex = 0
	if !f.agentAvailable() {
		t.Error("expected the installed agent to be available")
	}
	f.agentIndex = 1
	if f.agentAvailable() {
		t.Error("expected the missing agent to be unavailable")
	}
}

// A zero-value form has not probed PATH. Treat that as "unknown", not
// "missing", so a form built in a test or by a future caller never blocks a
// legitimate create.
func TestAgentAvailableDefaultsToTrueWhenUnprobed(t *testing.T) {
	var f newSessionForm
	if !f.agentAvailable() {
		t.Error("an unprobed form must not report the agent as unavailable")
	}
}

func TestViewShowsAgentAndNotInstalledMarker(t *testing.T) {
	f := newForm(projects.Project{Name: "app", DefaultBranch: "main"}, agent.IDClaude)
	f.available = []bool{true, false}

	if !strings.Contains(f.view(), "‹ claude ›") {
		t.Fatalf("expected the selected agent in the form view:\n%s", f.view())
	}
	f.agentIndex = 1
	got := f.view()
	if !strings.Contains(got, "‹ opencode (not installed) ›") {
		t.Fatalf("expected the not-installed marker:\n%s", got)
	}
}

func TestViewShowsSubmitError(t *testing.T) {
	f := newForm(projects.Project{Name: "app", DefaultBranch: "main"}, agent.IDClaude)
	f.submitError = "⚠ opencode is not on PATH — install it or pick another agent"
	if !strings.Contains(f.view(), "not on PATH") {
		t.Fatal("expected the submit error in the form view")
	}
}
```

- [ ] **Step 2: Update the existing `newForm` call sites in the tests**

`newForm` grows a second parameter, so the six existing calls in
`internal/ui/newsession_test.go` (`TestBranchHintNewBranch`,
`TestBranchHintExistingLocal`, `TestBranchHintRemoteOnly`,
`TestBranchHintLocalWinsOverRemote`, `TestBranchHintEmptyWhenBlank`,
`TestViewShowsFetchWarning`) and the one in `internal/ui/model_test.go`
(`TestBranchDefaultsToSessionName`) need a second argument. Run this rewrite,
which matches only the bare two-identifier form and leaves `errors.New` and
friends alone:

```bash
gofmt -r 'newForm(a) -> newForm(a, "")' -w internal/ui/newsession_test.go internal/ui/model_test.go
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/ui/`
Expected: FAIL — `too many arguments in call to newForm`, `f.selectedAgent
undefined`, `f.available undefined`.

- [ ] **Step 4: Add the agent field to the form**

In `internal/ui/newsession.go`, add `"github.com/bray/fleet/internal/agent"` to
the imports and make these changes.

The field constants:

```go
// form field indices, in tab order.
const (
	fieldSession = iota
	fieldBranch
	fieldBase
	fieldAgent
	fieldCount
)
```

The struct gains three fields (after `fetchWarning`):

```go
	// agentIndex selects from agent.All(); available caches whether each of
	// those agents was found on PATH when the form opened, in the same order.
	agentIndex int
	available  []bool
	// submitError is shown when submission was refused — the form stays open,
	// and the dashboard status line is not rendered while it is up.
	submitError string
```

`newForm` seeds both:

```go
// newForm seeds a form for a project with sensible defaults. defaultAgent is
// the configured agent ID; an empty or unrecognised value starts on the first
// registered agent.
func newForm(p projects.Project, defaultAgent string) newSessionForm {
	all := agent.All()
	idx := 0
	avail := make([]bool, len(all))
	for i, a := range all {
		if a.ID == defaultAgent {
			idx = i
		}
		// One PATH lookup per agent when the form opens, so typing in the form
		// never shells out.
		avail[i] = a.Available()
	}
	return newSessionForm{
		project:    p,
		base:       p.DefaultBranch,
		field:      fieldSession,
		agentIndex: idx,
		available:  avail,
	}
}
```

The accessors:

```go
// selectedAgent returns the agent the form is currently on, clamping a
// nonsensical index rather than panicking on a hand-built form.
func (f newSessionForm) selectedAgent() agent.Agent {
	all := agent.All()
	if f.agentIndex < 0 || f.agentIndex >= len(all) {
		return all[0]
	}
	return all[f.agentIndex]
}

// agentAvailable reports whether the selected agent's binary was found. A form
// that has not probed PATH reports true: "unknown" must not block a create.
func (f newSessionForm) agentAvailable() bool {
	if f.agentIndex < 0 || f.agentIndex >= len(f.available) {
		return true
	}
	return f.available[f.agentIndex]
}

// cycleAgent moves the selection by delta, wrapping in both directions.
func (f *newSessionForm) cycleAgent(delta int) {
	n := len(agent.All())
	f.agentIndex = ((f.agentIndex+delta)%n + n) % n
	f.submitError = "" // the user is responding to it
}

// agentLabel renders the selection, flagging an agent that is not installed so
// the reason a submit will be refused is visible before trying.
func (f newSessionForm) agentLabel() string {
	label := f.selectedAgent().Label
	if !f.agentAvailable() {
		label += " (not installed)"
	}
	return "‹ " + label + " ›"
}
```

In `view()`, add the row and the error. The rows slice becomes:

```go
	rows := []struct{ label, val string }{
		{"session", f.sessionName},
		{"branch", f.branch},
		{"base", f.base},
		{"agent", f.agentLabel()},
	}
```

After the `fetchWarning` block, add:

```go
	if f.submitError != "" {
		b.WriteString("\n" + warnStyle.Render(f.submitError) + "\n")
	}
```

And the footer hint:

```go
	b.WriteString("\n" + dimStyle.Render("tab next · ←/→ change agent · enter submit on last field · esc cancel"))
```

- [ ] **Step 5: Update the one production caller of `newForm`**

`keyProjectPicker` in `internal/ui/model.go:266` still calls `newForm` with one
argument, so the package would not compile. Pass an empty default for now —
Task 6 replaces it with the configured one:

```go
		m.form = newForm(p, "") // Task 6: m.defaultAgent
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go build ./... && go test -race ./internal/ui/`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
gofmt -l internal/ui
git add internal/ui/newsession.go internal/ui/newsession_test.go internal/ui/model.go internal/ui/model_test.go
git commit -m "feat(ui): add the agent field to the new-session form

The form cycles through the registered agents, seeded from the
configured default, and flags one whose binary is not on PATH."
```

---

### Task 6: Key handling, submit gate, and wiring

**Files:**
- Modify: `internal/ui/model.go:54-67` (`Actions`), `:99-108` (`New`),
  `:261-273` (`keyProjectPicker`), `:277-316` (`keyNewSession`), `:456-476` (`submitForm`)
- Modify: `main.go:95-108` (the `Create` action), `main.go:166` (`ui.New`)
- Test: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `newForm`, `selectedAgent`, `agentAvailable`, `cycleAgent`,
  `fieldAgent`, `submitError` (Task 5); `agent.Lookup` (Task 1);
  `session.Manager.Create` (Task 2); `config.Config.DefaultAgent` (Task 4).
- Produces:
  - `Actions.Create func(p projects.Project, name, branch, base, agentID string) error`
  - `func New(actions *Actions, version, defaultAgent string) Model`

- [ ] **Step 1: Write the failing model tests**

In `internal/ui/model_test.go`, update `TestFormSubmitCallsCreate` to assert the
agent is passed through, and add the two new tests. Add
`"github.com/bray/fleet/internal/agent"` to the imports.

```go
func TestFormSubmitCallsCreate(t *testing.T) {
	var gotName, gotBranch, gotBase, gotAgent string
	a := Actions{Create: func(p projects.Project, name, branch, base, agentID string) error {
		gotName, gotBranch, gotBase, gotAgent = name, branch, base, agentID
		return nil
	}}
	m := New(&a, "", "")
	m.state = stateNewSession
	m.form = newSessionForm{
		project:       projects.Project{Name: "app", Path: "/code/app", DefaultBranch: "main"},
		sessionName:   "fix",
		branch:        "fleet/fix",
		branchTouched: true, // user typed an explicit branch
		base:          "main",
		field:         fieldAgent, // last field; enter submits
		agentIndex:    1,          // opencode
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected create command")
	}
	_ = cmd() // execute create
	if gotName != "fix" || gotBranch != "fleet/fix" || gotBase != "main" {
		t.Fatalf("create got name=%q branch=%q base=%q", gotName, gotBranch, gotBase)
	}
	if gotAgent != agent.IDOpencode {
		t.Fatalf("create got agent=%q, want %q", gotAgent, agent.IDOpencode)
	}
}

func TestArrowKeysCycleAgentOnAgentField(t *testing.T) {
	m := New(nil, "", "")
	m.state = stateNewSession
	m.form = newSessionForm{field: fieldAgent}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if got := updated.(Model).form.agentIndex; got != 1 {
		t.Fatalf("after right agentIndex = %d, want 1", got)
	}
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyLeft})
	if got := updated.(Model).form.agentIndex; got != 0 {
		t.Fatalf("after left agentIndex = %d, want 0", got)
	}
}

// Creating a session with an agent that is not installed would leave a real
// worktree and branch behind for a window that immediately dies.
func TestSubmitBlockedWhenAgentNotInstalled(t *testing.T) {
	created := 0
	a := Actions{Create: func(projects.Project, string, string, string, string) error {
		created++
		return nil
	}}
	m := New(&a, "", "")
	m.state = stateNewSession
	m.form = newSessionForm{
		project:     projects.Project{Name: "app", DefaultBranch: "main"},
		sessionName: "fix",
		branch:      "fix",
		base:        "main",
		field:       fieldAgent,
		agentIndex:  1,
		available:   []bool{true, false}, // opencode missing
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if cmd != nil {
		_ = cmd()
	}
	if created != 0 {
		t.Fatalf("Create was called %d times, want 0", created)
	}
	if mm.state != stateNewSession {
		t.Fatalf("state = %v, want the form to stay open", mm.state)
	}
	if !strings.Contains(mm.form.submitError, "not on PATH") {
		t.Fatalf("submitError = %q, want a PATH message", mm.form.submitError)
	}
}
```

Ensure `"strings"` is imported by `model_test.go`.

- [ ] **Step 2: Rewrite the remaining `New` call sites**

`New` grows a third parameter. Rewrite every call in the ui tests — the pattern
matches only a bare two-argument `New(...)`, so `errors.New` (one argument) and
`spinner.New()` (none) are untouched:

```bash
gofmt -r 'New(a, b) -> New(a, b, "")' -w internal/ui/model_test.go
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/ui/`
Expected: FAIL — `too many arguments in call to New`, and the `Actions.Create`
literal does not match the struct field's type.

- [ ] **Step 4: Change the `Actions` and `New` signatures**

In `internal/ui/model.go`:

```go
	Create        func(p projects.Project, name, branch, base, agentID string) error
```

```go
// Model is the root Bubble Tea model.
type Model struct {
	...
	// defaultAgent is the configured agent ID each new form starts on.
	defaultAgent string
}
```

(add the field at the end of the struct, beside the other configuration-ish
fields), and:

```go
func New(actions *Actions, version, defaultAgent string) Model {
	var a Actions
	if actions != nil {
		a = *actions
	}
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = spinnerStyle
	return Model{actions: a, state: stateDashboard, spinner: sp, version: version,
		defaultAgent: defaultAgent}
}
```

In `keyProjectPicker`, replace the Task 5 stopgap with the configured default:

```go
		m.form = newForm(p, m.defaultAgent)
```

- [ ] **Step 5: Handle the new keys and the submit gate**

Replace the body of `keyNewSession` in `internal/ui/model.go`:

```go
func (m Model) keyNewSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateDashboard
		return m, nil
	case "tab", "down":
		m.form.field = (m.form.field + 1) % fieldCount
	case "shift+tab", "up":
		m.form.field = (m.form.field + fieldCount - 1) % fieldCount
	case "left":
		if m.form.field == fieldAgent {
			m.form.cycleAgent(-1)
		}
	case "right":
		if m.form.field == fieldAgent {
			m.form.cycleAgent(1)
		}
	case "backspace":
		if m.form.field == fieldAgent {
			return m, nil // an enum, not a text field
		}
		p := m.form.active()
		if len(*p) > 0 {
			*p = (*p)[:len(*p)-1]
		}
		if m.form.field == fieldBranch {
			m.form.branchTouched = true
		}
		m.form.syncBranchDefault()
	case "enter":
		m.form.syncBranchDefault()
		if m.form.field < fieldAgent {
			m.form.field++
			return m, nil
		}
		// Refuse rather than create a worktree and branch for a window whose
		// command would immediately fail. The message lives on the form: the
		// dashboard status line is not rendered while the form is up.
		if !m.form.agentAvailable() {
			m.form.submitError = fmt.Sprintf(
				"⚠ %s is not on PATH — install it or pick another agent",
				m.form.selectedAgent().Label)
			return m, nil
		}
		// Close the form and return to the dashboard; the create runs in the
		// background and a refresh will populate the new session.
		m.state = stateDashboard
		return m, m.submitForm()
	default:
		if m.form.field == fieldAgent {
			return m, nil // an enum, not a text field
		}
		if len(msg.Runes) > 0 {
			p := m.form.active()
			*p += string(msg.Runes)
			if m.form.field == fieldBranch {
				m.form.branchTouched = true
			}
			m.form.syncBranchDefault()
		}
	}
	return m, nil
}
```

`fmt` is already imported by `model.go`.

Also update `active()` in `internal/ui/newsession.go` so the agent field never
returns the base pointer — the `default` branch currently catches it:

```go
// active returns a pointer to the currently focused text field's string. The
// agent field is an enum with no text to edit, so callers must not reach here
// with it focused.
func (f *newSessionForm) active() *string {
	switch f.field {
	case fieldSession:
		return &f.sessionName
	case fieldBranch:
		return &f.branch
	default:
		return &f.base
	}
}
```

(The comment is the change; the key handling above is what guarantees it.)

- [ ] **Step 6: Pass the agent through `submitForm`**

```go
// submitForm invokes Create and triggers a refresh.
func (m Model) submitForm() tea.Cmd {
	f := m.form
	agentID := f.selectedAgent().ID
	create := m.actions.Create
	refreshFn := m.actions.Refresh
	return func() tea.Msg {
		if create != nil {
			if err := create(f.project, f.sessionName, f.branch, f.base, agentID); err != nil {
				return errorMsg{err: err}
			}
		}
		if refreshFn != nil {
			ss, err := refreshFn()
			if err != nil {
				return errorMsg{err: err}
			}
			return sessionsUpdatedMsg{sessions: ss}
		}
		return sessionsUpdatedMsg{}
	}
}
```

- [ ] **Step 7: Wire `main.go`**

Replace the Task 2 stopgap `Create` action (`agent` is already imported there):

```go
		Create: func(p projects.Project, name, branch, base, agentID string) error {
			_, err := mgr.Create(p, name, branch, base, agent.Lookup(agentID))
			return err
		},
```

and:

```go
	p := tea.NewProgram(ui.New(&actions, version, cfg.DefaultAgent), tea.WithAltScreen())
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go build ./... && go test -race ./...`
Expected: PASS across every package.

- [ ] **Step 9: Commit**

```bash
gofmt -l .
git add internal/ui main.go
git commit -m "feat(ui): choose the session's agent when creating it

Arrow keys cycle the agent field, submitting is refused when the chosen
agent is not on PATH, and the selection reaches the session manager."
```

---

### Task 7: Show the agent on the dashboard

**Files:**
- Modify: `internal/ui/views.go:92-109` (the detail line)
- Test: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `agent.Lookup` (Task 1), `session.Session.Agent` (Task 2).
- Produces: nothing new.

- [ ] **Step 1: Write the failing view test**

Append to `internal/ui/model_test.go`:

```go
func TestDashboardShowsAgentPerSession(t *testing.T) {
	m := New(nil, "", "")
	m.sessions = []session.Session{
		{Project: "app", Name: "one", Branch: "one", Base: "main", Agent: agent.IDOpencode},
		// A session created before agents were selectable has no agent ID and
		// is, in fact, running Claude Code.
		{Project: "app", Name: "two", Branch: "two", Base: "main"},
	}
	got := m.View()
	if !strings.Contains(got, "· opencode ·") {
		t.Fatalf("expected the opencode session labelled:\n%s", got)
	}
	if !strings.Contains(got, "· claude ·") {
		t.Fatalf("expected a legacy session labelled claude:\n%s", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/ui/ -run TestDashboardShowsAgentPerSession`
Expected: FAIL — "expected the opencode session labelled".

- [ ] **Step 3: Add the label to the detail line**

In `internal/ui/views.go`, add `"github.com/bray/fleet/internal/agent"` to the
imports and, immediately after `detail += s.Activity.Label()`:

```go
		// Which agent drives the session is not inferable from anything else on
		// the row, and a legacy session with no stored ID is running Claude.
		detail += " · " + agent.Lookup(s.Agent).Label
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/ui/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/ui
git add internal/ui/views.go internal/ui/model_test.go
git commit -m "feat(ui): show each session's agent on the dashboard

Two rows driven by different agents are otherwise indistinguishable."
```

---

### Task 8: Documentation and full verification

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/usage.md`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing.

- [ ] **Step 1: Update `CLAUDE.md`**

In the **Planned package layout** list, add this entry immediately before the
`config` entry:

```markdown
- `agent` — the registry of coding agents fleet can run (Claude Code,
  opencode): launch/resume commands, binary name, prompt markers.
```

In **Core design decisions**, extend the **Session model** bullet with a
sentence after the sentence ending "…running `claude` in its worktree.":

```markdown
  Which agent a window runs — Claude Code or opencode — is chosen per session in
  the new-session form and recorded in `.fleet/meta.json`; sessions created
  before that existed have no recorded agent and run Claude Code.
```

- [ ] **Step 2: Update `docs/usage.md`**

In the **Configuration** section, add the key to the sample YAML block, after
the `worktree_base_dir` entry:

```yaml

# Which coding agent new sessions start on: claude (default) or opencode.
default_agent: claude
```

...and to the bullet list below it, after the `worktree_base_dir` bullet:

```markdown
- `default_agent` seeds the new-session form's agent field — `claude` (the
  default) or `opencode`. The agent is chosen per session, so this only decides
  which one the form starts on. An unrecognised value is a startup error.
```

In the **Keybindings** section, replace the existing **New-session form**
paragraph with:

```markdown
**New-session form**: `Tab`/`Shift-Tab` (or `↑`/`↓`) to move between fields, type
to edit, `←`/`→` to change the agent, `Enter` to advance / submit on the last
field, `Esc` to cancel. The branch defaults to the sanitized session name and the
base to the project's default branch.

The **agent** field chooses what drives the session: `claude` (Claude Code) or
`opencode`. An agent whose binary is not on your `PATH` shows as
`(not installed)` and cannot be submitted. The choice is fixed for the life of
the session — to switch, delete the session and create a new one.
```

Note that this corrects a stale claim in that paragraph: the branch defaults to
the sanitized session name, not `fleet/<session>`.

- [ ] **Step 3: Verify the whole suite, format, vet, and lint**

Run each and confirm the expected result before continuing:

```bash
gofmt -l .                                   # expect: no output
go vet ./...                                 # expect: no output
go build ./...                               # expect: no output
go test -race ./...                          # expect: ok for every package
go test -race -tags smoke -run Smoke ./...   # expect: ok (or skipped without tmux/git)
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags=smoke ./...
```

- [ ] **Step 4: Manually verify the form**

Run `go run .`, press `n`, pick a project, tab to the **agent** field, and
confirm: ←/→ cycle between `claude` and `opencode`; `opencode` shows
`(not installed)` unless it is on your PATH; pressing enter on an uninstalled
agent shows the PATH warning and leaves the form open; picking an installed
agent creates the session, and the dashboard row shows that agent's name.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md docs/usage.md
git commit -m "docs: document per-session agent selection

Records the agent package in the layout, the default_agent config key,
and the new-session form field."
```

---

## Notes for the implementer

- **`AGENTS.md` is already done.** It was committed with the spec (`a1f1302`) as
  a symlink to `CLAUDE.md`. Do not create a second copy; edit `CLAUDE.md`.
- **Every commit builds.** Two tasks carry a deliberate one-line stopgap so that
  stays true while a signature is half-rewired: `main.go`'s `Create` action
  (Task 2 → replaced in Task 6) and `newForm(p, "")` (Task 5 → replaced in
  Task 6). Both are marked with a comment naming the task that removes them; do
  not leave either behind.
- **opencode's waiting-state markers are a deliberate gap.** Once someone has
  watched a live opencode session, add the real substrings to its `Markers` in
  `internal/agent/agent.go` — that is the only change needed, and
  `TestMarkers` in `agent_test.go` will need its opencode assertion inverted.
