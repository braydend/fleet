// Package ui is the Bubble Tea program for fleet.
package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/agent"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/selfupdate"
	"github.com/bray/fleet/internal/session"
)

type state int

const (
	stateDashboard state = iota
	stateProjectPicker
	stateNewSession
	stateCleanupMenu
	stateConfirm
	stateUpdateConfirm
)

type cleanupChoice int

const (
	cleanupDelete cleanupChoice = iota
	cleanupPushPR
	cleanupLeave
)

type cleanupOption struct {
	choice cleanupChoice
	label  string
}

// cleanupOptions returns the cleanup menu for a session. A broken session has
// no usable worktree, so pushing and leaving are meaningless — cleanup is the
// only thing left to do with it.
func cleanupOptions(s session.Session) []cleanupOption {
	if s.Broken {
		return []cleanupOption{{cleanupDelete, "🧹 clean up (remove broken session)"}}
	}
	return []cleanupOption{
		{cleanupDelete, "🗑  delete worktree + branch"},
		{cleanupPushPR, "🚀 push / open PR"},
		{cleanupLeave, "👋 leave (kill tmux only)"},
	}
}

// Actions the model needs from the rest of the app, injected for testability.
type Actions struct {
	Refresh  func() ([]session.Session, error)
	Projects func() ([]projects.Project, error)
	Create   func(p projects.Project, name, branch, base, agentID string) error
	// RememberedAgent returns the last agent used for a project, or "" when
	// there is none yet. Seeded per-project, it beats the global default only
	// when it names a known agent.
	RememberedAgent func(p projects.Project) string
	Branches        func(p projects.Project) (git.Branches, error)
	FetchBranches   func(p projects.Project) (git.Branches, error)
	Delete          func(s session.Session, deleteBranch bool) (session.DeleteResult, error)
	Leave           func(s session.Session) error
	PushPR          func(s session.Session) error
	Attach          func(s session.Session) tea.Cmd
	CheckUpdate     func() (selfupdate.CheckResult, error)
	ApplyUpdate     func(selfupdate.Release) error
}

// Model is the root Bubble Tea model.
type Model struct {
	actions  Actions
	state    state
	sessions []session.Session
	cursor   int
	status   string

	// version is the build version shown in the dashboard footer.
	version string

	// spinner animates the glyph beside working sessions (decorative only).
	spinner spinner.Model

	// new-session sub-state
	projects []projects.Project
	form     newSessionForm

	// cleanup sub-state
	cleanupIndex  int             // index into cleanupOptions(selected session)
	pendingDelete session.Session // awaiting confirm

	// self-update sub-state
	updateAvailable bool
	updateRelease   selfupdate.Release
	updateLatest    string

	// defaultAgent is the configured agent ID each new form starts on.
	defaultAgent string
}

// New builds a Model. actions may be the zero value in tests; version is the
// build version string shown in the dashboard footer ("" hides it); defaultAgent
// is the configured agent ID each new-session form starts on.
func New(actions *Actions, version, defaultAgent string) Model {
	var a Actions
	if actions != nil {
		a = *actions
	}
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = spinnerStyle
	return Model{actions: a, state: stateDashboard, spinner: sp, version: version,
		defaultAgent: defaultAgent}
}

// Init kicks off the first refresh, the tick loop, and the update check.
func (m Model) Init() tea.Cmd {
	return tea.Batch(refresh(m.actions.Refresh), tick(), m.spinner.Tick,
		checkUpdate(m.actions.CheckUpdate), scheduleUpdateCheck())
}

