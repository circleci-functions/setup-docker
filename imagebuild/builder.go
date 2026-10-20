package imagebuild

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// builderName is fixed so that Docker Layer Caching (DLC) keeps the builder's
// volume between jobs and gives a warm BuildKit cache.
const builderName = "setup-docker"

// archToEmulator maps a Linux architecture name to its QEMU emulator name.
var archToEmulator = map[string]string{
	"amd64":   "qemu-x86_64",
	"arm64":   "qemu-aarch64",
	"arm":     "qemu-arm",
	"386":     "qemu-i386",
	"ppc64le": "qemu-ppc64le",
	"s390x":   "qemu-s390x",
	"riscv64": "qemu-riscv64",
}

// archForPlatform returns the architecture component of a linux/arch[/variant] string.
func archForPlatform(platform string) string {
	arch := strings.TrimPrefix(platform, "linux/")
	if i := strings.Index(arch, "/"); i >= 0 {
		arch = arch[:i]
	}
	return arch
}

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

// binfmtStatus is the JSON output of tonistiigi/binfmt with no arguments.
type binfmtStatus struct {
	Emulators []string `json:"emulators"`
}

// registeredEmulators returns the set of emulator names currently registered.
func registeredEmulators(ctx context.Context) (map[string]bool, error) {
	out, err := exec.CommandContext(ctx, "docker", "run", "--privileged", "--rm", binfmtImage).Output()
	if err != nil {
		return nil, err
	}
	var status binfmtStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(status.Emulators))
	for _, e := range status.Emulators {
		result[e] = true
	}
	return result, nil
}

// ensureQEMU installs QEMU binfmt handlers for foreign platforms that are not
// already registered. It returns the emulator names it installed; the caller
// should uninstall them when the build is done.
func ensureQEMU(ctx context.Context, platforms []string) ([]string, error) {
	if len(platforms) == 0 {
		return nil, nil
	}

	nativeArch, err := engineArch(ctx)
	if err != nil {
		return nil, err
	}

	var foreign []string
	for _, p := range platforms {
		if !strings.HasPrefix(p, "linux/") {
			continue
		}
		arch := archForPlatform(p)
		if arch != nativeArch {
			foreign = append(foreign, arch)
		}
	}
	if len(foreign) == 0 {
		return nil, nil
	}

	registered, err := registeredEmulators(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup-docker: could not read registered emulators: %v\n", err)
	}

	var toInstall []string
	for _, arch := range foreign {
		if registered[archToEmulator[arch]] {
			continue
		}
		toInstall = append(toInstall, arch)
	}
	if len(toInstall) == 0 {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx,
		"docker", "run", "--privileged", "--rm",
		binfmtImage,
		"--install", strings.Join(toInstall, ","),
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var installed []string
	for _, arch := range toInstall {
		if emulator, ok := archToEmulator[arch]; ok {
			installed = append(installed, emulator)
		}
	}
	return installed, nil
}

// uninstallEmulators removes the QEMU binfmt handlers that ensureQEMU installed.
// Errors are logged to stderr; the build is never failed because of them.
func uninstallEmulators(ctx context.Context, emulators []string) {
	if len(emulators) == 0 {
		return
	}
	cmd := exec.CommandContext(ctx,
		"docker", "run", "--privileged", "--rm",
		binfmtImage,
		"--uninstall", strings.Join(emulators, ","),
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "setup-docker: failed to uninstall QEMU emulators %v: %v\n", emulators, err)
	}
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
