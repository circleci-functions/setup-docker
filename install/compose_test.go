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

func fakeComposeBinary(version string) []byte {
	return []byte(fmt.Sprintf("#!/bin/sh\necho 'Docker Compose version %s'\n", version))
}

func makeComposeServer(t *testing.T, version, asset string, binary []byte) *httptest.Server {
	t.Helper()
	h := sha256.Sum256(binary)
	checksum := hex.EncodeToString(h[:])
	sumsContent := fmt.Sprintf("%s  %s\n", checksum, asset)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker/compose/releases/download/" + version + "/checksums.txt":
			fmt.Fprint(w, sumsContent)
		case "/docker/compose/releases/download/" + version + "/" + asset:
			_, _ = w.Write(binary)
		case "/docker/compose/releases/latest":
			http.Redirect(w, r, "/docker/compose/releases/tag/"+version, http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestComposeInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const version = "v2.27.2"
	p := Platform{"linux", "amd64"}
	asset, err := p.composeAsset()
	assert.NilError(t, err)

	binary := fakeComposeBinary(version)
	srv := makeComposeServer(t, version, asset, binary)
	defer srv.Close()

	pluginDir := t.TempDir()
	installDir := t.TempDir()
	// Use only installDir on PATH — fake scripts use absolute #!/bin/sh shebangs,
	// and we must not let a real "docker" on the executor's PATH cause compose to
	// report an installed version and short-circuit the download.
	t.Setenv("PATH", installDir)

	ins := testInstaller(srv)
	ins.InstallDir = installDir
	ins.PluginDir = pluginDir

	ctx := context.Background()
	res, err := ins.Compose(ctx, p, version)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Tool, "compose"))
	assert.Check(t, cmp.Equal(res.Version, version))

	_, err = os.Stat(filepath.Join(pluginDir, "docker-compose"))
	assert.NilError(t, err)
}

func TestComposeChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const version = "v2.27.2"
	p := Platform{"linux", "amd64"}
	asset, err := p.composeAsset()
	assert.NilError(t, err)

	binary := fakeComposeBinary(version)
	badSums := fmt.Sprintf("0000000000000000000000000000000000000000000000000000000000000000  %s\n", asset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker/compose/releases/download/" + version + "/checksums.txt":
			fmt.Fprint(w, badSums)
		case "/docker/compose/releases/download/" + version + "/" + asset:
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
	// Isolate PATH so no real "docker" on the executor reports an installed
	// compose version and causes a keep-existing short-circuit.
	t.Setenv("PATH", t.TempDir())

	ctx := context.Background()
	_, err = ins.Compose(ctx, p, version)
	assert.Check(t, cmp.ErrorContains(err, "SHA-256 mismatch"))
}

func TestComposeKeepExistingLinksFromSystemDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based test not supported on Windows")
	}

	const version = "v2.27.2"

	// Put a fake docker-compose in a "system" plugin dir
	sysDir := t.TempDir()
	binary := fakeComposeBinary(version)
	assert.NilError(t, os.WriteFile(filepath.Join(sysDir, "docker-compose"), binary, 0o755))

	// Make docker-compose not on PATH initially (so installedVersion falls back to system lookup)
	// Instead, set up docker with "compose version" output
	fakeDockerDir := t.TempDir()
	dockerScript := fmt.Sprintf(`#!/bin/sh
case "$1" in
  compose) case "$2" in version) echo "Docker Compose version %s" ;; esac ;;
  --version) echo "Docker version 28.0.0, build abc" ;;
esac
`, version)
	assert.NilError(t, os.WriteFile(filepath.Join(fakeDockerDir, "docker"), []byte(dockerScript), 0o755))

	installDir := t.TempDir()
	t.Setenv("PATH", installDir+string(os.PathListSeparator)+fakeDockerDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ins := testInstaller(srv)
	ins.InstallDir = installDir
	ins.PluginDir = t.TempDir() // empty plugin dir
	ins.SystemPluginDirs = []string{sysDir}

	ctx := context.Background()
	res, err := ins.Compose(ctx, Platform{"linux", "amd64"}, version)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Kept, true))
}
