# ADR-017: NotificationRequest CRD Creator Responsibility

**Status**: ✅ **APPROVED**
**Date**: 2025-10-12
**Confidence**: 95%

---

## Context & Problem

The Notification Service uses a CRD-based architecture (`NotificationRequest` CRD) instead of an imperative HTTP API. This raises a critical architectural question:

**Which component should create NotificationRequest CRDs?**

Candidates:
1. RemediationOrchestrator (RemediationRequest controller)
2. WorkflowExecution controller
3. KubernetesExecution/Executor controller (DEPRECATED - ADR-025)
4. Dedicated Notification Trigger Service

This decision impacts:
- Architectural consistency (ADR-001: CRD Microservices Architecture)
- Audit trail completeness
- Notification deduplication
- System complexity
- Retry reliability

---

## Decision

**RemediationOrchestrator (RemediationRequest reconciler) SHALL create NotificationRequest CRDs.**

**Justification**:
1. **Architectural Consistency** (⭐ CRITICAL): Follows ADR-001 centralized orchestration pattern
2. **Global Visibility**: RemediationOrchestrator sees all remediation phases (Processing, AI Analysis, Workflow, Execution)
3. **Notification Deduplication**: Central point prevents duplicate notifications for same failure
4. **Audit Trail Completeness**: Clear parent-child relationship (RemediationRequest → NotificationRequest)
5. **Simplicity**: Single place for notification creation logic (no duplicate code across controllers)

---

## Alternatives Considered

### Alternative 1: RemediationOrchestrator Creates NotificationRequest (APPROVED)

**Confidence**: **95%** ✅

**Pros**:
- ✅ Centralized orchestration (ADR-001 compliance)
- ✅ Global visibility into all phases
- ✅ Notification deduplication at source
- ✅ Complete audit trail
- ✅ Clear owner references
- ✅ Kubernetes watch pattern handles retries

**Cons**:
- ⚠️ Adds ~50 lines per notification trigger to RemediationOrchestrator
- **Mitigation**: Extract to helper functions `CreateNotificationFor(event)`

---

### Alternative 2: WorkflowExecution Creates NotificationRequest

**Confidence**: **37.5%** ❌ **REJECTED**

**Pros**:
- ✅ Proximity to workflow failure
- ✅ Can include step-specific details

