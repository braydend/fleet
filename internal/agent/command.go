package agent

import (
	"fmt"
	"strings"
)

// claudeFresh builds the command for a brand-new session: create the
// conversation under a specific session ID and give it a display name. Using
// --session-id directly (not the resume chain) avoids a spurious "no
// conversation found" error flashing in the pane on first launch. A legacy
// session (empty id) launches bare claude.
func claudeFresh(id, name string) string {
	if id == "" {
		return "claude"
	}
	return fmt.Sprintf("claude --session-id %s -n %s", id, shellQuote(name))
}

// claudeResume builds the command for re-launching an existing session. It
// resumes the stored ID; if the session file is gone it recreates it under the
// same stable ID; and it finally falls back to a bare claude so the window is
// always usable. A legacy session (empty id) launches bare claude.
func claudeResume(id, name string) string {
	if id == "" {
		return "claude"
	}
	return fmt.Sprintf(
		"claude --resume %s || claude --session-id %s -n %s || claude",
		id, id, shellQuote(name),
	)
}

// opencodeFresh launches opencode in the session's worktree. opencode mints its
// own session IDs, so there is nothing for fleet to pass.
func opencodeFresh(_, _ string) string { return "opencode" }

// opencodeResume continues the last opencode session for the working directory.
// Every fleet session owns a unique worktree, so "the last session here" can
// only be this session's conversation. The fallback keeps the window usable
// when there is nothing to continue — a first launch, or a session opencode has
// forgotten.
//
// The fallback is position-pinned, not ID-pinned like Claude's resume chain,
// and that makes it unsafe in a way the Claude chain is not: `--continue`
// exits non-zero for any reason opencode can't continue — no prior session,
// but also a crash, a version that doesn't know the flag, or a corrupt
// session store — and `|| opencode` cannot tell those apart. In every one of
// those cases it silently starts a brand-new, empty session in this
// directory. That empty session then becomes "the last session here", so the
// *next* resume continues the empty one and the real conversation becomes
// unreachable through fleet, with no error surfaced anywhere: the window just
// looks fine and the prior context is gone.
func opencodeResume(_, _ string) string { return "opencode --continue || opencode" }

// shellQuote wraps s in single quotes safe for `sh -c` (tmux runs the window
// command through the shell), escaping any embedded single quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
