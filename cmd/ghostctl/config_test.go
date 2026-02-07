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
mirage_security_mode = "production"
mirage_tls_enabled = true
mirage_tls_mutual = true
mirage_tls_cert_file = "/etc/ghost/client.crt"
mirage_tls_key_file = "/etc/ghost/client.key"
mirage_tls_ca_file = "/etc/ghost/ca.crt"
mirage_tls_server_name = "mirage.local"
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
	if cfg.Mirage.SessionConfig.SecurityMode != "production" {
		t.Fatalf("unexpected security mode: %q", cfg.Mirage.SessionConfig.SecurityMode)
	}
	if !cfg.Mirage.SessionConfig.TLS.Enabled {
		t.Fatalf("expected tls enabled")
	}
	if !cfg.Mirage.SessionConfig.TLS.Mutual {
		t.Fatalf("expected mtls enabled")
	}
	if cfg.Mirage.SessionConfig.TLS.CertFile != "/etc/ghost/client.crt" {
		t.Fatalf("unexpected cert file: %q", cfg.Mirage.SessionConfig.TLS.CertFile)
	}
	if cfg.Mirage.SessionConfig.TLS.KeyFile != "/etc/ghost/client.key" {
		t.Fatalf("unexpected key file: %q", cfg.Mirage.SessionConfig.TLS.KeyFile)
	}
	if cfg.Mirage.SessionConfig.TLS.CAFile != "/etc/ghost/ca.crt" {
		t.Fatalf("unexpected ca file: %q", cfg.Mirage.SessionConfig.TLS.CAFile)
	}
	if cfg.Mirage.SessionConfig.TLS.ServerName != "mirage.local" {
		t.Fatalf("unexpected server name: %q", cfg.Mirage.SessionConfig.TLS.ServerName)
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
