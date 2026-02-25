## Controller Implementation

### Core Reconciliation Logic

```go
package controller

import (
    "context"
    "fmt"
    "strconv"
    "strings"
    "time"

    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/tools/record"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

    remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1"
    processingv1 "github.com/jordigilh/kubernaut/api/remediationprocessing/v1"
    aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1"
    workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1"
    kubernetesexecutionv1 "github.com/jordigilh/kubernaut/api/kubernetesexecution/v1" // DEPRECATED - ADR-025
)

type RemediationRequestReconciler struct {
    client.Client
    Scheme   *runtime.Scheme
    Recorder record.EventRecorder

    NotificationClient NotificationClient
    StorageClient      StorageClient
}

func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var remediation remediationv1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Handle finalizer for 24-hour retention
    if !remediation.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
            if err := r.finalizeRemediation(ctx, &remediation); err != nil {
                return ctrl.Result{}, err
            }

            controllerutil.RemoveFinalizer(&remediation, remediationFinalizerName)
            if err := r.Update(ctx, &remediation); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(&remediation, remediationFinalizerName) {
        controllerutil.AddFinalizer(&remediation, remediationFinalizerName)
        if err := r.Update(ctx, &remediation); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Initialize if new
    if remediation.Status.OverallPhase == "" {
        remediation.Status.OverallPhase = "pending"
        remediation.Status.StartTime = metav1.Now()
        if err := r.Status().Update(ctx, &remediation); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Handle terminal states (including Skipped per DD-RO-001)
    if remediation.Status.OverallPhase == "completed" ||
       remediation.Status.OverallPhase == "failed" ||
       remediation.Status.OverallPhase == "timeout" ||
       remediation.Status.OverallPhase == "Skipped" {
        return r.handleTerminalState(ctx, &remediation)
    }

    // Orchestrate service CRDs based on phase
    return r.orchestratePhase(ctx, &remediation)
}

// Orchestrate service CRD creation based on current phase
func (r *RemediationRequestReconciler) orchestratePhase(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) (ctrl.Result, error) {

    switch remediation.Status.OverallPhase {
    case "pending":
        // Create SignalProcessing CRD
        if remediation.Status.RemediationProcessingRef == nil {
            if err := r.createRemediationProcessing(ctx, remediation); err != nil {
                return ctrl.Result{}, err
            }
            remediation.Status.OverallPhase = "processing"
            return ctrl.Result{}, r.Status().Update(ctx, remediation)
        }

    case "processing":
        // Wait for RemediationProcessing completion, then create AIAnalysis
        var remediationProcessing processingv1.RemediationProcessing
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.RemediationProcessingRef.Name,
            Namespace: remediation.Status.RemediationProcessingRef.Namespace,
        }, &remediationProcessing); err != nil {
            return ctrl.Result{}, err
        }

        // Check timeout
        if r.isPhaseTimedOut(&remediationProcessing, remediation.Status.TimeoutConfig) {
            return r.handleTimeout(ctx, remediation, "remediation_processing")
        }

        if remediationProcessing.Status.Phase == "completed" {
            if remediation.Status.AIAnalysisRef == nil {
                if err := r.createAIAnalysis(ctx, remediation, &remediationProcessing); err != nil {
                    return ctrl.Result{}, err
                }
                remediation.Status.OverallPhase = "analyzing"
                return ctrl.Result{}, r.Status().Update(ctx, remediation)
            }
        } else if remediationProcessing.Status.Phase == "failed" {
            return r.handleFailure(ctx, remediation, "remediation_processing", "Remediation processing failed")
        }

    case "analyzing":
        // Wait for AIAnalysis completion, then create WorkflowExecution
        var aiAnalysis aianalysisv1.AIAnalysis
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.AIAnalysisRef.Name,
            Namespace: remediation.Status.AIAnalysisRef.Namespace,
        }, &aiAnalysis); err != nil {
            return ctrl.Result{}, err
        }

        // Check timeout
        if r.isPhaseTimedOut(&aiAnalysis, remediation.Status.TimeoutConfig) {
            return r.handleTimeout(ctx, remediation, "ai_analysis")
        }

        if aiAnalysis.Status.Phase == "completed" {
            if remediation.Status.WorkflowExecutionRef == nil {
                if err := r.createWorkflowExecution(ctx, remediation, &aiAnalysis); err != nil {
                    return ctrl.Result{}, err
                }
                remediation.Status.OverallPhase = "executing"
                return ctrl.Result{}, r.Status().Update(ctx, remediation)
            }
        } else if aiAnalysis.Status.Phase == "failed" {
            return r.handleFailure(ctx, remediation, "ai_analysis", "AI analysis failed")
        }

    case "executing":
        // Wait for WorkflowExecution completion, then create KubernetesExecution (DEPRECATED - ADR-025)
        var workflowExecution workflowexecutionv1.WorkflowExecution
        if err := r.Get(ctx, client.ObjectKey{
            Name:      remediation.Status.WorkflowExecutionRef.Name,
            Namespace: remediation.Status.WorkflowExecutionRef.Namespace,
        }, &workflowExecution); err != nil {
            return ctrl.Result{}, err
        }

        // Check timeout
        if r.isPhaseTimedOut(&workflowExecution, remediation.Status.TimeoutConfig) {
            return r.handleTimeout(ctx, remediation, "workflow_execution")
        }

        if workflowExecution.Status.Phase == "completed" {
            if remediation.Status.KubernetesExecutionRef == nil {
                if err := r.createKubernetesExecution(ctx, remediation, &workflowExecution); err != nil { // DEPRECATED - ADR-025
                    return ctrl.Result{}, err
                }

                // Wait for KubernetesExecution (DEPRECATED - ADR-025) to complete
                var kubernetesExecution kubernetesexecutionv1.KubernetesExecution
                if err := r.Get(ctx, client.ObjectKey{
                    Name:      remediation.Status.KubernetesExecutionRef.Name,
                    Namespace: remediation.Status.KubernetesExecutionRef.Namespace,
                }, &kubernetesExecution); err != nil {
                    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
                }

                if kubernetesExecution.Status.Phase == "completed" {
                    remediation.Status.OverallPhase = "completed"
                    remediation.Status.CompletionTime = &metav1.Time{Time: time.Now()}
                    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}
                    return ctrl.Result{}, r.Status().Update(ctx, remediation)
                } else if kubernetesExecution.Status.Phase == "failed" {
                    return r.handleFailure(ctx, remediation, "kubernetes_execution", "Kubernetes execution failed")
                }
            }
        } else if workflowExecution.Status.Phase == "failed" {
            return r.handleFailure(ctx, remediation, "workflow_execution", "Workflow execution failed")
        } else if workflowExecution.Status.Phase == "Skipped" {
            // DD-RO-001: Handle resource lock deduplication (BR-ORCH-032, BR-ORCH-033, BR-ORCH-034)
            return r.handleWorkflowExecutionSkipped(ctx, remediation, &workflowExecution)
        }

    // case "recovering": [Deprecated - Issue #180] Recovery flow (DD-RECOVERY-002) removed
    }

    // Requeue to check progress
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// Handle terminal state (completed, failed, timeout, Skipped)
func (r *RemediationRequestReconciler) handleTerminalState(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // DD-RO-001 (BR-ORCH-034): Send bulk notification if this RR has duplicates
    // Only for completed/failed (not skipped RRs themselves)
    if remediation.Status.DuplicateCount > 0 &&
       (remediation.Status.OverallPhase == "completed" || remediation.Status.OverallPhase == "failed") {
        // Get WorkflowExecution for notification context
        var we workflowexecutionv1.WorkflowExecution
        if remediation.Status.WorkflowExecutionRef != nil {
            if err := r.Get(ctx, client.ObjectKey{
                Name:      remediation.Status.WorkflowExecutionRef.Name,
                Namespace: remediation.Namespace,
            }, &we); err == nil {
                if err := r.sendBulkDuplicateNotification(ctx, remediation, &we); err != nil {
                    log.Error(err, "Failed to send bulk notification", "rr", remediation.Name)
                    // Non-fatal: remediation is complete regardless
                }
            }
        }
    }

    // Check if 24-hour retention has expired
    if remediation.Status.RetentionExpiryTime != nil {
        if time.Now().After(remediation.Status.RetentionExpiryTime.Time) {
            // Delete CRD (finalizer cleanup will be triggered)
            return ctrl.Result{}, r.Delete(ctx, remediation)
        }

        // Requeue to check expiry later
        requeueAfter := time.Until(remediation.Status.RetentionExpiryTime.Time)
        return ctrl.Result{RequeueAfter: requeueAfter}, nil
    }

    return ctrl.Result{}, nil
}

// Handle timeout
func (r *RemediationRequestReconciler) handleTimeout(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    phase string,
) (ctrl.Result, error) {

    remediation.Status.OverallPhase = "timeout"
    remediation.Status.TimeoutPhase = &phase
    remediation.Status.TimeoutTime = &metav1.Time{Time: time.Now()}
    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}

    // Escalate timeout
    if err := r.escalateTimeout(ctx, remediation, phase); err != nil {
        return ctrl.Result{}, err
    }

    // Record audit
    if err := r.recordAudit(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, r.Status().Update(ctx, remediation)
}

// Handle failure
func (r *RemediationRequestReconciler) handleFailure(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    phase string,
    reason string,
) (ctrl.Result, error) {

    remediation.Status.OverallPhase = "failed"
    remediation.Status.FailurePhase = &phase
    remediation.Status.FailureReason = &reason
    remediation.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}

    // Record audit
    if err := r.recordAudit(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, r.Status().Update(ctx, remediation)
}

// ============================================================================
// DD-RO-001: Resource Lock Deduplication Handling (BR-ORCH-032, BR-ORCH-033, BR-ORCH-034)
// ============================================================================

// Handle WorkflowExecution Skipped phase due to resource lock
func (r *RemediationRequestReconciler) handleWorkflowExecutionSkipped(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    we *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Extract skip details from WE status
    skipReason := we.Status.SkipDetails.Reason
    var parentRRName string

    switch skipReason {
    case "ResourceBusy":
        parentRRName = we.Status.SkipDetails.ConflictingWorkflow.RemediationRequestRef
    case "RecentlyRemediated":
        parentRRName = we.Status.SkipDetails.RecentRemediation.RemediationRequestRef
    }

    // Update this RR as skipped duplicate (BR-ORCH-032)
    rr.Status.OverallPhase = "Skipped"
    rr.Status.SkipReason = skipReason
    rr.Status.DuplicateOf = parentRRName
    rr.Status.Message = fmt.Sprintf("Skipped: %s - see %s", skipReason, parentRRName)
    rr.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    rr.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}

    if err := r.Status().Update(ctx, rr); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to update skipped RR: %w", err)
    }

    // Track duplicate on parent RR (BR-ORCH-033)
    if err := r.trackDuplicateOnParent(ctx, parentRRName, rr.Name); err != nil {
        log.Error(err, "Failed to track duplicate on parent",
            "parent", parentRRName, "duplicate", rr.Name)
        // Non-fatal: continue even if tracking fails
    }

    log.Info("RemediationRequest skipped due to resource lock",
        "rr", rr.Name,
        "skipReason", skipReason,
        "duplicateOf", parentRRName)

    // Record audit
    if err := r.recordAudit(ctx, rr); err != nil {
        log.Error(err, "Failed to record audit for skipped RR")
    }

    r.Recorder.Event(rr, "Normal", "Skipped",
        fmt.Sprintf("Remediation skipped: %s (duplicate of %s)", skipReason, parentRRName))

    return ctrl.Result{}, nil
}

// Track duplicate on parent RemediationRequest (BR-ORCH-033)
func (r *RemediationRequestReconciler) trackDuplicateOnParent(
    ctx context.Context,
    parentRRName string,
    duplicateRRName string,
) error {
    parentRR := &remediationv1.RemediationRequest{}
    if err := r.Get(ctx, types.NamespacedName{
        Name:      parentRRName,
        Namespace: r.Namespace,
    }, parentRR); err != nil {
        return fmt.Errorf("failed to get parent RR: %w", err)
    }

    // Update duplicate tracking
    parentRR.Status.DuplicateCount++
    parentRR.Status.DuplicateRefs = append(parentRR.Status.DuplicateRefs, duplicateRRName)

    return r.Status().Update(ctx, parentRR)
}

// Send bulk notification when parent completes with duplicates (BR-ORCH-034)
// Called from handleTerminalState when parent has duplicates
func (r *RemediationRequestReconciler) sendBulkDuplicateNotification(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    we *workflowexecutionv1.WorkflowExecution,
) error {
    // Count skip reasons
    resourceBusyCount := 0
    recentlyRemediatedCount := 0

    for _, dupRef := range rr.Status.DuplicateRefs {
        dupRR := &remediationv1.RemediationRequest{}
        if err := r.Get(ctx, types.NamespacedName{
            Name:      dupRef,
            Namespace: rr.Namespace,
        }, dupRR); err != nil {
            continue // Skip if can't fetch
        }
        switch dupRR.Status.SkipReason {
        case "ResourceBusy":
            resourceBusyCount++
        case "RecentlyRemediated":
            recentlyRemediatedCount++
        }
    }

    // Build notification body
    resultEmoji := "✅ Successful"
    if rr.Status.OverallPhase == "failed" {
        resultEmoji = "❌ Failed"
    }

    body := fmt.Sprintf(`Target: %s
