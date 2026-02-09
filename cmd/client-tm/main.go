package main

import (
	"errors"
	"flag"
	"os"

	"github.com/danmuck/edgectl/internal/logging"
	logs "github.com/danmuck/smplog"
)

const (
	ghostConfigPath  = "cmd/client-tm/ghost.toml"
	mirageConfigPath = "cmd/client-tm/mirage.toml"
)

var (
	// ErrNavigateBack signals caller-intent to return to the previous menu.
	ErrNavigateBack = errors.New("navigate back")
	// ErrNavigateExit signals caller-intent to exit the interactive client.
	ErrNavigateExit = errors.New("navigate exit")
)

func main() {
	var mode string
	flag.StringVar(&mode, "mode", "ghost", "client mode: ghost or mirage")
	flag.Parse()

	logging.ConfigureRuntime()
	app := NewApp(ghostConfigPath, mirageConfigPath, mode)
	if err := app.Run(); err != nil {
		logs.Errf("client-tm: %v", err)
		os.Exit(1)
	}
}
