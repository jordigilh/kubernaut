# DD-WE-003: Resource Lock Persistence Strategy

**Version**: 1.0
**Date**: 2025-12-03
**Status**: ⚠️ **SUPERSEDED BY DD-RO-002** (V1.0 Centralized Routing)
**Superseded**: 2025-12-15
**Author**: WorkflowExecution Team
**Confidence**: 94%

---

## ⚠️ **V1.0 UPDATE: ROUTING MOVED TO REMEDIATIONORCHESTRATOR**

**As of V1.0 (December 15, 2025), resource lock checking is now handled by RemediationOrchestrator, not WorkflowExecution.**

- **New Authority**: [DD-RO-002: Centralized Routing Responsibility](../decisions/DD-RO-002-centralized-routing-responsibility.md)
- **WE Role**: Pure executor - still uses deterministic PipelineRun naming for execution-time collision detection (Layer 2)
- **RO Role**: Router - checks for existing Running WFEs before creating new ones (Layer 1)

**NOTE**: The deterministic naming strategy described in this document is **still used by WE** for Layer 2 safety (execution-time race condition detection), but the routing decision (checking for locks) is now made by RO.

**This document remains for historical context and understanding the technical implementation details.**

---

## Context

WorkflowExecution implements resource locking (DD-WE-001) to prevent parallel and redundant sequential remediations on the same target resource. The initial design used in-memory locking, which has critical flaws:

**Problems with in-memory locking**:
- Controller restart loses all lock state
- Multi-replica deployment breaks locking
- Race conditions between check and create
- No audit trail of lock state

We need a **persistent, distributed, race-condition-free** locking mechanism.

---

## Decision

**Selected Approach**: Deterministic PipelineRun Name + Indexed CRD Query (Belt-and-Suspenders)

### Core Principle

**The PipelineRun IS the lock.** Its existence means the resource is locked.

```go
// Deterministic name based on target resource
func pipelineRunName(targetResource string) string {
    h := sha256.Sum256([]byte(targetResource))
    return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}
```

### Two-Layer Protection

| Layer | Purpose | Catches |
|-------|---------|---------|
| **Layer 1**: Indexed CRD Query | Fast path check | 99% of conflicts |
| **Layer 2**: Deterministic PipelineRun Name | Atomic K8s guarantee | Race condition (remaining 1%) |

---

## Alternatives Considered

### Option A: In-Memory Lock Map

**Description**: Simple `sync.Map` with target resource as key.

```go
type WorkflowExecutionReconciler struct {
    locks sync.Map  // targetResource -> wfeName
}
```

| Pros | Cons |
|------|------|
| ✅ Simple implementation | ❌ Lost on controller restart |
| ✅ Fast O(1) | ❌ Doesn't work with multiple replicas |
| | ❌ Race condition between check and create |
| | ❌ No audit trail |

**Decision**: ❌ Rejected - Not suitable for production

---

### Option B: Redis-Based Distributed Lock

**Description**: Use Redis SETNX with TTL for distributed locking.

```go
func (r *WorkflowExecutionReconciler) acquireLock(ctx context.Context, target string) (bool, error) {
    lockKey := fmt.Sprintf("wfe:lock:%s", target)
    return r.Redis.SetNX(ctx, lockKey, wfeName, 30*time.Minute).Result()
}
```

| Pros | Cons |
|------|------|
| ✅ True distributed lock | ❌ New dependency (Redis) |
| ✅ Fast O(1) | ❌ Redis failure handling needed |
| ✅ Built-in TTL | ❌ Lock orphaning if controller crashes |
| ✅ No race conditions | ❌ Operational complexity |

**Decision**: ❌ Rejected - Adds unnecessary dependency

---

### Option C: Kubernetes Lease-Based Lock

**Description**: Create Lease objects per target resource.

