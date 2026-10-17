package imagebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/install"
)

// Result holds the outcome of a successful build.
type Result struct {
	Refs   []string
	Digest string
}

// Build builds a container image according to f and returns the result.
func Build(ctx context.Context, client *sdk.Client, f Flags) (Result, error) {
	if err := validate(f); err != nil {
		return Result{}, err
	}

	p, err := prepareBuild(ctx, client, f)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(p.metadataFile)

	args := buildArgs(f, p)

	if f.Check {
		if err := runCheck(ctx, f, args); err != nil {
			return Result{}, err
		}
	}

	if err := runBuild(ctx, f, args); err != nil {
		return Result{}, err
	}

	return finaliseBuild(ctx, client, f, p)
}

// prepareBuild checks the engine, installs buildx, sets up the builder and
// cache, returning a ready plan.
func prepareBuild(ctx context.Context, client *sdk.Client, f Flags) (plan, error) {
	osType, err := engineOS(ctx)
	if err != nil {
		return plan{}, err
	}
	if osType == "windows" {
		return plan{}, fmt.Errorf("build needs a Linux Docker engine; this one runs windows containers")
	}

	if _, err := install.New("").Buildx(ctx, install.Current(), ""); err != nil {
		return plan{}, fmt.Errorf("ensure buildx: %w", err)
	}

	builder, err := ensureBuilder(ctx, f)
	if err != nil {
		return plan{}, err
	}

	platforms := parsePlatforms(f.Platforms)
	if len(platforms) > 0 {
		if err := ensureQEMU(ctx, platforms); err != nil {
			return plan{}, fmt.Errorf("ensure QEMU: %w", err)
		}
	}

	cacheDirectory := restoreCache(ctx, client, f)

	imgRefs, err := refs(f, os.Getenv("CIRCLE_SHA1"))
	if err != nil {
		return plan{}, err
	}

	metaFile, err := os.CreateTemp("", "setup-docker-meta-*.json")
	if err != nil {
		return plan{}, fmt.Errorf("create metadata file: %w", err)
	}
	_ = metaFile.Close()

	return plan{
		builder:      builder,
		refs:         imgRefs,
		load:         loadFor(f),
		cacheDir:     cacheDirectory,
		metadataFile: metaFile.Name(),
		sbomGen:      sbomScannerImage,
	}, nil
}

// finaliseBuild reads the digest, writes output files, registers the cache
// save, and prints the summary line.
func finaliseBuild(ctx context.Context, client *sdk.Client, f Flags, p plan) (Result, error) {
	digest, err := readDigest(p.metadataFile)
	if err != nil {
		return Result{}, err
	}

	if f.DigestFile != "" {
		if err := writeFile(f.DigestFile, digest); err != nil {
			return Result{}, fmt.Errorf("write digest file: %w", err)
		}
	}

	if f.SBOMFile != "" {
		if err := writeSBOM(ctx, f, p); err != nil {
			return Result{}, err
		}
	}

	registerCacheSave(ctx, client, f, p.cacheDir)

	suffix := ""
	if f.Push || f.PushByDigest {
		suffix = " and pushed"
	}
	fmt.Fprintf(os.Stderr, "setup-docker: built %s%s\n", strings.Join(p.refs, ", "), suffix)
	fmt.Fprintf(os.Stderr, "setup-docker: digest %s\n", digest)

	return Result{Refs: p.refs, Digest: digest}, nil
}

// runBuild executes docker buildx build, retrying on push failures.
func runBuild(ctx context.Context, f Flags, args []string) error {
	waits := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	var lastErr error
	var lastOutput []byte
	for attempt := 0; attempt <= f.Retries; attempt++ {
		if attempt > 0 {
			if !isPushFailure(lastOutput) {
				return lastErr
			}
			idx := attempt - 1
			if idx >= len(waits) {
				idx = len(waits) - 1
			}
			fmt.Fprintf(os.Stderr, "setup-docker: build attempt %d failed while pushing, retrying in %s\n",
				attempt, waits[idx])
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waits[idx]):
			}
		}

		var buf bytes.Buffer
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Stdout = os.Stderr
		cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
		lastErr = cmd.Run()
		lastOutput = buf.Bytes()
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

func isPushFailure(output []byte) bool {
	return bytes.Contains(bytes.ToLower(output), []byte("failed to push"))
}

// readDigest extracts containerimage.digest from the metadata JSON file.
func readDigest(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("read metadata file: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return "", nil
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("parse metadata: %w", err)
	}
	raw, ok := meta["containerimage.digest"]
	if !ok {
		return "", nil
	}
	var digest string
	if err := json.Unmarshal(raw, &digest); err != nil {
		return "", fmt.Errorf("parse digest: %w", err)
	}
	return digest, nil
}

// writeFile writes content to path, creating parent directories as needed.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0600) //nolint:gosec
}

// writeSBOM runs a second build to produce the SBOM file.
func writeSBOM(ctx context.Context, f Flags, p plan) error {
	tmp, err := os.MkdirTemp("", "setup-docker-sbom-*")
	if err != nil {
		return fmt.Errorf("create sbom temp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	sbomArgs := sbomBuildArgs(f, p, tmp)
	cmd := exec.CommandContext(ctx, "docker", sbomArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sbom build: %w", err)
	}

	src := filepath.Join(tmp, "sbom.spdx.json")
	data, err := os.ReadFile(src) //nolint:gosec
	if err != nil {
		return fmt.Errorf("read sbom: %w", err)
	}
	return writeFile(f.SBOMFile, string(data))
}

// sbomBuildArgs returns the argv for the SBOM-extraction build.
func sbomBuildArgs(f Flags, p plan, destDir string) []string {
	return []string{
		"buildx", "build",
		"--builder", p.builder,
		"--progress=plain",
		"-f", f.Dockerfile,
		"--sbom=generator=" + p.sbomGen,
		"--output", "type=local,dest=" + destDir,
		f.Context,
	}
}

func parsePlatforms(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
