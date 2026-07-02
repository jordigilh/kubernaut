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
	"time"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

// storePartialResult attaches a result to a session that is already in a
// terminal state (e.g. StatusCancelled). Delegates to Store.SetResult which
// does not change the session status — only the Result field. This preserves
// partial investigation state for snapshot retrieval (BR-SESSION-002).
func (m *Manager) storePartialResult(id string, result *katypes.InvestigationResult) {
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
	m.logger.Error(fmt.Errorf("panic: %v", r), "investigation panic recovered",
		"session_id", id,
	)
	if updateErr := m.store.Update(id, StatusFailed, nil, fmt.Errorf("panic: %v", r)); updateErr != nil {
		m.logger.Error(updateErr, "failed to update session after panic",
			"session_id", id, "target_status", string(StatusFailed))
	}
	m.emitSessionEvent(context.Background(), sessionEventParams{
		EventType: audit.EventTypeSessionFailed, Action: audit.ActionSessionFailed,
		Outcome: audit.OutcomeFailure, SessionID: id, CorrelationID: correlationID,
	}, fmt.Errorf("panic: %v", r))
}

// emitCompleteEvent sends an EventTypeComplete to the event sink (if active)
// to signal the SSE consumer that the investigation has finished.
func (m *Manager) emitCompleteEvent(id string) {
	m.store.mu.RLock()
	sess, ok := m.store.sessions[id]
	var sink *LazySink
	if ok {
		sink = sess.lazySink
	}
	m.store.mu.RUnlock()

	if sink == nil {
		return
	}
	ch := sink.Get()
	if ch == nil {
		return
	}
	select {
	case ch <- InvestigationEvent{Type: EventTypeComplete}:
	default:
	}
}

// EmitSessionEndedByRR emits a terminal InvestigationEvent with the given
// reason to the event channel of the user_driving session associated with the
// given rrID. This signals SSE consumers (EventLogBridge -> AF -> A2A) that
// the session is ending before the channel is closed (#1438, SI-4).
// No-op if no user_driving session matches.
func (m *Manager) EmitSessionEndedByRR(rrID, reason string) {
	sessionID, found := m.FindUserDrivingByRemediationID(rrID)
	if !found {
		m.logger.V(1).Info("EmitSessionEndedByRR: no user_driving session for rr_id",
			"rr_id", rrID, "reason", reason)
		return
	}
	m.emitTerminalEvent(sessionID, reason)
}

// emitTerminalEvent sends an EventTypeSessionEnded to the event sink (if
// active) with the given reason as Phase. Mirrors emitCompleteEvent but
// carries the release reason for Console phase transition (#1438).
func (m *Manager) emitTerminalEvent(id, reason string) {
	m.store.mu.RLock()
	sess, ok := m.store.sessions[id]
	var sink *LazySink
	if ok {
		sink = sess.lazySink
	}
	m.store.mu.RUnlock()

	if sink == nil {
		m.logger.V(1).Info("emitTerminalEvent: no sink for session",
			"session_id", id, "reason", reason)
		return
	}
	ch := sink.Get()
	if ch == nil {
		m.logger.V(1).Info("emitTerminalEvent: sink channel is nil",
			"session_id", id, "reason", reason)
		return
	}
	select {
	case ch <- InvestigationEvent{Type: EventTypeSessionEnded, Phase: reason}:
	default:
		m.logger.Info("terminal event dropped: channel full",
			"session_id", id, "reason", reason)
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

	event := audit.NewEvent(audit.EventTypeSessionAccessDenied, correlationID, audit.WithSessionID(sessionID))
	event.EventAction = audit.ActionSessionAccessDenied
	event.EventOutcome = audit.OutcomeFailure
	event.Data["endpoint"] = endpoint
	event.Data["requesting_user"] = requestingUser
	if sessionOwner != "" {
		event.Data["session_owner"] = sessionOwner
	}
	audit.StoreBestEffort(ctx, m.auditStore, event, m.logger)
}

// recordSessionMetrics records session completion metrics when the investigation
// goroutine exits. Reads final status from the store (COR-3) to handle the race
// where a session is cancelled while the goroutine is still running.
func (m *Manager) recordSessionMetrics(id string, start time.Time) {
	duration := time.Since(start).Seconds()
	m.store.mu.RLock()
	sess := m.store.sessions[id]
	var outcome string
	if sess != nil {
		outcome = string(sess.Status)
	}
	m.store.mu.RUnlock()
	if outcome == "" {
		outcome = "unknown"
	}
	m.metrics.RecordSessionCompleted(outcome, duration)
}

// sessionEventParams groups the fixed arguments shared by emitSessionEvent's
// callers. Extracted per AGENTS.md's 8+-param Options-pattern rule.
type sessionEventParams struct {
	EventType     string
	Action        string
	Outcome       string
	SessionID     string
	CorrelationID string
}

// emitSessionEvent builds and stores an audit event for a session lifecycle
// transition. Optional extraData key-value pairs are merged into the event
// data. Errors are fire-and-forget per ADR-038.
func (m *Manager) emitSessionEvent(ctx context.Context, p sessionEventParams, fnErr error, extraData ...string) {
	event := audit.NewEvent(p.EventType, p.CorrelationID, audit.WithSessionID(p.SessionID))
	event.EventAction = p.Action
	event.EventOutcome = p.Outcome
	if fnErr != nil {
		event.Data["error"] = fnErr.Error()
	}
	for i := 0; i+1 < len(extraData); i += 2 {
		event.Data[extraData[i]] = extraData[i+1]
	}
	audit.StoreBestEffort(ctx, m.auditStore, event, m.logger)
}
