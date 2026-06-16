# Release Binaries — Design

**Date:** 2026-06-16
**Issue:** [#9](https://github.com/braydend/fleet/issues/9) — build binaries as GitHub releases
**Status:** Approved, pending implementation plan

## Goal

Publish downloadable `fleet` binaries as GitHub Releases so the app can be
shared with people to try. Releases and version numbers are automated from
conventional commits; a human gates *when* a release cuts by merging a
release PR.

### Build targets

| OS              | Arch          |
|-----------------|---------------|
| linux           | amd64 (x64)   |
| linux           | arm64         |
| darwin (macOS)  | amd64 (x64)   |
| darwin (macOS)  | arm64         |

No Windows. macOS binaries are **unsigned** (see Non-goals).

## Release flow

```
push conventional commits to main
        │
        ▼
release-please opens / updates a "Release PR"
  (computes next semver from commits, writes CHANGELOG.md)
        │   (human reviews + merges when ready)
        ▼
release-please creates the git tag + GitHub Release (with notes)
        │   (same workflow run, chained job)
        ▼
GoReleaser builds the 4 binaries + checksums and APPENDS them to the release
```

**Versioning rules** (conventional commits): `feat:` → minor bump, `fix:` →
patch bump, `!` / `BREAKING CHANGE:` → major bump. Pre-1.0, `feat:` bumps the
minor (`0.x`).

## Components

### 1. `--version` flag (`main.go`)

Add package-level vars defaulting to `"dev"`:

```go
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)
```

Early in `main()`, before the dependency check, handle a version request:
if `len(os.Args) > 1` and `os.Args[1]` is one of `--version`, `-v`, `version`,
print `fleet <version> (<commit>, <date>)` to stdout and `os.Exit(0)`.

GoReleaser injects real values via ldflags (below). Pure stdlib — no new
dependency.

### 2. `.goreleaser.yaml` (repo root)

- `builds`: single entry — `main: .`, binary `fleet`,
  `goos: [linux, darwin]`, `goarch: [amd64, arm64]`, `env: [CGO_ENABLED=0]`,
  `ldflags`: `-s -w -X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}`.
- `archives`: format `tar.gz`, name template
  `fleet_{{ .Version }}_{{ .Os }}_{{ .Arch }}`, files include `README.md`.
- `checksums`: `checksums.txt` (SHA256).
- `changelog`: `disable: true` — release-please owns the changelog.
- `release`: `mode: append` — binaries are added to the GitHub Release that
  release-please already created, preserving release-please's notes.

### 3. release-please config (manifest mode, action v4)

- `release-please-config.json`:
  ```json
  {
    "packages": {
      ".": { "release-type": "go" }
    }
  }
  ```
- `.release-please-manifest.json`:
  ```json
  { ".": "0.0.0" }
  ```
  Seeded at `0.0.0`. The **first** release is forced to `0.1.0` via a
  `Release-As: 0.1.0` footer on the commit that introduces this tooling.
  After that, versions compute normally from commits (no config pin left
  behind).

### 4. `.github/workflows/release.yml`

One workflow, two chained jobs (a separate tag-triggered workflow would NOT
fire, because tags created by the built-in `GITHUB_TOKEN` don't trigger new
workflow runs — GitHub's recursion guard):

- Trigger: `push` to `main`.
- Permissions: `contents: write`, `pull-requests: write`.
- Job `release-please`: runs `googleapis/release-please-action@v4`; exposes
  outputs `release_created` and `tag_name`.
- Job `goreleaser`: `needs: release-please`,
  `if: ${{ needs.release-please.outputs.release_created }}`. Steps:
  - `actions/checkout@v4` with `fetch-depth: 0` and
    `ref: ${{ needs.release-please.outputs.tag_name }}`,
  - `actions/setup-go@v5` with `go-version: '1.26'`,
  - `goreleaser/goreleaser-action@v6` running `release --clean`.
- Auth: built-in `GITHUB_TOKEN` only — no repository secrets to configure.

### 5. README — "Install / Try it"

Add a section covering:
- where to download (the GitHub Releases page; archive naming pattern),
- how to extract and run,
- the macOS unsigned-binary note: testers run
  `xattr -d com.apple.quarantine ./fleet` (or right-click → Open) to bypass
  Gatekeeper.

## Non-goals

- macOS code signing / notarization (would need a paid Apple Developer
  account and CI secrets). Binaries ship unsigned with bypass instructions.
- Windows builds.
- Homebrew tap, Docker images, Linux packages (deb/rpm).
- Fully automatic releases on every push (release-please gates via the
  merge step, by design).

## Testing

- `go build ./...` and `fleet --version` (exercises the new flag locally;
  shows `dev` outside a GoReleaser build).
- `goreleaser check` — validates `.goreleaser.yaml`.
- `goreleaser build --snapshot --clean` — real local cross-compile of all 4
  targets without releasing.
- release-please and the GitHub Release/upload path are verified on the first
  live run (cannot be meaningfully exercised locally); the chained-job and
  `mode: append` choices are the documented, supported pattern for this
  combination.
