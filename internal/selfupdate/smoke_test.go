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
//
//	go test -tags smoke -run Smoke ./internal/selfupdate/ -v
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
