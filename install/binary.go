package install

import (
	"context"
	"os"
	"path/filepath"

	"github.com/circleci-functions/toolkit/pathlink"
)

// InstallBinary downloads binURL into dest, verifying its SHA-256 against the
// entry for binName in sumsURL. If dest already exists the download is skipped.
// It then links linkNames from filepath.Dir(dest) onto PATH and returns the dir
// the links were placed in.
func (i Installer) InstallBinary(ctx context.Context, binURL, sumsURL, binName, dest string, linkNames []string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil { //nolint:gosec // install dirs are world-readable by convention
		return "", err
	}
	if _, err := os.Stat(dest); err != nil {
		sum, err := i.releaseChecksum(ctx, sumsURL, binName)
		if err != nil {
			return "", err
		}
		if err := i.fetchVerified(ctx, binURL, sum, func(f string) error {
			return copyExecutable(f, dest)
		}); err != nil {
			return "", err
		}
	}
	if len(linkNames) == 0 {
		return "", nil
	}
	res, err := pathlink.Ensure(pathlink.Options{
		Names:     linkNames,
		FromDir:   filepath.Dir(dest),
		Dir:       i.InstallDir,
		Preferred: pathlink.UserBinDirs(),
	})
	if err != nil {
		return "", err
	}
	return res.Dir, nil
}
