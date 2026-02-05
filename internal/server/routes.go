package server

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/seed"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
)

func (g *Ghost) RegisterRoutesTMP() {
	g.httpRouter.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"uptime":  time.Since(g.appeared).String(),
			"service": "edge-api",
			"version": "0.0.1",
		})
	})

	g.httpRouter.GET("/metrics", gin.WrapH(promhttp.Handler()))

	g.httpRouter.GET("/ready", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"ready":   true,
			"uptime":  time.Since(g.appeared).String(),
			"service": "edge-api",
			"version": "0.0.1",
		})
	})

	g.httpRouter.GET("/health/:service", func(c *gin.Context) {
		service := c.Param("service")
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"uptime":  time.Since(g.appeared).String(),
			"service": service,
			"version": "0.0.1",
		})
	})

	g.httpRouter.GET("/seeds", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"seeds": g.Seeds(),
		})
	})

	g.httpRouter.GET("/seeds/:seed/health", func(c *gin.Context) {
		seedID := c.Param("seed")
		if local, ok := g.LocalSeed(seedID); ok {
			respondSeedHealth(c, local)
			return
		}

		seed, ok := g.Seed(seedID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "seed not found"})
			return
		}
		proxySeedRequest(c, g.Name, seed, http.MethodGet, "/health")
	})

	g.httpRouter.GET("/seeds/:seed/ready", func(c *gin.Context) {
		seedID := c.Param("seed")
		if local, ok := g.LocalSeed(seedID); ok {
			respondSeedReady(c, local)
			return
		}

		seed, ok := g.Seed(seedID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "seed not found"})
			return
		}
		proxySeedRequest(c, g.Name, seed, http.MethodGet, "/ready")
	})

	g.httpRouter.GET("/seeds/:seed/services", func(c *gin.Context) {
		seedID := c.Param("seed")
		if local, ok := g.LocalSeed(seedID); ok {
			c.JSON(http.StatusOK, gin.H{
				"services": local.ListServices(),
			})
			return
		}

		seed, ok := g.Seed(seedID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "seed not found"})
			return
		}
		proxySeedRequest(c, g.Name, seed, http.MethodGet, "/services")
	})

	g.httpRouter.GET("/seeds/:seed/metrics", func(c *gin.Context) {
		seedID := c.Param("seed")
		if _, ok := g.LocalSeed(seedID); ok {
			promhttp.Handler().ServeHTTP(c.Writer, c.Request)
			return
		}

		seed, ok := g.Seed(seedID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "seed not found"})
			return
		}

		proxySeedRequest(c, g.Name, seed, http.MethodGet, "/metrics")
	})

	g.httpRouter.POST("/seeds/:seed/services/:service/actions/:action", func(c *gin.Context) {
		seedID := c.Param("seed")
		serviceName := c.Param("service")
		actionName := c.Param("action")

		if local, ok := g.LocalSeed(seedID); ok {
			out, err := local.ExecuteAction(serviceName, actionName)
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, seed.ErrServiceNotFound) || errors.Is(err, seed.ErrActionNotFound) {
					status = http.StatusNotFound
				}
				c.JSON(status, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ok", "output": out})
			return
		}

		seed, ok := g.Seed(seedID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "seed not found"})
			return
		}

		proxySeedRequest(c, g.Name, seed, http.MethodPost, "/services/"+serviceName+"/actions/"+actionName)
	})

	g.httpRouter.GET("/reload", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
		})
	})
}

func proxySeedRequest(c *gin.Context, nodeName string, seed seed.Seed, method string, path string) {
	start := time.Now()
	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := seedBaseURL(seed)
	url := baseURL + path
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to build seed request"})
		observability.RecordSeedProxy(nodeName, seed.ID, method, path, http.StatusBadGateway, time.Since(start), false)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		log.Error().
			Str("seed", seed.ID).
			Str("method", method).
			Str("url", url).
			Err(err).
			Msg("seed_proxy_failed")
		observability.RecordSeedProxy(nodeName, seed.ID, method, path, http.StatusBadGateway, time.Since(start), false)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read seed response"})
		observability.RecordSeedProxy(nodeName, seed.ID, method, path, http.StatusBadGateway, time.Since(start), false)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
	log.Info().
		Str("seed", seed.ID).
		Str("method", method).
		Str("url", url).
		Int("status", resp.StatusCode).
		Msg("seed_proxy")
	observability.RecordSeedProxy(nodeName, seed.ID, method, path, resp.StatusCode, time.Since(start), resp.StatusCode < 400)
}

func seedBaseURL(seed seed.Seed) string {
	addr := strings.TrimSpace(seed.Addr)
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	host := strings.TrimSpace(seed.Host)
	if host == "" {
		host = "localhost"
	}
	if strings.HasPrefix(addr, ":") {
		return "http://" + host + addr
	}
	return "http://" + addr
}

func respondSeedHealth(c *gin.Context, local *seed.Seed) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"uptime":  time.Since(local.Appeared).String(),
		"service": local.ID,
		"version": "0.0.1",
	})
}

func respondSeedReady(c *gin.Context, local *seed.Seed) {
	c.JSON(http.StatusOK, gin.H{
		"ready":   true,
		"uptime":  time.Since(local.Appeared).String(),
		"service": local.ID,
		"version": "0.0.1",
	})
}
