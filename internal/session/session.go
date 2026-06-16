// Package session models a fleet session and manages its lifecycle.
package session

import (
	"time"

	"github.com/bray/fleet/internal/git"
)

// Session is one isolated Claude Code instance: a worktree + tmux session.
type Session struct {
	Project      string
	Name         string
	Branch       string
	Base         string
	RepoPath     string
	WorktreePath string
	TmuxName     string
	CreatedAt    time.Time
	Alive        bool // tmux session currently exists
	Exited       bool // worktree exists but tmux session is gone
	Git          git.Status
}
