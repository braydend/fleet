// Package forge wraps the gh CLI to open pull requests.
package forge

import (
	"errors"
	"os/exec"
)

var errNotFound = errors.New("gh not found")

// PRer can open a pull request for a branch checked out in a worktree.
type PRer interface {
	Available() bool
	OpenPR(worktreePath string) error
}

// GH implements PRer using the gh CLI. lookPath is injectable for testing.
type GH struct {
	lookPath func(string) (string, error)
}

// New returns a GH adapter using the real exec.LookPath.
func New() *GH { return &GH{lookPath: exec.LookPath} }

// Available reports whether the gh binary is on PATH.
func (g *GH) Available() bool {
	lp := g.lookPath
	if lp == nil {
		lp = exec.LookPath
	}
	_, err := lp("gh")
	return err == nil
}

// OpenPR runs `gh pr create --fill` in the worktree, pushing if needed.
func (g *GH) OpenPR(worktreePath string) error {
	cmd := exec.Command("gh", "pr", "create", "--fill")
	cmd.Dir = worktreePath
	return cmd.Run()
}
