package install

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Installer struct {
	GitHubURL  string
	DockerURL  string
	InstallDir string
	PluginDir  string
	CacheDir   string
	HTTP       *http.Client
}

func New(linkDir string) Installer {
	pluginDir := filepath.Join(pluginDirBase(), "cli-plugins")
	return Installer{
		GitHubURL:  "https://github.com",
		DockerURL:  "https://download.docker.com",
		InstallDir: linkDir,
		PluginDir:  pluginDir,
		CacheDir:   defaultCacheDir(),
		HTTP:       &http.Client{Timeout: 10 * time.Minute},
	}
}

func pluginDirBase() string {
	if dc := os.Getenv("DOCKER_CONFIG"); dc != "" {
		return dc
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".docker"
	}
	return filepath.Join(home, ".docker")
}

func defaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".cache", "setup-docker")
	}
	return filepath.Join(home, ".cache", "setup-docker")
}

func (i Installer) storeDir(tool, version string) string {
	return filepath.Join(i.CacheDir, tool, version)
}
