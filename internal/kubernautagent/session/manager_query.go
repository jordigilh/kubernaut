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

// WaitForCompletionByRemediationID returns a channel that closes once the
// most recent investigation goroutine for the given remediation_id has fully
// finished mutating session state (including any storePartialResult fallback
// write). #2155: this is the synchronization handleTakeover's
// context-reconstruction read waits on to close the race against a
// still-finishing autonomous investigation, replacing an earlier
// fixed-schedule retry loop that had no principled bound and unconditionally
// taxed every "genuinely no prior investigation" takeover with its full
// retry budget.
//
// Returns an already-closed channel if no session exists for rrID, or if the
// latest matching session never launched an investigation goroutine (e.g. a
// StatusPending deferred-interactive session) -- callers can unconditionally
// select on the result without special-casing "nothing to wait for".
func (m *Manager) WaitForCompletionByRemediationID(rrID string) <-chan struct{} {
	m.store.mu.RLock()
	var latest *Session
	for _, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] != rrID {
			continue
		}
		if latest == nil || sess.CreatedAt.After(latest.CreatedAt) {
			latest = sess
		}
	}
	var done chan struct{}
	if latest != nil {
		done = latest.done
	}
	m.store.mu.RUnlock()

	if done == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return done
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

// PersistPendingDecisionResult exposes Store.SetPendingDecisionResult so
// discover_workflows (mcp/tools) can preserve a discovered-but-unconfirmed
// workflow recommendation ahead of a possible inactivity timeout
// (#2019/#2020). See Store.SetPendingDecisionResult for the full rationale.
func (m *Manager) PersistPendingDecisionResult(id string, result *katypes.InvestigationResult) {
	m.store.SetPendingDecisionResult(id, result)
}

// CompleteUserDriving transitions a user-driven session to completed with the
// given result. This bridges the MCP tool completion path to the HTTP session
// store so AA's poll mechanism picks up the result.
func (m *Manager) CompleteUserDriving(id string, result *katypes.InvestigationResult) error {
	if err := m.store.CompleteUserDriving(id, result); err != nil {
		return err
	}
	m.closeEventChan(id)

	// #2020: hasWorkflow/humanReviewReason must be read from sess.Result under
	// the same lock acquisition as the sess lookup itself -- sess.Result is
	// mutated concurrently by Store.SetResult from the investigation goroutine
	// (see manager_events.go storePartialResult), so reading it after
	// RUnlock() is a genuine data race (caught by `go test -race` in CI).
	m.store.mu.RLock()
	sess := m.store.sessions[id]
	var correlationID string
	var hasWorkflow bool
	var humanReviewReason string
	var finalResult *katypes.InvestigationResult
	if sess != nil {
		if sess.Metadata != nil {
			correlationID = sess.Metadata["remediation_id"]
		}
		// #2020: read the final state from the session itself (sess.Result),
		// not the raw result parameter -- when the inactivity-timeout/
		// disconnect handlers call this with result=nil,
		// Store.CompleteUserDriving preserves whatever
		// SetPendingDecisionResult already attached, so logging the raw
		// parameter would misreport an actually-preserved discovery as
		// has_workflow=false.
		//
		// finalResult is captured here, still under RLock, for
		// fireTerminalHook below -- reading sess.Result again after
		// RUnlock() (as an earlier version of this code did) is a genuine
		// data race with Store.SetResult writing it concurrently from the
		// investigation goroutine (manager_events.go storePartialResult),
		// caught by `go test -race` in CI (issue #2170 CI failure).
		finalResult = sess.Result
		if finalResult != nil {
			hasWorkflow = finalResult.WorkflowID != ""
			humanReviewReason = finalResult.HumanReviewReason
		}
	}
	m.store.mu.RUnlock()

	m.emitSessionEvent(context.Background(), sessionEventParams{
		EventType: audit.EventTypeSessionCompleted, Action: audit.ActionSessionCompleted,
		Outcome: audit.OutcomeSuccess, SessionID: id, CorrelationID: correlationID,
	}, nil, "completion_mode", "user_driving")
	m.logger.Info("User-driven session completed",
		"session_id", id, "has_workflow", hasWorkflow, "human_review_reason", humanReviewReason)
	// BR-AA-KA-065.11: this is the winning commit point for a user-driven
	// completion (select_workflow/complete_no_action) -- fire the hook with
	// the final, actually-stored result (finalResult), not the raw
	// parameter, for the same #2020 reason the logging above reads it.
	if sess != nil {
		m.fireTerminalHook(id, correlationID, StatusCompleted, finalResult, nil)
	}
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
		// BR-AA-KA-065.11: this is the winning commit point for an
		// out-of-band force-complete -- the cancelled goroutine's own later
		// (rejected) store.Update, if any, will never reach this call site,
		// closing the "no silent drop" race by construction.
		m.fireTerminalHook(c.id, c.correlationID, StatusCompleted, result, nil)
	}
	return nil
}

// ForceCancelByRemediationID locates every non-terminal session (Running,
// Pending, or UserDriving) matching the given remediation ID and forces each
// one to StatusCancelled, cancelling its investigation goroutine if still
// running. Mirrors ForceCompleteByRemediationID's (#1654) iterate-then-fire-
// hooks-after-unlock pattern and multi-sibling-session semantics exactly.
//
// #2170 (DD-AA-KA-001 Amendment): this is the cleanup path for two
// call sites that have no other way to stop an orphaned investigation
// goroutine now that HTTP polling's CancelSession RPC is gone:
//   - Dispatcher.handleEvent's watch.Deleted case: the AgentSession is
//     already gone (directly deleted, or transitively via RR/AIAnalysis
//     cascade deletion) by the time this fires, so there is no CRD left to
//     poll or write a terminal status to -- the only actionable step is
//     stopping the in-memory goroutine so it does not run (and burn LLM/tool
//     budget) forever.
//   - Dispatcher.resync's TimesOutAt self-enforcement: KA independently
//     honors the same absolute deadline AA already enforces
//     (checkInvestigationTimeout, DD-TIMEOUT-002/#2176) so a
//     partitioned/crashed AA replica can never leave KA investigating
//     forever.
//
// Returns ErrSessionNotFound when no non-terminal session matched at all.
func (m *Manager) ForceCancelByRemediationID(rrID string) error {
	m.store.mu.Lock()
	type cancelledSession struct {
		id            string
		correlationID string
	}
	var cancelled []cancelledSession
	for id, sess := range m.store.sessions {
		if sess.Metadata["remediation_id"] != rrID || IsTerminal(sess.Status) {
			continue
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
		cancelled = append(cancelled, cancelledSession{id: id, correlationID: rrID})
	}
	m.store.mu.Unlock()

	if len(cancelled) == 0 {
		return ErrSessionNotFound
	}

	for _, c := range cancelled {
		m.logger.Info("Force-cancelled session by remediation ID",
			"remediation_id", rrID, "session_id", c.id)
		m.emitSessionEvent(context.Background(), sessionEventParams{
			EventType: audit.EventTypeSessionCancelled, Action: audit.ActionSessionCancelled,
			Outcome: audit.OutcomeSuccess, SessionID: c.id, CorrelationID: c.correlationID,
		}, nil)
		// Mirrors ForceCompleteByRemediationID's winning-commit-point
		// comment: firing the hook here (after unlock) is what makes this
		// the authoritative terminal transition, regardless of whether the
		// cancelled goroutine's own (now-rejected) completion race loses.
		m.fireTerminalHook(c.id, c.correlationID, StatusCancelled, nil, nil)
	}
	return nil
}
