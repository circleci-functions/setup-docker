package docker

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/circleci-functions/toolkit/pathlink"
)

var httpClient = &http.Client{Timeout: 5 * time.Minute}

const downloadRetries = 3

// resolveLatestFromRedirect follows a "…/releases/latest" URL to the versioned
// URL it redirects to and returns the trailing path segment, which is the
// release tag. GitHub serves the tag this way rather than in an API field, so a
// caller with no token can still resolve latest.
func resolveLatestFromRedirect(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	tag := path.Base(resp.Request.URL.Path)
	if tag == "" || tag == "." || tag == "latest" {
		return "", fmt.Errorf("could not resolve latest version from %s", url)
	}
	return tag, nil
}

// download fetches url into dest, retrying transient failures.
func download(ctx context.Context, url, dest string) error {
	var lastErr error
	for attempt := 1; attempt <= downloadRetries; attempt++ {
		if lastErr = downloadOnce(ctx, url, dest); lastErr == nil {
			return nil
		}
		if attempt < downloadRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return fmt.Errorf("download %s: %w", url, lastErr)
}

func downloadOnce(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}
	f, err := os.Create(dest) //nolint:gosec // dest is a path this process built under a temp dir
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body) //nolint:gosec // release artifact from a known host
	return err
}

// extractTarGz unpacks every regular file in archive into destDir, flattened to
// its base name, and returns the paths written. Docker's tarball nests its
// binaries under a docker/ directory, so flattening yields docker, dockerd, …
// directly in destDir.
func extractTarGz(archive, destDir string) ([]string, error) {
	f, err := os.Open(archive) //nolint:gosec // archive is a path this process built under a temp dir
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	var written []string
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		dest := filepath.Join(destDir, filepath.Base(hdr.Name))
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755) //nolint:gosec // dest is under a temp dir built by this process
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(out, tr); err != nil { //nolint:gosec // release artifact from a known host
			_ = out.Close()
			return nil, err
		}
		if err := out.Close(); err != nil {
			return nil, err
		}
		written = append(written, dest)
	}
	return written, nil
}

// placeBinary installs src at destDir/name with mode 0755, elevating with sudo
// when destDir is not writable by the current user, and returns the final path.
func placeBinary(ctx context.Context, src, destDir, name string) (string, error) {
	if err := ensureDir(ctx, destDir); err != nil {
		return "", err
	}
	dest := filepath.Join(destDir, name)
	if pathlink.Writable(destDir) {
		if err := copyFile(src, dest); err != nil {
			return "", err
		}
	} else if err := runElevated(ctx, "cp", src, dest); err != nil {
		return "", err
	}
	if err := chmodExec(ctx, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src) //nolint:gosec // src is under a temp dir built by this process
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755) //nolint:gosec // dest is an install directory chosen by the caller
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func chmodExec(ctx context.Context, path string) error {
	if pathlink.Writable(filepath.Dir(path)) {
		return os.Chmod(path, 0o755) //nolint:gosec // an installed CLI must be executable
	}
	return runElevated(ctx, "chmod", "0755", path)
}

func ensureDir(ctx context.Context, dir string) error {
	if _, err := os.Stat(dir); err == nil {
		return nil
	}
	parent := filepath.Dir(dir)
	if pathlink.Writable(parent) {
		return os.MkdirAll(dir, 0o755) //nolint:gosec // install directories such as /usr/local/bin are world-readable by convention
	}
	return runElevated(ctx, "mkdir", "-p", dir)
}

// useSudo reports whether commands that need write access should be prefixed
// with sudo: only when the process is not already root and sudo is available.
func useSudo() bool {
	if os.Geteuid() == 0 {
		return false
	}
	_, err := exec.LookPath("sudo")
	return err == nil
}

func runElevated(ctx context.Context, name string, args ...string) error {
	if useSudo() {
		args = append([]string{name}, args...)
		name = "sudo"
	}
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // command names are constant; args are install paths
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// linkOnPath makes the installed binaries resolvable by name in later steps by
// linking them into a directory already on PATH.
func linkOnPath(fromDir string, names []string, aliases map[string]string, verify func(bin string) error) error {
	_, err := pathlink.Ensure(pathlink.Options{
		Names:   names,
		FromDir: fromDir,
		Aliases: aliases,
		Verify:  verify,
	})
	return err
}

// installedVersionContains reports whether bin is on PATH and its `--version`
// output contains want. It is how each installer stays idempotent: a matching
// version already present is left in place.
func installedVersionContains(ctx context.Context, bin, want string) bool {
	path, err := exec.LookPath(bin)
	if err != nil {
		return false
	}
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput() //nolint:gosec // bin was resolved from PATH
	if err != nil {
		return false
	}
	return strings.Contains(string(out), want)
}

// verifyReportsVersion runs `bin --version` and confirms the output contains
// want. It is the Verify passed to linkOnPath: the link is only accepted if the
// binary PATH resolves is the one just installed.
func verifyReportsVersion(ctx context.Context, bin, want string) error {
	out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput() //nolint:gosec // bin is a binary this process installed
	if err != nil {
		return err
	}
	if !strings.Contains(string(out), want) {
		return fmt.Errorf("%s reports %q, expected %s", filepath.Base(bin), strings.TrimSpace(string(out)), want)
	}
	return nil
}

// readFile returns the file's contents as a string, or "" if it cannot be read.
func readFile(path string) string {
	b, err := os.ReadFile(path) //nolint:gosec // path is under a temp dir built by this process
	if err != nil {
		return ""
	}
	return string(b)
}
