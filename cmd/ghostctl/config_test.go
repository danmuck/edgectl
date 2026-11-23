package main

import (
	"os"
	"path/filepath"
	"strings"
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
	if cfg.AdminListenAddr != "127.0.0.1:7011" {
		t.Fatalf("unexpected admin listen: %q", cfg.AdminListenAddr)
	}
	if len(cfg.BuiltinSeedIDs) != 4 {
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
	if len(cfg.SeedInstall.Whitelist) != 2 || cfg.SeedInstall.Whitelist[0] != "seed.docker" || cfg.SeedInstall.Whitelist[1] != "seed.mongod" {
		t.Fatalf("unexpected seed install whitelist: %+v", cfg.SeedInstall.Whitelist)
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

// Ghost config may not carry install specs; installs come only from Mirage-approved proposals.
func TestLoadServiceConfigRejectsSeedInstallSpecs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := "id = \"ghost.alpha\"\n[[seed_install]]\nseed_id = \"seed.docker\"\nmethod = \"brew\"\npackage = \"docker\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := loadServiceConfig(path)
	if err == nil || !strings.Contains(err.Error(), "Mirage-approved proposal") {
		t.Fatalf("expected seed_install rejection, got %v", err)
	}
}

func TestResolveWorkspaceRootFallsBackToConfigDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("id = \"ghost.test\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	got := resolveWorkspaceRoot(cfgPath)
	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs dir: %v", err)
	}
	if got != want {
		t.Fatalf("unexpected workspace root fallback: got=%q want=%q", got, want)
	}
}

func TestLoadServiceConfigAdminFrameAuthToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("admin_frame_auth_token = \" s3cret \"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.AdminFrameAuthToken != "s3cret" {
		t.Fatalf("admin_frame_auth_token = %q", cfg.AdminFrameAuthToken)
	}
}
