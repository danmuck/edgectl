package config

import (
	"fmt"
	"os"

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
