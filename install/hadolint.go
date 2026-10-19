package install

import (
	"context"
	"os"
	"path/filepath"
)

func (i Installer) Hadolint(ctx context.Context, p Platform, want string) (Result, error) {
	have, _ := installedVersion(ctx, "hadolint", "--version")
	version, keep, err := chooseVersion(ctx, want, DefaultHadolintVersion, have, func(ctx context.Context) (string, error) {
		return i.latestRelease(ctx, "hadolint/hadolint")
	})
	if err != nil {
		return Result{}, err
	}
	if keep {
		return Result{Tool: "hadolint", Version: version, Kept: true}, nil
	}

	asset, err := p.hadolintAsset()
	if err != nil {
		return Result{}, err
	}

	store := i.storeDir("hadolint", version)
	bin := filepath.Join(store, p.exe("hadolint"))

	if _, err := os.Stat(bin); err != nil {
		base := i.GitHubURL + "/hadolint/hadolint/releases/download/" + version + "/"
		sum, err := i.releaseChecksum(ctx, base+"checksums.sha256", asset)
		if err != nil {
			return Result{}, err
		}
		if err := i.fetchVerified(ctx, base+asset, sum, func(f string) error {
			return copyExecutable(f, bin)
		}); err != nil {
			return Result{}, err
		}
	}

	dir, err := i.link(ctx, store, version, "hadolint")
	if err != nil {
		return Result{}, err
	}
	return Result{Tool: "hadolint", Version: version, Dir: dir}, nil
}
