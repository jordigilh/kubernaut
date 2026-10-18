# DD-WE-010: Job Identity per RemediationRequest and WorkflowExecution Attempt

**Status**: Approved  
**Version**: 1.1
**Date**: 2026-10-17
**Issue**: [#2512](https://github.com/jordigilh/kubernaut/issues/2512)
**Correction**: [#2529](https://github.com/jordigilh/kubernaut/issues/2529)
**Related**: DD-RO-002, DD-WE-003, DD-AUDIT-010, BR-WE-009/010/011/014/019, BR-AUDIT-005

## Decision

Job-backed WorkflowExecutions use a deterministic name derived from the parent
RemediationRequest name and a bounded WorkflowExecution attempt suffix:

```text
wfe-<spec.remediationRequestRef.name>-<4-character WFE UID suffix>
```

The RR name is the operator-facing identity and distinguishes remediation
cycles; the suffix distinguishes multiple WFE attempts associated with the same
RR. The sanitized RR-name portion is bounded so the result remains within the
Kubernetes Job-name limit. A same-WFE reconciliation reuses the same name
idempotently.

The target resource is not part of Job identity. It remains the routing/locking
key owned by RemediationOrchestrator.

## Rationale

Target-only Job names caused sequential RRs for the same target to reuse a Job
and its retained Pods. Deleting the previous Job before creating the next one is
not a safe identity mechanism because retention and asynchronous cleanup are
independent lifecycle concerns.

The WFE name is stable for one Kubernetes object, while the RR name and UID are
copied by `WorkflowExecutionCreator` from the parent RR. Using the RR name keeps
the Job directly traceable to the RR shown by Kubernetes operators; the RR UID
remains in immutable metadata for provenance and audit correlation without
making retention settings part of identity.

## Ownership and metadata

- The Job name and the WFE `Status.ExecutionRef` are authoritative for status,
  idempotency, and cleanup.
- Job/Pod labels and annotations expose WFE/RR/workflow provenance for watch
  routing, querying, and forensics. They are mutable metadata and are not the
  ownership trust boundary.
- A direct Job owner reference to the WFE is not used because Jobs run in the
  dedicated execution namespace while WFEs/RRs normally run in the controller
  namespace. The existing WFE→RR owner reference remains authoritative.
- A same-name resource with mismatched expected identity or bundle/image fails
  closed; it is never adopted or deleted as stale.
- During migration, an existing WFE whose `Status.ExecutionRef` records the
  former RR-UID-based or target-derived name may continue to observe/clean that
  exact legacy Job only when its legacy owner metadata and exact bundle image
  match. New WFEs never generate legacy names.

## Locking, cooldown, and retention

- RemediationOrchestrator remains the active-target lock authority through its
  resource check and distributed Lease.
- WorkflowExecution retains its defense-in-depth cooldown check, scoped to the
  same workflow ID. A different workflow on the same target is not blocked by
  the prior workflow's Job identity.
- Job retention/TTL settings control cleanup and diagnostics only; they never
  influence the Job name.
- Cleanup uses the owning WFE's `Status.ExecutionRef` and never derives the Job
  solely from `spec.targetResource`.

## Alternatives rejected

| Alternative | Reason rejected |
|---|---|
| Target-hash-only Job name | Reuses retained Jobs/Pods across RR cycles |
| Random `GenerateName` | A lost create response can create duplicate Jobs for one WFE |
| RR UID-based Job name | Opaque to operators when the RR name already provides a direct, bounded identity |
| Separate WE Lease | Duplicates RO's lock authority and expands lifecycle/race complexity |
| Delete old Job before reuse | Makes identity depend on retention/GC timing |

## Control objectives

- FedRAMP AU-2/AU-3/AU-9/AU-11/AC-6/SC-5/SI-10
- SOC2 CC7.2; CC8.1 applies to the reviewed change process
- OWASP ASVS V4.1.1/V4.1.3, V5.1.x, V7.1.1/V7.2.1

## Implementation and verification

- `pkg/workflowexecution/executor/job.go` owns name construction, exact image
  projection, idempotency, and status-reference cleanup.
- `internal/controller/workflowexecution` uses the Job-specific name for
  existence/audit idempotency and does not delete terminal Jobs for Job-name
  collisions.
- Unit, EnvTest integration, RO-lock integration, audit reconstruction, and
  real-cluster E2E scenarios are defined in `docs/tests/2512/TEST_PLAN.md`.
