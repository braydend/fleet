# Claude Code Session Resume Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fleet stores a stable Claude session ID per fleet-session and resumes it on every re-launch (respawn and post-restart) so conversation context is never lost.

**Architecture:** Generate a UUID at session-create time, persist it in `.fleet/meta.json`, thread it through the refresher into the `session.Session` model, and use it to build the `claude` launch command — `--session-id` on first launch, a self-healing `--resume … || --session-id … || claude` chain on every re-launch. Legacy sessions (no stored ID) keep launching bare `claude`.

**Tech Stack:** Go 1.26.4, standard library only (`crypto/rand` for UUIDs), existing internal packages (`meta`, `session`, `refresher`, `naming`, `tmux`).

## Global Constraints

- **Go version floor:** `go 1.26.4` (per `go.mod`). Do not bump.
- **No new dependencies.** UUIDs come from `crypto/rand` + `encoding/hex`; the module must keep only its current requires.
- **Conventional Commits** for every commit (`feat:`, `test:`, `docs:`, `refactor:`).
- **TDD:** write the failing test first, watch it fail, then implement. Commit after each task's tests pass.
- **Do not rename tmux windows.** The stable window name stays `fleet-<project>-<session>` (`naming.TmuxName`); the Claude session ID and the `-n` display name are separate from it.
- **Display name uses raw human names**, `"<project>/<session>"` (e.g. `My App/fix-bug`), not the sanitized tmux form.
- Run `gofmt -l .` clean and `go vet ./...` clean before the final commit.

---

### Task 1: Persist the Claude session ID in meta

**Files:**
- Modify: `internal/meta/meta.go` (add field to `Meta`)
- Test: `internal/meta/meta_test.go`

**Interfaces:**
- Consumes: nothing (leaf change).
- Produces: `meta.Meta.ClaudeSessionID string` (JSON key `claude_session_id`, omitempty). Read back as `""` when absent.

- [ ] **Step 1: Write the failing tests**

Add to `internal/meta/meta_test.go`:

```go
func TestWriteThenReadIncludesClaudeSessionID(t *testing.T) {
	dir := t.TempDir()
	in := Meta{
		Project:         "My App",
		Session:         "fix-bug",
		Branch:          "fleet/fix-bug",
		Base:            "main",
		RepoPath:        "/repos/my-app",
		CreatedAt:       time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC),
		CleanupIntent:   "delete",
		ClaudeSessionID: "6f9619ff-8b86-4d01-b42d-00cf4fc964ff",
	}
	if err := Write(dir, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != in {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, in)
	}
}

func TestReadLegacyMetaWithoutSessionID(t *testing.T) {
	dir := t.TempDir()
	// A meta.json written before this field existed must still read cleanly.
	legacy := `{"project":"p","session":"s","branch":"b","base":"main","repo_path":"/r","created_at":"2026-06-16T10:00:00Z"}`
	if err := writeRaw(dir, []byte(legacy)); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.ClaudeSessionID != "" {
		t.Fatalf("legacy meta should have empty ClaudeSessionID, got %q", got.ClaudeSessionID)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/meta/`
Expected: FAIL — compile error `unknown field 'ClaudeSessionID' in struct literal of type Meta`.

- [ ] **Step 3: Add the field**

In `internal/meta/meta.go`, add the field to the `Meta` struct after `CleanupIntent`:

```go
	CleanupIntent   string    `json:"cleanup_intent,omitempty"`
	ClaudeSessionID string    `json:"claude_session_id,omitempty"`
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/meta/`
Expected: PASS (all meta tests, including the two new ones).

- [ ] **Step 5: Commit**

```bash
git add internal/meta/meta.go internal/meta/meta_test.go
git commit -m "feat: persist Claude session ID in worktree meta"
```

---

### Task 2: Carry the session ID through the domain model and refresher

**Files:**
- Modify: `internal/session/session.go` (add field to `Session`)
- Modify: `internal/refresher/refresher.go` (copy `md.ClaudeSessionID` into the reconstructed `Session`)
- Test: `internal/refresher/refresher_test.go`

**Interfaces:**
- Consumes: `meta.Meta.ClaudeSessionID` (Task 1).
- Produces: `session.Session.ClaudeSessionID string`, populated by `refresher.Build` from each worktree's meta.

