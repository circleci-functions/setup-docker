package imagebuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

const (
	testDigestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testDigestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testDigestC = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	testDigestD = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	testDigestM = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff00"
)

func TestReadMergeDigests(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		want    []string
		wantErr string
	}{
		{
			name:  "Valid",
			files: map[string]string{"amd64": testDigestA, "arm64": testDigestB},
			want:  []string{testDigestA, testDigestB},
		},
		{
			name:    "InvalidContent",
			files:   map[string]string{"amd64": "notadigest"},
			wantErr: "invalid digest",
		},
		{
			name:    "EmptyDir",
			files:   map[string]string{},
			wantErr: "no digests in",
		},
		{
			name:  "HiddenFilesIgnored",
			files: map[string]string{"amd64": testDigestA, ".hidden": "garbage"},
			want:  []string{testDigestA},
		},
		{
			name:  "WhitespaceTrimmed",
			files: map[string]string{"amd64": "  " + testDigestA + "\n"},
			want:  []string{testDigestA},
		},
		{
			name:    "EmptyFile",
			files:   map[string]string{"amd64": ""},
			wantErr: "invalid digest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				assert.NilError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
			}

			got, err := readMergeDigests(dir)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.Check(t, cmp.DeepEqual(got, tt.want))
		})
	}
}

func TestMergeRefs(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		registry string
		tags     string
		sha      string
		wantRefs []string
		wantRepo string
	}{
		{
			name:  "TwoTags",
			image: "myorg/app", registry: "docker.io", tags: "v1,latest",
			wantRefs: []string{"docker.io/myorg/app:v1", "docker.io/myorg/app:latest"},
			wantRepo: "docker.io/myorg/app",
		},
		{
			name:  "GHCR",
			image: "ghcr.io/Org/App", registry: "docker.io", tags: "v2",
			wantRefs: []string{"ghcr.io/org/app:v2"},
			wantRepo: "ghcr.io/org/app",
		},
		{
			name:  "SHA1Fallback",
			image: "myorg/app", registry: "docker.io", sha: "deadbeef00",
			wantRefs: []string{"docker.io/myorg/app:deadbeef00"},
			wantRepo: "docker.io/myorg/app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Flags{Image: tt.image, Registry: tt.registry, Tags: tt.tags}
			got, err := refs(f, tt.sha)
			assert.NilError(t, err)
			assert.Check(t, cmp.DeepEqual(got, tt.wantRefs))
			assert.Check(t, cmp.Equal(digestRef(got), tt.wantRepo))
		})
	}
}

func TestInspectPlatforms_UnknownExcluded(t *testing.T) {
	rawJSON := `{"schemaVersion":2,"manifests":[` +
		`{"digest":"` + testDigestA + `","platform":{"architecture":"amd64","os":"linux"}},` +
		`{"digest":"` + testDigestB + `","platform":{"architecture":"arm64","os":"linux"}},` +
		`{"digest":"` + testDigestC + `","platform":{"architecture":"unknown","os":"unknown"}}` +
		`]}`

	writeFakeDocker(t, `
case "$*" in
  *"imagetools inspect"*"--raw"*) printf '%s' '`+rawJSON+`' ;;
esac
exit 0
`)

	platforms, err := inspectPlatforms(context.Background(), "myorg/app:v1")
	assert.NilError(t, err)
	assert.Check(t, cmp.DeepEqual(platforms, []string{"linux/amd64", "linux/arm64"}))
}

func TestMerge_RetryOnFailure(t *testing.T) {
	rawJSON := `{"schemaVersion":2,"manifests":[` +
		`{"digest":"` + testDigestC + `","platform":{"architecture":"amd64","os":"linux"}},` +
		`{"digest":"` + testDigestD + `","platform":{"architecture":"arm64","os":"linux"}}` +
		`]}`
	manifestJSON := `{"digest":"` + testDigestM + `"}`

	logFile := writeFakeDocker(t, `
case "$1 $2" in
  "buildx version") echo "github.com/docker/buildx v0.99.0" ;;
  "buildx imagetools")
    case "$3" in
      create)
        COUNT_FILE="${FAKE_DOCKER_LOG}.count"
        count=$(cat "$COUNT_FILE" 2>/dev/null || echo 0)
        count=$((count + 1))
        echo "$count" > "$COUNT_FILE"
        if [ "$count" -le 1 ]; then
          exit 1
        fi
        exit 0
        ;;
      inspect)
        case "$*" in
          *--raw*) printf '%s' '`+rawJSON+`' ;;
          *--format*) printf '%s' '`+manifestJSON+`' ;;
        esac
        ;;
    esac
    ;;
esac
exit 0
`)

	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "amd64"), []byte(testDigestA), 0o600))
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "arm64"), []byte(testDigestB), 0o600))

	t.Setenv("CIRCLE_SHA1", "abc123")

	f := MergeFlags{
		Image:      "myorg/app",
		Registry:   "docker.io",
		Tags:       "v1",
		DigestsDir: dir,
		Retries:    3,
	}

	result, err := Merge(context.Background(), f)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(result.Digest, testDigestM))

	lines := readLog(t, logFile)
	createCount := 0
	for _, l := range lines {
		if strings.Contains(l, "imagetools create") {
			createCount++
		}
	}
	assert.Check(t, createCount == 2,
		"expected 2 create calls (1 fail + 1 success), got %d; log: %v", createCount, lines)
}
