package main

import (
	"os"

	"github.com/danmuck/edgectl/internal/logging"
	"github.com/danmuck/edgectl/internal/mirage"
	logs "github.com/danmuck/smplog"
)

func main() {
	logging.ConfigureRuntime()
	svc := mirage.NewService()
	if err := svc.Run(); err != nil {
		logs.Errf("miragectl: %v", err)
		os.Exit(1)
	}
}
