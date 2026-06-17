package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/bray/fleet/internal/activity"
)

// Palette — every colour adapts to light vs dark terminals so the neon look
// stays legible on both. Dark values preserve fleet's original look.
var (
	accentColor  = lipgloss.AdaptiveColor{Light: "200", Dark: "212"} // hot pink selection
	dimColor     = lipgloss.AdaptiveColor{Light: "245", Dark: "241"} // secondary text
	projectColor = lipgloss.AdaptiveColor{Light: "31", Dark: "45"}   // cyan project headers
	workingColor = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}   // green
	waitingColor = lipgloss.AdaptiveColor{Light: "172", Dark: "220"} // amber
	idleColor    = lipgloss.AdaptiveColor{Light: "245", Dark: "244"} // grey
	exitedColor  = lipgloss.AdaptiveColor{Light: "248", Dark: "238"} // dark grey
	warnColor    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"} // red, destructive
)

// Styles built from the palette. Shared by every screen.
var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	dimStyle      = lipgloss.NewStyle().Foreground(dimColor)
	projectStyle  = lipgloss.NewStyle().Bold(true).Foreground(projectColor)

	workingStyle = lipgloss.NewStyle().Foreground(workingColor)
	waitingStyle = lipgloss.NewStyle().Foreground(waitingColor)
	idleStyle    = lipgloss.NewStyle().Foreground(idleColor)
	exitedStyle  = lipgloss.NewStyle().Foreground(exitedColor)
	warnStyle    = lipgloss.NewStyle().Bold(true).Foreground(warnColor)
	spinnerStyle = lipgloss.NewStyle().Foreground(workingColor)
)

// activityStyle maps a state to its colour style.
func activityStyle(s activity.State) lipgloss.Style {
	switch s {
	case activity.Working:
		return workingStyle
	case activity.Waiting:
		return waitingStyle
	case activity.Exited:
		return exitedStyle
	default:
		return idleStyle
	}
}
