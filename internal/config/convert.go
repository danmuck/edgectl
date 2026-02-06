package config

import (
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
)

func MirageGhosts(entries []GhostConfigEntry) []ghost.Ghost {
	ghosts := make([]ghost.Ghost, 0, len(entries))
	for _, entry := range entries {
		ghosts = append(ghosts, ghost.Ghost{
			ID:       entry.ID,
			Host:     entry.Host,
			Addr:     entry.Addr,
			Group:    entry.Group,
			Exec:     entry.Exec,
			Seeds:    entry.Seeds,
			Auth:     entry.Auth,
			Appeared: time.Now(),
		})
	}
	return ghosts
}
