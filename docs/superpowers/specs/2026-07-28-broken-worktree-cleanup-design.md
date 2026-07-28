# Fleet — cleaning up broken worktrees

**Date:** 2026-07-28
**Status:** Design — approved, pending implementation plan
**Relates to:** [`2026-06-16-fleet-tui-design.md`](2026-06-16-fleet-tui-design.md) (state management approach C)

## Goal

Make session cleanup survive a worktree whose files fleet cannot delete, so a
failed delete never strands an un-removable session on the dashboard. Surface
worktrees that git no longer recognises as a distinct **broken** state with a
cleanup action that always succeeds in removing the *session*, reporting any
files left on disk rather than failing.

## Background

### The observed failure

Deleting a session reports:

```
error: git worktree remove /home/bray/.local/share/fleet/worktrees/ascension/mago_consolidation --force
```

This is the **second** failure, not the first. Reproduced end-to-end:

1. A session runs Docker, which bind-mounts the worktree and writes files as
   **root** — `vendor/`, `.phpunit.cache`, `.php_cs.cache`. The real
   `ascension/mago_consolidation` has 73,359 root-owned files.
2. `session.Manager.Delete` runs `git worktree remove <path> --force`
   (`internal/git/git.go:67`).
3. git unlinks the worktree's `.git` file and its `.git/worktrees/<id>` admin
   entry **first**, then recursively deletes the tree. The delete hits `EACCES`
   on the root-owned subtree and aborts:
   `error: failed to delete '<path>': Permission denied` (exit 255).
4. The registration is gone; the directory — including `.fleet/meta.json` —
   survives.
5. `refresher.Build` (`internal/refresher/refresher.go:59`) discovers sessions
   by walking `worktree_base_dir` for `.fleet/meta.json`. It never asks git, so
   the dead session still renders on the dashboard.
6. Every retry now fails differently: `fatal: '<path>' is not a working tree`
   (exit 128) — the reported message.
7. `Delete` returns early on that error, so meta, the directory and the branch
   are never cleaned up. The session is permanently stuck.

The root cause is not the permission error itself — it is that
`git worktree remove` destroys the registration **before** the tree, so a
partial failure leaves a state fleet has no way to recover from.

### Damage at time of writing

| Worktree | State |
|---|---|
| `ascension/mag_make_fix` | orphaned, 78,891 root-owned files — stuck on dashboard |
| `ascension/mago_consolidation` | orphaned, 73,359 root-owned files — stuck on dashboard |
| `ascension/mago_raise_bar` | orphaned, 73,359 root-owned files — stuck on dashboard |
| `ascension/mago_144` | orphaned, no meta — invisible to fleet, dead disk weight |
| `ascension/mago_check_changed_files` | live, root-owned files present — will hit the same wall |

Branches `mag_make_fix`, `mago-track-a-consolidation` and
`mago-track-c-raise-safety-bar` also survive, because `DeleteBranch` never ran.

## Decisions

Settled during brainstorming:

| Decision | Choice | Why |
|----------|--------|-----|
| Undeletable files | **Best-effort + forget** — delete what is deletable, drop the session from fleet regardless, report the leftover path | The session always disappears from the dashboard. Chosen over refusing with a `sudo` instruction (leaves the session stuck) and over escalating to `sudo` from the TUI (fleet stays unprivileged). |
| Removal mechanism | **fleet deletes the tree itself, then `git worktree prune`** — never `git worktree remove` | Inverts the destructive order: a permission failure can no longer orphan a registration, and prune reconciles whatever state the registry is in. Makes `Delete` idempotent for free. |
| Broken detection | **`.git` entry missing from the worktree directory** | The marker git itself uses for a linked worktree. A single `stat` — no subprocess on the refresh tick. Exactly identifies today's orphans. |
| Broken representation | **`Session.Broken bool`**, not a new `activity.State` | Activity is what Claude is doing; broken is worktree integrity. Keeps the activity classifier and its tests untouched. |
| Broken cleanup UX | **Cleanup menu collapses to a single "clean up" choice** | Push/PR and leave are meaningless without a worktree. |
| Leftover reporting | **Status line notice** carrying the path and blocked-file count | The user needs the `sudo rm -rf` target; a silent partial delete would hide disk residue. |

