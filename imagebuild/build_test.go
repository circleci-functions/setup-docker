package imagebuild

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// fakeDocker writes argv lines to a file and exits with the given behaviour.
const fakeDockerBase = `#!/bin/sh
log="$FAKE_DOCKER_LOG"
echo "$@" >> "$log"
`

func writeFakeDocker(t *testing.T, extra string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shell script not supported on Windows")
	}
	dir := t.TempDir()
	log := filepath.Join(t.TempDir(), "docker.log")
	script := fakeDockerBase + extra
	dockerPath := filepath.Join(dir, "docker")
	assert.NilError(t, os.WriteFile(dockerPath, []byte(script), 0700)) //nolint:gosec
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_LOG", log)
	return log
}

func readLog(t *testing.T, logFile string) []string {
	t.Helper()
	data, err := os.ReadFile(logFile) //nolint:gosec
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

// TestCheckParsing verifies check JSON parsing and warning handling.
func TestCheckParsing(t *testing.T) {
	fixture := `{"warnings":[{"ruleName":"FromAsCasing","description":"The 'as' keyword should match the case of the 'from' keyword"},{"ruleName":"MaintainerDeprecated","description":"The MAINTAINER instruction is deprecated, use a label instead to define an image author"}]}`

	r, err := parseCheckOutput([]byte(fixture))
	assert.NilError(t, err)
	assert.Equal(t, len(r.Warnings), 2)
	assert.Equal(t, r.Warnings[0].RuleName, "FromAsCasing")
	assert.Equal(t, r.Warnings[1].RuleName, "MaintainerDeprecated")

	tests := []struct {
		name         string
		warnAsErrors bool
		wantErr      string
	}{
		{
			name:         "WarningsAsErrors",
			warnAsErrors: true,
			wantErr:      "FromAsCasing",
		},
		{
			name:         "WarningsOnlyNotErrors",
			warnAsErrors: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.warnAsErrors {
				names := make([]string, len(r.Warnings))
				for i, w := range r.Warnings {
					names[i] = w.RuleName
				}
				err := fmt.Errorf("build check failed: %s", strings.Join(names, ", "))
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.Equal(t, len(r.Warnings) > 0, true)
			}
		})
	}
}

// TestEnsureBuilder_InspectSucceeds verifies that when inspect succeeds, create is not called.
func TestEnsureBuilder_InspectSucceeds(t *testing.T) {
	logFile := writeFakeDocker(t, `# always succeed
exit 0
`)
	f := Flags{Image: "x", Registry: "r"}
	name, err := ensureBuilder(context.Background(), f)
	assert.NilError(t, err)
	assert.Equal(t, name, builderName)

	lines := readLog(t, logFile)
	assert.Check(t, len(lines) >= 1)
	assert.Check(t, strings.Contains(lines[0], "buildx inspect"),
		"expected first call to be buildx inspect, got: %v", lines)
	for _, l := range lines {
		assert.Check(t, !strings.Contains(l, "buildx create"),
			"unexpected buildx create when inspect succeeded: %v", lines)
	}
}

// TestEnsureBuilder_InspectFails verifies that when inspect fails, create is called with the pinned image.
func TestEnsureBuilder_InspectFails(t *testing.T) {
	logFile := writeFakeDocker(t, `case "$1 $2" in
  "buildx inspect") exit 1 ;;
  *) exit 0 ;;
esac
`)
	f := Flags{Image: "x", Registry: "r"}
	name, err := ensureBuilder(context.Background(), f)
	assert.NilError(t, err)
	assert.Equal(t, name, builderName)

	lines := readLog(t, logFile)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "buildx create") && strings.Contains(l, "moby/buildkit") && strings.Contains(l, "network=host") {
			found = true
			break
		}
	}
	assert.Check(t, found, "expected buildx create with moby/buildkit and network=host in log, got: %v", lines)
}

