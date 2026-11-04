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
	res, err := imagebuild.Build(ctx, c.Flags)
	if err != nil {
		return err
	}
	return setOutputs(ctx, client, res)
}
