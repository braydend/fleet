//go:build smoke

package refresher

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/tmux"
)

func sh(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
}

// TestSmokeRealAdapters exercises refresher.Build against a real git worktree
// and a real (live) tmux session, validating the integration the unit tests
// fake. Run with: go test -tags smoke -run Smoke ./internal/refresher/ -v
func TestSmokeRealAdapters(t *testing.T) {
	for _, bin := range []string{"git", "tmux"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	root := t.TempDir()
	repo := filepath.Join(root, "myrepo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	sh(t, repo, "git", "init", "-q", "-b", "main")
	sh(t, repo, "git", "config", "user.email", "t@t.test")
	sh(t, repo, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh(t, repo, "git", "add", ".")
	sh(t, repo, "git", "commit", "-q", "-m", "init")

	base := filepath.Join(root, "worktrees")
	cfg := config.Config{ScanRoot: root, WorktreeBaseDir: base}

	g := git.New()
	tm := tmux.New()

	// Simulate what Manager.Create does, but launch a benign command instead of
	// `claude` so the smoke test stays non-interactive.
	wt := naming.WorktreePath(base, "myrepo", "smoke")
	if err := g.AddWorktree(repo, wt, "fleet/smoke", "main"); err != nil {
		t.Fatalf("add worktree: %v", err)
	}
	if err := g.Ignore(wt, ".fleet/"); err != nil {
		t.Fatalf("ignore: %v", err)
	}
	if err := meta.Write(wt, meta.Meta{
		Project: "myrepo", Session: "smoke", Branch: "fleet/smoke", Base: "main",
		RepoPath: repo, CreatedAt: time.Unix(1, 0).UTC(),
	}); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	_ = tm.KillWorkspace()
	if _, err := tm.CreateWindow(naming.TmuxName("myrepo", "smoke"), wt, "sleep 60"); err != nil {
		t.Fatalf("tmux create window: %v", err)
	}
	t.Cleanup(func() { _ = tm.KillWorkspace() })

	// Dirty the worktree so we can verify git status flows through.
	if err := os.WriteFile(filepath.Join(wt, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	sessions, err := Build(cfg, tm, g, time.Now)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d: %+v", len(sessions), sessions)
	}
	s := sessions[0]
	if s.Project != "myrepo" || s.Name != "smoke" || !s.Alive || s.Exited {
		t.Fatalf("unexpected session flags: %+v", s)
	}
	if s.Git.Branch != "fleet/smoke" || !s.Git.Dirty || s.Git.ChangeCount != 1 {
		t.Fatalf("unexpected git status: %+v", s.Git)
	}

	// Kill tmux -> next Build should show it exited.
	if err := tm.KillWindow(naming.WindowTarget("myrepo", "smoke")); err != nil {
		t.Fatalf("kill window: %v", err)
	}
	sessions, err = Build(cfg, tm, g, time.Now)
	if err != nil {
		t.Fatalf("build after kill: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Alive || !sessions[0].Exited {
		t.Fatalf("expected exited session after kill, got %+v", sessions)
	}

	// Remove worktree -> it disappears from the list.
	if err := g.RemoveWorktree(repo, wt, true); err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
	sessions, err = Build(cfg, tm, g, time.Now)
	if err != nil {
		t.Fatalf("build after remove: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions after worktree removal, got %+v", sessions)
	}
}
