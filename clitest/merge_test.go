package clitest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

const (
	mergeDigestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mergeDigestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	mergeDigestM = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff00"
)

// newFakeMergeDocker writes a fake docker script that handles imagetools calls.
// rawIndex is the JSON returned for imagetools inspect --raw.
// The script logs all invocations to logFile and returns mergeDigestM for the manifest.
func newFakeMergeDocker(t *testing.T, logFile, rawIndex string) string {
	t.Helper()
	manifestJSON := fmt.Sprintf(`{"digest":"%s"}`, mergeDigestM)
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %s
case "$1 $2" in
  "buildx version") echo "github.com/docker/buildx v0.99.0" ;;
  "buildx imagetools")
    case "$3" in
      create) exit 0 ;;
      inspect)
        case "$*" in
          *--raw*) printf '%%s' '%s' ;;
          *--format*) printf '%%s' '%s' ;;
        esac
        ;;
    esac
    ;;
esac
exit 0
`, logFile, rawIndex, manifestJSON)
	return script
}

func writeDockerScript(t *testing.T, dir, script string) {
	t.Helper()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
}

// writeDigests writes digest files into dir and returns the dir path.
func writeDigests(t *testing.T, digests map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, d := range digests {
		assert.NilError(t, os.WriteFile(filepath.Join(dir, name), []byte(d), 0o600))
	}
	return dir
}

// TestCLI_Merge_CreateArgv verifies the imagetools create call uses correct tags and sources.
func TestCLI_Merge_CreateArgv(t *testing.T) {
	skipWindows(t)
	s, run := sandbox(t)

	dockerDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "docker.log")

	rawIndex := `{"schemaVersion":2,"manifests":[` +
		`{"digest":"` + mergeDigestA + `","platform":{"architecture":"amd64","os":"linux"}},` +
		`{"digest":"` + mergeDigestB + `","platform":{"architecture":"arm64","os":"linux"}}` +
		`]}`

	writeDockerScript(t, dockerDir, newFakeMergeDocker(t, logFile, rawIndex))

	digestsDir := writeDigests(t, map[string]string{
		"amd64": mergeDigestA,
		"arm64": mergeDigestB,
	})

	env := map[string]string{
		"PATH": dockerDir + string(os.PathListSeparator) + s.BinDir,
	}
	out, err := run(t, env,
		"merge",
		"--image=myorg/app",
		"--tags=v1,latest",
		"--digests-dir="+digestsDir,
	)
	assert.NilError(t, err, "merge failed:\n%s", out)

	logData, readErr := os.ReadFile(logFile)
	assert.NilError(t, readErr)
	log := string(logData)

	// imagetools create should have -t for each tag
	assert.Check(t, cmp.Contains(log, "imagetools create"), "create not called: %s", log)
	assert.Check(t, cmp.Contains(log, "docker.io/myorg/app:v1"), "tag v1 missing: %s", log)
	assert.Check(t, cmp.Contains(log, "docker.io/myorg/app:latest"), "tag latest missing: %s", log)

	// sources should be repo@digest
	assert.Check(t, cmp.Contains(log, "docker.io/myorg/app@"+mergeDigestA), "source amd64 missing: %s", log)
	assert.Check(t, cmp.Contains(log, "docker.io/myorg/app@"+mergeDigestB), "source arm64 missing: %s", log)

	// docker info must NOT be called (no daemon needed)
	assert.Check(t, !strings.Contains(log, "docker info"), "docker info unexpectedly called: %s", log)
}

// TestCLI_Merge_DigestFileWritten verifies the digest file is written.
func TestCLI_Merge_DigestFileWritten(t *testing.T) {
	skipWindows(t)
	s, run := sandbox(t)

	dockerDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "docker.log")

	rawIndex := `{"schemaVersion":2,"manifests":[` +
		`{"digest":"` + mergeDigestA + `","platform":{"architecture":"amd64","os":"linux"}},` +
		`{"digest":"` + mergeDigestB + `","platform":{"architecture":"arm64","os":"linux"}}` +
		`]}`

	writeDockerScript(t, dockerDir, newFakeMergeDocker(t, logFile, rawIndex))

	digestsDir := writeDigests(t, map[string]string{
		"amd64": mergeDigestA,
		"arm64": mergeDigestB,
	})

	digestFile := filepath.Join(t.TempDir(), "merged-digest.txt")

	env := map[string]string{
		"PATH": dockerDir + string(os.PathListSeparator) + s.BinDir,
	}
	out, err := run(t, env,
		"merge",
		"--image=myorg/app",
		"--tags=v1",
		"--digests-dir="+digestsDir,
		"--digest-file="+digestFile,
	)
	assert.NilError(t, err, "merge failed:\n%s", out)

	data, readErr := os.ReadFile(digestFile)
	assert.NilError(t, readErr, "digest file not written")
	assert.Check(t, cmp.Equal(strings.TrimSpace(string(data)), mergeDigestM))
}

// TestCLI_Merge_PlatformCountMismatch verifies that a mismatch between digest
// files and index platforms fails with an error.
func TestCLI_Merge_PlatformCountMismatch(t *testing.T) {
	skipWindows(t)
	s, run := sandbox(t)

	dockerDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "docker.log")

	// Index reports only 1 platform but we have 2 digest files.
	rawIndex := `{"schemaVersion":2,"manifests":[` +
		`{"digest":"` + mergeDigestA + `","platform":{"architecture":"amd64","os":"linux"}}` +
		`]}`

	writeDockerScript(t, dockerDir, newFakeMergeDocker(t, logFile, rawIndex))

	digestsDir := writeDigests(t, map[string]string{
		"amd64": mergeDigestA,
		"arm64": mergeDigestB,
	})

	env := map[string]string{
		"PATH": dockerDir + string(os.PathListSeparator) + s.BinDir,
	}
	out, err := run(t, env,
		"merge",
		"--image=myorg/app",
		"--tags=v1",
		"--digests-dir="+digestsDir,
	)
	assert.Assert(t, err != nil, "expected failure for platform count mismatch, got: %s", out)
	assert.Check(t, cmp.Contains(out, "platform"), "error should mention platform: %s", out)
}

// TestCLI_Merge_NoBashEnv verifies nothing is written to BASH_ENV.
func TestCLI_Merge_NoBashEnv(t *testing.T) {
	skipWindows(t)
	s, run := sandbox(t)

	dockerDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "docker.log")
	bashEnvFile := filepath.Join(t.TempDir(), "bash_env")

	rawIndex := `{"schemaVersion":2,"manifests":[` +
		`{"digest":"` + mergeDigestA + `","platform":{"architecture":"amd64","os":"linux"}}` +
		`]}`

	writeDockerScript(t, dockerDir, newFakeMergeDocker(t, logFile, rawIndex))

	digestsDir := writeDigests(t, map[string]string{"amd64": mergeDigestA})

	env := map[string]string{
		"PATH":     dockerDir + string(os.PathListSeparator) + s.BinDir,
		"BASH_ENV": bashEnvFile,
	}
	_, _ = run(t, env,
		"merge",
		"--image=myorg/app",
		"--tags=v1",
		"--digests-dir="+digestsDir,
	)

	_, statErr := os.Stat(bashEnvFile)
	assert.Check(t, os.IsNotExist(statErr), "BASH_ENV file must not be created")
}

// TestCLI_Merge_NoDaemonCall verifies docker info is never invoked.
func TestCLI_Merge_NoDaemonCall(t *testing.T) {
	skipWindows(t)
	s, run := sandbox(t)

	dockerDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "docker.log")

	rawIndex := `{"schemaVersion":2,"manifests":[` +
		`{"digest":"` + mergeDigestA + `","platform":{"architecture":"amd64","os":"linux"}}` +
		`]}`

	writeDockerScript(t, dockerDir, newFakeMergeDocker(t, logFile, rawIndex))

	digestsDir := writeDigests(t, map[string]string{"amd64": mergeDigestA})

	env := map[string]string{
		"PATH": dockerDir + string(os.PathListSeparator) + s.BinDir,
	}
	out, err := run(t, env,
		"merge",
		"--image=myorg/app",
		"--tags=v1",
		"--digests-dir="+digestsDir,
	)
	assert.NilError(t, err, "merge failed:\n%s", out)

	logData, readErr := os.ReadFile(logFile)
	assert.NilError(t, readErr)
	log := string(logData)
	assert.Check(t, !strings.Contains(log, " info"), "docker info must not be called: %s", log)
}
