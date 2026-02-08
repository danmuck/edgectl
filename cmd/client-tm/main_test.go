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
		if templates[i].Command.SeedSelector != "seed.fs" {
			t.Fatalf("unexpected seed selector in intent template: %q", templates[i].Command.SeedSelector)
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
