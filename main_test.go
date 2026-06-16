package main

import "testing"

func TestVersionRequested(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", []string{"fleet"}, false},
		{"long flag", []string{"fleet", "--version"}, true},
		{"short flag", []string{"fleet", "-v"}, true},
		{"subcommand", []string{"fleet", "version"}, true},
		{"unrelated", []string{"fleet", "other"}, false},
		{"empty", []string{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionRequested(tc.args); got != tc.want {
				t.Fatalf("versionRequested(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestVersionLine(t *testing.T) {
	want := "fleet dev (none, unknown)"
	if got := versionLine(); got != want {
		t.Fatalf("versionLine() = %q, want %q", got, want)
	}
}
