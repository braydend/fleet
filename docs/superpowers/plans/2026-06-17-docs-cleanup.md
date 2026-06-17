# Documentation Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure the docs so the README is a short quick-start, a new `docs/usage.md` holds the full user reference, and a new `CONTRIBUTING.md` becomes the canonical development doc that `CLAUDE.md` defers to.

**Architecture:** Pure documentation reorganisation — move the exhaustive user reference out of `README.md` into `docs/usage.md`; move development workflow/conventions into `CONTRIBUTING.md`; trim `CLAUDE.md`'s workflow sections to a pointer. No application code changes.

**Tech Stack:** Markdown only. Verification is by `grep` / file existence / manual link checks, not a test runner.

**Spec:** [`docs/superpowers/specs/2026-06-17-docs-cleanup-design.md`](../specs/2026-06-17-docs-cleanup-design.md)

**Conventions:** See [`CLAUDE.md`](../../../CLAUDE.md). Commits MUST follow Conventional Commits; documentation changes use the `docs:` type (no release bump).

## Global Constraints

- All commit messages use Conventional Commits; these are doc-only changes, so use `docs: ...`.
- No loss of information relative to today's `README.md` — every fact moves somewhere, nothing is dropped.
- The GIF is supplied later by the maintainer; this plan only leaves a documented placeholder (`docs/assets/demo.gif`). Do not fabricate the asset.
- All internal cross-links must resolve to real files at the end.
- No changes to application code or to existing `docs/superpowers/` specs and plans.

---

## File Structure

- Create: `docs/usage.md` — complete user reference (dependencies, install, configuration, keybindings, cleanup menu).
- Create: `CONTRIBUTING.md` — build/test commands, development workflow hard rules, working conventions.
- Modify: `README.md` — slim to description, GIF placeholder, how it works, quick start, short config note, essentials keybindings, status, footer links.
- Modify: `CLAUDE.md` — replace "Development workflow (hard rules)" and "Working conventions" sections with a pointer to `CONTRIBUTING.md`; trim "Build & run" to a pointer.

---

### Task 1: Create `docs/usage.md` (full user reference)

**Files:**
- Create: `docs/usage.md`

**Interfaces:**
- Produces: a file at `docs/usage.md` that the slimmed README (Task 3) and CONTRIBUTING (Task 2) will link to.

- [ ] **Step 1: Write the usage guide**

Create `docs/usage.md` with this content:

