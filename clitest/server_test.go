package clitest_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/circleci-functions/setup-docker/install"
)

const (
	// fakeDockerDownloadVersion is intentionally NOT in dockerSHA256 so the
	// installer takes the unverified path and our fake tarball is not rejected.
	fakeDockerDownloadVersion = "99.0.0"
	fakeBuildxVersion         = install.DefaultBuildxVersion
	fakeComposeVersion        = install.DefaultComposeVersion
	fakeDockerizeVersion      = install.DefaultDockerizeVersion
	fakeGossVersion           = install.DefaultGossVersion
)

type fakeServer struct {
	srv      *httptest.Server
	requests map[string]*atomic.Int64
}

func (f *fakeServer) URL() string { return f.srv.URL }

func (f *fakeServer) requestCount(path string) int64 {
	if c, ok := f.requests[path]; ok {
		return c.Load()
	}
	return 0
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func makeTarGzBytes(name string, content []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{
		Name:     name,
		Typeflag: tar.TypeReg,
		Size:     int64(len(content)),
		Mode:     0o755,
	})
	_, _ = tw.Write(content)
	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}

func fakeScript(versionLine string) []byte {
	return []byte(fmt.Sprintf("#!/bin/sh\necho '%s'\n", versionLine))
}

// platformPaths holds URL path segments for the current platform.
type platformPaths struct {
	dockerIndexPath   string
	dockerTarPath     string
	buildxAsset       string
	composeAsset      string
	dockerizeAsset    string
	gossAsset         string
	gossChecksumsFile string
}

func currentPlatformPaths() platformPaths {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	dockerOS := map[string]string{"linux": "linux", "darwin": "mac"}[goos]
	unameArch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[goarch]
	gossArch := map[string]string{"amd64": "x86_64", "arm64": "arm64"}[goarch]
	gossVer := strings.TrimPrefix(fakeGossVersion, "v")
	idx := fmt.Sprintf("%s/static/stable/%s/", dockerOS, unameArch)
	return platformPaths{
		dockerIndexPath:   "/" + idx,
		dockerTarPath:     fmt.Sprintf("/%sdocker-%s.tgz", idx, fakeDockerDownloadVersion),
		buildxAsset:       fmt.Sprintf("buildx-%s.%s-%s", fakeBuildxVersion, goos, goarch),
		composeAsset:      fmt.Sprintf("docker-compose-%s-%s", goos, unameArch),
		dockerizeAsset:    fmt.Sprintf("dockerize-%s-%s-%s.tar.gz", goos, goarch, fakeDockerizeVersion),
		gossAsset:         fmt.Sprintf("goss_%s_%s_%s.tar.gz", gossVer, goos, gossArch),
		gossChecksumsFile: fmt.Sprintf("goss_%s_SHA256SUMS", gossVer),
	}
}

// buildRoutes returns all static URL→body mappings the fake server needs.
func buildRoutes(pp platformPaths) map[string][]byte {
	// docker — dispatches buildx/compose to cli-plugins so installedVersion
	// detects them only after they are installed.
	dockerBin := []byte(fmt.Sprintf(
		"#!/bin/sh\ncase \"$1\" in\n"+
			"  --version) echo 'Docker version %s, build fake' ;;\n"+
			"  buildx) \"$HOME/.docker/cli-plugins/docker-buildx\" \"$2\" ;;\n"+
			"  compose) \"$HOME/.docker/cli-plugins/docker-compose\" \"$2\" ;;\n"+
			"esac\n", fakeDockerDownloadVersion))
	dockerTar := makeTarGzBytes("docker/docker", dockerBin)
	buildxBin := fakeScript(fmt.Sprintf("github.com/docker/buildx %s abc", fakeBuildxVersion))
	composeBin := fakeScript(fmt.Sprintf("Docker Compose version %s", fakeComposeVersion))
	dockerizeBin := fakeScript(fmt.Sprintf("dockerize version %s", fakeDockerizeVersion))
	dockerizeTar := makeTarGzBytes("dockerize", dockerizeBin)
	gossScript := fakeScript(fmt.Sprintf("goss version %s", fakeGossVersion))
	gossTar := makeTarGzBytes("goss", gossScript)
	dgossScript := fakeScript("dgoss")
	bb := "/docker/buildx/releases/download/" + fakeBuildxVersion + "/"
	cb := "/docker/compose/releases/download/" + fakeComposeVersion + "/"
	db := "/jwilder/dockerize/releases/download/" + fakeDockerizeVersion + "/"
	gb := "/goss-org/goss/releases/download/" + fakeGossVersion + "/"
	return map[string][]byte{
		pp.dockerIndexPath: []byte(fmt.Sprintf(
			`<a href="docker-%s.tgz">docker-%s.tgz</a>`, fakeDockerDownloadVersion, fakeDockerDownloadVersion)),
		pp.dockerTarPath:          dockerTar,
		bb + "checksums.txt":      []byte(fmt.Sprintf("%s  %s\n", hashBytes(buildxBin), pp.buildxAsset)),
		bb + pp.buildxAsset:       buildxBin,
		cb + "checksums.txt":      []byte(fmt.Sprintf("%s  %s\n", hashBytes(composeBin), pp.composeAsset)),
		cb + pp.composeAsset:      composeBin,
		db + "checksums.txt":      []byte(fmt.Sprintf("%s  %s\n", hashBytes(dockerizeTar), pp.dockerizeAsset)),
		db + pp.dockerizeAsset:    dockerizeTar,
		gb + pp.gossChecksumsFile: []byte(fmt.Sprintf("%s  %s\n", hashBytes(gossTar), pp.gossAsset)),
		gb + pp.gossAsset:         gossTar,
		gb + "dgoss.sha256":       []byte(fmt.Sprintf("%s  dgoss\n", hashBytes(dgossScript))),
		gb + "dgoss":              dgossScript,
	}
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	routes := buildRoutes(currentPlatformPaths())
	counters := make(map[string]*atomic.Int64, len(routes))
	for path := range routes {
		counters[path] = &atomic.Int64{}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, ok := counters[r.URL.Path]; ok {
			c.Add(1)
		}
		if data, ok := routes[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(data)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return &fakeServer{srv: srv, requests: counters}
}
