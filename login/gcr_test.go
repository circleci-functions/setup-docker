package login

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/base64"
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

const testKey = `{"type":"service_account","client_email":"ci@p.iam.gserviceaccount.com"}`

func TestGARRegex(t *testing.T) {
	for host, want := range map[string]bool{
		"us-docker.pkg.dev":           true,
		"europe-west2-docker.pkg.dev": true,
		"gcr.io":                      true,
		"eu.gcr.io":                   true,
		"us-npm.pkg.dev":              false,
		"docker.io":                   false,
		"gcr.io.evil.com":             false,
		"evilgcr.io":                  false,
	} {
		assert.Check(t, cmp.Equal(garRe.MatchString(host), want), host)
	}
}

func TestDecodeKey(t *testing.T) {
	got, err := decodeKey(testKey)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(string(got), testKey))

	got, err = decodeKey(base64.StdEncoding.EncodeToString([]byte(testKey)))
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(string(got), testKey))

	_, err = decodeKey("not a key")
	assert.Check(t, cmp.ErrorContains(err, "not JSON or base64-encoded JSON"))
	_, err = decodeKey("{broken")
	assert.Check(t, cmp.ErrorContains(err, "not valid JSON"))
}

// isolateGoogle points HOME at a temp dir and hides gcloud and any ADC env var.
func isolateGoogle(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("ADC lives under APPDATA on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
}

func TestEnsureADC(t *testing.T) {
	t.Run("writes the key", func(t *testing.T) {
		adc := isolateGoogle(t)
		t.Setenv("GCP_KEY", testKey)
		assert.NilError(t, ensureADC("GCP_KEY"))
		data, err := os.ReadFile(adc) //nolint:gosec
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(string(data), testKey))
		info, err := os.Stat(adc)
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(info.Mode().Perm(), os.FileMode(0o600)))
	})

	t.Run("keeps existing credentials", func(t *testing.T) {
		adc := isolateGoogle(t)
		assert.NilError(t, os.MkdirAll(filepath.Dir(adc), 0o700))
		assert.NilError(t, os.WriteFile(adc, []byte(`{"type":"external_account"}`), 0o600))
		t.Setenv("GCP_KEY", testKey)
		assert.NilError(t, ensureADC("GCP_KEY"))
		data, err := os.ReadFile(adc) //nolint:gosec
		assert.NilError(t, err)
		assert.Check(t, cmp.Contains(string(data), "external_account"))
	})

	t.Run("uses GOOGLE_APPLICATION_CREDENTIALS", func(t *testing.T) {
		adc := isolateGoogle(t)
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/some/key.json")
		t.Setenv("GCP_KEY", testKey)
		assert.NilError(t, ensureADC("GCP_KEY"))
		_, err := os.Stat(adc)
		assert.Check(t, os.IsNotExist(err))
	})

	t.Run("no credentials", func(t *testing.T) {
		isolateGoogle(t)
		err := ensureADC("GCP_KEY")
		assert.Check(t, cmp.ErrorContains(err, "set GCP_KEY to a service account key"))
	})
}

func TestLoginGAR(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exitCode int
		wantErr  string
	}{
		{name: "sets the helper"},
		{name: "fails when the helper gets no token", exitCode: 1, wantErr: "could not get a token for us-docker.pkg.dev: no token here"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateGoogle(t)
			configPath := withDockerConfig(t)
			t.Setenv("GCP_KEY", testKey)
			// The fake helper fails unless asked for the registry on stdin.
			script := fmt.Sprintf("#!/bin/sh\n[ \"$1\" = get ] && [ \"$(/bin/cat)\" = us-docker.pkg.dev ] || exit 2\n/bin/echo 'no token here' >&2\nexit %d\n", tc.exitCode)
			srv := gcrHelperServer(t, []byte(script))

			ins := install.New(t.TempDir())
			ins.CacheDir = t.TempDir()
			ins.GitHubURL = srv.URL
			err := Login(context.Background(), ins, install.Current(), Config{
				Registry:  "us-docker.pkg.dev",
				GCPKeyEnv: "GCP_KEY",
			})
			if tc.wantErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.wantErr))
				_, statErr := os.Stat(configPath)
				assert.Check(t, os.IsNotExist(statErr), "docker config must not be written")
				return
			}
			assert.NilError(t, err)
			data, err := os.ReadFile(configPath) //nolint:gosec
			assert.NilError(t, err)
			assert.Check(t, cmp.Contains(string(data), `"us-docker.pkg.dev": "gcr"`))
		})
	}
}

func gcrHelperServer(t *testing.T, helper []byte) *httptest.Server {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	assert.NilError(t, tw.WriteHeader(&tar.Header{Name: "docker-credential-gcr", Mode: 0o755, Size: int64(len(helper))}))
	_, err := tw.Write(helper)
	assert.NilError(t, err)
	assert.NilError(t, tw.Close())
	assert.NilError(t, gz.Close())
	archive := buf.Bytes()
	sum := sha256.Sum256(archive)

	p := install.Current()
	asset := fmt.Sprintf("docker-credential-gcr_%s_%s-%s.tar.gz", p.OS, p.Arch, install.DefaultGCRHelperVersion)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksums.txt"):
			fmt.Fprintf(w, "%x  %s\n", sum, asset)
		case strings.HasSuffix(r.URL.Path, "/"+asset):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}
