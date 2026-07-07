# Test Plan: Job Resource Governance and Transient-Failure Tolerance

**Test Plan Identifier**: TP-1572-v1.5
**Feature**: Per-workflow `execution.resources` (Job engine) + `PodFailurePolicy` transient-failure tolerance + audit retry-count completeness
**Version**: 1.5
**Created**: 2026-07-06
**Author**: WorkflowExecution Team
**Status**: Draft
**Branch**: `feature/dd-we-008-job-resources-podfailurepolicy`

---

## 1. Introduction

### 1.1 Purpose

Validates BR-WE-019 / DD-WE-008: workflow authors can declare per-workflow CPU/memory
requests/limits for the Job execution engine, and Job pods tolerate infrastructure-caused
transient failures (OOM-kill, node eviction) without weakening fail-fast behavior for genuine
remediation-script failures.

### 1.2 Objectives

1. **Schema round-trip correctness**: `execution.resources` YAML parses correctly into a typed
   `corev1.ResourceRequirements` via `resource.ParseQuantity()` (DD-WE-008 Finding 1), for both
   quoted and unquoted quantity forms.
2. **Registration-time fail-fast**: invalid quantities, `requests > limits`, and
   `resources` declared for a non-`job` engine are all rejected at DS registration time with a
   `SchemaValidationError`, not deferred to Job-admission time.
3. **End-to-end resolution**: a workflow's declared `resources` reaches the Job's `"workflow"`
   container unchanged, through `WorkflowQuerier` → `WFE.Status.Resources` → `buildJob()`.
4. **Backward compatibility**: workflows without `execution.resources` produce byte-identical
   Job specs to today (no `Resources` field set on the container).
5. **PodFailurePolicy correctness**: the Job manifest `buildJob()` produces carries the exact
   `Ignore` rules for OOM-kill (exit 137) and `DisruptionTarget`, with all other failures
   falling through to default `Count` behavior against the existing `backoffLimit: 0`.
6. **Real-cluster proof (Pyramid Invariant, BR-WE-019 AC9)**: `PodFailurePolicy`'s `Ignore`
   semantics are proven on a real Kubernetes cluster (real kubelet, real Job controller), not
   only structurally against envtest — envtest does not run the controller-loop evaluation
   that actually applies `Ignore` vs. `Count`.
7. **Audit-trail retry-count completeness (BR-WE-019 AC10)**: a remediation that required
   tolerating N AC4 pod-failure attempts before succeeding carries `retry_count: N` in its
   `workflow.completed` audit event — distinguishing it from a clean first-attempt success.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|---|---|---|
| Unit test pass rate | 100% | `go test ./pkg/datastorage/... ./pkg/workflowexecution/...` |
| Integration test pass rate | 100% | `go test ./test/integration/workflowexecution/...` |
| E2E test pass rate | 100% | `make test-e2e-workflowexecution` (job backend suite) |
| Backward compatibility | 0 regressions | All existing `UT-WE-054-JOB-*`, `UT-DS-006-*`, `UT-WE-650-*`, `IT-WE-014-*`, `E2E-WE-014-*` tests pass unmodified |

---

## 2. References

### 2.1 Authority

- [BR-WE-019: Job Resource Governance and Transient-Failure Tolerance](../../requirements/BR-WE-019-job-resource-governance-transient-failure-tolerance.md)
- [DD-WE-008: Job Resource Governance and Transient-Failure Tolerance](../../architecture/decisions/DD-WE-008-job-resource-governance-transient-failure-tolerance.md)
- Issue #1572 (implementation tracking), #1564 (originating issue)

### 2.2 Cross-References

- [DD-WE-006: Schema-Declared Dependencies](../../architecture/decisions/DD-WE-006-schema-declared-dependencies.md) — extraction pattern reused
- [BR-WE-016: Engine Config Discriminator](../../requirements/BR-WE-016-engine-config-discriminator.md) — on-demand `content` extraction pattern reused (not the `interface{}`/JSON round-trip itself — `resources` uses a concrete struct, see DD-WE-008 Finding 1)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|---|---|---|---|---|---|
| R1 | `resource.ParseQuantity()` mis-parses (or rejects) a quantity form real workflow authors use | Silent data loss or rejected valid YAML | Low (validated by spike; same parser Kubernetes admission uses) | UT-DS-008-001..004 | Explicit test matrix covering quoted/unquoted/whole-number/suffix forms |
| R2 | `PodFailurePolicy` rule ordering/shape wrong, real K8s API rejects the Job at admission | Job creation fails at runtime, WFE stuck | Medium | IT-WE-019-002 | Integration test creates the Job against envtest's real API server (structural validation), not just asserting the in-memory struct |
| R3 | Engine-gating check (`resources` only valid for `job`) has a gap allowing silent no-op for tekton/ansible | Workflow author believes resources apply when they don't (FedRAMP SI-10 silent-failure concern) | Low | UT-DS-008-005..006 | Explicit rejection test for `engine: tekton` + `resources` set |
| R4 | `requests > limits` validation only checks resource names present in both maps, misses cross-unit comparisons | False negative (invalid workflow registers successfully) | Low | UT-DS-008-007..008 | Test with cpu-only limit + both cpu/memory requests, and vice versa |
| R5 | New `WFE.Status.Resources` CRD field breaks existing CRD consumers (e.g. kubectl get -o yaml diffing tools) | Low — additive, optional field | Low | IT-WE-019-001 | CRD regen verified via `make manifests`; field is `omitempty` |
| R6 | `PodFailurePolicy`'s `Ignore` semantics only verified structurally (envtest), never against a real kubelet/Job-controller loop | Pyramid Invariant gap: a schema-valid manifest could still be misinterpreted by a real cluster in ways envtest can't catch | Low (K8s SIG Apps GA-tests the controller loop itself) but the specific rule *shape* chosen here is still ours to prove | E2E-WE-019-001, E2E-WE-019-002 | Real-cluster E2E tier added (DD-WE-008 Section 8) |
| R7 | New `job-oomkill` E2E fixture requires a manual, credentialed image push (`test/fixtures/job/build-and-push.sh`) before `E2E-WE-019-001` can run in CI | E2E test fails with ImagePullBackOff if the image isn't published first | Medium (one-time manual step, easy to forget) | E2E-WE-019-001 | Documented explicitly as a blocking pre-req in the implementation plan; test skipped with a clear message if the image is unreachable, not a silent pass |
| R8 | Tightening `reconciler_test.go`/`02_observability_test.go` assertions to exact `EventType`-filtered counts (Section 15) exposes a pre-existing test flake or an actual duplicate-audit-write bug unrelated to this PR | CI red on a bundled, supposedly low-risk change; could block the PR on an unrelated pre-existing bug | Low (these are lower-bound assertions today — tightening can only newly *fail* if the true count already silently exceeded 1, which is itself the bug this guard exists to catch) | Bundled hardening (Section 15), IT-WE-019-003 | Run each tightened test standalone before bundling into the PR; if a pre-existing bug surfaces, file it separately and revert that one assertion to its prior looseness rather than blocking #1572 |
| R9 | Wiring Point C's new OpenAPI `retry_count` field is added but `make generate-datastorage-client` is not re-run (or is run but the generated diff isn't committed), leaving the Go client's `WorkflowExecutionAuditPayload` struct out of sync with the spec | `payload.RetryCount` reference fails to compile, or (worse) silently uses a stale generated type | Low (build fails loudly if the client is stale — not a silent gap) | UT-WE-AUDIT-001, IT-WE-019-004 | `go build ./...` in Checkpoint 2 catches a stale/missing generated field immediately; CI's existing generated-code-drift check (if any) is the second line of defense |

