package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// View renders the current state.
func (m Model) View() string {
	switch m.state {
	case stateProjectPicker:
		return m.viewProjectPicker()
	case stateNewSession:
		return m.form.view()
	default:
		return m.viewDashboard()
	}
}

func (m Model) viewDashboard() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("fleet — sessions") + "\n\n")
	if len(m.sessions) == 0 {
		b.WriteString(dimStyle.Render("no sessions. press n to create one.") + "\n")
	}
	for i, s := range m.sessions {
		run := "●"
		if s.Exited {
			run = "○"
		}
		line := fmt.Sprintf("%s %s/%s  %s", run, s.Project, s.Name, s.Git.Branch)
		if s.Git.Dirty {
			line += fmt.Sprintf("  ✱%d", s.Git.ChangeCount)
		}
		if s.Git.Ahead > 0 || s.Git.Behind > 0 {
			line += fmt.Sprintf("  ↑%d↓%d", s.Git.Ahead, s.Git.Behind)
		}
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("n new · enter attach · d cleanup · r refresh · q quit"))
	if m.status != "" {
		b.WriteString("\n" + m.status)
	}
	return b.String()
}

func (m Model) viewProjectPicker() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("pick a project") + "\n\n")
	for i, p := range m.projects {
		prefix := "  "
		if i == m.cursor {
			prefix = "› "
		}
		b.WriteString(prefix + p.Name + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("enter select · esc cancel"))
	return b.String()
}
