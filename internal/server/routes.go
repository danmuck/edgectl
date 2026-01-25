package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (g *Ghost) RegisterRoutesTMP() {
	g.httpRouter.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"uptime":  g.appeared,
			"service": "edge-api",
			"version": "0.0.1",
		})
	})

	g.httpRouter.GET("/health/:service", func(c *gin.Context) {
		service := c.Param("service")
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"uptime":  g.appeared,
			"service": service,
			"version": "0.0.1",
		})
	})

	g.httpRouter.GET("/reboot", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
		})
	})
}
