# Support Creating Sessions for Existing Branches — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let fleet create a session worktree for an existing local branch or a remote-only branch, instead of always trying to create a brand-new branch.

**Architecture:** The git adapter (`internal/git`) gains thin query helpers (`LocalBranchExists`, `RemoteBranchExists`, `ListBranches`, `Fetch`) and two one-command worktree creators (`AddWorktreeExisting`, `AddWorktreeTracking`). The session manager (`internal/session`) resolves which of the three cases applies at submit time and calls the right command. The UI (`internal/ui`) loads a branch list when the new-session form opens (refreshing it via a background `git fetch`) to drive an advisory live hint; the manager remains authoritative.

**Tech Stack:** Go, Bubble Tea (Charm ecosystem), shelling out to `git`.

## Global Constraints

- Conventional Commits for every commit (e.g. `feat:`, `test:`, `refactor:`).
- TDD: write the failing test first, watch it fail, implement minimally, watch it pass, commit.
- Adapters stay thin: each `git.CLI` method wraps a single git command; branching logic lives in the manager, not the adapter.
- `git` is authoritative for branch state; the UI's cached branch list is advisory only.
- Build/test commands: `go build ./...`, `go test ./...`.
- Reference spec: `docs/superpowers/specs/2026-06-18-support-existing-branches-design.md`.

---

## File Structure

- `internal/git/git.go` — add `Branches` type, four query methods, two worktree creators to the `Git` interface and `CLI`.
- `internal/git/git_test.go` — real-repo tests for all new methods, plus shared helpers (`repoWithRemoteFeature`, `contains`).
- `internal/session/manager.go` — `Create` resolves the branch case; new helper `addWorktreeForBranch` + `isAlreadyCheckedOut`.
- `internal/session/manager_test.go` — extend `fakeGit` with the new methods/fields; tests for each case.
- `internal/refresher/refresher_test.go` — extend its `fakeGit` with no-op stubs for the new interface methods (compile-time `_ git.Git = fakeGit{}` assertion).
- `internal/ui/commands.go` — new message types and `loadBranches` / `fetchBranches` commands.
- `internal/ui/model.go` — `Actions.Branches` / `Actions.FetchBranches`; handle new messages; dispatch on form open; import `git`.
- `internal/ui/newsession.go` — form fields `localBranches`, `remoteBranches`, `fetchWarning`; `branchHint()`; render hint + warning; `containsStr` helper.
- `internal/ui/newsession_test.go` — new file: pure `branchHint` tests.
- `internal/ui/model_test.go` — tests that the new messages update form state.
- `main.go` — wire `Branches` and `FetchBranches` actions.

---

## Task 1: Git branch queries (`Branches`, existence checks, list, fetch)

**Files:**
- Modify: `internal/git/git.go` (interface + `CLI`)
- Test: `internal/git/git_test.go`
- Modify: `internal/session/manager_test.go` (extend `fakeGit`)
- Modify: `internal/refresher/refresher_test.go` (extend `fakeGit`)

**Interfaces:**
- Produces:
  - `type Branches struct { Local []string; Remote []string }`
  - `LocalBranchExists(repoPath, branch string) (bool, error)`
  - `RemoteBranchExists(repoPath, branch string) (bool, error)`
  - `ListBranches(repoPath string) (Branches, error)` — remote names have the `origin/` prefix stripped and exclude `HEAD`
  - `Fetch(repoPath string) error`

- [ ] **Step 1: Write failing tests + shared helper**

Add to `internal/git/git_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/git/ -run 'TestLocalBranchExists|TestRemoteBranchExists|TestListBranches|TestFetchPopulatesRemoteRefs' -v`
Expected: compile failure — `g.LocalBranchExists undefined` etc.

- [ ] **Step 3: Implement the query methods in `internal/git/git.go`**

Add `"os/exec"` is already imported. Add the type and methods, and extend the `Git` interface.

Add to the `Git` interface (after `Ignore`):

```go
	LocalBranchExists(repoPath, branch string) (bool, error)
	RemoteBranchExists(repoPath, branch string) (bool, error)
	ListBranches(repoPath string) (Branches, error)
	Fetch(repoPath string) error
```

