package architecture

import (
	"sync"
	"time"
)

// CachedArchitecture represents a cached architecture response
type CachedArchitecture struct {
	Response    *ArchitectureResponse
	RepoStateId string
	ComputedAt  time.Time
}

// ArchitectureCache provides in-memory caching for architecture views
// Architecture views are cached with full repoStateId
type ArchitectureCache struct {
	mu    sync.RWMutex
	cache map[string]*CachedArchitecture
}

// NewArchitectureCache creates a new architecture cache
func NewArchitectureCache() *ArchitectureCache {
	return &ArchitectureCache{
		cache: make(map[string]*CachedArchitecture),
	}
}

// Get retrieves a cached architecture response
func (c *ArchitectureCache) Get(repoStateId string) (*CachedArchitecture, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, found := c.cache[repoStateId]
	return cached, found
}

// Set stores an architecture response in the cache
func (c *ArchitectureCache) Set(repoStateId string, response *ArchitectureResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[repoStateId] = &CachedArchitecture{
		Response:    response,
		RepoStateId: repoStateId,
		ComputedAt:  time.Now(),
	}
}

// Invalidate removes a specific cached entry
func (c *ArchitectureCache) Invalidate(repoStateId string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, repoStateId)
}

// Clear removes all cached entries
func (c *ArchitectureCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*CachedArchitecture)
}

// Size returns the number of cached entries
func (c *ArchitectureCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.cache)
}
