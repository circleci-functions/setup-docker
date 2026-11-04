package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/imagebuild"
)

// outputs are the values build and merge hand to later steps in the job, read
// with ${{ steps.<id>.outputs.<name> }}.
var outputs = []functionyaml.Output{
	{Name: "digest", Type: "string", Description: "Digest of the image that was built or merged, when there is one."},
	{Name: "image-refs", Type: "list", Description: "The image references that were built or merged, such as registry/image:tag."},
}

// setOutputs sets the outputs of a build or merge that has a value. A job
// without outputs, or a run outside a job, is not an error.
func setOutputs(ctx context.Context, client *sdk.Client, res imagebuild.Result) error {
	if res.Digest != "" {
		if err := setOutput(ctx, client, "digest", res.Digest); err != nil {
			return err
		}
	}
	if len(res.Refs) > 0 {
		if err := setOutput(ctx, client, "image-refs", res.Refs); err != nil {
			return err
		}
	}
	return nil
}

func setOutput(ctx context.Context, client *sdk.Client, name string, value any) error {
	err := client.SetOutput(ctx, name, value)
	if err != nil && !errors.Is(err, sdk.ErrNoServer) && !errors.Is(err, sdk.ErrUnsupported) {
		return fmt.Errorf("set output %s: %w", name, err)
	}
	return nil
}
