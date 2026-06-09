package ka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PoolSession represents an MCP session managed by the pool.
// *mcp.ClientSession satisfies this interface.
type PoolSession interface {
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	Ping(ctx context.Context, params *mcp.PingParams) error
	Close() error
}

// SessionFactory creates new MCP sessions for the pool.
type SessionFactory func(ctx context.Context) (PoolSession, error)

type poolKey struct {
	rrID     string
	username string
}

// SessionIdentifier is an optional interface that PoolSession implementations
// can satisfy to provide a transport-level session ID for audit logging.
type SessionIdentifier interface {
	SessionID() string
}

type poolEntry struct {
	session   PoolSession
	sessionID string
	lastUsed  time.Time
}

func extractSessionID(s PoolSession) string {
	if si, ok := s.(SessionIdentifier); ok {
		return si.SessionID()
	}
	return "unknown"
}

// PoolConfig configures the KASessionPool.
type PoolConfig struct {
	Factory    SessionFactory
	MaxEntries int
	IdleTTL    time.Duration
	Logger     logr.Logger
}

// KASessionPool manages persistent MCP sessions keyed by (rr_id, username).
// Each interactive investigation gets a dedicated session that persists across
// multiple tool calls (G2), with strict user isolation via composite keys (G9).
type KASessionPool struct {
	factory    SessionFactory
	mu         sync.RWMutex
	entries    map[poolKey]*poolEntry
	maxEntries int
	idleTTL    time.Duration
	logger     logr.Logger
}

// NewKASessionPool creates a new session pool.
func NewKASessionPool(cfg PoolConfig) *KASessionPool {
	maxEntries := cfg.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 100
	}
	idleTTL := cfg.IdleTTL
	if idleTTL <= 0 {
		idleTTL = 10 * time.Minute
	}
	return &KASessionPool{
		factory:    cfg.Factory,
		entries:    make(map[poolKey]*poolEntry),
		maxEntries: maxEntries,
		idleTTL:    idleTTL,
		logger:     cfg.Logger,
	}
}

const pingTimeout = 2 * time.Second

// Acquire gets or creates a pooled session for the given (rrID, username).
// Sessions are keyed by composite (rrID, username) to enforce user isolation (G9).
// Cached sessions are proactively health-checked via Ping before reuse (#1387).
func (p *KASessionPool) Acquire(ctx context.Context, rrID, username string) (PoolSession, error) {
	key := poolKey{rrID: rrID, username: username}

	p.mu.Lock()
	entry, exists := p.entries[key]
	if exists && entry != nil {
		session := entry.session
		sid := entry.sessionID
		entry.lastUsed = time.Now()
		p.mu.Unlock()

		pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
		defer cancel()
		if err := session.Ping(pingCtx, nil); err != nil {
			p.logger.Info("cached session failed health check, evicting",
				"rr_id", rrID, "username", username, "mcp_session_id", sid, "error", err.Error())
			p.mu.Lock()
			if cur, ok := p.entries[key]; ok && cur.session == session {
				delete(p.entries, key)
			}
			p.mu.Unlock()
			_ = session.Close()
		} else {
			p.logger.Info("pool session reused (cache hit)",
				"rr_id", rrID, "username", username, "mcp_session_id", sid)
			return session, nil
		}
	} else {
		p.mu.Unlock()
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok := p.entries[key]; ok && entry != nil {
		entry.lastUsed = time.Now()
		return entry.session, nil
	}

	if len(p.entries) >= p.maxEntries {
		return nil, fmt.Errorf("session pool at max capacity (%d entries)", p.maxEntries)
	}

	session, err := p.factory(ctx)
	if err != nil {
		return nil, fmt.Errorf("create MCP session: %w", err)
	}

	sid := extractSessionID(session)
	p.entries[key] = &poolEntry{
		session:   session,
		sessionID: sid,
		lastUsed:  time.Now(),
	}

	return session, nil
}

// Inject places an externally-created session into the pool under the given
// key. If a session already exists for the key, the old one is closed and
// replaced. Used to hand off the investigation MCP session so that subsequent
// tool calls (discover_workflows, select_workflow) reuse the same connection
// and driver lease without requiring a separate takeover.
func (p *KASessionPool) Inject(rrID, username string, session PoolSession) {
	key := poolKey{rrID: rrID, username: username}
	sid := extractSessionID(session)

	p.mu.Lock()
	old, exists := p.entries[key]
	p.entries[key] = &poolEntry{
		session:   session,
		sessionID: sid,
		lastUsed:  time.Now(),
	}
	p.mu.Unlock()

	if exists && old.session != nil {
		_ = old.session.Close()
	}
	p.logger.Info("session injected into pool", "rr_id", rrID, "username", username, "mcp_session_id", sid)
}

// Release closes and removes the session for the given (rrID, username).
func (p *KASessionPool) Release(rrID, username string) {
	key := poolKey{rrID: rrID, username: username}

	p.mu.Lock()
	entry, exists := p.entries[key]
	if exists {
		delete(p.entries, key)
	}
	p.mu.Unlock()

	if exists && entry.session != nil {
		p.logger.Info("pool session released",
			"rr_id", rrID, "username", username, "mcp_session_id", entry.sessionID)
		_ = entry.session.Close()
	}
}

// DrainAll closes all pooled sessions. Used during graceful shutdown (G14).
func (p *KASessionPool) DrainAll(ctx context.Context) error {
	p.mu.Lock()
	snapshot := make(map[poolKey]*poolEntry, len(p.entries))
	for k, v := range p.entries {
		snapshot[k] = v
	}
	p.entries = make(map[poolKey]*poolEntry)
	p.mu.Unlock()

	for _, entry := range snapshot {
		if entry.session != nil {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("drain interrupted: %w", err)
			}
			_ = entry.session.Close()
		}
	}
	return nil
}

// EvictIdle removes pool entries that have been idle longer than the configured TTL.
// Returns the number of evicted entries. Safe for concurrent use.
func (p *KASessionPool) EvictIdle() int {
	cutoff := time.Now().Add(-p.idleTTL)

	p.mu.Lock()
	toClose := make([]PoolSession, 0, len(p.entries)) // #21: pre-allocate
	var evicted int
	for key, entry := range p.entries {
		if entry.lastUsed.Before(cutoff) {
			toClose = append(toClose, entry.session)
			delete(p.entries, key)
			evicted++
		}
	}
	p.mu.Unlock()

	for _, s := range toClose {
		if s != nil {
			_ = s.Close()
		}
	}
	return evicted
}

// Size returns the number of active pool entries.
func (p *KASessionPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.entries)
}
