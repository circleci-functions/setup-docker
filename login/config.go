package login

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// dockerConfig is a parsed Docker config.json that preserves unknown keys.
type dockerConfig struct {
	data map[string]json.RawMessage
}

func readDockerConfig() (*dockerConfig, error) {
	data, err := os.ReadFile(dockerConfigPath())
	if os.IsNotExist(err) {
		return &dockerConfig{data: map[string]json.RawMessage{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &dockerConfig{data: m}, nil
}

func (c *dockerConfig) setCredsStore(helper string) error {
	b, err := json.Marshal(helper)
	if err != nil {
		return err
	}
	c.data["credsStore"] = json.RawMessage(b)
	return nil
}

func (c *dockerConfig) setCredHelper(registry, helper string) error {
	var m map[string]string
	if raw, ok := c.data["credHelpers"]; ok {
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
	} else {
		m = map[string]string{}
	}
	m[registry] = helper
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.data["credHelpers"] = json.RawMessage(b)
	return nil
}

func (c *dockerConfig) write() error {
	p := dockerConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

func dockerConfigPath() string {
	if dc := os.Getenv("DOCKER_CONFIG"); dc != "" {
		return filepath.Join(dc, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".docker", "config.json")
	}
	return filepath.Join(home, ".docker", "config.json")
}
