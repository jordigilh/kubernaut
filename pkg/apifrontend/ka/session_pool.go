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
	Close() error
}

// SessionFactory creates new MCP sessions for the pool.
type SessionFactory func(ctx context.Context) (PoolSession, error)

type poolKey struct {
	rrID     string
	username string
}

type poolEntry struct {
	session  PoolSession
	lastUsed time.Time
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

// Acquire gets or creates a pooled session for the given (rrID, username).
// Sessions are keyed by composite (rrID, username) to enforce user isolation (G9).
// Uses a check-lock-recheck pattern to minimise lock contention (#78 race safety).
func (p *KASessionPool) Acquire(ctx context.Context, rrID, username string) (PoolSession, error) {
	key := poolKey{rrID: rrID, username: username}

	// Fast path: read-lock to check for existing session.
	p.mu.RLock()
	_, exists := p.entries[key]
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Recheck under write lock -- another goroutine may have created or deleted it.
	if entry, ok := p.entries[key]; ok || exists {
		if entry != nil {
			entry.lastUsed = time.Now()
			return entry.session, nil
		}
	}

	if len(p.entries) >= p.maxEntries {
		return nil, fmt.Errorf("session pool at max capacity (%d entries)", p.maxEntries)
	}

	session, err := p.factory(ctx)
	if err != nil {
		return nil, fmt.Errorf("create MCP session: %w", err)
	}

	p.entries[key] = &poolEntry{
		session:  session,
		lastUsed: time.Now(),
	}

	return session, nil
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
