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
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// FindByRemediationID scans running sessions for one whose metadata
// "remediation_id" matches the given rrID. Returns the session ID and true
// if found, or ("", false) otherwise. Uses RLock for safe concurrent access.
// BR-INTERACTIVE-004: enables dynamic takeover by mapping rrID → autonomous session.
func (m *Manager) FindByRemediationID(rrID string) (string, bool) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()
	for id, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] == rrID && sess.Status == StatusRunning {
			return id, true
		}
	}
	return "", false
}

// FindPendingByRemediationID scans for a pending interactive session whose
// metadata "remediation_id" matches the given rrID. BR-INTERACTIVE-010:
// enables MCP action=start to detect and launch deferred investigations.
func (m *Manager) FindPendingByRemediationID(rrID string) (string, bool) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()
	for id, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] == rrID && sess.Status == StatusPending {
			return id, true
		}
	}
	return "", false
}

// isFallbackSession reports whether sess is a placeholder created by
// createFallbackSession (mode=interactive_fallback) rather than a genuine
// investigation. Its canned RCASummary ("Interactive session — awaiting user
// direction") must never be surfaced by the RCA-summary lookups below as if
// it were real investigation output (#1640).
func isFallbackSession(sess *Session) bool {
	return sess.Metadata["mode"] == "interactive_fallback"
}

// GetLatestRCASummaryByRemediationID returns the RCA summary from the most
// recent completed/user-driving session for the given remediation_id, if any.
// BR-INTERACTIVE-010: enables context reconstruction to use the concise RCA
// summary instead of full audit trail reconstruction when available.
func (m *Manager) GetLatestRCASummaryByRemediationID(rrID string) (string, bool) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()
	var latestTime time.Time
	var latestSummary string
	for _, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] != rrID {
			continue
		}
		if isFallbackSession(sess) {
			continue
		}
		if sess.Result == nil || sess.Result.RCASummary == "" {
			continue
		}
		if sess.CreatedAt.After(latestTime) {
			latestTime = sess.CreatedAt
			latestSummary = sess.Result.RCASummary
		}
	}
	if latestSummary == "" {
		return "", false
	}
	return latestSummary, true
}

// GetLatestRCAResultByRemediationID returns the full InvestigationResult from
// the most recent completed (non-cancelled) session for the given remediation_id.
// This gives workflow discovery access to the complete RemediationTarget produced
// by the autonomous Phase 1 RCA, avoiding a lossy re-extraction from conversation.
// Cancelled sessions are excluded because their partial results may be stale or
// incomplete (KA-HIGH-5).
func (m *Manager) GetLatestRCAResultByRemediationID(rrID string) (*katypes.InvestigationResult, bool) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()
	var latestTime time.Time
	var latestResult *katypes.InvestigationResult
	for _, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] != rrID {
			continue
		}
		if isFallbackSession(sess) {
			continue
		}
		if sess.Result == nil {
			continue
		}
		if sess.Status == StatusCancelled {
			continue
		}
		if sess.CreatedAt.After(latestTime) {
			latestTime = sess.CreatedAt
			latestResult = sess.Result
		}
	}
	if latestResult == nil {
		return nil, false
	}
	return latestResult, true
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
		sess.lazySink.Set(ch)
		m.logger.Info("Subscribe: LazySink channel activated",
			"session_id", id,
			"status", string(sess.Status),
			"chan_ptr", fmt.Sprintf("%p", ch))
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
	m.emitSessionEvent(ctx, sessionEventParams{
		EventType: audit.EventTypeSessionObserved, Action: audit.ActionSessionObserved,
		Outcome: audit.OutcomeSuccess, SessionID: id, CorrelationID: correlationID,
	}, nil, extra...)

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
	sess.lazySink.Set(nil)
	if sess.eventChan != nil {
		close(sess.eventChan)
		sess.eventChan = nil
	}
}

// Shutdown cancels all running investigations to allow a clean process exit.
// It fires the context cancellation for each active session and transitions
// them to StatusCancelled. This is intended to be called from a SIGTERM
// handler so that in-flight LLM calls are aborted promptly.
func (m *Manager) Shutdown() {
	m.store.mu.Lock()
	var running []string
	for id, sess := range m.store.sessions {
		if sess.Status == StatusRunning || sess.Status == StatusPending {
			if sess.cancel != nil {
				sess.cancel()
			}
			sess.Status = StatusCancelled
			sess.lazySink.Set(nil)
			if sess.eventChan != nil {
				close(sess.eventChan)
				sess.eventChan = nil
			}
			running = append(running, id)
		}
	}
	m.store.mu.Unlock()

	for _, id := range running {
		m.logger.Info("shutdown: cancelled investigation", "session_id", id)
	}
}

// GetSession retrieves the current state of an investigation session.
func (m *Manager) GetSession(id string) (*Session, error) {
	return m.store.Get(id)
}

// GetSessionContext retrieves only the typed SessionContext for a session.
// Returns ErrSessionNotFound if the session does not exist.
func (m *Manager) GetSessionContext(id string) (*SessionContext, error) {
	sess, err := m.store.Get(id)
	if err != nil {
		return nil, err
	}
	ctx := sess.Context
	return &ctx, nil
}

