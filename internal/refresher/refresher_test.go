package refresher

import (
	"strings"
	"testing"
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/session"
	"github.com/bray/fleet/internal/tmux"
)

type fakeGit struct{ st git.Status }

func (f fakeGit) DefaultBranch(string) (string, error)     { return "main", nil }
func (f fakeGit) AddWorktree(_, _, _, _ string) error      { return nil }
func (f fakeGit) RemoveWorktree(_, _ string, _ bool) error { return nil }
func (f fakeGit) DeleteBranch(_, _ string, _ bool) error   { return nil }
func (f fakeGit) Status(string) (git.Status, error)        { return f.st, nil }
func (f fakeGit) Push(string, string) error                { return nil }
func (f fakeGit) IsRepo(string) bool                       { return true }
func (f fakeGit) Ignore(string, string) error              { return nil }
func (f fakeGit) LocalBranchExists(string, string) (bool, error)  { return false, nil }
func (f fakeGit) RemoteBranchExists(string, string) (bool, error) { return false, nil }
func (f fakeGit) ListBranches(string) (git.Branches, error)       { return git.Branches{}, nil }
func (f fakeGit) Fetch(string) error                              { return nil }

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
