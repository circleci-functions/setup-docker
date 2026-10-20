package clitest_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/setup-docker/install"
)

func skipLintOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("lint CLI tests use shell scripts, not supported on Windows")
	}
}

// hadolintAssetName mirrors install.Platform.hadolintAsset for tests in this package.
func hadolintAssetName() string {
	goos := map[string]string{"linux": "linux", "darwin": "macos", "windows": "windows"}[runtime.GOOS]
	arch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[runtime.GOARCH]
	name := fmt.Sprintf("hadolint-%s-%s", goos, arch)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// newHadolintServer returns a fake download server that serves a hadolint shell
// script. The script accepts --version, records its args, and respects
// HADOLINT_RECORD (path to write args) and HADOLINT_EXIT_CODE.
// It outputs minimal JUnit XML when "--format junit" appears in argv.
func newHadolintServer(t *testing.T) *httptest.Server {
	t.Helper()
	version := install.DefaultHadolintVersion
	// The fake hadolint script handles --version, arg recording, format detection,
	// and optional exit code injection.
	script := []byte(fmt.Sprintf(`#!/bin/sh
if [ "$1" = "--version" ]; then
  echo 'Haskell Dockerfile Linter %s'
  exit 0
fi
if [ -n "$HADOLINT_RECORD" ]; then
  printf '%%s\n' "$@" > "$HADOLINT_RECORD"
fi
for arg in "$@"; do
  if [ "$arg" = "junit" ]; then
    printf '<testsuites><testsuite name="hadolint" tests="1" failures="1"><testcase classname="Dockerfile" name="DL4000"><failure message="MAINTAINER deprecated">DL4000</failure></testcase></testsuite></testsuites>\n'
    exit "${HADOLINT_EXIT_CODE:-0}"
  fi
done
exit "${HADOLINT_EXIT_CODE:-0}"
`, version))

	h := sha256.Sum256(script)
	hexSum := hex.EncodeToString(h[:])
	asset := hadolintAssetName()
	base := "/hadolint/hadolint/releases/download/" + version + "/"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case base + "checksums.sha256":
			fmt.Fprintf(w, "%s *%s\n", hexSum, asset)
		case base + asset:
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(script)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestCLI_Lint_RecordsArgv verifies the argv passed to hadolint for the common flags.
func TestCLI_Lint_RecordsArgv(t *testing.T) {
	skipLintOnWindows(t)
	s, run := sandbox(t)
	hadolintSrv := newHadolintServer(t)

	dockerfile := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfile, []byte("FROM alpine:3.22\n"), 0o644))
	recordFile := filepath.Join(t.TempDir(), "args.txt")

	out, err := run(t, map[string]string{
		"PATH":            s.BinDir,
		"HADOLINT_RECORD": recordFile,
	},
		"lint",
		"--download-url", hadolintSrv.URL,
		"--failure-threshold", "error",
		"--ignore-rules", "DL3008,SC2086",
		"--trusted-registries", "docker.io",
		"--dockerfiles", dockerfile,
	)
	assert.NilError(t, err, "lint failed:\n%s", out)

	data, readErr := os.ReadFile(recordFile)
	assert.NilError(t, readErr, "args file not written")
	args := string(data)
	assert.Check(t, cmp.Contains(args, "--failure-threshold"), "missing --failure-threshold: %s", args)
	assert.Check(t, cmp.Contains(args, "error"), "missing error: %s", args)
	assert.Check(t, cmp.Contains(args, "--ignore"), "missing --ignore: %s", args)
	assert.Check(t, cmp.Contains(args, "DL3008"), "missing DL3008: %s", args)
	assert.Check(t, cmp.Contains(args, "SC2086"), "missing SC2086: %s", args)
	assert.Check(t, cmp.Contains(args, "--trusted-registry"), "missing --trusted-registry: %s", args)
	assert.Check(t, cmp.Contains(args, "docker.io"), "missing docker.io: %s", args)
	assert.Check(t, cmp.Contains(args, dockerfile), "missing dockerfile path: %s", args)
}