// Update is the reducer.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsUpdatedMsg:
		// A refresh may arrive on the periodic tick while the user is in the
		// project picker, new-session form, or a cleanup menu. Update the list
		// in the background without changing the current screen — flows that
		// should return to the dashboard set the state themselves.
		m.sessions = msg.sessions
		// A periodic refresh carries no notice, so it never clobbers a message
		// left by the last action.
		if msg.notice != "" {
			m.status = msg.notice
		}
		// Only the dashboard uses cursor to index sessions; don't disturb the
		// selection on other screens (the picker indexes projects).
		if m.state == stateDashboard && m.cursor >= len(m.sessions) {
			m.cursor = max(0, len(m.sessions)-1)
		}
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

	case branchesLoadedMsg:
		if m.state == stateNewSession {
			m.form.localBranches = msg.branches.Local
			m.form.remoteBranches = msg.branches.Remote
		}
		return m, nil

	case branchesRefreshedMsg:
		if m.state == stateNewSession {
			if len(msg.branches.Local) > 0 || len(msg.branches.Remote) > 0 {
				m.form.localBranches = msg.branches.Local
				m.form.remoteBranches = msg.branches.Remote
			}
			if msg.fetchErr != nil {
				m.form.fetchWarning = "⚠ couldn't fetch from origin — branch list may be stale"
			}
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case updateAvailableMsg:
		if msg.res.Available {
			m.updateAvailable = true
			m.updateRelease = msg.res.Release
			m.updateLatest = msg.res.Latest
		}
		return m, nil

	case updateAppliedMsg:
		m.updateAvailable = false
		m.status = fmt.Sprintf("✓ updated to v%s — restart fleet to apply", msg.version)
		return m, nil

	case updateTickMsg:
		return m, tea.Batch(checkUpdate(m.actions.CheckUpdate), scheduleUpdateCheck())

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
	case stateCleanupMenu:
		return m.keyCleanupMenu(msg)
	case stateConfirm:
		return m.keyConfirm(msg)
	case stateUpdateConfirm:
		return m.keyUpdateConfirm(msg)
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
	case "enter":
		if s, ok := m.selected(); ok && m.actions.Attach != nil {
			return m, m.actions.Attach(s)
		}
	case "d":
		if _, ok := m.selected(); ok {
			m.state = stateCleanupMenu
			m.cleanupIndex = 0
		}
	case "u":
		if m.updateAvailable {
			m.state = stateUpdateConfirm
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
		p := m.projects[m.cursor]
		seed := m.defaultAgent
		if m.actions.RememberedAgent != nil {
			if remembered := m.actions.RememberedAgent(p); remembered != "" && agent.Known(remembered) {
				seed = remembered
			}
		}
		m.form = newForm(p, seed)
		m.state = stateNewSession
		m.cursor = 0
		return m, tea.Batch(
			loadBranches(m.actions.Branches, p),
			fetchBranches(m.actions.FetchBranches, p),
		)
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
	case "left":
		if m.form.field == fieldAgent {
			m.form.cycleAgent(-1)
		}
	case "right":
		if m.form.field == fieldAgent {
			m.form.cycleAgent(1)
		}
	case "backspace":
		if m.form.field == fieldAgent {
			return m, nil // an enum, not a text field
		}
		p := m.form.active()
		if len(*p) > 0 {
			*p = (*p)[:len(*p)-1]
		}
		if m.form.field == fieldBranch {
			m.form.branchTouched = true
		}
		m.form.syncBranchDefault()
	case "enter":
		m.form.syncBranchDefault()
		if m.form.field < fieldAgent {
			m.form.field++
			return m, nil
		}
		// Refuse rather than create a worktree and branch for a window whose
		// command would immediately fail. The message lives on the form: the
		// dashboard status line is not rendered while the form is up.
		if !m.form.agentAvailable() {
			m.form.submitError = fmt.Sprintf(
				"⚠ %s is not on PATH — install it or pick another agent",
				m.form.selectedAgent().Label)
			return m, nil
		}
		// Close the form and return to the dashboard; the create runs in the
		// background and a refresh will populate the new session.
		m.state = stateDashboard
		return m, m.submitForm()
	default:
		if m.form.field == fieldAgent {
			return m, nil // an enum, not a text field
		}
		if len(msg.Runes) > 0 {
			p := m.form.active()
			*p += string(msg.Runes)
			if m.form.field == fieldBranch {
				m.form.branchTouched = true
			}
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

func (m Model) selected() (session.Session, bool) {
	if m.cursor >= 0 && m.cursor < len(m.sessions) {
		return m.sessions[m.cursor], true
	}
	return session.Session{}, false
}

func (m Model) keyCleanupMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		m.state = stateDashboard
		return m, nil
	}
	opts := cleanupOptions(s)
	switch msg.String() {
	case "esc":
		m.state = stateDashboard
	case "up", "k":
		if m.cleanupIndex > 0 {
			m.cleanupIndex--
		}
	case "down", "j":
		if m.cleanupIndex < len(opts)-1 {
			m.cleanupIndex++
		}
	case "enter":
		if m.cleanupIndex >= len(opts) {
			m.state = stateDashboard
			return m, nil
		}
		switch opts[m.cleanupIndex].choice {
		case cleanupLeave:
			m.state = stateDashboard
			return m, m.runThenRefresh(func() (string, error) { return "", m.callLeave(s) })
		case cleanupPushPR:
			m.state = stateDashboard
			return m, m.runThenRefresh(func() (string, error) { return "", m.callPushPR(s) })
		case cleanupDelete:
			// A broken session has no readable git status, so there is nothing
			// to warn about — go straight to the cleanup.
			if !s.Broken && (s.Git.Dirty || s.Git.Ahead > 0) {
				m.pendingDelete = s
				m.state = stateConfirm
				return m, nil
			}
			m.state = stateDashboard
			return m, m.runThenRefresh(func() (string, error) { return m.callDelete(s) })
		}
	}
	return m, nil
}

func (m Model) keyConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		s := m.pendingDelete
		m.state = stateDashboard
		return m, m.runThenRefresh(func() (string, error) { return m.callDelete(s) })
	case "n", "esc":
		m.state = stateDashboard
	}
	return m, nil
}

