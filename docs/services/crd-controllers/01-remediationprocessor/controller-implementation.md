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
    alertProcessingFinalizer = "alertprocessing.kubernaut.io/finalizer"

    // Timeout configuration
    defaultPhaseTimeout = 5 * time.Minute  // Max time per phase
)

// RemediationProcessingReconciler reconciles an RemediationProcessing object
type RemediationProcessingReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    Recorder       record.EventRecorder  // Event emission for visibility
    ContextService ContextService        // Stateless HTTP call to Context Service
    Classifier     *EnvironmentClassifier // In-process classification
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
func (r *RemediationProcessingReconciler) ensureOwnerReference(ctx context.Context, ap *alertprocessorv1.RemediationProcessing) error {
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
func (r *RemediationProcessingReconciler) determineRequeueStrategy(ap *alertprocessorv1.RemediationProcessing) ctrl.Result {
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
func (r *RemediationProcessingReconciler) reconcileDelete(ctx context.Context, ap *alertprocessorv1.RemediationProcessing) (ctrl.Result, error) {
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
    log.Info("Enriching alert with context", "fingerprint", ap.Spec.Alert.Fingerprint)

    // Call Context Service (stateless HTTP call)
    enrichmentResults, err := r.ContextService.GetContext(ctx, ap.Spec.Alert)
    if err != nil {
        log.Error(err, "Failed to get context")
        return ctrl.Result{RequeueAfter: time.Second * 30}, err
    }

    // Update status with enrichment results
    ap.Status.EnrichmentResults = enrichmentResults
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
            Name:      fmt.Sprintf("ai-analysis-%s", ap.Spec.Alert.Fingerprint),
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

    log.Info("Alert processing completed", "fingerprint", ap.Spec.Alert.Fingerprint, "duration", ap.Status.ProcessingTime)

    // Terminal state - no requeue
    return ctrl.Result{}, nil
}

func (r *RemediationProcessingReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&processingv1.RemediationProcessing{}).
        Complete(r)
}
```

---

