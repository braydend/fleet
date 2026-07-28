# CI on Pull Requests — Design

**Date:** 2026-07-28
**Issue:** none (direct request)
**Status:** Approved, pending implementation plan

## Goal

Run fleet's check suite automatically on every pull request so that a
regression, a formatting slip, or a commit message that would break
release-please's version inference is visible on the PR before a human reviews
it.

Today the repository has exactly one workflow — `.github/workflows/release.yml`
(release-please + goreleaser on push to `main`). Nothing runs on a pull
request, so every check documented in
[`CONTRIBUTING.md`](../../../CONTRIBUTING.md#build--run) depends on a
contributor remembering to run it locally.

## Non-goals

- **Branch protection.** The checks are advisory: they report on the PR but do
  not block merging. Making them required is a repository-settings change,
  recorded below for later, deliberately not applied as part of this work.
- **Cross-platform test execution.** CI runs on `ubuntu-latest` only, even
  though `.goreleaser.yaml` ships darwin binaries. Darwin-specific breakage
  (clipboard detection, tmux flag differences) is still only caught at release
  time. Revisit if that bites.
- **Multiple Go versions.** The toolchain comes from `go.mod` and nothing else.
- **Release/publish changes.** `release.yml` is untouched.
- **Automated enforcement of the spec+plan artefact rule** (CONTRIBUTING hard
  rule #3). Considered and rejected: it is noisy for pure refactors, and review
  already covers it.

## What "the CI suite" means

The suite is the one CONTRIBUTING.md already documents, plus two additions:

| Check | Source |
| --- | --- |
| `go build ./...` | CONTRIBUTING.md |
| `go test ./...` (with `-race`) | CONTRIBUTING.md |
| `go vet ./...` | CONTRIBUTING.md |
| `go test -tags smoke -run Smoke ./...` | CONTRIBUTING.md |
| `gofmt -l .` | new |
| `golangci-lint run` | new |
| Conventional Commits validation | new — enforces CONTRIBUTING hard rule #4 |

## Architecture

One new file, `.github/workflows/ci.yml`, containing three independent jobs.

### Triggers

```yaml
on:
  pull_request:
  push:
    branches: [main]
```

`pull_request` is the requirement. `push` to `main` is included for two
reasons: it gives `main` its own health signal (direct pushes and bad merge
resolutions are otherwise invisible until the next PR), and GitHub Actions
caches written on a PR branch cannot be read by sibling PR branches — only
default-branch caches are shared. Without the `push` trigger, most PRs would
start with a cold module and build cache.

`concurrency: ci-${{ github.ref }}` with `cancel-in-progress: true`, mirroring
the `release-${{ github.ref }}` group already used in `release.yml`. Superseded
runs are pointless work.

Top-level `permissions: contents: read`; the `commits` job additionally needs
`pull-requests: read`.

No `paths-ignore` filter. Because the checks are advisory, the usual footgun
(a required check that never reports leaves the PR permanently unmergeable)
does not apply — but docs-only runs are cheap, and omitting the filter keeps
the option open to make the checks required later without introducing that
trap.

### Job layout

Three jobs rather than one, so that a `gofmt` slip does not hide whether the
tests pass. Each job re-does checkout and Go setup (~20–30s), which buys
parallelism and three independent status lines on the PR.

All jobs run on `ubuntu-latest` with `actions/checkout@v4` and
`actions/setup-go@v5` using `go-version-file: go.mod`. Module and build caching
is on by default in setup-go v5, so no explicit `actions/cache` step is needed.

#### Job `lint`

1. `gofmt -l .` — fails printing the offending file list. `go vet` does not
   cover formatting, and the tree is currently clean, so this locks in the
   status quo at zero cost.
2. `go vet ./...`
3. `golangci/golangci-lint-action@v9` with `version: v2.12` and
   `args: --build-tags=smoke`.

The action major is pinned to v9 (which requires golangci-lint ≥ v2.1) and the
linter to the `v2.12` minor line, so patch releases are picked up but a minor
bump — which can introduce new findings — is an explicit commit.

No `.golangci.yml`. The default linter set (errcheck, govet, ineffassign,
staticcheck, unused) is the right starting point; a config file that only
restates defaults invites bikeshedding over which linters to add. If a specific
linter is wanted later, that is a separate change with its own justification.

`--build-tags=smoke` is passed so the two `//go:build smoke` test files are
linted too. Without it they are invisible to the linter, which is how two of the
nine findings below went unnoticed.

#### Job `test`

1. Ensure tmux is present: `command -v tmux || (sudo apt-get update && sudo
   apt-get install -y tmux)`. Conditional so the apt cost is only paid if the
   runner image ever stops shipping tmux.
2. Assert `git` and `tmux` are on PATH, hard-failing if not, and echo
   `git --version` / `tmux -V` into the log.
3. `go build ./...`
4. `go test -race ./...`
5. `go test -race -tags smoke -run Smoke ./...`

The assertion in step 2 is the load-bearing part. `internal/git` and
`internal/tmux` call `t.Skip` when their binary is absent, and the smoke tests
do the same. That is correct behaviour for a developer machine, but in CI it
means a runner-image change would silently convert real integration coverage
into a green tick with nothing to indicate the loss. The assertion turns that
into a loud failure.

Smoke tests are selected with `-run Smoke ./...` rather than by naming the two
packages, so a smoke test added anywhere later is picked up without editing the
workflow. The `-race` flag is applied to both test invocations; the suite is
fast enough (~1.5s per package) that there is no reason to run a non-race pass
as well.

Note that `claude` is **not** needed — neither smoke test shells out to it.

#### Job `commits`

`actions/checkout@v4` with `fetch-depth: 0`, then
`wagoid/commitlint-github-action@v6`.

Gated on `if: github.event_name == 'pull_request'`. Commit messages are
immutable once merged, so re-validating them on `main` can only produce a
failure nobody can act on.

Validation covers **every commit in the PR**, not the PR title. This repository
merges with merge commits (`Merge pull request #NN from …`), so each individual
commit lands on `main` and is parsed by release-please. A PR-title check —
correct for a squash-merge repository — would protect nothing here. commitlint
ignores merge commits by default — though at PR time no merge commit exists
yet, so that only matters if the trigger ever widens beyond `pull_request`.

`fetch-depth: 0` is belt-and-braces: for `pull_request` events the action
reads commit messages from the GitHub API (`pulls.listCommits`) rather than
local history, so a shallow checkout would suffice — but a full fetch is
negligible on a repo this size and keeps the job correct if the trigger ever
widens to `push`, where local history is used.

### Configuration file

`.commitlintrc.yml` at the repository root:

```yaml
extends:
  - "@commitlint/config-conventional"
rules:
  body-max-line-length: [0, always, 100]
```

(Level `0` disables the rule; the remaining tuple members are inert but keep the
value a valid three-element rule config — YAML has no `Infinity` literal, which
is the idiom a JS config would use here.)

The action falls back to `@commitlint/config-conventional` when no config file
exists, so this file is not strictly required — but it makes the ruleset
explicit and reviewable in-repo, and it disables `body-max-line-length`. That
default (100 chars) fails on long URLs pasted into a commit body, which is
friction with no upside. `type-enum` is left at its default, which already
covers every type CONTRIBUTING.md names (`feat`, `fix`, `docs`, `chore`,
`test`, `refactor`, `ci`), as are `header-max-length` (100) and the
subject-format rules.

## Pre-existing lint findings

`golangci-lint v2.12.2` with `--build-tags=smoke` reports nine issues, all
`errcheck`, all mechanical. They are fixed as part of this change so CI is
green from the first run:

| File | Issue |
| --- | --- |
| `internal/config/setup.go:40,41,61` | unchecked `fmt.Fprintf` / `fmt.Fprint` |
| `internal/git/git.go:132` | unchecked `defer f.Close()` |
| `internal/selfupdate/apply.go:90` | unchecked `defer resp.Body.Close()` |
| `internal/selfupdate/check.go:64` | unchecked `defer resp.Body.Close()` |
| `internal/selfupdate/extract.go:19` | unchecked `defer gr.Close()` |
| `internal/selfupdate/smoke_test.go:41,42` | unchecked `w.Write` |

Eight are resolved with an explicit `_ =` discard and change no behaviour: they
are writes to a terminal writer, writes to an `httptest` response writer, and
deferred closes of read-only handles, where a returned error has no recovery
path. Being explicit documents that the discard is deliberate.

The ninth is not a discard. `internal/git/git.go:132` is a deferred `Close()` on
a handle opened `O_APPEND|O_CREATE|O_WRONLY`, and on a write handle the close is
what flushes — so a failure to append the pattern to `info/exclude` was being
swallowed. That one is fixed properly: write, close explicitly, and return the
close error when the write succeeded. It is a real (if minor) bug fix that the
linter surfaced, which is a reasonable advertisement for adding the linter.

`gofmt` and `go vet` are already clean, and `go test -race ./...` plus both
smoke tests pass locally under `TERM=dumb` with stdin closed — confirming the
Bubble Tea and Lip Gloss tests do not depend on a TTY and will behave the same
on a runner.

## Documentation changes

`CONTRIBUTING.md#build--run` gains the two commands it does not yet mention —
`golangci-lint run` and the race flag — so the local loop matches CI exactly. A
developer should be able to reproduce any CI failure without reading the
workflow file.

## Testing strategy

A workflow cannot be unit-tested, so verification is layered:

1. **Every command is run locally first**, on this branch, in a CI-like
   environment (`TERM=dumb`, stdin closed). Already done for `gofmt`, `go vet`,
   `golangci-lint`, `go test -race`, and the smoke tests.
2. **The workflow is verified on a real PR.** The branch is pushed, the PR
   opened, and all three checks are confirmed to appear and pass before the work
   is called complete. A workflow that has never executed is unverified.
3. **The two guard steps are confirmed to fail when they should**, not just to
   pass. Both are shell snippets, so both are exercised locally: the tmux
   assertion is run with a `PATH` that excludes tmux and must exit non-zero, and
   the `gofmt` step is run against a deliberately misformatted temporary file
   and must exit non-zero naming that file. A guard that has only ever been
   observed passing is not known to be a guard.

## Making the checks required (deferred)

Left unapplied per the decision above. When the checks have proven themselves on
real PRs, the three contexts to require are `lint`, `test`, and `commits`:

```sh
gh api -X PUT repos/braydend/fleet/branches/main/protection/required_status_checks \
  -F strict=false \
  -f 'contexts[]=lint' -f 'contexts[]=test' -f 'contexts[]=commits'
```

Context names must match the job names exactly; a mismatch leaves `main`
unmergeable. Note that release-please's own PRs must be able to pass these
checks — they contain a single `chore(main): release x.y.z` commit, which is
valid Conventional Commits, and they touch only `CHANGELOG.md` and the version
manifest, so `lint` and `test` pass unchanged.

## Risks

- **golangci-lint minor bumps introduce findings.** Mitigated by pinning to the
  `v2.12` line; a bump is then a deliberate commit that carries any fixes with
  it.
- **The default linter set is a judgement call.** If errcheck proves noisy in
  practice, the response is a `.golangci.yml` with a justified exclusion, not
  disabling the job.
- **commitlint rejects a legitimate message.** The ruleset is in-repo and
  editable; `body-max-line-length` is already disabled as the most likely
  offender.
- **A third dependency on external actions** (`golangci-lint-action`,
  `commitlint-github-action`) alongside the two `release.yml` already uses. Both
  are pinned to a major tag, consistent with the existing convention in this
  repository.
