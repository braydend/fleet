package config

import (
	"os"
	"path/filepath"
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

func TestValidateRequiresScanRoot(t *testing.T) {
	if err := (Config{WorktreeBaseDir: "/tmp/wt"}).Validate(); err == nil {
		t.Fatal("expected error when scan_root is empty")
	}
	if err := (Config{ScanRoot: "/code", WorktreeBaseDir: "/tmp/wt"}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
