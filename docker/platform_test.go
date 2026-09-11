package docker

import (
	"runtime"
	"testing"

	"gotest.tools/v3/assert"
)

func TestUnameArch(t *testing.T) {
	got := unameArch()
	want := "x86_64"
	if runtime.GOARCH == "arm64" {
		want = "aarch64"
	}
	assert.Equal(t, got, want)
}

func TestGoArch(t *testing.T) {
	got := goArch()
	want := "amd64"
	if runtime.GOARCH == "arm64" {
		want = "arm64"
	}
	assert.Equal(t, got, want)
}

func TestOSHelpers(t *testing.T) {
	assert.Equal(t, isDarwin(), runtime.GOOS == "darwin")
	assert.Equal(t, isLinux(), runtime.GOOS == "linux")
}

func TestLooksAlpine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"os-release id", "NAME=\"Alpine Linux\"\nID=alpine\n", true},
		{"lowercased", "pretty_name=alpine", true},
		{"debian", "NAME=\"Debian GNU/Linux\"\nID=debian\n", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, looksAlpine(tc.in), tc.want)
		})
	}
}

func TestIsAlpineDoesNotPanic(t *testing.T) {
	_ = isAlpine()
}
