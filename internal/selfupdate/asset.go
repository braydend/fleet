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
