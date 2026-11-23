package login

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/circleci-functions/setup-docker/install"
)

// garRe matches Artifact Registry hosts, including gcr.io domains it now serves.
var garRe = regexp.MustCompile(`^([a-z0-9-]+-docker\.pkg\.dev|([a-z]+\.)?gcr\.io)$`)

// loginGAR points Docker at docker-credential-gcr, which reads Google
// Application Default Credentials (ADC) whenever Docker needs a token.
func loginGAR(ctx context.Context, ins install.Installer, p install.Platform, cfg Config) error {
	if err := ensureADC(cfg.GCPKeyEnv); err != nil {
		return err
	}
	bin, err := ins.GCRHelper(ctx, p)
	if err != nil {
		return fmt.Errorf("install docker-credential-gcr: %w", err)
	}
	if err := checkGCRToken(ctx, bin, cfg.Registry); err != nil {
		return err
	}
	dcfg, err := readDockerConfig()
	if err != nil {
		return err
	}
	if err := dcfg.setCredHelper(cfg.Registry, "gcr"); err != nil {
		return err
	}
	if err := dcfg.write(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "setup-docker: %s uses docker-credential-gcr\n", cfg.Registry)
	return nil
}

// ensureADC makes sure the helper will find Google credentials. A key in
// keyEnv is written to the well-known ADC file, which later steps find with
// no environment variable, unless credentials are already set up.
func ensureADC(keyEnv string) error {
	if f := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); f != "" {
		fmt.Fprintf(os.Stderr, "setup-docker: using Google credentials from GOOGLE_APPLICATION_CREDENTIALS (%s)\n", f)
		return nil
	}
	path := adcPath()
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "setup-docker: using existing Google credentials in %s\n", path)
		return nil
	}
	key := strings.TrimSpace(os.Getenv(keyEnv))
	if key == "" {
		if _, err := exec.LookPath("gcloud"); err == nil {
			return nil
		}
		return fmt.Errorf("no Google credentials: set %s to a service account key, or set up Application Default Credentials in an earlier step", keyEnv)
	}
	data, err := decodeKey(key)
	if err != nil {
		return fmt.Errorf("%s: %w", keyEnv, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // path is the well-known ADC file
		return err
	}
	fmt.Fprintf(os.Stderr, "setup-docker: wrote the key in %s to %s\n", keyEnv, path)
	return nil
}

// decodeKey accepts a service account key as JSON or base64-encoded JSON.
func decodeKey(key string) ([]byte, error) {
	data := []byte(key)
	if !strings.HasPrefix(key, "{") {
		var err error
		if data, err = base64.StdEncoding.DecodeString(key); err != nil {
			return nil, fmt.Errorf("not JSON or base64-encoded JSON")
		}
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("not valid JSON")
	}
	return data, nil
}

// adcPath is where Google's client libraries look for ADC by default.
func adcPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "gcloud", "application_default_credentials.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
}

// checkGCRToken asks the helper for a token now, so bad credentials fail this
// step rather than a later push. The token is discarded.
func checkGCRToken(ctx context.Context, bin, registry string) error {
	cmd := exec.CommandContext(ctx, bin, "get") //nolint:gosec // bin is the helper this process installed
	cmd.Stdin = strings.NewReader(registry)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker-credential-gcr could not get a token for %s: %s", registry, lastNonEmpty(stderr.String()))
	}
	return nil
}
