package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupWritesConfigFromInput(t *testing.T) {
	scanDir := t.TempDir()
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	var out strings.Builder

	cfg, err := Setup(cfgPath, strings.NewReader(scanDir+"\n"), &out)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if cfg.ScanRoot != scanDir {
		t.Fatalf("scan root = %q, want %q", cfg.ScanRoot, scanDir)
	}
	if cfg.WorktreeBaseDir == "" {
		t.Fatal("expected a default worktree base dir")
	}
	if !Exists(cfgPath) {
		t.Fatal("expected config file to be written")
	}
	// The written file must load back to the same values.
	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ScanRoot != scanDir {
		t.Fatalf("loaded scan root = %q, want %q", loaded.ScanRoot, scanDir)
	}
}

func TestSetupExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sub := filepath.Join(home, "code")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	var out strings.Builder

	cfg, err := Setup(cfgPath, strings.NewReader("~/code\n"), &out)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if cfg.ScanRoot != sub {
		t.Fatalf("scan root = %q, want %q", cfg.ScanRoot, sub)
	}
}

func TestSetupEmptyInputErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	var out strings.Builder
	if _, err := Setup(cfgPath, strings.NewReader("\n"), &out); err == nil {
		t.Fatal("expected error on empty input")
	}
	if Exists(cfgPath) {
		t.Fatal("config file should not be written when setup fails")
	}
}

func TestSetupNonexistentDirErrors(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	var out strings.Builder
	if _, err := Setup(cfgPath, strings.NewReader("/no/such/dir/here\n"), &out); err == nil {
		t.Fatal("expected error for a nonexistent scan_root")
	}
	if Exists(cfgPath) {
		t.Fatal("config file should not be written when validation fails")
	}
}
