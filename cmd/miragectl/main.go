package main

import (
	"flag"
	"os"

	"github.com/danmuck/edgectl/internal/logging"
	"github.com/danmuck/edgectl/internal/mirage"
	logs "github.com/danmuck/smplog"
)

// miragectl entrypoint that loads config and runs Mirage runtime.
func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "cmd/miragectl/config.toml", "path to miragectl config.toml")
	flag.Parse()

	logging.ConfigureRuntime()
	cfg, err := loadServiceConfig(configPath)
	if err != nil {
		logs.Errf("miragectl: %v", err)
		os.Exit(1)
	}

	svc := mirage.NewServiceWithConfig(cfg)
	if err := svc.Run(); err != nil {
		logs.Errf("miragectl: %v", err)
		os.Exit(1)
	}
}
