# AI Analysis Service - Controller Implementation

**Version**: v2.0
**Last Updated**: 2025-11-30
**Status**: ✅ V1.0 Scope Defined

---

## Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| v2.0 | 2025-11-30 | **REGENERATED**: Fixed SignalProcessing naming; Removed legacy phases (recommending→analyzing); Removed HolmesGPTConfig/InvestigationScope; Added DetectedLabels/CustomLabels/OwnerChain; V1.0 4-phase flow | DD-WORKFLOW-001 v1.8, DD-RECOVERY-002 |
| v1.1 | 2025-10-16 | Added self-documenting JSON format | DD-HOLMESGPT-009 |
| v1.0 | 2025-10-15 | Initial specification | - |

---

## Package Structure (V1.0)

```
internal/controller/aianalysis/
├── aianalysis_controller.go     # AIAnalysisReconciler (Kubebuilder controller)
├── phases/
│   ├── pending.go               # Validation and setup
│   ├── investigating.go         # HolmesGPT-API call (BR-AI-001, BR-AI-011)
│   ├── analyzing.go             # Rego policy evaluation (BR-AI-028)
│   └── completed.go             # Terminal state
├── rego/
│   └── evaluator.go             # Rego policy evaluation
└── holmesgpt/
    └── client.go                # HolmesGPT-API HTTP client

pkg/ai/holmesgpt/
├── client.go                    # HolmesGPT-API client interface
├── types.go                     # Request/response types
└── validation.go                # Workflow validation (BR-AI-023)

cmd/aianalysis/
└── main.go                      # Binary entry point

test/unit/controller/aianalysis/      # Unit tests (70%+)
test/integration/controller/aianalysis/ # Integration tests (<20%)
test/e2e/controller/aianalysis/       # E2E tests (<10%)
```

---

## Core Types (V1.0)

### Investigation Request

```go
// pkg/ai/holmesgpt/types.go
package holmesgpt

// InvestigationRequest - sent to HolmesGPT-API
// V1.0: No HolmesGPTConfig, no InvestigationScope (removed)
type InvestigationRequest struct {
    // Signal context
    SignalContext SignalContextInput `json:"signalContext"`

    // Kubernetes context from enrichment
    KubernetesContext *KubernetesContext `json:"kubernetesContext,omitempty"`

    // Labels for workflow filtering (DD-WORKFLOW-001 v1.8)
    DetectedLabels *DetectedLabels       `json:"detectedLabels,omitempty"`  // ADR-056: removed from EnrichmentResults
    CustomLabels   map[string][]string   `json:"customLabels,omitempty"`

    // Owner chain for DetectedLabels validation (DD-WORKFLOW-001 v1.7)
    OwnerChain []OwnerChainEntry `json:"ownerChain,omitempty"`  // ADR-055: removed from EnrichmentResults

    // Recovery context (if applicable)
    IsRecoveryAttempt  bool                `json:"isRecoveryAttempt,omitempty"`
    PreviousExecutions []PreviousExecution `json:"previousExecutions,omitempty"`
}

// InvestigationResponse - from HolmesGPT-API
type InvestigationResponse struct {
    InvestigationID string `json:"investigationId"`
    Status          string `json:"status"`

    // Workflow recommendation (from MCP catalog search)
    WorkflowRecommendation *WorkflowRecommendation `json:"workflowRecommendation"`

    // Investigation summary (for operator context)
    InvestigationSummary string `json:"investigationSummary"`
    RootCauseAnalysis    string `json:"rootCauseAnalysis"`
}

type WorkflowRecommendation struct {
    WorkflowID     string            `json:"workflowId"`     // UUID from catalog
    ContainerImage string            `json:"containerImage"` // OCI reference (BR-AI-075)
    Parameters     map[string]string `json:"parameters"`
    Confidence     float64           `json:"confidence"`     // 0.0-1.0
    Reasoning      string            `json:"reasoning"`
}
```

---

## Reconciler Implementation (V1.0)

### AIAnalysisReconciler

```go
// internal/controller/aianalysis/aianalysis_controller.go
package aianalysis

import (
    "context"
    "fmt"
    "time"

    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    "github.com/jordigilh/kubernaut/pkg/ai/holmesgpt"
)

const (
    aiAnalysisFinalizer      = "kubernaut.ai/cleanup"
    defaultInvestigateTimeout = 60 * time.Second
    defaultAnalyzeTimeout     = 5 * time.Second
)

// AIAnalysisReconciler reconciles AIAnalysis objects
type AIAnalysisReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder

    // HolmesGPT-API client
    HolmesGPTClient holmesgpt.Client

    // Rego policy evaluator
    RegoEvaluator *rego.Evaluator
}

// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Fetch AIAnalysis CRD
    var aiAnalysis aianalysisv1.AIAnalysis
    if err := r.Get(ctx, req.NamespacedName, &aiAnalysis); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion
    if !aiAnalysis.DeletionTimestamp.IsZero() {
        return r.handleDeletion(ctx, &aiAnalysis)
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&aiAnalysis, aiAnalysisFinalizer) {
        controllerutil.AddFinalizer(&aiAnalysis, aiAnalysisFinalizer)
        if err := r.Update(ctx, &aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Route to phase handler
    switch aiAnalysis.Status.Phase {
    case "", "Pending":
        return r.handlePendingPhase(ctx, &aiAnalysis)
    case "Investigating":
        return r.handleInvestigatingPhase(ctx, &aiAnalysis)
    case "Analyzing":
        return r.handleAnalyzingPhase(ctx, &aiAnalysis)
    case "Completed":
        return ctrl.Result{}, nil // Terminal state
    case "Failed":
        return ctrl.Result{}, nil // Terminal state
    default:
        log.Error(nil, "Unknown phase", "phase", aiAnalysis.Status.Phase)
        return ctrl.Result{}, nil
    }
}
```

