package seeds

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrSeedExists      = errors.New("seed already exists")
	ErrSeedNil         = errors.New("seed is nil")
	ErrInvalidMetadata = errors.New("invalid seed metadata")
)

// Registry stores seeds by stable identifier.
type Registry struct {
	items map[string]Seed
}

// NewRegistry creates an empty seed registry.
func NewRegistry() *Registry {
	return &Registry{items: make(map[string]Seed)}
}

// ValidateMetadata checks required metadata fields and id format.
func ValidateMetadata(meta SeedMetadata) error {
	id := strings.TrimSpace(meta.ID)
	name := strings.TrimSpace(meta.Name)
	desc := strings.TrimSpace(meta.Description)
	if id == "" || name == "" || desc == "" {
		return fmt.Errorf("%w: id, name, and description are required", ErrInvalidMetadata)
	}
	if !isValidID(id) {
		return fmt.Errorf("%w: invalid id format %q", ErrInvalidMetadata, id)
	}
	return nil
}

// Register adds a seed to the registry.
func (r *Registry) Register(seed Seed) error {
	if seed == nil {
		return ErrSeedNil
	}

	meta := seed.Metadata()
	if err := ValidateMetadata(meta); err != nil {
		return err
	}

	if _, ok := r.items[meta.ID]; ok {
		return ErrSeedExists
	}
	r.items[meta.ID] = seed
	return nil
}

// Resolve returns a seed by id.
func (r *Registry) Resolve(id string) (Seed, bool) {
	seed, ok := r.items[id]
	return seed, ok
}

// ListMetadata returns deterministic metadata ordering by id.
func (r *Registry) ListMetadata() []SeedMetadata {
	list := make([]SeedMetadata, 0, len(r.items))
	for _, seed := range r.items {
		list = append(list, seed.Metadata())
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	return list
}

func isValidID(id string) bool {
	if id == "" {
		return false
	}
	lastSep := false
	for i := 0; i < len(id); i++ {
		c := id[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		isSep := c == '.' || c == '-' || c == '_'
		if !(isLower || isDigit || isSep) {
			return false
		}
		if i == 0 || i == len(id)-1 {
			if isSep {
				return false
			}
		}
		if isSep && lastSep {
			return false
		}
		lastSep = isSep
	}
	return true
}
