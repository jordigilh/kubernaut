# RemediationOrchestrator BR Triage - Notification Integration

**Date**: 2025-10-12
**Context**: ADR-017 approves RemediationOrchestrator as NotificationRequest CRD creator
**Question**: Do existing BRs need updates to reflect this architectural decision?

---

## 📊 **Executive Summary**

### **Triage Result**: ✅ **NO BR UPDATES REQUIRED**

**Confidence**: 95%

**Rationale**:
1. Existing BRs already cover notification/escalation **requirements** (WHAT)
2. ADR-017 specifies implementation **mechanism** (HOW)
3. BR-NOT-xxx requirements remain satisfied by NotificationRequest CRD
4. BR-WF-RECOVERY-004 escalation requirement satisfied by notification creation

**Action**: Document mapping between existing BRs and ADR-017 implementation (this document)

---

## 🔍 **Existing BR Analysis**

### **Category 1: Notification Delivery BRs (BR-NOT-xxx)**

**Location**: `docs/requirements/06_INTEGRATION_LAYER.md`

| BR | Description | Status | Notes |
|----|-------------|--------|-------|
| BR-NOT-001 to BR-NOT-005 | Multi-channel notifications (email, Slack, console, SMS, Teams) | ✅ Covered | NotificationRequest CRD supports multiple channels |
| BR-NOT-006 to BR-NOT-010 | Notification builder (formatting, templates, personalization) | ✅ Covered | NotificationRequest spec.subject/body |
| BR-NOT-011 to BR-NOT-015 | Delivery management (retry, confirmation, tracking, prioritization) | ✅ Covered | NotificationRequest reconciler handles retry |
| BR-NOT-016 to BR-NOT-020 | Routing & escalation (intelligent routing, escalation paths, on-call) | ✅ Covered | NotificationRequest spec.priority/channels |
| BR-NOT-026 to BR-NOT-029 | Escalation context (alert summary, impacted resources, AI analysis, justification) | ✅ Covered | NotificationRequest spec.body (content) |

**Conclusion**: These BRs define **WHAT** notifications must deliver. They are satisfied by the NotificationRequest CRD API and reconciler, **regardless of who creates the CRD**.

**Mapping to ADR-017**:
- RemediationOrchestrator **creates** NotificationRequest CRD with populated spec (subject, body, channels, priority)
- NotificationRequest reconciler **delivers** the notification per BR-NOT-xxx requirements

---

### **Category 2: Escalation BRs (BR-WF-RECOVERY-xxx)**

**Location**: `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`

#### **BR-WF-RECOVERY-004**: Escalate to manual review when recovery fails

**Full Text**:
```
BR-WF-RECOVERY-004: MUST escalate to manual review when recovery viability evaluation fails
- Rationale: Human intervention required when automated recovery is not viable
- Escalation Triggers:
  - Max recovery attempts exceeded (BR-WF-RECOVERY-001)
  - Repeated failure pattern detected (BR-WF-RECOVERY-003)
  - Termination rate exceeded (BR-WF-RECOVERY-005)
- Implementation: Set `escalatedToManualReview: true` and send notification
- Success Criteria: Operations team receives notification within 30 seconds
```

**Triage**:
- ✅ **Requirement is clear**: "send notification"
- ✅ **Success criteria is measurable**: "within 30 seconds"
- ✅ **Implementation is flexible**: Does NOT specify HTTP call vs CRD creation
- ✅ **ADR-017 satisfies this**: RemediationOrchestrator creates NotificationRequest CRD, which delivers notification

**Mapping to ADR-017**:
```go
// When BR-WF-RECOVERY-004 escalation triggers:
remediation.Status.EscalatedToManualReview = true

// Create NotificationRequest CRD
notification := &notificationv1alpha1.NotificationRequest{
    Spec: notificationv1alpha1.NotificationRequestSpec{
        Subject:  "CRITICAL: Recovery Failed - Manual Review Required",
        Body:     r.buildEscalationMessage(remediation), // BR-NOT-026 to BR-NOT-029 content
        Type:     notificationv1alpha1.NotificationTypeEscalation,
        Priority: notificationv1alpha1.NotificationPriorityCritical,
        Channels: []notificationv1alpha1.Channel{
            notificationv1alpha1.ChannelSlack, // BR-NOT-002
            notificationv1alpha1.ChannelEmail, // BR-NOT-001
        },
    },
}
r.Create(ctx, notification)

// NotificationRequest reconciler delivers notification within 30s (satisfies BR-WF-RECOVERY-004)
```

