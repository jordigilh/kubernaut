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
	"fmt"
	"log/slog"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
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
// without blocking the investigation goroutine. The investigator uses
// non-blocking send semantics (select/default) so a slow SSE consumer
// cannot stall the investigation loop.
const eventChannelBuffer = 64

// StartInvestigation creates a new session and launches the investigation
// function in a background goroutine. Returns the session ID immediately.
// metadata is stored on the session for later retrieval (e.g., incident_id).
// If the context carries an authenticated user (auth.UserContextKey), the
// user identity is stored as "created_by" in session metadata for
// object-level authorization checks.
//
// The goroutine uses a cancellable child of context.Background() to ensure
// the investigation outlives the originating HTTP request while remaining
// cancellable via CancelInvestigation.
//
// A LazySink is placed on the context but starts with a nil channel.
// EventSinkFromContext returns nil until Subscribe activates the sink,
// ensuring autonomous investigations (no observer) use Chat (v1.4 parity).
//
// The goroutine includes recover() to catch panics in the investigation
// function, transitioning the session to StatusFailed instead of crashing.
//
// Audit: emits aiagent.session.started after the session transitions to
// StatusRunning, and aiagent.session.completed or aiagent.session.failed
// when the goroutine finishes. Audit errors are fire-and-forget (ADR-038).
func (m *Manager) StartInvestigation(ctx context.Context, fn InvestigateFunc, metadata map[string]string) (string, error) {
	id, err := m.store.Create()
	if err != nil {
		return "", err
	}
	if metadata == nil {
		metadata = make(map[string]string)
	}
	if user := auth.GetUserFromContext(ctx); user != "" {
		metadata["created_by"] = user
	}
	m.store.SetMetadata(id, metadata)

	bgCtx, cancelFn := context.WithCancel(context.Background())

	ls := &LazySink{}
	bgCtx = WithLazySink(bgCtx, ls)

	m.store.mu.Lock()
	sess := m.store.sessions[id]
	sess.cancel = cancelFn
	sess.lazySink = ls
	m.store.mu.Unlock()

	_ = m.store.Update(id, StatusRunning, nil, nil)

	correlationID := metadata["remediation_id"]
	var startExtra []string
	if v := metadata["incident_id"]; v != "" {
		startExtra = append(startExtra, "incident_id", v)
	}
	if v := metadata["signal_name"]; v != "" {
		startExtra = append(startExtra, "signal_name", v)
	}
	if v := metadata["severity"]; v != "" {
		startExtra = append(startExtra, "severity", v)
	}
	if v := metadata["created_by"]; v != "" {
		startExtra = append(startExtra, "created_by", v)
	}
	m.emitSessionEvent(ctx, audit.EventTypeSessionStarted, audit.ActionSessionStarted, audit.OutcomeSuccess, id, correlationID, nil, startExtra...)

	go func() {
		defer m.closeEventChan(id)
		defer m.recoverPanic(id, correlationID)

		result, fnErr := fn(bgCtx)
		m.emitCompleteEvent(id)
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
// for the given session. The event sink is lazily created on the first
// Subscribe call so that autonomous investigations (no observer) run without
// an event sink, preserving v1.4 Chat behavior. The channel is closed when
// the investigation ends.
//
// The context carries the authenticated user identity (via auth.UserContextKey)
// which is recorded in the aiagent.session.observed audit event for SOC2 CC8.1
// operator attribution.
//
// Returns ErrSessionNotFound if the session does not exist, or
// ErrSessionTerminal if the investigation has already concluded.
func (m *Manager) Subscribe(ctx context.Context, id string) (<-chan InvestigationEvent, error) {
	m.store.mu.Lock()

	sess, ok := m.store.sessions[id]
	if !ok {
		m.store.mu.Unlock()
		return nil, ErrSessionNotFound
	}
	if IsTerminal(sess.Status) && sess.eventChan == nil {
		m.store.mu.Unlock()
		return nil, ErrSessionTerminal
	}

	if sess.eventChan == nil {
		ch := make(chan InvestigationEvent, eventChannelBuffer)
		sess.eventChan = ch
		if sess.lazySink != nil {
			sess.lazySink.Set(ch)
		}
	}

	ch := sess.eventChan
	correlationID := sess.Metadata["remediation_id"]
	sessionOwner := sess.Metadata["created_by"]
	m.store.mu.Unlock()

	var extra []string
	if user := auth.GetUserFromContext(ctx); user != "" {
		extra = append(extra, "observer_user", user)
	}
	if sessionOwner != "" {
		extra = append(extra, "session_owner", sessionOwner)
	}
	m.emitSessionEvent(ctx, audit.EventTypeSessionObserved, audit.ActionSessionObserved, audit.OutcomeSuccess, id, correlationID, nil, extra...)

	return ch, nil
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

// recoverPanic catches panics in the investigation goroutine, transitions the
// session to StatusFailed, and logs with stack context. This prevents a
// panicking LLM tool or parser from crashing the entire KA process.
func (m *Manager) recoverPanic(id, correlationID string) {
	r := recover()
	if r == nil {
		return
	}
	m.logger.Error("investigation panic recovered",
		slog.String("session_id", id),
		slog.Any("panic", r),
	)
	_ = m.store.Update(id, StatusFailed, nil, fmt.Errorf("panic: %v", r))
	m.emitSessionEvent(context.Background(), audit.EventTypeSessionFailed, audit.ActionSessionFailed, audit.OutcomeFailure, id, correlationID, fmt.Errorf("panic: %v", r))
}

// emitCompleteEvent sends an EventTypeComplete to the event sink (if active)
// to signal the SSE consumer that the investigation has finished.
func (m *Manager) emitCompleteEvent(id string) {
	m.store.mu.RLock()
	sess, ok := m.store.sessions[id]
	m.store.mu.RUnlock()
	if !ok {
		return
	}
	if sess.lazySink == nil {
		return
	}
	ch := sess.lazySink.Get()
	if ch == nil {
		return
	}
	select {
	case ch <- InvestigationEvent{Type: EventTypeComplete}:
	default:
	}
}

// EmitAccessDenied records a failed session access attempt for SOC2 CC8.1
// failed-access audit trail. Includes correlationID and session_owner for
// forensic cross-event correlation (SEC-2). Fire-and-forget per ADR-038.
func (m *Manager) EmitAccessDenied(ctx context.Context, sessionID, endpoint, requestingUser string) {
	m.store.mu.RLock()
	sess := m.store.sessions[sessionID]
	var correlationID, sessionOwner string
	if sess != nil {
		correlationID = sess.Metadata["remediation_id"]
		sessionOwner = sess.Metadata["created_by"]
	}
	m.store.mu.RUnlock()

	event := audit.NewEvent(audit.EventTypeSessionAccessDenied, correlationID)
	event.EventAction = audit.ActionSessionAccessDenied
	event.EventOutcome = audit.OutcomeFailure
	event.Data["session_id"] = sessionID
	event.Data["endpoint"] = endpoint
	event.Data["requesting_user"] = requestingUser
	if sessionOwner != "" {
		event.Data["session_owner"] = sessionOwner
	}
	audit.StoreBestEffort(ctx, m.auditStore, event, m.logger)
}

// emitSessionEvent builds and stores an audit event for a session lifecycle
// transition. Optional extraData key-value pairs are merged into the event
// data. Errors are fire-and-forget per ADR-038.
func (m *Manager) emitSessionEvent(ctx context.Context, eventType, action, outcome, sessionID, correlationID string, fnErr error, extraData ...string) {
	event := audit.NewEvent(eventType, correlationID)
	event.EventAction = action
	event.EventOutcome = outcome
	event.Data["session_id"] = sessionID
	if fnErr != nil {
		event.Data["error"] = fnErr.Error()
	}
	for i := 0; i+1 < len(extraData); i += 2 {
		event.Data[extraData[i]] = extraData[i+1]
	}
	audit.StoreBestEffort(ctx, m.auditStore, event, m.logger)
}