- [ ] **Step 1: Write the failing test changes**

In `internal/refresher/refresher_test.go`, inside `TestBuildDerivesSessionsAndActivity`, add the ID to the **alive** worktree's meta write (the `wtA` block):

```go
	_ = meta.Write(wtA, meta.Meta{
		Project: "My App", Session: "alive", Branch: "fleet/alive", Base: "main",
		RepoPath: "/code/my-app", CreatedAt: time.Unix(1, 0).UTC(),
		ClaudeSessionID: "alive-session-id",
	})
```

Then extend the `case "alive":` assertion to also check the field:

```go
		case "alive":
			if !s.Alive || s.Exited || s.Activity != activity.Working || s.WindowIndex != 1 {
				t.Fatalf("alive session wrong: %+v", s)
			}
			if s.ClaudeSessionID != "alive-session-id" {
				t.Fatalf("expected ClaudeSessionID carried from meta, got %q", s.ClaudeSessionID)
			}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/refresher/`
Expected: FAIL — compile error `s.ClaudeSessionID undefined (type session.Session has no field or method ClaudeSessionID)`.

- [ ] **Step 3: Add the field to the domain model**

In `internal/session/session.go`, add to the `Session` struct after `WindowIndex`:

```go
	WindowIndex  int // 1-based tab number; 0 if no live window
	Git          git.Status
	// ClaudeSessionID is the stable Claude Code session ID fleet resumes on
	// re-launch. Empty for sessions created before this feature (launch bare).
	ClaudeSessionID string
```

- [ ] **Step 4: Populate it in the refresher**

In `internal/refresher/refresher.go`, in the `session.Session{…}` literal (the block ending `WindowIndex: w.Index, Git: st,`), add the field:

```go
				WindowIndex:  w.Index,
				Git:          st,
				ClaudeSessionID: md.ClaudeSessionID,
			}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/refresher/ ./internal/session/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/session/session.go internal/refresher/refresher.go internal/refresher/refresher_test.go
git commit -m "feat: carry Claude session ID from meta into session model"
```

---

### Task 3: UUID generator in the naming package

**Files:**
- Create: `internal/naming/sessionid.go`
- Test: `internal/naming/sessionid_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `naming.NewClaudeSessionID() string` — a random RFC 4122 v4 UUID. Used as the default `newID` in `Manager` (Task 5).

- [ ] **Step 1: Write the failing test**

Create `internal/naming/sessionid_test.go`:

```go
package naming