Result: %s
Duration: %s

Duplicates Suppressed: %d
├─ ResourceBusy: %d (signals during execution)
└─ RecentlyRemediated: %d (cooldown period)

First signal: %s
Last signal: %s`,
        buildTargetResource(rr),
        resultEmoji,
        rr.Status.CompletionTime.Sub(rr.Status.StartTime.Time),
        rr.Status.DuplicateCount,
        resourceBusyCount,
        recentlyRemediatedCount,
        rr.Spec.Deduplication.FirstOccurrence.Format(time.RFC3339),
        rr.Spec.Deduplication.LastOccurrence.Format(time.RFC3339),
    )

    // Create notification request
    notification := &notificationv1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-completion", rr.Name),
            Namespace: rr.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(rr, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: notificationv1.NotificationRequestSpec{
            EventType: "RemediationCompleted",
            Priority:  r.mapPriority(rr),
            Subject:   fmt.Sprintf("Remediation Completed: %s", we.Spec.WorkflowRef.WorkflowID),
            Body:      body,
            Metadata: map[string]string{
                "remediationRequestRef": rr.Name,
                "workflowId":            we.Spec.WorkflowRef.WorkflowID,
                "targetResource":        buildTargetResource(rr),
                "duplicateCount":        strconv.Itoa(rr.Status.DuplicateCount),
            },
        },
    }

    return r.Create(ctx, notification)
}

// Build targetResource string from RemediationRequest.spec.targetResource
func buildTargetResource(rr *remediationv1.RemediationRequest) string {
    tr := rr.Spec.TargetResource
    if tr.Namespace != "" {
        return fmt.Sprintf("%s/%s/%s", tr.Namespace, tr.Kind, tr.Name)
    }
    return fmt.Sprintf("%s/%s", tr.Kind, tr.Name)
}

// Finalizer cleanup
func (r *RemediationRequestReconciler) finalizeRemediation(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {

    // Record final audit before deletion
    return r.recordAudit(ctx, remediation)
}

const remediationFinalizerName = "kubernaut.ai/remediation-retention"
```

