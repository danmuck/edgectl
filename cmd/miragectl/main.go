package main

import (
	"github.com/danmuck/edgectl/internal/config"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/rs/zerolog/log"
)

// var startedAt = time.Now()

func main() {
	observability.InitLogger("mirage")
	configPath := "cmd/miragectl/config.toml"
	cfg, err := config.LoadMirageConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load mirage config")
	}

	log.Info().Str("path", configPath).Msg("loaded mirage config")
	controlPlane := mirage.Appear(cfg.Name, cfg.Addr, cfg.CorsOrigins)
	controlPlane.LoadGhosts(config.MirageGhosts(cfg.Ghosts))
	log.Info().Int("count", len(cfg.Ghosts)).Msg("loaded ghosts from config")
	localGhost := controlPlane.CreateLocalGhost(cfg.Name, "/local/"+cfg.Name)
	localGhost.Registry.Register(&seeds.AdminCommands{})
	localGhost.Registry.Register(&seeds.FlowSeed{})

	log.Info().Str("name", controlPlane.Name).Str("addr", cfg.Addr).Msg("mirage started")
	if err := controlPlane.Serve(); err != nil {
		log.Fatal().Err(err).Msg("mirage stopped")
	}
}
