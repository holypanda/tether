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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaults(), nil
	}
	if err != nil {
		return nil, err
	}
	cfg := defaults()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
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
