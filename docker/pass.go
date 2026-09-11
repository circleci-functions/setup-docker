package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const passKeyEmail = "setup-docker@circleci.com" //nolint:gosec // an email address, not a credential

const gpgKeyTemplate = `Key-Type: RSA
Key-Length: 2048
Name-Real: CircleCI setup-docker
Name-Email: ` + passKeyEmail + `
Expire-Date: 0
%no-protection
%no-ask-passphrase
%commit
`

// setupPass installs the pass password manager and initialises it with a fresh
// gpg key, which docker-credential-pass needs before it can store credentials.
// Requires an apt-based distribution.
func setupPass(ctx context.Context) error {
	if err := runElevated(ctx, "apt-get", "update", "--yes"); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
	}
	if err := runElevated(ctx, "apt-get", "install", "gnupg2", "pass", "--yes"); err != nil {
		return fmt.Errorf("install gnupg2 and pass: %w", err)
	}

	tmpl := filepath.Join(mustTempDir(), "gpg-key-template")
	if err := os.WriteFile(tmpl, []byte(gpgKeyTemplate), 0o600); err != nil {
		return err
	}
	if err := run(ctx, "gpg2", "--batch", "--gen-key", tmpl); err != nil {
		return fmt.Errorf("generate gpg key: %w", err)
	}

	out, err := exec.CommandContext(ctx, "gpg2", "--list-keys", "--with-fingerprint", "--with-colons", passKeyEmail).Output()
	if err != nil {
		return fmt.Errorf("read gpg key fingerprint: %w", err)
	}
	fingerprint, ok := parseFingerprint(string(out))
	if !ok {
		return fmt.Errorf("no gpg fingerprint found for %s", passKeyEmail)
	}
	if err := run(ctx, "pass", "init", fingerprint); err != nil {
		return fmt.Errorf("initialise pass: %w", err)
	}
	return nil
}

// parseFingerprint returns the key fingerprint from gpg's colon-delimited
// listing, which is field 10 of the fpr record.
func parseFingerprint(gpgColons string) (string, bool) {
	for _, line := range strings.Split(gpgColons, "\n") {
		if !strings.HasPrefix(line, "fpr:") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) >= 10 && fields[9] != "" {
			return fields[9], true
		}
	}
	return "", false
}

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // command name is constant; args are process-controlled
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
