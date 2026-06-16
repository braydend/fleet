package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/session"
)

// Messages exchanged inside the program.
type sessionsUpdatedMsg struct{ sessions []session.Session }
type errorMsg struct{ err error }
type tickMsg struct{}

// refresh runs the injected refresh function and returns its result as a msg.
func refresh(fn func() ([]session.Session, error)) tea.Cmd {
	return func() tea.Msg {
		if fn == nil {
			return sessionsUpdatedMsg{}
		}
		sessions, err := fn()
		if err != nil {
			return errorMsg{err: err}
		}
		return sessionsUpdatedMsg{sessions: sessions}
	}
}

// tick schedules the next periodic refresh.
func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return tickMsg{} })
}

type projectsLoadedMsg struct{ projects []projects.Project }