import (
	"regexp"
	"testing"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewClaudeSessionIDFormat(t *testing.T) {
	id := NewClaudeSessionID()
	if !uuidV4.MatchString(id) {
		t.Fatalf("not a v4 UUID: %q", id)
	}
}

func TestNewClaudeSessionIDUnique(t *testing.T) {
	if NewClaudeSessionID() == NewClaudeSessionID() {
		t.Fatal("expected distinct IDs on successive calls")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/naming/`
Expected: FAIL — compile error `undefined: NewClaudeSessionID`.

- [ ] **Step 3: Implement the generator**

Create `internal/naming/sessionid.go`:

```go
package naming

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewClaudeSessionID returns a random RFC 4122 version-4 UUID string, used as a
// stable Claude Code session identifier for one fleet session. It panics only
// if the system CSPRNG is unavailable, which is unrecoverable.
func NewClaudeSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("naming: reading random bytes: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx
	s := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/naming/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/naming/sessionid.go internal/naming/sessionid_test.go
git commit -m "feat: add crypto/rand UUIDv4 generator for Claude session IDs"
```

---

### Task 4: Launch-command builders

**Files:**
- Create: `internal/session/command.go`
- Test: `internal/session/command_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces (package-private, used by Task 5):
  - `launchFresh(id, name string) string`
  - `launchResume(id, name string) string`
  - Both return `"claude"` when `id == ""`.

- [ ] **Step 1: Write the failing tests**

Create `internal/session/command_test.go`:

```go
package session

import "testing"

func TestLaunchFresh(t *testing.T) {
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
			if got := launchFresh(tt.id, tt.name); got != tt.want {
				t.Fatalf("launchFresh(%q,%q) = %q, want %q", tt.id, tt.name, got, tt.want)
			}
		})
	}
}

func TestLaunchResume(t *testing.T) {
	tests := []struct {
		desc, id, name, want string
	}{
		{"legacy empty id", "", "p/s", "claude"},
		{"id and name", "abc-123", "My App/fix bug",
			`claude --resume abc-123 || claude --session-id abc-123 -n 'My App/fix bug' || claude`},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := launchResume(tt.id, tt.name); got != tt.want {
				t.Fatalf("launchResume(%q,%q) = %q, want %q", tt.id, tt.name, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run 'TestLaunch'`
Expected: FAIL — compile error `undefined: launchFresh` / `undefined: launchResume`.

- [ ] **Step 3: Implement the builders**

Create `internal/session/command.go`:

```go
package session

import (
	"fmt"
	"strings"
)

// launchFresh builds the command for a brand-new session: create the
// conversation under a specific session ID and give it a display name. Using
// --session-id directly (not the resume chain) avoids a spurious "no
// conversation found" error flashing in the pane on first launch. A legacy
// session (empty id) launches bare claude.
func launchFresh(id, name string) string {
	if id == "" {
		return "claude"
	}
	return fmt.Sprintf("claude --session-id %s -n %s", id, shellQuote(name))
}

// launchResume builds the command for re-launching an existing session. It
// resumes the stored ID; if the session file is gone it recreates it under the
// same stable ID; and it finally falls back to a bare claude so the window is
// always usable. A legacy session (empty id) launches bare claude.
func launchResume(id, name string) string {
	if id == "" {
		return "claude"
	}
	return fmt.Sprintf(
		"claude --resume %s || claude --session-id %s -n %s || claude",
		id, id, shellQuote(name),
	)
}

// shellQuote wraps s in single quotes safe for `sh -c` (tmux runs the window
// command through the shell), escaping embedded single quotes as '\''.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -run 'TestLaunch'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/command.go internal/session/command_test.go
git commit -m "feat: add Claude launch-command builders with resume chain"
```

---

### Task 5: Wire the ID and builders into the session Manager

**Files:**
- Modify: `internal/session/manager.go` (add `newID` field; extend `NewManager`; use ID in `Create`; use resume chain in `EnsureRunning`; remove the `claudeCommand` const)
- Modify: `main.go:93` (pass `nil` for the new `newID` argument)
- Test: `internal/session/manager_test.go` (capture launch commands in `fakeTmux`; inject a fixed `newID` in the helper; add assertions)

**Interfaces:**
- Consumes: `meta.Meta.ClaudeSessionID` (Task 1), `session.Session.ClaudeSessionID` (Task 2), `naming.NewClaudeSessionID` (Task 3), `launchFresh`/`launchResume` (Task 4).
- Produces: `NewManager(cfg config.Config, t tmuxPort, g git.Git, f forge.PRer, clock func() time.Time, newID func() string) *Manager` — `newID` defaults to `naming.NewClaudeSessionID` when nil (mirrors the nil-`clock` default). `Create` stores a fresh ID and launches with `launchFresh`; `EnsureRunning` launches with `launchResume`.

- [ ] **Step 1: Update the test fake and helper, then add assertions**

In `internal/session/manager_test.go`:

(a) Capture launch commands in `fakeTmux`. Add two fields and record the command in each method:

```go
type fakeTmux struct {
	created       []string // window names created
	createdCmds   []string // launch commands passed to CreateWindow
	killed        []string // targets killed
	respawned     []string // targets respawned
	respawnedCmds []string // launch commands passed to RespawnWindow
	windows       map[string]tmux.Window
}

func (f *fakeTmux) CreateWindow(name, _, cmd string) (int, error) {
	f.created = append(f.created, name)
	f.createdCmds = append(f.createdCmds, cmd)
	if f.windows == nil {
		f.windows = map[string]tmux.Window{}
	}
	idx := len(f.windows) + 1
	f.windows[name] = tmux.Window{Index: idx, Name: name}
	return idx, nil
}

func (f *fakeTmux) RespawnWindow(target, _, cmd string) error {
	f.respawned = append(f.respawned, target)
	f.respawnedCmds = append(f.respawnedCmds, cmd)
	return nil
}
```

(b) Inject a deterministic `newID` in the helper:

```go
func newManager(t *testing.T, g git.Git, tm tmuxPort) (*Manager, config.Config) {
	cfg := config.Config{ScanRoot: "/code", WorktreeBaseDir: t.TempDir()}
	fixed := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	m := NewManager(cfg, tm, g, nil,
		func() time.Time { return fixed },
		func() string { return "test-session-id" },
	)
	return m, cfg
}
```

(c) Extend `TestCreateAddsWorktreeMetaAndTmux` — after the existing `md` checks, assert the ID is stored and the fresh command was launched:

```go
	if md.ClaudeSessionID != "test-session-id" {
		t.Fatalf("expected stored session ID, got %q", md.ClaudeSessionID)
	}
	if s.ClaudeSessionID != "test-session-id" {
		t.Fatalf("expected session ID on returned session, got %q", s.ClaudeSessionID)
	}
	wantCmd := "claude --session-id test-session-id -n 'My App/fix-bug'"
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != wantCmd {
		t.Fatalf("create launch cmd = %v, want %q", ft.createdCmds, wantCmd)
	}
```

(d) Add three new tests:

```go
func TestEnsureRunningResumesWhenMissing(t *testing.T) {
	ft := &fakeTmux{}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", ClaudeSessionID: "sid-1"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	want := "claude --resume sid-1 || claude --session-id sid-1 -n 'p/s' || claude"
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != want {
		t.Fatalf("missing-window cmd = %v, want %q", ft.createdCmds, want)
	}
}

func TestEnsureRunningRespawnUsesResumeChain(t *testing.T) {
	ft := &fakeTmux{windows: map[string]tmux.Window{"fleet-p-s": {Index: 1, Name: "fleet-p-s", Dead: true}}}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", ClaudeSessionID: "sid-1"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	want := "claude --resume sid-1 || claude --session-id sid-1 -n 'p/s' || claude"
	if len(ft.respawnedCmds) != 1 || ft.respawnedCmds[0] != want {
		t.Fatalf("respawn cmd = %v, want %q", ft.respawnedCmds, want)
	}
}

func TestEnsureRunningLegacyLaunchesBareClaude(t *testing.T) {
	ft := &fakeTmux{}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt"} // no ClaudeSessionID
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != "claude" {
		t.Fatalf("legacy cmd = %v, want [claude]", ft.createdCmds)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/`
Expected: FAIL — compile error: `NewManager` called with 6 args but defined with 5 (`not enough arguments`).

- [ ] **Step 3: Update the Manager**

In `internal/session/manager.go`:

(a) Delete the `claudeCommand` const:

```go
// remove this line:
const claudeCommand = "claude"
```

(b) Add the `newID` field to the struct:

```go
type Manager struct {
	cfg   config.Config
	tmux  tmuxPort
	git   git.Git
	forge forge.PRer
	clock func() time.Time
	newID func() string
}
```

(c) Extend `NewManager` (defaults `newID` like `clock`):

```go
func NewManager(cfg config.Config, t tmuxPort, g git.Git, f forge.PRer, clock func() time.Time, newID func() string) *Manager {
	if clock == nil {
		clock = time.Now
	}
	if newID == nil {
		newID = naming.NewClaudeSessionID
	}
	return &Manager{cfg: cfg, tmux: t, git: g, forge: f, clock: clock, newID: newID}
}
```

(d) In `Create`, generate the ID, store it in meta and on the session, and launch with `launchFresh`. Replace the body from `now := m.clock()` onward:

```go
	now := m.clock()
	sessionID := m.newID()
	md := meta.Meta{
		Project: p.Name, Session: name, Branch: branch, Base: base,
		RepoPath: p.Path, CreatedAt: now, ClaudeSessionID: sessionID,
	}
	if err := meta.Write(wt, md); err != nil {
		return Session{}, err
	}
	wname := naming.TmuxName(p.Name, name)
	idx, err := m.tmux.CreateWindow(wname, wt, launchFresh(sessionID, p.Name+"/"+name))
	if err != nil {
		return Session{}, err
	}
	return Session{
		Project: p.Name, Name: name, Branch: branch, Base: base,
		RepoPath: p.Path, WorktreePath: wt, TmuxName: wname,
		CreatedAt: now, Alive: true, WindowIndex: idx,
		ClaudeSessionID: sessionID,
	}, nil
```

(e) In `EnsureRunning`, build the resume command once and use it for both branches:

```go
func (m *Manager) EnsureRunning(s Session) error {
	cmd := launchResume(s.ClaudeSessionID, s.Project+"/"+s.Name)
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

- [ ] **Step 4: Update the production call site**

In `main.go` line 93, pass `nil` for the new argument so it defaults to the real generator:

```go
	mgr := session.NewManager(cfg, tm, g, fg, time.Now, nil)
```

- [ ] **Step 5: Run the full suite and build**

Run: `go build ./... && go test ./...`
Expected: PASS — everything compiles and all package tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/session/manager.go internal/session/manager_test.go main.go
git commit -m "feat: launch Claude with stable session ID and resume on re-launch"
```

---

### Task 6: Final verification and doc sync

**Files:**
- Modify: `CLAUDE.md` (note session resume in the Status section)

**Interfaces:**
- Consumes: the whole feature. Produces: none (verification + docs).

- [ ] **Step 1: Format and vet**

Run: `gofmt -l . && go vet ./...`
Expected: `gofmt -l .` prints nothing; `go vet` prints nothing (both clean). If `gofmt -l .` lists files, run `gofmt -w <files>` and re-check.

- [ ] **Step 2: Full test run**

Run: `go test ./...`
Expected: PASS for every package.

- [ ] **Step 3: Manual smoke check (optional but recommended)**

With `git`, `tmux`, `claude` on PATH and a configured `scan_root`, run `go run .`, create a session, type a message in it, detach, kill its window from the dashboard (Leave), then re-attach. Expected: the prior conversation is present (resumed), not a blank session. Confirm `claude --session-id <uuid> -n 'x/y'` is accepted by the installed `claude` (no flag error) — this is the one runtime assumption the unit tests can't cover.

- [ ] **Step 4: Sync the Status note in CLAUDE.md**

In `CLAUDE.md`, under `## Status`, add a sentence after the self-update paragraph:

```markdown
Session resume is implemented: each session is created with a stable Claude
Code session ID (stored in `.fleet/meta.json`) and launched with `--session-id`;
every re-launch (respawn or after a fleet/machine restart) resumes that ID, so
conversation context survives. Sessions created before this feature launch a
fresh `claude`.
```

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: note Claude session resume in project status"
```

---

## Self-Review

**Spec coverage:**
- Data model (`meta.Meta` + `session.Session` + refresher copy) → Tasks 1, 2. ✓
- Launch-command builders (fresh `--session-id`; re-launch `--resume … || --session-id … || claude`; empty-id → bare `claude`) → Task 4, wired in Task 5. ✓
- ID generation (`crypto/rand` UUIDv4, injected as `newID`, defaults like `clock`) → Tasks 3, 5. ✓
- "New sessions only" (empty ID → bare claude) → covered by builder empty-id branch, tested in Task 5 `TestEnsureRunningLegacyLaunchesBareClaude`. ✓
- Display name `-n '<project/session>'`, raw + shell-quoted → Task 4 (`shellQuote`) + Task 5 (`p.Name+"/"+name`). ✓
- Testing list (builders, meta round-trip, Create stores ID + fresh cmd, EnsureRunning resume/legacy, refresher copies ID) → Tasks 1–5. ✓
- Non-goals (no `--fork-session`, no `--continue`, no backfill) → nothing in the plan introduces them. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code and every command shows expected output. ✓

**Type consistency:** `NewClaudeSessionID` (Task 3) matches its use as the default `newID` (Task 5). `launchFresh`/`launchResume` signatures (Task 4) match their calls in `Create`/`EnsureRunning` (Task 5). `ClaudeSessionID` field name is identical across `meta.Meta`, `session.Session`, the refresher copy, and the manager. The `NewManager` 6-arg signature in Task 5 matches the helper call and the `main.go` update. ✓
