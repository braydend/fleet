package tmux

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
}

// isolated returns a CLI pinned to a private tmux server unique to this test,
// and arranges for that server to be killed on cleanup. This keeps tests off the
// user's real tmux server: without it, the suite's KillWorkspace/kill-session
// calls would destroy a live "fleet-workspace" out from under an attached user
// (issue #5).
func isolated(t *testing.T) *CLI {
	t.Helper()
	c := NewWithSocket("fleettest-" + strings.ReplaceAll(t.Name(), "/", "_"))
	t.Cleanup(func() { _ = exec.Command("tmux", "-L", c.socket, "kill-server").Run() })
	return c
}

// tmuxOn runs a raw tmux command against the CLI's private socket, for test
// setup/assertions that bypass the CLI API. It fails the test on error.
func tmuxOn(t *testing.T, c *CLI, args ...string) []byte {
	t.Helper()
	out, err := exec.Command("tmux", c.withSocket(args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v: %v: %s", args, err, out)
	}
	return out
}

func TestCreateListHasKill(t *testing.T) {
	requireTmux(t)
	c := isolated(t)
	name := "fleet-testproj-testsess"
	if err := c.Create(name, t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("create: %v", err)
	}

	if !c.Has(name) {
		t.Fatal("expected Has to report the session alive")
	}
	sessions, err := c.List("fleet-")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, s := range sessions {
		if s.Name == name && s.Alive {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected to find %q in %+v", name, sessions)
	}

	if err := c.Kill(name); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if c.Has(name) {
		t.Fatal("expected session gone after kill")
	}
}

// TestSocketIsolation is the regression test for issue #5: fleet's tmux
// operations (including destructive ones like KillWorkspace) must be confined to
// a dedicated tmux socket and must never touch the default server, where the
// user's real sessions live. Before this, running the test suite would
// kill-session the live "fleet-workspace" out from under an attached user.
func TestSocketIsolation(t *testing.T) {
	requireTmux(t)
	c := isolated(t)

	if _, err := c.CreateWindow("fleet-proj-iso", t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, ok := c.LookupWindow("fleet-proj-iso"); !ok {
		t.Fatal("expected the window on fleet's isolated socket")
	}
	// The default tmux server must be untouched — it has no window we created.
	// (Read-only query on the DEFAULT socket — deliberately no -L; safe even if
	// the user has real sessions there.)
	out, _ := exec.Command("tmux", "list-windows", "-t", "fleet-workspace", "-F", "#{window_name}").CombinedOutput()
	if strings.Contains(string(out), "fleet-proj-iso") {
		t.Fatalf("isolated CLI leaked onto the default tmux server: %s", out)
	}
}

func TestAttachCmdShape(t *testing.T) {
	c := New()
	cmd := c.AttachCmd("fleet-x-y")
	if cmd.Args[0] != "tmux" || cmd.Args[1] != "attach" {
		t.Fatalf("unexpected attach command: %v", cmd.Args)
	}
}

func TestHumanizeKey(t *testing.T) {
	cases := map[string]string{
		"C-b": "Ctrl-b",
		"C-a": "Ctrl-a",
		"M-x": "Alt-x",
		"`":   "`",
		"":    "Ctrl-b",
		"F12": "F12",
	}
	for in, want := range cases {
		if got := humanizeKey(in); got != want {
			t.Errorf("humanizeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkspaceWindowLifecycle(t *testing.T) {
	requireTmux(t)
	c := isolated(t)

	// First window bootstraps the workspace at index 1.
	idx, err := c.CreateWindow("fleet-proj-one", t.TempDir(), "sleep 30")
	if err != nil {
		t.Fatalf("create window: %v", err)
	}
	if idx != 1 {
		t.Fatalf("first window index = %d, want 1", idx)
	}

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

// TestExistingWorkspaceKeepsWindowOnProcessExit reproduces issue #5: when the
// shared workspace already exists (it survives previous fleet runs), windows
// added to it must still be protected by remain-on-exit, so a window whose
// process exits is kept as a dead pane instead of vanishing. If it vanishes and
// it was the last window, the session — and, via exit-empty, the whole server —
// is destroyed, bouncing the attached user to the dashboard.
func TestExistingWorkspaceKeepsWindowOnProcessExit(t *testing.T) {
	requireTmux(t)
	c := isolated(t)

	// Simulate a pre-existing/stale workspace created OUTSIDE fleet's setup
	// branch (so it has none of fleet's options, exactly like one left over
	// from an earlier run). A bare new-session has remain-on-exit off.
	tmuxOn(t, c, "new-session", "-d", "-s", "fleet-workspace",
		"-n", "fleet-bootstrap", "-c", t.TempDir(), "sleep 30")

	// Adding a window now goes through the "workspace already exists" branch.
	if _, err := c.CreateWindow("fleet-proj-one", t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("create window: %v", err)
	}

	// End the window's process the way claude exiting would.
	pid := strings.TrimSpace(string(tmuxOn(t, c, "list-panes", "-t",
		"fleet-workspace:fleet-proj-one", "-F", "#{pane_pid}")))
	if _, err := exec.Command("kill", pid).CombinedOutput(); err != nil {
		t.Fatalf("kill pane process: %v", err)
	}

	// The window must still exist (kept as a dead pane), not vanish.
	var w Window
	var ok bool
	for i := 0; i < 50; i++ {
		if w, ok = c.LookupWindow("fleet-proj-one"); ok && w.Dead {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok {
		t.Fatal("window vanished after its process exited; remain-on-exit not applied to existing workspace")
	}
	if !w.Dead {
		t.Fatalf("expected window kept as a dead pane, got %+v", w)
	}
}

// TestEnsureConfiguredProtectsExistingWorkspace covers the attach path on a
// workspace that already exists with windows fleet didn't configure (a leftover
// from before this fix, or from a previous fleet version). EnsureConfigured must
// turn off exit-empty and retro-fit remain-on-exit onto the existing windows so
// they survive their process exiting.
func TestEnsureConfiguredProtectsExistingWorkspace(t *testing.T) {
	requireTmux(t)
	c := isolated(t)

	// Stale workspace with a window created outside fleet's setup (no options).
	tmuxOn(t, c, "new-session", "-d", "-s", "fleet-workspace",
		"-n", "fleet-legacy", "-c", t.TempDir(), "sleep 30")

	if err := c.EnsureConfigured(); err != nil {
		t.Fatalf("ensure configured: %v", err)
	}

	if got := strings.TrimSpace(string(tmuxOn(t, c, "show-options", "-s", "-v", "exit-empty"))); got != "off" {
		t.Fatalf("exit-empty = %q, want off", got)
	}

	// The pre-existing window must now survive its process exiting.
	pid := strings.TrimSpace(string(tmuxOn(t, c, "list-panes", "-t",
		"fleet-workspace:fleet-legacy", "-F", "#{pane_pid}")))
	if _, err := exec.Command("kill", pid).CombinedOutput(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	var ok bool
	var w Window
	for i := 0; i < 50; i++ {
		if w, ok = c.LookupWindow("fleet-legacy"); ok && w.Dead {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok || !w.Dead {
		t.Fatalf("legacy window not protected after EnsureConfigured: ok=%v %+v", ok, w)
	}
}

func TestEnsureConfiguredNoWorkspaceIsNoError(t *testing.T) {
	requireTmux(t)
	c := isolated(t)
	if err := c.EnsureConfigured(); err != nil {
		t.Fatalf("EnsureConfigured with no workspace should be a no-op, got %v", err)
	}
}

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
	c := isolated(t)
	if _, err := c.CreateWindow("fleet-proj-one", t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := c.ConfigureTabs(); err != nil {
		t.Fatalf("configure tabs: %v", err)
	}
	// window-status-format should reference our label option.
	out := tmuxOn(t, c, "show-options", "-t", "fleet-workspace", "-v", "window-status-format")
	if !strings.Contains(string(out), "@fleet_label") {
		t.Fatalf("window-status-format = %q, expected to reference @fleet_label", out)
	}
	// Mouse mode must be on so the wheel scrolls scrollback instead of
	// cycling the inner program's prompt history (issue #2).
	if got := strings.TrimSpace(string(tmuxOn(t, c, "show-options", "-t", "fleet-workspace", "-v", "mouse"))); got != "on" {
		t.Fatalf("mouse = %q, expected \"on\"", got)
	}
	if err := c.SelectWindow(1); err != nil {
		t.Fatalf("select window: %v", err)
	}
}

func TestListWindowsNoWorkspace(t *testing.T) {
	requireTmux(t)
	c := isolated(t)
	ws, err := c.ListWindows()
	if err != nil {
		t.Fatalf("expected no error when workspace absent, got %v", err)
	}
	if len(ws) != 0 {
		t.Fatalf("expected 0 windows, got %+v", ws)
	}
}

func TestDecorateSetsStatusBar(t *testing.T) {
	requireTmux(t)
	c := isolated(t)
	name := "fleet-decor-test"
	if err := c.Create(name, t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := c.Decorate(name, "myproj/feature"); err != nil {
		t.Fatalf("decorate: %v", err)
	}

	get := func(opt string) string {
		return string(tmuxOn(t, c, "show-options", "-t", name, "-v", opt))
	}
	if got := get("status"); got != "on\n" && got != "on" {
		t.Fatalf("status = %q, want on", got)
	}
	if right := get("status-right"); !strings.Contains(right, "fleet") {
		t.Fatalf("status-right = %q, expected to mention returning to fleet", right)
	}
	if left := get("status-left"); !strings.Contains(left, "myproj/feature") {
		t.Fatalf("status-left = %q, expected to mention the session label", left)
	}
}
