package install

import (
	"context"
	"os"
	"path/filepath"
)

func (i Installer) Dockerize(ctx context.Context, p Platform, want string) (Result, error) {
	have, _ := installedVersion(ctx, "dockerize", "--version")
	version, keep, err := chooseVersion(ctx, want, DefaultDockerizeVersion, have, func(ctx context.Context) (string, error) {
		return i.latestRelease(ctx, "jwilder/dockerize")
	})
	if err != nil {
		return Result{}, err
	}
	if keep {
		return Result{Tool: "dockerize", Version: version, Kept: true}, nil
	}

	store := i.storeDir("dockerize", version)
	bin := filepath.Join(store, "dockerize")
	if _, err := os.Stat(bin); err != nil {
		asset, err := p.dockerizeAsset(version, alpineRelease)
		if err != nil {
			return Result{}, err
		}
		base := i.GitHubURL + "/jwilder/dockerize/releases/download/" + version + "/"
		sum, err := i.releaseChecksum(ctx, base+"checksums.txt", asset)
		if err != nil {
			return Result{}, err
		}
		err = i.fetchVerified(ctx, base+asset, sum, func(f string) error {
			return extractFile(f, "dockerize", bin)
		})
		if err != nil {
			return Result{}, err
		}
	}

	dir, err := i.link(ctx, store, version, "dockerize")
	if err != nil {
		return Result{}, err
	}
	return Result{Tool: "dockerize", Version: version, Dir: dir}, nil
}
