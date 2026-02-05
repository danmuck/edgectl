package server

import (
	"time"

	"github.com/danmuck/edgectl/internal/node"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/seed"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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
	observability.RegisterMetrics()
	r := gin.New()

	// Middleware: keep it lean
	r.Use(gin.Recovery())
	r.Use(observability.RequestLogger(log.Logger))
	r.Use(observability.RequestMetricsMiddleware(name))
	r.Use(cors.New(cors.Config{
		AllowOrigins: normalizeOrigins(corsOrigins),
		AllowMethods: []string{"GET", "POST"},
		AllowHeaders: []string{"Origin", "Content-Type"},
		MaxAge:       12 * time.Hour,
	}))
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})
	g := &Ghost{
		Name:       name,
		addr:       addr,
		httpRouter: r,
		appeared:   time.Now(),
	}
	return g
}

func (g *Ghost) Serve() error {
	g.RegisterRoutesTMP()
	err := g.httpRouter.Run(g.addr)
	return err
}

// refresh Seeds, update local repo
func (g *Ghost) RefreshSeeds() error {
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
	if g.seedBank == nil {
		g.seedBank = map[string]seed.Seed{}
	}
	if existing, ok := g.local[id]; ok {
		return existing
	}
	localSeed := seed.Attach(id, g.httpRouter, basePath, nil)
	if registered, ok := g.seedBank[id]; ok {
		localSeed.Host = registered.Host
		localSeed.Addr = registered.Addr
		localSeed.Group = registered.Group
		localSeed.Exec = registered.Exec
		localSeed.Services = registered.Services
		localSeed.Auth = registered.Auth
	} else {
		localSeed.Host = "localhost"
		localSeed.Addr = g.addr
	}
	g.local[id] = localSeed
	g.seedBank[id] = *localSeed
	log.Info().
		Str("seed", localSeed.ID).
		Str("base_path", basePath).
		Msg("local seed attached")
	return localSeed
}

func (g *Ghost) LocalSeed(id string) (*seed.Seed, bool) {
	if g.local == nil {
		return nil, false
	}
	local, ok := g.local[id]
	return local, ok
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
		log.Info().
			Str("seed", seed.ID).
			Str("addr", seed.Addr).
			Str("host", seed.Host).
			Bool("exec", seed.Exec).
			Msg("seed registered")
	}
}

func normalizeOrigins(origins []string) []string {
	if len(origins) == 0 {
		return []string{"http://localhost:3000"}
	}
	return origins
}
