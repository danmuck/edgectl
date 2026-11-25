// Package depcatalog is the single source of dependency install paths and methods
// (docs/progress/mvp_p8.md Slice F, mvp_p9.md "Single Source-of-Truth Install Catalog").
//
// Ownership: Mirage owns the catalog and is the only component that builds install proposals from
// it. Ghost compiles in the same file solely to verify that every step of a received plan is a
// catalog entry, so a plan from any other source cannot install arbitrary sources.
package depcatalog

import (
	"reflect"
	"strings"

	"github.com/danmuck/edgectl/internal/seeds"
)

// CatalogEntry is one approved dependency install source for a seed. The catalog is the single
// Mirage-owned source of install paths and methods (docs/progress/mvp_p8.md Slice F,
// mvp_p9.md "Single Source-of-Truth Install Catalog"). Ghosts install only specs taken from here,
// carried in a Mirage-approved proposal, and Ghost verifies each step with Contains.
type CatalogEntry struct {
	SeedID   string
	Name     string
	Binary   string
	Required bool
	Spec     seeds.InstallSpec
	// Operator-facing caveat about what the source does and does not provide.
	Note string
}

// dependencyCatalog lists every dependency with a verified, allowlisted source. Every Spec must
// pass seeds.ValidateSpec (enforced by test). curl entries must pin version and sha256; github
// entries must pin a ref.
var dependencyCatalog = []CatalogEntry{
	{
		SeedID:   "seed.docker",
		Name:     "docker",
		Binary:   "docker",
		Required: true,
		Spec: seeds.InstallSpec{
			SeedID:       "seed.docker",
			Method:       seeds.InstallMethodBrew,
			Package:      "docker",
			Binary:       "docker",
			InstallToBin: true,
		},
		Note: "Homebrew docker formula provides the CLI only; the Docker daemon must be provisioned on the host out of band.",
	},
	{
		SeedID:   "seed.mongod",
		Name:     "mongosh",
		Binary:   "mongosh",
		Required: true,
		Spec: seeds.InstallSpec{
			SeedID:       "seed.mongod",
			Method:       seeds.InstallMethodBrew,
			Package:      "mongosh",
			Binary:       "mongosh",
			InstallToBin: true,
		},
	},
}

// Required dependencies with no allowlisted verified source yet; proposals report them as
// unresolved instead of guessing a source.
var uncatalogedDependencies = map[string][]string{
	"seed.mongod": {"mongod"},
}

// DependencyCatalog returns a copy of every catalog entry.
func DependencyCatalog() []CatalogEntry {
	return append([]CatalogEntry{}, dependencyCatalog...)
}

// CatalogFor returns the catalog entries for one seed.
func CatalogFor(seedID string) []CatalogEntry {
	id := strings.TrimSpace(seedID)
	var out []CatalogEntry
	for _, e := range dependencyCatalog {
		if e.SeedID == id {
			out = append(out, e)
		}
	}
	return out
}

// Contains reports whether spec exactly matches a catalog entry's install spec (all wire fields).
// Ghost uses it to refuse plan steps that did not come from the catalog.
func Contains(spec seeds.InstallSpec) bool {
	for _, e := range dependencyCatalog {
		if reflect.DeepEqual(normalizeForCompare(e.Spec), normalizeForCompare(spec)) {
			return true
		}
	}
	return false
}

// Zeroes host-local fields that never travel on the wire.
func normalizeForCompare(s seeds.InstallSpec) seeds.InstallSpec {
	s.BootstrapIfMissing = false
	s.BootstrapCommand = nil
	return s
}

// UncatalogedDependencies returns required dependency names for a seed that have no catalog source.
func UncatalogedDependencies(seedID string) []string {
	return append([]string{}, uncatalogedDependencies[strings.TrimSpace(seedID)]...)
}