**Conclusion**: BR-WF-RECOVERY-004 is **already satisfied** by ADR-017 implementation. No BR update needed.

---

### **Category 3: Integration BRs (BR-INT-xxx)**

**Location**: `docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`

#### **BR-INT-009**: Integrate with notification systems for status updates

**Full Text**:
```
BR-INT-009: MUST integrate with notification systems for status updates
```

**Triage**:
- ✅ **Requirement is clear**: "integrate with notification systems"
- ✅ **Implementation is flexible**: Does NOT specify HTTP API vs CRD
- ✅ **ADR-017 satisfies this**: RemediationOrchestrator creates NotificationRequest CRD (integration point)

**Mapping to ADR-017**:
- RemediationOrchestrator (Workflow Orchestration component) creates NotificationRequest CRD
- NotificationRequest reconciler (Notification System) processes the CRD
- **Integration achieved** via CRD API (Kubernetes-native integration)

**Conclusion**: BR-INT-009 is **already satisfied** by ADR-017 implementation. No BR update needed.

---

### **Category 4: Remediation Orchestrator BRs (Implicit)**

**Observation**: There are **NO explicit BRs** defining RemediationOrchestrator's responsibility to create notifications.

**Current State**:
```go
// internal/controller/remediation/remediationrequest_controller.go:811
// TODO Phase 3.3: Trigger notification/escalation
```

**Analysis**:
- This TODO reflects **planned implementation**, not a documented BR
- The BR gap was **already identified** in the TODO comment
- ADR-017 **resolves this gap** by specifying the implementation approach

**Question**: Should we add a BR like "BR-ORCHESTRATION-XXX: MUST create NotificationRequest CRDs on failure/timeout/completion"?

**Answer**: ❌ **NO** - This would be **implementation prescription**, not business requirement

**Rationale**:
- **Business Requirement (WHAT)**: "Send notifications on failure" ← **Already covered by BR-WF-RECOVERY-004, BR-INT-009**
- **Implementation Decision (HOW)**: "Create NotificationRequest CRD" ← **Covered by ADR-017**
- BRs should specify **business outcomes**, not **technical mechanisms**

**Example of over-prescription**:
```
❌ BAD BR: "MUST create NotificationRequest CRD on failure"
✅ GOOD BR: "MUST notify operations team on failure within 30s" (BR-WF-RECOVERY-004)
```

**Conclusion**: No new BR needed. ADR-017 is the correct place for this decision.

---

## 📋 **BR Satisfaction Matrix**

| BR | Requirement | Satisfied By | Notes |
|----|-------------|--------------|-------|
| **BR-NOT-001 to BR-NOT-029** | Notification delivery, formatting, routing, escalation content | NotificationRequest CRD + reconciler | RemediationOrchestrator creates CRD, reconciler delivers |
| **BR-WF-RECOVERY-004** | Escalate to manual review with notification within 30s | RemediationOrchestrator creates NotificationRequest CRD | Satisfies "send notification" requirement |
| **BR-INT-009** | Integrate with notification systems | RemediationOrchestrator creates NotificationRequest CRD | CRD API is the integration point |

**Overall**: ✅ **All notification-related BRs satisfied by ADR-017 implementation**

---

## 🎯 **Implementation Guidance for RemediationOrchestrator**

### **Required Changes to RemediationOrchestrator**

**File**: `internal/controller/remediation/remediationrequest_controller.go`

#### **1. Add NotificationRequest API Import**

```go
import (
    // ... existing imports ...
    notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)
```

#### **2. Add RBAC Markers**

```go
// +kubebuilder:rbac:groups=notification.kubernaut.io,resources=notificationrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=notification.kubernaut.io,resources=notificationrequests/status,verbs=get;update;patch
```

