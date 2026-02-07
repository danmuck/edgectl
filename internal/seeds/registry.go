package seeds

import "errors"

var ErrSeedExists = errors.New("seed already exists")

// Registry stores seeds by stable identifier.
type Registry struct {
	items map[string]Seed
}

// NewRegistry creates an empty seed registry.
func NewRegistry() *Registry {
	return &Registry{items: make(map[string]Seed)}
}

// Register adds a seed to the registry.
func (r *Registry) Register(seed Seed) error {
	id := seed.Metadata().ID
	if _, ok := r.items[id]; ok {
		return ErrSeedExists
	}
	r.items[id] = seed
	return nil
}

// Resolve returns a seed by id.
func (r *Registry) Resolve(id string) (Seed, bool) {
	seed, ok := r.items[id]
	return seed, ok
}
