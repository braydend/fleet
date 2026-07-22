package session

import "testing"

func TestLaunchFresh(t *testing.T) {
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
			if got := launchFresh(tt.id, tt.name); got != tt.want {
				t.Fatalf("launchFresh(%q,%q) = %q, want %q", tt.id, tt.name, got, tt.want)
			}
		})
	}
}

func TestLaunchResume(t *testing.T) {
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
			if got := launchResume(tt.id, tt.name); got != tt.want {
				t.Fatalf("launchResume(%q,%q) = %q, want %q", tt.id, tt.name, got, tt.want)
			}
		})
	}
}
