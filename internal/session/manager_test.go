package session

import (
	"testing"
	"time"

	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/projects"
)

// --- fakes ---

type fakeGit struct {
	added   []string // worktree paths added
	removed []string
	deleted []string
	status  git.Status
}

func (f *fakeGit) DefaultBranch(string) (string, error) { return "main", nil }
func (f *fakeGit) AddWorktree(_, wt, _, _ string) error { f.added = append(f.added, wt); return nil }
func (f *fakeGit) RemoveWorktree(_, wt string, _ bool) error {
	f.removed = append(f.removed, wt)
	return nil
}
func (f *fakeGit) DeleteBranch(_, b string, _ bool) error { f.deleted = append(f.deleted, b); return nil }
func (f *fakeGit) Status(string) (git.Status, error)      { return f.status, nil }
func (f *fakeGit) Push(string, string) error              { return nil }
func (f *fakeGit) IsRepo(string) bool                     { return true }
func (f *fakeGit) Ignore(string, string) error            { return nil }

type fakeTmux struct {
	created []string
	killed  []string
	alive   map[string]bool
}

func (f *fakeTmux) Create(name, _, _ string) error {
	f.created = append(f.created, name)
	if f.alive == nil {
		f.alive = map[string]bool{}
	}
	f.alive[name] = true
	return nil
}
func (f *fakeTmux) Kill(name string) error { f.killed = append(f.killed, name); delete(f.alive, name); return nil }
func (f *fakeTmux) Has(name string) bool   { return f.alive[name] }

// fakeTmux satisfies tmuxPort (Create, Kill, Has only — no AttachCmd, no List).

func newManager(t *testing.T, g git.Git, tm tmuxPort) (*Manager, config.Config) {
	cfg := config.Config{ScanRoot: "/code", WorktreeBaseDir: t.TempDir()}
	fixed := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	m := NewManager(cfg, tm, g, nil, func() time.Time { return fixed })
	return m, cfg
}

func TestCreateAddsWorktreeMetaAndTmux(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, cfg := newManager(t, fg, ft)

	proj := projects.Project{Name: "My App", Path: "/code/my-app", DefaultBranch: "main"}
	s, err := m.Create(proj, "fix-bug", "fleet/fix-bug", "main")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if len(fg.added) != 1 {
		t.Fatalf("expected 1 worktree add, got %v", fg.added)
	}
	if len(ft.created) != 1 || ft.created[0] != "fleet-My_App-fix_bug" {
		t.Fatalf("unexpected tmux create: %v", ft.created)
	}
	// meta.json should be readable from the worktree.
	md, err := meta.Read(s.WorktreePath)
	if err != nil {
		t.Fatalf("meta read: %v", err)
	}
	if md.Branch != "fleet/fix-bug" || md.Base != "main" || md.RepoPath != "/code/my-app" {
		t.Fatalf("unexpected meta: %+v", md)
	}
	if s.TmuxName != "fleet-My_App-fix_bug" || !s.Alive {
		t.Fatalf("unexpected session: %+v", s)
	}
	_ = cfg
}

func TestLeaveKillsTmuxOnly(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	s := Session{TmuxName: "fleet-p-s", WorktreePath: "/wt", RepoPath: "/r", Branch: "fleet/s"}
	if err := m.Leave(s); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if len(ft.killed) != 1 || len(fg.removed) != 0 {
		t.Fatalf("leave should kill tmux only: killed=%v removed=%v", ft.killed, fg.removed)
	}
}

func TestDeleteKillsRemovesAndOptionallyDropsBranch(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	s := Session{TmuxName: "fleet-p-s", WorktreePath: "/wt", RepoPath: "/r", Branch: "fleet/s"}

	if err := m.Delete(s, false); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(fg.removed) != 1 || len(fg.deleted) != 0 {
		t.Fatalf("expected remove only: removed=%v deleted=%v", fg.removed, fg.deleted)
	}

	if err := m.Delete(s, true); err != nil {
		t.Fatalf("delete+branch: %v", err)
	}
	if len(fg.deleted) != 1 || fg.deleted[0] != "fleet/s" {
		t.Fatalf("expected branch delete, got %v", fg.deleted)
	}
}

func TestEnsureRunningNoopWhenAlive(t *testing.T) {
	ft := &fakeTmux{alive: map[string]bool{"fleet-p-s": true}}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{TmuxName: "fleet-p-s", WorktreePath: "/wt"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.created) != 0 {
		t.Fatalf("expected no tmux create for a live session, got %v", ft.created)
	}
}

func TestEnsureRunningRecreatesWhenDead(t *testing.T) {
	ft := &fakeTmux{}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{TmuxName: "fleet-p-s", WorktreePath: "/wt"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.created) != 1 || ft.created[0] != "fleet-p-s" {
		t.Fatalf("expected tmux recreate for a dead session, got %v", ft.created)
	}
}
