# Test Plan: Gateway ManualReviewRequired Terminal Suppression

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-719-v1
**Feature**: Gateway suppresses new RR creation when ManualReviewRequired is terminal for the same fingerprint
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Agent
**Status**: Active
**Branch**: `fix/719-gateway-suppress-manual-review-terminal`

---

## 1. Introduction

### 1.1 Purpose

When an RR reaches a terminal phase (Failed, Completed) with `Outcome=ManualReviewRequired`, the Gateway should suppress new RR creation for the same signal fingerprint. Without this suppression, the Gateway continues creating new RRs from recurring alerts, which the RO immediately blocks — generating noise, wasting enrichment/audit resources, and confusing operators.

### 1.2 Objectives

1. **Suppression correctness**: Terminal RRs with `Outcome=ManualReviewRequired` must cause `ShouldDeduplicate` to return `true`, preventing new RR creation.
2. **Regression safety**: Terminal RRs without `ManualReviewRequired` must continue to allow new RR creation (no behavioral change for normal Failed/Completed/TimedOut).
3. **Phase coverage**: Suppression applies to both `Failed` and `Completed` terminal phases with `ManualReviewRequired` (covers AA confidence gate and IneffectiveChain paths).
4. **Mixed-RR correctness**: When multiple RRs exist for the same fingerprint, `ManualReviewRequired` on any terminal RR wins over non-ManualReviewRequired terminal RRs.
5. **Defense-in-depth with NextAllowedExecution**: Suppression works both with and without `NextAllowedExecution` set, providing defense-in-depth for paths where the delay is 0 or unconfigured.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/gateway/processing/... -ginkgo.v` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `ShouldDeduplicate` and `IsTerminalPhase` |
| Backward compatibility | 0 regressions | Existing phase checker tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-GATEWAY-181**: Deduplication prevents wasteful duplicate remediations
- **BR-ORCH-036**: Manual Review & Escalation Notification (v5.0)
- **BR-ORCH-042**: Consecutive failure blocking (RO responsibility)
- **DD-GATEWAY-011 v1.3**: Phase-Based Deduplication Checker
- **DD-WE-004**: Exponential Backoff Cooldown
- **Issue #719**: Gateway should suppress new RR creation when ManualReviewRequired is terminal

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Existing tests: `test/unit/gateway/processing/phase_checker_business_test.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | ManualReviewRequired suppression indefinitely blocks retry for the same fingerprint | Operators cannot auto-resolve without deleting the ManualReviewRequired RR | Medium | UT-GW-719-001, UT-GW-719-003 | Acceptable for v1.3: suppression is the desired behavior. Acknowledgment mechanism is a follow-up. |
| R2 | String literal "ManualReviewRequired" used instead of a constant | Typos break suppression silently | Low | All UT-GW-719-* | Consistent with codebase convention (kubebuilder enum). Follow-up to extract constant. |
| R3 | Multiple RRs: ManualReviewRequired RR scanned after a normal terminal RR — suppression missed | False negative: new RR created despite ManualReviewRequired existing | Medium | UT-GW-719-004 | New test validates that ManualReviewRequired on any terminal RR in the list triggers suppression, regardless of iteration order. |
| R4 | AA `handleNeedsHumanReview` also sets NextAllowedExecution, creating redundant suppression | Dual suppression is harmless (defense-in-depth) but confusing to debug | Low | N/A | Document in code comments; both mechanisms are correct. |
| R5 | Blocked→Failed transition preserves ManualReviewRequired Outcome | Outcome persists through phase transition; suppression applies to final state | Low | UT-GW-719-001 | Covered by existing test on Failed+ManualReviewRequired. |

### 3.1 Risk-to-Test Traceability

- **R1**: Covered by UT-GW-719-001 (Failed), UT-GW-719-003 (Completed) — both validate suppression returns existing RR.
- **R3**: Covered by UT-GW-719-004 (new) — validates mixed-RR scenario.
- **R5**: Covered by UT-GW-719-001 — Failed RR with ManualReviewRequired regardless of how it got there.

---

## 4. Scope

### 4.1 Features to be Tested

- **`ShouldDeduplicate` function** (`pkg/gateway/processing/phase_checker.go`): ManualReviewRequired outcome check on terminal RRs.

### 4.2 Features Not to be Tested