---

## 🔄 **Recovery Evaluation Logic** [Deprecated - Issue #180]

**Status**: Deprecated - Recovery flow (DD-RECOVERY-002) removed. See Issue #180.

---

This is the core decision logic that prevents infinite recovery loops:

```go
// evaluateRecoveryViability determines if recovery attempt is viable
// Returns (canRecover bool, reason string)
// [Deprecated - Issue #180: Recovery flow removed]
func (r *RemediationRequestReconciler) evaluateRecoveryViability(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    failedWorkflow *workflowexecutionv1.WorkflowExecution,
) (bool, string) {

    log := ctrl.LoggerFrom(ctx)

    // ========================================
    // CHECK 1: Recovery attempts limit (BR-WF-RECOVERY-001)
    // ========================================
    maxAttempts := remediation.Status.MaxRecoveryAttempts
    if maxAttempts == 0 {
        maxAttempts = 3  // Default max attempts
    }

    if remediation.Status.RecoveryAttempts >= maxAttempts {
        log.Info("Recovery attempts limit exceeded",
            "attempts", remediation.Status.RecoveryAttempts,
            "max", maxAttempts)

        r.metricsRecorder.RecordRecoveryViabilityDenied("max_attempts_exceeded")
        return false, "max_recovery_attempts_exceeded"
    }

    // ========================================
    // CHECK 2: Repeated failure pattern detection (BR-WF-RECOVERY-003)
    // ========================================
    if r.hasRepeatedFailurePattern(remediation, failedWorkflow) {
        log.Info("Repeated failure pattern detected - same strategy failing consistently")

        r.metricsRecorder.RecordRecoveryViabilityDenied("repeated_failure_pattern")
        return false, "repeated_failure_pattern"
    }

    // ========================================
    // CHECK 3: Termination rate check (BR-WF-RECOVERY-005)
    // ========================================
    terminationRate, err := r.calculateTerminationRate(ctx)
    if err != nil {
        log.Error(err, "Failed to calculate termination rate, proceeding with recovery")
    } else if terminationRate >= 0.10 {  // BR-WF-541: <10% termination rate
        log.Warn("System-wide termination rate exceeded threshold",
            "rate", terminationRate,
            "threshold", 0.10)

        r.metricsRecorder.RecordRecoveryViabilityDenied("termination_rate_exceeded")
        return false, "termination_rate_exceeded"
    }

    // All checks passed
    log.Info("Recovery viability evaluation passed",
        "recoveryAttempt", remediation.Status.RecoveryAttempts+1,
        "terminationRate", terminationRate)

    r.metricsRecorder.RecordRecoveryViabilityAllowed()
    return true, ""
}
```

