package docker

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestDockerizePlatform(t *testing.T) {
	got := dockerizePlatform()
	switch {
	case isDarwin():
		assert.Equal(t, got, "darwin-amd64")
	case isAlpine():
		assert.Equal(t, got, "alpine-linux-amd64")
	default:
		assert.Equal(t, got, "linux-"+goArch())
	}
}

func TestDockerizeDownloadURL(t *testing.T) {
	url := dockerizeDownloadURL("v0.9.3", "linux-amd64")
	assert.Equal(t, url, "https://github.com/jwilder/dockerize/releases/download/v0.9.3/dockerize-linux-amd64-v0.9.3.tar.gz")
}

func TestFindBinary(t *testing.T) {
	paths := []string{"/tmp/x/README", "/tmp/x/dockerize"}
	got, err := findBinary(paths, "dockerize")
	assert.NilError(t, err)
	assert.Equal(t, got, "/tmp/x/dockerize")

	_, err = findBinary(paths, "missing")
	assert.ErrorContains(t, err, "not found")
}
