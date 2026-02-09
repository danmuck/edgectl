package main

import (
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
)

func TestGhostCommandTemplatesForSeedListFiltersSeeds(t *testing.T) {
	seedList := []seeds.SeedMetadata{
		{ID: "seed.flow"},
	}
	templates := ghostCommandTemplatesForSeedList(seedList)
	if len(templates) == 0 {
		t.Fatalf("expected flow templates")
	}
	for i := range templates {
		if templates[i].SeedSelector != "seed.flow" {
			t.Fatalf("unexpected seed selector in filtered templates: %q", templates[i].SeedSelector)
		}
	}
}

func TestMirageIntentTemplatesForServicesFiltersBySeed(t *testing.T) {
	services := []MirageAvailableService{
		{SeedID: "seed.fs", GhostIDs: []string{"ghost.local"}},
	}
	templates := mirageIntentTemplatesForServices(services)
	if len(templates) == 0 {
		t.Fatalf("expected seed.fs intent templates")
	}
	for i := range templates {
		tpl := templates[i]
		for j := range tpl.SeedDependencies {
			if tpl.SeedDependencies[j] != "seed.fs" {
				t.Fatalf("unexpected seed dependency in intent template %q: %q", tpl.ID, tpl.SeedDependencies[j])
			}
		}
	}
}

func TestMirageIntentTemplatesForServicesIncludesMultiSeedOrchestrator(t *testing.T) {
	services := []MirageAvailableService{
		{SeedID: "seed.fs", GhostIDs: []string{"ghost.alpha"}},
		{SeedID: "seed.kv", GhostIDs: []string{"ghost.alpha"}},
	}
	templates := mirageIntentTemplatesForServices(services)
	found := false
	for i := range templates {
		if templates[i].ID == "intent.multi.store_and_index" {
			found = true
			if templates[i].Orchestrator == nil {
				t.Fatalf("expected orchestrator on multi-seed template")
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected store_and_index template when both seed.fs and seed.kv available")
	}
}

func TestMirageIntentTemplatesForServicesExcludesMultiSeedWhenPartial(t *testing.T) {
	// Only seed.fs available — seed.kv missing, so multi-seed template should be excluded.
	services := []MirageAvailableService{
		{SeedID: "seed.fs", GhostIDs: []string{"ghost.alpha"}},
	}
	templates := mirageIntentTemplatesForServices(services)
	for i := range templates {
		if templates[i].ID == "intent.multi.store_and_index" {
			t.Fatalf("multi-seed template should be excluded when seed.kv is unavailable")
		}
	}
}

func TestStoreAndIndexOrchestratorProducesMultiStage(t *testing.T) {
	catalog := mirageIntentTemplateCatalog()
	var tpl MirageIntentTemplate
	for i := range catalog {
		if catalog[i].ID == "intent.multi.store_and_index" {
			tpl = catalog[i]
			break
		}
	}
	if tpl.ID == "" {
		t.Fatalf("store_and_index template not found in catalog")
	}
	if tpl.Orchestrator == nil {
		t.Fatalf("expected orchestrator on store_and_index template")
	}

	ctx := MirageIntentContext{
		IntentID: "intent.test.1",
		Actor:    "user:test",
		Args:     map[string]string{"path": "data/test.txt", "content": "hello"},
		Routes: []MirageRoute{
			{GhostID: "ghost.alpha", Connected: true},
			{GhostID: "ghost.beta", Connected: true},
		},
		Services: []MirageAvailableService{
			{SeedID: "seed.fs", GhostIDs: []string{"ghost.alpha", "ghost.beta"}},
			{SeedID: "seed.kv", GhostIDs: []string{"ghost.alpha", "ghost.beta"}},
		},
	}

	stages, err := tpl.Orchestrator(ctx)
	if err != nil {
		t.Fatalf("orchestrator error: %v", err)
	}
	if len(stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(stages))
	}

	// Stage 1: one seed.fs write command.
	if stages[0].ID != "fs-write" {
		t.Fatalf("unexpected stage 1 id: %q", stages[0].ID)
	}
	if len(stages[0].Commands) != 1 {
		t.Fatalf("expected 1 command in stage 1, got %d", len(stages[0].Commands))
	}
	if stages[0].Commands[0].SeedSelector != "seed.fs" {
		t.Fatalf("expected seed.fs in stage 1, got %q", stages[0].Commands[0].SeedSelector)
	}

	// Stage 2: seed.kv put commands for each ghost providing seed.kv.
	if stages[1].ID != "kv-index" {
		t.Fatalf("unexpected stage 2 id: %q", stages[1].ID)
	}
	if len(stages[1].Commands) != 2 {
		t.Fatalf("expected 2 commands in stage 2, got %d", len(stages[1].Commands))
	}
	for _, cmd := range stages[1].Commands {
		if cmd.SeedSelector != "seed.kv" {
			t.Fatalf("expected seed.kv in stage 2, got %q", cmd.SeedSelector)
		}
		if cmd.Operation != "put" {
			t.Fatalf("expected put operation in stage 2, got %q", cmd.Operation)
		}
	}
}

func TestConnectedGhostCandidatesForSeed(t *testing.T) {
	routes := []MirageRoute{
		{GhostID: "ghost.a", Connected: true},
		{GhostID: "ghost.b", Connected: false},
		{GhostID: "ghost.c", Connected: true},
	}
	services := []MirageAvailableService{
		{SeedID: "seed.fs", GhostIDs: []string{"ghost.a", "ghost.b"}},
		{SeedID: "seed.flow", GhostIDs: []string{"ghost.c"}},
	}

	ghosts := connectedGhostCandidatesForSeed(routes, services, "seed.fs")
	if len(ghosts) != 1 || ghosts[0] != "ghost.a" {
		t.Fatalf("unexpected connected ghost candidates: %+v", ghosts)
	}
}

func TestStoreFileOrchestratorRequiresGhostSelectionAndUsesGhostID(t *testing.T) {
	catalog := mirageIntentTemplateCatalog()
	var tpl MirageIntentTemplate
	for i := range catalog {
		if catalog[i].ID == "intent.seed.fs.store_file" {
			tpl = catalog[i]
			break
		}
	}
	if tpl.ID == "" {
		t.Fatalf("store_file template not found in catalog")
	}
	if !tpl.RequiresGhostSelection {
		t.Fatalf("expected store_file template to require ghost selection")
	}
	if tpl.Orchestrator == nil {
		t.Fatalf("expected orchestrator on store_file template")
	}
	ctx := MirageIntentContext{
		Args:    map[string]string{"path": "a.txt", "content": "hi"},
		GhostID: "ghost.local",
	}
	stages, err := tpl.Orchestrator(ctx)
	if err != nil {
		t.Fatalf("orchestrator error: %v", err)
	}
	if len(stages) != 1 || len(stages[0].Commands) != 1 {
		t.Fatalf("unexpected stages: %+v", stages)
	}
	if stages[0].Commands[0].GhostID != "ghost.local" {
		t.Fatalf("unexpected ghost id in stage command: %+v", stages[0].Commands[0])
	}
}
