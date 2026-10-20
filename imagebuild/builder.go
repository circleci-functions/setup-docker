package imagebuild

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const builderName = "setup-docker"

// engineOS returns the OS type reported by the Docker engine.
// It returns a user-friendly error when the engine is unreachable.
func engineOS(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.OSType}}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf(
			"no Docker engine is reachable: use setup_remote_docker, a machine executor, or set DOCKER_HOST (%w)",
			err,
		)
	}
	return strings.TrimSpace(string(out)), nil
}

// ensureBuilder returns the name of the buildx builder to use, creating it
// if it does not exist yet.
func ensureBuilder(ctx context.Context, f Flags) (string, error) {
	if err := exec.CommandContext(ctx, "docker", "buildx", "inspect", builderName).Run(); err == nil {
		return builderName, nil
	}

	createArgs := []string{
		"buildx", "create",
		"--name", builderName,
		"--driver", "docker-container",
		"--driver-opt", "image=" + buildkitImage,
		"--driver-opt", "network=host",
	}
	if f.BuildkitdConfig != "" {
		createArgs = append(createArgs, "--buildkitd-config", f.BuildkitdConfig)
	}
	createArgs = append(createArgs, "--bootstrap")

	waits := []time.Duration{2 * time.Second, 4 * time.Second}
	var lastErr error
	for attempt := 0; attempt <= len(waits); attempt++ {
		if attempt > 0 {
			fmt.Fprintf(os.Stderr, "setup-docker: builder bootstrap attempt %d failed: %v\n", attempt, lastErr)
			idx := attempt - 1
			if idx >= len(waits) {
				idx = len(waits) - 1
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(waits[idx]):
			}
		}
		cmd := exec.CommandContext(ctx, "docker", createArgs...)
		cmd.Stderr = os.Stderr
		if lastErr = cmd.Run(); lastErr == nil {
			return builderName, nil
		}
	}
	return "", fmt.Errorf("create builder: %w", lastErr)
}

// ensureQEMU installs QEMU binfmt handlers for any platform in the list that
// differs from the engine's native architecture. Does nothing when all
// platforms are native.
func ensureQEMU(ctx context.Context, platforms []string) error {
	if len(platforms) == 0 {
		return nil
	}

	nativeArch, err := engineArch(ctx)
	if err != nil {
		return err
	}

	var foreign []string
	for _, p := range platforms {
		// Only care about linux/<arch> platforms.
		if !strings.HasPrefix(p, "linux/") {
			continue
		}
		arch := strings.TrimPrefix(p, "linux/")
		if arch != nativeArch {
			foreign = append(foreign, arch)
		}
	}
	if len(foreign) == 0 {
		return nil
	}

	cmd := exec.CommandContext(ctx,
		"docker", "run", "--privileged", "--rm",
		binfmtImage,
		"--install", strings.Join(foreign, ","),
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// engineArch returns the normalised architecture of the Docker engine
// (amd64 or arm64).
func engineArch(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.Architecture}}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker info: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	switch raw {
	case "x86_64":
		return "amd64", nil
	case "aarch64":
		return "arm64", nil
	default:
		return raw, nil
	}
}