```go
func (r *WorkflowExecutionReconciler) acquireLock(ctx context.Context, target string) (bool, error) {
    lease := &coordinationv1.Lease{
        ObjectMeta: metav1.ObjectMeta{
            Name: fmt.Sprintf("wfe-lock-%s", hash(target)),
        },
        Spec: coordinationv1.LeaseSpec{
            HolderIdentity:       ptr.To(wfeName),
            LeaseDurationSeconds: ptr.To(int32(1800)),
        },
    }
    return r.Create(ctx, lease) == nil, nil
}
```

| Pros | Cons |
|------|------|
| ✅ Kubernetes-native | ❌ Not designed for this use case |
| ✅ No external deps | ❌ Many Lease objects (cleanup needed) |
| ✅ Built-in expiry | ❌ etcd load with many leases |

**Decision**: ❌ Rejected - Lease API not designed for application locks

---

### Option D: CRD Query Only (No Deterministic Name)

**Description**: Query existing WorkflowExecution CRDs with matching targetResource.

```go
func (r *WorkflowExecutionReconciler) checkResourceLock(ctx context.Context, wfe *WFE) bool {
    var list WorkflowExecutionList
    r.List(ctx, &list, client.MatchingFields{"spec.targetResource": wfe.Spec.TargetResource})
    for _, existing := range list.Items {
        if existing.Status.Phase == PhaseRunning {
            return true  // Blocked
        }
    }
    return false
}
```

| Pros | Cons |
|------|------|
| ✅ No new dependencies | ❌ Race condition between check and create |
| ✅ Uses existing CRDs | ❌ Requires field index for performance |
| ✅ Survives restarts | |

**Decision**: ⚠️ Partial - Good for fast path, but race condition remains

---

### Option E: Deterministic PipelineRun Name + Indexed CRD Query (SELECTED)

**Description**: Combine indexed CRD query (fast path) with deterministic PipelineRun name (atomic guarantee).

```go
func (r *WorkflowExecutionReconciler) reconcilePending(ctx context.Context, wfe *WFE) (Result, error) {
    // Layer 1: Fast path check (catches 99%)
    if blocked, _ := r.checkResourceLock(ctx, wfe); blocked {
        return r.markSkipped(ctx, wfe, "ResourceBusy")
    }

    // Layer 2: Atomic create with deterministic name (catches race condition)
    pr := r.buildPipelineRun(wfe)  // Name = hash(targetResource)
    err := r.Create(ctx, pr)

    if apierrors.IsAlreadyExists(err) {
        // Race condition caught! Another WFE got there first.
        return r.markSkipped(ctx, wfe, "ResourceBusy")
    }

    return r.markRunning(ctx, wfe, pr.Name)
}
```

| Pros | Cons |
|------|------|
| ✅ Zero race conditions | ⚠️ One PipelineRun per target at a time |
| ✅ No new dependencies | (This is desired behavior) |
| ✅ Survives restarts | |
| ✅ Works with multiple replicas | |
| ✅ Audit trail (PipelineRun exists) | |
| ✅ Fast path + atomic guarantee | |

**Decision**: ✅ Selected

---

## Implementation

### 1. Deterministic PipelineRun Name

```go
import (
    "crypto/sha256"
    "encoding/hex"
)

// pipelineRunName generates a deterministic name based on target resource.
// Two WFEs targeting the same resource will generate the same name.
// Kubernetes will reject duplicate creation, providing atomic locking.
func pipelineRunName(targetResource string) string {
    h := sha256.Sum256([]byte(targetResource))
    return fmt.Sprintf("wfe-%s", hex.EncodeToString(h[:])[:16])
}

// Example:
// targetResource: "production/deployment/payment-api"
// pipelineRunName: "wfe-a3f8b2c1d4e5f6a7"
```

### 2. Field Index for Fast Path

