package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

func TestChecksumFor(t *testing.T) {
	tests := []struct {
		name    string
		content string
		release string
		want    string
		found   bool
	}{
		{
			name:    "checksums.txt multi-line",
			content: "aaa  docker-compose-linux-aarch64\nbbb  docker-compose-linux-x86_64\n",
			release: "docker-compose-linux-x86_64",
			want:    "bbb",
			found:   true,
		},
		{
			name:    "per-asset single line",
			content: "CCC  docker-compose-darwin-aarch64\n",
			release: "docker-compose-darwin-aarch64",
			want:    "ccc",
			found:   true,
		},
		{
			name:    "binary-mode star prefix",
			content: "ddd *docker-compose-linux-x86_64\n",
			release: "docker-compose-linux-x86_64",
			want:    "ddd",
			found:   true,
		},
		{
			name:    "not found",
			content: "aaa  docker-compose-linux-aarch64\n",
			release: "docker-compose-linux-x86_64",
			found:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := checksumFor(tc.content, tc.release)
			assert.Equal(t, ok, tc.found)
			if tc.found {
				assert.Equal(t, got, tc.want)
			}
		})
	}
}

func TestComposeReleaseName(t *testing.T) {
	platform := "linux"
	if isDarwin() {
		platform = "darwin"
	}
	assert.Equal(t, composeReleaseName(), "docker-compose-"+platform+"-"+unameArch())
}

func TestVerifyChecksum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob")
	data := []byte("some binary content")
	assert.NilError(t, os.WriteFile(path, data, 0o600))

	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:])

	assert.NilError(t, verifyChecksum(path, good))
	assert.ErrorContains(t, verifyChecksum(path, "deadbeef"), "checksum mismatch")
}
