package main

import (
	"context"

	"github.com/circleci-functions/setup-docker/imagebuild"
)

type buildCmd struct {
	imagebuild.Flags `embed:""`
}

func (c *buildCmd) Run(ctx context.Context) error {
	_, err := imagebuild.Build(ctx, c.Flags)
	return err
}
