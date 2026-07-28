package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/cleanup"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/tmux"
)

// --- fakes ---

type fakeGit struct {
	added   []string // worktree paths added
	pruned  []string // repo paths pruned
	deleted []string
	status  git.Status
}

func (f *fakeGit) DefaultBranch(string) (string, error) { return "main", nil }
func (f *fakeGit) AddWorktree(_, wt, _, _ string) error { f.added = append(f.added, wt); return nil }
func (f *fakeGit) PruneWorktrees(repo string) error {
	f.pruned = append(f.pruned, repo)
	return nil
}
func (f *fakeGit) IsWorktree(string) bool { return true }
func (f *fakeGit) DeleteBranch(_, b string, _ bool) error {
	f.deleted = append(f.deleted, b)
	return nil
}
func (f *fakeGit) Status(string) (git.Status, error) { return f.status, nil }
func (f *fakeGit) Push(string, string) error         { return nil }
func (f *fakeGit) IsRepo(string) bool                { return true }
func (f *fakeGit) Ignore(string, string) error       { return nil }

type fakeTmux struct {
	created   []string // window names created
	killed    []string // targets killed
	respawned []string // targets respawned
	windows   map[string]tmux.Window
}

func (f *fakeTmux) CreateWindow(name, _, _ string) (int, error) {
	f.created = append(f.created, name)
	if f.windows == nil {
		f.windows = map[string]tmux.Window{}
	}
	idx := len(f.windows) + 1
	f.windows[name] = tmux.Window{Index: idx, Name: name}
	return idx, nil
}
func (f *fakeTmux) KillWindow(target string) error {
	f.killed = append(f.killed, target)
	return nil
}
func (f *fakeTmux) RespawnWindow(target, _, _ string) error {
	f.respawned = append(f.respawned, target)
	return nil
}
func (f *fakeTmux) LookupWindow(name string) (tmux.Window, bool) {
	w, ok := f.windows[name]
	return w, ok
}

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
		t.Fatalf("unexpected window create: %v", ft.created)
	}
	md, err := meta.Read(s.WorktreePath)
	if err != nil {
		t.Fatalf("meta read: %v", err)
	}
	if md.Branch != "fleet/fix-bug" || md.Base != "main" || md.RepoPath != "/code/my-app" {
		t.Fatalf("unexpected meta: %+v", md)
	}
	if s.TmuxName != "fleet-My_App-fix_bug" || !s.Alive || s.WindowIndex != 1 {
		t.Fatalf("unexpected session: %+v", s)
	}
	_ = cfg
}

func TestLeaveKillsWindowOnly(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", RepoPath: "/r", Branch: "fleet/s"}
	if err := m.Leave(s); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if len(ft.killed) != 1 || ft.killed[0] != "fleet-workspace:fleet-p-s" || len(fg.pruned) != 0 {
		t.Fatalf("leave should kill window only: killed=%v pruned=%v", ft.killed, fg.pruned)
	}
}

func TestDeleteKillsRemovesAndOptionallyDropsBranch(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, cfg := newManager(t, fg, ft)
	wt := filepath.Join(cfg.WorktreeBaseDir, "p", "s")
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: wt, RepoPath: "/r", Branch: "fleet/s"}

	if _, err := m.Delete(s, false); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(ft.killed) != 1 || ft.killed[0] != "fleet-workspace:fleet-p-s" {
		t.Fatalf("delete should kill the window target: killed=%v", ft.killed)
	}
	if len(fg.pruned) != 1 || len(fg.deleted) != 0 {
		t.Fatalf("expected prune only: pruned=%v deleted=%v", fg.pruned, fg.deleted)
	}
	if _, err := m.Delete(s, true); err != nil {
		t.Fatalf("delete+branch: %v", err)
	}
	if len(fg.deleted) != 1 || fg.deleted[0] != "fleet/s" {
		t.Fatalf("expected branch delete, got %v", fg.deleted)
	}
}

