package docker

import (
	"os"
	"runtime"
	"strings"
)

// unameArch returns the machine architecture as `uname -m` reports it, which is
// how the Docker CLI and docker-compose downloads name their artifacts:
// x86_64 for amd64, aarch64 for arm64.
func unameArch() string {
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

// goArch returns amd64 or arm64, the naming dockerize and goss downloads use.
func goArch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "amd64"
}

func isDarwin() bool { return runtime.GOOS == "darwin" }

func isLinux() bool { return runtime.GOOS == "linux" }

// isAlpine reports whether the host is Alpine Linux, which ships neither the
// glibc dynamic loader some binaries expect nor curl/openssl by default.
func isAlpine() bool {
	for _, f := range []string{"/etc/os-release", "/etc/issue"} {
		if b, err := os.ReadFile(f); err == nil && looksAlpine(string(b)) { //nolint:gosec // fixed set of well-known OS identity files
			return true
		}
	}
	return false
}

func looksAlpine(osRelease string) bool {
	return strings.Contains(strings.ToLower(osRelease), "alpine")
}
