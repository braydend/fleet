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
	if err := m.tmux.Create(tname, wt, "claude"); err != nil {
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
