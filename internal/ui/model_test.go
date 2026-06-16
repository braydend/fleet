package ui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/projects"
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

func TestNKeyOpensProjectPicker(t *testing.T) {
	called := false
	a := Actions{Projects: func() ([]projects.Project, error) {
		called = true
		return []projects.Project{{Name: "app", Path: "/code/app", DefaultBranch: "main"}}, nil
	}}
	m := New(&a, nil)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	// The command loads projects; run it and feed the result back.
	if cmd == nil {
		t.Fatal("expected a command to load projects")
	}
	msg := cmd()
	updated, _ = updated.(Model).Update(msg)
	mm := updated.(Model)
	if !called {
		t.Fatal("expected Projects action to be called")
	}
	if mm.state != stateProjectPicker || len(mm.projects) != 1 {
		t.Fatalf("expected project picker with 1 project, got state=%v n=%d", mm.state, len(mm.projects))
	}
}

func TestFormSubmitCallsCreate(t *testing.T) {
	var gotName, gotBranch, gotBase string
	a := Actions{Create: func(p projects.Project, name, branch, base string) error {
		gotName, gotBranch, gotBase = name, branch, base
		return nil
	}}
	m := New(&a, nil)
	m.state = stateNewSession
	m.form = newSessionForm{
		project:     projects.Project{Name: "app", Path: "/code/app", DefaultBranch: "main"},
		sessionName: "fix",
		branch:      "fleet/fix",
		base:        "main",
		field:       fieldBase, // last field; enter submits
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected create command")
	}
	_ = cmd() // execute create
	if gotName != "fix" || gotBranch != "fleet/fix" || gotBase != "main" {
		t.Fatalf("create got name=%q branch=%q base=%q", gotName, gotBranch, gotBase)
	}
}

func TestEnterAttachesSelectedSession(t *testing.T) {
	var attached session.Session
	a := Actions{Attach: func(s session.Session) tea.Cmd {
		attached = s
		return func() tea.Msg { return nil }
	}}
	m := New(&a, nil)
	m.sessions = sample()
	m.cursor = 0
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected attach command")
	}
	if attached.Name != "a" {
		t.Fatalf("attached wrong session: %+v", attached)
	}
}

func TestDOpensCleanupMenu(t *testing.T) {
	m := New(nil, nil)
	m.sessions = sample()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if updated.(Model).state != stateCleanupMenu {
		t.Fatal("expected cleanup menu state")
	}
}

func TestCleanupLeaveCallsLeave(t *testing.T) {
	left := false
	a := Actions{
		Leave:   func(session.Session) error { left = true; return nil },
		Refresh: func() ([]session.Session, error) { return nil, nil },
	}
	m := New(&a, nil)
	m.sessions = sample()
	m.cursor = 0
	m.state = stateCleanupMenu
	m.cleanupChoice = cleanupLeave
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command")
	}
	_ = cmd()
	if !left {
		t.Fatal("expected Leave to be called")
	}
}

func TestDeleteDirtyRequiresConfirm(t *testing.T) {
	m := New(nil, nil)
	m.sessions = sample() // session "a" is dirty
	m.cursor = 0
	m.state = stateCleanupMenu
	m.cleanupChoice = cleanupDelete
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).state != stateConfirm {
		t.Fatal("expected confirm state for dirty delete")
	}
}

func TestRefreshDoesNotLeaveProjectPicker(t *testing.T) {
	m := New(nil, nil)
	m.state = stateProjectPicker
	m.projects = []projects.Project{{Name: "app"}}
	// A periodic refresh completing must not yank the user back to the dashboard.
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	mm := updated.(Model)
	if mm.state != stateProjectPicker {
		t.Fatalf("expected to stay in project picker, got state %v", mm.state)
	}
	if len(mm.sessions) != 2 {
		t.Fatalf("expected sessions to still update in the background, got %d", len(mm.sessions))
	}
}

func TestRefreshDoesNotLeaveNewSessionForm(t *testing.T) {
	m := New(nil, nil)
	m.state = stateNewSession
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	if updated.(Model).state != stateNewSession {
		t.Fatal("expected to stay in the new-session form during a background refresh")
	}
}

func TestFormSubmitReturnsToDashboard(t *testing.T) {
	a := Actions{Create: func(projects.Project, string, string, string) error { return nil }}
	m := New(&a, nil)
	m.state = stateNewSession
	m.form = newSessionForm{
		project:     projects.Project{Name: "app", DefaultBranch: "main"},
		sessionName: "fix",
		branch:      "fleet/fix",
		base:        "main",
		field:       fieldBase,
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).state != stateDashboard {
		t.Fatal("expected form submit to return to the dashboard")
	}
	if cmd == nil {
		t.Fatal("expected a create command")
	}
}
