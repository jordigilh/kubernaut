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

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

// InvestigateFunc is the function signature for running an investigation.
type InvestigateFunc func(ctx context.Context) (interface{}, error)

// Manager orchestrates investigation sessions, running each in a
// background goroutine and tracking progress via the Store.
type Manager struct {
	store      *Store
	logger     *slog.Logger
	auditStore audit.AuditStore
}

// NewManager creates a session manager backed by the given store.
// If auditStore is nil, a NopAuditStore is used (no audit events emitted).
func NewManager(store *Store, logger *slog.Logger, auditStore audit.AuditStore) *Manager {
	if auditStore == nil {
		auditStore = audit.NopAuditStore{}
	}
	return &Manager{store: store, logger: logger, auditStore: auditStore}
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
//
// Audit: emits aiagent.session.started after the session transitions to
// StatusRunning, and aiagent.session.completed or aiagent.session.failed
// when the goroutine finishes. Audit errors are fire-and-forget (ADR-038).
func (m *Manager) StartInvestigation(ctx context.Context, fn InvestigateFunc, metadata map[string]string) (string, error) {
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

	correlationID := metadata["remediation_id"]
	m.emitSessionEvent(ctx, audit.EventTypeSessionStarted, audit.ActionSessionStarted, audit.OutcomeSuccess, id, correlationID, nil)

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
				if bgCtx.Err() != nil {
					m.storePartialResult(id, nil)
				}
			} else {
				m.emitSessionEvent(context.Background(), audit.EventTypeSessionFailed, audit.ActionSessionFailed, audit.OutcomeFailure, id, correlationID, fnErr)
			}
			return
		}
		if updateErr := m.store.Update(id, StatusCompleted, result, nil); updateErr != nil {
			m.logger.Info("post-investigation status update rejected",
				slog.String("session_id", id),
				slog.String("attempted_status", string(StatusCompleted)),
				slog.String("reason", updateErr.Error()))
			if bgCtx.Err() != nil {
				m.storePartialResult(id, result)
			}
		} else {
			m.emitSessionEvent(context.Background(), audit.EventTypeSessionCompleted, audit.ActionSessionCompleted, audit.OutcomeSuccess, id, correlationID, nil)
		}
	}()

	return id, nil
}

// CancelInvestigation stops a running investigation by cancelling its context
// and transitioning its status to StatusCancelled. Returns ErrSessionNotFound
// if the session does not exist, or ErrSessionTerminal if it has already
// reached a terminal state.
//
// Audit: emits aiagent.session.cancelled after the status transition succeeds.
func (m *Manager) CancelInvestigation(id string) error {
	m.store.mu.Lock()

	sess, ok := m.store.sessions[id]
	if !ok {
		m.store.mu.Unlock()
		return ErrSessionNotFound
	}
	if IsTerminal(sess.Status) {
		m.store.mu.Unlock()
		return ErrSessionTerminal
	}
	if sess.cancel != nil {
		sess.cancel()
	}
	sess.Status = StatusCancelled
	correlationID := sess.Metadata["remediation_id"]
	m.store.mu.Unlock()

	m.emitSessionEvent(context.Background(), audit.EventTypeSessionCancelled, audit.ActionSessionCancelled, audit.OutcomeSuccess, id, correlationID, nil)
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

// storePartialResult attaches a result to a session that is already in a
// terminal state (e.g. StatusCancelled). Delegates to Store.SetResult which
// does not change the session status — only the Result field. This preserves
// partial investigation state for snapshot retrieval (BR-SESSION-002).
func (m *Manager) storePartialResult(id string, result interface{}) {
	m.store.SetResult(id, result)
}

// emitSessionEvent builds and stores an audit event for a session lifecycle
// transition. Errors are fire-and-forget per ADR-038.
func (m *Manager) emitSessionEvent(ctx context.Context, eventType, action, outcome, sessionID, correlationID string, fnErr error) {
	event := audit.NewEvent(eventType, correlationID)
	event.EventAction = action
	event.EventOutcome = outcome
	event.Data["session_id"] = sessionID
	if fnErr != nil {
		event.Data["error"] = fnErr.Error()
	}
	audit.StoreBestEffort(ctx, m.auditStore, event, m.logger)
}
