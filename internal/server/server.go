package server

import (
	"time"

	"github.com/danmuck/edgectl/internal/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Seeds are server nodes on the network
// Ghost can execute commands on registered seeds
type Seed struct {
	host     string
	addr     string
	registry *services.ServiceRegistry
	exec     bool
	group    string
	auth     string
	appeared time.Time
}

// // Services registered with a Seed
// type ServiceRegistry struct {
// 	repo map[string]*services.Service
// }

// Ghost:edgectl -- main system brain
type Ghost struct {
	Name       string
	addr       string
	httpRouter *gin.Engine
	appeared   time.Time

	// local repo
	seedBank map[string]Seed
}

func Appear() *Ghost {
	r := gin.New()

	// Middleware: keep it lean
	r.Use(gin.Recovery())
	r.Use(gin.Logger())
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"http://localhost:3000"},
		AllowMethods: []string{"GET"},
		AllowHeaders: []string{"Origin", "Content-Type"},
		MaxAge:       12 * time.Hour,
	}))
	g := &Ghost{
		Name:       "edge-ctl",
		addr:       ":9000",
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
	// tmp := g.seedBank
	g.seedBank = map[string]Seed{
		"edge-ctl": {
			host:     "local",
			addr:     g.addr,
			registry: services.NewServiceRegistry(),
			auth:     "temp-auth-key",
			group:    "root",
			exec:     true,
		},
		"pihole": {
			host:     "dps-server",
			addr:     "dps-server",
			registry: nil,
			auth:     "temp-auth-pihole-key",
			group:    "root",
			exec:     true,
		},
	}
	g.seedBank["edge-ctl"].registry.Register(&services.AdminCommands{})
	return nil
}
