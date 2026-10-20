package clitest_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/setup-docker/install"
)

func skipLoginOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("login CLI tests require shell scripts, not supported on Windows")
	}
}

// ecrLoginRoutes returns the httptest handler routes for the ECR credential helper.
// os-arch is computed from runtime.GOOS/GOARCH, matching install.Platform.ECRLoginURLs.
func newECRServer(t *testing.T) *httptest.Server {
	t.Helper()
	helperBin := fakeScript("docker-credential-ecr-login version v" + install.DefaultECRLoginVersion)
	h := sha256.Sum256(helperBin)
	hexSum := hex.EncodeToString(h[:])

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Prefix must match what install.Platform.ECRLoginURLs produces.
	prefix := fmt.Sprintf("/%s/%s-%s/docker-credential-ecr-login", install.DefaultECRLoginVersion, goos, goarch)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case prefix + ".sha256":
			fmt.Fprintf(w, "%s  docker-credential-ecr-login\n", hexSum)
		case prefix:
			_, _ = w.Write(helperBin)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestCLI_Login_PasswordStdin: docker login is called with the right argv and stdin,
// and output never contains the password.
func TestCLI_Login_PasswordStdin(t *testing.T) {
	skipLoginOnWindows(t)
	s, run := sandbox(t)

	// Fake docker that records argv and stdin into files.
	dockerDir := t.TempDir()
	argFile := filepath.Join(dockerDir, "args.txt")
	stdinFile := filepath.Join(dockerDir, "stdin.txt")
	// Use absolute paths for non-builtin commands since PATH only contains dockerDir.
	script := fmt.Sprintf("#!/bin/sh\n/bin/echo \"$@\" > %s\n/bin/cat > %s\n", argFile, stdinFile)
	assert.NilError(t, os.WriteFile(filepath.Join(dockerDir, "docker"), []byte(script), 0o755))

	const testPass = "s3cr3t-t0ken"
	env := map[string]string{
		"PATH":            dockerDir + string(os.PathListSeparator) + s.BinDir,
		"DOCKER_LOGIN":    "myuser",
		"DOCKER_PASSWORD": testPass,
	}
	out, err := run(t, env, "login", "--registry", "ghcr.io")
	assert.NilError(t, err, "login failed:\n%s", out)

	// Check argv
	argsData, readErr := os.ReadFile(argFile)
	assert.NilError(t, readErr)
	args := strings.TrimSpace(string(argsData))
	assert.Check(t, cmp.Contains(args, "login"), "argv must contain login: %s", args)
	assert.Check(t, cmp.Contains(args, "--username"), "argv must contain --username: %s", args)
	assert.Check(t, cmp.Contains(args, "myuser"), "argv must contain username: %s", args)
	assert.Check(t, cmp.Contains(args, "--password-stdin"), "argv must contain --password-stdin: %s", args)
	assert.Check(t, cmp.Contains(args, "ghcr.io"), "argv must contain registry: %s", args)
	assert.Check(t, !strings.Contains(args, testPass), "argv must not contain password: %s", args)

	// Check stdin was the password
	stdinData, readErr := os.ReadFile(stdinFile)
	assert.NilError(t, readErr)
	assert.Check(t, cmp.Equal(strings.TrimSpace(string(stdinData)), testPass), "stdin must be the password")

	// Output must not contain the password
	assert.Check(t, !strings.Contains(out, testPass), "output must not contain the password: %s", out)
}

// TestCLI_Login_ECR: ECR registry downloads the helper, updates Docker config, does NOT call docker login.
func TestCLI_Login_ECR(t *testing.T) {
	skipLoginOnWindows(t)
	s, run := sandbox(t)

	ecrSrv := newECRServer(t)

	// A fake docker that exits non-zero to catch any unexpected call.
	dockerDir := t.TempDir()
	callFile := filepath.Join(dockerDir, "called.txt")
	script := fmt.Sprintf(`#!/bin/sh
echo "docker was called unexpectedly" > %s
exit 1
`, callFile)
	assert.NilError(t, os.WriteFile(filepath.Join(dockerDir, "docker"), []byte(script), 0o755))

	env := map[string]string{
		"PATH": dockerDir + string(os.PathListSeparator) + s.BinDir,
	}
	const ecrRegistry = "123456789012.dkr.ecr.us-east-1.amazonaws.com"
	out, err := run(t, env, "login",
		"--registry", ecrRegistry,
		"--download-url", ecrSrv.URL,
	)
	assert.NilError(t, err, "ECR login failed:\n%s", out)
	assert.Assert(t, cmp.Contains(out, "docker-credential-ecr-login"), "should mention ecr-login: %s", out)

	// docker must NOT have been called
	_, statErr := os.Stat(callFile)
	assert.Assert(t, os.IsNotExist(statErr), "docker must not be called for ECR login")

	// Docker config must have credHelpers entry
	configPath := filepath.Join(s.Home, ".docker", "config.json")
	data, readErr := os.ReadFile(configPath)
	assert.NilError(t, readErr, "config.json must exist: %s", configPath)
	assert.Assert(t, cmp.Contains(string(data), "ecr-login"), "config must contain ecr-login: %s", string(data))
	assert.Assert(t, cmp.Contains(string(data), ecrRegistry), "config must contain registry: %s", string(data))
}

// TestCLI_Login_MissingPassword: missing password env exits non-zero and output does not name the value.
func TestCLI_Login_MissingPassword(t *testing.T) {
	skipLoginOnWindows(t)
	_, run := sandbox(t)

	env := map[string]string{
		"DOCKER_LOGIN": "user",
		// DOCKER_PASSWORD is not set
	}
	out, err := run(t, env, "login", "--registry", "ghcr.io")
	assert.Assert(t, err != nil, "expected non-zero exit for missing password: %s", out)
	assert.Assert(t, cmp.Contains(out, "DOCKER_PASSWORD"), "error must name the missing env var: %s", out)
}
