package compliance

import "sync"

var (
	registryMu sync.RWMutex
	registry   = map[FrameworkID]Framework{}
)

// Register adds a framework to the global registry.
// Called by each framework sub-package's init() function.
func Register(f Framework) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[f.ID()] = f
}

// Get returns a registered framework by ID.
func Get(id FrameworkID) (Framework, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[id]
	return f, ok
}

// All returns all registered frameworks.
func All() []Framework {
	registryMu.RLock()
	defer registryMu.RUnlock()
	result := make([]Framework, 0, len(registry))
	for _, f := range registry {
		result = append(result, f)
	}
	return result
}
