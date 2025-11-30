## Controller Implementation

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.2 | 2025-11-28 | API group standardized to kubernaut.io/v1alpha1, RBAC updated | [001-crd-api-group-rationale.md](../../../architecture/decisions/001-crd-api-group-rationale.md) |
> | v1.1 | 2025-11-27 | Rename: RemediationProcessing* → SignalProcessing* | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
> | v1.1 | 2025-11-27 | Context API deprecated: Recovery context from spec.failureData | [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md) |
> | v1.1 | 2025-11-27 | Categorization phase added | [DD-CATEGORIZATION-001](../../../architecture/decisions/DD-CATEGORIZATION-001-gateway-signal-processing-split-assessment.md) |
> | v1.1 | 2025-11-27 | Data access via Data Storage Service REST API | [ADR-032](../../../architecture/decisions/ADR-032-data-access-layer-isolation.md) |
> | v1.0 | 2025-01-15 | Initial controller implementation | - |

**Location**: `internal/controller/signalprocessing/signalprocessing_controller.go`

### Controller Configuration

**Critical Patterns from [KUBERNAUT_CRD_ARCHITECTURE.md](../../../architecture/KUBERNAUT_CRD_ARCHITECTURE.md)**:
1. **Owner References**: SignalProcessing CRD owned by RemediationRequest for cascade deletion
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

    kubernautv1alpha1 "github.com/jordigilh/kubernaut/api/kubernaut.io/v1alpha1"
)

const (
    // Finalizer for cleanup coordination
    signalProcessingFinalizer = "signalprocessing.kubernaut.io/finalizer"

    // Timeout configuration
    defaultPhaseTimeout = 5 * time.Minute  // Max time per phase
)

// SignalProcessingReconciler reconciles a SignalProcessing object
// Renamed from RemediationProcessingReconciler per DD-SIGNAL-PROCESSING-001
type SignalProcessingReconciler struct {
    client.Client
    Scheme             *runtime.Scheme
    Recorder           record.EventRecorder         // Event emission for visibility
    EnrichmentService  EnrichmentService            // Stateless HTTP call for K8s context enrichment
    Classifier         *EnvironmentClassifier       // In-process classification
    Categorizer        *PriorityCategorizer         // Priority assignment (DD-CATEGORIZATION-001)
    DataStorageClient  DataStorageClient            // Audit trail via REST API (ADR-032)
}

// EnrichmentService interface for Kubernetes context enrichment
type EnrichmentService interface {
    GetContext(ctx context.Context, signal kubernautv1alpha1.Signal) (*kubernautv1alpha1.EnrichmentResults, error)
}

// DataStorageClient interface for audit trail persistence
// See: ADR-032 - Data Access Layer Isolation
type DataStorageClient interface {
    CreateAuditRecord(ctx context.Context, audit *SignalProcessingAudit) error
}

// SignalProcessingAudit for audit trail persistence
type SignalProcessingAudit struct {
    SignalFingerprint    string                                       `json:"signalFingerprint"`
    ProcessingStartTime  time.Time                                    `json:"processingStartTime"`
    ProcessingEndTime    time.Time                                    `json:"processingEndTime"`
    EnrichmentResult     *signalprocessingv1.EnrichmentResults        `json:"enrichmentResult"`
    ClassificationResult *signalprocessingv1.EnvironmentClassification `json:"classificationResult"`
    Categorization       *signalprocessingv1.Categorization           `json:"categorization"`
    DegradedMode         bool                                         `json:"degradedMode"`
    IsRecoveryAttempt    bool                                         `json:"isRecoveryAttempt"`
}

// ========================================
// ARCHITECTURAL NOTE: Context API DEPRECATED
// ========================================
//
// Per DD-CONTEXT-006, Signal Processing NO LONGER queries Context API for recovery context.
// Recovery context is now embedded by Remediation Orchestrator in spec.failureData.
//
// See: docs/architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md
// See: docs/architecture/decisions/DD-001-recovery-context-enrichment.md (updated)

