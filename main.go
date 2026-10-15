// Command setup-docker installs the Docker CLI and its plugins in a CircleCI job.
package main

import (
	"context"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"
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
		kong.Description("Set up Docker in a CircleCI job."),
		kong.UsageOnError(),
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	kCtx.Bind(client)
	return kCtx.Run()
}
