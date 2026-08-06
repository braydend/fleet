package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bray/fleet/internal/agent"
	"github.com/bray/fleet/internal/cleanup"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/forge"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/tmux"
)

// tmuxPort is the subset of the tmux adapter the manager uses for window-based
// session lifecycle. Note the asymmetric addressing: CreateWindow and
// LookupWindow take a bare window name (the workspace is implied), while
// KillWindow and RespawnWindow take a full "workspace:name" target as produced
// by naming.WindowTarget.
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
	newID func() string

	// removeTree deletes a worktree directory best-effort; injectable for tests.
	removeTree func(string) (cleanup.Removal, error)
}

// NewManager builds a Manager. clock is injectable for deterministic tests; pass
// time.Now in production. forge may be nil if PR creation is unavailable. newID
// is injectable for deterministic tests; pass nil in production to default to
// naming.NewClaudeSessionID.
func NewManager(cfg config.Config, t tmuxPort, g git.Git, f forge.PRer, clock func() time.Time, newID func() string) *Manager {
	if clock == nil {
		clock = time.Now
	}
	if newID == nil {
		newID = naming.NewClaudeSessionID
	}
	return &Manager{
		cfg: cfg, tmux: t, git: g, forge: f, clock: clock, newID: newID,
		removeTree: cleanup.RemoveTree,
	}
}

// Create makes the worktree, writes meta, and launches the session's window in
// the shared workspace under the chosen agent.
func (m *Manager) Create(p projects.Project, name, branch, base string, ag agent.Agent) (Session, error) {
	wt := naming.WorktreePath(m.cfg.WorktreeBaseDir, p.Name, name)
	if err := m.addWorktreeForBranch(p.Path, wt, branch, base); err != nil {
		return Session{}, err
	}
	// Keep fleet's own .fleet/ bookkeeping out of git status and out of the
	// user's commits.
	if err := m.git.Ignore(wt, ".fleet/"); err != nil {
		return Session{}, err
	}
	now := m.clock()
	// Only agents whose session identity fleet owns get a minted ID; the rest
	// mint their own and are resumed by other means.
	sessionID := ""
	if ag.NeedsID {
		sessionID = m.newID()
	}
	md := meta.Meta{
		Project: p.Name, Session: name, Branch: branch, Base: base,
		RepoPath: p.Path, CreatedAt: now, ClaudeSessionID: sessionID,
		Agent: ag.ID,
	}
	if err := meta.Write(wt, md); err != nil {
		return Session{}, err
	}
	wname := naming.TmuxName(p.Name, name)
	idx, err := m.tmux.CreateWindow(wname, wt, ag.Fresh(sessionID, p.Name+"/"+name))
	if err != nil {
		return Session{}, err
	}
	return Session{
		Project: p.Name, Name: name, Branch: branch, Base: base,
		RepoPath: p.Path, WorktreePath: wt, TmuxName: wname,
		CreatedAt: now, Alive: true, WindowIndex: idx,
		ClaudeSessionID: sessionID, Agent: ag.ID,
	}, nil
}

// addWorktreeForBranch picks the right git worktree command: check out an
// existing local branch, track a remote-only branch, or create a new branch
// from base.
func (m *Manager) addWorktreeForBranch(repoPath, wt, branch, base string) error {
	local, err := m.git.LocalBranchExists(repoPath, branch)
	if err != nil {
		return err
	}
	if local {
		if err := m.git.AddWorktreeExisting(repoPath, wt, branch); err != nil {
			if isAlreadyCheckedOut(err) {
				return fmt.Errorf("branch %q is already checked out in another worktree", branch)
			}
			return err
		}
		return nil
	}
	remote, err := m.git.RemoteBranchExists(repoPath, branch)
	if err != nil {
		return err
	}
	if remote {
		return m.git.AddWorktreeTracking(repoPath, wt, branch)
	}
	return m.git.AddWorktree(repoPath, wt, branch, base)
}

