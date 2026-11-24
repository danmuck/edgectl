package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/tools"
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
	GhostAdminFrameAuthToken     string              `toml:"ghost_admin_frame_auth_token"`
	AdminTLSCertFile             string              `toml:"admin_tls_cert_file"`
	AdminTLSKeyFile              string              `toml:"admin_tls_key_file"`
	AdminTLSCAFile               string              `toml:"admin_tls_ca_file"`
	AdminTLSDeniedSerials        []string            `toml:"admin_tls_denied_serials"`
	LocalGhostAdminTLSCertFile   string              `toml:"local_ghost_admin_tls_cert_file"`
	LocalGhostAdminTLSKeyFile    string              `toml:"local_ghost_admin_tls_key_file"`
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
	SessionTLSDeniedSerials      []string            `toml:"session_tls_denied_serials"`
	GhostManifests               []ghostManifestFile `toml:"ghost_manifests"`
	CommandMaxAttempts           int                 `toml:"command_max_attempts"`
}

// ghostManifestFile maps one ghost_manifests TOML table row for remote deploy targets.
type ghostManifestFile struct {
	GhostID                         string   `toml:"ghost_id"`
	Host                            string   `toml:"host"`
	User                            string   `toml:"user"`
	SSHKey                          string   `toml:"ssh_key"`
	SSHPort                         int      `toml:"ssh_port"`
	Seeds                           []string `toml:"seeds"`
	AdminListen                     string   `toml:"admin_listen"`
	MiragePolicy                    string   `toml:"mirage_policy"`
	MirageAddress                   string   `toml:"mirage_address"`
	SeedInstallEnabled              bool     `toml:"seed_install_enabled"`
	SeedInstallRoot                 string   `toml:"seed_install_root"`
	SeedInstallBinRoot              string   `toml:"seed_install_bin_root"`
	SeedInstallAllowInternalDefault bool     `toml:"seed_install_allow_internal_defaults"`
	SeedInstallWhitelist            []string `toml:"seed_install_whitelist"`
	// Rejected when present; kept only to detect stale configs.
	SeedInstall        []map[string]any `toml:"seed_install"`
	ConfigTemplatePath string           `toml:"config_template_path"`
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
	// Mirage admin mTLS identity edgectl://mirage/<id>: serves the admin endpoint and dials Ghosts.
	if meta.IsDefined("admin_tls_cert_file") {
		cfg.AdminTLS.CertFile = strings.TrimSpace(raw.AdminTLSCertFile)
	}
	if meta.IsDefined("admin_tls_key_file") {
		cfg.AdminTLS.KeyFile = strings.TrimSpace(raw.AdminTLSKeyFile)
	}
	if meta.IsDefined("admin_tls_ca_file") {
		cfg.AdminTLS.CAFile = strings.TrimSpace(raw.AdminTLSCAFile)
	}
	if meta.IsDefined("admin_tls_denied_serials") {
		cfg.AdminTLS.DeniedSerials = tools.NormalizeNonEmptyList(raw.AdminTLSDeniedSerials)
	}
	// Token Mirage presents to Ghost admin endpoints (execute_envelope, install_dependencies).
	if meta.IsDefined("ghost_admin_frame_auth_token") {
		cfg.GhostAdminFrameAuthToken = strings.TrimSpace(raw.GhostAdminFrameAuthToken)
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
	// Revoked certificate serials (hex) rejected on every session handshake.
	if meta.IsDefined("session_tls_denied_serials") {
		cfg.Session.TLS.DeniedSerials = tools.NormalizeNonEmptyList(raw.SessionTLSDeniedSerials)
	}
	if meta.IsDefined("session_tls_ca_file") {
		cfg.Session.TLS.CAFile = strings.TrimSpace(raw.SessionTLSCAFile)
	}
	if meta.IsDefined("ghost_manifests") {
		// Install specs come only from Mirage's dependency catalog via approved proposals.
		for _, m := range raw.GhostManifests {
			if len(m.SeedInstall) > 0 {
				return mirage.ServiceConfig{}, fmt.Errorf(
					"ghost_manifests ghost_id=%q: seed_install specs are no longer accepted; use propose/approve/execute_dependencies",
					strings.TrimSpace(m.GhostID),
				)
			}
		}
		cfg.GhostManifests = normalizeGhostManifests(raw.GhostManifests)
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

	// Command dispatch attempts before retry_exhausted (reliability.toml retry_policy.command).
	if meta.IsDefined("command_max_attempts") {
		if raw.CommandMaxAttempts < 1 {
			return mirage.ServiceConfig{}, fmt.Errorf("command_max_attempts must be >= 1, got %d", raw.CommandMaxAttempts)
		}
		cfg.CommandMaxAttempts = raw.CommandMaxAttempts
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
	// The managed local Ghost accepts the same token Mirage presents.
	cfg.AdminFrameAuthToken = mCfg.GhostAdminFrameAuthToken
	// Managed local Ghost: its own admin identity, Mirage's CA and deny list, Mirage pinned.
	if certFile := strings.TrimSpace(raw.LocalGhostAdminTLSCertFile); certFile != "" {
		cfg.AdminTLS = adminauth.Credentials{
			CertFile:      certFile,
			KeyFile:       strings.TrimSpace(raw.LocalGhostAdminTLSKeyFile),
			CAFile:        mCfg.AdminTLS.CAFile,
			DeniedSerials: append([]string{}, mCfg.AdminTLS.DeniedSerials...),
		}
	}
	if mCfg.AdminTLS.Enabled() {
		cfg.MiragePrincipal = "edgectl://mirage/" + strings.TrimSpace(mCfg.MirageID)
	}
	if cfg.AdminListenAddr == "" {
		cfg.AdminListenAddr = ghost.DefaultServiceConfig().AdminListenAddr
	}
	cfg.BuiltinSeedIDs = []string{"seed.flow", "seed.kv", "seed.fs", "seed.mongod"}
	if meta.IsDefined("local_ghost_seeds") {
		cfg.BuiltinSeedIDs = tools.NormalizeNonEmptyList(raw.LocalGhostSeeds)
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

func normalizeGhostManifests(raw []ghostManifestFile) []mirage.GhostManifest {
	out := make([]mirage.GhostManifest, 0, len(raw))
	for _, r := range raw {
		id := strings.TrimSpace(r.GhostID)
		host := strings.TrimSpace(r.Host)
		if id == "" || host == "" {
			continue
		}
		out = append(out, mirage.GhostManifest{
			GhostID:                          id,
			Host:                             host,
			User:                             strings.TrimSpace(r.User),
			SSHKeyFile:                       strings.TrimSpace(r.SSHKey),
			SSHPort:                          r.SSHPort,
			Seeds:                            tools.NormalizeNonEmptyList(r.Seeds),
			AdminListen:                      strings.TrimSpace(r.AdminListen),
			MiragePolicy:                     strings.TrimSpace(r.MiragePolicy),
			MirageAddress:                    strings.TrimSpace(r.MirageAddress),
			SeedInstallEnabled:               r.SeedInstallEnabled,
			SeedInstallRoot:                  strings.TrimSpace(r.SeedInstallRoot),
			SeedInstallBinRoot:               strings.TrimSpace(r.SeedInstallBinRoot),
			SeedInstallAllowInternalDefaults: r.SeedInstallAllowInternalDefault,
			SeedInstallWhitelist:             tools.NormalizeNonEmptyList(r.SeedInstallWhitelist),
			ConfigTemplatePath:               strings.TrimSpace(r.ConfigTemplatePath),
		})
	}
	return out
}
