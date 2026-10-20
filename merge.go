package main

import (
	"context"

	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/imagebuild"
)

type mergeCmd struct {
	imagebuild.MergeFlags `embed:""`
}

func (c *mergeCmd) Run(ctx context.Context, _ *sdk.Client) error {
	_, err := imagebuild.Merge(ctx, c.MergeFlags)
	return err
}
