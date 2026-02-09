package main

import (
	"fmt"
	"sort"
	"strings"
)

// todo: more intengs
// Defines the stable intent wizard entries for Mirage issue submission.
func mirageIntentTemplateCatalog() []MirageIntentTemplate {
	var storeFile CommandTemplate
	for _, cmd := range ghostCommandTemplateCatalog() {
		if cmd.ID == "seed.fs.write" {
			storeFile = cmd
			break
		}
	}
	if storeFile.ID == "" {
		return []MirageIntentTemplate{}
	}
	return []MirageIntentTemplate{
		{
			ID:          "intent.seed.fs.store_file",
			Label:       "Store File (seed.fs)",
			Description: "Store a file on one connected ghost filesystem via seed.fs.",
			Command: CommandTemplate{
				ID:              "intent.seed.fs.store_file.command",
				Label:           "Store File Command",
				Description:     "Write file content to ghost seed.fs.",
				SeedSelector:    storeFile.SeedSelector,
				Operation:       storeFile.Operation,
				Args:            storeFile.Args,
				DefaultBlocking: true,
			},
		},
		{
			ID:          "intent.seed.fs.distribute_file",
			Label:       "Distribute File (seed.fs)",
			Description: "Write a file to all connected ghosts with seed.fs",
			Orchestrator: func(ctx MirageIntentContext) ([]MirageIssueStage, error) {
				ghosts := connectedGhostCandidatesForSeed(
					ctx.Routes,
					ctx.Services,
					"seed.fs",
				)

				if len(ghosts) == 0 {
					return nil, fmt.Errorf("no connected ghosts provide seed.fs")
				}

				stage := MirageIssueStage{
					ID:      "fs-write-fanout",
					Barrier: true,
				}

				for _, ghostID := range ghosts {
					stage.Commands = append(stage.Commands, MirageIssueCommand{
						GhostID:      ghostID,
						SeedSelector: "seed.fs",
						Operation:    "write",
						Args:         ctx.Args,
						Blocking:     true,
					})
				}

				return []MirageIssueStage{stage}, nil
			},
		},
	}
}

// Filters intent templates to those available in Mirage service discovery.
func mirageIntentTemplatesForServices(services []MirageAvailableService) []MirageIntentTemplate {
	availableSeeds := make(map[string]struct{}, len(services))
	for i := range services {
		seedID := strings.TrimSpace(services[i].SeedID)
		if seedID == "" || len(services[i].GhostIDs) == 0 {
			continue
		}
		availableSeeds[seedID] = struct{}{}
	}
	out := make([]MirageIntentTemplate, 0)
	for _, tpl := range mirageIntentTemplateCatalog() {
		if _, ok := availableSeeds[tpl.Command.SeedSelector]; !ok {
			continue
		}
		out = append(out, tpl)
	}
	sort.Slice(out, func(i int, j int) bool {
		if out[i].Command.SeedSelector == out[j].Command.SeedSelector {
			return out[i].Command.Operation < out[j].Command.Operation
		}
		return out[i].Command.SeedSelector < out[j].Command.SeedSelector
	})
	return out
}
