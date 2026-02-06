package seeds

import (
	"sync"
)

// Seed defines a runnable seed with status and actions.
type Seed interface {
	Name() string
	Status() (any, error)
	Actions() map[string]Action
}

// Action executes a seed command.
type Action func() (string, error)

// SeedRegistry stores seeds by name.
type SeedRegistry struct {
	repo map[string]Seed
	mu   sync.RWMutex
}

// NewSeedRegistry initializes an empty seed registry.
func NewSeedRegistry() *SeedRegistry {
	return &SeedRegistry{
		repo: make(map[string]Seed),
		mu:   sync.RWMutex{},
	}
}

// Register adds a seed to the registry by name.
func (sr *SeedRegistry) Register(p Seed) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.repo[p.Name()] = p
}

// All returns a snapshot of all registered seeds.
func (sr *SeedRegistry) All() map[string]Seed {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	out := make(map[string]Seed, len(sr.repo))
	for name, svc := range sr.repo {
		out[name] = svc
	}
	return out
}

// Get returns a seed by name.
func (sr *SeedRegistry) Get(name string) (Seed, bool) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	p, ok := sr.repo[name]
	return p, ok
}

// /////////////////////////////////////////////////////////////////////////////////
// Example
// //////////////////////////
type AdminCommands struct {
	Runner Runner
}

// Name returns the seed identifier.
func (ac *AdminCommands) Name() string {
	return "admin"
}

// Status returns the seed status output.
func (ac *AdminCommands) Status() (any, error) {
	out, err := ac.runner().Run("ls", "-al")
	return out, err
}

// Actions returns available admin actions.
func (ac *AdminCommands) Actions() map[string]Action {
	return map[string]Action{
		"net": func() (string, error) {
			return ac.runner().Run("ifconfig")
		},
		"repo": func() (string, error) {
			return ac.runner().Run("ls", "-al")
		},
		// "stream-log": func() error {
		// 	return ac.runner().RunStreaming("pihole", []string{"-t"}, os.Stdout, os.Stderr)
		// },
	}
}

// runner returns the configured runner or a default local runner.
func (ac *AdminCommands) runner() Runner {
	if ac.Runner != nil {
		return ac.Runner
	}
	return LocalRunner{}
}
