package imagebuild

import (
	"fmt"
	"strings"
	"unicode"
)

// plan holds computed values used to assemble docker buildx build arguments.
type plan struct {
	builder            string
	refs               []string
	load               bool
	metadataFile       string
	sbomGen            string
	installedEmulators []string
}

// refs computes the full image references (registry/image:tag) from flags and a SHA.
// sha is the value of CIRCLE_SHA1; it is used as the default tag when Tags is empty.
func refs(f Flags, sha string) ([]string, error) {
	repo := applyRegistry(f.Image, f.Registry)
	repo = strings.ToLower(repo)

	tags, err := parseTags(f.Tags, sha)
	if err != nil {
		return nil, err
	}

	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = repo + ":" + sanitiseTag(t)
	}
	return out, nil
}

// applyRegistry prefixes registry to image when the image's first path component
// has no "." or ":" and is not "localhost".
func applyRegistry(image, registry string) string {
	i := strings.IndexByte(image, '/')
	if i < 0 {
		return registry + "/" + image
	}
	first := image[:i]
	if first == "localhost" || strings.ContainsAny(first, ".:") {
		return image
	}
	return registry + "/" + image
}

// parseTags splits the tags string on commas and newlines, trims whitespace,
// drops empties, and falls back to sha when the result is empty.
func parseTags(tags, sha string) ([]string, error) {
	var out []string
	for _, t := range strings.FieldsFunc(tags, func(r rune) bool { return r == ',' || r == '\n' }) {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		if sha == "" {
			return nil, fmt.Errorf("tags is empty and CIRCLE_SHA1 is not set")
		}
		out = []string{sha}
	}
	return out, nil
}

// sanitiseTag replaces characters outside [A-Za-z0-9_.-] with '-', turns a
// leading '.' or '-' into '_', and truncates to 128 characters.
func sanitiseTag(tag string) string {
	out := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-' {
			return r
		}
		return '-'
	}, tag)
	if len(out) > 0 && (out[0] == '.' || out[0] == '-') {
		out = "_" + out[1:]
	}
	if len(out) > 128 {
		out = out[:128]
	}
	return out
}

