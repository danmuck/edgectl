package main

import (
	"fmt"

	"github.com/danmuck/edgectl/internal/server"
)

// var startedAt = time.Now()

func main() {
	ghost := server.Appear()
	fmt.Println("A Ghost has appeared ... " + ghost.Name)
	ghost.Serve()
}
