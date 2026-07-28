# Broken Worktree Cleanup — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-07-28-broken-worktree-cleanup-design.md`](../specs/2026-07-28-broken-worktree-cleanup-design.md)

**Goal:** A session delete can never strand an un-removable session on the dashboard. fleet removes the worktree tree itself (best-effort, continuing past files it cannot delete), reconciles the git registry with `git worktree prune`, always forgets the session, and reports any files left on disk. Worktrees git no longer recognises render as **broken** with a single-option cleanup menu.

**Architecture:** A new `internal/cleanup` package owns a post-order, best-effort filesystem walk with no git knowledge. `internal/git` drops `RemoveWorktree` in favour of `PruneWorktrees` (reconcile) and `IsWorktree` (a `.git` stat). `session.Manager.Delete` orchestrates kill → best-effort remove → force-forget meta → prune → delete branch, returning a `DeleteResult` describing leftovers. `refresher` sets `Session.Broken` from `IsWorktree`; `internal/ui` renders that state and carries the leftover report to the status line.

**Tech Stack:** Go, Bubble Tea/Lip Gloss (existing `theme.go` palette), git CLI.

## Global Constraints

- **Conventional Commits, mandatory.** This is a bug fix with new visible behaviour: use `fix(session): ...` / `feat(ui): ...` per task as indicated.
- **End every commit message** with the trailer: `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.
- **TDD:** failing test → watch it fail → minimal implementation → watch it pass → commit.
- **Never call `git worktree remove` again.** It destroys the registration before the tree, which is the root cause being fixed.
- **fleet never runs `sudo`** and never deletes outside `cfg.WorktreeBaseDir`.
- **Permission-fixture hygiene:** any test that `chmod`s a directory to make it unremovable MUST restore the mode in `t.Cleanup`, or `t.TempDir()` teardown fails the test run.
- **Build/test:** `go build ./...`, `go vet ./...`, `go test ./...`, run from repo root.

---

### Task 1: `internal/cleanup` — best-effort tree removal

**Files:**
- Create: `internal/cleanup/cleanup.go`
- Create: `internal/cleanup/cleanup_test.go`

**Interfaces:**
- Produces: `cleanup.Removal{BlockedCount int, Blocked []string}`, `Removal.Complete() bool`, `cleanup.RemoveTree(root string) (Removal, error)`.
- Consumes: `os`, `path/filepath`.

- [ ] **Step 1: Write the failing tests**

Create `internal/cleanup/cleanup_test.go`:

```go
package cleanup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveTreeRemovesEverything(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"top.txt", "a/mid.txt", "a/b/deep.txt"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rem, err := RemoveTree(root)
	if err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}
	if !rem.Complete() {
		t.Fatalf("expected complete removal, got %+v", rem)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("expected root to be gone")
	}
}

func TestRemoveTreeContinuesPastBlockedSubtree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wt")
	locked := filepath.Join(root, "vendor")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "pinned.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "removable.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// r-xr-xr-x: contents can be read but not unlinked, like a root-owned
	// directory written by a container.
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	rem, err := RemoveTree(root)
	if err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}
	if rem.Complete() || rem.BlockedCount == 0 {
		t.Fatalf("expected blocked entries, got %+v", rem)
	}
	if len(rem.Blocked) == 0 {
		t.Fatal("expected at least one sample blocked path")
	}
	// The deletable sibling is gone even though vendor/ survived.
	if _, err := os.Stat(filepath.Join(root, "removable.txt")); !os.IsNotExist(err) {
		t.Error("expected removable.txt to be deleted despite the blocked subtree")
	}
	if _, err := os.Stat(filepath.Join(locked, "pinned.txt")); err != nil {
		t.Errorf("expected blocked file to survive: %v", err)
	}
}

func TestRemoveTreeMissingRootIsSuccess(t *testing.T) {
	rem, err := RemoveTree(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("expected missing root to succeed, got %v", err)
	}
	if !rem.Complete() {
		t.Fatalf("expected complete, got %+v", rem)
	}
}

