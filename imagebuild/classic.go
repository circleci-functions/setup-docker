package imagebuild

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// validateClassic returns an error for each argument that requires BuildKit.
// Classic builds are used on Windows executors where BuildKit is unavailable.
func validateClassic(f Flags) error {
	type check struct {
		name string
		bad  bool
	}
	checks := []check{
		{"--platforms", f.Platforms != ""},
		{"--cache-from", f.CacheFrom != ""},
		{"--cache-to", f.CacheTo != ""},
		{"--sbom", f.SBOM},
		{"--sbom-file", f.SBOMFile != ""},
		{"--provenance", f.Provenance},
		{"--secret-envs", f.SecretEnvs != ""},
		{"--check", f.Check},
		{"--push-by-digest", f.PushByDigest},
		{"--buildkitd-config", f.BuildkitdConfig != ""},
		{"--load=false", f.Load == "false"},
	}
	for _, c := range checks {
		if c.bad {
			return fmt.Errorf("%s needs BuildKit, which Docker doesn't support for Windows containers", c.name)
		}
	}
	return nil
}

// classicBuildArgs returns the argv for docker build (classic, no BuildKit).
// Context is always the last argument.
func classicBuildArgs(f Flags, imgRefs []string) []string {
	args := appendDockerfile(f, []string{"build"})
	for _, r := range imgRefs {
		args = append(args, "-t", r)
	}
	if f.Target != "" {
		args = append(args, "--target", f.Target)
	}
	for _, line := range splitLines(f.BuildArgs) {
		args = append(args, "--build-arg", line)
	}
	for _, line := range splitLines(f.Labels) {
		args = append(args, "--label", line)
	}
	if f.Pull {
		args = append(args, "--pull")
	}
	if f.NoCache {
		args = append(args, "--no-cache")
	}
	extra, _ := splitShellWords(f.ExtraArgs)
	args = append(args, extra...)
	args = append(args, f.Context)
	return args
}

// pushDigest parses the image digest from docker push output.
// It expects a line like: <tag>: digest: sha256:<hex> size: <n>
func pushDigest(output string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		if i := strings.Index(line, " digest: "); i >= 0 {
			rest := line[i+len(" digest: "):]
			fields := strings.Fields(rest)
			if len(fields) > 0 && strings.HasPrefix(fields[0], "sha256:") {
				return fields[0], true
			}
		}
	}
	return "", false
}

// buildClassic builds with the classic docker builder (DOCKER_BUILDKIT=0).
// It is used when the Docker engine runs Windows containers.
func buildClassic(ctx context.Context, f Flags, imgRefs []string) (Result, error) {
	if f.Cache {
		fmt.Fprintf(os.Stderr, "setup-docker: the layer cache isn't available for Windows containers; building without it\n")
	}

	if err := validateClassic(f); err != nil {
		return Result{}, err
	}

	args := classicBuildArgs(f, imgRefs)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("docker build: %w", err)
	}

	if !f.Push && f.DigestFile != "" {
		return Result{}, fmt.Errorf("--digest-file requires --push with the classic builder")
	}

	var digest string
	if f.Push {
		var err error
		digest, err = pushAllClassic(ctx, f.Retries, imgRefs)
		if err != nil {
			return Result{}, err
		}
		if f.DigestFile != "" && digest != "" {
			if err := writeFile(f.DigestFile, digest); err != nil {
				return Result{}, fmt.Errorf("write digest file: %w", err)
			}
		}
	}

	suffix := ""
	if f.Push {
		suffix = " and pushed"
	}
	fmt.Fprintf(os.Stderr, "setup-docker: built %s with the classic builder%s\n", strings.Join(imgRefs, ", "), suffix)
	if digest != "" {
		fmt.Fprintf(os.Stderr, "setup-docker: digest %s\n", digest)
	}

	return Result{Refs: imgRefs, Digest: digest}, nil
}

func pushAllClassic(ctx context.Context, retries int, imgRefs []string) (string, error) {
	waits := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	var digest string
	for _, ref := range imgRefs {
		d, err := pushRefClassic(ctx, ref, retries, waits)
		if err != nil {
			return "", err
		}
		if d != "" {
			digest = d
		}
	}
	return digest, nil
}

func pushRefClassic(ctx context.Context, ref string, retries int, waits []time.Duration) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			idx := attempt - 1
			if idx >= len(waits) {
				idx = len(waits) - 1
			}
			fmt.Fprintf(os.Stderr, "setup-docker: push attempt %d failed, retrying in %s\n", attempt, waits[idx])
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(waits[idx]):
			}
		}
		var buf bytes.Buffer
		cmd := exec.CommandContext(ctx, "docker", "push", ref) //nolint:gosec
		cmd.Stdout = io.MultiWriter(os.Stderr, &buf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
		lastErr = cmd.Run()
		if lastErr == nil {
			d, _ := pushDigest(buf.String())
			return d, nil
		}
	}
	return "", fmt.Errorf("push %s: %w", ref, lastErr)
}
