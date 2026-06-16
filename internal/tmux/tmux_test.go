package tmux

import (
	"os/exec"
	"strings"
	"testing"
)

func requireTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
}

func TestCreateListHasKill(t *testing.T) {
	requireTmux(t)
	c := New()
	name := "fleet-testproj-testsess"
	_ = c.Kill(name) // ensure clean start
	if err := c.Create(name, t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = c.Kill(name) })

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

func TestAttachCmdShape(t *testing.T) {
	c := New()
	cmd := c.AttachCmd("fleet-x-y")
	if cmd.Args[0] != "tmux" || cmd.Args[1] != "attach" {
		t.Fatalf("unexpected attach command: %v", cmd.Args)
	}
}

func TestHumanizeKey(t *testing.T) {
	cases := map[string]string{
		"C-b":    "Ctrl-b",
		"C-a":    "Ctrl-a",
		"M-x":    "Alt-x",
		"`":      "`",
		"":       "Ctrl-b",
		"F12":    "F12",
	}
	for in, want := range cases {
		if got := humanizeKey(in); got != want {
			t.Errorf("humanizeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

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

func TestDecorateSetsStatusBar(t *testing.T) {
	requireTmux(t)
	c := New()
	name := "fleet-decor-test"
	_ = c.Kill(name)
	if err := c.Create(name, t.TempDir(), "sleep 30"); err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = c.Kill(name) })

	if err := c.Decorate(name, "myproj/feature"); err != nil {
		t.Fatalf("decorate: %v", err)
	}

	get := func(opt string) string {
		out, err := exec.Command("tmux", "show-options", "-t", name, "-v", opt).Output()
		if err != nil {
			t.Fatalf("show-options %s: %v", opt, err)
		}
		return string(out)
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
