package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/circleci-functions/toolkit/pathlink"
)

// GCRHelper installs the pinned docker-credential-gcr, links it onto PATH and
// returns the path of the installed binary.
func (i Installer) GCRHelper(ctx context.Context, p Platform) (string, error) {
	version := DefaultGCRHelperVersion
	asset, err := p.gcrHelperAsset()
	if err != nil {
		return "", err
	}
	store := i.storeDir("gcr-helper", version)
	bin := filepath.Join(store, p.exe("docker-credential-gcr"))
	if _, err := os.Stat(bin); err != nil {
		base := i.GitHubURL + "/GoogleCloudPlatform/docker-credential-gcr/releases/download/v" + version + "/"
		sum, err := i.releaseChecksum(ctx, base+"checksums.txt", asset)
		if err != nil {
			return "", err
		}
		if err := i.fetchVerified(ctx, base+asset, sum, func(f string) error {
			return extractFile(f, p.exe("docker-credential-gcr"), bin)
		}); err != nil {
			return "", err
		}
	}
	_, err = pathlink.Ensure(pathlink.Options{
		Names:     []string{"docker-credential-gcr"},
		FromDir:   store,
		Dir:       i.InstallDir,
		Preferred: pathlink.UserBinDirs(),
	})
	return bin, err
}

func (p Platform) gcrHelperAsset() (string, error) {
	goos, arch, err := p.pick(goOS, goArch)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("docker-credential-gcr_%s_%s-%s.tar.gz", goos, arch, DefaultGCRHelperVersion), nil
}
