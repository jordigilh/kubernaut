# Test Plan: Remediation Orchestrator — Distributed Locking for WorkflowExecution Creation

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-189-v1
**Feature**: Lease-based distributed lock keyed by `hash(targetResource)` before WorkflowExecution creation to prevent duplicate WFEs when multiple RemediationRequests (different fingerprints) race for the same target
**Version**: 1.0
**Created**: 2026-04-09
**Author**: Kubernaut Engineering
**Status**: Draft
**Branch**: `development/v1.3`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

Issue #189 addresses a race: multiple alerts for the same target produce multiple RemediationRequests with different fingerprints; both can pass `CheckResourceBusy` and `CheckDuplicateInProgress` (fingerprint-scoped), leading to two `WorkflowExecution` objects and a failure at execution level for the second. This plan defines tests that prove a **coordination.k8s.io Lease** lock, acquired before WE creation and released on failure or terminal WE phase, serializes WE creation per target and preserves operator-visible correctness without weakening existing deduplication semantics.

### 1.2 Objectives

1. **Lock acquisition semantics**: `DistributedLockManager` (shared package) acquires a Lease keyed by deterministic `hash(targetResource)` and returns clear errors on contention.
2. **RO integration**: Remediation Orchestrator reconciler acquires the lock immediately before `weCreator.Create`, releases on creation failure, and releases (or relies on TTL + re-acquire rules) consistent with ADR-052 patterns.
3. **Concurrency safety**: Parallel reconciles for two RRs targeting the same resource result in **exactly one** WFE (P0 integration outcome).
4. **Recovery**: After lock holder failure or terminal WE phase, a subsequent RR can acquire the lock and proceed.
5. **Observability & RBAC**: RO has `create`/`get`/`update`/`delete` on Leases; lock helper remains unit-testable in isolation from the full reconciler.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/... ./test/unit/shared/...` (as lock package lands) |
| Integration test pass rate | 100% | `go test ./test/integration/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on lock helper and key derivation |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on RO reconciler paths using envtest + real Lease API |
| Backward compatibility | 0 unintended regressions | Existing RO/WE tests pass; new tests document any intentional assertion changes |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-ORCH-025: WorkflowExecution creation orchestration (scope per BR text)
- BR-ORCH-031: WorkflowExecution creation orchestration (scope per BR text)
- ADR-052: Distributed locking pattern (Gateway); reuse for RO via shared `DistributedLockManager`
- BR-GATEWAY-190: Reference implementation context for `pkg/gateway/processing/distributed_lock.go`
- Issue #189: RO distributed locking for WE creation

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Lock never released (panic path) | Stuck target until Lease TTL | Medium | IT-RO-189-002, UT-RO-189-005 | `defer` release where safe; TTL + explicit release on WE creation failure; IT verifies recovery |
| R2 | Wrong key derivation (collision or instability) | Incorrect serialization or cross-target blocking | High | UT-RO-189-004 | Deterministic hash tests; documented canonical serialization of `targetResource` |
| R3 | RBAC / apiReader mis-wiring | Reconciler cannot create Leases; silent skip or error storm | Medium | IT-RO-189-001 | Integration tests with real apiserver; manifest RBAC verification |
| R4 | Deadlock with existing `CheckResourceBusy` | Ordering conflicts or double-blocking | Low | IT-RO-189-001 | Maintain ordering: existing checks then lock then Create; IT exercises parallel RRs |
| R5 | Metrics / lease name drift from Gateway | Operational inconsistency | Low | REFACTOR phase | Share `generateLeaseName` with prefix parameter; document ADR-052 reuse |

### 3.1 Risk-to-Test Traceability

- **R1**: IT-RO-189-002 (release on WE creation failure), UT-RO-189-005 (expired lease re-acquire)
- **R2**: UT-RO-189-004 (deterministic, collision-free keys)
- **R3**: IT-RO-189-001 (envtest Lease API + RO RBAC effective in test cluster)
- **R4**: IT-RO-189-001 (parallel reconciles, single WFE)

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **Shared lock manager** (`pkg/shared/...` after extraction from `pkg/gateway/processing/distributed_lock.go`): acquire, release, lease naming, TTL behavior
- **RO reconciler** (`internal/controller/remediationorchestrator/reconciler.go`): lock acquire before `weCreator.Create`, defer/release on failure, interaction with terminal WE phase
- **Routing / blocking context** (`routing/blocking.go`): Existing `CheckDuplicateInProgress`, `CheckResourceBusy` remain; tests prove lock closes the fingerprint race gap
- **RBAC**: `coordination.k8s.io` Leases — create, get, update, delete for RO ServiceAccount

