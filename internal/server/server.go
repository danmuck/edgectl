package server

import (
	"time"

	"github.com/danmuck/edgectl/internal/node"
	"github.com/danmuck/edgectl/internal/seed"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Ghost:edgectl -- main system brain
type Ghost struct {
	Name       string
	addr       string
	httpRouter *gin.Engine
	appeared   time.Time

	// local repo
	seedBank map[string]seed.Seed
	local    map[string]*seed.Seed
}

func Appear(name, addr string, corsOrigins []string) *Ghost {
	r := gin.New()

	// Middleware: keep it lean
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(cors.New(cors.Config{
		AllowOrigins: normalizeOrigins(corsOrigins),
		AllowMethods: []string{"GET"},
		AllowHeaders: []string{"Origin", "Content-Type"},
		MaxAge:       12 * time.Hour,
	}))
	g := &Ghost{
		Name:       name,
		addr:       addr,
		httpRouter: r,
		appeared:   time.Now(),
	}
	return g
}

func (g *Ghost) Serve() error {
	if len(g.seedBank) == 0 {
		_ = g.RefreshSeeds()
	}
	g.RegisterRoutesTMP()
	err := g.httpRouter.Run(g.addr)
	return err
}

// refresh Seeds, update local repo
func (g *Ghost) RefreshSeeds() error {
	// tmp := g.seedBank
	g.seedBank = map[string]seed.Seed{
		"edge-ctl": {
			ID:       "edge-ctl",
			Host:     "localhost",
			Addr:     "localhost:9000",
			Auth:     "temp-auth-key",
			Group:    "root",
			Exec:     true,
			Appeared: time.Now(),
		},
		"infra": {
			ID:       "infra",
			Host:     "localhost",
			Addr:     "localhost:8080",
			Auth:     "temp-auth-infra-key",
			Group:    "root",
			Exec:     true,
			Appeared: time.Now(),
		},
	}
	return nil
}

func (g *Ghost) Seed(id string) (seed.Seed, bool) {
	seed, ok := g.seedBank[id]
	return seed, ok
}

func (g *Ghost) Seeds() []seed.Seed {
	seeds := make([]seed.Seed, 0, len(g.seedBank))
	for _, seed := range g.seedBank {
		seeds = append(seeds, seed)
	}
	return seeds
}

func (g *Ghost) CreateLocalSeed(id string, basePath string) *seed.Seed {
	if g.local == nil {
		g.local = map[string]*seed.Seed{}
	}
	if existing, ok := g.local[id]; ok {
		return existing
	}
	localSeed := seed.Attach(id, g.httpRouter, basePath, nil)
	localSeed.Host = "localhost"
	localSeed.Addr = g.addr
	g.local[id] = localSeed
	return localSeed
}

func (g *Ghost) NodeID() string {
	return g.Name
}

func (g *Ghost) Kind() string {
	return "ghost"
}

func (g *Ghost) HTTPRouter() *gin.Engine {
	return g.httpRouter
}

var _ node.Node = (*Ghost)(nil)

func (g *Ghost) LoadSeeds(seeds []seed.Seed) {
	if g.seedBank == nil {
		g.seedBank = map[string]seed.Seed{}
	}
	for _, seed := range seeds {
		g.seedBank[seed.ID] = seed
	}
}

func normalizeOrigins(origins []string) []string {
	if len(origins) == 0 {
		return []string{"http://localhost:3000"}
	}
	return origins
}
