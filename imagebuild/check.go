package imagebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type checkResult struct {
	Warnings []checkWarning `json:"warnings"`
}

type checkWarning struct {
	RuleName    string `json:"ruleName"`
	Description string `json:"description"`
}

// runCheck runs the Dockerfile checks and returns an error when they fail.
// Warnings are printed to stderr unless CheckWarningsAsErrors is set.
func runCheck(ctx context.Context, f Flags, args []string) error {
	// Rebuild args for check mode: drop output-related flags, add --call=check,format=json.
	checkArgs := checkBuildArgs(args)

	var buf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", checkArgs...)
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err == nil {
		return nil
	}

	result, parseErr := parseCheckOutput(buf.Bytes())
	if parseErr != nil {
		// Could not parse JSON; surface the last line of output as the error.
		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		return fmt.Errorf("build check failed: %s", lines[len(lines)-1])
	}

	if len(result.Warnings) == 0 {
		return fmt.Errorf("build check failed")
	}

	if !f.CheckWarningsAsErrors {
		for _, w := range result.Warnings {
			fmt.Fprintf(os.Stderr, "setup-docker: check warning %s: %s\n", w.RuleName, w.Description)
		}
		return nil
	}

	names := make([]string, len(result.Warnings))
	for i, w := range result.Warnings {
		names[i] = w.RuleName
	}
	return fmt.Errorf("build check failed: %s", strings.Join(names, ", "))
}

// checkBuildArgs transforms the full build argv into a check argv.
// It replaces the output section (--push, --load, --output, --metadata-file,
// --cache-to) with --call=check,format=json and drops -t flags.
func checkBuildArgs(args []string) []string {
	drop := map[string]bool{
		"--push": true, "--load": true,
	}
	dropNext := map[string]bool{
		"-t": true, "--output": true, "--metadata-file": true, "--cache-to": true,
	}
	var out []string
	skipNext := false
	for _, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if drop[a] {
			continue
		}
		if dropNext[a] {
			skipNext = true
			continue
		}
		// Drop flags that start with these prefixes.
		if strings.HasPrefix(a, "--output=") || strings.HasPrefix(a, "--metadata-file=") ||
			strings.HasPrefix(a, "--cache-to=") || strings.HasPrefix(a, "--sbom=") ||
			strings.HasPrefix(a, "--provenance=") {
			continue
		}
		out = append(out, a)
	}
	out = append(out, "--call=check,format=json")
	return out
}

func parseCheckOutput(data []byte) (checkResult, error) {
	var r checkResult
	if err := json.Unmarshal(bytes.TrimSpace(data), &r); err != nil {
		return r, err
	}
	return r, nil
}