### Pattern Detection Logic (BR-WF-RECOVERY-003)

```go
// hasRepeatedFailurePattern detects if the same failure signature occurs twice
func (r *RemediationRequestReconciler) hasRepeatedFailurePattern(
    remediation *remediationv1.RemediationRequest,
    failedWorkflow *workflowexecutionv1.WorkflowExecution,
) bool {

    // Create failure signature from current failure
    currentSignature := failureSignature{
        Action:    *failedWorkflow.Status.FailedAction,
        ErrorType: *failedWorkflow.Status.ErrorType,
        Step:      *failedWorkflow.Status.FailedStep,
    }

    // Count how many times this signature has occurred
    count := 0
    for _, ref := range remediation.Status.WorkflowExecutionRefs {
        if ref.Outcome == "failed" &&
           ref.FailedStep != nil &&
           ref.FailureReason != nil {

            // Check if signature matches
            if *ref.FailedStep == currentSignature.Step &&
               containsErrorType(*ref.FailureReason, currentSignature.ErrorType) {
                count++
            }
        }
    }

    // If this signature has already occurred once, that's a repeated pattern
    return count >= 1
}

type failureSignature struct {
    Action    string  // "scale-deployment"
    ErrorType string  // "timeout", "permission_denied", etc.
    Step      int     // Step number that failed
}

func containsErrorType(failureReason, errorType string) bool {
    return strings.Contains(strings.ToLower(failureReason), strings.ToLower(errorType))
}
```

### Termination Rate Calculation (BR-WF-RECOVERY-005)

```go
// calculateTerminationRate computes system-wide workflow termination rate
// Returns (rate float64, error)
func (r *RemediationRequestReconciler) calculateTerminationRate(
    ctx context.Context,
) (float64, error) {

    // Query all RemediationRequests in last 1 hour
    oneHourAgo := metav1.Time{Time: time.Now().Add(-1 * time.Hour)}

    var remediations remediationv1.RemediationRequestList
    if err := r.List(ctx, &remediations); err != nil {
        return 0, fmt.Errorf("failed to list remediation requests: %w", err)
    }

    totalWorkflows := 0
    failedWorkflows := 0

    for _, remediation := range remediations.Items {
        // Only count remediations from last hour
        if remediation.Status.StartTime.Before(&oneHourAgo) {
            continue
        }

        // Count all workflow execution attempts
        for _, ref := range remediation.Status.WorkflowExecutionRefs {
            totalWorkflows++
            if ref.Outcome == "failed" {
                failedWorkflows++
            }
        }
    }

    if totalWorkflows == 0 {
        return 0.0, nil
    }

    rate := float64(failedWorkflows) / float64(totalWorkflows)

    // Update Prometheus metric
    r.metricsRecorder.UpdateTerminationRate(rate)

    return rate, nil
}
```

### Recovery Initiation (BR-WF-RECOVERY-008, BR-WF-RECOVERY-009)

**Implementation Note**: This function now includes Context API integration (Option B). For detailed Context API client implementation, see [`OPTION_B_CONTEXT_API_INTEGRATION.md`](./OPTION_B_CONTEXT_API_INTEGRATION.md).

