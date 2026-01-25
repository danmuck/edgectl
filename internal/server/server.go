package server

import (
	"time"

	"github.com/danmuck/edgectl/internal/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

type Seed struct {
	commands map[string]*services.Service
}

// edgectl main brain
type Ghost struct {
	name       string
	netedge    any
	httpRouter *gin.Engine
	appeared   time.Time
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
		name:       "edge-ctl",
		netedge:    nil,
		httpRouter: r,
		appeared:   time.Now(),
	}
	return g
}
