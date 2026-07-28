// Package cleanup removes worktree directories on a best-effort basis: unlike
// os.RemoveAll, which stops at the first error, it continues past entries it
// cannot delete and reports them. Container-written, root-owned files inside a
// session worktree would otherwise abandon the rest of the tree.
package cleanup

import (
	"os"
	"path/filepath"
)

// maxBlockedSamples caps how many blocked paths are retained for display. A
// blocked vendor/ directory can hold tens of thousands of entries; the count is
// what matters, plus a few examples.
const maxBlockedSamples = 5

// Removal reports what RemoveTree could not delete.
type Removal struct {
	BlockedCount int      // total entries that could not be removed
	Blocked      []string // up to maxBlockedSamples example paths
}

// Complete reports whether the tree was removed in full.
func (r Removal) Complete() bool { return r.BlockedCount == 0 }

func (r *Removal) block(path string) {
	r.BlockedCount++
	if len(r.Blocked) < maxBlockedSamples {
		r.Blocked = append(r.Blocked, path)
	}
}

// RemoveTree deletes root and everything beneath it, continuing past entries it
// cannot remove. A root that does not exist is success. The returned error is
// reserved for a root that exists but cannot be inspected; ordinary permission
// failures are reported in Removal.
func RemoveTree(root string) (Removal, error) {
	var r Removal
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return r, err
	}
	removeEntry(root, info.IsDir(), &r)
	return r, nil
}

// removeEntry deletes one path, recursing post-order into real directories so a
// directory is only unlinked once its children are gone. Symlinks are unlinked
// rather than followed, so a link out of the worktree never endangers its
// target. Returns true when the path is gone.
func removeEntry(path string, isDir bool, r *Removal) bool {
	if isDir {
		entries, err := os.ReadDir(path)
		if err != nil {
			r.block(path)
			return false
		}
		emptied := true
		for _, e := range entries {
			// e.IsDir() is Lstat-based, so a symlink to a directory reports
			// false and is unlinked below rather than descended into.
			if !removeEntry(filepath.Join(path, e.Name()), e.IsDir(), r) {
				emptied = false
			}
		}
		if !emptied {
			r.block(path)
			return false
		}
	}
	if err := os.Remove(path); err != nil {
		r.block(path)
		return false
	}
	return true
}
