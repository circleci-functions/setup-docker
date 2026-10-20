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
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/circleci-functions/toolkit/sdk/sdktest"
	"github.com/circleci-functions/toolkit/testing/runner"
)

// pushRegistryEnv returns the push registry address from the environment,
// skipping t if it is not set.
func pushRegistryEnv(t *testing.T) string {
	t.Helper()
	reg := os.Getenv("SETUP_DOCKER_TEST_PUSH_REGISTRY")
	if reg == "" {
		t.Skip("SETUP_DOCKER_TEST_PUSH_REGISTRY not set")
	}
	return reg
}

// skipUnlessBuildEngine skips t unless we are in a disposable environment with
// a reachable Docker engine.
func skipUnlessBuildEngine(t *testing.T) {
	t.Helper()
	if os.Getenv("SETUP_DOCKER_DISPOSABLE") == "" {
		sdktest.SkipUnlessInJob(t)
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker engine not reachable")
	}
}

// ensureDockerInstalled runs setup-docker (install command) to put docker on PATH.
func ensureDockerInstalled(t *testing.T) {
	t.Helper()
	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run()
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "setup-docker install failed: %s", out)
}

// getManifest fetches a manifest from the registry and returns the response.
func getManifest(t *testing.T, reg, repo, ref string) *http.Response {
	t.Helper()
	url := fmt.Sprintf("http://%s/v2/%s/manifests/%s", reg, repo, ref)
	req, err := http.NewRequest(http.MethodGet, url, nil) //nolint:noctx
	assert.NilError(t, err)
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ", "))
	resp, err := http.DefaultClient.Do(req)
	assert.NilError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// TestAcceptance_BuildAndPush builds and pushes to a plain-HTTP registry.
func TestAcceptance_BuildAndPush(t *testing.T) {
	reg := pushRegistryEnv(t)

	cfgPath := filepath.Join(t.TempDir(), "buildkitd.toml")
	cfgContent := fmt.Sprintf("[registry.%q]\n  http = true\n", reg)
	assert.NilError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0o600))

	dockerfilePath := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfilePath, []byte("FROM alpine:3.22\nRUN echo built > /f\n"), 0o600))

	digestFile := filepath.Join(t.TempDir(), "digest.txt")
	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run("build",
		"--image", reg+"/acc/app",
		"--tags", "one,two",
		"--push",
		"--digest-file", digestFile,
		"--dockerfile", dockerfilePath,
		"--buildkitd-config", cfgPath,
	)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "build failed: %s", out)

	digestBytes, err := os.ReadFile(digestFile) //nolint:gosec
	assert.NilError(t, err, "digest file not written")
	digest := strings.TrimSpace(string(digestBytes))
	assert.Assert(t, strings.HasPrefix(digest, "sha256:"), "digest should start with sha256:: %s", digest)

	// Verify tag "one" has the expected digest.
	resp := getManifest(t, reg, "acc/app", "one")
	assert.Equal(t, resp.StatusCode, 200, "GET manifest/one failed")
	got := resp.Header.Get("Docker-Content-Digest")
	assert.Check(t, cmp.Equal(got, digest), "digest header mismatch")

	// Verify tag "two" exists.
	resp2 := getManifest(t, reg, "acc/app", "two")
	assert.Equal(t, resp2.StatusCode, 200, "GET manifest/two failed")
}

