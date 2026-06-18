// Package git provides a thin adapter over the git CLI behind the Git interface.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Status is the git state of a worktree.
type Status struct {
	Branch      string
	Dirty       bool
	Ahead       int
	Behind      int
	ChangeCount int
}

// Branches is the set of branch names known to a repo: local heads and
// origin-tracked remotes (with the "origin/" prefix stripped, HEAD excluded).
type Branches struct {
	Local  []string
	Remote []string
}

// Git is the set of git operations fleet needs. Consumers depend on this
// interface so they can be tested with fakes.
type Git interface {
	DefaultBranch(repoPath string) (string, error)
	AddWorktree(repoPath, worktreePath, branch, base string) error
	RemoveWorktree(repoPath, worktreePath string, force bool) error
	DeleteBranch(repoPath, branch string, force bool) error
	Status(worktreePath string) (Status, error)
	Push(worktreePath, branch string) error
	IsRepo(path string) bool
	Ignore(worktreePath, pattern string) error
	LocalBranchExists(repoPath, branch string) (bool, error)
	RemoteBranchExists(repoPath, branch string) (bool, error)
	ListBranches(repoPath string) (Branches, error)
	Fetch(repoPath string) error
}

// CLI implements Git by shelling out to the git binary.
type CLI struct{}

// New returns a CLI git adapter.
func New() *CLI { return &CLI{} }

func (c *CLI) git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, errb.String())
	}
	return strings.TrimSpace(out.String()), nil
}

func (c *CLI) DefaultBranch(repoPath string) (string, error) {
	// Prefer the remote HEAD; fall back to the current branch.
	if out, err := c.git(repoPath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		return strings.TrimPrefix(out, "origin/"), nil
	}
	return c.git(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
}

func (c *CLI) AddWorktree(repoPath, worktreePath, branch, base string) error {
	_, err := c.git(repoPath, "worktree", "add", "-b", branch, worktreePath, base)
	return err
}

func (c *CLI) RemoveWorktree(repoPath, worktreePath string, force bool) error {
	args := []string{"worktree", "remove", worktreePath}
	if force {
		args = append(args, "--force")
	}
	_, err := c.git(repoPath, args...)
	return err
}

func (c *CLI) DeleteBranch(repoPath, branch string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := c.git(repoPath, "branch", flag, branch)
	return err
}

func (c *CLI) Status(worktreePath string) (Status, error) {
	out, err := c.git(worktreePath, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(out), nil
}

func (c *CLI) Push(worktreePath, branch string) error {
	_, err := c.git(worktreePath, "push", "-u", "origin", branch)
	return err
}

func (c *CLI) IsRepo(path string) bool {
	_, err := c.git(path, "rev-parse", "--git-dir")
	return err == nil
}

// Ignore adds pattern to the repo's info/exclude (resolved via the common git
// dir, so it applies to the repo and all its worktrees) so fleet's own
// bookkeeping (e.g. ".fleet/") never shows up as an untracked change or gets
// accidentally staged. It is idempotent. ".fleet/" only ever exists inside fleet
// worktrees, so excluding it repo-wide is harmless.
func (c *CLI) Ignore(worktreePath, pattern string) error {
	commonDir, err := c.git(worktreePath, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(worktreePath, commonDir)
	}
	infoDir := filepath.Join(commonDir, "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return err
	}
	excludePath := filepath.Join(infoDir, "exclude")
	if existing, err := os.ReadFile(excludePath); err == nil {
		for _, line := range strings.Split(string(existing), "\n") {
			if strings.TrimSpace(line) == pattern {
				return nil // already excluded
			}
		}
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(pattern + "\n")
	return err
}

// refExists reports whether ref resolves in repoPath. show-ref exits 0 when the
// ref exists, 1 when it does not, and >1 on a real error.
func (c *CLI) refExists(repoPath, ref string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = repoPath
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git show-ref %s: %w", ref, err)
}

func (c *CLI) LocalBranchExists(repoPath, branch string) (bool, error) {
	return c.refExists(repoPath, "refs/heads/"+branch)
}

func (c *CLI) RemoteBranchExists(repoPath, branch string) (bool, error) {
	return c.refExists(repoPath, "refs/remotes/origin/"+branch)
}

// ListBranches returns local head names and origin remote-tracking names
// (prefix stripped, HEAD excluded).
func (c *CLI) ListBranches(repoPath string) (Branches, error) {
	local, err := c.git(repoPath, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return Branches{}, err
	}
	remote, err := c.git(repoPath, "for-each-ref", "--format=%(refname:short)", "refs/remotes/origin")
	if err != nil {
		return Branches{}, err
	}
	var b Branches
	for _, line := range strings.Split(local, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			b.Local = append(b.Local, line)
		}
	}
	for _, line := range strings.Split(remote, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "origin/"))
		if line == "" || line == "HEAD" {
			continue
		}
		b.Remote = append(b.Remote, line)
	}
	return b, nil
}

func (c *CLI) Fetch(repoPath string) error {
	_, err := c.git(repoPath, "fetch", "origin")
	return err
}

// parseStatus reads `git status --porcelain=v2 --branch` output.
func parseStatus(out string) Status {
	var s Status
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			s.Branch = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.ab "):
			// Format: "# branch.ab +A -B"
			fields := strings.Fields(line)
			if len(fields) == 4 {
				s.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[2], "+"))
				s.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[3], "-"))
			}
		case line == "":
			// skip
		case line[0] == '1' || line[0] == '2' || line[0] == 'u' || line[0] == '?':
			s.ChangeCount++
		}
	}
	s.Dirty = s.ChangeCount > 0
	return s
}