func TestRemoveTreeUnlinksSymlinkWithoutFollowing(t *testing.T) {
	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(tmp, "wt")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	rem, err := RemoveTree(root)
	if err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}
	if !rem.Complete() {
		t.Fatalf("expected complete removal, got %+v", rem)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("symlink target must not be touched: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cleanup/ -v`
Expected: FAIL — package/`RemoveTree` undefined (build error).

- [ ] **Step 3: Implement `internal/cleanup/cleanup.go`**

```go
// Package cleanup removes worktree directories on a best-effort basis: unlike
// os.RemoveAll, which stops at the first error, it continues past entries it
// cannot delete and reports them. Container-written, root-owned files inside a
// session worktree would otherwise abandon the rest of the tree.
package cleanup

import (
	"os"
	"path/filepath"
)

// maxBlockedSamples caps how many blocked paths are retained for display. A
// blocked vendor/ directory can hold tens of thousands of entries; the count is
// what matters, plus a few examples.
const maxBlockedSamples = 5

// Removal reports what RemoveTree could not delete.
type Removal struct {
	BlockedCount int      // total entries that could not be removed
	Blocked      []string // up to maxBlockedSamples example paths
}

// Complete reports whether the tree was removed in full.
func (r Removal) Complete() bool { return r.BlockedCount == 0 }

func (r *Removal) block(path string) {
	r.BlockedCount++
	if len(r.Blocked) < maxBlockedSamples {
		r.Blocked = append(r.Blocked, path)
	}
}

// RemoveTree deletes root and everything beneath it, continuing past entries it
// cannot remove. A root that does not exist is success. The returned error is
// reserved for a root that exists but cannot be inspected; ordinary permission
// failures are reported in Removal.
func RemoveTree(root string) (Removal, error) {
	var r Removal
	if _, err := os.Lstat(root); err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return r, err
	}
	removeEntry(root, true, &r)
	return r, nil
}

// removeEntry deletes one path, recursing post-order into real directories so a
// directory is only unlinked once its children are gone. Symlinks are unlinked
// rather than followed, so a link out of the worktree never endangers its
// target. Returns true when the path is gone.
func removeEntry(path string, isDir bool, r *Removal) bool {
	if isDir {
		entries, err := os.ReadDir(path)
		if err != nil {
			r.block(path)
			return false
		}
		emptied := true
		for _, e := range entries {
			// e.IsDir() is Lstat-based, so a symlink to a directory reports
			// false and is unlinked below rather than descended into.
			if !removeEntry(filepath.Join(path, e.Name()), e.IsDir(), r) {
				emptied = false
			}
		}
		if !emptied {
			r.block(path)
			return false
		}
	}
	if err := os.Remove(path); err != nil {
		r.block(path)
		return false
	}
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cleanup/ -v`
Expected: PASS — all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/cleanup/
git commit -m "$(cat <<'EOF'
feat(cleanup): add best-effort worktree tree removal

os.RemoveAll aborts at the first undeletable entry, abandoning the rest of
the tree. RemoveTree continues and reports what it could not delete.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `internal/git` — prune and worktree detection

**Files:**
- Modify: `internal/git/git.go` (drop `RemoveWorktree`, add `PruneWorktrees` + `IsWorktree`)
- Modify: `internal/git/git_test.go` (replace `TestRemoveWorktree`)

**Interfaces:**
- Produces: `Git.PruneWorktrees(repoPath string) error`, `Git.IsWorktree(path string) bool`.
- Removes: `Git.RemoveWorktree(repoPath, worktreePath string, force bool) error`.

- [ ] **Step 1: Write the failing tests**

In `internal/git/git_test.go`, replace `TestRemoveWorktree` (lines 110-125) with:

```go
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
```

Ensure `internal/git/git_test.go` imports `"strings"` and `"os"` (add whichever is missing).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/git/ -run 'TestPruneWorktrees|TestIsWorktree' -v`
Expected: FAIL — `PruneWorktrees` / `IsWorktree` undefined (build error).

- [ ] **Step 3: Implement in `internal/git/git.go`**

In the `Git` interface (line 28), replace:

```go
	RemoveWorktree(repoPath, worktreePath string, force bool) error
```

with:

```go
	PruneWorktrees(repoPath string) error
	IsWorktree(path string) bool
```

Replace the `RemoveWorktree` method (lines 67-74) with:

```go
// PruneWorktrees drops registry entries whose directories are gone. fleet
// removes worktree trees itself (see internal/cleanup) and then prunes, rather
// than using `git worktree remove`: that command unlinks the registration
// before deleting the tree, so a partial delete — e.g. root-owned files written
// by a container — leaves an orphan that no later command can remove.
//
// Prune is repo-wide, so it also drops registrations for other worktrees of
// this repo whose directories are currently absent. That matches what `git gc`
// already does routinely.
func (c *CLI) PruneWorktrees(repoPath string) error {
	_, err := c.git(repoPath, "worktree", "prune")
	return err
}

// IsWorktree reports whether path is still a linked git worktree. A linked
// worktree is marked by a .git file pointing at the repo's admin directory;
// once that is gone git no longer recognises the path, which is exactly the
// state a partially failed removal leaves behind. Deliberately a stat rather
// than a subprocess: the refresher calls this for every session on every tick.
func (c *CLI) IsWorktree(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}
```

(`os` and `path/filepath` are already imported.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/git/ -v`
Expected: PASS. Other packages will not compile yet (their fakes still declare `RemoveWorktree`) — that is expected and fixed in Tasks 3 and 4.

- [ ] **Step 5: Commit** (deferred)

Do not commit yet: the tree does not build until Task 3 updates the callers. Commit at the end of Task 3.

---

### Task 3: `session.Manager.Delete` — resilient, always-forgets cleanup

**Files:**
- Modify: `internal/session/manager.go`
- Modify: `internal/session/manager_test.go`

**Interfaces:**
- Produces: `session.DeleteResult{Leftover string, LeftoverCount int, Samples []string}`; `(*Manager).Delete(s Session, deleteBranch bool) (DeleteResult, error)`.
- Consumes: `cleanup.RemoveTree` (via the injectable `removeTree` field), `git.PruneWorktrees`, `git.DeleteBranch`, `meta.Path`.

- [ ] **Step 1: Write the failing tests**

In `internal/session/manager_test.go`, update the fake (lines 17-37): replace the `RemoveWorktree` method with a prune recorder.

```go
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
```

Then fix any existing test that asserted on `f.removed` (search for `removed` in the file) to assert on `f.pruned` instead, and update every `mgr.Delete(...)` call to the two-value form. Append these tests:

```go
func TestDeletePartialRemovalStillForgetsSession(t *testing.T) {
	g := &fakeGit{}
	tm := &fakeTmux{}
	m, cfg := newManager(t, g, tm)

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
		return cleanup.Removal{BlockedCount: 42, Blocked: []string{wt + "/vendor"}}, nil
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
```

Add `"os"`, `"path/filepath"` and `"github.com/bray/fleet/internal/cleanup"` to the test imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -v`
Expected: FAIL — `m.removeTree` and `DeleteResult` undefined, `Delete` returns one value (build errors).

- [ ] **Step 3: Implement in `internal/session/manager.go`**

Set the import block to:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bray/fleet/internal/cleanup"
	"github.com/bray/fleet/internal/config"
	"github.com/bray/fleet/internal/forge"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/meta"
	"github.com/bray/fleet/internal/naming"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/tmux"
)
```

Add `removeTree` to the `Manager` struct (after `clock`):

```go
	// removeTree deletes a worktree directory best-effort; injectable for tests.
	removeTree func(string) (cleanup.Removal, error)
```

and default it in `NewManager`, alongside the existing `clock` default:

```go
	return &Manager{cfg: cfg, tmux: t, git: g, forge: f, clock: clock, removeTree: cleanup.RemoveTree}
```

Replace `Delete` (lines 101-114) with:

```go
// DeleteResult reports what a Delete left behind on disk.
type DeleteResult struct {
	Leftover      string   // worktree path if files survived; "" when fully clean
	LeftoverCount int
	Samples       []string
}

// Delete kills the session's window, removes the worktree, reconciles the
// repo's worktree registry, and optionally deletes the branch.
//
// Files it cannot delete — typically root-owned output written by a container
// into the bind-mounted worktree — do not fail the delete. The session is
// always forgotten (meta removed, registry pruned, branch deleted) and the
// leftovers are reported, so a blocked file can never strand an un-removable
// session on the dashboard.
func (m *Manager) Delete(s Session, deleteBranch bool) (DeleteResult, error) {
	// Safety net: fleet deletes trees itself now, so refuse anything that is
	// not strictly inside its own worktree base dir. A corrupt meta must never
	// be able to point the walk at a real repo.
	if !underBase(m.cfg.WorktreeBaseDir, s.WorktreePath) {
		return DeleteResult{}, fmt.Errorf("refusing to delete %s: outside worktree base dir %s",
			s.WorktreePath, m.cfg.WorktreeBaseDir)
	}

	_ = m.tmux.KillWindow(naming.WindowTarget(s.Project, s.Name)) // ignore: may already be gone

	rem, err := m.removeTree(s.WorktreePath)
	if err != nil {
		return DeleteResult{}, err
	}

	// Whatever survived, make sure the directory is neither a git worktree nor
	// a fleet session any more, so neither git nor the dashboard keeps tracking
	// it. Both live at the top of the worktree and are fleet/git-owned, so they
	// are removable even when the tree below is not.
	_ = os.RemoveAll(filepath.Join(s.WorktreePath, ".git"))
	_ = os.RemoveAll(filepath.Join(s.WorktreePath, ".fleet"))
	if _, statErr := os.Stat(meta.Path(s.WorktreePath)); statErr == nil {
		return DeleteResult{}, fmt.Errorf("could not remove %s: the session would stay on the dashboard",
			meta.Path(s.WorktreePath))
	}

	if err := m.git.PruneWorktrees(s.RepoPath); err != nil {
		return DeleteResult{}, err
	}
	if deleteBranch {
		// After the prune git no longer believes the branch is checked out, so
		// this succeeds even for a worktree whose files could not be removed.
		if err := m.git.DeleteBranch(s.RepoPath, s.Branch, true); err != nil {
			return DeleteResult{}, err
		}
	}

	if rem.Complete() {
		return DeleteResult{}, nil
	}
	return DeleteResult{
		Leftover:      s.WorktreePath,
		LeftoverCount: rem.BlockedCount,
		Samples:       rem.Blocked,
	}, nil
}

// underBase reports whether path sits strictly inside base.
func underBase(base, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ ./internal/git/ ./internal/cleanup/ -v`
Expected: PASS. `./internal/refresher` and `./internal/ui` still fail to build — fixed in Tasks 4 and 5.

- [ ] **Step 5: Commit**

```bash
git add internal/git/ internal/session/
git commit -m "$(cat <<'EOF'
fix(session): never strand a session when its worktree can't be deleted

git worktree remove unlinks the registration before deleting the tree, so a
partial delete (root-owned container output) orphaned the worktree: the
dashboard kept showing the session and every retry failed with "is not a
working tree". Remove the tree ourselves, always drop the meta, and reconcile
the registry with git worktree prune.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: `refresher` — detect and expose the broken state

**Files:**
- Modify: `internal/session/session.go` (add `Broken`)
- Modify: `internal/refresher/refresher.go`
- Modify: `internal/refresher/refresher_test.go` (fake + new test)
- Modify: `internal/refresher/smoke_test.go` (stop using `RemoveWorktree`)

**Interfaces:**
- Produces: `session.Session.Broken bool`.
- Consumes: `git.IsWorktree` (Task 2).

- [ ] **Step 1: Write the failing tests**

In `internal/refresher/refresher_test.go`, update the fake (lines 17-26):

```go
type fakeGit struct {
	st         git.Status
	notWorktree bool // when true, IsWorktree reports false for every path
}

func (f fakeGit) DefaultBranch(string) (string, error) { return "main", nil }
func (f fakeGit) AddWorktree(_, _, _, _ string) error  { return nil }
func (f fakeGit) PruneWorktrees(string) error          { return nil }
func (f fakeGit) IsWorktree(string) bool               { return !f.notWorktree }
func (f fakeGit) DeleteBranch(_, _ string, _ bool) error { return nil }
func (f fakeGit) Status(string) (git.Status, error)      { return f.st, nil }
func (f fakeGit) Push(string, string) error              { return nil }
func (f fakeGit) IsRepo(string) bool                     { return true }
func (f fakeGit) Ignore(string, string) error            { return nil }
```

Append a test (mirror the setup of the existing `Build` test in this file — reuse whatever helper it uses to write a worktree + meta under a temp `WorktreeBaseDir`):

```go
func TestBuildMarksUnregisteredWorktreeBroken(t *testing.T) {
	cfg, _ := seedWorktree(t) // same helper/setup the existing Build test uses
	tm := &fakeTmux{}

	healthy, err := Build(cfg, tm, fakeGit{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if len(healthy) != 1 || healthy[0].Broken {
		t.Fatalf("expected a healthy session, got %+v", healthy)
	}

	broken, err := Build(cfg, tm, fakeGit{notWorktree: true}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 1 || !broken[0].Broken {
		t.Fatalf("expected the session to be marked broken, got %+v", broken)
	}
	// Branch still comes from meta so the row remains identifiable.
	if broken[0].Branch == "" {
		t.Error("expected the meta branch to survive on a broken session")
	}
}
```

> If `refresher_test.go` has no reusable seeding helper, extract one from the existing `Build` test first (pure refactor, tests still green) and use it in both.

In `internal/refresher/smoke_test.go`, replace the `g.RemoveWorktree(repo, wt, true)` call (line 116) with the new removal path:

```go
	// Remove worktree -> it disappears from the list.
	if _, err := cleanup.RemoveTree(wt); err != nil {
		t.Fatalf("remove worktree: %v", err)
	}
	if err := g.PruneWorktrees(repo); err != nil {
		t.Fatalf("prune: %v", err)
	}
```

and add `"github.com/bray/fleet/internal/cleanup"` to that file's imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/refresher/ -v`
Expected: FAIL — `Broken` undefined on `session.Session`.

- [ ] **Step 3: Implement**

In `internal/session/session.go`, add to `Session` after `Exited`:

```go
	Broken       bool // worktree directory exists but git no longer tracks it
```

In `internal/refresher/refresher.go`, replace the status block (lines 81-84) with:

```go
			// A worktree git no longer tracks is broken: its files survive but
			// it has no .git, so every git query against it would fail. Skip
			// the doomed Status subprocess and fall back to the meta branch.
			broken := !g.IsWorktree(wt)
			st := git.Status{Branch: md.Branch}
			if !broken {
				if s, err := g.Status(wt); err == nil {
					st = s
				}
			}
```

and add `Broken: broken,` to the `session.Session` literal (next to `Exited`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/refresher/ ./internal/session/ -v`
Expected: PASS (the smoke test may SKIP without `git`/`tmux` — acceptable).

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/refresher/
git commit -m "$(cat <<'EOF'
feat(refresher): flag worktrees git no longer tracks as broken

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: `ui` — render the broken state

**Files:**
- Modify: `internal/ui/theme.go` (add `brokenIcon`)
- Modify: `internal/ui/views.go` (`viewDashboard`)
- Modify: `internal/ui/model_test.go` (add a test)

**Interfaces:**
- Consumes: `session.Session.Broken`, existing `warnStyle`/`dimStyle`.

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/model_test.go`:

```go
func TestDashboardRendersBrokenSession(t *testing.T) {
	ss := sample()
	ss[0].Broken = true
	m := New(nil, "")
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: ss})
	out := updated.(Model).View()

	if !strings.Contains(out, "🚫") {
		t.Errorf("expected the broken glyph on the dashboard.\n---\n%s", out)
	}
	if !strings.Contains(out, "broken") {
		t.Errorf("expected a broken detail line.\n---\n%s", out)
	}
	if !strings.Contains(out, "🚫 broken") {
		t.Errorf("expected the legend to explain the broken glyph.\n---\n%s", out)
	}
}
```

> Match the existing tests' construction of `New(...)` and `sample()` in this file; adjust the call if their signatures differ.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDashboardRendersBrokenSession -v`
Expected: FAIL — no `🚫` in the output.

- [ ] **Step 3: Implement**

In `internal/ui/theme.go`, next to `activityIcon`:

```go
// brokenIcon marks a session whose worktree git no longer tracks. It is not an
// activity.State: activity is what Claude is doing, broken is worktree
// integrity, and the two are independent.
const brokenIcon = "🚫"
```

In `internal/ui/views.go::viewDashboard`, replace the identity/detail construction (lines 66-98) with:

```go
		// Tab number: the window index, or "-" when there is no live window.
		// tmux is configured with base-index 1 (see tmux.CLI), so a live window
		// is always >= 1 and 0 reliably means "no window".
		num := "-"
		if s.WindowIndex > 0 {
			num = fmt.Sprintf("%d", s.WindowIndex)
		}
		icon := activityIcon(s.Activity)
		if s.Broken {
			icon = brokenIcon
		}
		identity := fmt.Sprintf("%s %s %s  %s ← %s", num, icon, s.Name, s.Branch, s.Base)
		if i == m.cursor {
			cur.lines = append(cur.lines, selectedStyle.Render("› "+identity))
		} else {
			cur.lines = append(cur.lines, "  "+identity)
		}

		if s.Broken {
			// No git status to report — the worktree is not a worktree.
			cur.lines = append(cur.lines,
				warnStyle.Render("    broken · worktree missing from git · d to clean up"))
			continue
		}

		// Detail line: activity word, git state, age. Working sessions get the
		// animated spinner frame; other states render plain.
		detail := "    "
		if s.Activity == activity.Working {
			detail += m.spinner.View() + " "
		}
		detail += s.Activity.Label()
		if s.Git.Dirty {
			detail += fmt.Sprintf(" · ✱%d", s.Git.ChangeCount)
		} else {
			detail += " · clean"
		}
		if s.Git.Ahead > 0 || s.Git.Behind > 0 {
			detail += fmt.Sprintf(" · ↑%d↓%d", s.Git.Ahead, s.Git.Behind)
		}
		if !s.CreatedAt.IsZero() {
			detail += " · created " + s.CreatedAt.Format("2006-01-02 15:04")
		}
		cur.lines = append(cur.lines, dimStyle.Render(detail))
```

Extend the legend (line 123-125):

```go
	legend := fmt.Sprintf("legend: %s working  %s waiting  %s idle  %s exited  %s broken",
		activityIcon(activity.Working), activityIcon(activity.Waiting),
		activityIcon(activity.Idle), activityIcon(activity.Exited), brokenIcon)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -v`
Expected: `TestDashboardRendersBrokenSession` passes; `./internal/ui` still fails to build overall until Task 6 updates `Actions.Delete`. If so, run Task 5 and Task 6 back to back and verify at the end of Task 6.

- [ ] **Step 5: Commit** (may be folded into Task 6 if the package does not build standalone)

```bash
git add internal/ui/theme.go internal/ui/views.go internal/ui/model_test.go
git commit -m "$(cat <<'EOF'
feat(ui): show broken worktrees on the dashboard

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: `ui` — broken cleanup flow and leftover reporting

**Files:**
- Modify: `internal/ui/commands.go` (`sessionsUpdatedMsg.notice`)
- Modify: `internal/ui/model.go` (`Actions.Delete`, `cleanupIndex`, `cleanupOptions`, `runThenRefresh`, `callDelete`)
- Modify: `internal/ui/views.go` (`viewCleanupMenu`)
- Modify: `internal/ui/model_test.go`
- Modify: `main.go` — no change expected (`Delete: mgr.Delete` is a direct method reference); verify it still compiles.

**Interfaces:**
- Produces: `Actions.Delete func(session.Session, bool) (session.DeleteResult, error)`; `cleanupOptions(s session.Session) []cleanupOption`.
- Consumes: `session.DeleteResult` (Task 3).

- [ ] **Step 1: Write the failing tests**

Add to `internal/ui/model_test.go`:

```go
func TestBrokenSessionCleanupMenuOffersOnlyCleanup(t *testing.T) {
	ss := sample()
	ss[0].Broken = true
	m := New(nil, "")
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: ss})
	withMenu, _ := updated.(Model).Update(keyMsg("d"))
	out := withMenu.(Model).View()

	if strings.Contains(out, "push / open PR") || strings.Contains(out, "leave (kill tmux only)") {
		t.Errorf("a broken session has no worktree to push or leave.\n---\n%s", out)
	}
	if !strings.Contains(out, "clean up") {
		t.Errorf("expected a clean-up option.\n---\n%s", out)
	}
}

func TestDeleteReportsLeftoverFilesInStatus(t *testing.T) {
	ss := sample()
	ss[0].Git = git.Status{} // sample()[0] is dirty; clean it so delete skips the confirm screen
	acts := Actions{
		Delete: func(session.Session, bool) (session.DeleteResult, error) {
			return session.DeleteResult{
				Leftover:      "/wt/proj/sess",
				LeftoverCount: 42,
				Samples:       []string{"/wt/proj/sess/vendor"},
			}, nil
		},
		Refresh: func() ([]session.Session, error) { return nil, nil },
	}
	m := New(&acts, "")
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: ss})
	withMenu, _ := updated.(Model).Update(keyMsg("d"))
	_, cmd := withMenu.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a delete command")
	}
	final, _ := withMenu.(Model).Update(cmd())
	out := final.(Model).View()

	if !strings.Contains(out, "/wt/proj/sess") || !strings.Contains(out, "42") {
		t.Errorf("expected the leftover path and count in the status line.\n---\n%s", out)
	}
}
```

> `git` and `tea` are already imported by `model_test.go`; `session` is too. `sample()[0]` is dirty, hence the explicit reset above — without it the delete routes to the confirm screen instead.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -v`
Expected: FAIL — `Actions.Delete` signature mismatch (build error).

- [ ] **Step 3: Implement**

`internal/ui/commands.go` line 14:

```go
type sessionsUpdatedMsg struct {
	sessions []session.Session
	notice   string // status-line message from the action that triggered this refresh
}
```

`internal/ui/model.go`:

1. `Actions.Delete` (line 40):

```go
	Delete      func(s session.Session, deleteBranch bool) (session.DeleteResult, error)
```

2. Rename the model field (line 67) `cleanupChoice cleanupChoice` → `cleanupIndex int`, keeping the comment.

3. In the `sessionsUpdatedMsg` case (after line 103), set the notice:

```go
		m.sessions = msg.sessions
		if msg.notice != "" {
			m.status = msg.notice
		}
```

(Periodic refreshes send an empty notice, so they never clobber the status line.)

4. In `keyDashboard` (line 193): `m.cleanupChoice = cleanupDelete` → `m.cleanupIndex = 0`.

5. Add the menu model next to the `cleanupChoice` constants:

```go
type cleanupOption struct {
	choice cleanupChoice
	label  string
}

// cleanupOptions returns the cleanup menu for a session. A broken session has
// no usable worktree, so pushing and leaving are meaningless — cleanup is the
// only thing left to do with it.
func cleanupOptions(s session.Session) []cleanupOption {
	if s.Broken {
		return []cleanupOption{{cleanupDelete, "🧹 clean up (remove broken session)"}}
	}
	return []cleanupOption{
		{cleanupDelete, "🗑  delete worktree + branch"},
		{cleanupPushPR, "🚀 push / open PR"},
		{cleanupLeave, "👋 leave (kill tmux only)"},
	}
}
```

`cleanupChoiceCount` becomes unused — delete it from the const block.

6. Replace `keyCleanupMenu` (lines 289-325):

```go
func (m Model) keyCleanupMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s, ok := m.selected()
	if !ok {
		m.state = stateDashboard
		return m, nil
	}
	opts := cleanupOptions(s)
	switch msg.String() {
	case "esc":
		m.state = stateDashboard
	case "up", "k":
		if m.cleanupIndex > 0 {
			m.cleanupIndex--
		}
	case "down", "j":
		if m.cleanupIndex < len(opts)-1 {
			m.cleanupIndex++
		}
	case "enter":
		if m.cleanupIndex >= len(opts) {
			m.state = stateDashboard
			return m, nil
		}
		switch opts[m.cleanupIndex].choice {
		case cleanupLeave:
			m.state = stateDashboard
			return m, m.runThenRefresh(func() (string, error) { return "", m.callLeave(s) })
		case cleanupPushPR:
			m.state = stateDashboard
			return m, m.runThenRefresh(func() (string, error) { return "", m.callPushPR(s) })
		case cleanupDelete:
			// A broken session has no readable git status, so there is nothing
			// to warn about — go straight to the cleanup.
			if !s.Broken && (s.Git.Dirty || s.Git.Ahead > 0) {
				m.pendingDelete = s
				m.state = stateConfirm
				return m, nil
			}
			m.state = stateDashboard
			return m, m.runThenRefresh(func() (string, error) { return m.callDelete(s) })
		}
	}
	return m, nil
}
```

7. `keyConfirm` (line 332): `return m, m.runThenRefresh(func() (string, error) { return m.callDelete(s) })`

8. Replace `callDelete` (lines 357-362) and `runThenRefresh` (lines 371-386):

```go
func (m Model) callDelete(s session.Session) (string, error) {
	if m.actions.Delete == nil {
		return "", nil
	}
	res, err := m.actions.Delete(s, true) // delete branch too
	if err != nil {
		return "", err
	}
	if res.Leftover != "" {
		return fmt.Sprintf("⚠ cleaned up %s/%s — %d files left at %s (sudo rm -rf to reclaim)",
			s.Project, s.Name, res.LeftoverCount, res.Leftover), nil
	}
	return fmt.Sprintf("✓ deleted %s/%s", s.Project, s.Name), nil
}

// runThenRefresh runs fn, then refreshes the session list. fn's string is an
// optional status-line notice, carried back with the refreshed list so the
// message and the new state land in one update.
func (m Model) runThenRefresh(fn func() (string, error)) tea.Cmd {
	refreshFn := m.actions.Refresh
	return func() tea.Msg {
		notice, err := fn()
		if err != nil {
			return errorMsg{err: err}
		}
		if refreshFn != nil {
			ss, err := refreshFn()
			if err != nil {
				return errorMsg{err: err}
			}
			return sessionsUpdatedMsg{sessions: ss, notice: notice}
		}
		return sessionsUpdatedMsg{notice: notice}
	}
}
```

`internal/ui/views.go` — replace `viewCleanupMenu` (lines 157-171):

```go
func (m Model) viewCleanupMenu() string {
	s, _ := m.selected()
	var b strings.Builder
	b.WriteString(gradientTitle("✨ cleanup — "+s.Project+"/"+s.Name+" ✨") + "\n\n")
	if s.Broken {
		b.WriteString(warnStyle.Render("this worktree is no longer registered with git") + "\n\n")
	}
	for i, o := range cleanupOptions(s) {
		if i == m.cleanupIndex {
			b.WriteString(selectedStyle.Render("› "+o.label) + "\n")
		} else {
			b.WriteString("  " + o.label + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("enter choose · esc cancel"))
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./... `
Expected: PASS across every package. `main.go` needs no edit — `Delete: mgr.Delete` picks up the new signature — but confirm the build is clean.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/ main.go
git commit -m "$(cat <<'EOF'
feat(ui): clean up broken sessions and report leftover files

Broken sessions get a single-option cleanup menu, and a partial delete now
reports the path and file count it could not remove instead of failing.

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 7: Documentation and full verification

**Files:**
- Modify: `docs/usage.md` (cleanup section — document the broken state)
- Modify: `CLAUDE.md` (add `cleanup` to the package layout list)

- [ ] **Step 1: Document the broken state**

In `docs/usage.md`, in the cleanup section, add a short subsection explaining that a session shows as `🚫 broken` when its worktree is no longer registered with git (typically after a delete was blocked by root-owned files a container wrote into the worktree), that `d` on such a session removes it from fleet, and that files fleet could not delete are reported in the status line for manual `sudo rm -rf`.

In `CLAUDE.md`, add to the package layout list, after the `config` entry:

```markdown
- `cleanup` — best-effort recursive removal of a worktree directory, reporting
  what it could not delete.
```

- [ ] **Step 2: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all pass (git/tmux integration tests may SKIP if those binaries are absent — acceptable; no FAILs).

- [ ] **Step 3: Verify against the real orphans**

The three stuck sessions are deliberately still on disk as live fixtures:
`ascension/mag_make_fix`, `ascension/mago_consolidation`, `ascension/mago_raise_bar`.

Run `go run .` and confirm:
- each renders with `🚫` and the broken detail line;
- `d` shows the single clean-up option;
- confirming it removes the session from the dashboard, reports the leftover
  path and a file count in the thousands, and does not error;
- `git -C /home/bray/code/ascension branch --list` no longer shows
  `mag_make_fix`, `mago-track-a-consolidation`, `mago-track-c-raise-safety-bar`;
- the leftover directories still exist (expected — they need `sudo rm -rf`), and
  re-running `go run .` no longer lists those sessions.

Then reclaim the disk manually:

```bash
sudo rm -rf /home/bray/.local/share/fleet/worktrees/ascension/{mag_make_fix,mago_consolidation,mago_raise_bar,mago_144}
```

(`mago_144` has no meta, so fleet never saw it — see the spec's Future scope.)

- [ ] **Step 4: Commit**

```bash
git add docs/ CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: document the broken-worktree state and cleanup package

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**1. Spec coverage:**
- Best-effort + forget → Task 1 (`RemoveTree`) + Task 3 (forced meta removal, leftovers reported, never fails on blocked files). ✓
- fleet removes the tree, then prunes; never `git worktree remove` → Tasks 2 & 3. ✓
- Broken detection via `.git` stat → Task 2 (`IsWorktree`) + Task 4 (refresher). ✓
- `Session.Broken bool`, not an `activity.State` → Task 4. ✓
- Broken cleanup menu collapses to one choice → Task 6 (`cleanupOptions`). ✓
- Leftover reporting in the status line → Task 6 (`notice`, `callDelete`). ✓
- Base-dir safety guard → Task 3 (`underBase` + refusal test). ✓
- Non-goals respected: no `sudo`, no create/attach/push/leave changes, no retry. ✓
- Future scope (meta-less orphans, bulk clean) deliberately not implemented. ✓

**2. Placeholder scan:** No "TBD" / "handle edge cases". Two steps are explicitly parameterised on existing test scaffolding rather than guessed: Task 4 Step 1 (`seedWorktree` — extract from the existing `Build` test) and Task 5/6 (`sample()` / `New(...)` call shapes). Both say so and give the fallback.

**3. Type consistency:** `cleanup.Removal` is produced by `RemoveTree` and consumed by `Manager.removeTree func(string) (cleanup.Removal, error)`. `session.DeleteResult` is produced by `Manager.Delete` and consumed by `ui.Actions.Delete` and `callDelete`. `cleanupOptions` returns `[]cleanupOption`, indexed by `m.cleanupIndex int`, dispatching on the `cleanupChoice` enum, which keeps its existing constants (minus `cleanupChoiceCount`, now unused). `runThenRefresh` takes `func() (string, error)` at all four call sites (leave, pushPR, delete, confirm).

**4. Build ordering:** Task 2 intentionally leaves the tree non-building (fakes still declare `RemoveWorktree`) and defers its commit into Task 3; Task 5 may likewise only build once Task 6 lands. Both are called out in-place so an executing agent does not mistake it for a failure.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-28-broken-worktree-cleanup.md`. Two execution options:

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks.

**2. Inline Execution** — execute here in batches with checkpoints.

Which approach?
