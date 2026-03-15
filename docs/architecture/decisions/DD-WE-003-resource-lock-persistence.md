# DD-WE-003: Resource Lock Persistence via Deterministic Naming

**Status**: Approved
**Version**: 1.1
**Date**: 2025-12-01
**Updated**: 2026-03-14
**Confidence**: 95%
**Author**: Kubernaut Architecture Team

---

## Decision Summary

The WorkflowExecution Controller uses **deterministic execution resource naming** (based on target resource hash) as an atomic locking mechanism. Kubernetes API uniqueness enforcement prevents concurrent duplicate execution on the same target resource. This document defines the lock lifecycle, including cleanup semantics for completed Jobs that would otherwise block sequential remediation cycles.

---

## Context and Problem Statement

### Multi-Cycle Remediation

When the same target resource requires repeated remediation across different RR cycles (e.g., OOMKill -> fix -> external revert -> OOMKill), the controller must:

1. **Prevent concurrent execution**: Two WFEs must not run simultaneously on the same target
2. **Allow sequential execution**: After a remediation cycle completes, a new cycle must be able to run

### The Locking Problem

Without a locking mechanism, two concurrent WFEs targeting the same resource could both create execution resources (Jobs or PipelineRuns) and run conflicting workflows in parallel. Standard K8s optimistic concurrency on the WFE CR is insufficient because two WFEs are independent objects.

---

## Design Decision

### Deterministic Naming as Atomic Lock

Execution resource names are derived deterministically from the target resource:

```
name = "wfe-" + sha256(targetResource)[:16]
```

For example, `targetResource = "default/deployment/my-app"` always produces `wfe-bd773c9f25ac4e1b`.

When a second WFE targets the same resource, it generates the identical name. The Kubernetes API rejects the duplicate creation with `AlreadyExists`, which the controller interprets as "target resource is locked".

### Lock Lifecycle

```
1. WFE created (Pending phase)
   |
2. exec.Create() -> Job/PipelineRun created -> LOCK ACQUIRED
   |
3. Execution runs (Running phase)
   |
4. Execution completes (Completed/Failed phase)
   |
5. ReconcileTerminal enforces cooldown (default 5 min)
   |
6. exec.Cleanup() deletes Job/PipelineRun -> LOCK RELEASED
   |
7. LockReleased event emitted
```

### TTL as Fallback

Jobs set `ttlSecondsAfterFinished: 600` (10 minutes) as a secondary cleanup mechanism. If the controller fails to clean up (e.g., WFE CR deleted before ReconcileTerminal runs), the K8s TTL controller eventually removes the Job.

### Pre-Execution Cleanup of Completed Jobs (Issue #374)

When `AlreadyExists` is returned during Job creation, the controller checks the state of the existing Job:

- **Running Job**: The lock is valid. The WFE is marked as failed with "target resource locked". This preserves BR-WE-009 (prevent parallel execution).
- **Completed/Failed Job**: The lock is stale. The controller deletes the completed Job and retries creation. This handles cases where ReconcileTerminal hasn't run yet (cooldown), TTL hasn't fired, or the Job was orphaned.

#### Background GC Requeue (Issue #383)

Cleanup uses `DeletePropagationBackground`, so the API server accepts the delete but the Job object may still exist until the garbage collector removes it. If the immediate retry `Create` gets `AlreadyExists` again, the controller requeues (500ms) instead of permanently failing the WFE. The next reconciliation finds the Job gone and creation succeeds. This eliminates an intermittent race where the GC lag caused WFEs to be incorrectly marked as `Failed`.

This ensures sequential remediation cycles are not blocked by stale completed Jobs while concurrent execution prevention remains intact.

---

## Alternatives Considered

| Alternative | Rejected Because |
|------------|------------------|
| Random Job names | Removes locking semantics; two WFEs could run concurrently on the same target |
| External lock store (Redis, etcd) | Adds infrastructure dependency; K8s API already provides atomic uniqueness |
| Lower TTL only | TTL cleanup is asynchronous and not guaranteed to fire before the next cycle |

---

## References

- **BR-WE-009**: Resource Locking -- Prevent Parallel Execution
- **BR-WE-010**: Cooldown -- Prevent Redundant Sequential Execution
- **BR-WE-011**: Lock Release -- Prevent Permanent Resource Blocking
- **ADR-052**: Distributed Locking (race condition analysis)
- **ADR-002**: Native Kubernetes Jobs (TTL management)
- **Issue #374**: Job name collision on repeated remediation
- **Issue #383**: Background GC race in pre-execution cleanup

---

## Implementation

| Component | File | Function |
|-----------|------|----------|
| Name generation | `pkg/workflowexecution/executor/tekton.go` | `ExecutionResourceName()` |
| Job creation | `pkg/workflowexecution/executor/job.go` | `Create()`, `buildJob()` |
| AlreadyExists handler | `internal/controller/workflowexecution/workflowexecution_controller.go` | `reconcilePending()` |
| Pre-execution cleanup | `internal/controller/workflowexecution/workflowexecution_controller.go` | `handleJobAlreadyExists()` |
| Cooldown + cleanup | `internal/controller/workflowexecution/workflowexecution_controller.go` | `ReconcileTerminal()` |
| Job terminal check | `pkg/workflowexecution/executor/job.go` | `IsCompleted(ctx, targetResource, namespace)` |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-01 | Initial design: deterministic naming as atomic lock |
| 1.1 | 2026-03-14 | Added pre-execution cleanup of completed Jobs (Issue #374) |
| 1.2 | 2026-03-04 | Fixed background GC race with requeue-on-pending (Issue #383) |
