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

cmd/aianalysis/
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
    AlertContext     *aianalysisv1.SignalContext
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
    AlertContext     *aianalysisv1.SignalContext
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

    corev1 "k8s.io/api/core/v1"
    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
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
        Owns(&workflowexecutionv1.WorkflowExecution{}).      // Watch owned WorkflowExecution
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

    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
        AlertContext:    &aiAnalysis.Spec.AnalysisRequest.SignalContext,
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
    apimeta.SetStatusCondition(&aiAnalysis.Status.Conditions, metav1.Condition{
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

### 🔄 **Embedded Historical Context for Recovery Scenarios**

---

> **📋 Design Decision Status**
>
> **Current Implementation**: **Alternative 2** (Approved Design)
> **Status**: ✅ **Production-Ready**
> **Confidence**: 95%
> **Design Decision**: [DD-001](../../../architecture/DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2)
> **Business Requirement**: BR-WF-RECOVERY-011
>
> <details>
> <summary><b>Why Alternative 2?</b> (Click to expand)</summary>
>
> - ✅ **Temporal consistency**: All contexts captured at same timestamp by RemediationProcessing
> - ✅ **Fresh contexts**: Recovery gets CURRENT cluster state (not stale from initial attempt)
> - ✅ **Immutable audit trail**: Each RemediationProcessing CRD is complete snapshot
> - ✅ **Self-contained CRDs**: AIAnalysis reads from spec only (no API calls)
> - ✅ **Simplified testing**: AIAnalysis has no external dependencies
>
> **Full Analysis**: See [DESIGN_DECISIONS.md - DD-001](../../../architecture/DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2)
> </details>

---

**Status**: ✅ Phase 1 Critical Fix - Updated for Alternative 2
**Reference**: [`docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md) (Version 1.2 - Alternative 2)

#### Overview

When AIAnalysis CRD is created for **recovery** (after a workflow failure), the **Remediation Orchestrator** copies complete enrichment data from RemediationProcessing CRD into the AIAnalysis CRD spec. This enrichment data includes:
- **Fresh monitoring context** (current cluster state)
- **Fresh business context** (current ownership/runbooks)
- **Recovery context** (historical failures from Context API)

The AIAnalysis controller simply reads this enrichment data - no external API calls needed.

**Key Principle**: Self-contained CRDs - all data needed for reconciliation is in the CRD spec.

#### How It Works: Data Flow (Alternative 2)

```
1. Workflow Fails
   ↓
2. Remediation Orchestrator detects failure
   ↓
3. Remediation Orchestrator creates RemediationProcessing #2 (recovery)
   ↓
4. RemediationProcessing Controller enriches with ALL contexts:
   • Monitoring context (FRESH!)
   • Business context (FRESH!)
   • Recovery context from Context API (FRESH!)
   ↓
5. Remediation Orchestrator watches RP completion
   ↓
6. Remediation Orchestrator copies enrichment data to AIAnalysis.spec.enrichmentData
   ↓
7. AIAnalysis Controller reads enrichment data (NO API CALL)
   ↓
8. AIAnalysis Controller uses all contexts for HolmesGPT prompt
```

**Benefits**:
- ✅ AIAnalysis controller is simpler, no external dependencies
- ✅ Fresh monitoring/business context for each recovery attempt
- ✅ Temporal consistency (all contexts captured at same timestamp)
- ✅ Immutable audit trail (separate RemediationProcessing CRDs)

#### AIAnalysis CRD Spec with Enrichment Data (Alternative 2)

```go
// api/ai/v1/aianalysis_types.go
type AIAnalysisSpec struct {
    // Existing fields...
    RemediationRequestRef corev1.LocalObjectReference `json:"remediationRequestRef"`

    // NEW: Reference to source RemediationProcessing CRD (Alternative 2)
    RemediationProcessingRef *corev1.LocalObjectReference `json:"remediationProcessingRef,omitempty"`

    // Recovery-specific fields
    IsRecoveryAttempt      bool                         `json:"isRecoveryAttempt,omitempty"`
    RecoveryAttemptNumber  int                          `json:"recoveryAttemptNumber,omitempty"` // 1, 2, 3...
    FailedWorkflowRef      *corev1.LocalObjectReference `json:"failedWorkflowRef,omitempty"`
    FailedStep             *int                         `json:"failedStep,omitempty"`
    FailureReason          *string                      `json:"failureReason,omitempty"`
    PreviousAIAnalysisRefs []corev1.LocalObjectReference `json:"previousAIAnalysisRefs,omitempty"`

    // NEW: Complete enrichment data from RemediationProcessing (Alternative 2)
    // Copied from RemediationProcessing.status.enrichmentResults by Remediation Orchestrator
    EnrichmentData *EnrichmentData `json:"enrichmentData,omitempty"`
}

// EnrichmentData contains ALL contexts from RemediationProcessing enrichment
// Copied from RemediationProcessing.status.enrichmentResults
// See: docs/services/crd-controllers/01-remediationprocessor/crd-schema.md
type EnrichmentData struct {
    // Monitoring context (FRESH for recovery!)
    MonitoringContext *MonitoringContext `json:"monitoringContext,omitempty"`

    // Business context (FRESH for recovery!)
    BusinessContext *BusinessContext `json:"businessContext,omitempty"`

    // Recovery context (historical failures from Context API)
    // Only present for recovery attempts (IsRecoveryAttempt = true)
    RecoveryContext *RecoveryContext `json:"recoveryContext,omitempty"`

    // Enrichment metadata
    EnrichedAt     metav1.Time `json:"enrichedAt"`      // When RemediationProcessing enriched data
    ContextQuality string      `json:"contextQuality"`  // "complete", "partial", "minimal", "degraded"
}

// MonitoringContext - Kubernetes cluster state
type MonitoringContext struct {
    ClusterMetrics    map[string]interface{} `json:"clusterMetrics,omitempty"`
    PodStates         []PodState             `json:"podStates,omitempty"`
    ResourceUsage     ResourceUsage          `json:"resourceUsage,omitempty"`
    RecentEvents      []KubernetesEvent      `json:"recentEvents,omitempty"`
}

// BusinessContext - Ownership and operational data
type BusinessContext struct {
    OwnerTeam       string            `json:"ownerTeam,omitempty"`
    RunbookVersion  string            `json:"runbookVersion,omitempty"`
    SLALevel        string            `json:"slaLevel,omitempty"`
    ContactInfo     map[string]string `json:"contactInfo,omitempty"`
}

// RecoveryContext - Historical failure data from Context API
// See: docs/services/crd-controllers/01-remediationprocessor/crd-schema.md
type RecoveryContext struct {
    ContextQuality       string                `json:"contextQuality"`
    PreviousFailures     []PreviousFailure     `json:"previousFailures,omitempty"`
    RelatedAlerts        []RelatedAlert        `json:"relatedAlerts,omitempty"`
    HistoricalPatterns   []HistoricalPattern   `json:"historicalPatterns,omitempty"`
    SuccessfulStrategies []SuccessfulStrategy  `json:"successfulStrategies,omitempty"`
    RetrievedAt          metav1.Time           `json:"retrievedAt"`
}
```

**Key Changes (Alternative 2)**:
- ✅ Added `RemediationProcessingRef` - tracks source of enrichment data
- ✅ Replaced `HistoricalContext` with `EnrichmentData` - includes ALL contexts
- ✅ `EnrichmentData` contains monitoring + business + recovery contexts
- ✅ All data copied from `RemediationProcessing.status.enrichmentResults`

#### Simplified Reconciler (No Context API Client Needed)

```go
// AIAnalysisReconciler struct - NO Context API client needed
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
```

**Key Simplification**: No `ContextAPIClient` dependency - context is already embedded in CRD spec by Remediation Orchestrator.

#### Simplified Investigating Phase - Read Enrichment Data (Alternative 2)

```go
// pkg/ai/analysis/phases/investigating.go
func (p *InvestigatingPhase) Handle(
    ctx context.Context,
    aiAnalysis *aianalysisv1.AIAnalysis,
) (ctrl.Result, error) {

    log := ctrl.LoggerFrom(ctx)

    // Build base investigation request
    req := buildInvestigationRequest(aiAnalysis)

    // Read enrichment data from spec (copied from RemediationProcessing by RR)
    if aiAnalysis.Spec.EnrichmentData != nil {
        log.Info("Using enrichment data from RemediationProcessing",
            "remediationProcessing", aiAnalysis.Spec.RemediationProcessingRef.Name,
            "contextQuality", aiAnalysis.Spec.EnrichmentData.ContextQuality,
            "enrichedAt", aiAnalysis.Spec.EnrichmentData.EnrichedAt,
            "isRecovery", aiAnalysis.Spec.IsRecoveryAttempt)

        // Add ALL contexts to investigation request
        req.MonitoringContext = aiAnalysis.Spec.EnrichmentData.MonitoringContext
        req.BusinessContext = aiAnalysis.Spec.EnrichmentData.BusinessContext

        // Recovery context only present for recovery attempts
        if aiAnalysis.Spec.IsRecoveryAttempt && aiAnalysis.Spec.EnrichmentData.RecoveryContext != nil {
            log.Info("Recovery analysis - using enriched recovery context",
                "attemptNumber", aiAnalysis.Spec.RecoveryAttemptNumber,
                "recoveryContextQuality", aiAnalysis.Spec.EnrichmentData.RecoveryContext.ContextQuality,
                "previousFailures", len(aiAnalysis.Spec.EnrichmentData.RecoveryContext.PreviousFailures),
                "relatedAlerts", len(aiAnalysis.Spec.EnrichmentData.RecoveryContext.RelatedAlerts))

            req.RecoveryContext = aiAnalysis.Spec.EnrichmentData.RecoveryContext
            req.IsRecoveryAttempt = true

            // Track recovery context metrics
            aiAnalysis.Status.HistoricalDataCount = len(aiAnalysis.Spec.EnrichmentData.RecoveryContext.PreviousFailures)
        }

        // Update status to reflect enrichment quality
        aiAnalysis.Status.ContextQuality = aiAnalysis.Spec.EnrichmentData.ContextQuality
    }

    // Proceed with HolmesGPT investigation
    result, err := p.Analyzer.Investigate(ctx, req)
    if err != nil {
        return ctrl.Result{}, err
    }

    // Store investigation result
    aiAnalysis.Status.InvestigationResult = result
    aiAnalysis.Status.Phase = "analyzing"

    if err := p.Client.Status().Update(ctx, aiAnalysis); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
```

**Key Changes (Alternative 2)**:
- ✅ No Context API client dependency (same as Option B)
- ✅ No HTTP requests during reconciliation (same as Option B)
- ✅ Reads `EnrichmentData` instead of `HistoricalContext`
- ✅ Has access to ALL contexts (monitoring + business + recovery)
- ✅ Fresh monitoring/business data for recovery attempts
- ✅ Graceful degradation handled by RemediationProcessing Controller

#### Enhanced Prompt Engineering for Recovery (Alternative 2)

When enrichment data is present in the CRD, the prompt to HolmesGPT includes ALL contexts:

```go
// pkg/ai/analysis/prompt_builder.go
func buildRecoveryPrompt(
    aiAnalysis *aianalysisv1.AIAnalysis,
) string {

    // Read enrichment data from spec
    enrichmentData := aiAnalysis.Spec.EnrichmentData
    if enrichmentData == nil {
        return buildStandardPrompt(aiAnalysis)
    }

    prompt := fmt.Sprintf(`RECOVERY ANALYSIS REQUEST

This is recovery attempt #%d after workflow failure.

ENRICHMENT QUALITY: %s (Enriched: %s)

CURRENT SITUATION (FRESH DATA):
• Alert: %s
• Severity: %s
• Target: %s
• Namespace: %s

`,
        aiAnalysis.Spec.RecoveryAttemptNumber,
        enrichmentData.ContextQuality,
        enrichmentData.EnrichedAt.Format(time.RFC3339),
        aiAnalysis.Spec.SignalContext.AlertName,
        aiAnalysis.Spec.SignalContext.Severity,
        aiAnalysis.Spec.SignalContext.TargetResource.Kind,
        aiAnalysis.Spec.SignalContext.TargetResource.Namespace,
    )

    // Add FRESH monitoring context (Alternative 2 benefit!)
    if enrichmentData.MonitoringContext != nil {
        prompt += "CURRENT MONITORING CONTEXT (FRESH!):\n"
        if enrichmentData.MonitoringContext.ResourceUsage != nil {
            prompt += fmt.Sprintf(`
• CPU Usage: %s
• Memory Usage: %s
• Pod Count: %d
• Recent Events: %d
`,
                enrichmentData.MonitoringContext.ResourceUsage.CPU,
                enrichmentData.MonitoringContext.ResourceUsage.Memory,
                len(enrichmentData.MonitoringContext.PodStates),
                len(enrichmentData.MonitoringContext.RecentEvents),
            )
        }
    }

    // Add FRESH business context (Alternative 2 benefit!)
    if enrichmentData.BusinessContext != nil {
        prompt += "CURRENT BUSINESS CONTEXT (FRESH!):\n"
        prompt += fmt.Sprintf(`
• Owner Team: %s
• Runbook: %s (may have been updated!)
• SLA Level: %s
`,
            enrichmentData.BusinessContext.OwnerTeam,
            enrichmentData.BusinessContext.RunbookVersion,
            enrichmentData.BusinessContext.SLALevel,
        )
    }

    // Add recovery context (historical failures)
    if aiAnalysis.Spec.IsRecoveryAttempt && enrichmentData.RecoveryContext != nil {
        if len(enrichmentData.RecoveryContext.PreviousFailures) > 0 {
            prompt += "\nPREVIOUS FAILURES:\n"
            for _, failure := range enrichmentData.RecoveryContext.PreviousFailures {
                prompt += fmt.Sprintf(`
Attempt #%d:
• Workflow: %s
• Failed Step: %d
• Action: %s
• Error: %s
• Timestamp: %s
`,
                    failure.AttemptNumber,
                    failure.WorkflowRef,
                    failure.FailedStep,
                    failure.Action,
                    failure.FailureReason,
                    failure.Timestamp.Format(time.RFC3339),
                )
            }
        }

        // Add pattern information from recovery context
        if len(enrichmentData.RecoveryContext.HistoricalPatterns) > 0 {
            prompt += "\nHISTORICAL PATTERNS:\n"
            for _, pattern := range enrichmentData.RecoveryContext.HistoricalPatterns {
                prompt += fmt.Sprintf("• %s: %d occurrences, %.1f%% success rate\n",
                    pattern.Pattern,
                    pattern.Occurrences,
                    pattern.SuccessRate*100,
                )
            }
        }

        // Add successful strategies from recovery context
        if len(enrichmentData.RecoveryContext.SuccessfulStrategies) > 0 {
            prompt += "\nSUCCESSFUL STRATEGIES FOR SIMILAR ALERTS:\n"
            for _, strategy := range enrichmentData.RecoveryContext.SuccessfulStrategies {
                prompt += fmt.Sprintf("• %s: %s (%.1f%% confidence, used %d times)\n",
                    strategy.Strategy,
                    strategy.Description,
                    strategy.Confidence*100,
                    strategy.SuccessCount,
                )
            }
        }
    }

    prompt += `
IMPORTANT INSTRUCTIONS:
1. The previous approach(es) FAILED - you must provide an ALTERNATIVE strategy
2. Analyze the failure patterns to avoid repeating the same mistakes
3. Consider successful strategies used for similar alerts
4. Provide a remediation plan that addresses the root cause differently
5. Include reasoning for why this approach will succeed where previous attempts failed

Please provide:
1. Root cause analysis (considering previous failures)
2. Alternative remediation strategy (different from previous attempts)
3. Justification for why this approach will work
4. Risk assessment and mitigation steps
5. Estimated success probability
`

    return prompt
}
```

#### Graceful Degradation (Handled by RemediationProcessing Controller)

**BR-WF-RECOVERY-011 Requirement**: If Context API is unavailable, proceed without historical context (don't fail)

**Implementation Note (Alternative 2)**: Graceful degradation is handled by **RemediationProcessing Controller** when it enriches the recovery attempt, NOT by AIAnalysis controller.

```go
// AIAnalysis controller doesn't need graceful degradation logic
// If enrichmentData is nil or degraded, controller just proceeds with what's available

if aiAnalysis.Spec.EnrichmentData == nil {
    // No enrichment data provided - proceed with standard analysis
    log.Info("No enrichment data available for analysis")
} else if aiAnalysis.Spec.EnrichmentData.ContextQuality == "degraded" {
    // Degraded context - use what's available
    log.Warn("Using degraded enrichment data",
        "contextQuality", aiAnalysis.Spec.EnrichmentData.ContextQuality)

    // Recovery context may be missing if Context API failed
    if aiAnalysis.Spec.IsRecoveryAttempt && aiAnalysis.Spec.EnrichmentData.RecoveryContext == nil {
        log.Warn("Recovery context unavailable - Context API may be down",
            "attemptNumber", aiAnalysis.Spec.RecoveryAttemptNumber)
    }
}
```

**Where Graceful Degradation Happens (Alternative 2)**:
- **Location**: RemediationProcessing Controller's `enrichRecoveryContext()` function
- **Fallback**: Build minimal recovery context from `FailedWorkflowRef` if Context API fails
- **See**: `docs/services/crd-controllers/01-remediationprocessor/controller-implementation.md`

#### AIAnalysis Status Fields for Context Tracking

```go
// api/ai/v1/aianalysis_types.go
type AIAnalysisStatus struct {
    // Existing fields...
    Phase string `json:"phase"`

    // Context quality tracking (from embedded context in spec)
    ContextQuality      string `json:"contextQuality"`                // "complete", "partial", "minimal", "degraded" (copied from spec)
    HistoricalDataCount int    `json:"historicalDataCount,omitempty"` // Number of previous failures used

    // Investigation result (may include historical context)
    InvestigationResult *InvestigationResult `json:"investigationResult,omitempty"`
}
```

**Simplified Status (Alternative 2)**:
- **Removed**: `ContextAPIAvailable` (no longer queries API directly)
- **Kept**: `ContextQuality` (copied from `spec.enrichmentData.contextQuality`)
- **Kept**: `HistoricalDataCount` (for metrics/observability - recovery context only)

#### Testing Strategy

**Unit Tests** (Simplified - No API Mocking):
```go
func TestEmbeddedContext_RecoveryScenario(t *testing.T) {
    // AIAnalysis with embedded historical context
    aiAnalysis := &aianalysisv1.AIAnalysis{
        Spec: aianalysisv1.AIAnalysisSpec{
            IsRecoveryAttempt:     true,
            RecoveryAttemptNumber: 2,
            RemediationRequestRef: corev1.LocalObjectReference{Name: "rr-001"},

            // Embedded context (populated by Remediation Orchestrator)
            HistoricalContext: &aianalysisv1.HistoricalContext{
                ContextQuality: "complete",
                PreviousFailures: []aianalysisv1.PreviousFailure{
                    {
                        WorkflowRef:   "workflow-001",
                        AttemptNumber: 1,
                        FailedStep:    3,
                        Action:        "scale-deployment",
                        FailureReason: "Operation timed out after 5m",
                    },
                },
                RetrievedAt: metav1.Now(),
            },
        },
    }

    phase := &InvestigatingPhase{
        // No Context API client needed!
    }

    result, err := phase.Handle(context.Background(), aiAnalysis)

    // Verify controller read embedded context successfully
    assert.NoError(t, err)
    assert.Equal(t, "complete", aiAnalysis.Status.ContextQuality)
    assert.Equal(t, 1, aiAnalysis.Status.HistoricalDataCount)
}

func TestEmbeddedContext_DegradedQuality(t *testing.T) {
    // AIAnalysis with degraded context (fallback data from Remediation Orchestrator)
    aiAnalysis := &aianalysisv1.AIAnalysis{
        Spec: aianalysisv1.AIAnalysisSpec{
            IsRecoveryAttempt:     true,
            RecoveryAttemptNumber: 1,
            RemediationRequestRef: corev1.LocalObjectReference{Name: "rr-001"},

            // Degraded context (Context API was unavailable, Remediation Orchestrator used fallback)
            HistoricalContext: &aianalysisv1.HistoricalContext{
                ContextQuality:   "degraded",
                PreviousFailures: []aianalysisv1.PreviousFailure{},
                RetrievedAt:      metav1.Now(),
            },
        },
    }

    phase := &InvestigatingPhase{}

    // Should proceed successfully even with degraded context
    result, err := phase.Handle(context.Background(), aiAnalysis)

    assert.NoError(t, err)
    assert.Equal(t, "degraded", aiAnalysis.Status.ContextQuality)
    assert.Equal(t, 0, aiAnalysis.Status.HistoricalDataCount)
}

func TestEmbeddedContext_NoContextProvided(t *testing.T) {
    // AIAnalysis without historical context (non-recovery or first attempt)
    aiAnalysis := &aianalysisv1.AIAnalysis{
        Spec: aianalysisv1.AIAnalysisSpec{
            IsRecoveryAttempt:     false,
            RemediationRequestRef: corev1.LocalObjectReference{Name: "rr-001"},
            HistoricalContext:     nil,  // No context
        },
    }

    phase := &InvestigatingPhase{}

    // Should proceed with standard analysis
    result, err := phase.Handle(context.Background(), aiAnalysis)

    assert.NoError(t, err)
    assert.Empty(t, aiAnalysis.Status.ContextQuality)
    assert.Equal(t, 0, aiAnalysis.Status.HistoricalDataCount)
}
```

**Key Testing Simplifications**:
- ✅ No Context API client mocking needed
- ✅ No HTTP response simulation needed
- ✅ Tests focus on reading embedded data
- ✅ Faster test execution (no network calls)

#### Context API Integration (Handled by Remediation Orchestrator)

**AIAnalysis Controller**: Does NOT integrate with Context API directly

**Integration Point**: Remediation Orchestrator queries Context API and embeds result in AIAnalysis CRD

**For Context API Details**: See `docs/services/crd-controllers/05-remediationorchestrator/controller-implementation.md` (C7)

#### Metrics

```go
// Simplified metrics - no API latency tracking needed in AIAnalysis controller
var (
    recoveryAnalysisWithContext = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_recovery_analysis_context_quality",
            Help: "Recovery analyses by embedded context quality",
        },
        []string{"quality"}, // complete, partial, minimal, degraded
    )

    embeddedContextSize = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "kubernaut_embedded_context_size_bytes",
            Help:    "Size of embedded historical context in AIAnalysis CRDs",
            Buckets: []float64{1024, 5120, 10240, 51200, 102400}, // 1KB to 100KB
        },
    )
)
```

#### Implementation Checklist (Option B - Simplified)

- [ ] Update AIAnalysis CRD API types to include `HistoricalContext` in spec
- [ ] Update InvestigatingPhase to read embedded `historicalContext` from spec
- [ ] Enhance prompt engineering for recovery with embedded historical context
- [ ] Add CRD spec fields: `IsRecoveryAttempt`, `RecoveryAttemptNumber`, `HistoricalContext`
- [ ] Add CRD status fields: `ContextQuality`, `HistoricalDataCount`
- [ ] Write unit tests for reading embedded context
- [ ] Write unit tests for degraded context handling
- [ ] Add metrics for embedded context size
- [ ] Update prompt engineering documentation

**Removed** (moved to Remediation Orchestrator):
- ~~Add Context API client interface~~ → See C7
- ~~Implement graceful degradation~~ → See C7
- ~~Mock Context API in tests~~ → See C7

#### Related Documentation

- **Architecture**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)
- **Design Decision**: [`OPTION_B_IMPLEMENTATION_SUMMARY.md`](../../../architecture/OPTION_B_IMPLEMENTATION_SUMMARY.md)
- **Business Requirements**: BR-WF-RECOVERY-011 in `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`
- **Remediation Orchestrator** (Context API integration): `docs/services/crd-controllers/05-remediationorchestrator/controller-implementation.md` (C7)
- **Prompt Engineering**: [`prompt-engineering-dependencies.md`](./prompt-engineering-dependencies.md) (needs update)
- **Integration Points**: [`integration-points.md`](./integration-points.md) (to be updated in C8)

#### Summary: Option B Benefits for AIAnalysis Controller

✅ **40% Code Reduction**: No Context API client, no HTTP handling, no graceful degradation
✅ **Simpler Testing**: No API mocking, faster test execution
✅ **No External Dependencies**: Self-contained CRD pattern
✅ **Better Failure Handling**: Context API failures handled before analysis, not during
✅ **Clear Audit Trail**: Complete context visible in CRD YAML

---

