package main

// lab_menu.go is the lab front end (docs/progress/lab_mode.md): it starts the lab, then offers a
// short menu built on the existing admin actions (admin_security.toml [authorization]).

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
	logs "github.com/danmuck/smplog"
)

// Mirage admin clients that can read per-Ghost seed catalogs (seed_catalog action).
type seedCatalogReader interface {
	SeedCatalog() ([]mirage.GhostSeedCatalog, error)
}

// Entry point for -mode lab: bring the lab up, run the menu, and always stop the lab on the way out.
func runLabMode(dir string) error {
	fmt.Println("Starting the EdgeCTL lab (throwaway certificates, loopback only)...")
	lab, err := prepareLab(dir)
	if err != nil {
		return err
	}
	fmt.Println("  building miragectl from this checkout")
	if err := lab.build(); err != nil {
		return err
	}
	if err := lab.start(); err != nil {
		return err
	}
	defer lab.stop()
	// Ctrl-C or a terminal hangup must not leave Mirage running.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sig
		fmt.Println("\nstopping lab")
		lab.stop()
		os.Exit(130)
	}()

	app := NewApp(lab.clientGhostCfg(), lab.clientMirageCfg(), "lab")
	app.lab = lab
	if err := app.loadOrInitConfigs(); err != nil {
		return err
	}
	target, ok := app.activeMirageTarget()
	if !ok {
		return errors.New("lab: no mirage target")
	}
	fmt.Println("  waiting for Mirage and its Ghost")
	ready := func() error {
		routes, err := target.Admin.RoutingTable()
		if err != nil {
			return err
		}
		for _, r := range routes {
			if r.GhostID == labGhostID && r.Connected {
				return nil
			}
		}
		return errors.New("managed ghost not attached yet")
	}
	if err := lab.waitReady(ready, 20*time.Second); err != nil {
		return err
	}
	err = app.runLabMenu(target)
	app.closeTargets()
	app.closeMirageTargets()
	fmt.Println("stopping lab")
	return err
}

// Lab menu loop. Each action prints its own result; errors are shown and the menu continues.
func (a *App) runLabMenu(target MirageTarget) error {
	if err := a.labOverview(target); err != nil {
		logs.Errf("overview failed: %v", err)
	}
	for {
		fmt.Println()
		fmt.Println("EdgeCTL Lab")
		fmt.Println("  1) Overview")
		fmt.Println("  2) Add a Ghost")
		fmt.Println("  3) Run a command")
		fmt.Println("  4) Recent activity")
		fmt.Println("  5) Full admin console")
		fmt.Println("  6) Exit (stops the lab)")
		choice, err := a.promptInt("Choose", 1, 6, false, true)
		if err != nil {
			if errors.Is(err, ErrNavigateExit) {
				return nil
			}
			return err
		}
		fmt.Println()
		var actionErr error
		switch choice {
		case 1:
			actionErr = a.labOverview(target)
		case 2:
			actionErr = a.labAddGhost(target)
		case 3:
			actionErr = a.labRunCommand(target)
		case 4:
			actionErr = a.labRecentActivity(target)
		case 5:
			actionErr = a.runMirageAdminConsole()
		case 6:
			return nil
		}
		switch {
		case actionErr == nil, errors.Is(actionErr, ErrNavigateBack):
		case errors.Is(actionErr, ErrNavigateExit):
			return nil
		default:
			fmt.Printf("Failed: %v\n", actionErr)
		}
	}
}

// What is running, who the client is acting as, and where the lab keeps its files.
func (a *App) labOverview(target MirageTarget) error {
	status, err := target.Admin.Status()
	if err != nil {
		return err
	}
	ghosts, err := labGhosts(target)
	if err != nil {
		return err
	}
	l := a.lab
	fmt.Println("Lab overview")
	fmt.Printf("  Mirage   %s  admin %s  session %s  phase %s\n", status.MirageID, l.mirageAdminAddr, l.sessionAddr, status.Phase)
	fmt.Printf("  You are  %s (mutual TLS)\n", identity.Principal{Role: identity.RoleOperator, ID: labOperator})
	fmt.Printf("  Intents  %d   Reports %d\n", status.ActiveIntents, status.ReportCount)
	fmt.Println("  Ghosts")
	for _, g := range ghosts {
		state := "connected"
		if !g.Connected {
			state = "NOT CONNECTED"
		}
		fmt.Printf("    %-24s %-16s %-14s %s\n", g.GhostID, g.AdminAddr, state, strings.Join(seedIDs(g.SeedCatalog), " "))
	}
	fmt.Println("  Files")
	fmt.Printf("    log           %s\n", l.display("mirage.log"))
	fmt.Printf("    certificates  %s (second operator: %s)\n", l.display("certs"), labSecondOperator)
	fmt.Printf("    seed.fs data  %s\n", l.display("local", "dir", "<ghost_id>"))
	return nil
}

