package imagebuild

import (
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestRefs(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		reg     string
		tags    string
		sha     string
		want    []string
		wantErr string
	}{
		{
			name:  "DefaultRegistry",
			image: "myorg/app", reg: "docker.io", sha: "abc123",
			want: []string{"docker.io/myorg/app:abc123"},
		},
		{
			name:  "GhcrKeepsHost",
			image: "ghcr.io/Org/App", reg: "docker.io", tags: "latest",
			want: []string{"ghcr.io/org/app:latest"},
		},
		{
			name:  "LocalhostNoPrefix",
			image: "localhost:5000/x", reg: "docker.io", tags: "v1",
			want: []string{"localhost:5000/x:v1"},
		},
		{
			name:  "TagsWithSpacesNewlines",
			image: "myorg/app", reg: "docker.io",
			tags: "v1.0, v1.0.1\nlatest",
			want: []string{
				"docker.io/myorg/app:v1.0",
				"docker.io/myorg/app:v1.0.1",
				"docker.io/myorg/app:latest",
			},
		},
		{
			name:  "UppercaseTagSanitised",
			image: "myorg/app", reg: "docker.io", tags: "MyBranch/feature",
			want: []string{"docker.io/myorg/app:MyBranch-feature"},
		},
		{
			name:  "NoTagsNoSHA",
			image: "myorg/app", reg: "docker.io",
			wantErr: "CIRCLE_SHA1",
		},
		{
			name:  "LongTagTruncated",
			image: "myorg/app", reg: "docker.io",
			tags: strings.Repeat("a", 129),
			want: []string{"docker.io/myorg/app:" + strings.Repeat("a", 128)},
		},
		{
			name:  "LeadingDotBecomesUnderscore",
			image: "myorg/app", reg: "docker.io", tags: ".v1",
			want: []string{"docker.io/myorg/app:_v1"},
		},
		{
			name:  "LeadingDashBecomesUnderscore",
			image: "myorg/app", reg: "docker.io", tags: "-beta",
			want: []string{"docker.io/myorg/app:_beta"},
		},
		{
			name:  "ImageWithNoSlash",
			image: "ubuntu", reg: "docker.io", tags: "22.04",
			want: []string{"docker.io/ubuntu:22.04"},
		},
		{
			name:  "RegistryWithPort",
			image: "myregistry.example.com:5000/myorg/app", reg: "docker.io", tags: "v1",
			want: []string{"myregistry.example.com:5000/myorg/app:v1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Flags{Image: tt.image, Registry: tt.reg, Tags: tt.tags}
			got, err := refs(f, tt.sha)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Check(t, cmp.DeepEqual(got, tt.want))
		})
	}
}

