// Package meta reads and writes the per-worktree .fleet/meta.json file, which
// holds the facts about a session that tmux and git cannot infer.
package meta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Meta is the persisted metadata for one session, stored inside its worktree.
type Meta struct {
	Project       string    `json:"project"`
	Session       string    `json:"session"`
	Branch        string    `json:"branch"`
	Base          string    `json:"base"`
	RepoPath      string    `json:"repo_path"`
	CreatedAt     time.Time `json:"created_at"`
	CleanupIntent string    `json:"cleanup_intent,omitempty"`
}

// dir returns the .fleet directory inside a worktree.
func dir(worktree string) string { return filepath.Join(worktree, ".fleet") }

// Path returns the meta.json path inside a worktree.
func Path(worktree string) string { return filepath.Join(dir(worktree), "meta.json") }

// Write serializes m into <worktree>/.fleet/meta.json, creating the directory.
func Write(worktree string, m Meta) error {
	if err := os.MkdirAll(dir(worktree), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeRaw(worktree, b)
}

// writeRaw writes bytes to the meta path (used by tests for malformed input).
func writeRaw(worktree string, b []byte) error {
	if err := os.MkdirAll(dir(worktree), 0o755); err != nil {
		return err
	}
	return os.WriteFile(Path(worktree), b, 0o644)
}

// Read loads and parses <worktree>/.fleet/meta.json.
func Read(worktree string) (Meta, error) {
	b, err := os.ReadFile(Path(worktree))
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, err
	}
	return m, nil
}
