//go:build acceptance

package setupdocker_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/toolkit/testing/runner"
)

// TestAcceptance_WindowsClassicBuild builds a Windows nanoserver image with the
// classic builder, runs it, and verifies a BuildKit-only flag is rejected.
func TestAcceptance_WindowsClassicBuild(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows classic build test only runs on Windows")
	}
	skipUnlessBuildEngine(t)

	dockerfile := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfile,
		[]byte("FROM mcr.microsoft.com/windows/nanoserver:ltsc2022\nCMD [\"cmd\", \"/c\", \"echo hello from windows\"]\n"),
		0o600))
	contextDir := filepath.Dir(dockerfile)

	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run("build",
		"--image", "acc/win",
		"--tags", "t",
		"--dockerfile", dockerfile,
		"--context", contextDir,
		"--cache=false",
	)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "build failed: %s", out)

	got, runErr := exec.Command("docker", "run", "--rm", "docker.io/acc/win:t").CombinedOutput() //nolint:gosec
	assert.NilError(t, runErr, "docker run failed: %s", got)
	assert.Assert(t, cmp.Contains(string(got), "hello from windows"),
		"expected 'hello from windows' in output: %s", got)

	r2 := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err = r2.Run("build",
		"--image", "acc/win",
		"--tags", "t",
		"--dockerfile", dockerfile,
		"--context", contextDir,
		"--platforms", "linux/amd64",
		"--cache=false",
	)
	out2 := r2.StdoutString() + r2.StderrString()
	assert.Assert(t, err != nil, "expected error for platforms on Windows: %s", out2)
	assert.Assert(t, cmp.Contains(out2, "BuildKit"),
		"expected BuildKit in error: %s", out2)
}
