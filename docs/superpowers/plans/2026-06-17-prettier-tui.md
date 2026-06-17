# Prettier Neon TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-06-17-prettier-tui-design.md`](../specs/2026-06-17-prettier-tui-design.md)
**Issue:** [#13](https://github.com/braydend/fleet/issues/13) — make it prettier

**Goal:** Give fleet a playful neon look — emoji status icons, an adaptive (light/dark) colour palette, a gradient title, and one animated spinner on `working` sessions — applied consistently across all five screens, with no behaviour change.

**Architecture:** All styling moves into a single in-package `internal/ui/theme.go` (palette, status-icon function, gradient helpers) that every view draws from. The only new model state is a decorative `spinner.Model` (Bubbles), driven by its own tick and rendered only beside `working` sessions. Icon selection and gradient colour generation are pure functions so they can be unit-tested without parsing ANSI.

**Tech Stack:** Go, Bubble Tea (`charmbracelet/bubbletea`), Lip Gloss (`charmbracelet/lipgloss`), Bubbles spinner (`charmbracelet/bubbles/spinner`), colour blending (`lucasb-eyer/go-colorful`), `muesli/termenv` (test color profile).

## Global Constraints

- **Conventional Commits, mandatory** (release-please drives versioning). User-visible styling is a feature → use `feat(ui): ...`; pure refactors with no visible change → `refactor(ui): ...`; test-only → `test(ui): ...`. A `!` or `BREAKING CHANGE:` only for breaking changes (none here).
- **End every commit message** with the trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- **TDD:** write the failing test first, watch it fail, implement minimally, watch it pass, commit.
- **Go module:** `github.com/bray/fleet`, Go 1.26.4. Pin new deps to versions already in the module cache: `bubbles@v1.0.0`, `go-colorful@v1.3.0`, `termenv@v0.16.0`.
- **No behaviour change:** keybindings, session semantics, and all existing `internal/ui/model_test.go` assertions must keep passing. Styling is strictly additive.
- **Adaptive colours only** — every palette colour is a `lipgloss.AdaptiveColor{Light, Dark}`. No fixed single-profile colours, no config-driven themes (future scope).
- **Build/test commands:** `go build ./...`, `go test ./...`, `go vet ./...`. Run from repo root.
- **Emoji uniformly for all activity states** so 2-cell glyphs keep columns aligned.

---

### Task 1: Test color profile + adaptive palette in `theme.go`

Move the inline style vars out of `views.go` into a new `theme.go`, converting every colour to `lipgloss.AdaptiveColor`. Add a package `TestMain` that forces an ASCII (colourless) profile so view-string assertions are stable.

**Files:**
- Create: `internal/ui/theme.go`
- Create: `internal/ui/setup_test.go`
- Create: `internal/ui/theme_test.go`
- Modify: `internal/ui/views.go:13-23` (remove the moved `var (...)` style block)

**Interfaces:**
- Produces: package-level vars `accentColor, dimColor, projectColor, workingColor, waitingColor, idleColor, exitedColor, warnColor lipgloss.AdaptiveColor`; styles `titleStyle, selectedStyle, dimStyle, projectStyle, workingStyle, waitingStyle, idleStyle, exitedStyle, warnStyle, spinnerStyle lipgloss.Style`; helper `activityStyle(activity.State) lipgloss.Style`.
- Consumes: `internal/activity` (`activity.State` and its constants).

- [ ] **Step 1: Write the failing test**

Create `internal/ui/setup_test.go`:

```go
package ui

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestMain forces a colourless profile so view-string assertions match the
// visible characters without ANSI escape codes interleaved.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}
```

Create `internal/ui/theme_test.go`:

```go
package ui

import "testing"

func TestPaletteIsAdaptive(t *testing.T) {
	cases := []struct {
		name        string
		light, dark string
		gotLight    string
		gotDark     string
	}{
		{"accent", "200", "212", accentColor.Light, accentColor.Dark},
		{"working", "28", "42", workingColor.Light, workingColor.Dark},
		{"waiting", "172", "220", waitingColor.Light, waitingColor.Dark},
		{"exited", "248", "238", exitedColor.Light, exitedColor.Dark},
		{"project", "31", "45", projectColor.Light, projectColor.Dark},
	}
	for _, c := range cases {
		if c.gotLight != c.light || c.gotDark != c.dark {
			t.Errorf("%s = {Light:%q Dark:%q}, want {Light:%q Dark:%q}",
				c.name, c.gotLight, c.gotDark, c.light, c.dark)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'TestPaletteIsAdaptive|TestMain' -v`
Expected: FAIL — build error, `accentColor` / `workingColor` / etc. undefined; `termenv` not yet a dependency.

- [ ] **Step 3: Add the `termenv` dependency**

Run:
```bash
go get github.com/muesli/termenv@v0.16.0
```
Expected: `go.mod` now lists `github.com/muesli/termenv v0.16.0` as a direct require (it was previously indirect).

- [ ] **Step 4: Create `theme.go`**

Create `internal/ui/theme.go`:

```go
package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/bray/fleet/internal/activity"
)

// Palette — every colour adapts to light vs dark terminals so the neon look
// stays legible on both. Dark values preserve fleet's original look.
var (
	accentColor  = lipgloss.AdaptiveColor{Light: "200", Dark: "212"} // hot pink selection
	dimColor     = lipgloss.AdaptiveColor{Light: "245", Dark: "241"} // secondary text
	projectColor = lipgloss.AdaptiveColor{Light: "31", Dark: "45"}   // cyan project headers
	workingColor = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}   // green
	waitingColor = lipgloss.AdaptiveColor{Light: "172", Dark: "220"} // amber
	idleColor    = lipgloss.AdaptiveColor{Light: "245", Dark: "244"} // grey
	exitedColor  = lipgloss.AdaptiveColor{Light: "248", Dark: "238"} // dark grey
	warnColor    = lipgloss.AdaptiveColor{Light: "160", Dark: "203"} // red, destructive
)

// Styles built from the palette. Shared by every screen.
var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	dimStyle      = lipgloss.NewStyle().Foreground(dimColor)
	projectStyle  = lipgloss.NewStyle().Bold(true).Foreground(projectColor)

	workingStyle = lipgloss.NewStyle().Foreground(workingColor)
	waitingStyle = lipgloss.NewStyle().Foreground(waitingColor)
	idleStyle    = lipgloss.NewStyle().Foreground(idleColor)
	exitedStyle  = lipgloss.NewStyle().Foreground(exitedColor)
	warnStyle    = lipgloss.NewStyle().Bold(true).Foreground(warnColor)
	spinnerStyle = lipgloss.NewStyle().Foreground(workingColor)
)

// activityStyle maps a state to its colour style.
func activityStyle(s activity.State) lipgloss.Style {
	switch s {
	case activity.Working:
		return workingStyle
	case activity.Waiting:
		return waitingStyle
	case activity.Exited:
		return exitedStyle
	default:
		return idleStyle
	}
}
```

- [ ] **Step 5: Remove the moved declarations from `views.go`**

In `internal/ui/views.go`, delete the old style `var (...)` block (lines 13-23) **and** the now-duplicated `activityStyle` function (old lines 25-37). The `glyph` function stays for now (it is removed in Task 2). After this edit `views.go` starts (after imports) directly at `// glyph renders...`.

The remaining import block in `views.go` must drop `lipgloss` if nothing else there uses it — but `glyph` still calls `activityStyle(...).Render(...)` (now defined in theme.go, no lipgloss reference in views.go) and the legend uses `workingStyle.Render` etc. (vars, not the package). Verify with the build in Step 6; if `lipgloss` becomes unused in `views.go`, remove it from that file's imports.

- [ ] **Step 6: Run build, tidy, and tests**

Run:
```bash
go build ./... && go mod tidy && go test ./internal/ui/ -v
```
Expected: build succeeds; `TestPaletteIsAdaptive` PASSES; all pre-existing `internal/ui` tests still PASS (they assert on plain substrings, now rendered colourless via `TestMain`).

- [ ] **Step 7: Commit**

```bash
git add internal/ui/theme.go internal/ui/setup_test.go internal/ui/theme_test.go internal/ui/views.go go.mod go.sum
git commit -m "$(cat <<'EOF'
refactor(ui): extract adaptive colour palette into theme.go

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Emoji status icons

Replace the single-rune `◉`/`○` glyphs with emoji used uniformly for every state, in both the session rows and the legend.

**Files:**
- Modify: `internal/ui/theme.go` (add `activityIcon`)
- Modify: `internal/ui/views.go` (use `activityIcon`; remove `glyph`; emoji legend)
- Modify: `internal/ui/theme_test.go` (add icon test)
- Modify: `internal/ui/model_test.go` (extend dashboard assertion)

**Interfaces:**
- Produces: `activityIcon(activity.State) string` returning `"🟢"` (working), `"🟡"` (waiting), `"💤"` (idle), `"⚫"` (exited).
- Consumes: palette/styles from Task 1.

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/theme_test.go`:

```go
import_marker_placeholder // (no new import needed; activity is already imported via theme.go, but theme_test.go needs it)
```

Replace that marker — add the import and test. The file's imports become:

```go
import (
	"testing"

	"github.com/bray/fleet/internal/activity"
)
```

And append:

```go
func TestActivityIcon(t *testing.T) {
	cases := []struct {
		state activity.State
		want  string
	}{
		{activity.Working, "🟢"},
		{activity.Waiting, "🟡"},
		{activity.Idle, "💤"},
		{activity.Exited, "⚫"},
	}
	for _, c := range cases {
		if got := activityIcon(c.state); got != c.want {
			t.Errorf("activityIcon(%v) = %q, want %q", c.state, got, c.want)
		}
	}
}
```

Also extend the dashboard test in `internal/ui/model_test.go` — change the want-slice in `TestDashboardShowsGroupingTabNumbersAndLegend` to include the emoji:

```go
	for _, want := range []string{"app", "fleet/a", "← main", "1", "working", "exited", "legend", "🟢", "⚫"} {
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'TestActivityIcon|TestDashboardShowsGroupingTabNumbersAndLegend' -v`
Expected: FAIL — `activityIcon` undefined; dashboard view missing `"🟢"`.

- [ ] **Step 3: Add `activityIcon` to `theme.go`**

Append to `internal/ui/theme.go`:

```go
// activityIcon returns the emoji shown for a session's state. Emoji are used
// for every state (not just some) so the 2-cell width keeps columns aligned.
func activityIcon(s activity.State) string {
	switch s {
	case activity.Working:
		return "🟢"
	case activity.Waiting:
		return "🟡"
	case activity.Exited:
		return "⚫"
	default: // Idle
		return "💤"
	}
}
```

- [ ] **Step 4: Use the icon in `views.go` and update the legend**

In `internal/ui/views.go`, delete the `glyph` function (old lines 39-42).

In `viewDashboard`, change the identity line to use `activityIcon`:

```go
		identity := fmt.Sprintf("%s %s %s  %s ← %s", num, activityIcon(s.Activity), s.Name, s.Branch, s.Base)
```

And replace the legend with the emoji version:

```go
	legend := fmt.Sprintf("legend: %s working  %s waiting  %s idle  %s exited",
		activityIcon(activity.Working), activityIcon(activity.Waiting),
		activityIcon(activity.Idle), activityIcon(activity.Exited))
```

If removing `glyph` leaves `session` imported only for the `glyph` signature, leave the import — `viewDashboard` still ranges over `m.sessions` (`[]session.Session`) and references `session.Session` indirectly; the build in Step 5 confirms. (`lipgloss` may now be unused in `views.go`; remove it from the import block if the build says so.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/ui/ -v`
Expected: PASS — `TestActivityIcon`, `TestDashboardShowsGroupingTabNumbersAndLegend`, and all existing tests.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/theme.go internal/ui/views.go internal/ui/theme_test.go internal/ui/model_test.go
git commit -m "$(cat <<'EOF'
feat(ui): use emoji status icons across the dashboard

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Gradient title

Add a pure gradient-colour generator and a `gradientTitle` renderer (pink → purple → cyan), and apply it to the dashboard title.

**Files:**
- Modify: `internal/ui/theme.go` (add gradient helpers + colorful import)
- Modify: `internal/ui/views.go` (dashboard title uses `gradientTitle`)
- Modify: `internal/ui/theme_test.go` (gradient tests)

**Interfaces:**
- Produces: `gradientColors(n int) []lipgloss.Color` (n interpolated colours; `nil` for n≤0; exact endpoints `#ff79c6` and `#8be9fd`); `gradientTitle(s string) string` (each rune coloured along the gradient; under the ASCII test profile the visible text equals `s`).
- Consumes: palette/styles from Task 1.

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/theme_test.go`:

```go
func TestGradientColorsLengthAndEndpoints(t *testing.T) {
	if got := gradientColors(0); got != nil {
		t.Errorf("gradientColors(0) = %v, want nil", got)
	}
	if got := gradientColors(1); len(got) != 1 {
		t.Fatalf("gradientColors(1) len = %d, want 1", len(got))
	}
	cols := gradientColors(5)
	if len(cols) != 5 {
		t.Fatalf("gradientColors(5) len = %d, want 5", len(cols))
	}
	if string(cols[0]) != "#ff79c6" {
		t.Errorf("first colour = %q, want %q", string(cols[0]), "#ff79c6")
	}
	if string(cols[4]) != "#8be9fd" {
		t.Errorf("last colour = %q, want %q", string(cols[4]), "#8be9fd")
	}
}

func TestGradientTitlePreservesText(t *testing.T) {
	// Under the ASCII test profile (see TestMain) Render emits no escape codes,
	// so the visible characters are exactly the input.
	in := "✨ fleet ✨"
	if got := gradientTitle(in); got != in {
		t.Errorf("gradientTitle visible text = %q, want %q", got, in)
	}
	if got := gradientTitle(""); got != "" {
		t.Errorf("gradientTitle(\"\") = %q, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'TestGradient' -v`
Expected: FAIL — `gradientColors` / `gradientTitle` undefined.

- [ ] **Step 3: Add the gradient helpers and the colorful dependency**

Run:
```bash
go get github.com/lucasb-eyer/go-colorful@v1.3.0
```

Append to `internal/ui/theme.go` and add `"strings"` and `colorful "github.com/lucasb-eyer/go-colorful"` to its imports. The import block of `theme.go` becomes:

```go
import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	colorful "github.com/lucasb-eyer/go-colorful"

	"github.com/bray/fleet/internal/activity"
)
```

Append the helpers:

```go
// Gradient stops for the title: pink → purple → cyan.
var (
	gradStart = mustHex("#ff79c6")
	gradMid   = mustHex("#bd93f9")
	gradEnd   = mustHex("#8be9fd")
)

func mustHex(s string) colorful.Color {
	c, err := colorful.Hex(s)
	if err != nil {
		panic("ui: bad gradient hex " + s + ": " + err.Error())
	}
	return c
}

// gradientColors returns n colours interpolated across the pink→purple→cyan
// stops. Endpoints are pinned to the exact stop hexes; n<=0 returns nil.
func gradientColors(n int) []lipgloss.Color {
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []lipgloss.Color{lipgloss.Color(gradStart.Hex())}
	}
	out := make([]lipgloss.Color, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1) // 0..1
		var c colorful.Color
		switch {
		case t < 0.5:
			c = gradStart.BlendLab(gradMid, t/0.5)
		default:
			c = gradMid.BlendLab(gradEnd, (t-0.5)/0.5)
		}
		out[i] = lipgloss.Color(c.Clamped().Hex())
	}
	// Pin exact endpoints (blend round-trips can drift by a unit).
	out[0] = lipgloss.Color(gradStart.Hex())
	out[n-1] = lipgloss.Color(gradEnd.Hex())
	return out
}

// gradientTitle renders s with a per-rune colour gradient.
func gradientTitle(s string) string {
	runes := []rune(s)
	cols := gradientColors(len(runes))
	var b strings.Builder
	for i, r := range runes {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(cols[i]).Render(string(r)))
	}
	return b.String()
}
```

- [ ] **Step 4: Apply the gradient to the dashboard title**

In `internal/ui/views.go`, in `viewDashboard`, change the title line from:

```go
	b.WriteString(titleStyle.Render("fleet — sessions") + "\n\n")
```

to:

```go
	b.WriteString(gradientTitle("✨ fleet · your sessions ✨") + "\n\n")
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/ui/ -v`
Expected: PASS — gradient tests and all existing tests. (`TestDashboardShowsGroupingTabNumbersAndLegend` does not assert on the title text, so the gradient does not break it.)

- [ ] **Step 6: Commit**

```bash
git add internal/ui/theme.go internal/ui/views.go internal/ui/theme_test.go go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(ui): add gradient dashboard title

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Animated spinner on working sessions

Add a Bubbles `spinner.Model` to the root model, tick it, and render its frame in the detail line of `working` sessions only.

**Files:**
- Modify: `internal/ui/model.go` (spinner field, `New`, `Init`, `Update`)
- Modify: `internal/ui/views.go` (`viewDashboard` renders the frame for working)
- Modify: `internal/ui/model_test.go` (spinner-tick + render tests)

**Interfaces:**
- Consumes: `spinner.New`, `spinner.MiniDot`, `spinner.TickMsg`, `(spinner.Model).Tick`, `(spinner.Model).Update`, `(spinner.Model).View` from `github.com/charmbracelet/bubbles/spinner`; `spinnerStyle` from Task 1.
- Produces: `Model.spinner spinner.Model`. After `New(nil, nil)` the spinner is `MiniDot` at frame 0, whose `View()` is `"⠋"` under the ASCII test profile.

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/model_test.go` (the `spinner` import is added in Step 3; tests reference `spinner.TickMsg`):

```go
func TestSpinnerTickKeepsStateAndReturnsCmd(t *testing.T) {
	m := New(nil, nil)
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	m = updated.(Model)
	next, cmd := m.Update(spinner.TickMsg{})
	if cmd == nil {
		t.Fatal("expected spinner tick to schedule the next tick")
	}
	mm := next.(Model)
	if mm.state != stateDashboard {
		t.Fatalf("spinner tick changed state to %v", mm.state)
	}
	if len(mm.sessions) != 2 {
		t.Fatalf("spinner tick disturbed sessions: got %d", len(mm.sessions))
	}
}

func TestDashboardSpinsOnlyWorkingSessions(t *testing.T) {
	m := New(nil, nil)
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	out := updated.(Model).View()
	// session "a" is Working: its detail line shows the MiniDot frame "⠋".
	if !strings.Contains(out, "⠋ working") {
		t.Fatalf("expected working session to show a spinner frame.\n---\n%s", out)
	}
	// session "b" is Exited: no spinner frame on its detail line.
	if strings.Contains(out, "⠋ exited") {
		t.Fatalf("did not expect a spinner frame on an exited session.\n---\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run 'TestSpinner|TestDashboardSpins' -v`
Expected: FAIL — `spinner` package not imported / not a dependency; `Model` has no spinner; working detail line has no `"⠋"`.

- [ ] **Step 3: Add the bubbles dependency and wire the spinner into the model**

Run:
```bash
go get github.com/charmbracelet/bubbles@v1.0.0
```

In `internal/ui/model.go`, add the import (grouped with the other charm import):

```go
import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/session"
)
```

Add the field to `Model` (after `status string`):

```go
	status string

	// spinner animates the glyph beside working sessions (decorative only).
	spinner spinner.Model
```

Initialise it in `New` — replace the return statement:

```go
func New(actions *Actions, _ any) Model {
	var a Actions
	if actions != nil {
		a = *actions
	}
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = spinnerStyle
	return Model{actions: a, state: stateDashboard, spinner: sp}
}
```

Batch the spinner tick into `Init`:

```go
func (m Model) Init() tea.Cmd {
	return tea.Batch(refresh(m.actions.Refresh), tick(), m.spinner.Tick)
}
```

Add a `spinner.TickMsg` case to `Update` (place it alongside the other `case` clauses, e.g. just before `case tea.KeyMsg:`):

```go
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
```

- [ ] **Step 4: Render the frame for working sessions in `views.go`**

In `internal/ui/views.go`, in `viewDashboard`, change the detail-line construction from:

```go
		// Detail line: activity word, git state, age.
		detail := "    " + s.Activity.Label()
```

to:

```go
		// Detail line: activity word, git state, age. Working sessions get the
		// animated spinner frame; other states render plain.
		detail := "    "
		if s.Activity == activity.Working {
			detail += m.spinner.View() + " "
		}
		detail += s.Activity.Label()
```

(`activity` is already imported in `views.go`.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/ui/ -v`
Expected: PASS — `TestSpinnerTickKeepsStateAndReturnsCmd`, `TestDashboardSpinsOnlyWorkingSessions`, and all existing tests.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/model.go internal/ui/views.go internal/ui/model_test.go go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(ui): animate a spinner on working sessions

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Restyle the remaining screens

Bring the project picker, new-session form, cleanup menu, and confirm dialog up to the neon look: gradient titles, 📂 project markers, emoji cleanup actions, and a ⚠️ destructive warning.

**Files:**
- Modify: `internal/ui/views.go` (`viewProjectPicker`, `viewCleanupMenu`, `viewConfirm`)
- Modify: `internal/ui/newsession.go` (`view`)
- Modify: `internal/ui/model_test.go` (per-screen view assertions)

**Interfaces:**
- Consumes: `gradientTitle`, `selectedStyle`, `dimStyle`, `projectStyle`, `warnStyle` from Tasks 1 & 3.
- Produces: no new exported surface — view output changes only.

- [ ] **Step 1: Write the failing tests**

Add to `internal/ui/model_test.go`:

```go
func TestProjectPickerHasFolderMarkers(t *testing.T) {
	a := Actions{Projects: func() ([]projects.Project, error) {
		return []projects.Project{{Name: "app"}, {Name: "web"}}, nil
	}}
	m := New(&a, nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated, _ := m.Update(cmd())
	out := updated.(Model).View()
	if !strings.Contains(out, "📂 app") || !strings.Contains(out, "📂 web") {
		t.Fatalf("project picker missing folder markers.\n---\n%s", out)
	}
}

func TestCleanupMenuHasEmojiActions(t *testing.T) {
	m := New(nil, nil)
	m.sessions = sample()
	m.cursor = 0
	m.state = stateCleanupMenu
	out := m.View()
	for _, want := range []string{"🗑", "🚀", "👋"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cleanup menu missing %q.\n---\n%s", want, out)
		}
	}
}

func TestConfirmDialogHasWarning(t *testing.T) {
	m := New(nil, nil)
	m.pendingDelete = sample()[0]
	m.state = stateConfirm
	out := m.View()
	if !strings.Contains(out, "⚠️") {
		t.Fatalf("confirm dialog missing warning marker.\n---\n%s", out)
	}
}

func TestNewSessionFormHasFleetTitle(t *testing.T) {
	f := newForm(projects.Project{Name: "app", DefaultBranch: "main"})
	out := f.view()
	if !strings.Contains(out, "app") {
		t.Fatalf("form title missing project name.\n---\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run 'TestProjectPicker|TestCleanupMenuHasEmoji|TestConfirmDialogHasWarning|TestNewSessionFormHasFleetTitle' -v`
Expected: FAIL — markers `📂` / `🗑` / `⚠️` not yet present. (`TestNewSessionFormHasFleetTitle` may pass already since the title contains "app"; that is fine — it guards the form change in Step 3.)

- [ ] **Step 3: Restyle the views**

In `internal/ui/views.go`:

`viewProjectPicker` — gradient title and 📂 markers:

```go
func (m Model) viewProjectPicker() string {
	var b strings.Builder
	b.WriteString(gradientTitle("✨ pick a project ✨") + "\n\n")
	for i, p := range m.projects {
		line := "📂 " + p.Name
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("enter select · esc cancel"))
	return b.String()
}
```

`viewCleanupMenu` — gradient title and emoji actions:

```go
func (m Model) viewCleanupMenu() string {
	s, _ := m.selected()
	var b strings.Builder
	b.WriteString(gradientTitle("✨ cleanup — "+s.Project+"/"+s.Name+" ✨") + "\n\n")
	choices := []string{"🗑  delete worktree + branch", "🚀 push / open PR", "👋 leave (kill tmux only)"}
	for i, c := range choices {
		if cleanupChoice(i) == m.cleanupChoice {
			b.WriteString(selectedStyle.Render("› "+c) + "\n")
		} else {
			b.WriteString("  " + c + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("enter choose · esc cancel"))
	return b.String()
}
```

`viewConfirm` — warning marker and red accent:

```go
func (m Model) viewConfirm() string {
	s := m.pendingDelete
	return warnStyle.Render("⚠️  confirm delete") + "\n\n" +
		fmt.Sprintf("%s/%s has uncommitted or unpushed changes.\n", s.Project, s.Name) +
		"Delete worktree and branch anyway? " + dimStyle.Render("(y/n)")
}
```

In `internal/ui/newsession.go`, in `view`, change the title and focus highlight:

```go
func (f newSessionForm) view() string {
	var b strings.Builder
	b.WriteString(gradientTitle("✨ new session — "+f.project.Name+" ✨") + "\n\n")
	rows := []struct{ label, val string }{
		{"session", f.sessionName},
		{"branch", f.branch},
		{"base", f.base},
	}
	for i, r := range rows {
		line := fmt.Sprintf("%-8s %s", r.label+":", r.val)
		if i == f.field {
			b.WriteString(selectedStyle.Render("› "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + dimStyle.Render("tab next · enter submit on last field · esc cancel"))
	return b.String()
}
```

(`newsession.go` already imports `fmt` and `strings`; `selectedStyle`, `dimStyle`, and `gradientTitle` are package-level. The old `titleStyle`/`dimStyle` references in this file are satisfied by theme.go.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/ui/ -v`
Expected: PASS — all four new tests and every existing test.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/views.go internal/ui/newsession.go internal/ui/model_test.go
git commit -m "$(cat <<'EOF'
feat(ui): restyle picker, form, cleanup, and confirm screens

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Full verification

Confirm the whole repo builds, vets, and tests cleanly, and that nothing outside `internal/ui` regressed.

**Files:** none (verification only).

- [ ] **Step 1: Run the full build, vet, and test suite**

Run:
```bash
go build ./... && go vet ./... && go test ./...
```
Expected: all succeed; the git/tmux integration tests may SKIP if those binaries are unavailable — that is acceptable, but no FAILs.

- [ ] **Step 2: Confirm the dependency graph is tidy**

Run:
```bash
go mod tidy && git diff --exit-code go.mod go.sum
```
Expected: no diff (Tasks 1, 3, 4 already tidied). If there is a diff, commit it:

```bash
git add go.mod go.sum
git commit -m "$(cat <<'EOF'
chore: tidy module graph after neon TUI work

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3: Manual smoke (optional, requires git/tmux/claude)**

Run: `go run .`
Expected: the dashboard shows the gradient title, 📂 project headers, emoji status icons, and an animated spinner beside any working session; pressing `n` shows the restyled picker → form; `d` shows the emoji cleanup menu; a dirty-delete shows the ⚠️ confirm dialog. Detach/quit with `q`.

---

## Self-Review

**1. Spec coverage:**
- Adaptive palette in `theme.go` → Task 1. ✓
- Emoji for every state, aligned columns → Task 2. ✓
- Working-only braille (`MiniDot`) spinner + Elm wiring (field/`New`/`Init`/`Update`/render) → Task 4. ✓
- Gradient title (pink→purple→cyan, per-rune) → Task 3. ✓
- All five screens restyled (dashboard across Tasks 2-4; picker/form/cleanup/confirm in Task 5) → ✓
- TDD with pure-function tests (`activityIcon`, `gradientColors`/`gradientTitle`) and ASCII-profile view tests (`TestMain`) → Tasks 1-5. ✓
- Existing `model_test.go` behaviour preserved → asserted by running the suite each task; Task 6 full run. ✓
- Non-goals (transitions, config themes) → not built. ✓

**2. Placeholder scan:** No "TBD"/"handle edge cases"/"similar to Task N"; every code step shows complete code. The one `import_marker_placeholder` token in Task 2 Step 1 is explicitly an instruction to replace it with the shown import block, not a leftover — the replacement is given inline immediately after.

**3. Type consistency:** `activityIcon(activity.State) string`, `gradientColors(int) []lipgloss.Color`, `gradientTitle(string) string`, `Model.spinner spinner.Model`, and the palette var names (`accentColor`, `workingColor`, …, `spinnerStyle`) are used identically wherever referenced across Tasks 1-5. Spinner methods (`Tick`, `Update`, `View`) and `spinner.TickMsg` match the verified Bubbles v1.0.0 API.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-17-prettier-tui.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
