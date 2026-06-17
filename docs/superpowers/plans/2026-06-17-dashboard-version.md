# Dashboard Version Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the running `fleet` version in the dashboard footer so the user always knows which version they are on.

**Architecture:** A pure `versionLabel` helper formats the version string; `viewDashboard` appends the label to the existing keybinds footer line. The build version is threaded into the UI by repurposing `ui.New`'s currently-unused second parameter into `version string`, stored on the `Model`, and passed from `main.go`.

**Tech Stack:** Go 1.26, Bubble Tea / Lip Gloss (existing). No new dependencies.

**Spec:** [`docs/superpowers/specs/2026-06-17-dashboard-version-design.md`](../specs/2026-06-17-dashboard-version-design.md)

## Global Constraints

- Display-only; no new keybinding, no interaction, no network/filesystem access.
- Footer label rules (exact): empty string → `""` (omit); `dev` → `dev` (verbatim, no `v` prefix); any other version `X` → `vX`.
- The label is appended to the footer ONLY when non-empty, so a zero-value `Model` (`version == ""`) renders the footer exactly as before this feature.
- Conventional Commits, mandatory. Feature commits use `feat(ui): ...`.
- Match existing code style in `internal/ui` (receiver patterns, `dimStyle`, comment density).
- TDD: failing test first, watch it fail, implement minimally, watch it pass, commit.
- Toolchain: `go` is on the Homebrew path. If `go` is not found, run `export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"` first (see memory: fleet-toolchain-paths). Run all commands from the repo root.

---

### Task 1: `versionLabel` helper

**Files:**
- Modify: `internal/ui/views.go` (add the helper)
- Test: `internal/ui/model_test.go` (add the test)

**Interfaces:**
- Consumes: nothing.
- Produces: `func versionLabel(v string) string` — `""`→`""`, `"dev"`→`"dev"`, otherwise `"v"+v`.

- [ ] **Step 1: Write the failing test**

Add to `internal/ui/model_test.go`:

```go
func TestVersionLabel(t *testing.T) {
	cases := map[string]string{
		"":      "",
		"dev":   "dev",
		"0.2.0": "v0.2.0",
		"1.0.0": "v1.0.0",
	}
	for in, want := range cases {
		if got := versionLabel(in); got != want {
			t.Errorf("versionLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ui/ -run TestVersionLabel -v`
