//go:build acceptance

package setupdocker_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/toolkit/testing/runner"
)

// registryEnv returns the registry address and credentials from the environment,
// skipping t if any are absent.
func registryEnv(t *testing.T) (string, string, string) {
	t.Helper()
	registry := os.Getenv("SETUP_DOCKER_TEST_REGISTRY")
	user := os.Getenv("SETUP_DOCKER_TEST_REGISTRY_USER")
	pass := os.Getenv("SETUP_DOCKER_TEST_REGISTRY_PASS")
	if registry == "" || user == "" || pass == "" {
		t.Skip("SETUP_DOCKER_TEST_REGISTRY, SETUP_DOCKER_TEST_REGISTRY_USER and SETUP_DOCKER_TEST_REGISTRY_PASS must all be set")
	}
	return registry, user, pass
}

// withDockerConfig returns os.Environ() with DOCKER_CONFIG overridden to dir.
func withDockerConfig(dir string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "DOCKER_CONFIG=") {
			env = append(env, kv)
		}
	}
	return append(env, "DOCKER_CONFIG="+dir)
}

// TestAcceptance_LoginToRegistry exercises the login subcommand against a real
// htpasswd-protected registry started by the CI job.
func TestAcceptance_LoginToRegistry(t *testing.T) {
	registry, user, pass := registryEnv(t)

	t.Run("password", func(t *testing.T) {
		dockerConfig := t.TempDir()
		r := runner.New(t, binaryPath,
			runner.WithUnsafeOSEnvVars(),
			runner.WithUnsafeAdditionalEnvVars(map[string]string{
				"DOCKER_CONFIG": dockerConfig,
				"REG_USER":      user,
				"REG_PASS":      pass,
			}))
		_, err := r.Run("login",
			"--registry", registry,
			"--username-env", "REG_USER",
			"--password-env", "REG_PASS",
		)
		out := r.StdoutString() + r.StderrString()
		assert.NilError(t, err, "login failed: %s", out)

		tag := fmt.Sprintf("%s/busybox:%d", registry, time.Now().UnixNano())
		env := withDockerConfig(dockerConfig)
		for _, step := range []struct {
			name string
			args []string
		}{
			{"pull", []string{"docker", "pull", "busybox"}},
			{"tag", []string{"docker", "tag", "busybox", tag}},
			{"push", []string{"docker", "push", tag}},
		} {
			cmd := exec.Command(step.args[0], step.args[1:]...) //nolint:gosec
			cmd.Env = env
			got, cmdErr := cmd.CombinedOutput()
			assert.NilError(t, cmdErr, "%s failed: %s", step.name, got)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		dockerConfig := t.TempDir()
		r := runner.New(t, binaryPath,
			runner.WithUnsafeOSEnvVars(),
			runner.WithUnsafeAdditionalEnvVars(map[string]string{
				"DOCKER_CONFIG": dockerConfig,
				"REG_USER":      user,
				"REG_PASS":      "wrongpassword",
			}))
		_, err := r.Run("login",
			"--registry", registry,
			"--username-env", "REG_USER",
			"--password-env", "REG_PASS",
			"--retries", "1",
		)
		out := r.StdoutString() + r.StderrString()
		assert.Assert(t, err != nil, "login with wrong password should fail; got: %s", out)
		assert.Assert(t, !strings.Contains(out, "wrongpassword"),
			"output must not contain the wrong password: %s", out)
		lower := strings.ToLower(out)
		assert.Assert(t, strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"),
			"expected 401 or unauthorized in output: %s", out)
	})

	t.Run("credentials store", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("credentials-store subtest is Linux only")
		}
		dockerConfig := t.TempDir()
		r := runner.New(t, binaryPath,
			runner.WithUnsafeOSEnvVars(),
			runner.WithUnsafeAdditionalEnvVars(map[string]string{
				"DOCKER_CONFIG": dockerConfig,
				"REG_USER":      user,
				"REG_PASS":      pass,
			}))
		_, err := r.Run("login",
			"--registry", registry,
			"--username-env", "REG_USER",
			"--password-env", "REG_PASS",
			"--credentials-store",
		)
		out := r.StdoutString() + r.StderrString()
		assert.NilError(t, err, "login with credentials-store failed: %s", out)

		configJSON, readErr := os.ReadFile(dockerConfig + "/config.json") //nolint:gosec
		assert.NilError(t, readErr)

		var cfg struct {
			CredsStore string                       `json:"credsStore"`
			Auths      map[string]map[string]string `json:"auths"`
		}
		assert.NilError(t, json.Unmarshal(configJSON, &cfg))
		assert.Check(t, cmp.Equal(cfg.CredsStore, "pass"))
		assert.Check(t, cmp.Equal(cfg.Auths[registry]["auth"], ""),
			"expected no inline auth for %s", registry)

		tag := fmt.Sprintf("%s/busybox:%d", registry, time.Now().UnixNano())
		env := withDockerConfig(dockerConfig)
		for _, step := range []struct {
			name string
			args []string
		}{
			{"tag", []string{"docker", "tag", "busybox", tag}},
			{"push", []string{"docker", "push", tag}},
		} {
			cmd := exec.Command(step.args[0], step.args[1:]...) //nolint:gosec
			cmd.Env = env
			got, cmdErr := cmd.CombinedOutput()
			assert.NilError(t, cmdErr, "%s failed: %s", step.name, got)
		}
	})
}
