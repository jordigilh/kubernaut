# Test Plan: EM ADR-EM-001 Implementation Gaps

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-573-v1
**Feature**: Close ADR-EM-001 implementation gaps (Failed phase, scheduled event, config knobs, assessment paths)
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.2`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates four implementation gaps between ADR-EM-001 and the current EM reconciler, identified during the triage of Issue #530 (dropped). The gaps affect audit trail completeness (SOC2 chain-of-custody), operator observability (`kubectl get ea`), configuration flexibility, and assessment path accuracy.

### 1.2 Objectives

1. **G1 — Failed phase**: EM transitions EA to `Failed` for unrecoverable conditions; RO handles `Failed` EA phase by setting the correct condition on the RR.
2. **G2 — Scheduled event timing**: `effectiveness.assessment.scheduled` audit event is emitted on all three entry transitions (WaitingForPropagation, Stabilizing, Assessing), not just Assessing.
3. **G3 — Config knobs**: `prometheusLookback`, `maxConcurrentReconciles`, and `scrapeInterval` are configurable via YAML, with defaults aligned to ADR-EM-001 §10.
4. **G4 — Assessment path differentiation**: Reconciler branches assessment depth based on WFE started/completed status per ADR-EM-001 §5.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/effectivenessmonitor/...` |
| Integration test pass rate | 100% | `go test ./test/integration/effectivenessmonitor/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| Backward compatibility | 0 regressions | Existing 29 UT + 22 IT pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- ADR-EM-001 (v2.5): `docs/architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md`
- DD-METRICS-001: `docs/architecture/decisions/DD-METRICS-001-controller-metrics-wiring-pattern.md`
- Issue #573: EM ADR-EM-001 implementation gaps

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- ADR-EM-001 §5: Failed/TimedOut remediation paths
- ADR-EM-001 §9.2.0: `assessment.scheduled` event timing
- ADR-EM-001 §10: Configuration schema
- ADR-EM-001 §11: Error handling / EA lifecycle table

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | G1: Adding `PhaseFailed` changes the EA terminal-state set; RO's EA watcher may not recognize it | RR stuck in Verifying forever | Medium | UT-EM-573-001, IT-EM-573-010 | Test RO condition update for Failed EA alongside Completed |
| R2 | G2: Emitting scheduled event earlier (WFP/Stabilizing) could duplicate events if reconciler re-enters | Audit trail noise / double-counted events | Low | UT-EM-573-005, IT-EM-573-011 | Guard emission with `phase == current target phase` (emit once per transition) |
| R3 | G3: Changing `PrometheusLookback` default from 10m to 30m may break E2E tests that rely on fast turnaround | E2E test timeout | Low | UT-EM-573-007 | E2E overrides default via config; unit tests validate default value only |
| R4 | G4: Adding `HasWorkflowCompleted` to `DataStorageQuerier` interface is a breaking change | Existing mock implementations fail to compile | Medium | UT-EM-573-009, IT-EM-573-013 | Add method to interface + update all mock implementations in same commit |

### 3.1 Risk-to-Test Traceability

- **R1 (HIGH)**: Mitigated by UT-EM-573-001 (phase transition to Failed), UT-EM-573-002 (completion fields for Failed), IT-EM-573-010 (RO handles Failed EA)
- **R2 (LOW)**: Mitigated by UT-EM-573-005 (event emitted once per transition), IT-EM-573-011 (scheduled event on WFP), IT-EM-573-012 (scheduled event on Stabilizing)
- **R3 (LOW)**: Mitigated by UT-EM-573-007 (default value assertion)
- **R4 (MEDIUM)**: Mitigated by UT-EM-573-009 (mock implements new method), IT-EM-573-013 (reconciler branches on HasWorkflowCompleted)

---

## 4. Scope

### 4.1 Features to be Tested

- **G1: Failed phase** (`internal/controller/effectivenessmonitor/reconciler.go`, `pkg/effectivenessmonitor/phase/types.go`): EM sets `phase=Failed` for unrecoverable conditions; `setCompletionFields` handles Failed reason; RO `trackEffectivenessStatus` handles Failed EA
- **G2: Scheduled event** (`internal/controller/effectivenessmonitor/reconciler.go`): `emitPendingTransitionEvents` (renamed/refactored to `emitScheduledEvent`) called on WFP, Stabilizing, and Assessing transitions
- **G3: Config knobs** (`internal/config/effectivenessmonitor/config.go`, `internal/controller/effectivenessmonitor/reconciler.go`, `cmd/effectivenessmonitor/main.go`): New fields `prometheusLookback`, `maxConcurrentReconciles`, `scrapeInterval` with ADR-aligned defaults
- **G4: Assessment paths** (`internal/controller/effectivenessmonitor/reconciler.go`, `pkg/effectivenessmonitor/client/interfaces.go`): `HasWorkflowCompleted` interface method; reconciler branches: no_execution / partial / full

### 4.2 Features Not to be Tested

- **EM TLS support (Issue #452)**: Already implemented and tested; not affected by these gaps
- **Alert decay detection (Issue #369)**: Already implemented and tested; not affected
- **RO EA creation logic**: Existing and tested; only RO's handling of `Failed` EA is in scope
- **EM Prometheus/AlertManager scoring algorithms**: Existing and tested; assessment paths change *which* components run, not *how* they score

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| `PhaseFailed` reserved for truly unrecoverable conditions (EA spec invalid, missing correlation ID) — not for DS transient failures | DS transient failures are handled by controller-runtime requeue with backoff; validity expiry handles the timeout case. `Failed` means "cannot even attempt assessment." |
| `assessment.scheduled` emitted once per EA lifecycle, on the *first* phase transition out of Pending | Avoids duplicate audit events. WFP/Stabilizing/Assessing are progressive — timing is computed on first transition and doesn't change. |
| `PrometheusLookback` default changed from 10m to 30m to match ADR §10 | ADR is authoritative. E2E tests override via config. |
| `HasWorkflowCompleted` added to `DataStorageQuerier` interface | Required for §5 path differentiation. No backward compatibility needed (pre-release). |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (config parsing, phase validation, assessment reason logic, defaults)
- **Integration**: >=80% of integration-testable code (reconciler lifecycle, DS querier, audit emission, K8s status updates)
- **E2E**: Deferred to CI — existing E2E suite covers EM end-to-end; these gaps don't require new E2E scenarios

### 5.2 Two-Tier Minimum

Every gap is covered by at least 2 test tiers (UT + IT).

### 5.3 Business Outcome Quality Bar

Tests validate observable business outcomes:
- **G1**: Operator sees `phase=Failed` in `kubectl get ea` for unrecoverable assessments
- **G2**: SOC2 auditor can trace assessment timeline from first phase transition (not just from Assessing)
- **G3**: Platform team can tune EM performance via YAML config without code changes
- **G4**: Assessment audit trail accurately reflects the depth of assessment performed

### 5.4 Pass/Fail Criteria

**PASS** — all of the following:
1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Per-tier code coverage meets >=80%
4. Existing 29 UT + 22 IT pass without modification (0 regressions)

**FAIL** — any of the following:
1. Any P0 test fails
2. Per-tier coverage below 80%
3. Regression in existing test suites

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken or existing tests regress after implementation.
**Resume**: Build fixed and all existing tests green.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/config/effectivenessmonitor/config.go` | `DefaultConfig`, `Validate`, `LoadFromFile` | ~188 |
| `internal/controller/effectivenessmonitor/reconciler.go` | `DefaultReconcilerConfig`, `ReconcilerConfig` struct | ~10 |
| `pkg/effectivenessmonitor/phase/types.go` | `CanTransition`, `ValidTransitions` | ~70 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/effectivenessmonitor/reconciler.go` | `Reconcile`, `emitPendingTransitionEvents`, `completeAssessmentWithReason`, `no_execution` guard | ~1700 |
| `pkg/effectivenessmonitor/client/interfaces.go` | `DataStorageQuerier.HasWorkflowCompleted` | ~5 |
| `pkg/effectivenessmonitor/client/ds_querier.go` | `HasWorkflowCompleted` HTTP implementation | ~30 |
| `cmd/effectivenessmonitor/main.go` | Config wiring to reconciler | ~510 |
| `internal/controller/remediationorchestrator/effectiveness_tracking.go` | `trackEffectivenessStatus` for Failed EA | ~50 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.2` HEAD | kubernaut-v1.2 worktree |

