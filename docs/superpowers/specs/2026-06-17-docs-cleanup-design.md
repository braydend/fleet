# Documentation cleanup — design spec

**Status:** Approved — ready for implementation plan
**Date:** 2026-06-17

## Goal

Restructure the project's documentation so that:

- The **README** is a short, welcoming entry point: a one-paragraph
  description, a demonstration GIF, and a quick getting-started path — enough to
  understand what `fleet` is and run it, without the exhaustive reference.
- The **full user reference** (dependencies, complete install, complete
  configuration, complete keybindings, cleanup menu) lives in a new
  `docs/usage.md`.
- The **development docs** (build/test commands, the mandatory workflow rules,
  working conventions) live in a new `CONTRIBUTING.md`, which becomes the single
  canonical source for contributors.
- `CLAUDE.md` keeps the agent-facing architecture context but defers to
  `CONTRIBUTING.md` for the development workflow, avoiding duplication.

## Background

The current `README.md` (167 lines) carries everything: description, how it
works, dependencies, full install, full configuration, getting started, full
keybinding tables, cleanup menu, a development command block, and status. This
makes it long for a first-time reader and mixes user-facing and
contributor-facing content.

Separately, `CLAUDE.md` holds the authoritative "Development workflow (hard
rules)" and "Working conventions" sections. These are written for the agent but
are equally relevant to human contributors, and there is no `CONTRIBUTING.md`
today.

## Design

### `README.md` — slim quick start

Self-contained short path. Sections:

1. **Title + one-paragraph description** — what `fleet` is.
2. **GIF placeholder** — a markdown image reference the maintainer will populate
   (e.g. `![fleet demo](docs/assets/demo.gif)`). Leave the path as a documented
   placeholder; do not fabricate the asset.
3. **How it works** — keep the existing bullet-list pitch; it sells the tool.
4. **Quick start** — the minimal happy path:
   - Install: the release-download one-liner, plus a one-line `go build`/`go
     run .` note for source.
   - Run `fleet`; on first run it prompts for `scan_root` and writes the config.
   - Press `n` to create a session, `Enter` to attach.
5. **Configuration** — short: reads `~/.config/fleet/config.yaml`, first run
   prompts you; link to `docs/usage.md` for all fields.
6. **Keybindings** — a compact essentials table (the few keys needed to be
   productive), linking to `docs/usage.md` for the complete set.
7. **Status** — one short line.
8. **Footer links** — to `docs/usage.md` (full usage) and `CONTRIBUTING.md`
   (development).

The README remains useful on its own; the usage guide is the deep reference.
Minor topic overlap between the README quick start and the usage guide is
expected and acceptable (quickstart vs. reference).

### `docs/usage.md` — complete user reference

The detail moved out of the README:

- **Dependencies** — required (`git`, `tmux`, `claude`) and optional (`gh`)
  tables; the Go 1.22+ build note; the startup-check behaviour.
- **Install** — download a release (incl. the macOS Gatekeeper / `xattr` note)
  and build from source.
- **Configuration** — all fields, defaults, and first-run behaviour.
- **Keybindings** — full dashboard, project-picker, and new-session-form tables.
- **Cleanup menu** — the three actions (delete / push+PR / leave) and their
  confirmation behaviour.

### `CONTRIBUTING.md` — canonical development doc

Written for human contributors, the single source of truth for how to work in
this repo:

- **Build & run / test** — the `go build`/`go vet`/`go test` commands and the
  tagged smoke test.
- **Development workflow (hard rules)** — the mandatory brainstorm → spec → plan
  flow, GitHub-issue linking, artefacts-checked-in-with-the-PR rule, and
  Conventional Commits requirement.
- **Working conventions** — TDD, thin adapters behind interfaces, surfacing
  errors in the status line, confirmation for destructive actions.

### `CLAUDE.md` — defer to CONTRIBUTING

`CLAUDE.md` keeps the agent-facing architecture context it uniquely provides
(what this is, status, tech stack, core design decisions, planned package
layout, MVP scope). Its "Development workflow (hard rules)" and "Working
conventions" sections are replaced with a short pointer to `CONTRIBUTING.md` so
there is exactly one canonical copy of those rules. The "Build & run" section
may stay brief or likewise point to `CONTRIBUTING.md`.

## Non-goals

- No changes to application code or behaviour.
- No new documentation content beyond reorganising and lightly editing what
  already exists (plus the GIF placeholder and cross-links).
- The maintainer supplies the actual GIF; this work only leaves a documented
  placeholder.
- No changes to the existing `docs/superpowers/` specs and plans.

## Success criteria

- `README.md` is materially shorter and contains only: description, GIF
  placeholder, how it works, quick start, short configuration note, essentials
  keybindings, status, and footer links.
- `docs/usage.md` exists and contains the complete user reference, with no loss
  of information relative to today's README.
- `CONTRIBUTING.md` exists and contains the build/test commands, the workflow
  hard rules, and the working conventions.
- `CLAUDE.md`'s workflow and conventions sections point to `CONTRIBUTING.md`
  rather than duplicating it; architecture context is preserved.
- All internal cross-links resolve.
