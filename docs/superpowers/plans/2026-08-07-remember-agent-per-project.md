# Remember the agent used per project Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-08-07-remember-agent-per-project-design.md`](../specs/2026-08-07-remember-agent-per-project-design.md)

**Goal:** When the new-session form opens for a project, its agent field defaults to the last agent used in that project instead of always starting on the global `default_agent`.

**Architecture:** A tiny `internal/memory` package reads/writes a plain-text per-project file (`<worktreeBaseDir>/<project>/.fleet-agent`) beside the worktrees. The write hooks into the `Actions.Create` wrapper in `main.go` (returning a status-line notice on failure, never failing the session); the read hooks into the form-open path via a new `Actions.RememberedAgent`, with per-project memory beating the global default only when it is a known agent ID. `session.Manager` and its tests are untouched.

**Tech Stack:** Go, Bubble Tea/Lip Gloss (TUI), standard library.

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
  are written to the memory file; never rename them.
- **Comment style:** explain *why*, in full sentences, matching the surrounding
  code. Do not add comments that restate the code.
- Full suite for any task: `go test -race ./...`.

---

## File Structure

**Created:**
- `internal/memory/memory.go` — the per-project agent memory: `Path`, `Read`,
  `Write`.
- `internal/memory/memory_test.go`.

**Modified:**
- `internal/ui/model.go` — `Actions.RememberedAgent` field; seed the form from
  it in `keyProjectPicker`; `Actions.Create` returns `(notice string, err error)`;
  `submitForm` threads the notice into `sessionsUpdatedMsg`.
- `internal/ui/model_test.go` — new seed/notice tests; update the three existing
  `Create` action literals to the new signature.
- `main.go` — set `Actions.RememberedAgent` (reads the file) and update
  `Actions.Create` (writes the file after `mgr.Create` succeeds).
- `CLAUDE.md` — add `memory` to the package layout; note the per-project default.
- `docs/usage.md` — note the per-project default in the new-session flow.

---

### Task 1: `internal/memory` package

**Files:**
- Create: `internal/memory/memory.go`
- Test: `internal/memory/memory_test.go`

**Interfaces:**
- Produces (later tasks use these exact signatures):
  - `func Path(base, project string) string` — `<base>/<Sanitize(project)>/.fleet-agent`.
  - `func Read(path string) string` — stored value, or `""` when the file is
    missing or unreadable. Dumb text read: does not know agent IDs (the caller
    decides whether a non-empty value is a known agent).
  - `func Write(path, value string) error` — writes `value + "\n"`, creating the
    project directory if needed.

- [ ] **Step 1: Write the failing tests**

```go
package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bray/fleet/internal/naming"
)

func TestPathSanitizesProject(t *testing.T) {
	got := Path("/base", "My App")
	want := filepath.Join("/base", naming.Sanitize("My App"), ".fleet-agent")
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestWriteThenRead(t *testing.T) {
	base := t.TempDir()
	path := Path(base, "app")
	if err := Write(path, "opencode"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := Read(path); got != "opencode" {
		t.Fatalf("Read = %q, want %q", got, "opencode")
	}
}

func TestWriteCreatesProjectDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "nested", "whee")
	path := Path(base, "app")
	if err := Write(path, "claude"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestReadMissingReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-project", ".fleet-agent")
	if got := Read(path); got != "" {
		t.Fatalf("Read missing = %q, want empty", got)
	}
}

// A file containing only whitespace holds no value; Read must not return it.
func TestReadWhitespaceOnlyReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app", ".fleet-agent")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("  \n\t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Read(path); got != "" {
		t.Fatalf("Read whitespace-only = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/memory/ -run 'TestPathSanitizesProject|TestWriteThenRead|TestWriteCreatesProjectDir|TestReadMissingReturnsEmpty|TestReadWhitespaceOnlyReturnsEmpty' -v`
Expected: build failure — `internal/memory` does not exist.

- [ ] **Step 3: Write the minimal implementation**

