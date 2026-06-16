package naming

import "testing"

func TestSanitizeReplacesNonAlnum(t *testing.T) {
	if got := Sanitize("my repo.v2"); got != "my_repo_v2" {
		t.Fatalf("got %q", got)
	}
	if got := Sanitize("already_ok1"); got != "already_ok1" {
		t.Fatalf("got %q", got)
	}
}

func TestTmuxNameFormat(t *testing.T) {
	if got := TmuxName("My App", "fix bug"); got != "fleet-My_App-fix_bug" {
		t.Fatalf("got %q", got)
	}
}

func TestParseTmuxName(t *testing.T) {
	project, session, ok := ParseTmuxName("fleet-My_App-fix_bug")
	if !ok || project != "My_App" || session != "fix_bug" {
		t.Fatalf("got project=%q session=%q ok=%v", project, session, ok)
	}
	if _, _, ok := ParseTmuxName("notfleet-x-y"); ok {
		t.Fatal("expected non-fleet name to be rejected")
	}
	if _, _, ok := ParseTmuxName("fleet-onlytwo"); ok {
		t.Fatal("expected malformed name to be rejected")
	}
}

func TestWorktreePathSanitizes(t *testing.T) {
	got := WorktreePath("/base", "My App", "fix bug")
	if got != "/base/My_App/fix_bug" {
		t.Fatalf("got %q", got)
	}
}
