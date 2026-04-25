# Test Plan: Dry-Run Mode (#712, #736)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-712-v1
**Feature**: Dry-run mode — pipeline stops after AI analysis without creating WFE/EA
**Version**: 1.0
**Created**: 2026-04-24
**Author**: AI Assistant
**Status**: Active
**Branch**: `feature/v1.0-remaining-bugs-demos`

---

## 1. Introduction

### 1.1 Purpose

Validate that the dry-run mode implementation correctly stops the remediation pipeline after AI analysis, produces the correct terminal state on the RemediationRequest, prevents Gateway infinite loops, and does not interfere with existing non-dry-run behavior.

### 1.2 Objectives

1. **Pipeline intercept**: When dry-run is enabled, the RO completes the RR immediately after AI analysis — no WFE or RAR is created.
2. **Terminal state correctness**: The RR reaches Completed phase with outcome `DryRun`, CompletedAt set, and NextAllowedExecution set for GW suppression.
3. **Regression safety**: When dry-run is disabled, all existing behavior is unchanged.
4. **Cross-service safety**: KA does not include `DryRun` in recurring detection.
5. **Config safety**: Invalid dry-run configuration is rejected at startup.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on touched files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |
| Build integrity | 0 errors | `go build ./...` |

---

## 2. References

### 2.1 Authority

