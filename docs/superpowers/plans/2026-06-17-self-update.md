# Self-Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user running a released `fleet` binary discover a newer GitHub Release and update in place with a single confirmation.

**Architecture:** A new `internal/selfupdate` package holds all domain logic (version compare, GitHub release check, throttle state, asset selection, checksum verification, archive extraction) behind small interfaces (`HTTPClient`, `Updater`) so it is unit-testable with fakes. The `ui` package fires an async check on startup and hourly, renders a non-intrusive banner, and runs the update through a confirm dialog. The real binary swap is delegated to `github.com/minio/selfupdate`. `main.go` wires the baked-in version and the concrete adapters into UI actions.

**Tech Stack:** Go 1.26, Bubble Tea / Lip Gloss (existing), stdlib `net/http` / `archive/tar` / `compress/gzip` / `crypto/sha256`, new dependency `github.com/minio/selfupdate` v0.6.0.

**Spec:** [`docs/superpowers/specs/2026-06-17-self-update-design.md`](../specs/2026-06-17-self-update-design.md)

## Global Constraints

- **Issue:** #21 — spec + plan committed in the same PR; a comment posted back on #21 linking both artefact paths.
- **Conventional Commits, mandatory.** Feature commits use `feat(selfupdate): ...` / `feat(ui): ...` (minor bump). Test-only or wiring-doc commits may use `test:` / `chore:` but each task below specifies its message.
- **Go module path:** `github.com/bray/fleet`. **GitHub repo (for the API):** `braydend/fleet`.
- **`selfupdate` imports no UI code** and holds no global state; effects go through the `HTTPClient` and `Updater` interfaces.
- **Errors are non-fatal.** A failed/offline check or a failed update is surfaced to the dashboard status line (`m.status`), never panics or blocks the TUI. Network calls run as `tea.Cmd`s.
- **Dev builds skip the check:** when the running version is `"dev"` or unparseable, no network call is made.
- **Release archive naming (from the release-binaries pipeline):** asset `fleet_{version}_{os}_{arch}.tar.gz` (version has no leading `v`; os ∈ {linux,darwin}; arch ∈ {amd64,arm64}); checksum asset `checksums.txt` with `sha256  filename` lines; the archive contains a `fleet` binary entry.
- **TDD:** write the failing test first, watch it fail, implement minimally, watch it pass, commit.
- **Toolchain note:** `go` is on the Homebrew path. If `go` is not found, run `export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"` first (see memory: fleet-toolchain-paths).

---

### Task 1: Version comparison

**Files:**
- Create: `internal/selfupdate/version.go`
- Test: `internal/selfupdate/version_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func IsDev(v string) bool` — true when `v == "dev"` or `v` is not parseable semver.
  - `func Newer(current, latest string) bool` — true when `latest` is a strictly greater semver than `current`; false on any parse failure or when `current` is dev.

- [ ] **Step 1: Write the failing test**

```go
package selfupdate

import "testing"

func TestIsDev(t *testing.T) {
	for _, v := range []string{"dev", "", "garbage", "v"} {
		if !IsDev(v) {
			t.Errorf("IsDev(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"0.1.0", "v0.1.0", "1.2.3"} {
		if IsDev(v) {
			t.Errorf("IsDev(%q) = true, want false", v)
		}
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.1.0", "0.2.0", true},
		{"v0.1.0", "v0.1.1", true},
		{"0.1.0", "1.0.0", true},
		{"0.2.0", "0.2.0", false},
		{"0.2.0", "0.1.9", false},
		{"dev", "0.2.0", false},
		{"0.1.0", "garbage", false},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.latest); got != c.want {
			t.Errorf("Newer(%q,%q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/selfupdate/ -run 'TestIsDev|TestNewer' -v`
Expected: FAIL — undefined `IsDev` / `Newer`.

- [ ] **Step 3: Write minimal implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/selfupdate/ -run 'TestIsDev|TestNewer' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/selfupdate/version.go internal/selfupdate/version_test.go
git commit -m "feat(selfupdate): add semver comparison helpers"
```

---

### Task 2: GitHub release check

**Files:**
- Create: `internal/selfupdate/check.go`
- Test: `internal/selfupdate/check_test.go`

**Interfaces:**
- Consumes: `Newer`, `IsDev` (Task 1).
- Produces:
  - `type HTTPClient interface { Do(*http.Request) (*http.Response, error) }`
  - `type Asset struct { Name string; URL string }`
  - `type Release struct { Version string; Assets []Asset }`
  - `type CheckResult struct { Available bool; Current string; Latest string; Release Release }`
  - `type Checker struct { Repo string; Client HTTPClient; BaseURL string }` (`BaseURL` defaults to `https://api.github.com` when empty).
  - `func (c Checker) Check(current string) (CheckResult, error)` — returns `CheckResult{Available:false}` and **makes no HTTP request** when `IsDev(current)`.

- [ ] **Step 1: Write the failing test**

