# DD-RO-001: Resource Lock Deduplication Handling

**Status**: ✅ Approved
**Version**: 1.0
**Date**: 2025-12-01
**Confidence**: 95%
**Author**: Kubernaut Architecture Team

---

## Decision Summary

The Remediation Orchestrator (RO) handles WorkflowExecution `Skipped` phases by marking RemediationRequests as deduplicated and sending bulk notifications when the parent remediation completes. This document defines the contract between RO and WE for resource-lock deduplication scenarios.

---

## Context and Problem Statement

### Multi-Layer Deduplication Architecture

Kubernaut implements deduplication at multiple layers:

```
┌─────────────────────────────────────────────────────────────────────┐
│                        DEDUPLICATION LAYERS                         │
├─────────────────────────────────────────────────────────────────────┤
│  Layer 1: Gateway - Fingerprint Deduplication (DD-GATEWAY-009)      │
│  - Same fingerprint → Update occurrenceCount (no new RR)            │
│  - Handles: 70-80% of duplicates                                    │
├─────────────────────────────────────────────────────────────────────┤
│  Layer 2: Gateway - Storm Aggregation (DD-GATEWAY-008)              │
│  - Different fingerprints, threshold >5 → Aggregate into 1 RR      │
│  - Handles: 15-25% of duplicates (storm scenarios)                  │
├─────────────────────────────────────────────────────────────────────┤
│  Layer 3: WE - Resource Locking (DD-WE-001)                         │
│  - Different fingerprints, same target resource → Skipped phase     │
│  - Handles: 5-10% edge cases (safety net)                           │
│  - THIS DOCUMENT: How RO handles Layer 3 skips                      │
└─────────────────────────────────────────────────────────────────────┘
```

### The Problem

When WorkflowExecution returns `Skipped` phase (due to `ResourceBusy` or `RecentlyRemediated`), RO must:
1. Track the relationship between skipped and active remediations
2. Notify users about skipped remediations
3. Avoid notification spam (10 skipped RRs should NOT send 10 notifications)

### Example Scenario: Node DiskPressure

```
Node: worker-node-1 (DiskPressure)
  ├─ Pod evicted in prod      → Signal A (fingerprint: prod-abc)
  ├─ Pod evicted in staging   → Signal B (fingerprint: staging-def)
  └─ Pod evicted in dev       → Signal C (fingerprint: dev-ghi)

Gateway: Different fingerprints → 3 separate RemediationRequests
AIAnalysis: All resolve to same workflow (node-disk-cleanup)
WE: First executes, rest are Skipped (ResourceBusy)
```

**Without this DD**: 3 separate notifications, confusing audit trail
**With this DD**: 1 bulk notification with complete context

---

## Decision

### 1. Track Skipped Remediations

When WE returns `Skipped` phase, RO updates the RemediationRequest status:

```go
type RemediationRequestStatus struct {
    // Existing fields...
    Phase   string `json:"phase"`   // "Skipped"
    Message string `json:"message"` // "Skipped: Resource busy - another workflow executing"

    // NEW: Deduplication tracking
    SkipReason  string `json:"skipReason,omitempty"`  // "ResourceBusy" | "RecentlyRemediated"
    DuplicateOf string `json:"duplicateOf,omitempty"` // "remediation-request-abc123"
}
```

### 2. Track Duplicates on Parent RR

The parent (first/active) RemediationRequest tracks its duplicates:

```go
type RemediationRequestStatus struct {
    // Existing fields...

    // NEW: Duplicate tracking (only on parent RR)
    DuplicateCount int      `json:"duplicateCount,omitempty"` // 5
    DuplicateRefs  []string `json:"duplicateRefs,omitempty"`  // ["rr-002", "rr-003", ...]
}
```

### 3. Bulk Notification on Parent Completion

When the parent RR completes (success OR failure), RO sends ONE notification:

