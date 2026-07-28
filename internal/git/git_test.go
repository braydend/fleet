package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
}

// newRepo creates a temp git repo with one commit on branch "main".
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "main")
	run(t, dir, "git", "config", "user.email", "t@t.test")
	run(t, dir, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-q", "-m", "init")
	return dir
}

func TestDefaultBranch(t *testing.T) {
	repo := newRepo(t)
	g := New()
	branch, err := g.DefaultBranch(repo)
	if err != nil {
		t.Fatalf("default branch: %v", err)
	}
	if branch != "main" {
		t.Fatalf("got %q", branch)
	}
}

func TestAddWorktreeThenStatus(t *testing.T) {
	repo := newRepo(t)
	g := New()
	wt := filepath.Join(t.TempDir(), "wt")
	if err := g.AddWorktree(repo, wt, "fleet/feature", "main"); err != nil {
		t.Fatalf("add worktree: %v", err)
	}
	st, err := g.Status(wt)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Branch != "fleet/feature" {
		t.Fatalf("got branch %q", st.Branch)
	}
	if st.Dirty {
		t.Fatal("expected clean worktree")
	}
	// Make it dirty and re-check.
	if err := os.WriteFile(filepath.Join(wt, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ = g.Status(wt)
	if !st.Dirty || st.ChangeCount != 1 {
		t.Fatalf("expected dirty with 1 change, got %+v", st)
	}
}

func TestIgnoreKeepsFleetMetaOutOfStatus(t *testing.T) {
	repo := newRepo(t)
	g := New()
	wt := filepath.Join(t.TempDir(), "wt")
	if err := g.AddWorktree(repo, wt, "fleet/ig", "main"); err != nil {
		t.Fatal(err)
	}
	if err := g.Ignore(wt, ".fleet/"); err != nil {
		t.Fatalf("ignore: %v", err)
	}
	// Write fleet's bookkeeping file; it must not register as a change.
	if err := os.MkdirAll(filepath.Join(wt, ".fleet"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".fleet", "meta.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := g.Status(wt)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Dirty || st.ChangeCount != 0 {
		t.Fatalf("expected clean worktree after ignoring .fleet, got %+v", st)
	}
	// A real user file still counts.
	if err := os.WriteFile(filepath.Join(wt, "real.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ = g.Status(wt)
	if !st.Dirty || st.ChangeCount != 1 {
		t.Fatalf("expected 1 change for the user file, got %+v", st)
	}
}

func TestPruneWorktreesReconcilesRegistry(t *testing.T) {
	repo := newRepo(t)
	g := New()
	wt := filepath.Join(t.TempDir(), "wt")
	if err := g.AddWorktree(repo, wt, "fleet/x", "main"); err != nil {
		t.Fatal(err)
	}
	// Simulate fleet having removed the tree itself.
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}

	if err := g.PruneWorktrees(repo); err != nil {
		t.Fatalf("prune: %v", err)
	}
	out, err := g.git(repo, "worktree", "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, wt) {
		t.Fatalf("expected %s to be pruned from the registry:\n%s", wt, out)
	}
	// Idempotent: a second prune is a no-op, not an error.
	if err := g.PruneWorktrees(repo); err != nil {
		t.Fatalf("second prune: %v", err)
	}
}

func TestIsWorktree(t *testing.T) {
	repo := newRepo(t)
	g := New()
	wt := filepath.Join(t.TempDir(), "wt")
	if err := g.AddWorktree(repo, wt, "fleet/x", "main"); err != nil {
		t.Fatal(err)
	}
	if !g.IsWorktree(wt) {
		t.Error("expected a freshly added worktree to be recognised")
	}
	// The state left behind by a partially failed removal.
	if err := os.Remove(filepath.Join(wt, ".git")); err != nil {
		t.Fatal(err)
	}
	if g.IsWorktree(wt) {
		t.Error("expected a worktree without .git to be unrecognised")
	}
	if g.IsWorktree(t.TempDir()) {
		t.Error("expected a plain directory to be unrecognised")
	}
}
