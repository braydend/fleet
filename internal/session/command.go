package session

import (
	"fmt"
	"strings"
)

// launchFresh builds the command for a brand-new session: create the
// conversation under a specific session ID and give it a display name. Using
// --session-id directly (not the resume chain) avoids a spurious "no
// conversation found" error flashing in the pane on first launch. A legacy
// session (empty id) launches bare claude.
func launchFresh(id, name string) string {
	if id == "" {
		return "claude"
	}
	return fmt.Sprintf("claude --session-id %s -n %s", id, shellQuote(name))
}

// launchResume builds the command for re-launching an existing session. It
// resumes the stored ID; if the session file is gone it recreates it under the
// same stable ID; and it finally falls back to a bare claude so the window is
// always usable. A legacy session (empty id) launches bare claude.
func launchResume(id, name string) string {
	if id == "" {
		return "claude"
	}
	return fmt.Sprintf(
		"claude --resume %s || claude --session-id %s -n %s || claude",
		id, id, shellQuote(name),
	)
}

// shellQuote wraps s in single quotes safe for `sh -c` (tmux runs the window
// command through the shell), escaping any embedded single quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