## Non-goals

- **No `sudo` escalation.** fleet never runs privileged commands.
- **No prevention** of root-owned files (container user-namespace mapping, etc.)
  — that is the individual project's Docker configuration, not fleet's business.
- **No change to the create/attach/push/leave paths.**
- **No retry/backoff.** A blocked file is blocked; report and move on.

## Design

### 1. `internal/cleanup` — best-effort tree removal (new package)

`os.RemoveAll` stops at the first error, which is exactly wrong here: one
root-owned `vendor/` would abandon the other 90% of the tree. A new package owns
the filesystem walk, with no git knowledge:

```go
// Removal reports what RemoveTree could not delete.
type Removal struct {
	BlockedCount int      // total entries that could not be removed
	Blocked      []string // up to maxBlockedSamples example paths, for the message
}

func (r Removal) Complete() bool { return r.BlockedCount == 0 }

// RemoveTree deletes root and everything under it, continuing past entries it
// cannot remove rather than stopping at the first error.
func RemoveTree(root string) (Removal, error)
```

Post-order recursion: recurse into directories, remove entries, and only remove
a directory once all its children are gone. Any failure records the path and
continues; a directory with surviving children is itself left in place.
Symlinks are unlinked, never followed (`fs.DirEntry.IsDir()` is `Lstat`-based,
so a symlink to a directory is not treated as one). A missing `root` is success,
not an error — that is what makes cleanup idempotent.

`error` is reserved for a genuinely unreadable root; ordinary permission
failures land in `Removal`, not the error.

### 2. `internal/git` — prune and worktree detection

`RemoveWorktree` leaves the `Git` interface entirely (nothing will call
`git worktree remove` any more). Two methods replace it:

```go
// PruneWorktrees drops registry entries whose directories are gone.
PruneWorktrees(repoPath string) error   // git worktree prune

// IsWorktree reports whether path is still a linked git worktree.
IsWorktree(path string) bool            // stat <path>/.git
```

`git worktree prune` reconciles the registry regardless of how it got out of
sync, which covers both the happy path (we just deleted the tree) and today's
orphans (registration already gone — prune is a no-op).

> **Caveat, accepted:** prune is repo-wide, so it also drops registrations for
> any *other* worktree of that repo whose directory is currently absent — for
> example one living on an unmounted drive. This matches what `git gc` already
> does routinely, and the alternative (hand-editing `.git/worktrees/`) means
> reaching into git internals.

### 3. `internal/session` — resilient `Delete`

`Delete` gains a structured result and a new order of operations:

```go
// DeleteResult reports what a Delete left behind.
type DeleteResult struct {
	Leftover      string   // worktree path if files survived; "" when fully clean
	LeftoverCount int
	Samples       []string
}

func (m *Manager) Delete(s Session, deleteBranch bool) (DeleteResult, error)
```

1. Kill the tmux window (ignore error — it may already be gone).
2. `m.removeTree(s.WorktreePath)` — best-effort, injected like `clock` so
   manager tests can fake it (defaults to `cleanup.RemoveTree`).
3. **Ensure forgotten:** if the directory survived, explicitly remove
   `<wt>/.git` and `<wt>/.fleet`. If `.fleet/meta.json` still exists afterwards,
   return a hard error — fleet would otherwise keep showing the session, which
   is the exact trap this change exists to remove.
4. `m.git.PruneWorktrees(s.RepoPath)`.
5. `m.git.DeleteBranch(...)` when requested. After the prune, git no longer
   believes the branch is checked out, so this now succeeds where it previously
   never ran.
6. Return the leftover report.

Steps 3–5 run even when step 2 was partial. That is the whole point: the
*session* is always cleaned up; only *files* may be left behind.

### 4. `internal/refresher` — the broken state

`Session` gains `Broken bool`. In `Build`, after reading meta:

```go
broken := !g.IsWorktree(wt)
```

When broken, skip the `g.Status(wt)` subprocess (it can only fail) and use
`git.Status{Branch: md.Branch}` directly. Activity classification is unchanged —
a broken worktree may still have a live tmux window, and the view gives `Broken`
display precedence anyway.

### 5. `internal/ui` — display and cleanup flow

