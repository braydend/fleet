package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/git"
	"github.com/bray/fleet/internal/projects"
	"github.com/bray/fleet/internal/selfupdate"
	"github.com/bray/fleet/internal/session"
)

// keyMsg builds a tea.KeyMsg for a single rune key, matching the form used
// throughout this file (e.g. tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}).
func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

var errSample = errors.New("boom")

func sample() []session.Session {
	return []session.Session{
		{Project: "app", Name: "a", Branch: "fleet/a", Base: "main", Alive: true,
			Activity: activity.Working, WindowIndex: 1,
			Git: git.Status{Branch: "fleet/a", ChangeCount: 1, Dirty: true}, CreatedAt: time.Unix(1, 0)},
		{Project: "app", Name: "b", Branch: "fleet/b", Base: "develop", Exited: true,
			Activity: activity.Exited, WindowIndex: 0, CreatedAt: time.Unix(2, 0)},
	}
}

func TestDashboardShowsGroupingTabNumbersAndLegend(t *testing.T) {
	m := New(nil, "")
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	out := updated.(Model).View()

	for _, want := range []string{"app", "fleet/a", "← main", "1", "working", "exited", "legend", "🟢", "⚫"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dashboard view missing %q.\n---\n%s", want, out)
		}
	}
}

func TestDashboardWrapsProjectsInBorders(t *testing.T) {
	m := New(nil, "")
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

func TestSessionsUpdatedPopulatesList(t *testing.T) {
	m := New(nil, "")
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	mm := updated.(Model)
	if len(mm.sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(mm.sessions))
	}
	if mm.state != stateDashboard {
		t.Fatalf("expected dashboard state, got %v", mm.state)
	}
	// View must not panic and should mention a branch.
	if out := mm.View(); out == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestQuitKey(t *testing.T) {
	m := New(nil, "")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestErrorMsgSetsStatus(t *testing.T) {
	m := New(nil, "")
	updated, _ := m.Update(errorMsg{err: errSample})
	if got := updated.(Model).status; got == "" {
		t.Fatal("expected status to be set on error")
	}
}

func TestNKeyOpensProjectPicker(t *testing.T) {
	called := false
	a := Actions{Projects: func() ([]projects.Project, error) {
		called = true
		return []projects.Project{{Name: "app", Path: "/code/app", DefaultBranch: "main"}}, nil
	}}
	m := New(&a, "")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	// The command loads projects; run it and feed the result back.
	if cmd == nil {
		t.Fatal("expected a command to load projects")
	}
	msg := cmd()
	updated, _ = updated.(Model).Update(msg)
	mm := updated.(Model)
	if !called {
		t.Fatal("expected Projects action to be called")
	}
	if mm.state != stateProjectPicker || len(mm.projects) != 1 {
		t.Fatalf("expected project picker with 1 project, got state=%v n=%d", mm.state, len(mm.projects))
	}
}

func TestFormSubmitCallsCreate(t *testing.T) {
	var gotName, gotBranch, gotBase string
	a := Actions{Create: func(p projects.Project, name, branch, base string) error {
		gotName, gotBranch, gotBase = name, branch, base
		return nil
	}}
	m := New(&a, "")
	m.state = stateNewSession
	m.form = newSessionForm{
		project:       projects.Project{Name: "app", Path: "/code/app", DefaultBranch: "main"},
		sessionName:   "fix",
		branch:        "fleet/fix",
		branchTouched: true, // user typed an explicit branch
		base:          "main",
		field:         fieldBase, // last field; enter submits
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected create command")
	}
	_ = cmd() // execute create
	if gotName != "fix" || gotBranch != "fleet/fix" || gotBase != "main" {
		t.Fatalf("create got name=%q branch=%q base=%q", gotName, gotBranch, gotBase)
	}
}

func TestBranchDefaultsToSessionName(t *testing.T) {
	f := newForm(projects.Project{Name: "app", DefaultBranch: "main"})
	f.sessionName = "fix bug"
	f.syncBranchDefault()
	if f.branch != "fix_bug" {
		t.Fatalf("branch = %q, want %q (session name, sanitized, no prefix)", f.branch, "fix_bug")
	}
}

// Regression: typing the session name one rune at a time must keep the branch
// tracking the *whole* name, not just the first character.
func TestBranchTracksFullSessionNameWhileTyping(t *testing.T) {
	m := New(nil, "")
	m.state = stateNewSession
	m.form = newForm(projects.Project{Name: "app", DefaultBranch: "main"})
	for _, r := range "fix" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if m.form.sessionName != "fix" {
		t.Fatalf("sessionName = %q, want %q", m.form.sessionName, "fix")
	}
	if m.form.branch != "fix" {
		t.Fatalf("branch = %q, want %q (should track the full session name)", m.form.branch, "fix")
	}
}

// Once the user edits the branch field directly, it must stop auto-tracking the
// session name.
func TestEditedBranchStopsTrackingSessionName(t *testing.T) {
	m := New(nil, "")
	m.state = stateNewSession
	m.form = newForm(projects.Project{Name: "app", DefaultBranch: "main"})
	m.form.field = fieldBranch
	for _, r := range "custom" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	// Switch to the session field and type; branch must not be overwritten.
	m.form.field = fieldSession
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(Model)
	if m.form.branch != "custom" {
		t.Fatalf("branch = %q, want %q (user edit must persist)", m.form.branch, "custom")
	}
}

func TestEnterAttachesSelectedSession(t *testing.T) {
	var attached session.Session
	a := Actions{Attach: func(s session.Session) tea.Cmd {
		attached = s
		return func() tea.Msg { return nil }
	}}
	m := New(&a, "")
	m.sessions = sample()
	m.cursor = 0
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected attach command")
	}
	if attached.Name != "a" {
		t.Fatalf("attached wrong session: %+v", attached)
	}
}

func TestDOpensCleanupMenu(t *testing.T) {
	m := New(nil, "")
	m.sessions = sample()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if updated.(Model).state != stateCleanupMenu {
		t.Fatal("expected cleanup menu state")
	}
}

func TestCleanupLeaveCallsLeave(t *testing.T) {
	left := false
	a := Actions{
		Leave:   func(session.Session) error { left = true; return nil },
		Refresh: func() ([]session.Session, error) { return nil, nil },
	}
	m := New(&a, "")
	m.sessions = sample()
	m.cursor = 0
	m.state = stateCleanupMenu
	m.cleanupChoice = cleanupLeave
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected command")
	}
	_ = cmd()
	if !left {
		t.Fatal("expected Leave to be called")
	}
}

