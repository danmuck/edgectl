package seeds

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	logs "github.com/danmuck/smplog"
)

var (
	ErrSeedExists      = errors.New("seed already exists")
	ErrSeedNil         = errors.New("seed is nil")
	ErrInvalidMetadata = errors.New("invalid seed metadata")
)

// Seeds package registry storing seeds by stable identifier.
type Registry struct {
	items map[string]Seed
}

// Seeds package constructor for an empty registry.
func NewRegistry() *Registry {
	logs.Debug("seeds.NewRegistry")
	return &Registry{items: make(map[string]Seed)}
}

// Seeds package metadata validator for required fields and id format.
func ValidateMetadata(meta SeedMetadata) error {
	logs.Debugf("seeds.ValidateMetadata id=%q", meta.ID)
	id := strings.TrimSpace(meta.ID)
	name := strings.TrimSpace(meta.Name)
	desc := strings.TrimSpace(meta.Description)
	if id == "" || name == "" || desc == "" {
		logs.Errf("seeds.ValidateMetadata invalid-empty id=%q", meta.ID)
		return fmt.Errorf("%w: id, name, and description are required", ErrInvalidMetadata)
	}
	if !isValidID(id) {
		logs.Errf("seeds.ValidateMetadata invalid-id id=%q", id)
		return fmt.Errorf("%w: invalid id format %q", ErrInvalidMetadata, id)
	}
	logs.Debugf("seeds.ValidateMetadata ok id=%q", id)
	return nil
}

// Seeds package registry insert operation for one seed.
func (r *Registry) Register(seed Seed) error {
	logs.Debugf("seeds.Register start")
	if seed == nil {
		logs.Err("seeds.Register nil seed")
		return ErrSeedNil
	}

	meta := seed.Metadata()
	if err := ValidateMetadata(meta); err != nil {
		logs.Errf("seeds.Register validate failed id=%q err=%v", meta.ID, err)
		return err
	}

	if _, ok := r.items[meta.ID]; ok {
		logs.Warnf("seeds.Register duplicate id=%q", meta.ID)
		return ErrSeedExists
	}
	r.items[meta.ID] = seed
	logs.Infof("seeds.Register ok id=%q", meta.ID)
	return nil
}

// Seeds package registry lookup by id.
func (r *Registry) Resolve(id string) (Seed, bool) {
	seed, ok := r.items[id]
	logs.Debugf("seeds.Resolve id=%q found=%v", id, ok)
	return seed, ok
}

// Seeds package metadata snapshot in deterministic id order.
func (r *Registry) ListMetadata() []SeedMetadata {
	logs.Debugf("seeds.ListMetadata count=%d", len(r.items))
	list := make([]SeedMetadata, 0, len(r.items))
	for _, seed := range r.items {
		list = append(list, seed.Metadata())
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].ID < list[j].ID
	})
	logs.Debugf("seeds.ListMetadata sorted_count=%d", len(list))
	return list
}

// Seeds package id-format validator for stable seed identifiers.
func isValidID(id string) bool {
	logs.Debugf("seeds.isValidID id=%q", id)
	if id == "" {
		logs.Debug("seeds.isValidID empty id")
		return false
	}
	lastSep := false
	for i := 0; i < len(id); i++ {
		c := id[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		isSep := c == '.' || c == '-' || c == '_'
		if !(isLower || isDigit || isSep) {
			logs.Debugf("seeds.isValidID invalid-char id=%q char=%q", id, c)
			return false
		}
		if i == 0 || i == len(id)-1 {
			if isSep {
				logs.Debugf("seeds.isValidID edge-separator id=%q", id)
				return false
			}
		}
		if isSep && lastSep {
			logs.Debugf("seeds.isValidID repeated-separator id=%q", id)
			return false
		}
		lastSep = isSep
	}
	logs.Debugf("seeds.isValidID ok id=%q", id)
	return true
}
