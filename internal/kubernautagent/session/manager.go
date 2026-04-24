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

package session

import (
	"context"
	"log/slog"
)

// InvestigateFunc is the function signature for running an investigation.
type InvestigateFunc func(ctx context.Context) (interface{}, error)

// Manager orchestrates investigation sessions, running each in a
// background goroutine and tracking progress via the Store.
type Manager struct {
	store  *Store
	logger *slog.Logger
}

// NewManager creates a session manager backed by the given store.
func NewManager(store *Store, logger *slog.Logger) *Manager {
	return &Manager{store: store, logger: logger}
}

// eventChannelBuffer is the capacity of the per-session event channel.
// 64 provides headroom for bursty LLM output (reasoning deltas + tool calls)
// without blocking the investigation goroutine. Non-blocking send semantics
// are deferred to PR 4 when events are actively written.
const eventChannelBuffer = 64

// StartInvestigation creates a new session and launches the investigation
// function in a background goroutine. Returns the session ID immediately.
// metadata is stored on the session for later retrieval (e.g., incident_id).
//
// The goroutine uses a cancellable child of context.Background() to ensure
// the investigation outlives the originating HTTP request while remaining
// cancellable via CancelInvestigation.
func (m *Manager) StartInvestigation(_ context.Context, fn InvestigateFunc, metadata map[string]string) (string, error) {
	id, err := m.store.Create()
	if err != nil {
		return "", err
	}
	if metadata != nil {
		m.store.SetMetadata(id, metadata)
	}

	bgCtx, cancelFn := context.WithCancel(context.Background())

	m.store.mu.Lock()
	sess := m.store.sessions[id]
	sess.cancel = cancelFn
	sess.eventChan = make(chan InvestigationEvent, eventChannelBuffer)
	m.store.mu.Unlock()

	_ = m.store.Update(id, StatusRunning, nil, nil)

	go func() {
		defer m.closeEventChan(id)
		result, fnErr := fn(bgCtx)
		if fnErr != nil {
			m.logger.Error("investigation failed",
				slog.String("session_id", id),
				slog.String("error", fnErr.Error()))
			if updateErr := m.store.Update(id, StatusFailed, nil, fnErr); updateErr != nil {
				m.logger.Info("post-investigation status update rejected",
					slog.String("session_id", id),
					slog.String("attempted_status", string(StatusFailed)),
					slog.String("reason", updateErr.Error()))
			}
			return
		}
		_ = m.store.Update(id, StatusCompleted, result, nil)
	}()

	return id, nil
}

// CancelInvestigation stops a running investigation by cancelling its context
// and transitioning its status to StatusCancelled. Returns ErrSessionNotFound
// if the session does not exist, or ErrSessionTerminal if it has already
// reached a terminal state.
func (m *Manager) CancelInvestigation(id string) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	sess, ok := m.store.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	if isTerminal(sess.Status) {
		return ErrSessionTerminal
	}
	if sess.cancel != nil {
		sess.cancel()
	}
	sess.Status = StatusCancelled
	return nil
}

// Subscribe returns a read-only channel that delivers investigation events
// for the given session. The channel is closed when the investigation ends.
// Returns ErrSessionNotFound if the session does not exist, or
// ErrSessionTerminal if the investigation has already concluded.
func (m *Manager) Subscribe(id string) (<-chan InvestigationEvent, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()

	sess, ok := m.store.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	if sess.eventChan == nil {
		return nil, ErrSessionTerminal
	}
	return sess.eventChan, nil
}

// closeEventChan closes the event channel for a session and sets it to nil,
// signaling to observers that the investigation has concluded. The nil-check
// guard prevents double-close panics.
func (m *Manager) closeEventChan(id string) {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	sess, ok := m.store.sessions[id]
	if !ok {
		return
	}
	if sess.eventChan != nil {
		close(sess.eventChan)
		sess.eventChan = nil
	}
}

// GetSession retrieves the current state of an investigation session.
func (m *Manager) GetSession(id string) (*Session, error) {
	return m.store.Get(id)
}
