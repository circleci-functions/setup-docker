//go:build acceptance

package setupdocker_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/toolkit/testing/runner"
)

// pushRegistryEnv returns the push registry address from the environment,
// skipping t if it is absent.
// TestAcceptance_Merge builds two per-platform images by digest and merges
// them into a single multi-arch tag on a local plain-HTTP registry.
func TestAcceptance_Merge(t *testing.T) {
	reg := pushRegistryEnv(t)

	digestsDir := t.TempDir()

	// Write a buildkitd.toml that allows plain HTTP for the local registry.
	tomlFile := filepath.Join(t.TempDir(), "buildkitd.toml")
	tomlContent := fmt.Sprintf("[registry.%q]\n  http = true\n", reg)
	assert.NilError(t, os.WriteFile(tomlFile, []byte(tomlContent), 0o600))

	image := reg + "/acc/merged"
	buildContext := writeArchDockerfile(t)

	// Build linux/amd64.
	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run("build",
		"--image", image,
		"--push-by-digest",
		"--platforms", "linux/amd64",
		"--digest-file", filepath.Join(digestsDir, "amd64"),
		"--buildkitd-config", tomlFile,
		"--context", buildContext,
	)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "build amd64 failed: %s", out)

	// Build linux/arm64 (QEMU registered by the build step itself).
	r2 := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err = r2.Run("build",
		"--image", image,
		"--push-by-digest",
		"--platforms", "linux/arm64",
		"--digest-file", filepath.Join(digestsDir, "arm64"),
		"--buildkitd-config", tomlFile,
		"--context", buildContext,
	)
	out2 := r2.StdoutString() + r2.StderrString()
	assert.NilError(t, err, "build arm64 failed: %s", out2)

	// Merge the two digests into a single tag.
	mergedDigestFile := filepath.Join(t.TempDir(), "merged-digest.txt")

	r3 := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err = r3.Run("merge",
		"--image", image,
		"--tags", "multi",
		"--digests-dir", digestsDir,
		"--digest-file", mergedDigestFile,
	)
	out3 := r3.StdoutString() + r3.StderrString()
	assert.NilError(t, err, "merge failed: %s", out3)

	// Verify the digest file was written.
	digestData, readErr := os.ReadFile(mergedDigestFile)
	assert.NilError(t, readErr, "merged digest file not written")
	mergedDigest := strings.TrimSpace(string(digestData))
	assert.Check(t, strings.HasPrefix(mergedDigest, "sha256:"),
		"expected sha256: digest, got: %s", mergedDigest)

	// Verify the index via the registry API.
	manifestURL := fmt.Sprintf("http://%s/v2/acc/merged/manifests/multi", reg)
	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	assert.NilError(t, err)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json")

	resp, err := http.DefaultClient.Do(req)
	assert.NilError(t, err)
	defer resp.Body.Close()

	assert.Check(t, cmp.Equal(resp.StatusCode, http.StatusOK),
		"registry returned %d for %s", resp.StatusCode, manifestURL)

	// The Content-Digest header should match the digest file.
	contentDigest := resp.Header.Get("Docker-Content-Digest")
	assert.Check(t, cmp.Equal(contentDigest, mergedDigest),
		"Content-Digest %q != digest file %q", contentDigest, mergedDigest)

	body, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)

	var idx struct {
		Manifests []struct {
			Platform struct {
				OS   string `json:"os"`
				Arch string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	assert.NilError(t, json.Unmarshal(body, &idx), "parse index: %s", body)

	var platforms []string
	for _, m := range idx.Manifests {
		if m.Platform.OS == "unknown" && m.Platform.Arch == "unknown" {
			continue
		}
		platforms = append(platforms, m.Platform.OS+"/"+m.Platform.Arch)
	}

	assert.Check(t, len(platforms) == 2, "expected 2 platforms, got %v", platforms)
	platformSet := map[string]bool{}
	for _, p := range platforms {
		platformSet[p] = true
	}
	assert.Check(t, platformSet["linux/amd64"], "linux/amd64 not in index: %v", platforms)
	assert.Check(t, platformSet["linux/arm64"], "linux/arm64 not in index: %v", platforms)
}

// TestAcceptance_FanOut exercises the full fan-out pattern using ttl.sh.
// Phase "build" builds natively and persists a digest file.
// Phase "merge" attaches the digests and creates the multi-arch tag.
func TestAcceptance_FanOut(t *testing.T) {
	img := os.Getenv("SETUP_DOCKER_TEST_FANOUT_IMAGE")
	phase := os.Getenv("SETUP_DOCKER_TEST_FANOUT_PHASE")
	digestsDir := os.Getenv("SETUP_DOCKER_TEST_DIGESTS_DIR")

	if img == "" || phase == "" || digestsDir == "" {
		t.Skip("SETUP_DOCKER_TEST_FANOUT_IMAGE, SETUP_DOCKER_TEST_FANOUT_PHASE and SETUP_DOCKER_TEST_DIGESTS_DIR must all be set")
	}
	// The binary runs in a directory of its own, and the job persists the digests from its working directory.
	digestsDir, err := filepath.Abs(digestsDir)
	assert.NilError(t, err)

	switch phase {
	case "build":
		arch := runtime.GOARCH
		assert.NilError(t, os.MkdirAll(digestsDir, 0o750)) //nolint:gosec

		r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
		_, err := r.Run("build",
			"--image", img,
			"--push-by-digest",
			"--platforms", "linux/"+arch,
			"--digest-file", filepath.Join(digestsDir, arch),
			"--context", writeArchDockerfile(t),
		)
		out := r.StdoutString() + r.StderrString()
		assert.NilError(t, err, "fanout build failed: %s", out)

	case "merge":
		r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
		_, err := r.Run("merge",
			"--image", img,
			"--tags", "1h",
			"--digests-dir", digestsDir,
		)
		out := r.StdoutString() + r.StderrString()
		assert.NilError(t, err, "fanout merge failed: %s", out)

		// Inspect the merged index via docker buildx imagetools.
		inspectBytes, inspectErr := exec.Command("docker", "buildx", "imagetools", "inspect", //nolint:gosec
			img+":1h", "--raw").CombinedOutput()
		assert.NilError(t, inspectErr, "inspect failed: %s", inspectBytes)
		inspectOut := string(inspectBytes)

		var idx struct {
			Manifests []struct {
				Platform struct {
					OS   string `json:"os"`
					Arch string `json:"architecture"`
				} `json:"platform"`
			} `json:"manifests"`
		}
		assert.NilError(t, json.Unmarshal([]byte(inspectOut), &idx), "parse index: %s", inspectOut)

		var platforms []string
		for _, m := range idx.Manifests {
			if m.Platform.OS == "unknown" && m.Platform.Arch == "unknown" {
				continue
			}
			platforms = append(platforms, m.Platform.OS+"/"+m.Platform.Arch)
		}

		assert.Check(t, len(platforms) == 2, "expected 2 platforms, got %v", platforms)
		platformSet := map[string]bool{}
		for _, p := range platforms {
			platformSet[p] = true
		}
		assert.Check(t, platformSet["linux/amd64"], "linux/amd64 not in index: %v", platforms)
		assert.Check(t, platformSet["linux/arm64"], "linux/arm64 not in index: %v", platforms)

	default:
		t.Fatalf("unknown SETUP_DOCKER_TEST_FANOUT_PHASE %q (expected build or merge)", phase)
	}
}

func writeArchDockerfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
		[]byte("FROM alpine:3.22\nRUN uname -m > /arch\nCMD [\"cat\", \"/arch\"]\n"), 0o600))
	return dir
}