### 4.2 Features Not to be Tested

- **Gateway-specific lock consumers**: Covered by existing Gateway tests; this plan focuses on RO + shared extraction
- **Full cluster E2E / multi-replica HA soak**: Deferred; envtest + integration tier validates Kubernetes Lease semantics sufficiently for this change
- **Altering fingerprint dedup semantics**: Out of scope unless a BR explicitly requires merging duplicate fingerprints

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Extract `DistributedLockManager` to `pkg/shared/` | Single implementation for ADR-052; RO and Gateway both depend on tested abstraction |
| Key = `hash(targetResource)` | Aligns lock granularity with “one WE pipeline per target” regardless of RR fingerprint |
| Lease API over ConfigMap locks | Native TTL, coordination primitives, matches existing Gateway pattern |
| envtest for P0 integration | Real Lease API without full Kind E2E cost |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (lock helper, key derivation, pure lease name logic)
- **Integration**: >=80% of **integration-testable** code (RO reconciler + apiserver Leases, parallel reconcile)
- **E2E**: Deferred — integration with envtest provides real coordination API; add E2E only if production incident warrants multi-controller soak

### 5.2 Two-Tier Minimum

- **Unit tests**: Lock behavior, key stability, TTL edge cases
- **Integration tests**: Parallel reconciles, RBAC, release on failure and terminal phase

### 5.3 Business Outcome Quality Bar

Tests prove the operator gets **at most one in-flight WE creation sequence per target** under concurrent RR admission, and that transient failures **do not permanently block** the target beyond defined TTL/release rules.

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures), including **IT-RO-189-001**
2. Per-tier code coverage meets >=80% threshold on lock helper and RO integration surfaces
3. No regressions in existing remediation orchestrator / workflow execution suites tied to this flow
4. RBAC manifests allow RO Lease verbs required by integration tests

**FAIL** — any of the following:

1. Any P0 test fails (especially duplicate WFE under parallel reconcile)
2. Per-tier coverage falls below 80% on the scoped packages
3. Lock leaks prevent a subsequent RR from proceeding after documented failure/terminal conditions

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:

- envtest / control-plane binaries unavailable or flaky
- Shared lock package not extracted (compilation blocks RED/GREEN)
- RBAC for Leases not merged, blocking realistic integration

**Resume testing when**:

- `go build ./...` succeeds with shared lock + RO wiring
- RBAC updated and loaded in test harness
- envtest environment healthy

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested, with version identification.

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `pkg/shared/.../distributed_lock.go` (TBD exact path) | `AcquireLock`, `ReleaseLock`, `generateLeaseName` (shared w/ prefix), key derivation helpers | TBD post-extraction |
| Key derivation helper(s) | `HashTargetResource` / equivalent | TBD |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | Reconcile path around `weCreator.Create`, lock acquire/release | ~3627 (file) |
| `pkg/remediationorchestrator/creator/workflowexecution.go` | WE creation (called under lock) | TBD |
| RO RBAC manifests | Lease permissions | — |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.3` HEAD | Feature branch for #189 |
| Reference pattern | `pkg/gateway/processing/distributed_lock.go` | Pre-extraction source |
| Kubernetes envtest | envtest-bundled API | coordination.k8s.io/v1 Lease |

---

## 7. BR Coverage Matrix

> Kubernaut-specific. Maps every business requirement to test scenarios across tiers.

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-025 | WE creation orchestration | P0 | Unit | UT-RO-189-001, UT-RO-189-002, UT-RO-189-003 | Pending |
| BR-ORCH-025 | WE creation orchestration | P0 | Integration | IT-RO-189-001, IT-RO-189-002 | Pending |
| BR-ORCH-031 | WE creation orchestration | P0 | Unit | UT-RO-189-004, UT-RO-189-005 | Pending |
| BR-ORCH-031 | WE creation orchestration | P1 | Integration | IT-RO-189-003 | Pending |
| ADR-052 | Distributed lock pattern reuse | P1 | Unit | UT-RO-189-001–003 | Pending |
| ADR-052 | Distributed lock pattern reuse | P1 | Integration | IT-RO-189-001–003 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTOR**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

> **IEEE 829 Test Design Specification** — How test cases are organized and grouped.

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}` — Issue **189** used as traceability anchor (aligned with `docs/tests/189/`).

