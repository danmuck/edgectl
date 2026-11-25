package kv

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/danmuck/edgectl/internal/seeds"
)

const (
	// SeedID is the canonical seed identifier for temporary key-value state storage.
	SeedID = "seed.kv"
)

// Seed is a temporary in-memory key-value state adapter.
type Seed struct {
	mu    sync.RWMutex
	store map[string]string
}

// NewSeed constructs an empty in-memory key-value seed.
func NewSeed() *Seed {
	return &Seed{
		store: make(map[string]string),
	}
}

// Metadata returns stable seed identity and capability details.
func (s *Seed) Metadata() seeds.SeedMetadata {
	return seeds.SeedMetadata{
		ID:          SeedID,
		Name:        "KV (temporary in-memory)",
		Description: "Temporary key-value state storage seed for control-plane persistence",
	}
}

// Operations returns supported key-value operations.
func (s *Seed) Operations() []seeds.OperationSpec {
	return []seeds.OperationSpec{
		{Name: "put", Description: "upsert key=value", Idempotent: true},
		{Name: "get", Description: "get value by key", Idempotent: true},
		{Name: "delete", Description: "delete key", Idempotent: true},
		{Name: "list", Description: "list keys (optional prefix)", Idempotent: true},
	}
}

// CommandCatalog returns guided operator-facing command templates for this seed.
func (s *Seed) CommandCatalog() []seeds.CommandTemplate {
	seedID := s.Metadata().ID
	return []seeds.CommandTemplate{
		{
			ID:           seedID + ".put",
			Label:        "KV Put",
			Description:  "Upsert key/value in seed.kv.",
			SeedSelector: seedID,
			Operation:    "put",
			Args: []seeds.CommandArgSpec{
				{Key: "key", Prompt: "key", Required: true},
				{Key: "value", Prompt: "value", Required: true},
			},
			DefaultBlocking: true,
		},
		{
			ID:           seedID + ".get",
			Label:        "KV Get",
			Description:  "Read value by key from seed.kv.",
			SeedSelector: seedID,
			Operation:    "get",
			Args: []seeds.CommandArgSpec{
				{Key: "key", Prompt: "key", Required: true},
			},
			DefaultBlocking: true,
		},
		{
			ID:           seedID + ".list",
			Label:        "KV List",
			Description:  "List keys from seed.kv.",
			SeedSelector: seedID,
			Operation:    "list",
			Args: []seeds.CommandArgSpec{
				{Key: "prefix", Prompt: "key prefix (optional)", Required: false},
			},
			DefaultBlocking: true,
		},
		{
			ID:           seedID + ".delete",
			Label:        "KV Delete",
			Description:  "Delete key from seed.kv.",
			SeedSelector: seedID,
			Operation:    "delete",
			Args: []seeds.CommandArgSpec{
				{Key: "key", Prompt: "key", Required: true},
			},
			DefaultBlocking: true,
		},
	}
}

// Execute applies one key-value operation.
func (s *Seed) Execute(action string, args map[string]string) (seeds.SeedResult, error) {
	switch strings.TrimSpace(action) {
	case "put":
		key := strings.TrimSpace(args["key"])
		if key == "" {
			return seeds.ErrorResult("missing key"), fmt.Errorf("seed.kv: missing key")
		}
		val := args["value"]
		s.mu.Lock()
		s.store[key] = val
		s.mu.Unlock()
		return seeds.OKResult(fmt.Sprintf("ok put key=%s\n", key)), nil
	case "get":
		key := strings.TrimSpace(args["key"])
		if key == "" {
			return seeds.ErrorResult("missing key"), fmt.Errorf("seed.kv: missing key")
		}
		s.mu.RLock()
		val, ok := s.store[key]
		s.mu.RUnlock()
		if !ok {
			return seeds.ErrorResult(fmt.Sprintf("missing key=%s", key)), fmt.Errorf("seed.kv: missing key=%s", key)
		}
		return seeds.OKResult(val + "\n"), nil
	case "delete":
		key := strings.TrimSpace(args["key"])
		if key == "" {
			return seeds.ErrorResult("missing key"), fmt.Errorf("seed.kv: missing key")
		}
		s.mu.Lock()
		delete(s.store, key)
		s.mu.Unlock()
		return seeds.OKResult(fmt.Sprintf("ok delete key=%s\n", key)), nil
	case "list":
		prefix := strings.TrimSpace(args["prefix"])
		s.mu.RLock()
		keys := make([]string, 0, len(s.store))
		for k := range s.store {
			if prefix == "" || strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
		s.mu.RUnlock()
		sort.Strings(keys)
		return seeds.OKResult(strings.Join(keys, "\n") + "\n"), nil
	default:
		return seeds.ErrorResult("unknown action"), fmt.Errorf("seed.kv: unknown action=%q", action)
	}
}

