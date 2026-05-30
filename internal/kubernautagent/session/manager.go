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
	"time"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/metrics"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// InvestigateFunc is the function signature for running an investigation.
// The interface{} return is intentional: the session subsystem is type-agnostic
// because the result is ultimately JSON-marshaled for the HTTP response. Using
// generics here would propagate type parameters through Manager/Store/Session
// with no safety benefit (#8 conscious decision).
type InvestigateFunc func(ctx context.Context) (*katypes.InvestigationResult, error)

// Manager orchestrates investigation sessions, running each in a
// background goroutine and tracking progress via the Store.
type Manager struct {
	store      *Store
	logger     logr.Logger
	auditStore audit.AuditStore
	metrics    *metrics.Metrics
}

// NewManager creates a session manager backed by the given store.
// If auditStore is nil, a NopAuditStore is used (no audit events emitted).
// metrics may be nil (all metric calls are nil-safe per OPS-1).
func NewManager(store *Store, logger logr.Logger, auditStore audit.AuditStore, m *metrics.Metrics) *Manager {
	if auditStore == nil {
		auditStore = audit.NopAuditStore{}
	}
	return &Manager{store: store, logger: logger, auditStore: auditStore, metrics: m}
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

	return m.launchInvestigation(ctx, id, fn, correlationID, metadata["signal_name"], metadata["severity"], startExtra)
}

// StartInvestigationWithContext creates a new session with typed SessionContext
// and launches the investigation function in a background goroutine.
// This is the typed alternative to StartInvestigation that preserves the full
// SignalContext for interactive takeover. The Metadata map is populated from
// SessionContext.ToMap() for backward compatibility with audit events and
// existing code that reads Metadata.
func (m *Manager) StartInvestigationWithContext(ctx context.Context, fn InvestigateFunc, sctx SessionContext) (string, error) {
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

	correlationID := sctx.RemediationID
	var startExtra []string
	if sctx.IncidentID != "" {
		startExtra = append(startExtra, "incident_id", sctx.IncidentID)
	}
	if sctx.Signal.Name != "" {
		startExtra = append(startExtra, "signal_name", sctx.Signal.Name)
	}
	if sctx.Signal.Severity != "" {
		startExtra = append(startExtra, "severity", sctx.Signal.Severity)
	}
	if sctx.CreatedBy != "" {
		startExtra = append(startExtra, "created_by", sctx.CreatedBy)
	}

	return m.launchInvestigation(ctx, id, fn, correlationID, sctx.Signal.Name, sctx.Signal.Severity, startExtra)
}

