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

// TerminalSnapshot carries the fields a TerminalHook needs to write a
// curated AgentSession.Status (DD-AA-KA-001 Amendment Gap 2, BR-AA-KA-065.11)
// without the hook implementation reaching back into Manager-internal
// locking. Status is the just-committed Store status: StatusCompleted,
// StatusFailed, or StatusCancelled for a genuine terminal outcome, or
// StatusUserDriving when an autonomous investigation self-resolved into an
// InteractiveHold (Result.InteractiveHold=true) -- distinct from an
// explicit human takeover, which fires InteractiveUpgradeHookFunc instead.
type TerminalSnapshot struct {
	SessionID     string
	RemediationID string
	Status        Status
	Result        *katypes.InvestigationResult
	Err           error
}

// TerminalHookFunc is invoked exactly once for the single call site that
// wins a session's terminal (or InteractiveHold) state transition --
// handleInvestigationSuccess/handleInvestigationFailure/terminateSession
// when the goroutine itself wins, or CompleteUserDriving/
// ForceCompleteByRemediationID when an out-of-band MCP completion wins
// instead. Nil-safe: Manager checks for a nil hook before calling.
type TerminalHookFunc func(TerminalSnapshot)

// InteractiveUpgradeHookFunc is invoked exactly once when a human driver
// takes over a session -- UpgradeToInteractive (in-place jump-in),
// TransitionToUserDriving, or ForceTransitionToUserDriving (dynamic
// takeover). Distinct from an autonomous InteractiveHold (which has no
// acting user yet and fires TerminalHookFunc instead). Nil-safe.
type InteractiveUpgradeHookFunc func(sessionID, remediationID, username string, groups []string)

