package main

import (
	"fmt"
	"os"

	"github.com/danmuck/edgectl/internal/ghost"
)

func main() {
	svc := ghost.NewService()
	if err := svc.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ghostctl: %v\n", err)
		os.Exit(1)
	}
}