```go
// ========================================
// initiateRecovery - Recovery Coordination (Alternative 2)
// 📋 Design Decision: DD-001 | ✅ Approved Design | Confidence: 95%
// See: docs/architecture/DESIGN_DECISIONS.md#dd-001-recovery-context-enrichment-alternative-2
// ========================================
//
// Creates new SignalProcessing CRD for recovery attempt.
// RemediationProcessing Controller will enrich with FRESH contexts:
// - Monitoring context (current cluster state)
// - Business context (current ownership/runbooks)
// - Recovery context (historical failures from Context API)
//
// WHY Alternative 2?
// - ✅ Fresh contexts: Recovery gets CURRENT data, not stale from initial attempt
// - ✅ Temporal consistency: All contexts captured at same timestamp
// - ✅ Immutable audit trail: Each SignalProcessing CRD is complete snapshot
func (r *RemediationRequestReconciler) initiateRecovery(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    failedWorkflow *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {

    log := ctrl.LoggerFrom(ctx)

    // ========================================
    // ALTERNATIVE 2 DESIGN: Create NEW SignalProcessing CRD
    // See: docs/architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md (Version 1.2)
    // See: BR-WF-RECOVERY-011
    // ========================================

    // ========================================
    // PHASE TRANSITION: executing → recovering (BR-WF-RECOVERY-009)
    // ========================================
    remediation.Status.OverallPhase = "recovering"
    remediation.Status.RecoveryAttempts++
    remediation.Status.LastFailureTime = &metav1.Time{Time: time.Now()}

    // Create descriptive recovery reason
    reason := fmt.Sprintf("workflow_%s_step_%d",
        *failedWorkflow.Status.ErrorType,
        *failedWorkflow.Status.FailedStep)
    remediation.Status.RecoveryReason = &reason

    log.Info("Transitioning to recovering phase",
        "recoveryAttempt", remediation.Status.RecoveryAttempts,
        "maxAttempts", remediation.Status.MaxRecoveryAttempts,
        "recoveryReason", reason)

    // ========================================
    // GET ORIGINAL RemediationProcessing (to copy signal data)
    // ========================================
    var originalRP processingv1.RemediationProcessing
    if err := r.Get(ctx, client.ObjectKey{
        Name:      remediation.Status.RemediationProcessingRefs[0].Name, // First ref is always original
        Namespace: remediation.Namespace,
    }, &originalRP); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to get original RemediationProcessing: %w", err)
    }

    // ========================================
    // CREATE NEW SignalProcessing CRD FOR RECOVERY (Alternative 2)
    // ========================================
    recoveryRP := &processingv1.RemediationProcessing{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-recovery-%d", remediation.Name, remediation.Status.RecoveryAttempts),
            Namespace: remediation.Namespace,
            Labels: map[string]string{
                "remediation-request": remediation.Name,
                "type":                "recovery",
                "attempt":             strconv.Itoa(remediation.Status.RecoveryAttempts),
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: processingv1.RemediationProcessingSpec{
            // Copy signal data from original RemediationProcessing
            Alert:                     originalRP.Spec.Alert,
            EnrichmentConfig:          originalRP.Spec.EnrichmentConfig,
            EnvironmentClassification: originalRP.Spec.EnvironmentClassification,
            RemediationRequestRef:     originalRP.Spec.RemediationRequestRef,

            // RECOVERY-SPECIFIC FIELDS (Alternative 2)
            IsRecoveryAttempt:     true,
            RecoveryAttemptNumber: remediation.Status.RecoveryAttempts,
            FailedWorkflowRef:     &corev1.LocalObjectReference{Name: failedWorkflow.Name},
            FailedStep:            failedWorkflow.Status.FailedStep,
            FailureReason:         failedWorkflow.Status.FailureReason,
            OriginalProcessingRef: &corev1.LocalObjectReference{Name: originalRP.Name},
        },
    }

    if err := r.Create(ctx, recoveryRP); err != nil {
        return ctrl.Result{}, fmt.Errorf("failed to create recovery RemediationProcessing: %w", err)
    }

    log.Info("Recovery RemediationProcessing created",
        "remediationProcessing", recoveryRP.Name,
        "willEnrich", "monitoring + business + recovery context (Context API)")

    // ========================================
    // UPDATE REFS ARRAYS (BR-WF-RECOVERY-006)
    // ========================================

    // Add new RemediationProcessing reference
    remediation.Status.RemediationProcessingRefs = append(
        remediation.Status.RemediationProcessingRefs,
        remediationv1.RemediationProcessingReference{
            Name:          recoveryRP.Name,
            Namespace:     recoveryRP.Namespace,
            Type:          "recovery",
            AttemptNumber: remediation.Status.RecoveryAttempts,
            Phase:         "enriching",
            CreatedAt:     metav1.Now(),
        },
    )

    // Set current processing ref (RR will watch for its completion)
    remediation.Status.CurrentProcessingRef = &corev1.LocalObjectReference{Name: recoveryRP.Name}

    // Add failed workflow with detailed outcome
    remediation.Status.WorkflowExecutionRefs = append(
        remediation.Status.WorkflowExecutionRefs,
        remediationv1.WorkflowExecutionReferenceWithOutcome{
            Name:           failedWorkflow.Name,
            Namespace:      failedWorkflow.Namespace,
            Outcome:        "failed",
            FailedStep:     failedWorkflow.Status.FailedStep,
            FailureReason:  failedWorkflow.Status.FailureReason,
            CompletionTime: failedWorkflow.Status.CompletionTime,
            AttemptNumber:  remediation.Status.RecoveryAttempts,
        },
    )

    // Clear current workflow ref (new one will be created after AIAnalysis completes)
    remediation.Status.CurrentWorkflowExecutionRef = nil

    // Update status
    if err := r.Status().Update(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    // Emit event for visibility
    r.Recorder.Event(remediation, corev1.EventTypeNormal, "RecoveryInitiated",
        fmt.Sprintf("Recovery attempt %d/%d initiated - RemediationProcessing will enrich with fresh context",
            remediation.Status.RecoveryAttempts,
            remediation.Status.MaxRecoveryAttempts,
            *failedWorkflow.Status.FailedStep))

    // Record metrics
    r.metricsRecorder.RecordRecoveryAttempt(remediation.Status.RecoveryAttempts)

    return ctrl.Result{}, nil
}

func copyAIAnalysisRefs(refs []remediationv1.AIAnalysisReference) []corev1.LocalObjectReference {
    result := make([]corev1.LocalObjectReference, len(refs))
    for i, ref := range refs {
        result[i] = corev1.LocalObjectReference{Name: ref.Name}
    }
    return result
}
```

### Escalation to Manual Review (BR-WF-RECOVERY-004)

