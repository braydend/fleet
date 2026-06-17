package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"

	"github.com/bray/fleet/internal/activity"
)

// Palette — every colour adapts to light vs dark terminals so the neon look
// stays legible on both. Dark values preserve fleet's original look.
var (
	accentColor  = lipgloss.AdaptiveColor{Light: "200", Dark: "212"} // hot pink selection
	dimColor     = lipgloss.AdaptiveColor{Light: "245", Dark: "241"} // secondary text
	projectColor = lipgloss.AdaptiveColor{Light: "31", Dark: "45"}   // cyan project headers
	workingColor = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}   // green
	warnColor    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"} // red, destructive
)

// Styles built from the palette. Shared by every screen.
var (
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	dimStyle      = lipgloss.NewStyle().Foreground(dimColor)
	projectStyle  = lipgloss.NewStyle().Bold(true).Foreground(projectColor)
	warnStyle     = lipgloss.NewStyle().Bold(true).Foreground(warnColor)
	spinnerStyle  = lipgloss.NewStyle().Foreground(workingColor)
)

// activityIcon returns the emoji shown for a session's state. Emoji are used
// for every state (not just some) so the 2-cell width keeps columns aligned.
func activityIcon(s activity.State) string {
	switch s {
	case activity.Working:
		return "🟢"
	case activity.Waiting:
		return "🟡"
	case activity.Exited:
		return "⚫"
	default: // Idle
		return "💤"
	}
}

// Gradient stops for the title: pink → purple → cyan.
var (
	gradStart = mustHex("#ff79c6")
	gradMid   = mustHex("#bd93f9")
	gradEnd   = mustHex("#8be9fd")
)

func mustHex(s string) colorful.Color {
	c, err := colorful.Hex(s)
	if err != nil {
		panic("ui: bad gradient hex " + s + ": " + err.Error())
	}
	return c
}

// gradientColors returns n colours interpolated across the pink→purple→cyan
// stops. Endpoints are pinned to the exact stop hexes; n<=0 returns nil.
func gradientColors(n int) []lipgloss.Color {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []lipgloss.Color{lipgloss.Color(gradStart.Hex())}
	}
	out := make([]lipgloss.Color, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1) // 0..1
		var c colorful.Color
		switch {
		case t < 0.5:
			c = gradStart.BlendLab(gradMid, t/0.5)
		default:
			c = gradMid.BlendLab(gradEnd, (t-0.5)/0.5)
		}
		out[i] = lipgloss.Color(c.Clamped().Hex())
	}
	// Pin exact endpoints (blend round-trips can drift by a unit).
	out[0] = lipgloss.Color(gradStart.Hex())
	out[n-1] = lipgloss.Color(gradEnd.Hex())
	return out
}

// gradientTitle renders s with a per-rune colour gradient.
func gradientTitle(s string) string {
	runes := []rune(s)
	cols := gradientColors(len(runes))
	var b strings.Builder
	for i, r := range runes {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(cols[i]).Render(string(r)))
	}
	return b.String()
}
