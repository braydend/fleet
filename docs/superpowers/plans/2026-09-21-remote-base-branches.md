# Fork new sessions from the remote base Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-09-21-remote-base-branches-design.md`](../specs/2026-09-21-remote-base-branches-design.md)

**Goal:** When the new-session form is opened, remote refs are refreshed (already
done) and new session branches are forked from `origin/<base>` rather than a
possibly-stale local base branch — without ever mutating the user's local branches.

**Architecture:** The only behavioural change is in
`session.Manager.addWorktreeForBranch`: for a *new* branch, prefer
`origin/<base>` as the base argument to `git worktree add -b` when the
remote-tracking ref exists, falling back to the local ref otherwise. The git
adapter is unchanged (`RemoteBranchExists` and `AddWorktree` already exist). A
parallel, cosmetic change makes the form's `branchHint` say
`new branch from origin/<base>` when the base is in the cached remote list, so
the form tells the truth about what it forks from.

**Tech Stack:** Go, Bubble Tea/Lip Gloss (TUI), `git` CLI (thin adapter).

## Global Constraints

- **No new module dependencies.** Everything uses the standard library.
- **TDD.** Write the failing test, watch it fail, then implement. Every task is
  ordered that way; do not reorder.
- **Conventional Commits**, one per task, as given in each task's commit step.
- **Format/vet/lint must stay clean:** `gofmt -l .` prints nothing,
  `go vet ./...` passes, and
  `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags=smoke ./...`
  passes. Run the linter at least once before the final task.
- **No local branch mutation.** fleet's git calls in this feature are read-only
  with respect to the repo's local heads.
- **`meta.Base` stays the user-facing name.** The form's `base` value (`main`,
  `develop`, …) is what gets persisted; only the fork commit uses `origin/`.
- **Comment style:** explain *why*, in full sentences, matching the surrounding
  code. Do not add comments that restate the code.
- Full suite for any task: `go test -race ./...`.

---

## File Structure

**Modified:**
- `internal/session/manager.go` — `addWorktreeForBranch` (lines 106-128): the
  new-branch case prefers `origin/<base>`.
- `internal/session/manager_test.go` — `fakeGit` gains `addedBases []string`
  (records the base argument to `AddWorktree`) and `remoteErrOn string`
  (injects a `RemoteBranchExists` error for one branch name); three new tests.
- `internal/ui/newsession.go` — `branchHint` (lines 105-116): new-branch
  wording prefers `origin/<base>`.
- `internal/ui/newsession_test.go` — two new `branchHint` tests.
- `docs/usage.md` — the new-session form paragraph (line 140) documents the
  fork-from-remote-base behaviour.

**Unchanged:** `internal/git` (adapter and interface), `main.go`, `internal/ui/model.go`.

---

### Task 1: `session.Manager` forks new branches from `origin/<base>`

**Files:**
- Modify: `internal/session/manager.go:106-128` (`addWorktreeForBranch`)
- Modify: `internal/session/manager_test.go:23-53` (fake)
- Test: `internal/session/manager_test.go`

**Interfaces:**
- Consumes: `git.Git.RemoteBranchExists(repoPath, base) (bool, error)`,
  `git.Git.AddWorktree(repoPath, wt, branch, base) error` (existing).
- Produces: `addWorktreeForBranch` forks a new branch from `origin/<base>` when
  `RemoteBranchExists(repoPath, base)` is true; falls back to the local `base`
  otherwise. `AddWorktree`'s base argument is now observable in tests via
  `fakeGit.addedBases`.

- [ ] **Step 1: Extend `fakeGit` to record the base argument and inject errors**

In `internal/session/manager_test.go`, add two fields to `fakeGit` (after
`remoteExists map[string]bool` at line 29):

```go
	addedBases  []string // base argument passed to AddWorktree
	remoteErrOn string   // branch name whose RemoteBranchExists check should error
```

Change the two methods to record / inject (lines 36 and 51):

```go
func (f *fakeGit) AddWorktree(_, wt, _, base string) error {
	f.added = append(f.added, wt)
	f.addedBases = append(f.addedBases, base)
	return nil
}
```

```go
func (f *fakeGit) RemoteBranchExists(_, b string) (bool, error) {
	if f.remoteErrOn != "" && b == f.remoteErrOn {
		return false, errors.New("git show-ref " + b + ": boom")
	}
	return f.remoteExists[b], nil
}
```

The `errors` package is already imported (line 4).

- [ ] **Step 2: Write the failing tests**

Append these three tests to `internal/session/manager_test.go`, after
`TestCreateNewBranchWhenNeitherExists` (line 394):

