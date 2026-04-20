# Test Plan: EM AssessmentReason PascalCase + Audit Store Fixes

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-749-v1
**Feature**: Rename EM AssessmentReason constants to PascalCase and fix audit store TIMER BUG
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/749-em-reason-pascalcase-and-audit-flush`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates two related changes shipping on a single branch:

**Change A (Issue #749)**: Rename all `AssessmentReason` constants in the
EffectivenessAssessment CRD from snake_case to PascalCase to comply with
Kubernetes API conventions (matching the existing `Phase` constants and
`pkg/shared/events/reasons.go` pattern). This is a big-bang rename across
the CRD types, CRD schema, OpenAPI spec, ogen-generated client, DS handler
logic, KA prompt history, Helm charts, and all downstream string literals.

**Change B**: Fix the audit store "TIMER BUG" false alarm by raising the drift
detection threshold from 2x to 5x, demoting debug-level `Info` logs in
`StoreAudit` to `V(1)`, and correcting the Helm ConfigMap
`flush_interval_seconds` from `0.1` to `1.0`.

### 1.2 Objectives

1. **PascalCase enforcement**: All 8 `AssessmentReason` constants (including new `Unrecoverable`) use PascalCase values matching Kubernetes conventions
2. **Zero orphaned snake_case**: No production or test code references the old snake_case reason values
3. **Audit store noise reduction**: TIMER BUG Error log only fires at genuine 5x drift; StoreAudit Info logs suppressed at default verbosity
4. **Helm config correctness**: `flush_interval_seconds` defaults to 1.0 in the chart
5. **No regressions**: All existing tests pass after string literal updates
6. **OpenAPI/ogen stability**: Regenerated ogen Go constant names are identical to pre-change names

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/...` |
| Integration test pass rate | 100% | `go test ./test/integration/...` |
| Build success | 0 errors | `go build ./...` |
| Lint compliance | 0 new errors | `golangci-lint run` |
| Orphaned snake_case | 0 hits | `grep -r '"spec_drift"\|"no_execution"\|"metrics_timed_out"\|"alert_decay_timeout"' --include='*.go'` |
| Backward compatibility | N/A | v1alpha1 pre-GA, no backward compat required |

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-EM-749**: EffectivenessAssessment Reason field must use PascalCase per Kubernetes API conventions
- **Issue #749**: EM Reason field PascalCase enforcement
- **DD-EM-002 v1.1**: Spec drift assessment design (governs `SpecDrift` reason semantics)
- **DD-AUDIT-002**: Audit shared library design (governs flush interval and logging)
- **ADR-EM-001**: Effectiveness Monitor service integration

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- [Kubernetes Event Reasons](../../pkg/shared/events/reasons.go) — PascalCase convention reference

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Ogen regeneration changes Go constant names | Build failure across DS client consumers | Low | All DS tests | Preflight verified: ogen normalizes both `spec_drift` and `SpecDrift` to same Go suffix `SpecDrift`. Post-regen `go build` gate. |
| R2 | Orphaned snake_case string literal in test or production code | Silent test pass with wrong assertion value | Medium | UT-EM-749-001 | Post-GREEN grep sweep + adversarial audit |
| R3 | DS effectiveness handler uses raw strings instead of constants | Breaks when enum value changes | Medium | UT-DS-749-001 | Replace literals with ogen-generated constants in REFACTOR |
| R4 | CRD schema enum not updated (CEL validation rejects PascalCase) | EA creation fails at API server | High | Integration tests | `make generate` regenerates from kubebuilder annotations; verify in Helm CRD copy |
| R5 | Audit store threshold change masks genuine timer bugs | Delayed detection of real Go scheduler issues | Low | UT-AUDIT-749-001/002 | 5x threshold still catches significant drift (>5s for 1s interval) |

### 3.1 Risk-to-Test Traceability

