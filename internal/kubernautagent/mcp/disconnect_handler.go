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
	"log/slog"
	"sync"
	"time"
)

// SessionClosedHandler processes disconnect events from the DelegatingEventStore.
// It runs as a goroutine reading from the closedSessions channel and invokes
// the onClose callback for each disconnect. DD-INTERACTIVE-002: triggers
// Release + reconstruction on disconnect.
type SessionClosedHandler struct {
	eventStore *DelegatingEventStore
	onClose    func(mcpSessionID string)
	logger     *slog.Logger
}

// NewSessionClosedHandler creates a handler that processes disconnect events.
func NewSessionClosedHandler(es *DelegatingEventStore, onClose func(string), logger *slog.Logger) *SessionClosedHandler {
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
				slog.String("mcp_session_id", mcpSessionID))
			h.onClose(mcpSessionID)
		}
	}
}

// SessionJanitor periodically checks for stale sessions that have exceeded
// their TTL and releases them. Provides a safety net when SessionClosed
// is not fired (e.g., process crash, network partition).
type SessionJanitor struct {
	interval time.Duration
	logger   *slog.Logger
	mu       sync.Mutex
	tracked  map[string]*janitorEntry
}

type janitorEntry struct {
	createdAt time.Time
	onExpire  func(sessionID string)
}

// NewSessionJanitor creates a janitor that checks for stale sessions at the given interval.
func NewSessionJanitor(interval time.Duration, logger *slog.Logger) *SessionJanitor {
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
		j.logger.Info("janitor: expiring stale session", slog.String("session_id", id))
		callbacks[i](id)
	}
}