**Dashboard.** A broken session renders `🚫` in place of the activity icon, and
its detail line becomes `broken · worktree missing from git · d to clean up`
instead of the activity/git/age detail. The legend gains `🚫 broken`.

**Cleanup menu, data-driven.** The menu becomes a list built from the session:

```go
func cleanupOptions(s session.Session) []cleanupOption
```

- healthy → today's three choices, unchanged;
- broken → a single `🧹 clean up (remove broken session)`.

`Model.cleanupChoice` becomes `cleanupIndex int` (an index into that slice) and
`enter` dispatches on `opts[m.cleanupIndex].choice`, so the enum keeps its
meaning as action identity while the menu length varies. Cursor clamping follows
`len(opts)`. A broken session skips the dirty/ahead confirmation — its git
status is unknowable, so there is nothing to warn about.

**Reporting.** `Actions.Delete` becomes
`func(session.Session, bool) (session.DeleteResult, error)`. `sessionsUpdatedMsg`
gains a `notice string`, set by `Update` when non-empty; `runThenRefresh` takes
`func() (string, error)` so a single round-trip carries both the refreshed list
and the message. Periodic refreshes send an empty notice, so they never clobber
the status line.

- clean: `✓ deleted ascension/mago_consolidation`
- partial: `⚠ cleaned up ascension/mago_consolidation — 73359 files left at <path> (sudo rm -rf to reclaim)`

### Data flow

Unchanged in shape: tmux + git stay authoritative for live state, meta stays
per-worktree. The change is that `.fleet/meta.json` removal is now the
*guaranteed* step of a delete, and the git registry is reconciled by prune
rather than assumed consistent.

## Testing

TDD throughout. `cleanup` and `git` are testable against real temp
directories/repos; `session` and `ui` use fakes.

- **`cleanup.RemoveTree`:** removes a whole tree; a nested unreadable directory
  (`chmod 0555`) leaves that subtree and reports it in `Blocked`/`BlockedCount`
  while everything else is gone; a missing root is success; a symlink to a
  directory outside the tree is unlinked without touching its target. Tests must
  restore permissions in `t.Cleanup` so the temp dir can be torn down.
- **`git.IsWorktree`:** true for a real linked worktree, false once `.git` is
  removed, false for a plain directory.
- **`git.PruneWorktrees`:** after deleting a worktree directory, prune removes it
  from `git worktree list`; running prune twice is a no-op.
- **`session.Manager.Delete`:** with a fake `removeTree` reporting blocked files,
  it still prunes, still deletes the branch, and returns a populated
  `DeleteResult`; with meta surviving, it returns an error; a fully clean removal
  reports `Leftover == ""`; deleting an already-gone worktree succeeds
  (idempotent).
- **`ui`:** a broken session renders `🚫` and the broken detail line; its cleanup
  menu shows exactly one option; `enter` on it dispatches delete without the
  confirm screen; a partial `DeleteResult` puts the leftover path and count in
  the status line.
- Existing `internal/ui`, `internal/refresher` and `internal/session` tests keep
  passing (signature updates aside).

## Risks

- **Deleting more than git would have.** fleet now removes the tree itself
  rather than delegating, so a bug here deletes user files. Mitigated by a hard
  guard in `Delete` that refuses any path not strictly under
  `cfg.WorktreeBaseDir` (so a corrupt meta can never point the walk at a real
  repo), the existing confirmation flow, and direct tests of the walk.
- **Repo-wide prune.** Documented above; accepted.
- **`.git` stat as the broken oracle.** A re-cloned repo leaves a `.git` file
  pointing at a dead gitdir, which this check calls healthy. Consequence is
  cosmetic only — the session shows normally and `Delete` still works, because
  correctness now comes from `Delete` being robust rather than from detection.

## Future scope (not now)

- **Untracked orphan directories.** `ascension/mago_144` has neither meta nor
  `.git`, so fleet cannot see it. Surfacing directories that lack *both* would
  let fleet reclaim them — and the "lacks both" test is what safely distinguishes
  them from healthy-but-untracked worktrees like
  `ascension/mago_unused_definitions_fixes`, which have `.git` and must never be
  offered for cleanup.
- A bulk "clean all broken sessions" action.
- Warning at session-create time when a project's worktree base already contains
  root-owned residue.
