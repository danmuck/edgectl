package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadServiceConfigDefaultsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
id = "ghost.alpha"
seeds = ["seed.flow", "none", "flow"]
heartbeat_interval = "750ms"
mirage_policy = "auto"
mirage_address = "127.0.0.1:9000"
mirage_peer_identity = "ghost.alpha"
mirage_max_connect_attempts = 3
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.GhostID != "ghost.alpha" {
		t.Fatalf("unexpected id: %q", cfg.GhostID)
	}
	if cfg.HeartbeatInterval != 750*time.Millisecond {
		t.Fatalf("unexpected heartbeat: %v", cfg.HeartbeatInterval)
	}
	if len(cfg.BuiltinSeedIDs) != 3 {
		t.Fatalf("unexpected seeds: %+v", cfg.BuiltinSeedIDs)
	}
	if cfg.Mirage.Policy != "auto" {
		t.Fatalf("unexpected mirage policy: %q", cfg.Mirage.Policy)
	}
	if cfg.Mirage.Address != "127.0.0.1:9000" {
		t.Fatalf("unexpected mirage address: %q", cfg.Mirage.Address)
	}
	if cfg.Mirage.PeerIdentity != "ghost.alpha" {
		t.Fatalf("unexpected mirage peer identity: %q", cfg.Mirage.PeerIdentity)
	}
	if cfg.Mirage.MaxConnectAttempts != 3 {
		t.Fatalf("unexpected max connect attempts: %d", cfg.Mirage.MaxConnectAttempts)
	}
}

func TestLoadServiceConfigHeartbeatMillis(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
heartbeat_interval_ms = 1200
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.HeartbeatInterval != 1200*time.Millisecond {
		t.Fatalf("unexpected heartbeat: %v", cfg.HeartbeatInterval)
	}
}

func TestLoadServiceConfigBadDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
heartbeat_interval = "abc"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := loadServiceConfig(path); err == nil {
		t.Fatalf("expected parse error")
	}
}
