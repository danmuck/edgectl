package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServiceConfigDefaultsAndOverrides(t *testing.T) {
	dir := t.TempDir()

	path := filepath.Join(dir, "config.toml")
	content := `
id = "mirage.alpha"
addr = "127.0.0.1:9443"
admin_listen_addr = "127.0.0.1:7020"
require_identity_binding = true
buildlog_persist_enabled = true
buildlog_seed_selector = "seed.kv"
buildlog_key_prefix = "local/buildlogs/"
session_security_mode = "production"
session_tls_enabled = true
session_tls_mutual = true
session_tls_cert_file = "/etc/mirage/server.crt"
session_tls_key_file = "/etc/mirage/server.key"
session_tls_ca_file = "/etc/mirage/ca.crt"
[[preload_ghost_admins]]
ghost_id = "ghost.remote.a"
admin_addr = "localhost:7011"
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
	if len(cfg.PreloadGhostAdmins) != 1 {
		t.Fatalf("expected one preload ghost admin, got %d", len(cfg.PreloadGhostAdmins))
	}
	if cfg.PreloadGhostAdmins[0].GhostID != "ghost.remote.a" {
		t.Fatalf("unexpected preload ghost id: %q", cfg.PreloadGhostAdmins[0].GhostID)
	}
	if cfg.PreloadGhostAdmins[0].AdminAddr != "localhost:7011" {
		t.Fatalf("unexpected preload admin addr: %q", cfg.PreloadGhostAdmins[0].AdminAddr)
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
	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.LocalGhostAdminAddr != "127.0.0.1:7010" {
		t.Fatalf("unexpected default local ghost admin addr: %q", cfg.LocalGhostAdminAddr)
	}
}

func TestLoadServiceConfigBuildlogAllowsSeedFS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
admin_listen_addr = "127.0.0.1:7020"
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

func TestLoadRuntimeConfigsIncludesManagedLocalGhost(t *testing.T) {
	dir := t.TempDir()
	miragePath := filepath.Join(dir, "mirage.toml")
	if err := os.WriteFile(miragePath, []byte(`
id = "mirage.alpha"
admin_listen_addr = "127.0.0.1:7020"
local_ghost_id = "ghost.local.managed"
local_ghost_admin_addr = "127.0.0.1:7010"
local_ghost_seeds = ["seed.flow","seed.fs"]
local_ghost_heartbeat_interval_ms = 1250
local_ghost_project_fetch_on_boot = false
`), 0o644); err != nil {
		t.Fatalf("write mirage config: %v", err)
	}

	mCfg, gCfg, err := loadRuntimeConfigs(miragePath)
	if err != nil {
		t.Fatalf("load runtime configs: %v", err)
	}
	if mCfg.LocalGhostID != "ghost.local.managed" {
		t.Fatalf("unexpected mirage local ghost id: %q", mCfg.LocalGhostID)
	}
	if gCfg.GhostID != "ghost.local.managed" {
		t.Fatalf("unexpected managed ghost id: %q", gCfg.GhostID)
	}
	if gCfg.AdminListenAddr != "127.0.0.1:7010" {
		t.Fatalf("unexpected managed ghost admin addr: %q", gCfg.AdminListenAddr)
	}
	if gCfg.Mirage.Policy != "headless" {
		t.Fatalf("expected managed ghost mirage policy=headless, got %q", gCfg.Mirage.Policy)
	}
	if len(gCfg.BuiltinSeedIDs) != 2 || gCfg.BuiltinSeedIDs[0] != "seed.flow" || gCfg.BuiltinSeedIDs[1] != "seed.fs" {
		t.Fatalf("unexpected managed ghost seeds: %+v", gCfg.BuiltinSeedIDs)
	}
	if gCfg.HeartbeatInterval.Milliseconds() != 1250 {
		t.Fatalf("unexpected managed ghost heartbeat interval: %v", gCfg.HeartbeatInterval)
	}
	if gCfg.ProjectFetchOnBoot {
		t.Fatalf("expected managed ghost project fetch disabled")
	}
}

func TestLoadServiceConfigPreloadGhostAdminsRequireFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`
admin_listen_addr = "127.0.0.1:7020"
[[preload_ghost_admins]]
ghost_id = ""
admin_addr = "127.0.0.1:7012"
	`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := loadServiceConfig(path)
	if err == nil {
		t.Fatalf("expected preload ghost validation error")
	}
}

func TestLoadServiceConfigGhostManifestSeedInstallPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[[ghost_manifests]]
ghost_id = "ghost.pi"
host = "pi.local"
seed_install_enabled = true
seed_install_root = "local/seeds"
seed_install_bin_root = "local/bin"
seed_install_allow_internal_defaults = true
seed_install_whitelist = ["seed.host", "seed.mongod", "seed.host"]
config_template_path = "cmd/ghostctl/pi.tls.config.toml"

[[ghost_manifests.seed_install]]
seed_id = "seed.mongod.pkg"
method = "brew"
package = "mongodb-community@7.0"
tap = "mongodb/brew"
bootstrap_if_missing = true
bootstrap_cmd = ["/bin/bash", "-c", "echo bootstrap"]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.GhostManifests) != 1 {
		t.Fatalf("unexpected ghost manifest count: %d", len(cfg.GhostManifests))
	}
	m := cfg.GhostManifests[0]
	if !m.SeedInstallEnabled {
		t.Fatalf("expected seed_install_enabled")
	}
	if m.SeedInstallRoot != "local/seeds" {
		t.Fatalf("unexpected seed install root: %q", m.SeedInstallRoot)
	}
	if m.SeedInstallBinRoot != "local/bin" {
		t.Fatalf("unexpected seed install bin root: %q", m.SeedInstallBinRoot)
	}
	if !m.SeedInstallAllowInternalDefaults {
		t.Fatalf("expected seed_install_allow_internal_defaults")
	}
	if len(m.SeedInstallWhitelist) != 3 || m.SeedInstallWhitelist[0] != "seed.host" || m.SeedInstallWhitelist[1] != "seed.mongod" || m.SeedInstallWhitelist[2] != "seed.host" {
		t.Fatalf("unexpected seed install whitelist: %+v", m.SeedInstallWhitelist)
	}
	if len(m.SeedInstall) != 1 {
		t.Fatalf("unexpected seed install spec count: %d", len(m.SeedInstall))
	}
	if m.SeedInstall[0].SeedID != "seed.mongod.pkg" || m.SeedInstall[0].Method != "brew" {
		t.Fatalf("unexpected seed install spec: %+v", m.SeedInstall[0])
	}
	if len(m.SeedInstall[0].BootstrapCmd) != 3 {
		t.Fatalf("unexpected bootstrap cmd: %+v", m.SeedInstall[0].BootstrapCmd)
	}
	if m.ConfigTemplatePath != "cmd/ghostctl/pi.tls.config.toml" {
		t.Fatalf("unexpected config template path: %q", m.ConfigTemplatePath)
	}
}
