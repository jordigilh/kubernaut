# AI Analysis Service - Controller Implementation

**Version**: v3.0
**Last Updated**: 2026-08-02
**Status**: ✅ Corrected for [#1806](https://github.com/jordigilh/kubernaut/issues/1806)

---

## Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| v3.0 | 2026-08-02 | **#1806 CORRECTION**: Full rewrite, superseding the v2.0 STALE banner. Fixed the package structure to match the real code (`internal/controller/aianalysis/` for the reconciler, `pkg/aianalysis/handlers/` for phase logic, `pkg/agentclient` for the KA client — not `pkg/ai/holmesgpt` / `internal/controller/aianalysis/holmesgpt`, which do not exist); replaced the fictional `HolmesGPTClient holmesgpt.Client` reconciler field and single synchronous `Investigate()` call with the real async submit/poll/result session flow (`AgentClientInterface.SubmitInvestigation`/`PollSession`/`GetSessionResult`, BR-AA-HAPI-064); replaced the 60s/5s hardcoded phase timeouts with the real `DefaultSessionPollInterval` (15s) and `DefaultMaxInvestigationDuration` (25m) wall-clock cap; corrected `AIApprovalRequest` references to `RemediationApprovalRequest` | #1806, BR-AA-HAPI-064 |
| v2.0 | 2025-11-30 | **REGENERATED**: Fixed SignalProcessing naming; Removed legacy phases (recommending→analyzing); Removed HolmesGPTConfig/InvestigationScope; Added DetectedLabels/CustomLabels/OwnerChain; V1.0 4-phase flow | DD-WORKFLOW-001 v1.8, DD-RECOVERY-002 |
| v1.1 | 2025-10-16 | Added self-documenting JSON format | DD-HOLMESGPT-009 |
| v1.0 | 2025-10-15 | Initial specification | - |

---

## Package Structure

```
internal/controller/aianalysis/
├── aianalysis_controller.go     # AIAnalysisReconciler (Kubebuilder controller), phase dispatch, predicates
├── phase_handlers.go            # reconcilePending / reconcileInvestigating / reconcileAnalyzing
├── deletion_handler.go          # Finalizer cleanup on deletion
├── metrics_recorder.go          # recordPhaseMetrics (confidence + failure metrics)
└── suite_test.go

pkg/aianalysis/
├── handler.go                   # Phase constants (PhasePending, PhaseInvestigating, ...)
├── conditions.go                # Status condition helpers, Outcome* constants
├── metrics/metrics.go           # Prometheus metrics (DD-METRICS-001)
├── audit/audit.go               # AuditClient (DD-AUDIT-003)
├── status/                      # status.Manager — atomic status updates (DD-PERF-001)
├── rego/evaluator.go            # Rego policy evaluation (BR-AI-011)
└── handlers/
    ├── interfaces.go            # AgentClientInterface, AuditClientInterface, RegoEvaluatorInterface
    ├── constants.go             # Retry/backoff + session constants (BR-AA-HAPI-064)
    ├── investigating.go         # InvestigatingHandler: async submit/poll/result session flow
    ├── analyzing.go             # AnalyzingHandler: Rego policy evaluation
    ├── request_builder.go       # Builds agentclient.IncidentRequest from AIAnalysis spec
    ├── response_processor.go    # Processes agentclient.IncidentResponse into AIAnalysis status
    ├── error_classifier.go      # Classifies KA errors as transient/permanent for retry
    └── is_checker.go            # InvestigationSessionChecker (BR-INTERACTIVE-010)

pkg/agentclient/                 # ogen-generated OpenAPI client for Kubernaut Agent (KA)
├── client.go                    # KubernautAgentClient: Investigate/SubmitInvestigation/PollSession/
│                                 # GetSessionResult/CancelSession (wraps the generated oas_*.go files)
└── oas_*.go                     # Generated request/response types, schemas, routing

cmd/aianalysis/
└── main.go                      # Binary entry point; wires InvestigatingHandler/AnalyzingHandler

test/unit/aianalysis/            # Unit tests (70%+)
test/integration/aianalysis/     # Integration tests (~20%)
test/e2e/aianalysis/             # E2E tests (~10%)
```

---

## Core Types

### Incident Request/Response (Kubernaut Agent contract)

The KA contract types are **generated** (ogen, from `internal/kubernautagent/api/openapi.json`) and live in `pkg/agentclient`, not hand-written in a `pkg/ai/holmesgpt` package.

```go
// pkg/agentclient (generated + client.go)
package agentclient

// IncidentRequest is submitted to Kubernaut Agent (KA) via SubmitInvestigation.
// BR-AI-080: Includes remediationId for audit correlation only (not used for RCA/matching).
type IncidentRequest struct {
    IncidentID        string   `json:"incident_id"`
    RemediationID     string   `json:"remediation_id"`
    SignalName        string   `json:"signal_name"`
    Severity          Severity `json:"severity"`
    SignalSource      string   `json:"signal_source"`
    ResourceNamespace string   `json:"resource_namespace"`
    ResourceKind      string   `json:"resource_kind"`
    ResourceName      string   `json:"resource_name"`
    ErrorMessage      string   `json:"error_message"`
    // Interactive: OptBool — set true when an InvestigationSession CRD exists
    // for the RemediationRequest (BR-INTERACTIVE-010)
    Interactive OptBool `json:"interactive,omitempty"`
    // ... additional fields (business classification, K8s context, etc.)
}

// IncidentResponse is the terminal investigation result, fetched via GetSessionResult
// once the session's status reaches "completed".
type IncidentResponse struct {
    IncidentID        string                             `json:"incident_id"`
    Analysis          string                             `json:"analysis"`
    RootCauseAnalysis IncidentResponseRootCauseAnalysis   `json:"root_cause_analysis"`
    SelectedWorkflow  OptNilIncidentResponseSelectedWorkflow `json:"selected_workflow"`
    Confidence        float64                            `json:"confidence"`
    NeedsHumanReview  OptBool                            `json:"needs_human_review"`
    // ... warnings, human_review_reason, timestamp, etc.
}

// SessionStatusResult is returned by PollSession while an investigation is in flight.
type SessionStatusResult struct {
    SessionID string `json:"session_id,omitempty"`
    // Status: "pending" | "investigating" | "user_driving" | "completed" | "failed" | "cancelled"
    Status   string `json:"status"`
    Error    string `json:"error,omitempty"`
    Progress string `json:"progress,omitempty"`
    // ActingUser/ActingUserGroups populated when Status == "user_driving" (DD-INTERACTIVE-002)
    ActingUser       string   `json:"acting_user,omitempty"`
    ActingUserGroups []string `json:"acting_user_groups,omitempty"`
}
```

### Agent Client Interface

```go
// pkg/aianalysis/handlers/interfaces.go
package handlers

// AgentClientInterface defines the contract for calling Kubernaut Agent (KA).
// BR-AI-007: KA integration for investigation
type AgentClientInterface interface {
    // Legacy synchronous method (being deprecated)
    Investigate(ctx context.Context, req *agentclient.IncidentRequest) (*agentclient.IncidentResponse, error)

    // Async session methods (BR-AA-HAPI-064) — the real, current flow
    SubmitInvestigation(ctx context.Context, req *agentclient.IncidentRequest) (string, error)
    PollSession(ctx context.Context, sessionID string) (*agentclient.SessionStatusResult, error)
    GetSessionResult(ctx context.Context, sessionID string) (*agentclient.IncidentResponse, error)

    // BR-INTERACTIVE-010: Cancel a running KA session
    CancelSession(ctx context.Context, sessionID string) error
}
```

---

## Reconciler Implementation

### AIAnalysisReconciler

```go
// internal/controller/aianalysis/aianalysis_controller.go
package aianalysis

import (
    "sync/atomic"

    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/audit"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/metrics"
    "github.com/jordigilh/kubernaut/pkg/aianalysis/status"
)

const FinalizerName = "kubernaut.ai/finalizer"

// AIAnalysisReconciler reconciles an AIAnalysis object
// BR-AI-001: CRD Lifecycle Management
// DD-AUDIT-003: P0 priority for audit traces
type AIAnalysisReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder
    Log      logr.Logger

    // DD-METRICS-001: Dependency-injected metrics (not global vars)
    Metrics *metrics.Metrics

    // DD-PERF-001: Atomic status updates (reduces K8s API calls, avoids races)
    StatusManager *status.Manager

    // Phase handlers, wired via dependency injection in cmd/aianalysis/main.go.
    // InvestigatingHandler uses atomic.Pointer so integration tests can swap
    // a mock handler while the controller manager is running.
    InvestigatingHandler atomic.Pointer[handlers.InvestigatingHandler]
    AnalyzingHandler     *handlers.AnalyzingHandler

    // AuditClient for DD-AUDIT-003 audit trail recording
    AuditClient *audit.AuditClient

    // ISPhaseUpdater cascades terminal-state transitions to the InvestigationSession CRD
    ISPhaseUpdater handlers.ISPhaseUpdater
}

// Reconcile implements the reconciliation loop for AIAnalysis
// BR-AI-001: Phase state machine: Pending → Investigating → Analyzing → Completed/Failed
func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    analysis := &aianalysisv1.AIAnalysis{}
    if err := r.Get(ctx, req.NamespacedName, analysis); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    if !analysis.DeletionTimestamp.IsZero() {
        return r.handleDeletion(ctx, analysis)
    }

    if requeueResult, added, err := r.ensureFinalizer(ctx, analysis, r.Log); added {
        return requeueResult, err
    }

    currentPhase := analysis.Status.Phase
    if currentPhase == "" {
        return r.initializePendingPhase(ctx, analysis, r.Log)
    }

    result, err := r.dispatchPhase(ctx, analysis, currentPhase, r.Log)
    r.recordPhaseMetrics(ctx, currentPhase, analysis, err)
    return result, err
}

// dispatchPhase routes to the phase-specific reconcile function.
func (r *AIAnalysisReconciler) dispatchPhase(ctx context.Context, analysis *aianalysisv1.AIAnalysis, currentPhase string, log logr.Logger) (ctrl.Result, error) {
    switch currentPhase {
    case PhasePending:
        return r.reconcilePending(ctx, analysis)
    case PhaseInvestigating:
        return r.reconcileInvestigating(ctx, analysis)
    case PhaseAnalyzing:
        return r.reconcileAnalyzing(ctx, analysis)
    case PhaseCompleted, PhaseFailed:
        return ctrl.Result{}, nil // Terminal states
    default:
        analysis.Status.Phase = PhaseFailed
        analysis.Status.Reason = "UnknownPhase"
        return ctrl.Result{}, nil
    }
}
```

---

## Phase Handlers

### Pending Phase

```go
// internal/controller/aianalysis/phase_handlers.go
func (r *AIAnalysisReconciler) reconcilePending(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    now := metav1.Now()
    analysis.Status.StartedAt = &now
    analysis.Status.Phase = PhaseInvestigating
    analysis.Status.Message = "AIAnalysis created, starting investigation"

    if err := r.Status().Update(ctx, analysis); err != nil {
        return ctrl.Result{}, err
    }
    r.AuditClient.RecordPhaseTransition(ctx, analysis, "Pending", PhaseInvestigating)
    return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
}
```

### Investigating Phase — Async Submit/Poll/Result (BR-AA-HAPI-064)

Unlike a single synchronous HTTP call, the Investigating phase is a **non-blocking session state machine** driven by `InvestigatingHandler.Handle` (`pkg/aianalysis/handlers/investigating.go`). Each reconcile either submits, polls, or fetches the result — it never blocks the reconcile loop waiting on Kubernaut Agent (KA):

```go
// pkg/aianalysis/handlers/investigating.go (condensed)
func (h *InvestigatingHandler) handleSessionBased(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    session := analysis.Status.KASession

    // SUBMIT: no session yet, or session ID cleared after a 404 (session lost)
    if session == nil || session.ID == "" {
        return h.handleSessionSubmit(ctx, analysis)
    }

    // POLL: session exists with an active ID
    return h.handleSessionPoll(ctx, analysis)
}

func (h *InvestigatingHandler) handleSessionSubmit(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    req := h.builder.BuildIncidentRequest(analysis)
    sessionID, err := h.kaClient.SubmitInvestigation(ctx, req) // POST /api/v1/incident/analyze -> 202 + session_id
    if err != nil {
        return h.handleError(ctx, analysis, err)
    }
    updateKASessionStatus(analysis, sessionID, /* interactive */ false)
    // Requeue for the first poll at the configured interval (default 15s)
    return ctrl.Result{RequeueAfter: h.sessionPollInterval}, nil
}

func (h *InvestigatingHandler) handleSessionPoll(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    session := analysis.Status.KASession
    status, err := h.kaClient.PollSession(ctx, session.ID) // GET /api/v1/incident/session/{id}
    if err != nil {
        return h.handleSessionPollError(ctx, analysis, err) // 404 -> session regeneration (BR-AA-HAPI-064.5)
    }

    switch status.Status {
    case "pending", "investigating":
        return h.handleSessionPollPending(ctx, analysis, status) // requeue after pollInterval
    case "user_driving":
        return h.handleSessionPollUserDriving(ctx, analysis, status) // DD-INTERACTIVE-002 takeover; still enforces the 25m cap
    case "completed":
        return h.handleSessionPollCompleted(ctx, analysis) // fetches result, delegates to ResponseProcessor
    case "failed":
        return h.handleSessionPollFailed(ctx, analysis, status) // -> PhaseFailed
    case "cancelled":
        return h.handleSessionPollCancelled(ctx, analysis)
    default:
        return h.handleSessionPollPending(ctx, analysis, status)
    }
}
```

**Wall-clock timeout** (`pkg/aianalysis/handlers/constants.go`): every poll checks `checkInvestigationTimeout`, which fails the analysis with `Reason=TransientError` if `time.Since(session.CreatedAt) > DefaultMaxInvestigationDuration` (**25 minutes**). This is a cap on the *entire session* (including any time spent in `user_driving` interactive mode) — it is not a per-HTTP-call timeout.

```go
// pkg/aianalysis/handlers/constants.go
const (
    DefaultSessionPollInterval      = 15 * time.Second // constant poll cadence, not backoff
    DefaultMaxInvestigationDuration = 25 * time.Minute  // wall-clock cap on the whole session
)
```

Once `PollSession` reports `"completed"`, the handler calls `GetSessionResult` (`GET /api/v1/incident/session/{id}/result`) to fetch the `IncidentResponse` and delegates to `ResponseProcessor.ProcessIncidentResponse`, which populates `status.rootCauseAnalysis`, `status.selectedWorkflow`, and transitions to `Analyzing`.

### Analyzing Phase

```go
// internal/controller/aianalysis/phase_handlers.go
func (r *AIAnalysisReconciler) reconcileAnalyzing(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    return r.AnalyzingHandler.Handle(ctx, analysis)
}
```

```go
// pkg/aianalysis/handlers/analyzing.go (condensed)
func (h *AnalyzingHandler) Handle(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    input := h.buildPolicyInput(analysis) // rego.PolicyInput: signal_context, target_resource,
                                           // classification, ka_response, remediation_target, action_type
    result, err := h.regoEvaluator.Evaluate(ctx, input)
    if err != nil {
        // BR-AI-014: graceful degradation — defaults to manual approval, never blocks
        h.metrics.RecordRegoEvaluation("error", true)
        analysis.Status.Phase = aianalysis.PhaseFailed
        return ctrl.Result{}, nil
    }

    outcome := aianalysis.OutcomeAutoApproved
    if result.ApprovalRequired {
        outcome = aianalysis.OutcomeRequiresApproval
    }
    h.metrics.RecordRegoEvaluation(outcome, result.Degraded)
    h.metrics.RecordApprovalDecision(outcome, getEnvironment(analysis))

    analysis.Status.ApprovalRequired = result.ApprovalRequired
    analysis.Status.ApprovalReason = result.Reason
    analysis.Status.Phase = aianalysis.PhaseCompleted
    now := metav1.Now()
    analysis.Status.CompletedAt = &now
    return ctrl.Result{}, nil
}
```

The Rego policy input (`pkg/aianalysis/rego/evaluator.go`) carries a `RemediationTarget *RemediationTargetInput` field (JSON key `remediation_target`) — sourced from `AIAnalysis.status.rootCauseAnalysis.remediationTarget` — **not** an `affected_resource` field.

---

## Utility Methods

### Deletion Handler

```go
// internal/controller/aianalysis/deletion_handler.go
func (r *AIAnalysisReconciler) handleDeletion(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    if controllerutil.ContainsFinalizer(analysis, FinalizerName) {
        // V1.0: no external cleanup needed (KA sessions expire server-side)
        controllerutil.RemoveFinalizer(analysis, FinalizerName)
        if err := r.Update(ctx, analysis); err != nil {
            return ctrl.Result{}, err
        }
    }
    return ctrl.Result{}, nil
}
```

---

## SetupWithManager

```go
// internal/controller/aianalysis/aianalysis_controller.go
//
// DD-CONTROLLER-001: a custom update predicate filters out status-only writes
// (e.g. PollCount/LastPolled during polling) so they don't trigger extra
// reconciles, letting RequeueAfter backoff intervals control poll timing.
// The controller also watches InvestigationSession CRDs to detect
// takeover/deletion (BR-INTERACTIVE-010).
func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
    if err := r.ValidateDependencies(); err != nil {
        return fmt.Errorf("aianalysis controller has nil dependencies: %w", err)
    }
    return ctrl.NewControllerManagedBy(mgr).
        For(&aianalysisv1.AIAnalysis{}).
        WatchesRawSource(source.Kind(mgr.GetCache(), &isv1alpha1.InvestigationSession{},
            handler.TypedEnqueueRequestsFromMapFunc(r.mapISToAIAnalysis),
            ISEventPredicate(),
        )).
        WithEventFilter(aiAnalysisUpdatePredicate()).
        Complete(r)
}
```

---

## Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **4-Phase Flow** | Pending → Investigating → Analyzing → Completed | "Approving" phase moved to RO (creates `RemediationApprovalRequest`) |
| **Async Session Model** | Submit/poll/result, not a blocking synchronous call | Non-blocking reconciles; investigations can take minutes without holding a goroutine or HTTP connection open (BR-AA-HAPI-064) |
| **25-Minute Wall-Clock Cap** | `DefaultMaxInvestigationDuration`, checked every poll | Bounds resource consumption from a stuck/slow KA session, including interactive takeover (#1078) |
| **Session Regeneration** | On 404 from `PollSession`/`GetSessionResult`, clear session ID and resubmit (capped at 5 regenerations) | Recovers from KA restarts without failing the whole investigation (BR-AA-HAPI-064.5/.6) |
| **No HolmesGPTConfig / InvestigationScope** | Removed | V1.0 uses a single Kubernaut Agent (KA) provider; KA decides investigation scope dynamically |
| **approvalRequired flag** | V1.0 signaling; RO creates `RemediationApprovalRequest` | No approval orchestration logic inside AIAnalysis itself |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [Reconciliation Phases](./reconciliation-phases.md) | Phase details |
| [Integration Points](./integration-points.md) | Service integration |
| [CRD Schema](./crd-schema.md) | Type definitions |
