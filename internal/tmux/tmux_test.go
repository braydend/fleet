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
