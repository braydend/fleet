package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CheckInterval is the minimum gap between network checks.
const CheckInterval = time.Hour

// State persists self-update bookkeeping next to fleet's config.
type State struct {
	LastChecked time.Time `json:"last_checked"`
}

// StatePath is the conventional state-file location.
func StatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "fleet", "state.json")
}

// LoadState reads state from path. A missing or unparseable file yields a zero
// State and no error — the check is then treated as due.
func LoadState(path string) State {
	b, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}
	}
	return s
}

// SaveState writes state to path, creating the parent directory if needed.
func SaveState(path string, s State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Due reports whether enough time has elapsed since the last check.
func (s State) Due(now time.Time) bool {
	return now.Sub(s.LastChecked) >= CheckInterval
}
