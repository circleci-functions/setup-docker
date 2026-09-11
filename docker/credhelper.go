package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const credHelperReleasesURL = "https://github.com/docker/docker-credential-helpers/releases" //nolint:gosec // a release URL, not a credential

// the directory the credential helper installs into; unlike the other tools it
// takes no install-dir, matching the orb.
const credHelperBinDir = "/usr/local/bin" //nolint:gosec // an install directory, not a credential

// CredentialHelperOptions configures the Docker credential-helper install.
type CredentialHelperOptions struct {
	Name       string
	Arch       string
	ReleaseTag string
}

// InstallCredentialHelper installs a Docker credential helper (osxkeychain on
// macOS, pass otherwise, unless named) into /usr/local/bin and links it onto
// PATH. Installing pass also provisions its gpg-backed store. A helper already
// on PATH is left in place.
func InstallCredentialHelper(ctx context.Context, opts CredentialHelperOptions) error {
	name := credHelperName(opts.Name, isDarwin())
	filename := "docker-credential-" + name

	if onPath(filename) {
		fmt.Fprintf(os.Stderr, "setup-docker: %s already installed\n", filename)
		return nil
	}

	if name == "pass" {
		if err := setupPass(ctx); err != nil {
			return err
		}
	}

	version := opts.ReleaseTag
	if version == "" {
		resolved, err := resolveLatestFromRedirect(ctx, credHelperReleasesURL+"/latest")
		if err != nil {
			return fmt.Errorf("resolve latest credential helper version: %w", err)
		}
		version = resolved
	}

	platform, arch := credHelperPlatformArch(isDarwin(), opts.Arch)
	url, tarball := credHelperDownloadURL(credHelperReleasesURL, filename, version, platform, arch, minorVersion(version))

	tmp := mustTempDir()
	var src string
	if tarball {
		archive := filepath.Join(tmp, filename+".tar.gz")
		if err := download(ctx, url, archive); err != nil {
			return err
		}
		extracted, err := extractTarGz(archive, tmp)
		if err != nil {
			return err
		}
		found, err := findBinary(extracted, filename)
		if err != nil {
			return err
		}
		src = found
	} else {
		src = filepath.Join(tmp, filename)
		if err := download(ctx, url, src); err != nil {
			return err
		}
	}

	if _, err := placeBinary(ctx, src, credHelperBinDir, filename); err != nil {
		return err
	}
	return linkOnPath(credHelperBinDir, []string{filename}, func(bin string) error {
		return exec.CommandContext(ctx, bin, "version").Run() //nolint:gosec // bin is the credential helper just installed
	})
}

func credHelperName(name string, darwin bool) string {
	if name != "" {
		return name
	}
	if darwin {
		return "osxkeychain"
	}
	return "pass"
}

// credHelperPlatformArch returns the release platform and architecture. On macOS
// the real architecture is used rather than a fixed one, so an Intel mac gets
// the amd64 build.
func credHelperPlatformArch(darwin bool, paramArch string) (platform, arch string) {
	if darwin {
		return "darwin", goArch()
	}
	return "linux", paramArch
}

// credHelperDownloadURL builds the asset URL and reports whether it is a
// tarball. Since v0.7.0 the helper is published as a bare
// <name>-<version>.<platform>-<arch> binary; earlier releases are
// <name>-<version>-<arch>.tar.gz.
func credHelperDownloadURL(base, filename, version, platform, arch string, minor int) (url string, tarball bool) {
	downloadBase := fmt.Sprintf("%s/download/%s/%s-%s", base, version, filename, version)
	if minor < 0 || minor > 6 {
		return fmt.Sprintf("%s.%s-%s", downloadBase, platform, arch), false
	}
	return fmt.Sprintf("%s-%s.tar.gz", downloadBase, arch), true
}

// minorVersion returns the minor component of a semver tag, or -1 if it cannot
// be parsed. A tag whose minor cannot be read is treated as a recent release by
// callers, which use the current asset naming.
func minorVersion(tag string) int {
	parts := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if len(parts) < 2 {
		return -1
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return -1
	}
	return n
}
