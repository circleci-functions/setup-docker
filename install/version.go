package install

import (
	"context"
	"os/exec"
	"regexp"
)

var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func installedVersion(ctx context.Context, bin string, args ...string) (string, bool) {
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return "", false
	}
	out, err := exec.CommandContext(ctx, resolved, args...).CombinedOutput()
	if err != nil {
		return "", false
	}
	m := versionRe.Find(out)
	if m == nil {
		return "", false
	}
	return string(m), true
}
