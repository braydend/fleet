# Self-Update — Design

**Date:** 2026-06-17
**Issue:** [#21](https://github.com/braydend/fleet/issues/21) — Can we add self-updating?
**Status:** Approved, pending implementation plan

## Goal

Let a user running a released `fleet` binary discover that a newer GitHub
Release exists and update to it **in place** with a single confirmation —
without manually visiting the Releases page, downloading a tarball, extracting
it, and clearing the macOS quarantine attribute.

This builds directly on the release pipeline described in
[`2026-06-16-release-binaries-design.md`](2026-06-16-release-binaries-design.md):
binaries are published as GitHub Releases with `tar.gz` archives named
`fleet_{version}_{os}_{arch}.tar.gz` plus a `checksums.txt`, and the running
binary's version is baked in via ldflags (`main.version`, `"dev"` for local
builds).

## Non-goals

- Auto-applying updates without user confirmation. The user always opts in.
- Auto-restarting fleet after the swap. Live tmux sessions are running; the
  user restarts when convenient.
- Updating local/`dev` builds. The check is skipped entirely when
  `version == "dev"`.
- Supporting install via package managers (there is no Homebrew tap etc.).
- Windows support (the release pipeline ships linux + darwin only).
- macOS code signing. Not needed here: a binary we download ourselves is **not**
  quarantined by Gatekeeper (quarantine is applied by browsers / LaunchServices,
  not by our own HTTP client), so in-place update sidesteps the manual-download
  Gatekeeper friction entirely.

## Architecture

A new `internal/selfupdate` package holds the domain logic. Following repo
conventions, external effects (HTTP, binary swap) sit behind interfaces so the
domain logic is unit-testable with fakes, and the package contains no direct
UI code.

The feature decomposes into four concerns:

1. **Check** — query GitHub for the latest release, compare to the running
   version, decide whether an update is available.
2. **Surface** — the UI shows a non-intrusive prompt when an update is
   available and offers a confirm-to-update action.
3. **Apply** — download the correct archive, verify its checksum, atomically
   swap the running binary.
4. **Finish** — tell the user to restart.

### Package boundaries

```
internal/selfupdate
  Checker   — given current version + an HTTP client, returns "up to date"
              or an available Release{Version, Assets, ...}. Pure logic over
              an injected client; no global state.
  Applier   — given a resolved asset URL + expected checksum, downloads,
              verifies SHA256, extracts the fleet binary, and applies the
              swap via minio/selfupdate. Behind an interface so the UI/tests
              substitute a fake.
  throttle  — read/write a last-checked timestamp in fleet's state dir;
              ShouldCheck(now) gates network calls.
```

The `ui` package owns *when* to call the checker (startup + dashboard
re-check) and renders the prompt; it depends on `selfupdate` through small
interfaces, never the reverse.

## Components

### 1. Check

- **Source:** GitHub REST API via stdlib `net/http` — no new dependency for
  the check, and no reliance on `gh` being installed or authenticated.
  `GET https://api.github.com/repos/braydend/fleet/releases/latest`
  (`Accept: application/vnd.github+json`), parse `tag_name` and the `assets`
  array (`name`, `browser_download_url`).
- **Comparison:** strip a leading `v` from `tag_name` and compare against
  `main.version` (passed into the package — `selfupdate` does not import
  `main`). Semver ordering; only a strictly-greater remote version counts as
  "available". Equal or unparseable → treated as up to date (never prompt on
  garbage).
- **Dev skip:** when the running version is `"dev"` (or otherwise unparseable),
  the checker short-circuits to "up to date" and performs no network call.
- **Unauthenticated rate limit:** 60 req/hr per IP. The throttle (below) keeps
  real usage far below this.
- **Failure handling:** network/parse errors are non-fatal — surfaced to the
  status line at most, never blocking or crashing the TUI.

### 2. Throttle

- A `last_checked` RFC3339 timestamp persisted alongside fleet's existing
  config/state (in the same dir as `~/.config/fleet/config.yaml`, e.g.
  `~/.config/fleet/state.json`; exact filename decided in the plan). Missing or
  unreadable file ⇒ treat as "never checked" ⇒ check is due.