func TestDeletePartialRemovalStillForgetsSession(t *testing.T) {
	g := &fakeGit{}
	m, cfg := newManager(t, g, &fakeTmux{})

	// A worktree whose files cannot all be removed, as when a container has
	// written root-owned output into it.
	wt := filepath.Join(cfg.WorktreeBaseDir, "proj", "sess")
	if err := os.MkdirAll(filepath.Join(wt, ".fleet"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".fleet", "meta.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.removeTree = func(string) (cleanup.Removal, error) {
		return cleanup.Removal{BlockedCount: 42, Blocked: []string{filepath.Join(wt, "vendor")}}, nil
	}

	s := Session{Project: "proj", Name: "sess", Branch: "b", RepoPath: "/repo", WorktreePath: wt}
	res, err := m.Delete(s, true)
	if err != nil {
		t.Fatalf("delete must not fail on blocked files: %v", err)
	}
	if res.Leftover != wt || res.LeftoverCount != 42 {
		t.Errorf("expected leftovers reported, got %+v", res)
	}
	// The session must be forgotten regardless: meta gone, registry pruned,
	// branch deleted.
	if _, err := os.Stat(filepath.Join(wt, ".fleet", "meta.json")); !os.IsNotExist(err) {
		t.Error("expected meta.json to be removed so the session leaves the dashboard")
	}
	if len(g.pruned) != 1 || g.pruned[0] != "/repo" {
		t.Errorf("expected the repo to be pruned, got %v", g.pruned)
	}
	if len(g.deleted) != 1 || g.deleted[0] != "b" {
		t.Errorf("expected the branch to be deleted, got %v", g.deleted)
	}
}

func TestDeleteCleanRemovalReportsNoLeftover(t *testing.T) {
	g := &fakeGit{}
	m, cfg := newManager(t, g, &fakeTmux{})
	wt := filepath.Join(cfg.WorktreeBaseDir, "proj", "sess")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := m.Delete(Session{Project: "proj", Name: "sess", RepoPath: "/repo", WorktreePath: wt}, false)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Leftover != "" {
		t.Errorf("expected no leftovers, got %+v", res)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Error("expected the worktree directory to be gone")
	}
}

func TestDeleteIsIdempotentOnAlreadyGoneWorktree(t *testing.T) {
	g := &fakeGit{}
	m, cfg := newManager(t, g, &fakeTmux{})
	wt := filepath.Join(cfg.WorktreeBaseDir, "proj", "gone")

	if _, err := m.Delete(Session{Project: "proj", Name: "gone", RepoPath: "/repo", WorktreePath: wt}, false); err != nil {
		t.Fatalf("deleting an already-removed worktree must succeed: %v", err)
	}
}

func TestDeleteRefusesPathOutsideWorktreeBase(t *testing.T) {
	g := &fakeGit{}
	m, _ := newManager(t, g, &fakeTmux{})
	outside := t.TempDir()

	if _, err := m.Delete(Session{Project: "p", Name: "s", RepoPath: "/repo", WorktreePath: outside}, false); err == nil {
		t.Fatal("expected a refusal for a path outside the worktree base dir")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("the directory must be untouched: %v", err)
	}
}

func TestEnsureRunningNoopWhenAlive(t *testing.T) {
	ft := &fakeTmux{windows: map[string]tmux.Window{"fleet-p-s": {Index: 1, Name: "fleet-p-s"}}}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.created) != 0 || len(ft.respawned) != 0 {
		t.Fatalf("expected no create/respawn for a live session: created=%v respawned=%v", ft.created, ft.respawned)
	}
}

func TestEnsureRunningCreatesWhenMissing(t *testing.T) {
	ft := &fakeTmux{}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.created) != 1 || ft.created[0] != "fleet-p-s" {
		t.Fatalf("expected window create for a missing session, got %v", ft.created)
	}
}

func TestEnsureRunningRespawnsWhenDead(t *testing.T) {
	ft := &fakeTmux{windows: map[string]tmux.Window{"fleet-p-s": {Index: 1, Name: "fleet-p-s", Dead: true}}}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.respawned) != 1 || ft.respawned[0] != "fleet-workspace:fleet-p-s" {
		t.Fatalf("expected respawn for a dead window, got %v", ft.respawned)
	}
}

func TestSessionHasActivityFields(t *testing.T) {
	s := Session{
		Activity:     activity.Working,
		LastActivity: time.Unix(5, 0),
		WindowIndex:  2,
	}
	if s.Activity != activity.Working || s.WindowIndex != 2 {
		t.Fatalf("unexpected session fields: %+v", s)
	}
}
