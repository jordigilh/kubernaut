package severity

import (
	"sync"
	"time"

	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
)

// RulesCache provides TTL-based caching for Prometheus /api/v1/rules responses.
// Thread-safe via sync.RWMutex.
type RulesCache struct {
	mu       sync.RWMutex
	groups   []prom.RuleGroup
	expireAt time.Time
	ttl      time.Duration
}

// NewRulesCache creates a new RulesCache with the given TTL in seconds.
func NewRulesCache(ttlSeconds int) *RulesCache {
	return &RulesCache{
		ttl: time.Duration(ttlSeconds) * time.Second,
	}
}

// Get returns the cached rules if still valid, or nil if expired.
func (c *RulesCache) Get() []prom.RuleGroup {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.groups == nil || time.Now().After(c.expireAt) {
		return nil
	}
	result := make([]prom.RuleGroup, len(c.groups))
	copy(result, c.groups)
	return result
}

// Set stores the rules in the cache with a fresh TTL.
func (c *RulesCache) Set(groups []prom.RuleGroup) {
	c.mu.Lock()
	defer c.mu.Unlock()

	stored := make([]prom.RuleGroup, len(groups))
	copy(stored, groups)
	c.groups = stored
	c.expireAt = time.Now().Add(c.ttl)
}

// Len returns the number of cached rule groups.
func (c *RulesCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.groups == nil {
		return 0
	}
	return len(c.groups)
}
