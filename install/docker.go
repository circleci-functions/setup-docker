package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

var dockerListRe = regexp.MustCompile(`docker-(\d+\.\d+\.\d+)\.tgz`)

func (i Installer) Docker(ctx context.Context, p Platform, want string) (Result, error) {
	have, _ := installedVersion(ctx, "docker", "--version")
	v, keep, err := chooseVersion(ctx, want, DefaultDockerVersion, have, func(ctx context.Context) (string, error) {
		return i.latestDocker(ctx, p)
	})
	if err != nil {
		return Result{}, err
	}
	if keep {
		return Result{Tool: "docker", Version: v, Kept: true}, nil
	}

	bin := filepath.Join(i.storeDir("docker", v), "docker")
	sum := dockerSHA256[strings.TrimPrefix(v, "v")][p.OS+"/"+p.Arch]
	unverified := sum == ""
	if _, err := os.Stat(bin); err != nil {
		tgzURL, err := p.dockerTarballPath(v)
		if err != nil {
			return Result{}, err
		}
		err = i.fetchVerified(ctx, i.DockerURL+"/"+tgzURL, sum, func(f string) error {
			return extractFile(f, "docker/docker", bin)
		})
		if err != nil {
			return Result{}, err
		}
	}

	dir, err := i.link(ctx, i.storeDir("docker", v), v, "docker")
	if err != nil {
		return Result{}, err
	}
	return Result{Tool: "docker", Version: v, Dir: dir, Unverified: unverified}, nil
}

func (i Installer) latestDocker(ctx context.Context, p Platform) (string, error) {
	idxPath, err := p.dockerIndexPath()
	if err != nil {
		return "", err
	}
	data, err := i.fetchBytes(ctx, i.DockerURL+"/"+idxPath)
	if err != nil {
		return "", err
	}
	matches := dockerListRe.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no docker releases found in listing")
	}
	best := ""
	for _, m := range matches {
		ver := string(m[1])
		if best == "" || semver.Compare("v"+ver, "v"+best) > 0 {
			best = ver
		}
	}
	return best, nil
}
