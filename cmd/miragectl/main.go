package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/danmuck/edgectl/internal/ghost"
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
	mirageCfg, ghostCfg, err := loadRuntimeConfigs(configPath)
	if err != nil {
		logs.Errf("miragectl: %v", err)
		os.Exit(1)
	}

	errCh := make(chan error, 2)
	ghostSvc := ghost.NewServiceWithConfig(ghostCfg)
	go func() {
		if err := ghostSvc.Run(); err != nil {
			errCh <- fmt.Errorf("managed local ghost failed: %w", err)
			return
		}
		errCh <- nil
	}()

	mirageSvc := mirage.NewServiceWithConfig(mirageCfg)
	go func() {
		if err := mirageSvc.Run(); err != nil {
			errCh <- fmt.Errorf("mirage runtime failed: %w", err)
			return
		}
		errCh <- nil
	}()

	if err := <-errCh; err != nil {
		logs.Errf("miragectl: %v", err)
		os.Exit(1)
	}
}
