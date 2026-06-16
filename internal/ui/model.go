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

	case projectsLoadedMsg:
		m.projects = msg.projects
		m.cursor = 0
		m.state = stateProjectPicker
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateProjectPicker:
		return m.keyProjectPicker(msg)
	case stateNewSession:
		return m.keyNewSession(msg)
	default:
		return m.keyDashboard(msg)
	}
}

func (m Model) keyDashboard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		return m, refresh(m.actions.Refresh)
	case "n":
		return m, m.loadProjects()
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

func (m Model) keyProjectPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateDashboard
		m.cursor = 0
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.projects)-1 {
			m.cursor++
		}
	case "enter":
		if len(m.projects) == 0 {
			return m, nil
		}
		m.form = newForm(m.projects[m.cursor])
		m.state = stateNewSession
		m.cursor = 0
	}
	return m, nil
}

func (m Model) keyNewSession(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = stateDashboard
		return m, nil
	case "tab", "down":
		m.form.field = (m.form.field + 1) % fieldCount
	case "shift+tab", "up":
		m.form.field = (m.form.field + fieldCount - 1) % fieldCount
	case "backspace":
		p := m.form.active()
		if len(*p) > 0 {
			*p = (*p)[:len(*p)-1]
		}
	case "enter":
		m.form.syncBranchDefault()
		if m.form.field < fieldBase {
			m.form.field++
			return m, nil
		}
		return m, m.submitForm()
	default:
		if len(msg.Runes) > 0 {
			p := m.form.active()
			*p += string(msg.Runes)
			m.form.syncBranchDefault()
		}
	}
	return m, nil
}

// loadProjects fetches projects and opens the picker.
func (m Model) loadProjects() tea.Cmd {
	return func() tea.Msg {
		if m.actions.Projects == nil {
			return errorMsg{err: nil}
		}
		ps, err := m.actions.Projects()
		if err != nil {
			return errorMsg{err: err}
		}
		return projectsLoadedMsg{projects: ps}
	}
}

// submitForm invokes Create and triggers a refresh.
func (m Model) submitForm() tea.Cmd {
	f := m.form
	create := m.actions.Create
	refreshFn := m.actions.Refresh
	return func() tea.Msg {
		if create != nil {
			if err := create(f.project, f.sessionName, f.branch, f.base); err != nil {
				return errorMsg{err: err}
			}
		}
		if refreshFn != nil {
			ss, err := refreshFn()
			if err != nil {
				return errorMsg{err: err}
			}
			return sessionsUpdatedMsg{sessions: ss}
		}
		return sessionsUpdatedMsg{}
	}
}
