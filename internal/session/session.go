// Package session models a fleet session and manages its lifecycle.
package session

import (
	"time"

	"github.com/bray/fleet/internal/activity"
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
	TmuxName     string // the stable window name inside the workspace
	CreatedAt    time.Time
	Alive        bool // window exists and process is running
	Exited       bool // worktree exists but window is missing or dead
	Activity     activity.State
	LastActivity time.Time
	WindowIndex  int // 1-based tab number; 0 if no live window
	Git          git.Status
}
