package docker

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestVersionNumber(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"with v", "v27.3.1", "27.3.1"},
		{"without v", "27.3.1", "27.3.1"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, versionNumber(tc.in), tc.want)
		})
	}
}

func TestDockerDownloadURL(t *testing.T) {
	url := dockerDownloadURL("27.3.1")
	platform := "linux"
	if isDarwin() {
		platform = "mac"
	}
	want := "https://download.docker.com/" + platform + "/static/stable/" + unameArch() + "/docker-27.3.1.tgz"
	assert.Equal(t, url, want)
}

func TestDockerFirst(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"docker last", []string{"dockerd", "runc", "docker"}, "docker"},
		{"docker first already", []string{"docker", "dockerd"}, "docker"},
		{"no docker", []string{"dockerd", "runc"}, "dockerd"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dockerFirst(tc.in)
			assert.Equal(t, got[0], tc.want)
		})
	}
}