```go
// Package memory persists the last agent used per project, so the new-session
// form can default to it. It is plain-text state kept beside the worktrees,
// separate from each session's meta.json.
package memory

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bray/fleet/internal/naming"
)

// fileName is the per-project memory file. A leading dot keeps it from being
// mistaken for a session directory by the refresher, which skips non-directories.
const fileName = ".fleet-agent"

// Path returns the per-project memory file for project, under base. The
// worktrees for a project already live in <base>/<Sanitize(project)>/, so the
// picker's project name maps to the same directory the memory file sits in.
func Path(base, project string) string {
	return filepath.Join(base, naming.Sanitize(project), fileName)
}

// Read returns the stored value, or "" when the file is missing or unreadable.
// Errors are swallowed because a missing file simply means "no memory"; there
// is nothing actionable at form-open time. The call site decides whether a
// non-empty value names a known agent.
func Read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Write stores value, creating the project directory if needed.
func Write(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value+"\n"), 0o644)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/memory/`
Expected: PASS.

- [ ] **Step 5: Format and vet**

Run: `gofmt -l . && go vet ./...`
Expected: `gofmt` prints nothing; `go vet` passes.

- [ ] **Step 6: Commit**

```bash
git add internal/memory/memory.go internal/memory/memory_test.go
git commit -m "feat(memory): persist the last agent used per project"
```

---

### Task 2: Seed the form from the remembered agent

**Files:**
- Modify: `internal/ui/model.go` (`Actions` struct + `keyProjectPicker` + imports)
- Modify: `internal/ui/model_test.go`
- Modify: `main.go`

**Interfaces:**
- Consumes: `memory.Path`, `memory.Read` from Task 1; `agent.Known` (exists).
- Produces:
  - `Actions.RememberedAgent func(p projects.Project) string` — last agent used
    for the project, `""` when there is none (nil-safe in the model).
  - Form seed logic: remembered value wins over `defaultAgent` only when it is
    non-empty **and** `agent.Known(remembered)`.

- [ ] **Step 1: Write the failing tests**

Append these to `internal/ui/model_test.go` (the file already imports `agent`
and `projects`):

```go
func TestFormSeedsRememberedAgent(t *testing.T) {
	a := Actions{RememberedAgent: func(projects.Project) string { return agent.IDOpencode }}
	m := New(&a, "", agent.IDClaude)
	m.state = stateProjectPicker
	m.projects = []projects.Project{{Name: "app", DefaultBranch: "main"}}
	m.cursor = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	f := updated.(Model).form
	if got := f.selectedAgent().ID; got != agent.IDOpencode {
		t.Fatalf("seeded agent = %q, want %q", got, agent.IDOpencode)
	}
}

func TestFormFallsBackToDefaultWhenNoMemory(t *testing.T) {
	a := Actions{RememberedAgent: func(projects.Project) string { return "" }}
	m := New(&a, "", agent.IDOpencode)
	m.state = stateProjectPicker
	m.projects = []projects.Project{{Name: "app", DefaultBranch: "main"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	f := updated.(Model).form
	if got := f.selectedAgent().ID; got != agent.IDOpencode {
		t.Fatalf("seeded agent = %q, want default %q", got, agent.IDOpencode)
	}
}

func TestFormIgnoresUnknownRememberedAgent(t *testing.T) {
	a := Actions{RememberedAgent: func(projects.Project) string { return "nonsense" }}
	m := New(&a, "", agent.IDClaude)
	m.state = stateProjectPicker
	m.projects = []projects.Project{{Name: "app", DefaultBranch: "main"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	f := updated.(Model).form
	if got := f.selectedAgent().ID; got != agent.IDClaude {
		t.Fatalf("seeded agent = %q, want fallback %q", got, agent.IDClaude)
	}
}

func TestNilRememberedAgentIsSafe(t *testing.T) {
	m := New(nil, "", agent.IDClaude)
	m.state = stateProjectPicker
	m.projects = []projects.Project{{Name: "app", DefaultBranch: "main"}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	f := updated.(Model).form
	if got := f.selectedAgent().ID; got != agent.IDClaude {
		t.Fatalf("seeded agent = %q, want default %q", got, agent.IDClaude)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestFormSeedsRememberedAgent|TestFormFallsBackToDefaultWhenNoMemory|TestFormIgnoresUnknownRememberedAgent|TestNilRememberedAgentIsSafe' -v`
Expected: FAIL — `Actions` has no field `RememberedAgent`.

- [ ] **Step 3: Add the action and seed logic**

In `internal/ui/model.go`:

1. Add the field to the `Actions` struct, after the `Create` field:

```go
	// RememberedAgent returns the last agent used for a project, or "" when
	// there is none yet. Seeded per-project, it beats the global default only
	// when it names a known agent.
	RememberedAgent func(p projects.Project) string
```

2. Add the import (keep the existing import block alphabetized):