---

## 7. BR Coverage Matrix

| Ref | Description | Priority | Tier | Test ID | Status |
|-----|-------------|----------|------|---------|--------|
| ADR-EM-001 §11 | EA transitions to Failed for unrecoverable conditions | P0 | Unit | UT-EM-573-001 | Pending |
| ADR-EM-001 §11 | setCompletionFields populates Failed-specific fields | P0 | Unit | UT-EM-573-002 | Pending |
| ADR-EM-001 §11 | RO sets EffectivenessAssessed condition for Failed EA | P0 | Unit | UT-EM-573-003 | Pending |
| ADR-EM-001 §11 | Phase state machine allows transitions to Failed | P1 | Unit | UT-EM-573-004 | Pending |
| ADR-EM-001 §9.2.0 | Scheduled event emitted once per EA lifecycle on first transition | P0 | Unit | UT-EM-573-005 | Pending |
| ADR-EM-001 §10 | Config: prometheusLookback parsed from YAML, default 30m | P0 | Unit | UT-EM-573-006 | Pending |
| ADR-EM-001 §10 | Config: default values match ADR §10 | P1 | Unit | UT-EM-573-007 | Pending |
| ADR-EM-001 §10 | Config: validation rejects invalid prometheusLookback/scrapeInterval | P1 | Unit | UT-EM-573-008 | Pending |
| ADR-EM-001 §5 | DataStorageQuerier.HasWorkflowCompleted returns correct boolean | P0 | Unit | UT-EM-573-009 | Pending |
| ADR-EM-001 §11 | Reconciler transitions EA to Failed on invalid spec | P0 | Integration | IT-EM-573-010 | Pending |
| ADR-EM-001 §9.2.0 | Reconciler emits assessment.scheduled on WFP transition | P0 | Integration | IT-EM-573-011 | Pending |
| ADR-EM-001 §9.2.0 | Reconciler emits assessment.scheduled on Stabilizing transition | P0 | Integration | IT-EM-573-012 | Pending |
| ADR-EM-001 §5 | Reconciler performs partial assessment when WFE started but not completed | P0 | Integration | IT-EM-573-013 | Pending |
| ADR-EM-001 §5 | Reconciler performs full assessment when WFE completed but RR failed | P1 | Integration | IT-EM-573-014 | Pending |
| ADR-EM-001 §10 | PrometheusLookback config wired from YAML to reconciler | P1 | Integration | IT-EM-573-015 | Pending |