```go
// A brand-new branch must fork from origin/<base>, not a stale local base.
func TestCreateNewBranchPrefersRemoteBase(t *testing.T) {
	fg := &fakeGit{remoteExists: map[string]bool{"main": true}}
	m, _ := newManager(t, fg, &fakeTmux{})
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}
	if _, err := m.Create(proj, "sess", "brand-new", "main", agent.Lookup(agent.IDClaude)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(fg.addedBases) != 1 || fg.addedBases[0] != "origin/main" {
		t.Fatalf("expected AddWorktree with origin/main, got %v", fg.addedBases)
	}
	if len(fg.addedTracking) != 0 {
		t.Fatalf("a new branch must not take the tracking path: %v", fg.addedTracking)
	}
}

// No remote-tracking ref for the base → fork from the local ref as before.
func TestCreateNewBranchFallsBackToLocalBase(t *testing.T) {
	fg := &fakeGit{}
	m, _ := newManager(t, fg, &fakeTmux{})
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}
	if _, err := m.Create(proj, "sess", "brand-new", "main", agent.Lookup(agent.IDClaude)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(fg.addedBases) != 1 || fg.addedBases[0] != "main" {
		t.Fatalf("expected AddWorktree with plain base, got %v", fg.addedBases)
	}
}

// A real error from the base-ref check must abort creation, like the other
// existence checks do.
func TestCreateNewBranchRemoteBaseCheckErrorPropagates(t *testing.T) {
	fg := &fakeGit{remoteErrOn: "main"}
	m, _ := newManager(t, fg, &fakeTmux{})
	proj := projects.Project{Name: "App", Path: "/code/app", DefaultBranch: "main"}
	if _, err := m.Create(proj, "sess", "brand-new", "main", agent.Lookup(agent.IDClaude)); err == nil {
		t.Fatal("expected a RemoteBranchExists error to abort creation")
	}
}
```

Note: `TestCreateNewBranchPrefersRemoteBase` and the error test both use
branch `brand-new` (no remote ref) with base `main`, so the *base* check —
not the branch-name check — is the one under test.

- [ ] **Step 3: Run the new tests to verify they fail**

Run:
`go test -race ./internal/session/ -run 'TestCreateNewBranchPrefersRemoteBase|TestCreateNewBranchFallsBackToLocalBase|TestCreateNewBranchRemoteBaseCheckErrorPropagates' -v`

Expected:
- `PrefersRemoteBase` FAIL: `addedBases` is empty (the base is never recorded
  today) — the current code calls `AddWorktree` with plain `base`, so
  `addedBases[0] == "main"`, not `"origin/main"`.
- `FallsBackToLocalBase` PASS (already records `main` once the fake records it).
- `RemoteBaseCheckErrorPropagates` FAIL: without the change the base is never
  checked, so `Create` succeeds.

The two failures confirm the missing behaviour.

- [ ] **Step 4: Implement the change**

In `internal/session/manager.go`, replace the final `return` of
`addWorktreeForBranch` (line 127):

```go
	return m.git.AddWorktree(repoPath, wt, branch, base)
```

with:

```go
	// New branch: prefer origin/<base> so the session never forks from a stale
	// local base branch; fall back to the local ref when no remote-tracking ref
	// exists (a local-only base, or a repo with no origin).
	if base != "" {
		if rb, err := m.git.RemoteBranchExists(repoPath, base); err != nil {
			return err
		} else if rb {
			return m.git.AddWorktree(repoPath, wt, branch, "origin/"+base)
		}
	}
	return m.git.AddWorktree(repoPath, wt, branch, base)
```

- [ ] **Step 5: Run the new tests to verify they pass**

Run the same command as Step 3.
Expected: all three PASS.

- [ ] **Step 6: Run the full session package suite**

Run: `go test -race ./internal/session/`
Expected: all tests pass, including the pre-existing strategy tests
(`TestCreateUsesExistingLocalBranch`, `TestCreateTracksRemoteBranch`,
`TestCreateNewBranchWhenNeitherExists`), which exercise the untouched paths.

- [ ] **Step 7: Commit**

```bash
git add internal/session/manager.go internal/session/manager_test.go
git commit -m "feat(session): fork new session branches from the remote base"
```

---

### Task 2: the form hint says `origin/<base>` for a new branch

**Files:**
- Modify: `internal/ui/newsession.go:105-116` (`branchHint`)
- Test: `internal/ui/newsession_test.go`

**Interfaces:**
- Consumes: `newSessionForm.branchHint() string`, the cached
  `remoteBranches []string` (populated by `branchesLoadedMsg` /
  `branchesRefreshedMsg`), and `f.base`.