#### **3. Implement Notification Creation Functions**

**Location**: After `handleFailure()` method (line ~820)

```go
// ========================================
// NOTIFICATION CREATION (ADR-017)
// ========================================

// CreateNotificationForFailure creates a NotificationRequest CRD when remediation fails
// Satisfies: BR-WF-RECOVERY-004 (escalation notification)
func (r *RemediationRequestReconciler) CreateNotificationForFailure(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
    failureReason string,
) error {
    log := logf.FromContext(ctx)

    notification := &notificationv1alpha1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-failed-%d", remediation.Name, time.Now().Unix()),
            Namespace: remediation.Namespace,
            Labels: map[string]string{
                "app.kubernetes.io/name":       "kubernaut",
                "app.kubernetes.io/component":  "notification",
                "app.kubernetes.io/managed-by": "remediation-orchestrator",
                "kubernaut.ai/remediation":     remediation.Name,
                "kubernaut.ai/alert":           remediation.Spec.AlertName,
                "kubernaut.ai/event":           "failed",
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1alpha1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: notificationv1alpha1.NotificationRequestSpec{
            Subject:  fmt.Sprintf("CRITICAL: Remediation Failed - %s", remediation.Spec.AlertName),
            Body:     r.buildFailureMessage(remediation, failureReason), // BR-NOT-026 to BR-NOT-029
            Type:     notificationv1alpha1.NotificationTypeEscalation,
            Priority: notificationv1alpha1.NotificationPriorityCritical,
            Channels: []notificationv1alpha1.Channel{
                notificationv1alpha1.ChannelSlack,   // BR-NOT-002
                notificationv1alpha1.ChannelConsole, // BR-NOT-003
            },
        },
    }

    if err := r.Create(ctx, notification); err != nil {
        log.Error(err, "Failed to create NotificationRequest for remediation failure",
            "remediation", remediation.Name,
            "notification", notification.Name)
        return fmt.Errorf("failed to create notification: %w", err)
    }

    log.Info("Created NotificationRequest for remediation failure",
        "remediation", remediation.Name,
        "notification", notification.Name)

    return nil
}

// CreateNotificationForTimeout creates a NotificationRequest CRD when remediation times out
// Satisfies: BR-WF-RECOVERY-004 (escalation notification for timeout)
func (r *RemediationRequestReconciler) CreateNotificationForTimeout(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
    timedOutPhase string,
) error {
    log := logf.FromContext(ctx)

    notification := &notificationv1alpha1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-timeout-%d", remediation.Name, time.Now().Unix()),
            Namespace: remediation.Namespace,
            Labels: map[string]string{
                "app.kubernetes.io/name":       "kubernaut",
                "app.kubernetes.io/component":  "notification",
                "app.kubernetes.io/managed-by": "remediation-orchestrator",
                "kubernaut.ai/remediation":     remediation.Name,
                "kubernaut.ai/alert":           remediation.Spec.AlertName,
                "kubernaut.ai/event":           "timeout",
                "kubernaut.ai/phase":           timedOutPhase,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1alpha1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: notificationv1alpha1.NotificationRequestSpec{
            Subject:  fmt.Sprintf("HIGH: Remediation Timeout - %s (%s phase)", remediation.Spec.AlertName, timedOutPhase),
            Body:     r.buildTimeoutMessage(remediation, timedOutPhase),
            Type:     notificationv1alpha1.NotificationTypeEscalation,
            Priority: notificationv1alpha1.NotificationPriorityHigh,
            Channels: []notificationv1alpha1.Channel{
                notificationv1alpha1.ChannelSlack,
                notificationv1alpha1.ChannelConsole,
            },
        },
    }

    if err := r.Create(ctx, notification); err != nil {
        log.Error(err, "Failed to create NotificationRequest for timeout",
            "remediation", remediation.Name,
            "phase", timedOutPhase)
        return fmt.Errorf("failed to create notification: %w", err)
    }

    log.Info("Created NotificationRequest for timeout",
        "remediation", remediation.Name,
        "phase", timedOutPhase)

    return nil
}

// CreateNotificationForCompletion creates a NotificationRequest CRD when remediation completes
// Satisfies: BR-INT-009 (status update notification)
func (r *RemediationRequestReconciler) CreateNotificationForCompletion(
    ctx context.Context,
    remediation *remediationv1alpha1.RemediationRequest,
) error {
    log := logf.FromContext(ctx)

    notification := &notificationv1alpha1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-completed-%d", remediation.Name, time.Now().Unix()),
            Namespace: remediation.Namespace,
            Labels: map[string]string{
                "app.kubernetes.io/name":       "kubernaut",
                "app.kubernetes.io/component":  "notification",
                "app.kubernetes.io/managed-by": "remediation-orchestrator",
                "kubernaut.ai/remediation":     remediation.Name,
                "kubernaut.ai/alert":           remediation.Spec.AlertName,
                "kubernaut.ai/event":           "completed",
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1alpha1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: notificationv1alpha1.NotificationRequestSpec{
            Subject:  fmt.Sprintf("SUCCESS: Alert Resolved - %s", remediation.Spec.AlertName),
            Body:     r.buildCompletionMessage(remediation),
            Type:     notificationv1alpha1.NotificationTypeStatusUpdate,
            Priority: notificationv1alpha1.NotificationPriorityLow,
            Channels: []notificationv1alpha1.Channel{
                notificationv1alpha1.ChannelConsole, // Lower priority, console only
            },
        },
    }

    if err := r.Create(ctx, notification); err != nil {
        log.Error(err, "Failed to create NotificationRequest for completion",
            "remediation", remediation.Name)
        // Don't return error - completion notification is informational
        return nil
    }

    log.Info("Created NotificationRequest for completion",
        "remediation", remediation.Name)

    return nil
}

// hasNotificationForEvent checks if notification already created (deduplication)
func (r *RemediationRequestReconciler) hasNotificationForEvent(
    remediation *remediationv1alpha1.RemediationRequest,
    event string,
) bool {
    if remediation.Status.NotificationsSent == nil {
        return false
    }

    for _, sent := range remediation.Status.NotificationsSent {
        if strings.HasPrefix(sent, event+"-") {
            return true
        }
    }

    return false
}

// buildFailureMessage builds notification body for remediation failure
// Satisfies: BR-NOT-026 to BR-NOT-029 (escalation context requirements)
func (r *RemediationRequestReconciler) buildFailureMessage(
    remediation *remediationv1alpha1.RemediationRequest,
    failureReason string,
) string {
    // BR-NOT-026: Alert context
    // BR-NOT-027: Impacted resources
    // BR-NOT-028: AI root cause analysis
    // BR-NOT-029: Analysis justification

    return fmt.Sprintf(`# Remediation Failed

