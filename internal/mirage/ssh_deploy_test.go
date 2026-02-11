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
			SeedInstall: []GhostSeedInstallSpec{
				{
					SeedID:             "seed.mongod.pkg",
					Method:             "brew",
					Package:            "mongodb-community@7.0",
					Tap:                "mongodb/brew",
					BootstrapIfMissing: true,
				},
			},
			ConfigTemplatePath: template,
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
	if !strings.Contains(cfg, `seed_id = "seed.mongod.pkg"`) {
		t.Fatalf("expected seed install spec emission, got:\n%s", cfg)
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
