package git

import (
	"os"
	"os/exec"
	"path/filepath"
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

func TestRemoveWorktree(t *testing.T) {
	repo := newRepo(t)
	g := New()
	wt := filepath.Join(t.TempDir(), "wt")
	if err := g.AddWorktree(repo, wt, "fleet/x", "main"); err != nil {
		t.Fatal(err)
	}
	if err := g.RemoveWorktree(repo, wt, true); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("expected worktree dir to be gone")
	}
}

// contains reports whether s is in xs.
func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// repoWithRemoteFeature returns a repo whose origin has a "feature" branch,
// tracked locally (refs/remotes/origin/feature) with no local branch.
func repoWithRemoteFeature(t *testing.T) string {
	t.Helper()
	repo := newRepo(t)
	origin := t.TempDir()
	run(t, origin, "git", "init", "-q", "--bare", "-b", "main")
	run(t, repo, "git", "remote", "add", "origin", origin)
	run(t, repo, "git", "push", "-q", "origin", "main")
	run(t, repo, "git", "branch", "feature", "main")
	run(t, repo, "git", "push", "-q", "origin", "feature")
	run(t, repo, "git", "branch", "-D", "feature")
	run(t, repo, "git", "fetch", "-q", "origin")
	return repo
}

func TestLocalBranchExists(t *testing.T) {
	repo := newRepo(t)
	g := New()
	ok, err := g.LocalBranchExists(repo, "main")
	if err != nil || !ok {
		t.Fatalf("expected main to exist: ok=%v err=%v", ok, err)
	}
	ok, err = g.LocalBranchExists(repo, "nope")
	if err != nil || ok {
		t.Fatalf("expected nope to be absent: ok=%v err=%v", ok, err)
	}
}

func TestRemoteBranchExists(t *testing.T) {
	repo := repoWithRemoteFeature(t)
	g := New()
	ok, err := g.RemoteBranchExists(repo, "feature")
	if err != nil || !ok {
		t.Fatalf("expected origin/feature to exist: ok=%v err=%v", ok, err)
	}
	ok, _ = g.RemoteBranchExists(repo, "missing")
	if ok {
		t.Fatal("expected origin/missing to be absent")
	}
	// feature must NOT be a local branch.
	ok, _ = g.LocalBranchExists(repo, "feature")
	if ok {
		t.Fatal("feature should be remote-only, not local")
	}
}

func TestListBranches(t *testing.T) {
	repo := repoWithRemoteFeature(t)
	g := New()
	br, err := g.ListBranches(repo)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !contains(br.Local, "main") {
		t.Fatalf("local missing main: %v", br.Local)
	}
	if contains(br.Local, "feature") {
		t.Fatalf("feature should not be local: %v", br.Local)
	}
	if !contains(br.Remote, "feature") {
		t.Fatalf("remote missing feature: %v", br.Remote)
	}
	if contains(br.Remote, "HEAD") {
		t.Fatalf("HEAD should be excluded from remote: %v", br.Remote)
	}
}

func TestFetchPopulatesRemoteRefs(t *testing.T) {
	repo := repoWithRemoteFeature(t)
	g := New()
	// Drop the tracking ref so only Fetch can repopulate it.
	run(t, repo, "git", "update-ref", "-d", "refs/remotes/origin/feature")
	if ok, _ := g.RemoteBranchExists(repo, "feature"); ok {
		t.Fatal("precondition: feature should not be tracked yet")
	}
	if err := g.Fetch(repo); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if ok, _ := g.RemoteBranchExists(repo, "feature"); !ok {
		t.Fatal("expected feature tracked after fetch")
	}
}
