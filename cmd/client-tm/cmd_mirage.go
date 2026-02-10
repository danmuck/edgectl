package main

import (
	"fmt"
	"sort"
	"strings"
)

// Defines the stable intent wizard entries for Mirage issue submission.
func mirageIntentTemplateCatalog() []MirageIntentTemplate {
	storeFileArgs := []CommandArgSpec{
		{Key: "path", Prompt: "filename (relative path)", Required: true},
		{Key: "content", Prompt: "file content", Required: true, Multiline: true, Terminator: ".done"},
	}
	for _, cmd := range ghostCommandTemplateCatalog() {
		if cmd.ID != "seed.fs.write" || len(cmd.Args) == 0 {
			continue
		}
		storeFileArgs = append([]CommandArgSpec(nil), cmd.Args...)
		break
	}

	return []MirageIntentTemplate{
		{
			ID:                     "intent.seed.fs.store_file",
			Label:                  "Store File (seed.fs)",
			Description:            "Store a file on one connected ghost filesystem via seed.fs.",
			RequiresGhostSelection: true,
			SeedDependencies:       []string{"seed.fs"},
			Args:                   storeFileArgs,
			Orchestrator: func(ctx MirageIntentContext) ([]MirageIssueStage, error) {
				return []MirageIssueStage{{
					ID:      "fs-write",
					Barrier: true,
					Commands: []MirageIssueCommand{{
						GhostID:      ctx.GhostID,
						SeedSelector: "seed.fs",
						Operation:    "write",
						Args:         ctx.Args,
						Blocking:     true,
					}},
				}}, nil
			},
		},
		{
			ID:               "intent.seed.fs.distribute_file",
			Label:            "Distribute File (seed.fs)",
			Description:      "Write a file to all connected ghosts with seed.fs",
			SeedDependencies: []string{"seed.fs"},
			Args: []CommandArgSpec{
				{Key: "path", Prompt: "filename (relative path)", Required: true},
				{Key: "content", Prompt: "file content", Required: true, Multiline: true, Terminator: ".done"},
			},
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
		{
			ID:               "intent.multi.store_and_index",
			Label:            "Store and Index File (seed.fs + seed.kv)",
			Description:      "Write a file to seed.fs then index its metadata in seed.kv. Multi-seed, multi-stage example.",
			SeedDependencies: []string{"seed.fs", "seed.kv"},
			Args: []CommandArgSpec{
				{Key: "path", Prompt: "filename (relative path)", Required: true},
				{Key: "content", Prompt: "file content", Required: true, Multiline: true, Terminator: ".done"},
			},
			Orchestrator: func(ctx MirageIntentContext) ([]MirageIssueStage, error) {
				fsGhosts := connectedGhostCandidatesForSeed(ctx.Routes, ctx.Services, "seed.fs")
				if len(fsGhosts) == 0 {
					return nil, fmt.Errorf("no connected ghosts provide seed.fs")
				}
				kvGhosts := connectedGhostCandidatesForSeed(ctx.Routes, ctx.Services, "seed.kv")
				if len(kvGhosts) == 0 {
					return nil, fmt.Errorf("no connected ghosts provide seed.kv")
				}

				// Stage 1: write file to the first ghost with seed.fs.
				writeStage := MirageIssueStage{
					ID:      "fs-write",
					Barrier: true,
					Commands: []MirageIssueCommand{
						{
							GhostID:      fsGhosts[0],
							SeedSelector: "seed.fs",
							Operation:    "write",
							Args:         ctx.Args,
							Blocking:     true,
						},
					},
				}

				// Stage 2: index file metadata in seed.kv on all ghosts that provide it.
				indexStage := MirageIssueStage{
					ID:      "kv-index",
					Barrier: true,
				}
				filePath := ctx.Args["path"]
				for _, ghostID := range kvGhosts {
					indexStage.Commands = append(indexStage.Commands, MirageIssueCommand{
						GhostID:      ghostID,
						SeedSelector: "seed.kv",
						Operation:    "put",
						Args: map[string]string{
							"key":   "index:" + filePath,
							"value": fmt.Sprintf("ghost=%s,path=%s,intent=%s", fsGhosts[0], filePath, ctx.IntentID),
						},
						Blocking: true,
					})
				}

				return []MirageIssueStage{writeStage, indexStage}, nil
			},
		},
		{
			ID:                     "intent.docker.deploy",
			Label:                  "Deploy Docker Container",
			Description:            "Verify host reachability, check docker daemon, run container, then confirm it is running. Multi-stage with barriers.",
			RequiresGhostSelection: true,
			SeedDependencies:       []string{"seed.docker", "seed.host"},
			Args: []CommandArgSpec{
				{Key: "image", Prompt: "docker image name", Required: true},
				{Key: "container", Prompt: "container name (optional)", Required: false},
				{Key: "flags", Prompt: "extra docker run flags (optional, e.g. -p 8080:80)", Required: false},
			},
			Orchestrator: func(ctx MirageIntentContext) ([]MirageIssueStage, error) {
				return []MirageIssueStage{
					{
						ID:      "host-check",
						Barrier: true,
						Commands: []MirageIssueCommand{{
							GhostID: ctx.GhostID, SeedSelector: "seed.host", Operation: "status",
						}},
					},
					{
						ID:      "docker-check",
						Barrier: true,
						Commands: []MirageIssueCommand{{
							GhostID: ctx.GhostID, SeedSelector: "seed.docker", Operation: "status",
						}},
					},
					{
						ID:      "docker-run",
						Barrier: true,
						Commands: []MirageIssueCommand{{
							GhostID: ctx.GhostID, SeedSelector: "seed.docker", Operation: "run",
							Args: ctx.Args, Blocking: true,
						}},
					},
					{
						ID:      "docker-verify",
						Barrier: false,
						Commands: []MirageIssueCommand{{
							GhostID: ctx.GhostID, SeedSelector: "seed.docker", Operation: "ps",
						}},
					},
				}, nil
			},
		},
		{
			ID:               "intent.fleet.inventory",
			Label:            "Collect Fleet Inventory",
			Description:      "Discover all connected ghosts, collect host status and listening ports from each, then store a summary in seed.kv.",
			SeedDependencies: []string{"seed.host"},
			Orchestrator: func(ctx MirageIntentContext) ([]MirageIssueStage, error) {
				hostGhosts := connectedGhostCandidatesForSeed(ctx.Routes, ctx.Services, "seed.host")
				if len(hostGhosts) == 0 {
					return nil, fmt.Errorf("no connected ghosts provide seed.host")
				}

				// Stage 1: collect status from all ghosts.
				statusStage := MirageIssueStage{ID: "host-status-fanout", Barrier: true}
				for _, gid := range hostGhosts {
					statusStage.Commands = append(statusStage.Commands, MirageIssueCommand{
						GhostID: gid, SeedSelector: "seed.host", Operation: "status",
					})
				}

				// Stage 2: collect ports from all ghosts.
				portsStage := MirageIssueStage{ID: "host-ports-fanout", Barrier: true}
				for _, gid := range hostGhosts {
					portsStage.Commands = append(portsStage.Commands, MirageIssueCommand{
						GhostID: gid, SeedSelector: "seed.host", Operation: "ports",
					})
				}

				stages := []MirageIssueStage{statusStage, portsStage}

				// Stage 3 (optional): store inventory count in seed.kv if available.
				kvGhosts := connectedGhostCandidatesForSeed(ctx.Routes, ctx.Services, "seed.kv")
				if len(kvGhosts) > 0 {
					kvStage := MirageIssueStage{ID: "kv-inventory-index", Barrier: false}
					summary := fmt.Sprintf("ghost_count=%d,intent=%s", len(hostGhosts), ctx.IntentID)
					kvStage.Commands = append(kvStage.Commands, MirageIssueCommand{
						GhostID: kvGhosts[0], SeedSelector: "seed.kv", Operation: "put",
						Args:     map[string]string{"key": "fleet:inventory:latest", "value": summary},
						Blocking: true,
					})
					stages = append(stages, kvStage)
				}

				return stages, nil
			},
		},
	}
}

// Filters intent templates to those whose seed dependencies are all available.
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
		allPresent := true
		for _, dep := range tpl.SeedDependencies {
			if _, ok := availableSeeds[strings.TrimSpace(dep)]; !ok {
				allPresent = false
				break
			}
		}
		if allPresent {
			out = append(out, tpl)
		}
	}
	sort.Slice(out, func(i int, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}
