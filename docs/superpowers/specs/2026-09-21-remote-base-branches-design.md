# Fleet — fork new session branches from the remote base

**Date:** 2026-09-21
**Status:** Design — approved, pending implementation plan
**Issue:** none (feature request raised directly)

## Goal

When the new-session form is opened, fleet refreshes the remote-tracking refs
(already done today) **and** new session branches are created from the remote's
base commit rather than a possibly-stale local base branch. A session never
forks from an out-of-date local `main`/`master` — and fleet never mutates the
user's local branch state to make that true.

## Background

When the new-session form opens, `keyProjectPicker` (model.go:266) fires two
commands:

- `loadBranches` — lists refs immediately from `refs/heads` and
  `refs/remotes/origin`.
- `fetchBranches` — runs `git fetch origin`, then re-lists. A failed fetch
  shows a soft warning; the branch list still reflects local refs.

So the remote branch *list* is already refreshed. The gap is that
`git fetch origin` only updates `refs/remotes/origin/*`; it never touches
`refs/heads/main`. When a session is created with a brand-new branch name,
`addWorktreeForBranch` runs `git worktree add -b <new> <path> <base>` with the
**local** `base` ref, so the new branch forks from whatever stale commit the
local default branch happens to be at.

Two families of fix were considered:

- **A — fast-forward the local default branch** after the fetch
  (`git merge --ff-only origin/<default>`). Updates the user's local branch
  ref *and* its working tree, with the usual pull safety rails.
- **B — fork the new worktree from `origin/<base>`** when that remote-tracking
  ref exists, falling back to the local ref otherwise. Never touches the
  user's local branches.

The user chose **B**: it cannot surprise the user about their own repo state.
No amount of `--ff-only` safety makes mutating the user's checkout the right
behaviour for a read-only operation like opening a form.

## Intent (decided during brainstorming)

1. **Scope — the form's `base` field, whatever it is.** Because the new branch
   is forked from `origin/<base>` when that ref exists, *any* base the user
   types benefits, not just the default branch. (The earlier question of
   "just the default branch vs. every branch behind its remote" dissolved: B
   needs no local-branch refresh at all, so there is nothing to scope.)
2. **No local branch mutation, ever.** The repo's local heads are read-only
   from fleet's point of view.
3. **`meta.Base` keeps the user-facing name.** The user chose `main`; the meta
   records `main`, not `origin/main`. Only the fork commit comes from the
   remote ref.
4. **Graceful fallback.** No `origin`, a local-only base, or an empty base
   degrades to today's behaviour (local ref / git's own error).

## Approach

### Rejected alternative: fast-forward the local default branch

`git merge --ff-only origin/<default>` in the repo's main worktree. It is the
"correct" primitive for updating a local branch, but:

- It changes the user's working tree (a real `git pull`), which is surprising
  for a form-open and can conflict with uncommitted work.
- It needs care when the default branch is checked out in a *fleet* worktree
  elsewhere (git refuses the merge there; skipping is right but adds a
  failure mode to reason about).
- It mutates state the user believes they own. Even `--ff-only` is a change
  to their checkout made by a tool they only asked to create a session.

## Design

### `session.Manager.addWorktreeForBranch`

The existing three-way strategy is unchanged:

1. branch exists **locally** → `AddWorktreeExisting` (checkout).
2. branch exists **remotely only** → `AddWorktreeTracking` (track `origin/<b>`).
3. branch is **new** → create from `base`.

Only case 3 changes (manager.go:127). Prefer the remote-tracking ref:

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

- **No git adapter change.** `RemoteBranchExists` (checks
  `refs/remotes/origin/<base>`) and `AddWorktree` already exist;
  `origin/<base>` is a valid base argument to `git worktree add -b`.
- **Error handling** matches the existing existence checks: a real
  `RemoteBranchExists` error (exit > 1 from `show-ref`) propagates; "ref not
  found" (exit 1) is the false → fallback path.
- **Timing.** `RemoteBranchExists` reflects whatever the last fetch wrote. The
  form-open `git fetch origin` populates it; if the fetch has not landed yet
  (submit raced the background fetch) the refs may be from an older fetch, and
  on a fresh clone with no remote they are absent → fallback. All strictly
  better than, or equal to, today.

### New-session form hint

`branchHint` (newsession.go:105) currently ends a new-branch classification
with `new branch from <base>`. When the base appears in the cached remote
list, say so:

```go
if containsStr(f.remoteBranches, f.base) {
    return "new branch from origin/" + f.base
}
return "new branch from " + f.base
```

Keeps the form honest about what it will fork from, matching the "never
surprise the user" rationale. The hint live-updates when the background fetch
lands.

## Testing (TDD)

- **manager** — new-branch case, base has a remote-tracking ref →
  `AddWorktree` called with `origin/<base>`; new-branch case, base has none →
  called with plain `base`; `RemoteBranchExists` error propagates. The three
  existing strategy tests (`TestCreateUsesExistingLocalBranch`,
  `TestCreateTracksRemoteBranch`, `TestCreateNewBranchWhenNeitherExists`) pass
  unchanged because their fakes return no remote base, i.e. the fallback path
  they already assert.
- **ui/newsession** — `branchHint` shows `origin/<base>` when the base is in
  the remote list and the plain wording when it is not; the existing hint
  classifications (existing local / tracking / new) are unchanged.

## Documentation

- `docs/usage.md` — the new-session flow paragraph (line ~139) gains a
  sentence: new branches are created from the latest fetched remote base
  (`origin/<base>`), falling back to the local ref, and fleet never changes
  the repo's local branches.

## Non-goals

- **No local branch refresh** (no fast-forwarding, no `git pull`-style
  mutation of the user's repo).
- **No fetch on submit.** The form-open fetch is the designed freshness point;
  re-fetching at create time is a race-prone extra round-trip for a sub-second
  window of extra freshness.
- **No `--prune` change** to the form-open fetch. Stale deleted-remote refs
  are cosmetic in the hint and out of scope here.
- **No update of `origin/HEAD`.** A repo whose remote default changed is a
  pre-existing `DefaultBranch` accuracy issue, unaffected by this change.

## Assumptions

- The user's local default branch is allowed to be behind its remote; that is
  the exact case this fixes, and B does not require touching it.
- A base that exists both locally and remotely should fork from the remote
  commit. The one case this surprises is a base with **local-only commits**
  the user wanted to build on — recorded in the design, accepted, and visible
  in the form hint.
- `git worktree add -b <new> <path> origin/<base>` succeeds and creates a
  plain (non-tracking) branch at the remote commit, identical to today's
  non-tracking `AddWorktree` behaviour.