```go
package selfupdate

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeClient returns a canned response/error for Do.
type fakeClient struct {
	resp   *http.Response
	err    error
	calls  int
	lastURL string
}

func (f *fakeClient) Do(req *http.Request) (*http.Response, error) {
	f.calls++
	f.lastURL = req.URL.String()
	return f.resp, f.err
}

func jsonResp(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
}

const releaseJSON = `{
  "tag_name": "v0.2.0",
  "assets": [
    {"name": "fleet_0.2.0_linux_amd64.tar.gz", "browser_download_url": "https://x/a.tar.gz"},
    {"name": "checksums.txt", "browser_download_url": "https://x/checksums.txt"}
  ]
}`

func TestCheckAvailable(t *testing.T) {
	fc := &fakeClient{resp: jsonResp(releaseJSON)}
	c := Checker{Repo: "braydend/fleet", Client: fc}
	res, err := c.Check("0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Available || res.Latest != "0.2.0" || res.Current != "0.1.0" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(res.Release.Assets) != 2 || res.Release.Assets[0].URL != "https://x/a.tar.gz" {
		t.Fatalf("assets not parsed: %+v", res.Release.Assets)
	}
	if !strings.Contains(fc.lastURL, "/repos/braydend/fleet/releases/latest") {
		t.Fatalf("wrong url: %s", fc.lastURL)
	}
}

func TestCheckUpToDate(t *testing.T) {
	fc := &fakeClient{resp: jsonResp(releaseJSON)}
	res, err := Checker{Repo: "braydend/fleet", Client: fc}.Check("0.2.0")
	if err != nil || res.Available {
		t.Fatalf("expected not available, got %+v err=%v", res, err)
	}
}

func TestCheckDevSkipsNetwork(t *testing.T) {
	fc := &fakeClient{err: io.ErrUnexpectedEOF} // would error if called
	res, err := Checker{Repo: "braydend/fleet", Client: fc}.Check("dev")
	if err != nil {
		t.Fatalf("dev check should not error, got %v", err)
	}
	if res.Available || fc.calls != 0 {
		t.Fatalf("dev check must not hit network: calls=%d res=%+v", fc.calls, res)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/selfupdate/ -run TestCheck -v`
Expected: FAIL — undefined `Checker` / types.

- [ ] **Step 3: Write minimal implementation**

```go
package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPClient is the minimal HTTP surface used by the package; *http.Client
// satisfies it.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Asset is one downloadable file attached to a release.
type Asset struct {
	Name string
	URL  string
}

// Release is the relevant subset of a GitHub release.
type Release struct {
	Version string // normalized, no leading "v"
	Assets  []Asset
}

// CheckResult is the outcome of a check.
type CheckResult struct {
	Available bool
	Current   string
	Latest    string
	Release   Release
}

// Checker queries GitHub's "latest release" endpoint and compares versions.
type Checker struct {
	Repo    string     // e.g. "braydend/fleet"
	Client  HTTPClient // required
	BaseURL string     // defaults to https://api.github.com
}

// Check returns whether a newer release than current is available. When current
// is a dev/unparseable version it returns immediately with no HTTP request.
func (c Checker) Check(current string) (CheckResult, error) {
	if IsDev(current) {
		return CheckResult{Available: false, Current: current}, nil
	}
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	url := fmt.Sprintf("%s/repos/%s/releases/latest", base, c.Repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return CheckResult{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return CheckResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return CheckResult{}, fmt.Errorf("github returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CheckResult{}, err
	}
	latest := strings.TrimPrefix(payload.TagName, "v")
	rel := Release{Version: latest}
	for _, a := range payload.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL})
	}
	return CheckResult{
		Available: Newer(current, latest),
		Current:   current,
		Latest:    latest,
		Release:   rel,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/selfupdate/ -run TestCheck -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/selfupdate/check.go internal/selfupdate/check_test.go
git commit -m "feat(selfupdate): query GitHub latest release and compare versions"
```

---

### Task 3: Throttle state

**Files:**
- Create: `internal/selfupdate/state.go`
- Test: `internal/selfupdate/state_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `const CheckInterval = time.Hour`
  - `type State struct { LastChecked time.Time }` (JSON tag `last_checked`).
  - `func StatePath() string` — `~/.config/fleet/state.json`.
  - `func LoadState(path string) State` — missing/garbage file ⇒ zero `State` (never errors).
  - `func SaveState(path string, s State) error` — creates parent dir as needed.
  - `func (s State) Due(now time.Time) bool` — `now.Sub(LastChecked) >= CheckInterval` (zero time ⇒ always due).

- [ ] **Step 1: Write the failing test**

```go
package selfupdate

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStateDue(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	if !(State{}).Due(now) {
		t.Error("zero state should be due")
	}
	if (State{LastChecked: now.Add(-30 * time.Minute)}).Due(now) {
		t.Error("30m ago should not be due")
	}
	if !(State{LastChecked: now.Add(-90 * time.Minute)}).Due(now) {
		t.Error("90m ago should be due")
	}
}

func TestLoadSaveState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	// Missing file => zero, no error.
	if got := LoadState(path); !got.LastChecked.IsZero() {
		t.Fatalf("missing file should be zero, got %+v", got)
	}
	want := State{LastChecked: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC)}
	if err := SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	got := LoadState(path)
	if !got.LastChecked.Equal(want.LastChecked) {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/selfupdate/ -run 'TestStateDue|TestLoadSaveState' -v`
Expected: FAIL — undefined `State` / `LoadState` / `SaveState`.

- [ ] **Step 3: Write minimal implementation**

```go
package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// CheckInterval is the minimum gap between network checks.
const CheckInterval = time.Hour

// State persists self-update bookkeeping next to fleet's config.
type State struct {
	LastChecked time.Time `json:"last_checked"`
}

// StatePath is the conventional state-file location.
func StatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "fleet", "state.json")
}

// LoadState reads state from path. A missing or unparseable file yields a zero
// State and no error — the check is then treated as due.
func LoadState(path string) State {
	b, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}
	}
	return s
}

