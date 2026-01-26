package config

import (
	"time"

	"github.com/danmuck/edgectl/internal/seed"
)

func GhostSeeds(entries []SeedConfig) []seed.Seed {
	seeds := make([]seed.Seed, 0, len(entries))
	for _, entry := range entries {
		seeds = append(seeds, seed.Seed{
			ID:       entry.ID,
			Host:     entry.Host,
			Addr:     entry.Addr,
			Group:    entry.Group,
			Exec:     entry.Exec,
			Services: entry.Services,
			Auth:     entry.Auth,
			Appeared: time.Now(),
		})
	}
	return seeds
}
