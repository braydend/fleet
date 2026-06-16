// Package tmux provides a thin adapter over the tmux CLI behind the Tmux
// interface. fleet runs each Claude Code instance in its own tmux session.
package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Session is a tmux session known to fleet.
type Session struct {
	Name  string
	Alive bool
}

// Tmux is the set of tmux operations fleet needs.
type Tmux interface {
	List(prefix string) ([]Session, error)
	Create(name, workdir, command string) error
	Kill(name string) error
	Has(name string) bool
	AttachCmd(name string) *exec.Cmd
}

// CLI implements Tmux by shelling out to the tmux binary.
type CLI struct{}

// New returns a CLI tmux adapter.
func New() *CLI { return &CLI{} }

func (c *CLI) tmux(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmux %s: %w: %s", strings.Join(args, " "), err, errb.String())
	}
	return strings.TrimSpace(out.String()), nil
}

// List returns sessions whose name starts with prefix. An absent tmux server
// (no sessions yet) is treated as an empty list, not an error.
func (c *CLI) List(prefix string) ([]Session, error) {
	out, err := c.tmux("list-sessions", "-F", "#{session_name}")
	if err != nil {
		// "no server running" is normal before any session exists.
		if strings.Contains(err.Error(), "no server running") ||
			strings.Contains(err.Error(), "no sessions") {
			return nil, nil
		}
		return nil, err
	}
	var sessions []Session
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, prefix) {
			continue
		}
		sessions = append(sessions, Session{Name: line, Alive: true})
	}
	return sessions, nil
}

// Create starts a detached session named name, with working directory workdir,
// running command.
func (c *CLI) Create(name, workdir, command string) error {
	_, err := c.tmux("new-session", "-d", "-s", name, "-c", workdir, command)
	return err
}

// Kill terminates the named session. Killing a nonexistent session is an error
// from tmux; callers that don't care should ignore it.
func (c *CLI) Kill(name string) error {
	_, err := c.tmux("kill-session", "-t", name)
	return err
}

// Has reports whether the named session currently exists.
func (c *CLI) Has(name string) bool {
	_, err := c.tmux("has-session", "-t", name)
	return err == nil
}

// AttachCmd returns the command to attach to a session. The caller runs it via
// tea.ExecProcess so the TUI suspends while attached.
func (c *CLI) AttachCmd(name string) *exec.Cmd {
	return exec.Command("tmux", "attach", "-t", name)
}

// Decorate configures a small status bar on the session (scoped to that session
// only) so that while the user is attached they can see which fleet session
// they're in and how to get back to the dashboard. label is shown on the left
// (e.g. "project/session"); the right shows the detach hint using the user's
// actual tmux prefix key. Best-effort: it returns the first error encountered.
func (c *CLI) Decorate(name, label string) error {
	// Escape '#' so tmux doesn't interpret it as a format directive.
	label = strings.ReplaceAll(label, "#", "##")
	prefix := c.prefixKey()
	left := fmt.Sprintf(" #[bold]fleet#[nobold] · %s ", label)
	right := fmt.Sprintf(" %s d → fleet dashboard ", prefix)

	opts := [][]string{
		{"set-option", "-t", name, "status", "on"},
		{"set-option", "-t", name, "status-style", "bg=colour237,fg=colour250"},
		{"set-option", "-t", name, "status-left-length", "80"},
		{"set-option", "-t", name, "status-right-length", "80"},
		{"set-option", "-t", name, "status-left", left},
		{"set-option", "-t", name, "status-right", right},
	}
	for _, args := range opts {
		if _, err := c.tmux(args...); err != nil {
			return err
		}
	}
	return nil
}

// prefixKey returns the user's tmux prefix in human form (e.g. "Ctrl-b"),
// falling back to "Ctrl-b" if it can't be determined.
func (c *CLI) prefixKey() string {
	out, err := c.tmux("show-options", "-gv", "prefix")
	if err != nil {
		return "Ctrl-b"
	}
	return humanizeKey(strings.TrimSpace(out))
}

// humanizeKey turns a tmux key name (e.g. "C-b", "M-x") into a friendlier label.
func humanizeKey(k string) string {
	switch {
	case k == "":
		return "Ctrl-b"
	case strings.HasPrefix(k, "C-"):
		return "Ctrl-" + strings.TrimPrefix(k, "C-")
	case strings.HasPrefix(k, "M-"):
		return "Alt-" + strings.TrimPrefix(k, "M-")
	default:
		return k
	}
}
