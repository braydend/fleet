package projects

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeInspector treats any path ending in "-repo" as a git repo on "main".
type fakeInspector struct{}

func (fakeInspector) IsRepo(path string) bool {
	return filepath.Base(path) == "a-repo" || filepath.Base(path) == "b-repo"
}
func (fakeInspector) DefaultBranch(path string) (string, error) { return "main", nil }

func TestScanFindsImmediateChildRepos(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a-repo", "b-repo", "not-a-repo", "afile"} {
		if name == "afile" {
			if err := os.WriteFile(filepath.Join(root, name), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Scan(root, fakeInspector{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 repos, got %d: %+v", len(got), got)
	}
	if got[0].Name != "a-repo" || got[0].DefaultBranch != "main" {
		t.Fatalf("got %+v", got[0])
	}
}
