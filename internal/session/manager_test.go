package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/agent"
	"github.com/bray/fleet/internal/cleanup"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/tmux"
)

// --- fakes ---

type fakeGit struct {
	added         []string // worktree paths added
	pruned        []string // repo paths pruned
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
func (f *fakeGit) PruneWorktrees(repo string) error {
	f.pruned = append(f.pruned, repo)
	return nil
}
func (f *fakeGit) IsWorktree(string) bool { return true }
func (f *fakeGit) DeleteBranch(_, b string, _ bool) error {
	f.deleted = append(f.deleted, b)
	return nil
}
func (f *fakeGit) Status(string) (git.Status, error)            { return f.status, nil }
func (f *fakeGit) Push(string, string) error                    { return nil }
func (f *fakeGit) IsRepo(string) bool                           { return true }
func (f *fakeGit) Ignore(string, string) error                  { return nil }
func (f *fakeGit) LocalBranchExists(_, b string) (bool, error)  { return f.localExists[b], nil }
func (f *fakeGit) RemoteBranchExists(_, b string) (bool, error) { return f.remoteExists[b], nil }
func (f *fakeGit) ListBranches(string) (git.Branches, error)    { return git.Branches{}, nil }
func (f *fakeGit) Fetch(string) error                           { return nil }
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
	created       []string // window names created
	createdCmds   []string // launch commands passed to CreateWindow
	killed        []string // targets killed
	respawned     []string // targets respawned
	respawnedCmds []string // launch commands passed to RespawnWindow
	windows       map[string]tmux.Window
}

