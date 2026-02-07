package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

// miragectl config.toml key mapping to Mirage runtime settings.
type fileConfig struct {
	Addr                string `toml:"addr"`
	ID                  string `toml:"id"`
	AdminListenAddr     string `toml:"admin_listen_addr"`
	RequireIdentityBind bool   `toml:"require_identity_binding"`
	RootGhostAdminAddr  string `toml:"root_ghost_admin_addr"`
	GhostConfigPath     string `toml:"ghost_config_path"`
	LocalGhostID        string `toml:"local_ghost_id"`
	LocalGhostAdminAddr string `toml:"local_ghost_admin_addr"`
	BuildlogPersist     bool   `toml:"buildlog_persist_enabled"`
	BuildlogSeed        string `toml:"buildlog_seed_selector"`
	BuildlogKeyPrefix   string `toml:"buildlog_key_prefix"`
	SessionSecurityMode string `toml:"session_security_mode"`
	SessionTLSEnabled   bool   `toml:"session_tls_enabled"`
	SessionTLSMutual    bool   `toml:"session_tls_mutual"`
	SessionTLSCertFile  string `toml:"session_tls_cert_file"`
	SessionTLSKeyFile   string `toml:"session_tls_key_file"`
	SessionTLSCAFile    string `toml:"session_tls_ca_file"`
}

// ghostFileConfig is the subset of ghost.toml required by mirage admin controller.
type ghostFileConfig struct {
	ID          string   `toml:"id"`
	AdminListen string   `toml:"admin_listen"`
	Seeds       []string `toml:"seeds"`
}