- Issue [#712](https://github.com/jordigilh/kubernaut/issues/712): Conditional EA policy
- Issue [#736](https://github.com/jordigilh/kubernaut/issues/736): EA policy decoupling
- [ADR-RO-001](../../architecture/decisions/ADR-RO-001-dry-run-mode-ea-policy-decoupling.md): Dry-run mode architectural decision

### 2.2 Cross-References

- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [ADR-062](../../architecture/decisions/ADR-062-phase-handler-registry-pattern.md): Phase handler registry pattern
- [ADR-EM-001](../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md): EA creation pipeline

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Gateway infinite loop when dry-run RRs complete immediately | System flooding, etcd write amplification | High if unmitigated | UT-RO-712-007, 007b | NextAllowedExecution set with minimum 5m hold period |
| R2 | Dry-run shadows AI "no action needed" determination | Silent miscategorization of NoActionRequired as DryRun | Medium | UT-RO-712-011 | Intercept placed AFTER WorkflowNotNeeded check |
| R3 | KA treats DryRun as completed remediation for recurring detection | False escalation warnings to operator | Medium | UT-RO-712-012 | DryRun excluded from completedOutcomes |
| R4 | DryRunHoldPeriod=0 causes infinite GW loop | System flooding | High if unmitigated | UT-RO-712-013 | Config validation enforces >= 5m |
| R5 | Double-reconcile emits duplicate metrics/audit | Incorrect dashboards and audit trail | Low | UT-RO-712-005b | apiReader idempotency guard |

---

## 4. Scope

### 4.1 Features to be Tested

- **TransitionIntent system** (`pkg/remediationorchestrator/phase/transition.go`): `CompleteWithoutVerification` constructor, `Validate()`, `String()`
- **ApplyTransition dispatch** (`internal/controller/remediationorchestrator/apply_transition.go`): `TransitionCompletedWithoutVerification` case
- **Reconciler terminal transition** (`internal/controller/remediationorchestrator/reconciler.go`): `transitionToCompletedWithoutVerification` method
- **AnalyzingHandler intercept** (`internal/controller/remediationorchestrator/analyzing_handler.go`): `IsDryRun` callback, dry-run check in `handleCompleted`
- **KA recurring detection** (`internal/kubernautagent/prompt/history.go`): `completedOutcomes` exclusion
- **Config validation** (`internal/config/remediationorchestrator/config.go`): `DryRunHoldPeriod` bounds

### 4.2 Features Not to be Tested

- **Helm chart values**: Deferred; operators use ConfigMap patches for v1.4
- **Dry-run notifications**: Deferred to v1.5 (#116)
- **Non-K8s target skip**: Deferred to Goose milestone
- **Integration/E2E tests**: Deferred; unit tests provide >=80% coverage. Integration test isolation requires suite changes incompatible with parallel execution.
- **Config hot-reload**: RO reads config once at startup; hot-reload is a separate issue

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code touched by this feature

### 5.2 Business Outcome Quality Bar

Each test validates a business outcome, not just code path coverage. Tests answer: "what does the operator/system get?" not "what function is called?"

### 5.3 Pass/Fail Criteria

**PASS**: All 15 tests pass, `go build ./...` succeeds, existing tests have 0 regressions.

**FAIL**: Any P0 test fails, or existing tests regress.

### 5.4 Suspension & Resumption

**Suspend**: If `go build ./...` fails due to unrelated changes on the branch.
**Resume**: When build is fixed.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/phase/transition.go` | `CompleteWithoutVerification`, `Validate`, `String` | ~10 |
| `internal/controller/remediationorchestrator/apply_transition.go` | `ApplyTransition` (new case) | ~5 |
| `internal/controller/remediationorchestrator/reconciler.go` | `transitionToCompletedWithoutVerification`, `SetDryRun` | ~50 |
| `internal/controller/remediationorchestrator/analyzing_handler.go` | `handleCompleted` (dry-run check), `IsDryRun` callback | ~5 |
| `internal/kubernautagent/prompt/history.go` | `completedOutcomes` map | ~1 |
| `internal/config/remediationorchestrator/config.go` | `Validate` (dry-run bounds) | ~5 |

---

## 7. BR Coverage Matrix

| AC ID | Description | Priority | Tier | Test IDs | Status |
|-------|-------------|----------|------|----------|--------|
| AC-736-3 | RO skips Verifying when EA policy says "no verification needed" | P0 | Unit | UT-RO-712-001, 002, 003, 005, 005b, 006, 008, 009 | Pending |
| AC-736-4 | KA completedOutcomes does NOT include DryRun | P0 | Unit | UT-RO-712-012 | Pending |
| AC-712-GW | GW does not re-trigger same alert in tight loop after dry-run | P0 | Unit | UT-RO-712-007, 007b | Pending |
| AC-REGR | Dry-run does NOT alter existing behavior when disabled | P0 | Unit | UT-RO-712-010, 011 | Pending |
| AC-CONFIG | Invalid dry-run configuration is rejected at startup | P1 | Unit | UT-RO-712-013 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Transition Intent System** (`test/unit/remediationorchestrator/transition_intent_test.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|-----------------------------|----------|-------|
| UT-RO-712-001 | The phase transition system accepts "complete without verification" as a valid, processable transition so the RO can express a dry-run terminal decision | P0 | Pending |
| UT-RO-712-002 | A dry-run completion intent carries no failure, block, or execution-related state — only a reason string — ensuring no accidental side effects leak into the transition | P1 | Pending |
| UT-RO-712-003 | The transition type registry includes "CompletedWithoutVerification" in its human-readable name map so metrics, logs, and audit events can identify dry-run transitions | P1 | Pending |

**RR Terminal State** (`test/unit/remediationorchestrator/controller/apply_transition_test.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|-----------------------------|----------|-------|
| UT-RO-712-005 | When the RO decides to skip verification, the RR reaches Completed phase with outcome "DryRun" — the operator sees a clean terminal state indicating the pipeline stopped by policy | P0 | Pending |
| UT-RO-712-005b | If transitionToCompletedWithoutVerification is called on an already-Completed RR (double-reconcile), it is a no-op — no duplicate status update, metrics, or audit | P0 | Pending |
| UT-RO-712-006 | A dry-run RR that completes has CompletedAt set, so retention TTL housekeeping and operator dashboards reflect the correct completion timestamp | P0 | Pending |
| UT-RO-712-007 | After dry-run completion, NextAllowedExecution is set to now+holdPeriod, preventing the Gateway from creating a new RR for the same still-firing alert | P0 | Pending |
| UT-RO-712-007b | If the RR already has a NextAllowedExecution from a prior failure backoff that is later than now+dryRunHoldPeriod, the existing value is preserved | P0 | Pending |

**Handler Dry-Run Intercept** (`test/unit/remediationorchestrator/controller/analyzing_handler_test.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|-----------------------------|----------|-------|
| UT-RO-712-008 | When dry-run is enabled and AI selects a high-confidence workflow (direct path), the RO completes the RR immediately — no WFE CRD is created | P0 | Pending |
| UT-RO-712-009 | When dry-run is enabled and AI selects a low-confidence workflow (approval path), the RO completes the RR immediately — no RAR is created | P0 | Pending |
| UT-RO-712-010 | When dry-run is disabled, the analyzing handler follows normal execution/approval paths unchanged — zero regression risk | P0 | Pending |
| UT-RO-712-011 | When AI determines no action is needed (WorkflowNotNeeded), the RR completes with NoActionRequired regardless of dry-run — dry-run does not shadow the AI's determination | P0 | Pending |

**Cross-Service Safety** (`test/unit/kubernautagent/prompt/history_test.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|-----------------------------|----------|-------|
| UT-RO-712-012 | DryRun outcomes are excluded from KA's recurring detection logic, so dry-run RRs do not trigger false escalation warnings | P0 | Pending |

**Configuration Safety** (`test/unit/remediationorchestrator/config_test.go`)

| ID | Business Outcome Under Test | Priority | Phase |
|----|-----------------------------|----------|-------|
| UT-RO-712-013 | A DryRunHoldPeriod below 5m is rejected at startup, preventing near-zero GW suppression that would risk infinite loops | P1 | Pending |

### Tier Skip Rationale

- **Integration**: Suite runs per-process reconcilers in parallel. `SetDryRun(true)` on the shared reconciler would affect all non-dry-run specs. Isolation requires either a separate suite invocation or Serial Describe, both adding CI complexity disproportionate to the value for a single-flag feature. Deferred.
- **E2E**: Requires Kind cluster with full pipeline. Coverage is adequate at the unit level for this feature. Deferred.

---

## 9. Test Cases (P0 Details)

### UT-RO-712-001: CompleteWithoutVerification constructor

**Priority**: P0
**File**: `test/unit/remediationorchestrator/transition_intent_test.go`

**Test Steps**:
1. **Given**: The phase transition system
2. **When**: `CompleteWithoutVerification("dry-run mode enabled")` is called
3. **Then**: Returns a `TransitionIntent` with `Type == TransitionCompletedWithoutVerification`, `Reason == "dry-run mode enabled"`, and `Validate()` returns nil

### UT-RO-712-008: Direct execution path intercepted by dry-run

**Priority**: P0
**File**: `test/unit/remediationorchestrator/controller/analyzing_handler_test.go`

**Test Steps**:
1. **Given**: An AnalyzingHandler with `IsDryRun` returning true, and an RR in Analyzing phase with a completed AIAnalysis that has a high-confidence workflow selected
2. **When**: `Handle(ctx, rr)` is called
3. **Then**: Returns `TransitionCompletedWithoutVerification` intent with reason "dry-run mode enabled". No WFE is created. No lock is acquired.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `fake.NewClientBuilder()` for K8s client
- **Location**: `test/unit/remediationorchestrator/`, `test/unit/kubernautagent/prompt/`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. All required types (`TransitionCompletedWithoutVerification`, `CompleteWithoutVerification`, `DryRun` outcome) are already scaffolded.

### 11.2 Execution Order (TDD Phases)

1. **Phase 1 (RED)**: Write all 15 failing tests
2. **Phase 2 (GREEN)**: Minimal implementation to pass all tests
3. **Phase 3 (REFACTOR)**: Constants, events, audit emission, cleanup, linter

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/712/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/remediationorchestrator/` | Ginkgo BDD test files |
| KA unit test | `test/unit/kubernautagent/prompt/` | completedOutcomes exclusion test |

---

## 13. Execution

```bash
# All dry-run tests
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-712" -ginkgo.v

# KA test
go test ./test/unit/kubernautagent/prompt/... -ginkgo.focus="UT-RO-712" -ginkgo.v

# Full RO unit suite (regression check)
go test ./test/unit/remediationorchestrator/... -ginkgo.v

# Build integrity
go build ./...
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `noopAnalyzingCallbacks()` in `analyzing_handler_test.go` | No `IsDryRun` field | Add `IsDryRun: func() bool { return false }` | New required callback field |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-24 | Initial test plan |
