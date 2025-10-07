## Controller Implementation

### Package Structure

```
pkg/ai/analysis/
├── reconciler.go              # AIAnalysisReconciler (Kubebuilder controller)
├── analyzer.go                # Analyzer interface (core business logic)
├── implementation.go          # Analyzer implementation
├── phases/
│   ├── investigating.go       # Investigation phase handler (BR-AI-011, BR-AI-012)
│   ├── analyzing.go           # Analysis phase handler (BR-AI-001, BR-AI-002, BR-AI-003)
│   ├── recommending.go        # Recommendation phase handler (BR-AI-006, BR-AI-007, BR-AI-008)
│   └── completed.go           # Completion phase handler (BR-AI-014)
├── integration/
│   ├── holmesgpt.go           # HolmesGPT client wrapper (uses pkg/ai/holmesgpt)
│   └── storage.go             # Historical pattern lookup (uses pkg/storage)
└── validation/
    ├── response_validator.go  # AI response validation (BR-AI-021)
    └── hallucination_detector.go  # Hallucination detection (BR-AI-023)

cmd/ai-analysis/
└── main.go                    # Binary entry point

test/unit/ai/analysis/         # Unit tests (70%+ coverage)
test/integration/ai/analysis/  # Integration tests (<20%)
test/e2e/ai/analysis/          # E2E tests (<10%)
```

### Core Interfaces

```go
// pkg/ai/analysis/analyzer.go
package analysis

import (
    "context"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
)

// Analyzer orchestrates AI-powered alert analysis
// Implements the core business logic for AIAnalysis CRD reconciliation
type Analyzer interface {
    // Investigate triggers HolmesGPT investigation (BR-AI-011, BR-AI-012)
    Investigate(ctx context.Context, req InvestigationRequest) (*InvestigationResult, error)

    // Analyze performs contextual analysis (BR-AI-001, BR-AI-002, BR-AI-003)
    Analyze(ctx context.Context, req AnalysisRequest) (*AnalysisResult, error)

    // Recommend generates ranked remediation recommendations (BR-AI-006, BR-AI-007, BR-AI-008)
    Recommend(ctx context.Context, req RecommendationRequest) (*RecommendationResult, error)

    // ValidateResponse validates AI responses (BR-AI-021, BR-AI-023)
    ValidateResponse(ctx context.Context, response interface{}) (*ValidationResult, error)
}

type InvestigationRequest struct {
    AlertContext     *aianalysisv1.AlertContext
    Scope            *aianalysisv1.InvestigationScope
    HolmesGPTConfig  *aianalysisv1.HolmesGPTConfig
}

type InvestigationResult struct {
    RootCauseHypotheses []RootCauseHypothesis
    CorrelatedAlerts    []CorrelatedAlert
    InvestigationReport string
    ContextualAnalysis  string
}

type AnalysisRequest struct {
    InvestigationResult *InvestigationResult
    AnalysisTypes       []string
    ConfidenceThreshold float64
}

type AnalysisResult struct {
    AnalysisTypes    []AnalysisTypeResult
    ValidationStatus ValidationStatus
}

type RecommendationRequest struct {
    AnalysisResult   *AnalysisResult
    AlertContext     *aianalysisv1.AlertContext
    ConstraintConfig *ConstraintConfig
}

type RecommendationResult struct {
    Recommendations []Recommendation
}
```

### Reconciler Implementation

