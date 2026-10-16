//go:build acceptance

package setupdocker_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
)

// TestAcceptance_LinksOntoTheRealPath runs setup-docker without --install-dir
// so it chooses a PATH directory itself. It only runs where the machine is
// disposable: a CircleCI job, or a container the caller vouches for with
// $SETUP_DOCKER_DISPOSABLE.
func TestAcceptance_LinksOntoTheRealPath(t *testing.T) {
	if os.Getenv("SETUP_DOCKER_DISPOSABLE") == "" {
		sdktest.SkipUnlessInJob(t)
	}

	bashEnv := filepath.Join(t.TempDir(), "bash_env")
	r := runner.New(t, binaryPath,
		runner.WithUnsafeOSEnvVars(),
		runner.WithUnsafeAdditionalEnvVars(map[string]string{
			"BASH_ENV": bashEnv,
		}))

	_, err := r.Run("--compose-version", "v2.29.7")
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "setup-docker failed: %s", out)

	t.Run("docker-compose resolves in a later step", func(t *testing.T) {
		got, stepErr := sdktest.LaterStep(t, "docker-compose", "version")
		assert.NilError(t, stepErr, "docker-compose not found: %s", got)
		assert.Assert(t, strings.Contains(got, "2.29.7"),
			"expected 2.29.7 in docker-compose version: %s", got)
	})

	t.Run("docker compose resolves in a later step", func(t *testing.T) {
		if _, lookErr := exec.LookPath("docker"); lookErr != nil {
			t.Skip("docker not on PATH")
		}
		got, stepErr := sdktest.LaterStep(t, "docker", "compose", "version")
		assert.NilError(t, stepErr, "docker compose failed: %s", got)
		assert.Assert(t, strings.Contains(got, "2.29.7"),
			"expected 2.29.7 in docker compose version: %s", got)
	})

	if runtime.GOOS == "windows" {
		t.Run("docker-compose resolves in PowerShell", func(t *testing.T) {
			got, stepErr := sdktest.LaterStep(t, "powershell.exe", "-Command", "docker-compose version")
			assert.NilError(t, stepErr, "PowerShell could not resolve docker-compose: %s", got)
			assert.Assert(t, strings.Contains(got, "2.29.7"),
				"expected 2.29.7 in docker-compose version: %s", got)
		})
	}

	t.Run("NothingIsExportedToAShell", func(t *testing.T) {
		_, statErr := os.Stat(bashEnv) //nolint:gosec // path this test chose
		assert.Assert(t, os.IsNotExist(statErr),
			"$BASH_ENV must not be written; got stat error: %v", statErr)
	})
}
