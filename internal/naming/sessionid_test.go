package naming

import (
	"regexp"
	"testing"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewClaudeSessionIDFormat(t *testing.T) {
	id := NewClaudeSessionID()
	if !uuidV4.MatchString(id) {
		t.Fatalf("not a v4 UUID: %q", id)
	}
}

func TestNewClaudeSessionIDUnique(t *testing.T) {
	first := NewClaudeSessionID()
	second := NewClaudeSessionID()
	if first == second {
		t.Fatal("expected distinct IDs on successive calls")
	}
}
