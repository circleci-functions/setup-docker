// Command setup-docker installs the Docker CLI and its plugins in a CircleCI job.
package main

import (
	"context"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/health"
	"github.com/circleci-functions/setup-docker/install"
)

type cli struct {
	Install installCmd `cmd:"" default:"withargs" help:"Install the Docker CLI and its plugins. Runs when no command is given."`
	Login   loginCmd   `cmd:"" help:"Log in to a container registry."`
	Build   buildCmd   `cmd:"" help:"Build an image with buildx and optionally push it."`
	Merge   mergeCmd   `cmd:"" help:"Combine per-platform images pushed by digest into one multi-platform tag."`
	Lint    lintCmd    `cmd:"" help:"Lint Dockerfiles with hadolint."`
	Health  healthCmd  `cmd:"" help:"Start an image and fail unless its health endpoint or HEALTHCHECK reports healthy."`
}

func main() {
	sdk.Run(run)
}

func run(ctx context.Context, client *sdk.Client) error {
	var flags cli
	kCtx := kong.Parse(&flags,
		kong.Name("setup-docker"),
		kong.Description("Set up Docker in a CircleCI job: install the Docker CLI, buildx and compose; log in to Docker Hub, GHCR, Amazon ECR or Google Artifact Registry; build and push images; merge multi-platform builds; lint Dockerfiles; and check that a built image starts healthy. Use this function instead of the circleci/docker orb."),
		kong.UsageOnError(),
		kong.Vars{
			"docker_version":             install.DefaultDockerVersion,
			"buildx_version":             install.DefaultBuildxVersion,
			"compose_version":            install.DefaultComposeVersion,
			"dockerize_version":          install.DefaultDockerizeVersion,
			"goss_version":               install.DefaultGossVersion,
			"credentials_helper_version": install.DefaultCredentialHelpersVersion,
			"hadolint_version":           install.DefaultHadolintVersion,
			"probe_image":                health.DefaultProbeImage,
		},
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	kCtx.Bind(client)
	return kCtx.Run()
}
