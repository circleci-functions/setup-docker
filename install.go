package main

import (
	"context"

	"github.com/circleci-functions/toolkit/sdk"
)

type installCmd struct{}

func (c *installCmd) Run(ctx context.Context, client *sdk.Client) error {
	return nil
}
