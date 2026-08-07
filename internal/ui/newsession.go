package ui

import (
	"fmt"
	"strings"

	"github.com/bray/fleet/internal/agent"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/projects"
)

// form field indices, in tab order.
const (
	fieldSession = iota
	fieldBranch
	fieldBase
	fieldAgent
	fieldCount
)

// newSessionForm collects the fields for a new session.
type newSessionForm struct {
	project     projects.Project
	sessionName string
	branch      string
	base        string
	field       int
	// branchTouched records whether the user has edited the branch field
	// directly. Until they do, the branch tracks the (sanitized) session name.
	branchTouched bool
	// branch list cached when the form opens, used only for the advisory hint.
	localBranches  []string
	remoteBranches []string
	// fetchWarning is a soft, non-blocking notice shown if the background
	// `git fetch` failed; branch detection then falls back to local refs.
	fetchWarning string
	// agentIndex selects from agent.All(); available caches whether each of
	// those agents was found on PATH when the form opened, in the same order.
	agentIndex int
	available  []bool
	// submitError is shown when submission was refused — the form stays open,
	// and the dashboard status line is not rendered while it is up.
	submitError string
}

// newForm seeds a form for a project with sensible defaults. defaultAgent is
// the configured agent ID; an empty or unrecognised value starts on the first
// registered agent.
func newForm(p projects.Project, defaultAgent string) newSessionForm {
	all := agent.All()
	idx := 0
	avail := make([]bool, len(all))
	for i, a := range all {
		if a.ID == defaultAgent {
			idx = i
		}
		// One PATH lookup per agent when the form opens, so typing in the form
		// never shells out.
		avail[i] = a.Available()
	}
	return newSessionForm{
		project:    p,
		base:       p.DefaultBranch,
		field:      fieldSession,
		agentIndex: idx,
		available:  avail,
	}
}

// syncBranchDefault keeps the branch defaulted to the (sanitized) session name
// until the user edits it explicitly. We recompute from the full session name
// on every change so the branch tracks the whole name, not just its first rune.
func (f *newSessionForm) syncBranchDefault() {
	if !f.branchTouched {
		f.branch = naming.Sanitize(f.sessionName)
	}
}

// active returns a pointer to the currently focused field's string.
func (f *newSessionForm) active() *string {
	switch f.field {
	case fieldSession:
		return &f.sessionName
	case fieldBranch:
		return &f.branch
	default:
		return &f.base
	}
}

// containsStr reports whether s is in xs.
func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// branchHint classifies the typed branch against the cached lists. Local wins
// over remote; base is only relevant when the branch is new.
func (f newSessionForm) branchHint() string {
	if f.branch == "" {
		return ""
	}
	if containsStr(f.localBranches, f.branch) {
		return "existing local branch — base ignored"
	}
	if containsStr(f.remoteBranches, f.branch) {
		return "tracks origin/" + f.branch + " — base ignored"
	}
	return "new branch from " + f.base
}

// selectedAgent returns the agent the form is currently on, clamping a
// nonsensical index rather than panicking on a hand-built form.
func (f newSessionForm) selectedAgent() agent.Agent {
	all := agent.All()
	if f.agentIndex < 0 || f.agentIndex >= len(all) {
		return all[0]
	}
	return all[f.agentIndex]
}

// agentAvailable reports whether the selected agent's binary was found. A form
// that has not probed PATH reports true: "unknown" must not block a create.
func (f newSessionForm) agentAvailable() bool {
	if f.agentIndex < 0 || f.agentIndex >= len(f.available) {
		return true
	}
	return f.available[f.agentIndex]
}

// cycleAgent moves the selection by delta, wrapping in both directions.
func (f *newSessionForm) cycleAgent(delta int) {
	n := len(agent.All())
	f.agentIndex = ((f.agentIndex+delta)%n + n) % n
	f.submitError = "" // the user is responding to it
}

// agentLabel renders the selection, flagging an agent that is not installed so
// the reason a submit will be refused is visible before trying.
func (f newSessionForm) agentLabel() string {
	label := f.selectedAgent().Label
	if !f.agentAvailable() {
		label += " (not installed)"
	}
	return "‹ " + label + " ›"
}

func (f newSessionForm) view() string {
	var b strings.Builder
	b.WriteString(gradientTitle("✨ new session — "+f.project.Name+" ✨") + "\n\n")
	rows := []struct{ label, val string }{
		{"session", f.sessionName},
		{"branch", f.branch},
		{"base", f.base},
		{"agent", f.agentLabel()},
	}
	for i, r := range rows {
		line := fmt.Sprintf("%-8s %s", r.label+":", r.val)
		if i == f.field {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
		if i == fieldBranch {
			if h := f.branchHint(); h != "" {
				b.WriteString("  " + dimStyle.Render("         "+h) + "\n")
			}
		}
	}
	if f.fetchWarning != "" {
		b.WriteString("\n" + warnStyle.Render(f.fetchWarning) + "\n")
	}
	if f.submitError != "" {
		b.WriteString("\n" + warnStyle.Render(f.submitError) + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("tab next · ←/→ change agent · enter submit on last field · esc cancel"))
	return b.String()
}
