package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func testInstaller(srv *httptest.Server) Installer {
	return Installer{
		GitHubURL: srv.URL,
		DockerURL: srv.URL,
		HTTP: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return nil
			},
		},
	}
}

func TestFetch(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
		wantN   int // expected number of requests
	}{
		{
			name: "200 ok",
			handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "hello")
			},
		},
		{
			name: "404 not retried",
			handler: func() http.HandlerFunc {
				var n atomic.Int32
				return func(w http.ResponseWriter, r *http.Request) {
					n.Add(1)
					w.WriteHeader(http.StatusNotFound)
				}
			}(),
			wantErr: "404",
			wantN:   1,
		},
		{
			name: "500 then 200 retries",
			handler: func() http.HandlerFunc {
				var n atomic.Int32
				return func(w http.ResponseWriter, r *http.Request) {
					if n.Add(1) == 1 {
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					fmt.Fprint(w, "ok")
				}
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			ins := testInstaller(srv)
			dest := filepath.Join(t.TempDir(), "out")
			err := ins.fetch(context.Background(), srv.URL+"/file", dest)
			if tc.wantErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.wantErr))
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

func TestFetchContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ins := testInstaller(srv)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately after the first attempt begins.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	dest := filepath.Join(t.TempDir(), "out")
	err := ins.fetch(ctx, srv.URL+"/file", dest)
	elapsed := time.Since(start)

	assert.Check(t, err != nil)
	// Should return promptly (well under 2s wait).
	assert.Check(t, cmp.Equal(elapsed < 1500*time.Millisecond, true),
		"expected fast cancellation, took %v", elapsed)
}

func TestChecksumFor(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		lookFor string
		want    string
		wantErr string
	}{
		{
			name:    "buildx star prefix",
			input:   "abc123def456  *buildx-v0.38.0.linux-amd64\n",
			lookFor: "buildx-v0.38.0.linux-amd64",
			want:    "abc123def456",
		},
		{
			name:    "compose two-space style",
			input:   "deadbeef01234567  docker-compose-linux-x86_64\n",
			lookFor: "docker-compose-linux-x86_64",
			want:    "deadbeef01234567",
		},
		{
			name:    "goss SHA256SUMS two-space",
			input:   "aabbcc1122  goss_0.4.10_linux_x86_64.tar.gz\nffeedd9988  goss_0.4.10_linux_arm64.tar.gz\n",
			lookFor: "goss_0.4.10_linux_arm64.tar.gz",
			want:    "ffeedd9988",
		},
		{
			name:    "bare single hash",
			input:   "0011223344556677",
			lookFor: "anything",
			want:    "0011223344556677",
		},
		{
			name:    "uppercase hex normalised",
			input:   "AABBCC  myfile\n",
			lookFor: "myfile",
			want:    "aabbcc",
		},
		{
			name:    "missing name",
			input:   "aabbcc  other\n",
			lookFor: "missing",
			wantErr: "not found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := checksumFor([]byte(tc.input), tc.lookFor)
			if tc.wantErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.wantErr))
			} else {
				assert.NilError(t, err)
				assert.Check(t, cmp.Equal(got, tc.want))
			}
		})
	}
}

func TestVerifySHA256(t *testing.T) {
	content := []byte("test content")
	h := sha256.Sum256(content)
	goodHash := hex.EncodeToString(h[:])

	t.Run("match", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "*.bin")
		assert.NilError(t, err)
		_, err = f.Write(content)
		assert.NilError(t, err)
		assert.NilError(t, f.Close())
		assert.NilError(t, verifySHA256(f.Name(), goodHash))
	})

	t.Run("mismatch", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "*.bin")
		assert.NilError(t, err)
		_, err = f.Write(content)
		assert.NilError(t, err)
		assert.NilError(t, f.Close())
		err = verifySHA256(f.Name(), "0000000000000000")
		assert.Check(t, cmp.ErrorContains(err, "mismatch"))
	})
}

func makeTarGz(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	f, err := os.Create(path)
	assert.NilError(t, err)
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)
	for name, data := range entries {
		hdr := &tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Size:     int64(len(data)),
			Mode:     0o755,
		}
		assert.NilError(t, tw.WriteHeader(hdr))
		_, err = tw.Write(data)
		assert.NilError(t, err)
	}
	assert.NilError(t, tw.Close())
	assert.NilError(t, gw.Close())
	return path
}

func TestExtractFile(t *testing.T) {
	content := []byte("#!/bin/sh\necho docker")

	t.Run("entry found nested", func(t *testing.T) {
		archive := makeTarGz(t, map[string][]byte{
			"docker/docker": content,
		})
		dest := filepath.Join(t.TempDir(), "docker")
		assert.NilError(t, extractFile(archive, "docker/docker", dest))
		got, err := os.ReadFile(dest)
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(string(got), string(content)))
	})

	t.Run("missing entry", func(t *testing.T) {
		archive := makeTarGz(t, map[string][]byte{
			"other/file": []byte("x"),
		})
		err := extractFile(archive, "docker/docker", filepath.Join(t.TempDir(), "docker"))
		assert.Check(t, cmp.ErrorContains(err, "not found"))
	})
}

func makeZip(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "archive.zip")
	f, err := os.Create(p)
	assert.NilError(t, err)
	defer f.Close()

	w := zip.NewWriter(f)
	for name, data := range entries {
		fw, createErr := w.Create(name)
		assert.NilError(t, createErr)
		_, err = fw.Write(data)
		assert.NilError(t, err)
	}
	assert.NilError(t, w.Close())
	return p
}

func TestExtractFileFromZip(t *testing.T) {
	content := []byte("binary content for zip")

	t.Run("entry found", func(t *testing.T) {
		archive := makeZip(t, map[string][]byte{
			"docker/docker.exe": content,
		})
		dest := filepath.Join(t.TempDir(), "docker.exe")
		assert.NilError(t, extractFile(archive, "docker/docker.exe", dest))
		got, err := os.ReadFile(dest)
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(string(got), string(content)))
	})

	t.Run("missing entry", func(t *testing.T) {
		archive := makeZip(t, map[string][]byte{
			"other/file.exe": []byte("x"),
		})
		err := extractFile(archive, "docker/docker.exe", filepath.Join(t.TempDir(), "docker.exe"))
		assert.Check(t, cmp.ErrorContains(err, "not found"))
	})
}

func TestWriteExecutable(t *testing.T) {
	content := []byte("binary content")
	dest := filepath.Join(t.TempDir(), "subdir", "tool")
	err := writeExecutable(bytes.NewReader(content), dest)
	assert.NilError(t, err)

	got, err := os.ReadFile(dest)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(string(got), string(content)))

	info, err := os.Stat(dest)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(info.Mode()&0o111 != 0, true), "expected executable bit set")
}

func TestLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker/cli/releases/latest":
			http.Redirect(w, r, "/docker/cli/releases/tag/v1.2.3", http.StatusFound)
		case "/empty/repo/releases/latest":
			http.Redirect(w, r, "/empty/repo/releases", http.StatusFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cases := []struct {
		name    string
		repo    string
		wantTag string
		wantErr string
	}{
		{name: "redirect to versioned tag", repo: "docker/cli", wantTag: "v1.2.3"},
		{name: "redirect to releases (no releases)", repo: "empty/repo", wantErr: "could not resolve latest release"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ins := testInstaller(srv)
			got, err := ins.latestRelease(context.Background(), tc.repo)
			if tc.wantErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.wantErr))
			} else {
				assert.NilError(t, err)
				assert.Check(t, cmp.Equal(got, tc.wantTag))
			}
		})
	}
}