### Status Legend

- **Pending**: Specification complete, implementation not started
- **RED**: Failing test written (TDD RED phase)
- **GREEN**: Minimal implementation passes (TDD GREEN phase)
- **REFACTORED**: Code cleaned up (TDD REFACTOR phase)
- **Pass**: Implemented and passing

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-EM-573-{SEQUENCE}` where TIER is `UT` (Unit) or `IT` (Integration).

### Tier 1: Unit Tests

**Testable code scope**: `internal/config/effectivenessmonitor/config.go`, `pkg/effectivenessmonitor/phase/types.go`, `internal/controller/effectivenessmonitor/reconciler.go` (ReconcilerConfig only)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-573-001` | EA transitions to `PhaseFailed` when correlation ID is missing from spec (operator sees `phase=Failed` in kubectl) | Pending |
| `UT-EM-573-002` | `setCompletionFields` for `PhaseFailed` populates `completedAt`, `assessmentReason=unrecoverable`, and `message` with the failure cause | Pending |
| `UT-EM-573-003` | RO `trackEffectivenessStatus` sets `EffectivenessAssessed=True` with reason `AssessmentFailed` when EA phase is Failed | Pending |
| `UT-EM-573-004` | Phase state machine: `CanTransition(Pending, Failed)=true`, `CanTransition(Assessing, Failed)=true`, `CanTransition(Completed, Failed)=false` | Pending |
| `UT-EM-573-005` | `emitScheduledEvent` is a no-op when EA already has `ValidityDeadline` set (prevents duplicate audit events on re-reconcile) | Pending |
| `UT-EM-573-006` | `LoadFromFile` parses `external.prometheusLookback: 30m` and `external.scrapeInterval: 60s` from YAML | Pending |
| `UT-EM-573-007` | `DefaultConfig()` returns `PrometheusLookback=30m`, `ScrapeInterval=60s`, `MaxConcurrentReconciles=10` | Pending |
| `UT-EM-573-008` | `Validate()` rejects `prometheusLookback < 1m`, `scrapeInterval < 5s`, `maxConcurrentReconciles < 1` | Pending |
| `UT-EM-573-009` | Mock `DataStorageQuerier.HasWorkflowCompleted` returns true when `workflowexecution.workflow.completed` event exists, false otherwise | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `internal/controller/effectivenessmonitor/reconciler.go` (full Reconcile flow), `cmd/effectivenessmonitor/main.go` (config wiring), `internal/controller/remediationorchestrator/effectiveness_tracking.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-EM-573-010` | Reconciler transitions EA to `Failed` when spec has empty correlationID; EA status shows `phase=Failed`, `assessmentReason=unrecoverable`, and descriptive `message` | Pending |
| `IT-EM-573-011` | Reconciler emits `effectiveness.assessment.scheduled` audit event when transitioning from Pending to WaitingForPropagation (async target with `hashComputeDelay`); event payload contains `validityDeadline`, `prometheusCheckAfter`, `alertManagerCheckAfter` | Pending |
| `IT-EM-573-012` | Reconciler emits `effectiveness.assessment.scheduled` audit event when transitioning from Pending to Stabilizing (sync target with stabilization window > 0); event payload contains correct derived timing | Pending |
| `IT-EM-573-013` | When DS reports WFE started but NOT completed, reconciler performs partial assessment: health checks run, hash comparison runs, but assessment completes with `reason=partial` and skips metrics/alert components | Pending |
| `IT-EM-573-014` | When DS reports WFE started AND completed but `RemediationRequestPhase=Failed`, reconciler performs full assessment: all configured components (health, hash, alert, metrics) are assessed | Pending |
| `IT-EM-573-015` | `PrometheusLookback` value from config YAML is wired through `main.go` to `ReconcilerConfig.PrometheusLookback`; Prometheus query uses the configured lookback window | Pending |

