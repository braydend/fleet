package ui

import (
	"fmt"
	"strings"

	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/projects"
)

// form field indices, in tab order.
const (
	fieldSession = iota
	fieldBranch
	fieldBase
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
}

// newForm seeds a form for a project with sensible defaults.
func newForm(p projects.Project) newSessionForm {
	return newSessionForm{
		project: p,
		base:    p.DefaultBranch,
		field:   fieldSession,
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

func (f newSessionForm) view() string {
	var b strings.Builder
	b.WriteString(gradientTitle("✨ new session — "+f.project.Name+" ✨") + "\n\n")
	rows := []struct{ label, val string }{
		{"session", f.sessionName},
		{"branch", f.branch},
		{"base", f.base},
	}
	for i, r := range rows {
		line := fmt.Sprintf("%-8s %s", r.label+":", r.val)
		if i == f.field {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("tab next · enter submit on last field · esc cancel"))
	return b.String()
}
