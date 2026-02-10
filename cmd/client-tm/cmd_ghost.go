package main

import (
	"sort"
	"strings"

	"github.com/danmuck/edgectl/internal/seeds"
)

// Stable list of CLI-exposed Ghost commands.
func ghostCommandTemplateCatalog() []CommandTemplate {
	return []CommandTemplate{
		// Flow
		// ////////
		{
			ID:           "seed.flow.status",
			Label:        "Flow Status",
			Description:  "Read deterministic flow status.",
			SeedSelector: "seed.flow",
			Operation:    "status",
		},
		{
			ID:           "seed.flow.step",
			Label:        "Flow Step",
			Description:  "Run deterministic flow step transition.",
			SeedSelector: "seed.flow",
			Operation:    "step",
			Args: []CommandArgSpec{
				{Key: "name", Prompt: "step name (init|plan|apply)", Required: true},
			},
		},
		{
			ID:           "seed.flow.echo",
			Label:        "Flow Echo",
			Description:  "Echo one key/value pair through seed.flow.",
			SeedSelector: "seed.flow",
			Operation:    "echo",
			Args: []CommandArgSpec{
				{Key: "message", Prompt: "message", Required: true},
			},
		},
		// MongoDB
		// ////////
		{
			ID:           "seed.mongod.status",
			Label:        "MongoDB Status",
			Description:  "Read mongod service status.",
			SeedSelector: "seed.mongod",
			Operation:    "status",
			Args: []CommandArgSpec{
				{Key: "unit", Prompt: "systemd unit", Required: false, DefaultValue: "mongod"},
			},
		},
		{
			ID:           "seed.mongod.start",
			Label:        "MongoDB Start",
			Description:  "Start mongod service.",
			SeedSelector: "seed.mongod",
			Operation:    "start",
			Args: []CommandArgSpec{
				{Key: "unit", Prompt: "systemd unit", Required: false, DefaultValue: "mongod"},
			},
		},
		{
			ID:           "seed.mongod.stop",
			Label:        "MongoDB Stop",
			Description:  "Stop mongod service.",
			SeedSelector: "seed.mongod",
			Operation:    "stop",
			Args: []CommandArgSpec{
				{Key: "unit", Prompt: "systemd unit", Required: false, DefaultValue: "mongod"},
			},
		},
		{
			ID:           "seed.mongod.restart",
			Label:        "MongoDB Restart",
			Description:  "Restart mongod service.",
			SeedSelector: "seed.mongod",
			Operation:    "restart",
			Args: []CommandArgSpec{
				{Key: "unit", Prompt: "systemd unit", Required: false, DefaultValue: "mongod"},
			},
		},
		{
			ID:           "seed.mongod.version",
			Label:        "MongoDB Version",
			Description:  "Read mongod binary version.",
			SeedSelector: "seed.mongod",
			Operation:    "version",
		},
		// Filesystem - on disk (default = local/)
		// ////////
		{
			ID:           "seed.fs.write",
			Label:        "Filesystem Write",
			Description:  "Write file content under ghost-scoped seed.fs root.",
			SeedSelector: "seed.fs",
			Operation:    "write",
			Args: []CommandArgSpec{
				{Key: "path", Prompt: "filename (relative path)", Required: true},
				{Key: "content", Prompt: "file content", Required: true, Multiline: true, Terminator: ".done"},
			},
			DefaultBlocking: true,
		},
		{
			ID:           "seed.fs.read",
			Label:        "Filesystem Read",
			Description:  "Read file content from ghost-scoped seed.fs root.",
			SeedSelector: "seed.fs",
			Operation:    "read",
			Args: []CommandArgSpec{
				{Key: "path", Prompt: "relative file path", Required: true},
			},
			DefaultBlocking: true,
		},
		{
			ID:           "seed.fs.list",
			Label:        "Filesystem List",
			Description:  "List file paths from ghost-scoped seed.fs root.",
			SeedSelector: "seed.fs",
			Operation:    "list",
			Args: []CommandArgSpec{
				{Key: "prefix", Prompt: "path prefix (optional)", Required: false},
			},
			DefaultBlocking: true,
		},
		{
			ID:           "seed.fs.delete",
			Label:        "Filesystem Delete",
			Description:  "Delete file path from ghost-scoped seed.fs root.",
			SeedSelector: "seed.fs",
			Operation:    "delete",
			Args: []CommandArgSpec{
				{Key: "path", Prompt: "relative file path", Required: true},
			},
			DefaultBlocking: true,
		},
		// KV Store - in memory
		// ////////
		{
			ID:           "seed.kv.put",
			Label:        "KV Put",
			Description:  "Upsert key/value in seed.kv.",
			SeedSelector: "seed.kv",
			Operation:    "put",
			Args: []CommandArgSpec{
				{Key: "key", Prompt: "key", Required: true},
				{Key: "value", Prompt: "value", Required: true},
			},
			DefaultBlocking: true,
		},
		{
			ID:           "seed.kv.get",
			Label:        "KV Get",
			Description:  "Read value by key from seed.kv.",
			SeedSelector: "seed.kv",
			Operation:    "get",
			Args: []CommandArgSpec{
				{Key: "key", Prompt: "key", Required: true},
			},
			DefaultBlocking: true,
		},
		{
			ID:           "seed.kv.list",
			Label:        "KV List",
			Description:  "List keys from seed.kv.",
			SeedSelector: "seed.kv",
			Operation:    "list",
			Args: []CommandArgSpec{
				{Key: "prefix", Prompt: "key prefix (optional)", Required: false},
			},
			DefaultBlocking: true,
		},
		{
			ID:           "seed.kv.delete",
			Label:        "KV Delete",
			Description:  "Delete key from seed.kv.",
			SeedSelector: "seed.kv",
			Operation:    "delete",
			Args: []CommandArgSpec{
				{Key: "key", Prompt: "key", Required: true},
			},
			DefaultBlocking: true,
		},
		// Docker
		// ////////
		{
			ID:           "seed.docker.status",
			Label:        "Docker Status",
			Description:  "Check docker daemon health.",
			SeedSelector: "seed.docker",
			Operation:    "status",
		},
		{
			ID:           "seed.docker.ps",
			Label:        "Docker PS",
			Description:  "List all containers.",
			SeedSelector: "seed.docker",
			Operation:    "ps",
			Args: []CommandArgSpec{
				{Key: "flags", Prompt: "extra flags (optional)", Required: false},
			},
		},
		{
			ID:              "seed.docker.run",
			Label:           "Docker Run",
			Description:     "Run a new container in detached mode.",
			SeedSelector:    "seed.docker",
			Operation:       "run",
			DefaultBlocking: true,
			Args: []CommandArgSpec{
				{Key: "image", Prompt: "image name", Required: true},
				{Key: "container", Prompt: "container name (optional)", Required: false},
				{Key: "flags", Prompt: "extra flags (optional, e.g. -p 8080:80)", Required: false},
			},
		},
		{
			ID:              "seed.docker.stop",
			Label:           "Docker Stop",
			Description:     "Stop a running container.",
			SeedSelector:    "seed.docker",
			Operation:       "stop",
			DefaultBlocking: true,
			Args: []CommandArgSpec{
				{Key: "container", Prompt: "container name or id", Required: true},
			},
		},
		{
			ID:              "seed.docker.rm",
			Label:           "Docker Remove",
			Description:     "Remove a container.",
			SeedSelector:    "seed.docker",
			Operation:       "rm",
			DefaultBlocking: true,
			Args: []CommandArgSpec{
				{Key: "container", Prompt: "container name or id", Required: true},
			},
		},
		{
			ID:           "seed.docker.logs",
			Label:        "Docker Logs",
			Description:  "Fetch container logs.",
			SeedSelector: "seed.docker",
			Operation:    "logs",
			Args: []CommandArgSpec{
				{Key: "container", Prompt: "container name or id", Required: true},
				{Key: "flags", Prompt: "extra flags (optional, e.g. --tail 50)", Required: false},
			},
		},
		{
			ID:           "seed.docker.inspect",
			Label:        "Docker Inspect",
			Description:  "Inspect a container.",
			SeedSelector: "seed.docker",
			Operation:    "inspect",
			Args: []CommandArgSpec{
				{Key: "container", Prompt: "container name or id", Required: true},
			},
		},
		// Host
		// ////////
		{
			ID:           "seed.host.status",
			Label:        "Host Status",
			Description:  "Read hostname, primary IP, OS, and architecture.",
			SeedSelector: "seed.host",
			Operation:    "status",
		},
		{
			ID:           "seed.host.ports",
			Label:        "Host Ports",
			Description:  "List listening TCP/UDP ports.",
			SeedSelector: "seed.host",
			Operation:    "ports",
		},
		{
			ID:           "seed.host.interfaces",
			Label:        "Host Interfaces",
			Description:  "List network interfaces with IP addresses.",
			SeedSelector: "seed.host",
			Operation:    "interfaces",
		},
	}
}

