package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	logs "github.com/danmuck/smplog"
)

func NewApp(ghostCfgPath string, mirageCfgPath string, mode string) *App {
	return &App{
		reader:        bufio.NewReader(os.Stdin),
		ghostCfgPath:  ghostCfgPath,
		mirageCfgPath: mirageCfgPath,
		targets:       make([]GhostTarget, 0),
		activeTarget:  -1,
		mirageTargets: make([]MirageTarget, 0),
		activeMirage:  -1,
		clearScreen:   false,
		launchMode:    normalizeClientMode(mode),
	}
}

func (a *App) printMainMenu() {
	fmt.Println()
	fmt.Println("Client TM")
	fmt.Printf("  ghost config:  %s (targets=%d)\n", a.ghostCfgPath, len(a.ghostCfg.Targets))
	fmt.Printf("  mirage config: %s (targets=%d)\n", a.mirageCfgPath, len(a.mirageCfg.Targets))
	fmt.Printf("  clear screen after command: %v\n", a.clearScreen)
	fmt.Println("  1) List ghost targets")
	fmt.Println("  2) Add/provision ghost target (persist)")
	fmt.Println("  3) Select active ghost target")
	fmt.Println("  4) Show active target summary")
	fmt.Println("  5) Ghost admin console")
	fmt.Println("  6) Remove ghost target")
	fmt.Println("  7) Config menu")
	fmt.Println("  8) Exit")
}

