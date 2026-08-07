package agent

import "testing"

func TestClaudeFresh(t *testing.T) {
	tests := []struct {
		desc, id, name, want string
	}{
		{"legacy empty id", "", "p/s", "claude"},
		{"id and name", "abc-123", "My App/fix bug",
			`claude --session-id abc-123 -n 'My App/fix bug'`},
		{"name with single quote", "abc-123", "o'brien/x",
			`claude --session-id abc-123 -n 'o'\''brien/x'`},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := claudeFresh(tt.id, tt.name); got != tt.want {
				t.Fatalf("claudeFresh(%q,%q) = %q, want %q", tt.id, tt.name, got, tt.want)
			}
		})
	}
}

func TestClaudeResume(t *testing.T) {
	tests := []struct {
		desc, id, name, want string
	}{
		{"legacy empty id", "", "p/s", "claude"},
		{"id and name", "abc-123", "My App/fix bug",
			`claude --resume abc-123 || claude --session-id abc-123 -n 'My App/fix bug' || claude`},
		{"name with single quote", "abc-123", "o'brien/x",
			`claude --resume abc-123 || claude --session-id abc-123 -n 'o'\''brien/x' || claude`},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			if got := claudeResume(tt.id, tt.name); got != tt.want {
				t.Fatalf("claudeResume(%q,%q) = %q, want %q", tt.id, tt.name, got, tt.want)
			}
		})
	}
}

// opencode mints its own session IDs, so fleet passes it nothing and resumes by
// continuing the last session for the worktree.
func TestOpencodeCommandsIgnoreIDAndName(t *testing.T) {
	if got := opencodeFresh("ignored", "p/s"); got != "opencode" {
		t.Fatalf("opencodeFresh = %q, want %q", got, "opencode")
	}
	want := "opencode --continue || opencode"
	if got := opencodeResume("ignored", "p/s"); got != want {
		t.Fatalf("opencodeResume = %q, want %q", got, want)
	}
}

// The registry must expose the same builders the tests above cover.
func TestRegistryWiresBuilders(t *testing.T) {
	if got := Lookup(IDClaude).Fresh("sid", "p/s"); got != `claude --session-id sid -n 'p/s'` {
		t.Fatalf("claude Fresh = %q", got)
	}
	if got := Lookup(IDOpencode).Resume("", "p/s"); got != "opencode --continue || opencode" {
		t.Fatalf("opencode Resume = %q", got)
	}
}
