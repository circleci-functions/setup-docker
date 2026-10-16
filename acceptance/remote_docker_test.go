//go:build acceptance

package setupdocker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
)

// skipUnlessRemoteDocker skips t unless DOCKER_HOST is set (setup_remote_docker
// sets it) and the machine is disposable.
func skipUnlessRemoteDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("DOCKER_HOST") == "" {
		t.Skip("DOCKER_HOST not set; skipping remote Docker test")
	}
	if os.Getenv("SETUP_DOCKER_DISPOSABLE") == "" {
		sdktest.SkipUnlessInJob(t)
	}
}

const remoteDockerComposeFile = `services:
  hello:
    image: busybox
    command: ["echo", "hello from compose"]
`

// TestAcceptance_ComposeOnRemoteDocker installs Compose via setup-docker and
// runs a one-shot service against the remote Docker daemon.
func TestAcceptance_ComposeOnRemoteDocker(t *testing.T) {
	skipUnlessRemoteDocker(t)

	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run("--compose-version", "v2.29.7")
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "setup-docker failed: %s", out)

	f := filepath.Join(t.TempDir(), "compose.yml")
	assert.NilError(t, os.WriteFile(f, []byte(remoteDockerComposeFile), 0o600))

	t.Run("compose up outputs hello from compose", func(t *testing.T) {
		got, stepErr := sdktest.LaterStep(t, "docker", "compose", "-f", f, "up", "--abort-on-container-exit")
		assert.NilError(t, stepErr, "docker compose up failed: %s", got)
		assert.Assert(t, strings.Contains(got, "hello from compose"),
			"expected 'hello from compose' in output: %s", got)
	})

	t.Run("docker-compose version reports 2.29.7", func(t *testing.T) {
		got, stepErr := sdktest.LaterStep(t, "docker-compose", "version")
		assert.NilError(t, stepErr, "docker-compose version failed: %s", got)
		assert.Assert(t, strings.Contains(got, "2.29.7"),
			"expected 2.29.7 in docker-compose version: %s", got)
	})
}
