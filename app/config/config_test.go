package config

import (
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	orig := &Config{
		VpsHost:          "1.2.3.4",
		VpsUser:          "root",
		VpsPort:          22,
		SharePath:        `D:\proj`,
		RemoteMountPoint: "~/local-code",
		RemoteTunnelPort: 2222,
		AdminKeyPath:     "./keys/admin_ed25519",
		Bootstrapped:     true,
	}
	if err := Save(path, orig); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if *loaded != *orig {
		t.Errorf("round trip mismatch:\n got  %+v\n want %+v", loaded, orig)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.json")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing should not err, got %v", err)
	}
	if cfg.Bootstrapped {
		t.Errorf("default should be not bootstrapped")
	}
	if cfg.VpsPort != 22 {
		t.Errorf("default VpsPort should be 22, got %d", cfg.VpsPort)
	}
}
