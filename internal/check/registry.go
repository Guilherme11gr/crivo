package check

import "sync"

// Registry holds all registered check providers
type Registry struct {
	mu        sync.RWMutex
	providers []Provider
}

// NewRegistry creates an empty registry
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a provider to the registry
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers = append(r.providers, p)
}

// All returns all registered providers
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Provider, len(r.providers))
	copy(result, r.providers)
	return result
}

// Get returns a provider by ID
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		if p.ID() == id {
			return p, true
		}
	}
	return nil, false
}
