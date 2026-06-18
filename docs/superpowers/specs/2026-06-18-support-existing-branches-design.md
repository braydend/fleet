# Support creating sessions for existing branches

**Issue:** [#26](https://github.com/braydend/fleet/issues/26) — allow creating session for existing branches
**Date:** 2026-06-18
**Status:** Design approved

## Problem

`git.CLI.AddWorktree` (`internal/git/git.go`) unconditionally runs:

```
git worktree add -b <branch> <worktreePath> <base>
```

The `-b <branch>` flag means "create a **new** branch named `<branch>` starting at
`<base>`". This produces two bugs reported in #26:

1. **Existing local branch → worktree creation fails.** git errors with
   `fatal: a branch named '<branch>' already exists`, and additionally refuses if
   that branch is already checked out in another worktree.
2. **Existing remote branch → divergent local branch.** Nothing ever inspects
   `origin/<branch>`, so naming a session after a branch that exists only on the
   remote forks a fresh local branch off `base` instead of checking out the
   remote branch.

The new-session form (`internal/ui/newsession.go`) has no notion of an existing
branch, so the user cannot opt into reusing one.

## Goal

When creating a session, fleet should detect whether the chosen branch name is a
new branch, an existing **local** branch, or a **remote-only** branch, and create
the worktree correctly in each case — with live feedback in the form and clear
errors when reuse is impossible.

## Approach

The decision of *which* git command to run lives in the **session manager**
(Approach A). The git adapter gains thin query helpers and one-command-each
worktree creators; the manager resolves the case authoritatively at submit time.
The UI separately caches a branch list to drive an advisory live hint — it is
never authoritative for creation.

Three branch cases map to three git invocations:

| Case | Command |
|------|---------|
| New branch | `git worktree add -b <branch> <wt> <base>` (unchanged) |
| Existing local branch | `git worktree add <wt> <branch>` |
| Remote-only branch | `git worktree add --track -b <branch> <wt> origin/<branch>` |

## Components

### 1. Git adapter — `internal/git/git.go`

Add to the `Git` interface and implement on `CLI`. Each method wraps a single
git command so the adapter stays thin.

**Queries**

- `LocalBranchExists(repoPath, branch string) (bool, error)`
  — `git show-ref --verify --quiet refs/heads/<branch>` (exit 0 → exists, exit 1
  → absent, other → error).
- `RemoteBranchExists(repoPath, branch string) (bool, error)`
  — same against `refs/remotes/origin/<branch>`.
- `ListBranches(repoPath string) (Branches, error)`
  — returns `Branches{Local []string, Remote []string}` (remote names with the
  `origin/` prefix stripped, `HEAD` excluded). Drives the UI hint cache.
- `Fetch(repoPath string) error` — `git fetch origin`.

**New type**

```go
type Branches struct {
    Local  []string
    Remote []string
}
```

**Worktree creation** — the existing `AddWorktree` is retained for the
new-branch case; two siblings are added:

- `AddWorktree(repoPath, worktreePath, branch, base string) error`
  → `worktree add -b <branch> <worktreePath> <base>` (unchanged).
- `AddWorktreeExisting(repoPath, worktreePath, branch string) error`
  → `worktree add <worktreePath> <branch>`.
- `AddWorktreeTracking(repoPath, worktreePath, branch string) error`
  → `worktree add --track -b <branch> <worktreePath> origin/<branch>`.

### 2. Session manager — `internal/session/manager.go`

`Create` resolves the case before touching the worktree:

```
localExists  := git.LocalBranchExists(p.Path, branch)
remoteExists := !localExists && git.RemoteBranchExists(p.Path, branch)

switch {
case localExists:
    err := git.AddWorktreeExisting(p.Path, wt, branch)
    // if err names an "already checked out" failure, wrap with a friendly message
case remoteExists:
    err := git.AddWorktreeTracking(p.Path, wt, branch)
default:
    err := git.AddWorktree(p.Path, wt, branch, base)   // new branch
}
```

The rest of `Create` (ignore-rule, meta write, tmux window) is unchanged.

`meta.json` continues to store the form's `Base` value even when it is ignored
for worktree creation — it remains the correct PR target for `PushPR`.
`meta.Branch` is the branch name as today.

**Checked-out-elsewhere handling:** when `AddWorktreeExisting` fails and the
error indicates the branch is already checked out (git's message contains
`already checked out`), the manager returns a friendly error such as
`branch "<branch>" is already checked out at <path>` so the UI status line shows
something actionable instead of a raw git failure. Creation aborts.

### 3. UI / new-session form — `internal/ui/newsession.go`, `model.go`, `views.go`

**Branch list loading.** When the new-session form opens, the model dispatches a
`tea.Cmd` that:

1. runs `ListBranches(p.Path)` immediately and returns the local-ref view, so the
   hint works instantly (works offline); and
2. concurrently runs `Fetch(p.Path)` then `ListBranches(p.Path)` again, returning
   the refreshed list plus a soft warning if the fetch failed.

This is modelled with messages, e.g. `branchesLoadedMsg{branches git.Branches}`
and `branchesRefreshedMsg{branches git.Branches, fetchErr error}`. The fetch runs
in the background while the user types.

**Form state.** `newSessionForm` gains:

- `localBranches []string`
- `remoteBranches []string`
- `fetchWarning string`

**Live hint (in memory, on each branch-field change).** Classification is a pure
function of the branch text and the cached lists:

- name ∈ `localBranches` → `existing local branch — base ignored`
- else name ∈ `remoteBranches` → `tracks origin/<branch> — base ignored`
- else → `new branch from <base>`

The hint renders under the branch row in `view()`. `fetchWarning`, if set,
renders softly (e.g. a dim line) and never blocks submit.

### 4. Error handling

| Situation | Behaviour |
|-----------|-----------|
| `git fetch` fails on form open | Soft warning in the form; fall back to the already-loaded local refs. Non-fatal. |
| Chosen branch already checked out in another worktree | Manager returns a friendly error; status line shows it; creation aborts. |
| `ListBranches` / existence query errors | Surfaced in the status line; creation aborts (hint simply absent if the open-time load failed). |

## Testing (TDD)

**`internal/git/git_test.go`** — real-repo tests:

- `LocalBranchExists` / `RemoteBranchExists` true/false cases.
- `ListBranches` returns expected local and (origin-stripped) remote names.
- `AddWorktreeExisting` checks out an existing branch into a new worktree.
- `AddWorktreeExisting` fails when the branch is already checked out elsewhere.
- `AddWorktreeTracking` creates a local tracking branch from `origin/<branch>`
  (set up a second repo acting as `origin`).
- `Fetch` against a configured `origin` updates remote-tracking refs.

**`internal/session/manager_test.go`** — `fakeGit` gains the new methods and
flags:

- new-branch name → `AddWorktree` called.
- existing local branch → `AddWorktreeExisting` called.
- remote-only branch → `AddWorktreeTracking` called.
- checked-out-elsewhere → `Create` returns the friendly error.

**`internal/ui`** — the hint classification is pure given cached lists:

- unit-test classification for new / local / remote inputs.
- test that `branchesLoadedMsg` / `branchesRefreshedMsg` update `localBranches`,
  `remoteBranches`, and `fetchWarning`.

## Out of scope (follow-ups)

- A richer **branch picker / autocomplete** in the form (this issue keeps the
  smart free-text field). File as a follow-up.
- Validating that an existing branch descends from `base`.
- Detaching/duplicating a branch that is checked out elsewhere (we block instead).
