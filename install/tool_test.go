package install

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func errLatest(ctx context.Context) (string, error) {
	return "", errors.New("upstream unreachable")
}

func TestChooseVersion(t *testing.T) {
	resolvedLatest := func(ctx context.Context) (string, error) { return "v29.9.1", nil }

	cases := []struct {
		name        string
		want        string
		def         string
		have        string
		latest      func(context.Context) (string, error)
		wantVersion string
		wantKeep    bool
		wantErr     string
	}{
		{
			name:        "empty want with have keeps",
			want:        "",
			def:         "29.0.0",
			have:        "28.0.0",
			latest:      resolvedLatest,
			wantVersion: "28.0.0",
			wantKeep:    true,
		},
		{
			name:        "empty want without have installs default",
			want:        "",
			def:         "29.0.0",
			have:        "",
			latest:      resolvedLatest,
			wantVersion: "29.0.0",
			wantKeep:    false,
		},
		{
			name:        "exact match keeps (no v prefix)",
			want:        "28.0.0",
			def:         "29.0.0",
			have:        "28.0.0",
			latest:      resolvedLatest,
			wantVersion: "28.0.0",
			wantKeep:    true,
		},
		{
			name:        "exact match keeps (with v prefix)",
			want:        "v28.0.0",
			def:         "29.0.0",
			have:        "28.0.0",
			latest:      resolvedLatest,
			wantVersion: "v28.0.0",
			wantKeep:    true,
		},
		{
			name:        "version mismatch installs",
			want:        "29.0.0",
			def:         "29.0.0",
			have:        "28.0.0",
			latest:      resolvedLatest,
			wantVersion: "29.0.0",
			wantKeep:    false,
		},
		{
			name:        "latest resolves upstream",
			want:        "latest",
			def:         "29.0.0",
			have:        "",
			latest:      resolvedLatest,
			wantVersion: "v29.9.1",
			wantKeep:    false,
		},
		{
			name:    "latest error propagates",
			want:    "latest",
			def:     "29.0.0",
			have:    "",
			latest:  errLatest,
			wantErr: "upstream unreachable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, keep, err := chooseVersion(context.Background(), tc.want, tc.def, tc.have, tc.latest)
			if tc.wantErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.wantErr))
				return
			}
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(v, tc.wantVersion), fmt.Sprintf("version: got %q, want %q", v, tc.wantVersion))
			assert.Check(t, cmp.Equal(keep, tc.wantKeep), fmt.Sprintf("keep: got %v, want %v", keep, tc.wantKeep))
		})
	}
}