### Tier 1: Unit Tests

**Testable code scope**: Shared `DistributedLockManager`, lease name generation, target hash/key derivation — **>=80%** coverage on extracted unit-testable surface.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-189-001` | Lock acquired when Lease available; reconciler path can proceed | Pending |
| `UT-RO-189-002` | Contention surfaces as explicit error; caller can requeue | Pending |
| `UT-RO-189-003` | Held lock released; Lease deleted or released per implementation contract | Pending |
| `UT-RO-189-004` | Same target → same key; distinct targets → distinct keys (no accidental sharing) | Pending |
| `UT-RO-189-005` | Expired / stale Lease allows re-acquisition per Kubernetes semantics | Pending |

### Tier 2: Integration Tests

**Testable code scope**: RO reconciler with fake K8s client **where permitted**, but Lease tests use **envtest** real API per no-mocks policy for integration — **>=80%** on integration-testable RO paths touching locks.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-189-001` | Two RRs same target, parallel reconciles → **only one** WFE created | Pending |
| `IT-RO-189-002` | WE creation fails → lock released → next reconcile can acquire | Pending |
| `IT-RO-189-003` | WE reaches terminal phase (Completed/Failed) → lock semantics allow future work (release or TTL as designed) | Pending |

### Tier 3: E2E Tests (if applicable)

**Testable code scope**: Not required for #189 initial delivery.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| — | — | N/A |

### Tier Skip Rationale (if any tier is omitted)

- **E2E**: Deferred; **IT-RO-189-001** provides P0 confidence with real Lease API via envtest. Revisit if multi-replica controller production issues arise.

---

## 9. Test Cases

> **IEEE 829 Test Case Specification** — Detailed specification for each test case.

### UT-RO-189-001: AcquireLock happy path

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/distributed_lock_test.go` (or `test/unit/remediationorchestrator/...` if colocated — **prefer package under test**)

**Preconditions**:

- Fake K8s client or envtest setup as required by package test conventions
- No pre-existing Lease for the derived lock name

**Test Steps**:

1. **Given**: Clean Lease namespace/name for target hash
2. **When**: `AcquireLock` invoked with valid context and TTL parameters
3. **Then**: Returns nil error; Lease object exists with expected holder identity / spec fields per ADR-052

**Expected Results**:

1. No error returned
2. Subsequent `Get` on coordination Lease succeeds

**Acceptance Criteria**:

- **Behavior**: Lock is acquired exactly once for the empty case
- **Correctness**: Lease metadata matches naming prefix + deterministic key
- **Accuracy**: No mutation of unrelated Leases

**Dependencies**: UT-RO-189-004 (key derivation stable)

---

### UT-RO-189-002: AcquireLock contention

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/distributed_lock_test.go`

**Preconditions**:

- Lease already held (simulate second holder or non-expired lease record)

**Test Steps**:

1. **Given**: Active Lease for same key held by another identity
2. **When**: Second `AcquireLock` call runs
3. **Then**: Returns **contention** error (typed or documented sentinel), no destructive override

**Expected Results**:

1. Error is non-nil and classifiable by caller for requeue/backoff
2. Existing Lease unchanged

**Acceptance Criteria**:

- **Behavior**: Second acquirer does not create duplicate WFE entry path
- **Correctness**: Error contract documented in package doc comment
- **Accuracy**: Holder field remains the first acquirer until released or expired

**Dependencies**: UT-RO-189-001

---

### UT-RO-189-003: ReleaseLock success

