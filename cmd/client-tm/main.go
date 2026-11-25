package main

import (
	"errors"
	"flag"
	"os"

	"github.com/danmuck/edgectl/internal/logging"
	logs "github.com/danmuck/smplog"
)

const (
	ghostConfigPath  = "cmd/client-tm/a.ghost.toml"
	mirageConfigPath = "cmd/client-tm/a.mirage.toml"
)

var (
	// ErrNavigateBack signals caller-intent to return to the previous menu.
	ErrNavigateBack = errors.New("navigate back")
	// ErrNavigateExit signals caller-intent to exit the interactive client.
	ErrNavigateExit = errors.New("navigate exit")
)

func main() {
	var mode string
	var labDir string
	flag.StringVar(&mode, "mode", "mirage", "client mode: mirage (default), ghost (headless operations), or lab (start a local stack)")
	flag.StringVar(&labDir, "lab-dir", labDefaultDir, "lab mode: directory for the throwaway stack's files")
	flag.Parse()

	logging.ConfigureRuntime()
	// Lab mode owns its own Mirage process and configs (docs/progress/lab_mode.md).
	if normalizeClientMode(mode) == "lab" {
		if err := runLabMode(labDir); err != nil {
			logs.Errf("client-tm lab: %v", err)
			os.Exit(1)
		}
		return
	}
	app := NewApp(ghostConfigPath, mirageConfigPath, mode)
	if err := app.Run(); err != nil {
		logs.Errf("client-tm: %v", err)
		os.Exit(1)
	}
}
