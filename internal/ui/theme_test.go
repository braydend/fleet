package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

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
		{"project", "31", "45", projectColor.Light, projectColor.Dark},
		{"dim", "245", "241", dimColor.Light, dimColor.Dark},
		{"warn", "160", "203", warnColor.Light, warnColor.Dark},
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

func TestProjectBoxStructure(t *testing.T) {
	out := projectBox("📂 app", []string{"line one", "two"}, 12)
	rows := strings.Split(out, "\n")
	if len(rows) != 4 { // top + 2 body + bottom
		t.Fatalf("expected 4 rows, got %d:\n%s", len(rows), out)
	}
	if !strings.HasPrefix(rows[0], "╭") || !strings.HasSuffix(rows[0], "╮") {
		t.Errorf("top row not framed: %q", rows[0])
	}
	if !strings.Contains(rows[0], "📂 app") {
		t.Errorf("label missing from top border: %q", rows[0])
	}
	last := rows[len(rows)-1]
	if !strings.HasPrefix(last, "╰") || !strings.HasSuffix(last, "╯") {
		t.Errorf("bottom row not framed: %q", last)
	}
	for _, r := range rows[1 : len(rows)-1] {
		if !strings.HasPrefix(r, "│") || !strings.HasSuffix(r, "│") {
			t.Errorf("body row not framed: %q", r)
		}
	}
	want := lipgloss.Width(rows[0])
	for _, r := range rows {
		if got := lipgloss.Width(r); got != want {
			t.Errorf("row width mismatch: %q is %d, want %d", r, got, want)
		}
	}
}

func TestProjectBoxOverlongLabelDoesNotPanic(t *testing.T) {
	// innerWidth smaller than the label must not panic and stays framed.
	out := projectBox("📂 a-very-long-project-name", []string{"x"}, 3)
	rows := strings.Split(out, "\n")
	if !strings.HasPrefix(rows[0], "╭") || !strings.HasSuffix(rows[0], "╮") {
		t.Errorf("top row not framed: %q", rows[0])
	}
}
