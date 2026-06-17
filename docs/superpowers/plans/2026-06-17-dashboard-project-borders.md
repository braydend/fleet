# Bordered Project Groups — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** [`docs/superpowers/specs/2026-06-17-dashboard-project-borders-design.md`](../specs/2026-06-17-dashboard-project-borders-design.md)

**Goal:** Wrap each project's sessions on the dashboard in a uniform-width rounded box with `📂 <name>` embedded in the top border, keeping the legend below the boxes and above the keybind footer. Presentation only.

**Architecture:** A new pure `projectBox` helper in `internal/ui/theme.go` draws a rounded box (manual border string, since lipgloss v1.1.0 has no border-title API) with widths measured by `lipgloss.Width`. `viewDashboard` groups the already-ordered sessions by project, reuses today's identity/detail line construction verbatim, computes one uniform inner width, and renders each group through the helper.

**Tech Stack:** Go, Lip Gloss (`lipgloss.Width`, `RoundedBorder` glyphs drawn manually), existing `theme.go` palette (`projectColor`, `projectStyle`).

## Global Constraints

- **Conventional Commits, mandatory.** Visible feature → `feat(ui): ...`.
- **End every commit message** with the trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- **TDD:** failing test → watch it fail → minimal implementation → watch it pass → commit.
- **Measure widths with `lipgloss.Width` only**, never `len()` — content contains ANSI escapes and 2-cell emoji.
- **No behaviour change:** keybindings, selection, spinner, and all existing `internal/ui` tests must keep passing. The identity/detail line content is reused verbatim; only its placement (inside boxes) changes.
- **No responsive truncation** for narrow terminals — out of scope (matches current behaviour).
- **Build/test:** `go build ./...`, `go test ./internal/ui/...`, run from repo root.

---

### Task 1: `projectBox` rounded-box helper

**Files:**
- Modify: `internal/ui/theme.go` (add `projectBox`)
- Modify: `internal/ui/theme_test.go` (add tests + imports)

**Interfaces:**
- Produces: `projectBox(label string, lines []string, innerWidth int) string` — returns a rounded box whose every row has visible width `innerWidth + 4` (when `innerWidth` ≥ the label width), with `label` embedded in the top border, body lines padded to `innerWidth`, border glyphs coloured via `projectColor` and the label via `projectStyle`. Must not panic when `label` is wider than `innerWidth`.
- Consumes: `projectColor`, `projectStyle` (theme.go); `lipgloss.Width`, `strings.Repeat`.

- [ ] **Step 1: Write the failing tests**

Set the import block of `internal/ui/theme_test.go` to (it currently imports only `"testing"` and the activity package):

```go
import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/bray/fleet/internal/activity"
)
```

Append these tests:

```go
func TestProjectBoxStructure(t *testing.T) {
	out := projectBox("📂 app", []string{"line one", "two"}, 12)
	rows := strings.Split(out, "\n")
	if len(rows) != 4 { // top + 2 body + bottom
		t.Fatalf("expected 4 rows, got %d:\n%s", len(rows), out)
	}
	if !strings.HasPrefix(rows[0], "╭") || !strings.HasSuffix(rows[0], "╮") {
		t.Errorf("top row not framed: %q", rows[0])
	}
	if !strings.Contains(rows[0], "📂 app") {
		t.Errorf("label missing from top border: %q", rows[0])
	}
	last := rows[len(rows)-1]
	if !strings.HasPrefix(last, "╰") || !strings.HasSuffix(last, "╯") {
		t.Errorf("bottom row not framed: %q", last)
	}
	for _, r := range rows[1 : len(rows)-1] {
		if !strings.HasPrefix(r, "│") || !strings.HasSuffix(r, "│") {
			t.Errorf("body row not framed: %q", r)
		}
	}
	want := lipgloss.Width(rows[0])
	for _, r := range rows {
		if got := lipgloss.Width(r); got != want {
			t.Errorf("row width mismatch: %q is %d, want %d", r, got, want)
		}
	}
}

func TestProjectBoxOverlongLabelDoesNotPanic(t *testing.T) {
	// innerWidth smaller than the label must not panic and stays framed.
	out := projectBox("📂 a-very-long-project-name", []string{"x"}, 3)
	rows := strings.Split(out, "\n")
	if !strings.HasPrefix(rows[0], "╭") || !strings.HasSuffix(rows[0], "╮") {
		t.Errorf("top row not framed: %q", rows[0])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run TestProjectBox -v`
Expected: FAIL — `projectBox` undefined (build error).

- [ ] **Step 3: Implement `projectBox` in `theme.go`**

Append to `internal/ui/theme.go` (imports `strings` and `lipgloss` are already present):

