# RemediationRequest CRD Schema Extension - BR-ORCH-029/030/031/034

**Version**: 1.4
**Date**: December 13, 2025
**Status**: 📋 **PROPOSED**
**Business Requirements**:
- [BR-ORCH-029](../../../requirements/BR-ORCH-029-031-notification-handling.md) (P0): User-Initiated Notification Cancellation
- [BR-ORCH-030](../../../requirements/BR-ORCH-029-031-notification-handling.md) (P1): Notification Status Tracking
- [BR-ORCH-031](../../../requirements/BR-ORCH-029-031-notification-handling.md) (P1): Cascade Cleanup for Child NotificationRequest CRDs
- [BR-ORCH-034](../../../requirements/BR-ORCH-032-034-resource-lock-deduplication.md) (P1): Bulk Notification for Duplicates

**Design Decision**: [DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md](DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md)
**Alternatives Triage**: [BR-ORCH-029-030-031-034_ALTERNATIVES_TRIAGE.md](implementation/BR-ORCH-029-030-031-034_ALTERNATIVES_TRIAGE.md) (95% confidence)

---

## 📋 Executive Summary

This document extends the RemediationRequest CRD schema to support notification lifecycle management:
1. **Tracking**: Store NotificationRequest references and delivery status
2. **Cancellation**: Detect and handle user-initiated notification cancellation
3. **Observability**: Provide queryable conditions for notification outcomes
4. **Bulk Notifications**: Support consolidated notifications for duplicate remediations

**Key Principle**: User deletion of NotificationRequest = intentional cancellation (not failure)

---

## 🔄 Schema Changes

### **New Status Fields**

Add to `RemediationRequestStatus` in `api/remediation/v1alpha1/remediationrequest_types.go`:

```go
// ========================================
// NOTIFICATION LIFECYCLE TRACKING (BR-ORCH-029/030/031)
// ========================================

// NotificationStatus tracks the delivery status of notification(s) for this remediation.
// Values: "Pending", "InProgress", "Sent", "Failed", "Cancelled"
//
// Status Mapping from NotificationRequest.Status.Phase:
// - NotificationRequest Pending → "Pending"
// - NotificationRequest Sending → "InProgress"
// - NotificationRequest Sent → "Sent"
// - NotificationRequest Failed → "Failed"
// - NotificationRequest deleted by user → "Cancelled"
//
// For bulk notifications (BR-ORCH-034), this reflects the status of the consolidated notification.
//
// Reference: BR-ORCH-030 (notification status tracking)
// +optional
// +kubebuilder:validation:Enum=Pending;InProgress;Sent;Failed;Cancelled
NotificationStatus string `json:"notificationStatus,omitempty"`

// Conditions represent observations of RemediationRequest state.
// Standard condition types:
// - "Ready": Aggregate - True on Completed/Skipped, False on Failed/TimedOut/Cancelled
// - "NotificationDelivered": True if notification sent successfully, False if cancelled/failed
//   - Reason "DeliverySucceeded": Notification sent
//   - Reason "UserCancelled": User deleted NotificationRequest before delivery
//   - Reason "DeliveryFailed": NotificationRequest failed to deliver
//
// Use constants from pkg/remediationrequest/conditions.go.
// Conditions follow Kubernetes API conventions (KEP-1623).
// Reference: BR-ORCH-029 (user cancellation), BR-ORCH-030 (status tracking)
// +optional
// +patchMergeKey=type
// +patchStrategy=merge
// +listType=map
// +listMapKey=type
Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
```

**Note**: `NotificationRequestRefs []corev1.ObjectReference` already exists in the schema (added in v1.1 for BR-ORCH-035).

---

## 📊 Schema Version History

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| **v1.4** | 2025-12-13 | **NOTIFICATION LIFECYCLE**: Added `NotificationStatus` enum field, `Conditions` array for notification delivery tracking | BR-ORCH-029/030/031 |
| v1.3 | 2025-12-06 | Added phase start time fields (`ProcessingStartTime`, `AnalyzingStartTime`, `ExecutingStartTime`) for per-phase timeout detection | BR-ORCH-027/028 |
| v1.2 | 2025-12-06 | Added `SignalProcessingRef`, `RequiresManualReview` for manual intervention scenarios | BR-ORCH-032, BR-ORCH-036 |
| v1.1 | 2025-12-06 | Added `NotificationRequestRefs` for audit trail and compliance tracking | BR-ORCH-035 |
| v1.0 | 2025-12-04 | Initial CRD schema with recovery support | - |

