package imagebuild

const (
	buildkitVersion    = "v0.34.1@sha256:99368f3e46177c6405b9740e4461d91cee004128daeecca78d4ceac50ec1da5c"      // renovate-digest: datasource=docker depName=moby/buildkit
	binfmtVersion      = "qemu-v10.2.3@sha256:400a4873b838d1b89194d982c45e5fb3cda4593fbfd7e08a02e76b03b21166f0" // renovate-digest: datasource=docker depName=tonistiigi/binfmt
	sbomScannerVersion = "1.12.1@sha256:873a7458c3c8ac8e32c7b796c10e1600b07b963d2aa000a72a2d97aaaa2d6315"       // renovate-digest: datasource=docker depName=docker/buildkit-syft-scanner

	buildkitImage    = "moby/buildkit:" + buildkitVersion
	binfmtImage      = "tonistiigi/binfmt:" + binfmtVersion
	sbomScannerImage = "docker/buildkit-syft-scanner:" + sbomScannerVersion
)

// --call=check needs buildx 0.15; images such as cimg/base:stable ship an older one.
const minBuildxVersion = "0.15.0"
