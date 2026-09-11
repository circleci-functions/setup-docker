package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ComposeOptions configures the docker-compose install.
type ComposeOptions struct {
	Version    string
	InstallDir string
}

// InstallDockerCompose installs the docker-compose binary into opts.InstallDir
// and links it onto PATH, verifying the download against its published SHA-256
// checksum. A matching version already on PATH is left in place.
func InstallDockerCompose(ctx context.Context, opts ComposeOptions) error {
	version := opts.Version
	if version == "latest" || version == "" {
		resolved, err := resolveLatestFromRedirect(ctx, "https://github.com/docker/compose/releases/latest")
		if err != nil {
			return fmt.Errorf("resolve latest docker-compose version: %w", err)
		}
		version = resolved
	}

	if installedVersionContains(ctx, "docker-compose", version) {
		fmt.Fprintf(os.Stderr, "setup-docker: docker-compose %s already installed\n", version)
		return nil
	}

	release := composeReleaseName()
	base := "https://github.com/docker/compose/releases/download/" + version
	tmp := mustTempDir()

	expected, err := composeExpectedChecksum(ctx, base, release, tmp)
	if err != nil {
		return err
	}

	binPath := filepath.Join(tmp, release)
	if err := download(ctx, base+"/"+release, binPath); err != nil {
		return err
	}
	if err := verifyChecksum(binPath, expected); err != nil {
		return err
	}

	if _, err := placeBinary(ctx, binPath, opts.InstallDir, "docker-compose"); err != nil {
		return err
	}
	return linkOnPath(opts.InstallDir, []string{"docker-compose"}, nil, func(bin string) error {
		return verifyReportsVersion(ctx, bin, version)
	})
}

// composeExpectedChecksum downloads the checksum for release, trying the
// combined checksums.txt first (used since v2.10.0) and the per-asset .sha256
// file as a fallback for older releases.
func composeExpectedChecksum(ctx context.Context, base, release, tmp string) (string, error) {
	sums := filepath.Join(tmp, "checksums.txt")
	if err := download(ctx, base+"/checksums.txt", sums); err == nil {
		if sum, ok := checksumFor(readFile(sums), release); ok {
			return sum, nil
		}
	}
	perAsset := filepath.Join(tmp, release+".sha256")
	if err := download(ctx, base+"/"+release+".sha256", perAsset); err != nil {
		return "", fmt.Errorf("download checksum for %s: %w", release, err)
	}
	if sum, ok := checksumFor(readFile(perAsset), release); ok {
		return sum, nil
	}
	return "", fmt.Errorf("no checksum found for %s", release)
}

func composeReleaseName() string {
	platform := "linux"
	if isDarwin() {
		platform = "darwin"
	}
	return "docker-compose-" + platform + "-" + unameArch()
}

// checksumFor returns the hex SHA-256 for the line naming release. Checksum
// files list one "<hex>  <filename>" pair per line; the filename may carry a
// path or a binary-mode "*" prefix, so it is matched by suffix.
func checksumFor(content, release string) (string, bool) {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if strings.HasSuffix(name, release) {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

func verifyChecksum(path, expected string) error {
	f, err := os.Open(path) //nolint:gosec // path is under a temp dir built by this process
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, expected) {
		return fmt.Errorf("checksum mismatch for %s: got %s, expected %s", filepath.Base(path), got, expected)
	}
	return nil
}