```markdown
# fleet usage guide

The complete reference for installing, configuring, and operating `fleet`. For a
quick start see the [README](../README.md).

## Dependencies

Required on your `PATH` at runtime:

| Tool | Purpose |
|------|---------|
| `git` | worktree and branch operations, status queries |
| `tmux` | runs each Claude Code instance in its own session |
| `claude` | the [Claude Code](https://claude.com/claude-code) CLI that each session launches |

Optional:

| Tool | Purpose |
|------|---------|
| `gh` | opening a pull request from the cleanup menu (push still works without it) |

To build from source you also need **Go 1.22+**.

`fleet` checks for `git`, `tmux`, and `claude` at startup and exits with a clear
message if any are missing.

## Install

### Download a release

Prebuilt binaries for Linux and macOS (x64 and ARM) are attached to each
[GitHub Release](https://github.com/braydend/fleet/releases). Download the
archive for your platform, then:

\`\`\`bash
tar -xzf fleet_<version>_<os>_<arch>.tar.gz
sudo mv fleet /usr/local/bin/   # or anywhere on your PATH
fleet --version
\`\`\`

**macOS:** the binaries are unsigned, so Gatekeeper will block the first run.
Clear the quarantine flag once after extracting:

\`\`\`bash
xattr -d com.apple.quarantine ./fleet
\`\`\`

(Alternatively, right-click the binary in Finder and choose **Open** the first
time.)

### Build from source

Clone and build the binary:

\`\`\`bash
git clone https://github.com/braydend/fleet.git
cd fleet
go build -o fleet .
\`\`\`

Then put `fleet` somewhere on your `PATH` (e.g. `~/.local/bin`), or run it from
the build directory. During development you can also run it directly with
`go run .`.

## Configuration

`fleet` reads `~/.config/fleet/config.yaml`.

**First run:** if the file doesn't exist, `fleet` prompts you for the directory
to scan for git projects and writes the config for you (using the default
worktree location). You don't have to create it by hand.

To configure it manually instead, create the file yourself:

\`\`\`yaml
# Directory scanned (one level deep) for git repositories to offer as projects.
scan_root: /home/you/code

# Where session worktrees are created: <worktree_base_dir>/<project>/<session>.
worktree_base_dir: /home/you/.local/share/fleet/worktrees
\`\`\`

- `scan_root` is **required** (the first-run prompt collects it; it must be an
  existing directory).
- `worktree_base_dir` defaults to `~/.local/share/fleet/worktrees` if omitted.

## Keybindings

**Dashboard**

| Key | Action |
|-----|--------|
| `n` | new session (opens the project picker) |
| `Enter` | attach to the selected session |
| `d` | cleanup menu for the selected session |
| `r` | refresh now |
| `↑`/`k`, `↓`/`j` | move selection |
| `q` / `Ctrl-c` | quit |

**Project picker / cleanup menu**: `↑`/`↓` to move, `Enter` to choose, `Esc` to cancel.

**New-session form**: `Tab`/`Shift-Tab` (or `↑`/`↓`) to move between fields, type
to edit, `Enter` to advance / submit on the last field, `Esc` to cancel. The
branch defaults to `fleet/<session>` and the base to the project's default branch.

## Cleanup menu

The cleanup menu (`d` on the dashboard) offers three actions for a session:

- **delete worktree + branch** — kills the tmux session and removes the worktree
  (and branch). If the worktree has uncommitted or unpushed changes, you'll be
  asked to confirm.
- **push / open PR** — pushes the branch and, if `gh` is available, opens a PR.
- **leave** — kills only the tmux session, leaving the worktree and branch for
  you to handle manually.
```

Note: in the steps above, the triple-backtick fences are shown escaped (`\``) only so they nest inside this plan. Write real triple backticks (```` ``` ````) into the actual file.

- [ ] **Step 2: Verify the file exists and contains all reference sections**

Run: `grep -E '^## (Dependencies|Install|Configuration|Keybindings|Cleanup menu)' docs/usage.md`
Expected: five matching lines, one per section.

- [ ] **Step 3: Commit**

```bash
git add docs/usage.md
git commit -m "docs: add full usage reference guide"
```

---

### Task 2: Create `CONTRIBUTING.md` (canonical development doc)

**Files:**
- Create: `CONTRIBUTING.md`

**Interfaces:**
- Consumes: nothing (content is sourced from today's `README.md` Development section and `CLAUDE.md`).
- Produces: `CONTRIBUTING.md` that `README.md` (Task 3) and `CLAUDE.md` (Task 4) will link to.

- [ ] **Step 1: Write the contributing guide**

Create `CONTRIBUTING.md` with this content (write real triple backticks where the fences are escaped as `\``):

