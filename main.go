// Command fleet is a TUI for managing multiple isolated Claude Code sessions.
package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/forge"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/refresher"
	"github.com/bray/fleet/internal/selfupdate"
	"github.com/bray/fleet/internal/session"
	"github.com/bray/fleet/internal/tmux"
	"github.com/bray/fleet/internal/ui"
)

// Build metadata, overridden at release time via -ldflags -X main.version=...
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// versionRequested reports whether argv asks for the version.
func versionRequested(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[1] {
	case "--version", "-v", "version":
		return true
	}
	return false
}

// versionLine is the single line printed by --version.
func versionLine() string {
	return fmt.Sprintf("fleet %s (%s, %s)", version, commit, date)
}

func main() {
	if versionRequested(os.Args) {
		fmt.Println(versionLine())
		return
	}
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

	cfgPath := config.DefaultPath()
	var cfg config.Config
	var err error
	if config.Exists(cfgPath) {
		cfg, err = config.Load(cfgPath)
	} else {
		// First run: prompt for the scan root and write the config file.
		cfg, err = config.Setup(cfgPath, os.Stdin, os.Stdout)
	}
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	g := git.New()
	// Run on fleet's own tmux server (default socket "fleet") so fleet is fully
	// isolated from the user's personal tmux — see config.TmuxSocket. An empty
	// socket falls back to the default tmux server.
	tm := tmux.NewWithSocket(cfg.TmuxSocket)
	fg := forge.New()
	const repo = "braydend/fleet"
	statePath := selfupdate.StatePath()
	checker := selfupdate.Checker{Repo: repo, Client: http.DefaultClient}
	applier := selfupdate.Applier{Client: http.DefaultClient, Updater: selfupdate.MinioUpdater{}}
	mgr := session.NewManager(cfg, tm, g, fg, time.Now)

	actions := ui.Actions{
		Refresh: func() ([]session.Session, error) {
			return refresher.Build(cfg, tm, g, time.Now)
		},
		Projects: func() ([]projects.Project, error) {
			return projects.Scan(cfg.ScanRoot, g)
		},
		Create: func(p projects.Project, name, branch, base string) error {
			_, err := mgr.Create(p, name, branch, base)
			return err
		},
		Branches: func(p projects.Project) (git.Branches, error) {
			return g.ListBranches(p.Path)
		},
		FetchBranches: func(p projects.Project) (git.Branches, error) {
			ferr := g.Fetch(p.Path)
			br, lerr := g.ListBranches(p.Path) // best-effort even if fetch failed
			if lerr != nil {
				return git.Branches{}, lerr
			}
			return br, ferr
		},
		Delete: mgr.Delete,
		Leave:  mgr.Leave,
		PushPR: mgr.PushPR,
		CheckUpdate: func() (selfupdate.CheckResult, error) {
			st := selfupdate.LoadState(statePath)
			if !st.Due(time.Now()) {
				return selfupdate.CheckResult{}, nil // throttled: no network
			}
			res, err := checker.Check(version)
			// Record the attempt regardless of outcome so a hard-down network
			// doesn't cause a tight retry loop.
			_ = selfupdate.SaveState(statePath, selfupdate.State{LastChecked: time.Now()})
			return res, err
		},
		ApplyUpdate: func(rel selfupdate.Release) error {
			if err := applier.Apply(rel); err != nil {
				if selfupdate.IsPermission(err) {
					return fmt.Errorf("can't replace binary (permission denied) — %s", selfupdate.ManualInstallHint())
				}
				return err
			}
			return nil
		},
		Attach: func(s session.Session) tea.Cmd {
			if err := mgr.EnsureRunning(s); err != nil {
				return func() tea.Msg { return ui.ErrorMsgFor(err) }
			}
			// (Re)apply fleet's workspace options every attach — most importantly
			// exit-empty off and per-window remain-on-exit — so a workspace that
			// survived a previous run (or predates this config) can't tear the
			// server down when a window's process exits. See issue #5.
			_ = tm.EnsureConfigured()
			// Configure the tab strip + switch keys (best-effort).
			_ = tm.ConfigureTabs()
			// Resolve the current index by name (it may have been renumbered) and
			// select it before attaching.
			if w, ok := tm.LookupWindow(s.TmuxName); ok {
				_ = tm.SelectWindow(w.Index)
			}
			return tea.ExecProcess(tm.AttachWorkspaceCmd(), func(err error) tea.Msg {
				ss, rerr := refresher.Build(cfg, tm, g, time.Now)
				if rerr != nil {
					return ui.ErrorMsgFor(rerr)
				}
				return ui.SessionsMsgFor(ss)
			})
		},
	}

	p := tea.NewProgram(ui.New(&actions, version), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
