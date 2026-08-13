package launcher

import (
	"sync"
	"time"
)

// DefaultRegistryTTL is the default max time-to-live for active context entries.
// Exposed as a variable for testability without config surface expansion.
var DefaultRegistryTTL = 2 * time.Hour

// DefaultRegistryIdleTimeout is the default idle timeout for active context
// entries. If no tool call refreshes the entry within this window, the session
// is considered stale and the next message starts a fresh session (#1446).
var DefaultRegistryIdleTimeout = 10 * time.Minute

type contextEntry struct {
	contextID  string
	createdAt  time.Time
	lastActive time.Time
}

// Clock abstracts time.Now for deterministic testing of idle-timeout/TTL
// expiry (#1446). Production code always uses RealClock; tests use MockClock
// to advance time instantly instead of racing the wall clock with
// time.Sleep(), which is a common source of CI flakiness for millisecond-scale
// idle timeouts.
type Clock interface {
	Now() time.Time
}

// RealClock provides actual system time for production use.
type RealClock struct{}

// Now returns the current system time.
func (RealClock) Now() time.Time { return time.Now() }

// MockClock provides controllable time for deterministic tests.
//
// Usage:
//
//	clock := launcher.NewMockClock(time.Now())
//	registry := launcher.NewActiveContextRegistryWithClock(2*time.Hour, 10*time.Millisecond, clock)
//	registry.Set("bob", "ctx-active")
//	clock.Advance(5 * time.Millisecond) // instant, no wall-clock wait
//	registry.Refresh("bob")
//	clock.Advance(7 * time.Millisecond)
type MockClock struct {
	mu          sync.Mutex
	currentTime time.Time
}

// NewMockClock creates a MockClock starting at the given time.
func NewMockClock(initialTime time.Time) *MockClock {
	return &MockClock{currentTime: initialTime}
}

// Now returns the current mock time.
func (c *MockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentTime
}

// Advance moves the mock clock forward by the given duration.
func (c *MockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.currentTime = c.currentTime.Add(d)
}

// ActiveContextRegistry maintains a per-user mapping of active A2A context IDs.
// It enables multi-turn conversation continuity (BR-SESS-020) by allowing the
// SessionInterceptor to redirect new messages into an existing ADK session.
//
// Entries expire when either the max TTL or the idle timeout is exceeded (#1446,
// SC-7 boundary protection). The idle timeout prevents stale investigation
// sessions from hijacking unrelated conversations.
//
// Thread-safe via sync.Map; passive TTL checked on Get (no background goroutine).
type ActiveContextRegistry struct {
	entries     sync.Map
	ttl         time.Duration
	idleTimeout time.Duration
	clock       Clock
}

// NewActiveContextRegistry creates a registry with the given max TTL and idle
// timeout. Entries older than ttl OR idle longer than idleTimeout are treated
// as expired on access. Uses the real system clock; see
// NewActiveContextRegistryWithClock for deterministic tests.
func NewActiveContextRegistry(ttl, idleTimeout time.Duration) *ActiveContextRegistry {
	return NewActiveContextRegistryWithClock(ttl, idleTimeout, RealClock{})
}

// NewActiveContextRegistryWithClock creates a registry using the given Clock,
// enabling deterministic idle-timeout/TTL tests via MockClock instead of
// time.Sleep() (#1446).
func NewActiveContextRegistryWithClock(ttl, idleTimeout time.Duration, clock Clock) *ActiveContextRegistry {
	return &ActiveContextRegistry{ttl: ttl, idleTimeout: idleTimeout, clock: clock}
}

// Set stores or overwrites the active context ID for the given username.
func (r *ActiveContextRegistry) Set(username, contextID string) {
	now := r.clock.Now()
	r.entries.Store(username, contextEntry{
		contextID:  contextID,
		createdAt:  now,
		lastActive: now,
	})
}

// Refresh updates the idle timer for the given username without modifying
// createdAt or contextID. No-op if the entry does not exist (SI-10).
// Called by the phase_guard after-callback on every successful tool call
// to keep active sessions alive (#1446, AU-3).
func (r *ActiveContextRegistry) Refresh(username string) {
	raw, ok := r.entries.Load(username)
	if !ok {
		return
	}
	entry, ok := raw.(contextEntry)
	if !ok {
		return
	}
	entry.lastActive = r.clock.Now()
	r.entries.Store(username, entry)
}

// Get returns the active context ID for the given username.
// Returns ("", false) if no entry exists or the entry has expired by
// either max TTL or idle timeout (#1446, SC-7).
func (r *ActiveContextRegistry) Get(username string) (string, bool) {
	raw, ok := r.entries.Load(username)
	if !ok {
		return "", false
	}
	entry, ok := raw.(contextEntry)
	if !ok {
		return "", false
	}
	now := r.clock.Now()
	if now.Sub(entry.createdAt) > r.ttl || now.Sub(entry.lastActive) > r.idleTimeout {
		r.entries.Delete(username)
		return "", false
	}
	return entry.contextID, true
}

// HasEntry returns true if an entry exists for the username, regardless of
// whether it is expired. Used by SessionInterceptor to distinguish between
// "no entry" and "stale entry evicted by Get" for audit logging (#1446).
func (r *ActiveContextRegistry) HasEntry(username string) bool {
	_, ok := r.entries.Load(username)
	return ok
}

// Clear removes the active context entry for the given username.
func (r *ActiveContextRegistry) Clear(username string) {
	r.entries.Delete(username)
}
