# Fleet Tab Switching + Richer Session Info Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Switch between live Claude sessions instantly via a shared tmux workspace (windows-as-tabs, Alt-key switching) and show richer, grouped per-session info with a four-state activity indicator.

**Architecture:** Replace "one tmux session per fleet session" with one shared `fleet-workspace` tmux session holding one window per fleet session. Switching is tmux's native `select-window` bound to Alt keys; the tab strip is tmux's window-list status bar. A new pure `activity` package classifies each session as working/waiting/idle/exited from tmux's activity timestamp plus a best-effort capture-pane prompt match. The dashboard is rewritten to a grouped, two-line-per-session layout with tab numbers, base branch, activity glyphs, and a legend.

**Tech Stack:** Go, Bubble Tea + Lip Gloss (TUI), tmux CLI adapter, git CLI adapter.

**Reference spec:** `docs/superpowers/specs/2026-06-16-fleet-tab-switching-design.md`

**Conventions to follow (from CLAUDE.md):** TDD (test first). Keep adapters thin and behind interfaces. tmux integration tests skip when tmux is absent (`requireTmux` helper in `internal/tmux/tmux_test.go`). Surface errors in the UI status line; never panic on malformed worktree/meta. Run `go build ./...` and `go test ./...` frequently.

---

## File Structure

- **Create** `internal/activity/activity.go` — `State` enum + `Classify` + `Glyph`/`TmuxColor` helpers. Pure, no I/O.
- **Create** `internal/activity/activity_test.go` — unit tests for the classifier.
- **Modify** `internal/naming/naming.go` — add `Workspace` constant and `WindowTarget` helper.
- **Modify** `internal/naming/naming_test.go` — test `WindowTarget`.
- **Modify** `internal/tmux/tmux.go` — add `Window` type and window/workspace methods.
- **Modify** `internal/tmux/tmux_test.go` — integration tests for window lifecycle.
- **Modify** `internal/session/session.go` — add `Activity`, `LastActivity`, `WindowIndex` fields.
- **Modify** `internal/session/manager.go` — rewire lifecycle to windows; widen `tmuxPort`.
- **Modify** `internal/session/manager_test.go` — update the `fakeTmux` and assertions.
- **Modify** `internal/refresher/refresher.go` — widen the tmux port, map windows, classify activity, set labels; add a clock param.
- **Modify** `internal/refresher/refresher_test.go` — update fake + assert activity.
- **Modify** `internal/refresher/smoke_test.go` — update for the windows model.
- **Modify** `internal/ui/views.go` — grouped dashboard + legend + activity styles.
- **Modify** `internal/ui/model_test.go` — sample sessions gain activity; view assertions.
- **Modify** `main.go` — wire EnsureRunning/ConfigureTabs/SelectWindow/AttachWorkspaceCmd and the refresher clock.
- **Modify** `CLAUDE.md` — update the session-model description (windows in one tmux session).

---

## Task 1: `activity` package — pure classifier

**Files:**
- Create: `internal/activity/activity.go`
- Test: `internal/activity/activity_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/activity/activity_test.go`:

```go
package activity

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		lastActivity time.Time
		paneTail     string
		missing      bool // no window at all
		dead         bool // window exists but process exited
		want         State
	}{
		{"missing window is exited", time.Time{}, "", true, false, Exited},
		{"dead window is exited", now, "anything", false, true, Exited},
		{"recent output is working", now.Add(-1 * time.Second), "Running tests...", false, false, Working},
		{"quiet with prompt is waiting", now.Add(-30 * time.Second), "Do you want to proceed?\n❯ 1. Yes", false, false, Waiting},
		{"quiet without prompt is idle", now.Add(-30 * time.Second), "all done. 4 passed", false, false, Idle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.lastActivity, now, c.paneTail, c.missing, c.dead)
			if got != c.want {
				t.Fatalf("Classify = %v, want %v", got, c.want)
			}
		})
	}
}

func TestGlyphAndColor(t *testing.T) {
	if Working.Glyph() != "◉" || Exited.Glyph() != "○" {
		t.Fatalf("unexpected glyphs: %q %q", Working.Glyph(), Exited.Glyph())
	}
	if Waiting.TmuxColor() == "" || Working.TmuxColor() == "" {
		t.Fatal("expected non-empty tmux colors for working/waiting")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/activity/`
