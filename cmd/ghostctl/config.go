package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/ghost"
)

type fileConfig struct {
	ID                  string   `toml:"id"`
	Seeds               []string `toml:"seeds"`
	Heartbeat           string   `toml:"heartbeat"`
	HeartbeatInterval   string   `toml:"heartbeat_interval"`
	HeartbeatIntervalMS int64    `toml:"heartbeat_interval_ms"`
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