- **R1**: Mitigated by Phase 2 step 7 (`go build ./...` after ogen regen)
- **R2**: Mitigated by UT-EM-749-001 (regex validation) + post-REFACTOR grep sweep
- **R3**: Mitigated by UT-DS-749-001 + REFACTOR phase constant replacement
- **R4**: Mitigated by `make generate` + Helm CRD copy in Phase 2 step 13
- **R5**: Mitigated by UT-AUDIT-749-001 and UT-AUDIT-749-002

---

## 4. Scope

### 4.1 Features to be Tested

- **EA CRD Types** (`api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go`): PascalCase constant values and kubebuilder enum annotation
- **EM Completion** (`internal/controller/effectivenessmonitor/completion.go`): `AssessmentReasonUnrecoverable` constant usage
- **DS Effectiveness Handler** (`pkg/datastorage/server/effectiveness_handler.go`): PascalCase `SpecDrift` short-circuit logic
- **KA Prompt History** (`internal/kubernautagent/prompt/history.go`): All spec_drift detection functions with PascalCase values
- **Audit Store** (`pkg/audit/store.go`): Timer drift threshold (5x) and log verbosity demotion
- **Helm Chart** (`charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`): `flush_interval_seconds` value
- **OpenAPI Spec** (`api/openapi/data-storage-v1.yaml`): PascalCase enum values in 3 locations
- **Ogen Client** (`pkg/datastorage/ogen-client/`): Regenerated with stable Go constant names

### 4.2 Features Not to be Tested

- **CRD version migration**: v1alpha1 is pre-GA; no backward compatibility layer
- **DataStorage DB schema**: Stores strings; no enum constraint at DB level
- **External dashboard consumers**: Breaking API change is accepted for pre-GA
- **E2E cluster deployment**: Deferred to post-merge validation

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Big-bang rename (no dual-value support) | Pre-GA v1alpha1 CRD; no production consumers depend on old values |
| Add `AssessmentReasonUnrecoverable` constant | `"unrecoverable"` was the only reason using a raw string literal in `completion.go` |
| 5x timer drift threshold | 2x was too sensitive for Go scheduler jitter on resource-constrained nodes; 5x catches genuine issues while tolerating normal variance |
| Demote StoreAudit Info logs to V(1) | Debug tracing pollutes production logs; V(1) preserves diagnostic capability when needed |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (constant definitions, history detection functions, audit store timer logic)
- **Integration**: Existing integration tests updated with PascalCase values; no new integration tests needed (same behavior, different string values)
- **E2E**: Deferred — requires Kind cluster with updated CRD; validated post-merge

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests**: Validate constant values, detection logic, threshold behavior
- **Integration tests**: Validate end-to-end EM lifecycle with PascalCase reasons (existing tests updated)

### 5.3 Business Outcome Quality Bar

Tests validate that:
- Operators see PascalCase values in `kubectl get ea` output (Reason column)
- DS API returns PascalCase assessment reasons in remediation history
- Audit store does not emit false-alarm Error logs under normal scheduling jitter
- KA prompt history correctly identifies spec drift patterns with PascalCase values

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:

1. All P0 tests pass (0 failures)
2. All P1 tests pass or have documented exceptions
3. `go build ./...` succeeds after ogen regeneration
4. Zero orphaned snake_case reason values in `*.go` files
5. No regressions in existing test suites

**FAIL** — any of the following:

1. Any P0 test fails
2. Ogen regeneration changes Go constant names
3. Existing tests that were passing before the change now fail (regression)

### 5.5 Suspension & Resumption Criteria

**Suspend testing when**:
- Ogen regeneration produces different Go constant names (R1 materialized)
- `make generate` fails (controller-gen issue)
- Build broken after constant rename (cascade failure)