// Adds a child Ghost of the managed Ghost: issues its certificate, then asks Mirage to spawn and
// bind it (spawn_local_ghost). The child's id is <parent id>.<name>.
func (a *App) labAddGhost(target MirageTarget) error {
	ghosts, err := labGhosts(target)
	if err != nil {
		return err
	}
	suggested := fmt.Sprintf("g%d", len(ghosts))
	raw, err := a.promptLine(fmt.Sprintf("Ghost name (blank = %s)", suggested))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(raw)
	if name == "" {
		name = suggested
	}
	suffix := normalizeSuffix(name)
	ghostID := labGhostID + "." + suffix
	for _, g := range ghosts {
		if g.GhostID == ghostID {
			return fmt.Errorf("%s already exists", ghostID)
		}
	}
	if err := a.lab.issue(identity.Principal{Role: identity.RoleGhost, ID: ghostID}); err != nil {
		return err
	}
	out, err := target.Admin.SpawnLocalGhost(mirage.SpawnGhostRequest{TargetName: suffix, AdminAddr: a.lab.nextChildAddr()})
	if err != nil {
		return err
	}
	fmt.Printf("Added %s on %s (certificate %s)\n", out.GhostID, out.AdminAddr, a.lab.display("certs", out.GhostID+".crt"))
	fmt.Printf("It runs the same seeds as %s and takes commands through Mirage.\n", labGhostID)
	return nil
}

// Runs one seed command end to end through Mirage (issue, command, seed, event, report), then
// reads the execution record from the Ghost to show what the seed printed.
func (a *App) labRunCommand(target MirageTarget) error {
	ghosts, err := labGhosts(target)
	if err != nil {
		return err
	}
	connected := make([]mirage.GhostSeedCatalog, 0, len(ghosts))
	ids := make([]string, 0, len(ghosts))
	for _, g := range ghosts {
		if g.Connected {
			connected = append(connected, g)
			ids = append(ids, g.GhostID)
		}
	}
	if len(connected) == 0 {
		return errors.New("no connected ghost")
	}
	chosen := connected[0]
	if len(connected) > 1 {
		id, err := a.promptGhostIDSelection("Select ghost", ids)
		if err != nil {
			return err
		}
		for _, g := range connected {
			if g.GhostID == id {
				chosen = g
			}
		}
	}
	caps := make([]ghost.SeedCapability, 0, len(chosen.SeedCatalog))
	for _, c := range chosen.SeedCatalog {
		caps = append(caps, ghost.SeedCapability(c))
	}
	templates := ghostCommandTemplatesForSeedCatalog(caps)
	if len(templates) == 0 {
		return fmt.Errorf("%s advertises no commands", chosen.GhostID)
	}
	fmt.Printf("Commands on %s\n", chosen.GhostID)
	tpl, err := a.promptCommandTemplateSelection("Select command", templates)
	if err != nil {
		return err
	}
	args, err := a.promptCommandArgs(tpl.Args)
	if err != nil {
		return err
	}

	// The actor is left empty: Mirage binds it to the caller's certificate principal.
	issue := MirageIssueRequest{
		IntentID:    fmt.Sprintf("intent.lab.%d", time.Now().UnixMilli()),
		TargetScope: "ghost:" + chosen.GhostID,
		Objective:   tpl.SeedSelector + " " + tpl.Operation,
		Stages: []MirageIssueStage{{ID: "stage.1", Commands: []MirageIssueCommand{{
			GhostID: chosen.GhostID, SeedSelector: tpl.SeedSelector, Operation: tpl.Operation, Args: args, Blocking: true,
		}}}},
	}
	if err := target.Admin.SubmitIssue(issue); err != nil {
		return err
	}
	report, err := target.Admin.ReconcileIntent(issue.IntentID)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Printf("%s %s on %s: %s (%s)\n", tpl.SeedSelector, tpl.Operation, chosen.GhostID, report.CompletionState, report.Outcome)
	fmt.Printf("  intent   %s\n  command  %s\n", report.IntentID, report.CommandID)
	if report.CompletionState != "satisfied" {
		fmt.Printf("  summary  %s\n", report.Summary)
	}
	a.labPrintExecution(chosen, report.CommandID)
	fmt.Printf("  trace    grep 'trace.%s' %s\n", report.IntentID, a.lab.display("mirage.log"))
	return nil
}