```go
	"github.com/bray/fleet/internal/agent"
```

3. Replace the `enter` case in `keyProjectPicker` (currently the block starting
   `case "enter":` around line 266) with:

```go
	case "enter":
		if len(m.projects) == 0 {
			return m, nil
		}
		p := m.projects[m.cursor]
		seed := m.defaultAgent
		if m.actions.RememberedAgent != nil {
			if remembered := m.actions.RememberedAgent(p); remembered != "" && agent.Known(remembered) {
				seed = remembered
			}
		}
		m.form = newForm(p, seed)
		m.state = stateNewSession
		m.cursor = 0
		return m, tea.Batch(
			loadBranches(m.actions.Branches, p),
			fetchBranches(m.actions.FetchBranches, p),
		)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Wire the action in main.go**

In `main.go`, add `"github.com/bray/fleet/internal/memory"` to the imports, then
add a `RememberedAgent` field to the `actions` literal (next to `Projects`):

```go
		RememberedAgent: func(p projects.Project) string {
			return memory.Read(memory.Path(cfg.WorktreeBaseDir, p.Name))
		},
```

- [ ] **Step 6: Build and run the full suite**

Run: `go build ./... && go test -race ./...`
Expected: build succeeds; all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/model.go internal/ui/model_test.go main.go
git commit -m "feat(ui): default the agent field to the last agent used in the project"
```

---

### Task 3: Create returns a notice; write memory on create

**Files:**
- Modify: `internal/ui/model.go` (`Actions.Create` signature + `submitForm`)
- Modify: `internal/ui/model_test.go`
- Modify: `main.go` (`Actions.Create` wrapper)

**Interfaces:**
- Consumes: `memory.Write` from Task 1.
- Produces:
  - `Actions.Create func(p projects.Project, name, branch, base, agentID string) (notice string, err error)`.
  - `submitForm` carries `notice` into `sessionsUpdatedMsg{sessions, notice}`.

- [ ] **Step 1: Update the failing tests**

Three existing `Create` action literals in `internal/ui/model_test.go` use the
old `error` return. Change each to the new `(string, error)` signature:

1. `TestFormSubmitCallsCreate` (line ~134):

```go
	a := Actions{Create: func(p projects.Project, name, branch, base, agentID string) (string, error) {
		gotName, gotBranch, gotBase, gotAgent = name, branch, base, agentID
		return "", nil
	}}
```

2. `TestSubmitBlockedWhenAgentNotInstalled` (line ~181):

```go
	a := Actions{Create: func(projects.Project, string, string, string, string) (string, error) {
		created++
		return "", nil
	}}
```

3. `TestFormSubmitReturnsToDashboard` (line ~420):

```go
	a := Actions{Create: func(projects.Project, string, string, string, string) (string, error) { return "", nil }}
```

Then add this test, asserting the create notice reaches the status line:

```go
func TestFormSubmitCarriesCreateNotice(t *testing.T) {
	a := Actions{
		Create: func(projects.Project, string, string, string, string) (string, error) {
			return "⚠ could not remember agent for app", nil
		},
		Refresh: func() ([]session.Session, error) { return nil, nil },
	}
	m := New(&a, "", "")
	m.state = stateNewSession
	m.form = newSessionForm{
		project:     projects.Project{Name: "app", DefaultBranch: "main"},
		sessionName: "fix",
		branch:      "fleet/fix",
		base:        "main",
		field:       fieldAgent,
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected create command")
	}
	msg := cmd()
	updated, _ = updated.(Model).Update(msg)
	mm := updated.(Model)
	if !strings.Contains(mm.status, "could not remember agent") {
		t.Fatalf("status = %q, want the create notice", mm.status)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/`
Expected: FAIL — the action literals no longer compile against the new
signature (this is the "failing test" step; the failures are compile-time).

- [ ] **Step 3: Change the signature and thread the notice**

In `internal/ui/model.go`:

1. Change the `Create` field in `Actions`:

```go
	// Create makes a new session. The returned notice is a non-fatal status
	// line message (e.g. a failed best-effort write), not an error.
	Create func(p projects.Project, name, branch, base, agentID string) (notice string, err error)
```

2. Replace `submitForm` (currently around line 485) with:

