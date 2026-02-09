package main

// menu_config.go contains the config runtime toggle menu and reset-to-defaults flow.

import (
	"errors"
	"fmt"
	"strings"

	logs "github.com/danmuck/smplog"
)

// runConfigMenu centralizes client runtime toggles and persistence actions.
func (a *App) runConfigMenu() error {
	for {
		fmt.Println()
		fmt.Println("Config Menu")
		fmt.Printf("  clear_screen_after_command: %v\n", a.clearScreen)
		fmt.Printf("  ghost config:  %s\n", a.ghostCfgPath)
		fmt.Printf("  mirage config: %s\n", a.mirageCfgPath)
		fmt.Println("  1) Toggle clear-screen")
		fmt.Println("  2) Save configs")
		fmt.Println("  3) Reset configs to defaults")
		fmt.Println("  4) Back")
		choice, err := a.promptInt("Choose", 1, 4, true, true)
		if err != nil {
			if errors.Is(err, ErrNavigateBack) {
				return nil
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			a.clearScreen = !a.clearScreen
			a.ghostCfg.ClearScreenAfterCommand = a.clearScreen
			logs.Infof("clear_screen_after_command=%v", a.clearScreen)
		case 2:
			if err := a.saveConfigs(); err != nil {
				logs.Errf("save failed: %v", err)
			} else {
				logs.Infof("config saved")
			}
		case 3:
			if err := a.resetToDefaultConfig(); err != nil {
				logs.Errf("reset config failed: %v", err)
			}
		case 4:
			return nil
		}
	}
}

// resetToDefaultConfig removes stale targets and restores baseline files.
func (a *App) resetToDefaultConfig() error {
	confirm, err := a.promptLine("Type RESET to confirm")
	if err != nil {
		return err
	}
	if strings.TrimSpace(confirm) != "RESET" {
		return errors.New("reset cancelled")
	}
	a.ghostCfg = ghostConfigFile{
		ClearScreenAfterCommand: false,
		Targets: []ghostTargetConfig{
			{Name: "local-ghost", Addr: "127.0.0.1:7010", GhostID: "ghost.local"},
		},
	}
	a.mirageCfg = mirageConfigFile{
		Targets: []mirageTargetConfig{
			{Name: "local-mirage", Addr: "127.0.0.1:7020", MirageID: "mirage.local"},
		},
		LocalGhostID:        "ghost.local",
		LocalGhostAdminAddr: "127.0.0.1:7010",
	}
	a.closeMirageTargets()
	a.targets = []GhostTarget{{Name: "local-ghost", Admin: NewRemoteGhostAdmin("127.0.0.1:7010")}}
	a.mirageTargets = []MirageTarget{{Name: "local-mirage", Admin: NewRemoteMirageAdmin("127.0.0.1:7020")}}
	a.activeTarget = 0
	a.activeMirage = 0
	a.clearScreen = false
	return a.saveConfigs()
}
