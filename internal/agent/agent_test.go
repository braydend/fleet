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
