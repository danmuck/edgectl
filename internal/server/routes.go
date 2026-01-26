package server

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/seed"
	"github.com/gin-gonic/gin"
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
		seed, ok := g.Seed(c.Param("seed"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "seed not found"})
			return
		}
		proxySeedRequest(c, seed, "/health")
	})

	g.httpRouter.GET("/seeds/:seed/ready", func(c *gin.Context) {
		seed, ok := g.Seed(c.Param("seed"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "seed not found"})
			return
		}
		proxySeedRequest(c, seed, "/ready")
	})

	g.httpRouter.GET("/seeds/:seed/services", func(c *gin.Context) {
		seed, ok := g.Seed(c.Param("seed"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "seed not found"})
			return
		}
		proxySeedRequest(c, seed, "/services")
	})

	g.httpRouter.GET("/reload", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
		})
	})
}

func proxySeedRequest(c *gin.Context, seed seed.Seed, path string) {
	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := seedBaseURL(seed)
	resp, err := client.Get(baseURL + path)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read seed response"})
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, body)
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
