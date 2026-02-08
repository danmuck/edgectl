package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

// miragectl config.toml key mapping to Mirage runtime settings.
type fileConfig struct {
	Addr                         string              `toml:"addr"`
	ID                           string              `toml:"id"`
	AdminListenAddr              string              `toml:"admin_listen_addr"`
	RequireIdentityBind          bool                `toml:"require_identity_binding"`
	RootGhostAdminAddr           string              `toml:"root_ghost_admin_addr"`
	LocalGhostID                 string              `toml:"local_ghost_id"`
	LocalGhostAdminAddr          string              `toml:"local_ghost_admin_addr"`
	LocalGhostSeeds              []string            `toml:"local_ghost_seeds"`
	LocalGhostHeartbeat          string              `toml:"local_ghost_heartbeat_interval"`
	LocalGhostHeartbeatMS        int64               `toml:"local_ghost_heartbeat_interval_ms"`
	LocalGhostProjectRoot        string              `toml:"local_ghost_project_root"`
	LocalGhostProjectFetchOnBoot bool                `toml:"local_ghost_project_fetch_on_boot"`
	PreloadGhostAdmins           []preloadGhostAdmin `toml:"preload_ghost_admins"`
	BuildlogPersist              bool                `toml:"buildlog_persist_enabled"`
	BuildlogSeed                 string              `toml:"buildlog_seed_selector"`
	BuildlogKeyPrefix            string              `toml:"buildlog_key_prefix"`
	SessionSecurityMode          string              `toml:"session_security_mode"`
	SessionTLSEnabled            bool                `toml:"session_tls_enabled"`
	SessionTLSMutual             bool                `toml:"session_tls_mutual"`
	SessionTLSCertFile           string              `toml:"session_tls_cert_file"`
	SessionTLSKeyFile            string              `toml:"session_tls_key_file"`
	SessionTLSCAFile             string              `toml:"session_tls_ca_file"`
}

// preloadGhostAdmin maps one preload_ghost_admins TOML table row.
type preloadGhostAdmin struct {
	GhostID   string `toml:"ghost_id"`
	AdminAddr string `toml:"admin_addr"`
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
	if meta.IsDefined("preload_ghost_admins") {
		targets, normErr := normalizePreloadGhostAdmins(raw.PreloadGhostAdmins)
		if normErr != nil {
			return mirage.ServiceConfig{}, normErr
		}
		cfg.PreloadGhostAdmins = targets
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

	if cfg.RootGhostAdminAddr == "" {
		cfg.RootGhostAdminAddr = cfg.LocalGhostAdminAddr
	}

	if strings.TrimSpace(cfg.AdminListenAddr) != "" {
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

func normalizePreloadGhostAdmins(raw []preloadGhostAdmin) ([]mirage.GhostAdminTarget, error) {
	if len(raw) == 0 {
		return []mirage.GhostAdminTarget{}, nil
	}
	out := make([]mirage.GhostAdminTarget, 0, len(raw))
	for i := range raw {
		row := raw[i]
		id := strings.TrimSpace(row.GhostID)
		addr := strings.TrimSpace(row.AdminAddr)
		if id == "" {
			return nil, fmt.Errorf("load mirage config: preload_ghost_admins[%d].ghost_id required", i)
		}
		if addr == "" {
			return nil, fmt.Errorf("load mirage config: preload_ghost_admins[%d].admin_addr required", i)
		}
		out = append(out, mirage.GhostAdminTarget{
			GhostID:   id,
			AdminAddr: addr,
		})
	}
	return out, nil
}

// loadRuntimeConfigs loads mirage config and managed local-ghost config.
func loadRuntimeConfigs(path string) (mirage.ServiceConfig, ghost.ServiceConfig, error) {
	mCfg, err := loadServiceConfig(path)
	if err != nil {
		return mirage.ServiceConfig{}, ghost.ServiceConfig{}, err
	}
	var raw fileConfig
	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return mirage.ServiceConfig{}, ghost.ServiceConfig{}, fmt.Errorf("load mirage config: %w", err)
	}

	cfg := ghost.DefaultServiceConfig()
	cfg.Mirage.Policy = ghost.MiragePolicyHeadless
	cfg.GhostID = strings.TrimSpace(mCfg.LocalGhostID)
	if cfg.GhostID == "" {
		cfg.GhostID = ghost.DefaultServiceConfig().GhostID
	}
	cfg.AdminListenAddr = strings.TrimSpace(mCfg.LocalGhostAdminAddr)
	if cfg.AdminListenAddr == "" {
		cfg.AdminListenAddr = ghost.DefaultServiceConfig().AdminListenAddr
	}
	cfg.BuiltinSeedIDs = []string{"seed.flow", "seed.kv", "seed.fs", "seed.mongod"}
	if meta.IsDefined("local_ghost_seeds") {
		cfg.BuiltinSeedIDs = normalizeList(raw.LocalGhostSeeds)
	}
	if meta.IsDefined("local_ghost_project_root") {
		cfg.ProjectRoot = strings.TrimSpace(raw.LocalGhostProjectRoot)
	}
	if meta.IsDefined("local_ghost_project_fetch_on_boot") {
		cfg.ProjectFetchOnBoot = raw.LocalGhostProjectFetchOnBoot
	}
	if meta.IsDefined("local_ghost_heartbeat_interval") {
		d, err := time.ParseDuration(strings.TrimSpace(raw.LocalGhostHeartbeat))
		if err != nil {
			return mirage.ServiceConfig{}, ghost.ServiceConfig{}, fmt.Errorf(
				"parse local_ghost_heartbeat_interval: %w",
				err,
			)
		}
		cfg.HeartbeatInterval = d
	}
	if meta.IsDefined("local_ghost_heartbeat_interval_ms") {
		cfg.HeartbeatInterval = time.Duration(raw.LocalGhostHeartbeatMS) * time.Millisecond
	}
	return mCfg, cfg, nil
}

func normalizeList(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}
