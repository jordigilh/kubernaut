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
	"encoding/json"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// StartInteractiveSession creates a new session in StatusPending without
// launching the investigation goroutine. The InvestigateFunc is stored for
// deferred execution via LaunchDeferredInvestigation when the user connects
// via MCP action=start. (BR-INTERACTIVE-010)
func (m *Manager) StartInteractiveSession(ctx context.Context, fn InvestigateFunc, metadata map[string]string) (string, error) {
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

	m.store.mu.Lock()
	sess := m.store.sessions[id]
	sess.deferredFn = fn
	m.store.mu.Unlock()

	m.logger.Info("interactive session created (pending)",
		"session_id", id,
		"remediation_id", metadata["remediation_id"],
	)
	return id, nil
}

// StartInteractiveSessionWithContext creates a pending interactive session with
// typed SessionContext, preserving the full SignalContext from the AA payload.
// This ensures discover_workflows can retrieve severity, environment, priority,
// and other signal fields without reading CRDs.
func (m *Manager) StartInteractiveSessionWithContext(ctx context.Context, fn InvestigateFunc, sctx SessionContext) (string, error) {
	if user := auth.GetUserFromContext(ctx); user != "" {
		sctx.CreatedBy = user
	}
	metadata := sctx.ToMap()
	id, err := m.store.Create()
	if err != nil {
		return "", err
	}
	m.store.SetMetadata(id, metadata)
	m.store.SetContext(id, sctx)

	m.store.mu.Lock()
	sess := m.store.sessions[id]
	sess.deferredFn = fn
	m.store.mu.Unlock()

	m.logger.Info("interactive session created with context (pending)",
		"session_id", id,
		"remediation_id", sctx.RemediationID,
	)
	return id, nil
}

// ErrSessionNotPending is returned when LaunchDeferredInvestigation is called
// on a session that is not in StatusPending.
var ErrSessionNotPending = fmt.Errorf("session must be in pending state to launch deferred investigation")

// LaunchDeferredInvestigation activates a pending interactive session by
// launching the stored InvestigateFunc in a background goroutine. This is
// triggered when a user connects via MCP action=start. (BR-INTERACTIVE-010)
func (m *Manager) LaunchDeferredInvestigation(id string) error {
	m.store.mu.Lock()
	sess, ok := m.store.sessions[id]
	if !ok {
		m.store.mu.Unlock()
		return ErrSessionNotFound
	}
	if sess.Status != StatusPending {
		m.store.mu.Unlock()
		return ErrSessionNotPending
	}
	fn := sess.deferredFn
	sess.deferredFn = nil
	correlationID := sess.Metadata["remediation_id"]
	signalName := sess.Metadata["signal_name"]
	severity := sess.Metadata["severity"]
	m.store.mu.Unlock()

	if fn == nil {
		return fmt.Errorf("no deferred investigation function stored for session %s", id)
	}

	var startExtra []string
	if v := correlationID; v != "" {
		startExtra = append(startExtra, "remediation_id", v)
	}

	_, err := m.launchInvestigation(context.Background(), investigationLaunchParams{
		ID:            id,
		Fn:            fn,
		CorrelationID: correlationID,
		SignalName:    signalName,
		Severity:      severity,
		StartExtra:    startExtra,
	})
	return err
}

// UpgradeToInteractive sets the interactive upgrade flag on a running session
// without cancelling its goroutine. The background goroutine reads this flag
// via InteractiveUpgradeFromContext to decide InteractiveHold. The store-level
// mutex serialization in Update() guarantees a deterministic outcome (#1390).
// Returns ErrSessionNotFound or ErrSessionTerminal on invalid state.
func (m *Manager) UpgradeToInteractive(id string, username string, groups []string) error {
	m.store.mu.Lock()
	sess, ok := m.store.sessions[id]
	if !ok {
		m.store.mu.Unlock()
		return ErrSessionNotFound
	}
	if IsTerminal(sess.Status) || sess.Status == StatusUserDriving {
		m.store.mu.Unlock()
		return ErrSessionTerminal
	}
	sess.interactiveUpgrade.Store(true)
	if sess.Metadata == nil {
		sess.Metadata = make(map[string]string)
	}
	sess.Metadata["acting_user"] = username
	if len(groups) > 0 {
		groupsJSON, _ := json.Marshal(groups)
		sess.Metadata["acting_user_groups"] = string(groupsJSON)
	}
	correlationID := sess.Metadata["remediation_id"]
	m.store.mu.Unlock()

	m.logger.Info("session upgraded to interactive in-place",
		"session_id", id,
		"acting_user", username,
		"remediation_id", correlationID)

	m.emitSessionEvent(context.Background(), sessionEventParams{
		EventType: audit.EventTypeSessionSuspended, Action: audit.ActionSessionSuspended,
		Outcome: audit.OutcomeSuccess, SessionID: id, CorrelationID: correlationID,
	}, nil, "acting_user", username, "upgrade_type", "jump_in")

	return nil
}

