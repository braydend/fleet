# Release Binaries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish `fleet` binaries (linux/darwin × amd64/arm64) as automated GitHub Releases driven by Conventional Commits.

**Architecture:** release-please reads Conventional Commits on `main` and maintains a gated "Release PR"; merging it tags + creates the GitHub Release. A chained job in the same workflow then runs GoReleaser to cross-compile the four binaries and append them (plus checksums) to that release. A `--version` flag in `main.go` self-reports the build, with values injected at build time via ldflags.

**Tech Stack:** Go 1.26, GoReleaser, release-please (GitHub Action v4), GitHub Actions.

**Spec:** [`docs/superpowers/specs/2026-06-16-release-binaries-design.md`](../specs/2026-06-16-release-binaries-design.md)
**Issue:** [#9](https://github.com/braydend/fleet/issues/9)
**Conventions:** see `CLAUDE.md` — spec+plan in the same PR, bidirectional issue linking, Conventional Commits mandatory.

---

## File Structure

- Create: `main_test.go` — unit tests for the version-flag helpers (package `main`).
- Modify: `main.go` — add `version`/`commit`/`date` vars, `versionRequested`/`versionLine` helpers, and wire them into `main()`.
- Create: `.goreleaser.yaml` — build/archive/checksum/release config.
- Create: `release-please-config.json` — release-please package config.
- Create: `.release-please-manifest.json` — seeded version state.
- Create: `.github/workflows/release.yml` — two chained jobs (release-please → goreleaser).
- Modify: `README.md:47-59` — add a "Download a release" subsection under `## Install`.

---

## Task 1: `--version` flag (TDD)

**Files:**
- Create: `main_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write the failing test**

Create `main_test.go`:

```go
package main

import "testing"

func TestVersionRequested(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", []string{"fleet"}, false},
		{"long flag", []string{"fleet", "--version"}, true},
		{"short flag", []string{"fleet", "-v"}, true},
		{"subcommand", []string{"fleet", "version"}, true},
		{"unrelated", []string{"fleet", "other"}, false},
		{"empty", []string{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionRequested(tc.args); got != tc.want {
				t.Fatalf("versionRequested(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestVersionLine(t *testing.T) {
	// Defaults are set at package level; ldflags override them in real builds.
	want := "fleet dev (none, unknown)"
	if got := versionLine(); got != want {
		t.Fatalf("versionLine() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestVersion' .`
Expected: FAIL — `undefined: versionRequested` / `undefined: versionLine` (compile error).

- [ ] **Step 3: Write minimal implementation**

In `main.go`, add the version vars and helpers after the `import` block (before `func main()`):

```go
// Build metadata, overridden at release time via -ldflags -X main.version=...
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// versionRequested reports whether argv asks for the version.
func versionRequested(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[1] {
	case "--version", "-v", "version":
		return true
	}
	return false
}

// versionLine is the single line printed by --version.
func versionLine() string {
	return fmt.Sprintf("fleet %s (%s, %s)", version, commit, date)
}
```

`fmt` is already imported in `main.go`, so no import change is needed.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run 'TestVersion' .`
Expected: PASS (all subtests ok).

- [ ] **Step 5: Wire the flag into `main()`**

Modify `func main()` in `main.go` so the version check runs before everything else:

```go
func main() {
	if versionRequested(os.Args) {
		fmt.Println(versionLine())
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fleet:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Verify the build and the flag end-to-end**

Run: `go build -o /tmp/fleet . && /tmp/fleet --version`
Expected: prints `fleet dev (none, unknown)` and exits 0 (no dependency-check error, no TUI).

- [ ] **Step 7: Run the full test suite**

Run: `go test ./...`
Expected: PASS (existing packages unaffected).

- [ ] **Step 8: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: add --version flag with build metadata"
```

---

## Task 2: GoReleaser config

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Write the config**

Create `.goreleaser.yaml`:

```yaml
version: 2

before:
  hooks:
    - go mod tidy

builds:
  - id: fleet
    main: .
    binary: fleet
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X main.version={{ .Version }}
      - -X main.commit={{ .Commit }}
      - -X main.date={{ .Date }}

archives:
  - id: fleet
    name_template: "fleet_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    formats: [tar.gz]
    files:
      - README.md

checksum:
  name_template: "checksums.txt"

changelog:
  disable: true

release:
  # release-please already created the GitHub Release; just attach binaries.
  mode: append
```

- [ ] **Step 2: Validate the config**

Run: `goreleaser check`
Expected: `config is valid` (no errors). If `goreleaser` is not installed, install it first (e.g. `go install github.com/goreleaser/goreleaser/v2@latest` or `brew install goreleaser`).

- [ ] **Step 3: Cross-compile all four targets locally (no release)**

Run: `goreleaser build --snapshot --clean`
Expected: succeeds; `dist/` contains four binaries:
`fleet_linux_amd64*`, `fleet_linux_arm64*`, `fleet_darwin_amd64*`, `fleet_darwin_arm64*`.

- [ ] **Step 4: Verify ldflags injection on a snapshot binary**

Run (Linux host): `./dist/fleet_linux_amd64_v1/fleet --version`
Expected: prints a non-`dev` version like `fleet 0.0.0-SNAPSHOT-... (<commit>, <date>)`, proving the ldflags wiring works. (On macOS, run the matching `darwin` binary instead.)

- [ ] **Step 5: Commit**

`dist/` is already gitignored, so only the config is committed.

```bash
git add .goreleaser.yaml
git commit -m "ci: add GoReleaser config for linux/darwin amd64+arm64 binaries"
```

---

## Task 3: release-please configuration

**Files:**
- Create: `release-please-config.json`
- Create: `.release-please-manifest.json`

- [ ] **Step 1: Write the package config**

Create `release-please-config.json`:

```json
{
  "$schema": "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json",
  "packages": {
    ".": {
      "release-type": "go"
    }
  }
}
```

- [ ] **Step 2: Write the seeded manifest**

Create `.release-please-manifest.json`:

```json
{
  ".": "0.0.0"
}
```

This records "nothing released yet". The first release is forced to `0.1.0`
by a `Release-As:` footer in Task 6 (not by editing this file).

- [ ] **Step 3: Validate JSON**

Run: `python3 -c "import json; json.load(open('release-please-config.json')); json.load(open('.release-please-manifest.json')); print('ok')"`
Expected: `ok`.

- [ ] **Step 4: Commit**

```bash
git add release-please-config.json .release-please-manifest.json
git commit -m "ci: add release-please config (Go, manifest mode)"
```

---

## Task 4: Release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Write the workflow**

Create `.github/workflows/release.yml`:

```yaml
name: release

on:
  push:
    branches:
      - main

permissions:
  contents: write
  pull-requests: write

jobs:
  release-please:
    runs-on: ubuntu-latest
    outputs:
      release_created: ${{ steps.release.outputs.release_created }}
      tag_name: ${{ steps.release.outputs.tag_name }}
    steps:
      - uses: googleapis/release-please-action@v4
        id: release
        with:
          config-file: release-please-config.json
          manifest-file: .release-please-manifest.json

  goreleaser:
    needs: release-please
    if: ${{ needs.release-please.outputs.release_created }}
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          ref: ${{ needs.release-please.outputs.tag_name }}
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 2: Lint the workflow YAML**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml')); print('ok')"`
Expected: `ok`. (If `pyyaml` is unavailable, install it or use any YAML linter; the goal is to catch syntax errors before pushing.)

- [ ] **Step 3: Sanity-check the chaining logic**

Confirm by reading: the `goreleaser` job has `needs: release-please` and is
gated by `if: needs.release-please.outputs.release_created`, so GoReleaser only
runs after a release PR is merged (not on ordinary pushes). This is required
because the tag created via `GITHUB_TOKEN` would not trigger a separate
tag-listening workflow.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow (release-please then GoReleaser)"
```

---

## Task 5: README install instructions

**Files:**
- Modify: `README.md` (under `## Install`, currently lines 47-59)

- [ ] **Step 1: Add a "Download a release" subsection**

In `README.md`, insert the following immediately after the `## Install` heading
(line 47), before the existing "Clone and build the binary" text:

```markdown
### Download a release

Prebuilt binaries for Linux and macOS (x64 and ARM) are attached to each
[GitHub Release](https://github.com/braydend/fleet/releases). Download the
archive for your platform, then:

```bash
tar -xzf fleet_<version>_<os>_<arch>.tar.gz
sudo mv fleet /usr/local/bin/   # or anywhere on your PATH
fleet --version
```

**macOS:** the binaries are unsigned, so Gatekeeper will block the first run.
Clear the quarantine flag once after extracting:

```bash
xattr -d com.apple.quarantine ./fleet
```

(Alternatively, right-click the binary in Finder and choose **Open** the first
time.)

### Build from source
```

The existing "Clone and build" block now sits under the new
**Build from source** heading.

- [ ] **Step 2: Verify the rendered structure**

Run: `grep -n '^#' README.md`
Expected: shows `### Download a release` and `### Build from source` nested under
`## Install`, with `## Configuration` still following.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document downloading prebuilt release binaries"
```

---

## Task 6: First-release footer + issue linkage

**Files:** none (git + GitHub interaction only)

- [ ] **Step 1: Force the first release version**

The next commit to land on `main` after this work should carry a `Release-As`
footer so release-please cuts `0.1.0` exactly. Use it on the final commit of
this branch (or the merge commit), e.g.:

```bash
git commit --allow-empty -m "$(printf 'chore: enable automated releases\n\nRelease-As: 0.1.0')"
```

Verify the footer is present:

Run: `git log -1 --format=%B`
Expected: body contains a line `Release-As: 0.1.0`.

- [ ] **Step 2: Post the bidirectional issue link**

Per CLAUDE.md, comment on the issue linking the committed artefacts:

```bash
gh issue comment 9 --body "Spec and plan committed:
- Spec: docs/superpowers/specs/2026-06-16-release-binaries-design.md
- Plan: docs/superpowers/plans/2026-06-16-release-binaries.md"
```

Expected: prints the URL of the new comment.

- [ ] **Step 3: Open the PR**

```bash
git push -u origin release_binaries
gh pr create --fill --base main
```

Expected: PR created containing the spec, plan, CLAUDE.md rule, code, and CI
config together (satisfies the "same PR" rule).

---

## Verification (whole feature)

- `go test ./...` passes.
- `goreleaser check` reports the config valid.
- `goreleaser build --snapshot --clean` produces all four binaries and
  `--version` reports injected build metadata.
- README documents both download and build-from-source paths, including the
  macOS quarantine step.
- Issue #9 has a comment linking the spec and plan; the PR carries spec, plan,
  and implementation together.
- **Live verification (post-merge, cannot be done locally):** after this PR
  merges, release-please opens a Release PR for `0.1.0`; merging that PR creates
  the tag + release and triggers GoReleaser to attach the four archives +
  `checksums.txt`. Confirm the artefacts appear on the release page.
