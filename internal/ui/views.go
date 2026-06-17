package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bray/fleet/internal/activity"
)

// View renders the current state.
func (m Model) View() string {
	switch m.state {
	case stateProjectPicker:
		return m.viewProjectPicker()
	case stateNewSession:
		return m.form.view()
	case stateCleanupMenu:
		return m.viewCleanupMenu()
	case stateConfirm:
		return m.viewConfirm()
	case stateUpdateConfirm:
		return m.viewUpdateConfirm()
	default:
		return m.viewDashboard()
	}
}

func (m Model) viewDashboard() string {
	var b strings.Builder
	b.WriteString(gradientTitle("✨ fleet · your sessions ✨") + "\n\n")
	if len(m.sessions) == 0 {
		b.WriteString(dimStyle.Render("no sessions. press n to create one.") + "\n")
	}

	// Sessions arrive already ordered by project then name (the refresher reads
	// dirs in os.ReadDir's sorted order), so a contiguous-run check groups each
	// project's sessions into one labelled block of styled content lines.
	type block struct {
		label string
		lines []string
	}
	var blocks []block
	lastProject := ""
	for i, s := range m.sessions {
		if s.Project != lastProject || len(blocks) == 0 {
			blocks = append(blocks, block{label: "📂 " + s.Project})
			lastProject = s.Project
		}
		cur := &blocks[len(blocks)-1]

		// Tab number: the window index, or "-" when there is no live window.
		// tmux is configured with base-index 1 (see tmux.CLI), so a live window
		// is always >= 1 and 0 reliably means "no window".
		num := "-"
		if s.WindowIndex > 0 {
			num = fmt.Sprintf("%d", s.WindowIndex)
		}
		identity := fmt.Sprintf("%s %s %s  %s ← %s", num, activityIcon(s.Activity), s.Name, s.Branch, s.Base)
		if i == m.cursor {
			cur.lines = append(cur.lines, selectedStyle.Render("› "+identity))
		} else {
			cur.lines = append(cur.lines, "  "+identity)
		}

		// Detail line: activity word, git state, age. Working sessions get the
		// animated spinner frame; other states render plain.
		detail := "    "
		if s.Activity == activity.Working {
			detail += m.spinner.View() + " "
		}
		detail += s.Activity.Label()
		if s.Git.Dirty {
			detail += fmt.Sprintf(" · ✱%d", s.Git.ChangeCount)
		} else {
			detail += " · clean"
		}
		if s.Git.Ahead > 0 || s.Git.Behind > 0 {
			detail += fmt.Sprintf(" · ↑%d↓%d", s.Git.Ahead, s.Git.Behind)
		}
		if !s.CreatedAt.IsZero() {
			detail += " · created " + s.CreatedAt.Format("2006-01-02 15:04")
		}
		cur.lines = append(cur.lines, dimStyle.Render(detail))
	}

	// Uniform inner width: the widest content line, and wide enough that every
	// label fits inside its top border.
	innerWidth := 0
	for _, bl := range blocks {
		if w := lipgloss.Width(bl.label); w > innerWidth {
			innerWidth = w
		}
		for _, ln := range bl.lines {
			if w := lipgloss.Width(ln); w > innerWidth {
				innerWidth = w
			}
		}
	}

	for i, bl := range blocks {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(projectBox(bl.label, bl.lines, innerWidth) + "\n")
	}

	// Legend for the activity glyphs, below the boxes and above the keybinds.
	legend := fmt.Sprintf("legend: %s working  %s waiting  %s idle  %s exited",
		activityIcon(activity.Working), activityIcon(activity.Waiting),
		activityIcon(activity.Idle), activityIcon(activity.Exited))
	b.WriteString("\n" + dimStyle.Render(legend))
	if m.updateAvailable {
		banner := fmt.Sprintf("⬆ update available: v%s → press u to update", m.updateLatest)
		b.WriteString("\n" + warnStyle.Render(banner))
	}
	b.WriteString("\n" + dimStyle.Render("n new · enter attach · d cleanup · r refresh · q quit"))
	if m.status != "" {
		b.WriteString("\n" + m.status)
	}
	return b.String()
}

func (m Model) viewProjectPicker() string {
	var b strings.Builder
	b.WriteString(gradientTitle("✨ pick a project ✨") + "\n\n")
	for i, p := range m.projects {
		line := "📂 " + p.Name
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("enter select · esc cancel"))
	return b.String()
}

func (m Model) viewCleanupMenu() string {
	s, _ := m.selected()
	var b strings.Builder
	b.WriteString(gradientTitle("✨ cleanup — "+s.Project+"/"+s.Name+" ✨") + "\n\n")
	choices := []string{"🗑  delete worktree + branch", "🚀 push / open PR", "👋 leave (kill tmux only)"}
	for i, c := range choices {
		if cleanupChoice(i) == m.cleanupChoice {
			b.WriteString(selectedStyle.Render("› "+c) + "\n")
		} else {
			b.WriteString("  " + c + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("enter choose · esc cancel"))
	return b.String()
}

func (m Model) viewConfirm() string {
	s := m.pendingDelete
	return warnStyle.Render("⚠️  confirm delete") + "\n\n" +
		fmt.Sprintf("%s/%s has uncommitted or unpushed changes.\n", s.Project, s.Name) +
		"Delete worktree and branch anyway? " + dimStyle.Render("(y/n)")
}

func (m Model) viewUpdateConfirm() string {
	return warnStyle.Render("⬆ update fleet") + "\n\n" +
		fmt.Sprintf("A new version is available: v%s.\n", m.updateLatest) +
		"Download and replace the running binary now? " + dimStyle.Render("(y/n)")
}
