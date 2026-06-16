package refresher

import (
	"testing"
	"time"

	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
)

type fakeGit struct{ st git.Status }

func (f fakeGit) DefaultBranch(string) (string, error)     { return "main", nil }
func (f fakeGit) AddWorktree(_, _, _, _ string) error      { return nil }
func (f fakeGit) RemoveWorktree(_, _ string, _ bool) error { return nil }
func (f fakeGit) DeleteBranch(_, _ string, _ bool) error   { return nil }
func (f fakeGit) Status(string) (git.Status, error)        { return f.st, nil }
func (f fakeGit) Push(string, string) error                { return nil }
func (f fakeGit) IsRepo(string) bool                       { return true }

type fakeTmux struct{ alive map[string]bool }

func (f fakeTmux) Has(name string) bool { return f.alive[name] }

func TestBuildDerivesSessionsFromDisk(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{ScanRoot: "/code", WorktreeBaseDir: base}

	// One worktree on disk with meta, tmux alive.
	wtAlive := naming.WorktreePath(base, "My App", "alive")
	_ = meta.Write(wtAlive, meta.Meta{
		Project: "My App", Session: "alive", Branch: "fleet/alive", Base: "main",
		RepoPath: "/code/my-app", CreatedAt: time.Unix(1, 0).UTC(),
	})
	// One worktree on disk with meta, tmux dead -> Exited.
	wtDead := naming.WorktreePath(base, "My App", "dead")
	_ = meta.Write(wtDead, meta.Meta{
		Project: "My App", Session: "dead", Branch: "fleet/dead", Base: "main",
		RepoPath: "/code/my-app", CreatedAt: time.Unix(2, 0).UTC(),
	})

	ft := fakeTmux{alive: map[string]bool{naming.TmuxName("My App", "alive"): true}}
	fg := fakeGit{st: git.Status{Branch: "fleet/alive", Dirty: true, ChangeCount: 3}}

	got, err := Build(cfg, ft, fg)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d: %+v", len(got), got)
	}

	byName := map[string]bool{}
	for _, s := range got {
		byName[s.Name] = s.Alive
		if s.Name == "alive" && (!s.Alive || s.Exited) {
			t.Fatalf("alive session wrong flags: %+v", s)
		}
		if s.Name == "dead" && (s.Alive || !s.Exited) {
			t.Fatalf("dead session wrong flags: %+v", s)
		}
	}
}
