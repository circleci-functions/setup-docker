package docker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// DockerizeOptions configures the dockerize install.
type DockerizeOptions struct {
	Version    string
	InstallDir string
}

// InstallDockerize installs dockerize into opts.InstallDir and links it onto
// PATH. A matching version already on PATH is left in place.
func InstallDockerize(ctx context.Context, opts DockerizeOptions) error {
	version := opts.Version
	if version == "latest" || version == "" {
		resolved, err := resolveLatestFromRedirect(ctx, "https://github.com/jwilder/dockerize/releases/latest")
		if err != nil {
			return fmt.Errorf("resolve latest dockerize version: %w", err)
		}
		version = resolved
	}

	if installedVersionContains(ctx, "dockerize", version) {
		fmt.Fprintf(os.Stderr, "setup-docker: dockerize %s already installed\n", version)
		return nil
	}

	if isAlpine() {
		if err := runElevated(ctx, "apk", "add", "--no-cache", "openssl"); err != nil {
			return fmt.Errorf("install openssl for dockerize: %w", err)
		}
	}

	platform := dockerizePlatform()
	url := dockerizeDownloadURL(version, platform)
	tmp := mustTempDir()
	archive := filepath.Join(tmp, "dockerize.tar.gz")
	if err := download(ctx, url, archive); err != nil {
		return err
	}
	extracted, err := extractTarGz(archive, tmp)
	if err != nil {
		return err
	}

	src, err := findBinary(extracted, "dockerize")
	if err != nil {
		return err
	}
	if _, err := placeBinary(ctx, src, opts.InstallDir, "dockerize"); err != nil {
		return err
	}
	return linkOnPath(opts.InstallDir, []string{"dockerize"}, func(bin string) error {
		return verifyReportsVersion(ctx, bin, version)
	})
}

// dockerizePlatform returns the release's platform-arch token. jwilder/dockerize
// publishes only a darwin-amd64 macOS build, so Apple-Silicon macs use it too.
func dockerizePlatform() string {
	switch {
	case isDarwin():
		return "darwin-amd64"
	case isAlpine():
		return "alpine-linux-amd64"
	default:
		return "linux-" + goArch()
	}
}

func dockerizeDownloadURL(version, platform string) string {
	return fmt.Sprintf("https://github.com/jwilder/dockerize/releases/download/%s/dockerize-%s-%s.tar.gz", version, platform, version)
}

// findBinary returns the extracted path whose base name is want.
func findBinary(paths []string, want string) (string, error) {
	for _, p := range paths {
		if filepath.Base(p) == want {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s not found in archive", want)
}
