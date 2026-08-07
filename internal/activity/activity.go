// Package activity classifies a session's live state from cheap tmux signals:
// the window's last-activity timestamp, whether its process has exited, and a
// best-effort match of the agent's input prompt in the captured pane tail.
//
// Callers supply the prompt markers to match, because what a prompt looks like
// is a property of the agent running in the window (see internal/agent), not of
// this package.
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
	Waiting              // quiet AND the agent's input prompt is showing
	Exited               // the process is gone (or no window exists)
)

// workingWindow is how recently output must have happened to count as
// "working": output seen within this window is treated as in-progress.
const workingWindow = 5 * time.Second

// Classify decides a session's state. markers are the pane-tail substrings that
// mean the agent is waiting for input; an empty list means the agent's prompt is
// unknown, so it is never reported as waiting. missing means no window exists
// for it; dead means the window exists but its process has exited.
func Classify(lastActivity, now time.Time, paneTail string, markers []string, missing, dead bool) State {
	if missing || dead {
		return Exited
	}
	if now.Sub(lastActivity) <= workingWindow {
		return Working
	}
	for _, mark := range markers {
		if strings.Contains(paneTail, mark) {
			return Waiting
		}
	}
	return Idle
}

// Glyph returns the single-rune indicator for the state. Only Exited gets a
// distinct (hollow) glyph; live states share a filled glyph and are
// distinguished by TmuxColor.
func (s State) Glyph() string {
	switch s {
	case Exited:
		return "○"
	default:
		return "◉"
	}
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