func (m Model) keyUpdateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		rel := m.updateRelease
		m.state = stateDashboard
		return m, applyUpdate(m.actions.ApplyUpdate, rel)
	case "n", "esc":
		m.state = stateDashboard
	}
	return m, nil
}

func (m Model) callLeave(s session.Session) error {
	if m.actions.Leave != nil {
		return m.actions.Leave(s)
	}
	return nil
}
func (m Model) callDelete(s session.Session) (string, error) {
	if m.actions.Delete == nil {
		return "", nil
	}
	res, err := m.actions.Delete(s, true) // delete branch too
	if err != nil {
		return "", err
	}
	if res.Leftover != "" {
		return fmt.Sprintf("⚠ cleaned up %s/%s — %d files left at %s (sudo rm -rf to reclaim)",
			s.Project, s.Name, res.LeftoverCount, res.Leftover), nil
	}
	return fmt.Sprintf("✓ deleted %s/%s", s.Project, s.Name), nil
}
func (m Model) callPushPR(s session.Session) error {
	if m.actions.PushPR != nil {
		return m.actions.PushPR(s)
	}
	return nil
}

// runThenRefresh runs fn, then refreshes the session list. fn's string is an
// optional status-line notice, carried back with the refreshed list so the
// message and the new state land in one update.
func (m Model) runThenRefresh(fn func() (string, error)) tea.Cmd {
	refreshFn := m.actions.Refresh
	return func() tea.Msg {
		notice, err := fn()
		if err != nil {
			return errorMsg{err: err}
		}
		if refreshFn != nil {
			ss, err := refreshFn()
			if err != nil {
				return errorMsg{err: err}
			}
			return sessionsUpdatedMsg{sessions: ss, notice: notice}
		}
		return sessionsUpdatedMsg{notice: notice}
	}
}

// submitForm invokes Create and triggers a refresh.
func (m Model) submitForm() tea.Cmd {
	f := m.form
	agentID := f.selectedAgent().ID
	create := m.actions.Create
	refreshFn := m.actions.Refresh
	return func() tea.Msg {
		if create != nil {
			if err := create(f.project, f.sessionName, f.branch, f.base, agentID); err != nil {
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
