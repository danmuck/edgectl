package server

import "github.com/gin-gonic/gin"

type Display int

const (
	DisplayDefault Display = iota
	ReactMain              = 1
	TUI                    = 2
)

type DisplAPI_Server struct {
	name    string
	port    string
	display *Display
	router  *gin.Engine
}

type Ghost struct {
	netedge *DisplAPI_Server
}
