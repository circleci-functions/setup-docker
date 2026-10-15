// Package install provides helpers for downloading and placing Docker binaries.
package install

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Platform holds the OS and architecture identifiers.
type Platform struct{ OS, Arch string }

func Current() Platform {
	return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

var (
	goOS      = map[string]string{"linux": "linux", "darwin": "darwin"}
	dockerOS  = map[string]string{"linux": "linux", "darwin": "mac"}
	goArch    = map[string]string{"amd64": "amd64", "arm64": "arm64"}
	unameArch = map[string]string{"amd64": "x86_64", "arm64": "aarch64"}
	gossArch  = map[string]string{"amd64": "x86_64", "arm64": "arm64"}
)

func (p Platform) pick(osNames, archNames map[string]string) (string, string, error) {
	goos, ok := osNames[p.OS]
	if !ok {
		return "", "", fmt.Errorf("unsupported platform %s/%s", p.OS, p.Arch)
	}
	arch, ok := archNames[p.Arch]
	if !ok {
		return "", "", fmt.Errorf("unsupported platform %s/%s", p.OS, p.Arch)
	}
	return goos, arch, nil
}

// dockerIndexPath returns the path component under DockerURL for this platform,
// e.g. "linux/static/stable/x86_64/".
func (p Platform) dockerIndexPath() (string, error) {
	goos, arch, err := p.pick(dockerOS, unameArch)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/static/stable/%s/", goos, arch), nil
}

// dockerTarballPath returns the full path component for a versioned Docker tarball.
func (p Platform) dockerTarballPath(version string) (string, error) {
	idx, err := p.dockerIndexPath()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%sdocker-%s.tgz", idx, strings.TrimPrefix(version, "v")), nil
}

func (p Platform) buildxAsset(version string) (string, error) {
	goos, arch, err := p.pick(goOS, goArch)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("buildx-%s.%s-%s", version, goos, arch), nil
}

func (p Platform) composeAsset() (string, error) {
	goos, arch, err := p.pick(goOS, unameArch)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("docker-compose-%s-%s", goos, arch), nil
}

func (p Platform) dockerizeAsset(version, alpineReleasePath string) (string, error) {
	goos, arch, err := p.pick(goOS, goArch)
	if err != nil {
		return "", err
	}
	if goos == "linux" {
		if _, err := os.Stat(alpineReleasePath); err == nil {
			goos = "alpine-linux"
		}
	}
	return fmt.Sprintf("dockerize-%s-%s-%s.tar.gz", goos, arch, version), nil
}

func (p Platform) gossAsset(version string) (asset, sums string, err error) {
	goos, arch, err := p.pick(goOS, gossArch)
	if err != nil {
		return "", "", err
	}
	ver := strings.TrimPrefix(version, "v")
	return fmt.Sprintf("goss_%s_%s_%s.tar.gz", ver, goos, arch),
		fmt.Sprintf("goss_%s_SHA256SUMS", ver),
		nil
}

const alpineRelease = "/etc/alpine-release"
