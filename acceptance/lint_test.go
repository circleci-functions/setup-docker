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

	t.Run("failure-threshold error passes a warning-only file", func(t *testing.T) {
		warnFile := filepath.Join(tmp, "Dockerfile.warn")
		assert.NilError(t, os.WriteFile(warnFile, []byte("FROM alpine:3.22\nRUN apk add curl\n"), 0o600))
		r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
		_, err := r.Run("lint", "--failure-threshold", "error", "--dockerfiles", warnFile)
		out := r.StdoutString() + r.StderrString()
		assert.NilError(t, err, "warnings are below the error threshold: %s", out)
	})

	t.Run("results file written with DL4000", func(t *testing.T) {
		resultsFile := filepath.Join(tmp, "results.xml")
		r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
		_, _ = r.Run("lint",
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
