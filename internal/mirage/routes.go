package mirage

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

func (g *Mirage) RegisterRoutesTMP() {
	g.httpRouter.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"uptime":    time.Since(g.appeared).String(),
			"component": "mirage-api",
			"version":   "0.0.1",
		})
	})

	g.httpRouter.GET("/metrics", gin.WrapH(promhttp.Handler()))

	g.httpRouter.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"ready":     true,
			"uptime":    time.Since(g.appeared).String(),
			"component": "mirage-api",
			"version":   "0.0.1",
		})
	})

	g.httpRouter.GET("/health/:component", func(c *gin.Context) {
		component := c.Param("component")
		// Placeholder for component-specific health checks.
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"uptime":    time.Since(g.appeared).String(),
			"component": component,
			"version":   "0.0.1",
		})
	})

	g.httpRouter.GET("/ghosts", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"ghosts": g.Ghosts(),
		})
	})

	g.httpRouter.GET("/ghosts/:ghost/health", func(c *gin.Context) {
		ghostID := c.Param("ghost")
		if local, ok := g.LocalGhost(ghostID); ok {
			respondGhostHealth(c, local)
			return
		}

		ghost, ok := g.Ghost(ghostID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "ghost not found"})
			return
		}
		proxyGhostRequest(c, g.Name, ghost, http.MethodGet, "/health")
	})

	g.httpRouter.GET("/ghosts/:ghost/ready", func(c *gin.Context) {
		ghostID := c.Param("ghost")
		if local, ok := g.LocalGhost(ghostID); ok {
			respondGhostReady(c, local)
			return
		}

		ghost, ok := g.Ghost(ghostID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "ghost not found"})
			return
		}
		proxyGhostRequest(c, g.Name, ghost, http.MethodGet, "/ready")
	})

	g.httpRouter.GET("/ghosts/:ghost/seeds", func(c *gin.Context) {
		ghostID := c.Param("ghost")
		if local, ok := g.LocalGhost(ghostID); ok {
			c.JSON(http.StatusOK, gin.H{
				"seeds": local.ListSeeds(),
			})
			return
		}

		ghost, ok := g.Ghost(ghostID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "ghost not found"})
			return
		}
		proxyGhostRequest(c, g.Name, ghost, http.MethodGet, "/seeds")
	})

	g.httpRouter.GET("/ghosts/:ghost/metrics", func(c *gin.Context) {
		ghostID := c.Param("ghost")
		if _, ok := g.LocalGhost(ghostID); ok {
			promhttp.Handler().ServeHTTP(c.Writer, c.Request)
			return
		}

		ghost, ok := g.Ghost(ghostID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "ghost not found"})
			return
		}

		proxyGhostRequest(c, g.Name, ghost, http.MethodGet, "/metrics")
	})

	g.httpRouter.POST("/ghosts/:ghost/seeds/:seed/actions/:action", func(c *gin.Context) {
		ghostID := c.Param("ghost")
		seedName := c.Param("seed")
		actionName := c.Param("action")

		if local, ok := g.LocalGhost(ghostID); ok {
			out, err := local.ExecuteAction(seedName, actionName)
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, ghost.ErrSeedNotFound) || errors.Is(err, ghost.ErrActionNotFound) {
					status = http.StatusNotFound
				}
				c.JSON(status, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ok", "output": out})
			return
		}

		ghost, ok := g.Ghost(ghostID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "ghost not found"})
			return
		}

		proxyGhostRequest(c, g.Name, ghost, http.MethodPost, "/seeds/"+seedName+"/actions/"+actionName)
	})

	g.httpRouter.GET("/reload", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
		})
	})
}

func proxyGhostRequest(c *gin.Context, nodeName string, ghost ghost.Ghost, method string, path string) {
	start := time.Now()
	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := ghostBaseURL(ghost)
	url := baseURL + path
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to build ghost request"})
		observability.RecordGhostProxy(nodeName, ghost.ID, method, path, http.StatusBadGateway, time.Since(start), false)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		log.Error().
			Str("ghost", ghost.ID).
			Str("method", method).
			Str("url", url).
			Err(err).
			Msg("ghost_proxy_failed")
		observability.RecordGhostProxy(nodeName, ghost.ID, method, path, http.StatusBadGateway, time.Since(start), false)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read ghost response"})
		observability.RecordGhostProxy(nodeName, ghost.ID, method, path, http.StatusBadGateway, time.Since(start), false)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
	log.Info().
		Str("ghost", ghost.ID).
		Str("method", method).
		Str("url", url).
		Int("status", resp.StatusCode).
		Msg("ghost_proxy")
	observability.RecordGhostProxy(nodeName, ghost.ID, method, path, resp.StatusCode, time.Since(start), resp.StatusCode < 400)
}

func ghostBaseURL(ghost ghost.Ghost) string {
	addr := strings.TrimSpace(ghost.Addr)
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	host := strings.TrimSpace(ghost.Host)
	if host == "" {
		host = "localhost"
	}
	if strings.HasPrefix(addr, ":") {
		return "http://" + host + addr
	}
	return "http://" + addr
}

func respondGhostHealth(c *gin.Context, local *ghost.Ghost) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"uptime":  time.Since(local.Appeared).String(),
		"ghost":   local.ID,
		"version": "0.0.1",
	})
}

func respondGhostReady(c *gin.Context, local *ghost.Ghost) {
	c.JSON(http.StatusOK, gin.H{
		"ready":   true,
		"uptime":  time.Since(local.Appeared).String(),
		"ghost":   local.ID,
		"version": "0.0.1",
	})
}
