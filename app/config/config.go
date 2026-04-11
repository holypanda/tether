package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

type Config struct {
	VpsHost          string `json:"vpsHost"`
	VpsUser          string `json:"vpsUser"`
	VpsPort          int    `json:"vpsPort"`
	SharePath        string `json:"sharePath"`
	RemoteMountPoint string `json:"remoteMountPoint"`
	RemoteTunnelPort int    `json:"remoteTunnelPort"`
	AdminKeyPath     string `json:"adminKeyPath"`
	Bootstrapped     bool   `json:"bootstrapped"`
}

func defaults() *Config {
	return &Config{
		VpsUser:          "root",
		VpsPort:          22,
		RemoteMountPoint: "~/local-code",
		RemoteTunnelPort: 2222,
		AdminKeyPath:     "./keys/admin_ed25519",
	}
}

// Load reads a stim-link config file. On any failure — missing file, permission
// error, malformed JSON — it returns a defaults()-initialised Config alongside
// the error so callers can still render the UI with sane values. The Config
// return is never nil.
func Load(path string) (*Config, error) {
	cfg := defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return defaults(), err
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
