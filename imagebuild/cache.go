package imagebuild

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/circleci-functions/toolkit/sdk"
)

// cacheDir returns the directory used to store BuildKit layer cache for scope.
func cacheDir(scope string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "setup-docker", "buildx", scope), nil
}

// cacheScope returns the sanitised scope string. When f.CacheScope is empty
// the image name is used with every character outside [a-z0-9-] replaced by -.
func cacheScope(f Flags) string {
	if f.CacheScope != "" {
		return f.CacheScope
	}
	return sanitiseScope(f.Image)
}

var nonScopeRe = regexp.MustCompile(`[^a-z0-9-]`)

func sanitiseScope(s string) string {
	return nonScopeRe.ReplaceAllString(strings.ToLower(s), "-")
}

// cacheKeys returns the save key and the restore keys for the layer cache.
func cacheKeys(f Flags) (saveKey string, restoreKeys []string) {
	scope := cacheScope(f)
	base := fmt.Sprintf("setup-docker-buildx-v1-%s-%s", sdk.OSArch(), scope)
	branch := os.Getenv("CIRCLE_BRANCH")
	sha := os.Getenv("CIRCLE_SHA1")

	saveKey = fmt.Sprintf("%s-%s-%s", base, branch, sha)
	restoreKeys = []string{
		base + "-" + branch + "-",
		base + "-",
	}
	return saveKey, restoreKeys
}

// restoreCache restores the layer cache into dir. All errors are logged, never fatal.
// Returns the cache directory path.
func restoreCache(ctx context.Context, client *sdk.Client, f Flags) string {
	if !f.Cache {
		return ""
	}
	scope := cacheScope(f)
	dir, err := cacheDir(scope)
	if err != nil {
		sdk.LogCacheErr("resolve cache dir", err)
		return ""
	}

	_, restoreKeys := cacheKeys(f)
	_, err = client.RestoreCache(ctx, restoreKeys, sdk.WithStepName("Restoring Docker layer cache"))
	sdk.LogCacheErr("restore", err)
	return dir
}

// registerCacheSave registers a teardown hook that saves the layer cache.
func registerCacheSave(ctx context.Context, client *sdk.Client, f Flags, dir string) {
	if !f.Cache || dir == "" {
		return
	}
	saveKey, _ := cacheKeys(f)
	sdk.LogCacheErr("save",
		client.AtTeardown().SaveCache(ctx, saveKey, []string{dir + ".new"},
			sdk.WithStepName("Saving Docker layer cache")))
}
