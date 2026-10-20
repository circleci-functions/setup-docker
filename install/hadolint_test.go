package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestHadolintAsset(t *testing.T) {
	cases := []struct {
		name    string
		p       Platform
		want    string
		wantErr string
	}{
		{name: "linux/amd64", p: Platform{"linux", "amd64"}, want: "hadolint-linux-x86_64"},
		{name: "linux/arm64", p: Platform{"linux", "arm64"}, want: "hadolint-linux-arm64"},
		{name: "darwin/amd64", p: Platform{"darwin", "amd64"}, want: "hadolint-macos-x86_64"},
		{name: "darwin/arm64", p: Platform{"darwin", "arm64"}, want: "hadolint-macos-arm64"},
		{name: "windows/amd64", p: Platform{"windows", "amd64"}, want: "hadolint-windows-x86_64.exe"},
		{name: "windows/arm64 unsupported", p: Platform{"windows", "arm64"}, wantErr: "hadolint has no Windows arm64 build"},
		{name: "freebsd/amd64 unsupported", p: Platform{"freebsd", "amd64"}, wantErr: "unsupported platform freebsd/amd64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.p.hadolintAsset()
			if tc.wantErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.wantErr))
				return
			}
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(got, tc.want))
		})
	}
}

func buildHadolintServer(t *testing.T, p Platform, version string, bin []byte) *httptest.Server {
	t.Helper()
	asset, err := p.hadolintAsset()
	assert.NilError(t, err)
	h := sha256.Sum256(bin)
	hexSum := hex.EncodeToString(h[:])
	base := "/hadolint/hadolint/releases/download/" + version + "/"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case base + "checksums.sha256":
			fmt.Fprintf(w, "%s *%s\n", hexSum, asset)
		case base + asset:
			_, _ = w.Write(bin)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestHadolintInstall(t *testing.T) {
	skipWindows(t)
	t.Setenv("PATH", t.TempDir())
	p := Current()
	version := DefaultHadolintVersion
	bin := []byte(fmt.Sprintf("#!/bin/sh\necho 'Haskell Dockerfile Linter %s'\n", version))

	srv := buildHadolintServer(t, p, version, bin)
	defer srv.Close()

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.CacheDir = t.TempDir()

	res, err := ins.Hadolint(context.Background(), p, "")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Tool, "hadolint"))
	assert.Check(t, cmp.Equal(res.Version, version))
	assert.Check(t, cmp.Equal(res.Kept, false))

	_, err = os.Stat(filepath.Join(ins.CacheDir, "hadolint", version, "hadolint"))
	assert.NilError(t, err)
	_, err = os.Stat(filepath.Join(ins.InstallDir, "hadolint"))
	assert.NilError(t, err)
}

func TestHadolintChecksumMismatch(t *testing.T) {
	skipWindows(t)
	t.Setenv("PATH", t.TempDir())
	p := Current()
	version := DefaultHadolintVersion
	bin := []byte(fmt.Sprintf("#!/bin/sh\necho 'Haskell Dockerfile Linter %s'\n", version))

	// Build a server that serves the wrong hash
	asset, err := p.hadolintAsset()
	assert.NilError(t, err)
	base := "/hadolint/hadolint/releases/download/" + version + "/"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case base + "checksums.sha256":
			fmt.Fprintf(w, "%s *%s\n", "0000000000000000000000000000000000000000000000000000000000000000", asset)
		case base + asset:
			_, _ = w.Write(bin)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.CacheDir = t.TempDir()

	_, err = ins.Hadolint(context.Background(), p, "")
	assert.Check(t, cmp.ErrorContains(err, "mismatch"))
}

func TestHadolintKeepExisting(t *testing.T) {
	skipWindows(t)
	p := Current()
	version := DefaultHadolintVersion

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dir := t.TempDir()
	script := []byte(fmt.Sprintf("#!/bin/sh\necho 'Haskell Dockerfile Linter %s'\n", version))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "hadolint"), script, 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.CacheDir = t.TempDir()

	res, err := ins.Hadolint(context.Background(), p, "")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, true))
	assert.Check(t, cmp.Equal(requests, 0), "expected no HTTP requests when already installed")
}