---

## 🎯 Field Usage Patterns

### **NotificationStatus Field**

#### **Purpose**
Provides high-level visibility into notification delivery without requiring users to query child NotificationRequest CRDs.

#### **Update Triggers**
RO updates `notificationStatus` when:
1. **NotificationRequest Created**: Set to `"Pending"`
2. **NotificationRequest Phase Changes**: Map phase to status
3. **NotificationRequest Deleted by User**: Set to `"Cancelled"`
4. **NotificationRequest Delivery Fails**: Set to `"Failed"`

#### **Status Mapping**

| NotificationRequest Phase | RemediationRequest.Status.NotificationStatus |
|---------------------------|---------------------------------------------|
| `Pending` | `"Pending"` |
| `Sending` | `"InProgress"` |
| `Sent` | `"Sent"` |
| `Failed` | `"Failed"` |
| Deleted by user (not cascade) | `"Cancelled"` |

#### **Example Usage**

```yaml
# Query all remediations with cancelled notifications
kubectl get remediationrequest -A -o json | \
  jq '.items[] | select(.status.notificationStatus == "Cancelled")'

# Query all remediations with failed notifications
kubectl get remediationrequest -A -o json | \
  jq '.items[] | select(.status.notificationStatus == "Failed")'
```

---

### **Conditions Field**

#### **Purpose**
Provides detailed, queryable audit trail for notification outcomes following Kubernetes standard condition patterns (KEP-1623).

#### **Standard Condition Types**

##### **1. NotificationDelivered Condition**

Tracks whether notification was successfully delivered.

**Possible States**:

| Status | Reason | Message Example | Scenario |
|--------|--------|-----------------|----------|
| `True` | `DeliverySucceeded` | "Notification delivered successfully" | NotificationRequest phase = `Sent` |
| `False` | `UserCancelled` | "NotificationRequest notif-123 deleted by user" | User deleted NotificationRequest (BR-ORCH-029) |
| `False` | `DeliveryFailed` | "Notification delivery failed: SMTP timeout" | NotificationRequest phase = `Failed` |

**Example**:
```yaml
status:
  conditions:
    - type: NotificationDelivered
      status: "False"
      reason: UserCancelled
      message: "NotificationRequest notif-rr-123 deleted by user"
      lastTransitionTime: "2025-12-13T12:00:00Z"
```

#### **Condition Update Logic**

```go
// When NotificationRequest is sent successfully
remediationrequest.SetNotificationDelivered(rr, true, remediationrequest.ReasonDeliverySucceeded, "Notification delivered successfully", h.Metrics)

// When user deletes NotificationRequest (BR-ORCH-029)
remediationrequest.SetNotificationDelivered(rr, false, remediationrequest.ReasonUserCancelled, "NotificationRequest deleted by user", h.Metrics)

// When NotificationRequest delivery fails
remediationrequest.SetNotificationDelivered(rr, false, remediationrequest.ReasonDeliveryFailed, notif.Status.Message, h.Metrics)
```

---

## 🔍 User Cancellation Detection (BR-ORCH-029)

### **Detection Logic**

RO distinguishes **user cancellation** from **cascade deletion**:

```go
func (r *RemediationOrchestratorReconciler) handleNotificationRequestDeletion(
    ctx context.Context,
    rr *remediationv1alpha1.RemediationRequest,
) error {
    // Check if RemediationRequest is being deleted
    if rr.DeletionTimestamp != nil {
        // Case 1: Cascade deletion (expected cleanup)
        log.Info("NotificationRequest deleted as part of RemediationRequest cleanup")
        return nil
    }

    // Case 2: User-initiated cancellation
    log.Info("NotificationRequest deleted by user (cancellation)")

    // Update notification tracking ONLY (DO NOT change overallPhase!)
    // CRITICAL: Notification lifecycle is SEPARATE from remediation lifecycle
    // Per DD-RO-001 Alternative 3: Deleting notification does NOT complete the remediation
    rr.Status.NotificationStatus = "Cancelled"
    rr.Status.Message = fmt.Sprintf(
        "NotificationRequest %s deleted by user before delivery completed",
        rr.Status.NotificationRequestRefs[0].Name,
    )

    // Set condition
    meta.SetStatusCondition(&rr.Status.Conditions, metav1.Condition{
        Type:    "NotificationDelivered",
        Status:  metav1.ConditionFalse,
        Reason:  "UserCancelled",
        Message: fmt.Sprintf("NotificationRequest deleted by user"),
    })

    return r.Status().Update(ctx, rr)
}
```

### **Key Decision: Mark as Completed (Not Failed)**

**Rationale**:
- ✅ User cancellation ≠ system failure
- ✅ Prevents false positive escalations
- ✅ Clear audit trail via conditions
- ✅ Respects user intent

**Evidence**: 95% confidence per [alternatives triage](implementation/BR-ORCH-029-030-031-034_ALTERNATIVES_TRIAGE.md)

---

## 🔗 Owner Reference Pattern (BR-ORCH-031)

### **Purpose**
Enable automatic cascade deletion when RemediationRequest is deleted, while allowing independent user deletion for cancellation.

### **Implementation**

```go
func (c *NotificationCreator) createNotificationRequest(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    notifType notificationv1.NotificationType,
) (*notificationv1.NotificationRequest, error) {
    notif := &notificationv1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("notif-%s-%s", notifType, rr.Name),
            Namespace: rr.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                {
                    APIVersion:         rr.APIVersion,
                    Kind:               rr.Kind,
                    Name:               rr.Name,
                    UID:                rr.UID,
                    Controller:         pointer.Bool(true),
                    BlockOwnerDeletion: pointer.Bool(false), // ⚡ CRITICAL
                },
            },
        },
        Spec: notificationv1.NotificationRequestSpec{
            // ... notification spec fields
        },
    }

    if err := c.client.Create(ctx, notif); err != nil {
        return nil, fmt.Errorf("failed to create NotificationRequest: %w", err)
    }

    // Track reference in RR status (BR-ORCH-035)
    rr.Status.NotificationRequestRefs = append(rr.Status.NotificationRequestRefs, corev1.ObjectReference{
        APIVersion: notif.APIVersion,
        Kind:       notif.Kind,
        Name:       notif.Name,
        Namespace:  notif.Namespace,
        UID:        notif.UID,
    })

    return notif, nil
}
```

### **Why `blockOwnerDeletion: false`?**

| Setting | Effect | Use Case |
|---------|--------|----------|
| `true` | Prevents RR deletion if NotificationRequest exists | ❌ Blocks cleanup |
| `false` | Allows independent deletion + cascade deletion | ✅ Enables cancellation |

**Decision**: Use `false` to support BR-ORCH-029 (user cancellation) while maintaining BR-ORCH-031 (cascade cleanup).

---

## 📦 Bulk Notification Integration (BR-ORCH-034)

### **Relationship with Other BRs**

BR-ORCH-034 (bulk notifications) is **orthogonal** to BR-ORCH-029/030/031:

| BR | Concern | Integration Point |
|----|---------|-------------------|
| **BR-ORCH-029** | User cancellation | Applies to ALL NotificationRequests (including bulk) |
| **BR-ORCH-030** | Status tracking | Applies to ALL NotificationRequests (including bulk) |
| **BR-ORCH-031** | Owner references | Applies to ALL NotificationRequests (including bulk) |
| **BR-ORCH-034** | Bulk notification content | Uses BR-ORCH-031 owner references, subject to BR-ORCH-029 cancellation |

### **Schema Support for Bulk Notifications**

**Existing Fields** (already in schema):
- ✅ `status.duplicateCount` - Number of skipped duplicates
- ✅ `status.duplicateRefs` - List of duplicate RR names

**New Fields** (added in v1.4):
- ✅ `status.notificationStatus` - Tracks bulk notification delivery
- ✅ `status.conditions` - Tracks bulk notification outcome

