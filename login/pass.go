package login

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const passKeyEmail = "setup-docker@localhost" //nolint:gosec // an email address, not a credential

const gpgKeyTemplate = `Key-Type: RSA
Key-Length: 2048
Name-Real: CircleCI setup-docker
Name-Email: ` + passKeyEmail + `
Expire-Date: 0
%no-protection
%no-ask-passphrase
%commit
`

// setupPass installs pass and gpg if missing, then initialises a throwaway
// gpg-backed store so docker-credential-pass can store credentials.
func setupPass(ctx context.Context) error {
	if _, err := exec.LookPath("pass"); err != nil {
		if _, serr := exec.LookPath("sudo"); serr == nil {
			if err := runCmd(ctx, "sudo", "apt-get", "update", "--yes"); err != nil {
				return fmt.Errorf("apt-get update: %w", err)
			}
			if err := runCmd(ctx, "sudo", "apt-get", "install", "--yes", "gnupg", "pass"); err != nil {
				return fmt.Errorf("install gnupg pass: %w", err)
			}
		} else {
			return fmt.Errorf("pass is not installed and sudo is not available; install gnupg and pass manually")
		}
	}

	tmp, err := os.MkdirTemp("", "gpg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	tmplPath := filepath.Join(tmp, "key-template")
	if err := os.WriteFile(tmplPath, []byte(gpgKeyTemplate), 0o600); err != nil {
		return err
	}
	if err := runCmd(ctx, "gpg", "--batch", "--gen-key", tmplPath); err != nil {
		return fmt.Errorf("generate gpg key: %w", err)
	}

	out, err := exec.CommandContext(ctx, "gpg", "--list-keys", "--with-fingerprint", "--with-colons", passKeyEmail).Output()
	if err != nil {
		return fmt.Errorf("read gpg key fingerprint: %w", err)
	}
	fingerprint, ok := parseFingerprint(string(out))
	if !ok {
		return fmt.Errorf("no gpg fingerprint found for %s", passKeyEmail)
	}
	if err := runCmd(ctx, "pass", "init", fingerprint); err != nil {
		return fmt.Errorf("initialise pass: %w", err)
	}
	return nil
}

// parseFingerprint extracts the key fingerprint from gpg's colon-delimited
// listing; field 10 of the fpr record.
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

func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // command name is constant; args are process-controlled
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
