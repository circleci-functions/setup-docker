package main

import (
	"context"
	"time"

	"github.com/circleci-functions/setup-docker/health"
)

type healthCmd struct {
	Image      string `required:"" help:"Image to run, e.g. myorg/app:test. Pulled if it isn't in the Docker engine."`
	Port       int    `default:"8080" help:"Container port the health endpoint listens on."`
	Path       string `help:"HTTP path to check, e.g. /healthz. Passes on a 2xx. Empty waits for the image's HEALTHCHECK instead."`
	Env        string `help:"Environment variables for the container, one KEY=VALUE per line."`
	PassEnv    string `help:"Names of job environment variables to pass to the container, comma or newline separated. Their values never appear in the command line."`
	Timeout    int    `default:"60" help:"Seconds to wait for the container to be healthy."`
	Command    string `help:"Command to run instead of the image's CMD, split like a shell would, never evaluated."`
	RunArgs    string `help:"Extra docker run arguments, split like a shell would, never evaluated."`
	ProbeImage string `default:"${probe_image}" help:"Image that checks the endpoint from inside the container's network, so it works with setup_remote_docker too. Must have wget."`
}

func (c *healthCmd) Run(ctx context.Context) error {
	return health.Check(ctx, health.Config{
		Image:      c.Image,
		Port:       c.Port,
		Path:       c.Path,
		Env:        c.Env,
		PassEnv:    c.PassEnv,
		Timeout:    time.Duration(c.Timeout) * time.Second,
		Command:    c.Command,
		RunArgs:    c.RunArgs,
		ProbeImage: c.ProbeImage,
	})
}
