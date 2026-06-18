package ui

import (
	"strings"
	"testing"

	"github.com/bray/fleet/internal/projects"
)

func TestBranchHintNewBranch(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.branch = "brand-new"
	if got := f.branchHint(); got != "new branch from main" {
		t.Fatalf("got %q", got)
	}
}

func TestBranchHintExistingLocal(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.localBranches = []string{"main", "feature"}
	f.branch = "feature"
	if got := f.branchHint(); !strings.Contains(got, "existing local branch") {
		t.Fatalf("got %q", got)
	}
}

func TestBranchHintRemoteOnly(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.remoteBranches = []string{"feature"}
	f.branch = "feature"
	got := f.branchHint()
	if !strings.Contains(got, "tracks origin/feature") {
		t.Fatalf("got %q", got)
	}
}

func TestBranchHintLocalWinsOverRemote(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.localBranches = []string{"feature"}
	f.remoteBranches = []string{"feature"}
	f.branch = "feature"
	if got := f.branchHint(); !strings.Contains(got, "existing local branch") {
		t.Fatalf("local should win, got %q", got)
	}
}

func TestBranchHintEmptyWhenBlank(t *testing.T) {
	f := newForm(projects.Project{DefaultBranch: "main"})
	f.branch = ""
	if got := f.branchHint(); got != "" {
		t.Fatalf("expected empty hint, got %q", got)
	}
}

func TestViewShowsFetchWarning(t *testing.T) {
	f := newForm(projects.Project{Name: "app", DefaultBranch: "main"})
	f.fetchWarning = "⚠ couldn't fetch from origin — branch list may be stale"
	if !strings.Contains(f.view(), "couldn't fetch from origin") {
		t.Fatal("expected fetch warning in form view")
	}
}
