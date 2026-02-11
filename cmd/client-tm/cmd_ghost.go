package main

import (
	"sort"
	"strings"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/seeds"
)

// ghostCommandTemplatesForSeedCatalog maps Ghost seed capabilities into client command templates.
func ghostCommandTemplatesForSeedCatalog(seedCatalog []ghost.SeedCapability) []CommandTemplate {
	out := make([]CommandTemplate, 0)
	for i := range seedCatalog {
		seedID := strings.TrimSpace(seedCatalog[i].Metadata.ID)
		if seedID == "" {
			continue
		}
		opSet := make(map[string]struct{}, len(seedCatalog[i].Operations))
		for j := range seedCatalog[i].Operations {
			op := strings.TrimSpace(seedCatalog[i].Operations[j].Name)
			if op == "" {
				continue
			}
			opSet[op] = struct{}{}
		}
		for j := range seedCatalog[i].CommandCatalog {
			tpl, ok := mapSeedCommandTemplate(seedID, opSet, seedCatalog[i].CommandCatalog[j])
			if !ok {
				continue
			}
			out = append(out, tpl)
		}
	}
	sort.Slice(out, func(i int, j int) bool {
		if out[i].SeedSelector == out[j].SeedSelector {
			if out[i].Operation == out[j].Operation {
				return out[i].ID < out[j].ID
			}
			return out[i].Operation < out[j].Operation
		}
		return out[i].SeedSelector < out[j].SeedSelector
	})
	return out
}

// mapSeedCommandTemplate converts one seed-owned command template into the client's stable shape.
func mapSeedCommandTemplate(seedID string, opSet map[string]struct{}, in seeds.CommandTemplate) (CommandTemplate, bool) {
	operation := strings.TrimSpace(in.Operation)
	if operation == "" {
		return CommandTemplate{}, false
	}
	if _, ok := opSet[operation]; !ok {
		return CommandTemplate{}, false
	}
	seedSelector := strings.TrimSpace(in.SeedSelector)
	if seedSelector == "" {
		seedSelector = seedID
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = seedSelector + "." + operation
	}
	return CommandTemplate{
		ID:              id,
		Label:           strings.TrimSpace(in.Label),
		Description:     strings.TrimSpace(in.Description),
		SeedSelector:    seedSelector,
		Operation:       operation,
		Args:            mapSeedCommandArgSpecs(in.Args),
		DefaultBlocking: in.DefaultBlocking,
	}, true
}

// mapSeedCommandArgSpecs performs a field-for-field conversion from seed arg specs.
func mapSeedCommandArgSpecs(in []seeds.CommandArgSpec) []CommandArgSpec {
	if len(in) == 0 {
		return []CommandArgSpec{}
	}
	out := make([]CommandArgSpec, len(in))
	for i := range in {
		out[i] = CommandArgSpec{
			Key:          strings.TrimSpace(in[i].Key),
			Prompt:       strings.TrimSpace(in[i].Prompt),
			Required:     in[i].Required,
			DefaultValue: strings.TrimSpace(in[i].DefaultValue),
			Multiline:    in[i].Multiline,
			Terminator:   strings.TrimSpace(in[i].Terminator),
		}
	}
	return out
}
