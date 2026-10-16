package install

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const downloadRetries = 3

func (i Installer) get(ctx context.Context, url string, read func(io.Reader) error) error {
	var lastErr error
	for attempt := 1; attempt <= downloadRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := i.HTTP.Do(req)
		if err != nil {
			lastErr = err
		} else {
			lastErr = i.consumeResponse(resp, url, read)
			if lastErr == nil {
				return nil
			}
		}
		if isNotFound(lastErr) {
			return lastErr
		}
		if attempt < downloadRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}
	return fmt.Errorf("fetch %s: %w", url, lastErr)
}

func (i Installer) consumeResponse(resp *http.Response, url string, read func(io.Reader) error) error {
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return &notFoundError{url: url}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return read(resp.Body)
}

func (i Installer) fetch(ctx context.Context, url, dest string) error {
	return i.get(ctx, url, func(r io.Reader) error {
		f, err := os.Create(dest) //nolint:gosec // dest is a path this process built under a temp dir
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(f, r)
		return err
	})
}

func (i Installer) fetchBytes(ctx context.Context, url string) ([]byte, error) {
	var result []byte
	err := i.get(ctx, url, func(r io.Reader) error {
		var readErr error
		result, readErr = io.ReadAll(r)
		return readErr
	})
	return result, err
}

// checksumFor parses a checksum file and returns the lowercase hex for name.
// Lines may be "<hex>  <name>", "<hex> *<name>", or a bare "<hex>" for
// single-file .sha256 files.
func checksumFor(sums []byte, name string) (string, error) {
	text := strings.TrimSpace(string(sums))
	lines := strings.Split(text, "\n")

	if len(lines) == 1 {
		parts := strings.Fields(lines[0])
		if len(parts) == 1 {
			return strings.ToLower(parts[0]), nil
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		hash := strings.ToLower(strings.TrimSpace(parts[0]))
		fname := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[1]), "*"))
		if fname == name {
			return hash, nil
		}
	}
	return "", &checksumMissingError{name: name}
}

type checksumMissingError struct{ name string }

func (e *checksumMissingError) Error() string {
	return fmt.Sprintf("checksum not found for %q", e.name)
}

func isChecksumNotFound(err error) bool {
	var cme *checksumMissingError
	return errors.As(err, &cme)
}

func verifySHA256(fpath, want string) error {
	f, err := os.Open(fpath) //nolint:gosec // fpath is an install directory path
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != strings.ToLower(want) {
		return fmt.Errorf("SHA-256 mismatch for %s: got %s, want %s", filepath.Base(fpath), got, want)
	}
	return nil
}

func extractFile(archive, entry, dest string) error {
	if strings.HasSuffix(archive, ".zip") {
		return extractFileFromZip(archive, entry, dest)
	}
	return extractFileFromTarGz(archive, entry, dest)
}

func extractFileFromTarGz(archive, entry, dest string) error {
	f, err := os.Open(archive) //nolint:gosec // archive is a path this process built under a temp dir
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if path.Clean(hdr.Name) == entry {
			return writeExecutable(tr, dest)
		}
	}
	return fmt.Errorf("entry %q not found in %s", entry, archive)
}

func extractFileFromZip(archive, entry, dest string) error {
	zr, err := zip.OpenReader(archive) //nolint:gosec // archive is a path this process built under a temp dir
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if path.Clean(f.Name) == entry {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("open zip entry %s: %w", f.Name, err)
			}
			writeErr := writeExecutable(rc, dest)
			if cerr := rc.Close(); cerr != nil && writeErr == nil {
				writeErr = cerr
			}
			return writeErr
		}
	}
	return fmt.Errorf("entry %q not found in %s", entry, archive)
}

func writeExecutable(src io.Reader, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil { //nolint:gosec // install dirs are world-readable by convention
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil { //nolint:gosec // an installed CLI must be executable
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, dest)
}

// latestRelease resolves the latest release tag by following GitHub's
// releases/latest redirect. Repos with no GitHub releases redirect to
// /releases rather than /releases/tag/<version>, which we treat as an error.
func (i Installer) latestRelease(ctx context.Context, repo string) (string, error) {
	url := i.GitHubURL + "/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := i.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	tag := path.Base(resp.Request.URL.Path)
	if tag == "" || tag == "." || tag == "latest" || tag == "releases" {
		return "", fmt.Errorf("could not resolve latest release from %s", url)
	}
	return tag, nil
}

// fetchVerified downloads url to a temp file, verifies its SHA-256 against sum
// (skipped when sum is empty), calls use with the file path, then removes the temp dir.
func (i Installer) fetchVerified(ctx context.Context, url, sum string, use func(string) error) error {
	tmp, err := os.MkdirTemp("", "install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	dest := filepath.Join(tmp, path.Base(url))
	if err := i.fetch(ctx, url, dest); err != nil {
		return err
	}
	if sum != "" {
		if err := verifySHA256(dest, sum); err != nil {
			return err
		}
	}
	return use(dest)
}

func (i Installer) releaseChecksum(ctx context.Context, sumsURL, name string) (string, error) {
	data, err := i.fetchBytes(ctx, sumsURL)
	if err != nil {
		return "", err
	}
	return checksumFor(data, name)
}

func copyExecutable(src, dest string) error {
	f, err := os.Open(src) //nolint:gosec
	if err != nil {
		return err
	}
	defer f.Close()
	return writeExecutable(f, dest)
}

type notFoundError struct{ url string }

func (e *notFoundError) Error() string { return fmt.Sprintf("GET %s: 404 Not Found", e.url) }

func isNotFound(err error) bool {
	var nfe *notFoundError
	return errors.As(err, &nfe)
}