func TestBuildArgs(t *testing.T) {
	baseFlags := func() Flags {
		return Flags{
			Image:      "myorg/app",
			Registry:   "docker.io",
			Tags:       "latest",
			Dockerfile: "Dockerfile",
			Context:    ".",
			Load:       "auto",
			Retries:    3,
		}
	}

	baseRefs := []string{"docker.io/myorg/app:latest"}

	tests := []struct {
		name   string
		flags  func() Flags
		plan   func(dir string) plan
		checks []func(t *testing.T, args []string)
	}{
		{
			name:  "BasicBuild",
			flags: baseFlags,
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: true, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				containsAll("buildx", "build", "--builder", "b", "--progress=plain", "-f", "Dockerfile"),
				containsAll("-t", "docker.io/myorg/app:latest"),
				containsAll("--load"),
				containsAll("--provenance=false"),
				endsWithContext("."),
			},
		},
		{
			name: "NoDockerfileUsesTheContexts",
			flags: func() Flags {
				f := baseFlags()
				f.Dockerfile = ""
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: true, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				func(t *testing.T, args []string) {
					assert.Check(t, !slices.Contains(args, "-f"), "args %v pass -f", args)
				},
				endsWithContext("."),
			},
		},
		{
			name: "Push",
			flags: func() Flags {
				f := baseFlags()
				f.Push = true
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: false, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				contains("--push"),
				notContains("--load"),
			},
		},
		{
			name: "PushByDigest",
			flags: func() Flags {
				f := baseFlags()
				f.PushByDigest = true
				f.DigestFile = "/tmp/digest"
				f.Tags = "" // push-by-digest doesn't use tags
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: []string{"docker.io/myorg/app:ignored"}, load: false, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				notContains("-t"),
				contains("push-by-digest=true"),
				notContains("--push"),
				notContains("--load"),
			},
		},
		{
			name: "BuildArgs",
			flags: func() Flags {
				f := baseFlags()
				f.BuildArgs = "FEATURES=a,b\nVERSION=1.0"
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: true, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				containsAll("--build-arg", "FEATURES=a,b"),
				containsAll("--build-arg", "VERSION=1.0"),
			},
		},
		{
			name: "Labels",
			flags: func() Flags {
				f := baseFlags()
				f.Labels = "org.label=foo\nother=bar"
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: true, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				containsAll("--label", "org.label=foo"),
				containsAll("--label", "other=bar"),
			},
		},
		{
			name: "SecretEnvs",
			flags: func() Flags {
				f := baseFlags()
				f.SecretEnvs = "TOKEN=MY_TOKEN,OTHER_SECRET"
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: true, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				containsAll("--secret", "id=TOKEN,env=MY_TOKEN"),
				containsAll("--secret", "id=OTHER_SECRET,env=OTHER_SECRET"),
			},
		},
		{
			name: "Platforms",
			flags: func() Flags {
				f := baseFlags()
				f.Platforms = "linux/amd64,linux/arm64"
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: false, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				containsAll("--platform", "linux/amd64,linux/arm64"),
			},
		},
		{
			name: "Target",
			flags: func() Flags {
				f := baseFlags()
				f.Target = "prod"
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: true, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				containsAll("--target", "prod"),
			},
		},
		{
			name: "PullNoCache",
			flags: func() Flags {
				f := baseFlags()
				f.Pull = true
				f.NoCache = true
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: true, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				contains("--pull"),
				contains("--no-cache"),
			},
		},
		{
			name: "SBOMGenerator",
			flags: func() Flags {
				f := baseFlags()
				f.SBOM = true
				f.Push = true
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: false, sbomGen: sbomScannerImage, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				containsPrefix("--sbom=generator="),
			},
		},
		{
			name: "ProvenanceTrue",
			flags: func() Flags {
				f := baseFlags()
				f.Provenance = true
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: true, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				notContains("--provenance=false"),
			},
		},
		{
			name: "ExtraArgsQuoting",
			flags: func() Flags {
				f := baseFlags()
				f.ExtraArgs = `--label "foo=bar baz" --label 'other=x'`
				return f
			},
			plan: func(dir string) plan {
				return plan{builder: "b", refs: baseRefs, load: true, metadataFile: "/tmp/m.json"}
			},
			checks: []func(t *testing.T, args []string){
				containsAll("--label", "foo=bar baz"),
				containsAll("--label", "other=x"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.flags()
			dir := t.TempDir()
			p := tt.plan(dir)
			args := buildArgs(f, p)
			for _, check := range tt.checks {
				check(t, args)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		flags   Flags
		wantErr string
	}{
		{
			name:  "Valid",
			flags: Flags{Image: "myorg/app", Registry: "docker.io", Load: "auto"},
		},
		{
			name:    "SBOMWithoutPushOrFile",
			flags:   Flags{Image: "x", Registry: "r", Load: "auto", SBOM: true},
			wantErr: "--sbom requires",
		},
		{
			name:    "SBOMWithLoad",
			flags:   Flags{Image: "x", Registry: "r", Load: "true", SBOM: true, SBOMFile: "/tmp/s"},
			wantErr: "--sbom cannot be used with --load",
		},
		{
			name:    "PushByDigestWithTags",
			flags:   Flags{Image: "x", Registry: "r", Load: "auto", PushByDigest: true, Tags: "latest", DigestFile: "/d"},
			wantErr: "--push-by-digest cannot be used with --tags",
		},
		{
			name:    "PushByDigestWithoutDigestFile",
			flags:   Flags{Image: "x", Registry: "r", Load: "auto", PushByDigest: true},
			wantErr: "--push-by-digest requires --digest-file",
		},
		{
			name:    "CheckWarningsAsErrorsWithoutCheck",
			flags:   Flags{Image: "x", Registry: "r", Load: "auto", CheckWarningsAsErrors: true},
			wantErr: "--check-warnings-as-errors requires --check",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate(tt.flags)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
		})
	}
}

func TestSplitShellWords(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{name: "Empty", input: "", want: nil},
		{name: "Simple", input: "a b c", want: []string{"a", "b", "c"}},
		{name: "SingleQuotes", input: `'foo bar'`, want: []string{"foo bar"}},
		{name: "DoubleQuotes", input: `"foo bar"`, want: []string{"foo bar"}},
		{name: "BackslashEscape", input: `foo\ bar`, want: []string{"foo bar"}},
		{name: "Mixed", input: `--label "foo=bar baz" --label 'other=x'`, want: []string{"--label", "foo=bar baz", "--label", "other=x"}},
		{name: "UnterminatedSingle", input: `'abc`, wantErr: "unterminated single quote"},
		{name: "UnterminatedDouble", input: `"abc`, wantErr: "unterminated double quote"},
		{name: "TrailingBackslash", input: `abc\`, wantErr: "trailing backslash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitShellWords(tt.input)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Check(t, cmp.DeepEqual(got, tt.want))
		})
	}
}

func TestLoadFor(t *testing.T) {
	tests := []struct {
		name string
		f    Flags
		want bool
	}{
		{name: "AutoSinglePlatform", f: Flags{Load: "auto"}, want: true},
		{name: "AutoMultiPlatform", f: Flags{Load: "auto", Platforms: "linux/amd64,linux/arm64"}, want: false},
		{name: "AutoPush", f: Flags{Load: "auto", Push: true}, want: false},
		{name: "AutoPushByDigest", f: Flags{Load: "auto", PushByDigest: true}, want: false},
		{name: "ExplicitTrue", f: Flags{Load: "true"}, want: true},
		{name: "ExplicitFalse", f: Flags{Load: "false"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, loadFor(tt.f), tt.want)
		})
	}
}

// --- helpers ---

func containsAll(vals ...string) func(*testing.T, []string) {
	return func(t *testing.T, args []string) {
		t.Helper()
		for i := 0; i <= len(args)-len(vals); i++ {
			match := true
			for j, v := range vals {
				if !strings.Contains(args[i+j], v) {
					match = false
					break
				}
			}
			if match {
				return
			}
		}
		t.Errorf("args %v did not contain sequence %v", args, vals)
	}
}

func contains(val string) func(*testing.T, []string) {
	return func(t *testing.T, args []string) {
		t.Helper()
		for _, a := range args {
			if strings.Contains(a, val) {
				return
			}
		}
		t.Errorf("args %v did not contain %q", args, val)
	}
}

func notContains(val string) func(*testing.T, []string) {
	return func(t *testing.T, args []string) {
		t.Helper()
		for _, a := range args {
			if strings.Contains(a, val) {
				t.Errorf("args %v unexpectedly contains %q", args, val)
				return
			}
		}
	}
}

func containsPrefix(prefix string) func(*testing.T, []string) {
	return func(t *testing.T, args []string) {
		t.Helper()
		for _, a := range args {
			if strings.HasPrefix(a, prefix) {
				return
			}
		}
		t.Errorf("args %v did not contain arg with prefix %q", args, prefix)
	}
}

func endsWithContext(ctx string) func(*testing.T, []string) {
	return func(t *testing.T, args []string) {
		t.Helper()
		if len(args) == 0 || args[len(args)-1] != ctx {
			t.Errorf("args %v did not end with context %q", args, ctx)
		}
	}
}
