//go:build acceptance

package setupdocker_test

import (
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/mod/semver"
	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"

	"github.com/circleci-functions/setup-docker/install"
)

// TestAcceptance_InstallsLatest runs setup-docker with latest for docker, buildx and compose,
// then asserts each tool reports a version at or above the pinned default.
func TestAcceptance_InstallsLatest(t *testing.T) {
	s := sdktest.NewSandbox(t)

	r := runner.New(t, binaryPath, runner.WithSandboxEnv(s.Env()))
	_, err := r.Run(
		"--docker-version", "latest",
		"--buildx-version", "latest",
		"--compose-version", "latest",
		"--install-dir", s.BinDir,
	)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "install latest: %s", out)

	dockerBin := filepath.Join(s.BinDir, "docker")
	vRe := regexp.MustCompile(`\d+\.\d+\.\d+`)

	for _, tc := range []struct {
		name string
		args []string
		min  string
	}{
		{name: "docker", args: []string{dockerBin, "--version"}, min: install.DefaultDockerVersion},
		{name: "buildx", args: []string{dockerBin, "buildx", "version"}, min: install.DefaultBuildxVersion},
		{name: "compose", args: []string{dockerBin, "compose", "version"}, min: install.DefaultComposeVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(tc.args[0], tc.args[1:]...) //nolint:gosec
			cmd.Env = sandboxCmdEnv(s.Home)
			got, cmdErr := cmd.CombinedOutput()
			assert.NilError(t, cmdErr, "cmd %v: %s", tc.args, got)

			m := vRe.Find(got)
			assert.Assert(t, m != nil, "no version in output of %v: %s", tc.args, got)

			gotV := string(m)
			if !strings.HasPrefix(gotV, "v") {
				gotV = "v" + gotV
			}
			minV := tc.min
			if !strings.HasPrefix(minV, "v") {
				minV = "v" + minV
			}
			assert.Assert(t, semver.IsValid(gotV), "extracted %q is not valid semver: %s", gotV, got)
			assert.Assert(t, semver.Compare(gotV, minV) >= 0,
				"expected version >= %s, got %s; output: %s", minV, gotV, got)
		})
	}
}
