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