- Produces: `branchHint` returns `new branch from origin/<base>` when the branch
  is new and `base` is in `remoteBranches`; `new branch from <base>` otherwise.

- [ ] **Step 1: Write the failing tests**

Append to `internal/ui/newsession_test.go`, after `TestBranchHintNewBranch`
(line 17):

```go
// When the base exists on the remote, the hint must say the branch will fork
// from origin/<base>, matching what Manager.Create does.
func TestBranchHintNewBranchPrefersRemoteBase(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, "")
	f.remoteBranches = []string{"main"}
	f.branch = "brand-new"
	if got := f.branchHint(); got != "new branch from origin/main" {
		t.Fatalf("got %q", got)
	}
}

// No remote ref for the base → the plain local-base wording, as before.
func TestBranchHintNewBranchFallsBackToLocalBase(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, "")
	f.branch = "brand-new"
	if got := f.branchHint(); got != "new branch from main" {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test -race ./internal/ui/ -run 'TestBranchHintNewBranchPrefersRemoteBase|TestBranchHintNewBranchFallsBackToLocalBase' -v`
Expected: `PrefersRemoteBase` FAIL (hint says `new branch from main`, no
`origin/` wording); `FallsBackToLocalBase` PASS.

- [ ] **Step 3: Implement the change**

In `internal/ui/newsession.go`, in `branchHint`, replace the final return
(line 115):

```go
	return "new branch from " + f.base
```

with:

```go
	if containsStr(f.remoteBranches, f.base) {
		return "new branch from origin/" + f.base
	}
	return "new branch from " + f.base
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run the same command as Step 2.
Expected: both PASS.

- [ ] **Step 5: Run the full ui package suite**

Run: `go test -race ./internal/ui/`
Expected: all pass, including the pre-existing hint tests
(`TestBranchHintExistingLocal`, `TestBranchHintRemoteOnly`,
`TestBranchHintLocalWinsOverRemote`), which are unaffected: a branch that is
already local or remote hits its classification before the base wording.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/newsession.go internal/ui/newsession_test.go
git commit -m "feat(ui): hint that new branches fork from the remote base"
```

---

### Task 3: document the fork-from-remote-base behaviour

**Files:**
- Modify: `docs/usage.md:140`

**Interfaces:**
- Consumes: nothing; documentation only.

- [ ] **Step 1: Update the new-session form paragraph**

In `docs/usage.md`, the sentence at line 140 currently ends:

> base to the project's default branch.

Extend it with a second sentence describing the fork behaviour:

```markdown
field, `Esc` to cancel. The branch defaults to the sanitized session name and the
base to the project's default branch. New branches fork from the latest fetched
remote base (`origin/<base>`) when it exists, falling back to the local ref;
fleet never rewrites the repo's local branches.
```

- [ ] **Step 2: Verify the suite still passes**

Run: `go test -race ./...`
Expected: all tests pass (no code changed).

- [ ] **Step 3: Format, vet, and lint the whole tree**

Run: `gofmt -l .` → prints nothing.
Run: `go vet ./...` → passes.
Run: `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags=smoke ./...` → passes.

- [ ] **Step 4: Commit**

```bash
git add docs/usage.md
git commit -m "docs: document that new session branches fork from the remote base"
```

---

## Self-Review

**Spec coverage:**

- "New branch prefers `origin/<base>`" → Task 1.
- "Fall back to local ref (local-only base, no origin, empty base)" → Task 1
  (`base != ""` guard; empty base falls through to the plain `AddWorktree`,
  unchanged).
- "`RemoteBranchExists` real error propagates" → Task 1
  (`TestCreateNewBranchRemoteBaseCheckErrorPropagates`).
- "meta.Base keeps the user-facing name" → no code needed; `Create` still
  receives `base` unchanged and persists it. Task 1 leaves `Manager.Create`
  untouched.
- "Form hint says `new branch from origin/<base>`" → Task 2.
- "docs/usage.md documents the behaviour" → Task 3.
- "No local branch mutation" → no git command in any task writes to
  `refs/heads/*`; `RemoteBranchExists` is `git show-ref` (read-only),
  `AddWorktree` creates a new branch.
- "No git adapter change" → Task 1 only calls existing interface methods.

**Placeholder scan:** none — every step contains concrete code or exact run
commands.

**Type consistency:** `addedBases []string` and `remoteErrOn string` are defined
in Task 1 Step 1 and used only in Task 1's tests; `branchHint` output strings in
Task 2 match the exact strings the pre-existing tests assert; `origin/<base>` is
constructed identically in the manager (Task 1) and the hint (Task 2).