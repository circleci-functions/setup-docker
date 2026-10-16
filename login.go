package main

import (
	"context"

	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/install"
	sdlogin "github.com/circleci-functions/setup-docker/login"
)

type loginCmd struct {
	Registry                 string `default:"docker.io" help:"Registry to log in to."`
	UsernameEnv              string `default:"DOCKER_LOGIN" help:"Name of the environment variable holding the username."`
	PasswordEnv              string `default:"DOCKER_PASSWORD" help:"Name of the environment variable holding the password or token."`
	Retries                  int    `default:"3" help:"Times to retry a failed login, with increasing waits."`
	CredentialsStore         bool   `help:"Store credentials with a credential helper instead of in the Docker config file."`
	CredentialsHelper        string `enum:",pass,osxkeychain" default:"" help:"Credential helper to use. Empty picks pass on Linux and osxkeychain on macOS."`
	CredentialsHelperVersion string `help:"Credential helper version, or latest. Empty installs ${credentials_helper_version}."`
	DownloadURL              string `hidden:"" help:"Base URL to download from in place of github.com and the ECR helper's download host."`
}

func (c *loginCmd) Run(ctx context.Context, _ *sdk.Client) error {
	ins := install.New("")
	if c.DownloadURL != "" {
		ins.GitHubURL = c.DownloadURL
	}
	return sdlogin.Login(ctx, ins, install.Current(), sdlogin.Config{
		Registry:                 c.Registry,
		UsernameEnv:              c.UsernameEnv,
		PasswordEnv:              c.PasswordEnv,
		Retries:                  c.Retries,
		CredentialsStore:         c.CredentialsStore,
		CredentialsHelper:        c.CredentialsHelper,
		CredentialsHelperVersion: c.CredentialsHelperVersion,
		DownloadURL:              c.DownloadURL,
	})
}