```go
func (r *WorkflowExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Create index on targetResource for O(1) lookup
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &workflowexecutionv1.WorkflowExecution{},
        "spec.targetResource",
        func(obj client.Object) []string {
            wfe := obj.(*workflowexecutionv1.WorkflowExecution)
            return []string{wfe.Spec.TargetResource}
        },
    ); err != nil {
        return err
    }

    return ctrl.NewControllerManagedBy(mgr).
        For(&workflowexecutionv1.WorkflowExecution{}).
        // ... watches ...
        Complete(r)
}
```

### 3. Two-Layer Lock Check

```go
func (r *WorkflowExecutionReconciler) reconcilePending(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // ========================================
    // LAYER 1: Fast path check (indexed query)
    // Catches 99% of conflicts without API call
    // ========================================
    if blocked, reason := r.checkResourceLock(ctx, wfe); blocked {
        log.Info("Resource locked (fast path)", "reason", reason)
        return r.markSkipped(ctx, wfe, reason)
    }

    // ========================================
    // LAYER 2: Atomic create with deterministic name
    // Catches race condition (remaining 1%)
    // Kubernetes guarantees object name uniqueness
    // ========================================
    pr := r.buildPipelineRun(wfe)
    err := r.Create(ctx, pr)

    if apierrors.IsAlreadyExists(err) {
        // Race condition caught! Another WFE created PipelineRun first.
        log.Info("Resource locked (race condition caught)",
            "targetResource", wfe.Spec.TargetResource,
            "pipelineRun", pr.Name)

        // Identify who holds the lock
        existingPR := &tektonv1.PipelineRun{}
        if getErr := r.Get(ctx, client.ObjectKey{
            Name:      pr.Name,
            Namespace: r.ExecutionNamespace,
        }, existingPR); getErr == nil {
            holder := existingPR.Labels["kubernaut.ai/workflow-execution"]
            return r.markSkipped(ctx, wfe, fmt.Sprintf("ResourceBusy (held by %s)", holder))
        }

        return r.markSkipped(ctx, wfe, "ResourceBusy")
    }

    if err != nil {
        log.Error(err, "Failed to create PipelineRun")
        return ctrl.Result{}, err
    }

    // Success - we hold the lock
    log.Info("Lock acquired, PipelineRun created",
        "targetResource", wfe.Spec.TargetResource,
        "pipelineRun", pr.Name)

    return r.markRunning(ctx, wfe, pr.Name)
}

func (r *WorkflowExecutionReconciler) checkResourceLock(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (blocked bool, reason string) {
    var wfeList workflowexecutionv1.WorkflowExecutionList

    // Indexed query - O(1) performance
    if err := r.List(ctx, &wfeList,
        client.MatchingFields{"spec.targetResource": wfe.Spec.TargetResource},
    ); err != nil {
        // Fail-open: allow execution if query fails
        return false, ""
    }

    for _, existing := range wfeList.Items {
        if existing.Name == wfe.Name {
            continue  // Skip self
        }

        // Check for active execution
        if existing.Status.Phase == PhaseRunning {
            return true, fmt.Sprintf("ResourceBusy (active: %s)", existing.Name)
        }

        // Check cooldown period
        if existing.Status.Phase == PhaseCompleted || existing.Status.Phase == PhaseFailed {
            if existing.Status.CompletionTime != nil {
                elapsed := time.Since(existing.Status.CompletionTime.Time)
                if elapsed < r.CooldownPeriod {
                    return true, fmt.Sprintf("RecentlyRemediated (by %s, %s ago)",
                        existing.Name, elapsed.Round(time.Second))
                }
            }
        }
    }

    return false, ""
}
```

### 4. Lock Lifecycle (Deletion)