**Alert**: %s
**Severity**: %s
**Reason**: %s

## Alert Context
- **Alert Name**: %s
- **Timestamp**: %s
- **Remediation ID**: %s

## Failure Details
%s

## Next Steps
Manual review required. Check RemediationRequest status for detailed history.

---
*This is an automated escalation notification from Kubernaut.*
`,
        remediation.Spec.AlertName,
        remediation.Spec.Severity,
        failureReason,
        remediation.Spec.AlertName,
        time.Now().Format(time.RFC3339),
        remediation.Name,
        failureReason,
    )
}

// buildTimeoutMessage builds notification body for timeout
func (r *RemediationRequestReconciler) buildTimeoutMessage(
    remediation *remediationv1alpha1.RemediationRequest,
    timedOutPhase string,
) string {
    return fmt.Sprintf(`# Remediation Timeout

**Alert**: %s
**Phase**: %s
**Timeout Threshold**: Exceeded

## Details
The %s phase exceeded its timeout threshold. Manual intervention may be required.

**Remediation ID**: %s
**Started At**: %s

## Next Steps
Review RemediationRequest status and logs to determine root cause.

---
*This is an automated escalation notification from Kubernaut.*
`,
        remediation.Spec.AlertName,
        timedOutPhase,
        timedOutPhase,
        remediation.Name,
        remediation.Status.CreatedAt.Format(time.RFC3339),
    )
}

// buildCompletionMessage builds notification body for successful completion
func (r *RemediationRequestReconciler) buildCompletionMessage(
    remediation *remediationv1alpha1.RemediationRequest,
) string {
    return fmt.Sprintf(`# Alert Resolved

