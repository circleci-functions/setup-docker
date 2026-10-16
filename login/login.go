// Package login implements docker registry login with optional credential helpers.
package login

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/circleci-functions/setup-docker/install"
)

const ecrLoginBase = "https://amazon-ecr-credential-helper-releases.s3.us-east-2.amazonaws.com"

var ecrRe = regexp.MustCompile(`^\d{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com(\.cn)?$`)

// Config holds flags for the login command.
type Config struct {
	Registry                 string
	UsernameEnv              string
	PasswordEnv              string
	Retries                  int
	CredentialsStore         bool
	CredentialsHelper        string
	CredentialsHelperVersion string
	DownloadURL              string
}

// Login logs in to a container registry, installing credential helpers as needed.
func Login(ctx context.Context, ins install.Installer, p install.Platform, cfg Config) error {
	if ecrRe.MatchString(cfg.Registry) {
		return loginECR(ctx, ins, p, cfg)
	}
	return loginPassword(ctx, ins, p, cfg)
}

func loginECR(ctx context.Context, ins install.Installer, p install.Platform, cfg Config) error {
	base := ecrLoginBase
	if cfg.DownloadURL != "" {
		base = cfg.DownloadURL
	}
	version := install.DefaultECRLoginVersion
	binURL, sumsURL, err := p.ECRLoginURLs(base, version)
	if err != nil {
		return err
	}
	binName := "docker-credential-ecr-login"
	dest := filepath.Join(ins.CacheDir, "ecr-login", version, p.Exe(binName))
	if _, err := ins.InstallBinary(ctx, binURL, sumsURL, binName, dest, []string{p.Exe(binName)}); err != nil {
		return fmt.Errorf("install ecr-login: %w", err)
	}
	dcfg, err := readDockerConfig()
	if err != nil {
		return err
	}
	if err := dcfg.setCredHelper(cfg.Registry, "ecr-login"); err != nil {
		return err
	}
	if err := dcfg.write(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "setup-docker: %s uses docker-credential-ecr-login\n", cfg.Registry)
	return nil
}

func loginPassword(ctx context.Context, ins install.Installer, p install.Platform, cfg Config) error {
	username := os.Getenv(cfg.UsernameEnv)
	password := os.Getenv(cfg.PasswordEnv)
	if username == "" {
		return fmt.Errorf("environment variable %s is not set", cfg.UsernameEnv)
	}
	if password == "" {
		return fmt.Errorf("environment variable %s is not set", cfg.PasswordEnv)
	}

	if cfg.CredentialsStore {
		helperVersion := cfg.CredentialsHelperVersion
		if helperVersion == "" {
			helperVersion = install.DefaultCredentialHelpersVersion
		}
		helperName := cfg.CredentialsHelper
		if helperName == "" {
			helperName = defaultCredHelper(p.OS)
		}
		if err := installCredHelper(ctx, ins, p, helperName, helperVersion); err != nil {
			return err
		}
		dcfg, err := readDockerConfig()
		if err != nil {
			return err
		}
		if err := dcfg.setCredsStore(helperName); err != nil {
			return err
		}
		if err := dcfg.write(); err != nil {
			return err
		}
	}

	if err := dockerLoginWithRetry(ctx, cfg.Registry, username, password, cfg.Retries); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "setup-docker: logged in to %s\n", cfg.Registry)
	return nil
}

// dockerLoginWithRetry makes up to retries+1 attempts. Waits 1s, 2s, 4s… between
// attempts. Does not retry on credential rejection.
func dockerLoginWithRetry(ctx context.Context, registry, username, password string, retries int) error {
	maxAttempts := retries + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			wait := time.Duration(1<<(attempt-2)) * time.Second
			fmt.Fprintf(os.Stderr, "setup-docker: retrying in %ds\n", int(wait.Seconds()))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		out, err := runDockerLogin(ctx, registry, username, password)
		lastLine := lastNonEmpty(string(out))
		if err == nil {
			return nil
		}
		fmt.Fprintf(os.Stderr, "setup-docker: docker login to %s failed (attempt %d of %d): %s\n",
			registry, attempt, maxAttempts, lastLine)
		lower := strings.ToLower(string(out))
		if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "incorrect username or password") {
			return fmt.Errorf("docker login to %s: credentials rejected: %s", registry, lastLine)
		}
	}
	return fmt.Errorf("docker login to %s failed after %d attempt(s)", registry, maxAttempts)
}

// lastNonEmpty returns the last non-blank line of s.
func lastNonEmpty(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	return ""
}

func runDockerLogin(ctx context.Context, registry, username, password string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "login", "--username", username, "--password-stdin", registry) //nolint:gosec
	cmd.Stdin = strings.NewReader(password)
	return cmd.CombinedOutput()
}