### Tier 3: E2E Tests

**Deferred**: Existing E2E suite (`test/e2e/effectivenessmonitor/`) covers the EM end-to-end. These gaps are internal reconciler improvements that don't add new user-facing flows. E2E validation deferred to CI.

### Tier Skip Rationale

- **E2E**: These gaps are reconciler-internal improvements (phase naming, event timing, config wiring, path branching). The existing E2E suite validates the full EM lifecycle including all four assessment components. No new E2E-observable behavior is introduced.

---

## 9. Test Cases

### UT-EM-573-001: EA transitions to Failed on missing correlationID

**Ref**: ADR-EM-001 §11
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/failed_phase_test.go`

**Preconditions**:
- EA CRD exists with `spec.correlationID = ""`

**Test Steps**:
1. **Given**: An EffectivenessAssessment with empty `correlationID`
2. **When**: The reconciler processes the EA
3. **Then**: EA status phase is `Failed`, assessmentReason is `unrecoverable`, message contains "correlationID is required"

**Acceptance Criteria**:
- **Behavior**: Reconciler short-circuits to Failed without attempting any component checks
- **Correctness**: `ea.Status.Phase == PhaseFailed`, `ea.Status.AssessmentReason == "unrecoverable"`
- **Accuracy**: `ea.Status.Message` describes the specific validation failure

---

### UT-EM-573-004: Phase state machine allows Failed transitions

**Ref**: ADR-EM-001 §11
**Priority**: P1
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/failed_phase_test.go`

**Preconditions**:
- Phase validation functions available

**Test Steps**:
1. **Given**: The phase state machine
2. **When**: Checking transitions to `Failed`
3. **Then**: `CanTransition(Pending, Failed)=true`, `CanTransition(WFP, Failed)=true`, `CanTransition(Stabilizing, Failed)=true`, `CanTransition(Assessing, Failed)=true`, `CanTransition(Completed, Failed)=false`, `CanTransition(Failed, Failed)=false`

**Acceptance Criteria**:
- **Behavior**: Failed is reachable from all non-terminal phases
- **Correctness**: Terminal phases (Completed, Failed) cannot transition to Failed

---

### UT-EM-573-006: Config parses prometheusLookback and scrapeInterval

**Ref**: ADR-EM-001 §10
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/config_573_test.go`

**Preconditions**:
- YAML config file with `external.prometheusLookback: 30m` and `external.scrapeInterval: 60s`

**Test Steps**:
1. **Given**: A YAML file with custom `prometheusLookback` and `scrapeInterval` values
2. **When**: `LoadFromFile` parses the YAML
3. **Then**: `config.External.PrometheusLookback == 30m`, `config.External.ScrapeInterval == 60s`

**Acceptance Criteria**:
- **Behavior**: New fields are parsed without affecting existing fields
- **Correctness**: Duration values are correctly deserialized from YAML string representation
- **Accuracy**: Zero-value/missing fields fall back to defaults

---

### IT-EM-573-011: Scheduled event on WFP transition

**Ref**: ADR-EM-001 §9.2.0
**Priority**: P0
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/scheduled_event_573_test.go`

**Preconditions**:
- envtest cluster with EA CRD installed
- EA with `config.hashComputeDelay = 4m` (async target)
- Mock AuditManager recording calls

**Test Steps**:
1. **Given**: An EA with async hash compute delay (WFP path)
2. **When**: The reconciler processes the EA for the first time
3. **Then**: EA transitions to `WaitingForPropagation` AND `RecordAssessmentScheduled` is called with the EA and validity window

