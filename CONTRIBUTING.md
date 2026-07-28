# Contributing to fleet

This is the canonical guide for developing `fleet`. It is the single source of
truth for the development workflow; `CLAUDE.md` defers here for those rules and
keeps only the architecture context an agent needs.

## Prerequisites

Install these before building or running. On a clean machine the build fails
until they are present:

- **Go 1.26.4 or newer** — required to build; `go.mod` declares `go 1.26.4`, so
  older toolchains are rejected. A clean machine has no `go` on PATH at all
  (`command not found: go`). Install with `brew install go` (macOS) or from
  <https://go.dev/dl/>. Verify with `go version`.
- **Runtime CLIs on PATH** — `fleet` shells out to these at runtime, so `go run .`
  needs them even though `go build ./...` does not:
  - `git` — worktree and branch operations (usually preinstalled on macOS).
  - `tmux` — the execution backend; **not preinstalled on macOS**, install with
    `brew install tmux`. Without it the unit tests still pass (integration tests
    skip when the binary is absent) but the app cannot create sessions.
  - `gh` — GitHub CLI, used for PR creation (`brew install gh`).
  - `claude` — the Claude Code CLI that each session runs.

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
