package install

import (
	"context"
	"os"
	"path/filepath"
)

func (i Installer) Compose(ctx context.Context, p Platform, want string) (Result, error) {
	have, _ := installedVersion(ctx, "docker", "compose", "version")
	if have == "" {
		have, _ = installedVersion(ctx, "docker-compose", "version")
	}

	v, keep, err := chooseVersion(ctx, want, defaultComposeVersion, have, func(ctx context.Context) (string, error) {
		return i.latestRelease(ctx, "docker/compose")
	})
	if err != nil {
		return Result{}, err
	}

	if keep {
		dir, linkErr := i.linkExistingCompose(ctx, v)
		if linkErr != nil {
			return Result{}, linkErr
		}
		return Result{Tool: "compose", Version: v, Dir: dir, Kept: true}, nil
	}

	asset, err := p.composeAsset()
	if err != nil {
		return Result{}, err
	}
	base := i.GitHubURL + "/docker/compose/releases/download/" + v + "/"
	sum, err := i.releaseChecksum(ctx, base+"checksums.txt", asset)
	if err != nil {
		return Result{}, err
	}

	dest := filepath.Join(i.PluginDir, "docker-compose")
	err = i.fetchVerified(ctx, base+asset, sum, func(f string) error {
		return copyExecutable(f, dest)
	})
	if err != nil {
		return Result{}, err
	}

	dir, err := i.link(ctx, i.PluginDir, v, "docker-compose")
	if err != nil {
		return Result{}, err
	}
	return Result{Tool: "compose", Version: v, Dir: dir}, nil
}

// linkExistingCompose searches PluginDir then SystemPluginDirs for docker-compose and links it.
func (i Installer) linkExistingCompose(ctx context.Context, v string) (string, error) {
	for _, d := range append([]string{i.PluginDir}, i.SystemPluginDirs...) {
		if _, err := os.Stat(filepath.Join(d, "docker-compose")); err == nil {
			return i.link(ctx, d, v, "docker-compose")
		}
	}
	return "", nil
}