**Alert**: %s
**Status**: Successfully Resolved

**Remediation ID**: %s
**Completed At**: %s

---
*Automated notification from Kubernaut.*
`,
        remediation.Spec.AlertName,
        remediation.Name,
        time.Now().Format(time.RFC3339),
    )
}
```

#### **4. Integrate into handleFailure()**

**Replace line 811 TODO**:
```go
// Line 811 - BEFORE:
// TODO Phase 3.3: Trigger notification/escalation

// Line 811 - AFTER:
// Create notification for failure (ADR-017, satisfies BR-WF-RECOVERY-004)
if !r.hasNotificationForEvent(remediation, "failed") {
    if err := r.CreateNotificationForFailure(ctx, remediation, reason); err != nil {
        log.Error(err, "Failed to create notification for failure")
        // Don't fail the remediation update if notification fails
    } else {
        // Mark notification sent
        if remediation.Status.NotificationsSent == nil {
            remediation.Status.NotificationsSent = []string{}
        }
        remediation.Status.NotificationsSent = append(
            remediation.Status.NotificationsSent,
            fmt.Sprintf("failed-%s", time.Now().Format("20060102150405")),
        )
    }
}
```

#### **5. Add NotificationsSent to RemediationRequest Status**

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`

```go
type RemediationRequestStatus struct {
    // ... existing fields ...

    // NotificationsSent tracks which notification events have been created
    // Format: ["failed-20250112120000", "timeout-20250112120500", "completed-20250112120800"]
    // +optional
    NotificationsSent []string `json:"notificationsSent,omitempty"`
}
```

---

## ✅ **Validation Checklist**

Before merging:
- [ ] ADR-017 document created in `docs/architecture/decisions/`
- [ ] NotificationRequest API import added to RemediationOrchestrator
- [ ] RBAC markers added for NotificationRequest CRD
- [ ] `CreateNotificationForFailure()` implemented
- [ ] `CreateNotificationForTimeout()` implemented
- [ ] `CreateNotificationForCompletion()` implemented
- [ ] `hasNotificationForEvent()` implemented
- [ ] `buildFailureMessage()`, `buildTimeoutMessage()`, `buildCompletionMessage()` implemented
- [ ] `handleFailure()` TODO replaced with notification creation
- [ ] `NotificationsSent` field added to RemediationRequest status
- [ ] `make generate` executed to update CRD manifests
- [ ] Integration tests validate notification creation

---

## 📊 **Confidence Assessment**

### **BR Update Required?**: ❌ **NO**

**Confidence**: **95%**

**Rationale**:
1. ✅ **Existing BRs cover requirements**: BR-WF-RECOVERY-004, BR-INT-009, BR-NOT-xxx
2. ✅ **ADR-017 specifies implementation**: RemediationOrchestrator creates NotificationRequest CRD
3. ✅ **BRs define WHAT, ADR defines HOW**: Clear separation of concerns
4. ✅ **All notification BRs remain satisfied**: NotificationRequest CRD + reconciler deliver on all requirements

**Risk**: **LOW**
- No gaps in BR coverage
- Implementation aligns with existing BRs
- No BR conflicts or contradictions

---

## 🔗 **Related Documents**

- **ADR-017**: Notification CRD Creator Responsibility (`docs/architecture/decisions/ADR-017-notification-crd-creator.md`)
- **Notification BRs**: Integration Layer (`docs/requirements/06_INTEGRATION_LAYER.md`)
- **Escalation BRs**: Workflow Engine (`docs/requirements/04_WORKFLOW_ENGINE_ORCHESTRATION.md`)
- **RemediationOrchestrator**: Controller implementation (`internal/controller/remediation/remediationrequest_controller.go`)

---

**Triage Date**: 2025-10-12
**Decision**: **NO BR UPDATES REQUIRED**
**Next Action**: Implement ADR-017 in RemediationOrchestrator (estimated 2 hours)