```go
// pkg/ai/analysis/reconciler.go
package analysis

import (
    "context"
    "fmt"
    "time"

    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
)

const aiAnalysisFinalizer = "aianalysis.kubernaut.io/aianalysis-cleanup"

// AIAnalysisReconciler reconciles a AIAnalysis object
type AIAnalysisReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder

    // Business logic
    Analyzer Analyzer

    // Phase handlers
    InvestigatingPhase PhaseHandler
    AnalyzingPhase     PhaseHandler
    RecommendingPhase  PhaseHandler
    CompletedPhase     PhaseHandler
}

// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/finalizers,verbs=update
// +kubebuilder:rbac:groups=workflowexecution.kubernaut.io,resources=workflowexecutions,verbs=create;get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// NO RBAC for RemediationProcessing - data is self-contained in AIAnalysis spec

func (r *AIAnalysisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Fetch AIAnalysis CRD
    var aiAnalysis aianalysisv1.AIAnalysis
    if err := r.Get(ctx, req.NamespacedName, &aiAnalysis); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion
    if !aiAnalysis.ObjectMeta.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&aiAnalysis, aiAnalysisFinalizer) {
            if err := r.cleanupAIAnalysisResources(ctx, &aiAnalysis); err != nil {
                return ctrl.Result{}, err
            }

            controllerutil.RemoveFinalizer(&aiAnalysis, aiAnalysisFinalizer)
            if err := r.Update(ctx, &aiAnalysis); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer
    if !controllerutil.ContainsFinalizer(&aiAnalysis, aiAnalysisFinalizer) {
        controllerutil.AddFinalizer(&aiAnalysis, aiAnalysisFinalizer)
        if err := r.Update(ctx, &aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Validate self-contained data (no cross-CRD reads)
    if err := r.validateSpecData(&aiAnalysis); err != nil {
        log.Error(err, "Spec data validation failed")
        aiAnalysis.Status.Phase = "failed"
        aiAnalysis.Status.FailureReason = fmt.Sprintf("invalid_spec: %v", err)
        if err := r.Status().Update(ctx, &aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{}, nil
    }

    // Check phase timeout
    if timeout, reason := r.checkPhaseTimeout(&aiAnalysis); timeout {
        r.Recorder.Event(&aiAnalysis, corev1.EventTypeWarning, "PhaseTimeout", reason)
        aiAnalysis.Status.Phase = "failed"
        aiAnalysis.Status.FailureReason = reason
        if err := r.Status().Update(ctx, &aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{}, nil
    }

    // Phase-specific reconciliation
    switch aiAnalysis.Status.Phase {
    case "", "investigating":
        return r.InvestigatingPhase.Handle(ctx, &aiAnalysis)
    case "analyzing":
        return r.AnalyzingPhase.Handle(ctx, &aiAnalysis)
    case "recommending":
        return r.RecommendingPhase.Handle(ctx, &aiAnalysis)
    case "completed":
        return r.CompletedPhase.Handle(ctx, &aiAnalysis)
    case "failed":
        // Terminal state, no requeue
        return ctrl.Result{}, nil
    default:
        log.Info("Unknown phase", "phase", aiAnalysis.Status.Phase)
        return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
    }
}

func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&aianalysisv1.AIAnalysis{}).                    // Watch own CRD
        Owns(&workflowv1.WorkflowExecution{}).      // Watch owned WorkflowExecution
        // NO Watch on RemediationProcessing - data is self-contained in spec
        Complete(r)
}
```

### Phase Handler Interface

```go
// pkg/ai/analysis/phases/handler.go
package phases

import (
    "context"
    ctrl "sigs.k8s.io/controller-runtime"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
)

type PhaseHandler interface {
    Handle(ctx context.Context, aiAnalysis *aianalysisv1.AIAnalysis) (ctrl.Result, error)
}
```

### Investigation Phase Implementation

```go
// pkg/ai/analysis/phases/investigating.go
package phases

import (
    "context"
    "fmt"
    "time"

    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
    "github.com/jordigilh/kubernaut/pkg/ai/analysis"
    "github.com/jordigilh/kubernaut/pkg/ai/analysis/integration"
)

type InvestigatingPhase struct {
    Client         client.Client
    Analyzer       analysis.Analyzer
    HolmesClient   *integration.HolmesGPTClient
    StorageClient  *integration.StorageClient
}

func (p *InvestigatingPhase) Handle(ctx context.Context, aiAnalysis *aianalysisv1.AIAnalysis) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    log.Info("Handling investigating phase")

    // Initialize phase if first time
    if aiAnalysis.Status.Phase == "" {
        aiAnalysis.Status.Phase = "investigating"
        if aiAnalysis.Status.PhaseTransitions == nil {
            aiAnalysis.Status.PhaseTransitions = make(map[string]metav1.Time)
        }
        aiAnalysis.Status.PhaseTransitions["investigating"] = metav1.Now()

        if err := p.Client.Status().Update(ctx, aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }

        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    // Trigger HolmesGPT investigation (BR-AI-011, BR-AI-012)
    investigationReq := analysis.InvestigationRequest{
        AlertContext:    &aiAnalysis.Spec.AnalysisRequest.AlertContext,
        Scope:           &aiAnalysis.Spec.AnalysisRequest.InvestigationScope,
        HolmesGPTConfig: &aiAnalysis.Spec.HolmesGPTConfig,
    }

    result, err := p.Analyzer.Investigate(ctx, investigationReq)
    if err != nil {
        log.Error(err, "Investigation failed")
        aiAnalysis.Status.Phase = "failed"
        aiAnalysis.Status.FailureReason = fmt.Sprintf("investigation_failed: %v", err)
        if err := p.Client.Status().Update(ctx, aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }
        return ctrl.Result{}, nil
    }

    // Update status with investigation results
    aiAnalysis.Status.InvestigationResult = &aianalysisv1.InvestigationResult{
        RootCauseHypotheses: convertRootCauses(result.RootCauseHypotheses),
        CorrelatedAlerts:    convertCorrelatedAlerts(result.CorrelatedAlerts),
        InvestigationReport: result.InvestigationReport,
        ContextualAnalysis:  result.ContextualAnalysis,
    }

    // Transition to analyzing phase
    aiAnalysis.Status.Phase = "analyzing"
    aiAnalysis.Status.PhaseTransitions["analyzing"] = metav1.Now()

    // Set condition
    meta.SetStatusCondition(&aiAnalysis.Status.Conditions, metav1.Condition{
        Type:    "InvestigationComplete",
        Status:  metav1.ConditionTrue,
        Reason:  "RootCauseIdentified",
        Message: fmt.Sprintf("Identified %d root cause hypotheses", len(result.RootCauseHypotheses)),
    })

    if err := p.Client.Status().Update(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
```


---