func (f *fakeTmux) CreateWindow(name, _, cmd string) (int, error) {
	f.created = append(f.created, name)
	f.createdCmds = append(f.createdCmds, cmd)
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
func (f *fakeTmux) RespawnWindow(target, _, cmd string) error {
	f.respawned = append(f.respawned, target)
	f.respawnedCmds = append(f.respawnedCmds, cmd)
	return nil
}
func (f *fakeTmux) LookupWindow(name string) (tmux.Window, bool) {
	w, ok := f.windows[name]
	return w, ok
}

func newManager(t *testing.T, g git.Git, tm tmuxPort) (*Manager, config.Config) {
	cfg := config.Config{ScanRoot: "/code", WorktreeBaseDir: t.TempDir()}
	fixed := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	m := NewManager(cfg, tm, g, nil,
		func() time.Time { return fixed },
		func() string { return "test-session-id" },
	)
	return m, cfg
}

func TestCreateAddsWorktreeMetaAndTmux(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, cfg := newManager(t, fg, ft)

	proj := projects.Project{Name: "My App", Path: "/code/my-app", DefaultBranch: "main"}
	s, err := m.Create(proj, "fix-bug", "fleet/fix-bug", "main", agent.Lookup(agent.IDClaude))
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
	if md.ClaudeSessionID != "test-session-id" {
		t.Fatalf("expected stored session ID, got %q", md.ClaudeSessionID)
	}
	if s.ClaudeSessionID != "test-session-id" {
		t.Fatalf("expected session ID on returned session, got %q", s.ClaudeSessionID)
	}
	wantCmd := "claude --session-id test-session-id -n 'My App/fix-bug'"
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != wantCmd {
		t.Fatalf("create launch cmd = %v, want %q", ft.createdCmds, wantCmd)
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

func TestEnsureRunningResumesWhenMissing(t *testing.T) {
	ft := &fakeTmux{}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", ClaudeSessionID: "sid-1"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	want := "claude --resume sid-1 || claude --session-id sid-1 -n 'p/s' || claude"
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != want {
		t.Fatalf("missing-window cmd = %v, want %q", ft.createdCmds, want)
	}
}

func TestEnsureRunningRespawnUsesResumeChain(t *testing.T) {
	ft := &fakeTmux{windows: map[string]tmux.Window{"fleet-p-s": {Index: 1, Name: "fleet-p-s", Dead: true}}}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", ClaudeSessionID: "sid-1"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	want := "claude --resume sid-1 || claude --session-id sid-1 -n 'p/s' || claude"
	if len(ft.respawnedCmds) != 1 || ft.respawnedCmds[0] != want {
		t.Fatalf("respawn cmd = %v, want %q", ft.respawnedCmds, want)
	}
}

func TestEnsureRunningLegacyLaunchesBareClaude(t *testing.T) {
	ft := &fakeTmux{}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt"} // no ClaudeSessionID
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != "claude" {
		t.Fatalf("legacy cmd = %v, want [claude]", ft.createdCmds)
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
	if _, err := m.Create(proj, "sess", "feature", "main", agent.Lookup(agent.IDClaude)); err != nil {
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
	if _, err := m.Create(proj, "sess", "feature", "main", agent.Lookup(agent.IDClaude)); err != nil {
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
	if _, err := m.Create(proj, "sess", "brand-new", "main", agent.Lookup(agent.IDClaude)); err != nil {
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
	_, err := m.Create(proj, "sess", "feature", "main", agent.Lookup(agent.IDClaude))
	if err == nil || !strings.Contains(err.Error(), "already checked out in another worktree") {
		t.Fatalf("expected friendly checked-out error, got %v", err)
	}
}

func TestCreateWithClaudeStoresAgentID(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}

	s, err := m.Create(proj, "sess", "b", "main", agent.Lookup(agent.IDClaude))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	md, err := meta.Read(s.WorktreePath)
	if err != nil {
		t.Fatalf("meta read: %v", err)
	}
	if md.Agent != agent.IDClaude || s.Agent != agent.IDClaude {
		t.Fatalf("agent = meta %q session %q, want %q", md.Agent, s.Agent, agent.IDClaude)
	}
	if md.ClaudeSessionID != "test-session-id" {
		t.Fatalf("claude sessions must store a minted ID, got %q", md.ClaudeSessionID)
	}
}

// opencode mints its own session IDs, so fleet stores none and launches it bare.
func TestCreateWithOpencodeStoresNoSessionID(t *testing.T) {
	fg := &fakeGit{}
	ft := &fakeTmux{}
	m, _ := newManager(t, fg, ft)
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}

	s, err := m.Create(proj, "sess", "b", "main", agent.Lookup(agent.IDOpencode))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	md, err := meta.Read(s.WorktreePath)
	if err != nil {
		t.Fatalf("meta read: %v", err)
	}
	if md.Agent != agent.IDOpencode || s.Agent != agent.IDOpencode {
		t.Fatalf("agent = meta %q session %q, want %q", md.Agent, s.Agent, agent.IDOpencode)
	}
	if md.ClaudeSessionID != "" || s.ClaudeSessionID != "" {
		t.Fatalf("opencode must store no session ID, got meta %q session %q",
			md.ClaudeSessionID, s.ClaudeSessionID)
	}
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != "opencode" {
		t.Fatalf("create launch cmd = %v, want [opencode]", ft.createdCmds)
	}
}

func TestEnsureRunningUsesOpencodeResumeChain(t *testing.T) {
	ft := &fakeTmux{windows: map[string]tmux.Window{"fleet-p-s": {Index: 1, Name: "fleet-p-s", Dead: true}}}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt", Agent: agent.IDOpencode}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	want := "opencode --continue || opencode"
	if len(ft.respawnedCmds) != 1 || ft.respawnedCmds[0] != want {
		t.Fatalf("respawn cmd = %v, want %q", ft.respawnedCmds, want)
	}
}

// A meta naming an agent this build does not know must still produce a usable
// window rather than an empty command.
func TestEnsureRunningUnknownAgentFallsBackToClaude(t *testing.T) {
	ft := &fakeTmux{}
	m, _ := newManager(t, &fakeGit{}, ft)
	s := Session{Project: "p", Name: "s", TmuxName: "fleet-p-s", WorktreePath: "/wt",
		Agent: "from-the-future", ClaudeSessionID: "sid-1"}
	if err := m.EnsureRunning(s); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	want := "claude --resume sid-1 || claude --session-id sid-1 -n 'p/s' || claude"
	if len(ft.createdCmds) != 1 || ft.createdCmds[0] != want {
		t.Fatalf("cmd = %v, want %q", ft.createdCmds, want)
	}
}
