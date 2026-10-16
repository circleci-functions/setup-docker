package install

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// makeFakeDocker writes a shell script to dir/docker that prints a version string.
func makeFakeDocker(t *testing.T, dir, version string) {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
  --version) echo "Docker version %s, build abc123" ;;
  buildx)
    case "$2" in
      version) echo "github.com/docker/buildx v0.23.0 abc" ;;
    esac ;;
  compose)
    case "$2" in
      version) echo "Docker Compose version v2.27.2" ;;
    esac ;;
esac
`, version)
	path := filepath.Join(dir, "docker")
	assert.NilError(t, os.WriteFile(path, []byte(script), 0o755))
}

func makeDockerTarGz(t *testing.T, version string) []byte {
	t.Helper()
	content := fmt.Sprintf("#!/bin/sh\necho 'Docker version %s, build test'\n", version)

	var buf []byte
	f, err := os.CreateTemp(t.TempDir(), "docker-*.tgz")
	assert.NilError(t, err)
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{
		Name:     "docker/docker",
		Typeflag: tar.TypeReg,
		Size:     int64(len(content)),
		Mode:     0o755,
	}
	assert.NilError(t, tw.WriteHeader(hdr))
	_, err = tw.Write([]byte(content))
	assert.NilError(t, err)
	assert.NilError(t, tw.Close())
	assert.NilError(t, gw.Close())

	buf, err = os.ReadFile(f.Name())
	assert.NilError(t, err)
	return buf
}

func TestDockerKeepExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fakeDir := t.TempDir()
	makeFakeDocker(t, fakeDir, "28.1.1")
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.CacheDir = t.TempDir()

	res, err := ins.Docker(ctx, Platform{"linux", "amd64"}, "")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, true))
	assert.Check(t, cmp.Equal(res.Version, "28.1.1"))
}

func TestDockerExactMatchKeeps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fakeDir := t.TempDir()
	makeFakeDocker(t, fakeDir, "29.9.1")
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.CacheDir = t.TempDir()

	res, err := ins.Docker(ctx, Platform{"linux", "amd64"}, "29.9.1")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, true))
}

func TestDockerInstallOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const newVer = "27.0.0"
	tgzData := makeDockerTarGz(t, newVer)

	var reqCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		switch r.URL.Path {
		case "/linux/static/stable/x86_64/":
			fmt.Fprintf(w, `<a href="docker-%s.tgz">docker-%s.tgz</a>`, newVer, newVer)
		case "/linux/static/stable/x86_64/docker-" + newVer + ".tgz":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(tgzData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	fakeDir := t.TempDir()
	makeFakeDocker(t, fakeDir, "28.1.1")

	installDir := t.TempDir()
	t.Setenv("PATH", installDir+string(os.PathListSeparator)+fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ins := testInstaller(srv)
	ins.InstallDir = installDir
	ins.CacheDir = t.TempDir()

	ctx := context.Background()
	res, err := ins.Docker(ctx, Platform{"linux", "amd64"}, newVer)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, false))
	assert.Check(t, cmp.Equal(res.Unverified, true)) // newVer not in dockerSHA256

	// Old fake docker binary untouched
	_, err = os.Stat(filepath.Join(fakeDir, "docker"))
	assert.NilError(t, err)

	// Rerun: the linked docker now reports newVer, so chooseVersion keeps it; no new downloads.
	before := reqCount.Load()
	res2, err := ins.Docker(ctx, Platform{"linux", "amd64"}, newVer)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res2.Version, newVer))
	after := reqCount.Load()
	assert.Check(t, cmp.Equal(after, before), "expected no new requests on rerun, got %d", after-before)
}

func TestDockerChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	tgzData := makeDockerTarGz(t, defaultDockerVersion)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/linux/static/stable/x86_64/docker-"+defaultDockerVersion+".tgz" {
			_, _ = w.Write(tgzData)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.CacheDir = t.TempDir()

	ctx := context.Background()
	_, err := ins.Docker(ctx, Platform{"linux", "amd64"}, defaultDockerVersion)
	assert.Check(t, cmp.ErrorContains(err, "SHA-256 mismatch"))
}

func TestDockerLatestPicksHighest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const expectedLatest = "29.10.0"
	listing := `<a href="docker-28.5.2.tgz">docker-28.5.2.tgz</a>
<a href="docker-29.9.1.tgz">docker-29.9.1.tgz</a>
<a href="docker-29.10.0.tgz">docker-29.10.0.tgz</a>`

	tgzData := makeDockerTarGz(t, expectedLatest)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/linux/static/stable/x86_64/":
			fmt.Fprint(w, listing)
		case "/linux/static/stable/x86_64/docker-" + expectedLatest + ".tgz":
			_, _ = w.Write(tgzData)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.CacheDir = t.TempDir()

	ins.HTTP = &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return nil
		},
	}

	ctx := context.Background()
	res, err := ins.Docker(ctx, Platform{"linux", "amd64"}, "latest")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Version, expectedLatest))
}