// ========================================
// ARCHITECTURAL NOTE: Remediation Orchestrator Pattern
// ========================================
//
// This controller (SignalProcessing) updates ONLY its own status.
// The RemediationRequest Controller (Remediation Orchestrator) watches this CRD and aggregates status.
//
// DO NOT update RemediationRequest.status from this controller.
// The watch-based coordination pattern handles all status aggregation automatically.
//
// See: docs/services/crd-controllers/05-remediationorchestrator/overview.md

//+kubebuilder:rbac:groups=kubernaut.io,resources=signalprocessings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kubernaut.io,resources=signalprocessings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubernaut.io,resources=signalprocessings/finalizers,verbs=update
//+kubebuilder:rbac:groups=kubernaut.io,resources=remediationrequests,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *SignalProcessingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Fetch SignalProcessing CRD
    var signalProcessing signalprocessingv1.SignalProcessing
    if err := r.Get(ctx, req.NamespacedName, &signalProcessing); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle deletion with finalizer
    if !signalProcessing.DeletionTimestamp.IsZero() {
        return r.reconcileDelete(ctx, &signalProcessing)
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&signalProcessing, signalProcessingFinalizer) {
        controllerutil.AddFinalizer(&signalProcessing, signalProcessingFinalizer)
        if err := r.Update(ctx, &signalProcessing); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Set owner reference to RemediationRequest (for cascade deletion)
    if err := r.ensureOwnerReference(ctx, &signalProcessing); err != nil {
        log.Error(err, "Failed to set owner reference")
        r.Recorder.Event(&signalProcessing, "Warning", "OwnerReferenceFailed",
            fmt.Sprintf("Failed to set owner reference: %v", err))
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Check for phase timeout (5 minutes per phase default)
    if r.isPhaseTimedOut(&signalProcessing) {
        return r.handlePhaseTimeout(ctx, &signalProcessing)
    }

    // Initialize phase if empty
    if signalProcessing.Status.Phase == "" {
        signalProcessing.Status.Phase = "enriching"
        signalProcessing.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        if err := r.Status().Update(ctx, &signalProcessing); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Reconcile based on current phase
    var result ctrl.Result
    var err error

    switch signalProcessing.Status.Phase {
    case "enriching":
        result, err = r.reconcileEnriching(ctx, &signalProcessing)
    case "classifying":
        result, err = r.reconcileClassifying(ctx, &signalProcessing)
    case "categorizing":
        result, err = r.reconcileCategorizing(ctx, &signalProcessing)
    case "completed":
        // Terminal state - use optimized requeue strategy
        return r.determineRequeueStrategy(&signalProcessing), nil
    default:
        log.Error(nil, "Unknown phase", "phase", signalProcessing.Status.Phase)
        r.Recorder.Event(&signalProcessing, "Warning", "UnknownPhase",
            fmt.Sprintf("Unknown phase: %s", signalProcessing.Status.Phase))
        return ctrl.Result{RequeueAfter: time.Second * 30}, nil
    }

    // Status update triggers RemediationRequest watch (automatic coordination)
    // No need to manually update RemediationRequest - watch mechanism handles it

    return result, err
}

// ensureOwnerReference sets RemediationRequest as owner for cascade deletion
func (r *SignalProcessingReconciler) ensureOwnerReference(ctx context.Context, sp *signalprocessingv1.SignalProcessing) error {
    // Skip if owner reference already set
    if len(sp.OwnerReferences) > 0 {
        return nil
    }

    // Fetch RemediationRequest to set as owner
    var remediationRequest remediationv1.RemediationRequest
    if err := r.Get(ctx, client.ObjectKey{
        Name:      sp.Spec.RemediationRequestRef.Name,
        Namespace: sp.Spec.RemediationRequestRef.Namespace,
    }, &remediationRequest); err != nil {
        return fmt.Errorf("failed to get RemediationRequest for owner reference: %w", err)
    }

    // Set owner reference
    if err := controllerutil.SetControllerReference(&remediationRequest, sp, r.Scheme); err != nil {
        return fmt.Errorf("failed to set controller reference: %w", err)
    }

    // Update with owner reference
    if err := r.Update(ctx, sp); err != nil {
        return fmt.Errorf("failed to update with owner reference: %w", err)
    }

    return nil
}

// isPhaseTimedOut checks if current phase has exceeded timeout
func (r *SignalProcessingReconciler) isPhaseTimedOut(sp *signalprocessingv1.SignalProcessing) bool {
    if sp.Status.PhaseStartTime == nil {
        return false
    }

    // Don't timeout completed phase
    if sp.Status.Phase == "completed" {
        return false
    }

    elapsed := time.Since(sp.Status.PhaseStartTime.Time)
    return elapsed > defaultPhaseTimeout
}

// handlePhaseTimeout handles phase timeout escalation
func (r *SignalProcessingReconciler) handlePhaseTimeout(ctx context.Context, sp *signalprocessingv1.SignalProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    elapsed := time.Since(sp.Status.PhaseStartTime.Time)
    log.Error(nil, "Phase timeout exceeded",
        "phase", sp.Status.Phase,
        "elapsed", elapsed,
        "timeout", defaultPhaseTimeout)

    // Emit timeout event
    r.Recorder.Event(sp, "Warning", "PhaseTimeout",
        fmt.Sprintf("Phase %s exceeded timeout of %s (elapsed: %s)",
            sp.Status.Phase, defaultPhaseTimeout, elapsed))

    // Add timeout condition
    timeoutCondition := metav1.Condition{
        Type:    "PhaseTimeout",
        Status:  metav1.ConditionTrue,
        Reason:  "TimeoutExceeded",
        Message: fmt.Sprintf("Phase %s exceeded %s timeout", sp.Status.Phase, defaultPhaseTimeout),
        LastTransitionTime: metav1.Now(),
    }
    apimeta.SetStatusCondition(&sp.Status.Conditions, timeoutCondition)

    // Record timeout metric
    ErrorsTotal.WithLabelValues("phase_timeout", sp.Status.Phase).Inc()

    // Update status
    if err := r.Status().Update(ctx, sp); err != nil {
        return ctrl.Result{}, err
    }

    // Move to next phase or fail based on phase
    switch sp.Status.Phase {
    case "enriching":
        // Use degraded mode - continue with basic context
        sp.Status.Phase = "classifying"
        sp.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        r.Recorder.Event(sp, "Normal", "DegradedMode",
            "Enrichment timeout - continuing with basic context")
    case "classifying":
        // Use default environment classification
        sp.Status.EnvironmentClassification = r.getDefaultClassification(sp)
        sp.Status.Phase = "categorizing"
        sp.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}
        r.Recorder.Event(sp, "Normal", "DefaultClassification",
            "Classification timeout - using default environment")
    case "categorizing":
        // Use default priority
        sp.Status.Categorization = r.getDefaultCategorization(sp)
        sp.Status.Phase = "completed"
        r.Recorder.Event(sp, "Normal", "DefaultPriority",
            "Categorization timeout - using default priority")
    }

    return ctrl.Result{Requeue: true}, r.Status().Update(ctx, sp)
}

// determineRequeueStrategy provides optimized requeue based on phase
func (r *SignalProcessingReconciler) determineRequeueStrategy(sp *signalprocessingv1.SignalProcessing) ctrl.Result {
    switch sp.Status.Phase {
    case "completed":
        // Terminal state - no requeue needed (watch handles updates)
        return ctrl.Result{}
    case "enriching", "classifying", "categorizing":
        // Active phases - short requeue for progress
        return ctrl.Result{RequeueAfter: time.Second * 10}
    default:
        // Unknown state - conservative requeue
        return ctrl.Result{RequeueAfter: time.Second * 30}
    }
}

// reconcileDelete handles cleanup before deletion
func (r *SignalProcessingReconciler) reconcileDelete(ctx context.Context, sp *signalprocessingv1.SignalProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if !controllerutil.ContainsFinalizer(sp, signalProcessingFinalizer) {
        return ctrl.Result{}, nil
    }

    log.Info("Cleaning up SignalProcessing resources", "name", sp.Name)

    // Perform cleanup tasks
    // - Audit data should already be persisted via Data Storage Service
    // - No additional cleanup needed

    // Emit deletion event
    r.Recorder.Event(sp, "Normal", "Cleanup",
        "SignalProcessing cleanup completed before deletion")

    // Remove finalizer
    controllerutil.RemoveFinalizer(sp, signalProcessingFinalizer)
    if err := r.Update(ctx, sp); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil
}

// getDefaultClassification provides fallback classification on timeout
func (r *SignalProcessingReconciler) getDefaultClassification(sp *signalprocessingv1.SignalProcessing) signalprocessingv1.EnvironmentClassification {
    // Use namespace prefix or labels as fallback
    environment := "unknown"
    if strings.HasPrefix(sp.Spec.Signal.Namespace, "prod") {
        environment = "production"
    } else if strings.HasPrefix(sp.Spec.Signal.Namespace, "stag") {
        environment = "staging"
    } else if strings.HasPrefix(sp.Spec.Signal.Namespace, "dev") {
        environment = "development"
    }

    return signalprocessingv1.EnvironmentClassification{
        Environment:         environment,
        Confidence:          0.5, // Low confidence fallback
        BusinessCriticality: "medium",
        SLARequirement:      "30m",
    }
}

// getDefaultCategorization provides fallback priority on timeout
func (r *SignalProcessingReconciler) getDefaultCategorization(sp *signalprocessingv1.SignalProcessing) signalprocessingv1.Categorization {
    return signalprocessingv1.Categorization{
        Priority:             "P2",
        PriorityScore:        50,
        CategorizationSource: "default",
        CategorizationTime:   metav1.Now(),
    }
}

func (r *SignalProcessingReconciler) reconcileEnriching(ctx context.Context, sp *signalprocessingv1.SignalProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    isRecovery := sp.Spec.IsRecoveryAttempt
    if isRecovery {
        log.Info("Enriching RECOVERY attempt with context",
            "fingerprint", sp.Spec.Signal.Fingerprint,
            "attemptNumber", sp.Spec.RecoveryAttemptNumber,
            "failedWorkflow", func() string {
                if sp.Spec.FailedWorkflowRef != nil {
                    return sp.Spec.FailedWorkflowRef.Name
                }
                return "unknown"
            }())
    } else {
        log.Info("Enriching signal with context", "fingerprint", sp.Spec.Signal.Fingerprint)
    }

    // Call enrichment service for K8s context (ALWAYS - gets FRESH data)
    enrichmentResults, err := r.EnrichmentService.GetContext(ctx, sp.Spec.Signal)
    if err != nil {
        log.Error(err, "Failed to get context")
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Update status with enrichment results
    sp.Status.EnrichmentResults = *enrichmentResults

    // ========================================
    // RECOVERY CONTEXT FROM EMBEDDED DATA
    // 📋 Context API DEPRECATED per DD-CONTEXT-006
    // Recovery context is now embedded by Remediation Orchestrator in spec.failureData
    // See: docs/architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md
    // ========================================

    if isRecovery && sp.Spec.FailureData != nil {
        log.Info("Recovery attempt detected - reading embedded failure data",
            "attemptNumber", sp.Spec.RecoveryAttemptNumber,
            "failedWorkflow", sp.Spec.FailureData.WorkflowRef)

        // Build recovery context from embedded failure data
        recoveryCtx := r.buildRecoveryContextFromFailureData(sp)
        sp.Status.EnrichmentResults.RecoveryContext = recoveryCtx

        log.Info("Recovery context built from embedded failure data",
            "contextQuality", recoveryCtx.ContextQuality,
            "failedStep", sp.Spec.FailureData.FailedStep,
            "errorType", sp.Spec.FailureData.ErrorType)
    } else if isRecovery {
        // Recovery attempt but no failure data - use degraded context
        log.Warn("Recovery attempt but no failureData embedded - using degraded context",
            "attemptNumber", sp.Spec.RecoveryAttemptNumber)

        recoveryCtx := r.buildDegradedRecoveryContext(sp)
        sp.Status.EnrichmentResults.RecoveryContext = recoveryCtx
    }

    sp.Status.Phase = "classifying"
    sp.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}

    if err := r.Status().Update(ctx, sp); err != nil {
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Trigger immediate reconciliation for next phase
    return ctrl.Result{Requeue: true}, nil
}

func (r *SignalProcessingReconciler) reconcileClassifying(ctx context.Context, sp *signalprocessingv1.SignalProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    log.Info("Classifying environment", "namespace", sp.Spec.Signal.Namespace)

    // Perform environment classification (in-process)
    classification, err := r.Classifier.ClassifyEnvironment(ctx, sp.Spec.Signal, sp.Status.EnrichmentResults)
    if err != nil {
        log.Error(err, "Failed to classify environment")
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Update status with classification
    sp.Status.EnvironmentClassification = classification
    sp.Status.Phase = "categorizing"
    sp.Status.PhaseStartTime = &metav1.Time{Time: time.Now()}

    if err := r.Status().Update(ctx, sp); err != nil {
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Trigger immediate reconciliation for next phase
    return ctrl.Result{Requeue: true}, nil
}

// reconcileCategorizing assigns priority based on enriched K8s context
// Added per DD-CATEGORIZATION-001: all categorization consolidated in Signal Processing
func (r *SignalProcessingReconciler) reconcileCategorizing(ctx context.Context, sp *signalprocessingv1.SignalProcessing) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    log.Info("Categorizing priority", "namespace", sp.Spec.Signal.Namespace)

    // Perform priority categorization based on enriched context (DD-CATEGORIZATION-001)
    categorization, err := r.Categorizer.AssignPriority(ctx, sp.Spec.Signal, sp.Status.EnrichmentResults, sp.Status.EnvironmentClassification)
    if err != nil {
        log.Error(err, "Failed to categorize priority")
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Update status with categorization
    sp.Status.Categorization = categorization

    // Set routing decision
    sp.Status.RoutingDecision = signalprocessingv1.RoutingDecision{
        NextService: "ai-analysis",
        RoutingKey:  sp.Spec.Signal.Fingerprint,
        Priority:    categorization.PriorityScore / 10, // Convert to 0-10 scale
    }

    // Mark as completed
    sp.Status.Phase = "completed"
    sp.Status.ProcessingTime = time.Since(sp.CreationTimestamp.Time).String()

    if err := r.Status().Update(ctx, sp); err != nil {
        return ctrl.Result{RequeueAfter: time.Second * 15}, err
    }

    // Persist audit trail via Data Storage Service (ADR-032)
    if err := r.persistAuditTrail(ctx, sp); err != nil {
        log.Error(err, "Failed to persist audit trail")
        // Don't fail the reconciliation - audit is secondary
        r.Recorder.Event(sp, "Warning", "AuditFailed",
            fmt.Sprintf("Failed to persist audit trail: %v", err))
    }

    log.Info("Signal processing completed",
        "fingerprint", sp.Spec.Signal.Fingerprint,
        "duration", sp.Status.ProcessingTime,
        "priority", categorization.Priority)

    // Terminal state - no requeue
    return ctrl.Result{}, nil
}

// ========================================
// RECOVERY CONTEXT FUNCTIONS
// Recovery context now built from embedded spec.failureData (not Context API)
// Per DD-CONTEXT-006: Context API deprecated
// ========================================

// buildRecoveryContextFromFailureData creates recovery context from embedded failure data
func (r *SignalProcessingReconciler) buildRecoveryContextFromFailureData(
    sp *signalprocessingv1.SignalProcessing,
) *signalprocessingv1.RecoveryContext {

    failureData := sp.Spec.FailureData

    return &signalprocessingv1.RecoveryContext{
        ContextQuality: "complete",
        PreviousFailure: &signalprocessingv1.PreviousFailure{
            WorkflowRef:   failureData.WorkflowRef,
            AttemptNumber: failureData.AttemptNumber,
            FailedStep:    failureData.FailedStep,
            Action:        failureData.Action,
            ErrorType:     failureData.ErrorType,
            FailureReason: failureData.FailureReason,
            Duration:      failureData.Duration,
            Timestamp:     failureData.FailedAt,
            ResourceState: failureData.ResourceState,
        },
        ProcessedAt: metav1.Now(),
    }
}

// buildDegradedRecoveryContext creates minimal recovery context when no failure data available
func (r *SignalProcessingReconciler) buildDegradedRecoveryContext(
    sp *signalprocessingv1.SignalProcessing,
) *signalprocessingv1.RecoveryContext {

    var previousFailure *signalprocessingv1.PreviousFailure

    if sp.Spec.FailedWorkflowRef != nil && sp.Spec.FailedStep != nil {
        previousFailure = &signalprocessingv1.PreviousFailure{
            WorkflowRef:   sp.Spec.FailedWorkflowRef.Name,
            AttemptNumber: sp.Spec.RecoveryAttemptNumber - 1,
            FailedStep:    *sp.Spec.FailedStep,
            FailureReason: func() string {
                if sp.Spec.FailureReason != nil {
                    return *sp.Spec.FailureReason
                }
                return "unknown"
            }(),
            Timestamp: metav1.Now(),
        }
    }

    return &signalprocessingv1.RecoveryContext{
        ContextQuality:  "degraded", // Indicates minimal context available
        PreviousFailure: previousFailure,
        ProcessedAt:     metav1.Now(),
    }
}

// persistAuditTrail sends audit record to Data Storage Service (ADR-032)
func (r *SignalProcessingReconciler) persistAuditTrail(ctx context.Context, sp *signalprocessingv1.SignalProcessing) error {
    audit := &SignalProcessingAudit{
        SignalFingerprint:    sp.Spec.Signal.Fingerprint,
        ProcessingStartTime:  sp.CreationTimestamp.Time,
        ProcessingEndTime:    time.Now(),
        EnrichmentResult:     &sp.Status.EnrichmentResults,
        ClassificationResult: &sp.Status.EnvironmentClassification,
        Categorization:       &sp.Status.Categorization,
        DegradedMode:         sp.Status.EnrichmentResults.EnrichmentQuality < 0.8,
        IsRecoveryAttempt:    sp.Spec.IsRecoveryAttempt,
    }

    return r.DataStorageClient.CreateAuditRecord(ctx, audit)
}

func (r *SignalProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&signalprocessingv1.SignalProcessing{}).
        Complete(r)
}
```

---

## Key Changes from Previous Implementation

### Service Rename (DD-SIGNAL-PROCESSING-001)
- `RemediationProcessingReconciler` → `SignalProcessingReconciler`
- `processingv1.RemediationProcessing` → `signalprocessingv1.SignalProcessing`
- Finalizer: `alertprocessing.kubernaut.io/finalizer` → `signalprocessing.kubernaut.io/finalizer`

### Context API Deprecated (DD-CONTEXT-006)
- Removed: `ContextAPIClient` field and interface
- Removed: `enrichRecoveryContext()` function that queried Context API
- Added: `buildRecoveryContextFromFailureData()` - reads from `spec.failureData`
- Added: `buildDegradedRecoveryContext()` - fallback when no failure data

### Categorization Phase Added (DD-CATEGORIZATION-001)
- Added: `reconcileCategorizing()` phase
- Added: `Categorizer *PriorityCategorizer` field
- Phase flow: `enriching` → `classifying` → `categorizing` → `completed`

### Data Access Layer (ADR-032)
- Added: `DataStorageClient` field for audit trail
- Added: `persistAuditTrail()` - sends audit to Data Storage Service REST API
- Removed: Direct PostgreSQL access

