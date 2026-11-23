package mirage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateConfigTemplateOverrides(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "pi.tls.config.toml")
	content := `
id = "ghost.pi"
admin_listen = "0.0.0.0:7011"
seeds = ["seed.flow", "seed.host"]
mirage_policy = "required"
mirage_address = "192.168.1.100:9000"
mirage_peer_identity = "ghost.pi"
seed_install_enabled = false

[[seed_install]]
seed_id = "seed.stale"
method = "brew"
package = "stale"
`
	if err := os.WriteFile(template, []byte(content), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	d := NewSSHDeployer()
	cfg, err := d.generateConfig(DeployGhostRequest{
		Manifest: GhostManifest{
			GhostID:                          "ghost.pi.alpha",
			MirageAddress:                    "10.0.0.7:9000",
			SeedInstallEnabled:               true,
			SeedInstallAllowInternalDefaults: true,
			ConfigTemplatePath:               template,
		},
	}, "0.0.0.0:7021")
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	if !strings.Contains(cfg, `id = "ghost.pi.alpha"`) {
		t.Fatalf("expected ghost id override, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, `admin_listen = "0.0.0.0:7021"`) {
		t.Fatalf("expected admin listen override, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, `mirage_peer_identity = "ghost.pi.alpha"`) {
		t.Fatalf("expected mirage peer identity override, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, `seed_install_bin_root = "local/bin"`) {
		t.Fatalf("expected default seed_install_bin_root, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, `seed_install_enabled = true`) {
		t.Fatalf("expected seed install enabled, got:\n%s", cfg)
	}
	// Install specs never ship in Ghost config, even when the template carries one.
	if strings.Contains(cfg, "seed_install]]") || strings.Contains(cfg, "seed_id") {
		t.Fatalf("generated ghost config must not contain seed_install specs, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, `seed_install_whitelist = ["seed.docker", "seed.flow", "seed.fs", "seed.host", "seed.kv", "seed.mongod"]`) {
		t.Fatalf("expected default internal whitelist, got:\n%s", cfg)
	}
}

func TestGenerateConfigUsesRequestTemplatePath(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "pi.tls.config.toml")
	if err := os.WriteFile(template, []byte(`id = "ghost.base"`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	d := NewSSHDeployer()
	cfg, err := d.generateConfig(DeployGhostRequest{
		Manifest: GhostManifest{
			GhostID: "ghost.pi.beta",
		},
		ConfigTemplatePath: template,
	}, "0.0.0.0:7010")
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	if !strings.Contains(cfg, `id = "ghost.pi.beta"`) {
		t.Fatalf("expected id from manifest override, got:\n%s", cfg)
	}
}

// Deployed Ghost config carries the admin token Mirage presents, with or without a template.
func TestGenerateConfigWritesAdminFrameAuthToken(t *testing.T) {
	d := NewSSHDeployer()
	d.adminFrameAuthToken = "s3cret"
	cfg, err := d.generateConfig(DeployGhostRequest{Manifest: GhostManifest{GhostID: "ghost.pi.gamma"}}, "0.0.0.0:7010")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(cfg, `admin_frame_auth_token = "s3cret"`) {
		t.Fatalf("expected token in generated config, got:\n%s", cfg)
	}

	template := filepath.Join(t.TempDir(), "tpl.toml")
	if err := os.WriteFile(template, []byte(`id = "ghost.base"`), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	cfg, err = d.generateConfig(DeployGhostRequest{Manifest: GhostManifest{GhostID: "ghost.pi.gamma"}, ConfigTemplatePath: template}, "0.0.0.0:7010")
	if err != nil {
		t.Fatalf("generate from template: %v", err)
	}
	if !strings.Contains(cfg, `admin_frame_auth_token = "s3cret"`) {
		t.Fatalf("expected token in templated config, got:\n%s", cfg)
	}
}
