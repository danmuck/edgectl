package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type MirageConfig struct {
	Name        string             `toml:"name"`
	Addr        string             `toml:"addr"`
	CorsOrigins []string           `toml:"cors_origins"`
	Ghosts      []GhostConfigEntry `toml:"ghosts"`
}

type GhostConfigEntry struct {
	ID    string   `toml:"id"`
	Host  string   `toml:"host"`
	Addr  string   `toml:"addr"`
	Group string   `toml:"group"`
	Exec  bool     `toml:"exec"`
	Auth  string   `toml:"auth"`
	Seeds []string `toml:"seeds"`
}

type GhostNodeConfig struct {
	ID          string   `toml:"id"`
	Host        string   `toml:"host"`
	Addr        string   `toml:"addr"`
	Group       string   `toml:"group"`
	Exec        bool     `toml:"exec"`
	CorsOrigins []string `toml:"cors_origins"`
	Seeds       []string `toml:"seeds"`
}

func LoadMirageConfig(path string) (MirageConfig, error) {
	var cfg MirageConfig
	if err := loadToml(path, &cfg); err != nil {
		return MirageConfig{}, err
	}
	if cfg.Name == "" {
		cfg.Name = "edge-ctl"
	}
	if cfg.Addr == "" {
		cfg.Addr = ":9000"
	}
	if err := ValidateMirageConfig(cfg); err != nil {
		return MirageConfig{}, err
	}
	return cfg, nil
}

func LoadGhostConfig(path string) (GhostNodeConfig, error) {
	var cfg GhostNodeConfig
	if err := loadToml(path, &cfg); err != nil {
		return GhostNodeConfig{}, err
	}
	if cfg.ID == "" {
		cfg.ID = "ghostctl"
	}
	if cfg.Addr == "" {
		cfg.Addr = ":9100"
	}
	if err := ValidateGhostConfig(cfg); err != nil {
		return GhostNodeConfig{}, err
	}
	return cfg, nil
}

func loadToml(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config load failed (%s): %w", path, err)
	}
	if err := toml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("config parse failed (%s): %w", path, err)
	}
	return nil
}

func ValidateMirageConfig(cfg MirageConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("mirage config missing name")
	}
	if strings.TrimSpace(cfg.Addr) == "" {
		return fmt.Errorf("mirage config missing addr")
	}
	for i, ghostCfg := range cfg.Ghosts {
		if err := ValidateGhostEntry(ghostCfg); err != nil {
			return fmt.Errorf("ghost[%d] invalid: %w", i, err)
		}
	}
	return nil
}

func ValidateGhostConfig(cfg GhostNodeConfig) error {
	if strings.TrimSpace(cfg.ID) == "" {
		return fmt.Errorf("ghost config missing id")
	}
	if strings.TrimSpace(cfg.Addr) == "" {
		return fmt.Errorf("ghost config missing addr")
	}
	if strings.HasPrefix(strings.TrimSpace(cfg.Addr), ":") &&
		strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("ghost config host required when addr is a port")
	}
	return nil
}

func ValidateGhostEntry(cfg GhostConfigEntry) error {
	if strings.TrimSpace(cfg.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(cfg.Addr) == "" {
		return fmt.Errorf("addr is required")
	}
	if strings.HasPrefix(strings.TrimSpace(cfg.Addr), ":") &&
		strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("host required when addr is a port")
	}
	return nil
}
