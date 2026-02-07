package main

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

// miragectl config.toml key mapping to Mirage runtime settings.
type fileConfig struct {
	Addr                string `toml:"addr"`
	RequireIdentityBind bool   `toml:"require_identity_binding"`
	RootGhostAdminAddr  string `toml:"root_ghost_admin_addr"`
	SessionSecurityMode string `toml:"session_security_mode"`
	SessionTLSEnabled   bool   `toml:"session_tls_enabled"`
	SessionTLSMutual    bool   `toml:"session_tls_mutual"`
	SessionTLSCertFile  string `toml:"session_tls_cert_file"`
	SessionTLSKeyFile   string `toml:"session_tls_key_file"`
	SessionTLSCAFile    string `toml:"session_tls_ca_file"`
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
	if meta.IsDefined("require_identity_binding") {
		cfg.RequireIdentityBinding = raw.RequireIdentityBind
	}
	if meta.IsDefined("root_ghost_admin_addr") {
		cfg.RootGhostAdminAddr = strings.TrimSpace(raw.RootGhostAdminAddr)
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
	cfg.Session = cfg.Session.WithDefaults()
	return cfg, nil
}
