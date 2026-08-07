package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/bray/fleet/internal/agent"
)

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

// found and missing build fake exec.LookPath replacements so these tests do
// not depend on what happens to be installed on the machine running them.
func found(string) (string, error) { return "/usr/bin/x", nil }

func missing(name string) func(string) (string, error) {
	return func(bin string) (string, error) {
		if bin == name {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + bin, nil
	}
}

func allAgentsAvailable(agent.Agent) bool { return true }
func noAgentsAvailable(agent.Agent) bool  { return false }

func TestCheckDependenciesMissingGit(t *testing.T) {
	err := checkDependencies(missing("git"), allAgentsAvailable)
	if err == nil || !strings.Contains(err.Error(), "git") {
		t.Fatalf("checkDependencies() = %v, want error naming git", err)
	}
}

func TestCheckDependenciesMissingTmux(t *testing.T) {
	err := checkDependencies(missing("tmux"), allAgentsAvailable)
	if err == nil || !strings.Contains(err.Error(), "tmux") {
		t.Fatalf("checkDependencies() = %v, want error naming tmux", err)
	}
}

// No registered agent installed must fail, and the error must name every
// agent in the registry so the user knows what to install.
func TestCheckDependenciesNoAgentInstalled(t *testing.T) {
	err := checkDependencies(found, noAgentsAvailable)
	if err == nil {
		t.Fatal("checkDependencies() = nil, want an error when no agent is installed")
	}
	for _, id := range agent.IDs() {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("checkDependencies() error %q does not name agent %q", err, id)
		}
	}
}

// Exactly one registered agent installed is enough — which agent drives a
// session is a per-session choice, not a startup requirement.
func TestCheckDependenciesOneAgentInstalled(t *testing.T) {
	onlyOpencode := func(a agent.Agent) bool { return a.ID == agent.IDOpencode }
	if err := checkDependencies(found, onlyOpencode); err != nil {
		t.Fatalf("checkDependencies() = %v, want nil when one agent is installed", err)
	}
}

func TestCheckDependenciesAllAgentsInstalled(t *testing.T) {
	if err := checkDependencies(found, allAgentsAvailable); err != nil {
		t.Fatalf("checkDependencies() = %v, want nil when every agent is installed", err)
	}
}
