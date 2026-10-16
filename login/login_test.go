package login

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/setup-docker/install"
)

// ecrRegistryRe mirrors the compiled regex in login.go for table tests.
var ecrRegistryRe = regexp.MustCompile(`^\d{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com(\.cn)?$`)

func TestECRRegex(t *testing.T) {
	cases := []struct {
		name  string
		input string
		match bool
	}{
		{name: "valid us-east-1", input: "123456789012.dkr.ecr.us-east-1.amazonaws.com", match: true},
		{name: "valid eu-west-2", input: "000000000001.dkr.ecr.eu-west-2.amazonaws.com", match: true},
		{name: "valid cn region", input: "123456789012.dkr.ecr.cn-north-1.amazonaws.com.cn", match: true},
		{name: "docker hub", input: "docker.io", match: false},
		{name: "ghcr", input: "ghcr.io", match: false},
		{name: "too few account digits", input: "12345678901.dkr.ecr.us-east-1.amazonaws.com", match: false},
		{name: "too many account digits", input: "1234567890123.dkr.ecr.us-east-1.amazonaws.com", match: false},
		{name: "missing dkr segment", input: "123456789012.ecr.us-east-1.amazonaws.com", match: false},
		{name: "gcr lookalike", input: "123456789012.dkr.ecr.us-east-1.gcr.io", match: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ecrRegistryRe.MatchString(tc.input)
			assert.Check(t, cmp.Equal(got, tc.match))
		})
	}
}

func TestLoginPassword_MissingUsernameEnv(t *testing.T) {
	withDockerConfig(t)
	ins := install.New("")
	cfg := Config{
		Registry:    "docker.io",
		UsernameEnv: "SD_TEST_MISSING_USER_VAR",
		PasswordEnv: "SD_TEST_MISSING_PASS_VAR",
		Retries:     1,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.Check(t, cmp.ErrorContains(err, "SD_TEST_MISSING_USER_VAR"))
	assert.Check(t, cmp.ErrorContains(err, "is not set"))
}

func TestLoginPassword_MissingPasswordEnv(t *testing.T) {
	withDockerConfig(t)
	ins := install.New("")
	t.Setenv("SD_TEST_USER_PRESENT", "testuser")
	cfg := Config{ //nolint:gosec // env var names are not credentials
		Registry:    "docker.io",
		UsernameEnv: "SD_TEST_USER_PRESENT",
		PasswordEnv: "SD_TEST_PASS_ABSENT",
		Retries:     1,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.Check(t, cmp.ErrorContains(err, "SD_TEST_PASS_ABSENT"))
	assert.Check(t, cmp.ErrorContains(err, "is not set"))
}

// TestDockerLoginRetry_ZeroRetries: retries=0 must still make exactly 1 attempt.
func TestDockerLoginRetry_ZeroRetries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script not supported on Windows")
	}
	withDockerConfig(t)
	ins := install.New("")
	t.Setenv("SD_TEST_ZERO_USER", "u")
	t.Setenv("SD_TEST_ZERO_PASS", "p")

	countFile := fakeDockerCounter(t, 1, "connection refused")

	cfg := Config{ //nolint:gosec // env var names are not credentials
		Registry:    "docker.io",
		UsernameEnv: "SD_TEST_ZERO_USER",
		PasswordEnv: "SD_TEST_ZERO_PASS",
		Retries:     0,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.Check(t, err != nil)

	countData, _ := os.ReadFile(countFile)
	assert.Check(t, cmp.Equal(strings.TrimSpace(string(countData)), "1"), "retries=0 must make exactly 1 attempt")
}

// TestDockerLoginRetry_TransientThenSuccess: 2 retries → up to 3 attempts; transient then success takes 2.
func TestDockerLoginRetry_TransientThenSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script not supported on Windows")
	}
	if testing.Short() {
		t.Skip("retry test has sleeps")
	}
	withDockerConfig(t)
	ins := install.New("")
	t.Setenv("SD_TEST_RETRY_USER", "u")
	t.Setenv("SD_TEST_RETRY_PASS", "p")

	// Fails on attempt 1, succeeds on attempt 2.
	dir := t.TempDir()
	countFile := filepath.Join(dir, "count")
	_ = os.WriteFile(countFile, []byte("0"), 0o600)
	script := fmt.Sprintf(`#!/bin/sh
n=$(/bin/cat %s 2>/dev/null || /bin/echo 0)
n=$((n+1))
/bin/echo $n > %s
if [ "$n" -lt "2" ]; then
  /bin/echo "connection refused" >&2
  exit 1
fi
exit 0
`, countFile, countFile)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	cfg := Config{ //nolint:gosec // env var names are not credentials
		Registry:    "docker.io",
		UsernameEnv: "SD_TEST_RETRY_USER",
		PasswordEnv: "SD_TEST_RETRY_PASS",
		Retries:     2,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.NilError(t, err)

	countData, _ := os.ReadFile(countFile)
	assert.Check(t, cmp.Equal(strings.TrimSpace(string(countData)), "2"), "expected exactly 2 attempts")
}

