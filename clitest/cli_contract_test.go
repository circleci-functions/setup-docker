package setupdocker_test

import (
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v4"
	"gotest.tools/v3/assert"

	"github.com/circleci-functions/toolkit/functionyaml"
)

func TestDescribe(t *testing.T) {
	stdout, _, code := run(t, "--describe")
	assert.Equal(t, code, 0)

	var d functionyaml.Descriptor
	assert.NilError(t, yaml.Unmarshal([]byte(stdout), &d))
	assert.Equal(t, d.Name, "setup-docker")

	byName := map[string]functionyaml.Flag{}
	for _, f := range d.Flags {
		byName[f.Name] = f
	}

	type want struct {
		typ  string
		def  string
		enum []string
	}
	expected := map[string]want{
		"install-docker":                {typ: "bool", def: "true"},
		"install-docker-compose":        {typ: "bool", def: "true"},
		"install-dockerize":             {typ: "bool", def: "true"},
		"install-goss":                  {typ: "bool", def: "true"},
		"install-credential-helper":     {typ: "bool", def: "false"},
		"version":                       {typ: "string", def: "latest"},
		"install-dir":                   {typ: "string", def: "/usr/local/bin"},
		"docker-compose-version":        {typ: "string", def: "latest"},
		"docker-compose-install-dir":    {typ: "string", def: "/usr/local/bin"},
		"dockerize-version":             {typ: "string", def: "latest"},
		"dockerize-install-dir":         {typ: "string", def: "/usr/local/bin"},
		"goss-version":                  {typ: "string", def: "latest"},
		"goss-install-dir":              {typ: "string", def: "/usr/local/bin"},
		"goss-architecture":             {typ: "string", def: "amd64", enum: []string{"amd64", "arm64"}},
		"credential-helper-name":        {typ: "string", enum: []string{"", "pass", "osxkeychain"}},
		"credential-helper-arch":        {typ: "string", def: "amd64", enum: []string{"amd64", "arm64"}},
		"credential-helper-release-tag": {typ: "string"},
	}

	for name, w := range expected {
		t.Run(name, func(t *testing.T) {
			f, ok := byName[name]
			assert.Assert(t, ok, "flag %s missing from descriptor", name)
			assert.Equal(t, f.Type, w.typ)
			assert.Equal(t, f.Default, w.def)
			if w.enum != nil {
				assert.DeepEqual(t, f.Enum, w.enum)
			}
		})
	}
}

func TestHelp(t *testing.T) {
	stdout, _, code := run(t, "--help")
	assert.Equal(t, code, 0)
	assert.Assert(t, strings.Contains(stdout, "Usage: setup-docker"))
}

func TestInvalidEnum(t *testing.T) {
	_, stderr, code := run(t, "--goss-architecture=ppc64")
	assert.Assert(t, code != 0)
	assert.Assert(t, strings.Contains(stderr, "goss-architecture"))
}

func TestUnknownFlag(t *testing.T) {
	_, _, code := run(t, "--definitely-not-a-flag")
	assert.Assert(t, code != 0)
}