// launchInvestigation is the shared goroutine launcher used by both
// StartInvestigation and StartInvestigationWithContext. It wires the cancel
// context, lazy sink, emits the started audit event, and spawns the goroutine.
func (m *Manager) launchInvestigation(ctx context.Context, id string, fn InvestigateFunc, correlationID, signalName, severity string, startExtra []string) (string, error) {
	bgCtx, cancelFn := context.WithCancel(context.Background())

	ls := &LazySink{}
	bgCtx = WithLazySink(bgCtx, ls)
	bgCtx = WithSessionID(bgCtx, id)

	m.store.mu.Lock()
	sess := m.store.sessions[id]
	sess.cancel = cancelFn
	sess.lazySink = ls
	m.logger.Info("launchInvestigation: LazySink attached to session",
		"session_id", id,
		"status", string(sess.Status),
		"has_deferred_fn", sess.deferredFn != nil)
	m.store.mu.Unlock()

	if updateErr := m.store.Update(id, StatusRunning, nil, nil); updateErr != nil {
		m.logger.Error(updateErr, "failed to update session",
			"session_id", id, "target_status", string(StatusRunning))
	}

	m.emitSessionEvent(ctx, audit.EventTypeSessionStarted, audit.ActionSessionStarted, audit.OutcomeSuccess, id, correlationID, nil, startExtra...)
	m.metrics.RecordSessionStarted(signalName, severity)

	go func() {
		start := time.Now()
		defer m.recordSessionMetrics(id, start)
		defer m.closeEventChan(id)
		defer m.recoverPanic(id, correlationID)

		result, fnErr := fn(bgCtx)
		m.emitCompleteEvent(id)
		if fnErr != nil {
			m.logger.Error(fnErr, "investigation failed", "session_id", id)
			if updateErr := m.store.Update(id, StatusFailed, nil, fnErr); updateErr != nil {
				m.logger.Info("post-investigation status update rejected",
					"session_id", id,
					"attempted_status", string(StatusFailed),
					"reason", updateErr.Error())
				if bgCtx.Err() != nil {
					m.storePartialResult(id, nil)
				}
			} else {
				m.emitSessionEvent(context.Background(), audit.EventTypeSessionFailed, audit.ActionSessionFailed, audit.OutcomeFailure, id, correlationID, fnErr)
			}
			return
		}
		targetStatus := StatusCompleted
		if result != nil && result.InteractiveHold {
			targetStatus = StatusUserDriving
		}
		if updateErr := m.store.Update(id, targetStatus, result, nil); updateErr != nil {
			m.logger.Info("post-investigation status update rejected",
				"session_id", id,
				"attempted_status", string(targetStatus),
				"reason", updateErr.Error())
			if bgCtx.Err() != nil {
				m.storePartialResult(id, result)
			}
		} else {
			m.emitSessionEvent(context.Background(), audit.EventTypeSessionCompleted, audit.ActionSessionCompleted, audit.OutcomeSuccess, id, correlationID, nil)
		}
	}()

	return id, nil
}

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

	_, err := m.launchInvestigation(context.Background(), id, fn, correlationID, signalName, severity, startExtra)
	return err
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
	correlationID := sess.Metadata["remediation_id"]
	m.store.mu.Unlock()

	m.emitSessionEvent(context.Background(), eventType, action, audit.OutcomeSuccess, id, correlationID, nil)
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

	m.emitSessionEvent(context.Background(),
		audit.EventTypeSessionSuspended, audit.ActionSessionSuspended,
		audit.OutcomeSuccess, id, correlationID, nil,
		"acting_user", username)

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
	for _, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] == rrID {
			prevStatus := sess.Status
			if sess.cancel != nil {
				sess.cancel()
			}
			sess.Status = StatusUserDriving
			if sess.Metadata == nil {
				sess.Metadata = make(map[string]string)
			}
			sess.Metadata["acting_user"] = username
			sess.Metadata["acting_user_groups"] = string(groupsJSON)
			m.logger.Info("Force-transitioned session to user-driving",
				"remediation_id", rrID, "previous_status", string(prevStatus),
				"acting_user", username)
			return nil
		}
	}
	return ErrSessionNotFound
}

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
// the most recent completed session for the given remediation_id. This gives
// workflow discovery access to the complete RemediationTarget produced by the
// autonomous Phase 1 RCA, avoiding a lossy re-extraction from conversation.
func (m *Manager) GetLatestRCAResultByRemediationID(rrID string) (*katypes.InvestigationResult, bool) {
	m.store.mu.RLock()
	defer m.store.mu.RUnlock()
	var latestTime time.Time
	var latestResult *katypes.InvestigationResult
	for _, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] != rrID {
			continue
		}
		if sess.Result == nil {
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
		if sess.lazySink != nil {
			sess.lazySink.Set(ch)
			m.logger.Info("Subscribe: LazySink channel activated",
				"session_id", id,
				"status", string(sess.Status),
				"has_lazy_sink", true,
				"chan_ptr", fmt.Sprintf("%p", ch))
		} else {
			m.logger.Info("Subscribe: LazySink is nil — events will NOT flow",
				"session_id", id,
				"status", string(sess.Status))
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
	m.store.mu.RLock()
	sess := m.store.sessions[id]
	var correlationID string
	if sess != nil && sess.Metadata != nil {
		correlationID = sess.Metadata["remediation_id"]
	}
	m.store.mu.RUnlock()

	m.emitSessionEvent(context.Background(),
		audit.EventTypeSessionCompleted, audit.ActionSessionCompleted,
		audit.OutcomeSuccess, id, correlationID, nil,
		"completion_mode", "user_driving")
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

// ForceCompleteByRemediationID locates any session (Running, UserDriving, or
// Completed) matching the given remediation ID and forces it to StatusCompleted
// with the provided result. Cancels the investigation goroutine if still running.
//
// This is the fallback path for MCP tools (complete_no_action, action:complete,
// select_workflow) when TransitionToUserDriving was not called or failed because
// the autonomous investigation started after MCP session acquisition, or had
// already completed before takeover.
func (m *Manager) ForceCompleteByRemediationID(rrID string, result *katypes.InvestigationResult) error {
	m.store.mu.Lock()
	defer m.store.mu.Unlock()
	for _, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] == rrID && !IsTerminal(sess.Status) {
			prevStatus := sess.Status
			if sess.cancel != nil {
				sess.cancel()
			}
			sess.Status = StatusCompleted
			sess.Result = result
			m.logger.Info("Force-completed session by remediation ID",
				"remediation_id", rrID, "previous_status", string(prevStatus))
			return nil
		}
	}
	return ErrSessionNotFound
}

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
	m.emitSessionEvent(context.Background(), audit.EventTypeSessionFailed, audit.ActionSessionFailed, audit.OutcomeFailure, id, correlationID, fmt.Errorf("panic: %v", r))
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

// emitSessionEvent builds and stores an audit event for a session lifecycle
// transition. Optional extraData key-value pairs are merged into the event
// data. Errors are fire-and-forget per ADR-038.
func (m *Manager) emitSessionEvent(ctx context.Context, eventType, action, outcome, sessionID, correlationID string, fnErr error, extraData ...string) {
	event := audit.NewEvent(eventType, correlationID, audit.WithSessionID(sessionID))
	event.EventAction = action
	event.EventOutcome = outcome
	if fnErr != nil {
		event.Data["error"] = fnErr.Error()
	}
	for i := 0; i+1 < len(extraData); i += 2 {
		event.Data[extraData[i]] = extraData[i+1]
	}
	audit.StoreBestEffort(ctx, m.auditStore, event, m.logger)
}
