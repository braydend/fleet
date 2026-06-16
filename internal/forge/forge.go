// Package forge wraps the gh CLI to open pull requests.
package forge

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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

// OpenPR runs `gh pr create --fill` in the worktree. On failure it wraps gh's
// stderr so the reason (e.g. "a pull request already exists") reaches the UI.
func (g *GH) OpenPR(worktreePath string) error {
	cmd := exec.Command("gh", "pr", "create", "--fill")
	cmd.Dir = worktreePath
	var errb bytes.Buffer
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("gh pr create: %w: %s", err, msg)
	}
	return nil
}
