package clitest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/setup-docker/install"
)

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("script-based tests not supported on Windows")
	}
}

// baseArgs returns the flags common to every install invocation in CLI tests.
// It always pins docker to fakeDockerDownloadVersion so checksums are skipped.
func baseArgs(serverURL, binDir string) []string {
	return []string{
		"--download-url", serverURL,
		"--install-dir", binDir,
		"--docker-version", fakeDockerDownloadVersion,
	}
}

// TestCLI_BareInstallsDefaults: no command → installs docker, buildx, compose;
// a second run with the same binaries in place reports already-installed for all three.
func TestCLI_BareInstallsDefaults(t *testing.T) {
	skipWindows(t)
	srv := newFakeServer(t)
	s, run := sandbox(t)
	binDir := s.BinDir
	args := baseArgs(srv.URL(), binDir)

	out, err := run(t, map[string]string{"PATH": binDir}, args...)
	assert.NilError(t, err, "first run failed:\n%s", out)
	assert.Assert(t, strings.Contains(out, "installed docker"), "expected docker install: %s", out)
	assert.Assert(t, strings.Contains(out, "buildx"), "expected buildx: %s", out)
	assert.Assert(t, strings.Contains(out, "compose"), "expected compose: %s", out)

	_, err = os.Stat(filepath.Join(binDir, "docker"))
	assert.NilError(t, err, "docker not in binDir after install")
	_, err = os.Stat(filepath.Join(binDir, "docker-compose"))
	assert.NilError(t, err, "docker-compose not in binDir after install")

	// Second run — everything already installed.
	out2, err := run(t, map[string]string{"PATH": binDir}, args...)
	assert.NilError(t, err, "second run failed:\n%s", out2)
	assert.Assert(t, strings.Contains(out2, "already installed"), "expected already installed on second run: %s", out2)

	pp := currentPlatformPaths()
	firstCount := srv.requestCount(pp.dockerTarPath)
	// After two runs the tarball should have been fetched exactly once.
	assert.Assert(t, firstCount == 1, "docker tar should be fetched once total, got %d", firstCount)
}

// TestCLI_InstallCommandSameAsDefault: explicit `install` subcommand behaves identically to bare invocation.
func TestCLI_InstallCommandSameAsDefault(t *testing.T) {
	skipWindows(t)
	srv := newFakeServer(t)
	s, run := sandbox(t)
	binDir := s.BinDir
	args := append([]string{"install"}, baseArgs(srv.URL(), binDir)...)

	out, err := run(t, map[string]string{"PATH": binDir}, args...)
	assert.NilError(t, err, "install subcommand failed:\n%s", out)
	assert.Assert(t, strings.Contains(out, "installed docker"), "expected docker install: %s", out)
}

// TestCLI_ComposeOnlyAtVersion: --no-install-docker --no-install-buildx --compose-version <v> installs only compose.
func TestCLI_ComposeOnlyAtVersion(t *testing.T) {
	skipWindows(t)
	srv := newFakeServer(t)
	s, run := sandbox(t)
	binDir := s.BinDir
	args := []string{
		"--download-url", srv.URL(),
		"--install-dir", binDir,
		"--no-install-docker",
		"--no-install-buildx",
		"--compose-version", fakeComposeVersion,
	}

	out, err := run(t, map[string]string{"PATH": binDir}, args...)
	assert.NilError(t, err, "compose-only run failed:\n%s", out)
	assert.Assert(t, !strings.Contains(out, "installed docker"), "docker should not be installed: %s", out)
	assert.Assert(t, strings.Contains(out, "compose"), "compose expected: %s", out)

	pp := currentPlatformPaths()
	assert.Assert(t, srv.requestCount(pp.dockerTarPath) == 0, "docker tar should not be requested")
}

// TestCLI_DockerizeAndGoss: --install-dockerize --install-goss installs both; goss and dgoss resolve.
func TestCLI_DockerizeAndGoss(t *testing.T) {
	skipWindows(t)
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("goss not supported on %s", runtime.GOOS)
	}
	srv := newFakeServer(t)
	s, run := sandbox(t)
	binDir := s.BinDir
	args := []string{
		"--download-url", srv.URL(),
		"--install-dir", binDir,
		"--no-install-docker",
		"--no-install-buildx",
		"--no-install-compose",
		"--install-dockerize",
		"--install-goss",
	}

	out, err := run(t, map[string]string{"PATH": binDir}, args...)
	assert.NilError(t, err, "dockerize+goss run failed:\n%s", out)
	assert.Assert(t, strings.Contains(out, "dockerize"), "dockerize expected: %s", out)
	assert.Assert(t, strings.Contains(out, "goss"), "goss expected: %s", out)

	_, err = os.Stat(filepath.Join(binDir, "goss"))
	assert.NilError(t, err, "goss not in binDir")
	_, err = os.Stat(filepath.Join(binDir, "dgoss"))
	assert.NilError(t, err, "dgoss not in binDir")
}

