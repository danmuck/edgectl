package main

import (
	"os"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/logging"
	logs "github.com/danmuck/smplog"
)

func main() {
	logging.ConfigureRuntime()
	svc := ghost.NewService()
	if err := svc.Run(); err != nil {
		logs.Errf("ghostctl: %v", err)
		os.Exit(1)
	}
}
