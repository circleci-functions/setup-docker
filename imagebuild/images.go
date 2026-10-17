package imagebuild

const (
	buildkitVersion    = "v0.34.0@sha256:b059f8d7226d0b326bc871af489a5dddf65e36af4d20d14251b072974d9ee87c"      // renovate-digest: datasource=docker depName=moby/buildkit
	binfmtVersion      = "qemu-v10.2.3@sha256:400a4873b838d1b89194d982c45e5fb3cda4593fbfd7e08a02e76b03b21166f0" // renovate-digest: datasource=docker depName=tonistiigi/binfmt
	sbomScannerVersion = "1.12.0@sha256:ae4f3b554449e7e25548e7d8ccc029d17357348e30c6e3df01b92bc93654d6a9"       // renovate-digest: datasource=docker depName=docker/buildkit-syft-scanner

	buildkitImage    = "moby/buildkit:" + buildkitVersion
	binfmtImage      = "tonistiigi/binfmt:" + binfmtVersion
	sbomScannerImage = "docker/buildkit-syft-scanner:" + sbomScannerVersion
)

// --call=check needs buildx 0.15; images such as cimg/base:stable ship an older one.
const minBuildxVersion = "0.15.0"
