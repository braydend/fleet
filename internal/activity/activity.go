// Package activity classifies a session's live state from cheap tmux signals:
// the window's last-activity timestamp, whether its process has exited, and a
// best-effort match of Claude's input prompt in the captured pane tail.
//
// This package is the ONLY place that knows what Claude's prompt looks like, so
// it is the single spot to update if Claude's TUI changes.
package activity

import (
	"strings"
	"time"
)

// State is a session's live activity state.
type State int

const (
	Idle    State = iota // quiet, nothing pending
	Working              // produced output recently
	Waiting              // quiet AND a Claude input prompt is showing
	Exited               // the process is gone (or no window exists)
)

// workingWindow is how recently output must have happened to count as "working".
const workingWindow = 5 * time.Second

// promptMarkers are substrings that indicate Claude is waiting for input.
// Best-effort and intentionally centralized; update here if the TUI changes.
var promptMarkers = []string{
	"❯ 1.",        // numbered choice prompt
	"Do you want", // confirmation prompt
	"(y/n)",       // yes/no prompt
}

// Classify decides a session's state. missing means no window exists for it;
// dead means the window exists but its process has exited.
func Classify(lastActivity, now time.Time, paneTail string, missing, dead bool) State {
	if missing || dead {
		return Exited
	}
	if now.Sub(lastActivity) <= workingWindow {
		return Working
	}
	for _, mark := range promptMarkers {
		if strings.Contains(paneTail, mark) {
			return Waiting
		}
	}
	return Idle
}

// Glyph returns the single-rune indicator for the state.
func (s State) Glyph() string {
	if s == Exited {
		return "○"
	}
	return "◉"
}

// TmuxColor returns a tmux colour name for the state (for status-bar labels).
func (s State) TmuxColor() string {
	switch s {
	case Working:
		return "colour42" // green
	case Waiting:
		return "colour220" // yellow
	case Exited:
		return "colour238" // dim
	default:
		return "colour244" // grey
	}
}

// Label returns the human word for the state, used in the dashboard detail line.
func (s State) Label() string {
	switch s {
	case Working:
		return "working"
	case Waiting:
		return "waiting for input"
	case Exited:
		return "exited"
	default:
		return "idle"
	}
}