```go
// projectBox draws a rounded box around a project's session lines, with the
// label embedded in the top border. innerWidth is the visible cell width each
// content line is padded to (uniform across boxes; computed by the caller).
// Border glyphs use projectColor; the label uses projectStyle. Widths are
// measured with lipgloss.Width so ANSI escapes and 2-cell emoji are counted
// correctly.
func projectBox(label string, lines []string, innerWidth int) string {
	border := lipgloss.NewStyle().Foreground(projectColor)
	span := innerWidth + 2 // content area + one space of padding each side

	dashes := span - lipgloss.Width(" "+label+" ")
	if dashes < 0 {
		dashes = 0
	}

	var b strings.Builder
	// Top: ╭ <label> <dashes>╮
	b.WriteString(border.Render("╭ "))
	b.WriteString(projectStyle.Render(label))
	b.WriteString(border.Render(" " + strings.Repeat("─", dashes) + "╮"))
	// Body: │ <line padded> │
	for _, ln := range lines {
		pad := innerWidth - lipgloss.Width(ln)
		if pad < 0 {
			pad = 0
		}
		b.WriteString("\n")
		b.WriteString(border.Render("│") + " " + ln + strings.Repeat(" ", pad) + " " + border.Render("│"))
	}
	// Bottom: ╰<dashes>╯
	b.WriteString("\n")
	b.WriteString(border.Render("╰" + strings.Repeat("─", span) + "╯"))
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ui/ -run TestProjectBox -v`
Expected: PASS — both tests.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/theme.go internal/ui/theme_test.go
git commit -m "$(cat <<'EOF'
feat(ui): add rounded project-box helper

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Render dashboard projects in boxes

**Files:**
- Modify: `internal/ui/views.go` (`viewDashboard`)
- Modify: `internal/ui/model_test.go` (add a test)