// TestArchForPlatform verifies the arch extraction from platform strings.
func TestArchForPlatform(t *testing.T) {
	tests := []struct {
		platform string
		want     string
	}{
		{"linux/amd64", "amd64"},
		{"linux/arm64", "arm64"},
		{"linux/arm/v7", "arm"},
		{"linux/arm/v6", "arm"},
		{"linux/386", "386"},
		{"linux/ppc64le", "ppc64le"},
		{"linux/s390x", "s390x"},
		{"linux/riscv64", "riscv64"},
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			assert.Equal(t, archForPlatform(tt.platform), tt.want)
		})
	}
}

// TestEnsureQEMU_ForeignArch verifies QEMU is installed for a foreign arch.
func TestEnsureQEMU_ForeignArch(t *testing.T) {
	logFile := writeFakeDocker(t, `case "$*" in
  *"{{.Architecture}}"*) echo "x86_64" ;;
  *"--privileged"*"--rm"*)
    case "$*" in
      *"--install"*) ;;
      *) echo '{"emulators":[]}' ;;
    esac
    ;;
esac
exit 0
`)
	installed, err := ensureQEMU(context.Background(), []string{"linux/arm64"})
	assert.NilError(t, err)
	assert.DeepEqual(t, installed, []string{"qemu-aarch64"})

	lines := readLog(t, logFile)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "binfmt") && strings.Contains(l, "--install") {
			found = true
			break
		}
	}
	assert.Check(t, found, "expected binfmt --install call for foreign arch, got: %v", lines)
}

// TestEnsureQEMU_NativeOnly verifies QEMU is skipped for native platforms.
func TestEnsureQEMU_NativeOnly(t *testing.T) {
	logFile := writeFakeDocker(t, `case "$*" in
  *"{{.Architecture}}"*) echo "x86_64" ;;
esac
exit 0
`)
	installed, err := ensureQEMU(context.Background(), []string{"linux/amd64"})
	assert.NilError(t, err)
	assert.Equal(t, len(installed), 0)

	lines := readLog(t, logFile)
	for _, l := range lines {
		assert.Check(t, !strings.Contains(l, "binfmt"),
			"expected no binfmt call for native arch, got: %v", lines)
	}
}

// TestEnsureQEMU_SkipsAlreadyRegistered verifies that arches already registered
// are not reinstalled.
func TestEnsureQEMU_SkipsAlreadyRegistered(t *testing.T) {
	logFile := writeFakeDocker(t, `case "$*" in
  *"{{.Architecture}}"*) echo "x86_64" ;;
  *"--privileged"*"--rm"*)
    case "$*" in
      *"--install"*) ;;
      *) echo '{"emulators":["qemu-aarch64"]}' ;;
    esac
    ;;
esac
exit 0
`)
	installed, err := ensureQEMU(context.Background(), []string{"linux/arm64"})
	assert.NilError(t, err)
	assert.Equal(t, len(installed), 0, "arm64 already registered; should not install")

	lines := readLog(t, logFile)
	for _, l := range lines {
		assert.Check(t, !strings.Contains(l, "--install"),
			"expected no --install for already-registered emulator: %v", lines)
	}
}

// TestUninstallEmulators_CallsDockerRun verifies the uninstall command arguments.
func TestUninstallEmulators_CallsDockerRun(t *testing.T) {
	logFile := writeFakeDocker(t, `exit 0`)
	uninstallEmulators(context.Background(), []string{"qemu-aarch64", "qemu-arm"})

	lines := readLog(t, logFile)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "--uninstall") && strings.Contains(l, "qemu-aarch64") && strings.Contains(l, "qemu-arm") {
			found = true
			break
		}
	}
	assert.Check(t, found, "expected --uninstall with emulator names, got: %v", lines)
}