Add the type near `Status`:

```go
// Branches is the set of branch names known to a repo: local heads and
// origin-tracked remotes (with the "origin/" prefix stripped, HEAD excluded).
type Branches struct {
	Local  []string
	Remote []string
}
```

Add the implementations:

```go
// refExists reports whether ref resolves in repoPath. show-ref exits 0 when the
// ref exists, 1 when it does not, and >1 on a real error.
func (c *CLI) refExists(repoPath, ref string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = repoPath
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git show-ref %s: %w", ref, err)
}

func (c *CLI) LocalBranchExists(repoPath, branch string) (bool, error) {
	return c.refExists(repoPath, "refs/heads/"+branch)
}

func (c *CLI) RemoteBranchExists(repoPath, branch string) (bool, error) {
	return c.refExists(repoPath, "refs/remotes/origin/"+branch)
}

// ListBranches returns local head names and origin remote-tracking names
// (prefix stripped, HEAD excluded).
func (c *CLI) ListBranches(repoPath string) (Branches, error) {
	local, err := c.git(repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return Branches{}, err
	}
	remote, err := c.git(repoPath, "for-each-ref", "--format=%(refname:short)", "refs/remotes/origin")
	if err != nil {
		return Branches{}, err
	}
	var b Branches
	for _, line := range strings.Split(local, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			b.Local = append(b.Local, line)
		}
	}
	for _, line := range strings.Split(remote, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "origin/"))
		if line == "" || line == "HEAD" {
			continue
		}
		b.Remote = append(b.Remote, line)
	}
	return b, nil
}

func (c *CLI) Fetch(repoPath string) error {
	_, err := c.git(repoPath, "fetch", "origin")
	return err
}
```

- [ ] **Step 4: Keep the fakes implementing the interface**

The compile-time assertion `_ git.Git = fakeGit{}` in `internal/refresher/refresher_test.go` and the `git.Git` parameter in `internal/session/manager_test.go` now require these methods on both fakes.

In `internal/refresher/refresher_test.go`, add (value receivers, matching the existing style):

```go
func (f fakeGit) LocalBranchExists(string, string) (bool, error)  { return false, nil }
func (f fakeGit) RemoteBranchExists(string, string) (bool, error) { return false, nil }
func (f fakeGit) ListBranches(string) (git.Branches, error)       { return git.Branches{}, nil }
func (f fakeGit) Fetch(string) error                              { return nil }
```

In `internal/session/manager_test.go`, add fields to `fakeGit` and the methods (pointer receivers, matching the existing style). Add the fields to the struct:

```go
	localExists  map[string]bool
	remoteExists map[string]bool
```

And the methods:

```go
func (f *fakeGit) LocalBranchExists(_, b string) (bool, error)  { return f.localExists[b], nil }
func (f *fakeGit) RemoteBranchExists(_, b string) (bool, error) { return f.remoteExists[b], nil }
func (f *fakeGit) ListBranches(string) (git.Branches, error)    { return git.Branches{}, nil }
func (f *fakeGit) Fetch(string) error                           { return nil }
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/git/ ./internal/session/ ./internal/refresher/ -v`
Expected: PASS (git tests green; session/refresher still compile and pass).

- [ ] **Step 6: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go internal/session/manager_test.go internal/refresher/refresher_test.go
git commit -m "feat(git): add branch existence, list, and fetch queries"
```

---

## Task 2: Git worktree creators for existing & remote branches

**Files:**
- Modify: `internal/git/git.go` (interface + `CLI`)
- Test: `internal/git/git_test.go`
- Modify: `internal/session/manager_test.go` (extend `fakeGit`)
- Modify: `internal/refresher/refresher_test.go` (extend `fakeGit`)

**Interfaces:**
- Consumes: `Branches`, `LocalBranchExists`, `repoWithRemoteFeature` (Task 1).
- Produces:
  - `AddWorktreeExisting(repoPath, worktreePath, branch string) error` → `git worktree add <wt> <branch>`
  - `AddWorktreeTracking(repoPath, worktreePath, branch string) error` → `git worktree add --track -b <branch> <wt> origin/<branch>`

- [ ] **Step 1: Write failing tests**

Add to `internal/git/git_test.go`:

```go
func TestAddWorktreeExistingChecksOutBranch(t *testing.T) {
	repo := newRepo(t)
	g := New()
	run(t, repo, "git", "branch", "feature", "main")
	wt := filepath.Join(t.TempDir(), "wt")
	if err := g.AddWorktreeExisting(repo, wt, "feature"); err != nil {
		t.Fatalf("add existing: %v", err)
	}
	st, err := g.Status(wt)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Branch != "feature" {
		t.Fatalf("got branch %q", st.Branch)
	}
}