// TestCLI_PreinstalledDockerKept: if a fake docker on PATH already reports the default version
// with no --docker-version flag, the tarball is never downloaded.
func TestCLI_PreinstalledDockerKept(t *testing.T) {
	skipWindows(t)
	srv := newFakeServer(t)
	s, run := sandbox(t)
	binDir := s.BinDir

	preDir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n  --version) echo 'Docker version %s, build pre' ;;\nesac\n",
		install.DefaultDockerVersion)
	assert.NilError(t, os.WriteFile(filepath.Join(preDir, "docker"), []byte(script), 0o755))

	pp := currentPlatformPaths()
	beforeCount := srv.requestCount(pp.dockerTarPath)

	// No --docker-version: empty want → keep whatever is on PATH.
	args := []string{
		"--download-url", srv.URL(),
		"--install-dir", binDir,
		"--no-install-buildx",
		"--no-install-compose",
	}
	out, err := run(t, map[string]string{"PATH": preDir + string(os.PathListSeparator) + binDir}, args...)
	assert.NilError(t, err, "pre-installed run failed:\n%s", out)
	assert.Assert(t, strings.Contains(out, "already installed"), "expected already installed: %s", out)

	afterCount := srv.requestCount(pp.dockerTarPath)
	assert.Assert(t, afterCount == beforeCount,
		"docker tarball should not be fetched, got %d new requests", afterCount-beforeCount)

	fi, err := os.Stat(filepath.Join(preDir, "docker"))
	assert.NilError(t, err)
	assert.Assert(t, fi.Mode()&os.ModeSymlink == 0, "pre-installed docker must not become a symlink")
}

// TestCLI_WritesNoBashEnv: a full install must not write $BASH_ENV.
func TestCLI_WritesNoBashEnv(t *testing.T) {
	skipWindows(t)
	srv := newFakeServer(t)
	s, run := sandbox(t)
	binDir := s.BinDir
	bashEnvFile := filepath.Join(t.TempDir(), "bash_env")

	args := baseArgs(srv.URL(), binDir)
	out, err := run(t, map[string]string{
		"PATH":     binDir,
		"BASH_ENV": bashEnvFile,
	}, args...)
	assert.NilError(t, err, "run failed:\n%s", out)

	_, statErr := os.Stat(bashEnvFile)
	assert.Assert(t, os.IsNotExist(statErr), "BASH_ENV must not be written, got: %v", statErr)
}

// TestCLI_UnknownFlag: an unknown flag exits non-zero.
func TestCLI_UnknownFlag(t *testing.T) {
	_, run := sandbox(t)
	_, err := run(t, nil, "--no-such-flag")
	assert.Assert(t, err != nil, "expected non-zero exit for unknown flag")
}

// TestCLI_Describe: --describe prints YAML listing the install command with install-compose and compose-version.
func TestCLI_Describe(t *testing.T) {
	_, run := sandbox(t)
	out, err := run(t, nil, "--describe")
	assert.NilError(t, err, "expected --describe to exit 0: %s", out)
	assert.Assert(t, strings.Contains(out, "install"), "--describe output must mention install command: %s", out)
	assert.Assert(t, strings.Contains(out, "install-compose") || strings.Contains(out, "compose"), "--describe must mention compose: %s", out)
	assert.Assert(t, strings.Contains(out, "compose-version") || strings.Contains(out, "compose_version"), "--describe must mention compose-version: %s", out)
}

// TestCLI_404ForRequestedVersion: a 404 for a requested version fails with the tool name and URL.
func TestCLI_404ForRequestedVersion(t *testing.T) {
	skipWindows(t)
	srv := newFakeServer(t)
	s, run := sandbox(t)
	binDir := s.BinDir
	args := []string{
		"--download-url", srv.URL(),
		"--install-dir", binDir,
		"--no-install-docker",
		"--no-install-buildx",
		"--compose-version", "v9.9.9",
	}

	out, err := run(t, map[string]string{"PATH": binDir}, args...)
	assert.Assert(t, err != nil, "expected non-zero exit for 404 version: %s", out)
	assert.Assert(t, strings.Contains(out, "compose:"), "error must mention tool name: %s", out)
	assert.Assert(t, strings.Contains(out, srv.URL()), "error must contain the URL: %s", out)
}

// TestCLI_EqualSignFlagForm: CircleCI passes each with: value as --flag=value,
// so booleans arrive as --install-docker=false and strings as --compose-version=<v>.
// Verify the equal-sign form produces the same outcome as --no-install-docker etc.
func TestCLI_EqualSignFlagForm(t *testing.T) {
	skipWindows(t)
	srv := newFakeServer(t)
	s, run := sandbox(t)
	binDir := s.BinDir

	args := []string{
		"--download-url", srv.URL(),
		"--install-dir", binDir,
		"--install-docker=false",
		"--install-buildx=false",
		"--compose-version=" + fakeComposeVersion,
		"--install-dockerize=true",
	}
	out, err := run(t, map[string]string{"PATH": binDir}, args...)
	assert.NilError(t, err, "equal-sign form failed:\n%s", out)
	assert.Assert(t, !strings.Contains(out, "installed docker "), "docker should not be installed: %s", out)
	assert.Assert(t, !strings.Contains(out, "installed buildx "), "buildx should not be installed: %s", out)
	assert.Assert(t, strings.Contains(out, "compose"), "compose expected: %s", out)
	assert.Assert(t, strings.Contains(out, "dockerize"), "dockerize expected: %s", out)

	pp := currentPlatformPaths()
	assert.Assert(t, srv.requestCount(pp.dockerTarPath) == 0, "docker tar should not be requested")

	t.Run("install-docker=true accepted", func(t *testing.T) {
		srv2 := newFakeServer(t)
		s2, run2 := sandbox(t)
		args2 := []string{
			"--download-url", srv2.URL(),
			"--install-dir", s2.BinDir,
			"--install-docker=true",
			"--no-install-buildx",
			"--no-install-compose",
			"--docker-version", fakeDockerDownloadVersion,
		}
		out2, err2 := run2(t, map[string]string{"PATH": s2.BinDir}, args2...)
		assert.NilError(t, err2, "--install-docker=true failed:\n%s", out2)
		assert.Assert(t, strings.Contains(out2, "installed docker"), "docker should be installed: %s", out2)
	})
}