// SaveState writes state to path, creating the parent directory if needed.
func SaveState(path string, s State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Due reports whether enough time has elapsed since the last check.
func (s State) Due(now time.Time) bool {
	return now.Sub(s.LastChecked) >= CheckInterval
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/selfupdate/ -run 'TestStateDue|TestLoadSaveState' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/selfupdate/state.go internal/selfupdate/state_test.go
git commit -m "feat(selfupdate): add throttle state for check timing"
```

---

### Task 4: Asset selection and checksum verification

**Files:**
- Create: `internal/selfupdate/asset.go`
- Test: `internal/selfupdate/asset_test.go`

**Interfaces:**
- Consumes: `Release`, `Asset` (Task 2).
- Produces:
  - `func ArchiveName(version string) string` — `fleet_<version>_<GOOS>_<GOARCH>.tar.gz` for the running platform.
  - `var ErrNoAsset = errors.New("no release asset for this platform")`
  - `func selectAssets(rel Release) (archive Asset, checksums Asset, err error)` — finds the platform archive and the `checksums.txt` asset; returns `ErrNoAsset` if either is missing.
  - `func verifyChecksum(archiveName string, archive []byte, checksumsFile []byte) error` — parses `sha256  name` lines, finds `archiveName`, compares the SHA256 of `archive`; returns an error on mismatch or missing entry.

- [ ] **Step 1: Write the failing test**

```go
package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"testing"
)

func TestArchiveName(t *testing.T) {
	want := fmt.Sprintf("fleet_0.2.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if got := ArchiveName("0.2.0"); got != want {
		t.Fatalf("ArchiveName = %q want %q", got, want)
	}
}

func TestSelectAssets(t *testing.T) {
	name := ArchiveName("0.2.0")
	rel := Release{Version: "0.2.0", Assets: []Asset{
		{Name: name, URL: "u-archive"},
		{Name: "checksums.txt", URL: "u-sums"},
	}}
	a, s, err := selectAssets(rel)
	if err != nil || a.URL != "u-archive" || s.URL != "u-sums" {
		t.Fatalf("selectAssets = %+v %+v err=%v", a, s, err)
	}

	_, _, err = selectAssets(Release{Version: "0.2.0", Assets: []Asset{{Name: "checksums.txt"}}})
	if !errors.Is(err, ErrNoAsset) {
		t.Fatalf("expected ErrNoAsset, got %v", err)
	}
}

func TestVerifyChecksum(t *testing.T) {
	archive := []byte("pretend-tarball")
	sum := sha256.Sum256(archive)
	line := fmt.Sprintf("%s  fleet_0.2.0.tar.gz\n%s  other.txt\n",
		hex.EncodeToString(sum[:]), "00")
	if err := verifyChecksum("fleet_0.2.0.tar.gz", archive, []byte(line)); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}
	if err := verifyChecksum("fleet_0.2.0.tar.gz", []byte("tampered"), []byte(line)); err == nil {
		t.Fatal("tampered archive should fail checksum")
	}
	if err := verifyChecksum("missing.tar.gz", archive, []byte(line)); err == nil {
		t.Fatal("missing entry should error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/selfupdate/ -run 'TestArchiveName|TestSelectAssets|TestVerifyChecksum' -v`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Write minimal implementation**

```go
package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// ErrNoAsset means the release has no archive for the running platform (or no
// checksums file).
var ErrNoAsset = errors.New("no release asset for this platform")

// ArchiveName is the release archive filename for the running OS/arch.
func ArchiveName(version string) string {
	return fmt.Sprintf("fleet_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
}

// selectAssets picks the platform archive and the checksums.txt asset.
func selectAssets(rel Release) (archive Asset, checksums Asset, err error) {
	wantArchive := ArchiveName(rel.Version)
	var gotA, gotC bool
	for _, a := range rel.Assets {
		switch a.Name {
		case wantArchive:
			archive, gotA = a, true
		case "checksums.txt":
			checksums, gotC = a, true
		}
	}
	if !gotA || !gotC {
		return Asset{}, Asset{}, fmt.Errorf("%w: want %q + checksums.txt", ErrNoAsset, wantArchive)
	}
	return archive, checksums, nil
}

// verifyChecksum confirms archive's SHA256 matches the entry for archiveName in
// a GoReleaser-style checksums file ("<hex sha256>  <filename>" per line).
func verifyChecksum(archiveName string, archive, checksumsFile []byte) error {
	want := ""
	for _, line := range strings.Split(string(checksumsFile), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archiveName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum entry for %q", archiveName)
	}
	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %q: got %s want %s", archiveName, got, want)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/selfupdate/ -run 'TestArchiveName|TestSelectAssets|TestVerifyChecksum' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/selfupdate/asset.go internal/selfupdate/asset_test.go
git commit -m "feat(selfupdate): select platform asset and verify checksum"
```

---

### Task 5: Extract the fleet binary from the archive

**Files:**
- Create: `internal/selfupdate/extract.go`
- Test: `internal/selfupdate/extract_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func extractBinary(targz []byte) ([]byte, error)` — gunzips and untars `targz`, returning the contents of the entry named `fleet`; errors if absent.

- [ ] **Step 1: Write the failing test**

```go
package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func makeTarGz(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, data := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractBinary(t *testing.T) {
	want := []byte("ELF-pretend-binary")
	tgz := makeTarGz(t, map[string][]byte{"README.md": []byte("hi"), "fleet": want})
	got, err := extractBinary(tgz)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("extracted %q want %q", got, want)
	}

	if _, err := extractBinary(makeTarGz(t, map[string][]byte{"README.md": []byte("hi")})); err == nil {
		t.Fatal("missing fleet entry should error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/selfupdate/ -run TestExtractBinary -v`
Expected: FAIL — undefined `extractBinary`.

- [ ] **Step 3: Write minimal implementation**

```go
package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"path"
)

// extractBinary returns the contents of the "fleet" entry inside a gzip-tar
// archive.
func extractBinary(targz []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(targz))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if path.Base(hdr.Name) == "fleet" && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, errors.New("archive does not contain a fleet binary")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/selfupdate/ -run TestExtractBinary -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/selfupdate/extract.go internal/selfupdate/extract_test.go
git commit -m "feat(selfupdate): extract fleet binary from release archive"
```

---

### Task 6: Applier orchestration (download → verify → swap)

**Files:**
- Create: `internal/selfupdate/apply.go`
- Test: `internal/selfupdate/apply_test.go`

**Interfaces:**
- Consumes: `HTTPClient`, `Release`, `Asset` (Task 2); `selectAssets`, `verifyChecksum`, `ArchiveName` (Task 4); `extractBinary` (Task 5).
- Produces:
  - `type Updater interface { Apply(binary io.Reader) error; CheckPermissions() error }`
  - `type Applier struct { Client HTTPClient; Updater Updater }`
  - `func (a Applier) Apply(rel Release) error` — checks permissions, downloads the archive + checksums, verifies, extracts, and applies the swap. Returns `ErrPermission` (below) when the install dir is not writable; wraps other failures with context.
  - `var ErrPermission = errors.New("install directory is not writable")`
  - `func IsPermission(err error) bool` — `errors.Is(err, ErrPermission)`.
  - `func ManualInstallHint() string` — a copy/paste command for manual install.

- [ ] **Step 1: Write the failing test**

```go
package selfupdate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// urlClient serves canned bodies keyed by request URL.
type urlClient struct{ bodies map[string][]byte }

func (u urlClient) Do(req *http.Request) (*http.Response, error) {
	b, ok := u.bodies[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b))}, nil
}

// fakeUpdater records the binary it was asked to apply.
type fakeUpdater struct {
	applied   []byte
	permErr   error
	applyErr  error
}

func (f *fakeUpdater) CheckPermissions() error { return f.permErr }
func (f *fakeUpdater) Apply(r io.Reader) error {
	if f.applyErr != nil {
		return f.applyErr
	}
	b, _ := io.ReadAll(r)
	f.applied = b
	return nil
}

// buildRelease returns a release whose archive contains bin, plus the matching
// archive bytes and a valid checksums file body.
func buildRelease(t *testing.T) (rel Release, archive []byte, checksums string, bin []byte) {
	t.Helper()
	bin = []byte("new-fleet-binary")
	archive = makeTarGz(t, map[string][]byte{"fleet": bin})
	sum := sha256.Sum256(archive)
	name := ArchiveName("0.2.0")
	checksums = fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), name)
	rel = Release{Version: "0.2.0", Assets: []Asset{
		{Name: name, URL: "https://x/archive"},
		{Name: "checksums.txt", URL: "https://x/sums"},
	}}
	return rel, archive, checksums, bin
}

func TestApplySuccess(t *testing.T) {
	rel, archive, checksums, bin := buildRelease(t)
	client := urlClient{bodies: map[string][]byte{
		"https://x/archive": archive,
		"https://x/sums":    []byte(checksums),
	}}
	up := &fakeUpdater{}
	if err := (Applier{Client: client, Updater: up}).Apply(rel); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(up.applied, bin) {
		t.Fatalf("applied %q want %q", up.applied, bin)
	}
}

func TestApplyPermissionDenied(t *testing.T) {
	rel, _, _, _ := buildRelease(t)
	up := &fakeUpdater{permErr: errors.New("EACCES")}
	err := (Applier{Client: urlClient{}, Updater: up}).Apply(rel)
	if !IsPermission(err) {
		t.Fatalf("expected ErrPermission, got %v", err)
	}
}

func TestApplyChecksumMismatchAborts(t *testing.T) {
	rel, archive, _, _ := buildRelease(t)
	client := urlClient{bodies: map[string][]byte{
		"https://x/archive": archive,
		"https://x/sums":    []byte("deadbeef  " + ArchiveName("0.2.0") + "\n"),
	}}
	up := &fakeUpdater{}
	if err := (Applier{Client: client, Updater: up}).Apply(rel); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if up.applied != nil {
		t.Fatal("must not apply on checksum mismatch")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/selfupdate/ -run TestApply -v`
Expected: FAIL — undefined `Applier` / `Updater` / `IsPermission`.

- [ ] **Step 3: Write minimal implementation**

```go
package selfupdate

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
)

// ErrPermission means the running binary's directory is not writable, so an
// in-place swap can't happen and the user must update manually.
var ErrPermission = errors.New("install directory is not writable")

// IsPermission reports whether err is (or wraps) ErrPermission.
func IsPermission(err error) bool { return errors.Is(err, ErrPermission) }

// ManualInstallHint returns a copy/paste command to update by hand.
func ManualInstallHint() string {
	return fmt.Sprintf(
		"download fleet_<version>_%s_%s.tar.gz from https://github.com/braydend/fleet/releases/latest and replace your fleet binary",
		runtime.GOOS, runtime.GOARCH)
}

// Updater abstracts the atomic binary swap (implemented by minio/selfupdate).
type Updater interface {
	// CheckPermissions reports whether the swap is permitted before downloading.
	CheckPermissions() error
	// Apply replaces the running binary with the contents of binary.
	Apply(binary io.Reader) error
}

// Applier downloads, verifies, and applies a release.
type Applier struct {
	Client  HTTPClient
	Updater Updater
}

// Apply performs the full update for rel. It fails fast on a non-writable
// install dir (returning ErrPermission) and never swaps a binary whose archive
// checksum does not verify.
func (a Applier) Apply(rel Release) error {
	if err := a.Updater.CheckPermissions(); err != nil {
		return fmt.Errorf("%w: %v", ErrPermission, err)
	}
	archive, checksums, err := selectAssets(rel)
	if err != nil {
		return err
	}
	archiveBytes, err := a.download(archive.URL)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}
	sumsBytes, err := a.download(checksums.URL)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}
	if err := verifyChecksum(ArchiveName(rel.Version), archiveBytes, sumsBytes); err != nil {
		return err
	}
	bin, err := extractBinary(archiveBytes)
	if err != nil {
		return err
	}
	if err := a.Updater.Apply(bytesReader(bin)); err != nil {
		if IsPermission(err) {
			return err
		}
		return fmt.Errorf("apply update: %w", err)
	}
	return nil
}

func (a Applier) download(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}
```

Also add the tiny helper (kept separate so `apply.go` has no extra import noise):

```go
// in apply.go
import "bytes"

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
```

> Note: merge the two `import` blocks into one when writing the file; `bytes`, `errors`, `fmt`, `io`, `net/http`, `runtime` are all used.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/selfupdate/ -run TestApply -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/selfupdate/apply.go internal/selfupdate/apply_test.go
git commit -m "feat(selfupdate): orchestrate download, verify, and binary swap"
```

---

### Task 7: minio/selfupdate adapter

**Files:**
- Create: `internal/selfupdate/minio.go`
- Modify: `go.mod`, `go.sum` (add dependency)
- Test: `internal/selfupdate/minio_test.go`

**Interfaces:**
- Consumes: `Updater` interface (Task 6).
- Produces:
  - `type MinioUpdater struct { TargetPath string }` — implements `Updater`. `TargetPath` empty ⇒ the running executable (production); set to a temp path in tests/smoke.
  - Compile-time assertion `var _ Updater = MinioUpdater{}`.

- [ ] **Step 1: Add the dependency**

Run:
```bash
export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"
go get github.com/minio/selfupdate@v0.6.0
```
Expected: `go.mod` gains `github.com/minio/selfupdate v0.6.0`.

- [ ] **Step 2: Write the failing test**

```go
package selfupdate

import "testing"

func TestMinioUpdaterImplementsUpdater(t *testing.T) {
	var u Updater = MinioUpdater{}
	_ = u // compile-time check that MinioUpdater satisfies Updater
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/selfupdate/ -run TestMinioUpdater -v`
Expected: FAIL — undefined `MinioUpdater`.

- [ ] **Step 4: Write minimal implementation**

```go
package selfupdate

import (
	"io"

	"github.com/minio/selfupdate"
)

// MinioUpdater applies updates via github.com/minio/selfupdate, which performs
// an atomic replace with rollback on failure.
type MinioUpdater struct {
	// TargetPath is the file to replace. Empty means the running executable.
	TargetPath string
}

var _ Updater = MinioUpdater{}

func (m MinioUpdater) opts() selfupdate.Options {
	return selfupdate.Options{TargetPath: m.TargetPath}
}

// CheckPermissions reports whether the swap is permitted.
func (m MinioUpdater) CheckPermissions() error {
	o := m.opts()
	return o.CheckPermissions()
}

// Apply replaces the target file with binary.
func (m MinioUpdater) Apply(binary io.Reader) error {
	return selfupdate.Apply(binary, m.opts())
}
```

- [ ] **Step 5: Run test + build to verify**

Run: `go test ./internal/selfupdate/ -run TestMinioUpdater -v && go build ./...`
Expected: PASS and clean build.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/selfupdate/minio.go internal/selfupdate/minio_test.go
git commit -m "feat(selfupdate): add minio/selfupdate adapter for binary swap"
```

---

### Task 8: UI — update messages, commands, and state

**Files:**
- Modify: `internal/ui/commands.go`, `internal/ui/model.go`
- Test: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `selfupdate.CheckResult`, `selfupdate.Release` (Tasks 2).
- Produces (in package `ui`):
  - `Actions.CheckUpdate func() (selfupdate.CheckResult, error)` and `Actions.ApplyUpdate func(selfupdate.Release) error`.
  - messages `updateAvailableMsg{ res selfupdate.CheckResult }`, `updateAppliedMsg{ version string }`.
  - commands `checkUpdate(fn func() (selfupdate.CheckResult, error)) tea.Cmd`, `applyUpdate(fn func(selfupdate.Release) error, rel selfupdate.Release, version string) tea.Cmd`, and `scheduleUpdateCheck() tea.Cmd`.
  - new state `stateUpdateConfirm`; model fields `updateAvailable bool`, `updateRelease selfupdate.Release`, `updateLatest string`.
  - `Init` fires `checkUpdate` + `scheduleUpdateCheck`; an `updateTickMsg` reschedules and re-checks; dashboard key `u` opens `stateUpdateConfirm` when an update is available; `keyUpdateConfirm` applies on `y`/`enter`, cancels on `n`/`esc`.

- [ ] **Step 1: Write the failing test**

```go
// in internal/ui/model_test.go — add these imports to the existing block:
//   "strings", "testing", "github.com/bray/fleet/internal/selfupdate"
// and reuse the file's existing tea.KeyMsg helper wherever `keyMsg(...)` appears.

func availableResult() selfupdate.CheckResult {
	return selfupdate.CheckResult{
		Available: true, Current: "0.1.0", Latest: "0.2.0",
		Release: selfupdate.Release{Version: "0.2.0"},
	}
}

func TestUpdateAvailableSetsBannerState(t *testing.T) {
	m := New(&Actions{}, nil)
	next, _ := m.Update(updateAvailableMsg{res: availableResult()})
	m = next.(Model)
	if !m.updateAvailable || m.updateLatest != "0.2.0" {
		t.Fatalf("update state not set: %+v", m)
	}
}

func TestUpdateNotAvailableLeavesBannerOff(t *testing.T) {
	m := New(&Actions{}, nil)
	res := availableResult()
	res.Available = false
	next, _ := m.Update(updateAvailableMsg{res: res})
	if next.(Model).updateAvailable {
		t.Fatal("banner should stay off when not available")
	}
}

func TestPressingUOpensConfirmWhenAvailable(t *testing.T) {
	m := New(&Actions{}, nil)
	m.updateAvailable = true
	m.updateRelease = selfupdate.Release{Version: "0.2.0"}
	next, _ := m.Update(keyMsg("u"))
	if next.(Model).state != stateUpdateConfirm {
		t.Fatalf("expected stateUpdateConfirm, got %v", next.(Model).state)
	}
}

func TestPressingUDoesNothingWhenNoUpdate(t *testing.T) {
	m := New(&Actions{}, nil)
	next, _ := m.Update(keyMsg("u"))
	if next.(Model).state != stateDashboard {
		t.Fatal("u with no update should stay on dashboard")
	}
}

func TestUpdateConfirmCancel(t *testing.T) {
	m := New(&Actions{}, nil)
	m.updateAvailable = true
	m.state = stateUpdateConfirm
	next, _ := m.Update(keyMsg("n"))
	if next.(Model).state != stateDashboard {
		t.Fatal("n should return to dashboard")
	}
}

func TestUpdateAppliedSetsStatusAndClearsBanner(t *testing.T) {
	m := New(&Actions{}, nil)
	m.updateAvailable = true
	next, _ := m.Update(updateAppliedMsg{version: "0.2.0"})
	m = next.(Model)
	if m.updateAvailable {
		t.Fatal("banner should clear after applying")
	}
	if !strings.Contains(m.status, "0.2.0") || !strings.Contains(m.status, "restart") {
		t.Fatalf("status %q should mention the new version and a restart", m.status)
	}
}
```

> **Reuse existing helpers.** `model_test.go` already constructs `tea.KeyMsg` values for the `keyDashboard` tests — find that helper (it may be a literal `tea.KeyMsg{...}` or a small func) and use the same form here instead of an invented `keyMsg("u")`. Add `"strings"` to the test file's imports for the `strings.Contains` assertions, and `"github.com/bray/fleet/internal/selfupdate"`. Drop the `"errors"` import — it is not used by these tests.

- [ ] **Step 2: Run test to verify it fails**

Run: `export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"; go test ./internal/ui/ -run TestUpdate -v`
Expected: FAIL — undefined `updateAvailableMsg`, `stateUpdateConfirm`, fields, etc.

- [ ] **Step 3: Write minimal implementation**

In `internal/ui/commands.go` add:

```go
import (
	// existing imports plus:
	"github.com/bray/fleet/internal/selfupdate"
)

type updateAvailableMsg struct{ res selfupdate.CheckResult }
type updateAppliedMsg struct{ version string }
type updateTickMsg struct{}

// checkUpdate runs the injected update check off the UI goroutine.
func checkUpdate(fn func() (selfupdate.CheckResult, error)) tea.Cmd {
	return func() tea.Msg {
		if fn == nil {
			return updateAvailableMsg{}
		}
		res, err := fn()
		if err != nil {
			return errorMsg{err: err}
		}
		return updateAvailableMsg{res: res}
	}
}

// applyUpdate downloads + swaps the binary, then reports the new version.
func applyUpdate(fn func(selfupdate.Release) error, rel selfupdate.Release) tea.Cmd {
	return func() tea.Msg {
		if fn == nil {
			return errorMsg{err: nil}
		}
		if err := fn(rel); err != nil {
			return errorMsg{err: err}
		}
		return updateAppliedMsg{version: rel.Version}
	}
}

// scheduleUpdateCheck re-fires an update check after the throttle interval.
func scheduleUpdateCheck() tea.Cmd {
	return tea.Tick(selfupdate.CheckInterval, func(time.Time) tea.Msg { return updateTickMsg{} })
}
```

In `internal/ui/model.go`:

- Add `stateUpdateConfirm` to the `state` const block (after `stateConfirm`).
- Add to `Actions`:
  ```go
  CheckUpdate func() (selfupdate.CheckResult, error)
  ApplyUpdate func(selfupdate.Release) error
  ```
  and import `"github.com/bray/fleet/internal/selfupdate"`.
- Add to `Model`:
  ```go
  updateAvailable bool
  updateRelease   selfupdate.Release
  updateLatest    string
  ```
- In `Init`, add the checks to the batch:
  ```go
  return tea.Batch(refresh(m.actions.Refresh), tick(), m.spinner.Tick,
  	checkUpdate(m.actions.CheckUpdate), scheduleUpdateCheck())
  ```
- In `Update`, add cases:
  ```go
  case updateAvailableMsg:
  	if msg.res.Available {
  		m.updateAvailable = true
  		m.updateRelease = msg.res.Release
  		m.updateLatest = msg.res.Latest
  	}
  	return m, nil
  case updateAppliedMsg:
  	m.updateAvailable = false
  	m.status = fmt.Sprintf("✓ updated to v%s — restart fleet to apply", msg.version)
  	return m, nil
  case updateTickMsg:
  	return m, tea.Batch(checkUpdate(m.actions.CheckUpdate), scheduleUpdateCheck())
  ```
  (add `"fmt"` to the model.go imports.)
- Add a state branch in `handleKey`:
  ```go
  case stateUpdateConfirm:
  	return m.keyUpdateConfirm(msg)
  ```
- In `keyDashboard`, add a case:
  ```go
  case "u":
  	if m.updateAvailable {
  		m.state = stateUpdateConfirm
  	}
  ```
- Add the handler:
  ```go
  func (m Model) keyUpdateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
  	switch msg.String() {
  	case "y", "enter":
  		rel := m.updateRelease
  		m.state = stateDashboard
  		return m, applyUpdate(m.actions.ApplyUpdate, rel)
  	case "n", "esc":
  		m.state = stateDashboard
  	}
  	return m, nil
  }
  ```

- [ ] **Step 4: Run test to verify it passes**

Run: `export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"; go test ./internal/ui/ -run TestUpdate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/commands.go internal/ui/model.go internal/ui/model_test.go
git commit -m "feat(ui): wire self-update check, confirm, and apply flow"
```

---

### Task 9: UI — dashboard banner and confirm view

**Files:**
- Modify: `internal/ui/views.go`
- Test: `internal/ui/model_test.go` (view assertions) — or `internal/ui/views_test.go` if one exists; otherwise add to `model_test.go`.

**Interfaces:**
- Consumes: `Model.updateAvailable`, `Model.updateLatest`, `stateUpdateConfirm` (Task 8).
- Produces: a banner line in `viewDashboard` and a new `viewUpdateConfirm` rendered for `stateUpdateConfirm`.

- [ ] **Step 1: Write the failing test**

```go
// in internal/ui/model_test.go
func TestDashboardShowsUpdateBanner(t *testing.T) {
	m := New(&Actions{}, nil)
	if strings.Contains(m.View(), "update available") {
		t.Fatal("banner should be absent when no update")
	}
	m.updateAvailable = true
	m.updateLatest = "0.2.0"
	out := m.View()
	if !strings.Contains(out, "0.2.0") || !strings.Contains(out, "u") {
		t.Fatalf("dashboard should advertise update + key: %q", out)
	}
}

func TestUpdateConfirmView(t *testing.T) {
	m := New(&Actions{}, nil)
	m.state = stateUpdateConfirm
	m.updateLatest = "0.2.0"
	out := m.View()
	if !strings.Contains(out, "0.2.0") || !strings.Contains(out, "y") {
		t.Fatalf("confirm view should show version + prompt: %q", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"; go test ./internal/ui/ -run 'TestDashboardShowsUpdateBanner|TestUpdateConfirmView' -v`
Expected: FAIL — no banner / `viewUpdateConfirm` not wired.

- [ ] **Step 3: Write minimal implementation**

In `views.go`, in `viewDashboard`, just before the keybinds line, add:

```go
	if m.updateAvailable {
		banner := fmt.Sprintf("⬆ update available: v%s → press u to update", m.updateLatest)
		b.WriteString("\n" + warnStyle.Render(banner))
	}
```

Add a `stateUpdateConfirm` case to `View`:

```go
	case stateUpdateConfirm:
		return m.viewUpdateConfirm()
```

And the view function:

```go
func (m Model) viewUpdateConfirm() string {
	return warnStyle.Render("⬆ update fleet") + "\n\n" +
		fmt.Sprintf("A new version is available: v%s.\n", m.updateLatest) +
		"Download and replace the running binary now? " + dimStyle.Render("(y/n)")
}
```

(`fmt`, `warnStyle`, `dimStyle` are already used in this package.)

- [ ] **Step 4: Run test to verify it passes**

Run: `export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"; go test ./internal/ui/ -v`
Expected: PASS (all ui tests).

- [ ] **Step 5: Commit**

```bash
git add internal/ui/views.go internal/ui/model_test.go
git commit -m "feat(ui): render update banner and confirm dialog"
```

---

### Task 10: Wire selfupdate into main

**Files:**
- Modify: `main.go`

**Interfaces:**
- Consumes: `selfupdate.Checker`, `selfupdate.Applier`, `selfupdate.MinioUpdater`, `selfupdate.{LoadState,SaveState,StatePath,State}` (Tasks 2,3,6,7); `ui.Actions.CheckUpdate`/`ApplyUpdate` (Task 8); package var `version`.
- Produces: the wired `CheckUpdate`/`ApplyUpdate` closures. No new exported API.

- [ ] **Step 1: Add the wiring**

In `main.go`'s `run()`, after `fg := forge.New()` (and before building `actions`), add:

```go
	const repo = "braydend/fleet"
	statePath := selfupdate.StatePath()
	checker := selfupdate.Checker{Repo: repo, Client: http.DefaultClient}
	applier := selfupdate.Applier{Client: http.DefaultClient, Updater: selfupdate.MinioUpdater{}}
```

Add these fields to the `ui.Actions{...}` literal:

```go
		CheckUpdate: func() (selfupdate.CheckResult, error) {
			st := selfupdate.LoadState(statePath)
			if !st.Due(time.Now()) {
				return selfupdate.CheckResult{}, nil // throttled: no network
			}
			res, err := checker.Check(version)
			// Record the attempt regardless of outcome so a hard-down network
			// doesn't cause a tight retry loop.
			_ = selfupdate.SaveState(statePath, selfupdate.State{LastChecked: time.Now()})
			return res, err
		},
		ApplyUpdate: func(rel selfupdate.Release) error {
			if err := applier.Apply(rel); err != nil {
				if selfupdate.IsPermission(err) {
					return fmt.Errorf("can't replace binary (permission denied) — %s", selfupdate.ManualInstallHint())
				}
				return err
			}
			return nil
		},
```

Add imports to `main.go`: `"net/http"` and `"github.com/bray/fleet/internal/selfupdate"`. (`fmt` and `time` are already imported.)

- [ ] **Step 2: Build and vet**

Run:
```bash
export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"
go build ./... && go vet ./...
```
Expected: clean build, no vet errors.

- [ ] **Step 3: Verify the dev-build path manually**

Run: `go run . --version` then `go run .` (Ctrl-C to exit). Because the local build reports `version == "dev"`, `CheckUpdate` performs no network call and no banner appears — confirming dev builds are inert.
Expected: app starts normally; no update banner.

- [ ] **Step 4: Run the whole suite**

Run: `export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"; go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "feat(selfupdate): wire throttled update check and apply into main"
```

---

### Task 11: End-to-end smoke test (build-tagged)

**Files:**
- Create: `internal/selfupdate/smoke_test.go`

**Interfaces:**
- Consumes: `Applier`, `MinioUpdater`, `Release`, `Asset`, `ArchiveName` (Tasks 2,4,6,7).
- Produces: a `//go:build smoke` test that serves a fake release over `httptest` and applies it to a temp `TargetPath` with the **real** `MinioUpdater`, exercising download → verify → extract → atomic swap without touching the test binary.

- [ ] **Step 1: Write the smoke test**

```go
//go:build smoke

package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestSmokeRealSwap runs the full apply path with the real minio/selfupdate
// adapter against a temp target file. Run with:
//   go test -tags smoke -run Smoke ./internal/selfupdate/ -v
func TestSmokeRealSwap(t *testing.T) {
	const version = "0.2.0"
	newBinary := []byte("#!/bin/sh\necho fleet-v2\n")

	// Build the archive + checksums the release would contain.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "fleet", Mode: 0o755, Size: int64(len(newBinary))})
	_, _ = tw.Write(newBinary)
	_ = tw.Close()
	_ = gw.Close()
	tgz := buf.Bytes()
	sum := sha256.Sum256(tgz)
	archiveName := ArchiveName(version)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), archiveName)

	mux := http.NewServeMux()
	mux.HandleFunc("/archive", func(w http.ResponseWriter, _ *http.Request) { w.Write(tgz) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(checksums)) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Seed an old binary at the target path.
	target := filepath.Join(t.TempDir(), "fleet")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	rel := Release{Version: version, Assets: []Asset{
		{Name: archiveName, URL: srv.URL + "/archive"},
		{Name: "checksums.txt", URL: srv.URL + "/sums"},
	}}
	app := Applier{Client: http.DefaultClient, Updater: MinioUpdater{TargetPath: target}}
	if err := app.Apply(rel); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBinary) {
		t.Fatalf("target not swapped: got %q want %q", got, newBinary)
	}
}
```

- [ ] **Step 2: Run the smoke test**

Run: `export PATH="/home/linuxbrew/.linuxbrew/bin:$PATH"; go test -tags smoke -run Smoke ./internal/selfupdate/ -v`
Expected: PASS — the temp target file now contains the new binary bytes.

- [ ] **Step 3: Confirm the default suite still excludes it**

Run: `go test ./internal/selfupdate/`
Expected: PASS, smoke test not run (build tag excludes it).

- [ ] **Step 4: Commit**

```bash
git add internal/selfupdate/smoke_test.go
git commit -m "test(selfupdate): add build-tagged end-to-end swap smoke test"
```

---

### Task 12: Update docs and link the issue

**Files:**
- Modify: `CLAUDE.md` (Status + external CLIs note), `README.md` (mention self-update if an Install section exists).

- [ ] **Step 1: Update CLAUDE.md Status**

Add a sentence to the **Status** section noting self-update is implemented: the app checks GitHub Releases on startup (and hourly), and offers an in-place update via `u` on the dashboard. Note the new `internal/selfupdate` package in the package layout list.

- [ ] **Step 2: README install note (only if an Install/Try-it section exists)**

If `README.md` has the "Install / Try it" section from the release-binaries work, add a line: "Once installed, fleet checks for new releases and can update itself in place — press `u` when the update banner appears." If no such section exists, skip this step (don't invent one).

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md README.md
git commit -m "docs: note self-update in status and readme"
```

- [ ] **Step 4: Link the issue (after the PR is opened)**

Per CLAUDE.md, post a comment back on issue #21 linking the committed artefacts:

```bash
gh issue comment 21 --body "Spec: docs/superpowers/specs/2026-06-17-self-update-design.md
Plan: docs/superpowers/plans/2026-06-17-self-update.md
Implemented in PR <link>."
```

---

## Self-Review

**Spec coverage:**
- Check via stdlib `net/http` + version compare + dev-skip → Tasks 1, 2. ✓
- Throttle / `last_checked` state → Task 3, wired in Task 10. ✓
- Async startup + hourly re-check, banner, confirm dialog → Tasks 8, 9. The spec's "dashboard re-check after ≥60 min" is realized by the hourly `updateTickMsg` self-reschedule (Task 8) plus the throttle gate in `CheckUpdate` (Task 10); the banner only renders on the dashboard (Task 9). ✓
- Asset selection per GOOS/GOARCH, checksum verify before swap, archive extraction → Tasks 4, 5, 6. ✓
- `minio/selfupdate` atomic swap + non-writable-dir fallback to manual command → Tasks 6 (`ErrPermission`, `ManualInstallHint`), 7 (`CheckPermissions`), 10 (fallback message). ✓
- "Updated — restart fleet" message, no auto-restart → Task 8 (`updateAppliedMsg` status). ✓
- Skip entirely when `version == "dev"` → Task 2 (`Checker.Check`), verified in Task 10 Step 3. ✓
- Build-tagged smoke test mirroring `internal/refresher` → Task 11. ✓
- Spec + plan in same PR, issue #21 linked bidirectionally → Task 12; spec header already cites #21. ✓
- Conventional Commits → every task's commit message. ✓

**Placeholder scan:** No TBD/TODO. The only conditional step (Task 12 Step 2 README) is explicitly gated on the section existing, which is a real decision, not a placeholder.

**Type consistency:** `CheckResult`, `Release`, `Asset`, `Checker`, `Applier`, `Updater`, `MinioUpdater`, `State`, `IsPermission`, `ManualInstallHint`, `ArchiveName`, `CheckInterval` are defined once (Tasks 1–7) and referenced consistently in Tasks 8–11. UI symbols `updateAvailableMsg`/`updateAppliedMsg`/`updateTickMsg`/`stateUpdateConfirm`/`checkUpdate`/`applyUpdate`/`scheduleUpdateCheck` are defined in Task 8 and used in Tasks 9–10. `Updater.Apply(io.Reader)` signature matches the fake (Task 6) and `MinioUpdater` (Task 7).
