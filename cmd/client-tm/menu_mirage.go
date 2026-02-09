package main

// menu_mirage.go contains Mirage admin console menu loops and display functions.

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
	logs "github.com/danmuck/smplog"
)

func (a *App) printMirageMenu() {
	fmt.Println()
	fmt.Println("Client TM (Mirage)")
	fmt.Printf("  ghost config:  %s (targets=%d)\n", a.ghostCfgPath, len(a.ghostCfg.Targets))
	fmt.Printf("  mirage config: %s (single control plane)\n", a.mirageCfgPath)
	fmt.Printf("  clear screen after command: %v\n", a.clearScreen)
	fmt.Println("  (*) not yet fully implemented")
	fmt.Println("  1) Show mirage control-plane config")
	fmt.Println("  2) Show mirage status")
	fmt.Println("  3) Mirage admin console (*)")
	fmt.Println("  4) Show connected ghosts")
	fmt.Println("  5) Open local ghost admin console")
	fmt.Println("  6) Config menu")
	fmt.Println("  7) Exit")
}

// Executes Mirage-first operator navigation.
func (a *App) runMirageClientLoop() error {
	for {
		a.printMirageMenu()
		choice, err := a.promptInt("Choose", 1, 7, false, true)
		if err != nil {
			if errors.Is(err, ErrNavigateExit) {
				return a.exitClient()
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			a.showMirageControlPlaneConfig()
		case 2:
			if err := a.showActiveMirageSummary(); err != nil {
				logs.Errf("show mirage summary failed: %v", err)
			}
		case 3:
			if err := a.runMirageAdminConsole(); err != nil {
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("mirage admin console error: %v", err)
			}
		case 4:
			if err := a.showMirageConnectedGhosts(); err != nil {
				logs.Errf("show connected ghosts failed: %v", err)
			}
		case 5:
			if err := a.openLocalGhostConsole(); err != nil {
				logs.Errf("open local ghost console failed: %v", err)
			}
		case 6:
			if err := a.runConfigMenu(); err != nil {
				if errors.Is(err, ErrNavigateBack) {
					continue
				}
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("config menu failed: %v", err)
			}
		case 7:
			return a.exitClient()
		}
	}
}

func (a *App) runMirageAdminConsole() error {
	target, ok := a.activeMirageTarget()
	if !ok {
		return errors.New("no active mirage target")
	}
	for {
		fmt.Println()
		fmt.Printf("Mirage Admin Console (%s @ %s)\n", target.Name, target.Admin.Address())
		fmt.Println("  (*) not yet fully implemented")
		fmt.Println("  1) Show status")
		fmt.Println("  2) Show available services")
		fmt.Println("  3) Issue intent")
		fmt.Println("  4) Reconcile intent")
		fmt.Println("  5) Reports")
		fmt.Println("  6) Ghost routing")
		fmt.Println("  7) Back")
		choice, err := a.promptInt("Choose", 1, 7, true, true)
		if err != nil {
			if errors.Is(err, ErrNavigateBack) {
				return nil
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			if err := a.showActiveMirageSummary(); err != nil {
				logs.Errf("show mirage summary failed: %v", err)
			}
		case 2:
			if err := a.showMirageAvailableServices(target); err != nil {
				logs.Errf("show available services failed: %v", err)
			}
		case 3:
			if err := a.submitMirageIssue(target); err != nil {
				logs.Errf("issue intent failed: %v", err)
			}
		case 4:
			if err := a.runMirageReconcileConsole(target); err != nil {
				logs.Errf("reconcile console failed: %v", err)
			}
		case 5:
			if err := a.runMirageReportsConsole(target); err != nil {
				logs.Errf("reports console failed: %v", err)
			}
		case 6:
			if err := a.runMirageGhostRoutingConsole(target); err != nil {
				logs.Errf("ghost routing console failed: %v", err)
			}
		case 7:
			return nil
		}
	}
}

func (a *App) showMirageControlPlaneConfig() {
	target, ok := a.activeMirageTarget()
	if !ok {
		fmt.Println("No configured mirage target.")
		return
	}
	fmt.Println()
	fmt.Println("Mirage Control-Plane Config")
	fmt.Printf("  mirage name:        %s\n", target.Name)
	fmt.Printf("  mirage admin addr:  %s\n", target.Admin.Address())
	fmt.Printf("  local ghost id:     %s\n", strings.TrimSpace(a.mirageCfg.LocalGhostID))
	fmt.Printf("  local ghost admin:  %s\n", strings.TrimSpace(a.mirageCfg.LocalGhostAdminAddr))
}

func (a *App) showActiveMirageSummary() error {
	target, ok := a.activeMirageTarget()
	if !ok {
		return errors.New("no active mirage target")
	}
	status, err := target.Admin.Status()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Printf("Active Mirage Target: %s\n", target.Name)
	fmt.Printf("  addr:      %s\n", target.Admin.Address())
	fmt.Printf("  mirage_id: %s\n", status.MirageID)
	fmt.Printf("  phase:     %s\n", status.Phase)
	fmt.Printf("  ghosts:    %d\n", status.RegisteredGhosts)
	fmt.Printf("  intents:   %d\n", status.ActiveIntents)
	fmt.Printf("  reports:   %d\n", status.ReportCount)
	fmt.Printf("  local_ghost_id:    %s\n", strings.TrimSpace(a.mirageCfg.LocalGhostID))
	fmt.Printf("  local_ghost_admin: %s\n", strings.TrimSpace(a.mirageCfg.LocalGhostAdminAddr))
	return nil
}

func (a *App) showMirageConnectedGhosts() error {
	target, ok := a.activeMirageTarget()
	if !ok {
		return errors.New("no active mirage target")
	}
	ghosts, err := target.Admin.RegisteredGhosts()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Connected Ghosts")
	if len(ghosts) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for i := range ghosts {
		g := ghosts[i]
		fmt.Printf(
			"  [%d] ghost_id=%s connected=%v remote=%s seeds=%d events=%d\n",
			i+1,
			g.GhostID,
			g.Connected,
			g.RemoteAddr,
			len(g.SeedList),
			g.EventCount,
		)
	}
	return nil
}

func (a *App) showMirageRoutingTable(target MirageTarget) error {
	routes, err := target.Admin.RoutingTable()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Mirage Routing Table")
	if len(routes) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for i := range routes {
		route := routes[i]
		fmt.Printf(
			"  [%d] ghost_id=%s admin_addr=%s connected=%v\n",
			i+1,
			route.GhostID,
			route.AdminAddr,
			route.Connected,
		)
	}
	return nil
}

func (a *App) showMirageAvailableServices(target MirageTarget) error {
	services, err := target.Admin.AvailableServices()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Mirage Available Services")
	if len(services) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for i := range services {
		svc := services[i]
		fmt.Printf("  [%d] seed=%s ghosts=%s\n", i+1, svc.SeedID, strings.Join(svc.GhostIDs, ","))
	}
	return nil
}

// openLocalGhostConsole opens the local ghost admin console configured for this Mirage control plane.
func (a *App) openLocalGhostConsole() error {
	target, ok := a.activeMirageTarget()
	if !ok {
		return errors.New("no active mirage target")
	}
	localGhostID := strings.TrimSpace(a.mirageCfg.LocalGhostID)
	localGhostAddr := strings.TrimSpace(a.mirageCfg.LocalGhostAdminAddr)
	if localGhostID == "" {
		return errors.New("mirage config local_ghost_id is required")
	}
	if localGhostAddr == "" {
		return errors.New("mirage config local_ghost_admin_addr is required")
	}
	logs.Infof(
		"opening local ghost admin console mirage=%q local_ghost_id=%q addr=%q",
		target.Name,
		localGhostID,
		localGhostAddr,
	)
	admin := NewRemoteGhostAdmin(localGhostAddr)
	defer admin.Close()
	return a.runGhostAdminConsoleForTarget(GhostTarget{
		Name:  localGhostID,
		Admin: admin,
	})
}

func (a *App) submitMirageIssue(target MirageTarget) error {
	routes, err := target.Admin.RoutingTable()
	if err != nil {
		return err
	}
	services, err := target.Admin.AvailableServices()
	if err != nil {
		return err
	}
	templates := mirageIntentTemplatesForServices(services)
	if len(templates) == 0 {
		return errors.New("no supported intent templates for available services")
	}

	fmt.Println()
	fmt.Println("Issue Intent")
	template, err := a.promptMirageIntentTemplateSelection("Select intent", templates)
	if err != nil {
		return err
	}
	targetGhostIDs := connectedGhostCandidatesForSeed(routes, services, template.Command.SeedSelector)
	if len(targetGhostIDs) == 0 {
		return fmt.Errorf("no connected ghost found for seed %q", template.Command.SeedSelector)
	}
	ghostID, err := a.promptGhostIDSelection("Select target ghost", targetGhostIDs)
	if err != nil {
		return err
	}
	args, err := a.promptCommandArgs(template.Command.Args)
	if err != nil {
		return err
	}
	actorRaw, err := a.promptLine("actor (blank = user:client-tm)")
	if err != nil {
		return err
	}
	actor := strings.TrimSpace(actorRaw)
	if actor == "" {
		actor = "user:client-tm"
	}
	intentID := fmt.Sprintf("intent.clienttm.%s.%d", normalizeSuffix(template.ID), time.Now().UnixMilli())
	commandPlan := []MirageIssueCommand{
		{
			GhostID:      ghostID,
			SeedSelector: template.Command.SeedSelector,
			Operation:    template.Command.Operation,
			Args:         args,
			Blocking:     template.Command.DefaultBlocking,
		},
	}
	req := MirageIssueRequest{
		IntentID:         intentID,
		Actor:            actor,
		TargetScope:      "ghost:" + ghostID,
		Objective:        template.Description,
		SeedDependencies: deriveSeedDependencies(commandPlan),
		CommandPlan:      commandPlan,
	}
	if err := target.Admin.SubmitIssue(req); err != nil {
		return err
	}
	fmt.Printf(
		"Issue submitted intent_id=%s template=%s ghost=%s deps=%s\n",
		req.IntentID,
		template.ID,
		ghostID,
		strings.Join(req.SeedDependencies, ","),
	)
	// For the current PoC template set (single blocking command), reconcile immediately.
	report, err := target.Admin.ReconcileIntent(req.IntentID)
	if err != nil {
		return err
	}
	printMirageReport("Issue Reconcile Result", report)
	return nil
}

func (a *App) runMirageReconcileConsole(target MirageTarget) error {
	for {
		fmt.Println()
		fmt.Printf("Reconcile Intent Console (%s)\n", target.Name)
		fmt.Println("  1) List intents")
		fmt.Println("  2) Reconcile one intent (*)")
		fmt.Println("  3) Reconcile all intents (*)")
		fmt.Println("  4) Back")
		choice, err := a.promptInt("Choose", 1, 4, true, true)
		if err != nil {
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			if err := a.listMirageIntents(target); err != nil {
				logs.Errf("list intents failed: %v", err)
			}
		case 2:
			if err := a.reconcileMirageIntent(target); err != nil {
				logs.Errf("reconcile intent failed: %v", err)
			}
		case 3:
			if err := a.reconcileAllMirageIntents(target); err != nil {
				logs.Errf("reconcile all failed: %v", err)
			}
		case 4:
			return nil
		}
	}
}

func (a *App) runMirageReportsConsole(target MirageTarget) error {
	for {
		fmt.Println()
		fmt.Printf("Reports Console (%s)\n", target.Name)
		fmt.Println("  1) Snapshot intent (*)")
		fmt.Println("  2) Show recent reports")
		fmt.Println("  3) Back")
		choice, err := a.promptInt("Choose", 1, 3, true, true)
		if err != nil {
			if errors.Is(err, ErrNavigateBack) {
				return nil
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			if err := a.snapshotMirageIntent(target); err != nil {
				logs.Errf("snapshot intent failed: %v", err)
			}
		case 2:
			if err := a.showMirageReports(target); err != nil {
				logs.Errf("show reports failed: %v", err)
			}
		case 3:
			return nil
		}
	}
}

func (a *App) runMirageGhostRoutingConsole(target MirageTarget) error {
	for {
		fmt.Println()
		fmt.Printf("Ghost Routing Console (%s)\n", target.Name)
		fmt.Println("  1) Spawn local ghost (*)")
		fmt.Println("  2) Connect ghost service (*)")
		fmt.Println("  3) Show routing table")
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
			if err := a.spawnMirageLocalGhost(target); err != nil {
				logs.Errf("spawn local ghost failed: %v", err)
			}
		case 2:
			if err := a.connectGhostServiceToMirage(target); err != nil {
				logs.Errf("connect ghost service failed: %v", err)
			}
		case 3:
			if err := a.showMirageRoutingTable(target); err != nil {
				logs.Errf("show routing table failed: %v", err)
			}
		case 4:
			return nil
		}
	}
}

func (a *App) listMirageIntents(target MirageTarget) error {
	intents, err := target.Admin.ListIntents()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Mirage Intents")
	if len(intents) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for i := range intents {
		fmt.Printf("  [%d] %s\n", i+1, intents[i])
	}
	return nil
}

func (a *App) reconcileMirageIntent(target MirageTarget) error {
	intentID, err := a.promptLine("intent_id")
	if err != nil {
		return err
	}
	report, err := target.Admin.ReconcileIntent(strings.TrimSpace(intentID))
	if err != nil {
		return err
	}
	printMirageReport("Reconcile Result", report)
	return nil
}

func (a *App) reconcileAllMirageIntents(target MirageTarget) error {
	reports, err := target.Admin.ReconcileAll()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Printf("Reconcile All Reports: %d\n", len(reports))
	for i := range reports {
		printMirageReport(fmt.Sprintf("Report %d", i+1), reports[i])
	}
	return nil
}

func (a *App) snapshotMirageIntent(target MirageTarget) error {
	intentID, err := a.promptLine("intent_id")
	if err != nil {
		return err
	}
	snapshot, found, err := target.Admin.SnapshotIntent(strings.TrimSpace(intentID))
	if err != nil {
		return err
	}
	fmt.Println()
	if !found {
		fmt.Printf("Intent %q not found\n", strings.TrimSpace(intentID))
		return nil
	}
	fmt.Printf("Intent Snapshot: %s\n", snapshot.Desired.Issue.IntentID)
	fmt.Printf("  pending_commands: %d\n", snapshot.PendingCount)
	fmt.Printf("  has_observed:     %v\n", snapshot.HasObserved)
	fmt.Printf("  desired_commands: %d\n", len(snapshot.Desired.Commands))
	if snapshot.HasObserved {
		fmt.Printf("  observed_events:  %d\n", len(snapshot.Observed.Events))
		fmt.Printf("  observed_reports: %d\n", len(snapshot.Observed.Reports))
	}
	return nil
}

func (a *App) showMirageReports(target MirageTarget) error {
	limit, err := a.promptOptionalLimit()
	if err != nil {
		return err
	}
	reports, err := target.Admin.RecentReports(limit)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Recent Mirage Reports")
	if len(reports) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for i := range reports {
		printMirageReport(fmt.Sprintf("Report %d", i+1), reports[i])
	}
	return nil
}

func (a *App) spawnMirageLocalGhost(target MirageTarget) error {
	name, err := a.promptLine("target_name suffix")
	if err != nil {
		return err
	}
	adminAddrRaw, err := a.promptLine("admin addr (host:port or port)")
	if err != nil {
		return err
	}
	adminAddr := strings.TrimSpace(adminAddrRaw)
	if adminAddr == "" {
		return errors.New("admin addr required")
	}
	if !strings.Contains(adminAddr, ":") {
		adminAddr = "127.0.0.1:" + adminAddr
	}
	req := mirage.SpawnGhostRequest{
		TargetName: normalizeSuffix(name),
		AdminAddr:  adminAddr,
	}
	out, err := target.Admin.SpawnLocalGhost(req)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Spawn Local Ghost Result")
	fmt.Printf("  target_name: %s\n", out.TargetName)
	fmt.Printf("  ghost_id:    %s\n", out.GhostID)
	fmt.Printf("  admin_addr:  %s\n", out.AdminAddr)
	return nil
}

func (a *App) connectGhostServiceToMirage(target MirageTarget) error {
	adminAddr, err := a.promptLine("ghost endpoint addr (host:port)")
	if err != nil {
		return err
	}
	addr := strings.TrimSpace(adminAddr)
	if addr == "" {
		return errors.New("ghost endpoint addr required")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("invalid ghost endpoint addr %q", addr)
	}
	out, err := target.Admin.AttachGhostAdmin(addr)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Connect Ghost Service Result")
	fmt.Printf("  ghost_id:   %s\n", out.GhostID)
	fmt.Printf("  endpoint:   %s\n", out.AdminAddr)
	return nil
}

func printMirageReport(header string, report session.Report) {
	ts := ""
	if report.TimestampMS > 0 {
		ts = time.UnixMilli(int64(report.TimestampMS)).Format(time.RFC3339)
	}
	fmt.Println()
	fmt.Println(header)
	fmt.Printf("  intent_id:         %s\n", report.IntentID)
	fmt.Printf("  phase:             %s\n", report.Phase)
	fmt.Printf("  completion_state:  %s\n", report.CompletionState)
	fmt.Printf("  summary:           %s\n", report.Summary)
	fmt.Printf("  command_id:        %s\n", report.CommandID)
	fmt.Printf("  execution_id:      %s\n", report.ExecutionID)
	fmt.Printf("  event_id:          %s\n", report.EventID)
	fmt.Printf("  outcome:           %s\n", report.Outcome)
	if ts != "" {
		fmt.Printf("  timestamp:         %s\n", ts)
	}
}
