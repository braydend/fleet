package selfupdate

import "testing"

func TestMinioUpdaterImplementsUpdater(t *testing.T) {
	var u Updater = MinioUpdater{}
	_ = u // compile-time check that MinioUpdater satisfies Updater
}
