// Package setupdocker_test exercises the compiled setup-docker binary's CLI
// contract hermetically: flag parsing, the emitted descriptor, and help output.
// It runs no installs and needs no network. The real installs are covered by
// acceptance/.
package setupdocker_test

import (
	"context"
	"fmt"
	"os"
	"testing"

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

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	r := runner.New(t, binaryPath)
	code, _ := r.Run(args...)
	return r.StdoutString(), r.StderrString(), code
}
