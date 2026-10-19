package imagebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/circleci-functions/setup-docker/install"
)

// MergeFlags holds parameters for the merge command. It is designed to be
// embedded in a kong CLI struct or called from other packages directly.
type MergeFlags struct {
	Image      string `required:"" help:"Image to publish, e.g. myorg/app or ghcr.io/org/app."`
	Registry   string `default:"docker.io" help:"Registry prefixed to image when image names no registry."`
	Tags       string `help:"Tags, comma or newline separated. Empty uses $CIRCLE_SHA1."`
	DigestsDir string `default:"digests" help:"Directory of digest files written by build with push-by-digest, one per platform."`
	DigestFile string `help:"Write the digest of the merged image to this path."`
	Retries    int    `default:"3" help:"Times to retry a failed merge."`
}

var digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// readMergeDigests reads all non-hidden regular files from dir and returns
// sorted digests. Returns an error if any file's content is not a valid digest,
// or if no files are found.
func readMergeDigests(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read digests dir %s: %w", dir, err)
	}

	var digests []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path) //nolint:gosec
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		d := strings.TrimSpace(string(data))
		if !digestRE.MatchString(d) {
			return nil, fmt.Errorf("%s: invalid digest %q (expected sha256:<64 hex>)", path, d)
		}
		digests = append(digests, d)
	}

	if len(digests) == 0 {
		return nil, fmt.Errorf("no digests in %s; write them with build's push-by-digest and digest-file", dir)
	}

	sort.Strings(digests)
	return digests, nil
}

// Merge combines per-platform images pushed by digest into a single multi-arch
// manifest index and returns the result.
func Merge(ctx context.Context, f MergeFlags) (Result, error) {
	digests, err := readMergeDigests(f.DigestsDir)
	if err != nil {
		return Result{}, err
	}

	imgRefs, err := refs(Flags{Image: f.Image, Registry: f.Registry, Tags: f.Tags}, os.Getenv("CIRCLE_SHA1"))
	if err != nil {
		return Result{}, err
	}

	repo := digestRef(imgRefs)
	sources := make([]string, len(digests))
	for i, d := range digests {
		sources[i] = repo + "@" + d
	}

	if _, err := install.New("").Buildx(ctx, install.Current(), ""); err != nil {
		return Result{}, fmt.Errorf("ensure buildx: %w", err)
	}

	createArgs := []string{"buildx", "imagetools", "create"}
	for _, r := range imgRefs {
		createArgs = append(createArgs, "-t", r)
	}
	createArgs = append(createArgs, sources...)

	if err := runMerge(ctx, f.Retries, createArgs); err != nil {
		return Result{}, fmt.Errorf("imagetools create: %w", err)
	}

	platforms, err := inspectPlatforms(ctx, imgRefs[0])
	if err != nil {
		return Result{}, err
	}

	if len(platforms) != len(digests) {
		return Result{}, fmt.Errorf("merged index has %d platform(s) but expected %d; got: %s",
			len(platforms), len(digests), strings.Join(platforms, ", "))
	}

	digest, err := inspectDigest(ctx, imgRefs[0])
	if err != nil {
		return Result{}, err
	}

	if f.DigestFile != "" {
		if err := writeFile(f.DigestFile, digest); err != nil {
			return Result{}, fmt.Errorf("write digest file: %w", err)
		}
	}

	sort.Strings(platforms)
	fmt.Fprintf(os.Stderr, "setup-docker: merged %d platforms (%s) into %s\n",
		len(platforms), strings.Join(platforms, ", "), strings.Join(imgRefs, ", "))
	fmt.Fprintf(os.Stderr, "setup-docker: digest %s\n", digest)

	return Result{Refs: imgRefs, Digest: digest}, nil
}

// runMerge runs docker with args, retrying on failure up to retries times.
func runMerge(ctx context.Context, retries int, args []string) error {
	waits := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			idx := attempt - 1
			if idx >= len(waits) {
				idx = len(waits) - 1
			}
			fmt.Fprintf(os.Stderr, "setup-docker: merge attempt %d failed, retrying in %s\n", attempt, waits[idx])
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waits[idx]):
			}
		}
		cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if lastErr = cmd.Run(); lastErr == nil {
			return nil
		}
	}
	return lastErr
}

type indexJSON struct {
	Manifests []struct {
		Platform struct {
			OS      string `json:"os"`
			Arch    string `json:"architecture"`
			Variant string `json:"variant,omitempty"`
		} `json:"platform"`
	} `json:"manifests"`
}

// inspectPlatforms runs imagetools inspect --raw and returns os/arch[/variant]
// strings, excluding unknown/unknown attestation entries.
func inspectPlatforms(ctx context.Context, ref string) ([]string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "buildx", "imagetools", "inspect", ref, "--raw") //nolint:gosec
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("imagetools inspect --raw: %w", err)
	}
	var idx indexJSON
	if err := json.Unmarshal(buf.Bytes(), &idx); err != nil {
		return nil, fmt.Errorf("parse index JSON: %w", err)
	}
	var platforms []string
	for _, m := range idx.Manifests {
		if m.Platform.OS == "unknown" && m.Platform.Arch == "unknown" {
			continue
		}
		p := m.Platform.OS + "/" + m.Platform.Arch
		if m.Platform.Variant != "" {
			p += "/" + m.Platform.Variant
		}
		platforms = append(platforms, p)
	}
	return platforms, nil
}

// inspectDigest runs imagetools inspect --format and returns the manifest digest.
func inspectDigest(ctx context.Context, ref string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "buildx", "imagetools", "inspect", ref, //nolint:gosec
		"--format", "{{json .Manifest}}")
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("imagetools inspect --format: %w", err)
	}
	var m struct {
		Digest string `json:"digest"`
	}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		return "", fmt.Errorf("parse manifest JSON: %w", err)
	}
	return m.Digest, nil
}
