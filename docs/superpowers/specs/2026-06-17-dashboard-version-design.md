# Dashboard Version Display — Design

**Date:** 2026-06-17
**Status:** Approved, pending implementation plan

## Goal

Show the running `fleet` version on the dashboard so a user always knows which
version they are on — without having to quit and run `fleet --version`. This
pairs naturally with the self-update banner
([`2026-06-17-self-update-design.md`](2026-06-17-self-update-design.md)): the
footer shows the current version, the banner shows the version available to
update to.

## Placement & format

Append the version to the existing keybinds footer line in `viewDashboard`
(`internal/ui/views.go`), rendered dimmed (`dimStyle`) like the rest of that
line:

```
n new · enter attach · d cleanup · r refresh · q quit · v0.2.0
```

A small helper produces the label from the raw version string:

- a real release version (e.g. `0.2.0`) → `v0.2.0`
- a dev build (`dev`) → `dev` (shown verbatim, no `v` prefix — honest, and
  useful when debugging a local build)
- an empty string → omit the label entirely (so a zero-value `Model`, as used
  in existing tests, renders the footer exactly as before)

The label is only appended to the footer when it is non-empty.

## Plumbing the version into the UI

The build version currently lives only as the package-level `version` var in
`main.go` (set via ldflags at release time, `"dev"` locally). The UI does not
receive it today.

`ui.New` already has an unused second parameter:
`func New(actions *Actions, _ any) Model`. Repurpose that dead parameter into
`version string`, store it on the `Model` (a new `version` field), and have
`main.go` pass the package `version`. This threads the value through cleanly and
removes the `any` placeholder.

Call-site changes:

- `main.go`: `ui.New(&actions, nil)` → `ui.New(&actions, version)`.
- UI tests: `New(&Actions{}, nil)` → `New(&Actions{}, "")` (and one test
  constructs it with a real version to assert the footer).

## Components

- `internal/ui/model.go`:
  - `New(actions *Actions, version string) Model` — stores `version` on the
    Model.
  - `Model` gains a `version string` field.
- `internal/ui/views.go`:
  - `func versionLabel(v string) string` — returns `""` for empty, `"dev"` for
    `"dev"`, otherwise `"v" + v`.
  - `viewDashboard` appends `" · " + label` to the keybinds line when
    `versionLabel(m.version) != ""`.
- `main.go`:
  - pass the package `version` into `ui.New`.

## Testing

`internal/ui/model_test.go` (view-level assertions on `viewDashboard`):

- constructed with `"0.2.0"` → footer contains `v0.2.0`.
- constructed with `"dev"` → footer contains `dev` (and not `vdev`).
- constructed with `""` → footer contains no version label (the dashboard
  footer is unchanged from before this feature).

No new unit needs network or filesystem access; this is pure rendering.

## Non-goals (YAGNI)

- No commit hash or build date on the dashboard — those remain available via
  `fleet --version`.
- No interaction or new keybinding; this is display-only.
- No change to the self-update banner.

## Conventions (per CLAUDE.md)

- This spec and its implementation plan are committed in the same PR as the
  implementation.
- The feature lands under `feat(ui): ...` so release-please bumps the minor
  version.
