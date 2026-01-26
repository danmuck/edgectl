package main

import (
	"log"

	"github.com/danmuck/edgectl/internal/config"
	"github.com/danmuck/edgectl/internal/seed"
)

func main() {
	cfg, err := config.LoadSeedConfig("cmd/seedctl/config.toml")
	if err != nil {
		log.Fatal(err)
	}

	server := seed.Appear(cfg.ID, cfg.Addr, cfg.CorsOrigins)
	server.Host = cfg.Host
	server.Group = cfg.Group
	server.Exec = cfg.Exec
	server.Services = cfg.Services

	log.Printf("Seed %s listening on %s", server.ID, server.Addr)
	if err := server.Serve(); err != nil {
		log.Fatal(err)
	}
}