```yaml
kind: NotificationRequest
spec:
  eventType: "RemediationCompleted"
  priority: "normal"
  subject: "Remediation Completed: node-disk-cleanup"
  body: |
    Target: node/worker-node-1
    Result: ✅ Successful
    Duration: 2m 34s

    Duplicates Suppressed: 5
    ├─ ResourceBusy: 3 (signals during execution)
    └─ RecentlyRemediated: 2 (cooldown period)

    First signal: 2025-12-01 14:30:52
    Last signal:  2025-12-01 14:31:18
  metadata:
    remediationRequestRef: "remediation-request-abc123"
    workflowId: "node-disk-cleanup"
    targetResource: "node/worker-node-1"
    duplicateCount: 5
    duplicateRefs: ["rr-002", "rr-003", "rr-004", "rr-005", "rr-006"]
```

---

## Technical Design

### RO Handler for WE Skipped Phase

```go
func (r *Reconciler) handleWorkflowExecutionSkipped(
    ctx context.Context,
    rr *RemediationRequest,
    we *WorkflowExecution,
) error {
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

    // Update this RR as skipped duplicate
    rr.Status.Phase = "Skipped"
    rr.Status.SkipReason = skipReason
    rr.Status.DuplicateOf = parentRRName
    rr.Status.Message = fmt.Sprintf("Skipped: %s - see %s", skipReason, parentRRName)

    if err := r.Status().Update(ctx, rr); err != nil {
        return fmt.Errorf("failed to update skipped RR: %w", err)
    }

    // Update parent RR's duplicate tracking
    if err := r.trackDuplicateOnParent(ctx, parentRRName, rr.Name); err != nil {
        log.Error(err, "Failed to track duplicate on parent",
            "parent", parentRRName, "duplicate", rr.Name)
        // Non-fatal: continue even if tracking fails
    }

    log.Info("RemediationRequest skipped due to resource lock",
        "rr", rr.Name,
        "skipReason", skipReason,
        "duplicateOf", parentRRName)

    return nil
}

func (r *Reconciler) trackDuplicateOnParent(
    ctx context.Context,
    parentRRName string,
    duplicateRRName string,
) error {
    parentRR := &RemediationRequest{}
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
```

### RO Handler for Parent Completion

```go
func (r *Reconciler) handleWorkflowExecutionCompleted(
    ctx context.Context,
    rr *RemediationRequest,
    we *WorkflowExecution,
) error {
    // ... existing completion logic ...

    // If this RR had duplicates, send bulk notification
    if rr.Status.DuplicateCount > 0 {
        if err := r.sendBulkDuplicateNotification(ctx, rr, we); err != nil {
            log.Error(err, "Failed to send bulk notification", "rr", rr.Name)
            // Non-fatal: remediation is complete regardless
        }
    }

    return nil
}

func (r *Reconciler) sendBulkDuplicateNotification(
    ctx context.Context,
    rr *RemediationRequest,
    we *WorkflowExecution,
) error {
    // Count skip reasons
    resourceBusyCount := 0
    recentlyRemediatedCount := 0

    for _, dupRef := range rr.Status.DuplicateRefs {
        dupRR := &RemediationRequest{}
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
    body := fmt.Sprintf(`Target: %s
Result: %s
Duration: %s

Duplicates Suppressed: %d
├─ ResourceBusy: %d (signals during execution)
└─ RecentlyRemediated: %d (cooldown period)

First signal: %s
Last signal: %s`,
        we.Spec.TargetResource,
        resultEmoji(we.Status.Phase),
        we.Status.CompletedAt.Sub(we.Status.StartedAt.Time),
        rr.Status.DuplicateCount,
        resourceBusyCount,
        recentlyRemediatedCount,
        rr.Spec.Deduplication.FirstOccurrence.Format(time.RFC3339),
        rr.Spec.Deduplication.LastOccurrence.Format(time.RFC3339),
    )

    // Create notification request
    notification := &NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-completion", rr.Name),
            Namespace: rr.Namespace,
        },
        Spec: NotificationRequestSpec{
            EventType: "RemediationCompleted",
            Priority:  r.mapPriority(rr),
            Subject:   fmt.Sprintf("Remediation Completed: %s", we.Spec.WorkflowRef.WorkflowID),
            Body:      body,
            Metadata: map[string]string{
                "remediationRequestRef": rr.Name,
                "workflowId":            we.Spec.WorkflowRef.WorkflowID,
                "targetResource":        we.Spec.TargetResource,
                "duplicateCount":        strconv.Itoa(rr.Status.DuplicateCount),
            },
        },
    }

    return r.Create(ctx, notification)
}
```

