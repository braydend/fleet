package session

import (
	"time"

	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/forge"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/tmux"
)

// claudeCommand is the command launched inside each session's tmux window.
const claudeCommand = "claude"

// tmuxPort is the subset of the tmux adapter the manager uses for window-based
// session lifecycle.
type tmuxPort interface {
	CreateWindow(name, workdir, command string) (int, error)
	KillWindow(target string) error
	RespawnWindow(target, workdir, command string) error
	LookupWindow(name string) (tmux.Window, bool)
}

// Manager creates and tears down sessions by composing git, meta and tmux.
type Manager struct {
	cfg   config.Config
	tmux  tmuxPort
	git   git.Git
	forge forge.PRer
	clock func() time.Time
}

// NewManager builds a Manager. clock is injectable for deterministic tests; pass
// time.Now in production. forge may be nil if PR creation is unavailable.
func NewManager(cfg config.Config, t tmuxPort, g git.Git, f forge.PRer, clock func() time.Time) *Manager {
	if clock == nil {
		clock = time.Now
	}
	return &Manager{cfg: cfg, tmux: t, git: g, forge: f, clock: clock}
}

// Create makes the worktree, writes meta, and launches the session's window in
// the shared workspace.
func (m *Manager) Create(p projects.Project, name, branch, base string) (Session, error) {
	wt := naming.WorktreePath(m.cfg.WorktreeBaseDir, p.Name, name)
	if err := m.git.AddWorktree(p.Path, wt, branch, base); err != nil {
		return Session{}, err
	}
	if err := m.git.Ignore(wt, ".fleet/"); err != nil {
		return Session{}, err
	}
	now := m.clock()
	md := meta.Meta{
		Project: p.Name, Session: name, Branch: branch, Base: base,
		RepoPath: p.Path, CreatedAt: now,
	}
	if err := meta.Write(wt, md); err != nil {
		return Session{}, err
	}
	wname := naming.TmuxName(p.Name, name)
	idx, err := m.tmux.CreateWindow(wname, wt, claudeCommand)
	if err != nil {
		return Session{}, err
	}
	return Session{
		Project: p.Name, Name: name, Branch: branch, Base: base,
		RepoPath: p.Path, WorktreePath: wt, TmuxName: wname,
		CreatedAt: now, Alive: true, WindowIndex: idx,
	}, nil
}

// EnsureRunning makes sure the session has a live window, creating it if it is
// missing (e.g. a pre-upgrade session) or respawning it if its process exited.
// Safe to call right before attaching.
func (m *Manager) EnsureRunning(s Session) error {
	w, ok := m.tmux.LookupWindow(s.TmuxName)
	if !ok {
		_, err := m.tmux.CreateWindow(s.TmuxName, s.WorktreePath, claudeCommand)
		return err
	}
	if w.Dead {
		return m.tmux.RespawnWindow(naming.WindowTarget(s.Project, s.Name), s.WorktreePath, claudeCommand)
	}
	return nil
}

// Leave ends the running Claude instance but keeps the worktree and branch.
func (m *Manager) Leave(s Session) error {
	_ = m.tmux.KillWindow(naming.WindowTarget(s.Project, s.Name)) // ignore: may already be gone
	return nil
}

// Delete kills the session's window, removes the worktree, and optionally
// deletes the branch.
func (m *Manager) Delete(s Session, deleteBranch bool) error {
	_ = m.tmux.KillWindow(naming.WindowTarget(s.Project, s.Name))
	if err := m.git.RemoveWorktree(s.RepoPath, s.WorktreePath, true); err != nil {
		return err
	}
	if deleteBranch {
		if err := m.git.DeleteBranch(s.RepoPath, s.Branch, true); err != nil {
			return err
		}
	}
	return nil
}

// PushPR pushes the branch and, if a forge is configured and available, opens a
// pull request.
func (m *Manager) PushPR(s Session) error {
	if err := m.git.Push(s.WorktreePath, s.Branch); err != nil {
		return err
	}
	if m.forge != nil && m.forge.Available() {
		return m.forge.OpenPR(s.WorktreePath)
	}
	return nil
}