```go
// escalateToManualReview transitions to failed state and notifies operations team
func (r *RemediationRequestReconciler) escalateToManualReview(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    reason string,
) (ctrl.Result, error) {

    log := ctrl.LoggerFrom(ctx)

    log.Warn("Escalating to manual review",
        "reason", reason,
        "recoveryAttempts", remediation.Status.RecoveryAttempts,
        "workflowFailures", len(remediation.Status.WorkflowExecutionRefs))

    // Update status to terminal failed state
    remediation.Status.OverallPhase = "failed"
    remediation.Status.EscalatedToManualReview = true
    failurePhase := "recovery_evaluation"
    remediation.Status.FailurePhase = &failurePhase
    remediation.Status.FailureReason = &reason
    remediation.Status.CompletionTime = &metav1.Time{Time: time.Now()}
    remediation.Status.RetentionExpiryTime = &metav1.Time{Time: time.Now().Add(24 * time.Hour)}

    // Update status
    if err := r.Status().Update(ctx, remediation); err != nil {
        return ctrl.Result{}, err
    }

    // Send notification to operations team
    if err := r.NotificationClient.SendManualReviewNotification(ctx, ManualReviewNotification{
        RemediationRequestID:  remediation.Name,
        EscalationReason:      reason,
        RecoveryAttempts:      remediation.Status.RecoveryAttempts,
        LastFailureTime:       remediation.Status.LastFailureTime,
        WorkflowFailureHistory: buildFailureHistory(remediation.Status.WorkflowExecutionRefs),
        SignalContext:         buildSignalContextSummary(&remediation.Spec),
        Priority:              "high",  // Manual review is always high priority
    }); err != nil {
        log.Error(err, "Failed to send manual review notification")
        // Don't return error - escalation already recorded in status
    }

    // Emit event for visibility
    r.Recorder.Event(remediation, corev1.EventTypeWarning, "EscalatedToManualReview",
        fmt.Sprintf("Escalated to manual review: %s (attempts: %d)",
            reason, remediation.Status.RecoveryAttempts))

    // Record metrics
    r.metricsRecorder.RecordManualReviewEscalation(reason)

    // Record audit
    if err := r.recordAudit(ctx, remediation); err != nil {
        log.Error(err, "Failed to record audit for manual review escalation")
    }

    return ctrl.Result{}, nil
}

type ManualReviewNotification struct {
    RemediationRequestID   string
    EscalationReason       string
    RecoveryAttempts       int
    LastFailureTime        *metav1.Time
    WorkflowFailureHistory []WorkflowFailureSummary
    SignalContext          SignalContextSummary
    Priority               string
}

type WorkflowFailureSummary struct {
    WorkflowName   string
    AttemptNumber  int
    FailedStep     int
    FailureReason  string
    CompletionTime metav1.Time
}

type SignalContextSummary struct {
    SignalName  string
    Severity    string
    Environment string
    Namespace   string
    Resource    string
}

func buildFailureHistory(refs []remediationv1.WorkflowExecutionReferenceWithOutcome) []WorkflowFailureSummary {
    history := make([]WorkflowFailureSummary, 0, len(refs))
    for _, ref := range refs {
        if ref.Outcome == "failed" {
            history = append(history, WorkflowFailureSummary{
                WorkflowName:   ref.Name,
                AttemptNumber:  ref.AttemptNumber,
                FailedStep:     *ref.FailedStep,
                FailureReason:  *ref.FailureReason,
                CompletionTime: *ref.CompletionTime,
            })
        }
    }
    return history
}

func buildSignalContextSummary(spec *remediationv1.RemediationRequestSpec) SignalContextSummary {
    // Extract key signal context fields for notification
    return SignalContextSummary{
        SignalName:  spec.SignalName,
        Severity:    spec.Severity,
        Environment: spec.Environment,
        // Additional fields extracted from ProviderData...
    }
}
```

### Metrics Collection

```go
// MetricsRecorder interface for recovery metrics
type MetricsRecorder interface {
    RecordRecoveryViabilityAllowed()
    RecordRecoveryViabilityDenied(reason string)
    RecordRecoveryAttempt(attemptNumber int)
    RecordManualReviewEscalation(reason string)
    UpdateTerminationRate(rate float64)
}

// Prometheus metrics
var (
    recoveryViabilityEvaluations = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_recovery_viability_evaluations_total",
            Help: "Total recovery viability evaluations",
        },
        []string{"outcome", "reason"},  // outcome: allowed/denied
    )

    recoveryAttemptsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_recovery_attempts_total",
            Help: "Total recovery attempts",
        },
        []string{"attempt_number"},  // 1, 2, 3
    )

    manualReviewEscalations = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernaut_manual_review_escalations_total",
            Help: "Total escalations to manual review",
        },
        []string{"reason"},  // max_attempts_exceeded, repeated_failure_pattern, termination_rate_exceeded
    )

    workflowTerminationRate = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "kubernaut_workflow_termination_rate",
            Help: "Current workflow termination rate (failed/total in last hour)",
        },
    )
)
```

### Testing Strategy

```go
func TestRecoveryViabilityEvaluation_MaxAttemptsExceeded(t *testing.T) {
    remediation := &remediationv1.RemediationRequest{
        Status: remediationv1.RemediationRequestStatus{
            RecoveryAttempts:    3,
            MaxRecoveryAttempts: 3,
        },
    }

    reconciler := &RemediationRequestReconciler{}
    canRecover, reason := reconciler.evaluateRecoveryViability(context.Background(), remediation, nil)

    assert.False(t, canRecover)
    assert.Equal(t, "max_recovery_attempts_exceeded", reason)
}

func TestRecoveryViabilityEvaluation_RepeatedPattern(t *testing.T) {
    remediation := &remediationv1.RemediationRequest{
        Status: remediationv1.RemediationRequestStatus{
            RecoveryAttempts:    1,
            MaxRecoveryAttempts: 3,
            WorkflowExecutionRefs: []remediationv1.WorkflowExecutionReferenceWithOutcome{
                {
                    Outcome:       "failed",
                    FailedStep:    ptr.To(3),
                    FailureReason: ptr.To("timeout: operation timed out"),
                },
            },
        },
    }

    failedWorkflow := &workflowexecutionv1.WorkflowExecution{
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            FailedStep:    ptr.To(3),
            FailedAction:  ptr.To("scale-deployment"),
            ErrorType:     ptr.To("timeout"),
            FailureReason: ptr.To("timeout: operation timed out"),
        },
    }

    reconciler := &RemediationRequestReconciler{}
    canRecover, reason := reconciler.evaluateRecoveryViability(context.Background(), remediation, failedWorkflow)

    assert.False(t, canRecover)
    assert.Equal(t, "repeated_failure_pattern", reason)
}

func TestRecoveryInitiation_Success(t *testing.T) {
    remediation := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "rr-001",
            Namespace: "default",
        },
        Status: remediationv1.RemediationRequestStatus{
            OverallPhase:        "executing",
            RecoveryAttempts:    0,
            MaxRecoveryAttempts: 3,
            AIAnalysisRefs: []remediationv1.AIAnalysisReference{
                {Name: "ai-001", Namespace: "default"},
            },
        },
    }

    failedWorkflow := &workflowexecutionv1.WorkflowExecution{
        ObjectMeta: metav1.ObjectMeta{Name: "workflow-001"},
        Status: workflowexecutionv1.WorkflowExecutionStatus{
            Phase:         "failed",
            FailedStep:    ptr.To(3),
            FailedAction:  ptr.To("scale-deployment"),
            ErrorType:     ptr.To("timeout"),
            FailureReason: ptr.To("Operation timed out"),
        },
    }

    fakeClient := fake.NewClientBuilder().WithObjects(remediation).Build()
    reconciler := &RemediationRequestReconciler{Client: fakeClient}

    result, err := reconciler.initiateRecovery(context.Background(), remediation, failedWorkflow)

    assert.NoError(t, err)
    assert.Equal(t, ctrl.Result{}, result)
    assert.Equal(t, "recovering", remediation.Status.OverallPhase)
    assert.Equal(t, 1, remediation.Status.RecoveryAttempts)
    assert.Len(t, remediation.Status.AIAnalysisRefs, 2)  // Initial + recovery
    assert.Len(t, remediation.Status.WorkflowExecutionRefs, 1)
}
```

