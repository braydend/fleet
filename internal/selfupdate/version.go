// Package selfupdate checks for and applies fleet binary updates from GitHub
// Releases. All effects (HTTP, binary swap) sit behind interfaces so the logic
// is unit-testable with fakes; the package imports no UI code.
package selfupdate

import (
	"strconv"
	"strings"
)

// parseSemver splits a "MAJOR.MINOR.PATCH" string (optional leading "v") into
// three ints. ok is false if the shape is wrong or any field is non-numeric.
func parseSemver(v string) (parts [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	fields := strings.Split(v, ".")
	if len(fields) != 3 {
		return parts, false
	}
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

// IsDev reports whether v is a local/dev build or otherwise not a real release
// version. Such versions never trigger an update check.
func IsDev(v string) bool {
	if v == "dev" {
		return true
	}
	_, ok := parseSemver(v)
	return !ok
}

// Newer reports whether latest is a strictly greater release than current.
// Any parse failure (including a dev current version) yields false.
func Newer(current, latest string) bool {
	c, okc := parseSemver(current)
	l, okl := parseSemver(latest)
	if !okc || !okl {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
