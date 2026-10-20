package clitest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func skipBuildOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("build CLI tests require shell scripts, not supported on Windows")
	}
}

// newFakeBuildDocker creates a fake docker script in a temp dir. It responds to
// the calls prepareBuild and runBuild make: info (returning linux), buildx version,
// buildx inspect, and buildx build.
// Returns the script directory and a file path where each invocation's argv is recorded.
func newFakeBuildDocker(t *testing.T) (dir, argFile string) {
	t.Helper()
	dir = t.TempDir()
	argFile = filepath.Join(t.TempDir(), "build-args.txt")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %s
case "$1 $2" in
  "info --format")
    if [ "$3" = '{{.OSType}}' ]; then
      echo "linux"
    elif [ "$3" = '{{.Architecture}}' ]; then
      echo "x86_64"
    fi
    ;;
  "buildx version") echo "github.com/docker/buildx v0.38.0 x" ;;
  "buildx inspect") exit 0 ;;
  "buildx build")
    prev=""
    for arg in "$@"; do
      if [ "$prev" = "--metadata-file" ]; then
        printf '{"containerimage.digest":"sha256:abc"}' > "$arg"
      fi
      prev="$arg"
    done
    ;;
esac
exit 0
`, argFile)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	return dir, argFile
}

// readArgFile reads all recorded argv lines from the fake docker arg file.
func readBuildArgLines(t *testing.T, argFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argFile) //nolint:gosec
	if os.IsNotExist(err) {
		return nil
	}
	assert.NilError(t, err)
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// buildLine returns the first line in lines that contains "buildx build".
func buildLine(lines []string) string {
	for _, l := range lines {
		if strings.Contains(l, "buildx build") {
			return l
		}
	}
	return ""
}

// TestCLI_Build_FlagsProduceExpectedArgs exercises the function-step argument form.
func TestCLI_Build_FlagsProduceExpectedArgs(t *testing.T) {
	skipBuildOnWindows(t)
	_, run := sandbox(t)
	dockerDir, argFile := newFakeBuildDocker(t)
	digestFile := filepath.Join(t.TempDir(), "digest.txt")

	out, err := run(t, map[string]string{
		"PATH":        dockerDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"CIRCLE_SHA1": "deadbeef",
	},
		"build",
		"--image=myorg/app",
		"--tags=v1,latest",
		"--push",
		"--build-args=A=1\nB=x,y",
		"--digest-file="+digestFile,
		"--cache=false",
	)
	assert.NilError(t, err, "build failed:\n%s", out)

	lines := readBuildArgLines(t, argFile)
	bl := buildLine(lines)
	assert.Assert(t, bl != "", "no buildx build line found in: %v", lines)

	assert.Check(t, cmp.Contains(bl, "-t docker.io/myorg/app:v1"), "missing -t v1: %s", bl)
	assert.Check(t, cmp.Contains(bl, "-t docker.io/myorg/app:latest"), "missing -t latest: %s", bl)
	assert.Check(t, cmp.Contains(bl, "--push"), "missing --push: %s", bl)
	assert.Check(t, cmp.Contains(bl, "--build-arg A=1"), "missing --build-arg A=1: %s", bl)
	assert.Check(t, cmp.Contains(bl, "--build-arg B=x,y"), "missing --build-arg B=x,y: %s", bl)

	digest, readErr := os.ReadFile(digestFile) //nolint:gosec
	assert.NilError(t, readErr, "digest file not written")
	assert.Check(t, cmp.Equal(string(digest), "sha256:abc"), "digest file content")
}

// TestCLI_Build_MissingImage fails when --image is not given.
func TestCLI_Build_MissingImage(t *testing.T) {
	skipBuildOnWindows(t)
	_, run := sandbox(t)
	dockerDir, _ := newFakeBuildDocker(t)
	_, err := run(t, map[string]string{
		"PATH": dockerDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, "build")
	assert.Assert(t, err != nil, "expected error when --image is missing")
}

// newFakeWindowsDocker creates a fake docker script that reports a Windows engine.
// It records each invocation's argv in argFile and the build-time DOCKER_BUILDKIT
// env var in envFile. Push calls print a fake digest line.
func newFakeWindowsDocker(t *testing.T) (dir, argFile, envFile string) {
	t.Helper()
	dir = t.TempDir()
	argFile = filepath.Join(t.TempDir(), "args.txt")
	envFile = filepath.Join(t.TempDir(), "env.txt")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %s
case "$1" in
  info)
    echo "windows"
    ;;
  build)
    echo "DOCKER_BUILDKIT=$DOCKER_BUILDKIT" >> %s
    ;;
  push)
    printf '%%s: digest: sha256:pushdigest123 size: 1234\n' "$2"
    ;;
esac
exit 0
`, argFile, envFile)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	return dir, argFile, envFile
}