**Resume testing when**:
- Root cause identified and alternative approach approved
- Build restored to green

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go` | Constants block | ~25 |
| `internal/kubernautagent/prompt/history.go` | `FormatTier1Entry`, `FormatTier2Summary`, `DetectDecliningEffectiveness`, `DetectCompletedButRecurring`, `AllZeroEffectiveness`, `DetectSpecDriftCausalChains`, `BuildRemediationHistorySection` | ~300 |
| `pkg/audit/store.go` | `backgroundWriter` (timer tick case), `StoreAudit` | ~80 |
| `pkg/datastorage/server/effectiveness_handler.go` | `BuildEffectivenessResponse` | ~30 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/effectivenessmonitor/completion.go` | `completeAssessment`, `failAssessment`, `determineAssessmentReason` | ~100 |
| `internal/controller/remediationorchestrator/effectiveness_tracking.go` | `trackEffectivenessStatus` | ~80 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/749-em-reason-pascalcase-and-audit-flush` HEAD | Branch from main |
| ogen | v1.18.0 | Pinned in `gen.go` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-EM-749 | AssessmentReason constants use PascalCase | P0 | Unit | UT-EM-749-001 | Pending |
| BR-EM-749 | Unrecoverable reason uses constant | P0 | Unit | UT-EM-749-002 | Pending |
| BR-EM-749 | KA history: FormatTier1Entry recognizes PascalCase SpecDrift | P0 | Unit | UT-KA-749-001 | Pending |
| BR-EM-749 | KA history: DetectDecliningEffectiveness skips SpecDrift | P1 | Unit | UT-KA-749-002 | Pending |
| BR-EM-749 | KA history: DetectCompletedButRecurring skips SpecDrift | P1 | Unit | UT-KA-749-003 | Pending |
| BR-EM-749 | KA history: AllZeroEffectiveness skips SpecDrift | P1 | Unit | UT-KA-749-004 | Pending |
| BR-EM-749 | KA history: DetectSpecDriftCausalChains matches SpecDrift | P1 | Unit | UT-KA-749-005 | Pending |
| BR-EM-749 | KA history: BuildRemediationHistorySection HasSpecDrift | P1 | Unit | UT-KA-749-006 | Pending |
| BR-EM-749 | DS: BuildEffectivenessResponse short-circuits SpecDrift to 0.0 | P0 | Unit | UT-DS-749-001 | Pending |
| BR-AUDIT-749 | Timer drift at 3x does NOT emit Error log | P0 | Unit | UT-AUDIT-749-001 | Pending |
| BR-AUDIT-749 | Timer drift at 6x DOES emit Error log | P0 | Unit | UT-AUDIT-749-002 | Pending |
| BR-AUDIT-749 | StoreAudit suppresses Info logs at V(0) | P1 | Unit | UT-AUDIT-749-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: EM (EffectivenessMonitor), KA (KubernautAgent), DS (DataStorage), AUDIT (Audit)
- **BR_NUMBER**: 749
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: EA types constants, KA history functions, audit store timer logic, DS effectiveness handler

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-749-001` | All AssessmentReason constants are PascalCase (no underscores) | Pending |
| `UT-EM-749-002` | AssessmentReasonUnrecoverable constant exists and equals "Unrecoverable" | Pending |
| `UT-KA-749-001` | FormatTier1Entry renders "INCONCLUSIVE (spec drift)" for PascalCase SpecDrift | Pending |
| `UT-KA-749-002` | DetectDecliningEffectiveness skips SpecDrift entries (not counted in score trends) | Pending |
| `UT-KA-749-003` | DetectCompletedButRecurring skips SpecDrift entries (not counted as recurring) | Pending |
| `UT-KA-749-004` | AllZeroEffectiveness skips SpecDrift entries (not counted as zero-effectiveness) | Pending |
| `UT-KA-749-005` | DetectSpecDriftCausalChains matches SpecDrift assessment reason | Pending |
| `UT-KA-749-006` | BuildRemediationHistorySection sets HasSpecDrift=true for PascalCase SpecDrift | Pending |
| `UT-DS-749-001` | BuildEffectivenessResponse short-circuits to score 0.0 for PascalCase SpecDrift | Pending |
| `UT-AUDIT-749-001` | Timer drift at 3x expected interval does NOT emit Error log (threshold is 5x) | Pending |
| `UT-AUDIT-749-002` | Timer drift at 6x expected interval DOES emit Error log | Pending |
| `UT-AUDIT-749-003` | StoreAudit does not emit Info-level logs at default verbosity V(0) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Existing EM integration tests updated with PascalCase values

