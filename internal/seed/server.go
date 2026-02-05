package seed

import (
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/danmuck/edgectl/internal/node"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

type Seed struct {
	ID       string                    `json:"id"`
	Host     string                    `json:"host"`
	Addr     string                    `json:"addr"`
	Group    string                    `json:"group"`
	Exec     bool                      `json:"exec"`
	Services []string                  `json:"services,omitempty"`
	Appeared time.Time                 `json:"appeared"`
	Auth     string                    `json:"-"`
	Registry *services.ServiceRegistry `json:"-"`

	router   *gin.Engine
	basePath string
}

var _ node.Node = (*Seed)(nil)

func Appear(id, addr string, corsOrigins []string) *Seed {
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

	return &Seed{
		ID:       id,
		Addr:     addr,
		Exec:     true,
		router:   r,
		Registry: services.NewServiceRegistry(),
		Appeared: time.Now(),
	}
}

func Attach(id string, router *gin.Engine, basePath string, registry *services.ServiceRegistry) *Seed {
	if registry == nil {
		registry = services.NewServiceRegistry()
	}
	return &Seed{
		ID:       id,
		Exec:     true,
		router:   router,
		basePath: basePath,
		Registry: registry,
		Appeared: time.Now(),
	}
}

func (s *Seed) NodeID() string {
	return s.ID
}

func (s *Seed) Kind() string {
	return "seed"
}

func (s *Seed) HTTPRouter() *gin.Engine {
	return s.router
}

func (s *Seed) RegisterRoutes() {
	routes := s.routes()
	routes.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"uptime":  time.Since(s.Appeared).String(),
			"service": s.ID,
			"version": "0.0.1",
		})
	})

	routes.GET("/metrics", gin.WrapH(promhttp.Handler()))

	routes.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"ready":   true,
			"uptime":  time.Since(s.Appeared).String(),
			"service": s.ID,
			"version": "0.0.1",
		})
	})

	routes.GET("/services", func(c *gin.Context) {
		servicesList := s.ListServices()
		c.JSON(http.StatusOK, gin.H{
			"services": servicesList,
		})
	})

	routes.POST("/services/:service/actions/:action", func(c *gin.Context) {
		serviceName := c.Param("service")
		actionName := c.Param("action")

		out, err := s.ExecuteAction(serviceName, actionName)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrServiceNotFound) || errors.Is(err, ErrActionNotFound) {
				status = http.StatusNotFound
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok", "output": out})
	})
}

var (
	ErrServiceNotFound = errors.New("service not found")
	ErrActionNotFound  = errors.New("action not found")
)

func (s *Seed) ExecuteAction(serviceName, actionName string) (string, error) {
	registry := s.registry()
	service, ok := registry.Get(serviceName)
	if !ok || service == nil {
		return "", ErrServiceNotFound
	}

	action, ok := service.Actions()[actionName]
	if !ok {
		return "", ErrActionNotFound
	}

	out, err := action()
	if err != nil {
		log.Error().
			Str("seed", s.ID).
			Str("service", serviceName).
			Str("action", actionName).
			Err(err).
			Msg("service action failed")
		return "", err
	}

	log.Info().
		Str("seed", s.ID).
		Str("service", serviceName).
		Str("action", actionName).
		Msg("service action executed")
	return out, nil
}

func (s *Seed) ListServices() []ServiceInfo {
	return listServices(s.registry())
}

func (s *Seed) Serve() error {
	s.RegisterRoutes()
	return s.router.Run(s.Addr)
}

func (s *Seed) routes() gin.IRoutes {
	if s.basePath == "" {
		return s.router
	}
	return s.router.Group(s.basePath)
}

type ServiceInfo struct {
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

func listServices(registry *services.ServiceRegistry) []ServiceInfo {
	if registry == nil {
		return nil
	}
	entries := registry.All()
	list := make([]ServiceInfo, 0, len(entries))
	for name, service := range entries {
		if service == nil {
			continue
		}
		actions := make([]string, 0, len(service.Actions()))
		for action := range service.Actions() {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		list = append(list, ServiceInfo{
			Name:    name,
			Actions: actions,
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

func (s *Seed) registry() *services.ServiceRegistry {
	if s.Registry == nil {
		s.Registry = services.NewServiceRegistry()
	}
	return s.Registry
}

func normalizeOrigins(origins []string) []string {
	if len(origins) == 0 {
		return []string{"http://localhost:3000"}
	}
	return origins
}
