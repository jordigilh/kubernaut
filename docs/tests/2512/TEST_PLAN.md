# Test Plan: Per-WorkflowExecution Job Identity

> IEEE 829-2008 hybrid test plan for [issue #2512](https://github.com/jordigilh/kubernaut/issues/2512).

**Test Plan Identifier**: TP-2512-v1  
**Feature**: Prevent stale Kubernetes Job/Pod reuse across sequential same-target WorkflowExecutions.  
**Created**: 2026-10-14  
**Status**: Implemented — focused/unit scenarios pass; EnvTest/E2E execution is blocked or unrun as noted below  
**Branch**: `issue-2512-triage`

### Confirmed Job-name contract

```text
wfe-<RemediationRequestRef.name>-<4-character WFE UID suffix>
```

The RR name provides the operator-facing identity across RR cycles. The
four-character WFE suffix distinguishes multiple attempts under one RR. The
sanitized RR-name portion is bounded so the result remains within the
Kubernetes Job-name limit.

## 1. Purpose and objectives

The tests prove that a new WorkflowExecution/RR cycle receives and observes its
own Job execution resource and workflow bundle, even when an earlier completed or
failed Job is retained for diagnosis. They also prove that separating Job identity
from target locking does not weaken active-target safety or audit reconstruction.

Objectives:

1. Prove distinct WFEs targeting the same resource receive distinct, stable,
   Kubernetes-valid Job names.
2. Prove the Job container image, WFE/RR metadata, status reference, and audit
   payload all belong to the same immutable execution snapshot.
3. Prove retained terminal Jobs/Pods remain diagnosable and are never adopted,
   deleted, or reused by a later WFE merely because the target matches.
4. Prove same-workflow cooldown and RO active-target locking retain their safety
   behavior while a different workflow on the same target can proceed.

## 2. Authority and control mapping

### Business requirements

- BR-WE-009, BR-WE-010, BR-WE-011
- BR-WE-014, BR-WE-019
- BR-AUDIT-005

### Control objectives

| Test concern | FedRAMP | SOC2 | OWASP ASVS |
|---|---|---|---|
| Correct execution event/resource identity | AU-2, AU-3 | CC7.2 | V7.1.1, V7.2.1 |
| Retained evidence and no stale reuse | AU-9, AU-11 | CC7.2 | V4.1.1, V4.1.3 |
| Active target safety | SC-5, AC-6 | CC7.2 | V4.1.1, V4.1.3 |
| Input-derived Job names/metadata and exact immutable image | SI-10, AC-6 | CC7.2 | V5.1.x, V7.2.1 |
| Change process | — | CC8.1 | — |

CC8.1 is recorded as change-management evidence for this implementation plan;
runtime audit completeness is mapped to CC7.2 per the repository's corrected
SOC2 mapping.

OWASP ASVS control IDs in this plan are pinned to **ASVS v4.0.3** (V4 access
control, V5 validation/sanitization/encoding, and V7 error logging/auditing).

## 3. Risks driving test design

| ID | Risk | Priority | Tests |
|---|---|---:|---|
| R1 | A later WFE gets the earlier Job name/image | P0 | UT-WE-2512-001/003, IT-WE-2512-001/002, E2E-WE-2512-001 |
| R2 | Same-WFE retry creates a duplicate Job | P0 | UT-WE-2512-002, IT-WE-2512-003 |
| R3 | Old cleanup deletes or adopts a later Job | P0 | UT-WE-2512-004, IT-WE-2512-002 |
| R4 | Unique names remove the active target lock | P0 | IT-RO-2512-001 |
| R5 | Retention/TTL changes identity or removes evidence early | P0 | UT-WE-2512-005, IT-WE-2512-001/002 |
| R6 | Audit data conflates RR cycles | P0 | IT-WE-2512-005 |
| R7 | Name/metadata construction exceeds Kubernetes limits or leaks secret/output data | P1 | UT-WE-2512-006, IT-WE-2512-007 |

## 4. Scope

### In scope

- Job name generation and Job/Pod identity metadata.
- Exact `WorkflowRef.ExecutionBundle` projection.
- Same-WFE create idempotency and foreign-collision fail-closed behavior.
- Cleanup through `Status.ExecutionRef` and retention coexistence.
- Same-workflow cooldown behavior and RO active-target lock regression.
- Audit correlation/resource identity for sequential cycles.

### Out of scope

- Tekton PipelineRun naming or behavior; its target-deterministic path remains a
  separate executor contract unless the approved design decision expands scope.
- Ansible/AWX launch identity.
- New retention configuration, new RBAC, or new audit event types.
- Structural coverage-only tests for defensive branches without a business case.

## 5. Approach and pass criteria

- Tests use Ginkgo/Gomega; no `testing.T`, `Skip`, `XIt`, or `PIt`.
- Unit tests use fake clients only for external Kubernetes API behavior; business
  name/identity/image logic remains real.
- Integration tests use the real controller against EnvTest and no mocks for the
  production dispatch path.
- E2E uses the deployed Job backend and real Kubernetes Job/Pod behavior where the
  fixture can observe retained resources.
- Unit business-scenario coverage target: **≥90%**, with all P0/critical scenarios
  at **100%**. This is a scenario target, not a reason to add duplicate assertions.
- Integration wiring coverage: **100%** of the manifest rows in the implementation
  plan.
- E2E control-objective coverage: at least one passing journey for AU-9/AU-11,
  AU-3/CC7.2, AC-6/V4.1.x, and SC-5.

PASS requires all P0 scenarios, all critical control scenarios, affected package
tests, and no regression in existing executor/controller/RO suites. A failure due
to the pre-existing unrelated investigator changes or unavailable test
infrastructure must be recorded separately, not silently attributed to #2512.

Verification note: the focused RED run failed at the expected missing API
surface, then the Job executor/controller/RO unit suites and integration/E2E
package compilation passed after GREEN. The focused EnvTest attempt ran zero
specs because the shared DataStorage image build exhausted host disk space;
the E2E journey was not run.

## 6. Test items

### Unit-testable

| File | Scope |
|---|---|
| `pkg/workflowexecution/executor/job.go` | Job identity, metadata, exact image, idempotent create, cleanup name/owner logic |
| `internal/controller/workflowexecution/workflowexecution_pending.go` | Job resource-name selection and audit existence guard |
| `internal/controller/workflowexecution/workflowexecution_lifecycle.go` | Same-workflow cooldown filter, if the defense-in-depth gate is retained |

### Integration/E2E-testable

| File | Scope |
|---|---|
| `test/integration/workflowexecution/job_lifecycle_integration_test.go` | Real controller Job creation, status reference, retained sequential cycles |
| `test/integration/remediationorchestrator/locking_integration_test.go` | RO production active-target lock with unique Job identities |
| `test/e2e/workflowexecution/03_job_backend_test.go` | Real-cluster Job/Pod image and identity journey |
| WFE audit integration suite | RR-correlated audit reconstruction and exact execution name |

## 7. BR coverage matrix

| BR | Outcome | Tier | Scenario | Status |
|---|---|---|---|---|
| BR-WE-014/011 | Same target, distinct WFE identities produce distinct valid Job names | Unit | UT-WE-2512-001 | UT pass; IT blocked/unrun |
| BR-WE-014 | Same WFE reconcile is idempotent and does not create a second Job | Unit/IT | UT-WE-2512-002, IT-WE-2512-003 | UT pass; IT blocked/unrun |
| BR-WE-014 | Pre-existing WFE can read and clean its legacy target-named Job through Status.ExecutionRef | Unit | UT-WE-2512-011/012 | UT pass |
| BR-WE-014 | Later WFE image equals its exact bundle | Unit/IT/E2E | UT-WE-2512-003, IT-WE-2512-001, E2E-WE-2512-001 | UT pass; IT/E2E blocked/unrun |
| BR-WE-019 | Retained completed/failed resources remain diagnosable and are not reused | Unit/IT/E2E | UT-WE-2512-005, IT-WE-2512-001/002, E2E-WE-2512-001 | UT pass; IT/E2E blocked/unrun |
| BR-WE-009 | Active same-target execution remains blocked | Unit/IT | UT-WE-2512-008, IT-RO-2512-001 | Unit/compile pass; RO IT blocked |
| BR-WE-010 | Same workflow cooldown blocks; different workflow proceeds | Unit/IT | UT-WE-2512-009, IT-WE-2512-004 | UT pass; IT blocked |
| BR-AUDIT-005 | Each RR correlation reconstructs the correct Job and image | IT/E2E | IT-WE-2512-005, E2E-WE-2512-001 | Tests added; IT/E2E blocked/unrun |
| BR-AUDIT-005 | Repeated lifecycle writes use one durable logical audit event and isolate recreated RR UIDs | Unit/IT | UT-AUDIT-2512-001, UT-DS-AUDIT-2512-001/002/003/004 | Unit pass; DB runtime pending |

## 8. Scenarios

### Unit scenarios

| ID | Business behavior and control objective | Priority |
|---|---|---:|
| **UT-WE-2512-001** | The RR name is present in the stable Job name, a 4-character WFE suffix distinguishes same-RR attempts, and the name remains DNS-label valid (BR-WE-014/011, SI-10, ASVS V5.1.x) | P0 |
| **UT-WE-2512-002** | Recreating the same WFE after a lost status update returns/recognizes the same Job without creating or deleting a second resource (BR-WE-014, AC-6, ASVS V4.1.3) | P0 |
| **UT-WE-2512-003** | Job container image equals the immutable WFE bundle exactly; WFE/RR labels and annotations identify the same execution (AU-3, CC7.2, ASVS V7.1.1) | P0 |
| **UT-WE-2512-004** | Cleanup uses `Status.ExecutionRef` and refuses a Job whose WFE identity/UID does not match (AU-9, AC-6, ASVS V4.1.1/V4.1.3) | P0 |
| **UT-WE-2512-005** | Changing `RetainFailedExecutions` changes only TTL/cleanup policy, never the Job name or image (AU-9/AU-11) | P0 |
| **UT-WE-2512-006** | Long target/RR names are bounded/sanitized while full provenance remains in annotations; no secret or workflow output is copied to labels (SI-10, ASVS V5.1.x/V7.2.1) | P1 |
| **UT-WE-2512-007** | A foreign Job occupying the new identity fails closed and is never deleted/adopted (AC-6, ASVS V4.1.3) | P0 |
| **UT-WE-2512-008** | Active target lock behavior remains represented by the RO lock contract; a unique Job name is not treated as authorization to run in parallel (SC-5, CC7.2) | P0 |
| **UT-WE-2512-009** | Same workflow+target is cooldown-blocked, while a different workflow+target is not blocked solely by the prior workflow (BR-WE-010, SC-5) | P0 |
| **UT-WE-2512-010** | A same-image Job with a changed deterministic execution template fails closed rather than being adopted (AC-6, ASVS V4.1.3) | P0 |
| **UT-WE-2512-011** | A pre-existing WFE with RR UID but a legacy target-derived Status.ExecutionRef can still read its completed Job (BR-WE-014, AU-9, CC7.2) | P0 |
| **UT-WE-2512-012** | A pre-existing WFE can clean its legacy target-named Job only when the recorded name, legacy owner metadata, and exact image match (BR-WE-014, AC-6, ASVS V4.1.3) | P0 |
| **UT-WE-2512-019** | A pre-existing WFE with an RR-UID-based Status.ExecutionRef can still read its completed Job after the readable RR-name contract is introduced (BR-WE-014, AU-9, CC7.2) | P0 |
| **UT-WE-2512-013** | Long WFE names route by full UID rather than truncated mutable labels (SI-10, AC-4/AC-6) | P1 |
| **UT-WE-2512-014** | Controller-side execution-resource reads route through the configured local/remote ClientFactory (BR-FLEET-054, FedRAMP AC-4/AC-6) | P0 |
| **UT-WE-2512-015** | Legacy Job status reads fail closed on wrong owner or bundle image (AU-3, AU-9, AC-6) | P0 |
| **UT-WE-2512-016** | A terminal WFE retries its missing terminal audit after the phase was persisted and records success before cooldown cleanup (AU-2/AU-3, CC7.2) | P0 |
| **UT-WE-2512-017** | Credential-like workflow parameters are redacted in started/completed audit payloads while non-sensitive parameters remain reconstructable (BR-AUDIT-005, AU-3/AU-9/AU-11, ASVS V7.1.1/V7.2.1) | P0 |
| **UT-WE-2512-018** | Job pending validation rejects malformed OCI bundles, malformed target identities, and missing RR UIDs on new Job WFEs (BR-WE-014, SI-10, ASVS V5.1.x) | P0 |
| **UT-AUDIT-2512-001** | Repeated WorkflowExecution lifecycle emission derives the same deterministic event ID for one WFE/action and different IDs for different actions (AU-2/AU-3, CC7.2) | P0 |
| **UT-AUDIT-2512-002** | WorkflowExecution audit payload carries generic execution reference and immutable RR UID (AU-3, CC7.2) | P0 |
| **UT-DS-AUDIT-2512-001** | DataStorage preserves a client event ID through request conversion for durable deduplication (AU-2/AU-3, CC7.2) | P0 |
| **UT-DS-AUDIT-2512-002** | Single-event DataStorage replay returns the existing hash-chain state and does not insert a second row (AU-2/AU-3/AU-9) | P0 |
| **UT-DS-AUDIT-2512-003** | Batch DataStorage replay preserves the current chain head for following new events (AU-2/AU-3/AU-9) | P0 |
| **UT-DS-AUDIT-2512-004** | Reconstruction rejects mixed RR UIDs under one recreated RR name instead of merging forensic lifecycles (AU-3, CC7.2) | P0 |

### Integration scenarios

| ID | Business behavior and control objective | Priority |
|---|---|---:|
| **IT-WE-2512-001** | WFE1 completes with bundle A and its Job remains observable; WFE2 targets the same resource with bundle B and receives a second Job whose image is exactly B and whose status ref differs (AU-9/AU-11, CC7.2) | P0 |
| **IT-WE-2512-002** | WFE1 fails with retention enabled; WFE2 with a different bundle creates its own Job without deleting/adopting WFE1's failed Job/Pods (AU-9/AU-11, AC-6, ASVS V4.1.3) | P0 |
| **IT-WE-2512-003** | Production controller re-entry after Job creation is idempotent: one WFE, one Job, one execution reference, and no duplicate start/audit identity (AU-2/AU-3, CC7.2) | P0 |
| **IT-WE-2512-004** | Production pending flow allows a different workflow on the same target and still blocks the same workflow during cooldown (BR-WE-010, SC-5) | P0 |
| **IT-WE-2512-005** | Audit queries by RR correlation return the correct WFE/Job/image for both sequential cycles, with no cross-cycle resource identity (AU-2/AU-3, CC7.2) | P0 |
| **IT-RO-2512-001** | RO's real active-target check and distributed lock prevent parallel same-target WFE creation even though candidate Job names are unique (BR-WE-009, SC-5, ASVS V4.1.1) | P0 |

### E2E scenario

| ID | Business behavior and control objective | Priority |
|---|---|---:|
| **E2E-WE-2512-001** | On a real cluster, sequential same-target Job workflows with different bundles produce separate Jobs/Pods, exact images, and inspectable WFE/RR provenance; prior retained resources are not reused (AU-3/AU-9/AU-11, CC7.2, ASVS V4.1.x/V7.1.1) | P0 |

## 9. Detailed P0 acceptance cases

### IT-WE-2512-001 — retained completed Job

**Given** WFE1 targets resource R with bundle A and reaches Completed while its
Job remains available for diagnosis.  
**When** a new WFE/RR2 targets R with a different workflow ID and bundle B.  
**Then** the controller creates a Job with a different name, exact image B,
WFE/RR2 metadata, and WFE2 `Status.ExecutionRef` points to it. The old Job and
its Pods are not selected, deleted, or used for WFE2.

### IT-WE-2512-002 — retained failed Job

**Given** WFE1 fails and failed-resource retention is enabled.  
**When** WFE2/RR2 targets the same R with bundle B.  
**Then** WFE2 creates a distinct Job and exact image B while WFE1's failed Job
remains owned by WFE1 for diagnosis. A cleanup attempt for WFE1 cannot delete
WFE2's Job.

### IT-RO-2512-001 — lock preservation

**Given** RO has created an active WFE1 for target R.  
**When** a second RR for R is processed before WFE1 becomes terminal.  
**Then** RO reports the existing resource-lock/blocked outcome and does not create
a second active WFE, regardless of the fact that a unique Job name is available.

### IT-WE-2512-005 — audit reconstruction

**Given** two sequential RR correlations produce two WFE/Job cycles.  
**When** the audit store is queried by each RR correlation ID.  
**Then** each lifecycle contains the matching WFE, workflow ID, exact bundle/image,
and execution Job name; no event from the other cycle is returned or substituted.

## 10. Test pyramid and stopping rule

- **UT proves logic**: identity, metadata, exact image, ownership, retention
  independence, and cooldown decision behavior.
- **IT proves wiring**: real WFE controller dispatch, status reference, cleanup,
  audit persistence, and real RO locking.
- **E2E proves the journey**: real Job/Pod creation and stale-resource separation.

Do not add tests solely to raise a line percentage. Once the twenty-two UT behavior
scenarios, six IT wiring/control scenarios, and the E2E journey pass, all critical
contracts are covered, and the changed-code unit scenario target is ≥90%, stop
coverage expansion. If the real-cluster fixture cannot retain both resources
without unrelated infrastructure changes, document that boundary and retain the
IT proof; do not weaken assertions or serialize unrelated suites.

## 11. Required validation

```text
go test ./pkg/workflowexecution/executor/...
go test ./internal/controller/workflowexecution/...
go test ./pkg/remediationorchestrator/locking/... ./pkg/remediationorchestrator/routing/...
go build ./...
golangci-lint run --timeout=5m
make test
make test-integration-workflowexecution
```

Run the focused Ginkgo IDs first during RED/GREEN; run the broader commands at
verification. Record unrelated baseline failures separately.
