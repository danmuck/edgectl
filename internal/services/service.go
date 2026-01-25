package services

import (
	// "os"
	"sync"
)

type Service interface {
	Name() string
	Status() (any, error)
	Actions() map[string]Action
}
type Action func() error

type ServiceRegistry struct {
	repo map[string]*Service
	mu   sync.RWMutex
}

func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		repo: make(map[string]*Service),
		mu:   sync.RWMutex{},
	}
}

func (sr *ServiceRegistry) Register(p Service) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	sr.repo[p.Name()] = &p
}

func (sr *ServiceRegistry) All() map[string]*Service {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	return sr.repo
}

func (sr *ServiceRegistry) Get(name string) (*Service, bool) {
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

func (ac *AdminCommands) Name() string {
	return "admin"
}

func (ac *AdminCommands) Status() (any, error) {
	out, err := ac.runner().Run("ls", "-al")
	return out, err
}

func (ac *AdminCommands) Actions() map[string]Action {
	return map[string]Action{
		"net": func() error {
			_, err := ac.runner().Run("ifconfig")
			return err
		},
		"repo": func() error {
			_, err := ac.runner().Run("ls", "-al")
			return err
		},
		// "stream-log": func() error {
		// 	return ac.runner().RunStreaming("pihole", []string{"-t"}, os.Stdout, os.Stderr)
		// },
	}
}

func (ac *AdminCommands) runner() Runner {
	if ac.Runner != nil {
		return ac.Runner
	}
	return LocalRunner{}
}
