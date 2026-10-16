# Implementation Plan: Per-WorkflowExecution Job Identity

> Draft plan for [Kubernaut issue #2512](https://github.com/jordigilh/kubernaut/issues/2512).

**Plan Identifier**: IP-2512-v1  
**Feature**: Prevent Kubernetes Job reuse across sequential WorkflowExecution/RR cycles while preserving target locking, retention, provenance, and audit reconstruction.  
**Created**: 2026-10-14  
**Status**: Implemented — focused/unit verification complete; EnvTest execution blocked by host disk exhaustion during shared DataStorage image setup  
**Branch**: `issue-2512-triage` (based on `origin/main`)

## 1. Outcome

Each Job-backed WorkflowExecution receives a deterministic, Kubernetes-valid name
derived from the full parent RemediationRequest UUID and a 4-character WFE
attempt suffix, for example:

```text
wfe-550e8400-e29b-41d4-a716-446655440000-7a3c
```

The first segment is the full `spec.remediationRequestRef.uid` UUID and the second
is a 4-character suffix from the WFE UID/attempt identity. The resulting name is
45 characters, below the Kubernetes Job-name limit. The identity is stable
across reconciliation retries for the same WFE, but differs for every new WFE/RR
cycle. A later WFE therefore cannot attach to a prior Job or
its Pods merely because it targets the same resource. The Job image is copied
exactly from the later WFE's immutable `spec.workflowRef.executionBundle`.

Target locking remains owned by RemediationOrchestrator (DD-RO-002): the Job name
is no longer used as the lock. Same-workflow cooldown remains enforced; a
different workflow on the same target is not blocked by a stale Job identity.

## 2. Business Requirements and Control Objectives

### Business requirements

- **BR-WE-009**: prevent parallel execution on the same target through RO's active-WFE/resource-lock path.
- **BR-WE-010**: block only the same workflow+target during cooldown; allow a different workflow on the same target.
- **BR-WE-011**: preserve target-resource identity and fleet-cluster scoping.
- **BR-WE-014**: create, monitor, and clean up the correct Job execution backend resource.
- **BR-WE-019**: preserve failed execution diagnostics and retention behavior without making retention part of identity.
- **BR-AUDIT-005**: keep the execution lifecycle reconstructable by RR correlation and retain the exact execution-resource identity.

### Control objectives covered by behavior tests

| Control family | Objective applied to this change | Proof planned |
|---|---|---|
| FedRAMP AU-2/AU-3 | Started/completed/failed evidence contains the correct WFE, RR, workflow, image, and Job identity | UT + IT audit reconstruction |
| FedRAMP AU-9/AU-11 | Retained completed/failed Jobs are not deleted or reused solely to make a later execution possible | UT + IT retained-resource journeys |
| FedRAMP AC-6 | A WFE can only adopt/clean up its own Job; a foreign Job collision fails closed | UT + IT ownership tests |
| FedRAMP SC-5 | Active target locking still prevents parallel execution after Job names become unique | RO IT regression |
| FedRAMP SI-10 | Job identity and metadata are generated from validated, immutable WFE/RR inputs; no arbitrary name/image mutation | UT builder validation |
| SOC2 CC7.2 | Monitoring/forensic reconstruction distinguishes every RR cycle and execution resource | IT audit query by correlation ID |
| SOC2 CC8.1 | Plan approval, RED/GREEN/REFACTOR, and regression evidence provide change-management traceability | Review/process evidence; not a runtime event mapping |
| OWASP ASVS V4.1.1/V4.1.3 | Ownership and authorization checks cannot adopt or delete another WFE's Job | UT + IT foreign-owner scenarios |
| OWASP ASVS V5.1.x | Generated names/labels are bounded and Kubernetes-valid; immutable inputs are not replaced by untrusted values | UT name/metadata scenarios |
| OWASP ASVS V7.1.1/V7.2.1 | Operational identity and collision decisions are observable without exposing workflow output or credentials | UT/IT metadata and log/event assertions |

All OWASP ASVS identifiers above refer to **ASVS v4.0.3**; the version is
explicit so future ASVS revisions do not silently change the control baseline.

AC-4 and SC-8 are unchanged by this local naming change; they remain covered by
the existing fleet/client-factory suites rather than receiving speculative tests.

## 3. Preflight Findings

### Current defect path

1. `pkg/workflowexecution/executor/tekton.go:ExecutionResourceName` hashes only
   `spec.targetResource`.
2. `pkg/workflowexecution/executor/job.go:buildJob` uses that target-only name,
   so every WFE for a target shares the same Job name.
3. `JobExecutor.Cleanup` also derives the name from the target instead of using
   the WFE's recorded `Status.ExecutionRef`.
4. `internal/controller/workflowexecution/workflowexecution_pending.go` computes
   the same target-only name for existence/audit idempotency.
5. `internal/controller/workflowexecution/workflowexecution_collision.go` deletes
   a terminal Job on `AlreadyExists` and retries. This is the behavior that lets
   stale identity and retained Pods become relevant; it must not remain the normal
   Job path after names are WFE-specific.
6. Job labels currently identify the WFE and source namespace, but not the RR
   identity or WFE UID. The WFE itself already carries the RR owner reference.

### Existing safety boundary

`pkg/remediationorchestrator/routing.FindActiveWFEForTarget` and
`RoutingEngine.CheckResourceBusy` already enforce active-target locking, with the
RO distributed Lease covering concurrent WFE creation. `checkRecentlyRemediated`
already matches the same workflow ID. This is the production lock path to retain;
the target-hash Job name is an accidental second lock, not the current authority
per DD-RO-002.

### Integration points

- `cmd/workflowexecution/main.go:buildExecutorRegistry` already constructs the
  Job executor; no new production entry point or dependency is required.
- `WorkflowExecutionCreator` creates `we-<RR name>` WFEs and sets the existing
  WFE→RR controller owner reference.
- Jobs are created in the dedicated execution namespace, normally different from
  the WFE/RR namespace. A direct Job owner reference to a namespaced WFE would
    not be a valid cross-namespace garbage-collection relationship; labels and
    annotations carry provenance for routing/querying, while the immutable Job
    name and WFE→RR owner reference remain authoritative.

### Baseline

Passed on `origin/main` before implementation:

```text
go test ./pkg/workflowexecution/executor/... ./internal/controller/workflowexecution/...
```

The prepared worktree was verified clean of the unrelated investigator
modifications before production implementation. Only the #2512 plan/DD/tests
and implementation changes listed in this worktree are in scope.

## 4. Spikes and Decisions

### Spike 1 — Reproduce the identity collision path: YES

**Evidence**: `buildJob`, `Cleanup`, pending existence checks, and the Job
`AlreadyExists` handler all derive identity from the target hash. The existing
Job tests and integration tests assert the same deterministic name.

**Decision**: change Job identity; do not attempt to make deletion timing solve
the collision.

### Spike 2 — Can Job identity be decoupled from target locking: YES

**Evidence**: RO checks active WFEs by target and execution cluster, and its
Lease protects concurrent WFE creation. RO also performs workflow-specific
cooldown. DD-RO-002 explicitly assigns routing to RO and execution to WE.

**Decision**: keep RO's lock as the authoritative concurrency guard. Add a
regression test at the RO production path; do not create a second WE Lease or
external lock store.

### Spike 3 — Can a same-WFE reconcile remain idempotent: YES

**Evidence**: a WFE has an immutable Kubernetes UID and immutable spec. A name
derived from the full RR UID plus WFE UID suffix is stable for retries of that WFE and unique to
new WFE/RR attempts. `Status.ExecutionRef` becomes the canonical cleanup/status
reference after creation.

**Decision**: use deterministic full-RR-UUID-plus-4-character-WFE-suffix naming,
    not random `GenerateName`; treat a same-owner `AlreadyExists` as idempotent
    and never delete it.

### Spike 4 — Can retained old Jobs coexist safely: YES

**Evidence**: Job names are namespace-scoped and the old WFE's `Status.ExecutionRef`
   identifies its resource. Retention/TTL only controls cleanup; it need not
   participate in the new Job name. Existing controller finalization can clean
   each WFE's recorded Job independently.

**Decision**: no pre-create deletion for the Job backend. A foreign existing Job
   is a fail-closed collision, not a stale resource to remove.

### Spike 5 — Cross-namespace owner identity: YES, with labels/annotations

**Evidence**: WFE/RR resources are created in the controller namespace while Jobs
   run in `ExecutionNamespace`. Existing watch routing already uses
   `workflow-execution` and `source-namespace` labels.

**Decision**: add WFE UID and RR name/namespace/UID labels plus full-value
    annotations as mutable routing/query hints. Do not add an invalid
    cross-namespace Job owner reference; use the immutable name and status
    reference for ownership decisions.

### Spike 6 — Existing package behavior: PASS

The affected executor and controller packages pass their current tests. The
semantic Engram index was unavailable twice due to a shared workspace lock and
Engram reflection failed because the configured Vertex AI dependency is absent;
exact Engram symbol/reference/search tools remained available and were used for
the discovery above.

## 5. Alternatives

| Option | Summary | Result |
|---|---|---|
| **A — full-RR-UUID Job name + RO lock (recommended)** | Use the full RR UUID plus a 4-character WFE UID/attempt suffix; add WFE/RR metadata hints; use `Status.ExecutionRef`; remove Job stale replacement | Matches issue #2512 with no new API/dependency and follows DD-RO-002 |
| B — random `GenerateName` | Let the API generate a new Job every create | Rejected: a lost create response/status update can create duplicate Jobs for one WFE; weak idempotency and audit linkage |
| C — separate WE target Lease/lock | Keep unique Jobs but add a second Lease implementation to WE | Rejected: duplicates RO's lock authority, increases race/cleanup surface, and violates routing ownership |
| D — retain target-only name and improve deletion/waiting | Make old Job replacement more reliable | Rejected: still permits stale identity/POD association and explicitly violates “do not rely on deleting old Jobs” |

**Decision recorded**: Option A is approved for implementation. Because this
changes the relationship between Job identity and DD-WE-003's historical lock
description, implementation should add/update a design-decision entry (proposed
`DD-WE-010`) and reference it from the code/docs.

## 6. Proposed Scope

### Production code

- `pkg/workflowexecution/executor/job.go`
  - Add `JobExecutionResourceName(wfe)` using the full
    `wfe.Spec.RemediationRequestRef.UID` plus a 4-character WFE UID/attempt
    suffix. Use a stable RR-name fallback for objects with no RR UID, and accept
    an existing WFE's recorded legacy target-derived `Status.ExecutionRef` only
    during migration with exact image/owner corroboration.
  - Build Job and Pod metadata with WFE UID and RR name/namespace/UID.
  - Use exact `WorkflowRef.ExecutionBundle` as the container image.
  - Make same-WFE create idempotent and foreign collisions fail closed.
  - Make cleanup prefer `Status.ExecutionRef.Name`; retain a safe legacy fallback
    only for pre-existing WFEs with no execution reference.
- `pkg/workflowexecution/executor/executor.go`
  - Replace the generic “all execution resources are target-deterministic”
    contract comment with engine-specific identity semantics.
- `internal/controller/workflowexecution/workflowexecution_pending.go`
  - Compute the Job existence/audit key with the new Job helper.
  - Scope any remaining WE cooldown guard by workflow ID, or remove the duplicate
    routing gate if the approved DD-WE-010 chooses full DD-RO-002 convergence.
- `internal/controller/workflowexecution/workflowexecution_lifecycle.go`
  - Update `CheckCooldownActive` signature/logic only if the approved design
    keeps the WE defense-in-depth guard; different workflow IDs must not be
    blocked by the same-workflow cooldown.
- `internal/controller/workflowexecution/workflowexecution_collision.go`
  - Remove target-based stale Job deletion/recreation. Keep Tekton collision
    behavior unchanged. Foreign Job identity collisions must not delete a Job.
- `docs/architecture/decisions/DD-WE-003-resource-lock-persistence.md`,
  `docs/architecture/DESIGN_DECISIONS.md`, and the WorkflowExecution service
  docs: document Job identity versus RO lock ownership and the cross-namespace
  metadata chain.
- `api/openapi/data-storage-v1.yaml`, generated DataStorage client, audit
  conversion/repository paths, and `pkg/workflowexecution/audit`: add
  deterministic audit event IDs and generic execution/RR-UID payload identity
  per DD-AUDIT-010.

### Test code

- Extend `pkg/workflowexecution/executor/job_test.go` for identity, exact image,
  metadata, retention independence, idempotency, and owner-safe cleanup.
- Update `pkg/workflowexecution/reconcile_pending_orchestration_test.go` and
  controller collision tests for the new Job key and same/different workflow
  cooldown contract.
- Replace target-name assumptions and add sequential retained-resource journeys
  in `test/integration/workflowexecution/job_lifecycle_integration_test.go`.
- Add/extend the RO production locking proof in
  `test/integration/remediationorchestrator/locking_integration_test.go`.
- Extend `test/e2e/workflowexecution/03_job_backend_test.go` with a real-cluster
  same-target, different-bundle identity journey if the existing fixture can
  keep both Jobs observable; otherwise document the approved IT-only boundary
  and link the demo-scenario reproduction as external E2E evidence.

## 7. Wiring Manifest

| Component | Production entry point | Wiring location | IT proof |
|---|---|---|---|
| Per-WFE Job identity | Job executor registry → `Executor.Create` | `cmd/workflowexecution/main.go:buildExecutorRegistry`, `pkg/workflowexecution/executor/job.go` | `IT-WE-2512-001` |
| Exact bundle/metadata projection | WFE pending reconciliation → Job creation | `internal/controller/workflowexecution/workflowexecution_pending.go` → `JobExecutor.Create`/`buildJob` | `IT-WE-2512-002` |
| Status-ref cleanup | WFE terminal/finalizer reconciliation | `workflowexecution_lifecycle.go` → `JobExecutor.Cleanup` | `IT-WE-2512-003` |
| Same-workflow cooldown guard | Pending WFE reconciliation | `checkPendingCooldownOrBlock` → `CheckCooldownActive` | `IT-WE-2512-004` |
| Target lock preservation | RO post-analysis routing | `RoutingEngine.CheckResourceBusy` and distributed Lease | `IT-RO-2512-001` |
| Audit reconstruction | WFE terminal/start callbacks | `AuditManager.RecordExecutionWorkflowStarted` and terminal audit path | `IT-WE-2512-005` |
| Durable audit idempotency | DataStorage audit request/repository | OpenAPI `event_id`, conversion, single/batch conflict handling | `UT-DS-AUDIT-2512-001` |

## 8. TDD Sequence

### RED

Write the failing Ginkgo/Gomega tests in `docs/tests/2512/TEST_PLAN.md` before
production edits. The P0 tests must assert business outcomes, not helper calls:

1. Full RR UUID plus WFE suffix → distinct Job names for separate RR/attempt identities.
2. Same WFE retry → same Job identity and no second Job.
3. Later WFE → exact later bundle image, WFE/RR identity metadata, and separate
   `Status.ExecutionRef`.
4. Retained completed/failed prior Job remains diagnosable and is not deleted or
   reused to create the later Job.
5. Same workflow cooldown remains blocked; different workflow is allowed.
6. RO still blocks concurrent same-target WFEs.
7. Audit records for each RR correlation identify the matching execution resource.

### GREEN

Implement the smallest code changes that make RED pass, wire through the existing
Job executor registry, and immediately run CHECKPOINT W for every manifest row.
Do not add a new lock service or make retention a name input.

### REFACTOR

Name the concrete cleanup: centralize Job identity/metadata construction, remove
obsolete target-based Job cleanup/collision deletion, preserve explicit error
handling and secret-safe observability, then run the mandatory build and affected
tests. REFACTOR is not merely “tests pass”.

## 9. Success Criteria and Risk Mitigation

- Every new WFE/RR cycle gets a distinct valid Job name; same-WFE re-entry is stable.
- The later Job image equals the later WFE bundle byte-for-byte.
- Job and Pod metadata identify WFE UID plus RR name/namespace/UID without secrets.
- Old terminal Jobs/Pods are not deleted or adopted by a later WFE merely to create
  the new Job; cleanup uses the owning WFE's execution reference.
- Same-target active lock and same-workflow cooldown remain safe; different workflow
  is not incorrectly blocked by target-only WE cooldown.
- Audit queries by RR correlation reconstruct the correct execution resource and
  workflow image for each cycle.
- P0 UT/IT/E2E scenarios pass; in-scope UT business scenarios ≥90%, critical ones
  100%; no coverage-only tests are added.
- `go build ./...`, affected tests, full required test commands, lint, and the
  anti-pattern scan pass without attributing unrelated baseline failures to this change.

| Risk | Mitigation | Proof |
|---|---|---|
| Unique Job names bypass active locking | Keep RO lock authority and test the real RO path | `IT-RO-2512-001` |
| Lost create response creates a duplicate | Deterministic WFE-identity name plus same-owner idempotency | `UT/IT-WE-2512-002` |
| Old WFE cleanup deletes a later Job | Cleanup uses `Status.ExecutionRef` and owner UID, never target-only lookup | `UT/IT-WE-2512-003` |
| Name length/collision | Full RR UUID plus 4-character WFE suffix, Kubernetes-name validation, fail-closed foreign collision | `UT-WE-2512-004` |
| Retention changes identity | Name helper does not receive retention options; run both retention modes | `UT/IT-WE-2512-005` |
| Audit trail conflates cycles | RR correlation plus exact execution-resource payload assertions | `IT-WE-2512-005` |
| Existing WFE references a legacy target-named Job | Accept only the recorded legacy name with exact image/owner corroboration; reject arbitrary references | `UT-WE-2512-011/012` |

## 10. Risk Spike Reassessment

- **RR UID propagation: PASS** — production `WorkflowExecutionCreator` copies
  `rr.UID` into `spec.remediationRequestRef.uid`; direct test fixtures need only
  retain the documented fallback when they omit an assigned UID.
- **Name length: PASS** — full UUID (36) + prefix/separator (5) + four-character
  WFE suffix produces a 45-character Job name, below the Kubernetes Job limit.
- **Retention independence: PASS by inspection and existing tests** — retention
  options affect TTL/cleanup behavior but are not inputs to resource identity;
  existing fullpipeline fixtures already combine retention with an extended
  cooldown so terminal resources remain observable.
- **Affected package regression: PASS** — `pkg/workflowexecution/...`,
  `pkg/remediationorchestrator/locking/...`, and
  `pkg/remediationorchestrator/routing/...` all pass their current tests.
- **Legacy identity migration: PASS** — RED tests for status lookup and cleanup
  through an existing target-derived `Status.ExecutionRef` now pass without
  weakening the new identity path.
- **E2E retained-resource fixture: OPEN** — not run during triage; it remains a
  focused implementation/verification risk rather than a design blocker.
- **Worktree isolation: PASS** — unrelated investigator modifications were
  discarded from this prepared worktree before production edits; they remain
  untouched outside this task.

## 11. Approval and Confidence

**Reassessed confidence: 96%** for preflight/design; **94%** for implementation
readiness after focused verification.

Justification: the selected identity contract is explicit and bounded: full RR
UUID for cross-cycle uniqueness plus a four-character WFE suffix for same-RR
attempts. Production propagation, Kubernetes length, retention independence,
RO lock preservation, cooldown scoping, exact image/provenance projection,
foreign-collision fail-closed behavior, and affected package behavior are now
implemented and evidenced by passing focused/unit suites and build validation.
Remaining risk is execution of the real EnvTest/DataStorage retained-resource
journeys: the attempted run stopped before any spec because the host ran out
of disk while building the shared DataStorage image. E2E remains unrun.
