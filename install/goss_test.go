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
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func gossInstaller(t *testing.T, srv *httptest.Server) Installer {
	t.Helper()
	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.CacheDir = t.TempDir()
	return ins
}

func hashBytes(data []byte) string {
	s := sha256.Sum256(data)
	return hex.EncodeToString(s[:])
}

type gossRelease struct {
	version    string
	gossBytes  []byte
	dgossBytes []byte
	oldLayout  bool
}

func buildGossServer(t *testing.T, p Platform, rel gossRelease, badDgossHash bool) *httptest.Server {
	t.Helper()
	version := rel.version

	dgossHash := hashBytes(rel.dgossBytes)
	if badDgossHash {
		dgossHash = strings.Repeat("0", 64)
	}

	// pre-compute current-layout data
	tarAsset, sumsFile, _ := p.gossAsset(version)
	arch := makeTarGz(t, map[string][]byte{"goss": rel.gossBytes})
	tarbytes, err := os.ReadFile(arch)
	assert.NilError(t, err)
	sumsContent := fmt.Sprintf("%s  %s\n", hashBytes(tarbytes), tarAsset)

	// pre-compute old-layout asset name (may error on unsupported platforms; tested only on linux/darwin)
	oldAsset, _ := p.gossOldAsset()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/goss-org/goss/releases/download/" + version + "/"
		switch r.URL.Path {
		case base + sumsFile:
			if rel.oldLayout {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			fmt.Fprint(w, sumsContent)
		case base + tarAsset:
			if rel.oldLayout {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Write(tarbytes) //nolint:errcheck
		case base + oldAsset:
			w.Write(rel.gossBytes) //nolint:errcheck
		case base + oldAsset + ".sha256":
			fmt.Fprint(w, hashBytes(rel.gossBytes))
		case base + "dgoss.sha256":
			fmt.Fprintf(w, "%s  dgoss\n", dgossHash)
		case base + "dgoss":
			w.Write(rel.dgossBytes) //nolint:errcheck
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestGossCurrentLayout(t *testing.T) {
	skipWindows(t)
	p := Current()
	if p.OS != "linux" && p.OS != "darwin" {
		t.Skipf("goss not supported on %s", p.OS)
	}

	version := "v0.4.10"
	rel := gossRelease{
		version:    version,
		gossBytes:  []byte("#!/bin/sh\necho 'goss version v0.4.10'\n"),
		dgossBytes: []byte("#!/bin/sh\necho dgoss\n"),
	}
	srv := buildGossServer(t, p, rel, false)
	defer srv.Close()

	ins := gossInstaller(t, srv)
	res, err := ins.Goss(context.Background(), p, "")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Tool, "goss"))
	assert.Check(t, cmp.Equal(res.Version, version))
	assert.Check(t, cmp.Equal(res.Kept, false))

	_, err = os.Stat(filepath.Join(ins.InstallDir, "goss"))
	assert.NilError(t, err)
	_, err = os.Stat(filepath.Join(ins.InstallDir, "dgoss"))
	assert.NilError(t, err)
}

func TestGossOldLayoutFallback(t *testing.T) {
	skipWindows(t)
	p := Current()
	if p.OS != "linux" && p.OS != "darwin" {
		t.Skipf("goss not supported on %s", p.OS)
	}

	version := "v0.4.9"
	rel := gossRelease{
		version:    version,
		gossBytes:  []byte("#!/bin/sh\necho 'goss version v0.4.9'\n"),
		dgossBytes: []byte("#!/bin/sh\necho dgoss\n"),
		oldLayout:  true,
	}
	srv := buildGossServer(t, p, rel, false)
	defer srv.Close()

	ins := gossInstaller(t, srv)
	res, err := ins.Goss(context.Background(), p, version)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Tool, "goss"))
	assert.Check(t, cmp.Equal(res.Version, version))

	_, err = os.Stat(filepath.Join(ins.CacheDir, "goss", version, "goss"))
	assert.NilError(t, err)
	_, err = os.Stat(filepath.Join(ins.CacheDir, "goss", version, "dgoss"))
	assert.NilError(t, err)
}

func TestGossDgossChecksumMismatch(t *testing.T) {
	skipWindows(t)
	p := Current()
	if p.OS != "linux" && p.OS != "darwin" {
		t.Skipf("goss not supported on %s", p.OS)
	}

	version := "v0.4.10"
	rel := gossRelease{
		version:    version,
		gossBytes:  []byte("#!/bin/sh\necho 'goss version v0.4.10'\n"),
		dgossBytes: []byte("#!/bin/sh\necho dgoss\n"),
	}
	srv := buildGossServer(t, p, rel, true)
	defer srv.Close()

	ins := gossInstaller(t, srv)
	_, err := ins.Goss(context.Background(), p, "")
	assert.Check(t, cmp.ErrorContains(err, "mismatch"))
}

func TestGossKeepExisting(t *testing.T) {
	skipWindows(t)
	p := Current()
	if p.OS != "linux" && p.OS != "darwin" {
		t.Skipf("goss not supported on %s", p.OS)
	}

	version := "v0.4.10"
	rel := gossRelease{
		version:    version,
		gossBytes:  []byte("#!/bin/sh\necho 'goss version v0.4.10'\n"),
		dgossBytes: []byte("#!/bin/sh\necho dgoss\n"),
	}

	var requests int
	inner := buildGossServer(t, p, rel, false)
	defer inner.Close()
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		inner.Config.Handler.ServeHTTP(w, r)
	}))
	defer counting.Close()

	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "goss"), rel.gossBytes, 0o755))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "dgoss"), rel.dgossBytes, 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ins := gossInstaller(t, counting)
	res, err := ins.Goss(context.Background(), p, "")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, true))
	assert.Check(t, cmp.Equal(requests, 0), "expected no HTTP requests when already installed")
}

func TestGossReinstallWhenDgossMissing(t *testing.T) {
	skipWindows(t)
	p := Current()
	if p.OS != "linux" && p.OS != "darwin" {
		t.Skipf("goss not supported on %s", p.OS)
	}

	version := "v0.4.10"
	rel := gossRelease{
		version:    version,
		gossBytes:  []byte("#!/bin/sh\necho 'goss version v0.4.10'\n"),
		dgossBytes: []byte("#!/bin/sh\necho dgoss\n"),
	}
	srv := buildGossServer(t, p, rel, false)
	defer srv.Close()

	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "goss"), rel.gossBytes, 0o755))
	// dgoss intentionally absent
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ins := gossInstaller(t, srv)
	res, err := ins.Goss(context.Background(), p, "")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, false), "should reinstall when dgoss is missing")

	_, err = os.Stat(filepath.Join(ins.CacheDir, "goss", version, "dgoss"))
	assert.NilError(t, err)
}
