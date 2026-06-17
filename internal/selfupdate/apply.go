package selfupdate

import (
	"bytes"
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

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