### Target Resource Format

RO builds `targetResource` from `RemediationRequest.spec.targetResource`:

```go
func buildTargetResource(rr *RemediationRequest) string {
    tr := rr.Spec.TargetResource
    if tr.Namespace != "" {
        return fmt.Sprintf("%s/%s/%s", tr.Namespace, tr.Kind, tr.Name)
    }
    return fmt.Sprintf("%s/%s", tr.Kind, tr.Name)
}
```

---

## Skip Reason Handling Matrix

| Skip Reason | RO Action | Notification |
|-------------|-----------|--------------|
| `ResourceBusy` | Mark Skipped, track on parent | Include in bulk notification |
| `RecentlyRemediated` | Mark Skipped, track on parent | Include in bulk notification |

---

## Cascade Deletion Handling

When parent RR is deleted:

| Scenario | Behavior |
|----------|----------|
| Parent deleted before completion | Duplicates remain with stale `duplicateOf` reference |
| Parent deleted after completion | Notification already sent, duplicates remain for audit |

**V1.0 Decision**: Duplicates are NOT cascade-deleted. The `duplicateOf` field is informational only.

**V2.0 Consideration**: Add condition to duplicates if parent is missing:
```yaml
conditions:
  - type: ParentAvailable
    status: "False"
    reason: ParentDeleted
    message: "Parent RR 'rr-abc123' no longer exists"
```

---

## Failed Parent Handling

When parent RR fails:

| Scenario | RO Action |
|----------|-----------|
| Parent workflow fails | All duplicates remain Skipped |
| Parent times out | All duplicates remain Skipped |

**V1.0 Decision**: Duplicates share the parent's fate. No automatic retry with duplicates.

**Rationale**:
- Same root cause likely means same failure
- Retry storms are dangerous
- Operator can manually create new signal if needed

---

## Metrics

```go
// Deduplication metrics
deduplication_skipped_total{reason="ResourceBusy"}
deduplication_skipped_total{reason="RecentlyRemediated"}

// Bulk notification metrics
bulk_notification_sent_total{result="success"}
bulk_notification_sent_total{result="failure"}
bulk_notification_duplicate_count_histogram
```

---

## Business Requirements Generated

| BR ID | Title | Priority |
|-------|-------|----------|
| **BR-ORCH-032** | Handle WE Skipped Phase | P0 |
| **BR-ORCH-033** | Track Duplicate Remediations | P1 |
| **BR-ORCH-034** | Bulk Notification for Duplicates | P1 |

See: [BUSINESS_REQUIREMENTS.md](../../services/crd-controllers/05-remediationorchestrator/BUSINESS_REQUIREMENTS.md)

---

## Integration Points

### RO → WE Contract (from DD-WE-001)

| Field | Direction | Description |
|-------|-----------|-------------|
| `spec.targetResource` | RO → WE | Format: `namespace/kind/name` or `kind/name` |
| `status.phase` | WE → RO | Includes `Skipped` phase |
| `status.skipDetails` | WE → RO | Reason, conflicting workflow, recent remediation |

### RO → Notification Contract

| Field | Description |
|-------|-------------|
| `spec.eventType` | `RemediationCompleted` |
| `spec.metadata.duplicateCount` | Number of skipped duplicates |
| `spec.metadata.duplicateRefs` | List of skipped RR names |

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| **DD-WE-001** | Resource locking that triggers Skipped phase |
| **DD-GATEWAY-009** | Fingerprint deduplication (Layer 1) |
| **DD-GATEWAY-008** | Storm aggregation (Layer 2) |
| **DD-ORCHESTRATOR-001** | Storm/dedup data propagation to AIAnalysis |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-01 | Initial DD: Resource lock deduplication handling |

---

**Document Version**: 1.0
**Last Updated**: December 1, 2025
**Maintained By**: Kubernaut Architecture Team
**Status**: ✅ Approved

