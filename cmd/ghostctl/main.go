package main

import (
	"github.com/danmuck/edgectl/internal/config"
	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/rs/zerolog/log"
)

func main() {
	observability.InitLogger("ghost")
	configPath := "cmd/ghostctl/config.toml"
	cfg, err := config.LoadGhostConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load ghost config")
	}
	log.Info().Str("path", configPath).Msg("loaded ghost config")
	server := ghost.Appear(cfg.ID, cfg.Addr, cfg.CorsOrigins)
	server.Host = cfg.Host
	server.Group = cfg.Group
	server.Exec = cfg.Exec
	server.Seeds = cfg.Seeds

	log.Info().Str("id", server.ID).Str("addr", server.Addr).Msg("ghost started")
	if err := server.Serve(); err != nil {
		log.Fatal().Err(err).Msg("ghost stopped")
	}
}
