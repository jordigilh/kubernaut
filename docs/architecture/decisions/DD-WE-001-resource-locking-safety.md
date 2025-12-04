# DD-WE-001: Resource Locking Safety for Workflow Execution

**Status**: ✅ Approved
**Version**: 1.0
**Date**: 2025-12-01
**Confidence**: 95%
**Author**: Kubernaut Architecture Team

---

## Decision Summary

The WorkflowExecution Controller implements **resource-level locking** to prevent parallel and redundant workflow executions on the same Kubernetes resource. This is a **V1.0 safety feature** that prioritizes correctness over throughput.

---

## Context and Problem Statement

### The Node DiskPressure Scenario

When a node experiences DiskPressure, multiple pods across namespaces may be evicted. Each eviction generates a separate signal:

```
Node: worker-node-1 (DiskPressure)
  ├─ Namespace: prod     → Pod evicted → Signal (fingerprint: prod-pod-abc)
  ├─ Namespace: staging  → Pod evicted → Signal (fingerprint: staging-pod-def)
  ├─ Namespace: dev      → Pod evicted → Signal (fingerprint: dev-pod-ghi)
  └─ ... 10+ more signals
```

**Problem 1**: Each signal has a unique fingerprint (Gateway deduplication doesn't catch this).

**Problem 2**: AIAnalysis resolves ALL signals to the same root cause: "Node DiskPressure → Run `node-disk-cleanup` workflow".

**Problem 3**: 10+ WorkflowExecutions target the same node with the same workflow.

### Cross-Workflow Conflict Scenario

Even different workflows targeting the same resource can conflict:

```
WE-1: increase-memory     WE-2: restart-pods
     │                         │
     ▼                         ▼
   [Patch deployment]    [Delete pods]
         ↓                    ↓
      ⚠️ CONFLICT ⚠️
```

**Problem**: Two workflows may operate on stale state, causing unpredictable outcomes.

---

## Decision

### V1.0: Safe by Default

1. **Resource Lock**: Only ONE workflow can execute on a target resource at any time, regardless of workflow type
2. **Cooldown**: Same workflow+target combination is blocked for 5 minutes after completion
3. **Lock Scope**: Target resource only (e.g., `payment/deployment/payment-api`)
4. **Skip, Don't Queue**: Blocked executions are marked `Skipped`, not queued

### Rejected Alternatives

| Alternative | Why Rejected |
|-------------|--------------|
| **Queue blocked executions** | Complexity; unknown cluster state after first execution |
| **Workflow-specific locks** | Different workflows can still conflict |
| **Lock groups** | V2.0 feature - too complex for V1.0 |
| **Opt-out flag** | V2.0 feature - safe by default for V1.0 |

---

## Decision Drivers

1. **Safety First**: Non-idempotent workflows can cause cascading failures
2. **Unknown State**: After a failed workflow, cluster state is unknown
3. **User Feedback**: Explicit skip with reason is better than silent queue
4. **Simplicity**: V1.0 prioritizes correctness over optimization

---

## Technical Design

### Target Resource Format

```
spec:
  targetResource: "namespace/kind/name"  # namespaced
  targetResource: "kind/name"            # cluster-scoped

Examples:
  - "payment/deployment/payment-api"
  - "node/worker-node-1"
  - "kube-system/configmap/coredns"
```

### Lock Check Algorithm

```go
// Pseudo-code for resource lock check
func checkResourceLock(wfe WorkflowExecution) (SkipDetails, bool) {
    target := wfe.Spec.TargetResource
    workflowID := wfe.Spec.WorkflowRef.WorkflowID

    for existing := range listWorkflowExecutions(wfe.Namespace) {
        if existing.Spec.TargetResource != target {
            continue // Different target - OK
        }

        // Check 1: Is another workflow RUNNING on this target?
        if existing.Status.Phase in ["Running", "Pending"] {
            return SkipDetails{
                Reason: "ResourceBusy",
                ConflictingWorkflow: existing,
            }, true
        }

        // Check 2: Was SAME workflow recently executed on this target?
        if existing.Spec.WorkflowRef.WorkflowID == workflowID {
            if existing.CompletedWithin(5 * time.Minute) {
                return SkipDetails{
                    Reason: "RecentlyRemediated",
                    RecentRemediation: existing,
                }, true
            }
        }
    }

    return nil, false // Proceed with execution
}
```

### Skip Decision Matrix

| Existing WE | New WE | Same Target? | Same Workflow? | Decision |
|-------------|--------|--------------|----------------|----------|
| Running | Any | Yes | Any | **Skip (ResourceBusy)** |
| Completed <5m | Any | Yes | Yes | **Skip (RecentlyRemediated)** |
| Completed <5m | Any | Yes | No | **Allow** (different workflow) |
| Completed >5m | Any | Yes | Yes | **Allow** (cooldown expired) |
| Any | Any | No | Any | **Allow** (different target) |

---

## CRD Schema Changes

### New Spec Field

```go
type WorkflowExecutionSpec struct {
    // ... existing fields ...

    // TargetResource identifies the K8s resource being remediated
    // Format: "namespace/kind/name" or "kind/name" for cluster-scoped
    TargetResource string `json:"targetResource"`
}
```

### New Status Phase

```go
// Phase enum now includes Skipped
// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed;Skipped
Phase string `json:"phase"`
```

### New Status Fields

```go
type WorkflowExecutionStatus struct {
    // ... existing fields ...

    // SkipDetails explains why execution was skipped
    // Populated when Phase=Skipped
    SkipDetails *SkipDetails `json:"skipDetails,omitempty"`
}

type SkipDetails struct {
    Reason              string                   `json:"reason"` // ResourceBusy | RecentlyRemediated
    Message             string                   `json:"message"`
    SkippedAt           metav1.Time              `json:"skippedAt"`
    ConflictingWorkflow *ConflictingWorkflowRef  `json:"conflictingWorkflow,omitempty"`
    RecentRemediation   *RecentRemediationRef    `json:"recentRemediation,omitempty"`
}
```

---

## Integration Points

### RemediationOrchestrator Impact

RO must populate `targetResource` when creating WorkflowExecution:

```go
// RO creates WorkflowExecution with targetResource from signal context
wfe := &WorkflowExecution{
    Spec: WorkflowExecutionSpec{
        TargetResource: buildTargetResource(signal), // e.g., "payment/deployment/payment-api"
        // ... other fields ...
    },
}
```

### Audit Trail

Skipped executions are recorded:

```go
// Audit event for skipped execution
r.AuditClient.WriteExecutionSkipped(ctx, wfe)

// Audit record includes:
// - remediation_id
// - workflow_id
// - target_resource
// - skip_reason
// - conflicting_workflow (if ResourceBusy)
// - recent_remediation (if RecentlyRemediated)
```

### Notification

Skipped executions generate notifications:

```yaml
kind: NotificationRequest
spec:
  eventType: "WorkflowSkipped"
  severity: "info"
  message: "Workflow execution skipped: Another workflow is remediating this resource"
  context:
    workflowId: "node-disk-cleanup"
    targetResource: "node/worker-node-1"
    skipReason: "ResourceBusy"
    conflictingWorkflow: "workflow-node-disk-001"
```

---

## Business Requirements Generated

This DD generates the following BRs:

| BR ID | Title | Priority |
|-------|-------|----------|
| **BR-WE-009** | Resource Locking - Prevent Parallel Execution | P0 |
| **BR-WE-010** | Cooldown - Prevent Redundant Sequential Execution | P0 |
| **BR-WE-011** | Target Resource Identification | P0 |

See: [BR-WE-009-011-resource-locking.md](../../requirements/BR-WE-009-011-resource-locking.md)

---

## V2.0 Considerations (Out of Scope)

These features are deferred to V2.0 based on production feedback:

| Feature | Description | V2.0 Consideration |
|---------|-------------|-------------------|
| **Lock Groups** | Workflows declare compatible groups | Allow read-only workflows to run in parallel |
| **Opt-out Flag** | Workflow metadata: `exclusiveAccess: false` | For idempotent workflows |
| **Queueing** | Queue blocked executions | If cluster state is predictable |
| **Configurable Cooldown** | Per-workflow cooldown periods | Based on workflow characteristics |

---

## Consequences

### Positive

- ✅ Prevents cascading failures from parallel workflows
- ✅ Prevents redundant remediation attempts
- ✅ Clear audit trail for skipped executions
- ✅ User notification when execution is skipped
- ✅ Simple, deterministic behavior for V1.0

### Negative

- ❌ May skip valid executions in rare edge cases
- ❌ Fixed cooldown may be suboptimal for some workflows
- ❌ No queueing means some remediations may never execute

### Mitigations

- Cooldown is configurable at controller level
- Skipped executions are clearly visible in audit trail
- V2.0 can add opt-out for specific workflows

---

## Related Documents

| Document | Relationship |
|----------|--------------|
| **DD-CONTRACT-001 v1.4** | Updated with resource locking contract |
| **DD-GATEWAY-008** | Storm aggregation at Gateway level |
| **DD-GATEWAY-009** | Fingerprint deduplication at Gateway level |
| **DD-ORCHESTRATOR-001** | Storm/dedup context propagation |
| **BR-WE-009** | Business requirement: Prevent parallel execution |
| **BR-WE-010** | Business requirement: Cooldown |
| **BR-WE-011** | Business requirement: Target resource identification |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-01 | Initial DD: Resource locking safety for V1.0 |

---

**Document Version**: 1.0
**Last Updated**: December 1, 2025
**Maintained By**: Kubernaut Architecture Team
**Status**: ✅ Approved



