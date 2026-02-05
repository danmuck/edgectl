package main

import (
	"github.com/danmuck/edgectl/internal/config"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/server"
	"github.com/danmuck/edgectl/internal/services"
	"github.com/rs/zerolog/log"
)

// var startedAt = time.Now()

func main() {
	observability.InitLogger("ghost")
	configPath := "cmd/edgectl/config.toml"
	cfg, err := config.LoadGhostConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load ghost config")
	}

	log.Info().Str("path", configPath).Msg("loaded ghost config")
	ghost := server.Appear(cfg.Name, cfg.Addr, cfg.CorsOrigins)
	ghost.LoadSeeds(config.GhostSeeds(cfg.Seeds))
	log.Info().Int("count", len(cfg.Seeds)).Msg("loaded seeds from config")
	localSeed := ghost.CreateLocalSeed(cfg.Name, "/local/"+cfg.Name)
	localSeed.Registry.Register(&services.AdminCommands{})

	log.Info().Str("name", ghost.Name).Str("addr", cfg.Addr).Msg("ghost started")
	if err := ghost.Serve(); err != nil {
		log.Fatal().Err(err).Msg("ghost stopped")
	}
}
