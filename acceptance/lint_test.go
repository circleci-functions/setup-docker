//go:build acceptance

package setupdocker_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
)

func TestAcceptance_Lint(t *testing.T) {
	if os.Getenv("SETUP_DOCKER_DISPOSABLE") == "" {
		sdktest.SkipUnlessInJob(t)
	}

	tmp := t.TempDir()
	cleanFile := filepath.Join(tmp, "Dockerfile.clean")
	badFile := filepath.Join(tmp, "Dockerfile.bad")
	assert.NilError(t, os.WriteFile(cleanFile,
		[]byte("FROM alpine:3.22 AS build\nRUN echo hi\n"), 0o644))
	assert.NilError(t, os.WriteFile(badFile,
		[]byte("FROM alpine:3.22\nMAINTAINER someone\n"), 0o644))

	t.Run("clean file passes", func(t *testing.T) {
		r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
		_, err := r.Run("lint", "--dockerfiles", cleanFile)
		out := r.StdoutString() + r.StderrString()
		assert.NilError(t, err, "clean Dockerfile should pass lint: %s", out)
	})

	t.Run("bad file fails", func(t *testing.T) {
		r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
		_, err := r.Run("lint", "--dockerfiles", badFile)
		out := r.StdoutString() + r.StderrString()
		assert.Assert(t, err != nil, "bad Dockerfile should fail lint; got: %s", out)
	})

	t.Run("failure-threshold none passes bad file", func(t *testing.T) {
		r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
		_, err := r.Run("lint", "--failure-threshold", "none", "--dockerfiles", badFile)
		out := r.StdoutString() + r.StderrString()
		assert.NilError(t, err, "--failure-threshold none should pass any file: %s", out)
	})

	t.Run("results file written with DL4000", func(t *testing.T) {
		resultsFile := filepath.Join(tmp, "results.xml")
		r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
		_, _ = r.Run("lint",
			"--failure-threshold", "none",
			"--dockerfiles", badFile,
			"--results-file", resultsFile,
		)
		data, readErr := os.ReadFile(resultsFile)
		assert.NilError(t, readErr, "results file not written")
		xml := string(data)
		assert.Check(t, cmp.Contains(xml, "DL4000"),
			"results file must contain DL4000 finding: %s", xml)
		assert.Check(t, strings.Contains(xml, "testsuites"),
			"results file must be valid JUnit XML: %s", xml)
	})
}
