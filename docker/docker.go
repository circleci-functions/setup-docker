package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DockerOptions configures the Docker CLI install.
type DockerOptions struct {
	Version    string
	InstallDir string
}

// InstallDocker installs the Docker CLI static binaries into opts.InstallDir and
// links them onto PATH. A matching version already on PATH is left in place.
func InstallDocker(ctx context.Context, opts DockerOptions) error {
	version := opts.Version
	if version == "latest" || version == "" {
		resolved, err := resolveLatestFromRedirect(ctx, "https://github.com/docker/cli/releases/latest")
		if err != nil {
			return fmt.Errorf("resolve latest docker version: %w", err)
		}
		version = resolved
	}
	number := versionNumber(version)

	if installedVersionContains(ctx, "docker", number) {
		fmt.Fprintf(os.Stderr, "setup-docker: docker %s already installed\n", number)
		return nil
	}

	url := dockerDownloadURL(number)
	dl := filepath.Join(mustTempDir(), "docker.tgz")
	if err := download(ctx, url, dl); err != nil {
		return err
	}

	staging, err := os.MkdirTemp("", "docker-extract-")
	if err != nil {
		return err
	}
	extracted, err := extractTarGz(dl, staging)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(extracted))
	for _, src := range extracted {
		name := filepath.Base(src)
		if _, err := placeBinary(ctx, src, opts.InstallDir, name); err != nil {
			return err
		}
		names = append(names, name)
	}
	names = dockerFirst(names)

	return linkOnPath(opts.InstallDir, names, nil, func(bin string) error {
		out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput() //nolint:gosec // bin is the docker binary just installed
		if err != nil {
			return err
		}
		if !strings.Contains(string(out), number) {
			return fmt.Errorf("installed docker reports %q, expected %s", strings.TrimSpace(string(out)), number)
		}
		return nil
	})
}

func dockerDownloadURL(number string) string {
	platform := "linux"
	if isDarwin() {
		platform = "mac"
	}
	return fmt.Sprintf("https://download.docker.com/%s/static/stable/%s/docker-%s.tgz", platform, unameArch(), number)
}

// versionNumber strips a leading v from a release tag, since the Docker download
// path names its tarball by the bare version number.
func versionNumber(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

// dockerFirst reorders names so docker is first, which is the name linkOnPath
// verifies.
func dockerFirst(names []string) []string {
	for i, n := range names {
		if n == "docker" {
			names[0], names[i] = names[i], names[0]
			break
		}
	}
	return names
}

func mustTempDir() string {
	dir, err := os.MkdirTemp("", "setup-docker-")
	if err != nil {
		panic(err)
	}
	return dir
}