```markdown
# Contributing to fleet

This is the canonical guide for developing `fleet`. It is the single source of
truth for the development workflow; `CLAUDE.md` defers here for those rules and
keeps only the architecture context an agent needs.

## Build & run

- Build: `go build ./...` (or `go build -o fleet .`)
- Test: `go test ./...` (git/tmux integration tests skip if those binaries are
  absent). A build-tagged real-CLI smoke test lives in `internal/refresher`:
  `go test -tags smoke -run Smoke ./internal/refresher/`.
- Vet: `go vet ./...`
- Run: `go run .` — requires `git`, `tmux`, and `claude` on PATH. On first run
  (no config file) it prompts for `scan_root` and writes the config; thereafter
  it loads `~/.config/fleet/config.yaml`.

See [`docs/usage.md`](docs/usage.md) for the full configuration reference.

## Development workflow (hard rules)

These rules are **mandatory**, not advisory. They exist so that every change
leaves behind durable design documentation that future agentic workers can read
to understand *why* the code looks the way it does. The artefacts under
`docs/superpowers/` are the authoritative historical record of the project's
design decisions.

### 1. Every feature request follows the spec → plan flow

No feature work begins without documentation. For **every** feature request,
regardless of size:

1. **Brainstorm** the intent and requirements before designing.
2. Write a **design spec** in `docs/superpowers/specs/` named
   `YYYY-MM-DD-<short-name>-design.md`.
3. Write an **implementation plan** in `docs/superpowers/plans/` named
   `YYYY-MM-DD-<short-name>.md`. The plan must reference its spec and these
   conventions, and use checkbox (`- [ ]`) task syntax.

Both a spec **and** a plan are required for every feature — never one without the
other. Match the structure of the existing artefacts in those directories
(Goal / Background / Status header, etc.).

### 2. GitHub issues are a valid entrypoint — and must be linked

A GitHub issue may be the entrypoint for a feature **or** a bug fix. When
addressing an issue:

- Still produce the spec + plan artefacts described above and check them in.
- **Link bidirectionally:** the spec/plan header must cite the issue
  (e.g. `**Issue:** #12`), and a comment must be posted back on the issue
  linking to the committed spec/plan paths. Use `gh` for issue interaction.
- Bug fixes that are genuinely trivial (one-line, no design choice) still need a
  plan documenting the fix and the issue link; a full design spec is at your
  discretion only when there is no design decision to record — when in doubt,
  write both.

### 3. Artefacts are checked in with the work

The spec and plan must be committed within the **same PR** as the implementation
(commit order is flexible). A PR that changes behaviour without its accompanying
documentation is incomplete and should not be merged.

### 4. Commit messages MUST follow Conventional Commits

Releases and version numbers are automated from commit history via
release-please (see
[`docs/superpowers/specs/2026-06-16-release-binaries-design.md`](docs/superpowers/specs/2026-06-16-release-binaries-design.md)).
Versioning therefore depends on every commit following the
[Conventional Commits](https://www.conventionalcommits.org/) format:

- `feat: ...` → minor bump, `fix: ...` → patch bump.
- A `!` after the type (`feat!:`) or a `BREAKING CHANGE:` footer → major bump.
- Other types (`docs:`, `chore:`, `test:`, `refactor:`, `ci:`, etc.) do not
  trigger a release on their own.
- An optional scope is encouraged: `feat(ui): ...`, `fix(tmux): ...`.

A non-conforming commit message silently breaks version inference, so this is
**mandatory** for every commit.

## Working conventions

- **TDD:** write tests before implementation.
- Keep adapters (`tmux`, `git`) thin and behind interfaces; keep domain logic
  free of direct CLI calls.
- Surface errors in the UI status line; never panic on a malformed worktree or
  meta file.
- Destructive actions (delete with uncommitted/unpushed changes) require
  confirmation.
```

- [ ] **Step 2: Verify the file exists and contains all top-level sections**

Run: `grep -E '^## (Build & run|Development workflow|Working conventions)' CONTRIBUTING.md`
Expected: three matching lines.

- [ ] **Step 3: Commit**

```bash
git add CONTRIBUTING.md
git commit -m "docs: add CONTRIBUTING guide as canonical dev doc"
```

---

### Task 3: Slim down `README.md`

**Files:**
- Modify: `README.md` (full rewrite)

**Interfaces:**
- Consumes: `docs/usage.md` (Task 1) and `CONTRIBUTING.md` (Task 2) for footer/cross links.

- [ ] **Step 1: Replace `README.md` with the slim version**

Overwrite `README.md` with this content (write real triple backticks where the fences are escaped as `\``):