### Implementation Checklist

- [ ] Add WorkflowExecution watch in SetupWithManager
- [ ] Implement evaluateRecoveryViability with all 3 checks
- [ ] Implement hasRepeatedFailurePattern logic
- [ ] Implement calculateTerminationRate logic
- [ ] Implement initiateRecovery CRD creation
- [ ] Implement escalateToManualReview notification
- [ ] Add metrics collection for all recovery events
- [ ] Write unit tests for viability evaluation
- [ ] Write integration tests for recovery flow
- [ ] Add monitoring dashboard for recovery metrics
- [ ] Document escalation procedures for operations team
- [ ] Implement NotificationClient for manual review alerts

### Related Documentation

- **Architecture**: [`PROPOSED_FAILURE_RECOVERY_SEQUENCE.md`](../../../architecture/PROPOSED_FAILURE_RECOVERY_SEQUENCE.md)
- **Business Requirements**: BR-WF-RECOVERY-001 through 011 in `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`
- **CRD Schema**: [`crd-schema.md`](./crd-schema.md) (Recovery tracking fields)
- **WorkflowExecution**: `docs/services/crd-controllers/03-workflowexecution/controller-implementation.md` (Failure detection)
- **AIAnalysis**: `docs/services/crd-controllers/02-aianalysis/controller-implementation.md` (Context API integration)

---

## V1.0 Approval Notification Implementation (BR-ORCH-001)

**Business Requirement**: BR-ORCH-001 (RemediationOrchestrator Notification Creation)
**ADR Reference**: ADR-018 (Approval Notification V1.0 Integration)

### Watch Configuration

```go
// internal/controller/remediationorchestrator/remediationorchestrator_controller.go
package remediationorchestrator

import (
    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/handler"
    "sigs.k8s.io/controller-runtime/pkg/source"
)

func (r *RemediationRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&remediationv1alpha1.RemediationRequest{}).
        Watches(
            &source.Kind{Type: &aianalysisv1alpha1.AIAnalysis{}},
            handler.EnqueueRequestsFromMapFunc(r.findRemediationRequestsForAIAnalysis),
        ).
        Complete(r)
}

func (r *RemediationRequestReconciler) findRemediationRequestsForAIAnalysis(aiAnalysis client.Object) []ctrl.Request {
    // Find owning RemediationRequest via OwnerReferences
    for _, ownerRef := range aiAnalysis.GetOwnerReferences() {
        if ownerRef.Kind == "RemediationRequest" {
            return []ctrl.Request{{
                NamespacedName: types.NamespacedName{
                    Name:      ownerRef.Name,
                    Namespace: aiAnalysis.GetNamespace(),
                },
            }}
        }
    }
    return []ctrl.Request{}
}
```

### Reconcile Logic Extension

```go
// internal/controller/remediationorchestrator/remediationorchestrator_controller.go
func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)

    // Fetch RemediationRequest
    var remediation remediationv1alpha1.RemediationRequest
    if err := r.Get(ctx, req.NamespacedName, &remediation); err != nil {
        if errors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    // ... existing orchestration logic ...

    // Check if AIAnalysis requires approval (BR-ORCH-001)
    if remediation.Status.AIAnalysisRef != nil {
        var aiAnalysis aianalysisv1alpha1.AIAnalysis
        aiAnalysisKey := types.NamespacedName{
            Name:      remediation.Status.AIAnalysisRef.Name,
            Namespace: remediation.Status.AIAnalysisRef.Namespace,
        }

        if err := r.Get(ctx, aiAnalysisKey, &aiAnalysis); err != nil {
            if errors.IsNotFound(err) {
                log.Info("AIAnalysis not found yet", "aiAnalysisName", aiAnalysisKey.Name)
                return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
            }
            return ctrl.Result{}, err
        }

        // Approval notification triggering (V1.0)
        if aiAnalysis.Status.Phase == "Approving" && !remediation.Status.ApprovalNotificationSent {
            log.Info("AIAnalysis requires approval, creating notification",
                "aiAnalysisName", aiAnalysis.Name,
                "confidence", aiAnalysis.Status.Confidence)

            if err := r.createApprovalNotification(ctx, &remediation, &aiAnalysis); err != nil {
                log.Error(err, "Failed to create approval notification")
                r.Recorder.Event(&remediation, corev1.EventTypeWarning, "NotificationFailed",
                    fmt.Sprintf("Failed to create approval notification: %v", err))
                return ctrl.Result{}, fmt.Errorf("failed to create approval notification: %w", err)
            }

            remediation.Status.ApprovalNotificationSent = true
            if err := r.Status().Update(ctx, &remediation); err != nil {
                log.Error(err, "Failed to update RemediationRequest status")
                return ctrl.Result{}, err
            }

            log.Info("Approval notification created successfully", "aiAnalysisName", aiAnalysis.Name)
            r.Recorder.Event(&remediation, corev1.EventTypeNormal, "NotificationCreated",
                "Approval notification sent to operators")
        }
    }

    return ctrl.Result{}, nil
}
```

