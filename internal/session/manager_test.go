package session

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/tmux"
)

// --- fakes ---

type fakeGit struct {
	added         []string // worktree paths added
	removed       []string
	deleted       []string
	status        git.Status
	localExists   map[string]bool
	remoteExists  map[string]bool
	addedExisting []string
	addedTracking []string
	existingErr   error // returned by AddWorktreeExisting when set
}

func (f *fakeGit) DefaultBranch(string) (string, error) { return "main", nil }
func (f *fakeGit) AddWorktree(_, wt, _, _ string) error { f.added = append(f.added, wt); return nil }
func (f *fakeGit) RemoveWorktree(_, wt string, _ bool) error {
	f.removed = append(f.removed, wt)
	return nil
}
func (f *fakeGit) DeleteBranch(_, b string, _ bool) error {
	f.deleted = append(f.deleted, b)
	return nil
}
func (f *fakeGit) Status(string) (git.Status, error) { return f.status, nil }
func (f *fakeGit) Push(string, string) error         { return nil }
func (f *fakeGit) IsRepo(string) bool                { return true }
func (f *fakeGit) Ignore(string, string) error       { return nil }
func (f *fakeGit) LocalBranchExists(_, b string) (bool, error)  { return f.localExists[b], nil }
func (f *fakeGit) RemoteBranchExists(_, b string) (bool, error) { return f.remoteExists[b], nil }
func (f *fakeGit) ListBranches(string) (git.Branches, error)    { return git.Branches{}, nil }
func (f *fakeGit) Fetch(string) error { return nil }
func (f *fakeGit) AddWorktreeExisting(_, wt, _ string) error {
	if f.existingErr != nil {
		return f.existingErr
	}
	f.addedExisting = append(f.addedExisting, wt)
	return nil
}
func (f *fakeGit) AddWorktreeTracking(_, wt, _ string) error {
	f.addedTracking = append(f.addedTracking, wt)
	return nil
}

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
	if len(ft.killed) != 1 || ft.killed[0] != "fleet-workspace:fleet-p-s" || len(fg.removed) != 0 {
		t.Fatalf("leave should kill window only: killed=%v removed=%v", ft.killed, fg.removed)
	}
}

func TestDeleteKillsRemovesAndOptionallyDropsBranch(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", RepoPath: "/r", Branch: "fleet/s"}

	if err := m.Delete(s, false); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(ft.killed) != 1 || ft.killed[0] != "fleet-workspace:fleet-p-s" {
		t.Fatalf("delete should kill the window target: killed=%v", ft.killed)
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

func TestCreateUsesExistingLocalBranch(t *testing.T) {
	fg := &fakeGit{localExists: map[string]bool{"feature": true}}
	m, _ := newManager(t, fg, &fakeTmux{})
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}
	if _, err := m.Create(proj, "sess", "feature", "main"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(fg.addedExisting) != 1 {
		t.Fatalf("expected existing checkout, got %+v", fg)
	}
	if len(fg.added) != 0 || len(fg.addedTracking) != 0 {
		t.Fatalf("wrong worktree path taken: %+v", fg)
	}
}

func TestCreateTracksRemoteBranch(t *testing.T) {
	fg := &fakeGit{remoteExists: map[string]bool{"feature": true}}
	m, _ := newManager(t, fg, &fakeTmux{})
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}
	if _, err := m.Create(proj, "sess", "feature", "main"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(fg.addedTracking) != 1 {
		t.Fatalf("expected tracking checkout, got %+v", fg)
	}
	if len(fg.added) != 0 || len(fg.addedExisting) != 0 {
		t.Fatalf("wrong worktree path taken: %+v", fg)
	}
}

func TestCreateNewBranchWhenNeitherExists(t *testing.T) {
	fg := &fakeGit{}
	m, _ := newManager(t, fg, &fakeTmux{})
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}
	if _, err := m.Create(proj, "sess", "brand-new", "main"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(fg.added) != 1 {
		t.Fatalf("expected new-branch worktree, got %+v", fg)
	}
	if len(fg.addedExisting) != 0 || len(fg.addedTracking) != 0 {
		t.Fatalf("wrong worktree path taken: %+v", fg)
	}
}

func TestCreateExistingBranchCheckedOutElsewhere(t *testing.T) {
	fg := &fakeGit{
		localExists: map[string]bool{"feature": true},
		existingErr: errors.New("fatal: 'feature' is already checked out at '/x'"),
	}
	m, _ := newManager(t, fg, &fakeTmux{})
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}
	_, err := m.Create(proj, "sess", "feature", "main")
	if err == nil || !strings.Contains(err.Error(), "already checked out in another worktree") {
		t.Fatalf("expected friendly checked-out error, got %v", err)
	}
}
