package ui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/session"
)

var errSample = errors.New("boom")

func sample() []session.Session {
	return []session.Session{
		{Project: "app", Name: "a", Branch: "fleet/a", Alive: true,
			Git: git.Status{Branch: "fleet/a", ChangeCount: 1, Dirty: true}, CreatedAt: time.Unix(1, 0)},
		{Project: "app", Name: "b", Branch: "fleet/b", Exited: true, CreatedAt: time.Unix(2, 0)},
	}
}

func TestSessionsUpdatedPopulatesList(t *testing.T) {
	m := New(nil, nil)
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	mm := updated.(Model)
	if len(mm.sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(mm.sessions))
	}
	if mm.state != stateDashboard {
		t.Fatalf("expected dashboard state, got %v", mm.state)
	}
	// View must not panic and should mention a branch.
	if out := mm.View(); out == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestQuitKey(t *testing.T) {
	m := New(nil, nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestErrorMsgSetsStatus(t *testing.T) {
	m := New(nil, nil)
	updated, _ := m.Update(errorMsg{err: errSample})
	if got := updated.(Model).status; got == "" {
		t.Fatal("expected status to be set on error")
	}
}
