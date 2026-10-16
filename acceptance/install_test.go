//go:build acceptance

package setupdocker_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
)

func TestAcceptance_InstallsRequestedVersions(t *testing.T) {
	s := sdktest.NewSandbox(t)

	r := runner.New(t, binaryPath, runner.WithSandboxEnv(s.Env()))
	_, err := r.Run(
		"--docker-version", "29.9.1",
		"--buildx-version", "v0.38.0",
		"--compose-version", "v2.29.7",
		"--install-dockerize",
		"--install-goss",
		"--install-dir", s.BinDir,
	)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "first run: %s", out)

	dockerBin := filepath.Join(s.BinDir, "docker")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "docker version", args: []string{dockerBin, "--version"}, want: "29.9.1"},
		{name: "buildx version", args: []string{dockerBin, "buildx", "version"}, want: "v0.38.0"},
		{name: "compose version", args: []string{dockerBin, "compose", "version"}, want: "2.29.7"},
		{name: "docker-compose version", args: []string{filepath.Join(s.BinDir, "docker-compose"), "version"}, want: "2.29.7"},
		{name: "dockerize version", args: []string{filepath.Join(s.BinDir, "dockerize"), "--version"}, want: "v0.15.2"},
		{name: "goss version", args: []string{filepath.Join(s.BinDir, "goss"), "--version"}, want: "0.4.10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(tc.args[0], tc.args[1:]...) //nolint:gosec
			cmd.Env = sandboxCmdEnv(s.Home)
			got, cmdErr := cmd.CombinedOutput()
			assert.NilError(t, cmdErr, "cmd %v: %s", tc.args, got)
			assert.Assert(t, strings.Contains(string(got), tc.want),
				"expected %q in output of %v: %s", tc.want, tc.args, got)
		})
	}

	t.Run("dgoss exists", func(t *testing.T) {
		_, statErr := os.Stat(filepath.Join(s.BinDir, "dgoss"))
		assert.NilError(t, statErr, "dgoss not found in BinDir %s", s.BinDir)
	})

	t.Run("AlreadyInstalled", func(t *testing.T) {
		r.Reset()
		_, err = r.Run(
			"--docker-version", "29.9.1",
			"--buildx-version", "v0.38.0",
			"--compose-version", "v2.29.7",
			"--install-dockerize",
			"--install-goss",
			"--install-dir", s.BinDir,
		)
		secondOut := r.StdoutString() + r.StderrString()
		assert.NilError(t, err, "second run: %s", secondOut)
		assert.Assert(t, strings.Contains(secondOut, "is already installed"),
			"expected 'is already installed' in second run output: %s", secondOut)
	})
}

// sandboxCmdEnv returns the current process env with HOME replaced by
// sandboxHome. Docker reads $HOME to find its cli-plugins directory, so the
// sandbox home ensures it finds plugins setup-docker installed there.
func sandboxCmdEnv(sandboxHome string) []string {
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "HOME=") || strings.HasPrefix(kv, "DOCKER_CONFIG=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "HOME="+sandboxHome)
}