```go
// PipelineRun deletion = lock release
// Three actors can delete:
// 1. WE Controller (after cooldown)
// 2. WFE Finalizer (when WFE deleted)
// 3. Tekton TTL (backup safety net)

func (r *WorkflowExecutionReconciler) reconcileTerminal(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if wfe.Status.CompletionTime == nil {
        return ctrl.Result{}, nil
    }

    elapsed := time.Since(wfe.Status.CompletionTime.Time)

    // Wait for cooldown before releasing lock
    if elapsed < r.CooldownPeriod {
        remaining := r.CooldownPeriod - elapsed
        log.V(1).Info("Waiting for cooldown",
            "remaining", remaining,
            "targetResource", wfe.Spec.TargetResource)
        return ctrl.Result{RequeueAfter: remaining}, nil
    }

    // Cooldown expired - delete PipelineRun to release lock
    prName := pipelineRunName(wfe.Spec.TargetResource)
    pr := &tektonv1.PipelineRun{
        ObjectMeta: metav1.ObjectMeta{
            Name:      prName,
            Namespace: r.ExecutionNamespace,
        },
    }

    if err := r.Delete(ctx, pr); err != nil && !apierrors.IsNotFound(err) {
        log.Error(err, "Failed to delete PipelineRun")
        return ctrl.Result{}, err
    }

    log.Info("Lock released after cooldown",
        "targetResource", wfe.Spec.TargetResource,
        "cooldownPeriod", r.CooldownPeriod)

    return ctrl.Result{}, nil
}

func (r *WorkflowExecutionReconciler) reconcileDelete(
    ctx context.Context,
    wfe *workflowexecutionv1.WorkflowExecution,
) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    if controllerutil.ContainsFinalizer(wfe, workflowExecutionFinalizer) {
        // Delete associated PipelineRun (releases lock)
        prName := pipelineRunName(wfe.Spec.TargetResource)
        pr := &tektonv1.PipelineRun{
            ObjectMeta: metav1.ObjectMeta{
                Name:      prName,
                Namespace: r.ExecutionNamespace,
            },
        }

        if err := r.Delete(ctx, pr); err != nil && !apierrors.IsNotFound(err) {
            log.Error(err, "Failed to delete PipelineRun during finalization")
            return ctrl.Result{}, err
        }

        log.Info("Finalizer: deleted associated PipelineRun", "pipelineRun", prName)

        // Remove finalizer
        controllerutil.RemoveFinalizer(wfe, workflowExecutionFinalizer)
        if err := r.Update(ctx, wfe); err != nil {
            return ctrl.Result{}, err
        }
    }

    return ctrl.Result{}, nil
}
```

---

## Mitigations

### M1: Race Condition

**Risk**: Two WFEs check lock simultaneously, both pass, both create PipelineRun.

**Mitigation**: Deterministic PipelineRun name. Kubernetes atomically rejects duplicate object creation.

**Confidence**: 100% - Kubernetes guarantees object name uniqueness.

---

### M2: Controller Restart

**Risk**: Lock state lost on restart.

**Mitigation**: Lock state is the PipelineRun itself. On restart:
- Running PipelineRuns still exist → lock still held
- Completed WFEs recalculate cooldown from `CompletionTime`

**Confidence**: 98%

---

### M3: PipelineRun Stuck Forever

**Risk**: PipelineRun never completes, lock held indefinitely.

**Mitigation**:
- Tekton Pipeline timeout (defined in workflow YAML)
- Tekton TTL cleanup (configurable, 1 hour default)

**Confidence**: 95%

---

### M4: PipelineRun Deleted Externally

**Risk**: Someone manually deletes PipelineRun, lock released unexpectedly.

**Mitigation**:
- RBAC restricts delete access to `kubernaut-workflows` namespace
- Audit trail via Kubernetes audit logs
- Not a safety issue - allows new remediation to proceed

**Confidence**: 90% (acceptable risk)

---

### M5: Hash Collision

**Risk**: Two different target resources produce same hash → same lock.

**Mitigation**: SHA256 with 16 hex characters = 2^64 combinations. Collision probability is negligible.

**Confidence**: 99.9999%

---

### M6: Finalizer Fails to Delete PipelineRun

**Risk**: WFE stuck in Terminating state.

**Mitigation**:
- Retry on next reconcile
- Log error for operator investigation
- Tekton TTL as final cleanup

**Confidence**: 95%

---