// Manager orchestrates investigation sessions, running each in a
// background goroutine and tracking progress via the Store.
type Manager struct {
	store      *Store
	logger     logr.Logger
	auditStore audit.AuditStore
	metrics    *metrics.Metrics

	// terminalHook and interactiveUpgradeHook are optional, nil-safe
	// notification points for a CRD-status writer (agentsession.Dispatcher)
	// registered once at wiring time (DD-AA-KA-001 Amendment Gap 2). Manager
	// itself has no CRD awareness; it only reports the fact that a
	// transition was committed.
	terminalHook           TerminalHookFunc
	interactiveUpgradeHook InteractiveUpgradeHookFunc
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

// SetTerminalHook registers fn as the Manager's TerminalHook. Overwrites
// any previously registered hook. Passing nil disables the hook.
func (m *Manager) SetTerminalHook(fn TerminalHookFunc) {
	m.terminalHook = fn
}

// SetInteractiveUpgradeHook registers fn as the Manager's
// InteractiveUpgradeHook. Overwrites any previously registered hook.
// Passing nil disables the hook.
func (m *Manager) SetInteractiveUpgradeHook(fn InteractiveUpgradeHookFunc) {
	m.interactiveUpgradeHook = fn
}

// fireTerminalHook invokes the registered TerminalHook, if any, with a
// snapshot built from the session's post-commit state. correlationID is
// passed explicitly (rather than re-read from sess) since callers already
// hold it from the same critical section that committed the transition.
func (m *Manager) fireTerminalHook(id, correlationID string, status Status, result *katypes.InvestigationResult, err error) {
	if m.terminalHook == nil {
		return
	}
	m.terminalHook(TerminalSnapshot{
		SessionID:     id,
		RemediationID: correlationID,
		Status:        status,
		Result:        result,
		Err:           err,
	})
}

// fireInteractiveUpgradeHook invokes the registered InteractiveUpgradeHook,
// if any.
func (m *Manager) fireInteractiveUpgradeHook(id, correlationID, username string, groups []string) {
	if m.interactiveUpgradeHook == nil {
		return
	}
	m.interactiveUpgradeHook(id, correlationID, username, groups)
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

	return m.launchInvestigation(ctx, investigationLaunchParams{
		ID:            id,
		Fn:            fn,
		CorrelationID: correlationID,
		SignalName:    metadata["signal_name"],
		Severity:      metadata["severity"],
		StartExtra:    startExtra,
	})
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

	return m.launchInvestigation(ctx, investigationLaunchParams{
		ID:            id,
		Fn:            fn,
		CorrelationID: correlationID,
		SignalName:    sctx.Signal.Name,
		Severity:      sctx.Signal.Severity,
		ClusterName:   sctx.Signal.ClusterName,
		StartExtra:    startExtra,
	})
}

// investigationLaunchParams groups the arguments shared by launchInvestigation's
// callers. Extracted per AGENTS.md's 8+-param Options-pattern rule.
type investigationLaunchParams struct {
	ID            string
	Fn            InvestigateFunc
	CorrelationID string
	SignalName    string
	Severity      string
	ClusterName   string
	StartExtra    []string
}

// launchInvestigation is the shared goroutine launcher used by both
// StartInvestigation and StartInvestigationWithContext. It wires the cancel
// context, lazy sink, emits the started audit event, and spawns the goroutine.
func (m *Manager) launchInvestigation(ctx context.Context, p investigationLaunchParams) (string, error) {
	id, fn, correlationID, signalName, severity, clusterName, startExtra :=
		p.ID, p.Fn, p.CorrelationID, p.SignalName, p.Severity, p.ClusterName, p.StartExtra

	// The cancel func returned here is also stored on the session entry by
	// attachInvestigationContext, so cancellation (e.g. session abandonment)
	// is driven from there rather than from this call site.
	bgCtx, _, done := m.attachInvestigationContext(id, correlationID, clusterName)

	if updateErr := m.store.Update(id, StatusRunning, nil, nil); updateErr != nil {
		m.logger.Error(updateErr, "failed to update session",
			"session_id", id, "target_status", string(StatusRunning))
	}

	m.emitSessionEvent(ctx, sessionEventParams{
		EventType: audit.EventTypeSessionStarted, Action: audit.ActionSessionStarted,
		Outcome: audit.OutcomeSuccess, SessionID: id, CorrelationID: correlationID,
	}, nil, startExtra...)
	m.metrics.RecordSessionStarted(signalName, severity)

	go m.runInvestigation(bgCtx, id, correlationID, fn, done) //nolint:contextcheck // runInvestigation is launched as a detached goroutine that must outlive the triggering request

	return id, nil
}

// attachInvestigationContext builds the background context an investigation
// runs under (independent of the request context, carrying session ID,
// cluster name, correlation ID, and a lazily-attached event sink) and wires
// its cancel func + sink onto the session store entry so later operations
// (cancellation, interactive upgrade) can reach them. The returned done
// channel is closed exactly once by runInvestigation's cleanup, after it has
// fully finished mutating session state (#2155) -- see the doc comment on
// Session.done for the race this closes.
func (m *Manager) attachInvestigationContext(id, correlationID, clusterName string) (context.Context, context.CancelFunc, chan struct{}) {
	bgCtx, cancelFn := context.WithCancel(context.Background())
	done := make(chan struct{})

	ls := &LazySink{}
	bgCtx = WithLazySink(bgCtx, ls)
	bgCtx = WithSessionID(bgCtx, id)
	bgCtx = audit.WithClusterID(bgCtx, clusterName)
	// GAP-13 (Issue #1505): correlationID on ctx lets deep call sites (e.g.
	// the K8s resolver's secret-access observer) emit correctly-correlated
	// audit events without threading correlationID through every signature.
	bgCtx = audit.WithCorrelationID(bgCtx, correlationID)

	m.store.mu.Lock()
	sess := m.store.sessions[id]
	sess.cancel = cancelFn
	sess.done = done
	sess.lazySink = ls
	bgCtx = WithInteractiveUpgrade(bgCtx, sess.interactiveUpgrade)
	m.logger.Info("launchInvestigation: LazySink attached to session",
		"session_id", id,
		"status", string(sess.Status),
		"has_deferred_fn", sess.deferredFn != nil)
	m.store.mu.Unlock()

	return bgCtx, cancelFn, done
}

// runInvestigation executes fn in the background, then reconciles the
// session's terminal status (failed/completed/user-driving) and emits the
// corresponding lifecycle audit event. Runs as its own goroutine; panics are
// recovered and session/duration metrics are always recorded.
//
// #2155: done is closed last (deferred first, so by Go's LIFO defer order it
// runs after recoverPanic/recordSessionMetrics and after every session-state
// mutation below), giving WaitForCompletionByRemediationID a real
// happens-before signal that this goroutine has fully finished.
func (m *Manager) runInvestigation(bgCtx context.Context, id, correlationID string, fn InvestigateFunc, done chan struct{}) {
	start := time.Now()
	defer close(done)
	defer m.recordSessionMetrics(id, start)
	defer m.recoverPanic(id, correlationID) //nolint:contextcheck // recoverPanic emits a best-effort failure audit event; must survive whatever context state caused the panic

	result, fnErr := fn(bgCtx)
	m.emitCompleteEvent(id, result)
	if fnErr != nil {
		m.handleInvestigationFailure(bgCtx, id, correlationID, result, fnErr)
		return
	}
	m.handleInvestigationSuccess(bgCtx, id, correlationID, result)
}

// handleInvestigationFailure marks the session failed and emits the
// SessionFailed audit event, falling back to storing a partial result when
// the status update is rejected (e.g. the session was cancelled/superseded)
// and the background context was itself cancelled.
func (m *Manager) handleInvestigationFailure(bgCtx context.Context, id, correlationID string, result *katypes.InvestigationResult, fnErr error) {
	m.logger.Error(fnErr, "investigation failed", "session_id", id)
	if updateErr := m.store.Update(id, StatusFailed, nil, fnErr); updateErr != nil {
		m.logger.Info("post-investigation status update rejected",
			"session_id", id,
			"attempted_status", string(StatusFailed),
			"reason", updateErr.Error())
		if bgCtx.Err() != nil {
			m.storePartialResult(id, result)
		}
		return
	}
	m.closeEventChan(id)
	m.emitSessionEvent(context.Background(), sessionEventParams{ //nolint:contextcheck // emitSessionEvent forwards to audit.StoreBestEffort by design, decoupled from the triggering goroutine's context
		EventType: audit.EventTypeSessionFailed, Action: audit.ActionSessionFailed,
		Outcome: audit.OutcomeFailure, SessionID: id, CorrelationID: correlationID,
	}, fnErr)
	m.fireTerminalHook(id, correlationID, StatusFailed, result, fnErr)
}

// handleInvestigationSuccess determines the session's terminal status from
// the result (completed, or user-driving when the investigation handed off
// to an interactive hold), updates the store, and emits the SessionCompleted
// audit event. Falls back to storing a partial result when the status
// update is rejected and the background context was cancelled.
func (m *Manager) handleInvestigationSuccess(bgCtx context.Context, id, correlationID string, result *katypes.InvestigationResult) {
	targetStatus := StatusCompleted
	if result != nil && result.InteractiveHold {
		targetStatus = StatusUserDriving
	}
	if result != nil && result.HumanReviewNeeded && result.HumanReviewReason == katypes.HumanReviewReasonAlignmentCheckFailed {
		targetStatus = StatusCompleted
	}
	if updateErr := m.store.Update(id, targetStatus, result, nil); updateErr != nil {
		m.logger.Info("post-investigation status update rejected",
			"session_id", id,
			"attempted_status", string(targetStatus),
			"reason", updateErr.Error())
		if bgCtx.Err() != nil {
			m.storePartialResult(id, result)
		}
		return
	}
	if targetStatus != StatusUserDriving {
		m.closeEventChan(id)
	}
	m.emitSessionEvent(context.Background(), sessionEventParams{ //nolint:contextcheck // emitSessionEvent forwards to audit.StoreBestEffort by design, decoupled from the triggering goroutine's context
		EventType: audit.EventTypeSessionCompleted, Action: audit.ActionSessionCompleted,
		Outcome: audit.OutcomeSuccess, SessionID: id, CorrelationID: correlationID,
	}, nil)
	// Re-read the committed session rather than trusting the local
	// targetStatus: store.Update's #1390 deterministic-upgrade path can
	// silently promote StatusCompleted -> StatusUserDriving (and mutate
	// result.InteractiveHold) inside its own lock when a concurrent
	// UpgradeToInteractive won the race -- the hook must report what was
	// actually committed, not what this goroutine merely requested.
	finalStatus, finalResult := targetStatus, result
	if sess, getErr := m.store.Get(id); getErr == nil {
		finalStatus, finalResult = sess.Status, sess.Result
	}
	m.fireTerminalHook(id, correlationID, finalStatus, finalResult, nil)
}
