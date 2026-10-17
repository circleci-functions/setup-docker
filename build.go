package main

import (
	"context"

	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/imagebuild"
)

type buildCmd struct {
	imagebuild.Flags `embed:""`
}

func (c *buildCmd) Run(ctx context.Context, client *sdk.Client) error {
	_, err := imagebuild.Build(ctx, client, c.Flags)
	return err
}