- **RO blocking logic**: Separate concern; RO's IneffectiveChain blocking is tested in `test/unit/remediationorchestrator/`.
- **AA ManualReviewRequired setting**: Tested in `test/unit/remediationorchestrator/aianalysis_handler_test.go`.
- **Notification creation**: Tested in `test/integration/remediationorchestrator/needs_human_review_integration_test.go`.
- **Acknowledgment/close mechanism**: Follow-up for post-v1.3.

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Check Outcome before NextAllowedExecution | ManualReviewRequired is a semantic state (human must act), not a time-based one. Checking it first ensures suppression regardless of backoff configuration. |
| Return existing ManualReviewRequired RR | Allows `UpdateDeduplicationStatus` to increment occurrence count, giving operators visibility into duplicate alert frequency. |
| Unit-only testing (no IT/E2E) | `ShouldDeduplicate` is pure logic with a mock K8s client. Integration coverage comes from existing E2E pipeline tests. |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of `ShouldDeduplicate` logic paths (terminal ManualReviewRequired, normal terminal, non-terminal, backoff, mixed-RR)
- **Integration**: N/A — `ShouldDeduplicate` is a self-contained function with a mock K8s client. The Gateway integration path is covered by E2E.
- **E2E**: Existing full-pipeline E2E tests exercise Gateway→RO→AA flow including ManualReviewRequired paths.

### 5.2 Two-Tier Minimum

- **Unit tests**: Validate suppression logic correctness, regression safety, and edge cases.
- **E2E tests**: Existing `test/e2e/remediationorchestrator/needs_human_review_e2e_test.go` covers the full pipeline. No new E2E needed.

### 5.3 Business Outcome Quality Bar

Each test answers: "When a ManualReviewRequired RR exists for a fingerprint, does the Gateway correctly prevent new RR creation (or correctly allow it for normal terminal RRs)?"

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All 4 UT-GW-719-* tests pass (0 failures)
2. All existing phase checker tests pass (0 regressions)
3. `ShouldDeduplicate` path coverage >=80%

**FAIL** — any of the following:

1. Any UT-GW-719-* test fails
2. Any existing phase checker test regresses
3. New suppression logic introduces a code path not covered by tests

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Code does not compile
- `remediationv1alpha1` types change in a way that breaks test fixtures

**Resume testing when**:
- Build fixed and green on CI
- Type definitions stabilized

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/processing/phase_checker.go` | `ShouldDeduplicate` (ManualReviewRequired check, lines 148-150) | ~3 |
| `pkg/gateway/processing/phase_checker.go` | `IsTerminalPhase` (unchanged, regression baseline) | ~10 |

### 6.2 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/719-gateway-suppress-manual-review-terminal` HEAD | Branch |
| Dependency: `remediationv1alpha1` | Current `api/remediation/v1alpha1` | CRD types |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-GATEWAY-181 | Deduplication prevents wasteful duplicate remediations | P0 | Unit | UT-GW-719-001 | Pass |
| BR-GATEWAY-181 | Regression: normal Failed RRs still allow new RR | P0 | Unit | UT-GW-719-002 | Pass |
| BR-GATEWAY-181 | Completed + ManualReviewRequired also suppresses | P0 | Unit | UT-GW-719-003 | Pass |
| BR-GATEWAY-181 | Mixed RRs: ManualReviewRequired wins over normal terminal | P1 | Unit | UT-GW-719-004 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-GW-719-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: `pkg/gateway/processing/phase_checker.go` — `ShouldDeduplicate` function

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-719-001` | Failed RR with ManualReviewRequired suppresses new RR — human intervention required | Pass |
| `UT-GW-719-002` | Failed RR without ManualReviewRequired still allows new RR (regression safety) | Pass |
| `UT-GW-719-003` | Completed RR with ManualReviewRequired also suppresses (covers AA confidence gate path) | Pass |
| `UT-GW-719-004` | Mixed RRs (ManualReviewRequired + normal terminal): suppression wins | Pending |

### Tier Skip Rationale

- **Integration**: `ShouldDeduplicate` uses a mock K8s client (fake.Client). There is no real I/O, DB, or HTTP to test. Integration-testable behavior is covered by E2E.
- **E2E**: Existing `test/e2e/remediationorchestrator/needs_human_review_e2e_test.go` validates the full ManualReviewRequired pipeline. The Gateway suppression change is exercised implicitly.

---

## 9. Test Cases

### UT-GW-719-001: Failed RR with ManualReviewRequired suppresses new RR

**BR**: BR-GATEWAY-181
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/processing/phase_checker_business_test.go`

**Preconditions**:
- Fake K8s client with RemediationRequest scheme and fingerprint index

