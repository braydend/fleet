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
//
// socket, when non-empty, pins every tmux invocation to a dedicated server via
// `tmux -L <socket>`. Production uses the empty default (the user's server);
// tests MUST use a private socket so their destructive operations (KillWorkspace
// etc.) can never reach the user's real sessions — see issue #5.
type CLI struct{ socket string }

// New returns a CLI tmux adapter on the default tmux server.
func New() *CLI { return &CLI{} }

// NewWithSocket returns a CLI pinned to a private tmux server (`tmux -L
// socket`). Used by tests for isolation.
func NewWithSocket(socket string) *CLI { return &CLI{socket: socket} }

// withSocket prepends the `-L <socket>` server flag when this CLI is pinned to a
// private socket; otherwise it returns args unchanged.
func (c *CLI) withSocket(args ...string) []string {
	if c.socket == "" {
		return args
	}
	return append([]string{"-L", c.socket}, args...)
}

func (c *CLI) tmux(args ...string) (string, error) {
	cmd := exec.Command("tmux", c.withSocket(args...)...)
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
	return exec.Command("tmux", c.withSocket("attach", "-t", name)...)
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
	Name         string // stable identity: the fleet-<proj>-<sess> name
	Dead         bool   // process exited but window kept by remain-on-exit
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

// configureWorkspace applies the session- and server-scoped options fleet
// relies on. It is idempotent and MUST run whether or not fleet created the
// session this run: tmux sessions persist across fleet restarts, so a workspace
// that already exists would otherwise keep tmux's defaults.
//
// exit-empty (a server option) is turned off so a momentarily-empty workspace
// can never make the whole tmux server exit and tear down every session at once
// — a key part of the issue #5 crash.
func (c *CLI) configureWorkspace() error {
	for _, opt := range [][]string{
		{"set-option", "-s", "exit-empty", "off"},
		{"set-option", "-t", workspace, "base-index", "1"},
		{"set-option", "-t", workspace, "renumber-windows", "on"},
	} {
		if _, err := c.tmux(opt...); err != nil {
			return err
		}
	}
	return nil
}

// configureWindow applies the per-window options fleet relies on to the window
// at target ("workspace:index"). These MUST be set at window scope: in modern
// tmux remain-on-exit/automatic-rename are pane/window options, and a
// session-scoped set is silently ineffective for the window's pane.
//
// remain-on-exit keeps the window as a dead pane when its process exits, instead
// of letting the window vanish (which, for the last window, destroys the session
// and bounces the attached user to the dashboard — the issue #5 crash).
// automatic-rename off keeps the fleet-<proj>-<sess> name stable for lookups and
// tab labels.
func (c *CLI) configureWindow(target string) error {
	for _, opt := range [][]string{
		{"set-option", "-w", "-t", target, "remain-on-exit", "on"},
		{"set-option", "-w", "-t", target, "automatic-rename", "off"},
	} {
		if _, err := c.tmux(opt...); err != nil {
			return err
		}
	}
	return nil
}

// EnsureConfigured (re)applies fleet's workspace and per-window options to an
// existing workspace. It is safe to call on every attach: it is idempotent and a
// no-op when the workspace does not exist. This protects workspaces that fleet
// did not create this run (they survive restarts) and retro-fits options onto
// windows created before this configuration existed.
func (c *CLI) EnsureConfigured() error {
	if !c.hasSession(workspace) {
		return nil
	}
	if err := c.configureWorkspace(); err != nil {
		return err
	}
	ws, err := c.ListWindows()
	if err != nil {
		return err
	}
	for _, w := range ws {
		if err := c.configureWindow(fmt.Sprintf("%s:%d", workspace, w.Index)); err != nil {
			return err
		}
	}
	return nil
}

// CreateWindow ensures the workspace exists and adds a window named name
// running command in workdir. Returns the new window's 1-based index.
func (c *CLI) CreateWindow(name, workdir, command string) (int, error) {
	if !c.hasSession(workspace) {
		if _, err := c.tmux("new-session", "-d", "-s", workspace, "-n", name, "-c", workdir, command); err != nil {
			return 0, err
		}
		if err := c.configureWorkspace(); err != nil {
			return 0, err
		}
		// Renumber so windows start at base-index (1), regardless of the user's
		// global base-index. The single bootstrap window becomes index 1, so
		// Alt-1 selects it.
		if _, err := c.tmux("move-window", "-r", "-t", workspace); err != nil {
			return 0, err
		}
		if err := c.configureWindow(fmt.Sprintf("%s:1", workspace)); err != nil {
			return 0, err
		}
		return 1, nil
	}
	// The workspace already exists (e.g. it survived a previous fleet run).
	// Re-apply session/server config so options like exit-empty are present even
	// though we didn't create the session this run.
	if err := c.configureWorkspace(); err != nil {
		return 0, err
	}
	out, err := c.tmux("new-window", "-P", "-F", "#{window_index}", "-t", workspace, "-n", name, "-c", workdir, command)
	if err != nil {
		return 0, err
	}
	idx, _ := strconv.Atoi(strings.TrimSpace(out))
	if err := c.configureWindow(fmt.Sprintf("%s:%d", workspace, idx)); err != nil {
		return 0, err
	}
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
			strings.Contains(msg, "no current target") ||
			strings.Contains(msg, "error connecting to") || // named socket, server not started yet
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
	return exec.Command("tmux", c.withSocket("attach", "-t", workspace)...)
}

// ConfigureTabs sets the workspace status bar to render windows as numbered
// tabs (using each window's @fleet_label) and binds prefix-less switch keys:
// Alt-1..9 jump to a tab, Alt-Left/Right move prev/next. It also enables mouse
// mode so the wheel scrolls scrollback (issue #2) and rebinds mouse paste/copy
// to the system clipboard so mouse mode doesn't break paste (issue #14). The
// status bar advertises Shift+right-click, the always-available terminal-native
// paste bypass that works even without a clipboard CLI installed (issue #14).
// Best-effort; it returns the first error encountered.
func (c *CLI) ConfigureTabs() error {
	prefix := c.prefixKey()
	opts := [][]string{
		// Mouse mode on: the wheel scrolls tmux scrollback instead of being
		// forwarded to the inner program as arrow keys (issue #2).
		{"set-option", "-t", workspace, "mouse", "on"},
		{"set-option", "-t", workspace, "status", "on"},
		{"set-option", "-t", workspace, "status-style", "bg=colour237,fg=colour250"},
		{"set-option", "-t", workspace, "status-left", " #[bold]fleet#[nobold] "},
		{"set-option", "-t", workspace, "status-left-length", "20"},
		{"set-option", "-t", workspace, "status-right", fmt.Sprintf(" Shift+RightClick paste · %s d → dashboard ", prefix)},
		{"set-option", "-t", workspace, "status-right-length", "60"},
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
	// Mouse mode (above) makes the terminal forward clicks to tmux, which breaks
	// native right-click/middle-click paste; rebind them to the system clipboard
	// so paste keeps working (issue #14).
	return c.bindMouseClipboard()
}

// clipboardTool describes how to read and write the system clipboard via an
// external CLI. fleet binds tmux's mouse paste/copy to these so paste works
// even with mouse mode on (issue #14): once mouse reporting is enabled the
// terminal forwards clicks to tmux instead of pasting natively, so tmux has to
// perform the paste itself by shelling out to the clipboard tool.
type clipboardTool struct {
	copy         string // shell command reading stdin into the clipboard
	paste        string // shell command writing the clipboard to stdout
	pastePrimary string // shell command writing the primary selection to stdout
}

// detectClipboard returns the first available clipboard helper, preferring
// Wayland (wl-clipboard), then X11 (xclip, xsel), then macOS (pbcopy/pbpaste).
// ok is false when none is on PATH — fleet then leaves mouse paste unbound, and
// wheel-scroll plus terminal-level Ctrl+Shift+V keep working regardless. look
// is injected (exec.LookPath in production) so the choice is unit-testable.
func detectClipboard(look func(string) (string, error)) (clipboardTool, bool) {
	has := func(bin string) bool { _, err := look(bin); return err == nil }
	switch {
	case has("wl-copy") && has("wl-paste"):
		return clipboardTool{
			copy:         "wl-copy",
			paste:        "wl-paste --no-newline",
			pastePrimary: "wl-paste --no-newline --primary",
		}, true
	case has("xclip"):
		return clipboardTool{
			copy:         "xclip -selection clipboard -in",
			paste:        "xclip -selection clipboard -out",
			pastePrimary: "xclip -selection primary -out",
		}, true
	case has("xsel"):
		return clipboardTool{
			copy:         "xsel --clipboard --input",
			paste:        "xsel --clipboard --output",
			pastePrimary: "xsel --primary --output",
		}, true
	case has("pbcopy") && has("pbpaste"):
		// macOS has no primary selection; middle-click falls back to clipboard.
		return clipboardTool{copy: "pbcopy", paste: "pbpaste", pastePrimary: "pbpaste"}, true
	}
	return clipboardTool{}, false
}

// bindMouseClipboard rebinds the mouse paste/copy events that mouse mode steals
// from the terminal (issue #14): right-click pastes the clipboard, middle-click
// pastes the primary selection, and finishing a drag-select copies into the
// clipboard. Pastes use `paste-buffer -p` (bracketed) so multi-line text is not
// auto-submitted by the inner program. It is a no-op when no clipboard CLI is
// installed. The inner `tmux` calls inherit $TMUX, so they reach fleet's server.
func (c *CLI) bindMouseClipboard() error {
	clip, ok := detectClipboard(exec.LookPath)
	if !ok {
		return nil
	}
	pasteFrom := func(read string) string {
		return fmt.Sprintf(`tmux set-buffer -- "$(%s)" && tmux paste-buffer -p`, read)
	}
	for _, b := range [][]string{
		{"set-option", "-t", workspace, "set-clipboard", "on"},
		// Right-click pastes the clipboard, middle-click the primary selection.
		{"bind-key", "-n", "MouseDown3Pane", "run-shell", pasteFrom(clip.paste)},
		{"bind-key", "-n", "MouseDown2Pane", "run-shell", pasteFrom(clip.pastePrimary)},
		// Finishing a drag-select copies into the clipboard. copy-pipe is a
		// copy-mode command, so it must be bound in the copy-mode key tables
		// (emacs and vi) via send-keys -X, not the root table.
		{"bind-key", "-T", "copy-mode", "MouseDragEnd1Pane", "send-keys", "-X", "copy-pipe-and-cancel", clip.copy},
		{"bind-key", "-T", "copy-mode-vi", "MouseDragEnd1Pane", "send-keys", "-X", "copy-pipe-and-cancel", clip.copy},
	} {
		if _, err := c.tmux(b...); err != nil {
			return err
		}
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
