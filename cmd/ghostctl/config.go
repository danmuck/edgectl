package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

type fileConfig struct {
	ID                  string   `toml:"id"`
	Seeds               []string `toml:"seeds"`
	Heartbeat           string   `toml:"heartbeat"`
	HeartbeatInterval   string   `toml:"heartbeat_interval"`
	HeartbeatIntervalMS int64    `toml:"heartbeat_interval_ms"`
	MiragePolicy        string   `toml:"mirage_policy"`
	MirageAddress       string   `toml:"mirage_address"`
	MiragePeerIdentity  string   `toml:"mirage_peer_identity"`
	MirageMaxAttempts   int      `toml:"mirage_max_connect_attempts"`
	MirageSecurityMode  string   `toml:"mirage_security_mode"`
	MirageTLSEnabled    bool     `toml:"mirage_tls_enabled"`
	MirageTLSMutual     bool     `toml:"mirage_tls_mutual"`
	MirageTLSCertFile   string   `toml:"mirage_tls_cert_file"`
	MirageTLSKeyFile    string   `toml:"mirage_tls_key_file"`
	MirageTLSCAFile     string   `toml:"mirage_tls_ca_file"`
	MirageTLSServerName string   `toml:"mirage_tls_server_name"`
	MirageTLSInsecure   bool     `toml:"mirage_tls_insecure_skip_verify"`
}

func loadServiceConfig(path string) (ghost.ServiceConfig, error) {
	cfg := ghost.DefaultServiceConfig()

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

	return cfg, nil
}

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
