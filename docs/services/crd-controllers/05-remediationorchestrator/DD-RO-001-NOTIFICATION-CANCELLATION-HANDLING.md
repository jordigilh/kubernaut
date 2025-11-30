# DD-RO-001: RemediationOrchestrator Notification Cancellation Handling

**Status**: 📋 **PROPOSAL** (2025-11-28)
**Last Reviewed**: 2025-11-28
**Confidence**: 90%
**Scope**: RemediationOrchestrator behavior when NotificationRequest CRDs are deleted
**Implementation Status**: ⏸️ **PENDING** (RO not yet implemented - behavior specification for future)

---

## Context & Problem

### **Problem Statement**

When RemediationOrchestrator (RO) creates NotificationRequest CRDs as part of the remediation workflow, users may delete these CRDs for various reasons:

1. **User-initiated cancellation**: "I don't want this notification to be sent"
2. **Accidental deletion**: Operator mistake during troubleshooting
3. **Cascade deletion**: RemediationRequest deleted → ownerReference triggers NotificationRequest deletion

**Key Question**: How should RemediationOrchestrator respond when NotificationRequest is deleted?

### **Related Context**

- **DD-NOT-005**: NotificationRequest spec is immutable (users cannot update spec, only delete)
- **ADR-017**: Defines that RemediationOrchestrator creates NotificationRequest CRDs
- **BR-NOT-050**: Notification data loss prevention requirements
- **BR-RO-XXX**: User-initiated cancellation handling (to be defined)

---

## Key Requirements

### **Business Requirements** (To Be Formalized)

1. **User Intent Respect**: User deletion should be treated as intentional cancellation, not system failure
2. **Audit Trail Integrity**: Deleted NotificationRequest should leave clear audit trail in RemediationRequest status
3. **No Automatic Escalation**: User cancellation should NOT trigger automatic escalation workflows
4. **Cascade Cleanup**: Deleting RemediationRequest should automatically delete child NotificationRequest
5. **Observable State**: RO status should clearly indicate notification was cancelled vs failed

### **Technical Requirements**

1. **Owner Reference Pattern**: NotificationRequest must have ownerReference to RemediationRequest
2. **Watch-Based Detection**: RO must watch NotificationRequest CRDs and detect deletions via `NotFound` errors
3. **State Differentiation**: RO must distinguish user cancellation from cascade deletion
4. **Status Propagation**: RO must update RemediationRequest status with cancellation reason
5. **Graceful Handling**: No error logs for expected user-initiated cancellations

---

## Alternatives Considered

### Alternative 1: Mark RR as Failed (User Deletion = System Error) ❌

**Approach**: Treat NotificationRequest deletion as system failure, mark RemediationRequest as `Failed`.

```yaml
# User deletes NotificationRequest
kubectl delete notificationrequest notif-123

# RO marks RemediationRequest as Failed
status:
  phase: Failed
  reason: NotificationDeleted
  message: "NotificationRequest notif-123 was deleted"
```

**Pros**:
- ✅ Simple logic (all deletions = failures)

**Cons**:
- ❌ **User cancellation ≠ system failure**: Conflates user intent with system errors
- ❌ **Automatic escalation**: RO might trigger escalation for user-initiated cancellation
- ❌ **SRE confusion**: Failed status implies system problem requiring investigation
- ❌ **Audit trail misleading**: Suggests remediation failed, not notification cancelled

**Confidence**: 20% (rejected - incorrect semantics)

---

### Alternative 2: Recreate NotificationRequest (Fight User Intent) ❌

**Approach**: RO automatically recreates NotificationRequest when deleted.

```go
if apierrors.IsNotFound(err) {
    // Assume deletion was accidental → recreate
    log.Warn("NotificationRequest deleted, recreating")
    newNotif := createNotificationRequest(rr)
    r.Create(ctx, newNotif)
}
```

**Pros**:
- ✅ Protects against accidental deletions

**Cons**:
- ❌ **Fights user intent**: User explicitly deleted, RO recreates (control loop battle)
- ❌ **No escape hatch**: User cannot cancel notification even if desired
- ❌ **Kubernetes anti-pattern**: Controllers should respect user actions on CRDs
- ❌ **Confusing UX**: Users expect deletions to be final

**Confidence**: 10% (rejected - fights user intent)

---

### Alternative 3: Mark RR as Completed with Cancellation Condition ✅ **RECOMMENDED**

**Approach**: Treat user deletion as intentional cancellation, mark RemediationRequest as `Completed` with a cancellation condition.