- `ShouldCheck(now)` returns true when `now - last_checked >= 60m`.
- Updated to `now` after each completed network check (success or failure),
  so a hard-down network doesn't cause a tight retry loop.

### 3. Surface (UI)

- The check runs as a non-blocking Bubble Tea `tea.Cmd`:
  - **On startup:** fired once when the app launches.
  - **On dashboard:** when the user lands on / returns to the dashboard, if
    `ShouldCheck(now)` is true, fire another check. This gives the
    "re-check after 60 minutes of use" behaviour without a background ticker.
- When the result is "update available", the dashboard renders a
  non-intrusive banner, e.g. `Update available: vX.Y.Z → press u to update`
  (the exact key is reconciled against the existing keymap in the plan).
- Pressing the key opens the existing **confirm-dialog** component showing
  `current vX.Y.Z → new vA.B.C`. Confirming triggers Apply (also a `tea.Cmd`,
  so the download doesn't freeze the UI). Declining dismisses the prompt.

### 4. Apply

- **Asset selection:** choose the asset whose name matches
  `fleet_{ver}_{runtime.GOOS}_{runtime.GOARCH}.tar.gz`. If no asset matches the
  current platform, abort with a clear message (don't guess).
- **Checksum verification:** download `checksums.txt` from the release, look up
  the line for the selected archive, and verify the downloaded archive's
  SHA256 **before** applying. A mismatch aborts the update (no swap attempted).
- **Swap:** extract the `fleet` binary from the verified `tar.gz` and pass the
  reader to `minio/selfupdate`'s `Apply`, which performs an atomic replace of
  the currently-running executable with rollback-on-failure and permission
  preservation.
- **Non-writable install dir:** if the swap fails on a permission error
  (binary installed in a root-owned location such as `/usr/local/bin`), detect
  it and fall back to surfacing the manual install command in the status line,
  rather than failing with an opaque error.

### 5. Finish

- On success, the status line shows `Updated to vA.B.C — restart fleet to
  apply.` fleet is **not** auto-restarted; the user's tmux sessions persist and
  they restart the TUI when convenient.

## Data flow

```
launch ──► tea.Cmd: Checker.Check(version) ─┐
dashboard (≥60m since last) ────────────────┘
        │
        ▼
   Release available? ──no──► (nothing shown)
        │ yes
        ▼
   banner on dashboard ──► user presses key ──► confirm dialog
                                                     │ confirm
                                                     ▼
                              tea.Cmd: Applier.Apply(asset, checksum)
                                 download → verify SHA256 → swap
                                     │success            │perm error
                                     ▼                    ▼
                        "restart fleet to apply"   "run: <manual cmd>"
```

## Testing

Following the project's TDD workflow, tests are written first. Unit tests run
against a fake HTTP client and a fake updater — no real network or binary swap:

- **Version compare:** newer remote ⇒ available; equal / older / unparseable
  remote ⇒ up to date; leading-`v` stripping.
- **Dev skip:** `version == "dev"` ⇒ no network call, always up to date.
- **Asset selection:** correct archive chosen for each
  GOOS/GOARCH combination; no-matching-asset ⇒ abort.
- **Checksum verification:** matching SHA256 ⇒ proceed; mismatch ⇒ abort with
  no swap.
- **Throttle:** missing timestamp ⇒ due; `<60m` ⇒ not due; `≥60m` ⇒ due;
  timestamp updated after a check (including a failed one).
- **Non-writable dir:** permission error from the applier ⇒ fallback message,
  not a crash.

The real end-to-end download + swap path (which can't be meaningfully
unit-tested) is covered by a build-tagged smoke test mirroring the existing
`internal/refresher` smoke-test pattern.

## Dependencies

- New direct dependency: `github.com/minio/selfupdate` (binary swap with
  atomic replace + rollback). Everything else uses the standard library
  (`net/http`, `archive/tar`, `compress/gzip`, `crypto/sha256`).

## Conventions (per CLAUDE.md)

- This spec and its implementation plan are committed in the **same PR** as the
  implementation, both citing issue #21.
- A comment is posted back on issue #21 linking the committed spec/plan paths.
- Commits follow Conventional Commits — the feature lands under `feat:` (e.g.
  `feat(selfupdate): ...`) so release-please bumps the minor version.
