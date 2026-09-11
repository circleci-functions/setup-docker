package docker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// GossOptions configures the goss/dgoss install.
type GossOptions struct {
	Version      string
	InstallDir   string
	Architecture string
	// Explicit is true when the caller asked for goss directly rather than
	// leaving it on by default. It decides how an unsupported platform is
	// handled: an explicit request errors, a defaulted one is skipped.
	Explicit bool
}

// InstallGoss installs goss and the dgoss wrapper into opts.InstallDir and links
// them onto PATH. goss is Linux-only: off Linux an explicit request errors and a
// defaulted one is skipped. A matching version already on PATH is left in place.
func InstallGoss(ctx context.Context, opts GossOptions) error {
	skip, err := gossSupport(isLinux(), opts.Explicit)
	if err != nil {
		return err
	}
	if skip {
		fmt.Fprintln(os.Stderr, "setup-docker: goss is only supported on Linux; skipping")
		return nil
	}

	version := opts.Version
	if version == "latest" || version == "" {
		resolved, rerr := resolveLatestFromRedirect(ctx, gossReleasesURL+"/latest")
		if rerr != nil {
			return fmt.Errorf("resolve latest goss version: %w", rerr)
		}
		version = resolved
	}
	number := versionNumber(version)

	if installedVersionContains(ctx, "goss", number) && onPath("dgoss") {
		fmt.Fprintf(os.Stderr, "setup-docker: goss %s already installed\n", version)
		return nil
	}

	base := gossReleasesURL + "/download/" + version
	tmp := mustTempDir()
	archive := filepath.Join(tmp, "goss.tar.gz")
	gossURL := fmt.Sprintf("%s/goss_%s_linux_%s.tar.gz", base, number, gossArch(opts.Architecture))
	if err := download(ctx, gossURL, archive); err != nil {
		return err
	}
	extracted, err := extractTarGz(archive, tmp)
	if err != nil {
		return err
	}
	gossBin, err := findBinary(extracted, "goss")
	if err != nil {
		return err
	}
	if _, err := placeBinary(ctx, gossBin, opts.InstallDir, "goss"); err != nil {
		return err
	}
	if err := linkOnPath(opts.InstallDir, []string{"goss"}, func(bin string) error {
		return verifyReportsVersion(ctx, bin, number)
	}); err != nil {
		return err
	}

	return installDgoss(ctx, opts, base, tmp)
}

// installDgoss installs the dgoss wrapper published alongside a goss release. A
// release without one is not fatal: goss stays installed and the caller is told
// dgoss was skipped.
func installDgoss(ctx context.Context, opts GossOptions, base, tmp string) error {
	dgossURL := base + "/dgoss"
	if !urlExists(ctx, dgossURL) {
		fmt.Fprintln(os.Stderr, "setup-docker: no dgoss wrapper for this goss release; skipping dgoss")
		return nil
	}
	dgossBin := filepath.Join(tmp, "dgoss")
	if err := download(ctx, dgossURL, dgossBin); err != nil {
		return err
	}
	if _, err := placeBinary(ctx, dgossBin, opts.InstallDir, "dgoss"); err != nil {
		return err
	}
	return linkOnPath(opts.InstallDir, []string{"dgoss"}, nil)
}

const gossReleasesURL = "https://github.com/goss-org/goss/releases"

// gossArch maps the amd64/arm64 flag value to the token goss uses in its release
// asset names, where amd64 is spelled x86_64.
func gossArch(arch string) string {
	if arch == "arm64" {
		return "arm64"
	}
	return "x86_64"
}

// gossSupport reports whether the install should be skipped, or errors, for the
// current platform. goss builds only for Linux.
func gossSupport(linux, explicit bool) (skip bool, err error) {
	if linux {
		return false, nil
	}
	if explicit {
		return false, fmt.Errorf("goss is only supported on Linux")
	}
	return true, nil
}
