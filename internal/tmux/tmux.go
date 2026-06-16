// Package tmux provides a thin adapter over the tmux CLI behind the Tmux
// interface. fleet runs each Claude Code instance in its own tmux session.
package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
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

// Window is one window in the shared fleet workspace session.
type Window struct {
	Index        int
	Name         string    // stable identity: the fleet-<proj>-<sess> name
	Dead         bool      // process exited but window kept by remain-on-exit
	LastActivity time.Time
}

// workspace is the shared session name. Kept local to avoid importing naming;
// it must match naming.Workspace.
const workspace = "fleet-workspace"

func (c *CLI) hasSession(name string) bool {
	_, err := c.tmux("has-session", "-t", name)
	return err == nil
}

// KillWorkspace removes the whole shared session (used by tests and when the
// last window is gone). Killing an absent session is not treated as an error.
func (c *CLI) KillWorkspace() error {
	if !c.hasSession(workspace) {
		return nil
	}
	_, err := c.tmux("kill-session", "-t", workspace)
	return err
}

// CreateWindow ensures the workspace exists and adds a window named name
// running command in workdir. Returns the new window's 1-based index.
func (c *CLI) CreateWindow(name, workdir, command string) (int, error) {
	if !c.hasSession(workspace) {
		if _, err := c.tmux("new-session", "-d", "-s", workspace, "-n", name, "-c", workdir, command); err != nil {
			return 0, err
		}
		for _, opt := range [][]string{
			{"set-option", "-t", workspace, "base-index", "1"},
			{"set-option", "-t", workspace, "renumber-windows", "on"},
			{"set-option", "-t", workspace, "automatic-rename", "off"},
			{"set-option", "-t", workspace, "remain-on-exit", "on"},
		} {
			if _, err := c.tmux(opt...); err != nil {
				return 0, err
			}
		}
		// Renumber so windows start at base-index (1), regardless of the user's
		// global base-index. The single bootstrap window becomes index 1, so
		// Alt-1 selects it.
		if _, err := c.tmux("move-window", "-r", "-t", workspace); err != nil {
			return 0, err
		}
		return 1, nil
	}
	out, err := c.tmux("new-window", "-P", "-F", "#{window_index}", "-t", workspace, "-n", name, "-c", workdir, command)
	if err != nil {
		return 0, err
	}
	idx, _ := strconv.Atoi(strings.TrimSpace(out))
	return idx, nil
}

// ListWindows returns the workspace's windows. An absent workspace (or no tmux
// server) is an empty list, not an error.
func (c *CLI) ListWindows() ([]Window, error) {
	out, err := c.tmux("list-windows", "-t", workspace, "-F",
		"#{window_index}\t#{window_name}\t#{pane_dead}\t#{window_activity}")
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "no server running") ||
			strings.Contains(msg, "can't find session") ||
			strings.Contains(msg, "no sessions") {
			return nil, nil
		}
		return nil, err
	}
	var ws []Window
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 {
			continue
		}
		idx, _ := strconv.Atoi(f[0])
		secs, _ := strconv.ParseInt(f[3], 10, 64)
		ws = append(ws, Window{
			Index:        idx,
			Name:         f[1],
			Dead:         f[2] == "1",
			LastActivity: time.Unix(secs, 0),
		})
	}
	return ws, nil
}

// LookupWindow finds a window by its stable name. ok is false if absent.
func (c *CLI) LookupWindow(name string) (Window, bool) {
	ws, err := c.ListWindows()
	if err != nil {
		return Window{}, false
	}
	for _, w := range ws {
		if w.Name == name {
			return w, true
		}
	}
	return Window{}, false
}

// KillWindow removes a window by target ("workspace:name").
func (c *CLI) KillWindow(target string) error {
	_, err := c.tmux("kill-window", "-t", target)
	return err
}

// RespawnWindow restarts the process in an existing (dead) window.
func (c *CLI) RespawnWindow(target, workdir, command string) error {
	_, err := c.tmux("respawn-window", "-t", target, "-c", workdir, command)
	return err
}

// CapturePane returns the visible text of a window's active pane.
func (c *CLI) CapturePane(target string) (string, error) {
	return c.tmux("capture-pane", "-p", "-t", target)
}

// SetWindowLabel sets the @fleet_label window option used to render the tab.
func (c *CLI) SetWindowLabel(target, label string) error {
	_, err := c.tmux("set-option", "-w", "-t", target, "@fleet_label", label)
	return err
}

// SelectWindow makes the given 1-based window index active in the workspace.
func (c *CLI) SelectWindow(index int) error {
	_, err := c.tmux("select-window", "-t", fmt.Sprintf("%s:%d", workspace, index))
	return err
}

// AttachWorkspaceCmd returns the command to attach to the shared workspace, for
// use with tea.ExecProcess. Select the target window first via SelectWindow.
func (c *CLI) AttachWorkspaceCmd() *exec.Cmd {
	return exec.Command("tmux", "attach", "-t", workspace)
}

// ConfigureTabs sets the workspace status bar to render windows as numbered
// tabs (using each window's @fleet_label) and binds prefix-less switch keys:
// Alt-1..9 jump to a tab, Alt-Left/Right move prev/next. Best-effort; it
// returns the first error encountered.
func (c *CLI) ConfigureTabs() error {
	prefix := c.prefixKey()
	opts := [][]string{
		{"set-option", "-t", workspace, "status", "on"},
		{"set-option", "-t", workspace, "status-style", "bg=colour237,fg=colour250"},
		{"set-option", "-t", workspace, "status-left", " #[bold]fleet#[nobold] "},
		{"set-option", "-t", workspace, "status-left-length", "20"},
		{"set-option", "-t", workspace, "status-right", fmt.Sprintf(" %s d → dashboard ", prefix)},
		{"set-option", "-t", workspace, "status-right-length", "40"},
		{"set-option", "-t", workspace, "window-status-format", " #{window_index} #{@fleet_label} "},
		{"set-option", "-t", workspace, "window-status-current-format", "#[reverse] #{window_index} #{@fleet_label} #[noreverse]"},
	}
	for _, o := range opts {
		if _, err := c.tmux(o...); err != nil {
			return err
		}
	}
	// Prefix-less switch keys (global to this tmux server; fleet owns it).
	for i := 1; i <= 9; i++ {
		n := strconv.Itoa(i)
		if _, err := c.tmux("bind-key", "-n", "M-"+n, "select-window", "-t", ":"+n); err != nil {
			return err
		}
	}
	if _, err := c.tmux("bind-key", "-n", "M-Left", "previous-window"); err != nil {
		return err
	}
	if _, err := c.tmux("bind-key", "-n", "M-Right", "next-window"); err != nil {
		return err
	}
	return nil
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
