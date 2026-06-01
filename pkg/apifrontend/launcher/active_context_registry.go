package launcher

import (
	"sync"
	"time"
)

// DefaultRegistryTTL is the default time-to-live for active context entries.
// Exposed as a variable for testability without config surface expansion.
var DefaultRegistryTTL = 2 * time.Hour

type contextEntry struct {
	contextID string
	createdAt time.Time
}

// ActiveContextRegistry maintains a per-user mapping of active A2A context IDs.
// It enables multi-turn conversation continuity (BR-SESS-020) by allowing the
// SessionInterceptor to redirect new messages into an existing ADK session.
//
// Thread-safe via sync.Map; passive TTL checked on Get (no background goroutine).
type ActiveContextRegistry struct {
	entries sync.Map
	ttl     time.Duration
}

// NewActiveContextRegistry creates a registry with the given entry TTL.
// Entries older than ttl are treated as expired on access.
func NewActiveContextRegistry(ttl time.Duration) *ActiveContextRegistry {
	return &ActiveContextRegistry{ttl: ttl}
}

// Set stores or overwrites the active context ID for the given username.
func (r *ActiveContextRegistry) Set(username, contextID string) {
	r.entries.Store(username, contextEntry{
		contextID: contextID,
		createdAt: time.Now(),
	})
}

// Get returns the active context ID for the given username.
// Returns ("", false) if no entry exists or the entry has expired.
func (r *ActiveContextRegistry) Get(username string) (string, bool) {
	raw, ok := r.entries.Load(username)
	if !ok {
		return "", false
	}
	entry := raw.(contextEntry)
	if time.Since(entry.createdAt) > r.ttl {
		r.entries.Delete(username)
		return "", false
	}
	return entry.contextID, true
}

// Clear removes the active context entry for the given username.
func (r *ActiveContextRegistry) Clear(username string) {
	r.entries.Delete(username)
}
