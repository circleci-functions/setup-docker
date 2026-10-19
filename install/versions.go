package install

const (
	DefaultDockerVersion            = "29.9.1"  // renovate: datasource=github-tags depName=docker/cli extractVersion=^v(?<version>.+)$
	DefaultBuildxVersion            = "v0.38.0" // renovate: datasource=github-releases depName=docker/buildx
	DefaultComposeVersion           = "v5.6.0"  // renovate: datasource=github-releases depName=docker/compose
	DefaultDockerizeVersion         = "v0.15.2" // renovate: datasource=github-releases depName=jwilder/dockerize
	DefaultGossVersion              = "v0.4.10" // renovate: datasource=github-releases depName=goss-org/goss
	DefaultECRLoginVersion          = "0.12.0"  // renovate: datasource=github-releases depName=awslabs/amazon-ecr-credential-helper extractVersion=^v(?<version>.+)$
	DefaultCredentialHelpersVersion = "v0.9.10" // renovate: datasource=github-releases depName=docker/docker-credential-helpers
	DefaultHadolintVersion          = "v2.15.1" // renovate: datasource=github-releases depName=hadolint/hadolint
)

// download.docker.com publishes no checksums, so the default version's are kept here.
// Regenerate them with `task docker-checksums VERSION=<version>` when bumping it.
var dockerSHA256 = map[string]map[string]string{
	"29.9.1": {
		"linux/amd64":   "c158ecc55adb3afb01bbd3539ed3b997d20c26dbdc418080506cc1bf5243ff57",
		"linux/arm64":   "3e93cc6d1a686456a3b18eefdde26467a837a7a2b257af22e27cba1dbc1b2018",
		"darwin/amd64":  "9fc063bfae58ecf3c46e7528beaad9814968f7e624e9ba04a7f5cba5becad26c",
		"darwin/arm64":  "7f2c1637313578046e7dd61153e4e4cd1188d83a1e4b71825ab914776c208460",
		"windows/amd64": "5d9b66509390553fd97d590ec2ff0ce10b04c2ac653fa52f2abf2f05c4c197ca",
	},
}