**Acceptance Criteria**:
- **Behavior**: Audit event emitted on first phase transition, not deferred to Assessing
- **Correctness**: Event payload contains `validityDeadline`, `prometheusCheckAfter`, `alertManagerCheckAfter` matching computed derived timing
- **Accuracy**: Event is emitted exactly once (not duplicated on subsequent reconciles while still in WFP)

---

### IT-EM-573-013: Partial assessment when WFE started but not completed

**Ref**: ADR-EM-001 §5
**Priority**: P0
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/assessment_paths_573_test.go`

**Preconditions**:
- envtest cluster with EA CRD
- DS mock: `HasWorkflowStarted = true`, `HasWorkflowCompleted = false`
- Health check and hash comparison dependencies available

**Test Steps**:
1. **Given**: An EA whose correlation ID maps to a WFE that started but never completed (e.g., execution timed out)
2. **When**: The reconciler completes assessment
3. **Then**: Health and hash components are assessed; metrics and alert components are skipped; EA completes with `reason=partial`

**Acceptance Criteria**:
- **Behavior**: Assessment scope is narrowed to components that are meaningful without a completed workflow
- **Correctness**: `ea.Status.Components.HealthAssessed == true`, `ea.Status.Components.HashComputed == true`, `ea.Status.Components.MetricsAssessed == false`, `ea.Status.Components.AlertAssessed == false`
- **Accuracy**: `ea.Status.AssessmentReason == "partial"`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `DataStorageQuerier` (mock for `HasWorkflowCompleted`), `AuditManager` (mock for `RecordAssessmentScheduled` call tracking)
- **Location**: `test/unit/effectivenessmonitor/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for K8s API (envtest). httptest for DS querier HTTP endpoint. AuditManager uses real implementation against httptest DS server.
- **Infrastructure**: envtest (K8s API), httptest (DS, Prometheus, AlertManager)
- **Location**: `test/integration/effectivenessmonitor/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| envtest | Latest | K8s API for integration tests |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| None | - | - | - | - |

### 11.2 Execution Order

1. **Phase 1 (G3 — Config)**: Unit tests for config parsing/validation/defaults → implementation → integration test for wiring
2. **Phase 2 (G1 — Failed phase)**: Unit tests for phase transitions and completion fields → reconciler implementation → integration test for end-to-end Failed path → RO handling
3. **Phase 3 (G2 — Scheduled event)**: Unit test for emission guard → reconciler refactoring → integration tests for WFP and Stabilizing emission
4. **Phase 4 (G4 — Assessment paths)**: Unit test for HasWorkflowCompleted → reconciler branching → integration tests for partial/full assessment paths

Rationale: Config (Phase 1) is foundational — the reconciler needs the new config fields before other gaps can reference them. Failed phase (Phase 2) is highest risk (R1). Scheduled event (Phase 3) and assessment paths (Phase 4) are independent.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/573/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/effectivenessmonitor/` | New files: `failed_phase_test.go`, `config_573_test.go` + updates to `phase_test.go` |
| Integration test suite | `test/integration/effectivenessmonitor/` | New files: `scheduled_event_573_test.go`, `assessment_paths_573_test.go`, `failed_phase_integration_test.go` |

---

## 13. Execution

```bash
# Unit tests (all EM)
go test ./test/unit/effectivenessmonitor/... -ginkgo.v

# Integration tests (all EM)
go test ./test/integration/effectivenessmonitor/... -ginkgo.v

# Specific gap tests by ID
go test ./test/unit/effectivenessmonitor/... -ginkgo.focus="UT-EM-573"
go test ./test/integration/effectivenessmonitor/... -ginkgo.focus="IT-EM-573"

# Coverage
go test ./test/unit/effectivenessmonitor/... -coverprofile=coverage-ut.out
go test ./test/integration/effectivenessmonitor/... -coverprofile=coverage-it.out
go tool cover -func=coverage-ut.out
go tool cover -func=coverage-it.out
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/effectivenessmonitor/phase_test.go` | Tests `ValidTransitions` map; `Failed` may not have explicit test cases | Add assertions for `CanTransition(*, Failed)` for all source phases | G1: Failed phase now reachable from non-terminal phases |
| `test/unit/effectivenessmonitor/config_test.go` | Tests `DefaultConfig()` and `Validate()` for existing fields | May need update if `DefaultConfig()` returns new default values (PrometheusLookback 30m) | G3: New config fields and changed defaults |
| Any test using mock `DataStorageQuerier` | Mock implements `QueryPreRemediationHash` and `HasWorkflowStarted` | Must add `HasWorkflowCompleted` method to compile | G4: Interface expansion |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan: 9 unit tests, 6 integration tests across 4 gaps |
