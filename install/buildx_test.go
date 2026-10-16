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
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func fakeBuildxBinary(version string) []byte {
	return []byte(fmt.Sprintf("#!/bin/sh\necho 'github.com/docker/buildx %s abc'\n", version))
}

func makeBuildxServer(t *testing.T, version, asset string, binary []byte) *httptest.Server {
	t.Helper()
	h := sha256.Sum256(binary)
	checksum := hex.EncodeToString(h[:])
	sumsContent := fmt.Sprintf("%s  *%s\n", checksum, asset)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker/buildx/releases/download/" + version + "/checksums.txt":
			fmt.Fprint(w, sumsContent)
		case "/docker/buildx/releases/download/" + version + "/" + asset:
			_, _ = w.Write(binary)
		case "/docker/buildx/releases/latest":
			http.Redirect(w, r, "/docker/buildx/releases/tag/"+version, http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestBuildxInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const version = "v0.23.0"
	p := Platform{"linux", "amd64"}
	asset, err := p.buildxAsset(version)
	assert.NilError(t, err)

	binary := fakeBuildxBinary(version)
	srv := makeBuildxServer(t, version, asset, binary)
	defer srv.Close()

	// No docker on PATH: installedVersion returns empty (install from scratch),
	// and verifyBuildxOnPath is skipped.
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)

	pluginDir := t.TempDir()
	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.PluginDir = pluginDir

	ctx := context.Background()
	res, err := ins.Buildx(ctx, p, version)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Tool, "buildx"))
	assert.Check(t, cmp.Equal(res.Version, version))
	assert.Check(t, cmp.Equal(res.Dir, pluginDir))

	_, err = os.Stat(filepath.Join(pluginDir, "docker-buildx"))
	assert.NilError(t, err)
}

func TestBuildxChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const version = "v0.23.0"
	p := Platform{"linux", "amd64"}
	asset, err := p.buildxAsset(version)
	assert.NilError(t, err)

	binary := fakeBuildxBinary(version)
	badSums := fmt.Sprintf("0000000000000000000000000000000000000000000000000000000000000000  *%s\n", asset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker/buildx/releases/download/" + version + "/checksums.txt":
			fmt.Fprint(w, badSums)
		case "/docker/buildx/releases/download/" + version + "/" + asset:
			_, _ = w.Write(binary)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	pluginDir := t.TempDir()
	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.PluginDir = pluginDir

	ctx := context.Background()
	_, err = ins.Buildx(ctx, p, version)
	assert.Check(t, cmp.ErrorContains(err, "SHA-256 mismatch"))

	// Plugin must not have been written
	_, statErr := os.Stat(filepath.Join(pluginDir, "docker-buildx"))
	assert.Check(t, cmp.Equal(os.IsNotExist(statErr), true))
}

func TestBuildxKeepExisting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	fakeDir := t.TempDir()
	script := `#!/bin/sh
case "$1" in
  buildx) case "$2" in version) echo "github.com/docker/buildx v0.23.0 abc" ;; esac ;;
esac
`
	assert.NilError(t, os.WriteFile(filepath.Join(fakeDir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.PluginDir = t.TempDir()

	ctx := context.Background()
	res, err := ins.Buildx(ctx, Platform{"linux", "amd64"}, "v0.23.0")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, true))
}
