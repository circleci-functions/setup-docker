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

	// Isolate from any real docker on the executor: if "docker buildx version"
	// reports exactly v0.23.0 the keep-existing path fires and skips the checksum.
	t.Setenv("PATH", t.TempDir())

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

// TestBuildxChecksumSignedFallback covers macOS/Windows releases where the
// asset is only in checksums-signed.txt, not checksums.txt.
func TestBuildxChecksumSignedFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const version = "v0.38.0"
	p := Platform{"darwin", "arm64"}
	asset, err := p.buildxAsset(version)
	assert.NilError(t, err)

	binary := fakeBuildxBinary(version)
	h := sha256.Sum256(binary)
	checksum := hex.EncodeToString(h[:])
	signedSums := fmt.Sprintf("%s *%s\n", checksum, asset)

	// No docker on PATH so verifyBuildxOnPath is skipped.
	t.Setenv("PATH", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/docker/buildx/releases/download/" + version + "/"
		switch r.URL.Path {
		case base + "checksums.txt":
			// Only has linux entries — darwin asset absent.
			fmt.Fprintf(w, "abc123  buildx-%s.linux-amd64\n", version)
		case base + "checksums-signed.txt":
			fmt.Fprint(w, signedSums)
		case base + asset:
			_, _ = w.Write(binary)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.PluginDir = t.TempDir()

	ctx := context.Background()
	res, err := ins.Buildx(ctx, p, version)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Version, version))
}

// TestBuildxChecksumInNeither covers the case where the asset is in neither file.
func TestBuildxChecksumInNeither(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const version = "v0.38.0"
	p := Platform{"darwin", "arm64"}
	asset, err := p.buildxAsset(version)
	assert.NilError(t, err)

	t.Setenv("PATH", t.TempDir())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := "/docker/buildx/releases/download/" + version + "/"
		switch r.URL.Path {
		case base + "checksums.txt", base + "checksums-signed.txt":
			fmt.Fprintf(w, "abc123  buildx-%s.linux-amd64\n", version)
		case base + asset:
			_, _ = w.Write(fakeBuildxBinary(version))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ins := testInstaller(srv)
	ins.InstallDir = t.TempDir()
	ins.PluginDir = t.TempDir()

	ctx := context.Background()
	_, err = ins.Buildx(ctx, p, version)
	assert.Check(t, cmp.ErrorContains(err, asset))
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