// TestDockerLoginRetry_Unauthorized_NoRetry: credential rejection → exactly 1 attempt, error names reason.
func TestDockerLoginRetry_Unauthorized_NoRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script not supported on Windows")
	}
	withDockerConfig(t)
	ins := install.New("")
	t.Setenv("SD_TEST_UNAUTH_USER", "u")
	t.Setenv("SD_TEST_UNAUTH_PASS", "wrong")

	countFile := fakeDockerCounter(t, 1, "unauthorized: incorrect username or password")

	cfg := Config{ //nolint:gosec // env var names are not credentials
		Registry:    "docker.io",
		UsernameEnv: "SD_TEST_UNAUTH_USER",
		PasswordEnv: "SD_TEST_UNAUTH_PASS",
		Retries:     3,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.Check(t, cmp.ErrorContains(err, "credentials rejected"))

	countData, _ := os.ReadFile(countFile)
	assert.Check(t, cmp.Equal(strings.TrimSpace(string(countData)), "1"), "should not retry on unauthorized")
}

// TestDockerLoginRetry_Exhausted: exhausted → error names registry, never password.
func TestDockerLoginRetry_Exhausted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script not supported on Windows")
	}
	if testing.Short() {
		t.Skip("retry test has sleeps")
	}
	withDockerConfig(t)
	ins := install.New("")
	t.Setenv("SD_TEST_EXH_USER", "u")
	t.Setenv("SD_TEST_EXH_PASS", "secretpassword")
	fakeDockerSimple(t, 1, "connection reset by peer")

	cfg := Config{ //nolint:gosec // env var names are not credentials
		Registry:    "myregistry.example.com",
		UsernameEnv: "SD_TEST_EXH_USER",
		PasswordEnv: "SD_TEST_EXH_PASS",
		Retries:     2,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.Check(t, err != nil)
	if err != nil {
		assert.Check(t, cmp.Contains(err.Error(), "myregistry.example.com"), "error should name the registry")
		assert.Check(t, !strings.Contains(err.Error(), "secretpassword"), "error must not contain password")
	}
}

// TestDockerLogin_ErrorOutputNeverContainsPassword: docker output is surfaced in the error/stderr
// but the password must never appear there.
func TestDockerLogin_ErrorOutputNeverContainsPassword(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script not supported on Windows")
	}
	withDockerConfig(t)
	ins := install.New("")

	secret := "ultra-secret-xy7z" //nolint:gosec // test string, not a real credential
	t.Setenv("SD_TEST_NCP_USER", "user")
	t.Setenv("SD_TEST_NCP_PASS", secret)

	// Fake docker that prints a typical error message (no password) and exits 1.
	fakeDockerSimple(t, 1, "Error response from daemon: login attempt to https://registry-1.docker.io/v2/ failed with status: 401 Unauthorized")

	cfg := Config{ //nolint:gosec // env var names are not credentials
		Registry:    "docker.io",
		UsernameEnv: "SD_TEST_NCP_USER",
		PasswordEnv: "SD_TEST_NCP_PASS",
		Retries:     0,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.Check(t, err != nil)
	if err != nil {
		assert.Check(t, !strings.Contains(err.Error(), secret), "error must not contain password: %s", err)
	}
}

func TestLoginECR_ConfigUpdated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script not supported on Windows")
	}
	withDockerConfig(t)

	helperContent := []byte("#!/bin/sh\necho fake-ecr-login\n")
	h := sha256.Sum256(helperContent)
	sha256Content := fmt.Sprintf("%x  docker-credential-ecr-login\n", h)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".sha256"):
			_, _ = w.Write([]byte(sha256Content))
		default:
			_, _ = w.Write(helperContent)
		}
	}))
	defer srv.Close()

	ins := install.New("")
	ins.CacheDir = t.TempDir()

	cfg := Config{
		Registry:    "123456789012.dkr.ecr.us-east-1.amazonaws.com",
		UsernameEnv: "DOCKER_LOGIN",
		PasswordEnv: "DOCKER_PASSWORD",
		Retries:     1,
		DownloadURL: srv.URL,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.NilError(t, err)

	data, err := os.ReadFile(dockerConfigPath())
	assert.NilError(t, err)
	assert.Assert(t, cmp.Contains(string(data), "ecr-login"))
	assert.Assert(t, cmp.Contains(string(data), cfg.Registry))
}

// fakeDockerCounter creates a fake docker that counts calls and always exits with exitCode.
// Returns the count file path.
func fakeDockerCounter(t *testing.T, exitCode int, output string) string {
	t.Helper()
	dir := t.TempDir()
	countFile := filepath.Join(dir, "count")
	_ = os.WriteFile(countFile, []byte("0"), 0o600)
	script := fmt.Sprintf(`#!/bin/sh
n=$(/bin/cat %s 2>/dev/null || /bin/echo 0)
/bin/echo $((n+1)) > %s
/bin/echo '%s' >&2
exit %d
`, countFile, countFile, output, exitCode)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return countFile
}

// fakeDockerSimple creates a fake docker that always exits with exitCode and prints output.
func fakeDockerSimple(t *testing.T, exitCode int, output string) {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\n/bin/echo '%s' >&2\nexit %d\n", output, exitCode)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}
