package login

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/circleci-functions/setup-docker/install"
)

// installCredHelper downloads, verifies and links docker-credential-{name}
// from github.com/docker/docker-credential-helpers. On Linux it also sets up
// the pass credential store.
func installCredHelper(ctx context.Context, ins install.Installer, p install.Platform, name, version string) error {
	if p.OS == "windows" {
		return fmt.Errorf("credentials-store is not supported on Windows")
	}
	if name == "" {
		name = defaultCredHelper(p.OS)
	}
	asset, err := p.CredHelperAsset(name, version)
	if err != nil {
		return err
	}
	base := ins.GitHubURL + "/docker/docker-credential-helpers/releases/download/" + version + "/"
	dest := filepath.Join(ins.CacheDir, "credential-helper", version, "docker-credential-"+name)
	if _, err := ins.InstallBinary(ctx, base+asset, base+"checksums.txt", asset, dest, []string{"docker-credential-" + name}); err != nil {
		return fmt.Errorf("install docker-credential-%s: %w", name, err)
	}
	if name == "pass" {
		if err := setupPass(ctx); err != nil {
			return err
		}
	}
	return nil
}

func defaultCredHelper(goos string) string {
	if goos == "darwin" {
		return "osxkeychain"
	}
	return "pass"
}
