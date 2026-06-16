package forge

import "testing"

func TestAvailableFalseWhenGhMissing(t *testing.T) {
	g := &GH{lookPath: func(string) (string, error) { return "", errNotFound }}
	if g.Available() {
		t.Fatal("expected Available to be false when gh is missing")
	}
}

func TestAvailableTrueWhenGhPresent(t *testing.T) {
	g := &GH{lookPath: func(string) (string, error) { return "/usr/bin/gh", nil }}
	if !g.Available() {
		t.Fatal("expected Available to be true when gh is present")
	}
}
