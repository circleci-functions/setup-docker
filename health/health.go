// Package health starts a container and waits for it to report healthy.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/circleci-functions/setup-docker/shellwords"
)

// DefaultProbeImage is pinned by digest. It checks the endpoint from inside the
// app container's network, which works whether or not the job can reach the
// engine's ports (with setup_remote_docker it can't).
const DefaultProbeImage = "busybox:1.37@sha256:a7dbc319b1438115a207e11930dc4bb3e643dcb3ec0bdd694bb36cd8fad289ec"

const (
	pollInterval = 2 * time.Second
	probeTimeout = 5 * time.Second
	logLines     = "100"
)

// Config holds the flags for the health command.
type Config struct {
	Image      string
	Port       int
	Path       string
	Env        string
	PassEnv    string
	Timeout    time.Duration
	Command    string
	RunArgs    string
	ProbeImage string
}

type state struct {
	Running  bool
	ExitCode int
	Health   *struct {
		Status string
		Log    []struct{ Output string }
	}
}

// Check starts cfg.Image, waits until it's healthy, then removes the container.
// With a path it waits for an HTTP 2xx; without one, for the image's HEALTHCHECK.
func Check(ctx context.Context, cfg Config) error {
	if runtime.GOOS == "windows" {
		return errors.New("health is not supported on Windows executors")
	}
	args, err := runArgs(cfg)
	if err != nil {
		return err
	}
	if cfg.Path != "" {
		if err := ensureImage(ctx, cfg.ProbeImage); err != nil {
			return err
		}
	}

	out, err := exec.CommandContext(ctx, "docker", args...).Output() //nolint:gosec // args are built from flags, never a shell
	if err != nil {
		return fmt.Errorf("docker run %s: %w", cfg.Image, cmdErr(err))
	}
	id := strings.TrimSpace(string(out))
	defer remove(id)

	if err := wait(ctx, cfg, id); err != nil {
		printLogs(id)
		return err
	}
	fmt.Fprintf(os.Stderr, "setup-docker: %s is healthy\n", cfg.Image)
	return nil
}

func runArgs(cfg Config) ([]string, error) {
	args := []string{"run", "-d"}
	for _, kv := range splitLines(cfg.Env) {
		if !strings.Contains(kv, "=") {
			return nil, fmt.Errorf("env: %q is not KEY=VALUE", kv)
		}
		args = append(args, "-e", kv)
	}
	for _, name := range splitCommaNewline(cfg.PassEnv) {
		if _, ok := os.LookupEnv(name); !ok {
			return nil, fmt.Errorf("pass-env: %s is not set", name)
		}
		// docker reads the value from its own environment, so it stays out of argv.
		args = append(args, "-e", name)
	}
	if cfg.Path == "" {
		args = append(args, "--health-interval", pollInterval.String())
	}
	extra, err := shellwords.Split(cfg.RunArgs)
	if err != nil {
		return nil, fmt.Errorf("run-args: %w", err)
	}
	command, err := shellwords.Split(cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("command: %w", err)
	}
	args = append(args, extra...)
	args = append(args, cfg.Image)
	return append(args, command...), nil
}

func wait(ctx context.Context, cfg Config, id string) error {
	path := "/" + strings.TrimPrefix(cfg.Path, "/")
	deadline := time.Now().Add(cfg.Timeout)
	for {
		st, err := inspect(ctx, id)
		if err != nil {
			return err
		}
		if !st.Running {
			return fmt.Errorf("%s exited with code %d before it was healthy", cfg.Image, st.ExitCode)
		}
		var healthy bool
		var last string
		if cfg.Path == "" {
			healthy, last, err = healthcheck(cfg.Image, st)
		} else {
			healthy, last = checkHTTP(ctx, cfg, id, path)
		}
		if err != nil || healthy {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s was not healthy after %s: %s", cfg.Image, cfg.Timeout, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// checkHTTP reports whether the endpoint answered with a 2xx, and if not, why.
func checkHTTP(ctx context.Context, cfg Config, id, path string) (bool, string) {
	status, err := probe(ctx, cfg.ProbeImage, id, fmt.Sprintf("http://127.0.0.1:%d%s", cfg.Port, path))
	switch {
	case err != nil:
		return false, err.Error()
	case status >= 200 && status < 300:
		return true, ""
	}
	return false, fmt.Sprintf("GET %s returned %d", path, status)
}

func healthcheck(image string, st state) (bool, string, error) {
	if st.Health == nil {
		return false, "", fmt.Errorf("%s has no HEALTHCHECK: set path to check an HTTP endpoint instead", image)
	}
	switch st.Health.Status {
	case "healthy":
		return true, "", nil
	case "unhealthy":
		return false, "", fmt.Errorf("%s HEALTHCHECK reported unhealthy%s", image, lastHealthOutput(st))
	}
	return false, "HEALTHCHECK status " + st.Health.Status, nil
}

func inspect(ctx context.Context, id string) (state, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{json .State}}", id).Output() //nolint:gosec
	if err != nil {
		return state{}, fmt.Errorf("docker inspect: %w", cmdErr(err))
	}
	var st state
	if err := json.Unmarshal(out, &st); err != nil {
		return state{}, fmt.Errorf("docker inspect: %w", err)
	}
	return st, nil
}

func lastHealthOutput(st state) string {
	if n := len(st.Health.Log); n > 0 {
		if out := strings.TrimSpace(st.Health.Log[n-1].Output); out != "" {
			return ": " + out
		}
	}
	return ""
}

var wgetStatusRe = regexp.MustCompile(`HTTP/[0-9.]+ (\d{3})`)

// probe runs wget in a container that shares the app container's network.
func probe(ctx context.Context, probeImage, id, u string) (int, error) {
	out, err := exec.CommandContext(ctx, "docker", "run", "--rm", "--network", "container:"+id, //nolint:gosec
		probeImage, "wget", "-q", "-S", "-O", "/dev/null", "-T", strconv.Itoa(int(probeTimeout.Seconds())), u).CombinedOutput()
	return parseWget(string(out), err)
}

func parseWget(out string, err error) (int, error) {
	if m := wgetStatusRe.FindAllStringSubmatch(out, -1); len(m) > 0 {
		return strconv.Atoi(m[len(m)-1][1])
	}
	if err != nil {
		return 0, errors.New(lastLine(out))
	}
	return http.StatusOK, nil
}

func ensureImage(ctx context.Context, image string) error {
	if exec.CommandContext(ctx, "docker", "image", "inspect", image).Run() == nil { //nolint:gosec
		return nil
	}
	if out, err := exec.CommandContext(ctx, "docker", "pull", image).CombinedOutput(); err != nil { //nolint:gosec
		return fmt.Errorf("docker pull %s: %s", image, lastLine(string(out)))
	}
	return nil
}

func printLogs(id string) {
	fmt.Fprintf(os.Stderr, "setup-docker: last %s lines of the container's logs:\n", logLines)
	cmd := exec.Command("docker", "logs", "--tail", logLines, id) //nolint:gosec
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

// remove uses its own context so the container goes even if the step was cancelled.
func remove(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "docker", "rm", "-f", id).Run() //nolint:gosec
}

func cmdErr(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return errors.New(lastLine(string(exitErr.Stderr)))
	}
	return err
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func splitCommaNewline(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
