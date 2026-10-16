package install

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

type platformAssets struct {
	indexPath string
	tarball   string
	buildx    string
	compose   string
	dockerize string
	gossAsset string
	gossSums  string
}

func collectAssets(t *testing.T, p Platform, version, alpineReleasePath string) (platformAssets, error) {
	t.Helper()
	idx, err := p.dockerIndexPath()
	if err != nil {
		return platformAssets{}, err
	}
	tarball, err := p.dockerTarballPath(version)
	if err != nil {
		return platformAssets{}, err
	}
	buildx, err := p.buildxAsset(version)
	if err != nil {
		return platformAssets{}, err
	}
	compose, err := p.composeAsset()
	if err != nil {
		return platformAssets{}, err
	}
	dockerize, err := p.dockerizeAsset(version, alpineReleasePath)
	if err != nil {
		return platformAssets{}, err
	}
	ga, gs, err := p.gossAsset(version)
	if err != nil {
		return platformAssets{}, err
	}
	return platformAssets{idx, tarball, buildx, compose, dockerize, ga, gs}, nil
}

func TestPlatformAssets(t *testing.T) {
	alpineFile := filepath.Join(t.TempDir(), "alpine-release")
	assert.NilError(t, os.WriteFile(alpineFile, []byte("3.18.0"), 0o644))

	const ver = "v29.9.1"
	const noAlpine = "/no/such/path"

	cases := []struct {
		name              string
		p                 Platform
		alpineReleasePath string
		want              platformAssets
	}{
		{
			name:              "linux/amd64",
			p:                 Platform{"linux", "amd64"},
			alpineReleasePath: noAlpine,
			want: platformAssets{
				indexPath: "linux/static/stable/x86_64/",
				tarball:   "linux/static/stable/x86_64/docker-29.9.1.tgz",
				buildx:    "buildx-v29.9.1.linux-amd64",
				compose:   "docker-compose-linux-x86_64",
				dockerize: "dockerize-linux-amd64-v29.9.1.tar.gz",
				gossAsset: "goss_29.9.1_linux_x86_64.tar.gz",
				gossSums:  "goss_29.9.1_SHA256SUMS",
			},
		},
		{
			name:              "linux/arm64",
			p:                 Platform{"linux", "arm64"},
			alpineReleasePath: noAlpine,
			want: platformAssets{
				indexPath: "linux/static/stable/aarch64/",
				tarball:   "linux/static/stable/aarch64/docker-29.9.1.tgz",
				buildx:    "buildx-v29.9.1.linux-arm64",
				compose:   "docker-compose-linux-aarch64",
				dockerize: "dockerize-linux-arm64-v29.9.1.tar.gz",
				gossAsset: "goss_29.9.1_linux_arm64.tar.gz",
				gossSums:  "goss_29.9.1_SHA256SUMS",
			},
		},
		{
			name:              "darwin/arm64",
			p:                 Platform{"darwin", "arm64"},
			alpineReleasePath: noAlpine,
			want: platformAssets{
				indexPath: "mac/static/stable/aarch64/",
				tarball:   "mac/static/stable/aarch64/docker-29.9.1.tgz",
				buildx:    "buildx-v29.9.1.darwin-arm64",
				compose:   "docker-compose-darwin-aarch64",
				dockerize: "dockerize-darwin-arm64-v29.9.1.tar.gz",
				gossAsset: "goss_29.9.1_darwin_arm64.tar.gz",
				gossSums:  "goss_29.9.1_SHA256SUMS",
			},
		},
		{
			name:              "linux/amd64 alpine",
			p:                 Platform{"linux", "amd64"},
			alpineReleasePath: alpineFile,
			want: platformAssets{
				indexPath: "linux/static/stable/x86_64/",
				tarball:   "linux/static/stable/x86_64/docker-29.9.1.tgz",
				buildx:    "buildx-v29.9.1.linux-amd64",
				compose:   "docker-compose-linux-x86_64",
				dockerize: "dockerize-alpine-linux-amd64-v29.9.1.tar.gz",
				gossAsset: "goss_29.9.1_linux_x86_64.tar.gz",
				gossSums:  "goss_29.9.1_SHA256SUMS",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collectAssets(t, tc.p, ver, tc.alpineReleasePath)
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(got.indexPath, tc.want.indexPath))
			assert.Check(t, cmp.Equal(got.tarball, tc.want.tarball))
			assert.Check(t, cmp.Equal(got.buildx, tc.want.buildx))
			assert.Check(t, cmp.Equal(got.compose, tc.want.compose))
			assert.Check(t, cmp.Equal(got.dockerize, tc.want.dockerize))
			assert.Check(t, cmp.Equal(got.gossAsset, tc.want.gossAsset))
			assert.Check(t, cmp.Equal(got.gossSums, tc.want.gossSums))
		})
	}
}

func TestUnsupportedPlatform(t *testing.T) {
	p := Platform{"freebsd", "amd64"}
	_, err := p.dockerIndexPath()
	assert.Check(t, cmp.ErrorContains(err, "unsupported platform freebsd/amd64"))
}

func TestWindowsPlatformAssets(t *testing.T) {
	const ver = "v29.9.1"

	cases := []struct {
		name    string
		p       Platform
		idx     string
		tarball string
		buildx  string
		compose string
		idxErr  string
	}{
		{
			name:    "windows/amd64",
			p:       Platform{"windows", "amd64"},
			idx:     "win/static/stable/x86_64/",
			tarball: "win/static/stable/x86_64/docker-29.9.1.zip",
			buildx:  "buildx-v29.9.1.windows-amd64.exe",
			compose: "docker-compose-windows-x86_64.exe",
		},
		{
			name:    "windows/arm64 docker unsupported",
			p:       Platform{"windows", "arm64"},
			idxErr:  "unsupported platform windows/arm64",
			buildx:  "buildx-v29.9.1.windows-arm64.exe",
			compose: "docker-compose-windows-aarch64.exe",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx, err := tc.p.dockerIndexPath()
			if tc.idxErr != "" {
				assert.Check(t, cmp.ErrorContains(err, tc.idxErr))
			} else {
				assert.NilError(t, err)
				assert.Check(t, cmp.Equal(idx, tc.idx))
				tarball, tarErr := tc.p.dockerTarballPath(ver)
				assert.NilError(t, tarErr)
				assert.Check(t, cmp.Equal(tarball, tc.tarball))
			}

			buildx, err := tc.p.buildxAsset(ver)
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(buildx, tc.buildx))

			compose, err := tc.p.composeAsset()
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(compose, tc.compose))

			_, dockerizeErr := tc.p.dockerizeAsset(ver, "/no/such/path")
			assert.Check(t, cmp.ErrorContains(dockerizeErr, "dockerize has no Windows build"))

			_, _, gossErr := tc.p.gossAsset(ver)
			assert.Check(t, cmp.ErrorContains(gossErr, "goss is not supported on Windows"))
		})
	}
}

func TestExeHelper(t *testing.T) {
	cases := []struct {
		p    Platform
		name string
		want string
	}{
		{Platform{"linux", "amd64"}, "docker", "docker"},
		{Platform{"darwin", "arm64"}, "docker", "docker"},
		{Platform{"windows", "amd64"}, "docker", "docker.exe"},
		{Platform{"windows", "amd64"}, "docker-buildx", "docker-buildx.exe"},
	}
	for _, tc := range cases {
		got := tc.p.exe(tc.name)
		assert.Check(t, cmp.Equal(got, tc.want))
	}
}