// Shows a command's stdout, stderr and exit code from the Ghost that ran it. Reports carry only
// the outcome, so this is a second, read-only request to that Ghost's admin endpoint.
func (a *App) labPrintExecution(g mirage.GhostSeedCatalog, commandID string) {
	if strings.TrimSpace(commandID) == "" {
		return
	}
	admin := a.newGhostAdmin(g.AdminAddr, g.GhostID)
	defer admin.Close()
	state, found, err := admin.ExecutionByCommandID(commandID)
	if err != nil {
		fmt.Printf("  output   unavailable: %v\n", err)
		return
	}
	if !found {
		fmt.Println("  output   no execution record on the ghost (the command did not run there)")
		return
	}
	fmt.Printf("  exit     %d\n", state.SeedResult.ExitCode)
	if out := strings.TrimRight(string(state.SeedResult.Stdout), "\n"); out != "" {
		fmt.Println("  stdout")
		fmt.Println(indentLines(out, "    "))
	}
	if errOut := strings.TrimRight(string(state.SeedResult.Stderr), "\n"); errOut != "" {
		fmt.Println("  stderr")
		fmt.Println(indentLines(errOut, "    "))
	}
}

// Latest reports from Mirage and the latest audit and custody lines from the lab log.
func (a *App) labRecentActivity(target MirageTarget) error {
	reports, err := target.Admin.RecentReports(8)
	if err != nil {
		return err
	}
	fmt.Println("Recent reports")
	if len(reports) == 0 {
		fmt.Println("  none yet: run a command")
	}
	for _, r := range reports {
		fmt.Printf("  %-12s %-10s %s\n", r.CompletionState, r.Outcome, labReportSubject(r))
	}
	raw, err := os.ReadFile(a.lab.logPath())
	if err != nil {
		return err
	}
	audit, custody := labActivityLines(string(raw), 8)
	fmt.Println("Admin decisions (who asked for what)")
	for _, line := range audit {
		fmt.Println("  " + line)
	}
	fmt.Println("Custody chain (messages between Mirage and Ghosts)")
	for _, line := range custody {
		fmt.Println("  " + line)
	}
	fmt.Printf("Full log: %s\n", a.lab.display("mirage.log"))
	return nil
}

func labReportSubject(r session.Report) string {
	if strings.TrimSpace(r.CommandID) != "" {
		return r.IntentID + "  " + r.CommandID
	}
	return r.IntentID
}

// Per-Ghost seed catalogs from Mirage, sorted by ghost id.
func labGhosts(target MirageTarget) ([]mirage.GhostSeedCatalog, error) {
	reader, ok := target.Admin.(seedCatalogReader)
	if !ok {
		return nil, errors.New("mirage admin client cannot read seed catalogs")
	}
	ghosts, err := reader.SeedCatalog()
	if err != nil {
		return nil, err
	}
	sort.Slice(ghosts, func(i, j int) bool { return ghosts[i].GhostID < ghosts[j].GhostID })
	return ghosts, nil
}

func seedIDs(caps []mirage.SeedCapability) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, strings.TrimPrefix(c.Metadata.ID, "seed."))
	}
	sort.Strings(out)
	return out
}

var labLogField = regexp.MustCompile(`([a-z_]+)=("(?:[^"\\]|\\.)*"|\S+)`)

// key=value fields of one runtime log line, with surrounding quotes removed.
func labLogFields(line string) map[string]string {
	out := make(map[string]string)
	for _, m := range labLogField.FindAllStringSubmatch(line, -1) {
		out[m[1]] = strings.Trim(m[2], `"`)
	}
	return out
}

// Condenses the last limit admin.audit and custody lines of a runtime log into short readable rows.
func labActivityLines(log string, limit int) (audit []string, custody []string) {
	for _, line := range strings.Split(log, "\n") {
		f := labLogFields(line)
		switch {
		case strings.Contains(line, "admin.audit "):
			row := fmt.Sprintf("%-5s %-28s -> %-7s %s", f["decision"], f["principal"], f["component"], f["action"])
			if f["decision"] == "deny" {
				row += "  (code " + f["error_code"] + ")"
			}
			audit = append(audit, row)
		case strings.Contains(line, "custody "):
			row := fmt.Sprintf("%-7s %-8s %-28s %-12s %s", f["component"], f["direction"], f["peer"], f["status"], f["request_id"])
			custody = append(custody, row)
		}
	}
	last := func(in []string) []string {
		if len(in) > limit {
			return in[len(in)-limit:]
		}
		if len(in) == 0 {
			return []string{"none yet"}
		}
		return in
	}
	return last(audit), last(custody)
}
