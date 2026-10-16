// Command setup-docker installs the Docker CLI and its plugins in a CircleCI job.
package main

import (
	"context"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/install"
)

type cli struct {
	Install installCmd `cmd:"" default:"withargs" help:"Install the Docker CLI and its plugins. Runs when no command is given."`
}

func main() {
	sdk.Run(run)
}

func run(ctx context.Context, client *sdk.Client) error {
	var flags cli
	kCtx := kong.Parse(&flags,
		kong.Name("setup-docker"),
		kong.Description("Set up Docker in a CircleCI job: the Docker CLI, buildx and compose, and optionally dockerize and goss."),
		kong.UsageOnError(),
		kong.Vars{
			"docker_version":    install.DefaultDockerVersion,
			"buildx_version":    install.DefaultBuildxVersion,
			"compose_version":   install.DefaultComposeVersion,
			"dockerize_version": install.DefaultDockerizeVersion,
			"goss_version":      install.DefaultGossVersion,
		},
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	kCtx.Bind(client)
	return kCtx.Run()
}