No new integration tests are needed. Existing tests that use `eav1.AssessmentReasonSpecDrift` constants will automatically pick up the new PascalCase values. Tests using string literals will be updated in the GREEN phase.

### Tier Skip Rationale

- **E2E**: Deferred to post-merge. Requires Kind cluster with updated CRD installed. The unit and integration tiers provide sufficient coverage for a string-value rename.

---

## 9. Test Cases

### UT-EM-749-001: All AssessmentReason constants are PascalCase

**BR**: BR-EM-749
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/ea_types_test.go`

**Preconditions**:
- `eav1` package importable

**Test Steps**:
1. **Given**: The set of all AssessmentReason constants (`Full`, `Partial`, `NoExecution`, `MetricsTimedOut`, `Expired`, `SpecDrift`, `AlertDecayTimeout`, `Unrecoverable`)
2. **When**: Each constant value is checked against the regex `^[A-Z][a-zA-Z]+$`
3. **Then**: All values match PascalCase pattern (no underscores, starts with uppercase)

**Expected Results**:
1. All 8 constants match PascalCase regex
2. No constant contains an underscore character

### UT-EM-749-002: AssessmentReasonUnrecoverable constant exists

**BR**: BR-EM-749
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/ea_types_test.go`

**Preconditions**:
- `eav1` package importable

**Test Steps**:
1. **Given**: The `eav1.AssessmentReasonUnrecoverable` constant
2. **When**: Its value is inspected
3. **Then**: It equals `"Unrecoverable"`

### UT-KA-749-001: FormatTier1Entry recognizes PascalCase SpecDrift

