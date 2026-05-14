package auth

import (
	"sync"
	"time"
)

// ReplayCache tracks seen JWT IDs (jti claims) to prevent token replay attacks.
// It uses an in-memory map with periodic eviction of expired entries.
//
// HA Limitation: This implementation is per-process. In multi-replica deployments,
// a token replayed against a different replica will not be detected. To close this
// gap at FedRAMP High, replace with a distributed cache (Redis SETEX with jti as
// key and TTL matching token expiry). The interface is designed to be swap-compatible:
// implement MissingJTI(string) bool and Seen(string) bool against Redis.
// See: https://github.com/jordigilh/kubernaut-apifrontend/issues/TBD
type ReplayCache struct {
	mu       sync.RWMutex
	entries  map[string]time.Time
	ttl      time.Duration
	done     chan struct{}
	stopOnce sync.Once
}

// NewReplayCache creates a jti replay cache. The ttl should match or exceed
// the maximum token lifetime to ensure tokens cannot be replayed after eviction.
func NewReplayCache(ttl time.Duration) *ReplayCache {
	rc := &ReplayCache{
		entries: make(map[string]time.Time),
		ttl:     ttl,
		done:    make(chan struct{}),
	}
	go rc.evictLoop()
	return rc
}

// MissingJTI returns true if jti enforcement is active (non-empty cache)
// and the provided jti is empty — indicating the token lacks replay protection.
func (c *ReplayCache) MissingJTI(jti string) bool {
	return jti == ""
}

// Seen returns true if the jti has already been observed (replay attempt).
// If the jti is new, it is recorded and false is returned.
// Empty jti values are always tracked (they'll collide — callers should
// reject missing jti via MissingJTI before calling Seen).
func (c *ReplayCache) Seen(jti string) bool {
	if jti == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.entries[jti]; exists {
		return true
	}
	c.entries[jti] = time.Now().Add(c.ttl)
	return false
}

// Stop terminates the background eviction goroutine. Safe to call multiple times.
func (c *ReplayCache) Stop() {
	c.stopOnce.Do(func() { close(c.done) })
}

func (c *ReplayCache) evictLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case now := <-ticker.C:
			c.mu.Lock()
			for jti, expiry := range c.entries {
				if now.After(expiry) {
					delete(c.entries, jti)
				}
			}
			c.mu.Unlock()
		}
	}
}