```go
// submitForm invokes Create and triggers a refresh. Create's notice is threaded
// into the refreshed message so a non-fatal warning lands on the status line.
func (m Model) submitForm() tea.Cmd {
	f := m.form
	agentID := f.selectedAgent().ID
	create := m.actions.Create
	refreshFn := m.actions.Refresh
	return func() tea.Msg {
		var notice string
		if create != nil {
			var err error
			notice, err = create(f.project, f.sessionName, f.branch, f.base, agentID)
			if err != nil {
				return errorMsg{err: err}
			}
		}
		if refreshFn != nil {
			ss, err := refreshFn()
			if err != nil {
				return errorMsg{err: err}
			}
			return sessionsUpdatedMsg{sessions: ss, notice: notice}
		}
		return sessionsUpdatedMsg{notice: notice}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Write memory on create in main.go**

In `main.go`, replace the `Create` entry in the `actions` literal:

```go
		Create: func(p projects.Project, name, branch, base, agentID string) (string, error) {
			if _, err := mgr.Create(p, name, branch, base, agent.Lookup(agentID)); err != nil {
				return "", err
			}
			// Remembering the agent is best-effort: the session exists, so a
			// failed write is a status-line notice, never a failed create.
			if err := memory.Write(memory.Path(cfg.WorktreeBaseDir, p.Name), agentID); err != nil {
				return fmt.Sprintf("⚠ could not remember agent for %s", p.Name), nil
			}
			return "", nil
		},
```

- [ ] **Step 6: Build and run the full suite**

Run: `go build ./... && go test -race ./...`
Expected: build succeeds; all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/ui/model.go internal/ui/model_test.go main.go
git commit -m "feat(ui): remember the session's agent for its project on create"
```

---

### Task 4: Documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `docs/usage.md`

- [ ] **Step 1: Update CLAUDE.md**

1. In the package layout list, add `memory` after the `meta` line:

```
- `memory` — per-project agent memory (last agent used per project), read to
  seed the new-session form and written on create.
```

2. In the **Core design decisions** → **Session model** paragraph (the one
   describing which agent a window runs), append a sentence:

```
The new-session form defaults its agent field to the last agent used in that
project (a per-project `.fleet-agent` file beside the worktrees), falling back
to the configured `default_agent`.
```

- [ ] **Step 2: Update docs/usage.md**

After the `default_agent` bullet (line ~92), add:

```
- The **agent** field starts on the last agent you used in that project (kept
  in `<worktree_base_dir>/<project>/.fleet-agent`), falling back to
  `default_agent`. The default is silent — the field looks and behaves exactly
  as before, just pre-selected.
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md docs/usage.md
git commit -m "docs: document the per-project agent default"
```

---

### Task 5: Final verification

**Files:** none.

- [ ] **Step 1: Format and vet**

Run: `gofmt -l . && go vet ./...`
Expected: `gofmt` prints nothing; `go vet` passes.

- [ ] **Step 2: Full test suite with race detector**

Run: `go test -race ./...`
Expected: all pass.

- [ ] **Step 3: Lint**

Run: `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags=smoke ./...`
Expected: clean.

- [ ] **Step 4: Smoke test**

Run: `go test -race -tags smoke -run Smoke ./...`
Expected: passes (skips if git/tmux unavailable).

- [ ] **Step 5: Confirm git state**

Run: `git log --oneline -6`
Expected: the four feature commits on top of the spec commit:
`feat(memory)`, `feat(ui): default the agent field...`, `feat(ui): remember...`,
`docs: document...`.

- [ ] **Step 6: Commit any leftover artefacts**

If the spec, plan, or anything else is uncommitted:

```bash
git add -A
git commit -m "chore: commit implementation artefacts"
```

---

## Self-Review Notes

- **Spec coverage:** memory package → Task 1; per-project > global seed with
  `agent.Known` guard → Task 2; write-on-create + notice + never-fail → Task 3;
  docs → Task 4. No backfill, silent default, keep-on-cleanup, no meta schema
  change are all intentionally absent (non-goals).
- **Type consistency:** `Actions.Create` is `(string, error)` everywhere
  (Task 3 produces it; the three test literals and `submitForm` consume it).
  `Actions.RememberedAgent` is `func(p projects.Project) string` everywhere
  (Task 2). `memory.Path/Read/Write` signatures are identical across Tasks 1-3.
- **Ambiguity resolution:** the spec's "garbled content reads back as `""`" is
  resolved as *dumb text read* — `Read` returns trimmed content; the unknown-ID
  fallback is the `agent.Known` guard in Task 2's seed logic, keeping `memory`
  free of agent knowledge.
