// Package ui is the Bubble Tea program for fleet.
package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/session"
)

type state int

const (
	stateDashboard state = iota
	stateProjectPicker
	stateNewSession
	stateCleanupMenu
	stateConfirm
)

// Actions the model needs from the rest of the app, injected for testability.
type Actions struct {
	Refresh  func() ([]session.Session, error)
	Projects func() ([]projects.Project, error)
	Create   func(p projects.Project, name, branch, base string) error
	Delete   func(s session.Session, deleteBranch bool) error
	Leave    func(s session.Session) error
	PushPR   func(s session.Session) error
	Attach   func(s session.Session) tea.Cmd
}

// Model is the root Bubble Tea model.
type Model struct {
	actions  Actions
	state    state
	sessions []session.Session
	cursor   int
	status   string

	// new-session sub-state
	projects []projects.Project
	form     newSessionForm
}

// New builds a Model. actions may be the zero value in tests; refreshFn is the
// initial-load function (usually actions.Refresh).
func New(actions *Actions, _ any) Model {
	var a Actions
	if actions != nil {
		a = *actions
	}
	return Model{actions: a, state: stateDashboard}
}

// Init kicks off the first refresh and the tick loop.
func (m Model) Init() tea.Cmd {
	return tea.Batch(refresh(m.actions.Refresh), tick())
}

// Update is the reducer.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsUpdatedMsg:
		m.sessions = msg.sessions
		if m.cursor >= len(m.sessions) {
			m.cursor = max(0, len(m.sessions)-1)
		}
		m.state = stateDashboard
		return m, nil

	case errorMsg:
		if msg.err != nil {
			m.status = "error: " + msg.err.Error()
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(refresh(m.actions.Refresh), tick())

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Dashboard keys only for this task; later tasks extend per-state.
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		return m, refresh(m.actions.Refresh)
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.sessions)-1 {
			m.cursor++
		}
	}
	return m, nil
}
