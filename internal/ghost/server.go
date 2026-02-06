package ghost

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/danmuck/edgectl/internal/node"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

type Ghost struct {
	ID       string              `json:"id"`
	Host     string              `json:"host"`
	Addr     string              `json:"addr"`
	Group    string              `json:"group"`
	Exec     bool                `json:"exec"`
	Seeds    []string            `json:"seeds,omitempty"`
	Appeared time.Time           `json:"appeared"`
	Auth     string              `json:"-"`
	Registry *seeds.SeedRegistry `json:"-"`

	router   *gin.Engine
	basePath string
}

var _ node.Node = (*Ghost)(nil)

func Appear(id, addr string, corsOrigins []string) *Ghost {
	observability.RegisterMetrics()
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(observability.RequestLogger(log.Logger))
	r.Use(observability.RequestMetricsMiddleware(id))
	r.Use(cors.New(cors.Config{
		AllowOrigins: normalizeOrigins(corsOrigins),
		AllowMethods: []string{"GET", "POST"},
		AllowHeaders: []string{"Origin", "Content-Type"},
		MaxAge:       12 * time.Hour,
	}))
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})

	return &Ghost{
		ID:       id,
		Addr:     addr,
		Exec:     true,
		router:   r,
		Registry: seeds.NewSeedRegistry(),
		Appeared: time.Now(),
	}
}

func Attach(id string, router *gin.Engine, basePath string, registry *seeds.SeedRegistry) *Ghost {
	if registry == nil {
		registry = seeds.NewSeedRegistry()
	}
	return &Ghost{
		ID:       id,
		Exec:     true,
		router:   router,
		basePath: basePath,
		Registry: registry,
		Appeared: time.Now(),
	}
}

func (s *Ghost) NodeID() string {
	return s.ID
}

func (s *Ghost) Kind() string {
	return "ghost"
}

func (s *Ghost) HTTPRouter() *gin.Engine {
	return s.router
}

func (s *Ghost) RegisterRoutes() {
	routes := s.routes()
	routes.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"uptime":  time.Since(s.Appeared).String(),
			"ghost":   s.ID,
			"version": "0.0.1",
		})
	})

	routes.GET("/metrics", gin.WrapH(promhttp.Handler()))

	routes.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"ready":   true,
			"uptime":  time.Since(s.Appeared).String(),
			"ghost":   s.ID,
			"version": "0.0.1",
		})
	})

	routes.GET("/seeds", func(c *gin.Context) {
		seedsList := s.ListSeeds()
		c.JSON(http.StatusOK, gin.H{
			"seeds": seedsList,
		})
	})

	routes.POST("/seeds/:seed/actions/:action", func(c *gin.Context) {
		seedName := c.Param("seed")
		actionName := c.Param("action")

		out, err := s.ExecuteAction(seedName, actionName)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrSeedNotFound) || errors.Is(err, ErrActionNotFound) {
				status = http.StatusNotFound
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "output": out})
	})
}

var (
	ErrSeedNotFound   = errors.New("seed not found")
	ErrActionNotFound = errors.New("action not found")
)

func (s *Ghost) ExecuteAction(seedName, actionName string) (string, error) {
	registry := s.registry()
	seed, ok := registry.Get(seedName)
	if !ok || seed == nil {
		return "", ErrSeedNotFound
	}

	action, ok := seed.Actions()[actionName]
	if !ok {
		return "", ErrActionNotFound
	}

	out, err := action()
	if err != nil {
		log.Error().
			Str("ghost", s.ID).
			Str("seed", seedName).
			Str("action", actionName).
			Err(err).
			Msg("seed action failed")
		return "", err
	}

	log.Info().
		Str("ghost", s.ID).
		Str("seed", seedName).
		Str("action", actionName).
		Msg("seed action executed")
	return out, nil
}

func (s *Ghost) ListSeeds() []SeedInfo {
	return listSeeds(s.registry())
}

func (s *Ghost) Serve() error {
	s.RegisterRoutes()
	return s.router.Run(s.Addr)
}

func (s *Ghost) routes() gin.IRoutes {
	if s.basePath == "" {
		return s.router
	}
	return s.router.Group(s.basePath)
}

type SeedInfo struct {
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

func listSeeds(registry *seeds.SeedRegistry) []SeedInfo {
	if registry == nil {
		return nil
	}
	entries := registry.All()
	list := make([]SeedInfo, 0, len(entries))
	for name, seed := range entries {
		if seed == nil {
			continue
		}
		actions := make([]string, 0, len(seed.Actions()))
		for action := range seed.Actions() {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		list = append(list, SeedInfo{
			Name:    name,
			Actions: actions,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

func (s *Ghost) registry() *seeds.SeedRegistry {
	if s.Registry == nil {
		s.Registry = seeds.NewSeedRegistry()
	}
	return s.Registry
}

func normalizeOrigins(origins []string) []string {
	if len(origins) == 0 {
		return []string{"http://localhost:3000"}
	}
	return origins
}
