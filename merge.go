package main

import (
	"context"

	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/imagebuild"
)

type mergeCmd struct {
	imagebuild.MergeFlags `embed:""`
}

func (c *mergeCmd) Run(ctx context.Context, client *sdk.Client) error {
	res, err := imagebuild.Merge(ctx, c.MergeFlags)
	if err != nil {
		return err
	}
	return setOutputs(ctx, client, res)
}
