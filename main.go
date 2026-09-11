// Command setup-docker installs the Docker CLI and, alongside it, the Docker
// tooling the orb bundled — docker-compose, dockerize, and goss/dgoss — each of
// which can be turned off, plus an opt-in Docker credential helper. Every
// installed binary is linked onto PATH so later steps resolve it by name.
//
// Supported platforms: linux and darwin. goss is Linux-only.
package main

import (
	"context"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"
)

type cli struct {
	InstallDocker           bool `help:"Install the Docker CLI." default:"true"`
	InstallDockerCompose    bool `help:"Install docker-compose." default:"true"`
	InstallDockerize        bool `help:"Install dockerize." default:"true"`
	InstallGoss             bool `help:"Install goss and dgoss (Linux only)." default:"true"`
	InstallCredentialHelper bool `help:"Install a Docker credential helper." default:"false"`

	Version    string `help:"Docker CLI version to install, or 'latest'." default:"latest"`
	InstallDir string `help:"Directory to install the Docker CLI binaries into." default:"/usr/local/bin"`

	DockerComposeVersion    string `help:"docker-compose version to install, or 'latest'." default:"latest"`
	DockerComposeInstallDir string `help:"Directory to install docker-compose into." default:"/usr/local/bin"`

	DockerizeVersion    string `help:"dockerize version to install, or 'latest'." default:"latest"`
	DockerizeInstallDir string `help:"Directory to install dockerize into." default:"/usr/local/bin"`

	GossVersion      string `help:"goss/dgoss version to install, or 'latest'." default:"latest"`
	GossInstallDir   string `help:"Directory to install goss and dgoss into." default:"/usr/local/bin"`
	GossArchitecture string `help:"goss architecture to install." enum:"amd64,arm64" default:"amd64"`

	CredentialHelperName       string `help:"Credential helper to install; chosen by platform if empty." enum:",pass,osxkeychain" default:""`
	CredentialHelperArch       string `help:"Credential helper architecture (Linux)." enum:"amd64,arm64" default:"amd64"`
	CredentialHelperReleaseTag string `help:"Credential helper release tag to install, or empty for latest." default:""`
}

func (c *cli) Run(ctx context.Context, client *sdk.Client) error {
	return nil
}

func main() {
	sdk.Run(run)
}

func run(ctx context.Context, client *sdk.Client) error {
	var flags cli
	kCtx := kong.Parse(&flags,
		kong.Name("setup-docker"),
		kong.Description("Install the Docker CLI and related tooling.\n\nExample:\n  circleci run github.com/circleci-functions/setup-docker@<version> -- --install-docker-compose=false"),
		kong.UsageOnError(),
		functionyaml.Embed(),
	)
	kCtx.BindTo(ctx, (*context.Context)(nil))
	kCtx.Bind(client)
	return kCtx.Run(ctx)
}