---

## Phase Handlers (V1.0)

### Pending Phase

```go
func (r *AIAnalysisReconciler) handlePendingPhase(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Validate spec
    if aiAnalysis.Spec.EnrichmentResults == nil {
        return r.failWithReason(ctx, aiAnalysis, "EnrichmentResults is required")
    }

    // Initialize status
    aiAnalysis.Status.Phase = "Investigating"
    aiAnalysis.Status.StartTime = &metav1.Time{Time: time.Now()}

    if err := r.Status().Update(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, err
    }

    r.Recorder.Event(aiAnalysis, corev1.EventTypeNormal, "PhaseTransition", "Transitioned to Investigating")
    log.Info("Phase transition", "from", "Pending", "to", "Investigating")

    return ctrl.Result{Requeue: true}, nil
}
```

### Investigating Phase

```go
func (r *AIAnalysisReconciler) handleInvestigatingPhase(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Check timeout
    if r.isPhaseTimedOut(aiAnalysis, defaultInvestigateTimeout) {
        return r.failWithReason(ctx, aiAnalysis, "Investigation timeout exceeded (60s)")
    }

    // Build investigation request (V1.0 structure)
    req := r.buildInvestigationRequest(aiAnalysis)

    // Call HolmesGPT-API
    resp, err := r.HolmesGPTClient.Investigate(ctx, req)
    if err != nil {
        log.Error(err, "HolmesGPT-API call failed")
        // Retry with requeue
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    // Validate workflow recommendation exists
    if resp.WorkflowRecommendation == nil {
        return r.failWithReason(ctx, aiAnalysis, "No workflow recommendation received")
    }

    // Store investigation result
    aiAnalysis.Status.InvestigationSummary = resp.InvestigationSummary
    aiAnalysis.Status.SelectedWorkflow = &aianalysisv1.SelectedWorkflow{
        WorkflowID:     resp.WorkflowRecommendation.WorkflowID,
        ContainerImage: resp.WorkflowRecommendation.ContainerImage,
        Parameters:     resp.WorkflowRecommendation.Parameters,
        Confidence:     resp.WorkflowRecommendation.Confidence,
        Reasoning:      resp.WorkflowRecommendation.Reasoning,
    }

    // Transition to Analyzing
    aiAnalysis.Status.Phase = "Analyzing"
    if err := r.Status().Update(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, err
    }

    r.Recorder.Event(aiAnalysis, corev1.EventTypeNormal, "InvestigationComplete",
        fmt.Sprintf("Workflow recommended: %s (confidence: %.2f)",
            resp.WorkflowRecommendation.WorkflowID,
            resp.WorkflowRecommendation.Confidence))

    return ctrl.Result{Requeue: true}, nil
}

func (r *AIAnalysisReconciler) buildInvestigationRequest(
    aiAnalysis *aianalysisv1.AIAnalysis,
) *holmesgpt.InvestigationRequest {
    req := &holmesgpt.InvestigationRequest{
        SignalContext: aiAnalysis.Spec.SignalContext,
    }

    // Add enrichment data
    if aiAnalysis.Spec.EnrichmentResults != nil {
        req.KubernetesContext = aiAnalysis.Spec.EnrichmentResults.KubernetesContext
        req.DetectedLabels = aiAnalysis.Spec.EnrichmentResults.DetectedLabels  // ADR-056: removed from EnrichmentResults
        req.CustomLabels = aiAnalysis.Spec.EnrichmentResults.CustomLabels
        req.OwnerChain = aiAnalysis.Spec.EnrichmentResults.OwnerChain  // ADR-055: removed from EnrichmentResults
    }

    // Add recovery context (if applicable)
    if aiAnalysis.Spec.IsRecoveryAttempt {
        req.IsRecoveryAttempt = true
        req.PreviousExecutions = aiAnalysis.Spec.PreviousExecutions
    }

    return req
}
```

### Analyzing Phase