// TestCLI_Build_Windows_Push verifies the classic Windows build path: uses
// docker build (not buildx), sets DOCKER_BUILDKIT=0, pushes, and writes the digest.
func TestCLI_Build_Windows_Push(t *testing.T) {
	skipBuildOnWindows(t)
	_, run := sandbox(t)
	dockerDir, argFile, envFile := newFakeWindowsDocker(t)
	digestFile := filepath.Join(t.TempDir(), "digest.txt")

	out, err := run(t, map[string]string{
		"PATH":        dockerDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"CIRCLE_SHA1": "deadbeef",
	},
		"build",
		"--image=org/win",
		"--tags=t",
		"--push=true",
		"--digest-file="+digestFile,
		"--cache=false",
	)
	assert.NilError(t, err, "build failed:\n%s", out)

	lines := readBuildArgLines(t, argFile)

	var buildArg, pushArg string
	for _, l := range lines {
		if strings.HasPrefix(l, "build ") {
			buildArg = l
		}
		if strings.HasPrefix(l, "push ") {
			pushArg = l
		}
	}
	assert.Assert(t, buildArg != "", "no build line found: %v", lines)
	assert.Check(t, !strings.Contains(buildArg, " -f "), "the context's Dockerfile is the default: %s", buildArg)
	assert.Check(t, cmp.Contains(buildArg, "-t docker.io/org/win:t"), "missing -t: %s", buildArg)
	assert.Assert(t, strings.HasSuffix(strings.TrimSpace(buildArg), "."), "context not last: %s", buildArg)
	assert.Assert(t, !strings.Contains(buildArg, "buildx"), "must not use buildx: %s", buildArg)

	envData, envErr := os.ReadFile(envFile) //nolint:gosec
	assert.NilError(t, envErr, "env file not written")
	assert.Check(t, cmp.Contains(string(envData), "DOCKER_BUILDKIT=0"), "DOCKER_BUILDKIT not 0: %s", envData)

	assert.Assert(t, pushArg != "", "no push line found: %v", lines)
	assert.Check(t, cmp.Contains(pushArg, "docker.io/org/win:t"), "wrong push target: %s", pushArg)

	digest, readErr := os.ReadFile(digestFile) //nolint:gosec
	assert.NilError(t, readErr, "digest file not written")
	assert.Check(t, cmp.Equal(string(digest), "sha256:pushdigest123"), "digest mismatch")
}

// TestCLI_Build_Windows_BuildKitError verifies that BuildKit-only flags fail on
// a Windows engine and that the cache notice is printed.
func TestCLI_Build_Windows_BuildKitError(t *testing.T) {
	skipBuildOnWindows(t)
	_, run := sandbox(t)
	dockerDir, _, _ := newFakeWindowsDocker(t)

	out, err := run(t, map[string]string{
		"PATH":        dockerDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"CIRCLE_SHA1": "deadbeef",
	},
		"build",
		"--image=org/win",
		"--tags=t",
		"--platforms=linux/amd64",
	)
	assert.Assert(t, err != nil, "expected error for platforms on Windows")
	assert.Assert(t, cmp.Contains(out, "BuildKit"), "expected BuildKit in error: %s", out)
	assert.Assert(t, cmp.Contains(out, "Windows containers"), "expected Windows containers in message: %s", out)
	assert.Assert(t, cmp.Contains(out, "layer cache"), "expected cache notice: %s", out)
}

// TestCLI_Build_WritesNoBashEnv verifies nothing is written to $BASH_ENV.
func TestCLI_Build_WritesNoBashEnv(t *testing.T) {
	skipBuildOnWindows(t)
	_, run := sandbox(t)
	dockerDir, _ := newFakeBuildDocker(t)
	bashEnvFile := filepath.Join(t.TempDir(), "bash_env")

	_, err := run(t, map[string]string{
		"PATH":        dockerDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"CIRCLE_SHA1": "deadbeef",
		"BASH_ENV":    bashEnvFile,
	}, "build", "--image=myorg/app", "--tags=v1", "--cache=false")
	assert.NilError(t, err)

	_, statErr := os.Stat(bashEnvFile)
	assert.Assert(t, os.IsNotExist(statErr), "BASH_ENV must not be written, got: %v", statErr)
}

// TestCLI_Build_CacheFalsePassesNoLocalCacheArgs verifies --cache=false omits type=local args.
func TestCLI_Build_CacheFalsePassesNoLocalCacheArgs(t *testing.T) {
	skipBuildOnWindows(t)
	_, run := sandbox(t)
	dockerDir, argFile := newFakeBuildDocker(t)

	_, err := run(t, map[string]string{
		"PATH":        dockerDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"CIRCLE_SHA1": "deadbeef",
	}, "build", "--image=myorg/app", "--tags=v1", "--cache=false")
	assert.NilError(t, err)

	lines := readBuildArgLines(t, argFile)
	bl := buildLine(lines)
	assert.Assert(t, bl != "", "no buildx build line found in: %v", lines)
	assert.Assert(t, !strings.Contains(bl, "type=local"),
		"expected no type=local cache arg when --cache=false: %s", bl)
}