// CancelInvestigation stops a running investigation by cancelling its context
// and transitioning its status to StatusCancelled. Returns ErrSessionNotFound
// if the session does not exist, or ErrSessionTerminal if it has already
// reached a terminal state.
//
// Audit: emits aiagent.session.cancelled after the status transition succeeds.
func (m *Manager) CancelInvestigation(id string) error {
	return m.terminateSession(id, audit.EventTypeSessionCancelled, audit.ActionSessionCancelled)
}

// SuspendInvestigation suspends a running autonomous investigation for interactive
// takeover. Semantically identical to CancelInvestigation but emits
// aiagent.session.suspended (DD-INTERACTIVE-002, BR-INTERACTIVE-004).
// Added in v1.5 for dynamic takeover support (BR-INTERACTIVE-004).
func (m *Manager) SuspendInvestigation(id string) error {
	return m.terminateSession(id, audit.EventTypeSessionSuspended, audit.ActionSessionSuspended)
}

// terminateSession is the shared implementation for CancelInvestigation and
// SuspendInvestigation. It cancels the session context, transitions to
// StatusCancelled, and emits the specified audit event type.
func (m *Manager) terminateSession(id, eventType, action string) error {
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
	sess.lazySink.Set(nil)
	if sess.eventChan != nil {
		close(sess.eventChan)
		sess.eventChan = nil
	}
	correlationID := sess.Metadata["remediation_id"]
	m.store.mu.Unlock()

	m.emitSessionEvent(context.Background(), sessionEventParams{
		EventType: eventType, Action: action,
		Outcome: audit.OutcomeSuccess, SessionID: id, CorrelationID: correlationID,
	}, nil)
	if eventType == audit.EventTypeSessionSuspended {
		m.metrics.RecordSessionSuspended()
	}
	return nil
}

// TransitionToUserDriving transitions an autonomous investigation session to
// user-driven mode. Cancels the investigation goroutine (stops autonomous work),
// sets Status to StatusUserDriving, and writes acting_user and acting_user_groups
// to session metadata for identity propagation to AA via the poll response.
//
// This replaces SuspendInvestigation in the takeover path. Unlike suspend, the
// session remains pollable (StatusUserDriving is non-terminal) so AA can observe
// the user's identity and session completion.
//
// Emits aiagent.session.suspended audit event for the autonomous → user transition.
func (m *Manager) TransitionToUserDriving(id, username string, groups []string) error {
	groupsJSON, err := json.Marshal(groups)
	if err != nil {
		return fmt.Errorf("marshal groups: %w", err)
	}

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
	sess.Status = StatusUserDriving

	if sess.Metadata == nil {
		sess.Metadata = make(map[string]string)
	}
	sess.Metadata["acting_user"] = username
	sess.Metadata["acting_user_groups"] = string(groupsJSON)
	correlationID := sess.Metadata["remediation_id"]
	m.store.mu.Unlock()

	m.emitSessionEvent(context.Background(), sessionEventParams{
		EventType: audit.EventTypeSessionSuspended, Action: audit.ActionSessionSuspended,
		Outcome: audit.OutcomeSuccess, SessionID: id, CorrelationID: correlationID,
	}, nil, "acting_user", username)

	m.logger.Info("Session transitioned to user-driving",
		"session_id", id, "acting_user", username,
		"groups_count", len(groups))

	return nil
}

// ForceTransitionToUserDriving locates any session matching the remediation ID
// (regardless of status) and forces it to StatusUserDriving with identity
// metadata. Unlike TransitionToUserDriving, this works even on terminal
// sessions (e.g., completed investigations). Used when the autonomous
// investigation finishes before the interactive user's takeover/start action.
func (m *Manager) ForceTransitionToUserDriving(rrID, username string, groups []string) error {
	groupsJSON, err := json.Marshal(groups)
	if err != nil {
		return fmt.Errorf("marshal groups: %w", err)
	}

	m.store.mu.Lock()
	defer m.store.mu.Unlock()

	var latest *Session
	for _, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] == rrID {
			if latest == nil || sess.CreatedAt.After(latest.CreatedAt) {
				latest = sess
			}
		}
	}
	if latest == nil {
		return ErrSessionNotFound
	}

	prevStatus := latest.Status
	if latest.cancel != nil {
		latest.cancel()
	}
	latest.Status = StatusUserDriving
	if latest.Metadata == nil {
		latest.Metadata = make(map[string]string)
	}
	latest.Metadata["acting_user"] = username
	latest.Metadata["acting_user_groups"] = string(groupsJSON)
	m.logger.Info("Force-transitioned session to user-driving",
		"remediation_id", rrID, "previous_status", string(prevStatus),
		"acting_user", username)
	return nil
}