// TestAcceptance_MultiArch builds a multi-arch image and checks the index
// contains exactly linux/amd64 and linux/arm64 (no unknown/unknown, because
// provenance is off by default).
func TestAcceptance_MultiArch(t *testing.T) {
	reg := pushRegistryEnv(t)

	cfgPath := filepath.Join(t.TempDir(), "buildkitd.toml")
	assert.NilError(t, os.WriteFile(cfgPath,
		[]byte(fmt.Sprintf("[registry.%q]\n  http = true\n", reg)), 0o600))

	dockerfilePath := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfilePath, []byte("FROM alpine:3.22\n"), 0o600))

	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run("build",
		"--image", reg+"/acc/multi",
		"--tags", "latest",
		"--platforms", "linux/amd64,linux/arm64",
		"--push",
		"--dockerfile", dockerfilePath,
		"--buildkitd-config", cfgPath,
	)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "multi-arch build failed: %s", out)

	resp := getManifest(t, reg, "acc/multi", "latest")
	assert.Equal(t, resp.StatusCode, 200)
	body, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)

	var index struct {
		Manifests []struct {
			Platform struct {
				OS   string `json:"os"`
				Arch string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	assert.NilError(t, json.Unmarshal(body, &index), "parse index: %s", body)

	var platforms []string
	for _, m := range index.Manifests {
		platforms = append(platforms, m.Platform.OS+"/"+m.Platform.Arch)
	}
	assert.Assert(t, cmp.Contains(platforms, "linux/amd64"), "platforms: %v", platforms)
	assert.Assert(t, cmp.Contains(platforms, "linux/arm64"), "platforms: %v", platforms)
	for _, p := range platforms {
		assert.Assert(t, p != "unknown/unknown",
			"unexpected unknown/unknown platform (provenance should be off): %v", platforms)
	}
}

// TestAcceptance_SBOM pushes with --sbom and verifies the index has an
// attestation manifest and the sbom-file is valid in-toto SPDX JSON.
func TestAcceptance_SBOM(t *testing.T) {
	reg := pushRegistryEnv(t)

	cfgPath := filepath.Join(t.TempDir(), "buildkitd.toml")
	assert.NilError(t, os.WriteFile(cfgPath,
		[]byte(fmt.Sprintf("[registry.%q]\n  http = true\n", reg)), 0o600))

	dockerfilePath := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfilePath, []byte("FROM alpine:3.22\n"), 0o600))

	sbomFile := filepath.Join(t.TempDir(), "sbom.json")

	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run("build",
		"--image", reg+"/acc/sbom",
		"--tags", "latest",
		"--push",
		"--sbom",
		"--sbom-file", sbomFile,
		"--dockerfile", dockerfilePath,
		"--buildkitd-config", cfgPath,
	)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "sbom build failed: %s", out)

	resp := getManifest(t, reg, "acc/sbom", "latest")
	assert.Equal(t, resp.StatusCode, 200)
	body, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)

	var index struct {
		Manifests []struct {
			Platform struct {
				OS   string `json:"os"`
				Arch string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	assert.NilError(t, json.Unmarshal(body, &index), "parse index: %s", body)

	hasAttestation := false
	for _, m := range index.Manifests {
		if m.Platform.OS == "unknown" && m.Platform.Arch == "unknown" {
			hasAttestation = true
		}
	}
	assert.Assert(t, hasAttestation, "expected unknown/unknown attestation manifest in index")

	sbomBytes, err := os.ReadFile(sbomFile) //nolint:gosec
	assert.NilError(t, err, "sbom file not written")
	assert.Assert(t, json.Valid(sbomBytes), "sbom file is not valid JSON")

	// The SBOM is an in-toto statement wrapping SPDX data.
	var stmt struct {
		Predicate struct {
			Packages []json.RawMessage `json:"packages"`
		} `json:"predicate"`
		// Also accept top-level packages for direct SPDX format.
		Packages []json.RawMessage `json:"packages"`
	}
	assert.NilError(t, json.Unmarshal(sbomBytes, &stmt), "parse sbom")
	pkgCount := len(stmt.Predicate.Packages) + len(stmt.Packages)
	assert.Assert(t, pkgCount > 0, "sbom must have at least one package")
}

// TestAcceptance_Check runs the check subcommand and verifies warning output.
func TestAcceptance_Check(t *testing.T) {
	skipUnlessBuildEngine(t)

	dockerfilePath := filepath.Join(t.TempDir(), "Dockerfile")
	// FromAsCasing warning: 'as' should match 'FROM' case.
	assert.NilError(t, os.WriteFile(dockerfilePath,
		[]byte("FROM alpine:3.22 as build\nRUN echo hi\n"), 0o600))

	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run("build",
		"--image", "acc/check",
		"--check",
		"--dockerfile", dockerfilePath,
	)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "check with warnings should succeed: %s", out)
	assert.Assert(t, cmp.Contains(out, "check warning FromAsCasing"),
		"expected check warning FromAsCasing in output: %s", out)

	r2 := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err = r2.Run("build",
		"--image", "acc/check",
		"--check",
		"--check-warnings-as-errors",
		"--dockerfile", dockerfilePath,
	)
	assert.Assert(t, err != nil, "check-warnings-as-errors should fail: %s",
		r2.StdoutString()+r2.StderrString())
}

// TestAcceptance_BuildOnRemoteDocker builds a local image and runs it.
func TestAcceptance_BuildOnRemoteDocker(t *testing.T) {
	skipUnlessRemoteDocker(t)
	ensureDockerInstalled(t)

	dockerfilePath := filepath.Join(t.TempDir(), "Dockerfile")
	assert.NilError(t, os.WriteFile(dockerfilePath,
		[]byte("FROM alpine:3.22\nCMD [\"sh\", \"-c\", \"echo hello from build\"]\n"), 0o600))

	r := runner.New(t, binaryPath, runner.WithUnsafeOSEnvVars())
	_, err := r.Run("build",
		"--image", "acc/remote",
		"--tags", "t",
		"--dockerfile", dockerfilePath,
	)
	out := r.StdoutString() + r.StderrString()
	assert.NilError(t, err, "build failed: %s", out)

	got, runErr := exec.Command("docker", "run", "--rm", "docker.io/acc/remote:t").CombinedOutput() //nolint:gosec
	assert.NilError(t, runErr, "docker run failed: %s", got)
	assert.Assert(t, cmp.Contains(string(got), "hello from build"),
		"expected 'hello from build' in output: %s", got)
}
