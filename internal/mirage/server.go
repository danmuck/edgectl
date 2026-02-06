package mirage

import (
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/node"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Mirage:edgectl -- main system brain
type Mirage struct {
	Name       string
	addr       string
	httpRouter *gin.Engine
	appeared   time.Time

	// local repo
	ghostBank map[string]ghost.Ghost
	local     map[string]*ghost.Ghost
}

func Appear(name, addr string, corsOrigins []string) *Mirage {
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
	g := &Mirage{
		Name:       name,
		addr:       addr,
		httpRouter: r,
		appeared:   time.Now(),
	}
	return g
}

func (g *Mirage) Serve() error {
	g.RegisterRoutesTMP()
	err := g.httpRouter.Run(g.addr)
	return err
}

// refresh Ghosts, update local repo
func (g *Mirage) RefreshGhosts() error {
	return nil
}

func (g *Mirage) Ghost(id string) (ghost.Ghost, bool) {
	ghost, ok := g.ghostBank[id]
	return ghost, ok
}

func (g *Mirage) Ghosts() []ghost.Ghost {
	ghosts := make([]ghost.Ghost, 0, len(g.ghostBank))
	for _, ghost := range g.ghostBank {
		ghosts = append(ghosts, ghost)
	}
	return ghosts
}

func (g *Mirage) CreateLocalGhost(id string, basePath string) *ghost.Ghost {
	if g.local == nil {
		g.local = map[string]*ghost.Ghost{}
	}
	if g.ghostBank == nil {
		g.ghostBank = map[string]ghost.Ghost{}
	}
	if existing, ok := g.local[id]; ok {
		return existing
	}
	localGhost := ghost.Attach(id, g.httpRouter, basePath, nil)
	if registered, ok := g.ghostBank[id]; ok {
		localGhost.Host = registered.Host
		localGhost.Addr = registered.Addr
		localGhost.Group = registered.Group
		localGhost.Exec = registered.Exec
		localGhost.Seeds = registered.Seeds
		localGhost.Auth = registered.Auth
	} else {
		localGhost.Host = "localhost"
		localGhost.Addr = g.addr
	}
	g.local[id] = localGhost
	g.ghostBank[id] = *localGhost
	log.Info().
		Str("ghost", localGhost.ID).
		Str("base_path", basePath).
		Msg("local ghost attached")
	return localGhost
}

func (g *Mirage) LocalGhost(id string) (*ghost.Ghost, bool) {
	if g.local == nil {
		return nil, false
	}
	local, ok := g.local[id]
	return local, ok
}

func (g *Mirage) NodeID() string {
	return g.Name
}

func (g *Mirage) Kind() string {
	return "mirage"
}

func (g *Mirage) HTTPRouter() *gin.Engine {
	return g.httpRouter
}

var _ node.Node = (*Mirage)(nil)

func (g *Mirage) LoadGhosts(ghosts []ghost.Ghost) {
	if g.ghostBank == nil {
		g.ghostBank = map[string]ghost.Ghost{}
	}
	for _, ghost := range ghosts {
		g.ghostBank[ghost.ID] = ghost
		log.Info().
			Str("ghost", ghost.ID).
			Str("addr", ghost.Addr).
			Str("host", ghost.Host).
			Bool("exec", ghost.Exec).
			Msg("ghost registered")
	}
}

func normalizeOrigins(origins []string) []string {
	if len(origins) == 0 {
		return []string{"http://localhost:3000"}
	}
	return origins
}
