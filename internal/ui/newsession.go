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
// until the user edits it explicitly. For simplicity we recompute whenever
// branch is empty.
func (f *newSessionForm) syncBranchDefault() {
	if f.branch == "" && f.sessionName != "" {
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
	b.WriteString(titleStyle.Render("new session — "+f.project.Name) + "\n\n")
	rows := []struct{ label, val string }{
		{"session", f.sessionName},
		{"branch", f.branch},
		{"base", f.base},
	}
	for i, r := range rows {
		marker := "  "
		if i == f.field {
			marker = "› "
		}
		b.WriteString(fmt.Sprintf("%s%-8s %s\n", marker, r.label+":", r.val))
	}
	b.WriteString("\n" + dimStyle.Render("tab next · enter submit on last field · esc cancel"))
	return b.String()
}
