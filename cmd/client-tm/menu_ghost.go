package main

// menu_ghost.go contains Ghost admin console menu loops and display functions.

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

func (a *App) runGhostAdminConsole() error {
	target, ok := a.active()
	if !ok {
		return errors.New("no active target")
	}
	return a.runGhostAdminConsoleForTarget(target)
}

// Drives one admin session for the selected Ghost target.
func (a *App) runGhostAdminConsoleForTarget(target GhostTarget) error {
	for {
		fmt.Println()
		fmt.Printf("Ghost Admin Console (%s @ %s)\n", target.Name, target.Admin.Address())
		fmt.Println("  1) Show status")
		fmt.Println("  2) List seeds and operations")
		fmt.Println("  3) Execute seed command")
		fmt.Println("  4) Lookup execution by command_id")
		fmt.Println("  5) Show recent events")
		fmt.Println("  6) Protocol/message verification view")
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
			a.showGhostTargetSummary(target)
		case 2:
			if err := a.listSeedOperations(target); err != nil {
				logs.Errf("list seed operations failed: %v", err)
			}
		case 3:
			if err := a.executeSeedCommand(target); err != nil {
				logs.Errf("execute command failed: %v", err)
			}
		case 4:
			if err := a.lookupExecution(target); err != nil {
				logs.Errf("lookup execution failed: %v", err)
			}
		case 5:
			if err := a.showRecentEvents(target); err != nil {
				logs.Errf("show events failed: %v", err)
			}
		case 6:
			if err := a.showVerification(target); err != nil {
				logs.Errf("show verification failed: %v", err)
			}
		case 7:
			return nil
		}
	}
}

func (a *App) showActiveTargetSummary() {
	target, ok := a.active()
	if !ok {
		fmt.Println("No active target. Add/select one first.")
		return
	}
	a.showGhostTargetSummary(target)
}

