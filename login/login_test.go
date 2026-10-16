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

func TestLoginPassword_ErrorNeverContainsPassword(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script not supported on Windows")
	}
	withDockerConfig(t)
	ins := install.New("")
	secret := "super-secret-value-xyz"
	t.Setenv("SD_TEST_USER_NCP", "user")
	t.Setenv("SD_TEST_PASS_NCP", secret)
	fakeDockerDir(t, 1, "unauthorized: incorrect username or password")
	cfg := Config{ //nolint:gosec // env var names are not credentials
		Registry:    "docker.io",
		UsernameEnv: "SD_TEST_USER_NCP",
		PasswordEnv: "SD_TEST_PASS_NCP",
		Retries:     1,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.Check(t, err != nil)
	if err != nil {
		assert.Check(t, !strings.Contains(err.Error(), secret), "error must not contain the password")
	}
}

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

	dir := t.TempDir()
	countFile := filepath.Join(dir, "count")
	_ = os.WriteFile(countFile, []byte("0"), 0o600)
	script := fmt.Sprintf(`#!/bin/sh
n=$(cat %s 2>/dev/null || echo 0)
n=$((n+1))
printf "%%s" "$n" > %s
if [ "$n" -lt "2" ]; then
  echo "connection refused" >&2
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
		Retries:     3,
	}
	err := Login(context.Background(), ins, install.Current(), cfg)
	assert.NilError(t, err)

	countData, _ := os.ReadFile(countFile)
	assert.Check(t, cmp.Equal(strings.TrimSpace(string(countData)), "2"), "expected exactly 2 attempts")
}

func TestDockerLoginRetry_Unauthorized_NoRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake script not supported on Windows")
	}
	withDockerConfig(t)
	ins := install.New("")
	t.Setenv("SD_TEST_UNAUTH_USER", "u")
	t.Setenv("SD_TEST_UNAUTH_PASS", "wrong")

	dir := t.TempDir()
	countFile := filepath.Join(dir, "count")
	_ = os.WriteFile(countFile, []byte("0"), 0o600)
	script := fmt.Sprintf(`#!/bin/sh
n=$(cat %s 2>/dev/null || echo 0)
printf "%%s" "$((n+1))" > %s
echo "unauthorized: incorrect username or password" >&2
exit 1
`, countFile, countFile)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

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

func TestDockerLoginRetry_Exhausted_ErrorNamedRegistry(t *testing.T) {
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
	fakeDockerDir(t, 1, "connection reset by peer")

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

// fakeDockerDir creates a fake docker binary in a temp dir and prepends it to PATH.
func fakeDockerDir(t *testing.T, exitCode int, output string) {
	t.Helper()
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' '%s' >&2\nexit %d\n", output, exitCode)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}