func TestDeleteDirtyRequiresConfirm(t *testing.T) {
	m := New(nil, "")
	m.sessions = sample() // session "a" is dirty
	m.cursor = 0
	m.state = stateCleanupMenu
	m.cleanupChoice = cleanupDelete
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).state != stateConfirm {
		t.Fatal("expected confirm state for dirty delete")
	}
}

func TestRefreshDoesNotLeaveProjectPicker(t *testing.T) {
	m := New(nil, "")
	m.state = stateProjectPicker
	m.projects = []projects.Project{{Name: "app"}}
	// A periodic refresh completing must not yank the user back to the dashboard.
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	mm := updated.(Model)
	if mm.state != stateProjectPicker {
		t.Fatalf("expected to stay in project picker, got state %v", mm.state)
	}
	if len(mm.sessions) != 2 {
		t.Fatalf("expected sessions to still update in the background, got %d", len(mm.sessions))
	}
}

func TestRefreshDoesNotLeaveNewSessionForm(t *testing.T) {
	m := New(nil, "")
	m.state = stateNewSession
	updated, _ := m.Update(sessionsUpdatedMsg{sessions: sample()})
	if updated.(Model).state != stateNewSession {
		t.Fatal("expected to stay in the new-session form during a background refresh")
	}
}

func TestFormSubmitReturnsToDashboard(t *testing.T) {
	a := Actions{Create: func(projects.Project, string, string, string) error { return nil }}
	m := New(&a, "")
	m.state = stateNewSession
	m.form = newSessionForm{
		project:     projects.Project{Name: "app", DefaultBranch: "main"},
		sessionName: "fix",
		branch:      "fleet/fix",
		base:        "main",
		field:       fieldBase,
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(Model).state != stateDashboard {
		t.Fatal("expected form submit to return to the dashboard")
	}
	if cmd == nil {
		t.Fatal("expected a create command")
	}
}

func TestSpinnerTickKeepsStateAndReturnsCmd(t *testing.T) {
	m := New(nil, "")
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
	m := New(nil, "")
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

func TestProjectPickerHasFolderMarkers(t *testing.T) {
	a := Actions{Projects: func() ([]projects.Project, error) {
		return []projects.Project{{Name: "app"}, {Name: "web"}}, nil
	}}
	m := New(&a, "")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	updated, _ := m.Update(cmd())
	out := updated.(Model).View()
	if !strings.Contains(out, "📂 app") || !strings.Contains(out, "📂 web") {
		t.Fatalf("project picker missing folder markers.\n---\n%s", out)
	}
}

func TestCleanupMenuHasEmojiActions(t *testing.T) {
	m := New(nil, "")
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
	m := New(nil, "")
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
	if !strings.Contains(out, "✨") {
		t.Fatalf("form title missing sparkle emoji.\n---\n%s", out)
	}
	if !strings.Contains(out, "new session") {
		t.Fatalf("form title missing \"new session\" text.\n---\n%s", out)
	}
}

func availableResult() selfupdate.CheckResult {
	return selfupdate.CheckResult{
		Available: true, Current: "0.1.0", Latest: "0.2.0",
		Release: selfupdate.Release{Version: "0.2.0"},
	}
}

func TestUpdateAvailableSetsBannerState(t *testing.T) {
	m := New(&Actions{}, "")
	next, _ := m.Update(updateAvailableMsg{res: availableResult()})
	m = next.(Model)
	if !m.updateAvailable || m.updateLatest != "0.2.0" {
		t.Fatalf("update state not set: %+v", m)
	}
}

func TestUpdateNotAvailableLeavesBannerOff(t *testing.T) {
	m := New(&Actions{}, "")
	res := availableResult()
	res.Available = false
	next, _ := m.Update(updateAvailableMsg{res: res})
	if next.(Model).updateAvailable {
		t.Fatal("banner should stay off when not available")
	}
}

func TestPressingUOpensConfirmWhenAvailable(t *testing.T) {
	m := New(&Actions{}, "")
	m.updateAvailable = true
	m.updateRelease = selfupdate.Release{Version: "0.2.0"}
	next, _ := m.Update(keyMsg("u"))
	if next.(Model).state != stateUpdateConfirm {
		t.Fatalf("expected stateUpdateConfirm, got %v", next.(Model).state)
	}
}

func TestPressingUDoesNothingWhenNoUpdate(t *testing.T) {
	m := New(&Actions{}, "")
	next, _ := m.Update(keyMsg("u"))
	if next.(Model).state != stateDashboard {
		t.Fatal("u with no update should stay on dashboard")
	}
}

func TestUpdateConfirmCancel(t *testing.T) {
	m := New(&Actions{}, "")
	m.updateAvailable = true
	m.state = stateUpdateConfirm
	next, _ := m.Update(keyMsg("n"))
	if next.(Model).state != stateDashboard {
		t.Fatal("n should return to dashboard")
	}
}

func TestUpdateAppliedSetsStatusAndClearsBanner(t *testing.T) {
	m := New(&Actions{}, "")
	m.updateAvailable = true
	next, _ := m.Update(updateAppliedMsg{version: "0.2.0"})
	m = next.(Model)
	if m.updateAvailable {
		t.Fatal("banner should clear after applying")
	}
	if !strings.Contains(m.status, "0.2.0") || !strings.Contains(m.status, "restart") {
		t.Fatalf("status %q should mention the new version and a restart", m.status)
	}
}

func TestDashboardShowsUpdateBanner(t *testing.T) {
	m := New(&Actions{}, "")
	if strings.Contains(m.View(), "update available") {
		t.Fatal("banner should be absent when no update")
	}
	m.updateAvailable = true
	m.updateLatest = "0.2.0"
	out := m.View()
	if !strings.Contains(out, "0.2.0") || !strings.Contains(out, "u") {
		t.Fatalf("dashboard should advertise update + key: %q", out)
	}
}

func TestUpdateConfirmView(t *testing.T) {
	m := New(&Actions{}, "")
	m.state = stateUpdateConfirm
	m.updateLatest = "0.2.0"
	out := m.View()
	if !strings.Contains(out, "0.2.0") || !strings.Contains(out, "y") {
		t.Fatalf("confirm view should show version + prompt: %q", out)
	}
}

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
	if strings.Contains(out, "vdev") {
		t.Fatalf("dev build must not be shown with a v prefix.\n---\n%s", out)
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