### 3.1 Risk-to-Test Traceability

All risks R1-R7 have at least one dedicated test; no coverage gaps identified. R6/R7 were
added after discovering, mid-review, that the original plan had zero E2E coverage — a
Pyramid Invariant violation (UT+IT only is "prototyped, not implemented" per AGENTS.md) — see
Section 4.2 for the corrected scope.

---

## 4. Scope

### 4.1 Features to be Tested

- **Schema parser** (`pkg/datastorage/schema/parser.go`): `ExtractResources()`, engine-gating and requests<=limits validation in `validateWorkflowExecution`
- **WorkflowQuerier** (`pkg/workflowexecution/client/workflow_querier.go`): `Resources` field on `WorkflowCatalogMetadata`, populated in `ResolveWorkflowCatalogMetadata`
- **WFE CRD** (`api/workflowexecution/v1alpha1/workflowexecution_types.go`): `Status.Resources` field
- **Controller wiring** (`internal/controller/workflowexecution/workflowexecution_catalog.go`): `resolveWorkflowCatalog` sets `Status.Resources`
- **Job executor** (`pkg/workflowexecution/executor/job.go`): `buildJob()` consumes `Status.Resources`; `PodFailurePolicy` added unconditionally; `buildStatusSummary()` unconditionally captures `RetryCount` from `job.Status.Failed` (Wiring Point C, BR-WE-019 AC10)
- **WFE CRD status summary** (`api/workflowexecution/v1alpha1/workflowexecution_types.go`): `ExecutionStatusSummary.RetryCount` (Wiring Point C)
- **Audit payload schema/builder** (`api/openapi/data-storage-v1.yaml`, `pkg/workflowexecution/audit/manager.go`): optional `retry_count` field on `WorkflowExecutionAuditPayload`; `buildWorkflowExecutionAuditPayload()` populates it from `wfe.Status.ExecutionStatus.RetryCount` (Wiring Point C)

### 4.2 Features Not to be Tested