### createApprovalNotification Function

```go
// internal/controller/remediationorchestrator/approval_notification.go
package remediationorchestrator

import (
    "context"
    "fmt"
    "strings"

    remediationv1alpha1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
    aianalysisv1alpha1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createApprovalNotification creates NotificationRequest CRD for operator approval
// Business Requirement: BR-ORCH-001
func (r *RemediationRequestReconciler) createApprovalNotification(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
    aiAnalysis *aianalysisv1alpha1.AIAnalysis,
) error {
    notification := &notificationv1alpha1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("approval-notification-%s-%s", remediation.Name, aiAnalysis.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1alpha1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: notificationv1alpha1.NotificationRequestSpec{
            Subject:  fmt.Sprintf("🚨 Approval Required: %s", aiAnalysis.Status.ApprovalContext.Reason),
            Body:     r.formatApprovalBody(remediation, aiAnalysis),
            Priority: notificationv1alpha1.NotificationPriorityHigh,
            Channels: []notificationv1alpha1.Channel{
                notificationv1alpha1.ChannelSlack,
                notificationv1alpha1.ChannelConsole,
            },
            Metadata: map[string]string{
                "remediationRequest": remediation.Name,
                "aiAnalysis":         aiAnalysis.Name,
                "aiApprovalRequest":  aiAnalysis.Status.ApprovalRequestName,
                "confidence":         fmt.Sprintf("%.2f", aiAnalysis.Status.Confidence),
            },
        },
    }

    return r.Create(ctx, notification)
}

// formatApprovalBody formats approval context into notification body
func (r *RemediationRequestReconciler) formatApprovalBody(
    remediation *remediationv1alpha1.RemediationRequest,
    aiAnalysis *aianalysisv1alpha1.AIAnalysis,
) string {
    ctx := aiAnalysis.Status.ApprovalContext
    return fmt.Sprintf(`
Investigation Summary:
%s

Evidence:
%s

Recommended Actions:
%s

Alternatives Considered:
%s

Why Approval Required:
%s

Confidence: %.1f%% (medium)
`,
        ctx.InvestigationSummary,
        formatEvidence(ctx.EvidenceCollected),
        formatActions(ctx.RecommendedActions),
        formatAlternatives(ctx.AlternativesConsidered),
        ctx.WhyApprovalRequired,
        ctx.ConfidenceScore*100)
}

// Helper: Format evidence list
func formatEvidence(evidence []string) string {
    lines := make([]string, len(evidence))
    for i, e := range evidence {
        lines[i] = fmt.Sprintf("- %s", e)
    }
    return strings.Join(lines, "\n")
}

// Helper: Format actions list
func formatActions(actions []aianalysisv1alpha1.RecommendedAction) string {
    lines := make([]string, len(actions))
    for i, a := range actions {
        lines[i] = fmt.Sprintf("- %s: %s", a.Action, a.Rationale)
    }
    return strings.Join(lines, "\n")
}

// Helper: Format alternatives list
func formatAlternatives(alts []aianalysisv1alpha1.AlternativeApproach) string {
    lines := make([]string, len(alts))
    for i, alt := range alts {
        lines[i] = fmt.Sprintf("- %s\n  %s", alt.Approach, alt.ProsCons)
    }
    return strings.Join(lines, "\n")
}
```

### TDD Approach

**Integration Test**:
```go
// test/integration/remediationorchestrator/approval_notification_test.go
package remediationorchestrator_test

var _ = Describe("RemediationOrchestrator Approval Notification", func() {
    It("should create NotificationRequest when AIAnalysis requires approval", func() {
        // Create RemediationRequest
        remediation := createRemediationRequest()
        Expect(k8sClient.Create(ctx, remediation)).To(Succeed())

        // Create AIAnalysis with Approving phase
        aiAnalysis := createAIAnalysisWithApprovalRequired(remediation)
        Expect(k8sClient.Create(ctx, aiAnalysis)).To(Succeed())

        // Wait for NotificationRequest creation
        Eventually(func() bool {
            var notifications notificationv1alpha1.NotificationRequestList
            Expect(k8sClient.List(ctx, &notifications)).To(Succeed())
            return len(notifications.Items) > 0
        }, timeout, interval).Should(BeTrue())

        // Verify notification content
        var notification notificationv1alpha1.NotificationRequest
        notificationKey := types.NamespacedName{
            Name:      fmt.Sprintf("approval-notification-%s-%s", remediation.Name, aiAnalysis.Name),
            Namespace: remediation.Namespace,
        }
        Expect(k8sClient.Get(ctx, notificationKey, &notification)).To(Succeed())

        Expect(notification.Spec.Subject).To(ContainSubstring("Approval Required"))
        Expect(notification.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityHigh))
        Expect(notification.Spec.Channels).To(ContainElement(notificationv1alpha1.ChannelSlack))

        // Verify idempotency
        Expect(k8sClient.Get(ctx, remediation.GetObjectMeta(), remediation)).To(Succeed())
        Expect(remediation.Status.ApprovalNotificationSent).To(BeTrue())
    })
})
```

---

