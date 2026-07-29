# CI on Pull Requests Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run fleet's check suite automatically on every pull request, so regressions, formatting slips, and commit messages that would break release-please's version inference are visible before review.

**Architecture:** One new workflow file, `.github/workflows/ci.yml`, with three independent jobs — `lint` (gofmt + go vet + golangci-lint), `test` (build + race tests + smoke tests, guarded by an assertion that `git`/`tmux` are present), and `commits` (Conventional Commits validation over every commit in the PR). Nine pre-existing `errcheck` findings are fixed first so the first CI run is green. The checks are advisory: no branch-protection changes are made.

**Tech Stack:** GitHub Actions, Go 1.26 toolchain from `go.mod`, `golangci-lint` v2.12, `commitlint` via `@commitlint/config-conventional`. No new Go module dependencies.

**Spec:** [`docs/superpowers/specs/2026-07-28-ci-pull-requests-design.md`](../specs/2026-07-28-ci-pull-requests-design.md)

## Global Constraints

- **No GitHub issue.** This was a direct request, so CONTRIBUTING hard rule #2 (issue linking) does not apply. Rule #3 does: the spec and this plan ship in the **same PR** as the implementation. The spec is already committed (`972e9c9`).
- **Conventional Commits, mandatory.** Each task below specifies its exact commit message. None of these commits should trigger a release: use `refactor:`, `ci:`, and `docs:` only — never `feat:` or `fix:`.
- **Pinned action versions, exactly these:** `actions/checkout@v4`, `actions/setup-go@v5`, `golangci/golangci-lint-action@v9` (with `version: v2.12.2`, matching the exact patch pin `CONTRIBUTING.md`'s local fallback command uses), `wagoid/commitlint-github-action@v6` (with `configFile: .commitlintrc.yml`).
- **Job names are load-bearing:** `lint`, `test`, `commits`. They are the status-check contexts recorded in the spec for future branch protection. Do not rename them.
- **Do not modify `.github/workflows/release.yml`** and do not change any repository setting, including branch protection. The checks are advisory by explicit decision.
- **Runner is `ubuntu-latest` only.** No OS matrix, no Go version matrix — the toolchain comes from `go-version-file: go.mod`.
- **No `.golangci.yml`.** Default linters are the chosen set. If a finding seems wrong, fix the code; do not silence the linter.
- **Repo:** `braydend/fleet`. **Go module path:** `github.com/bray/fleet`.
- **Toolchain note:** `go` is on the Homebrew path. If `go` is not found, run `export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"` first.
- **Evidence before claims.** Every "Expected:" line below is something you must actually observe in the terminal. Do not mark a step complete from inference.

---

### Task 1: Fix the nine pre-existing errcheck findings

`golangci-lint` reports nine unchecked-error issues. They must be fixed before the workflow lands, or the first CI run is red for reasons unrelated to the change.

Eight are genuine "nothing to do with this error" cases, resolved with an explicit `_ =` discard that documents the deliberate choice. The ninth (`internal/git/git.go`) is **not** — it is a deferred `Close()` on a *write* handle, where the close is what flushes and can report a write failure. That one gets a real fix, not a discard.

**Files:**
- Modify: `internal/config/setup.go:40-41`, `internal/config/setup.go:61`
- Modify: `internal/git/git.go:172-180`
- Modify: `internal/selfupdate/apply.go:90`
- Modify: `internal/selfupdate/check.go:64`
- Modify: `internal/selfupdate/extract.go:19`
- Modify: `internal/selfupdate/smoke_test.go:41-42`

**Interfaces:**
- Consumes: nothing.
- Produces: no signature changes. `func (c *CLI) Ignore(worktreePath, pattern string) error` in `internal/git/git.go:152` (the function containing line 180) keeps its signature and existing call sites; it now also reports a `Close` failure that it previously swallowed.

- [ ] **Step 1: Observe the current failures**

Run:

```bash
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags=smoke ./...
```

Expected: exit status 1. Against current `main`, ten findings exist: nine `errcheck` findings (the ones this task fixes, matching the six files above) plus one `staticcheck` `SA4000` finding in `internal/naming/sessionid_test.go` — see the spec's "Pre-existing lint findings" section for why that one is separate from this task and how it was fixed. Output ends with `10 issues:`, `* errcheck: 9`, and `* staticcheck: 1`. If the counts or the file list differ from that, stop and reconcile with the spec before changing code — new findings mean the tree moved.

- [ ] **Step 2: Fix the three `fmt.Fprint*` calls in `internal/config/setup.go`**

These write prompts to the terminal during first-run setup. If the terminal is gone, there is no recovery path and no useful place to report it.

```go
	_, _ = fmt.Fprintf(out, "No config found at %s.\n", path)
	_, _ = fmt.Fprint(out, "Enter the directory to scan for git projects (scan_root): ")
```

and, further down in the same function:

```go
	_, _ = fmt.Fprintf(out, "Wrote config to %s\n", path)
```

- [ ] **Step 3: Fix the write-handle close in `internal/git/git.go`**

Current code (the tail of the function that appends a pattern to the repo's `info/exclude`):

```go
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(pattern + "\n")
	return err
```

Replace the `defer` with an explicit close, so a failure to flush is reported instead of discarded:

```go
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(pattern + "\n"); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
```

Note the ordering: on a write error, the close still happens (discarding its error, since the write error is the more informative one), and the write error is returned. On success, the close error becomes the return value.

- [ ] **Step 4: Fix the three read-side deferred closes**

In `internal/selfupdate/apply.go` and `internal/selfupdate/check.go`, both currently `defer resp.Body.Close()`:

```go
	defer func() { _ = resp.Body.Close() }()
```

In `internal/selfupdate/extract.go`, currently `defer gr.Close()` on the gzip reader:

```go
	defer func() { _ = gr.Close() }()
```

These are read-only handles; a close error carries no information the caller can act on.

- [ ] **Step 5: Fix the two `w.Write` calls in `internal/selfupdate/smoke_test.go`**

These are `httptest` handlers whose writes cannot meaningfully fail:

```go
	mux.HandleFunc("/archive", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tgz) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(checksums)) })
```

- [ ] **Step 6: Verify lint is now clean**

Run:

```bash
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags=smoke ./...
```

Expected: exit status 0, no output.

- [ ] **Step 7: Verify nothing broke**

Run:

```bash
gofmt -l . && go vet ./... && go test -race ./... && go test -race -tags smoke -run Smoke ./...
```

Expected: `gofmt -l .` prints nothing; every package reports `ok`. Packages with no smoke test report `ok ... [no tests to run]` — that is success, not a skip.

- [ ] **Step 8: Commit**

```bash
git add internal/config/setup.go internal/git/git.go internal/selfupdate/apply.go internal/selfupdate/check.go internal/selfupdate/extract.go internal/selfupdate/smoke_test.go
git commit -m "refactor: check or explicitly discard ignored error returns

Clears the nine errcheck findings that would otherwise fail the new CI
lint job on its first run. Eight are deliberate discards on terminal
writes, httptest writes, and read-side closes. The ninth is a real fix:
internal/git deferred Close on a write handle, so a failure to flush the
info/exclude append was silently swallowed; it is now returned."
```

---

### Task 2: Add the `lint` and `test` jobs

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: the green lint state from Task 1.
- Produces: a workflow named `ci` with jobs `lint` and `test`. Task 3 appends a third job, `commits`, to the same file and does not modify these two.

- [ ] **Step 1: Create the workflow file**

Create `.github/workflows/ci.yml` with exactly this content:

```yaml
name: ci

on:
  pull_request:
  push:
    branches:
      - main

permissions:
  contents: read

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: gofmt
        run: |
          unformatted=$(gofmt -l .)
          if [ -n "$unformatted" ]; then
            echo "::error::These files are not gofmt'd:"
            echo "$unformatted"
            exit 1
          fi
      - name: go vet
        run: go vet ./...
      - uses: golangci/golangci-lint-action@v9
        with:
          version: v2.12.2
          args: --build-tags=smoke

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Ensure tmux is installed
        run: command -v tmux >/dev/null || (sudo apt-get update && sudo apt-get install -y tmux)
      - name: Assert integration test dependencies
        run: |
          for bin in git tmux; do
            if ! command -v "$bin" >/dev/null; then
              echo "::error::$bin is not on PATH — the integration tests would silently skip"
              exit 1
            fi
          done
          git --version
          tmux -V
      - name: Build
        run: go build ./...
      - name: Test
        run: go test -race ./...
      - name: Smoke tests
        run: go test -race -tags smoke -run Smoke ./...
```

Points worth understanding rather than copying blindly:

- `concurrency` mirrors the `release-${{ github.ref }}` group already in `release.yml`. Superseded runs are cancelled.
- `setup-go@v5` enables module and build caching by default — no `actions/cache` step is needed, and adding one would fight it.
- `--build-tags=smoke` makes the linter see the two `//go:build smoke` files. Two of the nine findings in Task 1 were only visible with it.
- The "Assert integration test dependencies" step is the point of the whole job. `internal/git` and `internal/tmux` call `t.Skip` when their binary is missing, so without this assertion a runner-image change would turn real coverage into a green tick with nothing to show for it.
- `-run Smoke ./...` selects smoke tests by name across all packages, so a smoke test added elsewhere later is picked up without editing this file.

- [ ] **Step 2: Verify the workflow file parses as a valid workflow**

Run:

```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci.yml
```

Expected: no output, exit status 0.

If the module cannot be fetched (no network for that host), skip this step and note it — it is a convenience check. The authoritative verification is Task 5, where the workflow actually runs.

- [ ] **Step 3: Prove the gofmt guard fails when it should**

A guard only ever observed passing is not known to be a guard. Create a deliberately misformatted file, run the exact shell body from the `gofmt` step, and confirm it rejects it:

```bash
dir=$(mktemp -d)
printf 'package main\n\nfunc  Bad( ) {}\n' > "$dir/main.go"
unformatted=$(cd "$dir" && gofmt -l .)
if [ -n "$unformatted" ]; then echo "GUARD FIRED:"; echo "$unformatted"; else echo "GUARD FAILED TO FIRE"; fi
rm -rf "$dir"
```

Expected: prints `GUARD FIRED:` followed by `main.go`.

- [ ] **Step 4: Prove the tmux assertion fails when tmux is absent**

Run the assertion body with a `PATH` that has no tmux on it:

```bash
env PATH=/usr/bin:/bin sh -c '
for bin in git tmux; do
  if ! command -v "$bin" >/dev/null; then
    echo "::error::$bin is not on PATH — the integration tests would silently skip"
    exit 1
  fi
done
echo "ASSERTION PASSED"
'
echo "exit status: $?"
```

Expected: prints the `::error::tmux is not on PATH` line and `exit status: 1`. It must **not** print `ASSERTION PASSED`.

(tmux is installed via Homebrew on this machine, at `/home/linuxbrew/.linuxbrew/bin/tmux`, so trimming `PATH` to the system directories removes it while leaving `/usr/bin/git` reachable. If `tmux` happens to be at `/usr/bin/tmux` on the machine you are running this on, use `env PATH=/nonexistent` and accept that `git` will be reported first — the assertion firing is what matters.)

- [ ] **Step 5: Verify the same assertion passes normally**

```bash
for bin in git tmux; do command -v "$bin" >/dev/null || { echo "MISSING $bin"; exit 1; }; done; git --version; tmux -V
```

Expected: a `git version ...` line and a `tmux 3.x` line, no `MISSING`.

- [ ] **Step 6: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: run lint and test suites on pull requests

Adds a ci workflow with a lint job (gofmt, go vet, golangci-lint with the
smoke build tag) and a test job (build, race tests, smoke tests). The test
job asserts git and tmux are on PATH first, because those integration
tests self-skip when the binaries are missing and would otherwise report
a green tick having covered nothing."
```

---

### Task 3: Add the `commits` job and commitlint config

Every commit on a branch reaches `main` here — this repository merges with merge commits (`Merge pull request #NN from …`), so release-please parses each one individually. Validating the PR *title*, which is what a squash-merge repository would do, would protect nothing.

**Files:**
- Create: `.commitlintrc.yml`
- Modify: `.github/workflows/ci.yml` (append the `commits` job)

**Interfaces:**
- Consumes: the `ci.yml` file created in Task 2. Do not alter the `lint` or `test` jobs.
- Produces: a third status-check context named `commits`.

- [ ] **Step 1: Create `.commitlintrc.yml`**

At the repository root:

```yaml
extends:
  - "@commitlint/config-conventional"
rules:
  body-max-line-length: [0, always, 100]
```

This file only takes effect once Step 4 passes `configFile: .commitlintrc.yml` to `wagoid/commitlint-github-action@v6` — the action resolves its config solely from that input (defaulting to `commitlint.config.mjs`) and performs no cosmiconfig discovery of its own, so without the explicit input this file would be silently ignored and the bare `@commitlint/config-conventional` defaults would apply instead. It disables `body-max-line-length`; that default (100 characters) fails on a long URL pasted into a commit body, which is friction with no upside. Level `0` disables the rule; the remaining tuple members are inert but keep it a valid three-element rule config.

Leave `type-enum` at its default — it already covers every type CONTRIBUTING.md names (`feat`, `fix`, `docs`, `chore`, `test`, `refactor`, `ci`).

- [ ] **Step 2: Verify the config loads and accepts this branch's commits**

`node` and `npx` are available on this machine (v22.14.0). Run:

```bash
npx --yes --package @commitlint/cli --package @commitlint/config-conventional -- \
  commitlint --from origin/main --to HEAD --verbose
```

Expected: one `✔ found 0 problems, 0 warnings` line per commit on the branch. If npx cannot reach the registry, note it and rely on Task 5's real run instead.

- [ ] **Step 3: Prove commitlint rejects a bad message**

```bash
echo "added some stuff" | npx --yes --package @commitlint/cli --package @commitlint/config-conventional -- commitlint --verbose
echo "exit status: $?"
```

Expected: `✖ subject may not be empty` and `✖ type may not be empty`, then `exit status: 1`.

- [ ] **Step 4: Append the `commits` job to `.github/workflows/ci.yml`**

Add at the end of the `jobs:` block, after `test`:

```yaml
  commits:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: read
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: wagoid/commitlint-github-action@v6
        with:
          configFile: .commitlintrc.yml
```

- `if: github.event_name == 'pull_request'` — commit messages are immutable once merged, so re-validating on `main` could only ever produce a failure nobody can act on.
- `configFile: .commitlintrc.yml` is required, not optional: the action resolves its config solely from this input (defaulting to `commitlint.config.mjs`) and performs no cosmiconfig discovery of its own directories or filenames. Omit it and the in-repo `.commitlintrc.yml` is never read — the job still runs and still reports a `commits` check, but silently enforces un-overridden `@commitlint/config-conventional` defaults instead of this repo's rules.
- `fetch-depth: 0` is harmless belt-and-braces, not a requirement. This
  action never reads local git history: for `pull_request` events it fetches
  commit messages from the GitHub API (`pulls.listCommits`), and for `push`
  events from `repos.compareCommits`. A shallow checkout would work just as
  well; a full fetch costs nothing on a repo this size, so it stays.
- commitlint ignores merge commits by default. At PR time no merge commit
  exists yet, so this matters only if the trigger ever widens beyond
  `pull_request`.

- [ ] **Step 5: Re-validate the workflow file**

```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/ci.yml
```

Expected: no output, exit status 0. (Skip if it was unfetchable in Task 2.)

- [ ] **Step 6: Commit**

```bash
git add .commitlintrc.yml .github/workflows/ci.yml
git commit -m "ci: validate conventional commits on pull requests

Adds a commits job running commitlint over every commit in the PR. This
repository merges with merge commits, so each commit lands on main and is
parsed by release-please — a PR-title check would protect nothing. The
in-repo config disables body-max-line-length so a pasted URL cannot fail
a build."
```

---

### Task 4: Document the suite in CONTRIBUTING.md

A developer should be able to reproduce any CI failure without opening the workflow file. The current "Build & run" section predates both the linter and the second smoke test.

**Files:**
- Modify: `CONTRIBUTING.md` (the `## Build & run` section)

**Interfaces:**
- Consumes: the command set established in Tasks 2 and 3.
- Produces: nothing other tasks depend on.

- [ ] **Step 1: Replace the `## Build & run` section**

The current section reads:

```markdown
## Build & run

- Build: `go build ./...` (or `go build -o fleet .`)
- Test: `go test ./...` (git/tmux integration tests skip if those binaries are
  absent). A build-tagged real-CLI smoke test lives in `internal/refresher`:
  `go test -tags smoke -run Smoke ./internal/refresher/`.
- Vet: `go vet ./...`
- Run: `go run .` — requires `git`, `tmux`, and `claude` on PATH. On first run
  (no config file) it prompts for `scan_root` and writes the config; thereafter
  it loads `~/.config/fleet/config.yaml`.
```

Replace it with:

```markdown
## Build & run

- Build: `go build ./...` (or `go build -o fleet .`)
- Test: `go test -race ./...` (git/tmux integration tests skip if those
  binaries are absent — install both to actually run them).
- Smoke tests: `go test -race -tags smoke -run Smoke ./...` — build-tagged
  real-CLI tests in `internal/refresher` and `internal/selfupdate`.
- Vet: `go vet ./...`
- Format: `gofmt -l .` — must print nothing.
- Lint: `golangci-lint run --build-tags=smoke ./...`, or without installing it:
  `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run --build-tags=smoke ./...`
- Run: `go run .` — requires `git`, `tmux`, and `claude` on PATH. On first run
  (no config file) it prompts for `scan_root` and writes the config; thereafter
  it loads `~/.config/fleet/config.yaml`.

Every check above runs on each pull request via
[`.github/workflows/ci.yml`](.github/workflows/ci.yml), which also validates
commit messages with commitlint. The checks are advisory — they report on the
PR but do not block merging.
```

- [ ] **Step 2: Verify the documented commands are the ones CI runs**

Read `.github/workflows/ci.yml` and confirm every `run:` command in the `lint` and `test` jobs appears in the section above (allowing for the `gofmt -l .` wrapper, which the docs state as "must print nothing"). Fix any drift now — this is the check that keeps the doc honest.

- [ ] **Step 3: Commit**

```bash
git add CONTRIBUTING.md
git commit -m "docs: document the lint, race, and smoke commands CI runs

The build-and-run section predated golangci-lint and the second smoke
test, so a contributor could not reproduce a CI failure locally without
reading the workflow."
```

---

### Task 5: Verify on a real pull request

A workflow that has never executed is unverified. This task is not optional and not satisfiable by reasoning about the YAML.

**Files:**
- Modify: none.

**Interfaces:**
- Consumes: everything above.
- Produces: a PR with three green checks.

- [ ] **Step 1: Confirm the artefacts are committed**

```bash
git log --oneline main..HEAD
git status --short
```

Expected: commits for the spec, this plan, and Tasks 1–4; a clean working tree. CONTRIBUTING hard rule #3 requires the spec and plan to ship in this PR — if either is missing, commit it before pushing.

- [ ] **Step 2: Push the branch**

```bash
git push -u origin add_ci
```

- [ ] **Step 3: Open the PR**

```bash
gh pr create --base main --title "ci: run the check suite on pull requests" --body "$(cat <<'EOF'
Adds `.github/workflows/ci.yml` running three advisory checks on every PR
(and on pushes to `main`, which also warms the caches PR branches restore
from):

- **lint** — `gofmt`, `go vet`, and `golangci-lint` v2.12 with the `smoke`
  build tag.
- **test** — `go build`, `go test -race ./...`, and the tagged smoke tests,
  after asserting `git` and `tmux` are on PATH. Those integration tests
  self-skip when the binaries are missing, so without the assertion a
  runner-image change would turn real coverage into a green tick.
- **commits** — commitlint over every commit in the PR. This repo merges
  with merge commits, so each one reaches `main` and is parsed by
  release-please; a PR-title check would protect nothing.

Also fixes the nine pre-existing `errcheck` findings so the first run is
green. Eight are deliberate discards; the ninth was a real bug — a
deferred `Close()` on a write handle in `internal/git` swallowed any
failure to flush the `info/exclude` append.

Checks are advisory by design; branch protection is unchanged. The spec
records the `gh api` command to require them later.

Spec: `docs/superpowers/specs/2026-07-28-ci-pull-requests-design.md`
Plan: `docs/superpowers/plans/2026-07-28-ci-pull-requests.md`
EOF
)"
```

- [ ] **Step 4: Watch the run to completion**

```bash
gh pr checks --watch
```

Expected: three checks — `lint`, `test`, `commits` — all reporting **pass**. Do not proceed on a partial result.

- [ ] **Step 5: Confirm the integration tests actually ran**

A green `test` job is not sufficient evidence. Open the job log and confirm the tmux/git coverage really executed:

```bash
gh run view "$(gh run list --branch add_ci --workflow ci --limit 1 --json databaseId --jq '.[0].databaseId')" --log \
  | grep -E "tmux [0-9]|git version|ok +github.com/bray/fleet/internal/(tmux|git|refresher|selfupdate)"
```

Expected: a `tmux 3.x` line, a `git version` line, and `ok github.com/bray/fleet/internal/tmux` / `internal/git` lines. If any package reports `[no tests to run]` for the non-smoke run, or a package is missing entirely, investigate before calling this done.

- [ ] **Step 6: Report the outcome**

State plainly which checks passed, with the run URL. If anything failed, report the failure and its log output rather than the intent — a red check is the result, and the next step is fixing it, not narrating around it.

---

## Deferred (do not do as part of this plan)

- **Branch protection.** Recorded in the spec's "Making the checks required" section with the exact `gh api` command and the three context names. Left unapplied by explicit decision.
- **macOS / multi-Go matrix.** Darwin breakage is still only caught at release time.
- **A `.golangci.yml`.** Only if a default linter proves noisy in practice, with a justified exclusion.