- **Tekton engine resources**: out of scope per DD-WE-008 (Tekton has no pod-level resources injection point; unchanged in this change)
- **Ansible engine**: out of scope, no Job/Pod construction
- **A genuine, kubelet-induced cgroup OOM event under real memory pressure**: not reproduced in any tier — inducing real memory pressure deterministically in CI is unreliable. Instead, `E2E-WE-019-001` uses a fixture that deterministically `exit 137`s (the same exit code a real OOM-kill produces) to prove the *Job-controller's* `Ignore` evaluation on a real cluster, which is the actual behavior this decision changes. The `job.go` comment documents the accepted limitation that exit 137 cannot be structurally distinguished from an arbitrary `SIGKILL` at the Kubernetes API level (see DD-WE-008 "Known limitation") — this is a K8s API constraint, not a test-coverage gap.
- **The full `ActiveDeadlineSeconds` (30 min) timeout journey for a chronically-OOMing workflow**: `E2E-WE-019-001` proves the *tolerated-retry* behavior within a short bounded window (>= 2 pod attempts observed, WFE still `Running`) and explicitly does not run the test to its natural 30-minute `DeadlineExceeded` conclusion — that would make the E2E suite prohibitively slow for CI. The `ActiveDeadlineSeconds` mechanism itself is pre-existing (BR-WORKFLOW-008) and already has its own regression coverage (`UT-WE-054-JOB-016/017`).
- **Root-cause attribution per individual retry attempt** (OOM-kill vs. disruption, specifically): explicitly out of scope for AC10 — see BR-WE-019 "Known Limitations" and DD-WE-008 Section 9. AC10 hard-guarantees the retry *count* only.
- **Forcing deterministic Tekton PipelineRun completion in envtest** (`audit_flow_integration_test.go`'s intentionally loose `>= 1` assertions): out of scope, unrelated to Job/`PodFailurePolicy` work, tracked as its own follow-up issue rather than bundled here — see Section 4.4 for what *is* bundled
- **`LimitRange`/`ResourceQuota` operator guidance**: documentation-only, tracked in kubernaut-docs#197, not implementation in this repo

### 4.4 Wiring Point C (Audit Retry-Count Completeness, BR-WE-019 AC10) and Bundled Audit-Count Regression Guards (Cite BR-AUDIT-005)

Investigating SOC2 CC8.1/AU-3 alignment for this PR (DD-WE-008 Section 9) originally concluded
the audit-*content* fix (recording how many retries occurred) required a cross-cutting
OpenAPI/ogen change and should be deferred. Re-investigation, prompted by user challenge, found
that justification incorrect: `WorkflowExecutionAuditPayload` is exclusively WFE-owned, ogen
regen is a single command, and the CRD/reconciler plumbing (`ExecutionStatusSummary` ->
`wfe.Status.ExecutionStatus`) already exists. This fix is now **in scope**, as **Wiring Point C**,
alongside a related but distinct concern (audit-event *count*, not content) that surfaced from
the same investigation and remains a bundled *regression guard* rather than a new fix:

**Wiring Point C (genuine RED->GREEN, cites BR-WE-019 AC10)**:

- `UT-WE-054-JOB-025`/`026`: `buildStatusSummary()` unconditionally captures `RetryCount` from
  `job.Status.Failed`, including on the success path (`Succeeded > 0`) where it was previously
  never read
- `UT-WE-AUDIT-001`: `buildWorkflowExecutionAuditPayload()` sets `payload.RetryCount` from
  `wfe.Status.ExecutionStatus.RetryCount` when > 0 (new test file — `manager.go` had zero
  existing unit coverage; scoped narrowly to this new field, not a full-file backfill)
- `IT-WE-019-004`: through the real reconciler entry point, a WFE whose Job shows
  `Status.Failed: N` before eventually `Status.Succeeded: 1` produces a `workflow.completed`
  audit event whose typed `event_data.retry_count` equals N (envtest-provable — no real Job
  controller needed, since the test sets Job status directly, mirroring existing
  `UT-WE-054-JOB-005`-style fixtures)
- `E2E-WE-019-001` (further extended, real-cluster proof of AC10): reusing the same tolerated
  OOM-kill run, the resulting `workflow.completed` event's `retry_count` is >= 2

**Bundled regression guards (no genuine RED phase, cite BR-AUDIT-005 — distinct from AC10 above
because they guard the audit-event *count*, not its *content*)**:

- `IT-WE-019-003`: the WFE controller emits exactly one `workflow.completed`/`workflow.failed`
  audit event regardless of how many times `reconcileRunning` polls a still-`Running` Job —
  tests the audit-emission-is-terminal-transition-gated invariant directly, without needing a
  real Job controller (envtest-provable)
- `E2E-WE-019-001` (extended): after the real cluster tolerates >= 2 pod attempts, exactly one
  `workflow.completed` audit event exists in DataStorage for the WFE's correlation ID
- **Honesty note**: neither of these two guards has a genuine RED phase — the invariant already
  holds today, with or without this PR's changes (audit only fires on terminal transitions;
  tolerated retries never reach one). Forward-looking regression guards, not proof of a new
  guarantee this PR creates.
- **Existing-test hardening** (mechanical, not new test IDs — see Section 15): `reconciler_test.go`
  (3 assertions) and `02_observability_test.go` (2 assertions, E2E) have `>= 1`/existence-style
  audit assertions that don't filter by `EventType`, so they under-specify what they're proving.
  Tightened to exact per-type counts using the existing `countEventsByType` helper.
