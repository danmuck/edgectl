package main

import (
	"fmt"
	"os"

	"github.com/danmuck/edgectl/internal/mirage"
)

func main() {
	svc := mirage.NewService()
	if err := svc.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "miragectl: %v\n", err)
		os.Exit(1)
	}
}
