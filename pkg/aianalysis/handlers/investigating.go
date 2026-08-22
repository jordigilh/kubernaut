/*
Copyright 2025 Jordi Gil.

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

// Package handlers implements phase handlers for the AIAnalysis controller.
package handlers

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
)

// P2.2 Refactoring: Constants moved to constants.go

// InvestigatingHandler handles the Investigating phase
// BR-AI-007: Get-or-create the AgentSession backing the investigation, then
// process its Status once KA reports a terminal outcome
// BR-AI-009: Retry transient errors with exponential backoff
// BR-AI-010: Fail immediately on permanent errors
// BR-AA-KA-065 (DD-AA-KA-001): AA<->KA channel is the AgentSession CRD --
// replaces the retired pkg/agentclient HTTP submit/poll/result flow.
// GetOrCreate is naturally idempotent, so submit and poll collapse into the
// single Handle() call site below.
// Refactoring P1.1: Uses ResponseProcessor for response handling
type InvestigatingHandler struct {
	log                      logr.Logger
	agentSessionGetOrCreator AgentSessionGetOrCreator
	metrics                  *metrics.Metrics     // DD-METRICS-001: Injected metrics
	auditClient              AuditClientInterface // DD-AUDIT-003: Injected audit client
	processor                *ResponseProcessor   // P1.1: Response processing logic
	errorClassifier          *ErrorClassifier     // P2.1: Error classification and retry logic
	maxInvestigationDuration time.Duration        // #1078: Wall-clock cap on investigation before PhaseFailed
	recorder                 record.EventRecorder // DD-EVENT-001: K8s event recorder for session lifecycle events
}

// InvestigatingHandlerOption is a functional option for InvestigatingHandler configuration.
type InvestigatingHandlerOption func(*InvestigatingHandler)

// WithRecorder injects a Kubernetes EventRecorder for session lifecycle events (DD-EVENT-001).
// When set, the handler emits SessionCreated and UserDriving events.
func WithRecorder(r record.EventRecorder) InvestigatingHandlerOption {
	return func(h *InvestigatingHandler) {
		h.recorder = r
	}
}

// WithMaxInvestigationDuration sets the wall-clock cap for an investigation session.
// If the session exceeds this duration, the handler transitions to PhaseFailed with
// Reason=TransientError. Default: DefaultMaxInvestigationDuration (25m).
func WithMaxInvestigationDuration(d time.Duration) InvestigatingHandlerOption {
	return func(h *InvestigatingHandler) {
		h.maxInvestigationDuration = d
	}
}

// WithLowConfidenceFloor sets the operator-configurable floor for
// auto-proceeding with a KA-selected workflow (BR-AI-088.4, Issue #1828).
// nil (the default when this option is omitted) means "use the built-in 70%
// floor". Delegates to the internal ResponseProcessor, constructed before
// options are applied in NewInvestigatingHandler.
func WithLowConfidenceFloor(floor *float64) InvestigatingHandlerOption {
	return func(h *InvestigatingHandler) {
		h.processor.WithLowConfidenceFloor(floor)
	}
}

// P1.3 Refactoring: AuditClientInterface moved to interfaces.go

// NewInvestigatingHandler creates a new InvestigatingHandler.
// Refactoring P1.1: Initializes ResponseProcessor
// Refactoring P2.1: Initializes ErrorClassifier with configurable backoff parameters
// DD-AA-KA-001: getOrCreator is mandatory -- it is the sole channel to KA.
func NewInvestigatingHandler(getOrCreator AgentSessionGetOrCreator, log logr.Logger, m *metrics.Metrics, auditClient AuditClientInterface, opts ...InvestigatingHandlerOption) *InvestigatingHandler {
	if m == nil {
		panic("metrics cannot be nil: metrics are mandatory for observability")
	}
	if getOrCreator == nil {
		panic("agent session get-or-creator cannot be nil: KA investigation requires a configured AgentSession channel (BR-AI-023, DD-AA-KA-001)")
	}
	handlerLog := log.WithName("investigating-handler")
	h := &InvestigatingHandler{
		agentSessionGetOrCreator: getOrCreator,
		metrics:                  m,
		auditClient:              auditClient,
		log:                      handlerLog,
		maxInvestigationDuration: DefaultMaxInvestigationDuration, // #1078: Wall-clock cap
		processor:                NewResponseProcessor(log, m, auditClient),
		errorClassifier:          NewErrorClassifier(handlerLog),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes the Investigating phase.
// BR-AI-007: Get-or-create the AgentSession and branch on its Status.Phase.
// GetOrCreate is naturally idempotent: Create on the very first reconcile
// (no AgentSession exists yet for this AIAnalysis), a plain Get on every
// reconcile thereafter -- so there is no separate submit-vs-poll branch.
func (h *InvestigatingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
	h.log.Info("Processing Investigating phase", "name", analysis.Name)

	// AA-KA-001: Idempotency is handled at controller level (phase_handlers.go)
	// via AtomicStatusUpdate callback with APIReader refetch. No handler-level check needed.

	firstObservation := analysis.Status.KASession == nil

	as, err := h.agentSessionGetOrCreator.GetOrCreate(ctx, analysis)
	if err != nil {
		return h.handleError(ctx, analysis, err)
	}

	// BR-AI-009/AA-CRIT-2: a successful GetOrCreate is this design's equivalent
	// of the retired "successful poll" -- reset the transient-failure counter
	// so it doesn't keep accumulating across a healthy AgentSession channel.
	analysis.Status.EnsureInvestigationMetadata().ConsecutiveFailures = 0

	h.syncKASessionStatus(analysis, as, firstObservation)

	if firstObservation {
		h.finalizeSessionSubmit(ctx, analysis, as)
	}

	switch as.Status.Phase {
	case agentsessionv1.AgentSessionPhaseCompleted:
		return h.handleSessionCompleted(ctx, analysis, as)
	case agentsessionv1.AgentSessionPhaseFailed:
		return h.handleSessionFailed(ctx, analysis, as)
	case agentsessionv1.AgentSessionPhaseCancelled:
		return h.handleSessionCancelled(ctx, analysis, as)
	default:
		// "", Pending, Investigating -- still running (possibly interactive/user-driving).
		return h.handleSessionRunning(ctx, analysis, as)
	}
}

// syncKASessionStatus mirrors the AgentSession's observable fields onto
// analysis.Status.KASession for backward-compatible observability (same CRD
// field shape, repurposed semantics -- DD-AA-KA-001).
//
// ID is set to the AgentSession's own deterministic object name rather than
// KA's internal session ID: it is known the instant GetOrCreate succeeds
// (unlike the historical value here, only known once KA actually dispatched),
// which preserves the reconciler-level idempotency check in
// phase_handlers.go (KASession != nil && KASession.ID != ""). KA's own
// internal session ID remains available on demand via as.Status.SessionID
// for audit correlation (see finalizeSessionSubmit/handleSessionCompleted).
func (h *InvestigatingHandler) syncKASessionStatus(analysis *aianalysisv1.AIAnalysis, as *agentsessionv1.AgentSession, firstObservation bool) {
	session := analysis.Status.KASession
	if session == nil {
		session = &aianalysisv1.KASession{}
		analysis.Status.KASession = session
	}
	session.ID = as.Name
	session.Interactive = as.Status.Interactive
	if session.CreatedAt == nil {
		created := as.CreationTimestamp
		session.CreatedAt = &created
	}
	if firstObservation {
		session.PollCount = 0
	} else {
		session.PollCount++
	}
	now := metav1.Now()
	session.LastPolled = &now

	// #774: Propagate identity from KA's live status to CR status while a
	// user is driving (DD-INTERACTIVE-002).
	if as.Status.Interactive && (as.Status.ActingUser != "" || len(as.Status.ActingUserGroups) > 0 || as.Status.SessionID != "") {
		if analysis.Status.InteractiveSession == nil {
			analysis.Status.InteractiveSession = &aianalysisv1.InteractiveSessionInfo{}
		}
		analysis.Status.InteractiveSession.ActingUser = as.Status.ActingUser
		analysis.Status.InteractiveSession.ActingUserGroups = as.Status.ActingUserGroups
		if as.Status.SessionID != "" {
			analysis.Status.InteractiveSession.SessionID = as.Status.SessionID
		}
		if analysis.Status.InteractiveSession.StartedAt == nil {
			analysis.Status.InteractiveSession.StartedAt = &now
		}
	}
}

// finalizeSessionSubmit sets the SessionCreated condition, records the
// mandatory submit audit event, emits the SessionCreated K8s event, and logs
// the outcome. Called once, on the reconcile where GetOrCreate first creates
// the AgentSession.
func (h *InvestigatingHandler) finalizeSessionSubmit(ctx context.Context, analysis *aianalysisv1.AIAnalysis, as *agentsessionv1.AgentSession) {
	aianalysis.SetInvestigationSessionReady(analysis, true, aianalysis.ReasonSessionCreated,
		fmt.Sprintf("AgentSession %s created", as.Name))

	// DD-AUDIT-003: Record submit audit event
	h.auditClient.RecordAIAgentSubmit(ctx, analysis, as.Name)

	// DD-EVENT-001: Emit SessionCreated K8s event for observability
	if h.recorder != nil {
		h.recorder.Event(analysis, corev1.EventTypeNormal, events.EventReasonSessionCreated,
			fmt.Sprintf("AgentSession %s created", as.Name))
	}

	h.log.Info("AgentSession created", "agentSession", as.Name)
}

// handleSessionRunning covers the AgentSession's non-terminal phases (unset,
// Pending, Investigating). It enforces AA's own wall-clock investigation cap
// independently of KA (checkInvestigationTimeout), emits UserDriving
// observability when a human is currently driving, and requeues exactly at
// the investigation's own deadline (backstopRequeueAfter) -- the
// AgentSession watch wakes the reconciler immediately on any actual KA
// status write, so this requeue is a pure backstop: it guarantees the
// timeout check still runs even if KA never writes another update (e.g. it
// hangs), without polling on any interval unrelated to that deadline (#2204).
func (h *InvestigatingHandler) handleSessionRunning(ctx context.Context, analysis *aianalysisv1.AIAnalysis, as *agentsessionv1.AgentSession) (ctrl.Result, error) {
	session := analysis.Status.KASession

	if h.checkInvestigationTimeout(ctx, analysis, session, as.Status.Interactive) {
		return ctrl.Result{}, nil
	}

	requeueAfter := h.backstopRequeueAfter(analysis, session)

	if as.Status.Interactive {
		h.log.Info("Session under user control, continuing to poll",
			"agentSession", as.Name,
			"actingUser", as.Status.ActingUser,
			"nextCheckIn", requeueAfter,
			"pollCount", session.PollCount,
		)
		if h.recorder != nil {
			h.recorder.Eventf(analysis, corev1.EventTypeNormal, events.EventReasonUserDriving,
				"Interactive session %s: user is driving investigation", as.Name)
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	h.log.Info("Investigation still in progress, requeuing",
		"agentSession", as.Name,
		"phase", as.Status.Phase,
		"nextCheckIn", requeueAfter,
		"pollCount", session.PollCount,
	)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// minBackstopRequeueAfter floors backstopRequeueAfter's result. Defensive
// only: checkInvestigationTimeout already short-circuits handleSessionRunning
// before this is ever computed once the deadline has actually passed, so in
// practice the raw remaining-time value is always positive here -- this
// floor exists purely to avoid a zero/negative RequeueAfter (which
// controller-runtime treats as "requeue immediately", a tight busy-loop)
// from ever reaching the workqueue if clock skew or test timing ever closes
// that gap.
const minBackstopRequeueAfter = 1 * time.Second

// investigationDeadline returns the same authoritative deadline
// checkInvestigationTimeout evaluates against: RO's absolute
// Spec.TimesOutAt (DD-TIMEOUT-002 / Issue #2176) when set, else the
// back-compat session.CreatedAt+maxInvestigationDuration fallback.
func (h *InvestigatingHandler) investigationDeadline(analysis *aianalysisv1.AIAnalysis, session *aianalysisv1.KASession) time.Time {
	if analysis.Spec.TimesOutAt != nil {
		return analysis.Spec.TimesOutAt.Time
	}
	var createdAt time.Time
	if session != nil && session.CreatedAt != nil {
		createdAt = session.CreatedAt.Time
	}
	return createdAt.Add(h.maxInvestigationDuration)
}

// backstopRequeueAfter computes how long until the investigation's own
// deadline (investigationDeadline) and returns that as the safety-net
// requeue duration -- a single reconcile scheduled exactly when a timeout
// check is next actually needed, instead of a periodic poll interval
// unrelated to that deadline (#2204: the prior flat sessionPollInterval
// design generated reconcile/API-server volume with no relationship to
// when a check was actually due).
func (h *InvestigatingHandler) backstopRequeueAfter(analysis *aianalysisv1.AIAnalysis, session *aianalysisv1.KASession) time.Duration {
	remaining := time.Until(h.investigationDeadline(analysis, session))
	if remaining < minBackstopRequeueAfter {
		return minBackstopRequeueAfter
	}
	return remaining
}

// checkInvestigationTimeout fails analysis in place and returns true if
// session has exceeded h.maxInvestigationDuration, in which case the caller
// must return (ctrl.Result{}, nil) without further handling; returns false
// otherwise, in which case the caller should proceed normally. Issue #1530
// (dupl): interactive selects between the "Interactive session/investigation"
// -worded messages (AA-CRIT-1: user_driving must NOT bypass
// MaxInvestigationDuration) and the plain "Investigation"-worded messages.
func (h *InvestigatingHandler) checkInvestigationTimeout(ctx context.Context, analysis *aianalysisv1.AIAnalysis, session *aianalysisv1.KASession, interactive bool) bool {
	if session.CreatedAt == nil {
		return false
	}
	elapsed := time.Since(session.CreatedAt.Time)

	// DD-TIMEOUT-002 / Issue #2176: prefer RO's authoritative absolute
	// deadline (Spec.TimesOutAt, propagated from
	// RemediationRequest.Status.TimeoutConfig.Analyzing) when set. Fall back
	// to the hardcoded session.CreatedAt+maxInvestigationDuration default
	// when RO has no authoritative Analyzing timeout (back-compat /
	// defensive -- e.g. AIAnalysis CRDs created before this field existed).
	deadline := h.investigationDeadline(analysis, session)
	timedOut := metav1.Now().After(deadline)
	var limitDesc string
	if analysis.Spec.TimesOutAt != nil {
		limitDesc = "deadline " + analysis.Spec.TimesOutAt.Format(time.RFC3339)
	} else {
		limitDesc = h.maxInvestigationDuration.String()
	}
	if !timedOut {
		return false
	}

	exceededLogMsg := "Investigation exceeded max duration, failing"
	timeoutMsgFmt := "Investigation timed out after %s (limit: %s)"
	if interactive {
		exceededLogMsg = "Interactive session exceeded max duration, failing"
		timeoutMsgFmt = "Interactive investigation timed out after %s (limit: %s)"
	}

	h.log.Info(exceededLogMsg,
		"sessionID", session.ID,
		"elapsed", elapsed,
		"maxDuration", h.maxInvestigationDuration,
		"timesOutAt", analysis.Spec.TimesOutAt,
	)
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = aianalysisv1.ReasonTransientError
	analysis.Status.SubReason = aianalysisv1.SubReasonTransientError
	analysis.Status.Message = fmt.Sprintf(timeoutMsgFmt, elapsed.Truncate(time.Second), limitDesc)

	// BR-AUDIT-005 Gap #7 / Issue #2176: this terminal-failure path previously
	// did not emit a failure audit event, unlike every other checkInvestigationTimeout
	// sibling in this file (e.g. failMaxRetriesExceeded, failPermanentError).
	timeoutErr := fmt.Errorf("investigation timed out after %s (limit: %s)", elapsed.Truncate(time.Second), limitDesc)
	if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, timeoutErr); auditErr != nil {
		h.log.V(1).Info("Failed to record analysis timeout audit", "error", auditErr)
	}

	return true
}

// handleSessionCompleted handles the AgentSession's Completed phase.
// BR-AA-KA-065.3: Status.Result carries the curated outcome KA already
// wrote -- no separate result-fetch call is needed (unlike the retired
// GetSessionResult HTTP call).
func (h *InvestigatingHandler) handleSessionCompleted(ctx context.Context, analysis *aianalysisv1.AIAnalysis, as *agentsessionv1.AgentSession) (ctrl.Result, error) {
	session := analysis.Status.KASession

	h.log.Info("KA session completed, processing result",
		"agentSession", as.Name,
		"sessionID", as.Status.SessionID,
		"pollCount", session.PollCount,
	)

	// BR-INTERACTIVE-001: If a user was driving the session, record when the
	// interactive phase ended. This lets the RO timeout logic distinguish
	// "interactive session just completed" from "never had an interactive user".
	if iss := analysis.Status.InteractiveSession; iss != nil && iss.StartedAt != nil && iss.CompletedAt == nil {
		now := metav1.Now()
		iss.CompletedAt = &now
	}

	var investigationTime int64
	if session.CreatedAt != nil {
		investigationTime = time.Since(session.CreatedAt.Time).Milliseconds()
	}
	analysis.Status.EnsureInvestigationMetadata().InvestigationTime = investigationTime
	analysis.Status.ObservedGeneration = analysis.Generation

	// #2204 (2026-08-20): DD-AUDIT-003's "record result retrieval audit
	// event" call used to live here, inside the handler invoked from
	// InvestigatingHandler.Handle -- which itself runs inside
	// StatusManager.AtomicStatusUpdate's k8sretry.RetryOnConflict closure
	// (phase_handlers.go's runInvestigatingHandler). A resourceVersion
	// Conflict on that closure's own Status().Update() call re-runs the
	// WHOLE closure, including this non-idempotent audit write, double-
	// recording "aianalysis.aiagent.call" for one logical completion. Per
	// DD-WE-009 ("audit outside the retryable closure") -- the exact
	// convention this codebase already applies to RecordPhaseTransition
	// (AA-BUG-001, see finalizeInvestigatingTransition) -- the audit call
	// has moved to phase_handlers.go's finalizeInvestigatingTransition,
	// which runs exactly once, after AtomicStatusUpdate has durably
	// committed. investigationTimeMs is threaded through
	// investigatingUpdateOutcome via a before/after comparison of
	// InvestigationMetadata.InvestigationTime (still safely set above,
	// inside the closure -- a plain status field write is idempotent across
	// retries, unlike an external audit call).

	res := as.Status.Result
	if res == nil {
		return h.handleError(ctx, analysis, fmt.Errorf("AgentSession %s completed with no Result", as.Name))
	}

	result, err := h.processor.ProcessAgentSessionResult(ctx, analysis, res)
	if err == nil {
		h.setRetryCount(analysis, 0)
	}

	// #2088 (main port of #2086 Fix 4): KA's own session inactivity timeout
	// (e.g. a 10-minute interactive-session idle limit) completes with no
	// InvestigationResult ever produced. KA synthesizes a placeholder for
	// this with has_workflow=false and human_review_reason="" --
	// ProcessAgentSessionResult cannot distinguish this from a genuine
	// "investigated and found no matching workflow" conclusion, so it
	// always classifies both identically as SubReason=NoMatchingWorkflows.
	// That is misleading: the investigation never actually reached a
	// conclusion. Correct the classification here. FedRAMP AU-3 (truthful
	// audit content) / SI-11 (accurate error handling).
	if analysis.Status.SubReason == "NoMatchingWorkflows" && isSessionTimedOutWithoutResult(res) {
		analysis.Status.SubReason = "InvestigationInconclusive"
		analysis.Status.EnsureReview().HumanReviewReason = "investigation_inconclusive"
		analysis.Status.Message = "KA session completed without producing a result " +
			"(likely an inactivity timeout); the investigation did not reach a conclusion"
	}

	return result, err
}

// isSessionTimedOutWithoutResult reports whether res is KA's nil-result
// synthesis placeholder rather than a genuine investigation outcome. #2088
// (main port of #2086): this is the only signal available on the wire that
// distinguishes "the session completed without KA ever producing a result"
// (e.g. an inactivity timeout) from "KA investigated and concluded no
// workflow matches".
func isSessionTimedOutWithoutResult(res *agentsessionv1.AgentSessionResult) bool {
	return res.Analysis == "Investigation completed without result" && res.Confidence == 0
}

// handleSessionFailed handles the AgentSession's Failed phase.
// BR-AA-KA-065: Surface KA-side failure to operators via CRD status.
// AA-MED-1: Ensure Reason and SubReason are set for structured failure reporting.
func (h *InvestigatingHandler) handleSessionFailed(ctx context.Context, analysis *aianalysisv1.AIAnalysis, as *agentsessionv1.AgentSession) (ctrl.Result, error) {
	session := analysis.Status.KASession

	if as.Status.Reason == agentsessionv1.AgentSessionReasonCapacityExceeded {
		if result, retried := h.retryCapacityExceeded(ctx, analysis, as, session); retried {
			return result, nil
		}
		// Retry budget exhausted: fall through to the permanent-fail path
		// below, unchanged -- a capacity-exceeded failure that has
		// exhausted its retries is reported identically to any other.
	}

	h.log.Info("KA session failed",
		"agentSession", as.Name,
		"pollCount", session.PollCount,
		"error", as.Status.Error,
	)

	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.Reason = aianalysisv1.ReasonAPIError
	analysis.Status.SubReason = "InvestigationFailed"
	analysis.Status.CompletedAt = &now
	analysis.Status.ObservedGeneration = analysis.Generation
	analysis.Status.Message = as.Status.Error
	if analysis.Status.Message == "" {
		analysis.Status.Message = "Investigation failed on KA side"
	}

	// Record failure audit
	failureErr := fmt.Errorf("KA session failed: %s", as.Status.Error)
	if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, failureErr); auditErr != nil {
		h.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	aianalysis.SetInvestigationComplete(analysis, false, analysis.Status.Message)
	return ctrl.Result{}, nil
}

// retryCapacityExceeded implements the BR-AI-009 retry path for a
// KA-dispatch-time capacity rejection (DD-AA-KA-001 amendment,
// AgentSessionReasonCapacityExceeded): a transient, self-resolving
// backpressure condition (session.ErrMaxInvestigationsReached) that must
// not permanently fail the AIAnalysis while retry budget remains.
//
// The retry budget is bounded by investigationDeadline (RO's authoritative
// Spec.TimesOutAt, else session.CreatedAt+maxInvestigationDuration) rather
// than a fixed attempt count. #2189 E2E-AA-065 finding: a burst of
// concurrent investigations against a low KA capacity limit can need more
// than a handful of retries to win a capacity slot -- how long a capacity
// rejection needs to resolve scales with queue depth, not a fixed number of
// quick retries, so gating exhaustion on attempt count (as the generic
// ErrorClassifier.ShouldRetry/MaxRetries does for ordinary transient
// errors) permanently failed investigations that would have completed well
// within their own deadline.
//
// Still reuses ErrorClassifier's already-tested HTTP-429/rate-limit branch
// (via a synthetic status error) for backoff *pacing* only, and
// KASession.Generation (vestigial after the retired HTTP-session
// regeneration mechanism) as the backoff-attempt counter, avoiding a new
// field. Returns (result, true) when a retry was taken and the caller must
// return result immediately; (zero, false) when the deadline has passed,
// in which case the caller falls through to the unchanged permanent-fail
// path.
func (h *InvestigatingHandler) retryCapacityExceeded(ctx context.Context, analysis *aianalysisv1.AIAnalysis, as *agentsessionv1.AgentSession, session *aianalysisv1.KASession) (ctrl.Result, bool) {
	syntheticErr := apierrors.NewTooManyRequests(as.Status.Error, 0)
	classification := h.errorClassifier.ClassifyError(syntheticErr)
	attempt := int(session.Generation)

	deadline := h.investigationDeadline(analysis, session)
	remaining := time.Until(deadline)

	if !classification.IsRetryable || remaining <= 0 {
		h.log.Info("capacity-exceeded retry budget exhausted (investigation deadline reached), failing permanently",
			"agentSession", as.Name, "attempts", attempt, "deadline", deadline)
		return ctrl.Result{}, false
	}

	if err := h.agentSessionGetOrCreator.DeleteForRetry(ctx, as); err != nil {
		h.log.Error(err, "failed to delete AgentSession for capacity-exceeded retry, failing permanently",
			"agentSession", as.Name)
		return ctrl.Result{}, false
	}

	backoff := h.errorClassifier.GetRetryDelay(attempt)
	if backoff > remaining {
		// Don't let backoff pacing overshoot the deadline -- retry sooner
		// so the next reconcile still lands before TimesOutAt instead of
		// silently overshooting it.
		backoff = remaining
	}
	analysis.Status.KASession = &aianalysisv1.KASession{Generation: session.Generation + 1}

	h.log.Info("KA dispatch capacity exceeded, retrying with backoff",
		"agentSession", as.Name, "attempt", attempt+1, "backoff", backoff, "deadline", deadline)

	return ctrl.Result{RequeueAfter: backoff}, true
}

// handleSessionCancelled handles the AgentSession's Cancelled phase: the
// interactive driver disconnected without a takeover. DD-AA-KA-001 Amendment
// (Gap 1): dispatch happens exactly once per AgentSession (Lease-based), so
// unlike the retired HTTP design (where a new session could be resubmitted
// under the same AIAnalysis), Cancelled is terminal for this investigation --
// there is no takeover-resubmit branch to check for here anymore.
func (h *InvestigatingHandler) handleSessionCancelled(ctx context.Context, analysis *aianalysisv1.AIAnalysis, as *agentsessionv1.AgentSession) (ctrl.Result, error) {
	h.log.Info("KA session cancelled", "agentSession", as.Name)

	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.Reason = aianalysisv1.ReasonInteractiveCancelled
	analysis.Status.CompletedAt = &now
	analysis.Status.ObservedGeneration = analysis.Generation
	analysis.Status.Message = "Investigation cancelled (interactive session ended)"

	cancelErr := fmt.Errorf("KA session cancelled: interactive session ended")
	if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, cancelErr); auditErr != nil {
		h.log.V(1).Info("Failed to record cancellation audit", "error", auditErr)
	}

	aianalysis.SetInvestigationComplete(analysis, false, "Investigation cancelled")
	return ctrl.Result{}, nil
}

// handleError processes errors from GetOrCreate (e.g. a transient K8s API
// failure) or a nil Result on a Completed AgentSession.
// BR-AI-009: Retry transient errors with exponential backoff
// BR-AI-010: Fail immediately on permanent errors
// Refactoring P2.1: Uses ErrorClassifier for error classification and retry logic
func (h *InvestigatingHandler) handleError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error) (ctrl.Result, error) {
	// P2.1: Classify error type using error classifier
	classification := h.errorClassifier.ClassifyError(err)

	// Increment failure count before retry check
	analysis.Status.EnsureInvestigationMetadata().ConsecutiveFailures++

	// P2.1: Check if error should be retried based on classification and attempt count
	if h.errorClassifier.ShouldRetry(classification, int(analysis.Status.InvestigationMetadata.ConsecutiveFailures)) {
		return h.retryTransientError(analysis, err, classification)
	}

	// If we get here, either max retries exceeded or error is not retryable
	if classification.IsRetryable {
		return h.failMaxRetriesExceeded(ctx, analysis, err, classification)
	}

	// BR-AI-010: Fail immediately on permanent errors
	return h.failPermanentError(ctx, analysis, err, classification)
}

// retryTransientError requeues with exponential backoff for a transient
// error that hasn't yet exhausted its retry budget (BR-AI-009).
func (h *InvestigatingHandler) retryTransientError(analysis *aianalysisv1.AIAnalysis, err error, classification ErrorClassification) (ctrl.Result, error) {
	// P2.1: Use error classifier to calculate backoff duration
	backoffDuration := h.errorClassifier.GetRetryDelay(int(analysis.Status.InvestigationMetadata.ConsecutiveFailures))

	h.log.Info("Transient error - retrying with backoff",
		"error", err,
		"errorType", classification.ErrorType,
		"attempts", analysis.Status.InvestigationMetadata.ConsecutiveFailures,
		"backoff", backoffDuration,
	)

	// Update status to indicate retry
	analysis.Status.Message = fmt.Sprintf("Transient error (attempt %d/%d): %v",
		analysis.Status.InvestigationMetadata.ConsecutiveFailures, MaxRetries, err)
	analysis.Status.Reason = aianalysisv1.ReasonTransientError
	analysis.Status.SubReason = mapErrorTypeToSubReason(classification.ErrorType) // Map to valid CRD enum

	// Record metric for transient errors
	h.metrics.RecordFailure(aianalysisv1.SubReasonTransientError, "Retrying")

	// Requeue with exponential backoff (error classifier handles jitter internally)
	return ctrl.Result{RequeueAfter: backoffDuration}, nil
}

// failMaxRetriesExceeded transitions analysis to permanent Failed after a
// retryable error has exhausted its retry budget.
func (h *InvestigatingHandler) failMaxRetriesExceeded(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error, classification ErrorClassification) (ctrl.Result, error) {
	h.log.Info("Transient error exceeded max retries - failing permanently",
		"error", err,
		"errorType", classification.ErrorType,
		"attempts", analysis.Status.InvestigationMetadata.ConsecutiveFailures,
		"maxRetries", h.errorClassifier.GetMaxRetries(),
	)

	// Transition to permanent failure after max retries
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now
	analysis.Status.Message = fmt.Sprintf("Transient error exceeded max retries (%d attempts): %v",
		analysis.Status.InvestigationMetadata.ConsecutiveFailures, err)
	analysis.Status.Reason = aianalysisv1.ReasonAPIError
	analysis.Status.SubReason = aianalysisv1.SubReasonMaxRetriesExceeded

	// Record metric for max retries exceeded
	h.metrics.RecordFailure(string(aianalysisv1.ReasonAPIError), aianalysisv1.SubReasonMaxRetriesExceeded)

	// BR-AUDIT-005 Gap #7: Record failure audit with standardized error details
	if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, err); auditErr != nil {
		h.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	aianalysis.SetInvestigationComplete(analysis, false, fmt.Sprintf("Transient error exceeded max retries (%d attempts): %v", analysis.Status.InvestigationMetadata.ConsecutiveFailures, err))
	return ctrl.Result{}, nil
}

// failPermanentError transitions analysis to Failed immediately for a
// non-retryable error (BR-AI-010).
func (h *InvestigatingHandler) failPermanentError(ctx context.Context, analysis *aianalysisv1.AIAnalysis, err error, classification ErrorClassification) (ctrl.Result, error) {
	h.log.Info("Permanent error - failing immediately",
		"error", err,
		"errorType", classification.ErrorType,
	)
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.ObservedGeneration = analysis.Generation // DD-CONTROLLER-001
	analysis.Status.CompletedAt = &now                       // Per crd-schema.md: set on terminal state
	analysis.Status.Message = fmt.Sprintf("Permanent error: %v", err)
	analysis.Status.Reason = aianalysisv1.ReasonAPIError
	analysis.Status.SubReason = mapErrorTypeToSubReason(classification.ErrorType) // Map to valid CRD enum

	// Record metric for permanent errors
	h.metrics.RecordFailure("APIError", string(classification.ErrorType))

	// BR-AUDIT-005 Gap #7: Record failure audit with standardized error details
	if auditErr := h.auditClient.RecordAnalysisFailed(ctx, analysis, err); auditErr != nil {
		h.log.V(1).Info("Failed to record analysis failure audit", "error", auditErr)
	}

	aianalysis.SetInvestigationComplete(analysis, false, fmt.Sprintf("Permanent error: %v", err))
	return ctrl.Result{}, nil
}

// setRetryCount writes retry count to annotations
func (h *InvestigatingHandler) setRetryCount(analysis *aianalysisv1.AIAnalysis, count int) {
	if analysis.Annotations == nil {
		analysis.Annotations = make(map[string]string)
	}
	analysis.Annotations[RetryCountAnnotation] = strconv.Itoa(count)
}

// mapErrorTypeToSubReason maps error classifier ErrorType to valid AIAnalysis CRD SubReason enum values
// per config/crd/bases/kubernaut.ai_aianalyses.yaml line 134-144
func mapErrorTypeToSubReason(errorType ErrorType) string {
	switch errorType {
	case ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeRateLimit, ErrorTypeTransient:
		// All transient/retryable errors map to "TransientError"
		return aianalysisv1.SubReasonTransientError
	case ErrorTypePermanent, ErrorTypeAuthentication, ErrorTypeAuthorization, ErrorTypeConfiguration:
		// All non-retryable errors map to "PermanentError"
		return "PermanentError"
	default:
		// Fallback for unknown error types
		return aianalysisv1.SubReasonTransientError
	}
}
