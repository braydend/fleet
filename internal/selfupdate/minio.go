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
