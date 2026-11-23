//go:build acceptance

package setupdocker_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/toolkit/pathlink"
	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
)

const fakeServiceAccount = "setup-docker-acc@example.iam.gserviceaccount.com"

// TestAcceptance_LoginGAR logs in to Artifact Registry with a made-up service
// account key. The real docker-credential-gcr signs its own token from the key
// without calling Google, so this covers everything short of Google accepting it.
func TestAcceptance_LoginGAR(t *testing.T) {
	if os.Getenv("SETUP_DOCKER_DISPOSABLE") == "" {
		sdktest.SkipUnlessInJob(t)
	}
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" {
		t.Skip("GOOGLE_APPLICATION_CREDENTIALS is set; it would be used instead of the test key")
	}
	adc := adcPath(t)
	if _, err := os.Stat(adc); err == nil {
		t.Skipf("%s already exists; it would be used instead of the test key", adc)
	}
	t.Cleanup(func() { _ = os.Remove(adc) })

	dockerConfig := t.TempDir()
	r := runner.New(t, binaryPath,
		runner.WithUnsafeOSEnvVars(),
		runner.WithUnsafeAdditionalEnvVars(map[string]string{
			"DOCKER_CONFIG":      dockerConfig,
			"GCLOUD_SERVICE_KEY": fakeServiceAccountKey(t),
		}))
	_, err := r.Run("login", "--registry", "us-docker.pkg.dev")
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "login failed: %s", out)
	assert.Check(t, cmp.Contains(out, "us-docker.pkg.dev uses docker-credential-gcr"))

	data, err := os.ReadFile(filepath.Join(dockerConfig, "config.json")) //nolint:gosec
	assert.NilError(t, err)
	assert.Check(t, cmp.Contains(string(data), `"us-docker.pkg.dev": "gcr"`))

	t.Run("a later step gets a token for the key", func(t *testing.T) {
		bin, err := exec.LookPath("docker-credential-gcr")
		assert.NilError(t, err, "docker-credential-gcr is not on PATH")
		cmd := pathlink.Command(bin, "get")
		cmd.Stdin = strings.NewReader("us-docker.pkg.dev")
		got, err := cmd.Output()
		assert.NilError(t, err, "docker-credential-gcr get failed")

		var creds struct{ Username, Secret string }
		assert.NilError(t, json.Unmarshal(got, &creds))
		assert.Check(t, strings.HasPrefix(creds.Username, "_dcgcr_"), "username: %s", creds.Username)
		assert.Check(t, cmp.Equal(jwtIssuer(t, creds.Secret), fakeServiceAccount))
	})
}

func adcPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "gcloud", "application_default_credentials.json")
	}
	home, err := os.UserHomeDir()
	assert.NilError(t, err)
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
}

func fakeServiceAccountKey(t *testing.T) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NilError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(k)
	assert.NilError(t, err)
	key, err := json.Marshal(map[string]string{ //nolint:gosec // a throwaway key made for this test
		"type":           "service_account",
		"project_id":     "setup-docker-acc",
		"private_key_id": "1",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"client_email":   fakeServiceAccount,
		"client_id":      "1",
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	assert.NilError(t, err)
	return string(key)
}

func jwtIssuer(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	assert.Assert(t, cmp.Len(parts, 3), "not a JWT")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	assert.NilError(t, err)
	var claims struct{ Iss string }
	assert.NilError(t, json.Unmarshal(payload, &claims))
	return claims.Iss
}
