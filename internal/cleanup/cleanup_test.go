package cleanup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveTreeRemovesEverything(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"top.txt", "a/mid.txt", "a/b/deep.txt"} {
		if err := os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rem, err := RemoveTree(root)
	if err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}
	if !rem.Complete() {
		t.Fatalf("expected complete removal, got %+v", rem)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("expected root to be gone")
	}
}

func TestRemoveTreeContinuesPastBlockedSubtree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "wt")
	locked := filepath.Join(root, "vendor")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "pinned.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "removable.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// r-xr-xr-x: contents can be read but not unlinked, like a root-owned
	// directory written by a container.
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	rem, err := RemoveTree(root)
	if err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}
	if rem.Complete() || rem.BlockedCount == 0 {
		t.Fatalf("expected blocked entries, got %+v", rem)
	}
	if len(rem.Blocked) == 0 {
		t.Fatal("expected at least one sample blocked path")
	}
	// The deletable sibling is gone even though vendor/ survived.
	if _, err := os.Stat(filepath.Join(root, "removable.txt")); !os.IsNotExist(err) {
		t.Error("expected removable.txt to be deleted despite the blocked subtree")
	}
	if _, err := os.Stat(filepath.Join(locked, "pinned.txt")); err != nil {
		t.Errorf("expected blocked file to survive: %v", err)
	}
}

func TestRemoveTreeMissingRootIsSuccess(t *testing.T) {
	rem, err := RemoveTree(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("expected missing root to succeed, got %v", err)
	}
	if !rem.Complete() {
		t.Fatalf("expected complete, got %+v", rem)
	}
}

func TestRemoveTreeUnlinksSymlinkWithoutFollowing(t *testing.T) {
	tmp := t.TempDir()
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(tmp, "wt")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}

	rem, err := RemoveTree(root)
	if err != nil {
		t.Fatalf("RemoveTree: %v", err)
	}
	if !rem.Complete() {
		t.Fatalf("expected complete removal, got %+v", rem)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("symlink target must not be touched: %v", err)
	}
}