// miragectl loader for TOML config with default overlay.
func loadServiceConfig(path string) (mirage.ServiceConfig, error) {
	cfg := mirage.DefaultServiceConfig()

	var raw fileConfig
	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return mirage.ServiceConfig{}, fmt.Errorf("load mirage config: %w", err)
	}

	if meta.IsDefined("addr") {
		cfg.ListenAddr = strings.TrimSpace(raw.Addr)
	}
	if meta.IsDefined("id") {
		cfg.MirageID = strings.TrimSpace(raw.ID)
	}
	if meta.IsDefined("admin_listen_addr") {
		cfg.AdminListenAddr = strings.TrimSpace(raw.AdminListenAddr)
	}
	if meta.IsDefined("require_identity_binding") {
		cfg.RequireIdentityBinding = raw.RequireIdentityBind
	}
	if meta.IsDefined("root_ghost_admin_addr") {
		cfg.RootGhostAdminAddr = strings.TrimSpace(raw.RootGhostAdminAddr)
	}
	if meta.IsDefined("local_ghost_id") {
		cfg.LocalGhostID = strings.TrimSpace(raw.LocalGhostID)
	}
	if meta.IsDefined("local_ghost_admin_addr") {
		cfg.LocalGhostAdminAddr = strings.TrimSpace(raw.LocalGhostAdminAddr)
	}
	if meta.IsDefined("buildlog_persist_enabled") {
		cfg.BuildlogPersistEnabled = raw.BuildlogPersist
	}
	if meta.IsDefined("buildlog_seed_selector") {
		cfg.BuildlogSeedSelector = strings.TrimSpace(raw.BuildlogSeed)
	}
	if meta.IsDefined("buildlog_key_prefix") {
		cfg.BuildlogKeyPrefix = strings.TrimSpace(raw.BuildlogKeyPrefix)
	}
	if meta.IsDefined("session_security_mode") {
		cfg.Session.SecurityMode = session.SecurityMode(strings.TrimSpace(raw.SessionSecurityMode))
	}
	if meta.IsDefined("session_tls_enabled") {
		cfg.Session.TLS.Enabled = raw.SessionTLSEnabled
	}
	if meta.IsDefined("session_tls_mutual") {
		cfg.Session.TLS.Mutual = raw.SessionTLSMutual
	}
	if meta.IsDefined("session_tls_cert_file") {
		cfg.Session.TLS.CertFile = strings.TrimSpace(raw.SessionTLSCertFile)
	}
	if meta.IsDefined("session_tls_key_file") {
		cfg.Session.TLS.KeyFile = strings.TrimSpace(raw.SessionTLSKeyFile)
	}
	if meta.IsDefined("session_tls_ca_file") {
		cfg.Session.TLS.CAFile = strings.TrimSpace(raw.SessionTLSCAFile)
	}

	if cfg.BuildlogPersistEnabled {
		selector := strings.TrimSpace(cfg.BuildlogSeedSelector)
		if selector == "" {
			selector = "seed.fs"
			cfg.BuildlogSeedSelector = selector
		}
		if selector != "seed.fs" && selector != "seed.kv" {
			return mirage.ServiceConfig{}, fmt.Errorf(
				"load mirage config: unsupported buildlog seed selector %q (expected seed.fs or seed.kv)",
				selector,
			)
		}
	}

	ghostPath := strings.TrimSpace(raw.GhostConfigPath)
	if ghostPath != "" {
		ghostCfg, err := loadGhostRuntimeConfig(path, ghostPath)
		if err != nil {
			return mirage.ServiceConfig{}, err
		}
		if cfg.LocalGhostID == "" {
			cfg.LocalGhostID = strings.TrimSpace(ghostCfg.ID)
		}
		if cfg.LocalGhostAdminAddr == "" {
			cfg.LocalGhostAdminAddr = strings.TrimSpace(ghostCfg.AdminListen)
		}
		if cfg.RootGhostAdminAddr == "" {
			cfg.RootGhostAdminAddr = cfg.LocalGhostAdminAddr
		}
		if cfg.BuildlogPersistEnabled && !hasSeed(ghostCfg.Seeds, cfg.BuildlogSeedSelector) {
			return mirage.ServiceConfig{}, fmt.Errorf(
				"load mirage config: ghost config %q must include %s when buildlog_persist_enabled=true",
				ghostPath,
				cfg.BuildlogSeedSelector,
			)
		}
	}

	if strings.TrimSpace(cfg.AdminListenAddr) != "" {
		if strings.TrimSpace(raw.GhostConfigPath) == "" {
			return mirage.ServiceConfig{}, fmt.Errorf(
				"load mirage config: ghost_config_path is required when admin_listen_addr is set",
			)
		}
		if strings.TrimSpace(cfg.LocalGhostID) == "" {
			return mirage.ServiceConfig{}, fmt.Errorf(
				"load mirage config: local ghost id is required for mirage admin controller",
			)
		}
		if strings.TrimSpace(cfg.LocalGhostAdminAddr) == "" {
			return mirage.ServiceConfig{}, fmt.Errorf(
				"load mirage config: local ghost admin addr is required for mirage admin controller",
			)
		}
	}

	cfg.Session = cfg.Session.WithDefaults()
	return cfg, nil
}

func loadGhostRuntimeConfig(mirageConfigPath string, ghostConfigPath string) (ghostFileConfig, error) {
	resolved := strings.TrimSpace(ghostConfigPath)
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(mirageConfigPath), resolved)
	}
	if _, err := os.Stat(resolved); err != nil {
		return ghostFileConfig{}, fmt.Errorf(
			"load mirage config: ghost config path %q: %w",
			ghostConfigPath,
			err,
		)
	}
	var out ghostFileConfig
	if _, err := toml.DecodeFile(resolved, &out); err != nil {
		return ghostFileConfig{}, fmt.Errorf(
			"load mirage config: parse ghost config %q: %w",
			ghostConfigPath,
			err,
		)
	}
	return out, nil
}

func hasSeed(seeds []string, seedID string) bool {
	target := strings.TrimSpace(seedID)
	for _, raw := range seeds {
		if strings.TrimSpace(raw) == target {
			return true
		}
	}
	return false
}
