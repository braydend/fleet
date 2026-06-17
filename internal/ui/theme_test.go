package ui

import (
	"testing"

	"github.com/bray/fleet/internal/activity"
)

func TestPaletteIsAdaptive(t *testing.T) {
	cases := []struct {
		name        string
		light, dark string
		gotLight    string
		gotDark     string
	}{
		{"accent", "200", "212", accentColor.Light, accentColor.Dark},
		{"working", "28", "42", workingColor.Light, workingColor.Dark},
		{"waiting", "172", "220", waitingColor.Light, waitingColor.Dark},
		{"exited", "248", "238", exitedColor.Light, exitedColor.Dark},
		{"project", "31", "45", projectColor.Light, projectColor.Dark},
	}
	for _, c := range cases {
		if c.gotLight != c.light || c.gotDark != c.dark {
			t.Errorf("%s = {Light:%q Dark:%q}, want {Light:%q Dark:%q}",
				c.name, c.gotLight, c.gotDark, c.light, c.dark)
		}
	}
}

func TestActivityIcon(t *testing.T) {
	cases := []struct {
		state activity.State
		want  string
	}{
		{activity.Working, "🟢"},
		{activity.Waiting, "🟡"},
		{activity.Idle, "💤"},
		{activity.Exited, "⚫"},
	}
	for _, c := range cases {
		if got := activityIcon(c.state); got != c.want {
			t.Errorf("activityIcon(%v) = %q, want %q", c.state, got, c.want)
		}
	}
}

func TestGradientColorsLengthAndEndpoints(t *testing.T) {
	if got := gradientColors(0); got != nil {
		t.Errorf("gradientColors(0) = %v, want nil", got)
	}
	if got := gradientColors(1); len(got) != 1 {
		t.Fatalf("gradientColors(1) len = %d, want 1", len(got))
	}
	cols := gradientColors(5)
	if len(cols) != 5 {
		t.Fatalf("gradientColors(5) len = %d, want 5", len(cols))
	}
	if string(cols[0]) != "#ff79c6" {
		t.Errorf("first colour = %q, want %q", string(cols[0]), "#ff79c6")
	}
	if string(cols[4]) != "#8be9fd" {
		t.Errorf("last colour = %q, want %q", string(cols[4]), "#8be9fd")
	}
}

func TestGradientTitlePreservesText(t *testing.T) {
	// Under the ASCII test profile (see TestMain) Render emits no escape codes,
	// so the visible characters are exactly the input.
	in := "✨ fleet ✨"
	if got := gradientTitle(in); got != in {
		t.Errorf("gradientTitle visible text = %q, want %q", got, in)
	}
	if got := gradientTitle(""); got != "" {
		t.Errorf("gradientTitle(\"\") = %q, want empty", got)
	}
}