**BR**: BR-ORCH-025
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/distributed_lock_test.go`

**Preconditions**:

- Lock held by test identity from UT-RO-189-001 setup

**Test Steps**:

1. **Given**: Held lock from happy path
2. **When**: `ReleaseLock` (or defer hook) executes
3. **Then**: Lease deleted or transitioned per implementation; `AcquireLock` can succeed again without manual cleanup beyond documented wait/TTL

**Expected Results**:

1. Release completes without error
2. Follow-up acquire for same target succeeds

**Acceptance Criteria**:

- **Behavior**: Explicit release frees the slot for the same target
- **Correctness**: No orphaned duplicate lease names for one target
- **Accuracy**: apiserver state matches expected object count

**Dependencies**: UT-RO-189-001

---

### UT-RO-189-004: Lock key derivation

**BR**: BR-ORCH-031
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/distributed_lock_test.go`

**Preconditions**:

- Canonical serialization of `targetResource` defined in implementation

**Test Steps**:

1. **Given**: Fixed target A and target B (distinct)
2. **When**: Keys computed for A, A (repeat), and B
3. **Then**: `key(A)==key(A)`; `key(A)!=key(B)`; stable across process invocations (hash algorithm fixed)

**Expected Results**:

1. Byte-stable or string-stable key equality
2. Collision probability bounded by hash choice (document SHA or FNV, etc.)

**Acceptance Criteria**:

- **Behavior**: Same logical target always maps to same lock
- **Correctness**: Different targets do not share a lock in test matrix
- **Accuracy**: Serialization includes all fields that define “same target” per RO

**Dependencies**: None

---

### UT-RO-189-005: Lock timeout / TTL — re-acquire after expiry

**BR**: BR-ORCH-031
**Priority**: P1
**Type**: Unit
**File**: `test/unit/shared/distributed_lock_test.go`

**Preconditions**:

- Ability to simulate clock or use short TTL with envtest time advancement (choose one in implementation plan)

**Test Steps**:

1. **Given**: Lease with TTL elapsed or `RenewTime` in the past per API rules
2. **When**: `AcquireLock` runs for same target
3. **Then**: New holder can acquire (per Kubernetes lease semantics)

**Expected Results**:

1. No permanent deadlock after expiry
2. Documented interaction with `LeaseSpec` duration fields

**Acceptance Criteria**:

- **Behavior**: Expired lease does not block forever
- **Correctness**: Matches `coordination.k8s.io` Lease lifecycle
- **Accuracy**: Test uses real API when in integration; unit uses injected clock if applicable

**Dependencies**: UT-RO-189-001

---

### IT-RO-189-001: Parallel reconciles — single WFE (P0)

**BR**: BR-ORCH-025
**Priority**: P0
**Type**: Integration
**File**: `test/integration/remediationorchestrator/distributed_lock_we_test.go` (TBD naming)

**Preconditions**:

- envtest control plane running
- RO reconciler wired with real apiserver client and RBAC for Leases
- Two `RemediationRequest` objects same target, different fingerprints, both passing existing routing checks

**Test Steps**:

1. **Given**: RR1 and RR2 admitted for same target resource
2. **When**: Two parallel `Reconcile` invocations (goroutines) race before WE creation completes
3. **Then**: Exactly **one** `WorkflowExecution` is created for that target; second reconcile observes contention and requeues without creating second WFE

**Expected Results**:

1. WFE count for target = 1 after stabilization window
2. No execution-level “second WFE” failure mode for this race

**Acceptance Criteria**:

- **Behavior**: Operator sees single WE pipeline for concurrent duplicate-target RRs
- **Correctness**: List/watch WFE objects confirms singleton
- **Accuracy**: RR statuses reflect backoff/requeue, not silent drop

**Dependencies**: GREEN phase: shared lock + RO wiring + RBAC

---

### IT-RO-189-002: Lock released on WE creation failure

**BR**: BR-ORCH-025
**Priority**: P1
**Type**: Integration
**File**: `test/integration/remediationorchestrator/distributed_lock_we_test.go`

**Preconditions**:

- Forced `weCreator.Create` error (e.g., invalid WFE template or apiserver 409 injected per harness capability)

**Test Steps**:

1. **Given**: RR eligible for WE creation
2. **When**: Create fails after lock acquired
3. **Then**: Lock released (or TTL allows immediate retry per design); subsequent reconcile can acquire

**Expected Results**:

1. No permanent Lease stuck in “held” for successful retry path
2. Metrics optional: contention counter stable

**Acceptance Criteria**:

