# Fleet — a prettier, playful TUI

**Date:** 2026-06-17
**Status:** Design — approved, pending implementation plan
**Issue:** [#13](https://github.com/braydend/fleet/issues/13) — make it prettier

## Goal

Fleet's UI is functional but utilitarian. Issue #13 asks for "some cute colours
and animations or something". This spec gives fleet a deliberate, playful
**neon** aesthetic — emoji status icons, a saturated colour palette, a gradient
title, and a single, meaningful animation — applied consistently across every
screen, while keeping the layout legible and the rendering logic testable.

It is a presentation change only: no behaviour, keybindings, or session
semantics change.

## Background

All styling lives today in `internal/ui/views.go` as a handful of inline
`lipgloss.Style` vars: a bold title, a hot-pink selection (`Color("212")`),
grey dims, four fixed activity colours on `◉`/`○` glyphs, and a faint-underlined
project header. The five screens are rendered by `views.go` (dashboard, project
picker, cleanup menu, confirm dialog) and `newsession.go` (the new-session
form). Activity state (working / waiting / idle / exited) already exists in
`internal/activity` and is surfaced per session.

The colours are fixed values that assume a dark terminal, the glyphs are plain,
and there is no motion anywhere.

## Decisions

These were settled during brainstorming (see issue #13 and the brainstorming
session):

| Decision | Choice | Why |
|----------|--------|-----|
| Aesthetic direction | **Playful / neon** — emoji status icons, saturated colours, gradient title | Directly answers "cute colours"; the user picked this mock-up over minimal/bordered/dashboard alternatives. |
| Colour adaptivity | **`lipgloss.AdaptiveColor`** (light + dark variants) | Keeps the neon legible on light *and* dark terminals; safest default. |
| Animation | **Animated spinner on `working` sessions only.** No inter-screen transitions. | Motion goes where it carries meaning ("something is happening"). True view transitions can only be faked by redrawing frames and are the jankiest part of TUI animation — explicitly dropped. |
| Status icon strategy | **Emoji for every state, used consistently** | Emoji are 2 cells wide; mixing them with 1-cell glyphs misaligns columns. Using emoji for *all* states keeps alignment stable. |
| Code organization | **Extract an in-package `theme`** (`internal/ui/theme.go`) | One place owns the palette + glyph set + render helpers, shared by all screens; avoids per-screen drift and keeps `views.go` from ballooning. Stays in-package, so no new import cycles. |
| Scope | **All five screens** | Consistency — a single plain screen mid-flow reads as broken. |

## Non-goals

- **View / inter-screen transitions** (slide, wipe, fade) — dropped; terminals
  cannot truly fade and faked transitions flicker.
- **Config-driven / user-selectable themes** (colours in `config.yaml`) — the
  user chose *adaptive* over *configurable*; this is future scope only.
- Any change to layout structure beyond styling (no bordered cards, no table
  columns, no status bar — those alternatives were considered and not chosen).
- Restyling tmux itself, attached panes, or anything outside the Bubble Tea
  views.
- New keybindings or behaviour.

## Design

### 1. `theme.go` — the single source of styling truth

A new `internal/ui/theme.go` (same `package ui`) holds:

- **Palette** — every colour as a `lipgloss.AdaptiveColor{Light, Dark}`:
  accent/selection (hot-pink, dark stays `212`), cyan project headers, the four
  activity colours (green / amber / grey / dark-grey), and dim body text. Each
  gets a light-mode counterpart tuned so nothing washes out on a light
  background.
- **Styles** — the `lipgloss.Style` values built from the palette (replacing
  the current inline vars in `views.go`): `titleStyle`, `selectedStyle`,
  `dimStyle`, `projectStyle`, and the per-activity styles.
- **Status icons** — a pure function `activityIcon(activity.State) string`
  returning the emoji for each state: 🟢 working · 🟡 waiting · 💤 idle ·
  ⚫ exited. Pure and unit-testable (no styling, just the rune).
- **Gradient title helper** — `gradientTitle(s string) string`, a pure function
  that interpolates a per-rune foreground colour across the string
  (pink → purple → cyan) and returns the styled result. Unit-testable for
  stability (same input → same output, correct rune count).

Keeping icon selection and the gradient as **pure functions returning strings**
is what makes the neon styling testable without asserting on raw ANSI.

### 2. The spinner (the one animation)

`working` is the only state that means "in progress", so it is the only state
that animates. To preserve column alignment (the emoji must not be swapped for a
1-cell glyph), the **emoji stays put** and an animated braille spinner
(`spinner.Spinner` / Bubbles `spinner`, frames `⠋⠙⠹⠸…`) is appended in the
session's **detail line**, coloured with the working green:

```
🟢 make_it_prettier   main ← main
    ⠙ working · ✱3 · ↑1
```

Non-working sessions render their detail line unchanged (no spinner).

**Wiring (Elm architecture):**

- `Model` gains a `spinner spinner.Model` field.
- `Init()` batches `m.spinner.Tick` with the existing refresher init command.
- `Update()` routes `spinner.TickMsg` to `m.spinner.Update`, returning the next
  tick so it keeps animating. This is additive — it must not disturb existing
  message handling (refresher ticks, key handling).
- The spinner ticks continuously but is only *rendered* next to `working`
  sessions; when no session is working it is simply not drawn.

### 3. Per-screen treatment

All screens pull from `theme.go`. No screen keeps its own ad-hoc colour.

- **Dashboard (`viewDashboard`)** — `gradientTitle("✨ fleet · your sessions ✨")`;
  📂 prefix on project headers; emoji status icon + the working spinner per
  session; emoji-flecked legend and footer
  (`💖 n new · ↵ attach · 🗑 d · q quit`). The legend maps each emoji to its
  state.
- **Project picker (`viewProjectPicker`)** — gradient title; 📂 per project;
  neon accent selection bar.
- **New-session form (`newsession.go`)** — gradient title; accent-coloured
  focused field and labels.
- **Cleanup menu (`viewCleanupMenu`)** — emoji per action (🗑 delete · 🚀 push /
  open PR · 👋 leave); neon accent selection.
- **Confirm dialog (`viewConfirm`)** — ⚠️ and a red accent to mark the
  destructive warning.

### Data flow

Unchanged. Styling is a pure function of existing model state
(`session.Session`, `activity.State`, cursor/selection, focus). The only new
state is the decorative `spinner.Model`, driven by its own tick; it feeds
rendering only and never influences session logic.

## Testing

TDD, per the project workflow. Tests are written before implementation.

- **Pure helpers** — `activityIcon` (each state → expected emoji) and
  `gradientTitle` (rune count preserved, stable output, handles empty string)
  are unit-tested directly on their string return values.
- **View rendering** — view tests force a colourless profile
  (`lipgloss.SetColorProfile(termenv.Ascii)`) or assert on emoji / structural
  substrings rather than raw ANSI escape codes, so colour changes don't make
  assertions brittle.
- **Spinner wiring** — assert `Init` includes a spinner tick command and that a
  `spinner.TickMsg` is handled by `Update` without disturbing existing
  behaviour.
- **Regression** — the existing `internal/ui/model_test.go` behaviour must keep
  passing; styling is strictly additive.

## Risks

- **Emoji width / terminal support** — emoji are 2 cells and render
  inconsistently on some terminals/fonts. Mitigated by using emoji uniformly for
  every state so columns stay aligned regardless. (A non-emoji fallback is *not*
  in scope; if a terminal mangles emoji the layout still aligns.)
- **Spinner redraw cost** — a continuous tick causes periodic re-renders. The
  fleet dashboard is small, so this is negligible; the spinner uses the standard
  Bubbles cadence.

## Future scope (explicitly not now)

- User-configurable themes via `config.yaml`.
- Inter-screen transitions, should terminals/Bubble Tea make them cheap.
- Non-emoji glyph fallback for emoji-hostile terminals.
