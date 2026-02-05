package main

import (
	"github.com/danmuck/edgectl/internal/config"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/seed"
	"github.com/rs/zerolog/log"
)

func main() {
	observability.InitLogger("seed")
	configPath := "cmd/seedctl/config.toml"
	cfg, err := config.LoadSeedConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load seed config")
	}
	log.Info().Str("path", configPath).Msg("loaded seed config")
	server := seed.Appear(cfg.ID, cfg.Addr, cfg.CorsOrigins)
	server.Host = cfg.Host
	server.Group = cfg.Group
	server.Exec = cfg.Exec
	server.Services = cfg.Services

	log.Info().Str("id", server.ID).Str("addr", server.Addr).Msg("seed started")
	if err := server.Serve(); err != nil {
		log.Fatal().Err(err).Msg("seed stopped")
	}
}
