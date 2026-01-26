package node

import "github.com/gin-gonic/gin"

type Node interface {
	NodeID() string
	Kind() string
	HTTPRouter() *gin.Engine
}
