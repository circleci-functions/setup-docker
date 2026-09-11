// Command setup-docker installs the Docker CLI and, alongside it, the Docker
// tooling the orb bundled — docker-compose, dockerize, and goss/dgoss — each of
// which can be turned off, plus an opt-in Docker credential helper. Every
// installed binary is linked onto PATH so later steps resolve it by name.
//
// Supported platforms: linux and darwin. goss is Linux-only.
package main

import (
	"context"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/docker"
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

	gossExplicit bool
}

func (c *cli) Run(ctx context.Context) error {
	if c.InstallDocker {
		if err := docker.InstallDocker(ctx, docker.DockerOptions{
			Version:    c.Version,
			InstallDir: c.InstallDir,
		}); err != nil {
			return err
		}
	}
	if c.InstallDockerCompose {
		if err := docker.InstallDockerCompose(ctx, docker.ComposeOptions{
			Version:    c.DockerComposeVersion,
			InstallDir: c.DockerComposeInstallDir,
		}); err != nil {
			return err
		}
	}
	if c.InstallDockerize {
		if err := docker.InstallDockerize(ctx, docker.DockerizeOptions{
			Version:    c.DockerizeVersion,
			InstallDir: c.DockerizeInstallDir,
		}); err != nil {
			return err
		}
	}
	if c.InstallGoss {
		if err := docker.InstallGoss(ctx, docker.GossOptions{
			Version:      c.GossVersion,
			InstallDir:   c.GossInstallDir,
			Architecture: c.GossArchitecture,
			Explicit:     c.gossExplicit,
		}); err != nil {
			return err
		}
	}
	if c.InstallCredentialHelper {
		if err := docker.InstallCredentialHelper(ctx, docker.CredentialHelperOptions{
			Name:       c.CredentialHelperName,
			Arch:       c.CredentialHelperArch,
			ReleaseTag: c.CredentialHelperReleaseTag,
		}); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	sdk.Run(run)
}

func run(ctx context.Context, _ *sdk.Client) error {
	var flags cli
	kCtx := kong.Parse(&flags,
		kong.Name("setup-docker"),
		kong.Description("Install the Docker CLI and related tooling.\n\nExample:\n  circleci run github.com/circleci-functions/setup-docker@<version> -- --install-docker-compose=false"),
		kong.UsageOnError(),
		functionyaml.Embed(),
	)
	flags.gossExplicit = gossFlagProvided(os.Args[1:])
	kCtx.BindTo(ctx, (*context.Context)(nil))
	return kCtx.Run(ctx)
}

// gossFlagProvided reports whether the caller passed --install-goss explicitly,
// which decides whether an unsupported platform errors or is quietly skipped.
func gossFlagProvided(args []string) bool {
	for _, a := range args {
		if a == "--install-goss" || strings.HasPrefix(a, "--install-goss=") {
			return true
		}
	}
	return false
}
