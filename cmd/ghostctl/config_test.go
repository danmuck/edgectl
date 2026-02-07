package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadServiceConfigDefaultsAndOverrides(t *testing.T) {
	root := resolveWorkspaceRoot("cmd/ghostctl/ex.config.toml")
	path := filepath.Join(root, "cmd", "ghostctl", "ex.config.toml")

	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.GhostID != "ghost.local" {
		t.Fatalf("unexpected id: %q", cfg.GhostID)
	}
	if !cfg.ProjectFetchOnBoot {
		t.Fatalf("expected project fetch on boot enabled")
	}
	if cfg.HeartbeatInterval != 5*time.Second {
		t.Fatalf("unexpected heartbeat: %v", cfg.HeartbeatInterval)
	}
	if cfg.AdminListenAddr != "127.0.0.1:7010" {
		t.Fatalf("unexpected admin listen: %q", cfg.AdminListenAddr)
	}
	if len(cfg.BuiltinSeedIDs) != 2 {
		t.Fatalf("unexpected seeds: %+v", cfg.BuiltinSeedIDs)
	}
	if cfg.Mirage.Policy != "headless" {
		t.Fatalf("unexpected mirage policy: %q", cfg.Mirage.Policy)
	}
	if cfg.Mirage.Address != "127.0.0.1:9000" {
		t.Fatalf("unexpected mirage address: %q", cfg.Mirage.Address)
	}
	if cfg.Mirage.PeerIdentity != "ghost.local" {
		t.Fatalf("unexpected mirage peer identity: %q", cfg.Mirage.PeerIdentity)
	}
	if cfg.Mirage.MaxConnectAttempts != 0 {
		t.Fatalf("unexpected max connect attempts: %d", cfg.Mirage.MaxConnectAttempts)
	}
	if cfg.Mirage.SessionConfig.SecurityMode != "development" {
		t.Fatalf("unexpected security mode: %q", cfg.Mirage.SessionConfig.SecurityMode)
	}
	if cfg.Mirage.SessionConfig.TLS.Enabled {
		t.Fatalf("expected tls disabled")
	}
	if cfg.Mirage.SessionConfig.TLS.Mutual {
		t.Fatalf("expected mtls disabled")
	}
	if cfg.Mirage.SessionConfig.TLS.CertFile != "" {
		t.Fatalf("unexpected cert file: %q", cfg.Mirage.SessionConfig.TLS.CertFile)
	}
	if cfg.Mirage.SessionConfig.TLS.KeyFile != "" {
		t.Fatalf("unexpected key file: %q", cfg.Mirage.SessionConfig.TLS.KeyFile)
	}
	if cfg.Mirage.SessionConfig.TLS.CAFile != "" {
		t.Fatalf("unexpected ca file: %q", cfg.Mirage.SessionConfig.TLS.CAFile)
	}
	if cfg.Mirage.SessionConfig.TLS.ServerName != "" {
		t.Fatalf("unexpected server name: %q", cfg.Mirage.SessionConfig.TLS.ServerName)
	}
	if !cfg.SeedInstall.Enabled {
		t.Fatalf("expected seed install enabled")
	}
	if cfg.SeedInstall.InstallRoot != "local/seeds" {
		t.Fatalf("unexpected seed install root: %q", cfg.SeedInstall.InstallRoot)
	}
	if len(cfg.SeedInstall.Whitelist) != 2 || cfg.SeedInstall.Whitelist[0] != "seed.mongod" || cfg.SeedInstall.Whitelist[1] != "seed.mongod.pkg" {
		t.Fatalf("unexpected seed install whitelist: %+v", cfg.SeedInstall.Whitelist)
	}
	if len(cfg.SeedInstall.Specs) != 2 {
		t.Fatalf("unexpected seed install spec count: %d", len(cfg.SeedInstall.Specs))
	}
	if cfg.SeedInstall.Specs[0].SeedID != "seed.mongod" {
		t.Fatalf("unexpected seed install id: %q", cfg.SeedInstall.Specs[0].SeedID)
	}
	if cfg.SeedInstall.Specs[0].Method != "github" {
		t.Fatalf("unexpected seed install method: %q", cfg.SeedInstall.Specs[0].Method)
	}
	if cfg.SeedInstall.Specs[1].SeedID != "seed.mongod.pkg" {
		t.Fatalf("unexpected seed install id: %q", cfg.SeedInstall.Specs[1].SeedID)
	}
	if cfg.SeedInstall.Specs[1].Method != "brew" {
		t.Fatalf("unexpected seed install method: %q", cfg.SeedInstall.Specs[1].Method)
	}
	if cfg.SeedInstall.Specs[1].Package != "mongodb-community@7.0" {
		t.Fatalf("unexpected seed install package: %q", cfg.SeedInstall.Specs[1].Package)
	}
	if cfg.SeedInstall.Specs[1].Tap != "mongodb/brew" {
		t.Fatalf("unexpected seed install tap: %q", cfg.SeedInstall.Specs[1].Tap)
	}
	if !cfg.SeedInstall.Specs[1].BootstrapIfMissing {
		t.Fatalf("expected bootstrap_if_missing enabled")
	}
	if len(cfg.SeedInstall.Specs[1].BootstrapCommand) != 3 {
		t.Fatalf("unexpected bootstrap command: %+v", cfg.SeedInstall.Specs[1].BootstrapCommand)
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

func TestLoadServiceConfigProjectFetchOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
project_fetch_on_boot = false
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ProjectFetchOnBoot {
		t.Fatalf("expected project fetch on boot disabled")
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

func TestParseSeedInstallSpecsMissingSeedID(t *testing.T) {
	_, err := parseSeedInstallSpecs([]fileSeedInstall{{
		Method: "github",
		Repo:   "https://github.com/example/repo.git",
	}})
	if err == nil {
		t.Fatalf("expected parse error")
	}
}