Expected: FAIL — `undefined: versionLabel`.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/ui/views.go` (e.g. just above `viewDashboard`):

```go
// versionLabel formats the build version for display. A dev/local build shows
// "dev" verbatim; a real release X shows "vX"; an empty version shows nothing.
func versionLabel(v string) string {
	switch v {
	case "":
		return ""
	case "dev":
		return "dev"
	default:
		return "v" + v
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ui/ -run TestVersionLabel -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/views.go internal/ui/model_test.go
git commit -m "feat(ui): add versionLabel helper for dashboard version"
```

---

### Task 2: Thread version into the Model and render it in the footer

**Files:**
- Modify: `internal/ui/model.go` (`New` signature + `Model` field)
- Modify: `internal/ui/views.go` (`viewDashboard` footer)
- Modify: `main.go` (pass `version`)
- Test: `internal/ui/model_test.go` (footer view tests + update all `New(...)` call sites)

**Interfaces:**
- Consumes: `versionLabel` (Task 1).
- Produces: `func New(actions *Actions, version string) Model` (replaces `New(actions *Actions, _ any)`); `Model` gains a `version string` field rendered in the dashboard footer.

- [ ] **Step 1: Write the failing tests**

Add to `internal/ui/model_test.go` (`strings` is already imported there):

```go
func TestDashboardFooterShowsReleaseVersion(t *testing.T) {
	m := New(&Actions{}, "0.2.0")
	out := m.View()
	if !strings.Contains(out, "q quit · v0.2.0") {
		t.Fatalf("footer should show release version.\n---\n%s", out)
	}
}

func TestDashboardFooterShowsDevVersion(t *testing.T) {
	m := New(&Actions{}, "dev")
	out := m.View()
	if !strings.Contains(out, "q quit · dev") {
		t.Fatalf("footer should show dev version.\n---\n%s", out)
	}
}

func TestDashboardFooterOmitsEmptyVersion(t *testing.T) {
	m := New(&Actions{}, "")
	out := m.View()
	if !strings.Contains(out, "q quit") {
		t.Fatalf("footer keybinds missing.\n---\n%s", out)
	}
	if strings.Contains(out, "q quit · ") {
		t.Fatalf("empty version should append no label.\n---\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/ -run TestDashboardFooter -v`
Expected: FAIL — these call `New(&Actions{}, "0.2.0")` etc. while `New`'s second parameter is still `any`/the footer has no version. (Compile error on the string arg, or assertion failure.)

- [ ] **Step 3a: Change the `New` signature and add the Model field**

In `internal/ui/model.go`, change the constructor:

```go
// New builds a Model. actions may be the zero value in tests; version is the
// build version string shown in the dashboard footer ("" hides it).
func New(actions *Actions, version string) Model {
	var a Actions
	if actions != nil {
		a = *actions
	}
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = spinnerStyle
	return Model{actions: a, state: stateDashboard, spinner: sp, version: version}
}
```

And add the field to the `Model` struct (near the other top-level fields, e.g. after `status`):

```go
	// version is the build version shown in the dashboard footer.
	version string
```

- [ ] **Step 3b: Render the version in the footer**

In `internal/ui/views.go`, in `viewDashboard`, replace the keybinds line:

```go
	b.WriteString("\n" + dimStyle.Render("n new · enter attach · d cleanup · r refresh · q quit"))
```

with:

```go
	footer := "n new · enter attach · d cleanup · r refresh · q quit"
	if label := versionLabel(m.version); label != "" {
		footer += " · " + label
	}
	b.WriteString("\n" + dimStyle.Render(footer))
```

- [ ] **Step 3c: Pass the version from main.go**

In `main.go`, update the single call site:

```go
	p := tea.NewProgram(ui.New(&actions, version), tea.WithAltScreen())
```

(`version` is the existing package-level var.)

- [ ] **Step 3d: Update all other `New(...)` call sites in the test file**

In `internal/ui/model_test.go`, every existing `New(...)` call passes `nil` as the second argument; that no longer compiles. Change the second argument of each to `""`:
- `New(nil, nil)` → `New(nil, "")`
- `New(&a, nil)` → `New(&a, "")`
- `New(&Actions{}, nil)` → `New(&Actions{}, "")`

(There are ~28 such calls. The new footer tests from Step 1 already pass real version strings and must NOT be changed to `""`.) Confirm none remain:

Run: `grep -n 'New(.*, nil)' internal/ui/model_test.go`
Expected: no output.

- [ ] **Step 4: Run tests + build to verify they pass**

Run:
```bash
go test ./internal/ui/ -v
go build ./...
```
Expected: all ui tests PASS (the three new footer tests plus all pre-existing tests), clean build.

- [ ] **Step 5: Verify formatting and the whole suite**

Run:
```bash
gofmt -l internal/ui/ main.go
go test ./...
```
Expected: `gofmt -l` prints nothing (clean); all packages pass.

- [ ] **Step 6: Commit**

```bash
git add internal/ui/model.go internal/ui/views.go internal/ui/model_test.go main.go
git commit -m "feat(ui): show running version in dashboard footer"
```

---

## Self-Review

**Spec coverage:**
- Footer placement + dim styling → Task 2 Step 3b. ✓
- Label format (`""`/`dev`/`vX`) → Task 1 `versionLabel` + tests. ✓
- Label appended only when non-empty; zero-value Model unchanged → Task 2 Step 3b conditional + `TestDashboardFooterOmitsEmptyVersion`. ✓
- Plumbing: repurpose `New`'s second param into `version string`, Model field, `main.go` passes `version` → Task 2 Steps 3a/3c. ✓
- Call-site updates (`main.go`, UI tests) → Task 2 Steps 3c/3d. ✓
- Tests: `0.2.0`→`v0.2.0`, `dev`→`dev` (not `vdev`), `""`→no label → Task 1 + Task 2 Step 1. ✓ (`TestVersionLabel` covers `dev`→`dev`, distinct from `vdev`; the footer test asserts `q quit · dev` which would fail for `vdev`.)
- Non-goals (no commit/date, no keybinding, banner untouched) → nothing added beyond the above. ✓
- Spec + plan committed in same PR; `feat(ui):` commits → headers + commit messages. ✓

**Placeholder scan:** No TBD/TODO; all steps contain concrete code or exact commands.

**Type consistency:** `versionLabel(string) string` defined in Task 1 and called in Task 2. `New(actions *Actions, version string)` defined in Task 2 Step 3a and used by every call site updated in Steps 3c/3d and by the new tests in Step 1. `Model.version` field set in `New` and read in `viewDashboard`. Consistent throughout.