```markdown
# fleet

A terminal UI for running and managing multiple isolated [Claude Code](https://claude.com/claude-code)
sessions at once.

Each session runs in its own **git worktree** inside its own **tmux session**, so
sessions are isolated from each other and from your main checkout — and they
survive restarts of the TUI itself. `fleet` gives you one dashboard to create,
attach to, watch, and tear down sessions across all your projects.

![fleet demo](docs/assets/demo.gif)

## How it works

- Pick a project from a list auto-discovered by scanning a root directory for git repos.
- Create a session: `fleet` makes a git worktree on a new branch (you choose the
  base and branch name) and launches `claude` inside a dedicated tmux session.
- The dashboard shows each session's run state, git info (branch, dirty count,
  ahead/behind), project, worktree path, and creation time — refreshed live.
- Attach to drop into the real interactive Claude Code terminal; detach to come back.
- Clean up from the TUI: delete the worktree + branch, push and open a PR, or just
  leave it for manual handling.

State is always derived from the real world (tmux + git) plus a small
per-worktree `.fleet/meta.json` for the few facts they can't infer — there's no
separate database to drift out of sync.

## Quick start

You need `git`, `tmux`, and `claude` on your `PATH` (and `gh` if you want to open
PRs from the cleanup menu).

Install a prebuilt binary from the
[releases page](https://github.com/braydend/fleet/releases), or build from source:

\`\`\`bash
git clone https://github.com/braydend/fleet.git
cd fleet
go build -o fleet .   # or: go run .
\`\`\`

Then run it:

1. Run `fleet`. On first run it prompts for the directory to scan for git
   projects (`scan_root`) and writes `~/.config/fleet/config.yaml` for you.
2. Press `n`, pick a project, name the session, accept or edit the base/branch,
   and submit. A new session appears on the dashboard.
3. Select it and press `Enter` to attach — you're now in a live Claude Code
   session in an isolated worktree. Detach (`<prefix> d`, e.g. `Ctrl-b d`) to
   return to the dashboard.

If a session has exited (shown as `○`), selecting it relaunches Claude Code in
its existing worktree.

See the [usage guide](docs/usage.md) for full install, configuration, and
keybinding details.

## Configuration

`fleet` reads `~/.config/fleet/config.yaml`; on first run it prompts you and
writes the file for you. The two fields are `scan_root` (required — the directory
scanned for projects) and `worktree_base_dir` (where session worktrees are
created; defaults to `~/.local/share/fleet/worktrees`). See the
[usage guide](docs/usage.md#configuration) for the full reference.

## Keybindings

The essentials on the dashboard:

| Key | Action |
|-----|--------|
| `n` | new session (opens the project picker) |
| `Enter` | attach to the selected session |
| `d` | cleanup menu for the selected session |
| `r` | refresh now |
| `q` / `Ctrl-c` | quit |

See the [usage guide](docs/usage.md#keybindings) for the project picker,
new-session form, and cleanup menu keys.

## Status

MVP. Create / attach / kill sessions, live dashboard, and in-TUI cleanup work
across multiple projects. Planned future work is tracked in the
[design doc](docs/superpowers/specs/2026-06-16-fleet-tui-design.md).

## Contributing

Build/test commands, the development workflow, and working conventions live in
[`CONTRIBUTING.md`](CONTRIBUTING.md).
```

- [ ] **Step 2: Verify the README is slim and links resolve**

Run: `wc -l README.md && grep -c 'docs/usage.md' README.md && test -f docs/usage.md && test -f CONTRIBUTING.md && echo links-ok`
Expected: line count well under the original 167; at least one `docs/usage.md` reference; `links-ok` printed.

- [ ] **Step 3: Confirm the removed reference content now lives in usage.md**

