// Package imagebuild builds container images with docker buildx.
package imagebuild

// Flags holds parameters for the build command. It is designed to be embedded
// in a kong CLI struct or called from other packages directly.
type Flags struct {
	Image                 string `required:"" help:"Image to build, e.g. myorg/app or ghcr.io/org/app."`
	Registry              string `default:"docker.io" help:"Registry prefixed to image when image names no registry."`
	Tags                  string `help:"Tags, comma or newline separated. Empty uses $CIRCLE_SHA1."`
	Dockerfile            string `help:"Path to the Dockerfile. Empty uses the Dockerfile in context."`
	Context               string `default:"." help:"Build context directory."`
	Target                string `help:"Build stage to stop at."`
	Platforms             string `help:"Target platforms, comma separated, e.g. linux/amd64,linux/arm64."`
	Push                  bool   `help:"Push the image after building."`
	Load                  string `enum:"auto,true,false" default:"auto" help:"Load the image into the Docker engine. auto loads single-platform builds that aren't pushed."`
	BuildArgs             string `help:"Build arguments, one KEY=VALUE per line."`
	Labels                string `help:"Image labels, one KEY=VALUE per line."`
	SecretEnvs            string `help:"Build secrets from environment variables, comma or newline separated: ID=ENV_NAME, or ENV_NAME to use it as the ID."`
	Pull                  bool   `help:"Always pull newer base images."`
	NoCache               bool   `help:"Build without using any cache."`
	CacheFrom             string `help:"Extra cache sources, whitespace separated, e.g. type=registry,ref=org/app:cache."`
	CacheTo               string `help:"Extra cache destinations, whitespace separated."`
	Provenance            bool   `help:"Attach a provenance attestation."`
	SBOM                  bool   `name:"sbom" help:"Attach an SBOM attestation. Needs push or sbom-file."`
	SBOMFile              string `name:"sbom-file" help:"Also write the SBOM (SPDX JSON) to this path."`
	Check                 bool   `help:"Run the Dockerfile's build checks first. Warnings are printed; errors fail the build."`
	CheckWarningsAsErrors bool   `help:"With check, fail on warnings too."`
	PushByDigest          bool   `help:"Push without tags and write the digest to digest-file, for merging per-platform builds."`
	DigestFile            string `help:"Write the image digest to this path."`
	BuildkitdConfig       string `help:"Path to a buildkitd.toml for the builder, e.g. for a registry mirror or an insecure registry."`
	ExtraArgs             string `help:"Extra docker buildx build arguments, split like a shell would, never evaluated."`
	Retries               int    `default:"3" help:"Times to retry a build that fails while pushing."`
}
