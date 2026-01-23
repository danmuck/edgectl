package plugins

import "sync"

var (
	mu       sync.RWMutex
	registry = map[string]Plugin{}
)

func Register(p Plugin) {
	mu.Lock()
	defer mu.Unlock()
	registry[p.Name()] = p
}

func All() map[string]Plugin {
	mu.RLock()
	defer mu.RUnlock()
	return registry
}

func Get(name string) (Plugin, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := registry[name]
	return p, ok
}
