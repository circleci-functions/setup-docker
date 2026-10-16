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

// Config holds all flags for the login command.
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

// IsECR reports whether registry is an ECR endpoint.
func IsECR(registry string) bool {
	return ecrRe.MatchString(registry)
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

func dockerLoginWithRetry(ctx context.Context, registry, username, password string, retries int) error {
	for attempt := 0; attempt < retries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		out, err := runDockerLogin(ctx, registry, username, password)
		if err == nil {
			return nil
		}
		lower := strings.ToLower(string(out))
		if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "incorrect username or password") {
			return fmt.Errorf("docker login to %s: credentials rejected", registry)
		}
	}
	return fmt.Errorf("docker login to %s failed after %d attempt(s)", registry, retries)
}

func runDockerLogin(ctx context.Context, registry, username, password string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "login", "--username", username, "--password-stdin", registry) //nolint:gosec
	cmd.Stdin = strings.NewReader(password)
	return cmd.CombinedOutput()
}
