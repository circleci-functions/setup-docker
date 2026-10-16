package install

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func (i Installer) Goss(ctx context.Context, p Platform, want string) (Result, error) {
	have, _ := installedVersion(ctx, "goss", "--version")
	if have != "" {
		if _, err := exec.LookPath("dgoss"); err != nil {
			have = ""
		}
	}

	version, keep, err := chooseVersion(ctx, want, defaultGossVersion, have, func(ctx context.Context) (string, error) {
		return i.latestRelease(ctx, "goss-org/goss")
	})
	if err != nil {
		return Result{}, err
	}
	if keep {
		return Result{Tool: "goss", Version: version, Kept: true}, nil
	}

	store := i.storeDir("goss", version)
	gossbin := filepath.Join(store, "goss")
	dgossbin := filepath.Join(store, "dgoss")

	if _, err := os.Stat(gossbin); err != nil {
		if err := i.downloadGoss(ctx, p, version, gossbin); err != nil {
			return Result{}, err
		}
	}
	if _, err := os.Stat(dgossbin); err != nil {
		if err := i.downloadDgoss(ctx, version, dgossbin); err != nil {
			return Result{}, err
		}
	}

	dir, err := i.link(ctx, store, version, "goss", "dgoss")
	if err != nil {
		return Result{}, err
	}
	return Result{Tool: "goss", Version: version, Dir: dir}, nil
}

func (p Platform) gossOldAsset() (string, error) {
	goos, arch, err := p.pick(goOS, goArch)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("goss-%s-%s", goos, arch), nil
}

func (i Installer) downloadGoss(ctx context.Context, p Platform, version, bin string) error {
	base := i.GitHubURL + "/goss-org/goss/releases/download/" + version + "/"
	asset, sumsFile, err := p.gossAsset(version)
	if err != nil {
		return err
	}
	sum, err := i.releaseChecksum(ctx, base+sumsFile, asset)
	if err != nil {
		if !isNotFound(err) {
			return err
		}
		// v0.4.9 and earlier publish a raw binary without a tarball.
		return i.downloadGossOldLayout(ctx, p, base, bin)
	}
	return i.fetchVerified(ctx, base+asset, sum, func(f string) error {
		return extractFile(f, "goss", bin)
	})
}

func (i Installer) downloadGossOldLayout(ctx context.Context, p Platform, base, bin string) error {
	asset, err := p.gossOldAsset()
	if err != nil {
		return err
	}
	sum, err := i.releaseChecksum(ctx, base+asset+".sha256", asset)
	if err != nil {
		return err
	}
	return i.fetchVerified(ctx, base+asset, sum, func(f string) error {
		return copyExecutable(f, bin)
	})
}

func (i Installer) downloadDgoss(ctx context.Context, version, bin string) error {
	base := i.GitHubURL + "/goss-org/goss/releases/download/" + version + "/"
	sum, err := i.releaseChecksum(ctx, base+"dgoss.sha256", "dgoss")
	if err != nil {
		return err
	}
	return i.fetchVerified(ctx, base+"dgoss", sum, func(f string) error {
		return copyExecutable(f, bin)
	})
}