// Run executes the main interactive menu loop.
func (a *App) Run() error {
	if err := a.loadOrInitConfigs(); err != nil {
		return err
	}
	logs.Infof(
		"client-tm loaded ghost_targets=%d mirage_targets=%d",
		len(a.ghostCfg.Targets),
		len(a.mirageCfg.Targets),
	)
	if a.launchMode != "" && a.launchMode != "ghost" && a.launchMode != "mirage" {
		return fmt.Errorf("invalid mode %q (expected ghost or mirage)", a.launchMode)
	}
	if a.launchMode == "mirage" {
		return a.runMirageClientLoop()
	}

	for {
		a.printMainMenu()
		choice, err := a.promptInt("Choose", 1, 8, false, true)
		if err != nil {
			if errors.Is(err, ErrNavigateExit) {
				return a.exitClient()
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			a.listTargets()
		case 2:
			if err := a.addGhostTarget(); err != nil {
				logs.Errf("add target failed: %v", err)
			}
		case 3:
			if err := a.selectActiveTarget(); err != nil {
				if errors.Is(err, ErrNavigateBack) {
					continue
				}
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("select target failed: %v", err)
			}
		case 4:
			a.showActiveTargetSummary()
		case 5:
			if err := a.runGhostAdminConsole(); err != nil {
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("ghost admin console error: %v", err)
			}
		case 6:
			if err := a.removeGhostTarget(); err != nil {
				if errors.Is(err, ErrNavigateBack) {
					continue
				}
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("remove target failed: %v", err)
			}
		case 7:
			if err := a.runConfigMenu(); err != nil {
				if errors.Is(err, ErrNavigateBack) {
					continue
				}
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("config menu failed: %v", err)
			}
		case 8:
			return a.exitClient()
		}
	}
}

// exitClient saves current config and closes active admin connections.
func (a *App) exitClient() error {
	if err := a.saveConfigs(); err != nil {
		logs.Warnf("save on exit failed: %v", err)
	}
	a.closeTargets()
	a.closeMirageTargets()
	logs.Infof("client-tm exiting")
	return nil
}

// loadOrInitConfigs loads persisted files and initializes runtime targets.
func (a *App) loadOrInitConfigs() error {
	if err := ensureFile(a.ghostCfgPath); err != nil {
		return err
	}
	if err := ensureFile(a.mirageCfgPath); err != nil {
		return err
	}

	if _, err := toml.DecodeFile(a.ghostCfgPath, &a.ghostCfg); err != nil {
		return fmt.Errorf("load ghost config: %w", err)
	}
	if _, err := toml.DecodeFile(a.mirageCfgPath, &a.mirageCfg); err != nil {
		return fmt.Errorf("load mirage config: %w", err)
	}
	a.clearScreen = a.ghostCfg.ClearScreenAfterCommand
	needsSave := false

	if len(a.ghostCfg.Targets) == 0 {
		a.ghostCfg.Targets = append(a.ghostCfg.Targets, ghostTargetConfig{
			Name:    "local-ghost",
			Addr:    "127.0.0.1:7010",
			GhostID: "ghost.local",
		})
		needsSave = true
	}
	if len(a.mirageCfg.Targets) == 0 {
		a.mirageCfg.Targets = append(a.mirageCfg.Targets, mirageTargetConfig{
			Name:     "local-mirage",
			Addr:     "127.0.0.1:7020",
			MirageID: "mirage.local",
		})
		needsSave = true
	}
	if len(a.mirageCfg.Targets) > 1 {
		logs.Warnf("client-tm mirage config has %d targets; only first target is supported", len(a.mirageCfg.Targets))
		a.mirageCfg.Targets = a.mirageCfg.Targets[:1]
		needsSave = true
	}
	if strings.TrimSpace(a.mirageCfg.LocalGhostID) == "" {
		a.mirageCfg.LocalGhostID = "ghost.local"
		needsSave = true
	}
	if strings.TrimSpace(a.mirageCfg.LocalGhostAdminAddr) == "" {
		a.mirageCfg.LocalGhostAdminAddr = "127.0.0.1:7010"
		needsSave = true
	}
	for i, cfg := range a.ghostCfg.Targets {
		name := strings.TrimSpace(cfg.Name)
		addr := strings.TrimSpace(cfg.Addr)
		if name == "" || addr == "" {
			continue
		}
		ghostID := strings.TrimSpace(cfg.GhostID)
		admin := NewRemoteGhostAdmin(addr)
		if ghostID == "" {
			if status, err := admin.Status(); err == nil && strings.TrimSpace(status.GhostID) != "" {
				ghostID = strings.TrimSpace(status.GhostID)
			} else {
				ghostID = inferGhostIDFromTargetName(name)
			}
			a.ghostCfg.Targets[i].GhostID = ghostID
			needsSave = true
		}
		a.targets = append(a.targets, GhostTarget{
			Name:  name,
			Admin: admin,
		})
	}
	if len(a.targets) > 0 {
		a.activeTarget = 0
	}
	cfg := a.mirageCfg.Targets[0]
	name := strings.TrimSpace(cfg.Name)
	addr := strings.TrimSpace(cfg.Addr)
	if name == "" || addr == "" {
		return errors.New("mirage config requires non-empty target name and addr")
	}
	mirageID := strings.TrimSpace(cfg.MirageID)
	admin := NewRemoteMirageAdmin(addr)
	if mirageID == "" {
		if status, err := admin.Status(); err == nil && strings.TrimSpace(status.MirageID) != "" {
			mirageID = strings.TrimSpace(status.MirageID)
		} else {
			mirageID = "mirage.local"
		}
		a.mirageCfg.Targets[0].MirageID = mirageID
		needsSave = true
	}
	a.mirageTargets = append(a.mirageTargets, MirageTarget{
		Name:  name,
		Admin: admin,
	})
	if len(a.mirageTargets) > 0 {
		a.activeMirage = 0
	}
	if needsSave {
		if err := a.saveConfigs(); err != nil {
			return err
		}
	}
	return nil
}

// saveConfigs writes current Ghost and Mirage target lists to disk.
func (a *App) saveConfigs() error {
	buf := strings.Builder{}
	if err := toml.NewEncoder(&buf).Encode(a.ghostCfg); err != nil {
		return err
	}
	if err := os.WriteFile(a.ghostCfgPath, []byte(buf.String()), 0o644); err != nil {
		return err
	}

	buf.Reset()
	if err := toml.NewEncoder(&buf).Encode(a.mirageCfg); err != nil {
		return err
	}
	if err := os.WriteFile(a.mirageCfgPath, []byte(buf.String()), 0o644); err != nil {
		return err
	}
	return nil
}
