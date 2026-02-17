// Package catalog aggregates per-seed external dependency declarations into a
// central lookup keyed by seed ID. This is the single Mirage-owned dependency
// catalog source file required by Phase 8 Slice F and Phase 9 Slice D.
//
// Contract: docs/progress/mvp_p8.md, docs/progress/mvp_p9.md
package catalog

import (
	"github.com/danmuck/edgectl/internal/seeds"
	seeddocker "github.com/danmuck/edgectl/internal/seeds/docker"
	seedhost "github.com/danmuck/edgectl/internal/seeds/host"
	seedmongod "github.com/danmuck/edgectl/internal/seeds/mongod"
)

// DependencyCatalog returns the complete dependency catalog keyed by seed ID.
// The catalog is hardcoded and versioned with the codebase per Phase 9 requirements.
func DependencyCatalog() map[string][]seeds.DepSpec {
	catalog := make(map[string][]seeds.DepSpec)

	if deps := seeddocker.Deps(); len(deps) > 0 {
		catalog["seed.docker"] = deps
	}
	if deps := seedmongod.Deps(); len(deps) > 0 {
		catalog["seed.mongod"] = deps
	}
	if deps := seedhost.Deps(); len(deps) > 0 {
		catalog["seed.host"] = deps
	}

	return catalog
}

// LookupDeps returns the dependency list for a given seed ID, or nil if none declared.
func LookupDeps(seedID string) []seeds.DepSpec {
	return DependencyCatalog()[seedID]
}
