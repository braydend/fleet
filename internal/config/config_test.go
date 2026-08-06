package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.WorktreeBaseDir == "" {
		t.Fatal("expected a default worktree base dir")
	}
}

func TestLoadMergesFileOverDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "scan_root: /home/me/code\nworktree_base_dir: /tmp/wt\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ScanRoot != "/home/me/code" || cfg.WorktreeBaseDir != "/tmp/wt" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestTmuxSocketDefaultsToFleet(t *testing.T) {
	// Absent from the file → fleet runs on its own dedicated socket so it can't
	// disturb (or be disturbed by) the user's personal tmux server (issue #5).
	if got := Default().TmuxSocket; got != "fleet" {
		t.Fatalf("Default().TmuxSocket = %q, want \"fleet\"", got)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("scan_root: /code\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.TmuxSocket != "fleet" {
		t.Fatalf("absent tmux_socket should default to \"fleet\", got %q", cfg.TmuxSocket)
	}
}

func TestTmuxSocketOverrides(t *testing.T) {
	cases := map[string]string{
		"tmux_socket: custom\n": "custom", // explicit name
		"tmux_socket: \"\"\n":   "",       // explicit empty → user's default tmux socket
	}
	for body, want := range cases {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte("scan_root: /code\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("load %q: %v", body, err)
		}
		if cfg.TmuxSocket != want {
			t.Fatalf("body %q: TmuxSocket = %q, want %q", body, cfg.TmuxSocket, want)
		}
	}
}

func TestValidateRequiresScanRoot(t *testing.T) {
	if err := (Config{WorktreeBaseDir: "/tmp/wt"}).Validate(); err == nil {
		t.Fatal("expected error when scan_root is empty")
	}
	if err := (Config{ScanRoot: "/code", WorktreeBaseDir: "/tmp/wt"}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultAgentDefaultsToClaude(t *testing.T) {
	if got := Default().DefaultAgent; got != "claude" {
		t.Fatalf("Default().DefaultAgent = %q, want \"claude\"", got)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("scan_root: /code\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DefaultAgent != "claude" {
		t.Fatalf("absent default_agent should default to \"claude\", got %q", cfg.DefaultAgent)
	}
}

func TestDefaultAgentOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("scan_root: /code\ndefault_agent: opencode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.DefaultAgent != "opencode" {
		t.Fatalf("DefaultAgent = %q, want \"opencode\"", cfg.DefaultAgent)
	}
}

// A typo must fail loudly at startup rather than silently launching the wrong
// agent on every new session.
func TestValidateRejectsUnknownDefaultAgent(t *testing.T) {
	cfg := Config{ScanRoot: "/code", WorktreeBaseDir: "/wt", DefaultAgent: "opencoder"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected an error for an unknown default_agent")
	}
	if !strings.Contains(err.Error(), "opencode") || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("error should name the valid agents, got %v", err)
	}
}

func TestValidateAcceptsEmptyDefaultAgent(t *testing.T) {
	cfg := Config{ScanRoot: "/code", WorktreeBaseDir: "/wt"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("an empty default_agent means \"the built-in default\", got %v", err)
	}
}