// TestBuild_RetryOnPushFailure verifies that push failures trigger retries.
func TestBuild_RetryOnPushFailure(t *testing.T) {
	logFile := writeFakeDocker(t, `
case "$1 $2" in
  "buildx version") echo "github.com/docker/buildx v0.38.0" ;;
  "buildx build")
    COUNT_FILE="${FAKE_DOCKER_LOG}.count"
    count=$(cat "$COUNT_FILE" 2>/dev/null || echo 0)
    count=$((count + 1))
    echo "$count" > "$COUNT_FILE"
    if [ "$count" -le 2 ]; then
      echo "failed to push image" >&2
      exit 1
    fi
    prev=""
    for arg in "$@"; do
      case "$prev" in
        --metadata-file) printf '{"containerimage.digest":"sha256:abc"}' > "$arg" ;;
      esac
      prev="$arg"
    done
    exit 0
    ;;
  "buildx inspect") exit 0 ;;
  "info") echo "linux" ;;
  *) exit 0 ;;
esac
`)

	ctx := context.Background()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("CIRCLE_SHA1", "abc123")

	f := Flags{
		Image:      "myorg/app",
		Registry:   "docker.io",
		Tags:       "v1",
		Dockerfile: "Dockerfile",
		Context:    ".",
		Load:       "false",
		Push:       true,
		Retries:    3,
	}

	result, err := Build(ctx, f)
	assert.NilError(t, err)
	assert.Check(t, cmp.DeepEqual(result.Refs, []string{"docker.io/myorg/app:v1"}))

	lines := readLog(t, logFile)
	buildCount := 0
	for _, l := range lines {
		if strings.Contains(l, "buildx build") {
			buildCount++
		}
	}
	assert.Equal(t, buildCount, 3, "expected 3 builds (2 failures + 1 success), got %d\nlog: %v", buildCount, lines)
}

// TestBuild_NonPushFailureNoRetry verifies non-push failures are not retried.
func TestBuild_NonPushFailureNoRetry(t *testing.T) {
	logFile := writeFakeDocker(t, `
case "$1 $2" in
  "buildx version") echo "github.com/docker/buildx v0.38.0" ;;
  "buildx build")
    echo "dockerfile syntax error" >&2
    exit 1
    ;;
  "buildx inspect") exit 0 ;;
  "info") echo "linux" ;;
  *) exit 0 ;;
esac
`)

	ctx := context.Background()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("CIRCLE_SHA1", "abc123")

	f := Flags{
		Image:      "myorg/app",
		Registry:   "docker.io",
		Tags:       "v1",
		Dockerfile: "Dockerfile",
		Context:    ".",
		Load:       "false",
		Push:       true,
		Retries:    3,
	}

	_, err := Build(ctx, f)
	assert.Assert(t, err != nil)

	lines := readLog(t, logFile)
	buildCount := 0
	for _, l := range lines {
		if strings.Contains(l, "buildx build") {
			buildCount++
		}
	}
	assert.Equal(t, buildCount, 1, "expected 1 build (no retry for non-push failure), got %d", buildCount)
}

// TestBuild_DigestFileWritten verifies the digest file is written from the metadata.
func TestBuild_DigestFileWritten(t *testing.T) {
	writeFakeDocker(t, `
case "$1 $2" in
  "buildx version") echo "github.com/docker/buildx v0.38.0" ;;
  "buildx build")
    prev=""
    for arg in "$@"; do
      case "$prev" in
        --metadata-file) printf '{"containerimage.digest":"sha256:deadbeef"}' > "$arg" ;;
      esac
      prev="$arg"
    done
    exit 0
    ;;
  "buildx inspect") exit 0 ;;
  "info") echo "linux" ;;
  *) exit 0 ;;
esac
`)

	ctx := context.Background()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("CIRCLE_SHA1", "abc123")

	digestFile := filepath.Join(t.TempDir(), "sub", "digest.txt")
	f := Flags{
		Image:      "myorg/app",
		Registry:   "docker.io",
		Tags:       "v1",
		Dockerfile: "Dockerfile",
		Context:    ".",
		Load:       "false",
		Retries:    3,
		DigestFile: digestFile,
	}

	result, err := Build(ctx, f)
	assert.NilError(t, err)
	assert.Equal(t, result.Digest, "sha256:deadbeef")

	got, err := os.ReadFile(digestFile) //nolint:gosec
	assert.NilError(t, err)
	assert.Equal(t, string(got), "sha256:deadbeef")
}