- **Explicitly NOT bundled**: `audit_flow_integration_test.go`'s 2 loose assertions — these are
  loose *on purpose* (non-deterministic Tekton PipelineRun completion timing in envtest, per the
  test's own docstring) and need a design decision (force determinism vs. leave loose), not a
  mechanical fix. Tracked as its own follow-up issue.

### 4.3 Design Decisions

| Decision | Rationale |
|---|---|
| No dedicated DB column/migration for `resources` | Mirrors `dependencies`/`engineConfig` on-demand extraction (DD-WE-008 Scenario 3 vs 4) |
| PodFailurePolicy verified structurally via envtest AND behaviorally via a real-cluster E2E tier | envtest has no kubelet and cannot exercise the Job controller's pod-failure-policy evaluation loop; the *evaluation itself* (not just manifest schema-validity) is what this decision changes, so it needs real-cluster proof (Pyramid Invariant) |
| `resources` tests use `UT-DS-008-*` numbering; new E2E tests use `E2E-WE-019-*` | Follows repo convention of `{TIER}-{SERVICE}-{DD/BR-NUMBER}-{SEQ}`; 008 = DD-WE-008, 019 = BR-WE-019 |
| Audit-completeness *content* fix (retry count) folded into #1572 as Wiring Point C, reversing the original deferral | Re-investigation showed the original "cross-cutting audit infrastructure" justification for deferring was incorrect — the schema is WFE-exclusive and the fix is comparable in size to Wiring Point A (see DD-WE-008 Section 9) |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of new/changed unit-testable code (`ExtractResources`, `validateWorkflowExecution` additions, `resourcesFor`, `buildJob` `PodFailurePolicy` construction)
- **Integration**: 100% of wiring points (DS → WorkflowQuerier → WFE.Status → Job manifest)
- **E2E**: AC4/AC5/AC9 (transient-failure tolerance and its fail-fast boundary) proven on a real cluster — per the Pyramid Invariant, IT-level (envtest) verification alone is schema-validity proof, not behavioral proof, for a controller-loop feature like `PodFailurePolicy`

### 5.2 Coverage Tiers Per Acceptance Criterion

AC1-AC3, AC6-AC8 are covered by Unit + Integration tests (two-tier minimum, sufficient — these
are data-plumbing/validation concerns, not controller-loop behavior). **AC4, AC5, and AC9 are
covered by Unit + Integration + E2E** (three-tier), because they concern real Job-controller
behavior that envtest cannot exercise (see BR Coverage Matrix, Section 7).

### 5.4 Pass/Fail Criteria

**PASS**: all new tests pass; all pre-existing `UT-WE-054-JOB-*`, `UT-DS-006-*`, `UT-WE-650-*`, `IT-WE-014-*`, `E2E-WE-014-*` tests continue to pass unmodified (zero regressions); `go build ./...` and `golangci-lint run` are clean.

**FAIL**: any P0 test fails, or any pre-existing test regresses.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `pkg/datastorage/models/workflow_schema.go` | `WorkflowExecution.Resources` field | ~5 |
| `pkg/datastorage/schema/parser.go` | `ExtractResources`, `validateWorkflowExecution` (extended) | ~30 |
| `pkg/workflowexecution/client/workflow_querier.go` | `ResolveWorkflowCatalogMetadata` (extended) | ~10 |
| `pkg/workflowexecution/executor/job.go` | `resourcesFor`, `buildJob` (extended, `PodFailurePolicy`), `buildStatusSummary` (extended, `RetryCount`) | ~35 |
| `api/workflowexecution/v1alpha1/workflowexecution_types.go` | `ExecutionStatusSummary.RetryCount` field | ~4 |
| `pkg/workflowexecution/audit/manager.go` | `buildWorkflowExecutionAuditPayload` (extended, `RetryCount`) | ~4 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|---|---|---|
| `internal/controller/workflowexecution/workflowexecution_catalog.go` | `resolveWorkflowCatalog` (extended) | ~2 |
| `test/integration/workflowexecution/job_lifecycle_integration_test.go` | New IT scenarios | N/A (test-only) |
| `api/openapi/data-storage-v1.yaml` | `WorkflowExecutionAuditPayload.retry_count` (new optional field) | ~4 |

---

## 7. BR Coverage Matrix

| BR ID | AC | Description | Tier | Test ID | Status |
|---|---|---|---|---|---|
| BR-WE-019 | AC1 | Per-workflow resources declared, reach Job container | Unit | UT-DS-008-001 | Pending |
| BR-WE-019 | AC1 | Per-workflow resources declared, reach Job container | Unit | UT-WE-1572-001 | Pending |
| BR-WE-019 | AC1 | Per-workflow resources declared, reach Job container | Unit | UT-WE-054-JOB-021 | Pending |
| BR-WE-019 | AC1 | Per-workflow resources declared, reach Job container | Integration | IT-WE-019-001 | Pending |
| BR-WE-019 | AC2 | Engine-scoping enforcement | Unit | UT-DS-008-005 | Pending |
| BR-WE-019 | AC3 | Fail-fast registration-time validation (invalid quantity) | Unit | UT-DS-008-002 | Pending |
| BR-WE-019 | AC3 | Fail-fast registration-time validation (requests>limits) | Unit | UT-DS-008-007 | Pending |
| BR-WE-019 | AC4 | OOM-kill / disruption tolerated (Ignore) | Unit | UT-WE-054-JOB-022 | Pending |
| BR-WE-019 | AC4 | OOM-kill / disruption tolerated (Ignore) | Integration | IT-WE-019-002 | Pending |
| BR-WE-019 | AC4 | OOM-kill tolerated on a real cluster (>= 2 pod attempts, WFE stays Running) | E2E | E2E-WE-019-001 | Pending |
| BR-WE-019 | AC5 | Genuine failures still fail-fast (Count, unchanged) | Unit | UT-WE-054-JOB-023 | Pending |
| BR-WE-019 | AC5 | Genuine failure still fails fast on a real cluster (exactly 1 attempt) | E2E | E2E-WE-019-002 | Pending |
| BR-WE-019 | AC6 | ActiveDeadlineSeconds remains outer bound | Unit | UT-WE-054-JOB-016 (existing, regression) | Pass (pre-existing) |
| BR-WE-019 | AC7 | No behavior change for absent resources (backward compat) | Unit | UT-WE-054-JOB-024 | Pending |
| BR-WE-019 | AC8 | No fleet default shipped in this repo | N/A | Verified by code review (no Helm/operator changes in this PR) | N/A |
| BR-WE-019 | AC9 | Real-cluster proof requirement itself | E2E | E2E-WE-019-001, E2E-WE-019-002 | Pending |
| BR-WE-019 | AC10 | `buildStatusSummary()` captures `RetryCount` unconditionally (incl. success path) | Unit | UT-WE-054-JOB-025, UT-WE-054-JOB-026 | Pending |
| BR-WE-019 | AC10 | Audit payload builder sets `RetryCount` from `wfe.Status.ExecutionStatus.RetryCount` | Unit | UT-WE-AUDIT-001 | Pending |
| BR-WE-019 | AC10 | `workflow.completed` event's `retry_count` equals observed pod-failure count | Integration | IT-WE-019-004 | Pending |
| BR-WE-019 | AC10 | `retry_count` >= 2 in the completion event on a real cluster after tolerated OOM-kills | E2E | E2E-WE-019-001 (extended assertion) | Pending |
| BR-AUDIT-005 | CC8.1/AU-3 (count, not content) | Exactly 1 completion audit event despite multiple reconciles of a still-Running Job | Integration | IT-WE-019-003 | Pending |
| BR-AUDIT-005 | CC8.1/AU-3 (count, not content) | Exactly 1 completion audit event despite >= 2 tolerated pod attempts on a real cluster | E2E | E2E-WE-019-001 (extended assertion) | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **Pass**: Implemented and passing

### Known, Out-of-Scope Item (Not in This Matrix)

Root-cause attribution *per individual retry attempt* (distinguishing OOM-kill from disruption
for each specific tolerated failure) remains best-effort only and is explicitly not a test gap —
see BR-WE-019 "Known Limitations" (AC10 note) and DD-WE-008 Section 9. This is a deliberate scope
boundary set alongside AC10, not a deferred fix; no follow-up test plan is expected for it unless
a future BR expands this guarantee.

---

## 8. Test Scenarios

### Test ID Naming Convention

`{TIER}-{SERVICE}-{DD/BR/ISSUE_NUMBER}-{SEQUENCE}` — following existing repo convention
(`UT-DS-006-*` for DD-WE-006, `UT-WE-650-*` for issue #650).

### Tier 1: Unit Tests

**File**: `pkg/datastorage/schema_resources_test.go` (new)

| ID | Business Outcome Under Test |
|---|---|
| `UT-DS-008-001` | `execution.resources` with quoted and unquoted quantities (`cpu: 100m`, `memory: 512Mi`, `cpu: "1"`) parses into the exact typed `corev1.ResourceRequirements` |
| `UT-DS-008-002` | An invalid quantity string (`cpu: not-a-number`) is rejected with a `SchemaValidationError` naming `execution.resources` |
| `UT-DS-008-003` | Absent `execution.resources` → `ExtractResources` returns `(nil, nil)` (backward compatible) |
| `UT-DS-008-004` | `resources` with only `requests` (no `limits`), and vice versa, both parse successfully |
| `UT-DS-008-005` | `execution.resources` declared with `engine: tekton` is rejected with a clear error naming the engine |
| `UT-DS-008-006` | `execution.resources` declared with `engine: ansible` is rejected |
| `UT-DS-008-007` | `requests.cpu` exceeding `limits.cpu` is rejected with a `SchemaValidationError` |
| `UT-DS-008-008` | `requests.memory` exceeding `limits.memory` is rejected (independent resource-name check) |
| `UT-DS-008-009` | `requests.cpu <= limits.cpu` AND `requests.memory <= limits.memory` (valid, mixed) parses successfully |

**File**: `pkg/workflowexecution/workflow_querier_test.go` (extended)

| ID | Business Outcome Under Test |
|---|---|
| `UT-WE-1572-001` | `ResolveWorkflowCatalogMetadata` returns `Resources` populated from a schema `content` declaring `execution.resources` |
| `UT-WE-1572-002` | `ResolveWorkflowCatalogMetadata` returns `Resources: nil` when the catalog entry declares none |
| `UT-WE-1572-003` | `ResolveWorkflowCatalogMetadata` propagates a resources parse/validation error from the schema parser |

**File**: `pkg/workflowexecution/executor/job_test.go` (extended)

| ID | Business Outcome Under Test |
|---|---|
| `UT-WE-054-JOB-021` | `buildJob()` sets `Containers[0].Resources` from `wfe.Status.Resources` when populated |
| `UT-WE-054-JOB-022` | `buildJob()`'s `Job.Spec.PodFailurePolicy` has an `Ignore` rule for `onExitCodes: [137]` on the `"workflow"` container, and an `Ignore` rule for `onPodConditions: [DisruptionTarget=True]` |
| `UT-WE-054-JOB-023` | `Job.Spec.BackoffLimit` remains `0` (unchanged) — genuine failures are not exempted by `PodFailurePolicy` |
| `UT-WE-054-JOB-024` | `buildJob()` with `wfe.Status.Resources == nil` produces a container with the zero-value `Resources{}` (byte-identical to pre-change behavior) |
| `UT-WE-054-JOB-025` | `buildStatusSummary()` sets `summary.RetryCount == job.Status.Failed` when the Job has BOTH `Succeeded > 0` AND `Failed > 0` (previously the success branch never read `Failed`) |
| `UT-WE-054-JOB-026` | `buildStatusSummary()` sets `summary.RetryCount == 0` (omitted) when the Job succeeded with zero prior failures (backward-compatible: no spurious `retryCount: 0` behavior change for the common case) |

**File**: `pkg/workflowexecution/audit/manager_test.go` (new — `manager.go` has zero pre-existing unit coverage; this file is scoped narrowly to the new field)

| ID | Business Outcome Under Test |
|---|---|
| `UT-WE-AUDIT-001` | `buildWorkflowExecutionAuditPayload()` sets `payload.RetryCount` from `wfe.Status.ExecutionStatus.RetryCount` when it is > 0 |
| `UT-WE-AUDIT-002` | `buildWorkflowExecutionAuditPayload()` leaves `payload.RetryCount` unset when `wfe.Status.ExecutionStatus` is `nil` or `RetryCount == 0` |

### Tier 2: Integration Tests

**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go` (extended)

| ID | Business Outcome Under Test |
|---|---|
| `IT-WE-019-001` | A WFE whose catalog entry declares `execution.resources` results in a real (envtest API server-accepted) Job whose container `Resources` match the declared values, and `wfe.Status.Resources` reflects the resolved value |
| `IT-WE-019-002` | The Job created by the WFE reconciler is accepted by the envtest API server with the exact `PodFailurePolicy` shape (proves the manifest is schema-valid against a real Kubernetes API server, not just structurally correct in Go) |
| `IT-WE-019-003` | *(Bundled, cites BR-AUDIT-005)* A WFE's Job is observed as still-`Running` across N reconcile passes before terminating; exactly 1 `workflow.completed` audit event is recorded for its correlation ID regardless of N |
| `IT-WE-019-004` | *(Wiring Point C, BR-WE-019 AC10)* A WFE's Job shows `Status.Failed: N` (N >= 1) before eventually reaching `Status.Succeeded: 1`; the resulting `workflow.completed` audit event's typed `event_data.retry_count` equals N |

### Tier 3: E2E Tests

**File**: `test/e2e/workflowexecution/03_job_backend_test.go` (extended — same file as the
existing `E2E-WE-014-*` Job backend suite; same KIND cluster, same controller deployment, no
new test infrastructure)

| ID | Business Outcome Under Test |
|---|---|
| `E2E-WE-019-001` | On a real cluster, a Job pod that unconditionally exits 137 (`job-oomkill` fixture) does not permanently fail the WFE on the first attempt: within a bounded window, `job.Status.Failed` reaches >= 2 while the WFE remains `Running`, proving the real Job controller applies `Ignore` (not just that the manifest is schema-valid). *Extended (bundled, cites BR-AUDIT-005)*: once the WFE terminates, exactly 1 `workflow.completed` audit event exists for its correlation ID — no duplicates caused by the tolerated retries. *Extended further (Wiring Point C, BR-WE-019 AC10)*: that same event's `retry_count` is >= 2, proving the count reaches the durable audit trail on a real cluster, not just in envtest |
| `E2E-WE-019-002` | On a real cluster, a genuine remediation-script failure (`test-job-intentional-failure` fixture, `exit 1`, no new image) still fails fast: `job.Status.Failed` reaches exactly `1` and the WFE reaches `Failed` within the existing SLA window — proves the unconditional `PodFailurePolicy` addition does not weaken today's fail-fast contract |

---

## 9. Test Cases (P0 detail)

### UT-DS-008-002: reject invalid resource quantity at registration time

**BR**: BR-WE-019 (AC3)
**Priority**: P0
**Type**: Unit
**File**: `pkg/datastorage/schema_resources_test.go`

**Test Steps**:
1. **Given**: a `workflow-schema.yaml` with `execution.engine: job` and `execution.resources.requests.cpu: "not-a-number"`
2. **When**: `Parser.ParseAndValidate(content)` is called
3. **Then**: an error is returned wrapping a `SchemaValidationError` with `Field == "execution.resources"`

### IT-WE-019-002: PodFailurePolicy manifest accepted by real API server

**BR**: BR-WE-019 (AC4)
**Priority**: P0
**Type**: Integration
**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go`

**Test Steps**:
1. **Given**: a WFE resolved to `engine: job`, no special resources
2. **When**: the WFE reconciler creates the Job via `JobExecutor.Create`
3. **Then**: `fakeClient`/envtest `Get` on the created Job succeeds and `Job.Spec.PodFailurePolicy.Rules` contains exactly 2 rules matching the DD-WE-008 shape (Ignore/DisruptionTarget, Ignore/exit-137)

### IT-WE-019-003: exactly 1 completion audit event across N reconciles (bundled, BR-AUDIT-005)

**BR**: BR-AUDIT-005 (CC8.1/AU-3 — count, not content)
**Priority**: P0
**Type**: Integration
**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go`

**Test Steps**:
1. **Given**: a WFE with a Job whose `Status.Active == 1` (still running); the reconciler is
   triggered to observe this same "still Running" state N times (e.g. N=3, via repeated
   `Reconcile()` calls or requeue) before the Job is finally updated to `Status.Succeeded == 1`
2. **When**: the WFE reconciler processes all N+1 reconcile passes
3. **Then**: querying DataStorage by the WFE's `correlation_id` and filtering
   `EventType == "workflow.completed"` (via the existing `countEventsByType` helper) returns
   exactly 1 event, proving audit emission is gated on the terminal phase transition, not on
   reconcile count — this holds regardless of `PodFailurePolicy`, so it is a regression guard,
   not proof of a new guarantee this PR introduces

### IT-WE-019-004: retry count reaches the durable audit trail (Wiring Point C, BR-WE-019 AC10)

**BR**: BR-WE-019 (AC10)
**Priority**: P0
**Type**: Integration
**File**: `test/integration/workflowexecution/job_lifecycle_integration_test.go`

**Test Steps**:
1. **Given**: a WFE with an associated Job whose status is set directly (no real Job controller
   needed, envtest-provable) to `Status.Failed: 2, Status.Succeeded: 1` with a `JobComplete`
   condition — simulating 2 tolerated attempts before eventual success
2. **When**: the WFE reconciler's `GetStatus`/completion path processes this Job and emits the
   `workflow.completed` audit event
3. **Then**: querying DataStorage by `correlation_id`, the event's typed
   `event_data.GetWorkflowExecutionAuditPayload().RetryCount` is set and equals `2`

### E2E-WE-019-001: OOM-kill tolerated on a real cluster

**BR**: BR-WE-019 (AC4, AC9)
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/workflowexecution/03_job_backend_test.go`
**Pre-req**: `quay.io/kubernaut-cicd/test-workflows/job-oomkill:v1.0.0` published (one-time manual step, see DD-WE-008 Section 8)

**Test Steps**:
1. **Given**: a WFE referencing the `test-job-oomkill` catalog workflow (container unconditionally `exit 137`s)
2. **When**: the WFE is created against the real KIND cluster
3. **Then**: within 90s, `job.Status.Failed >= 2` (the real Job controller created at least 2 pod attempts) while `wfe.Status.Phase == Running` (not `Failed`) — proving `Ignore` is applied by the real controller loop, not just accepted as schema-valid
4. **Then (bundled, BR-AUDIT-005)**: after the fixture is allowed to complete and the WFE reaches a terminal phase, querying DataStorage by `correlation_id` filtered to `EventType == "workflow.completed"` returns exactly 1 event — no duplicate audit writes caused by the tolerated retries
5. **Then (Wiring Point C, BR-WE-019 AC10)**: that same event's `event_data.retry_count` is >= 2, matching the observed `job.Status.Failed` count from step 3
6. **Cleanup**: WFE deleted immediately after the assertion (test does not wait for `ActiveDeadlineSeconds`)

### E2E-WE-019-002: Genuine failure still fails fast on a real cluster (regression)

**BR**: BR-WE-019 (AC5, AC9)
**Priority**: P0
**Type**: E2E
**File**: `test/e2e/workflowexecution/03_job_backend_test.go`

**Test Steps**:
1. **Given**: a WFE referencing the existing `test-job-intentional-failure` catalog workflow (container `exit 1`s)
2. **When**: the WFE is created against the real KIND cluster
3. **Then**: `job.Status.Failed == 1` (exactly one attempt, `Count` behavior against `backoffLimit: 0`, unchanged by the new unconditional `PodFailurePolicy`) and `wfe.Status.Phase == Failed` within the existing SLA window (mirrors `E2E-WE-014-002`, with the added explicit `job.Status.Failed == 1` assertion this DD's regression concern requires)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: none beyond existing (`mockClientFactory`, fake controller-runtime client)
- **Location**: `pkg/datastorage/`, `pkg/workflowexecution/`, `pkg/workflowexecution/executor/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest (real API server + etcd, per existing `test/integration/workflowexecution/suite_test.go`)
- **Location**: `test/integration/workflowexecution/`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: KIND cluster (real kubelet + Job controller), per existing `test/e2e/workflowexecution/workflowexecution_e2e_suite_test.go`
- **Location**: `test/e2e/workflowexecution/`
- **New fixture**: `test/fixtures/job/oomkill/` (Dockerfile + `workflow-schema.yaml`), registered as `test-job-oomkill` in `test/infrastructure/workflow_bundles.go`
- **Blocking pre-req**: `job-oomkill` image published to `quay.io/kubernaut-cicd/test-workflows` before `E2E-WE-019-001` can run (manual, one-time — see DD-WE-008 Section 8)

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — no external issue blocks this work; DD-WE-008 is self-contained.

### 11.2 Execution Order

Per AGENTS.md's Wiring-First TDD Sequence (`RED: IT→UT`, `GREEN: wire→implement`), applied to
each of the three wiring points independently — see `IMPLEMENTATION_PLAN.md` Phases 1-7 for full
detail:

1. **Wiring Point A** (`resources`: catalog → `WFE.Status` → Job container):
   RED (`IT-WE-019-001` first, then `UT-DS-008-*`/`UT-WE-1572-*`/`UT-WE-054-JOB-021`/`024`) →
   GREEN (wire the plumbing until IT passes, then implement full extraction/validation logic
   until UT passes)
2. **Wiring Point B** (`PodFailurePolicy`):
   RED (`IT-WE-019-002` first, then `UT-WE-054-JOB-022`/`023`) → GREEN (wire the policy into
   `buildJob`)
3. **Wiring Point C** (audit retry-count completeness, BR-WE-019 AC10):
   RED (`IT-WE-019-004` first, then `UT-WE-054-JOB-025`/`026`/`UT-WE-AUDIT-001`/`002`) → GREEN
   (add the `RetryCount` CRD field + OpenAPI field + regen, wire capture in `buildStatusSummary`,
   wire payload population in `buildWorkflowExecutionAuditPayload`)
4. **Bundled audit-count regression guards** (BR-AUDIT-005, no genuine RED phase — see Section
   4.4): `IT-WE-019-003` and the `E2E-WE-019-001` count-assertion extension, plus mechanical
   hardening of 5 pre-existing loose assertions (Section 15)
5. **Real-cluster proof of Wiring Point B and C** (Pyramid Invariant — envtest cannot exercise
   the real Job-controller evaluation loop): `job-oomkill` fixture built and published, then
   `E2E-WE-019-001`/`002` RED→GREEN against the already-wired code, extended with the bundled
   BR-AUDIT-005 count guard and the AC10 `retry_count` assertion. E2E, not IT, is Wiring Point
   B's actual GREEN-completion gate; Wiring Point C's real-cluster assertion rides along on the
   same run rather than needing its own fixture
6. **REFACTOR**: per wiring point, name the concrete improvement or mark `N/A` with a reason —
   not a bare "confirm build/lint/tests pass" (AGENTS.md's REFACTOR-as-Validation anti-pattern)
7. **Wiring verification**: CHECKPOINT W against the Wiring Manifest (Section 14) confirmed as
   part of each wiring point's own GREEN checkpoint above, not deferred to the end

---

## 12. Test Deliverables

| Deliverable | Location |
|---|---|
| This test plan | `docs/tests/1572/TEST_PLAN.md` |
| Unit tests | `pkg/datastorage/schema_resources_test.go`, `pkg/workflowexecution/workflow_querier_test.go`, `pkg/workflowexecution/executor/job_test.go`, `pkg/workflowexecution/audit/manager_test.go` (new) |
| Integration tests | `test/integration/workflowexecution/job_lifecycle_integration_test.go` |
| E2E tests | `test/e2e/workflowexecution/03_job_backend_test.go` (extended) |
| E2E fixture | `test/fixtures/job/oomkill/{Dockerfile,workflow-schema.yaml}` (new) |

---

## 13. Execution

```bash
# Unit tests
go test ./pkg/datastorage/... ./pkg/workflowexecution/... -ginkgo.v

# Integration tests
go test ./test/integration/workflowexecution/... -ginkgo.v

# E2E tests (requires KIND cluster + job-oomkill image published)
make test-e2e-workflowexecution

# Focused run
go test ./pkg/datastorage/... -ginkgo.focus="UT-DS-008"
go test ./pkg/workflowexecution/executor/... -ginkgo.focus="UT-WE-054-JOB-02"
go test ./test/e2e/workflowexecution/... -ginkgo.focus="E2E-WE-019"
```

---

## 14. Wiring Verification (CHECKPOINT W — GREEN Phase Gate, per AGENTS.md; IMPLEMENTATION_PLAN.md Phases 2 and 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|---|---|---|---|---|
| `execution.resources` → `WorkflowCatalogMetadata.Resources` | DS `content` (registered schema) | `ResolveWorkflowCatalogMetadata` return value | `UT-WE-1572-001` (unit; querier has no HTTP entry point of its own — see note) | Pending |
| `WorkflowCatalogMetadata.Resources` → `WFE.Status.Resources` | `resolveWorkflowCatalog` (Pending-phase reconcile) | WFE CRD status | `IT-WE-019-001` | Pending |
| `WFE.Status.Resources` → Job container `Resources` | `JobExecutor.Create` | Real Job object (envtest API server) | `IT-WE-019-001` | Pending |
| `buildJob()` → `PodFailurePolicy` | `JobExecutor.Create` | Real Job object (envtest API server) | `IT-WE-019-002` | Pending |
| `PodFailurePolicy` → real Job-controller `Ignore` evaluation | `JobExecutor.Create` (via real WFE reconcile) | Real Job/Pod state on a KIND cluster | `E2E-WE-019-001` | Pending |
| `backoffLimit: 0` + `PodFailurePolicy` → real Job-controller `Count`/fail-fast evaluation | `JobExecutor.Create` (via real WFE reconcile) | Real Job/Pod state on a KIND cluster | `E2E-WE-019-002` | Pending |
| *(Bundled, BR-AUDIT-005)* WFE terminal-phase transition → single audit write | Reconciler's audit-emission call site (pre-existing, unchanged by this PR) | DataStorage `workflow.completed`/`workflow.failed` event | `IT-WE-019-003`, `E2E-WE-019-001` (extended) | Pending |
| *(Wiring Point C, BR-WE-019 AC10)* `job.Status.Failed` → `ExecutionStatusSummary.RetryCount` → `WFE.Status.ExecutionStatus` → audit `retry_count` | `JobExecutor.GetStatus` (via real WFE reconcile) | DataStorage `workflow.completed` event's `event_data.retry_count` | `IT-WE-019-004`, `E2E-WE-019-001` (extended) | Pending |

**Note**: `WorkflowQuerier` is a DS HTTP client, not an HTTP entry point itself — its "wiring" proof is that `resolveWorkflowCatalog` (a real production reconcile path) calls it and the result reaches `WFE.Status`, which `IT-WE-019-001` proves end-to-end through the actual reconciler.

---

## 15. Existing Tests Requiring Updates

`test/infrastructure/workflow_bundles.go`'s `bundleWorkflows` list gains one new entry
(`test-job-oomkill`) and `test/fixtures/job/build-and-push.sh`'s `IMAGES` array gains one new
entry (`job-oomkill`) — both additive, no existing entries modified.

Additionally, bundled per Section 4.4 (mechanical, low-risk, unrelated to this PR's core
change but tightened opportunistically to avoid a separate PR/CI cycle):

| File | Current Assertion | Tightened To | Risk |
|---|---|---|---|
| `test/integration/workflowexecution/reconciler_test.go` | 3 assertions use `>= 1` / bare existence on audit event counts, no `EventType` filter | Exact count via `countEventsByType`, filtered by the specific `EventType` each assertion is actually about | Low — mirrors the pattern already proven in `audit_workflow_refs_integration_test.go` |
| `test/e2e/workflowexecution/02_observability_test.go` | 2 assertions use `>= 1` / bare existence on audit event counts, no `EventType` filter | Exact count via `countEventsByType`, filtered by `EventType` | Low — same pattern, E2E tier |

**Explicitly not touched**: `test/integration/workflowexecution/audit_flow_integration_test.go`'s
2 loose (`>= 1`) assertions on Tekton PipelineRun-related audit events — these are loose by
design due to non-deterministic PipelineRun completion timing in envtest (per the test's own
docstring), and tightening them requires a design decision (force determinism vs. leave loose),
not a mechanical fix. Tracked as a separate follow-up issue rather than bundled here.

---

## 16. Changelog

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-07-06 | Initial test plan |
| 1.1 | 2026-07-06 | Added Tier 3 (E2E) covering AC4/AC5/AC9 on a real cluster (`E2E-WE-019-001/002`), closing a Pyramid Invariant gap (UT+IT only is insufficient for a controller-loop behavior per AGENTS.md). Added R6/R7 risks. Documented the deferred audit-completeness gap (SOC2 CC8.1/AU-3) as explicitly out of scope, not a silent omission. |
| 1.2 | 2026-07-06 | Updated R1 and references to reflect the `ResourcesSchema` struct replacing `Resources interface{}` (DD-WE-008 v1.2) — no JSON round-trip, quantities parsed directly via `resource.ParseQuantity()`. No test scenario/ID changes; implementation-detail wording only. |
| 1.3 | 2026-07-06 | Rewrote Section 11.2 (Execution Order) to follow AGENTS.md's Wiring-First TDD Sequence per wiring point (IT-then-UT in RED, wire-then-implement in GREEN), matching IMPLEMENTATION_PLAN.md v1.3's restructuring. Relabeled Section 14 heading (was tied to a stale "TDD Phase 4" numbering). No test scenario/ID changes. |
| 1.4 | 2026-07-06 | Bundled audit-count regression guards discovered during preflight (Section 4.4): added `IT-WE-019-003` (new) and extended `E2E-WE-019-001` to prove exactly 1 completion audit event despite tolerated retries/reconciles — both cite **BR-AUDIT-005**, not BR-WE-019 (documented as regression guards with no genuine RED phase, not new-guarantee proof). Bundled mechanical hardening of 5 pre-existing loose (`>= 1`) audit assertions in `reconciler_test.go`/`02_observability_test.go` (Section 15). Added R8 risk. Explicitly excluded `audit_flow_integration_test.go`'s Tekton-timing-related loose assertions — tracked as a separate follow-up issue (needs a design decision, not a mechanical fix). |
| 1.5 | 2026-07-06 | **Reversed the v1.4 "deferred" framing for the audit-*content* gap** after re-investigation showed the original justification incorrect. Added **Wiring Point C** (BR-WE-019 AC10, genuine RED->GREEN): `UT-WE-054-JOB-025`/`026`, new `UT-WE-AUDIT-001`/`002` (new `manager_test.go`), `IT-WE-019-004`, and a further extension of `E2E-WE-019-001`. Updated BR Coverage Matrix, Wiring Verification, Section 6 test-item tables, Section 11.2 execution order, and added R9 risk. Section 4.4 restructured to clearly separate Wiring Point C (real fix) from the pre-existing bundled BR-AUDIT-005 count guards (no genuine RED). |
