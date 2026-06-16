package meta

import (
	"testing"
	"time"
)

func TestWriteThenRead(t *testing.T) {
	dir := t.TempDir()
	in := Meta{
		Project:       "My App",
		Session:       "fix-bug",
		Branch:        "fleet/fix-bug",
		Base:          "main",
		RepoPath:      "/repos/my-app",
		CreatedAt:     time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC),
		CleanupIntent: "delete",
	}
	if err := Write(dir, in); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != in {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, in)
	}
}

func TestReadMissingReturnsError(t *testing.T) {
	if _, err := Read(t.TempDir()); err == nil {
		t.Fatal("expected error reading missing meta")
	}
}

func TestReadMalformedReturnsError(t *testing.T) {
	dir := t.TempDir()
	if err := writeRaw(dir, []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir); err == nil {
		t.Fatal("expected error reading malformed meta")
	}
}
