package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServiceConfigDefaultsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	ghostPath := filepath.Join(dir, "ghost.toml")
	if err := os.WriteFile(ghostPath, []byte(`
id = "ghost.local"
admin_listen = "127.0.0.1:7010"
seeds = ["seed.flow","seed.kv"]
`), 0o644); err != nil {
		t.Fatalf("write ghost config: %v", err)
	}

	path := filepath.Join(dir, "config.toml")
	content := `
id = "mirage.alpha"
addr = "127.0.0.1:9443"
admin_listen_addr = "127.0.0.1:7020"
ghost_config_path = "ghost.toml"
require_identity_binding = true
buildlog_persist_enabled = true
buildlog_seed_selector = "seed.kv"
buildlog_key_prefix = "buildlog/"
session_security_mode = "production"
session_tls_enabled = true
session_tls_mutual = true
session_tls_cert_file = "/etc/mirage/server.crt"
session_tls_key_file = "/etc/mirage/server.key"
session_tls_ca_file = "/etc/mirage/ca.crt"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.MirageID != "mirage.alpha" {
		t.Fatalf("unexpected mirage id: %q", cfg.MirageID)
	}
	if cfg.ListenAddr != "127.0.0.1:9443" {
		t.Fatalf("unexpected listen addr: %q", cfg.ListenAddr)
	}
	if cfg.AdminListenAddr != "127.0.0.1:7020" {
		t.Fatalf("unexpected admin listen addr: %q", cfg.AdminListenAddr)
	}
	if cfg.LocalGhostID != "ghost.local" {
		t.Fatalf("unexpected local ghost id: %q", cfg.LocalGhostID)
	}
	if cfg.LocalGhostAdminAddr != "127.0.0.1:7010" {
		t.Fatalf("unexpected local ghost admin addr: %q", cfg.LocalGhostAdminAddr)
	}
	if !cfg.BuildlogPersistEnabled {
		t.Fatalf("expected buildlog persistence enabled")
	}
	if cfg.Session.SecurityMode != "production" {
		t.Fatalf("unexpected security mode: %q", cfg.Session.SecurityMode)
	}
}

func TestLoadServiceConfigAdminRequiresGhostConfigPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
admin_listen_addr = "127.0.0.1:7020"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := loadServiceConfig(path); err == nil {
		t.Fatalf("expected error when admin_listen_addr set without ghost_config_path")
	}
}

func TestLoadServiceConfigBuildlogRequiresSeedKV(t *testing.T) {
	dir := t.TempDir()
	ghostPath := filepath.Join(dir, "ghost.toml")
	if err := os.WriteFile(ghostPath, []byte(`
id = "ghost.local"
admin_listen = "127.0.0.1:7010"
seeds = ["seed.flow"]
`), 0o644); err != nil {
		t.Fatalf("write ghost config: %v", err)
	}

	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
admin_listen_addr = "127.0.0.1:7020"
ghost_config_path = "ghost.toml"
buildlog_persist_enabled = true
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := loadServiceConfig(path); err == nil {
		t.Fatalf("expected error when buildlog persistence enabled but seed.kv missing")
	}
}

func TestLoadServiceConfigBuildlogAllowsSeedFS(t *testing.T) {
	dir := t.TempDir()
	ghostPath := filepath.Join(dir, "ghost.toml")
	if err := os.WriteFile(ghostPath, []byte(`
id = "ghost.local"
admin_listen = "127.0.0.1:7010"
seeds = ["seed.flow","seed.fs"]
`), 0o644); err != nil {
		t.Fatalf("write ghost config: %v", err)
	}

	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
admin_listen_addr = "127.0.0.1:7020"
ghost_config_path = "ghost.toml"
buildlog_persist_enabled = true
buildlog_seed_selector = "seed.fs"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.BuildlogSeedSelector != "seed.fs" {
		t.Fatalf("unexpected buildlog seed selector: %q", cfg.BuildlogSeedSelector)
	}
}
