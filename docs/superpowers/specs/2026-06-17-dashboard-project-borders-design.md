# Fleet — bordered project groups on the dashboard

**Date:** 2026-06-17
**Status:** Design — approved, pending implementation plan
**Follows:** [`2026-06-17-prettier-tui-design.md`](2026-06-17-prettier-tui-design.md) (the neon TUI makeover)

## Goal

Strengthen the dashboard's visual hierarchy by wrapping each project's sessions
in a rounded border box with the project name embedded in the top border, and
keep the activity legend directly below the boxes (above the keybind footer).
Presentation only — no behaviour, keybinding, or session-semantics change.

## Background

`internal/ui/views.go::viewDashboard` currently renders a flat list: a gradient
title, then for each session a project header line (shown once per contiguous
project run, `📂`-prefixed since the neon work), an identity line, and a detail
line. The activity legend and keybind footer follow. Sessions arrive already
ordered by project then name (the refresher reads dirs in sorted order), so a
contiguous-run check already identifies project group boundaries.

Today nothing visually delimits one project's sessions from the next beyond the
header line, which is the hierarchy weakness this addresses.

## Decisions

Settled during brainstorming:

| Decision | Choice | Why |
|----------|--------|-----|
| Grouping visual | **Rounded box per project, `📂 <name>` embedded in the top border** | Cleanest hierarchy; the name stays attached to its group. Chosen over name-as-header-row and square borders. |
| Box width | **Uniform across all boxes** (widest session line / title wins) | Boxes line up in a tidy column rather than ragged differing widths. |
| Legend position | **Below the boxes, above the keybind footer** | Order: boxes → legend → keybinds → status. (Same relative order as today, now sitting under the boxes.) |
| Box drawing | **Manual rounded-box helper**, not `lipgloss` border styling | lipgloss v1.1.0 has no border-title API, so embedding `📂 <name>` in the top border requires drawing the border string ourselves. |
| Width measurement | **`lipgloss.Width`** | Correctly accounts for ANSI escapes and wide runes (the emoji are 2 cells). |

## Non-goals

- Responsive width handling / truncation for narrow terminals — the dashboard
  already does not wrap or truncate long lines; that behaviour is unchanged and
  out of scope here.
- Any change to the box *content* (identity/detail line format, spinner, emoji,
  selection highlight) — these are reused verbatim inside the boxes.
- Restyling other screens, the title, the keybind footer, or the legend text.
- Switching to a `lipgloss` table/border-title approach or upgrading lipgloss.

## Design

### 1. `projectBox` — a pure box-drawing helper (`internal/ui/theme.go`)

```
func projectBox(label string, lines []string, innerWidth int) string
```

- `label` — the plain title text to embed in the top border (e.g. `"📂 app"`).
  The helper styles it (bold cyan, `projectStyle`) when rendering.
- `lines` — the already-styled content lines for this project's sessions
  (identity + detail lines, exactly as built today, with their `› `/`  `
  prefixes and colour).
- `innerWidth` — the visible cell width every content line is padded to (the
  uniform width computed by the caller). Each line is right-padded with spaces
  to `innerWidth` using `innerWidth - lipgloss.Width(line)` spaces (never
  negative).

Rendering (rounded border, cyan via `projectColor`):

```
╭ <label> <dashes> ╮      top:    "╭ " + styled(label) + " " + "─"×fill + "╮"
│ <line padded>    │      body:   "│ " + line + pad + " │"   (per line)
╰ <dashes>         ╯      bottom: "╰" + "─"×(innerWidth+2) + "╯"
```

The span between the corner glyphs is `innerWidth + 2` cells (one space of
padding on each side of the content). The top border's dash fill is
`innerWidth + 2 − lipgloss.Width(" "+label+" ")`. The caller guarantees
`innerWidth` is large enough that this fill is ≥ 0 (see §2). Border glyphs are
coloured with a border style built from `projectColor`; the label uses
`projectStyle`.

This is a pure function: same inputs → same output, independently testable on
its string structure.

### 2. `viewDashboard` refactor (`internal/ui/views.go`)

1. Keep the gradient title and the empty-state message unchanged.
2. Walk the (project-ordered) sessions, accumulating each session's identity and
   detail lines — built exactly as today, including the working spinner — into a
   per-project slice of content lines, plus the project label `"📂 " + project`.
3. Compute one **uniform `innerWidth`** =
   `max( max line width over all projects, max(lipgloss.Width(" "+label+" ") − 2) over all labels )`
   using `lipgloss.Width`. The label term ensures the title always fits inside
   the top border.
4. Render each project group via `projectBox(label, lines, innerWidth)`, joining
   boxes with a blank line.
5. Append the legend (unchanged text), then the keybind footer (unchanged), then
   the status line (unchanged) — in that order.

The identity/detail line construction, selection highlight (`selectedStyle`),
spinner rendering, tab-number logic, and git/age detail are all reused as-is;
only their *placement* (collected into per-project boxes) changes.

### Data flow

Unchanged. The view remains a pure function of existing model state plus the
decorative spinner. No new model state.

## Testing

TDD. Tests run under the existing ASCII colour profile (`setup_test.go`'s
`TestMain`), so border glyphs render plain and assertions match visible
characters.

- **`projectBox`** (pure):
  - Top line starts with `╭` and ends with `╮`; bottom starts with `╰`, ends
    with `╯`; the label text appears in the top line.
  - Every rendered line has the same `lipgloss.Width` (uniform box width).
  - Each body line starts with `│` and ends with `│`.
  - A label longer than `innerWidth` is still framed (fill never goes negative)
    — caller-guaranteed, but the helper must not panic; verify with a focused
    case.
- **`viewDashboard`** (via the existing model test harness):
  - Output contains the rounded corners (`╭`, `╰`) and the project name inside
    the top border (e.g. `📂 app` on the same line as `╭`).
  - Legend and keybind footer still present, legend before keybinds.
  - The existing `TestDashboardShowsGroupingTabNumbersAndLegend` assertions
    (project name, branch, `← main`, tab number, `working`/`exited`, `legend`,
    emoji) continue to pass.

## Risks

- **Box wider than the terminal** on narrow windows or very long branch names —
  pre-existing behaviour (no truncation today); accepted, see Non-goals.
- **Width arithmetic with emoji/ANSI** — mitigated by using `lipgloss.Width`
  exclusively for measurement and padding, never `len()`.

## Future scope (not now)

- Responsive truncation/ellipsis when a box would exceed the terminal width.
- A `lipgloss`-native border-title approach if/when the pinned lipgloss version
  gains that API.
