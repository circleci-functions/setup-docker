package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/toolkit/functionyaml"
	"github.com/circleci-functions/toolkit/sdk"
	"github.com/circleci-functions/toolkit/sdk/sdktest"

	"github.com/circleci-functions/setup-docker/imagebuild"
)

func TestSetOutputs(t *testing.T) {
	cases := []struct {
		name string
		res  imagebuild.Result
		want map[string]any
	}{
		{
			name: "pushed with tags",
			res:  imagebuild.Result{Digest: "sha256:4f1c", Refs: []string{"reg/api:1.2.3", "reg/api:latest"}},
			want: map[string]any{"digest": "sha256:4f1c", "image-refs": []string{"reg/api:1.2.3", "reg/api:latest"}},
		},
		{
			name: "pushed by digest",
			res:  imagebuild.Result{Digest: "sha256:4f1c"},
			want: map[string]any{"digest": "sha256:4f1c"},
		},
		{
			name: "loaded without a digest",
			res:  imagebuild.Result{Refs: []string{"api:dev"}},
			want: map[string]any{"image-refs": []string{"api:dev"}},
		},
		{
			name: "nothing to set",
			want: map[string]any{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := sdktest.NewServer(t)
			assert.NilError(t, setOutputs(context.Background(), srv.Client(), tc.res))
			assert.Check(t, cmp.DeepEqual(srv.Outputs(), tc.want))
		})
	}
}

func TestSetOutputs_AgentWithoutOutputs(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	err := setOutputs(context.Background(), sdk.NewWithBaseURL(srv.URL), imagebuild.Result{Digest: "sha256:4f1c"})
	assert.NilError(t, err)
}

func TestOutputsAreValid(t *testing.T) {
	err := functionyaml.Validate(functionyaml.Descriptor{Name: "setup-docker", Outputs: outputs})
	assert.NilError(t, err)
}