// loadFor returns whether to load the image based on the Load flag and build options.
// auto means true when there is at most one platform and neither Push nor PushByDigest.
func loadFor(f Flags) bool {
	switch f.Load {
	case "true":
		return true
	case "false":
		return false
	default: // auto
		platforms := strings.FieldsFunc(f.Platforms, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
		return len(platforms) <= 1 && !f.Push && !f.PushByDigest
	}
}

// validate checks flag combinations that are always invalid.
func validate(f Flags) error {
	if f.SBOM && !f.Push && !f.PushByDigest && f.SBOMFile == "" {
		return fmt.Errorf("--sbom requires --push, --push-by-digest, or --sbom-file")
	}
	if f.SBOM && loadFor(f) {
		return fmt.Errorf("--sbom cannot be used with --load")
	}
	if f.PushByDigest && f.Tags != "" {
		return fmt.Errorf("--push-by-digest cannot be used with --tags")
	}
	if f.PushByDigest && f.DigestFile == "" {
		return fmt.Errorf("--push-by-digest requires --digest-file")
	}
	if f.CheckWarningsAsErrors && !f.Check {
		return fmt.Errorf("--check-warnings-as-errors requires --check")
	}
	return nil
}

// buildArgs assembles the argv for docker buildx build.
func buildArgs(f Flags, p plan) []string {
	args := []string{
		"buildx", "build",
		"--builder", p.builder,
		"--progress=plain",
	}
	args = appendDockerfile(f, args)
	if !f.PushByDigest {
		for _, r := range p.refs {
			args = append(args, "-t", r)
		}
	}
	if f.Platforms != "" {
		args = append(args, "--platform", f.Platforms)
	}
	if f.Target != "" {
		args = append(args, "--target", f.Target)
	}
	args = appendBuildInputArgs(f, args)
	args = appendCacheArgs(f, args)
	args = appendAttestArgs(f, p, args)
	args = appendOutputArgs(f, p, args)
	args = append(args, "--metadata-file", p.metadataFile)
	extra, _ := splitShellWords(f.ExtraArgs)
	args = append(args, extra...)
	args = append(args, f.Context)
	return args
}

// appendBuildInputArgs appends --build-arg, --label, --secret, --pull, --no-cache.
func appendDockerfile(f Flags, args []string) []string {
	if f.Dockerfile == "" {
		return args
	}
	return append(args, "-f", f.Dockerfile)
}

func appendBuildInputArgs(f Flags, args []string) []string {
	for _, line := range splitLines(f.BuildArgs) {
		args = append(args, "--build-arg", line)
	}
	for _, line := range splitLines(f.Labels) {
		args = append(args, "--label", line)
	}
	for _, sec := range splitCommaNewline(f.SecretEnvs) {
		id, env := parseSecretEnv(sec)
		args = append(args, "--secret", "id="+id+",env="+env)
	}
	if f.Pull {
		args = append(args, "--pull")
	}
	if f.NoCache {
		args = append(args, "--no-cache")
	}
	return args
}

// appendCacheArgs appends --cache-from and --cache-to entries.
func appendCacheArgs(f Flags, args []string) []string {
	for _, entry := range strings.Fields(f.CacheFrom) {
		args = append(args, "--cache-from", entry)
	}
	for _, entry := range strings.Fields(f.CacheTo) {
		args = append(args, "--cache-to", entry)
	}
	return args
}

// appendAttestArgs appends --provenance and --sbom.
func appendAttestArgs(f Flags, p plan, args []string) []string {
	if !f.Provenance {
		args = append(args, "--provenance=false")
	}
	if f.SBOM {
		args = append(args, "--sbom=generator="+p.sbomGen)
	}
	return args
}

// appendOutputArgs appends the output specification (--push, --output, or --load).
func appendOutputArgs(f Flags, p plan, args []string) []string {
	switch {
	case f.PushByDigest:
		ref := digestRef(p.refs)
		args = append(args, "--output",
			"type=image,name="+ref+",push-by-digest=true,name-canonical=true,push=true")
	case f.Push:
		args = append(args, "--push")
	case p.load:
		args = append(args, "--load")
	}
	return args
}

// digestRef strips the tag from the first ref for push-by-digest output.
func digestRef(refs []string) string {
	if len(refs) == 0 {
		return ""
	}
	ref := refs[0]
	if i := strings.LastIndexByte(ref, ':'); i >= 0 {
		return ref[:i]
	}
	return ref
}

// splitLines returns non-empty lines from s.
func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// splitCommaNewline splits on commas and newlines, trimming spaces, dropping empties.
func splitCommaNewline(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parseSecretEnv parses a secret spec "ID=ENV" or "ENV" (ID equals ENV).
func parseSecretEnv(s string) (id, env string) {
	if i := strings.IndexByte(s, '='); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, s
}

// shellParser holds state for splitShellWords.
type shellParser struct {
	cur      strings.Builder
	inSingle bool
	inDouble bool
	escape   bool
}

func (p *shellParser) consumeByte(c byte) (flush bool) {
	if p.escape {
		p.cur.WriteByte(c)
		p.escape = false
		return false
	}
	switch {
	case c == '\\' && !p.inSingle:
		p.escape = true
	case c == '\'' && !p.inDouble:
		p.inSingle = !p.inSingle
	case c == '"' && !p.inSingle:
		p.inDouble = !p.inDouble
	case isShellSpace(c) && !p.inSingle && !p.inDouble:
		return true
	default:
		p.cur.WriteByte(c)
	}
	return false
}

func (p *shellParser) endError() error {
	switch {
	case p.inSingle:
		return fmt.Errorf("unterminated single quote in extra-args")
	case p.inDouble:
		return fmt.Errorf("unterminated double quote in extra-args")
	case p.escape:
		return fmt.Errorf("trailing backslash in extra-args")
	}
	return nil
}

func isShellSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n'
}

// splitShellWords splits s like a POSIX shell, supporting single/double quotes
// and backslash escapes. It never evaluates variables or runs subshells.
func splitShellWords(s string) ([]string, error) {
	var words []string
	p := &shellParser{}
	for i := 0; i < len(s); i++ {
		if p.consumeByte(s[i]) && p.cur.Len() > 0 {
			words = append(words, p.cur.String())
			p.cur.Reset()
		}
	}
	if err := p.endError(); err != nil {
		return nil, err
	}
	if p.cur.Len() > 0 {
		words = append(words, p.cur.String())
	}
	return words, nil
}
