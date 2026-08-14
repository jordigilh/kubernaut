package auth

import (
	"sync"
	"time"
)

// ReplayCacheStore is the interface implemented by both the in-memory
// ReplayCache and the distributed ValkeyReplayCache (GAP-08, kubernaut#1505),
// letting JWTValidator remain agnostic to the backing store.
type ReplayCacheStore interface {
	// MissingJTI returns true if jti enforcement is active and the token
	// lacks a jti claim needed for replay protection.
	MissingJTI(jti string) bool
	// Seen atomically records (jti, sourceKey) as observed and reports
	// whether jti was already present from a *different* sourceKey — a
	// replay attempt (#1999, BR-SECURITY-1505). A jti re-presented from the
	// same sourceKey is legitimate Bearer-token reuse, not a replay, and
	// returns false. sourceKey is typically the caller's spoof-resistant
	// client IP (see trustedSourceResolver); an empty sourceKey is treated
	// as its own distinct "unknown" source, consistent across calls that
	// don't supply one.
	Seen(jti, sourceKey string) bool
	// Stop releases any background resources held by the cache.
	Stop()
}

// ReplayCache tracks seen JWT IDs (jti claims) to prevent token replay attacks.
// It uses an in-memory map with periodic eviction of expired entries.
//
// HA Limitation: This implementation is per-process. In multi-replica deployments,
// a token replayed against a different replica will not be detected. Deployments
// running more than one APIFrontend replica should use ValkeyReplayCache instead
// (auth.replayCache.backend: redis in config), which shares replay state across
// all replicas via the cluster's Valkey/Redis instance (GAP-08, kubernaut#1505).
type ReplayCache struct {
	mu       sync.RWMutex
	entries  map[string]replayEntry
	ttl      time.Duration
	done     chan struct{}
	stopOnce sync.Once
}

// replayEntry records which source first presented a jti and when that
// record expires. sourceKey is compared on subsequent presentations to
// distinguish legitimate reuse from a different-source replay (#1999).
type replayEntry struct {
	sourceKey string
	expiry    time.Time
}

// NewReplayCache creates a jti replay cache. The ttl should match or exceed
// the maximum token lifetime to ensure tokens cannot be replayed after eviction.
func NewReplayCache(ttl time.Duration) *ReplayCache {
	rc := &ReplayCache{
		entries: make(map[string]replayEntry),
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

// Seen returns true if jti has already been observed from a *different*
// sourceKey than the one that first recorded it (a replay attempt). A jti
// re-presented from the same sourceKey is legitimate Bearer-token reuse and
// returns false. If jti is new, it is recorded with sourceKey and false is
// returned. Empty jti values are always tracked (they'll collide — callers
// should reject missing jti via MissingJTI before calling Seen).
func (c *ReplayCache) Seen(jti, sourceKey string) bool {
	if jti == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, exists := c.entries[jti]; exists {
		return existing.sourceKey != sourceKey
	}
	c.entries[jti] = replayEntry{sourceKey: sourceKey, expiry: time.Now().Add(c.ttl)}
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
			for jti, entry := range c.entries {
				if now.After(entry.expiry) {
					delete(c.entries, jti)
				}
			}
			c.mu.Unlock()
		}
	}
}
