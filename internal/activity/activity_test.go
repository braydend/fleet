package activity

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		lastActivity time.Time
		paneTail     string
		missing      bool // no window at all
		dead         bool // window exists but process exited
		want         State
	}{
		{"missing window is exited", time.Time{}, "", true, false, Exited},
		{"dead window is exited", now, "anything", false, true, Exited},
		{"recent output is working", now.Add(-1 * time.Second), "Running tests...", false, false, Working},
		{"quiet with prompt is waiting", now.Add(-30 * time.Second), "Do you want to proceed?\n❯ 1. Yes", false, false, Waiting},
		{"quiet without prompt is idle", now.Add(-30 * time.Second), "all done. 4 passed", false, false, Idle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.lastActivity, now, c.paneTail, c.missing, c.dead)
			if got != c.want {
				t.Fatalf("Classify = %v, want %v", got, c.want)
			}
		})
	}
}

func TestGlyphAndColor(t *testing.T) {
	if Working.Glyph() != "◉" || Exited.Glyph() != "○" {
		t.Fatalf("unexpected glyphs: %q %q", Working.Glyph(), Exited.Glyph())
	}
	if Waiting.TmuxColor() == "" || Working.TmuxColor() == "" {
		t.Fatal("expected non-empty tmux colors for working/waiting")
	}
}
