package services

import (
	"sync"
)

type Service interface {
	Name() string
	Status() (any, error)
	Actions() map[string]Action
}
type Action func() error

var (
	mu       sync.RWMutex
	registry = map[string]Service{}
)

func Register(p Service) {
	mu.Lock()
	defer mu.Unlock()
	registry[p.Name()] = p
}

func All() map[string]Service {
	mu.RLock()
	defer mu.RUnlock()
	return registry
}

func Get(name string) (Service, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := registry[name]
	return p, ok
}
