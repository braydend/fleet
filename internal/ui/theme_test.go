package ui

import "testing"

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