```yaml
# User deletes NotificationRequest
kubectl delete notificationrequest notif-123

# RO updates RemediationRequest status
status:
  phase: Completed  # Overall workflow succeeded
  notificationStatus: Cancelled
  reason: NotificationCancelled
  message: "NotificationRequest deleted by user before delivery completed"
  conditions:
    - type: NotificationDelivered
      status: "False"
      reason: UserCancelled
      message: "NotificationRequest notif-123 deleted by user"
      lastTransitionTime: "2025-11-28T12:00:00Z"
    - type: RemediationExecuted
      status: "True"  # Remediation itself may have succeeded
      reason: ExecutionSucceeded
      message: "Workflow execution completed successfully"
```

**Pros**:
- ✅ **Correct semantics**: User cancellation ≠ system failure
- ✅ **Clear audit trail**: Condition shows exactly what happened
- ✅ **No automatic escalation**: `Completed` phase doesn't trigger retries
- ✅ **User intent respected**: Deletion is final, no recreation
- ✅ **Observable state**: SREs can query for `NotificationDelivered=False` with `reason=UserCancelled`
- ✅ **Kubernetes pattern**: Controllers respect user actions on CRDs

**Cons**:
- ⚠️ **Accidental deletions permanent**: If user deletes by mistake, cannot undo (must recreate)
  - **Mitigation**: User documentation emphasizes deletion = cancellation
- ⚠️ **New condition type**: Requires `NotificationDelivered` condition (standard pattern)

**Confidence**: 90% (recommended - correct semantics, clear audit trail)

---

### Alternative 4: Add RR Status Field `spec.cancelled: bool` (Mutable Toggle)

**Approach**: Instead of deletion, users set `spec.cancelled: true` to cancel notifications.

**Comparison with Alternative 3**:
- ❌ More complex (need to handle spec mutation for toggle field)
- ❌ Requires selective mutability for RemediationRequest spec
- ❌ Edge case: What if `cancelled=true` set after remediation completed?
- ✅ More explicit (vs. deletion implicit cancellation)

**Decision**: **Defer to V2** - Alternative 3 (deletion) is simpler for V1. If users request explicit cancellation toggles, add in V2.

**Confidence**: 60% (defer to V2 - keep V1 simple)

---

## Decision

### **APPROVED: Alternative 3 - Completed with Cancellation Condition** ⭐

**Rationale**:

1. **Semantically Correct**: User deletion = user intent, not system failure
2. **Clear Audit Trail**: Condition `NotificationDelivered=False` + reason `UserCancelled` provides perfect observability
3. **No Escalation**: `Completed` phase prevents automatic retry/escalation
4. **Kubernetes Native**: Deletion as cancellation is standard pattern (Jobs, Pods, etc.)
5. **Simple Implementation**: No need for spec toggles or recreation logic

---

## Implementation

### **1. Owner Reference Pattern**

**File**: `internal/controller/remediationorchestrator/notification_creator.go` (future)

**Pattern**:
```go
func (r *RemediationOrchestratorReconciler) createNotificationRequest(
    ctx context.Context,
    rr *remediationv1alpha1.RemediationRequest,
) (*notificationv1alpha1.NotificationRequest, error) {
    notif := &notificationv1alpha1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("notif-%s", rr.Name),
            Namespace: rr.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                {
                    APIVersion:         rr.APIVersion,
                    Kind:               rr.Kind,
                    Name:               rr.Name,
                    UID:                rr.UID,
                    Controller:         pointer.Bool(true),
                    BlockOwnerDeletion: pointer.Bool(false), // CRITICAL: Allow independent deletion
                },
            },
        },
        Spec: notificationv1alpha1.NotificationRequestSpec{
            Type:     notificationv1alpha1.NotificationTypeEscalation,
            Priority: mapPriority(rr.Spec.Severity),
            Subject:  fmt.Sprintf("Remediation Alert: %s", rr.Spec.AlertName),
            Body:     buildNotificationBody(rr),
            // ... other fields
        },
    }

    if err := r.Create(ctx, notif); err != nil {
        return nil, fmt.Errorf("failed to create NotificationRequest: %w", err)
    }

    // Store NotificationRequest name in RR status for tracking
    rr.Status.NotificationRequestName = notif.Name
    return notif, nil
}
```

**Key**: `BlockOwnerDeletion: false` allows users to delete NotificationRequest independently.

---

### **2. Watch and Detection Logic**

**File**: `internal/controller/remediationorchestrator/notification_watcher.go` (future)