**Example: Bulk Notification Lifecycle**

```yaml
# Parent RR with 5 duplicates
status:
  duplicateCount: 5
  duplicateRefs: ["rr-2", "rr-3", "rr-4", "rr-5", "rr-6"]
  notificationStatus: "Sent"  # Bulk notification delivered
  notificationRequestRefs:
    - name: "notif-bulk-rr-1"
      kind: "NotificationRequest"
      # ... other ref fields
  conditions:
    - type: NotificationDelivered
      status: "True"
      reason: DeliverySucceeded
      message: "Bulk notification delivered: 5 duplicates suppressed"
```

---

## 🎯 Acceptance Criteria Mapping

### **BR-ORCH-029: User-Initiated Notification Cancellation**

| AC ID | Criterion | Schema Support |
|-------|-----------|----------------|
| AC-029-1 | User deletion detected via watch | Owner reference pattern (BR-ORCH-031) |
| AC-029-2 | RR marked as `Completed` (not `Failed`) | `status.overallPhase = Completed` |
| AC-029-3 | Condition `NotificationDelivered=False` with reason `UserCancelled` | `status.conditions[]` |
| AC-029-4 | No automatic escalation triggered | Phase classification (Completed is terminal) |
| AC-029-5 | Cascade deletion handled gracefully | `deletionTimestamp` check in reconciler |
| AC-029-6 | Audit trail clearly indicates user cancellation | `status.notificationStatus = "Cancelled"` + condition |

### **BR-ORCH-030: Notification Status Tracking**

| AC ID | Criterion | Schema Support |
|-------|-----------|----------------|
| AC-030-1 | RO watches NotificationRequest status updates | Watch pattern in `SetupWithManager()` |
| AC-030-2 | `status.notificationStatus` updated based on NotificationRequest phase | `status.notificationStatus` enum field |
| AC-030-3 | `NotificationDelivered` condition set with accurate reason | `status.conditions[]` |
| AC-030-4 | SREs can query RemediationRequests by notification status | Queryable via `kubectl get rr -o json \| jq` |
| AC-030-5 | Metrics expose notification status distribution | Prometheus metrics (implementation) |

### **BR-ORCH-031: Cascade Cleanup**

| AC ID | Criterion | Schema Support |
|-------|-----------|----------------|
| AC-031-1 | NotificationRequest has ownerReference to RemediationRequest | `ownerReferences[]` in NotificationRequest metadata |
| AC-031-2 | `blockOwnerDeletion = false` allows independent user deletion | `blockOwnerDeletion: false` in owner reference |
| AC-031-3 | Deleting RemediationRequest automatically deletes NotificationRequest | Kubernetes garbage collection |
| AC-031-4 | No orphaned NotificationRequest CRDs remain after RR deletion | Kubernetes garbage collection |
| AC-031-5 | RO distinguishes cascade deletion from user cancellation | `deletionTimestamp` check in reconciler |

### **BR-ORCH-034: Bulk Notification**

| AC ID | Criterion | Schema Support |
|-------|-----------|----------------|
| AC-034-1 | ONE notification sent when parent completes | Single NotificationRequest in `status.notificationRequestRefs` |
| AC-034-2 | Notification includes duplicate count and skip reasons | `status.duplicateCount`, `status.duplicateRefs` |
| AC-034-3 | Notification sent for both success AND failure outcomes | Reconciler logic (implementation) |
| AC-034-4 | Duplicate RR names included in notification metadata | `status.duplicateRefs[]` |
| AC-034-5 | No notification spam (10 duplicates = 1 notification) | Single NotificationRequest created |

---

## 🔧 CRD Manifest Regeneration

### **Required Actions**

1. **Update Type Definition**:
   ```bash
   # Edit api/remediation/v1alpha1/remediationrequest_types.go
   # Add NotificationStatus and Conditions fields
   ```

2. **Regenerate Deepcopy**:
   ```bash
   make generate
   ```

3. **Regenerate CRD Manifests**:
   ```bash
   make manifests
   ```

4. **Verify CRD Changes**:
   ```bash
   git diff config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml
   ```

### **Expected CRD Changes**

