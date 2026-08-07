// Package memory persists the last agent used per project, so the new-session
// form can default to it. It is plain-text state kept beside the worktrees,
// separate from each session's meta.json.
package memory

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bray/fleet/internal/naming"
)

// fileName is the per-project memory file. A leading dot keeps it from being
// mistaken for a session directory by the refresher, which skips non-directories.
const fileName = ".fleet-agent"

// Path returns the per-project memory file for project, under base. The
// worktrees for a project already live in <base>/<Sanitize(project)>/, so the
// picker's project name maps to the same directory the memory file sits in.
func Path(base, project string) string {
	return filepath.Join(base, naming.Sanitize(project), fileName)
}

// Read returns the stored value, or "" when the file is missing or unreadable.
// Errors are swallowed because a missing file simply means "no memory"; there
// is nothing actionable at form-open time. The call site decides whether a
// non-empty value names a known agent.
func Read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Write stores value, creating the project directory if needed.
func Write(path, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value+"\n"), 0o644)
}
