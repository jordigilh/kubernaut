/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcp

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// SessionClosedHandler processes disconnect events from the DelegatingEventStore.
// It runs as a goroutine reading from the closedSessions channel and invokes
// the onClose callback for each disconnect. DD-INTERACTIVE-002: triggers
// Release + reconstruction on disconnect.
type SessionClosedHandler struct {
	eventStore *DelegatingEventStore
	onClose    func(mcpSessionID string)
	logger     logr.Logger
}

// NewSessionClosedHandler creates a handler that processes disconnect events.
func NewSessionClosedHandler(es *DelegatingEventStore, onClose func(string), logger logr.Logger) *SessionClosedHandler {
	return &SessionClosedHandler{
		eventStore: es,
		onClose:    onClose,
		logger:     logger,
	}
}

// Run processes disconnect events until ctx is cancelled.
func (h *SessionClosedHandler) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case mcpSessionID, ok := <-h.eventStore.ClosedSessions():
			if !ok {
				return
			}
			h.logger.Info("MCP session closed, triggering release",
				"mcp_session_id", mcpSessionID)
			h.onClose(mcpSessionID)
		}
	}
}

// GracefulSessionClosedHandler extends SessionClosedHandler with a configurable
// grace period. Instead of immediately invoking onClose on MCP disconnect, it
// defers the callback by gracePeriod. If CancelPendingRelease is called before
// the timer fires (e.g., because the client reconnected), the release is aborted.
// DD-INTERACTIVE-002 / BR-INTERACTIVE-001: decouples MCP transport lifecycle
// from interactive lease lifecycle.
type GracefulSessionClosedHandler struct {
	eventStore     *DelegatingEventStore
	onClose        func(mcpSessionID string)
	gracePeriod    time.Duration
	logger         logr.Logger
	mu             sync.Mutex
	pendingRelease map[string]*pendingEntry // interactiveSessionID -> pending
}

type pendingEntry struct {
	timer        *time.Timer
	mcpSessionID string
}

// NewGracefulSessionClosedHandler creates a handler that defers disconnect
// processing by gracePeriod, allowing reconnection without losing the
// interactive lease.
func NewGracefulSessionClosedHandler(es *DelegatingEventStore, onClose func(string), gracePeriod time.Duration, logger logr.Logger) *GracefulSessionClosedHandler {
	return &GracefulSessionClosedHandler{
		eventStore:     es,
		onClose:        onClose,
		gracePeriod:    gracePeriod,
		logger:         logger,
		pendingRelease: make(map[string]*pendingEntry),
	}
}

// Run processes disconnect events until ctx is cancelled. For each disconnect,
// a grace timer is started instead of immediately releasing.
func (h *GracefulSessionClosedHandler) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.cancelAll()
			return
		case mcpSessionID, ok := <-h.eventStore.ClosedSessions():
			if !ok {
				h.cancelAll()
				return
			}

			interactiveSessionID, ok := h.eventStore.LookupInteractiveSession(mcpSessionID)
			if !ok {
				h.logger.V(1).Info("MCP session closed without interactive mapping (autonomous or already released)",
					"mcp_session_id", mcpSessionID)
				continue
			}

			h.logger.Info("MCP session closed, starting grace period before release",
				"mcp_session_id", mcpSessionID,
				"interactive_session_id", interactiveSessionID,
				"grace_period", h.gracePeriod.String())

			h.mu.Lock()
			if existing, exists := h.pendingRelease[interactiveSessionID]; exists {
				existing.timer.Stop()
			}
			timer := time.AfterFunc(h.gracePeriod, func() {
				h.mu.Lock()
				delete(h.pendingRelease, interactiveSessionID)
				h.mu.Unlock()

				h.logger.Info("grace period expired, executing release",
					"mcp_session_id", mcpSessionID,
					"interactive_session_id", interactiveSessionID)
				h.onClose(mcpSessionID)
			})
			h.pendingRelease[interactiveSessionID] = &pendingEntry{
				timer:        timer,
				mcpSessionID: mcpSessionID,
			}
			h.mu.Unlock()
		}
	}
}

// CancelPendingRelease cancels a deferred release for the given interactive
// session. Returns true if a pending release was actually cancelled.
// Called when a client reconnects (Takeover detects same user for same rrID).
func (h *GracefulSessionClosedHandler) CancelPendingRelease(interactiveSessionID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry, exists := h.pendingRelease[interactiveSessionID]
	if !exists {
		return false
	}

	stopped := entry.timer.Stop()
	delete(h.pendingRelease, interactiveSessionID)

	if stopped {
		h.logger.Info("pending release cancelled (client reconnected)",
			"interactive_session_id", interactiveSessionID,
			"mcp_session_id", entry.mcpSessionID)
	}
	return stopped
}

// PendingCount returns the number of sessions with pending deferred releases.
// Exposed for testing.
func (h *GracefulSessionClosedHandler) PendingCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.pendingRelease)
}

// cancelAll stops all pending timers. Called on context cancellation or
// channel close during shutdown.
func (h *GracefulSessionClosedHandler) cancelAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, entry := range h.pendingRelease {
		entry.timer.Stop()
		delete(h.pendingRelease, id)
	}
}

// SessionJanitor periodically checks for stale sessions that have exceeded
// their TTL and releases them. Provides a safety net when SessionClosed
// is not fired (e.g., process crash, network partition).
type SessionJanitor struct {
	interval time.Duration
	logger   logr.Logger
	mu       sync.Mutex
	tracked  map[string]*janitorEntry
}

type janitorEntry struct {
	createdAt time.Time
	onExpire  func(sessionID string)
}

// NewSessionJanitor creates a janitor that checks for stale sessions at the given interval.
func NewSessionJanitor(interval time.Duration, logger logr.Logger) *SessionJanitor {
	return &SessionJanitor{
		interval: interval,
		logger:   logger,
		tracked:  make(map[string]*janitorEntry),
	}
}

// Track registers a session for janitor monitoring.
func (j *SessionJanitor) Track(sessionID string, createdAt time.Time, onExpire func(string)) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.tracked[sessionID] = &janitorEntry{
		createdAt: createdAt,
		onExpire:  onExpire,
	}
}

// Untrack removes a session from janitor monitoring (e.g., after clean release).
func (j *SessionJanitor) Untrack(sessionID string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.tracked, sessionID)
}

// Run starts the janitor loop until ctx is cancelled.
func (j *SessionJanitor) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.sweep()
		}
	}
}

func (j *SessionJanitor) sweep() {
	j.mu.Lock()
	var expired []string
	var callbacks []func(string)
	for id, entry := range j.tracked {
		if time.Since(entry.createdAt) > j.interval {
			expired = append(expired, id)
			callbacks = append(callbacks, entry.onExpire)
		}
	}
	for _, id := range expired {
		delete(j.tracked, id)
	}
	j.mu.Unlock()

	for i, id := range expired {
		j.logger.Info("janitor: expiring stale session", "session_id", id)
		callbacks[i](id)
	}
}
