package selfupdate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	applied  []byte
	permErr  error
	applyErr error
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

func TestApplyPermissionDeniedAtSwap(t *testing.T) {
	rel, archive, checksums, _ := buildRelease(t)
	client := urlClient{bodies: map[string][]byte{
		"https://x/archive": archive,
		"https://x/sums":    []byte(checksums),
	}}
	// A permission error surfacing from the real swap (minio returns a raw
	// *os.PathError wrapping EACCES, matched by fs.ErrPermission) must map to
	// ErrPermission so the UI shows the manual-install hint.
	up := &fakeUpdater{applyErr: fs.ErrPermission}
	err := (Applier{Client: client, Updater: up}).Apply(rel)
	if !IsPermission(err) {
		t.Fatalf("swap-time permission error should map to ErrPermission, got %v", err)
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
