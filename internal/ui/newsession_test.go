package ui

import (
	"strings"
	"testing"

	"github.com/bray/fleet/internal/agent"
	"github.com/bray/fleet/internal/projects"
)

func TestBranchHintNewBranch(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, "")
	f.branch = "brand-new"
	if got := f.branchHint(); got != "new branch from main" {
		t.Fatalf("got %q", got)
	}
}

func TestBranchHintExistingLocal(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, "")
	f.localBranches = []string{"main", "feature"}
	f.branch = "feature"
	if got := f.branchHint(); !strings.Contains(got, "existing local branch") {
		t.Fatalf("got %q", got)
	}
}

func TestBranchHintRemoteOnly(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, "")
	f.remoteBranches = []string{"feature"}
	f.branch = "feature"
	got := f.branchHint()
	if !strings.Contains(got, "tracks origin/feature") {
		t.Fatalf("got %q", got)
	}
}

func TestBranchHintLocalWinsOverRemote(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, "")
	f.localBranches = []string{"feature"}
	f.remoteBranches = []string{"feature"}
	f.branch = "feature"
	if got := f.branchHint(); !strings.Contains(got, "existing local branch") {
		t.Fatalf("local should win, got %q", got)
	}
}

func TestBranchHintEmptyWhenBlank(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, "")
	f.branch = ""
	if got := f.branchHint(); got != "" {
		t.Fatalf("expected empty hint, got %q", got)
	}
}

func TestViewShowsFetchWarning(t *testing.T) {
	f := newForm(projects.Project{Name: "app", DefaultBranch: "main"}, "")
	f.fetchWarning = "⚠ couldn't fetch from origin — branch list may be stale"
	if !strings.Contains(f.view(), "couldn't fetch from origin") {
		t.Fatal("expected fetch warning in form view")
	}
}

func TestNewFormSeedsAgentFromDefault(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, agent.IDOpencode)
	if got := f.selectedAgent().ID; got != agent.IDOpencode {
		t.Fatalf("selectedAgent = %q, want %q", got, agent.IDOpencode)
	}
}

// An empty or unrecognised default is not a startup failure here — config
// validation already rejects a bad value — so the form just starts on the first
// registered agent.
func TestNewFormUnknownDefaultStartsOnFirstAgent(t *testing.T) {
	for _, def := range []string{"", "nonsense"} {
		f := newForm(projects.Project{DefaultBranch: "main"}, def)
		if got := f.selectedAgent().ID; got != agent.All()[0].ID {
			t.Errorf("default %q: selectedAgent = %q, want %q", def, got, agent.All()[0].ID)
		}
	}
}

func TestCycleAgentWrapsBothWays(t *testing.T) {
	n := len(agent.All())
	f := newForm(projects.Project{DefaultBranch: "main"}, agent.IDClaude)

	f.cycleAgent(1)
	if f.agentIndex != 1%n {
		t.Fatalf("after +1 agentIndex = %d, want %d", f.agentIndex, 1%n)
	}
	f.cycleAgent(1) // wraps past the end
	if f.agentIndex != 0 {
		t.Fatalf("after wrapping forward agentIndex = %d, want 0", f.agentIndex)
	}
	f.cycleAgent(-1) // wraps past the start
	if f.agentIndex != n-1 {
		t.Fatalf("after wrapping backward agentIndex = %d, want %d", f.agentIndex, n-1)
	}
}

func TestAgentAvailability(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"}, agent.IDClaude)
	f.available = []bool{true, false}

	f.agentIndex = 0
	if !f.agentAvailable() {
		t.Error("expected the installed agent to be available")
	}
	f.agentIndex = 1
	if f.agentAvailable() {
		t.Error("expected the missing agent to be unavailable")
	}
}

// A zero-value form has not probed PATH. Treat that as "unknown", not
// "missing", so a form built in a test or by a future caller never blocks a
// legitimate create.
func TestAgentAvailableDefaultsToTrueWhenUnprobed(t *testing.T) {
	var f newSessionForm
	if !f.agentAvailable() {
		t.Error("an unprobed form must not report the agent as unavailable")
	}
}

func TestViewShowsAgentAndNotInstalledMarker(t *testing.T) {
	f := newForm(projects.Project{Name: "app", DefaultBranch: "main"}, agent.IDClaude)
	f.available = []bool{true, false}

	if !strings.Contains(f.view(), "‹ claude ›") {
		t.Fatalf("expected the selected agent in the form view:\n%s", f.view())
	}
	f.agentIndex = 1
	got := f.view()
	if !strings.Contains(got, "‹ opencode (not installed) ›") {
		t.Fatalf("expected the not-installed marker:\n%s", got)
	}
}

func TestViewShowsSubmitError(t *testing.T) {
	f := newForm(projects.Project{Name: "app", DefaultBranch: "main"}, agent.IDClaude)
	f.submitError = "⚠ opencode is not on PATH — install it or pick another agent"
	if !strings.Contains(f.view(), "not on PATH") {
		t.Fatal("expected the submit error in the form view")
	}
}
