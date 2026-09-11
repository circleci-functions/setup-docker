//go:build acceptance

package setupdocker_test

import (
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
)

// versionPattern matches a dotted version like 0.15.0 or v2.29.7, so a tool's
// --version output can be checked without pinning to a specific release.
var versionPattern = regexp.MustCompile(`\d+\.\d+`)

// onlyThisTool returns the flags that disable every install except the named
// one, so a test exercises a single tool.
func onlyThisTool(keep string) []string {
	all := []string{"docker", "docker-compose", "dockerize", "goss"}
	var flags []string
	for _, t := range all {
		if t != keep {
			flags = append(flags, "--install-"+t+"=false")
		}
	}
	return flags
}

func TestInstallDockerComposePinned(t *testing.T) {
	dir := t.TempDir()
	args := append(onlyThisTool("docker-compose"),
		"--docker-compose-install-dir", dir,
		"--docker-compose-version", "v2.29.7",
	)
	out, err := runInstall(t, args...)
	assert.NilError(t, err, out)

	later, err := sdktest.LaterStep(t, "docker-compose", "--version")
	assert.NilError(t, err, later)
	assert.Assert(t, strings.Contains(later, "v2.29.7"), later)
}

func TestInstallDockerPinned(t *testing.T) {
	dir := t.TempDir()
	args := append(onlyThisTool("docker"),
		"--install-dir", dir,
		"--version", "v27.3.1",
	)
	out, err := runInstall(t, args...)
	assert.NilError(t, err, out)

	later, err := sdktest.LaterStep(t, "docker", "--version")
	assert.NilError(t, err, later)
	assert.Assert(t, strings.Contains(later, "27.3.1"), later)
}

func TestInstallDockerizeLatest(t *testing.T) {
	dir := t.TempDir()
	args := append(onlyThisTool("dockerize"), "--dockerize-install-dir", dir)
	out, err := runInstall(t, args...)
	assert.NilError(t, err, out)

	later, err := sdktest.LaterStep(t, "dockerize", "--version")
	assert.NilError(t, err, later)
	assert.Assert(t, versionPattern.MatchString(later), later)
}

func TestInstallGossLatest(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("goss is only supported on Linux")
	}
	dir := t.TempDir()
	args := append(onlyThisTool("goss"), "--goss-install-dir", dir)
	out, err := runInstall(t, args...)
	assert.NilError(t, err, out)

	later, err := sdktest.LaterStep(t, "goss", "--version")
	assert.NilError(t, err, later)
	assert.Assert(t, versionPattern.MatchString(later), later)
}

func TestGossSkippedWhenDefaultedOffLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("goss is supported on Linux; skip only applies elsewhere")
	}
	out, err := runInstall(t,
		"--install-docker=false",
		"--install-docker-compose=false",
		"--install-dockerize=false",
	)
	assert.NilError(t, err, out)
	assert.Assert(t, strings.Contains(out, "skipping"), out)
}

func TestGossErrorsWhenRequestedOffLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("goss is supported on Linux; the error only applies elsewhere")
	}
	out, err := runInstall(t,
		"--install-docker=false",
		"--install-docker-compose=false",
		"--install-dockerize=false",
		"--install-goss",
	)
	assert.Assert(t, err != nil, out)
}