## Lifecycle Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                     LOCK LIFECYCLE                                   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  1. WFE Created (Pending)                                           │
│        │                                                             │
│        ▼                                                             │
│  2. Layer 1: Indexed CRD Query                                      │
│        │                                                             │
│        ├──► Blocked? → markSkipped("ResourceBusy")                  │
│        │                                                             │
│        ▼                                                             │
│  3. Layer 2: Create PipelineRun (deterministic name)                │
│        │                                                             │
│        ├──► AlreadyExists? → markSkipped("ResourceBusy")            │
│        │                                                             │
│        ▼                                                             │
│  4. Lock Acquired → markRunning()                                   │
│        │                                                             │
│        ▼                                                             │
│  5. PipelineRun Executes (Tekton)                                   │
│        │                                                             │
│        ▼                                                             │
│  6. PipelineRun Completes → markCompleted/markFailed()              │
│        │                                                             │
│        ▼                                                             │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │  COOLDOWN PERIOD (5 min default)                            │    │
│  │  PipelineRun still exists = lock still held                 │    │
│  │  New WFEs for same target → Skipped                         │    │
│  └─────────────────────────────────────────────────────────────┘    │
│        │                                                             │
│        ▼                                                             │
│  7. Cooldown Expires → Delete PipelineRun (lock released)           │
│        │                                                             │
│        ▼                                                             │
│  8. New WFE can now target same resource                            │
│                                                                      │
│  ═══════════════════════════════════════════════════════════════    │
│  ALTERNATIVE: WFE Deleted Before Cooldown                           │
│        │                                                             │
│        ▼                                                             │
│  9. WFE Finalizer → Delete PipelineRun (lock released immediately)  │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Edge Cases

| Scenario | Behavior | Result |
|----------|----------|--------|
| Two WFEs, same target, simultaneous | First `Create` wins, second gets `AlreadyExists` | ✅ One executes, one skipped |
| Controller restart during Running | PipelineRun exists, reconcile continues monitoring | ✅ No duplicate |
| Controller restart during Cooldown | Recalculate from `CompletionTime` | ✅ Cooldown preserved |
| WFE deleted during Running | Finalizer deletes PipelineRun (cancels execution) | ✅ Clean cleanup |
| WFE deleted during Cooldown | Finalizer deletes PipelineRun immediately | ✅ Lock released early |
| Different workflow, same target | Same deterministic name | ✅ Blocked (by design) |
| Same workflow, different target | Different deterministic name | ✅ Both allowed |

---

## Testing Requirements

| Test | Purpose | Type |
|------|---------|------|
| `pipelineRunName()` determinism | Same input → same output | Unit |
| `pipelineRunName()` uniqueness | Different inputs → different outputs | Unit |
| `checkResourceLock()` finds Running | Layer 1 blocks | Unit |
| `checkResourceLock()` finds Cooldown | Layer 1 blocks | Unit |
| Concurrent WFE creation | Layer 2 race prevention | Integration |
| Controller restart | Lock persistence | Integration |
| Finalizer cleanup | PipelineRun deleted | Integration |
| Full lifecycle | Create → Run → Complete → Cooldown → Release | E2E |

---

## Configuration

```yaml
# workflowexecution-config ConfigMap
resource_locking:
  cooldown_period: 5m          # Default: 5 minutes
  # Note: No additional config needed - lock is implicit in PipelineRun existence
```

---

## Related Documents

- [DD-WE-001: Resource Locking Safety](./DD-WE-001-resource-locking-safety.md) - Business requirements for locking
- [DD-WE-002: Dedicated Execution Namespace](./DD-WE-002-dedicated-execution-namespace.md) - Where PipelineRuns run
- [BR-WE-009-011: Resource Locking Requirements](../../requirements/BR-WE-009-011-resource-locking.md)

---

## Document Maintenance

| Date | Version | Changes |
|------|---------|---------|
| 2025-12-03 | 1.0 | Initial decision - deterministic name + indexed query |

