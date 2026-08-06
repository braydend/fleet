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

func TestWriteThenReadIncludesClaudeSessionID(t *testing.T) {
	dir := t.TempDir()
	in := Meta{
		Project:         "My App",
		Session:         "fix-bug",
		Branch:          "fleet/fix-bug",
		Base:            "main",
		RepoPath:        "/repos/my-app",
		CreatedAt:       time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC),
		CleanupIntent:   "delete",
		ClaudeSessionID: "6f9619ff-8b86-4d01-b42d-00cf4fc964ff",
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

func TestReadLegacyMetaWithoutSessionID(t *testing.T) {
	dir := t.TempDir()
	// A meta.json written before this field existed must still read cleanly.
	legacy := `{"project":"p","session":"s","branch":"b","base":"main","repo_path":"/r","created_at":"2026-06-16T10:00:00Z"}`
	if err := writeRaw(dir, []byte(legacy)); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.ClaudeSessionID != "" {
		t.Fatalf("legacy meta should have empty ClaudeSessionID, got %q", got.ClaudeSessionID)
	}
}

func TestWriteReadRoundTripsAgent(t *testing.T) {
	wt := t.TempDir()
	if err := Write(wt, Meta{Project: "p", Session: "s", Agent: "opencode"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Read(wt)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Agent != "opencode" {
		t.Fatalf("Agent = %q, want %q", got.Agent, "opencode")
	}
}

// Sessions created before agents were selectable have no "agent" key. They must
// read back as empty, which callers resolve to Claude Code.
func TestReadLegacyMetaHasNoAgent(t *testing.T) {
	wt := t.TempDir()
	body := []byte(`{"project":"p","session":"s","branch":"b","base":"main"}`)
	if err := writeRaw(wt, body); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Read(wt)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Agent != "" {
		t.Fatalf("legacy Agent = %q, want empty", got.Agent)
	}
}
