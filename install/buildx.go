package install

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func (i Installer) Buildx(ctx context.Context, p Platform, want string) (Result, error) {
	have, _ := installedVersion(ctx, "docker", "buildx", "version")
	v, keep, err := chooseVersion(ctx, want, DefaultBuildxVersion, have, func(ctx context.Context) (string, error) {
		return i.latestRelease(ctx, "docker/buildx")
	})
	if err != nil {
		return Result{}, err
	}
	if keep {
		return Result{Tool: "buildx", Version: v, Kept: true}, nil
	}

	asset, err := p.buildxAsset(v)
	if err != nil {
		return Result{}, err
	}
	base := i.GitHubURL + "/docker/buildx/releases/download/" + v + "/"
	sum, err := i.buildxChecksum(ctx, base, asset)
	if err != nil {
		return Result{}, err
	}

	dest := filepath.Join(i.PluginDir, p.exe("docker-buildx"))
	err = i.fetchVerified(ctx, base+asset, sum, func(f string) error {
		return copyExecutable(f, dest)
	})
	if err != nil {
		return Result{}, err
	}

	if _, lookErr := exec.LookPath("docker"); lookErr == nil {
		if err := i.verifyBuildxOnPath(ctx, v); err != nil {
			return Result{}, err
		}
	}

	return Result{Tool: "buildx", Version: v, Dir: i.PluginDir}, nil
}

// buildxChecksum looks up asset in checksums.txt; recent macOS/Windows releases
// moved signed binaries to checksums-signed.txt, so fall back to that file.
// Recent releases list their signed macOS and Windows binaries in checksums-signed.txt instead.
func (i Installer) buildxChecksum(ctx context.Context, base, asset string) (string, error) {
	sum, err := i.releaseChecksum(ctx, base+"checksums.txt", asset)
	if err == nil {
		return sum, nil
	}
	if !isChecksumNotFound(err) {
		return "", err
	}
	return i.releaseChecksum(ctx, base+"checksums-signed.txt", asset)
}

func (i Installer) verifyBuildxOnPath(ctx context.Context, v string) error {
	out, err := exec.CommandContext(ctx, "docker", "buildx", "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker buildx version: %w", err)
	}
	if got := versionRe.FindString(string(out)); got != strings.TrimPrefix(v, "v") {
		return fmt.Errorf("docker buildx reports %q, not %s", got, v)
	}
	return nil
}