**Test Steps**:
1. **Given**: A Failed RR exists with `Outcome="ManualReviewRequired"` and matching fingerprint
2. **When**: `ShouldDeduplicate` is called with the same namespace and fingerprint
3. **Then**: Returns `(true, existingRR, nil)` — new RR creation is suppressed

**Expected Results**:
1. `shouldDedup == true`
2. `existingRR.Name == "rr-manual-review-terminal"`
3. No error returned

**Acceptance Criteria**:
- **Behavior**: Gateway does not create a new RR
- **Correctness**: Returns the ManualReviewRequired RR as the existing match
- **Accuracy**: Occurrence tracking can be updated on the returned RR

### UT-GW-719-002: Normal Failed RR allows new RR (regression)

**BR**: BR-GATEWAY-181
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/processing/phase_checker_business_test.go`

**Preconditions**:
- Fake K8s client with a Failed RR that has no Outcome set

**Test Steps**:
1. **Given**: A Failed RR exists with empty Outcome and matching fingerprint
2. **When**: `ShouldDeduplicate` is called
3. **Then**: Returns `(false, nil, nil)` — new RR creation is allowed

**Expected Results**:
1. `shouldDedup == false`
2. `existingRR == nil`
3. No error returned

**Acceptance Criteria**:
- **Behavior**: Normal terminal behavior unchanged
- **Correctness**: Only ManualReviewRequired triggers suppression
- **Accuracy**: No false-positive suppression

### UT-GW-719-003: Completed RR with ManualReviewRequired also suppresses

**BR**: BR-GATEWAY-181
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/processing/phase_checker_business_test.go`

**Preconditions**:
- Fake K8s client with a Completed RR that has `Outcome="ManualReviewRequired"`

**Test Steps**:
1. **Given**: A Completed RR exists with `Outcome="ManualReviewRequired"` (AA confidence gate path via `handleNeedsHumanReview`)
2. **When**: `ShouldDeduplicate` is called
3. **Then**: Returns `(true, existingRR, nil)`

**Expected Results**:
1. `shouldDedup == true`
2. `existingRR` is non-nil
3. No error returned

**Acceptance Criteria**:
- **Behavior**: Suppression is phase-agnostic — any terminal phase with ManualReviewRequired suppresses
- **Correctness**: Covers both AA handler paths (handleNeedsHumanReview → Completed, handleWorkflowResolutionFailed → Failed)

### UT-GW-719-004: Mixed RRs — ManualReviewRequired wins over normal terminal

**BR**: BR-GATEWAY-181
**Priority**: P1
**Type**: Unit
**File**: `test/unit/gateway/processing/phase_checker_business_test.go`

**Preconditions**:
- Fake K8s client with TWO RRs for the same fingerprint:
  - RR-A: Completed with `Outcome="Remediated"` (normal successful completion)
  - RR-B: Failed with `Outcome="ManualReviewRequired"` (subsequent failure requiring human review)

**Test Steps**:
1. **Given**: Both RRs exist in the same namespace with the same fingerprint
2. **When**: `ShouldDeduplicate` is called
3. **Then**: Returns `(true, existingRR, nil)` where existingRR is the ManualReviewRequired one

**Expected Results**:
1. `shouldDedup == true` (ManualReviewRequired wins)
2. `existingRR.Status.Outcome == "ManualReviewRequired"`
3. No error returned

**Acceptance Criteria**:
- **Behavior**: Even when a normal terminal RR coexists, ManualReviewRequired suppression applies
- **Correctness**: The iteration order of RRs does not affect the result
- **Accuracy**: The ManualReviewRequired RR is returned (not the Remediated one) for occurrence tracking

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `sigs.k8s.io/controller-runtime/pkg/client/fake` for K8s client
- **Location**: `test/unit/gateway/processing/`
- **Resources**: Minimal (no I/O, no containers)

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — all dependencies (CRD types, fake client) are available.

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Write UT-GW-719-004 (the gap test). Verify 001-003 are passing.
2. **Phase 2 (TDD GREEN)**: Verify implementation passes UT-GW-719-004. May require no changes if current implementation already handles mixed-RR case.
3. **Phase 3 (TDD REFACTOR)**: Improve code documentation, verify no duplication with backoff logic.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/719/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/gateway/processing/phase_checker_business_test.go` | UT-GW-719-001 through 004 |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/gateway/processing/... -ginkgo.v

# Specific test by ID
go test ./test/unit/gateway/processing/... -ginkgo.focus="UT-GW-719"

# Coverage
go test ./test/unit/gateway/processing/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | N/A | Existing tests are unaffected by the ManualReviewRequired suppression |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan (retroactive — implementation already landed in commit 1a7ae269c) |
