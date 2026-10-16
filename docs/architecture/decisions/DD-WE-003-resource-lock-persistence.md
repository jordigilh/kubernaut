# DD-WE-003: Resource Lock Persistence via Deterministic Naming

**Status**: Approved
**Version**: 1.4
**Date**: 2025-12-01
**Updated**: 2026-10-14
**Confidence**: 95%
**Author**: Kubernaut Architecture Team

---

## Decision Summary

The WorkflowExecution Controller uses deterministic execution-resource names for
backend idempotency, while **RemediationOrchestrator owns active-target locking**
through its routing checks and distributed Lease (DD-RO-002). Tekton retains its
target-derived compatibility name. Job-backed WorkflowExecutions use the
per-RR/per-WFE identity defined by DD-WE-010 so retained Jobs and Pods cannot be
reused across remediation cycles.

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

### Backend-specific deterministic identity

Tekton's compatibility identity remains derived from the target resource:

```
name = "wfe-" + sha256(targetResource)[:16]
```

For example, `targetResource = "default/deployment/my-app"` always produces `wfe-bd773c9f25ac4e1b`.

Job identity is not a target lock. It is:

```
name = "wfe-" + full(spec.remediationRequestRef.uid) + "-" + last-four(wfe.metadata.uid)
```

For a standard UUID this is 45 characters. A same-WFE retry reuses the same
name idempotently; a new RR/WFE cycle targeting the same resource receives a
different name. The Kubernetes API's `AlreadyExists` result is therefore a
same-WFE idempotency/collision signal, not a replacement permission.

### Lock Lifecycle

```
1. WFE created (Pending phase)
   |
2. RO routing/Lease grants active-target admission; exec.Create() creates the
   backend resource with engine-specific identity
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

### Pre-Execution Collision Handling

When `AlreadyExists` is returned during execution-resource creation:

- **Legacy target-derived collision**: a running resource can still be
  classified as `Deduplicated` for compatibility with pre-RR-UID fixtures.
  The WFE is marked as `Failed` with `FailureReason=Deduplicated` and
  `DeduplicatedBy=<originalWFE>` (Issue #190) via `MarkFailedAsDeduplicated`;
  no retained Job is deleted.
- **Job backend**: the executor verifies the expected name, WFE/RR provenance
  hints, and exact immutable bundle image. An exact same-WFE resource is treated
  as idempotent. A foreign, name-mismatched, or image-mismatched resource fails
  closed and is never deleted or adopted, regardless of terminal state or TTL.

#### Background GC and retention

Cleanup uses `DeletePropagationBackground` for the owning WFE's recorded
execution reference. Retention/TTL is not an input to Job identity, and no old
Job is deleted merely to make a new Job possible. This keeps completed/failed
diagnostics available while the next RR cycle creates its own resource.

This ensures sequential remediation cycles are not blocked by stale completed Jobs while concurrent execution prevention remains intact.

---

## Alternatives Considered

| Alternative | Rejected Because |
|------------|------------------|
| Random Job names | Weakens same-WFE idempotency; a lost create response could create duplicate Jobs |
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
| Tekton name generation | `pkg/workflowexecution/executor/tekton.go` | `ExecutionResourceName()` |
| Job creation | `pkg/workflowexecution/executor/job.go` | `Create()`, `buildJob()` |
| Job identity | `pkg/workflowexecution/executor/job.go` | `JobExecutionResourceName()` |
| AlreadyExists handler | `internal/controller/workflowexecution/workflowexecution_controller.go` | `reconcilePending()` |
| Job collision verification | `pkg/workflowexecution/executor/job.go` | same-WFE idempotency / fail-closed collision |
| Cooldown + cleanup | `internal/controller/workflowexecution/workflowexecution_controller.go` | `ReconcileTerminal()` |
| Job cleanup identity | `pkg/workflowexecution/executor/job.go` | `Cleanup()` via `Status.ExecutionRef` |

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-01 | Initial design: deterministic naming as atomic lock |
| 1.1 | 2026-03-14 | Added pre-execution cleanup of completed Jobs (Issue #374) |
| 1.2 | 2026-03-04 | Fixed background GC race with requeue-on-pending (Issue #383) |
| 1.3 | 2026-03-04 | Added execution-time dedup classification via MarkFailedAsDeduplicated, DeduplicatedBy field, and audit trail for inherited RO transitions (Issue #190) |
| 1.4 | 2026-10-14 | Job identity moved to full RR UID + WFE suffix; RO remains lock authority; retained Job replacement removed (Issue #2512, DD-WE-010) |