// ghostCommandTemplatesForSeedList filters command templates to those supported by connected Ghost seeds.
func ghostCommandTemplatesForSeedList(seedList []seeds.SeedMetadata) []CommandTemplate {
	seedSet := make(map[string]struct{}, len(seedList))
	opSetBySeed := make(map[string]map[string]struct{}, len(seedList))
	for i := range seedList {
		seedID := strings.TrimSpace(seedList[i].ID)
		if seedID == "" {
			continue
		}
		seedSet[seedID] = struct{}{}
		specs := operationsForSeed(seedID)
		opSet := make(map[string]struct{}, len(specs))
		for j := range specs {
			opSet[strings.TrimSpace(specs[j].Name)] = struct{}{}
		}
		opSetBySeed[seedID] = opSet
	}
	out := make([]CommandTemplate, 0)
	for _, tpl := range ghostCommandTemplateCatalog() {
		if _, ok := seedSet[tpl.SeedSelector]; !ok {
			continue
		}
		if opSet, ok := opSetBySeed[tpl.SeedSelector]; ok {
			if _, exists := opSet[tpl.Operation]; !exists {
				continue
			}
		}
		out = append(out, tpl)
	}
	sort.Slice(out, func(i int, j int) bool {
		if out[i].SeedSelector == out[j].SeedSelector {
			return out[i].Operation < out[j].Operation
		}
		return out[i].SeedSelector < out[j].SeedSelector
	})
	return out
}
