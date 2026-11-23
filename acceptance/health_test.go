//go:build acceptance

package setupdocker_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/toolkit/testing/runner"
)

// healthDockerfile serves /www/$HEALTH_FILE, so a check only passes if the
// container got its environment.
const healthDockerfile = `FROM busybox:1.37
CMD mkdir -p /www && echo ok > "/www/$HEALTH_FILE" && exec httpd -f -p 8080 -h /www
`

// TestAcceptance_Health builds an image with build, then checks it with
// health. On remote Docker the HTTP check runs through the probe container.
func TestAcceptance_Health(t *testing.T) {
	skipUnlessBuildEngine(t)
	ensureDockerInstalled(t)

	const image = "docker.io/acc/health:t"
	dockerfile := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfile, []byte(healthDockerfile), 0o600))
	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run("build", "--image", "acc/health", "--tags", "t", "--dockerfile", dockerfile)
	assert.NilError(t, err, "build failed: %s", r.StdoutString()+r.StderrString())

	health := func(t *testing.T, env map[string]string, args ...string) (string, error) {
		t.Helper()
		r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars(), runner.WithUnsafeAdditionalEnvVars(env))
		_, err := r.Run(append([]string{"health", "--image", image}, args...)...)
		return r.StdoutString() + r.StderrString(), err
	}

	t.Run("http endpoint answers", func(t *testing.T) {
		out, err := health(t, nil, "--path", "/healthz", "--env", "HEALTH_FILE=healthz")
		assert.NilError(t, err, out)
		assert.Check(t, cmp.Contains(out, "is healthy"))
	})

	t.Run("pass-env reaches the container", func(t *testing.T) {
		out, err := health(t, map[string]string{"HEALTH_FILE": "ready"}, "--path", "/ready", "--pass-env", "HEALTH_FILE")
		assert.NilError(t, err, out)
	})

	t.Run("wrong path fails", func(t *testing.T) {
		out, err := health(t, nil, "--path", "/nope", "--env", "HEALTH_FILE=healthz", "--timeout", "6")
		assert.Check(t, err != nil, out)
		assert.Check(t, cmp.Contains(out, "returned 404"))
	})

	t.Run("crash fails with the container's logs", func(t *testing.T) {
		out, err := health(t, nil, "--path", "/", "--command", `sh -c "echo boom; exit 3"`)
		assert.Check(t, err != nil, out)
		assert.Check(t, cmp.Contains(out, "exited with code 3"))
		assert.Check(t, cmp.Contains(out, "boom"))
	})

	t.Run("HEALTHCHECK passes", func(t *testing.T) {
		out, err := health(t, nil, "--env", "HEALTH_FILE=healthz", "--run-args=--health-cmd 'test -f /www/healthz'")
		assert.NilError(t, err, out)
	})

	t.Run("no path and no HEALTHCHECK fails", func(t *testing.T) {
		out, err := health(t, nil, "--env", "HEALTH_FILE=healthz")
		assert.Check(t, err != nil, out)
		assert.Check(t, cmp.Contains(out, "has no HEALTHCHECK"))
	})

	t.Run("no containers are left behind", func(t *testing.T) {
		out, err := exec.Command("docker", "ps", "-aq", "--filter", "ancestor="+image).Output() //nolint:gosec
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(strings.TrimSpace(string(out)), ""))
	})
}
