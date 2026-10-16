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
	goOS      = map[string]string{"linux": "linux", "darwin": "darwin", "windows": "windows"}
	dockerOS  = map[string]string{"linux": "linux", "darwin": "mac", "windows": "win"}
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

func (p Platform) exe(name string) string {
	if p.OS == "windows" {
		return name + ".exe"
	}
	return name
}

// dockerIndexPath returns the path component under DockerURL for this platform,
// e.g. "linux/static/stable/x86_64/". Docker publishes no Windows arm64 build.
func (p Platform) dockerIndexPath() (string, error) {
	if p.OS == "windows" && p.Arch == "arm64" {
		return "", fmt.Errorf("unsupported platform windows/arm64")
	}
	goos, arch, err := p.pick(dockerOS, unameArch)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/static/stable/%s/", goos, arch), nil
}

// dockerTarballPath returns the full path component for a versioned Docker archive.
func (p Platform) dockerTarballPath(version string) (string, error) {
	idx, err := p.dockerIndexPath()
	if err != nil {
		return "", err
	}
	ext := ".tgz"
	if p.OS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("%sdocker-%s%s", idx, strings.TrimPrefix(version, "v"), ext), nil
}

func (p Platform) buildxAsset(version string) (string, error) {
	goos, arch, err := p.pick(goOS, goArch)
	if err != nil {
		return "", err
	}
	return p.exe(fmt.Sprintf("buildx-%s.%s-%s", version, goos, arch)), nil
}

func (p Platform) composeAsset() (string, error) {
	goos, arch, err := p.pick(goOS, unameArch)
	if err != nil {
		return "", err
	}
	return p.exe(fmt.Sprintf("docker-compose-%s-%s", goos, arch)), nil
}

func (p Platform) dockerizeAsset(version, alpineReleasePath string) (string, error) {
	if p.OS == "windows" {
		return "", fmt.Errorf("dockerize has no Windows build")
	}
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
	if p.OS == "windows" {
		return "", "", fmt.Errorf("goss is not supported on Windows")
	}
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

// ECRLoginURLs returns the download URL and .sha256 URL for docker-credential-ecr-login.
func (p Platform) ECRLoginURLs(base, version string) (binURL, sumsURL string, err error) {
	goos, arch, err := p.pick(goOS, goArch)
	if err != nil {
		return "", "", err
	}
	bin := fmt.Sprintf("%s/%s/%s-%s/docker-credential-ecr-login", base, version, goos, arch)
	if p.OS == "windows" {
		bin += ".exe"
	}
	return bin, bin + ".sha256", nil
}

// CredHelperAsset returns the asset filename for docker-credential-{name} from
// github.com/docker/docker-credential-helpers. Windows is not supported.
func (p Platform) CredHelperAsset(name, version string) (string, error) {
	if p.OS == "windows" {
		return "", fmt.Errorf("credentials-store is not supported on Windows")
	}
	goos, arch, err := p.pick(goOS, goArch)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("docker-credential-%s-%s.%s-%s", name, version, goos, arch), nil
}

// Exe appends .exe on Windows, otherwise returns name unchanged.
func (p Platform) Exe(name string) string {
	return p.exe(name)
}
