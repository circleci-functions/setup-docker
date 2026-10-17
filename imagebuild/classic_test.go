package imagebuild

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestValidateClassic(t *testing.T) {
	tests := []struct {
		name    string
		f       Flags
		wantErr string
	}{
		{
			name:    "Platforms",
			f:       Flags{Platforms: "linux/amd64"},
			wantErr: "--platforms needs BuildKit",
		},
		{
			name:    "CacheFrom",
			f:       Flags{CacheFrom: "type=registry,ref=org/app:cache"},
			wantErr: "--cache-from needs BuildKit",
		},
		{
			name:    "CacheTo",
			f:       Flags{CacheTo: "type=registry,ref=org/app:cache"},
			wantErr: "--cache-to needs BuildKit",
		},
		{
			name:    "SBOM",
			f:       Flags{SBOM: true},
			wantErr: "--sbom needs BuildKit",
		},
		{
			name:    "SBOMFile",
			f:       Flags{SBOMFile: "/tmp/sbom.json"},
			wantErr: "--sbom-file needs BuildKit",
		},
		{
			name:    "Provenance",
			f:       Flags{Provenance: true},
			wantErr: "--provenance needs BuildKit",
		},
		{
			name:    "SecretEnvs",
			f:       Flags{SecretEnvs: "MY_SECRET"},
			wantErr: "--secret-envs needs BuildKit",
		},
		{
			name:    "Check",
			f:       Flags{Check: true},
			wantErr: "--check needs BuildKit",
		},
		{
			name:    "PushByDigest",
			f:       Flags{PushByDigest: true, DigestFile: "/tmp/d.txt"},
			wantErr: "--push-by-digest needs BuildKit",
		},
		{
			name:    "BuildkitdConfig",
			f:       Flags{BuildkitdConfig: "/etc/buildkitd.toml"},
			wantErr: "--buildkitd-config needs BuildKit",
		},
		{
			name:    "LoadFalse",
			f:       Flags{Load: "false"},
			wantErr: "--load=false needs BuildKit",
		},
		{
			name: "ValidSet",
			f: Flags{
				Image:      "myorg/app",
				Registry:   "docker.io",
				Tags:       "v1",
				Dockerfile: "Dockerfile",
				Context:    ".",
				Push:       true,
				Load:       "auto",
				BuildArgs:  "A=1",
				Labels:     "k=v",
				Pull:       true,
				NoCache:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateClassic(tt.f)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				assert.ErrorContains(t, err, "Windows containers")
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

func TestClassicBuildArgs(t *testing.T) {
	tests := []struct {
		name     string
		f        Flags
		refs     []string
		wantArgs []string
		wantLast string
	}{
		{
			name:     "Minimal",
			f:        Flags{Dockerfile: "Dockerfile", Context: "."},
			refs:     []string{"docker.io/myorg/app:v1"},
			wantArgs: []string{"build", "-f", "Dockerfile", "-t", "docker.io/myorg/app:v1"},
			wantLast: ".",
		},
		{
			name:     "MultipleTags",
			f:        Flags{Dockerfile: "Dockerfile", Context: "."},
			refs:     []string{"docker.io/org/app:v1", "docker.io/org/app:latest"},
			wantArgs: []string{"build", "-f", "Dockerfile", "-t", "docker.io/org/app:v1", "-t", "docker.io/org/app:latest"},
			wantLast: ".",
		},
		{
			name: "Target",
			f:    Flags{Dockerfile: "Dockerfile", Context: ".", Target: "release"},
			refs: []string{"docker.io/org/app:v1"},
			wantArgs: []string{"build", "-f", "Dockerfile", "-t", "docker.io/org/app:v1",
				"--target", "release"},
			wantLast: ".",
		},
		{
			name: "BuildArgs",
			f:    Flags{Dockerfile: "Dockerfile", Context: ".", BuildArgs: "A=1\nB=2"},
			refs: []string{"docker.io/org/app:v1"},
			wantArgs: []string{"build", "-f", "Dockerfile", "-t", "docker.io/org/app:v1",
				"--build-arg", "A=1", "--build-arg", "B=2"},
			wantLast: ".",
		},
		{
			name: "Labels",
			f:    Flags{Dockerfile: "Dockerfile", Context: ".", Labels: "org.version=1"},
			refs: []string{"docker.io/org/app:v1"},
			wantArgs: []string{"build", "-f", "Dockerfile", "-t", "docker.io/org/app:v1",
				"--label", "org.version=1"},
			wantLast: ".",
		},
		{
			name:     "Pull",
			f:        Flags{Dockerfile: "Dockerfile", Context: ".", Pull: true},
			refs:     []string{"docker.io/org/app:v1"},
			wantArgs: []string{"build", "-f", "Dockerfile", "-t", "docker.io/org/app:v1", "--pull"},
			wantLast: ".",
		},
		{
			name:     "NoCache",
			f:        Flags{Dockerfile: "Dockerfile", Context: ".", NoCache: true},
			refs:     []string{"docker.io/org/app:v1"},
			wantArgs: []string{"build", "-f", "Dockerfile", "-t", "docker.io/org/app:v1", "--no-cache"},
			wantLast: ".",
		},
		{
			name: "ExtraArgs",
			f:    Flags{Dockerfile: "Dockerfile", Context: ".", ExtraArgs: "--network host"},
			refs: []string{"docker.io/org/app:v1"},
			wantArgs: []string{"build", "-f", "Dockerfile", "-t", "docker.io/org/app:v1",
				"--network", "host"},
			wantLast: ".",
		},
		{
			name:     "CustomContext",
			f:        Flags{Dockerfile: "app/Dockerfile", Context: "app"},
			refs:     []string{"docker.io/org/app:v1"},
			wantArgs: []string{"build", "-f", "app/Dockerfile", "-t", "docker.io/org/app:v1"},
			wantLast: "app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classicBuildArgs(tt.f, tt.refs)
			assert.Assert(t, len(got) > 0)
			assert.Check(t, cmp.Equal(got[len(got)-1], tt.wantLast), "context must be last arg")
			joined := strings.Join(got, " ")
			for _, want := range tt.wantArgs {
				assert.Check(t, cmp.Contains(joined, want), "missing %q in args: %v", want, got)
			}
		})
	}
}

func TestPushDigest(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
		ok     bool
	}{
		{
			name: "TypicalPushOutput",
			output: `The push refers to repository [docker.io/myorg/app]
abc123: Preparing
abc123: Pushed
latest: digest: sha256:deadbeef1234567890abcdef size: 1234`,
			want: "sha256:deadbeef1234567890abcdef",
			ok:   true,
		},
		{
			name: "MultipleTagsUsesFirst",
			output: `v1: digest: sha256:aaa111 size: 100
v2: digest: sha256:bbb222 size: 100`,
			want: "sha256:aaa111",
			ok:   true,
		},
		{
			name:   "MissingDigest",
			output: `The push refers to repository [docker.io/myorg/app]\nabc123: Pushed`,
			want:   "",
			ok:     false,
		},
		{
			name:   "Empty",
			output: "",
			want:   "",
			ok:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := pushDigest(tt.output)
			assert.Check(t, cmp.Equal(ok, tt.ok))
			assert.Check(t, cmp.Equal(got, tt.want))
		})
	}
}