**Implementation**:
```go
func (r *RemediationOrchestratorReconciler) watchNotificationRequest(
    ctx context.Context,
    rr *remediationv1alpha1.RemediationRequest,
) error {
    log := log.FromContext(ctx)

    // Get NotificationRequest by name stored in status
    notif := &notificationv1alpha1.NotificationRequest{}
    notifName := types.NamespacedName{
        Namespace: rr.Namespace,
        Name:      rr.Status.NotificationRequestName,
    }

    err := r.Get(ctx, notifName, notif)
    if err != nil {
        if apierrors.IsNotFound(err) {
            // NotificationRequest deleted
            return r.handleNotificationRequestDeletion(ctx, rr)
        }
        return fmt.Errorf("failed to get NotificationRequest: %w", err)
    }

    // NotificationRequest exists → check status
    return r.updateStatusFromNotification(ctx, rr, notif)
}

func (r *RemediationOrchestratorReconciler) handleNotificationRequestDeletion(
    ctx context.Context,
    rr *remediationv1alpha1.RemediationRequest,
) error {
    log := log.FromContext(ctx)

    // Distinguish cascade deletion from user cancellation
    if rr.DeletionTimestamp != nil {
        // Case 1: RemediationRequest being deleted → cascade deletion (expected)
        log.Info("NotificationRequest deleted as part of RemediationRequest cleanup",
            "rr", rr.Name,
            "notification", rr.Status.NotificationRequestName)
        return nil
    }

    // Case 2: User-initiated cancellation (NotificationRequest deleted independently)
    log.Info("NotificationRequest deleted by user (cancellation)",
        "rr", rr.Name,
        "notification", rr.Status.NotificationRequestName)

    // Update RemediationRequest status
    rr.Status.Phase = remediationv1alpha1.RemediationPhaseCompleted
    rr.Status.NotificationStatus = "Cancelled"
    rr.Status.Reason = "NotificationCancelled"
    rr.Status.Message = fmt.Sprintf(
        "NotificationRequest %s deleted by user before delivery completed",
        rr.Status.NotificationRequestName,
    )

    // Set condition: NotificationDelivered = False
    meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
        Type:    "NotificationDelivered",
        Status:  metav1.ConditionFalse,
        Reason:  "UserCancelled",
        Message: fmt.Sprintf("NotificationRequest %s deleted by user", rr.Status.NotificationRequestName),
    })

    // DO NOT trigger escalation (user intent respected)
    // DO NOT recreate NotificationRequest

    return r.Status().Update(ctx, rr)
}

func (r *RemediationOrchestratorReconciler) updateStatusFromNotification(
    ctx context.Context,
    rr *remediationv1alpha1.RemediationRequest,
    notif *notificationv1alpha1.NotificationRequest,
) error {
    // Update based on NotificationRequest status
    switch notif.Status.Phase {
    case notificationv1alpha1.NotificationPhaseSent:
        rr.Status.NotificationStatus = "Sent"
        meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
            Type:    "NotificationDelivered",
            Status:  metav1.ConditionTrue,
            Reason:  "DeliverySucceeded",
            Message: "Notification delivered successfully",
        })

    case notificationv1alpha1.NotificationPhaseFailed:
        rr.Status.NotificationStatus = "Failed"
        meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
            Type:    "NotificationDelivered",
            Status:  metav1.ConditionFalse,
            Reason:  "DeliveryFailed",
            Message: notif.Status.Message,
        })
        // CONSIDER: Trigger escalation workflow

    case notificationv1alpha1.NotificationPhaseSending:
        rr.Status.NotificationStatus = "InProgress"

    default:
        rr.Status.NotificationStatus = "Pending"
    }

    return r.Status().Update(ctx, rr)
}
```

---

