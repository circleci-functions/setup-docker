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

func TestGCRHelperAsset(t *testing.T) {
	cases := []struct {
		p    Platform
		want string
	}{
		{Platform{"linux", "amd64"}, "docker-credential-gcr_linux_amd64-" + DefaultGCRHelperVersion + ".tar.gz"},
		{Platform{"linux", "arm64"}, "docker-credential-gcr_linux_arm64-" + DefaultGCRHelperVersion + ".tar.gz"},
		{Platform{"darwin", "arm64"}, "docker-credential-gcr_darwin_arm64-" + DefaultGCRHelperVersion + ".tar.gz"},
		{Platform{"windows", "amd64"}, "docker-credential-gcr_windows_amd64-" + DefaultGCRHelperVersion + ".tar.gz"},
	}
	for _, tc := range cases {
		got, err := tc.p.gcrHelperAsset()
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(got, tc.want))
	}
	_, err := Platform{"linux", "s390x"}.gcrHelperAsset()
	assert.Check(t, cmp.ErrorContains(err, "unsupported platform"))
}

func TestGCRHelperInstall(t *testing.T) {
	skipWindows(t)
	p := Current()
	asset, err := p.gcrHelperAsset()
	assert.NilError(t, err)
	archive := makeTarGz(t, map[string][]byte{
		"docker-credential-gcr": []byte("#!/bin/sh\necho gcr\n"),
		"LICENSE":               []byte("license"),
	})
	data, err := os.ReadFile(archive) //nolint:gosec
	assert.NilError(t, err)
	sum := sha256.Sum256(data)

	for _, tc := range []struct {
		name    string
		hash    string
		wantErr string
	}{
		{name: "verified", hash: hex.EncodeToString(sum[:])},
		{name: "checksum mismatch", hash: hex.EncodeToString(make([]byte, 32)), wantErr: "SHA-256 mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := "/GoogleCloudPlatform/docker-credential-gcr/releases/download/v" + DefaultGCRHelperVersion + "/"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case base + "checksums.txt":
					fmt.Fprintf(w, "%s  %s\n", tc.hash, asset)
				case base + asset:
					http.ServeFile(w, r, archive)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)
			ins := dockerizeInstaller(t, srv)

			_, err := ins.GCRHelper(context.Background(), p)
			if tc.wantErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.wantErr))
				return
			}
			assert.NilError(t, err)
			_, err = os.Stat(filepath.Join(ins.InstallDir, "docker-credential-gcr"))
			assert.Check(t, err)
		})
	}
}
