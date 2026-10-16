package install

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/circleci-functions/toolkit/pathlink"
)

type Result struct {
	Tool       string
	Version    string
	Dir        string
	Kept       bool
	Unverified bool
}

// chooseVersion applies the version rule: with no version asked for, keep whatever is installed or
// fall back to def; with one asked for, keep only an exact match.
func chooseVersion(ctx context.Context, want, def, have string, latest func(context.Context) (string, error)) (
	version string, keep bool, err error) {
	if want == "" {
		if have != "" {
			return have, true, nil
		}
		want = def
	}
	if want == "latest" {
		if want, err = latest(ctx); err != nil {
			return "", false, err
		}
	}
	return want, have != "" && have == strings.TrimPrefix(want, "v"), nil
}

func (i Installer) link(ctx context.Context, fromDir, version string, names ...string) (string, error) {
	res, err := pathlink.Ensure(pathlink.Options{
		Names:     names,
		FromDir:   fromDir,
		Dir:       i.InstallDir,
		Preferred: pathlink.UserBinDirs(),
		Verify: func(bin string) error {
			return reportsVersion(ctx, bin, version)
		},
	})
	return res.Dir, err
}

func reportsVersion(ctx context.Context, bin, version string) error {
	out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not report its version: %w", err)
	}
	if got := versionRe.FindString(string(out)); got != strings.TrimPrefix(version, "v") {
		return fmt.Errorf("reports version %q, not %s", got, version)
	}
	return nil
}
