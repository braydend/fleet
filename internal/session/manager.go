package session

import (
	"os/exec"
	"time"

	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/forge"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/projects"
)

// claudeCommand is the command launched inside each session's tmux session.
const claudeCommand = "claude"

// tmuxPort is the subset of tmux.Tmux the manager uses for lifecycle ops.
// AttachCmd is exposed separately so tests can supply a fake without os/exec.
type tmuxPort interface {
	Create(name, workdir, command string) error
	Kill(name string) error
	Has(name string) bool
}

// attacher can produce the attach command; satisfied by *tmux.CLI.
type attacher interface {
	AttachCmd(name string) *exec.Cmd
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

// Create makes the worktree, writes meta, and launches the tmux session.
func (m *Manager) Create(p projects.Project, name, branch, base string) (Session, error) {
	wt := naming.WorktreePath(m.cfg.WorktreeBaseDir, p.Name, name)
	if err := m.git.AddWorktree(p.Path, wt, branch, base); err != nil {
		return Session{}, err
	}
	// Keep fleet's own .fleet/ bookkeeping out of git status and out of the
	// user's commits.
	if err := m.git.Ignore(wt, ".fleet/"); err != nil {
		return Session{}, err
	}
	now := m.clock()
	md := meta.Meta{
		Project:   p.Name,
		Session:   name,
		Branch:    branch,
		Base:      base,
		RepoPath:  p.Path,
		CreatedAt: now,
	}
	if err := meta.Write(wt, md); err != nil {
		return Session{}, err
	}
	tname := naming.TmuxName(p.Name, name)
	if err := m.tmux.Create(tname, wt, claudeCommand); err != nil {
		return Session{}, err
	}
	return Session{
		Project:      p.Name,
		Name:         name,
		Branch:       branch,
		Base:         base,
		RepoPath:     p.Path,
		WorktreePath: wt,
		TmuxName:     tname,
		CreatedAt:    now,
		Alive:        true,
	}, nil
}

// Kill terminates the tmux session (ignoring "no such session").
func (m *Manager) Kill(s Session) error {
	return m.tmux.Kill(s.TmuxName)
}

// EnsureRunning makes sure the session's tmux session exists, recreating it (and
// relaunching claude in the existing worktree) if it has exited — e.g. because
// claude was quit with Ctrl-D. It is a no-op when the session is already alive,
// so it is safe to call right before attaching.
func (m *Manager) EnsureRunning(s Session) error {
	if m.tmux.Has(s.TmuxName) {
		return nil
	}
	return m.tmux.Create(s.TmuxName, s.WorktreePath, claudeCommand)
}

// Leave ends the running Claude instance but keeps the worktree and branch.
func (m *Manager) Leave(s Session) error {
	_ = m.tmux.Kill(s.TmuxName) // ignore: session may already be gone
	return nil
}

// Delete kills the tmux session, removes the worktree, and optionally deletes
// the branch. The worktree removal is forced because the caller has already
// confirmed any dirty/unpushed state.
func (m *Manager) Delete(s Session, deleteBranch bool) error {
	_ = m.tmux.Kill(s.TmuxName)
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

// AttachCmd returns the command to attach to the session's tmux, for use with
// tea.ExecProcess. Requires the tmux port to also implement attacher.
func (m *Manager) AttachCmd(s Session) *exec.Cmd {
	if a, ok := m.tmux.(attacher); ok {
		return a.AttachCmd(s.TmuxName)
	}
	return exec.Command("tmux", "attach", "-t", s.TmuxName)
}
