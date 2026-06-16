package tmux

import (
	"os/exec"
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
