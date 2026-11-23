package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/tools"
)

// ghostctl config.toml key mapping to Ghost runtime settings.
type fileConfig struct {
	ID                   string   `toml:"id"`
	ProjectRoot          string   `toml:"project_root"`
	ProjectFetchOnBoot   bool     `toml:"project_fetch_on_boot"`
	Seeds                []string `toml:"seeds"`
	Heartbeat            string   `toml:"heartbeat"`
	HeartbeatInterval    string   `toml:"heartbeat_interval"`
	HeartbeatIntervalMS  int64    `toml:"heartbeat_interval_ms"`
	AdminListen          string   `toml:"admin_listen"`
	AdminFrameAuthToken  string   `toml:"admin_frame_auth_token"`
	MiragePolicy         string   `toml:"mirage_policy"`
	MirageAddress        string   `toml:"mirage_address"`
	MiragePeerIdentity   string   `toml:"mirage_peer_identity"`
	MirageMaxAttempts    int      `toml:"mirage_max_connect_attempts"`
	MirageSecurityMode   string   `toml:"mirage_security_mode"`
	MirageTLSEnabled     bool     `toml:"mirage_tls_enabled"`
	MirageTLSMutual      bool     `toml:"mirage_tls_mutual"`
	MirageTLSCertFile    string   `toml:"mirage_tls_cert_file"`
	MirageTLSKeyFile     string   `toml:"mirage_tls_key_file"`
	MirageTLSCAFile      string   `toml:"mirage_tls_ca_file"`
	MirageTLSServerName  string   `toml:"mirage_tls_server_name"`
	MirageTLSInsecure    bool     `toml:"mirage_tls_insecure_skip_verify"`
	SeedInstallEnabled   bool     `toml:"seed_install_enabled"`
	SeedInstallRoot      string   `toml:"seed_install_root"`
	SeedInstallBinRoot   string   `toml:"seed_install_bin_root"`
	SeedInstallWhitelist []string `toml:"seed_install_whitelist"`
}