- **Behavior**: Target not bricked after transient create failure
- **Correctness**: Lease object absent or free post-failure
- **Accuracy**: Logs/events contain identifiable lock release reason

**Dependencies**: IT-RO-189-001

---

### IT-RO-189-003: Lock released on terminal WE phase

**BR**: BR-ORCH-031
**Priority**: P1
**Type**: Integration
**File**: `test/integration/remediationorchestrator/distributed_lock_we_test.go`

**Preconditions**:

- WFE transitions to `Completed` or `Failed` (as defined by API)

**Test Steps**:

1. **Given**: WFE exists and reaches terminal phase while Lease association defined by implementation
2. **When**: RO observes terminal phase cleanup path
3. **Then**: Lock is released or not required for next RR per documented rules; new RR can proceed

**Expected Results**:

1. Subsequent RR for same target not blocked incorrectly
2. Consistent with REFACTOR: contention metrics still valid

**Acceptance Criteria**:

- **Behavior**: Terminal phase does not leave stale lock forever
- **Correctness**: Matches status phase enum used in production
- **Accuracy**: Only one release path fires (no double-delete panic)

**Dependencies**: IT-RO-189-001

---

## 10. Environmental Needs

> **IEEE 829 §13** — Hardware, software, tools, and infrastructure required.

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Kubernetes client fakes acceptable **only** at unit tier for lock unit tests; prefer testing through manager interfaces
- **Location**: `test/unit/shared/` or co-located with extracted package
- **Resources**: Default CI runner

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: **ZERO** mocks of Kubernetes API for Lease semantics — use envtest
- **Infrastructure**: envtest (apiserver + etcd), RO CRDs/scheme registered as per existing RO integration suite
- **Location**: `test/integration/remediationorchestrator/`
- **Resources**: Sufficient CPU for parallel tests; typical CI sizing

### 10.3 E2E Tests (if applicable)

- Not required for initial #189 delivery.

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | Project `go.mod` | Build and test |
| Ginkgo CLI | v2.x | BDD runner |
| controller-runtime envtest | Project version | Real Lease API |

---

## 11. Dependencies & Schedule

> **IEEE 829 §12/§16** — Remaining tasks, blocking issues, and execution order.

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Extraction of `DistributedLockManager` | Code | Open until GREEN | Cannot import shared lock from RO | Temporary duplicate (anti-pattern — avoid) |
| RO RBAC for Leases | Manifest | Open until GREEN | IT-RO-189-001 fails | None — must land with code |
| envtest assets | Infra | Existing | Integration blocked | Use existing suite bootstrap |

### 11.2 Execution Order (TDD)

1. **RED**: IT-RO-189-001 — parallel reconcile demonstrates **two WFEs** (current bug) or documents failing assertion; unit tests RED for lock helper
2. **GREEN**: Extract `DistributedLockManager` to `pkg/shared/`, RO acquire before `weCreator.Create`, `defer` release, wire RBAC + `apiReader` as needed; make IT/UT green
3. **REFACTOR**: Share `generateLeaseName` with prefix parameter, add contention metrics, document ADR-052 reuse in code/comments

---

## 12. Test Deliverables

> **IEEE 829 §11** — What artifacts this test effort produces.

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/189/TEST_PLAN.md` | Strategy and traceability |
| Unit test suite | `test/unit/shared/` (TBD) | Lock manager tests |
| Integration test suite | `test/integration/remediationorchestrator/` | envtest parallel reconcile |
| Coverage report | CI artifact | Per-tier >=80% |

---

## 13. Execution

```bash
# Unit tests (adjust package path after extraction)
go test ./test/unit/shared/... -ginkgo.v
go test ./test/unit/remediationorchestrator/... -ginkgo.v

# Integration tests
go test ./test/integration/remediationorchestrator/... -ginkgo.v

# Focus by ID
go test ./test/integration/remediationorchestrator/... -ginkgo.focus="IT-RO-189-001"

# Coverage
go test ./pkg/shared/... -coverprofile=coverage-shared.out
go tool cover -func=coverage-shared.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| TBD during RED | May assume parallel WFE creation possible | Expect single WFE + requeue when second RR races | New locking semantics |

*Populate concrete rows when RED phase identifies exact files.*

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan for Issue #189 |