func TestAddWorktreeExistingFailsWhenCheckedOut(t *testing.T) {
	repo := newRepo(t)
	g := New()
	run(t, repo, "git", "branch", "feature", "main")
	wt1 := filepath.Join(t.TempDir(), "wt1")
	if err := g.AddWorktreeExisting(repo, wt1, "feature"); err != nil {
		t.Fatalf("first add: %v", err)
	}
	wt2 := filepath.Join(t.TempDir(), "wt2")
	if err := g.AddWorktreeExisting(repo, wt2, "feature"); err == nil {
		t.Fatal("expected error: branch already checked out elsewhere")
	}
}

func TestAddWorktreeTrackingCreatesLocalFromRemote(t *testing.T) {
	repo := repoWithRemoteFeature(t)
	g := New()
	wt := filepath.Join(t.TempDir(), "wt")
	if err := g.AddWorktreeTracking(repo, wt, "feature"); err != nil {
		t.Fatalf("tracking: %v", err)
	}
	st, err := g.Status(wt)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Branch != "feature" {
		t.Fatalf("got branch %q", st.Branch)
	}
	if ok, _ := g.LocalBranchExists(repo, "feature"); !ok {
		t.Fatal("expected local feature branch created")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/git/ -run 'TestAddWorktreeExisting|TestAddWorktreeTracking' -v`
Expected: compile failure — `g.AddWorktreeExisting undefined`, `g.AddWorktreeTracking undefined`.

- [ ] **Step 3: Implement in `internal/git/git.go`**

Add to the `Git` interface (after the `Fetch` line from Task 1):

```go
	AddWorktreeExisting(repoPath, worktreePath, branch string) error
	AddWorktreeTracking(repoPath, worktreePath, branch string) error
```

Add the implementations next to `AddWorktree`:

```go
// AddWorktreeExisting checks an existing local branch out into a new worktree.
// git refuses if the branch is already checked out in another worktree.
func (c *CLI) AddWorktreeExisting(repoPath, worktreePath, branch string) error {
	_, err := c.git(repoPath, "worktree", "add", worktreePath, branch)
	return err
}

// AddWorktreeTracking creates a local branch tracking origin/<branch> in a new
// worktree.
func (c *CLI) AddWorktreeTracking(repoPath, worktreePath, branch string) error {
	_, err := c.git(repoPath, "worktree", "add", "--track", "-b", branch, worktreePath, "origin/"+branch)
	return err
}
```

- [ ] **Step 4: Keep the fakes implementing the interface**

In `internal/refresher/refresher_test.go`, add:

```go
func (f fakeGit) AddWorktreeExisting(_, _, _ string) error { return nil }
func (f fakeGit) AddWorktreeTracking(_, _, _ string) error { return nil }
```

In `internal/session/manager_test.go`, add recording fields to `fakeGit`:

```go
	addedExisting []string
	addedTracking []string
	existingErr   error // returned by AddWorktreeExisting when set
```

And the methods:

```go
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/git/ ./internal/session/ ./internal/refresher/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/git/git.go internal/git/git_test.go internal/session/manager_test.go internal/refresher/refresher_test.go
git commit -m "feat(git): add worktree creators for existing and remote branches"
```

---

## Task 3: Manager resolves the branch case

**Files:**
- Modify: `internal/session/manager.go`
- Test: `internal/session/manager_test.go`

**Interfaces:**
- Consumes: `git.LocalBranchExists`, `git.RemoteBranchExists`, `git.AddWorktree`, `git.AddWorktreeExisting`, `git.AddWorktreeTracking`; `fakeGit` fields `localExists`, `remoteExists`, `addedExisting`, `addedTracking`, `existingErr` (Tasks 1–2).
- Produces: unchanged `Create` signature; `Create` now picks the right worktree command and returns a friendly error when the branch is checked out elsewhere.

- [ ] **Step 1: Write failing tests**

Add to `internal/session/manager_test.go`. Add `"errors"` and `"strings"` to its imports.

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/ -run TestCreate -v`
Expected: `TestCreateUsesExistingLocalBranch` / `TestCreateTracksRemoteBranch` / `TestCreateExistingBranchCheckedOutElsewhere` FAIL (current `Create` always calls `AddWorktree`, so `addedExisting` / `addedTracking` are empty and no friendly error is produced).

- [ ] **Step 3: Implement in `internal/session/manager.go`**

Add `"fmt"` and `"strings"` to the imports. Replace the first line of `Create`'s body (the `AddWorktree` call) so it delegates to a resolver:

Change:

```go
	wt := naming.WorktreePath(m.cfg.WorktreeBaseDir, p.Name, name)
	if err := m.git.AddWorktree(p.Path, wt, branch, base); err != nil {
		return Session{}, err
	}
```

to:

```go
	wt := naming.WorktreePath(m.cfg.WorktreeBaseDir, p.Name, name)
	if err := m.addWorktreeForBranch(p.Path, wt, branch, base); err != nil {
		return Session{}, err
	}
```

Add these helpers below `Create`:

```go
// addWorktreeForBranch picks the right git worktree command: check out an
// existing local branch, track a remote-only branch, or create a new branch
// from base.
func (m *Manager) addWorktreeForBranch(repoPath, wt, branch, base string) error {
	local, err := m.git.LocalBranchExists(repoPath, branch)
	if err != nil {
		return err
	}
	if local {
		if err := m.git.AddWorktreeExisting(repoPath, wt, branch); err != nil {
			if isAlreadyCheckedOut(err) {
				return fmt.Errorf("branch %q is already checked out in another worktree", branch)
			}
			return err
		}
		return nil
	}
	remote, err := m.git.RemoteBranchExists(repoPath, branch)
	if err != nil {
		return err
	}
	if remote {
		return m.git.AddWorktreeTracking(repoPath, wt, branch)
	}
	return m.git.AddWorktree(repoPath, wt, branch, base)
}

// isAlreadyCheckedOut detects git's "branch is in use by another worktree"
// failure across git versions.
func isAlreadyCheckedOut(err error) bool {
	s := err.Error()
	return strings.Contains(s, "already checked out") || strings.Contains(s, "already used by worktree")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/ -v`
Expected: PASS (including the pre-existing `TestCreateAddsWorktreeMetaAndTmux`, whose `fakeGit{}` has neither branch, so it still takes the `AddWorktree` path).

- [ ] **Step 5: Commit**

```bash
git add internal/session/manager.go internal/session/manager_test.go
git commit -m "feat(session): create worktrees for existing and remote branches"
```

---

## Task 4: UI plumbing — load branch list on form open

**Files:**
- Modify: `internal/ui/commands.go`
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/newsession.go` (add state fields only)
- Test: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `git.Branches` (Task 1); `projects.Project`.
- Produces:
  - messages `branchesLoadedMsg{branches git.Branches}` and `branchesRefreshedMsg{branches git.Branches; fetchErr error}`
  - commands `loadBranches(fn func(projects.Project) (git.Branches, error), p projects.Project) tea.Cmd` and `fetchBranches(fn func(projects.Project) (git.Branches, error), p projects.Project) tea.Cmd`
  - `Actions.Branches` and `Actions.FetchBranches` fields
  - `newSessionForm` fields `localBranches []string`, `remoteBranches []string`, `fetchWarning string`

- [ ] **Step 1: Write failing tests**

Add to `internal/ui/model_test.go`:

```go
func TestBranchesLoadedMsgPopulatesForm(t *testing.T) {
	m := New(nil, "")
	m.state = stateNewSession
	m.form = newForm(projects.Project{Name: "app", DefaultBranch: "main"})
	updated, _ := m.Update(branchesLoadedMsg{branches: git.Branches{
		Local:  []string{"main", "feature"},
		Remote: []string{"feature", "remote-only"},
	}})
	f := updated.(Model).form
	if !strings.Contains(strings.Join(f.localBranches, ","), "feature") {
		t.Fatalf("local branches not stored: %v", f.localBranches)
	}
	if !strings.Contains(strings.Join(f.remoteBranches, ","), "remote-only") {
		t.Fatalf("remote branches not stored: %v", f.remoteBranches)
	}
}

func TestBranchesRefreshedMsgFetchErrorSetsWarning(t *testing.T) {
	m := New(nil, "")
	m.state = stateNewSession
	m.form = newForm(projects.Project{Name: "app", DefaultBranch: "main"})
	updated, _ := m.Update(branchesRefreshedMsg{fetchErr: errors.New("offline")})
	f := updated.(Model).form
	if f.fetchWarning == "" {
		t.Fatal("expected a fetch warning to be set")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestBranchesLoadedMsg|TestBranchesRefreshedMsg' -v`
Expected: compile failure — `branchesLoadedMsg` / `branchesRefreshedMsg` / form fields undefined.

- [ ] **Step 3: Add the form state fields**

In `internal/ui/newsession.go`, add to the `newSessionForm` struct (after `branchTouched`):

```go
	// branch list cached when the form opens, used only for the advisory hint.
	localBranches  []string
	remoteBranches []string
	// fetchWarning is a soft, non-blocking notice shown if the background
	// `git fetch` failed; branch detection then falls back to local refs.
	fetchWarning string
```

- [ ] **Step 4: Add messages and commands**

In `internal/ui/commands.go`, add `"github.com/bray/fleet/internal/git"` to the imports, then add:

```go
type branchesLoadedMsg struct{ branches git.Branches }
type branchesRefreshedMsg struct {
	branches git.Branches
	fetchErr error
}

// loadBranches lists branches from local refs immediately (works offline) so
// the form hint is usable right away.
func loadBranches(fn func(projects.Project) (git.Branches, error), p projects.Project) tea.Cmd {
	return func() tea.Msg {
		if fn == nil {
			return branchesLoadedMsg{}
		}
		br, err := fn(p)
		if err != nil {
			return errorMsg{err: err}
		}
		return branchesLoadedMsg{branches: br}
	}
}

// fetchBranches refreshes remote refs in the background, then re-lists. A fetch
// failure is reported softly via fetchErr; branches still reflect local refs.
func fetchBranches(fn func(projects.Project) (git.Branches, error), p projects.Project) tea.Cmd {
	return func() tea.Msg {
		if fn == nil {
			return branchesRefreshedMsg{}
		}
		br, err := fn(p)
		return branchesRefreshedMsg{branches: br, fetchErr: err}
	}
}
```

- [ ] **Step 5: Add Actions fields and message handlers, dispatch on form open**

In `internal/ui/model.go`, add `"github.com/bray/fleet/internal/git"` to the imports.

Add to the `Actions` struct (after `Create`):

```go
	Branches      func(p projects.Project) (git.Branches, error)
	FetchBranches func(p projects.Project) (git.Branches, error)
```

Add two cases to the `Update` switch (next to `projectsLoadedMsg`):

```go
	case branchesLoadedMsg:
		if m.state == stateNewSession {
			m.form.localBranches = msg.branches.Local
			m.form.remoteBranches = msg.branches.Remote
		}
		return m, nil

	case branchesRefreshedMsg:
		if m.state == stateNewSession {
			if msg.fetchErr != nil {
				m.form.fetchWarning = "⚠ couldn't fetch from origin — branch list may be stale"
			} else {
				m.form.localBranches = msg.branches.Local
				m.form.remoteBranches = msg.branches.Remote
			}
		}
		return m, nil
```

In `keyProjectPicker`, change the `enter` case to dispatch the loads. Replace:

```go
	case "enter":
		if len(m.projects) == 0 {
			return m, nil
		}
		m.form = newForm(m.projects[m.cursor])
		m.state = stateNewSession
		m.cursor = 0
	}
	return m, nil
```

with:

```go
	case "enter":
		if len(m.projects) == 0 {
			return m, nil
		}
		p := m.projects[m.cursor]
		m.form = newForm(p)
		m.state = stateNewSession
		m.cursor = 0
		return m, tea.Batch(
			loadBranches(m.actions.Branches, p),
			fetchBranches(m.actions.FetchBranches, p),
		)
	}
	return m, nil
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestBranchesLoadedMsg|TestBranchesRefreshedMsg' -v`
Expected: PASS.

- [ ] **Step 7: Run the full ui package tests**

Run: `go test ./internal/ui/`
Expected: PASS (existing tests unaffected; `New(nil, ...)` leaves the new actions nil and the commands no-op).

- [ ] **Step 8: Commit**

```bash
git add internal/ui/commands.go internal/ui/model.go internal/ui/newsession.go internal/ui/model_test.go
git commit -m "feat(ui): load branch list when the new-session form opens"
```

---

## Task 5: UI hint — classify the typed branch and render feedback

**Files:**
- Modify: `internal/ui/newsession.go`
- Create: `internal/ui/newsession_test.go`

**Interfaces:**
- Consumes: `newSessionForm` fields `branch`, `base`, `localBranches`, `remoteBranches`, `fetchWarning` (Task 4).
- Produces: `func (f newSessionForm) branchHint() string`; `containsStr([]string, string) bool`; hint + warning rendered in `view()`.

- [ ] **Step 1: Write failing tests**

Create `internal/ui/newsession_test.go`:

```go
package ui

import (
	"strings"
	"testing"

	"github.com/bray/fleet/internal/projects"
)

func TestBranchHintNewBranch(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.branch = "brand-new"
	if got := f.branchHint(); got != "new branch from main" {
		t.Fatalf("got %q", got)
	}
}

func TestBranchHintExistingLocal(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.localBranches = []string{"main", "feature"}
	f.branch = "feature"
	if got := f.branchHint(); !strings.Contains(got, "existing local branch") {
		t.Fatalf("got %q", got)
	}
}

func TestBranchHintRemoteOnly(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.remoteBranches = []string{"feature"}
	f.branch = "feature"
	got := f.branchHint()
	if !strings.Contains(got, "tracks origin/feature") {
		t.Fatalf("got %q", got)
	}
}

func TestBranchHintLocalWinsOverRemote(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.localBranches = []string{"feature"}
	f.remoteBranches = []string{"feature"}
	f.branch = "feature"
	if got := f.branchHint(); !strings.Contains(got, "existing local branch") {
		t.Fatalf("local should win, got %q", got)
	}
}

func TestBranchHintEmptyWhenBlank(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.branch = ""
	if got := f.branchHint(); got != "" {
		t.Fatalf("expected empty hint, got %q", got)
	}
}

func TestViewShowsFetchWarning(t *testing.T) {
	f := newForm(projects.Project{Name: "app", DefaultBranch: "main"})
	f.fetchWarning = "⚠ couldn't fetch from origin — branch list may be stale"
	if !strings.Contains(f.view(), "couldn't fetch from origin") {
		t.Fatal("expected fetch warning in form view")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestBranchHint|TestViewShowsFetchWarning' -v`
Expected: compile failure — `f.branchHint undefined`.

- [ ] **Step 3: Implement `branchHint`, `containsStr`, and render them**

In `internal/ui/newsession.go`, add the helper and method:

```go
// containsStr reports whether s is in xs.
func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// branchHint classifies the typed branch against the cached lists. Local wins
// over remote; base is only relevant when the branch is new.
func (f newSessionForm) branchHint() string {
	if f.branch == "" {
		return ""
	}
	if containsStr(f.localBranches, f.branch) {
		return "existing local branch — base ignored"
	}
	if containsStr(f.remoteBranches, f.branch) {
		return "tracks origin/" + f.branch + " — base ignored"
	}
	return "new branch from " + f.base
}
```

In `view()`, render the hint right after the branch row and the warning before the footer. Replace the rows loop:

```go
	for i, r := range rows {
		line := fmt.Sprintf("%-8s %s", r.label+":", r.val)
		if i == f.field {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
```

with:

```go
	for i, r := range rows {
		line := fmt.Sprintf("%-8s %s", r.label+":", r.val)
		if i == f.field {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
		if i == fieldBranch {
			if h := f.branchHint(); h != "" {
				b.WriteString("  " + dimStyle.Render("         "+h) + "\n")
			}
		}
	}
```

And replace the footer tail:

```go
	b.WriteString("\n" + dimStyle.Render("tab next · enter submit on last field · esc cancel"))
	return b.String()
```

with:

```go
	if f.fetchWarning != "" {
		b.WriteString("\n" + warnStyle.Render(f.fetchWarning) + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("tab next · enter submit on last field · esc cancel"))
	return b.String()
```

Note: `dimStyle`, `selectedStyle`, and `warnStyle` are defined in the `ui` package styles (used in `views.go`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -run 'TestBranchHint|TestViewShowsFetchWarning' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/newsession.go internal/ui/newsession_test.go
git commit -m "feat(ui): show live branch hint and fetch warning in new-session form"
```

---

## Task 6: Wire the actions in `main.go`

**Files:**
- Modify: `main.go`

**Interfaces:**
- Consumes: `git.CLI.ListBranches`, `git.CLI.Fetch` (Task 1); `ui.Actions.Branches`, `ui.Actions.FetchBranches` (Task 4).
- Produces: a fully wired binary.

- [ ] **Step 1: Add the action wiring**

In `main.go`, inside the `ui.Actions{...}` literal, after the `Create` field, add:

```go
		Branches: func(p projects.Project) (git.Branches, error) {
			return g.ListBranches(p.Path)
		},
		FetchBranches: func(p projects.Project) (git.Branches, error) {
			ferr := g.Fetch(p.Path)
			br, lerr := g.ListBranches(p.Path) // best-effort even if fetch failed
			if lerr != nil {
				return git.Branches{}, lerr
			}
			return br, ferr
		},
```

(`git` and `projects` are already imported in `main.go`.)

- [ ] **Step 2: Build the whole module**

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 4: Manual smoke test (optional but recommended)**

With a configured `~/.config/fleet/config.yaml`:
1. `go run .`
2. Press `n`, pick a project with an existing local branch.
3. Type that branch's exact name → hint reads `existing local branch — base ignored`. Submit → session is created on that branch (no "already exists" failure), unless it's checked out elsewhere (then the status line shows `branch "X" is already checked out in another worktree`).
4. Type a branch name that exists only on origin → hint reads `tracks origin/<name> — base ignored`. Submit → a local tracking branch is created.
5. Type a fresh name → hint reads `new branch from <base>`. Submit → new branch as before.

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "feat: wire branch list + fetch actions into the new-session flow"
```

---

## Self-Review

**Spec coverage:**
- Git queries (`LocalBranchExists`, `RemoteBranchExists`, `ListBranches`, `Fetch`) → Task 1.
- Worktree creators (existing local, remote tracking) + new-branch path retained → Task 2.
- Manager resolves case authoritatively + friendly checked-out error → Task 3.
- Async fetch on form open, soft warning + fallback to local refs → Tasks 4 & 6.
- In-memory live hint from cached lists; base ignored for existing branches → Task 5.
- `meta.Base` still stored for PR target → unchanged in `Create` (verified: only the worktree-creation line changes; meta write is untouched).
- Tests for git (real-repo), manager (fakes), UI (pure hint + message handling) → Tasks 1–5.
- Out of scope (branch picker, base-descends-from validation, detach/duplicate) → not built. ✓

**Placeholder scan:** No TBD/TODO; every code step shows full code and exact commands. ✓

**Type consistency:** `Branches{Local,Remote []string}`, `LocalBranchExists`/`RemoteBranchExists(repoPath, branch)`, `ListBranches(repoPath)`, `Fetch(repoPath)`, `AddWorktreeExisting`/`AddWorktreeTracking(repoPath, worktreePath, branch)`, `branchesLoadedMsg{branches}`, `branchesRefreshedMsg{branches, fetchErr}`, `loadBranches`/`fetchBranches(fn, p)`, form fields `localBranches`/`remoteBranches`/`fetchWarning`, `branchHint()`, `containsStr` — names used consistently across all tasks. ✓