// ghostctl loader for TOML config with default overlay.
func loadServiceConfig(path string) (ghost.ServiceConfig, error) {
	cfg := ghost.DefaultServiceConfig()
	workspaceRoot := resolveWorkspaceRoot(path)
	cfg.ProjectRoot = workspaceRoot
	cfg.SeedInstall.WorkspaceRoot = workspaceRoot

	var raw fileConfig
	meta, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return ghost.ServiceConfig{}, fmt.Errorf("load ghost config: %w", err)
	}

	if meta.IsDefined("id") {
		id := strings.TrimSpace(raw.ID)
		if id != "" {
			cfg.GhostID = id
		}
	}
	if meta.IsDefined("project_root") {
		cfg.ProjectRoot = strings.TrimSpace(raw.ProjectRoot)
	}
	if meta.IsDefined("project_fetch_on_boot") {
		cfg.ProjectFetchOnBoot = raw.ProjectFetchOnBoot
	}

	if meta.IsDefined("seeds") {
		cfg.BuiltinSeedIDs = tools.NormalizeNonEmptyList(raw.Seeds)
	}

	if meta.IsDefined("heartbeat") {
		d, err := time.ParseDuration(strings.TrimSpace(raw.Heartbeat))
		if err != nil {
			return ghost.ServiceConfig{}, fmt.Errorf("parse heartbeat: %w", err)
		}
		cfg.HeartbeatInterval = d
	}

	if meta.IsDefined("heartbeat_interval") {
		d, err := time.ParseDuration(strings.TrimSpace(raw.HeartbeatInterval))
		if err != nil {
			return ghost.ServiceConfig{}, fmt.Errorf("parse heartbeat_interval: %w", err)
		}
		cfg.HeartbeatInterval = d
	}

	if meta.IsDefined("heartbeat_interval_ms") {
		cfg.HeartbeatInterval = time.Duration(raw.HeartbeatIntervalMS) * time.Millisecond
	}
	if meta.IsDefined("admin_listen") {
		cfg.AdminListenAddr = strings.TrimSpace(raw.AdminListen)
	}

	if meta.IsDefined("mirage_policy") {
		cfg.Mirage.Policy = ghost.MirageSessionPolicy(strings.TrimSpace(raw.MiragePolicy))
	}

	if meta.IsDefined("mirage_address") {
		cfg.Mirage.Address = strings.TrimSpace(raw.MirageAddress)
	}

	if meta.IsDefined("mirage_peer_identity") {
		cfg.Mirage.PeerIdentity = strings.TrimSpace(raw.MiragePeerIdentity)
	}

	if meta.IsDefined("mirage_max_connect_attempts") {
		cfg.Mirage.MaxConnectAttempts = raw.MirageMaxAttempts
	}
	if meta.IsDefined("mirage_security_mode") {
		cfg.Mirage.SessionConfig.SecurityMode = session.SecurityMode(strings.TrimSpace(raw.MirageSecurityMode))
	}
	if meta.IsDefined("mirage_tls_enabled") {
		cfg.Mirage.SessionConfig.TLS.Enabled = raw.MirageTLSEnabled
	}
	if meta.IsDefined("mirage_tls_mutual") {
		cfg.Mirage.SessionConfig.TLS.Mutual = raw.MirageTLSMutual
	}
	if meta.IsDefined("mirage_tls_cert_file") {
		cfg.Mirage.SessionConfig.TLS.CertFile = strings.TrimSpace(raw.MirageTLSCertFile)
	}
	if meta.IsDefined("mirage_tls_key_file") {
		cfg.Mirage.SessionConfig.TLS.KeyFile = strings.TrimSpace(raw.MirageTLSKeyFile)
	}
	if meta.IsDefined("mirage_tls_ca_file") {
		cfg.Mirage.SessionConfig.TLS.CAFile = strings.TrimSpace(raw.MirageTLSCAFile)
	}
	if meta.IsDefined("mirage_tls_server_name") {
		cfg.Mirage.SessionConfig.TLS.ServerName = strings.TrimSpace(raw.MirageTLSServerName)
	}
	if meta.IsDefined("mirage_tls_insecure_skip_verify") {
		cfg.Mirage.SessionConfig.TLS.InsecureSkipVerify = raw.MirageTLSInsecure
	}
	// Shared secret for admin execute_envelope frames and required for install_dependencies plans.
	if meta.IsDefined("admin_frame_auth_token") {
		cfg.AdminFrameAuthToken = strings.TrimSpace(raw.AdminFrameAuthToken)
	}
	if meta.IsDefined("seed_install_enabled") {
		cfg.SeedInstall.Enabled = raw.SeedInstallEnabled
	}
	if meta.IsDefined("seed_install_root") {
		cfg.SeedInstall.InstallRoot = strings.TrimSpace(raw.SeedInstallRoot)
	}
	if meta.IsDefined("seed_install_bin_root") {
		cfg.SeedInstall.BinRoot = strings.TrimSpace(raw.SeedInstallBinRoot)
	}
	if meta.IsDefined("seed_install_whitelist") {
		cfg.SeedInstall.Whitelist = tools.NormalizeNonEmptyList(raw.SeedInstallWhitelist)
	}
	// Install plans come only from Mirage-approved proposals (docs/progress/mvp_p8.md Slice F).
	if meta.IsDefined("seed_install") {
		return ghost.ServiceConfig{}, fmt.Errorf("seed_install specs are no longer accepted in ghost config: dependency installs require a Mirage-approved proposal")
	}

	return cfg, nil
}

// ghostctl workspace-root resolver using nearest parent directory with go.mod.
func resolveWorkspaceRoot(configPath string) string {
	start := filepath.Dir(configPath)
	if start == "" {
		start = "."
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}
		dir = parent
	}
}
