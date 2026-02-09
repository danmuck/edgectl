package main

// targets.go contains Ghost and Mirage target management (list, add, remove, select, close).

import (
	"errors"
	"fmt"
	"strings"

	"github.com/danmuck/edgectl/internal/ghost"
	logs "github.com/danmuck/smplog"
)

func (a *App) activeMirageTarget() (MirageTarget, bool) {
	if a.activeMirage < 0 || a.activeMirage >= len(a.mirageTargets) {
		return MirageTarget{}, false
	}
	return a.mirageTargets[a.activeMirage], true
}

func (a *App) active() (GhostTarget, bool) {
	if a.activeTarget < 0 || a.activeTarget >= len(a.targets) {
		return GhostTarget{}, false
	}
	return a.targets[a.activeTarget], true
}

func (a *App) listTargets() {
	fmt.Println()
	fmt.Println("Ghost Targets")
	if len(a.targets) == 0 {
		fmt.Println("  (none)")
		return
	}
	for i := range a.targets {
		target := a.targets[i]
		marker := " "
		if a.activeTarget == i {
			marker = "*"
		}
		status, err := target.Admin.Status()
		if err != nil {
			fmt.Printf("  %s [%d] %s addr=%s (status err: %v)\n", marker, i+1, target.Name, target.Admin.Address(), err)
			continue
		}
		fmt.Printf(
			"  %s [%d] %s addr=%s ghost_id=%s phase=%s seeds=%d\n",
			marker,
			i+1,
			target.Name,
			target.Admin.Address(),
			status.GhostID,
			status.Phase,
			status.SeedCount,
		)
	}
}

func (a *App) addGhostTarget() error {
	nameRaw, err := a.promptLine("Target suffix/name")
	if err != nil {
		return err
	}
	addrRaw, err := a.promptLine("Ghost admin addr (host:port)")
	if err != nil {
		return err
	}
	name := strings.TrimSpace(nameRaw)
	addr := strings.TrimSpace(addrRaw)
	if name == "" || addr == "" {
		return errors.New("name and addr are required")
	}
	root, ok := a.active()
	if !ok {
		return errors.New("no active root ghost target selected")
	}
	rootStatus, err := root.Admin.Status()
	if err != nil {
		return fmt.Errorf("active root ghost unavailable: %w", err)
	}
	addr, err = normalizeTargetAddr(root.Admin.Address(), addr)
	if err != nil {
		return err
	}

	suffix := normalizeSuffix(name)
	targetName := normalizeSuffix(root.Name) + "." + suffix
	ghostID := rootStatus.GhostID + "." + suffix
	if a.targetExists(targetName, addr) {
		return fmt.Errorf("target exists name=%q addr=%q", targetName, addr)
	}

	spawnReq := ghost.SpawnGhostRequest{
		TargetName: suffix,
		AdminAddr:  addr,
	}
	spawnOut, spawnErr := root.Admin.SpawnGhost(spawnReq)
	if spawnErr != nil {
		return fmt.Errorf("provision ghost target failed: %w", spawnErr)
	}
	ghostID = spawnOut.GhostID
	addr = spawnOut.AdminAddr

	cfg := ghostTargetConfig{Name: targetName, Addr: addr, GhostID: ghostID}
	a.ghostCfg.Targets = append(a.ghostCfg.Targets, cfg)
	a.targets = append(a.targets, GhostTarget{Name: targetName, Admin: NewRemoteGhostAdmin(addr)})
	if a.activeTarget < 0 {
		a.activeTarget = 0
	}
	logs.Infof("provisioned ghost target name=%q ghost_id=%q addr=%q", targetName, ghostID, addr)
	return a.saveConfigs()
}

// removeGhostTarget deletes one target from runtime and persisted config.
func (a *App) removeGhostTarget() error {
	if len(a.targets) == 0 {
		return errors.New("no targets to remove")
	}
	a.listTargets()
	choice, err := a.promptInt("Remove target", 1, len(a.targets), true, true)
	if err != nil {
		return err
	}
	idx := choice - 1
	name := a.targets[idx].Name
	admin := a.targets[idx].Admin
	a.targets = append(a.targets[:idx], a.targets[idx+1:]...)
	a.ghostCfg.Targets = append(a.ghostCfg.Targets[:idx], a.ghostCfg.Targets[idx+1:]...)
	_ = admin.Close()
	if len(a.targets) == 0 {
		a.activeTarget = -1
	} else if a.activeTarget >= len(a.targets) {
		a.activeTarget = len(a.targets) - 1
	}
	logs.Infof("removed target name=%q", name)
	return a.saveConfigs()
}

func (a *App) selectActiveTarget() error {
	if len(a.targets) == 0 {
		return errors.New("no targets available")
	}
	a.listTargets()
	choice, err := a.promptInt("Select target", 1, len(a.targets), true, true)
	if err != nil {
		return err
	}
	a.activeTarget = choice - 1
	logs.Infof("active target set name=%q", a.targets[a.activeTarget].Name)
	return nil
}

func (a *App) targetExists(name string, addr string) bool {
	for _, t := range a.ghostCfg.Targets {
		if strings.EqualFold(strings.TrimSpace(t.Name), strings.TrimSpace(name)) {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(t.Addr), strings.TrimSpace(addr)) {
			return true
		}
	}
	return false
}

func (a *App) closeTargets() {
	for _, t := range a.targets {
		_ = t.Admin.Close()
	}
}

func (a *App) closeMirageTargets() {
	for _, t := range a.mirageTargets {
		_ = t.Admin.Close()
	}
}
