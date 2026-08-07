// Package session models a fleet session and manages its lifecycle.
package session

import (
	"time"

	"github.com/bray/fleet/internal/activity"
	"github.com/bray/fleet/internal/git"
)

// Session is one isolated coding-agent instance: a worktree + tmux session.
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
	Broken       bool // worktree directory exists but git no longer tracks it
	Activity     activity.State
	LastActivity time.Time
	WindowIndex  int // 1-based tab number; 0 if no live window
	Git          git.Status
	// ClaudeSessionID is the stable Claude Code session ID fleet resumes on
	// re-launch. Empty for sessions created before this feature (launch bare).
	ClaudeSessionID string
	// Agent is the ID of the coding agent driving this session (see
	// internal/agent). Empty for sessions created before agents were
	// selectable, which agent.Lookup resolves to Claude Code.
	Agent string
}
