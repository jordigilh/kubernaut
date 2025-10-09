## Controller Implementation

**Location**: `internal/controller/alertprocessing_controller.go`

### Controller Configuration

**Critical Patterns from [MULTI_CRD_RECONCILIATION_ARCHITECTURE.md](../../architecture/MULTI_CRD_RECONCILIATION_ARCHITECTURE.md)**:
1. **Owner References**: RemediationProcessing CRD owned by RemediationRequest for cascade deletion
2. **Finalizers**: Cleanup coordination before deletion
3. **Watch Optimization**: Status updates trigger RemediationRequest reconciliation
4. **Timeout Handling**: Phase-level timeout detection and escalation
5. **Event Emission**: Operational visibility through Kubernetes events

```go
package controller

import (
    "context"
    "fmt"
    "strings"
    "time"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    apimeta "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
    "sigs.k8s.io/controller-runtime/pkg/log"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/ai/v1"
)

const (
    // Finalizer for cleanup coordination
    alertProcessingFinalizer = "remediationprocessing.kubernaut.io/finalizer"

    // Timeout configuration
    defaultPhaseTimeout = 5 * time.Minute  // Max time per phase
)

// RemediationProcessingReconciler reconciles an RemediationProcessing object
type RemediationProcessingReconciler struct {
    client.Client
    Scheme          *runtime.Scheme
    Recorder        record.EventRecorder     // Event emission for visibility
    ContextService  ContextService           // Stateless HTTP call to Context Service
    ContextAPIClient ContextAPIClient        // NEW: Context API client for recovery context (Alternative 2)
    Classifier      *EnvironmentClassifier   // In-process classification
}

// ContextAPIClient interface for querying Context API recovery context
// See: docs/services/stateless/context-api/api-specification.md
// See: docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md (Alternative 2)
type ContextAPIClient interface {
    GetRemediationContext(ctx context.Context, remediationRequestID string) (*ContextAPIResponse, error)
}

// ContextAPIResponse from Context API recovery endpoint
type ContextAPIResponse struct {
    RemediationRequestID string                `json:"remediationRequestId"`
    CurrentAttempt       int                   `json:"currentAttempt"`
    ContextQuality       string                `json:"contextQuality"`
    PreviousFailures     []PreviousFailureData `json:"previousFailures"`
    RelatedAlerts        []RelatedAlertData    `json:"relatedAlerts"`
    HistoricalPatterns   []HistoricalPatternData `json:"historicalPatterns"`
    SuccessfulStrategies []SuccessfulStrategyData `json:"successfulStrategies"`
}

// Context API data structures (mirrors API response)
type PreviousFailureData struct {
    WorkflowRef      string            `json:"workflowRef"`
    AttemptNumber    int               `json:"attemptNumber"`
    FailedStep       int               `json:"failedStep"`
    Action           string            `json:"action"`
    ErrorType        string            `json:"errorType"`
    FailureReason    string            `json:"failureReason"`
    Duration         string            `json:"duration"`
    Timestamp        string            `json:"timestamp"`
    ClusterState     map[string]string `json:"clusterState"`
    ResourceSnapshot map[string]string `json:"resourceSnapshot"`
}

type RelatedAlertData struct {
    AlertFingerprint string  `json:"alertFingerprint"`
    AlertName        string  `json:"alertName"`
    Correlation      float64 `json:"correlation"`
    Timestamp        string  `json:"timestamp"`
}

type HistoricalPatternData struct {
    Pattern             string  `json:"pattern"`
    Occurrences         int     `json:"occurrences"`
    SuccessRate         float64 `json:"successRate"`
    AverageRecoveryTime string  `json:"averageRecoveryTime"`
}

type SuccessfulStrategyData struct {
    Strategy     string  `json:"strategy"`
    Description  string  `json:"description"`
    SuccessCount int     `json:"successCount"`
    LastUsed     string  `json:"lastUsed"`
    Confidence   float64 `json:"confidence"`
}

// ========================================
// ARCHITECTURAL NOTE: Remediation Orchestrator Pattern
// ========================================
//
// This controller (RemediationProcessing) updates ONLY its own status.
// The RemediationRequest Controller (Remediation Orchestrator) watches this CRD and aggregates status.
//
// DO NOT update RemediationRequest.status from this controller.
// The watch-based coordination pattern handles all status aggregation automatically.
//
// See: docs/services/crd-controllers/05-remediationorchestrator/overview.md
// See: docs/services/crd-controllers/CENTRAL_CONTROLLER_VIOLATION_ANALYSIS.md

//+kubebuilder:rbac:groups=kubernaut.io,resources=alertprocessings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertprocessings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertprocessings/finalizers,verbs=update
//+kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=create
//+kubebuilder:rbac:groups=kubernaut.io,resources=alertremediations,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *RemediationProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Fetch RemediationProcessing CRD
    var alertProcessing processingv1.RemediationProcessing
    if err := r.Get(ctx, req.NamespacedName, &alertProcessing); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion with finalizer
    if !alertProcessing.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &alertProcessing)
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&alertProcessing, alertProcessingFinalizer) {
        controllerutil.AddFinalizer(&alertProcessing, alertProcessingFinalizer)
        if err := r.Update(ctx, &alertProcessing); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Set owner reference to RemediationRequest (for cascade deletion)
    if err := r.ensureOwnerReference(ctx, &alertProcessing); err != nil {
        log.Error(err, "Failed to set owner reference")
        r.Recorder.Event(&alertProcessing, "Warning", "OwnerReferenceFailed",
            fmt.Sprintf("Failed to set owner reference: %v", err))
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Check for phase timeout (5 minutes per phase default)
    if r.isPhaseTimedOut(&alertProcessing) {
        return r.handlePhaseTimeout(ctx, &alertProcessing)
    }

    // Initialize phase if empty
    if alertProcessing.Status.Phase == "" {
        alertProcessing.Status.Phase = "enriching"
        alertProcessing.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        if err := r.Status().Update(ctx, &alertProcessing); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on current phase
    var result ctrl.Result
    var err error

    switch alertProcessing.Status.Phase {
    case "enriching":
        result, err = r.reconcileEnriching(ctx, &alertProcessing)
    case "classifying":
        result, err = r.reconcileClassifying(ctx, &alertProcessing)
    case "routing":
        result, err = r.reconcileRouting(ctx, &alertProcessing)
    case "completed":
        // Terminal state - use optimized requeue strategy
        return r.determineRequeueStrategy(&alertProcessing), nil
    default:
        log.Error(nil, "Unknown phase", "phase", alertProcessing.Status.Phase)
        r.Recorder.Event(&alertProcessing, "Warning", "UnknownPhase",
            fmt.Sprintf("Unknown phase: %s", alertProcessing.Status.Phase))
        return ctrl.Result{RequeueAfter: time.Second * 30}, nil
    }

    // Status update triggers RemediationRequest watch (automatic coordination)
    // No need to manually update RemediationRequest - watch mechanism handles it

    return result, err
}

// ensureOwnerReference sets RemediationRequest as owner for cascade deletion
func (r *RemediationProcessingReconciler) ensureOwnerReference(ctx context.Context, ap *processingv1.RemediationProcessing) error {
    // Skip if owner reference already set
    if len(ap.OwnerReferences) > 0 {
        return nil
    }

    // Fetch RemediationRequest to set as owner
    var alertRemediation remediationv1.RemediationRequest
    if err := r.Get(ctx, client.ObjectKey{
        Name:      ap.Spec.RemediationRequestRef.Name,
        Namespace: ap.Spec.RemediationRequestRef.Namespace,
    }, &alertRemediation); err != nil {
        return fmt.Errorf("failed to get RemediationRequest for owner reference: %w", err)
    }

    // Set owner reference
    if err := controllerutil.SetControllerReference(&alertRemediation, ap, r.Scheme); err != nil {
        return fmt.Errorf("failed to set controller reference: %w", err)
    }

    // Update with owner reference
    if err := r.Update(ctx, ap); err != nil {
        return fmt.Errorf("failed to update with owner reference: %w", err)
    }

    return nil
}

// isPhaseTimedOut checks if current phase has exceeded timeout
func (r *RemediationProcessingReconciler) isPhaseTimedOut(ap *processingv1.RemediationProcessing) bool {
    if ap.Status.PhaseStartTime == nil {
        return false
    }

    // Don't timeout completed phase
    if ap.Status.Phase == "completed" {
        return false
    }

    elapsed := time.Since(ap.Status.PhaseStartTime.Time)
    return elapsed > defaultPhaseTimeout
}

// handlePhaseTimeout handles phase timeout escalation
func (r *RemediationProcessingReconciler) handlePhaseTimeout(ctx context.Context, ap *processingv1.RemediationProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    elapsed := time.Since(ap.Status.PhaseStartTime.Time)
    log.Error(nil, "Phase timeout exceeded",
        "phase", ap.Status.Phase,
        "elapsed", elapsed,
        "timeout", defaultPhaseTimeout)

    // Emit timeout event
    r.Recorder.Event(ap, "Warning", "PhaseTimeout",
        fmt.Sprintf("Phase %s exceeded timeout of %s (elapsed: %s)",
            ap.Status.Phase, defaultPhaseTimeout, elapsed))

    // Add timeout condition
    timeoutCondition := metav1.Condition{
        Type:    "PhaseTimeout",
        Status:  metav1.ConditionTrue,
        Reason:  "TimeoutExceeded",
        Message: fmt.Sprintf("Phase %s exceeded %s timeout", ap.Status.Phase, defaultPhaseTimeout),
        LastTransitionTime: metav1.Now(),
    }
    apimeta.SetStatusCondition(&ap.Status.Conditions, timeoutCondition)

    // Record timeout metric
    ErrorsTotal.WithLabelValues("phase_timeout", ap.Status.Phase).Inc()

    // Update status
    if err := r.Status().Update(ctx, ap); err != nil {
        return ctrl.Result{}, err
    }

    // Move to next phase or fail based on phase
    switch ap.Status.Phase {
    case "enriching":
        // Use degraded mode - continue with basic context
        ap.Status.Phase = "classifying"
        ap.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        r.Recorder.Event(ap, "Normal", "DegradedMode",
            "Enrichment timeout - continuing with basic context")
    case "classifying":
        // Use default environment classification
        ap.Status.EnvironmentClassification = r.getDefaultClassification(ap)
        ap.Status.Phase = "routing"
        ap.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        r.Recorder.Event(ap, "Normal", "DefaultClassification",
            "Classification timeout - using default environment")
    case "routing":
        // Routing timeout is critical - emit error event
        r.Recorder.Event(ap, "Warning", "RoutingFailed",
            "Routing phase timeout - manual intervention required")
        return ctrl.Result{RequeueAfter: time.Minute * 2}, fmt.Errorf("routing timeout")
    }

    return ctrl.Result{Requeue: true}, r.Status().Update(ctx, ap)
}

// determineRequeueStrategy provides optimized requeue based on phase
func (r *RemediationProcessingReconciler) determineRequeueStrategy(ap *processingv1.RemediationProcessing) ctrl.Result {
    switch ap.Status.Phase {
    case "completed":
        // Terminal state - no requeue needed (watch handles updates)
        return ctrl.Result{}
    case "enriching", "classifying", "routing":
        // Active phases - short requeue for progress
        return ctrl.Result{RequeueAfter: time.Second * 10}
    default:
        // Unknown state - conservative requeue
        return ctrl.Result{RequeueAfter: time.Second * 30}
    }
}

// reconcileDelete handles cleanup before deletion
func (r *RemediationProcessingReconciler) reconcileDelete(ctx context.Context, ap *processingv1.RemediationProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if !controllerutil.ContainsFinalizer(ap, alertProcessingFinalizer) {
        return ctrl.Result{}, nil
    }

    log.Info("Cleaning up RemediationProcessing resources", "name", ap.Name)

    // Perform cleanup tasks
    // - Audit data should already be persisted
    // - No additional cleanup needed (AIAnalysis CRD creation is idempotent)

    // Emit deletion event
    r.Recorder.Event(ap, "Normal", "Cleanup",
        "RemediationProcessing cleanup completed before deletion")

    // Remove finalizer
    controllerutil.RemoveFinalizer(ap, alertProcessingFinalizer)
    if err := r.Update(ctx, ap); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}

// getDefaultClassification provides fallback classification on timeout
func (r *RemediationProcessingReconciler) getDefaultClassification(ap *processingv1.RemediationProcessing) processingv1.EnvironmentClassification {
    // Use namespace prefix or labels as fallback
    environment := "unknown"
    if strings.HasPrefix(ap.Spec.Alert.Namespace, "prod") {
        environment = "production"
    } else if strings.HasPrefix(ap.Spec.Alert.Namespace, "stag") {
        environment = "staging"
    } else if strings.HasPrefix(ap.Spec.Alert.Namespace, "dev") {
        environment = "development"
    }

    return processingv1.EnvironmentClassification{
        Environment:      environment,
        Confidence:       0.5, // Low confidence fallback
        BusinessPriority: "P2",
        SLARequirement:   "30m",
    }
}

func (r *RemediationProcessingReconciler) reconcileEnriching(ctx context.Context, ap *processingv1.RemediationProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    isRecovery := ap.Spec.IsRecoveryAttempt
    if isRecovery {
        log.Info("Enriching RECOVERY attempt with context",
            "fingerprint", ap.Spec.Signal.Fingerprint,
            "attemptNumber", ap.Spec.RecoveryAttemptNumber,
            "failedWorkflow", func() string {
                if ap.Spec.FailedWorkflowRef != nil {
                    return ap.Spec.FailedWorkflowRef.Name
                }
                return "unknown"
            }())
    } else {
        log.Info("Enriching alert with context", "fingerprint", ap.Spec.Signal.Fingerprint)
    }

    // Call Context Service for monitoring/business enrichment (ALWAYS - gets FRESH data)
    enrichmentResults, err := r.ContextService.GetContext(ctx, ap.Spec.Alert)
    if err != nil {
        log.Error(err, "Failed to get context")
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Update status with enrichment results
    ap.Status.EnrichmentResults = enrichmentResults

    // ========================================
    // RECOVERY CONTEXT ENRICHMENT (Alternative 2)
    // 📋 Design Decision: DD-001 | ✅ Approved Design | Confidence: 95%
    // See: docs/architecture/DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2
    // See: docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md (Version 1.2)
    // See: BR-WF-RECOVERY-011
    //
    // WHY Alternative 2?
    // - ✅ Temporal consistency: All contexts (monitoring + business + recovery) at same timestamp
    // - ✅ Fresh contexts: Recovery gets CURRENT cluster state, not stale from initial attempt
    // - ✅ Immutable audit trail: Each RemediationProcessing CRD is complete snapshot
    // - ✅ Self-contained CRDs: AIAnalysis reads from spec only (no API calls)
    // ========================================

    if isRecovery {
        log.Info("Recovery attempt detected - querying Context API for historical context",
            "attemptNumber", ap.Spec.RecoveryAttemptNumber,
            "remediationRequestID", ap.Spec.RemediationRequestRef.Name)

        // Query Context API for recovery context
        recoveryCtx, err := r.enrichRecoveryContext(ctx, ap)
        if err != nil {
            // Graceful degradation: Use fallback context
            log.Warn("Context API unavailable, using fallback recovery context",
                "error", err,
                "attemptNumber", ap.Spec.RecoveryAttemptNumber)

            r.Recorder.Event(ap, "Warning", "ContextAPIUnavailable",
                fmt.Sprintf("Context API query failed: %v. Using fallback context from workflow history.", err))

            recoveryCtx = r.buildFallbackRecoveryContext(ap)
        }

        // Add recovery context to enrichment results
        ap.Status.EnrichmentResults.RecoveryContext = recoveryCtx

        log.Info("Recovery context enrichment completed",
            "contextQuality", recoveryCtx.ContextQuality,
            "previousFailuresCount", len(recoveryCtx.PreviousFailures),
            "relatedAlertsCount", len(recoveryCtx.RelatedAlerts),
            "historicalPatternsCount", len(recoveryCtx.HistoricalPatterns),
            "successfulStrategiesCount", len(recoveryCtx.SuccessfulStrategies))
    }

    ap.Status.Phase = "classifying"

    if err := r.Status().Update(ctx, ap); err != nil {
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Trigger immediate reconciliation for next phase
    return ctrl.Result{Requeue: true}, nil
}

func (r *RemediationProcessingReconciler) reconcileClassifying(ctx context.Context, ap *processingv1.RemediationProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    log.Info("Classifying environment", "namespace", ap.Spec.Alert.Namespace)

    // Perform environment classification (in-process)
    classification, err := r.Classifier.ClassifyEnvironment(ctx, ap.Spec.Alert, ap.Status.EnrichmentResults)
    if err != nil {
        log.Error(err, "Failed to classify environment")
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Update status with classification
    ap.Status.EnvironmentClassification = classification
    ap.Status.Phase = "routing"

    if err := r.Status().Update(ctx, ap); err != nil {
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Trigger immediate reconciliation for next phase
    return ctrl.Result{Requeue: true}, nil
}

func (r *RemediationProcessingReconciler) reconcileRouting(ctx context.Context, ap *processingv1.RemediationProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Create AIAnalysis CRD for next service
    aiAnalysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("ai-analysis-%s", ap.Spec.Signal.Fingerprint),
            Namespace: ap.Namespace,
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            RemediationRequestRef: ap.Spec.RemediationRequestRef,
            AnalysisRequest:     buildAnalysisRequest(ap.Status.EnrichmentResults, ap.Status.EnvironmentClassification),
        },
    }

    if err := r.Create(ctx, aiAnalysis); err != nil && !errors.IsAlreadyExists(err) {
        log.Error(err, "Failed to create AIAnalysis CRD")
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Mark as completed
    ap.Status.Phase = "completed"
    ap.Status.ProcessingTime = time.Since(ap.CreationTimestamp.Time).String()

    if err := r.Status().Update(ctx, ap); err != nil {
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    log.Info("Alert processing completed", "fingerprint", ap.Spec.Signal.Fingerprint, "duration", ap.Status.ProcessingTime)

    // Terminal state - no requeue
    return ctrl.Result{}, nil
}

// ========================================
// RECOVERY CONTEXT ENRICHMENT FUNCTIONS (Alternative 2)
// 📋 Design Decision: DD-001 | ✅ Approved Design
// See: docs/architecture/DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2
// ========================================

// enrichRecoveryContext queries Context API for historical failure context
// See: docs/services/stateless/context-api/api-specification.md (Recovery Context API)
// See: BR-WF-RECOVERY-011
func (r *RemediationProcessingReconciler) enrichRecoveryContext(
    ctx context.Context,
    ap *processingv1.RemediationProcessing,
) (*processingv1.RecoveryContext, error) {

    // Query Context API
    contextResp, err := r.ContextAPIClient.GetRemediationContext(
        ctx,
        ap.Spec.RemediationRequestRef.Name,
    )

    if err != nil {
        return nil, fmt.Errorf("Context API query failed: %w", err)
    }

    // Convert Context API response to CRD RecoveryContext format
    return convertToRecoveryContext(contextResp), nil
}

// buildFallbackRecoveryContext creates minimal recovery context when Context API unavailable
// Extracts what we know from the RemediationProcessing spec (graceful degradation)
func (r *RemediationProcessingReconciler) buildFallbackRecoveryContext(
    ap *processingv1.RemediationProcessing,
) *processingv1.RecoveryContext {

    // Build minimal context from what we know
    var previousFailures []processingv1.PreviousFailure

    if ap.Spec.FailedWorkflowRef != nil && ap.Spec.FailedStep != nil {
        previousFailures = append(previousFailures, processingv1.PreviousFailure{
            WorkflowRef:   ap.Spec.FailedWorkflowRef.Name,
            AttemptNumber: ap.Spec.RecoveryAttemptNumber - 1,
            FailedStep:    *ap.Spec.FailedStep,
            FailureReason: func() string {
                if ap.Spec.FailureReason != nil {
                    return *ap.Spec.FailureReason
                }
                return "unknown"
            }(),
            Timestamp: metav1.Now(),
        })
    }

    return &processingv1.RecoveryContext{
        ContextQuality:       "degraded", // Indicates fallback was used
        PreviousFailures:     previousFailures,
        RelatedAlerts:        []processingv1.RelatedSignal{},        // Empty - no data available
        HistoricalPatterns:   []processingv1.HistoricalPattern{},   // Empty - no data available
        SuccessfulStrategies: []processingv1.SuccessfulStrategy{}, // Empty - no data available
        RetrievedAt:          metav1.Now(),
    }
}

// convertToRecoveryContext converts Context API response to CRD RecoveryContext format
func convertToRecoveryContext(resp *ContextAPIResponse) *processingv1.RecoveryContext {
    return &processingv1.RecoveryContext{
        ContextQuality:       resp.ContextQuality,
        PreviousFailures:     convertPreviousFailures(resp.PreviousFailures),
        RelatedAlerts:        convertRelatedAlerts(resp.RelatedAlerts),
        HistoricalPatterns:   convertHistoricalPatterns(resp.HistoricalPatterns),
        SuccessfulStrategies: convertSuccessfulStrategies(resp.SuccessfulStrategies),
        RetrievedAt:          metav1.Now(),
    }
}

func convertPreviousFailures(data []PreviousFailureData) []processingv1.PreviousFailure {
    result := make([]processingv1.PreviousFailure, len(data))
    for i, f := range data {
        timestamp, _ := time.Parse(time.RFC3339, f.Timestamp)
        result[i] = processingv1.PreviousFailure{
            WorkflowRef:      f.WorkflowRef,
            AttemptNumber:    f.AttemptNumber,
            FailedStep:       f.FailedStep,
            Action:           f.Action,
            ErrorType:        f.ErrorType,
            FailureReason:    f.FailureReason,
            Duration:         f.Duration,
            Timestamp:        metav1.NewTime(timestamp),
            ClusterState:     f.ClusterState,
            ResourceSnapshot: f.ResourceSnapshot,
        }
    }
    return result
}

func convertRelatedAlerts(data []RelatedAlertData) []processingv1.RelatedSignal {
    result := make([]processingv1.RelatedSignal, len(data))
    for i, a := range data {
        timestamp, _ := time.Parse(time.RFC3339, a.Timestamp)
        result[i] = processingv1.RelatedSignal{
            AlertFingerprint: a.AlertFingerprint,
            AlertName:        a.AlertName,
            Correlation:      a.Correlation,
            Timestamp:        metav1.NewTime(timestamp),
        }
    }
    return result
}

func convertHistoricalPatterns(data []HistoricalPatternData) []processingv1.HistoricalPattern {
    result := make([]processingv1.HistoricalPattern, len(data))
    for i, p := range data {
        result[i] = processingv1.HistoricalPattern{
            Pattern:             p.Pattern,
            Occurrences:         p.Occurrences,
            SuccessRate:         p.SuccessRate,
            AverageRecoveryTime: p.AverageRecoveryTime,
        }
    }
    return result
}

func convertSuccessfulStrategies(data []SuccessfulStrategyData) []processingv1.SuccessfulStrategy {
    result := make([]processingv1.SuccessfulStrategy, len(data))
    for i, s := range data {
        lastUsed, _ := time.Parse(time.RFC3339, s.LastUsed)
        result[i] = processingv1.SuccessfulStrategy{
            Strategy:     s.Strategy,
            Description:  s.Description,
            SuccessCount: s.SuccessCount,
            LastUsed:     metav1.NewTime(lastUsed),
            Confidence:   s.Confidence,
        }
    }
    return result
}

func (r *RemediationProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&processingv1.RemediationProcessing{}).
        Complete(r)
}
```

---