```yaml
# config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml
status:
  properties:
    # ... existing fields ...

    notificationStatus:
      description: Notification delivery status
      enum:
        - Pending
        - InProgress
        - Sent
        - Failed
        - Cancelled
      type: string

    conditions:
      description: Conditions represent observations of RemediationRequest state
      items:
        properties:
          lastTransitionTime:
            format: date-time
            type: string
          message:
            type: string
          observedGeneration:
            format: int64
            type: integer
          reason:
            type: string
          status:
            enum:
              - "True"
              - "False"
              - Unknown
            type: string
          type:
            type: string
        required:
          - lastTransitionTime
          - message
          - reason
          - status
          - type
        type: object
      type: array
      x-kubernetes-list-map-keys:
        - type
      x-kubernetes-list-type: map
```

---

## 📊 Observability Queries

### **Query Examples**

#### **1. Find all remediations with cancelled notifications**
```bash
kubectl get remediationrequest -A -o json | \
  jq '.items[] | select(.status.notificationStatus == "Cancelled") | {name: .metadata.name, namespace: .metadata.namespace, phase: .status.overallPhase}'
```

#### **2. Find all remediations with failed notification delivery**
```bash
kubectl get remediationrequest -A -o json | \
  jq '.items[] | select(.status.conditions[] | select(.type=="NotificationDelivered" and .status=="False" and .reason=="DeliveryFailed"))'
```

#### **3. Find all remediations where user cancelled notification**
```bash
kubectl get remediationrequest -A -o json | \
  jq '.items[] | select(.status.conditions[] | select(.type=="NotificationDelivered" and .reason=="UserCancelled"))'
```

#### **4. Find all bulk notifications**
```bash
kubectl get remediationrequest -A -o json | \
  jq '.items[] | select(.status.duplicateCount > 0) | {name: .metadata.name, duplicates: .status.duplicateCount, notificationStatus: .status.notificationStatus}'
```

---

## 🎓 User Documentation Impact

### **New User Workflows**

#### **Cancelling In-Flight Notification**

```bash
# 1. Find the NotificationRequest associated with your RemediationRequest
kubectl get notificationrequest -n kubernaut-system

# 2. Delete the NotificationRequest to cancel
kubectl delete notificationrequest notif-rr-123 -n kubernaut-system

# 3. Verify cancellation status
kubectl get remediationrequest rr-123 -o yaml | grep -A 10 conditions

# Expected output:
conditions:
  - type: NotificationDelivered
    status: "False"
    reason: UserCancelled
    message: "NotificationRequest notif-rr-123 deleted by user"
```

#### **Checking Notification Status**

```bash
# Quick status check
kubectl get remediationrequest rr-123 -o jsonpath='{.status.notificationStatus}'

# Detailed condition check
kubectl get remediationrequest rr-123 -o jsonpath='{.status.conditions[?(@.type=="NotificationDelivered")]}'
```

---

## 🚨 Breaking Changes

**None** - This is an additive schema change.

**Backward Compatibility**:
- ✅ New fields are optional (`+optional`)
- ✅ Existing RRs without new fields will continue to work
- ✅ No migration required for existing RRs

---

## 📚 Related Documents

- [BR-ORCH-029-031-notification-handling.md](../../../requirements/BR-ORCH-029-031-notification-handling.md) - Business requirements
- [BR-ORCH-032-034-resource-lock-deduplication.md](../../../requirements/BR-ORCH-032-034-resource-lock-deduplication.md) - Bulk notification requirements
- [DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md](DD-RO-001-NOTIFICATION-CANCELLATION-HANDLING.md) - Design decision
- [BR-ORCH-029-030-031-034_ALTERNATIVES_TRIAGE.md](implementation/BR-ORCH-029-030-031-034_ALTERNATIVES_TRIAGE.md) - Alternatives analysis (95% confidence)
- [crd-schema.md](crd-schema.md) - Complete CRD schema reference

---

**Document Version**: 1.0
**Last Updated**: December 13, 2025
**Maintained By**: Kubernaut RO Team
**Status**: 📋 **PROPOSED** - Ready for Implementation Planning
**Confidence**: 95% (exceeds 90% threshold ✅)

