package docker

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestCredHelperName(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		darwin bool
		want   string
	}{
		{"explicit wins", "pass", true, "pass"},
		{"darwin default", "", true, "osxkeychain"},
		{"linux default", "", false, "pass"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, credHelperName(tc.in, tc.darwin), tc.want)
		})
	}
}

func TestCredHelperPlatformArch(t *testing.T) {
	p, a := credHelperPlatformArch(false, "arm64")
	assert.Equal(t, p, "linux")
	assert.Equal(t, a, "arm64")

	p, a = credHelperPlatformArch(true, "amd64")
	assert.Equal(t, p, "darwin")
	assert.Equal(t, a, goArch())
}

func TestCredHelperDownloadURL(t *testing.T) {
	base := "https://example.com/releases"

	url, tarball := credHelperDownloadURL(base, "docker-credential-pass", "v0.8.0", "linux", "amd64", 8)
	assert.Equal(t, tarball, false)
	assert.Equal(t, url, "https://example.com/releases/download/v0.8.0/docker-credential-pass-v0.8.0.linux-amd64")

	url, tarball = credHelperDownloadURL(base, "docker-credential-pass", "v0.6.3", "linux", "amd64", 6)
	assert.Equal(t, tarball, true)
	assert.Equal(t, url, "https://example.com/releases/download/v0.6.3/docker-credential-pass-v0.6.3-amd64.tar.gz")

	_, tarball = credHelperDownloadURL(base, "docker-credential-pass", "vunknown", "linux", "amd64", -1)
	assert.Equal(t, tarball, false)
}

func TestMinorVersion(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"v0.7.0", 7},
		{"0.6.3", 6},
		{"v1.0.0", 0},
		{"garbage", -1},
		{"v2", -1},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, minorVersion(tc.in), tc.want)
		})
	}
}

func TestParseFingerprint(t *testing.T) {
	colons := `tru::1:1700000000:0:3:1:5
pub:u:2048:1:ABCDEF:...
fpr:::::::::ABCDEF0123456789ABCDEF0123456789ABCDEF01:
uid:u::::::CircleCI setup-docker <setup-docker@circleci.com>::
`
	fp, ok := parseFingerprint(colons)
	assert.Equal(t, ok, true)
	assert.Equal(t, fp, "ABCDEF0123456789ABCDEF0123456789ABCDEF01")

	_, ok = parseFingerprint("no fingerprint here\n")
	assert.Equal(t, ok, false)
}
