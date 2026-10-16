package install

import (
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestDockerSHA256HasDefaultVersion(t *testing.T) {
	sums, ok := dockerSHA256[DefaultDockerVersion]
	assert.Check(t, cmp.Equal(ok, true), "dockerSHA256 missing entry for DefaultDockerVersion %q", DefaultDockerVersion)

	required := []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64"}
	for _, key := range required {
		_, present := sums[key]
		assert.Check(t, cmp.Equal(present, true), "dockerSHA256[%q][%q] missing", DefaultDockerVersion, key)
	}
}