### **3. RemediationRequest CRD Status Fields**

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`

**Add Status Fields**:
```go
type RemediationRequestStatus struct {
    // ... existing fields ...

    // Name of child NotificationRequest CRD (for tracking)
    // +optional
    NotificationRequestName string `json:"notificationRequestName,omitempty"`

    // Notification delivery status
    // Values: Pending, InProgress, Sent, Failed, Cancelled
    // +optional
    NotificationStatus string `json:"notificationStatus,omitempty"`

    // Conditions represent observations of RemediationRequest state
    // Standard conditions:
    // - RemediationExecuted: True if workflow executed successfully
    // - NotificationDelivered: True if notification sent, False if cancelled/failed
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

---

### **4. Observability and Metrics**

**Metrics to Add** (future):
```go
// RemediationOrchestrator metrics
var (
    notificationCancellationTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "ro_notification_cancellations_total",
            Help: "Total number of user-initiated notification cancellations",
        },
        []string{"namespace"},
    )

    notificationStatusGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "ro_notification_status",
            Help: "Current notification status (0=Pending, 1=Sent, 2=Failed, 3=Cancelled)",
        },
        []string{"namespace", "rr_name"},
    )
)
```

**Log Levels**:
```go
// User cancellation: INFO level (expected user action, not error)
log.Info("NotificationRequest deleted by user (cancellation)", ...)

// Cascade deletion: INFO level (expected cleanup)
log.Info("NotificationRequest deleted as part of RemediationRequest cleanup", ...)

// Delivery failure: WARN level (unexpected system error)
log.Warn("NotificationRequest delivery failed", ...)
```

---

### **5. User Documentation**

**File**: `docs/services/crd-controllers/05-remediationorchestrator/user-guide.md` (future)

```markdown
## Cancelling Notifications

### How to Cancel In-Flight Notification

If you want to prevent a notification from being sent:

```bash
# Find the NotificationRequest associated with your RemediationRequest
kubectl get notificationrequest -n kubernaut-system

# Delete the NotificationRequest
kubectl delete notificationrequest notif-rr-123 -n kubernaut-system
```

**Result**: RemediationOrchestrator will mark the RemediationRequest as `Completed` with a cancellation condition. The notification will NOT be sent.

### Check Cancellation Status

```bash
# View RemediationRequest status
kubectl get remediationrequest rr-123 -o yaml | grep -A 10 conditions

# Example output:
conditions:
  - type: NotificationDelivered
    status: "False"
    reason: UserCancelled
    message: "NotificationRequest notif-rr-123 deleted by user"
```

### Important Notes

- ⚠️ Deletion is **permanent** - you cannot undo a cancellation
- ⚠️ The remediation workflow itself continues (cancellation only affects notification)
- ✅ RemediationRequest will be marked as `Completed` (not `Failed`)
- ✅ No automatic escalation will be triggered
```

---

## Consequences

### **Positive**

- ✅ **Correct Semantics**: User cancellation distinguished from system failure
- ✅ **Clear Audit Trail**: Conditions provide perfect observability (query `NotificationDelivered=False` + `reason=UserCancelled`)
- ✅ **No Escalation Noise**: User-initiated cancellations don't trigger automatic escalations
- ✅ **Kubernetes Native**: Deletion as cancellation is standard pattern
- ✅ **Simple Implementation**: No recreation logic, no spec toggles
- ✅ **Observable State**: Metrics + logs distinguish cancellation from failure

### **Negative**

- ⚠️ **Accidental Deletion Permanent**: If user deletes by mistake, must manually recreate
  - **Mitigation**: User documentation emphasizes deletion = cancellation
  - **Mitigation**: Consider adding confirmation prompts in UI (future)
- ⚠️ **Learning Curve**: Users may expect "pause" or "disable" toggle instead of deletion
  - **Mitigation**: User guide explains deletion as cancellation mechanism

### **Neutral**

- 🔄 **New Status Field**: `notificationStatus` added to RemediationRequest status
- 🔄 **New Condition Type**: `NotificationDelivered` condition (standard K8s pattern)
- 🔄 **Owner Reference Pattern**: Standard Kubernetes ownership pattern

---

## Validation Results

### **Confidence Assessment**

- Initial assessment: 85% (analyzing alternatives)
- After DD-NOT-005 integration: 90% (immutability design aligns)
- Implementation confidence: 90% (standard Kubernetes patterns)

### **Key Validation Points**

- ✅ **DD-NOT-005 Alignment**: NotificationRequest immutability drives deletion as cancellation
- ✅ **Kubernetes Convention**: Owner references + deletion handling is standard pattern
- ✅ **Clear Semantics**: User action (deletion) → user intent (cancellation)
- ✅ **Audit Trail**: Conditions provide queryable cancellation history
- ✅ **No Escalation Noise**: Cancellations don't trigger false positive escalations

---

## Related Decisions

- **DD-NOT-005**: NotificationRequest Spec Immutability (drives deletion as cancellation)
- **ADR-001**: CRD Microservices Architecture (owner reference pattern)
- **ADR-017**: NotificationRequest CRD Creator (RO creates NotificationRequest)
- **BR-ORCH-029**: User-Initiated Notification Cancellation
- **BR-ORCH-030**: Notification Status Tracking
- **BR-ORCH-031**: Cascade Cleanup for Child NotificationRequest CRDs

---

## Business Requirements

### **Formalized Business Requirements**

The following business requirements have been formally documented in [RemediationOrchestrator Business Requirements](./BUSINESS_REQUIREMENTS.md):

**BR-ORCH-029: User-Initiated Notification Cancellation** (P0 - CRITICAL)
- Users can cancel in-flight notifications by deleting NotificationRequest CRDs
- RO marks RemediationRequest as `Completed` (not `Failed`) on user cancellation
- RO sets condition `NotificationDelivered=False` with reason `UserCancelled`
- No automatic escalation triggered for user cancellations
- Clear audit trail for user-initiated cancellations

**BR-ORCH-030: Notification Status Tracking** (P1 - HIGH)
- RO tracks NotificationRequest delivery status and propagates to RemediationRequest
- `status.notificationStatus` updated based on NotificationRequest phase
- `NotificationDelivered` condition set with accurate reason
- SREs can query RemediationRequests by notification status
- Metrics expose notification status distribution

**BR-ORCH-031: Cascade Cleanup** (P1 - HIGH)
- Owner references enable automatic cascade deletion when RemediationRequest deleted
- `blockOwnerDeletion: false` allows independent NotificationRequest deletion
- No orphaned NotificationRequest CRDs after RemediationRequest deletion
- RO distinguishes cascade deletion from user cancellation

See [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md) for complete BR specifications including acceptance criteria, test coverage, and related documentation.

---

## Review & Evolution

### **When to Revisit**

This decision should be reconsidered if:
1. ❌ Users frequently request "undo" for accidental notification deletions (>5 requests/month)
2. ❌ User feedback strongly prefers explicit cancellation toggle (e.g., `spec.cancelled: bool`)
3. ❌ Audit requirements demand preserving deleted NotificationRequest CRDs (DLP/GDPR)
4. ❌ Escalation workflows become too noisy due to cancellation handling

### **Success Metrics**

- **User Cancellations**: Track `ro_notification_cancellations_total` metric
- **Accidental Deletions**: Monitor user feedback on deletion mistakes (target: <1/month)
- **Escalation Noise**: Ensure cancellations don't trigger false positive escalations (target: 0)
- **Audit Trail Clarity**: SRE feedback on condition-based observability (target: >90% satisfaction)

---

## Implementation Timeline

### **Phase 1: RO Core Implementation** (Future - 10 days)

**Prerequisites**:
- RemediationRequest CRD complete
- RemediationOrchestrator controller scaffolding
- NotificationController operational (✅ COMPLETED)

**Tasks**:
1. Implement `createNotificationRequest()` with owner reference (2 hours)
2. Implement `watchNotificationRequest()` with deletion detection (4 hours)
3. Implement `handleNotificationRequestDeletion()` logic (3 hours)
4. Add `notificationStatus` and conditions to RemediationRequest status (2 hours)
5. Unit tests for cancellation handling (4 hours)
6. Integration tests for owner reference + watch pattern (6 hours)
7. Metrics and logging (2 hours)
8. User documentation (3 hours)

**Total Effort**: ~26 hours (~3-4 days)

---

### **Phase 2: Observability & Metrics** (Future - 2 days)

**Tasks**:
1. Prometheus metrics for cancellations
2. Grafana dashboard for notification status tracking
3. Alert rules for anomalous cancellation rates

---

### **Phase 3: V2 Enhancements** (Future - Optional)

**If user feedback requests**:
1. Add `spec.cancelled: bool` toggle to RemediationRequest (explicit cancellation)
2. Add "undo cancellation" workflow (recreate NotificationRequest)
3. Add confirmation prompts in UI for deletion operations

---

## Migration Considerations

**N/A** - RemediationOrchestrator not yet implemented. This DD captures expected behavior for future implementation.

---

## Future Pattern

This decision establishes the **cancellation-via-deletion** pattern for RemediationOrchestrator. Future child CRD relationships (e.g., WorkflowExecution, AIAnalysis) should follow similar patterns:

1. **Owner References**: Parent owns children with `blockOwnerDeletion: false`
2. **Deletion Detection**: Watch for `NotFound` errors, distinguish cascade from user action
3. **Status Propagation**: Update parent status with child cancellation conditions
4. **No Automatic Recreation**: Respect user deletions as intentional
5. **Clear Logging**: INFO level for expected cancellations, WARN for failures

---

**Prepared By**: AI Assistant (DD-RO-001: Notification Cancellation Handling)
**Date**: 2025-11-28
**User Input**: "If the notification is deleted, should RO flag RR as failed?"
**Status**: PROPOSAL - Awaiting user approval
**Confidence**: 90% (standard Kubernetes patterns, DD-NOT-005 alignment)
**Implementation Effort**: ~26 hours (when RO is implemented)
**Priority**: P1 (foundational behavior for RO, but RO implementation pending)

