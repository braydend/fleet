package selfupdate

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStateDue(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	if !(State{}).Due(now) {
		t.Error("zero state should be due")
	}
	if (State{LastChecked: now.Add(-30 * time.Minute)}).Due(now) {
		t.Error("30m ago should not be due")
	}
	if !(State{LastChecked: now.Add(-90 * time.Minute)}).Due(now) {
		t.Error("90m ago should be due")
	}
}

func TestLoadSaveState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	// Missing file => zero, no error.
	if got := LoadState(path); !got.LastChecked.IsZero() {
		t.Fatalf("missing file should be zero, got %+v", got)
	}
	want := State{LastChecked: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)}
	if err := SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	got := LoadState(path)
	if !got.LastChecked.Equal(want.LastChecked) {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
}
