package refresher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/agent"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/session"
	"github.com/bray/fleet/internal/tmux"
)

type fakeGit struct {
	st          git.Status
	notWorktree bool // when true, IsWorktree reports false for every path
}

func (f fakeGit) DefaultBranch(string) (string, error)            { return "main", nil }
func (f fakeGit) AddWorktree(_, _, _, _ string) error             { return nil }
func (f fakeGit) PruneWorktrees(string) error                     { return nil }
func (f fakeGit) IsWorktree(string) bool                          { return !f.notWorktree }
func (f fakeGit) DeleteBranch(_, _ string, _ bool) error          { return nil }
func (f fakeGit) Status(string) (git.Status, error)               { return f.st, nil }
func (f fakeGit) Push(string, string) error                       { return nil }
func (f fakeGit) IsRepo(string) bool                              { return true }
func (f fakeGit) Ignore(string, string) error                     { return nil }
func (f fakeGit) LocalBranchExists(string, string) (bool, error)  { return false, nil }
func (f fakeGit) RemoteBranchExists(string, string) (bool, error) { return false, nil }
func (f fakeGit) ListBranches(string) (git.Branches, error)       { return git.Branches{}, nil }
func (f fakeGit) Fetch(string) error                              { return nil }
func (f fakeGit) AddWorktreeExisting(_, _, _ string) error        { return nil }
func (f fakeGit) AddWorktreeTracking(_, _, _ string) error        { return nil }

type fakeTmux struct {
	windows []tmux.Window
	tails   map[string]string
	labels  map[string]string // target -> last label set
}

func (f *fakeTmux) ListWindows() ([]tmux.Window, error) { return f.windows, nil }
func (f *fakeTmux) CapturePane(target string) (string, error) {
	return f.tails[target], nil
}
func (f *fakeTmux) SetWindowLabel(target, label string) error {
	if f.labels == nil {
		f.labels = map[string]string{}
	}
	f.labels[target] = label
	return nil
}

// Compile-time assertions that the fakes satisfy the ports Build depends on,
// so drift surfaces here rather than at the Build call site.
var (
	_ workspaceTmux = (*fakeTmux)(nil)
	_ git.Git       = fakeGit{}
)

func TestTabLabelEscapesHash(t *testing.T) {
	s := session.Session{Project: "c#", Name: "feat#1", Activity: activity.Idle}
	got := tabLabel(s)
	// The literal '#' from user names must be doubled; the intentional "#["
	// style directives must remain single.
	if !strings.Contains(got, "c##/feat##1") {
		t.Fatalf("expected escaped names in label, got %q", got)
	}
	if !strings.Contains(got, "#[fg=") || !strings.Contains(got, "#[default]") {
		t.Fatalf("style directives should stay literal, got %q", got)
	}
}

func TestBuildDerivesSessionsAndActivity(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{ScanRoot: "/code", WorktreeBaseDir: base}
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	// Two worktrees: one live & working, one with no window (exited).
	wtA := naming.WorktreePath(base, "My App", "alive")
	_ = meta.Write(wtA, meta.Meta{
		Project: "My App", Session: "alive", Branch: "fleet/alive", Base: "main",
		RepoPath: "/code/my-app", CreatedAt: time.Unix(1, 0).UTC(),
		ClaudeSessionID: "alive-session-id",
	})
	wtD := naming.WorktreePath(base, "My App", "dead")
	_ = meta.Write(wtD, meta.Meta{
		Project: "My App", Session: "dead", Branch: "fleet/dead", Base: "main",
		RepoPath: "/code/my-app", CreatedAt: time.Unix(2, 0).UTC(),
	})

	nameAlive := naming.TmuxName("My App", "alive")
	ft := &fakeTmux{
		windows: []tmux.Window{
			{Index: 1, Name: nameAlive, Dead: false, LastActivity: now.Add(-1 * time.Second)},
		},
	}
	fg := fakeGit{st: git.Status{Branch: "fleet/alive", Dirty: true, ChangeCount: 3}}

	got, err := Build(cfg, ft, fg, func() time.Time { return now })
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d: %+v", len(got), got)
	}

	for _, s := range got {
		switch s.Name {
		case "alive":
			if !s.Alive || s.Exited || s.Activity != activity.Working || s.WindowIndex != 1 {
				t.Fatalf("alive session wrong: %+v", s)
			}
			if s.ClaudeSessionID != "alive-session-id" {
				t.Fatalf("expected ClaudeSessionID carried from meta, got %q", s.ClaudeSessionID)
			}
		case "dead":
			if s.Alive || !s.Exited || s.Activity != activity.Exited {
				t.Fatalf("dead session wrong: %+v", s)
			}
		}
	}

	// The live session should have had its tab label pushed.
	target := naming.WindowTarget("My App", "alive")
	if ft.labels[target] == "" {
		t.Fatalf("expected a label set for %q, got %v", target, ft.labels)
	}
}

// A session's agent comes from its meta and reaches the dashboard, and an
// agent with no known prompt markers is never reported as waiting even when its
// pane contains another agent's prompt text.
func TestBuildCarriesAgentAndSkipsWaitingWithoutMarkers(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{ScanRoot: "/code", WorktreeBaseDir: base}
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	wt := filepath.Join(base, "proj", "sess")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := meta.Write(wt, meta.Meta{
		Project: "proj", Session: "sess", Branch: "b", Base: "main",
		Agent: agent.IDOpencode,
	}); err != nil {
		t.Fatal(err)
	}

	wname := naming.TmuxName("proj", "sess")
	target := naming.WindowTarget("proj", "sess")
	ft := &fakeTmux{
		windows: []tmux.Window{{Index: 1, Name: wname, LastActivity: now.Add(-30 * time.Second)}},
		tails:   map[string]string{target: "Do you want to proceed?\n❯ 1. Yes"},
	}

	got, err := Build(cfg, ft, fakeGit{}, func() time.Time { return now })
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d", len(got))
	}
	if got[0].Agent != agent.IDOpencode {
		t.Fatalf("Agent = %q, want %q", got[0].Agent, agent.IDOpencode)
	}
	if got[0].Activity != activity.Idle {
		t.Fatalf("Activity = %v, want Idle — opencode has no prompt markers", got[0].Activity)
	}
}

func TestBuildMarksUnregisteredWorktreeBroken(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{ScanRoot: "/code", WorktreeBaseDir: base}
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	wt := naming.WorktreePath(base, "My App", "orphan")
	_ = meta.Write(wt, meta.Meta{
		Project: "My App", Session: "orphan", Branch: "fleet/orphan", Base: "main",
		RepoPath: "/code/my-app", CreatedAt: time.Unix(1, 0).UTC(),
	})
	ft := &fakeTmux{}

	healthy, err := Build(cfg, ft, fakeGit{st: git.Status{Branch: "fleet/orphan"}}, clock)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(healthy) != 1 || healthy[0].Broken {
		t.Fatalf("expected a healthy session, got %+v", healthy)
	}

	broken, err := Build(cfg, ft, fakeGit{notWorktree: true}, clock)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(broken) != 1 || !broken[0].Broken {
		t.Fatalf("expected the session to be marked broken, got %+v", broken)
	}
	// Branch still comes from meta so the row remains identifiable.
	if broken[0].Branch != "fleet/orphan" {
		t.Errorf("expected the meta branch to survive on a broken session, got %q", broken[0].Branch)
	}
}