```go
func (r *AIAnalysisReconciler) handleAnalyzingPhase(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Check timeout
    if r.isPhaseTimedOut(aiAnalysis, defaultAnalyzeTimeout) {
        return r.failWithReason(ctx, aiAnalysis, "Analysis timeout exceeded (5s)")
    }

    // Validate workflow exists in catalog (BR-AI-023 - hallucination detection)
    if err := r.validateWorkflowExists(ctx, aiAnalysis.Status.SelectedWorkflow); err != nil {
        return r.failWithReason(ctx, aiAnalysis,
            fmt.Sprintf("Workflow validation failed: %v", err))
    }

    // Evaluate Rego approval policy
    decision, err := r.evaluateApprovalPolicy(ctx, aiAnalysis)
    if err != nil {
        log.Error(err, "Rego policy evaluation failed")
        return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
    }

    // Set approval decision (V1.0: RO handles notification)
    aiAnalysis.Status.ApprovalRequired = (decision == "MANUAL_APPROVAL_REQUIRED")
    if aiAnalysis.Status.ApprovalRequired {
        aiAnalysis.Status.ApprovalReason = r.buildApprovalReason(aiAnalysis)
    }

    // Transition to Completed
    aiAnalysis.Status.Phase = "Completed"
    aiAnalysis.Status.CompletionTime = &metav1.Time{Time: time.Now()}

    if err := r.Status().Update(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, err
    }

    r.Recorder.Event(aiAnalysis, corev1.EventTypeNormal, "AnalysisComplete",
        fmt.Sprintf("Analysis complete. Approval required: %v", aiAnalysis.Status.ApprovalRequired))

    return ctrl.Result{}, nil
}

func (r *AIAnalysisReconciler) evaluateApprovalPolicy(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) (string, error) {
    // Build policy input
    input := rego.ApprovalPolicyInput{
        Confidence:  aiAnalysis.Status.SelectedWorkflow.Confidence,
        Environment: aiAnalysis.Spec.SignalContext.Environment,
        Severity:    aiAnalysis.Spec.SignalContext.Severity,
        ActionType:  "workflow_execution", // Generic for V1.0

        // Labels for advanced policy decisions
        DetectedLabels: aiAnalysis.Spec.EnrichmentResults.DetectedLabels,  // ADR-056: removed from EnrichmentResults
        CustomLabels:   aiAnalysis.Spec.EnrichmentResults.CustomLabels,

        // Recovery context
        IsRecoveryAttempt:     aiAnalysis.Spec.IsRecoveryAttempt,
        RecoveryAttemptNumber: aiAnalysis.Spec.RecoveryAttemptNumber,
    }

    return r.RegoEvaluator.Evaluate(ctx, input)
}
```

---

## Utility Methods

### Timeout Check

```go
func (r *AIAnalysisReconciler) isPhaseTimedOut(
    aiAnalysis *aianalysisv1.AIAnalysis,
    timeout time.Duration,
) bool {
    if aiAnalysis.Status.StartTime == nil {
        return false
    }
    return time.Since(aiAnalysis.Status.StartTime.Time) > timeout
}
```

### Failure Handler

```go
func (r *AIAnalysisReconciler) failWithReason(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
    reason string,
) (ctrl.Result, error) {
    aiAnalysis.Status.Phase = "Failed"
    aiAnalysis.Status.FailureReason = &reason
    aiAnalysis.Status.CompletionTime = &metav1.Time{Time: time.Now()}

    if err := r.Status().Update(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, err
    }

    r.Recorder.Event(aiAnalysis, corev1.EventTypeWarning, "AnalysisFailed", reason)
    return ctrl.Result{}, nil
}
```

### Deletion Handler

```go
func (r *AIAnalysisReconciler) handleDeletion(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {
    if controllerutil.ContainsFinalizer(aiAnalysis, aiAnalysisFinalizer) {
        // Perform cleanup (if any external resources)
        // V1.0: No external cleanup needed

        // Remove finalizer
        controllerutil.RemoveFinalizer(aiAnalysis, aiAnalysisFinalizer)
        if err := r.Update(ctx, aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }
    }
    return ctrl.Result{}, nil
}
```

---

## SetupWithManager

```go
func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&aianalysisv1.AIAnalysis{}).
        Complete(r)
}
```

---

## Key Design Decisions (V1.0)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **4-Phase Flow** | Pending → Investigating → Analyzing → Completed | Simplified from 5-phase; "Approving" moved to RO |
| **No "Recommending" Phase** | Merged into Analyzing | HolmesGPT-API returns workflow recommendation directly |
| **No HolmesGPTConfig** | Removed | V1.0 uses single HolmesGPT-API provider |
| **No InvestigationScope** | Removed | HolmesGPT decides investigation scope dynamically |
| **DetectedLabels + CustomLabels** | Added to request | DD-WORKFLOW-001 v1.8 for workflow filtering (ADR-056: DetectedLabels removed from EnrichmentResults) |
| **OwnerChain** | Added to request | DD-WORKFLOW-001 v1.7 for label validation (ADR-055: removed from EnrichmentResults) |
| **PreviousExecutions as slice** | Added | Tracks ALL recovery attempts (not just last) |
| **approvalRequired flag** | V1.0 signaling | RO orchestrates notification (no AIApprovalRequest CRD) |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [Reconciliation Phases](./reconciliation-phases.md) | Phase details |
| [Integration Points](./integration-points.md) | Service integration |
| [CRD Schema](./crd-schema.md) | Type definitions |
| [DD-RECOVERY-002](../../../architecture/decisions/DD-RECOVERY-002-direct-aianalysis-recovery-flow.md) | Recovery flow |