**BR**: BR-EM-749
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/prompt/history_test.go`

**Test Steps**:
1. **Given**: A Tier1Entry with `AssessmentReason: "SpecDrift"`
2. **When**: `FormatTier1Entry` is called
3. **Then**: Output contains "INCONCLUSIVE (spec drift)"

### UT-DS-749-001: BuildEffectivenessResponse short-circuits SpecDrift

**BR**: BR-EM-749
**Priority**: P0
**Type**: Unit
**File**: `test/unit/datastorage/effectiveness_score_test.go`

**Test Steps**:
1. **Given**: EM events with `reason: "SpecDrift"` in assessment.completed event data
2. **When**: `BuildEffectivenessResponse` is called
3. **Then**: `AssessmentStatus == "SpecDrift"` and `Score == 0.0`

### UT-AUDIT-749-001: Timer drift at 3x does NOT emit Error

**BR**: BR-AUDIT-749
**Priority**: P0
**Type**: Unit
**File**: `test/unit/audit/store_timer_test.go`

**Test Steps**:
1. **Given**: A BufferedAuditStore with FlushInterval=100ms
2. **When**: Timer tick fires with actual interval = 300ms (3x expected)
3. **Then**: No Error-level log is emitted (below 5x threshold)

### UT-AUDIT-749-002: Timer drift at 6x DOES emit Error

**BR**: BR-AUDIT-749
**Priority**: P0
**Type**: Unit
**File**: `test/unit/audit/store_timer_test.go`

**Test Steps**:
1. **Given**: A BufferedAuditStore with FlushInterval=100ms
2. **When**: Timer tick fires with actual interval = 600ms (6x expected)
3. **Then**: Error-level log containing "TIMER BUG" is emitted

### UT-AUDIT-749-003: StoreAudit suppresses Info at V(0)

**BR**: BR-AUDIT-749
**Priority**: P1
**Type**: Unit
**File**: `test/unit/audit/store_timer_test.go`

**Test Steps**:
1. **Given**: A BufferedAuditStore with a logger at verbosity V(0)
2. **When**: `StoreAudit` is called with a valid event
3. **Then**: No Info-level log messages are emitted (all demoted to V(1))

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None for EA types / KA history tests; mock DataStorageClient for audit store tests
- **Location**: `test/unit/effectivenessmonitor/`, `test/unit/kubernautagent/prompt/`, `test/unit/audit/`, `test/unit/datastorage/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (No-Mocks Policy)
- **Infrastructure**: envtest (API server + etcd) for EM lifecycle tests
- **Location**: `test/integration/effectivenessmonitor/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25.7 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-gen | v0.19.0 | CRD generation |
| ogen | v1.18.0 | OpenAPI client generation |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| None | — | — | — | — |

### 11.2 Execution Order

1. **Phase 1 (TDD RED)**: Write 12 failing unit tests
2. **Phase 2 (TDD GREEN)**: Rename constants, regenerate CRD/ogen, update all downstream, fix audit store
3. **Checkpoint 1**: Adversarial + security audit
4. **Phase 3 (TDD REFACTOR)**: Replace string literals with constant references, grep sweep
5. **Final Checkpoint**: Build, lint, full test pass, confidence assessment

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/749/TEST_PLAN.md` | Strategy and test design |
| EA types unit tests | `test/unit/effectivenessmonitor/ea_types_test.go` | PascalCase constant validation |
| KA history unit tests | `test/unit/kubernautagent/prompt/history_test.go` | SpecDrift detection with PascalCase |
| Audit store unit tests | `test/unit/audit/store_timer_test.go` | Timer threshold and log verbosity |
| DS effectiveness tests | `test/unit/datastorage/effectiveness_score_test.go` | SpecDrift short-circuit with PascalCase |

---

## 13. Execution

```bash
# Unit tests (all services)
go test ./test/unit/... -ginkgo.v

# Specific 749 tests
go test ./test/unit/effectivenessmonitor/... -ginkgo.focus="UT-EM-749"
go test ./test/unit/kubernautagent/prompt/... -ginkgo.focus="UT-KA-749"
go test ./test/unit/audit/... -ginkgo.focus="UT-AUDIT-749"
go test ./test/unit/datastorage/... -ginkgo.focus="UT-DS-749"

# Build verification
go build ./...

# Orphan check
grep -r '"spec_drift"\|"no_execution"\|"metrics_timed_out"\|"alert_decay_timeout"' --include='*.go' .
```

---

## 14. Existing Tests Requiring Updates (if applicable)

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/kubernautagent/prompt/history_test.go` (7 places) | `AssessmentReason: "spec_drift"` | Change to `"SpecDrift"` | Constant value renamed |
| `test/unit/datastorage/effectiveness_score_test.go` (9 places) | `"spec_drift"` in event data and assertions | Change to `"SpecDrift"` | Constant value renamed |
| `test/unit/datastorage/remediation_history_logic_test.go` (4 places) | `"spec_drift"` in event data and assertions | Change to `"SpecDrift"` | Constant value renamed |
| `test/unit/effectivenessmonitor/failed_phase_test.go` (2 places) | `"unrecoverable"` in assertions | Change to `"Unrecoverable"` | Constant value renamed |
| `test/integration/datastorage/remediation_history_integration_test.go` (4 places) | `"spec_drift"` in event data and assertions | Change to `"SpecDrift"` | Constant value renamed |
| `test/e2e/datastorage/25_remediation_history_api_test.go` (4 places) | `"spec_drift"` in event data and assertions | Change to `"SpecDrift"` | Constant value renamed |
| ~12 additional test files | Various snake_case reason values | Change to PascalCase equivalents | Constant values renamed |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
