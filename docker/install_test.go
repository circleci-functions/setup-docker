package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
)

func TestDownload(t *testing.T) {
	body := []byte("payload")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	t.Run("ok", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		assert.NilError(t, download(context.Background(), srv.URL+"/ok", dest))
		got, err := os.ReadFile(dest) //nolint:gosec // test-controlled temp path
		assert.NilError(t, err)
		assert.DeepEqual(t, got, body)
	})

	t.Run("not found", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		assert.ErrorContains(t, download(context.Background(), srv.URL+"/missing", dest), "404")
	})
}

func TestResolveLatestFromRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			http.Redirect(w, r, "/releases/tag/v2.29.7", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tag, err := resolveLatestFromRedirect(context.Background(), srv.URL+"/releases/latest")
	assert.NilError(t, err)
	assert.Equal(t, tag, "v2.29.7")
}

func TestExtractTarGz(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := map[string]string{
		"docker/docker":  "docker-bin",
		"docker/dockerd": "dockerd-bin",
	}
	for name, content := range files {
		assert.NilError(t, tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}))
		_, err := tw.Write([]byte(content))
		assert.NilError(t, err)
	}
	assert.NilError(t, tw.Close())
	assert.NilError(t, gz.Close())

	dir := t.TempDir()
	archive := filepath.Join(dir, "docker.tgz")
	assert.NilError(t, os.WriteFile(archive, buf.Bytes(), 0o600))

	destDir := t.TempDir()
	written, err := extractTarGz(archive, destDir)
	assert.NilError(t, err)
	assert.Equal(t, len(written), 2)

	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(destDir, filepath.Base(name))) //nolint:gosec // test-controlled temp path
		assert.NilError(t, err)
		assert.Equal(t, string(got), content)
	}
}

func TestPlaceBinary(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "tool")
	assert.NilError(t, os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o600))

	destDir := t.TempDir()
	dest, err := placeBinary(context.Background(), src, destDir, "tool")
	assert.NilError(t, err)
	assert.Equal(t, dest, filepath.Join(destDir, "tool"))

	info, err := os.Stat(dest)
	assert.NilError(t, err)
	assert.Equal(t, info.Mode().Perm()&0o100, os.FileMode(0o100))
}

func TestInstalledVersionContains(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "faketool")
	assert.NilError(t, os.WriteFile(script, []byte("#!/bin/sh\necho 'faketool version 1.2.3'\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx := context.Background()
	assert.Equal(t, installedVersionContains(ctx, "faketool", "1.2.3"), true)
	assert.Equal(t, installedVersionContains(ctx, "faketool", "9.9.9"), false)
	assert.Equal(t, installedVersionContains(ctx, "definitely-missing-tool", "1"), false)
}

func TestLinkOnPath(t *testing.T) {
	sdktest.SkipUnlessInJob(t)

	dir := t.TempDir()
	name := "faketool"
	assert.NilError(t, os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\ntrue\n"), 0o755)) //nolint:gosec // test binary must be executable
	err := linkOnPath(dir, []string{name}, nil, func(bin string) error { return nil })
	assert.NilError(t, err)
}
