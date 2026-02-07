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
addr = "127.0.0.1:9443"
require_identity_binding = true
session_security_mode = "production"
session_tls_enabled = true
session_tls_mutual = true
session_tls_cert_file = "/etc/mirage/server.crt"
session_tls_key_file = "/etc/mirage/server.key"
session_tls_ca_file = "/etc/mirage/ca.crt"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := loadServiceConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:9443" {
		t.Fatalf("unexpected listen addr: %q", cfg.ListenAddr)
	}
	if !cfg.RequireIdentityBinding {
		t.Fatalf("expected require_identity_binding true")
	}
	if cfg.Session.SecurityMode != "production" {
		t.Fatalf("unexpected security mode: %q", cfg.Session.SecurityMode)
	}
	if !cfg.Session.TLS.Enabled || !cfg.Session.TLS.Mutual {
		t.Fatalf("expected tls+mtls enabled")
	}
	if cfg.Session.TLS.CertFile != "/etc/mirage/server.crt" {
		t.Fatalf("unexpected cert file: %q", cfg.Session.TLS.CertFile)
	}
	if cfg.Session.TLS.KeyFile != "/etc/mirage/server.key" {
		t.Fatalf("unexpected key file: %q", cfg.Session.TLS.KeyFile)
	}
	if cfg.Session.TLS.CAFile != "/etc/mirage/ca.crt" {
		t.Fatalf("unexpected ca file: %q", cfg.Session.TLS.CAFile)
	}
}
