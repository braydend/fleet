// Package config loads and validates fleet's user configuration.
package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the user-facing configuration.
type Config struct {
	ScanRoot        string `yaml:"scan_root"`
	WorktreeBaseDir string `yaml:"worktree_base_dir"`
}

// DefaultPath returns the conventional config file location.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "fleet", "config.yaml")
}

// Default returns the built-in defaults (used when the file is absent).
func Default() Config {
	home, _ := os.UserHomeDir()
	return Config{
		WorktreeBaseDir: filepath.Join(home, ".local", "share", "fleet", "worktrees"),
	}
}

// Load reads the YAML file at path, layering it over Default. A missing file is
// not an error — defaults are returned.
func Load(path string) (Config, error) {
	cfg := Default()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate ensures the configuration is usable.
func (c Config) Validate() error {
	if c.ScanRoot == "" {
		return errors.New("scan_root is required: set it in " + DefaultPath())
	}
	if c.WorktreeBaseDir == "" {
		return errors.New("worktree_base_dir is required")
	}
	return nil
}
