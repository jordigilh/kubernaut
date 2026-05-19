package tools

import (
	"sync"
	"time"
)

// DiscoveryCache provides a TTL-based cache for discover_workflows responses.
type DiscoveryCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	result    *DiscoverWorkflowsResult
	expiresAt time.Time
}

// NewDiscoveryCache creates a cache with the given TTL duration.
func NewDiscoveryCache(ttl time.Duration) *DiscoveryCache {
	return &DiscoveryCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves a cached result if present and not expired.
func (c *DiscoveryCache) Get(key string) (*DiscoverWorkflowsResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.result, true
}

// Set stores a result in the cache with the configured TTL.
func (c *DiscoveryCache) Set(key string, result *DiscoverWorkflowsResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
}
