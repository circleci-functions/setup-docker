package main

import (
	"context"
	"fmt"
	"os"

	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/install"
)

type installCmd struct {
	InstallDocker    bool   `default:"true" negatable:"" help:"Install the Docker CLI."`
	InstallBuildx    bool   `default:"true" negatable:"" help:"Install the buildx plugin."`
	InstallCompose   bool   `default:"true" negatable:"" help:"Install the compose plugin, also on PATH as docker-compose."`
	InstallDockerize bool   `help:"Install dockerize."`
	InstallGoss      bool   `help:"Install goss and dgoss."`
	DockerVersion    string `help:"Docker CLI version, or latest. Empty keeps the docker on PATH, else installs ${docker_version}."`
	BuildxVersion    string `help:"buildx version, or latest. Empty keeps an installed buildx, else installs ${buildx_version}."`
	ComposeVersion   string `help:"compose version, or latest. Empty keeps an installed compose, else installs ${compose_version}." example:"v2.29.7"`
	DockerizeVersion string `help:"dockerize version, or latest. Empty keeps the dockerize on PATH, else installs ${dockerize_version}."`
	GossVersion      string `help:"goss version, or latest. Empty keeps the goss on PATH, else installs ${goss_version}."`
	InstallDir       string `help:"Link binaries into this directory instead of one already on PATH."`
	DownloadURL      string `hidden:"" help:"Base URL to download from in place of github.com and download.docker.com."`
}

func (c *installCmd) Run(ctx context.Context, _ *sdk.Client) error {
	i := install.New(c.InstallDir)
	if c.DownloadURL != "" {
		i.GitHubURL = c.DownloadURL
		i.DockerURL = c.DownloadURL
	}
	p := install.Current()

	if c.InstallDocker {
		if err := report(i.Docker(ctx, p, c.DockerVersion)); err != nil {
			return fmt.Errorf("docker: %w", err)
		}
	}
	if c.InstallBuildx {
		if err := report(i.Buildx(ctx, p, c.BuildxVersion)); err != nil {
			return fmt.Errorf("buildx: %w", err)
		}
	}
	if c.InstallCompose {
		if err := report(i.Compose(ctx, p, c.ComposeVersion)); err != nil {
			return fmt.Errorf("compose: %w", err)
		}
	}
	if c.InstallDockerize {
		if err := report(i.Dockerize(ctx, p, c.DockerizeVersion)); err != nil {
			return fmt.Errorf("dockerize: %w", err)
		}
	}
	if c.InstallGoss {
		if err := report(i.Goss(ctx, p, c.GossVersion)); err != nil {
			return fmt.Errorf("goss: %w", err)
		}
	}
	return nil
}

func report(res install.Result, err error) error {
	if err != nil {
		return err
	}
	if res.Kept {
		fmt.Fprintf(os.Stderr, "setup-docker: %s %s is already installed\n", res.Tool, res.Version)
	} else {
		msg := fmt.Sprintf("setup-docker: installed %s %s", res.Tool, res.Version)
		if res.Dir != "" {
			msg += ", linked into " + res.Dir
		}
		fmt.Fprintln(os.Stderr, msg)
	}
	if res.Unverified {
		fmt.Fprintf(os.Stderr,
			"setup-docker: download.docker.com publishes no checksum for %s %s, so the download was not verified\n",
			res.Tool, res.Version)
	}
	return nil
}
