// Package agent describes the coding agents fleet can run in a session window.
// It is the single place that knows anything agent-specific: how to launch and
// resume one, what its binary is called, whether fleet owns its session ID, and
// what its "waiting for input" prompt looks like.
package agent

import "os/exec"

// Agent IDs. These strings are persisted in .fleet/meta.json, so renaming one
// would orphan every session created with it.
const (
	IDClaude   = "claude"
	IDOpencode = "opencode"
)

// Agent describes one coding agent fleet can run in a session window.
type Agent struct {
	ID      string // persisted in meta
	Label   string // shown in the new-session form and on the dashboard
	Bin     string // binary looked up on PATH
	NeedsID bool   // fleet mints and stores a stable session ID for this agent

	// Fresh and Resume build the shell command tmux runs in the window, for a
	// first launch and a re-launch respectively. Agents that mint their own
	// session IDs ignore both arguments.
	Fresh  func(id, name string) string
	Resume func(id, name string) string

	// Markers are pane-tail substrings that mean the agent is waiting for input.
	Markers []string
}

// lookPath is exec.LookPath, indirected so tests can decide what is installed
// rather than depending on the machine running them.
var lookPath = exec.LookPath

// All returns the known agents. The order is the order the new-session form
// cycles through, and the first entry is the fallback for an unknown ID.
func All() []Agent {
	return []Agent{
		{
			ID: IDClaude, Label: "claude", Bin: "claude", NeedsID: true,
			Fresh: claudeFresh, Resume: claudeResume,
			// Best-effort and intentionally centralized; update here if the
			// Claude Code TUI changes.
			Markers: []string{"❯ 1.", "Do you want", "(y/n)"},
		},
		{
			ID: IDOpencode, Label: "opencode", Bin: "opencode", NeedsID: false,
			Fresh: opencodeFresh, Resume: opencodeResume,
			// opencode's prompt text has not been observed yet. An empty list
			// means its sessions never classify as "waiting", which is better
			// than guessed strings producing false "waiting" badges.
			Markers: nil,
		},
	}
}

// Lookup resolves a persisted agent ID. It is total: an empty ID (a session
// created before agents were selectable) or an unrecognised one degrades to
// Claude Code rather than breaking every session on the dashboard.
func Lookup(id string) Agent {
	all := All()
	for _, a := range all {
		if a.ID == id {
			return a
		}
	}
	return all[0]
}

// Known reports whether id names a registered agent. Unlike Lookup it does not
// fall back, so configuration can reject a typo instead of silently launching
// something the user did not ask for.
func Known(id string) bool {
	for _, a := range All() {
		if a.ID == id {
			return true
		}
	}
	return false
}

// IDs returns the registered agent IDs in registry order, for error messages.
func IDs() []string {
	all := All()
	out := make([]string, 0, len(all))
	for _, a := range all {
		out = append(out, a.ID)
	}
	return out
}

// Available reports whether the agent's binary is on PATH.
func (a Agent) Available() bool {
	_, err := lookPath(a.Bin)
	return err == nil
}
