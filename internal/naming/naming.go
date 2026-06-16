// Package naming encodes the conventions that map a (project, session) pair to
// a tmux session name and a worktree directory. Names are sanitized so that the
// "-" separator in tmux names is unambiguous.
package naming

import (
	"path/filepath"
	"strings"
)

const prefix = "fleet"

// Sanitize replaces every rune that is not [a-zA-Z0-9_] with "_".
func Sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// TmuxName returns the tmux session name for a project/session pair.
func TmuxName(project, session string) string {
	return prefix + "-" + Sanitize(project) + "-" + Sanitize(session)
}

// ParseTmuxName splits a fleet tmux session name back into sanitized project and
// session components. ok is false if name is not a well-formed fleet name.
func ParseTmuxName(name string) (project, session string, ok bool) {
	parts := strings.Split(name, "-")
	if len(parts) != 3 || parts[0] != prefix {
		return "", "", false
	}
	return parts[1], parts[2], true
}

// WorktreePath returns the on-disk worktree directory for a project/session.
func WorktreePath(base, project, session string) string {
	return filepath.Join(base, Sanitize(project), Sanitize(session))
}
