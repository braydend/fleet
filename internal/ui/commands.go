package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/selfupdate"
	"github.com/bray/fleet/internal/session"
)

// Messages exchanged inside the program.
type sessionsUpdatedMsg struct{ sessions []session.Session }
type errorMsg struct{ err error }
type tickMsg struct{}
type updateAvailableMsg struct{ res selfupdate.CheckResult }
type updateAppliedMsg struct{ version string }
type updateTickMsg struct{}

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

// ErrorMsgFor and SessionsMsgFor let main construct the messages the attach
// callback returns.
func ErrorMsgFor(err error) tea.Msg              { return errorMsg{err: err} }
func SessionsMsgFor(s []session.Session) tea.Msg { return sessionsUpdatedMsg{sessions: s} }

// checkUpdate runs the injected update check off the UI goroutine.
func checkUpdate(fn func() (selfupdate.CheckResult, error)) tea.Cmd {
	return func() tea.Msg {
		if fn == nil {
			return updateAvailableMsg{}
		}
		res, err := fn()
		if err != nil {
			return errorMsg{err: err}
		}
		return updateAvailableMsg{res: res}
	}
}

// applyUpdate downloads + swaps the binary, then reports the new version.
func applyUpdate(fn func(selfupdate.Release) error, rel selfupdate.Release) tea.Cmd {
	return func() tea.Msg {
		if fn == nil {
			return errorMsg{err: nil}
		}
		if err := fn(rel); err != nil {
			return errorMsg{err: err}
		}
		return updateAppliedMsg{version: rel.Version}
	}
}

// scheduleUpdateCheck re-fires an update check after the throttle interval.
func scheduleUpdateCheck() tea.Cmd {
	return tea.Tick(selfupdate.CheckInterval, func(time.Time) tea.Msg { return updateTickMsg{} })
}
