package main

import (
	"flag"
	"os"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/logging"
	logs "github.com/danmuck/smplog"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "cmd/ghostctl/config.toml", "path to ghostctl config.toml")
	flag.Parse()

	logging.ConfigureRuntime()
	cfg, err := loadServiceConfig(configPath)
	if err != nil {
		logs.Errf("ghostctl: %v", err)
		os.Exit(1)
	}

	svc := ghost.NewServiceWithConfig(cfg)
	if err := svc.Run(); err != nil {
		logs.Errf("ghostctl: %v", err)
		os.Exit(1)
	}
}
