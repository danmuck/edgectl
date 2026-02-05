package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type GhostConfig struct {
	Name        string       `toml:"name"`
	Addr        string       `toml:"addr"`
	CorsOrigins []string     `toml:"cors_origins"`
	Seeds       []SeedConfig `toml:"seeds"`
}

type SeedConfig struct {
	ID       string   `toml:"id"`
	Host     string   `toml:"host"`
	Addr     string   `toml:"addr"`
	Group    string   `toml:"group"`
	Exec     bool     `toml:"exec"`
	Auth     string   `toml:"auth"`
	Services []string `toml:"services"`
}

type SeedNodeConfig struct {
	ID          string   `toml:"id"`
	Host        string   `toml:"host"`
	Addr        string   `toml:"addr"`
	Group       string   `toml:"group"`
	Exec        bool     `toml:"exec"`
	CorsOrigins []string `toml:"cors_origins"`
	Services    []string `toml:"services"`
}

func LoadGhostConfig(path string) (GhostConfig, error) {
	var cfg GhostConfig
	if err := loadToml(path, &cfg); err != nil {
		return GhostConfig{}, err
	}
	if cfg.Name == "" {
		cfg.Name = "edge-ctl"
	}
	if cfg.Addr == "" {
		cfg.Addr = ":9000"
	}
	if err := ValidateGhostConfig(cfg); err != nil {
		return GhostConfig{}, err
	}
	return cfg, nil
}

func LoadSeedConfig(path string) (SeedNodeConfig, error) {
	var cfg SeedNodeConfig
	if err := loadToml(path, &cfg); err != nil {
		return SeedNodeConfig{}, err
	}
	if cfg.ID == "" {
		cfg.ID = "seedctl"
	}
	if cfg.Addr == "" {
		cfg.Addr = ":9100"
	}
	if err := ValidateSeedConfig(cfg); err != nil {
		return SeedNodeConfig{}, err
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

func ValidateGhostConfig(cfg GhostConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("ghost config missing name")
	}
	if strings.TrimSpace(cfg.Addr) == "" {
		return fmt.Errorf("ghost config missing addr")
	}
	for i, seedCfg := range cfg.Seeds {
		if err := ValidateSeedEntry(seedCfg); err != nil {
			return fmt.Errorf("seed[%d] invalid: %w", i, err)
		}
	}
	return nil
}

func ValidateSeedConfig(cfg SeedNodeConfig) error {
	if strings.TrimSpace(cfg.ID) == "" {
		return fmt.Errorf("seed config missing id")
	}
	if strings.TrimSpace(cfg.Addr) == "" {
		return fmt.Errorf("seed config missing addr")
	}
	if strings.HasPrefix(strings.TrimSpace(cfg.Addr), ":") &&
		strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("seed config host required when addr is a port")
	}
	return nil
}

func ValidateSeedEntry(cfg SeedConfig) error {
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
