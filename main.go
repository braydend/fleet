// Command fleet is a TUI for managing multiple isolated Claude Code sessions.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/forge"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/refresher"
	"github.com/bray/fleet/internal/session"
	"github.com/bray/fleet/internal/tmux"
	"github.com/bray/fleet/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fleet:", err)
		os.Exit(1)
	}
}

func run() error {
	// Dependency check.
	for _, bin := range []string{"git", "tmux", "claude"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Errorf("required command %q not found on PATH", bin)
		}
	}

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	g := git.New()
	tm := tmux.New()
	fg := forge.New()
	mgr := session.NewManager(cfg, tm, g, fg, time.Now)

	actions := ui.Actions{
		Refresh: func() ([]session.Session, error) {
			return refresher.Build(cfg, tm, g)
		},
		Projects: func() ([]projects.Project, error) {
			return projects.Scan(cfg.ScanRoot, g)
		},
		Create: func(p projects.Project, name, branch, base string) error {
			_, err := mgr.Create(p, name, branch, base)
			return err
		},
		Delete: mgr.Delete,
		Leave:  mgr.Leave,
		PushPR: mgr.PushPR,
		Attach: func(s session.Session) tea.Cmd {
			return tea.ExecProcess(mgr.AttachCmd(s), func(err error) tea.Msg {
				// After detaching, refresh the list.
				ss, rerr := refresher.Build(cfg, tm, g)
				if rerr != nil {
					return ui.ErrorMsgFor(rerr)
				}
				return ui.SessionsMsgFor(ss)
			})
		},
	}

	p := tea.NewProgram(ui.New(&actions, nil), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