**Cons**:
- ❌ **VIOLATES ADR-001**: Creates nested orchestration (3-level hierarchy)
- ❌ Limited visibility (can't see AI Analysis or Processing failures)
- ❌ Duplicate notification logic across multiple controllers
- ❌ Notification deduplication becomes complex
- ❌ Sets bad precedent (other controllers would also create notifications)

---

### Alternative 3: Executor Creates NotificationRequest

**Confidence**: **21.5%** ❌ **STRONGLY REJECTED**

**Cons**:
- ❌ **VIOLATES Leaf Controller Pattern**: 4-level CRD hierarchy
- ❌ Zero context (only knows about its own action)
- ❌ Notification spam (every failed action sends notification)
- ❌ Wrong abstraction level (users care about remediation, not individual actions)

---

### Alternative 4: Dedicated Notification Trigger Service

**Confidence**: **60%** ⚠️ **REJECTED** (over-engineering)

**Pros**:
- ✅ Separation of concerns
- ✅ Flexible rule-based triggers

**Cons**:
- ❌ **Over-engineering**: Adds new service for simple use case
- ❌ Watch overhead (must watch ALL CRDs)
- ❌ Race conditions (may process status before RemediationOrchestrator)
- ❌ Duplicate state management
- ❌ Increased operational complexity

---

## Implementation Details

### Notification Trigger Events

| Event | RemediationRequest Phase | Severity | Example |
|-------|-------------------------|----------|---------|
| **Remediation Failed** | `failed` | CRITICAL | "All retry attempts exhausted" |
| **Remediation Timeout** | `timeout` | HIGH | "AIAnalysis exceeded 10min timeout" |
| **Recovery Initiated** | `recovery` | MEDIUM | "Starting recovery workflow" |
| **Recovery Failed** | `failed` | CRITICAL | "Recovery workflow failed" |
| **Remediation Completed** | `completed` | INFO | "Successfully resolved alert" |

### Integration Point

**File**: `internal/controller/remediation/remediationrequest_controller.go`

**New Functions** (~150 lines total):
```go
// CreateNotificationForFailure creates NotificationRequest CRD on remediation failure
func (r *Reconciler) CreateNotificationForFailure(ctx context.Context, remediation *remediationv1alpha1.RemediationRequest) error

// CreateNotificationForTimeout creates NotificationRequest CRD on timeout
func (r *Reconciler) CreateNotificationForTimeout(ctx context.Context, remediation *remediationv1alpha1.RemediationRequest, timedOutPhase string) error

// CreateNotificationForCompletion creates NotificationRequest CRD on success
func (r *Reconciler) CreateNotificationForCompletion(ctx context.Context, remediation *remediationv1alpha1.RemediationRequest) error

// hasNotificationForEvent checks if notification already created (deduplication)
func (r *Reconciler) hasNotificationForEvent(remediation *remediationv1alpha1.RemediationRequest, event string) bool
```

**Reconcile Loop Integration**:
```go
switch remediation.Status.Phase {
case "failed":
    if !r.hasNotificationForEvent(remediation, "failed") {
        r.CreateNotificationForFailure(ctx, remediation)
    }
case "timeout":
    if !r.hasNotificationForEvent(remediation, "timeout") {
        r.CreateNotificationForTimeout(ctx, remediation, timedOutPhase)
    }
case "completed":
    if !r.hasNotificationForEvent(remediation, "completed") {
        r.CreateNotificationForCompletion(ctx, remediation)
    }
}
```

### Deduplication Strategy

**Add to RemediationRequest Status**:
```go
type RemediationRequestStatus struct {
    // ... existing fields ...
    NotificationsSent []string `json:"notificationsSent,omitempty"`
    // Example: ["failed-20250112120000", "completed-20250112120500"]
}
```

**Owner Reference Setup**:
```go
notification := &notificationv1alpha1.NotificationRequest{
    ObjectMeta: metav1.ObjectMeta{
        Name: fmt.Sprintf("%s-failed-%d", remediation.Name, time.Now().Unix()),
        Namespace: remediation.Namespace,
        Labels: map[string]string{
            "kubernaut.ai/remediation": remediation.Name,
            "kubernaut.ai/alert":       remediation.Spec.AlertName,
            "kubernaut.ai/event":       "failed",
        },
        OwnerReferences: []metav1.OwnerReference{
            *metav1.NewControllerRef(remediation,
                remediationv1alpha1.GroupVersion.WithKind("RemediationRequest")),
        },
    },
    // ... spec ...
}
```

---

## Consequences

### Positive

✅ **Architectural Consistency**
- Maintains ADR-001 centralized orchestration pattern
- Preserves flat CRD hierarchy
- No nested orchestration precedent

✅ **Audit Trail Completeness**
- Single source of truth: "What notifications were sent for this remediation?"
- Clear lineage: Alert → Remediation → Notification
- Owner references enable cascade deletion

✅ **Notification Deduplication**
- Central tracking prevents duplicate notifications
- Example: Workflow timeout = 1 notification (not 1 per failed action)

✅ **Retry Reliability**
- Kubernetes watch pattern handles retries automatically
- If notification creation fails, reconcile retries
- No manual retry logic needed

✅ **Simplicity**
- Single place for notification creation logic
- No duplicate code across controllers
- Easy to understand and maintain

### Negative

⚠️ **RemediationOrchestrator Complexity**
- Adds notification creation logic (~150 lines total)
- **Mitigation**: Extract to helper functions for clarity

⚠️ **Notification Delay**
- Notifications created on next reconciliation cycle (~1-2s delay)
- **Mitigation**: Acceptable for escalation use case (not real-time alerts)

### Neutral

🔄 **Tight Coupling**
- RemediationOrchestrator depends on NotificationRequest API
- **Acceptable**: NotificationRequest is stable CRD API (minimal churn)

---

## Business Requirement Alignment

All Notification BRs (BR-NOT-050 to BR-NOT-058) remain satisfied:

| BR | Description | How ADR-017 Satisfies |
|----|-------------|----------------------|
| BR-NOT-050 | Data Loss Prevention | NotificationRequest CRD persists in etcd ✅ |
| BR-NOT-051 | Complete Audit Trail | DeliveryAttempts tracked + owner references ✅ |
| BR-NOT-052 | Automatic Retry | NotificationRequest reconciler retries ✅ |
| BR-NOT-053 | At-Least-Once Delivery | Reconciliation loop guarantees ✅ |
| BR-NOT-054 | Observability | Prometheus metrics from NotificationRequest ✅ |
| BR-NOT-055 | Graceful Degradation | Per-channel failure handling ✅ |
| BR-NOT-056 | CRD Lifecycle | Phase state machine ✅ |
| BR-NOT-057 | Priority Handling | Priority field in CRD spec ✅ |
| BR-NOT-058 | Validation | CRD kubebuilder validation ✅ |

**No BR updates required** - This decision is about *who creates* the CRD, not *what* the CRD provides.

---

## Related Decisions

- **Builds On**: [ADR-001: CRD Microservices Architecture](ADR-001-crd-microservices-architecture.md) - Centralized orchestration pattern
- **Related**: [ADR-014: Notification Service External Auth](ADR-014-notification-service-external-auth.md) - Notification architecture
- **Supports**: BR-NOT-050 to BR-NOT-058 (Complete Audit Trail, Data Loss Prevention)

---

## Review & Evolution

### When to Revisit

- If RemediationOrchestrator complexity becomes unmanageable (>500 lines of notification logic)
- If notification creation latency becomes critical requirement (<100ms)
- If notification rules become highly dynamic (require external configuration)

### Success Metrics

- **Notification Deduplication Rate**: Target >95% (no duplicate notifications for same failure)
- **Notification Delivery Rate**: Target >99% (with automatic retry)
- **Audit Trail Completeness**: Target 100% (all notifications traceable to RemediationRequest)

### Implementation Timeline

- **Day 7**: Add notification creation to RemediationOrchestrator (2 hours)
- **Day 8**: Integration tests validate notification creation (1 hour)
- **Day 10**: E2E tests validate end-to-end flow (1 hour)

---

## References

- **Full Analysis**: `docs/services/crd-controllers/06-notification/implementation/NOTIFICATION_CRD_CREATOR_CONFIDENCE_ASSESSMENT.md`
- **ADR-001**: CRD Microservices Architecture (centralized orchestration)
- **RemediationOrchestrator**: `internal/controller/remediation/remediationrequest_controller.go`
- **NotificationRequest API**: `api/notification/v1alpha1/notificationrequest_types.go`

---

**Decision Made By**: Architecture Review
**Approved By**: User (2025-10-12)
**Implementation Owner**: RemediationOrchestrator Team
**Status**: ✅ **APPROVED** (95% confidence)