// TestCLI_Lint_EqualSignFlag verifies --failure-threshold=error (= form) works.
func TestCLI_Lint_EqualSignFlag(t *testing.T) {
	skipLintOnWindows(t)
	s, run := sandbox(t)
	hadolintSrv := newHadolintServer(t)

	dockerfile := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfile, []byte("FROM alpine:3.22\n"), 0o644))

	out, err := run(t, map[string]string{"PATH": s.BinDir},
		"lint",
		"--download-url", hadolintSrv.URL,
		"--failure-threshold=error",
		"--dockerfiles", dockerfile,
	)
	assert.NilError(t, err, "lint with = flag form failed:\n%s", out)
}

// TestCLI_Lint_FailingHadolint verifies that a non-zero hadolint exit fails the command.
func TestCLI_Lint_FailingHadolint(t *testing.T) {
	skipLintOnWindows(t)
	s, run := sandbox(t)
	hadolintSrv := newHadolintServer(t)

	dockerfile := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfile, []byte("FROM alpine:3.22\n"), 0o644))

	out, err := run(t, map[string]string{
		"PATH":               s.BinDir,
		"HADOLINT_EXIT_CODE": "1",
	},
		"lint",
		"--download-url", hadolintSrv.URL,
		"--dockerfiles", dockerfile,
	)
	assert.Assert(t, err != nil, "expected non-zero exit when hadolint fails; got: %s", out)
	assert.Check(t, cmp.Contains(out, dockerfile), "error should mention the failing file: %s", out)
}

// TestCLI_Lint_ResultsFile verifies that --results-file writes JUnit XML.
func TestCLI_Lint_ResultsFile(t *testing.T) {
	skipLintOnWindows(t)
	s, run := sandbox(t)
	hadolintSrv := newHadolintServer(t)

	dockerfile := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfile, []byte("FROM alpine:3.22\n"), 0o644))
	resultsFile := filepath.Join(t.TempDir(), "results", "lint.xml")

	out, err := run(t, map[string]string{"PATH": s.BinDir},
		"lint",
		"--download-url", hadolintSrv.URL,
		"--dockerfiles", dockerfile,
		"--results-file", resultsFile,
	)
	assert.NilError(t, err, "lint failed:\n%s", out)

	data, readErr := os.ReadFile(resultsFile)
	assert.NilError(t, readErr, "results file not written")
	xml := string(data)
	assert.Check(t, cmp.Contains(xml, "testsuites"), "expected testsuites element: %s", xml)
	assert.Check(t, cmp.Contains(xml, "DL4000"), "expected DL4000 in results: %s", xml)
}

// TestCLI_Lint_WritesNoBashEnv verifies the lint command never writes $BASH_ENV.
func TestCLI_Lint_WritesNoBashEnv(t *testing.T) {
	skipLintOnWindows(t)
	s, run := sandbox(t)
	hadolintSrv := newHadolintServer(t)

	dockerfile := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfile, []byte("FROM alpine:3.22\n"), 0o644))
	bashEnvFile := filepath.Join(t.TempDir(), "bash_env")

	out, err := run(t, map[string]string{
		"PATH":     s.BinDir,
		"BASH_ENV": bashEnvFile,
	},
		"lint",
		"--download-url", hadolintSrv.URL,
		"--dockerfiles", dockerfile,
	)
	assert.NilError(t, err, "lint failed:\n%s", out)

	_, statErr := os.Stat(bashEnvFile)
	assert.Assert(t, os.IsNotExist(statErr), "BASH_ENV must not be written; got: %v", statErr)
}

// TestCLI_Lint_MissingDockerfile verifies that a missing Dockerfile returns an error naming the path.
func TestCLI_Lint_MissingDockerfile(t *testing.T) {
	skipLintOnWindows(t)
	s, run := sandbox(t)
	hadolintSrv := newHadolintServer(t)

	missing := "/no/such/Dockerfile"
	out, err := run(t, map[string]string{"PATH": s.BinDir},
		"lint",
		"--download-url", hadolintSrv.URL,
		"--dockerfiles", missing,
	)
	assert.Assert(t, err != nil, "expected non-zero exit for missing Dockerfile: %s", out)
	assert.Check(t, cmp.Contains(out, missing), "error must name the missing file: %s", out)
}

// TestCLI_Lint_Describe verifies --describe lists the lint command and its key arguments.
func TestCLI_Lint_Describe(t *testing.T) {
	_, run := sandbox(t)
	out, err := run(t, nil, "--describe")
	assert.NilError(t, err, "--describe failed: %s", out)
	assert.Check(t, strings.Contains(out, "lint"), "--describe must list lint command: %s", out)
}