Run: `grep -E 'xattr|com.apple.quarantine|worktree_base_dir' docs/usage.md`
Expected: matches found (the detail dropped from the README is present in the usage guide).

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: slim README to quick start with links to usage guide"
```

---

### Task 4: Point `CLAUDE.md` at `CONTRIBUTING.md`

**Files:**
- Modify: `CLAUDE.md` — replace the "Development workflow (hard rules)" and "Working conventions" sections with a pointer; trim "Build & run" to a short pointer.

**Interfaces:**
- Consumes: `CONTRIBUTING.md` (Task 2).

- [ ] **Step 1: Replace the "Build & run" section body with a pointer**

In `CLAUDE.md`, replace the entire "## Build & run" section (the bullet list and config block) with:

```markdown
## Build & run

Build/test/run commands live in [`CONTRIBUTING.md`](CONTRIBUTING.md#build--run);
the full config reference is in [`docs/usage.md`](docs/usage.md#configuration).
In short: `go build ./...`, `go test ./...`, `go run .` (needs `git`, `tmux`,
`claude` on PATH).
```

- [ ] **Step 2: Replace the "Development workflow (hard rules)" section**

In `CLAUDE.md`, replace the entire "## Development workflow (hard rules)" section (including all four numbered subsections) with:

```markdown
## Development workflow (hard rules)

The mandatory development workflow — the brainstorm → spec → plan flow, GitHub
issue linking, artefacts-checked-in-with-the-PR rule, and Conventional Commits
requirement — is documented in [`CONTRIBUTING.md`](CONTRIBUTING.md#development-workflow-hard-rules).
These rules are mandatory for every change; follow them exactly.
```

- [ ] **Step 3: Replace the "Working conventions" section**

In `CLAUDE.md`, replace the entire "## Working conventions" section with:

```markdown
## Working conventions

See [`CONTRIBUTING.md`](CONTRIBUTING.md#working-conventions) for the working
conventions (TDD, thin adapters behind interfaces, errors surfaced in the status
line, confirmation for destructive actions).
```

- [ ] **Step 4: Verify CLAUDE.md no longer duplicates the rules and still has architecture context**

Run: `grep -c 'CONTRIBUTING.md' CLAUDE.md && grep -E '^## (Core design decisions|Planned package layout|MVP scope)' CLAUDE.md`
Expected: at least 3 `CONTRIBUTING.md` references; the architecture sections still present.

- [ ] **Step 5: Confirm the detailed rule text is gone from CLAUDE.md**

The pointer paragraphs legitimately name "Conventional Commits" once as a
reference, so the count is `1`, not `0`. What must be gone is the *detailed*
ruleset — the `feat:`/`fix:`/`BREAKING CHANGE:` bump semantics that now live only
in `CONTRIBUTING.md`.

Run: `grep -E 'feat: \.\.\.|BREAKING CHANGE' CLAUDE.md`
Expected: no matches (the detailed Conventional Commits semantics are no longer
duplicated in CLAUDE.md; only the single pointer reference remains).

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: defer CLAUDE.md workflow sections to CONTRIBUTING.md"
```

---

### Task 5: Final cross-link verification

**Files:**
- None (verification only).

- [ ] **Step 1: Check every internal markdown link target exists**

Run:
```bash
for f in README.md CONTRIBUTING.md CLAUDE.md docs/usage.md; do
  grep -oE '\]\(([^)]+)\)' "$f" | sed -E 's/\]\(|\)//g' | grep -vE '^https?://|^#' | while read -r link; do
    target="${link%%#*}"
    [ -f "$target" ] || echo "BROKEN in $f: $link"
  done
done
echo done
```
Expected: only `done` printed — no `BROKEN` lines.

- [ ] **Step 2: Confirm placeholder GIF path is documented but asset is intentionally absent**

Run: `grep 'docs/assets/demo.gif' README.md && test ! -f docs/assets/demo.gif && echo "placeholder-ok (maintainer supplies gif)"`
Expected: the README line prints and `placeholder-ok ...` prints.

- [ ] **Step 3: No further commit needed**

If Steps 1–2 pass, the work is complete. If any `BROKEN` link appears, fix the
referencing file and commit with `docs: fix internal documentation link`.
```
