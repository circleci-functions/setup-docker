//go:build acceptance

// Package setupdocker_test exercises the compiled setup-docker binary against
// real downloads: each install reaches the upstream release host and the
// resulting binary is resolved through PATH in a later step. Tagged out of
// `task test` because it needs network and mutates PATH directories. The CLI
// contract itself is covered hermetically by clitest/.
package setupdocker_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/compiler"
	"github.com/circleci-functions/toolkit/testing/runner"
)

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

// runInstall runs the compiled binary in a fresh sandbox environment and returns
// its combined output and exit error.
func runInstall(t *testing.T, args ...string) (string, error) {
	t.Helper()
	s := sdktest.NewSandbox(t)
	r := runner.New(t, binaryPath, runner.WithSandboxEnv(s.Env()))
	_, err := r.Run(args...)
	return r.StdoutString() + r.StderrString(), err
}
