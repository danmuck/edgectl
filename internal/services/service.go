package services

import (
	"sync"
)

// Service defines a runnable service with status and actions.
type Service interface {
	Name() string
	Status() (any, error)
	Actions() map[string]Action
}

// Action executes a service command.
type Action func() (string, error)

// ServiceRegistry stores services by name.
type ServiceRegistry struct {
	repo map[string]Service
	mu   sync.RWMutex
}

// NewServiceRegistry initializes an empty service registry.
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		repo: make(map[string]Service),
		mu:   sync.RWMutex{},
	}
}

// Register adds a service to the registry by name.
func (sr *ServiceRegistry) Register(p Service) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.repo[p.Name()] = p
}

// All returns a snapshot of all registered services.
func (sr *ServiceRegistry) All() map[string]Service {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	out := make(map[string]Service, len(sr.repo))
	for name, svc := range sr.repo {
		out[name] = svc
	}
	return out
}

// Get returns a service by name.
func (sr *ServiceRegistry) Get(name string) (Service, bool) {
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

// Name returns the service identifier.
func (ac *AdminCommands) Name() string {
	return "admin"
}

// Status returns the service status output.
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