**Interfaces:**
- Consumes: `projectBox` (Task 1); `lipgloss.Width`; existing `activityIcon`, `selectedStyle`, `dimStyle`, `gradientTitle`, `m.spinner`.
- Produces: no new exported surface — `viewDashboard` output changes (project groups wrapped in boxes).

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/model_test.go` (it already imports `strings`):

```go
func TestDashboardWrapsProjectsInBorders(t *testing.T) {
	m := New(nil, nil)
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	out := updated.(Model).View()

	if !strings.Contains(out, "╭") || !strings.Contains(out, "╰") {
		t.Fatalf("dashboard missing rounded box borders.\n---\n%s", out)
	}
	// Project name sits inside the top border.
	found := false
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "╭") && strings.Contains(ln, "📂 app") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("project name not embedded in a top border.\n---\n%s", out)
	}
	// Legend stays before the keybind footer.
	li := strings.Index(out, "legend:")
	ki := strings.Index(out, "n new ·")
	if li < 0 || ki < 0 || li > ki {
		t.Errorf("legend should appear before keybinds (legend=%d keybind=%d)", li, ki)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestDashboardWrapsProjectsInBorders -v`
Expected: FAIL — no `╭`/`╰` in the current flat output.

- [ ] **Step 3: Refactor `viewDashboard` in `views.go`**

Replace the entire `viewDashboard` function (lines 26-88) with:

```go
func (m Model) viewDashboard() string {
	var b strings.Builder
	b.WriteString(gradientTitle("✨ fleet · your sessions ✨") + "\n\n")
	if len(m.sessions) == 0 {
		b.WriteString(dimStyle.Render("no sessions. press n to create one.") + "\n")
	}

	// Sessions arrive already ordered by project then name (the refresher reads
	// dirs in os.ReadDir's sorted order), so a contiguous-run check groups each
	// project's sessions into one labelled block of styled content lines.
	type block struct {
		label string
		lines []string
	}
	var blocks []block
	lastProject := ""
	for i, s := range m.sessions {
		if s.Project != lastProject || len(blocks) == 0 {
			blocks = append(blocks, block{label: "📂 " + s.Project})
			lastProject = s.Project
		}
		cur := &blocks[len(blocks)-1]

		// Tab number: the window index, or "-" when there is no live window.
		// tmux is configured with base-index 1 (see tmux.CLI), so a live window
		// is always >= 1 and 0 reliably means "no window".
		num := "-"
		if s.WindowIndex > 0 {
			num = fmt.Sprintf("%d", s.WindowIndex)
		}
		identity := fmt.Sprintf("%s %s %s  %s ← %s", num, activityIcon(s.Activity), s.Name, s.Branch, s.Base)
		if i == m.cursor {
			cur.lines = append(cur.lines, selectedStyle.Render("› "+identity))
		} else {
			cur.lines = append(cur.lines, "  "+identity)
		}

		// Detail line: activity word, git state, age. Working sessions get the
		// animated spinner frame; other states render plain.
		detail := "    "
		if s.Activity == activity.Working {
			detail += m.spinner.View() + " "
		}
		detail += s.Activity.Label()
		if s.Git.Dirty {
			detail += fmt.Sprintf(" · ✱%d", s.Git.ChangeCount)
		} else {
			detail += " · clean"
		}
		if s.Git.Ahead > 0 || s.Git.Behind > 0 {
			detail += fmt.Sprintf(" · ↑%d↓%d", s.Git.Ahead, s.Git.Behind)
		}
		if !s.CreatedAt.IsZero() {
			detail += " · created " + s.CreatedAt.Format("2006-01-02 15:04")
		}
		cur.lines = append(cur.lines, dimStyle.Render(detail))
	}

	// Uniform inner width: the widest content line, and wide enough that every
	// label fits inside its top border.
	innerWidth := 0
	for _, bl := range blocks {
		if w := lipgloss.Width(bl.label); w > innerWidth {
			innerWidth = w
		}
		for _, ln := range bl.lines {
			if w := lipgloss.Width(ln); w > innerWidth {
				innerWidth = w
			}
		}
	}

	for i, bl := range blocks {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(projectBox(bl.label, bl.lines, innerWidth) + "\n")
	}

	// Legend for the activity glyphs, below the boxes and above the keybinds.
	legend := fmt.Sprintf("legend: %s working  %s waiting  %s idle  %s exited",
		activityIcon(activity.Working), activityIcon(activity.Waiting),
		activityIcon(activity.Idle), activityIcon(activity.Exited))
	b.WriteString("\n" + dimStyle.Render(legend))
	b.WriteString("\n" + dimStyle.Render("n new · enter attach · d cleanup · r refresh · q quit"))
	if m.status != "" {
		b.WriteString("\n" + m.status)
	}
	return b.String()
}
```

Add `"github.com/charmbracelet/lipgloss"` to the `views.go` import block (it currently imports only `"fmt"`, `"strings"`, and the activity package):

```go
import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bray/fleet/internal/activity"
)
```

- [ ] **Step 4: Run the focused test, then the full package**

Run: `go build ./... && go test ./internal/ui/ -v`
Expected: PASS — `TestDashboardWrapsProjectsInBorders`, the pre-existing `TestDashboardShowsGroupingTabNumbersAndLegend` (project name now lives in the top border, branch/tab/states/legend/emoji all still present), and every other `internal/ui` test.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/views.go internal/ui/model_test.go
git commit -m "$(cat <<'EOF'
feat(ui): wrap dashboard project groups in rounded boxes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Full verification

**Files:** none (verification only).

- [ ] **Step 1: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all pass (git/tmux integration tests may SKIP if those binaries are absent — acceptable; no FAILs).

- [ ] **Step 2: Manual smoke (optional, requires git/tmux/claude)**

Run: `go run .`
Expected: each project's sessions sit in a rounded box with `📂 <name>` in the top border, boxes share one width, the legend sits below the boxes and above the `n new · …` footer, and selection/spinner/emoji behave as before.

---

## Self-Review

**1. Spec coverage:**
- Rounded box per project, `📂 <name>` in top border → Task 1 (`projectBox`) + Task 2 (label `"📂 "+project`). ✓
- Uniform width across boxes → Task 2 `innerWidth` computation. ✓
- Legend below boxes, above keybinds → Task 2 ordering + test assertion. ✓
- Manual box drawing (no lipgloss border-title) → Task 1 implementation. ✓
- Width via `lipgloss.Width` → Tasks 1 & 2. ✓
- Content reused verbatim (identity/detail/spinner/selection) → Task 2 keeps the exact construction. ✓
- Existing tests keep passing → asserted in Task 2 Step 4 and Task 3. ✓
- Non-goals (no truncation, no content change) → respected. ✓

**2. Placeholder scan:** No "TBD"/"handle edge cases"/"similar to". Every code step shows complete code; both view functions and the helper are given in full.

**3. Type consistency:** `projectBox(label string, lines []string, innerWidth int) string` defined in Task 1 is called with exactly those argument types in Task 2. `block` is a local type in `viewDashboard`. `lipgloss.Width` used consistently. The `cur := &blocks[len(blocks)-1]` pointer is re-fetched each iteration after any append to `blocks`, so it is never stale.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-06-17-dashboard-project-borders.md`. Two execution options:

**1. Subagent-Driven (recommended)** — fresh subagent per task, review between tasks.

**2. Inline Execution** — execute here in batches with checkpoints.

Which approach?
