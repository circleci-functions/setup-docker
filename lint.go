package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/circleci-functions/toolkit/sdk"

	"github.com/circleci-functions/setup-docker/install"
)

type lintCmd struct {
	Dockerfiles       string `default:"Dockerfile" help:"Dockerfiles to lint, newline or colon separated (semicolon on Windows)."`
	FailureThreshold  string `enum:"error,warning,info,style,ignore,none" default:"info" help:"Fail on findings at or above this level, as hadolint does: style, ignore and none fail on any finding."`
	IgnoreRules       string `help:"Rules to ignore, comma separated, e.g. DL3008,SC2086."`
	TrustedRegistries string `help:"Registries base images may come from, comma separated. Others are reported."`
	ResultsFile       string `help:"Also write the findings as JUnit XML to this path, for store_test_results."`
	HadolintVersion   string `help:"hadolint version, or latest. Empty installs ${hadolint_version}."`
	DownloadURL       string `hidden:"" help:"Base URL to download from in place of github.com."`
}

func (c *lintCmd) Run(ctx context.Context, _ *sdk.Client) error {
	i := install.New("")
	if c.DownloadURL != "" {
		i.GitHubURL = c.DownloadURL
	}
	p := install.Current()
	if err := report(i.Hadolint(ctx, p, c.HadolintVersion)); err != nil {
		return fmt.Errorf("hadolint: %w", err)
	}

	files := splitDockerfiles(c.Dockerfiles, filepath.ListSeparator)
	if len(files) == 0 {
		return fmt.Errorf("no Dockerfiles specified")
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("%s: no such file", f)
		}
	}

	var failed []string
	for _, f := range files {
		args := hadolintArgs(c.FailureThreshold, c.IgnoreRules, c.TrustedRegistries, "", f)
		cmd := exec.CommandContext(ctx, "hadolint", args...) //nolint:gosec
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			failed = append(failed, f)
		}
	}

	if c.ResultsFile != "" {
		if err := writeResults(ctx, c, files); err != nil {
			return err
		}
	}

	if len(failed) > 0 {
		for _, f := range failed {
			fmt.Fprintf(os.Stderr, "setup-docker: %s has findings at or above %s\n", f, c.FailureThreshold)
		}
		return fmt.Errorf("lint found issues in %d file(s)", len(failed))
	}
	return nil
}

func writeResults(ctx context.Context, c *lintCmd, files []string) error {
	var parts [][]byte
	for _, f := range files {
		args := hadolintArgs(c.FailureThreshold, c.IgnoreRules, c.TrustedRegistries, "junit", f)
		var buf bytes.Buffer
		cmd := exec.CommandContext(ctx, "hadolint", args...) //nolint:gosec
		cmd.Stdout = &buf
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		if buf.Len() > 0 {
			parts = append(parts, buf.Bytes())
		}
	}
	if err := os.MkdirAll(filepath.Dir(c.ResultsFile), 0o755); err != nil { //nolint:gosec
		return err
	}
	return os.WriteFile(c.ResultsFile, mergeJUnit(parts), 0o644) //nolint:gosec
}

func mergeJUnit(parts [][]byte) []byte {
	var inner strings.Builder
	for _, p := range parts {
		s := string(p)
		start := strings.Index(s, "<testsuites")
		if start < 0 {
			continue
		}
		tagEnd := strings.Index(s[start:], ">")
		if tagEnd < 0 {
			continue
		}
		content := s[start+tagEnd+1:]
		if end := strings.LastIndex(content, "</testsuites>"); end >= 0 {
			content = content[:end]
		}
		inner.WriteString(content)
	}
	return []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<testsuites>\n" + inner.String() + "</testsuites>\n")
}

func hadolintArgs(threshold, ignoreRules, trustedRegs, format, file string) []string {
	args := []string{"--failure-threshold", threshold}
	if format != "" {
		args = append(args, "--format", format)
	}
	for _, r := range splitCSV(ignoreRules) {
		args = append(args, "--ignore", r)
	}
	for _, r := range splitCSV(trustedRegs) {
		args = append(args, "--trusted-registry", r)
	}
	return append(args, file)
}

// splitDockerfiles splits on newlines and the OS path-list separator: ':' as the orb did, but ';' on Windows,
// where ':' is part of a drive letter.
func splitDockerfiles(s string, listSep rune) []string {
	var result []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == listSep }) {
		if t := strings.TrimSpace(part); t != "" {
			result = append(result, t)
		}
	}
	return result
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			result = append(result, t)
		}
	}
	return result
}