Expected: FAIL — package/`State`/`Classify` undefined (build error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/activity/activity.go`:

```go
// Package activity classifies a session's live state from cheap tmux signals:
// the window's last-activity timestamp, whether its process has exited, and a
// best-effort match of Claude's input prompt in the captured pane tail.
//
// This package is the ONLY place that knows what Claude's prompt looks like, so
// it is the single spot to update if Claude's TUI changes.
package activity

import (
	"strings"
	"time"
)

// State is a session's live activity state.
type State int

const (
	Idle    State = iota // quiet, nothing pending
	Working              // produced output recently
	Waiting              // quiet AND a Claude input prompt is showing
	Exited               // the process is gone (or no window exists)
)

// workingWindow is how recently output must have happened to count as "working".
const workingWindow = 5 * time.Second

// promptMarkers are substrings that indicate Claude is waiting for input.
// Best-effort and intentionally centralized; update here if the TUI changes.
var promptMarkers = []string{
	"❯ 1.",          // numbered choice prompt
	"Do you want",   // confirmation prompt
	"(y/n)",         // yes/no prompt
}

// Classify decides a session's state. missing means no window exists for it;
// dead means the window exists but its process has exited.
func Classify(lastActivity, now time.Time, paneTail string, missing, dead bool) State {
	if missing || dead {
		return Exited
	}
	if now.Sub(lastActivity) <= workingWindow {
		return Working
	}
	for _, mark := range promptMarkers {
		if strings.Contains(paneTail, mark) {
			return Waiting
		}
	}
	return Idle
}

// Glyph returns the single-rune indicator for the state.
func (s State) Glyph() string {
	if s == Exited {
		return "○"
	}
	return "◉"
}

// TmuxColor returns a tmux colour name for the state (for status-bar labels).
func (s State) TmuxColor() string {
	switch s {
	case Working:
		return "colour42" // green
	case Waiting:
		return "colour220" // yellow
	case Exited:
		return "colour238" // dim
	default:
		return "colour244" // grey
	}
}

// Label returns the human word for the state, used in the dashboard detail line.
func (s State) Label() string {
	switch s {
	case Working:
		return "working"
	case Waiting:
		return "waiting for input"
	case Exited:
		return "exited"
	default:
		return "idle"
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/activity/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/activity/
git commit -m "feat(activity): pure session activity classifier"
```

---

## Task 2: `naming` — workspace constant + window target helper

**Files:**
- Modify: `internal/naming/naming.go`
- Test: `internal/naming/naming_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/naming/naming_test.go` (new test function; leave existing tests untouched):

```go
func TestWorkspaceAndWindowTarget(t *testing.T) {
	if Workspace != "fleet-workspace" {
		t.Fatalf("Workspace = %q", Workspace)
	}
	// Window target is workspace:windowName, where windowName is the usual
	// sanitized fleet name.
	got := WindowTarget("My App", "fix bug")
	want := "fleet-workspace:fleet-My_App-fix_bug"
	if got != want {
		t.Fatalf("WindowTarget = %q, want %q", got, want)
	}
}
```

(If `naming_test.go` lacks a `testing` import or package clause, it already has them — it's an existing file in package `naming`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/naming/`
Expected: FAIL — `Workspace` / `WindowTarget` undefined.

- [ ] **Step 3: Write minimal implementation**

In `internal/naming/naming.go`, add after the `prefix` const:

```go
// Workspace is the single shared tmux session that holds one window per fleet
// session. Windows-as-tabs live here.
const Workspace = "fleet-workspace"
```

And add a function (e.g. after `TmuxName`):

```go
// WindowTarget returns the tmux target ("session:window") addressing a
// project/session's window inside the shared workspace.
func WindowTarget(project, session string) string {
	return Workspace + ":" + TmuxName(project, session)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/naming/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/naming/
git commit -m "feat(naming): workspace constant and window target helper"
```

---

## Task 3: `tmux` — Window type, lifecycle, queries

**Files:**
- Modify: `internal/tmux/tmux.go`
- Test: `internal/tmux/tmux_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tmux/tmux_test.go`:

```go
func TestWorkspaceWindowLifecycle(t *testing.T) {
	requireTmux(t)
	c := New()
	_ = c.KillWorkspace() // clean start

	// First window bootstraps the workspace at index 1.
	idx, err := c.CreateWindow("fleet-proj-one", t.TempDir(), "sleep 30")
	if err != nil {
		t.Fatalf("create window: %v", err)
	}
	if idx != 1 {
		t.Fatalf("first window index = %d, want 1", idx)
	}
	t.Cleanup(func() { _ = c.KillWorkspace() })

	// Second window gets index 2.
	idx2, err := c.CreateWindow("fleet-proj-two", t.TempDir(), "sleep 30")
	if err != nil {
		t.Fatalf("create window 2: %v", err)
	}
	if idx2 != 2 {
		t.Fatalf("second window index = %d, want 2", idx2)
	}

	ws, err := c.ListWindows()
	if err != nil {
		t.Fatalf("list windows: %v", err)
	}
	if len(ws) != 2 {
		t.Fatalf("expected 2 windows, got %d: %+v", len(ws), ws)
	}

	w, ok := c.LookupWindow("fleet-proj-one")
	if !ok || w.Index != 1 || w.Dead {
		t.Fatalf("lookup proj-one = %+v ok=%v", w, ok)
	}

	// Capturing a pane returns its text without error.
	if _, err := c.CapturePane("fleet-workspace:fleet-proj-one"); err != nil {
		t.Fatalf("capture: %v", err)
	}

	// Setting a label must not error.
	if err := c.SetWindowLabel("fleet-workspace:fleet-proj-one", "◉ proj/one"); err != nil {
		t.Fatalf("set label: %v", err)
	}

	// Killing a window drops it; renumber-windows keeps the rest contiguous.
	if err := c.KillWindow("fleet-workspace:fleet-proj-one"); err != nil {
		t.Fatalf("kill window: %v", err)
	}
	if _, ok := c.LookupWindow("fleet-proj-one"); ok {
		t.Fatal("expected proj-one gone after kill")
	}
}

func TestListWindowsNoWorkspace(t *testing.T) {
	requireTmux(t)
	c := New()
	_ = c.KillWorkspace()
	ws, err := c.ListWindows()
	if err != nil {
		t.Fatalf("expected no error when workspace absent, got %v", err)
	}
	if len(ws) != 0 {
		t.Fatalf("expected 0 windows, got %+v", ws)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux/ -run TestWorkspace`
Expected: FAIL — `Window`, `CreateWindow`, etc. undefined.

- [ ] **Step 3: Write minimal implementation**

In `internal/tmux/tmux.go`, add the import for `strconv` and `time` (the file already imports `bytes`, `fmt`, `os/exec`, `strings`):

```go
import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)
```

Add the `Window` type near `Session`:

```go
// Window is one window in the shared fleet workspace session.
type Window struct {
	Index        int
	Name         string    // stable identity: the fleet-<proj>-<sess> name
	Dead         bool      // process exited but window kept by remain-on-exit
	LastActivity time.Time
}
```

Add these methods to `*CLI` (use the existing private `c.tmux(...)` helper and the `naming.Workspace` value — but to avoid an import cycle, hardcode the constant here as a package-local; see note below):

```go
// workspace is the shared session name. Kept local to avoid importing naming
// (which would create a cycle if naming ever depends on tmux); it must match
// naming.Workspace.
const workspace = "fleet-workspace"

func (c *CLI) hasSession(name string) bool {
	_, err := c.tmux("has-session", "-t", name)
	return err == nil
}

// KillWorkspace removes the whole shared session (used by tests and when the
// last window is gone). Killing an absent session is not treated as an error.
func (c *CLI) KillWorkspace() error {
	if !c.hasSession(workspace) {
		return nil
	}
	_, err := c.tmux("kill-session", "-t", workspace)
	return err
}

// CreateWindow ensures the workspace exists and adds a window named name
// running command in workdir. Returns the new window's 1-based index.
func (c *CLI) CreateWindow(name, workdir, command string) (int, error) {
	if !c.hasSession(workspace) {
		if _, err := c.tmux("new-session", "-d", "-s", workspace, "-n", name, "-c", workdir, command); err != nil {
			return 0, err
		}
		for _, opt := range [][]string{
			{"set-option", "-t", workspace, "base-index", "1"},
			{"set-option", "-t", workspace, "renumber-windows", "on"},
			{"set-option", "-t", workspace, "automatic-rename", "off"},
			{"set-option", "-t", workspace, "remain-on-exit", "on"},
		} {
			if _, err := c.tmux(opt...); err != nil {
				return 0, err
			}
		}
		// Renumber so windows start at base-index (1), regardless of the user's
		// global base-index. The single bootstrap window becomes index 1, so
		// Alt-1 selects it.
		if _, err := c.tmux("move-window", "-r", "-t", workspace); err != nil {
			return 0, err
		}
		return 1, nil
	}
	out, err := c.tmux("new-window", "-P", "-F", "#{window_index}", "-t", workspace, "-n", name, "-c", workdir, command)
	if err != nil {
		return 0, err
	}
	idx, _ := strconv.Atoi(strings.TrimSpace(out))
	return idx, nil
}

// ListWindows returns the workspace's windows. An absent workspace (or no tmux
// server) is an empty list, not an error.
func (c *CLI) ListWindows() ([]Window, error) {
	out, err := c.tmux("list-windows", "-t", workspace, "-F",
		"#{window_index}\t#{window_name}\t#{pane_dead}\t#{window_activity}")
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "no server running") ||
			strings.Contains(msg, "can't find session") ||
			strings.Contains(msg, "no sessions") {
			return nil, nil
		}
		return nil, err
	}
	var ws []Window
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		idx, _ := strconv.Atoi(f[0])
		secs, _ := strconv.ParseInt(f[3], 10, 64)
		ws = append(ws, Window{
			Index:        idx,
			Name:         f[1],
			Dead:         f[2] == "1",
			LastActivity: time.Unix(secs, 0),
		})
	}
	return ws, nil
}

// LookupWindow finds a window by its stable name. ok is false if absent.
func (c *CLI) LookupWindow(name string) (Window, bool) {
	ws, err := c.ListWindows()
	if err != nil {
		return Window{}, false
	}
	for _, w := range ws {
		if w.Name == name {
			return w, true
		}
	}
	return Window{}, false
}

// KillWindow removes a window by target ("workspace:name").
func (c *CLI) KillWindow(target string) error {
	_, err := c.tmux("kill-window", "-t", target)
	return err
}

// RespawnWindow restarts the process in an existing (dead) window.
func (c *CLI) RespawnWindow(target, workdir, command string) error {
	_, err := c.tmux("respawn-window", "-t", target, "-c", workdir, command)
	return err
}

// CapturePane returns the visible text of a window's active pane.
func (c *CLI) CapturePane(target string) (string, error) {
	return c.tmux("capture-pane", "-p", "-t", target)
}

// SetWindowLabel sets the @fleet_label window option used to render the tab.
func (c *CLI) SetWindowLabel(target, label string) error {
	_, err := c.tmux("set-option", "-w", "-t", target, "@fleet_label", label)
	return err
}
```

> **Import-cycle note:** `naming` does not import `tmux`, so `tmux` *could* import `naming`. But to keep the adapter self-contained we duplicate the `workspace` constant here with a comment that it must match `naming.Workspace`. Task 2's test pins `naming.Workspace == "fleet-workspace"`; this constant must equal it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tmux/ -run 'TestWorkspace|TestListWindowsNoWorkspace'`
Expected: PASS (or SKIP if tmux is absent).

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/
git commit -m "feat(tmux): workspace window lifecycle and queries"
```

---

## Task 4: `tmux` — attach, select, and tab configuration

**Files:**
- Modify: `internal/tmux/tmux.go`
- Test: `internal/tmux/tmux_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/tmux/tmux_test.go`:

```go
func TestAttachWorkspaceCmdShape(t *testing.T) {
	c := New()
	cmd := c.AttachWorkspaceCmd()
	if cmd.Args[0] != "tmux" || cmd.Args[1] != "attach" {
		t.Fatalf("unexpected attach command: %v", cmd.Args)
	}
	found := false
	for _, a := range cmd.Args {
		if a == "fleet-workspace" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected attach to target fleet-workspace: %v", cmd.Args)
	}
}

func TestConfigureTabsAndSelect(t *testing.T) {
	requireTmux(t)
	c := New()
	_ = c.KillWorkspace()
	if _, err := c.CreateWindow("fleet-proj-one", t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = c.KillWorkspace() })

	if err := c.ConfigureTabs(); err != nil {
		t.Fatalf("configure tabs: %v", err)
	}
	// window-status-format should reference our label option.
	out, err := exec.Command("tmux", "show-options", "-t", "fleet-workspace", "-v", "window-status-format").Output()
	if err != nil {
		t.Fatalf("show-options: %v", err)
	}
	if !strings.Contains(string(out), "@fleet_label") {
		t.Fatalf("window-status-format = %q, expected to reference @fleet_label", out)
	}
	if err := c.SelectWindow(1); err != nil {
		t.Fatalf("select window: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tmux/ -run 'TestAttachWorkspaceCmdShape|TestConfigureTabsAndSelect'`
Expected: FAIL — `AttachWorkspaceCmd`, `ConfigureTabs`, `SelectWindow` undefined.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/tmux/tmux.go`:

```go
// SelectWindow makes the given 1-based window index active in the workspace.
func (c *CLI) SelectWindow(index int) error {
	_, err := c.tmux("select-window", "-t", fmt.Sprintf("%s:%d", workspace, index))
	return err
}

// AttachWorkspaceCmd returns the command to attach to the shared workspace, for
// use with tea.ExecProcess. Select the target window first via SelectWindow.
func (c *CLI) AttachWorkspaceCmd() *exec.Cmd {
	return exec.Command("tmux", "attach", "-t", workspace)
}

// ConfigureTabs sets the workspace status bar to render windows as numbered
// tabs (using each window's @fleet_label) and binds prefix-less switch keys:
// Alt-1..9 jump to a tab, Alt-Left/Right move prev/next. Best-effort; it
// returns the first error encountered.
func (c *CLI) ConfigureTabs() error {
	prefix := c.prefixKey()
	opts := [][]string{
		{"set-option", "-t", workspace, "status", "on"},
		{"set-option", "-t", workspace, "status-style", "bg=colour237,fg=colour250"},
		{"set-option", "-t", workspace, "status-left", " #[bold]fleet#[nobold] "},
		{"set-option", "-t", workspace, "status-left-length", "20"},
		{"set-option", "-t", workspace, "status-right", fmt.Sprintf(" %s d → dashboard ", prefix)},
		{"set-option", "-t", workspace, "status-right-length", "40"},
		{"set-option", "-t", workspace, "window-status-format", " #{window_index} #{@fleet_label} "},
		{"set-option", "-t", workspace, "window-status-current-format", "#[reverse] #{window_index} #{@fleet_label} #[noreverse]"},
	}
	for _, o := range opts {
		if _, err := c.tmux(o...); err != nil {
			return err
		}
	}
	// Prefix-less switch keys (global to this tmux server; fleet owns it).
	for i := 1; i <= 9; i++ {
		n := strconv.Itoa(i)
		if _, err := c.tmux("bind-key", "-n", "M-"+n, "select-window", "-t", ":"+n); err != nil {
			return err
		}
	}
	if _, err := c.tmux("bind-key", "-n", "M-Left", "previous-window"); err != nil {
		return err
	}
	if _, err := c.tmux("bind-key", "-n", "M-Right", "next-window"); err != nil {
		return err
	}
	return nil
}
```

> `prefixKey()` already exists in `tmux.go`. The legacy `Decorate`, `Create`, `Kill`, `Has`, `List`, and `AttachCmd` methods remain (still covered by their existing tests) but are no longer used by production code after Task 8.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tmux/`
Expected: PASS (tmux-gated tests SKIP if tmux is absent).

- [ ] **Step 5: Commit**

```bash
git add internal/tmux/
git commit -m "feat(tmux): tab strip configuration and workspace attach/select"
```

---

## Task 5: `session` model — activity fields

**Files:**
- Modify: `internal/session/session.go`

- [ ] **Step 1: Write the failing test**

Add a small compile-time test to `internal/session/manager_test.go` (it's in package `session`):

```go
func TestSessionHasActivityFields(t *testing.T) {
	s := Session{
		Activity:     activity.Working,
		LastActivity: time.Unix(5, 0),
		WindowIndex:  2,
	}
	if s.Activity != activity.Working || s.WindowIndex != 2 {
		t.Fatalf("unexpected session fields: %+v", s)
	}
}
```

Add the import `"github.com/bray/fleet/internal/activity"` to `manager_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/session/ -run TestSessionHasActivityFields`
Expected: FAIL — unknown fields `Activity`, `LastActivity`, `WindowIndex`.

- [ ] **Step 3: Write minimal implementation**

In `internal/session/session.go`, add the import and three fields:

```go
import (
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/git"
)
```

```go
type Session struct {
	Project      string
	Name         string
	Branch       string
	Base         string
	RepoPath     string
	WorktreePath string
	TmuxName     string // the stable window name inside the workspace
	CreatedAt    time.Time
	Alive        bool // window exists and process is running
	Exited       bool // worktree exists but window is missing or dead
	Activity     activity.State
	LastActivity time.Time
	WindowIndex  int // 1-based tab number; 0 if no live window
	Git          git.Status
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/session/ -run TestSessionHasActivityFields`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/
git commit -m "feat(session): activity, last-activity, and window-index fields"
```

---

## Task 6: `session` manager — window-based lifecycle

**Files:**
- Modify: `internal/session/manager.go`
- Test: `internal/session/manager_test.go`

- [ ] **Step 1: Write the failing test**

Replace the `fakeTmux` and lifecycle assertions in `internal/session/manager_test.go`. New `fakeTmux`:

```go
type fakeTmux struct {
	created   []string // window names created
	killed    []string // targets killed
	respawned []string // targets respawned
	windows   map[string]tmux.Window
}

func (f *fakeTmux) CreateWindow(name, _, _ string) (int, error) {
	f.created = append(f.created, name)
	if f.windows == nil {
		f.windows = map[string]tmux.Window{}
	}
	idx := len(f.windows) + 1
	f.windows[name] = tmux.Window{Index: idx, Name: name}
	return idx, nil
}
func (f *fakeTmux) KillWindow(target string) error {
	f.killed = append(f.killed, target)
	return nil
}
func (f *fakeTmux) RespawnWindow(target, _, _ string) error {
	f.respawned = append(f.respawned, target)
	return nil
}
func (f *fakeTmux) LookupWindow(name string) (tmux.Window, bool) {
	w, ok := f.windows[name]
	return w, ok
}
```

Add imports `"github.com/bray/fleet/internal/tmux"` (and keep `activity`, `time` from Task 5).

Update the affected tests:

```go
func TestCreateAddsWorktreeMetaAndTmux(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, cfg := newManager(t, fg, ft)

	proj := projects.Project{Name: "My App", Path: "/code/my-app", DefaultBranch: "main"}
	s, err := m.Create(proj, "fix-bug", "fleet/fix-bug", "main")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(fg.added) != 1 {
		t.Fatalf("expected 1 worktree add, got %v", fg.added)
	}
	if len(ft.created) != 1 || ft.created[0] != "fleet-My_App-fix_bug" {
		t.Fatalf("unexpected window create: %v", ft.created)
	}
	md, err := meta.Read(s.WorktreePath)
	if err != nil {
		t.Fatalf("meta read: %v", err)
	}
	if md.Branch != "fleet/fix-bug" || md.Base != "main" || md.RepoPath != "/code/my-app" {
		t.Fatalf("unexpected meta: %+v", md)
	}
	if s.TmuxName != "fleet-My_App-fix_bug" || !s.Alive || s.WindowIndex != 1 {
		t.Fatalf("unexpected session: %+v", s)
	}
	_ = cfg
}

func TestLeaveKillsWindowOnly(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", RepoPath: "/r", Branch: "fleet/s"}
	if err := m.Leave(s); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if len(ft.killed) != 1 || ft.killed[0] != "fleet-workspace:fleet-p-s" || len(fg.removed) != 0 {
		t.Fatalf("leave should kill window only: killed=%v removed=%v", ft.killed, fg.removed)
	}
}

func TestDeleteKillsRemovesAndOptionallyDropsBranch(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", RepoPath: "/r", Branch: "fleet/s"}

	if err := m.Delete(s, false); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(fg.removed) != 1 || len(fg.deleted) != 0 {
		t.Fatalf("expected remove only: removed=%v deleted=%v", fg.removed, fg.deleted)
	}
	if err := m.Delete(s, true); err != nil {
		t.Fatalf("delete+branch: %v", err)
	}
	if len(fg.deleted) != 1 || fg.deleted[0] != "fleet/s" {
		t.Fatalf("expected branch delete, got %v", fg.deleted)
	}
}

func TestEnsureRunningNoopWhenAlive(t *testing.T) {
	ft := &fakeTmux{windows: map[string]tmux.Window{"fleet-p-s": {Index: 1, Name: "fleet-p-s"}}}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.created) != 0 || len(ft.respawned) != 0 {
		t.Fatalf("expected no create/respawn for a live session: created=%v respawned=%v", ft.created, ft.respawned)
	}
}

func TestEnsureRunningCreatesWhenMissing(t *testing.T) {
	ft := &fakeTmux{}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.created) != 1 || ft.created[0] != "fleet-p-s" {
		t.Fatalf("expected window create for a missing session, got %v", ft.created)
	}
}

func TestEnsureRunningRespawnsWhenDead(t *testing.T) {
	ft := &fakeTmux{windows: map[string]tmux.Window{"fleet-p-s": {Index: 1, Name: "fleet-p-s", Dead: true}}}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.respawned) != 1 || ft.respawned[0] != "fleet-workspace:fleet-p-s" {
		t.Fatalf("expected respawn for a dead window, got %v", ft.respawned)
	}
}
```

(Delete the old `TestLeaveKillsTmuxOnly` and `TestEnsureRunningRecreatesWhenDead`; they're replaced above.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/session/`
Expected: FAIL — `tmuxPort` mismatch / methods (`CreateWindow`, etc.) undefined on Manager usage.

- [ ] **Step 3: Write minimal implementation**

In `internal/session/manager.go`, replace the `tmuxPort`/`attacher` interfaces and the lifecycle methods. New port:

```go
// tmuxPort is the subset of the tmux adapter the manager uses for window-based
// session lifecycle.
type tmuxPort interface {
	CreateWindow(name, workdir, command string) (int, error)
	KillWindow(target string) error
	RespawnWindow(target, workdir, command string) error
	LookupWindow(name string) (tmux.Window, bool)
}
```

Add the import `"github.com/bray/fleet/internal/tmux"`. Remove the now-unused `attacher` interface and the `AttachCmd` method (attach is wired in `main.go` against the workspace now); remove `"os/exec"` if it becomes unused.

Rewrite the lifecycle methods:

```go
// Create makes the worktree, writes meta, and launches the session's window in
// the shared workspace.
func (m *Manager) Create(p projects.Project, name, branch, base string) (Session, error) {
	wt := naming.WorktreePath(m.cfg.WorktreeBaseDir, p.Name, name)
	if err := m.git.AddWorktree(p.Path, wt, branch, base); err != nil {
		return Session{}, err
	}
	if err := m.git.Ignore(wt, ".fleet/"); err != nil {
		return Session{}, err
	}
	now := m.clock()
	md := meta.Meta{
		Project: p.Name, Session: name, Branch: branch, Base: base,
		RepoPath: p.Path, CreatedAt: now,
	}
	if err := meta.Write(wt, md); err != nil {
		return Session{}, err
	}
	wname := naming.TmuxName(p.Name, name)
	idx, err := m.tmux.CreateWindow(wname, wt, claudeCommand)
	if err != nil {
		return Session{}, err
	}
	return Session{
		Project: p.Name, Name: name, Branch: branch, Base: base,
		RepoPath: p.Path, WorktreePath: wt, TmuxName: wname,
		CreatedAt: now, Alive: true, WindowIndex: idx,
	}, nil
}

// EnsureRunning makes sure the session has a live window, creating it if it is
// missing (e.g. a pre-upgrade session) or respawning it if its process exited.
// Safe to call right before attaching.
func (m *Manager) EnsureRunning(s Session) error {
	w, ok := m.tmux.LookupWindow(s.TmuxName)
	if !ok {
		_, err := m.tmux.CreateWindow(s.TmuxName, s.WorktreePath, claudeCommand)
		return err
	}
	if w.Dead {
		return m.tmux.RespawnWindow(naming.WindowTarget(s.Project, s.Name), s.WorktreePath, claudeCommand)
	}
	return nil
}

// Leave ends the running Claude instance but keeps the worktree and branch.
func (m *Manager) Leave(s Session) error {
	_ = m.tmux.KillWindow(naming.WindowTarget(s.Project, s.Name)) // ignore: may already be gone
	return nil
}

// Delete kills the session's window, removes the worktree, and optionally
// deletes the branch.
func (m *Manager) Delete(s Session, deleteBranch bool) error {
	_ = m.tmux.KillWindow(naming.WindowTarget(s.Project, s.Name))
	if err := m.git.RemoveWorktree(s.RepoPath, s.WorktreePath, true); err != nil {
		return err
	}
	if deleteBranch {
		if err := m.git.DeleteBranch(s.RepoPath, s.Branch, true); err != nil {
			return err
		}
	}
	return nil
}
```

Delete the old `Kill` and `AttachCmd` methods. `PushPR` is unchanged.

> Note: `Leave`/`Delete`/`EnsureRunning` build the target from `s.Project`/`s.Name`. The tests above set both fields plus `TmuxName`; production sessions come from the refresher which sets all three.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/session/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/
git commit -m "feat(session): window-based lifecycle in the shared workspace"
```

---

## Task 7: `refresher` — map windows, classify activity, set labels

**Files:**
- Modify: `internal/refresher/refresher.go`
- Test: `internal/refresher/refresher_test.go`

- [ ] **Step 1: Write the failing test**

Rewrite `internal/refresher/refresher_test.go`'s fake and test:

```go
package refresher

import (
	"testing"
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/tmux"
)

type fakeGit struct{ st git.Status }

func (f fakeGit) DefaultBranch(string) (string, error)     { return "main", nil }
func (f fakeGit) AddWorktree(_, _, _, _ string) error      { return nil }
func (f fakeGit) RemoveWorktree(_, _ string, _ bool) error { return nil }
func (f fakeGit) DeleteBranch(_, _ string, _ bool) error   { return nil }
func (f fakeGit) Status(string) (git.Status, error)        { return f.st, nil }
func (f fakeGit) Push(string, string) error                { return nil }
func (f fakeGit) IsRepo(string) bool                       { return true }
func (f fakeGit) Ignore(string, string) error              { return nil }

type fakeTmux struct {
	windows  []tmux.Window
	tails    map[string]string
	labels   map[string]string // target -> last label set
}

func (f *fakeTmux) ListWindows() ([]tmux.Window, error) { return f.windows, nil }
func (f *fakeTmux) CapturePane(target string) (string, error) {
	return f.tails[target], nil
}
func (f *fakeTmux) SetWindowLabel(target, label string) error {
	if f.labels == nil {
		f.labels = map[string]string{}
	}
	f.labels[target] = label
	return nil
}

func TestBuildDerivesSessionsAndActivity(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{ScanRoot: "/code", WorktreeBaseDir: base}
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	// Two worktrees: one live & working, one with no window (exited).
	wtA := naming.WorktreePath(base, "My App", "alive")
	_ = meta.Write(wtA, meta.Meta{
		Project: "My App", Session: "alive", Branch: "fleet/alive", Base: "main",
		RepoPath: "/code/my-app", CreatedAt: time.Unix(1, 0).UTC(),
	})
	wtD := naming.WorktreePath(base, "My App", "dead")
	_ = meta.Write(wtD, meta.Meta{
		Project: "My App", Session: "dead", Branch: "fleet/dead", Base: "main",
		RepoPath: "/code/my-app", CreatedAt: time.Unix(2, 0).UTC(),
	})

	nameAlive := naming.TmuxName("My App", "alive")
	ft := &fakeTmux{
		windows: []tmux.Window{
			{Index: 1, Name: nameAlive, Dead: false, LastActivity: now.Add(-1 * time.Second)},
		},
	}
	fg := fakeGit{st: git.Status{Branch: "fleet/alive", Dirty: true, ChangeCount: 3}}

	got, err := Build(cfg, ft, fg, func() time.Time { return now })
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d: %+v", len(got), got)
	}

	for _, s := range got {
		switch s.Name {
		case "alive":
			if !s.Alive || s.Exited || s.Activity != activity.Working || s.WindowIndex != 1 {
				t.Fatalf("alive session wrong: %+v", s)
			}
		case "dead":
			if s.Alive || !s.Exited || s.Activity != activity.Exited {
				t.Fatalf("dead session wrong: %+v", s)
			}
		}
	}

	// The live session should have had its tab label pushed.
	target := naming.WindowTarget("My App", "alive")
	if ft.labels[target] == "" {
		t.Fatalf("expected a label set for %q, got %v", target, ft.labels)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/refresher/`
Expected: FAIL — `Build` signature mismatch (4th arg) and tmux port methods undefined.

- [ ] **Step 3: Write minimal implementation**

Rewrite `internal/refresher/refresher.go`:

```go
// Package refresher rebuilds the live list of sessions from the filesystem
// (the set of worktrees with meta), tmux (window liveness + activity), and git
// (status).
package refresher

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/session"
	"github.com/bray/fleet/internal/tmux"
)

// workspaceTmux is the subset of the tmux adapter the refresher needs.
type workspaceTmux interface {
	ListWindows() ([]tmux.Window, error)
	CapturePane(target string) (string, error)
	SetWindowLabel(target, label string) error
}

// Build returns all sessions discovered under cfg.WorktreeBaseDir. Worktrees
// whose meta cannot be read are skipped. now supplies the clock for activity
// classification (pass time.Now in production).
func Build(cfg config.Config, t workspaceTmux, g git.Git, now func() time.Time) ([]session.Session, error) {
	base := cfg.WorktreeBaseDir
	projectDirs, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Index live windows by their stable name.
	windows := map[string]tmux.Window{}
	if wins, err := t.ListWindows(); err == nil {
		for _, w := range wins {
			windows[w.Name] = w
		}
	}

	var out []session.Session
	for _, pd := range projectDirs {
		if !pd.IsDir() {
			continue
		}
		sessDirs, err := os.ReadDir(filepath.Join(base, pd.Name()))
		if err != nil {
			continue
		}
		for _, sd := range sessDirs {
			if !sd.IsDir() {
				continue
			}
			wt := filepath.Join(base, pd.Name(), sd.Name())
			md, err := meta.Read(wt)
			if err != nil {
				continue
			}
			wname := naming.TmuxName(md.Project, md.Session)
			target := naming.WindowTarget(md.Project, md.Session)
			w, present := windows[wname]
			alive := present && !w.Dead

			var tail string
			if alive {
				tail, _ = t.CapturePane(target) // best-effort
			}
			state := activity.Classify(w.LastActivity, now(), tail, !present, w.Dead)

			st, err := g.Status(wt)
			if err != nil {
				st = git.Status{Branch: md.Branch}
			}

			s := session.Session{
				Project:      md.Project,
				Name:         md.Session,
				Branch:       md.Branch,
				Base:         md.Base,
				RepoPath:     md.RepoPath,
				WorktreePath: wt,
				TmuxName:     wname,
				CreatedAt:    md.CreatedAt,
				Alive:        alive,
				Exited:       !alive,
				Activity:     state,
				LastActivity: w.LastActivity,
				WindowIndex:  w.Index,
				Git:          st,
			}
			out = append(out, s)

			if alive {
				_ = t.SetWindowLabel(target, tabLabel(s)) // best-effort
			}
		}
	}
	return out, nil
}

// tabLabel builds the tmux tab text for a session: a coloured glyph, the
// project/session name, and a dirty marker.
func tabLabel(s session.Session) string {
	dirty := ""
	if s.Git.Dirty {
		dirty = fmt.Sprintf("✱%d", s.Git.ChangeCount)
	}
	return fmt.Sprintf("#[fg=%s]%s#[default] %s/%s%s",
		s.Activity.TmuxColor(), s.Activity.Glyph(), s.Project, s.Name, dirty)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/refresher/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/refresher/
git commit -m "feat(refresher): window mapping, activity classification, tab labels"
```

---

## Task 8: Update the smoke test for the windows model

**Files:**
- Modify: `internal/refresher/smoke_test.go`

- [ ] **Step 1: Update the test to use window APIs**

Replace the tmux create/kill and `Build` calls in `smoke_test.go`:

Change the session launch (was `tm.Create(tname, wt, "sleep 60")`):

```go
	_ = tm.KillWorkspace()
	if _, err := tm.CreateWindow(naming.TmuxName("myrepo", "smoke"), wt, "sleep 60"); err != nil {
		t.Fatalf("tmux create window: %v", err)
	}
	t.Cleanup(func() { _ = tm.KillWorkspace() })
```

Remove the now-unused `tname` variable (or keep it as `wname := naming.TmuxName("myrepo", "smoke")` and reuse it).

Update all three `Build(cfg, tm, g)` calls to pass a clock:

```go
	sessions, err := Build(cfg, tm, g, time.Now)
```

Replace the "kill tmux -> exited" step (was `tm.Kill(tname)`):

```go
	if err := tm.KillWindow(naming.WindowTarget("myrepo", "smoke")); err != nil {
		t.Fatalf("kill window: %v", err)
	}
```

(The final "remove worktree -> disappears" assertion is unchanged except for the `Build` signature.)

- [ ] **Step 2: Verify it builds (smoke tag)**

Run: `go vet -tags smoke ./internal/refresher/`
Expected: no errors. (Running the smoke test itself is optional: `go test -tags smoke -run Smoke ./internal/refresher/`.)

- [ ] **Step 3: Commit**

```bash
git add internal/refresher/smoke_test.go
git commit -m "test(refresher): update smoke test for the windows model"
```

---

## Task 9: `ui` — grouped dashboard, activity glyphs, legend

**Files:**
- Modify: `internal/ui/views.go`
- Test: `internal/ui/model_test.go`

- [ ] **Step 1: Write the failing test**

Update `sample()` in `internal/ui/model_test.go` to carry activity + window index, and add a view assertion. Replace `sample()`:

```go
func sample() []session.Session {
	return []session.Session{
		{Project: "app", Name: "a", Branch: "fleet/a", Base: "main", Alive: true,
			Activity: activity.Working, WindowIndex: 1,
			Git: git.Status{Branch: "fleet/a", ChangeCount: 1, Dirty: true}, CreatedAt: time.Unix(1, 0)},
		{Project: "app", Name: "b", Branch: "fleet/b", Base: "develop", Exited: true,
			Activity: activity.Exited, WindowIndex: 0, CreatedAt: time.Unix(2, 0)},
	}
}
```

Add the import `"github.com/bray/fleet/internal/activity"`.

Add a new test:

```go
func TestDashboardShowsGroupingTabNumbersAndLegend(t *testing.T) {
	m := New(nil, nil)
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	out := updated.(Model).View()

	for _, want := range []string{"app", "fleet/a", "← main", "1", "working", "exited", "legend"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dashboard view missing %q.\n---\n%s", want, out)
		}
	}
}
```

Add the import `"strings"` to `model_test.go` if not present (it is not currently — add it).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDashboardShowsGroupingTabNumbersAndLegend`
Expected: FAIL — view lacks grouping/legend/"working".

- [ ] **Step 3: Write minimal implementation**

In `internal/ui/views.go`, add activity styles and rewrite `viewDashboard`. Add imports `"github.com/bray/fleet/internal/activity"` and `"github.com/bray/fleet/internal/session"`.

Add styles near the existing `var (...)` block:

```go
var (
	workingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	waitingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	idleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	exitedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	projectStyle = lipgloss.NewStyle().Faint(true).Underline(true)
)

// activityStyle maps a state to its glyph style.
func activityStyle(s activity.State) lipgloss.Style {
	switch s {
	case activity.Working:
		return workingStyle
	case activity.Waiting:
		return waitingStyle
	case activity.Exited:
		return exitedStyle
	default:
		return idleStyle
	}
}

// glyph renders the coloured activity glyph for a session.
func glyph(s session.Session) string {
	return activityStyle(s.Activity).Render(s.Activity.Glyph())
}
```

Rewrite `viewDashboard`:

```go
func (m Model) viewDashboard() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("fleet — sessions") + "\n\n")
	if len(m.sessions) == 0 {
		b.WriteString(dimStyle.Render("no sessions. press n to create one.") + "\n")
	}

	lastProject := ""
	for i, s := range m.sessions {
		if s.Project != lastProject {
			b.WriteString(projectStyle.Render(s.Project) + "\n")
			lastProject = s.Project
		}

		// Tab number: the window index, or "-" when there is no live window.
		num := "-"
		if s.WindowIndex > 0 {
			num = fmt.Sprintf("%d", s.WindowIndex)
		}
		identity := fmt.Sprintf("%s %s %s  %s ← %s", num, glyph(s), s.Name, s.Branch, s.Base)
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("› "+identity) + "\n")
		} else {
			b.WriteString("  " + identity + "\n")
		}

		// Detail line: activity word, git state, age.
		detail := "    " + s.Activity.Label()
		if s.Git.Dirty {
			detail += fmt.Sprintf(" · ✱%d", s.Git.ChangeCount)
		} else {
			detail += " · clean"
		}
		if s.Git.Ahead > 0 || s.Git.Behind > 0 {
			detail += fmt.Sprintf(" · ↑%d↓%d", s.Git.Ahead, s.Git.Behind)
		}
		if !s.CreatedAt.IsZero() {
			detail += " · created " + s.CreatedAt.Format("2006-01-02 15:04")
		}
		b.WriteString(dimStyle.Render(detail) + "\n")
	}

	// Legend for the activity glyphs.
	legend := fmt.Sprintf("legend: %s working  %s waiting  %s idle  %s exited",
		workingStyle.Render("◉"), waitingStyle.Render("◉"),
		idleStyle.Render("◉"), exitedStyle.Render("○"))
	b.WriteString("\n" + dimStyle.Render(legend))
	b.WriteString("\n" + dimStyle.Render("n new · enter attach · d cleanup · r refresh · q quit"))
	if m.status != "" {
		b.WriteString("\n" + m.status)
	}
	return b.String()
}
```

> The `Activity.Label()` helper was added in Task 1. `fmt` is already imported in `views.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/`
Expected: PASS (existing UI tests still pass — attach/cleanup logic is unchanged).

- [ ] **Step 5: Commit**

```bash
git add internal/ui/
git commit -m "feat(ui): grouped dashboard with tab numbers, activity glyphs, legend"
```

---

## Task 10: `main.go` — wire workspace attach + refresher clock

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Update the wiring**

In `main.go`, update the `Refresh` action and the `Attach` action.

`Refresh` (add the clock):

```go
		Refresh: func() ([]session.Session, error) {
			return refresher.Build(cfg, tm, g, time.Now)
		},
```

`Attach` (select the workspace window and attach to the workspace; configure tabs first):

```go
		Attach: func(s session.Session) tea.Cmd {
			if err := mgr.EnsureRunning(s); err != nil {
				return func() tea.Msg { return ui.ErrorMsgFor(err) }
			}
			// Configure the tab strip + switch keys (best-effort).
			_ = tm.ConfigureTabs()
			// Resolve the current index by name (it may have been renumbered) and
			// select it before attaching.
			if w, ok := tm.LookupWindow(s.TmuxName); ok {
				_ = tm.SelectWindow(w.Index)
			}
			return tea.ExecProcess(tm.AttachWorkspaceCmd(), func(err error) tea.Msg {
				ss, rerr := refresher.Build(cfg, tm, g, time.Now)
				if rerr != nil {
					return ui.ErrorMsgFor(rerr)
				}
				return ui.SessionsMsgFor(ss)
			})
		},
```

Remove the old `tm.Decorate(...)` and `mgr.AttachCmd(s)` usage (both replaced above). `time` is already imported in `main.go`.

- [ ] **Step 2: Build the whole program**

Run: `go build ./...`
Expected: success, no unused-import or signature errors.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: PASS across all packages (tmux integration tests SKIP if tmux is absent).

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat(main): attach via shared workspace with tab switching"
```

---

## Task 11: Manual smoke + docs

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Manual smoke (requires tmux + claude)**

Run: `go run .`
Verify:
1. Create two sessions in different projects.
2. The dashboard groups by project, shows tab numbers (1, 2), base branch, and a colored activity glyph + legend.
3. `enter` on a session attaches to the workspace at that window; the tmux status bar shows numbered tabs.
4. `Alt-2` / `Alt-1` and `Alt-Left`/`Alt-Right` switch between sessions instantly.
5. `prefix d` returns to the dashboard.
6. A session you quit (Ctrl-D in Claude) shows `exited` (○) and `enter` respawns it.

If anything fails, debug with `superpowers:systematic-debugging` before proceeding.

- [ ] **Step 2: Update CLAUDE.md**

In `CLAUDE.md`, update the **Core design decisions → Session model** bullet to describe the windows model. Replace:

```
- **Session model:** tmux-backed. Each instance = a tmux session named
  `fleet-<project>-<session>` running `claude` in its worktree.
```

with:

```
- **Session model:** tmux-backed. All instances share one tmux session
  (`fleet-workspace`); each instance is a *window* named
  `fleet-<project>-<session>` running `claude` in its worktree. Windows act as
  tabs — switch with Alt-1..9 / Alt-←/→ while attached. Per-session activity
  (working / waiting / idle / exited) is derived from tmux's window-activity
  timestamp plus a best-effort capture-pane prompt match (`internal/activity`).
```

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: describe the shared-workspace windows session model"
```

---

## Self-Review Notes (for the implementer)

- **Type consistency:** `Window{Index, Name, Dead, LastActivity}`, `activity.State` (`Idle/Working/Waiting/Exited` with `Glyph()`, `TmuxColor()`, `Label()`), `Session` adds `Activity/LastActivity/WindowIndex`. `tmuxPort` (session) and `workspaceTmux` (refresher) are distinct narrow interfaces, both satisfied by `*tmux.CLI`.
- **`Build` signature changed** to 4 args `(cfg, t, g, now)`; all call sites updated in Tasks 7, 8, 10.
- **Targeting vs. labeling:** window *names* stay stable (`naming.TmuxName`) for matching/targeting; the tab text is the `@fleet_label` option (`SetWindowLabel`). Never rename windows.
- **Legacy methods** (`tmux.Create/Kill/Has/List/AttachCmd/Decorate`, plus their tests) remain but are unused by production code — leave them to keep the existing tmux tests green; a later cleanup task may remove them.
- **Best-effort, never fatal:** `CapturePane`, `SetWindowLabel`, `ConfigureTabs`, `SelectWindow` failures must not break a refresh or block attach.