func (a *App) showGhostTargetSummary(target GhostTarget) {
	status, err := target.Admin.Status()
	if err != nil {
		fmt.Printf("Status error: %v\n", err)
		return
	}
	seedCatalog, err := target.Admin.ListSeedCatalog()
	if err != nil {
		fmt.Printf("Seed list error: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Printf("Active Target: %s\n", target.Name)
	fmt.Printf("  addr:     %s\n", target.Admin.Address())
	fmt.Printf("  ghost_id: %s\n", status.GhostID)
	fmt.Printf("  phase:    %s\n", status.Phase)
	fmt.Printf("  seeds:    %d\n", status.SeedCount)
	fmt.Println("  seed ids:")
	for i := range seedCatalog {
		seedID := strings.TrimSpace(seedCatalog[i].Metadata.ID)
		if seedID == "" {
			continue
		}
		fmt.Printf("    - %s\n", seedID)
	}
}

func (a *App) listSeedOperations(target GhostTarget) error {
	seedCatalog, err := target.Admin.ListSeedCatalog()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Seed Operations")
	for i := range seedCatalog {
		seedID := strings.TrimSpace(seedCatalog[i].Metadata.ID)
		if seedID == "" {
			continue
		}
		fmt.Printf("  %s\n", seedID)
		specs := append([]seeds.OperationSpec(nil), seedCatalog[i].Operations...)
		sort.Slice(specs, func(a, b int) bool {
			return specs[a].Name < specs[b].Name
		})
		if len(specs) == 0 {
			fmt.Println("    - (operations unknown)")
			continue
		}
		for _, spec := range specs {
			idempotent := "no"
			if spec.Idempotent {
				idempotent = "yes"
			}
			fmt.Printf("    - %s (idempotent=%s)\n", spec.Name, idempotent)
		}
	}
	return nil
}

func (a *App) executeSeedCommand(target GhostTarget) error {
	seedCatalog, err := target.Admin.ListSeedCatalog()
	if err != nil {
		return err
	}
	templates := ghostCommandTemplatesForSeedCatalog(seedCatalog)
	if len(templates) == 0 {
		return errors.New("no supported command templates for connected seeds")
	}
	fmt.Println()
	fmt.Println("Ghost Command Wizard")
	template, err := a.promptCommandTemplateSelection("Select command template", templates)
	if err != nil {
		return err
	}
	args, err := a.promptCommandArgs(template.Args)
	if err != nil {
		return err
	}
	intentID := fmt.Sprintf("intent.clienttm.%s.%d", normalizeSuffix(template.ID), time.Now().UnixMilli())
	cmd := GhostAdminCommand{
		IntentID:     intentID,
		SeedSelector: template.SeedSelector,
		Operation:    template.Operation,
		Args:         args,
	}

	execState, event, err := target.Admin.Execute(cmd)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Execution Result")
	fmt.Printf("  template:      %s\n", template.ID)
	fmt.Printf("  command_id:    %s\n", execState.CommandID)
	fmt.Printf("  execution_id:  %s\n", execState.ExecutionID)
	fmt.Printf("  outcome:       %s\n", event.Outcome)
	fmt.Printf("  seed_status:   %s\n", execState.SeedResult.Status)
	fmt.Printf("  seed_exitcode: %d\n", execState.SeedResult.ExitCode)
	if len(execState.SeedResult.Stdout) > 0 {
		fmt.Printf("  stdout:\n%s", indentLines(string(execState.SeedResult.Stdout), "    "))
	}
	if len(execState.SeedResult.Stderr) > 0 {
		fmt.Printf("  stderr:\n%s", indentLines(string(execState.SeedResult.Stderr), "    "))
	}
	return nil
}

func (a *App) lookupExecution(target GhostTarget) error {
	commandIDRaw, err := a.promptLine("command_id")
	if err != nil {
		return err
	}
	commandID := strings.TrimSpace(commandIDRaw)
	if commandID == "" {
		return errors.New("command_id required")
	}

	execState, ok, err := target.Admin.ExecutionByCommandID(commandID)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Printf("No execution found for command_id=%s\n", commandID)
		return nil
	}

	fmt.Println()
	fmt.Println("Execution State")
	fmt.Printf("  command_id:    %s\n", execState.CommandID)
	fmt.Printf("  execution_id:  %s\n", execState.ExecutionID)
	fmt.Printf("  phase:         %s\n", execState.Phase)
	fmt.Printf("  seed_selector: %s\n", execState.SeedSelector)
	fmt.Printf("  operation:     %s\n", execState.Operation)
	fmt.Printf("  outcome:       %s\n", execState.Outcome)
	fmt.Printf("  event_id:      %s\n", execState.Event.EventID)
	return nil
}

func (a *App) showRecentEvents(target GhostTarget) error {
	limit, err := a.promptOptionalLimit()
	if err != nil {
		return err
	}

	events, err := target.Admin.RecentEvents(limit)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Recent Events")
	if len(events) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for _, evt := range events {
		ts := time.UnixMilli(int64(evt.TimestampMS)).Format(time.RFC3339)
		fmt.Printf(
			"  event_id=%s command_id=%s seed_id=%s outcome=%s ts=%s\n",
			evt.EventID,
			evt.CommandID,
			evt.SeedID,
			evt.Outcome,
			ts,
		)
	}
	return nil
}

func (a *App) showVerification(target GhostTarget) error {
	limit, err := a.promptOptionalLimit()
	if err != nil {
		return err
	}
	records, err := target.Admin.Verification(limit)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Protocol/Message Verification")
	if len(records) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	fmt.Printf("  records=%d\n", len(records))
	fmt.Println()
	fmt.Println("  #  command_id                 execution_id               outcome  seed_status  exit")
	fmt.Println("  -- -------------------------- -------------------------- -------- ------------ ----")
	for i, rec := range records {
		fmt.Printf(
			"  %-2d %-26s %-26s %-8s %-12s %-4d\n",
			i+1,
			truncateRight(rec.CommandID, 26),
			truncateRight(rec.ExecutionID, 26),
			truncateRight(rec.Outcome, 8),
			truncateRight(rec.SeedStatus, 12),
			rec.ExitCode,
		)
	}
	fmt.Println()
	for i, rec := range records {
		ts := time.UnixMilli(int64(rec.TimestampMS)).Format(time.RFC3339)
		fmt.Printf("  [%d] request=%s trace=%s\n", i+1, rec.RequestID, rec.TraceID)
		fmt.Printf("      message: id=%d type=%d -> event_type=%d\n", rec.CommandMessageID, rec.CommandMessageType, rec.EventMessageType)
		fmt.Printf("      ids: command=%s execution=%s event=%s\n", rec.CommandID, rec.ExecutionID, rec.EventID)
		fmt.Printf("      target: ghost=%s seed=%s operation=%s\n", rec.GhostID, rec.SeedID, rec.Operation)
		fmt.Printf("      result: outcome=%s seed_status=%s exit=%d status=%s ts=%s\n", rec.Outcome, rec.SeedStatus, rec.ExitCode, rec.Status, ts)
	}
	return nil
}