// GetSignalForRemediation looks up any non-terminal session associated with the
// given remediationID and returns its SignalContext. This enables interactive
// tools (discover_workflows, select_workflow) to inherit the full signal context
// (severity, environment, priority) from the original AA payload without reading
// CRDs. Searches Running, Pending, and UserDriving sessions.
// Returns ErrSessionNotFound if no matching session has a stored signal.
func (m *Manager) GetSignalForRemediation(rrID string) (*katypes.SignalContext, error) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()
	for _, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] != rrID {
			continue
		}
		if sess.Context.Signal.Name != "" || sess.Context.Signal.Severity != "" {
			signal := sess.Context.Signal
			return &signal, nil
		}
	}
	return nil, ErrSessionNotFound
}

// CompleteUserDriving transitions a user-driven session to completed with the
// given result. This bridges the MCP tool completion path to the HTTP session
// store so AA's poll mechanism picks up the result.
func (m *Manager) CompleteUserDriving(id string, result *katypes.InvestigationResult) error {
	if err := m.store.CompleteUserDriving(id, result); err != nil {
		return err
	}
	m.closeEventChan(id)

	m.store.mu.RLock()
	sess := m.store.sessions[id]
	var correlationID string
	if sess != nil && sess.Metadata != nil {
		correlationID = sess.Metadata["remediation_id"]
	}
	m.store.mu.RUnlock()

	m.emitSessionEvent(context.Background(), sessionEventParams{
		EventType: audit.EventTypeSessionCompleted, Action: audit.ActionSessionCompleted,
		Outcome: audit.OutcomeSuccess, SessionID: id, CorrelationID: correlationID,
	}, nil, "completion_mode", "user_driving")
	m.logger.Info("User-driven session completed",
		"session_id", id, "has_workflow", result != nil && result.WorkflowID != "")
	return nil
}

// FindUserDrivingByRemediationID scans user-driving sessions for one whose
// metadata "remediation_id" matches the given rrID. Returns the session ID and
// true if found. Used by select_workflow and complete_no_action to locate the
// HTTP session for result propagation.
func (m *Manager) FindUserDrivingByRemediationID(rrID string) (string, bool) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()
	for id, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] == rrID && sess.Status == StatusUserDriving {
			return id, true
		}
	}
	return "", false
}

// GetSessionLazySink returns the LazySink for the given session ID so that
// callers (e.g. handleDiscoverWorkflows) can attach it to a context for
// streaming events during workflow discovery (#1384).
func (m *Manager) GetSessionLazySink(id string) (*LazySink, bool) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()
	sess, ok := m.store.sessions[id]
	if !ok {
		return nil, false
	}
	return sess.lazySink, true
}

// ForceCompleteByRemediationID locates every non-terminal session (Running,
// Pending, or UserDriving) matching the given remediation ID and forces each
// one to StatusCompleted with the provided result, cancelling its
// investigation goroutine if still running.
//
// This is the fallback path for MCP tools (complete_no_action, action:complete,
// select_workflow) when TransitionToUserDriving was not called or failed because
// the autonomous investigation started after MCP session acquisition, or had
// already completed before takeover.
//
// #1654: iterates over ALL matching non-terminal sessions rather than
// returning after the first. Duplicate sessions for the same remediation_id
// can coexist (e.g. an MCP action=start fallback session alongside AA's own
// autonomous investigation session) — completing only the first one found
// left the other stuck non-terminal, with AA (or the inactivity timer)
// waiting on a session that would never transition. Returns
// ErrSessionNotFound only when no non-terminal session matched at all.
func (m *Manager) ForceCompleteByRemediationID(rrID string, result *katypes.InvestigationResult) error {
	m.store.mu.Lock()
	type completedSession struct {
		id            string
		prevStatus    Status
		correlationID string
	}
	var completed []completedSession
	for id, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] != rrID || IsTerminal(sess.Status) {
			continue
		}
		prevStatus := sess.Status
		if sess.cancel != nil {
			sess.cancel()
		}
		sess.Status = StatusCompleted
		if result != nil {
			sess.Result = result
		}
		sess.lazySink.Set(nil)
		if sess.eventChan != nil {
			close(sess.eventChan)
			sess.eventChan = nil
		}
		completed = append(completed, completedSession{id: id, prevStatus: prevStatus, correlationID: rrID})
	}
	m.store.mu.Unlock()

	if len(completed) == 0 {
		return ErrSessionNotFound
	}

	for _, c := range completed {
		m.logger.Info("Force-completed session by remediation ID",
			"remediation_id", rrID, "session_id", c.id, "previous_status", string(c.prevStatus))
		m.emitSessionEvent(context.Background(), sessionEventParams{
			EventType: audit.EventTypeSessionCompleted, Action: audit.ActionSessionCompleted,
			Outcome: audit.OutcomeSuccess, SessionID: c.id, CorrelationID: c.correlationID,
		}, nil, "completion_mode", "force_complete", "previous_status", string(c.prevStatus))
	}
	return nil
}
