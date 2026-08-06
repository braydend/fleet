package activity

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	claudeMarkers := []string{"❯ 1.", "Do you want", "(y/n)"}

	cases := []struct {
		name         string
		lastActivity time.Time
		paneTail     string
		markers      []string
		missing      bool // no window at all
		dead         bool // window exists but process exited
		want         State
	}{
		{"missing window is exited", time.Time{}, "", claudeMarkers, true, false, Exited},
		{"dead window is exited", now, "anything", claudeMarkers, false, true, Exited},
		{"recent output is working", now.Add(-1 * time.Second), "Running tests...", claudeMarkers, false, false, Working},
		{"quiet with prompt is waiting", now.Add(-30 * time.Second), "Do you want to proceed?\n❯ 1. Yes", claudeMarkers, false, false, Waiting},
		{"quiet without prompt is idle", now.Add(-30 * time.Second), "all done. 4 passed", claudeMarkers, false, false, Idle},
		// An agent with no known prompt text can never be reported as waiting,
		// however suggestive its pane looks.
		{"no markers is never waiting", now.Add(-30 * time.Second), "Do you want to proceed?", nil, false, false, Idle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.lastActivity, now, c.paneTail, c.markers, c.missing, c.dead)
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

func TestStateRendering(t *testing.T) {
	cases := []struct {
		state State
		glyph string
		label string
	}{
		{Idle, "◉", "idle"},
		{Working, "◉", "working"},
		{Waiting, "◉", "waiting for input"},
		{Exited, "○", "exited"},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			if got := c.state.Glyph(); got != c.glyph {
				t.Errorf("Glyph = %q, want %q", got, c.glyph)
			}
			if got := c.state.Label(); got != c.label {
				t.Errorf("Label = %q, want %q", got, c.label)
			}
			if got := c.state.TmuxColor(); got == "" {
				t.Errorf("TmuxColor = empty, want non-empty")
			}
		})
	}
}
