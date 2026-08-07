package memory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bray/fleet/internal/naming"
)

func TestPathSanitizesProject(t *testing.T) {
	got := Path("/base", "My App")
	want := filepath.Join("/base", naming.Sanitize("My App"), ".fleet-agent")
	if got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestWriteThenRead(t *testing.T) {
	base := t.TempDir()
	path := Path(base, "app")
	if err := Write(path, "opencode"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := Read(path); got != "opencode" {
		t.Fatalf("Read = %q, want %q", got, "opencode")
	}
}

func TestWriteCreatesProjectDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "nested", "whee")
	path := Path(base, "app")
	if err := Write(path, "claude"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestReadMissingReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-project", ".fleet-agent")
	if got := Read(path); got != "" {
		t.Fatalf("Read missing = %q, want empty", got)
	}
}

// A file containing only whitespace holds no value; Read must not return it.
func TestReadWhitespaceOnlyReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app", ".fleet-agent")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("  \n\t"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Read(path); got != "" {
		t.Fatalf("Read whitespace-only = %q, want empty", got)
	}
}
