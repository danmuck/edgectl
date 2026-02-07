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
	"github.com/danmuck/edgectl/internal/seeds"
)

// ghostctl config.toml key mapping to Ghost runtime settings.
type fileConfig struct {
	ID                   string            `toml:"id"`
	Seeds                []string          `toml:"seeds"`
	Heartbeat            string            `toml:"heartbeat"`
	HeartbeatInterval    string            `toml:"heartbeat_interval"`
	HeartbeatIntervalMS  int64             `toml:"heartbeat_interval_ms"`
	AdminListen          string            `toml:"admin_listen"`
	MiragePolicy         string            `toml:"mirage_policy"`
	MirageAddress        string            `toml:"mirage_address"`
	MiragePeerIdentity   string            `toml:"mirage_peer_identity"`
	MirageMaxAttempts    int               `toml:"mirage_max_connect_attempts"`
	MirageSecurityMode   string            `toml:"mirage_security_mode"`
	MirageTLSEnabled     bool              `toml:"mirage_tls_enabled"`
	MirageTLSMutual      bool              `toml:"mirage_tls_mutual"`
	MirageTLSCertFile    string            `toml:"mirage_tls_cert_file"`
	MirageTLSKeyFile     string            `toml:"mirage_tls_key_file"`
	MirageTLSCAFile      string            `toml:"mirage_tls_ca_file"`
	MirageTLSServerName  string            `toml:"mirage_tls_server_name"`
	MirageTLSInsecure    bool              `toml:"mirage_tls_insecure_skip_verify"`
	SeedInstallEnabled   bool              `toml:"seed_install_enabled"`
	SeedInstallRoot      string            `toml:"seed_install_root"`
	SeedInstallWhitelist []string          `toml:"seed_install_whitelist"`
	SeedInstall          []fileSeedInstall `toml:"seed_install"`
}

// ghostctl seed-install table mapping from config.toml.
type fileSeedInstall struct {
	SeedID             string   `toml:"seed_id"`
	Method             string   `toml:"method"`
	Repo               string   `toml:"repo"`
	Branch             string   `toml:"branch"`
	Ref                string   `toml:"ref"`
	Source             string   `toml:"source"`
	Destination        string   `toml:"destination"`
	Package            string   `toml:"package"`
	Tap                string   `toml:"tap"`
	BootstrapIfMissing bool     `toml:"bootstrap_if_missing"`
	BootstrapCommand   []string `toml:"bootstrap_cmd"`
}

// ghostctl loader for TOML config with default overlay.
func loadServiceConfig(path string) (ghost.ServiceConfig, error) {
	cfg := ghost.DefaultServiceConfig()
	cfg.SeedInstall.WorkspaceRoot = resolveWorkspaceRoot(path)

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

	if meta.IsDefined("seeds") {
		cfg.BuiltinSeedIDs = normalizeSeeds(raw.Seeds)
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
	if meta.IsDefined("seed_install_enabled") {
		cfg.SeedInstall.Enabled = raw.SeedInstallEnabled
	}
	if meta.IsDefined("seed_install_root") {
		cfg.SeedInstall.InstallRoot = strings.TrimSpace(raw.SeedInstallRoot)
	}
	if meta.IsDefined("seed_install_whitelist") {
		cfg.SeedInstall.Whitelist = normalizeList(raw.SeedInstallWhitelist)
	}
	if meta.IsDefined("seed_install") {
		specs, err := parseSeedInstallSpecs(raw.SeedInstall)
		if err != nil {
			return ghost.ServiceConfig{}, err
		}
		cfg.SeedInstall.Specs = specs
	}

	return cfg, nil
}

// ghostctl seed list normalizer that trims ids and drops empty entries.
func normalizeSeeds(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(in))
	for _, seed := range in {
		v := strings.TrimSpace(seed)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

// ghostctl string-list normalizer that trims values and drops empty entries.
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

// ghostctl seed-install parser from config table entries into install specs.
func parseSeedInstallSpecs(in []fileSeedInstall) ([]seeds.InstallSpec, error) {
	out := make([]seeds.InstallSpec, 0, len(in))
	for _, row := range in {
		seedID := strings.TrimSpace(row.SeedID)
		method := strings.TrimSpace(row.Method)
		if seedID == "" {
			return nil, fmt.Errorf("parse seed_install: missing seed_id")
		}
		if method == "" {
			return nil, fmt.Errorf("parse seed_install seed_id=%q: missing method", seedID)
		}
		out = append(out, seeds.InstallSpec{
			SeedID:             seedID,
			Method:             seeds.InstallMethod(strings.ToLower(method)),
			RepoURL:            strings.TrimSpace(row.Repo),
			Branch:             strings.TrimSpace(row.Branch),
			Ref:                strings.TrimSpace(row.Ref),
			SourcePath:         strings.TrimSpace(row.Source),
			Destination:        strings.TrimSpace(row.Destination),
			Package:            strings.TrimSpace(row.Package),
			Tap:                strings.TrimSpace(row.Tap),
			BootstrapIfMissing: row.BootstrapIfMissing,
			BootstrapCommand:   normalizeList(row.BootstrapCommand),
		})
	}
	return out, nil
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
			return dir
		}
		dir = parent
	}
}
