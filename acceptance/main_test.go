//go:build acceptance

// Package setupdocker_test exercises setup-docker against real infrastructure:
// real downloads and a real CircleCI task-agent when running inside a job.
// Tagged out of `task test` because it needs network.
package setupdocker_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/circleci-functions/toolkit/testing/compiler"
)

// binaryPathEnvVar names a prebuilt setup-docker to test, for an executor with
// no Go toolchain. The smoke jobs set it, paired with a cross-compiled suite.
const binaryPathEnvVar = "SETUP_DOCKER_BINARY"

var binaryPath string

func TestMain(m *testing.M) {
	status, err := runTests(m)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(status)
}

func runTests(m *testing.M) (int, error) {
	if bin := os.Getenv(binaryPathEnvVar); bin != "" {
		abs, err := filepath.Abs(bin)
		if err != nil {
			return 0, fmt.Errorf("%s=%q: %w", binaryPathEnvVar, bin, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return 0, fmt.Errorf("%s=%q: %w", binaryPathEnvVar, bin, err)
		}
		binaryPath = abs
		return m.Run(), nil
	}

	p := compiler.NewParallel(1)
	defer p.Cleanup()

	p.Add(compiler.Work{
		Result: &binaryPath,
		Name:   "setup-docker",
		Target: "..",
		Source: ".",
	})

	if err := p.Run(context.Background()); err != nil {
		return 0, err
	}
	return m.Run(), nil
}
