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
}

// NewActiveContextRegistry creates a registry with the given max TTL and idle
// timeout. Entries older than ttl OR idle longer than idleTimeout are treated
// as expired on access.
func NewActiveContextRegistry(ttl, idleTimeout time.Duration) *ActiveContextRegistry {
	return &ActiveContextRegistry{ttl: ttl, idleTimeout: idleTimeout}
}

// Set stores or overwrites the active context ID for the given username.
func (r *ActiveContextRegistry) Set(username, contextID string) {
	now := time.Now()
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
	entry := raw.(contextEntry)
	entry.lastActive = time.Now()
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
	entry := raw.(contextEntry)
	now := time.Now()
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