// isAlreadyCheckedOut detects git's "branch is in use by another worktree"
// failure across git versions.
func isAlreadyCheckedOut(err error) bool {
	s := err.Error()
	return strings.Contains(s, "already checked out") || strings.Contains(s, "already used by worktree")
}

// EnsureRunning makes sure the session has a live window, creating it if it is
// missing (e.g. a pre-upgrade session) or respawning it if its process exited.
// Safe to call right before attaching.
func (m *Manager) EnsureRunning(s Session) error {
	ag := agent.Lookup(s.Agent)
	cmd := ag.Resume(s.ClaudeSessionID, s.Project+"/"+s.Name)
	w, ok := m.tmux.LookupWindow(s.TmuxName)
	if !ok {
		_, err := m.tmux.CreateWindow(s.TmuxName, s.WorktreePath, cmd)
		return err
	}
	if w.Dead {
		return m.tmux.RespawnWindow(naming.WindowTarget(s.Project, s.Name), s.WorktreePath, cmd)
	}
	return nil
}

// Leave ends the running Claude instance but keeps the worktree and branch.
func (m *Manager) Leave(s Session) error {
	_ = m.tmux.KillWindow(naming.WindowTarget(s.Project, s.Name)) // ignore: may already be gone
	return nil
}

// DeleteResult reports what a Delete left behind on disk.
type DeleteResult struct {
	Leftover      string // worktree path if files survived; "" when fully clean
	LeftoverCount int
	Samples       []string
}

// Delete kills the session's window, removes the worktree, reconciles the
// repo's worktree registry, and optionally deletes the branch.
//
// Files it cannot delete — typically root-owned output written by a container
// into the bind-mounted worktree — do not fail the delete. The session is
// always forgotten (meta removed, registry pruned, branch deleted) and the
// leftovers are reported, so a blocked file can never strand an un-removable
// session on the dashboard.
func (m *Manager) Delete(s Session, deleteBranch bool) (DeleteResult, error) {
	// Safety net: fleet deletes trees itself now, so refuse anything that is
	// not strictly inside its own worktree base dir. A corrupt meta must never
	// be able to point the walk at a real repo.
	if !underBase(m.cfg.WorktreeBaseDir, s.WorktreePath) {
		return DeleteResult{}, fmt.Errorf("refusing to delete %s: outside worktree base dir %s",
			s.WorktreePath, m.cfg.WorktreeBaseDir)
	}

	_ = m.tmux.KillWindow(naming.WindowTarget(s.Project, s.Name)) // ignore: may already be gone

	rem, err := m.removeTree(s.WorktreePath)
	if err != nil {
		return DeleteResult{}, err
	}

	// Whatever survived, make sure the directory is neither a git worktree nor
	// a fleet session any more, so neither git nor the dashboard keeps tracking
	// it. Both live at the top of the worktree and are fleet/git-owned, so they
	// are removable even when the tree below is not.
	_ = os.RemoveAll(filepath.Join(s.WorktreePath, ".git"))
	_ = os.RemoveAll(filepath.Join(s.WorktreePath, ".fleet"))
	if _, statErr := os.Stat(meta.Path(s.WorktreePath)); statErr == nil {
		return DeleteResult{}, fmt.Errorf("could not remove %s: the session would stay on the dashboard",
			meta.Path(s.WorktreePath))
	}

	if err := m.git.PruneWorktrees(s.RepoPath); err != nil {
		return DeleteResult{}, err
	}
	if deleteBranch {
		// After the prune git no longer believes the branch is checked out, so
		// this succeeds even for a worktree whose files could not be removed.
		if err := m.git.DeleteBranch(s.RepoPath, s.Branch, true); err != nil {
			return DeleteResult{}, err
		}
	}

	if rem.Complete() {
		return DeleteResult{}, nil
	}
	return DeleteResult{
		Leftover:      s.WorktreePath,
		LeftoverCount: rem.BlockedCount,
		Samples:       rem.Blocked,
	}, nil
}

// underBase reports whether path sits strictly inside base.
func underBase(base, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
