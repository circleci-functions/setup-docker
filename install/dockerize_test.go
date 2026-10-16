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
	"runtime"
	"sync/atomic"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func skipWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevation on Windows")
	}
}

func dockerizeInstaller(t *testing.T, srv *httptest.Server) Installer {
	t.Helper()
	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.CacheDir = t.TempDir()
	return ins
}

func buildDockerizeServer(t *testing.T, version string, p Platform, hash string) *httptest.Server {
	t.Helper()
	asset, err := p.dockerizeAsset(version, "/no/such/path")
	assert.NilError(t, err)

	bin := makeTarGz(t, map[string][]byte{
		"dockerize": []byte("#!/bin/sh\necho v0.15.2"),
		"README.md": []byte("readme"),
		"LICENSE":   []byte("license"),
	})
	archBytes, err := os.ReadFile(bin)
	assert.NilError(t, err)

	s := sha256.Sum256(archBytes)
	actualHash := hex.EncodeToString(s[:])
	if hash == "" {
		hash = actualHash
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/jwilder/dockerize/releases/download/" + version + "/"
		switch r.URL.Path {
		case base + "checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", hash, asset)
		case base + asset:
			http.ServeFile(w, r, bin)
		case "/jwilder/dockerize/releases/latest":
			http.Redirect(w, r, "/jwilder/dockerize/releases/tag/"+version, http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestDockerizeInstall(t *testing.T) {
	skipWindows(t)
	p := Current()
	version := "v0.15.2"

	cases := []struct {
		name    string
		badHash string
		wantErr string
	}{
		{name: "happy path"},
		{name: "checksum mismatch", badHash: "00000000000000000000000000000000000000000000000000000000000000aa", wantErr: "mismatch"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := buildDockerizeServer(t, version, p, tc.badHash)
			defer srv.Close()

			ins := dockerizeInstaller(t, srv)
			res, err := ins.Dockerize(context.Background(), p, "")
			if tc.wantErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.wantErr))
				return
			}
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(res.Tool, "dockerize"))
			assert.Check(t, cmp.Equal(res.Version, version))
			assert.Check(t, cmp.Equal(res.Kept, false))

			bin := filepath.Join(ins.CacheDir, "dockerize", version, "dockerize")
			_, err = os.Stat(bin)
			assert.NilError(t, err)
		})
	}
}

func TestDockerizeAlpineVariant(t *testing.T) {
	p := Platform{"linux", "amd64"}

	tmp := filepath.Join(t.TempDir(), "alpine-release")
	assert.NilError(t, os.WriteFile(tmp, []byte("3.18.0"), 0o644))

	asset, err := p.dockerizeAsset("v0.15.2", tmp)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(asset, "dockerize-alpine-linux-amd64-v0.15.2.tar.gz"))

	asset, err = p.dockerizeAsset("v0.15.2", "/no/such/path")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(asset, "dockerize-linux-amd64-v0.15.2.tar.gz"))
}

func TestDockerizeKeepExisting(t *testing.T) {
	skipWindows(t)
	p := Current()
	version := "v0.15.2"
	srv := buildDockerizeServer(t, version, p, "")
	defer srv.Close()

	var downloads atomic.Int32
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jwilder/dockerize/releases/download/"+version+"/checksums.txt" &&
			r.URL.Path != "/jwilder/dockerize/releases/latest" {
			downloads.Add(1)
		}
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	defer counting.Close()

	dir := t.TempDir()
	script := []byte("#!/bin/sh\necho v0.15.2\n")
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "dockerize"), script, 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ins := dockerizeInstaller(t, counting)
	res, err := ins.Dockerize(context.Background(), p, "")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, true))
	assert.Check(t, cmp.Equal(downloads.Load(), int32(0)), "expected no downloads when already installed")
}

func TestDockerizeExactMatch(t *testing.T) {
	skipWindows(t)
	p := Current()
	version := "v0.15.2"
	srv := buildDockerizeServer(t, version, p, "")
	defer srv.Close()

	var downloads atomic.Int32
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	defer counting.Close()

	dir := t.TempDir()
	script := []byte("#!/bin/sh\necho v0.15.2\n")
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "dockerize"), script, 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ins := dockerizeInstaller(t, counting)
	res, err := ins.Dockerize(context.Background(), p, "v0.15.2")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, true))
	assert.Check(t, cmp.Equal(downloads.Load(), int32(0)), "exact match should skip download")
}

func TestDockerizeOnlyBinaryExtracted(t *testing.T) {
	skipWindows(t)
	p := Current()
	version := "v0.15.2"

	asset, err := p.dockerizeAsset(version, "/no/such/path")
	assert.NilError(t, err)

	arch := makeTarGz(t, map[string][]byte{
		"dockerize": []byte("#!/bin/sh\necho v0.15.2"),
		"README.md": []byte("readme"),
		"LICENSE":   []byte("license"),
	})
	archBytes, err := os.ReadFile(arch)
	assert.NilError(t, err)
	s := sha256.Sum256(archBytes)
	hash := hex.EncodeToString(s[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/jwilder/dockerize/releases/download/" + version + "/"
		switch r.URL.Path {
		case base + "checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", hash, asset)
		case base + asset:
			http.ServeFile(w, r, arch)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ins := dockerizeInstaller(t, srv)
	_, err = ins.Dockerize(context.Background(), p, "")
	assert.NilError(t, err)

	store := filepath.Join(ins.CacheDir, "dockerize", version)
	entries, err := os.ReadDir(store)
	assert.NilError(t, err)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Check(t, cmp.DeepEqual(names, []string{"dockerize"}),
		"store should contain only the dockerize binary")
}

func TestDockerizeRerunSkipsDownload(t *testing.T) {
	skipWindows(t)
	p := Current()
	version := "v0.15.2"

	srv := buildDockerizeServer(t, version, p, "")
	defer srv.Close()

	var downloads atomic.Int32
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asset, _ := p.dockerizeAsset(version, "/no/such/path")
		if r.URL.Path == "/jwilder/dockerize/releases/download/"+version+"/"+asset {
			downloads.Add(1)
		}
		srv.Config.Handler.ServeHTTP(w, r)
	}))
	defer counting.Close()

	ins := dockerizeInstaller(t, counting)
	_, err := ins.Dockerize(context.Background(), p, "")
	assert.NilError(t, err)

	_, err = ins.Dockerize(context.Background(), p, "")
	assert.NilError(t, err)

	assert.Check(t, cmp.Equal(downloads.Load(), int32(1)), "second run should skip download")
}
