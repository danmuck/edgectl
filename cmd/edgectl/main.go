package main

import (
	"fmt"
	"log"

	"github.com/danmuck/edgectl/internal/config"
	"github.com/danmuck/edgectl/internal/server"
)

// var startedAt = time.Now()

func main() {
	cfg, err := config.LoadGhostConfig("cmd/edgectl/config.toml")
	if err != nil {
		log.Fatal(err)
	}

	ghost := server.Appear(cfg.Name, cfg.Addr, cfg.CorsOrigins)
	ghost.LoadSeeds(config.GhostSeeds(cfg.Seeds))

	fmt.Println("A Ghost has appeared ... " + ghost.Name)
	if err := ghost.Serve(); err != nil {
		log.Fatal(err)
	}
}
