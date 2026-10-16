package login

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func withDockerConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DOCKER_CONFIG", dir)
	return filepath.Join(dir, "config.json")
}

func TestDockerConfig_CreatesFile(t *testing.T) {
	path := withDockerConfig(t)

	c, err := readDockerConfig()
	assert.NilError(t, err)
	assert.NilError(t, c.setCredHelper("ghcr.io", "myhelper"))
	assert.NilError(t, c.write())

	data, err := os.ReadFile(path)
	assert.NilError(t, err)

	var m map[string]json.RawMessage
	assert.NilError(t, json.Unmarshal(data, &m))
	assert.Assert(t, cmp.Contains(string(m["credHelpers"]), "ghcr.io"))

	info, err := os.Stat(path)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(info.Mode().Perm(), os.FileMode(0o600)))
}

func TestDockerConfig_PreservesKeys(t *testing.T) {
	withDockerConfig(t)

	initial := `{"auths":{"docker.io":{}},"experimental":"enabled"}`
	assert.NilError(t, os.WriteFile(dockerConfigPath(), []byte(initial), 0o600))

	c, err := readDockerConfig()
	assert.NilError(t, err)
	assert.NilError(t, c.setCredHelper("registry.example.com", "ecr-login"))
	assert.NilError(t, c.write())

	data, err := os.ReadFile(dockerConfigPath())
	assert.NilError(t, err)
	assert.Assert(t, cmp.Contains(string(data), `"experimental"`), "should preserve unknown keys")
	assert.Assert(t, cmp.Contains(string(data), `"auths"`), "should preserve auths")
	assert.Assert(t, cmp.Contains(string(data), `"registry.example.com"`))
}

func TestDockerConfig_SetsCredsStore(t *testing.T) {
	withDockerConfig(t)

	c, err := readDockerConfig()
	assert.NilError(t, err)
	assert.NilError(t, c.setCredsStore("pass"))
	assert.NilError(t, c.write())

	data, err := os.ReadFile(dockerConfigPath())
	assert.NilError(t, err)
	assert.Assert(t, cmp.Contains(string(data), `"credsStore"`))
}

func TestDockerConfig_MergesCredHelpers(t *testing.T) {
	withDockerConfig(t)

	existing := `{"credHelpers":{"old.example.com":"old-helper"}}`
	assert.NilError(t, os.WriteFile(dockerConfigPath(), []byte(existing), 0o600))

	c, err := readDockerConfig()
	assert.NilError(t, err)
	assert.NilError(t, c.setCredHelper("new.example.com", "new-helper"))
	assert.NilError(t, c.write())

	data, err := os.ReadFile(dockerConfigPath())
	assert.NilError(t, err)
	assert.Assert(t, cmp.Contains(string(data), `"old.example.com"`), "should keep existing entry")
	assert.Assert(t, cmp.Contains(string(data), `"new.example.com"`), "should add new entry")
}
