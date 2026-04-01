# Test Plan: Context-Aware Verification Summary

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-596-v1
**Feature**: Context-aware verification summary — distinguish "all checks passed" from "all checks ran but some failed"
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.2.0-rc2`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for Issue #596: when `AssessmentReason=full` but individual component scores indicate failure (e.g., AlertScore=0.0, MetricsScore=0.0), the completion notification summary must use a qualified message ("all checks were performed, but some indicate the remediation was not fully effective") instead of the affirmative "Verification passed" message. The `VerificationContext.Outcome` must reflect this distinction ("completed" vs "passed") to enable accurate downstream routing.

### 1.2 Objectives

1. **Correct notification text**: When `reason=full` and any component score < 1.0, the notification body must NOT contain "Verification passed" and must instead contain a qualified completion message.
2. **Preserve affirmative path**: When `reason=full` and all component scores >= 1.0, the notification body must still contain "Verification passed" with `Outcome="passed"`.
3. **Routing correctness**: `VerificationContext.Outcome` must be `"completed"` (not `"passed"`) when components indicate failure, enabling routing rules to distinguish true passes from assessed-but-failed.
4. **Zero regressions**: All existing verification summary tests (UT-RO-318-*, UT-RO-546-*) must pass without modification.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/... -ginkgo.v -ginkgo.focus="596\|318\|546"` |
| Unit-testable code coverage | >=80% | `BuildVerificationSummary` + `BuildComponentBullets` fully covered |
| Backward compatibility | 0 regressions | All existing UT-RO-318-* and UT-RO-546-* pass unmodified |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-ORCH-035: Completion notifications must accurately represent verification results
- BR-EM-012: Assessment lifecycle and component scoring
- Issue #596: Completion notification 'Verification passed' contradicts component bullets when scores < 1.0

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Issue #318: Original verification summary implementation

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | New `Outcome="completed"` value breaks routing rules that match on `"passed"` | Notifications for assessed-but-failed cases stop matching "passed" rules | Low | UT-RO-596-001 | This is the intended behavior -- assessed-but-failed should NOT route as "passed". Document new value in DD. |
| R2 | `VerificationContext.Summary` field becomes stale if not updated alongside text | CRD stores old "passed" text while notification body shows qualified text | Medium | UT-RO-596-002 | Test explicitly asserts `ctx.Summary` matches the qualified text |
| R3 | Nil score (disabled component) incorrectly treated as failing | Notification says "not fully effective" when components were simply disabled | High | UT-RO-596-003 | Nil scores are excluded from the failure check -- only non-nil scores < 1.0 count as failing |

### 3.1 Risk-to-Test Traceability

- R1 mitigated by UT-RO-596-001 (verifies `Outcome="completed"` for failing case)
- R2 mitigated by UT-RO-596-002 (verifies `ctx.Summary` matches rendered text)
- R3 mitigated by UT-RO-596-003 (verifies disabled components don't trigger qualified message)

---

## 4. Scope

### 4.1 Features to be Tested

- **`BuildVerificationSummary`** (`pkg/remediationorchestrator/creator/notification.go`): Context-aware summary text selection when `reason=full`
- **`VerificationContext.Outcome`**: New `"completed"` value for assessed-but-failed scenarios
- **`VerificationContext.Summary`**: CRD field consistency with rendered notification text

### 4.2 Features Not to be Tested

- **EM `determineAssessmentReason`**: Confirmed correct; `full` means "all assessed", not "all passed"
- **`BuildComponentBullets`**: Already tested in UT-RO-318-009/010; no changes
- **Notification routing engine**: Consumes `Outcome` via `FlattenToMap`; no code change needed, routing rules are user-configured

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use "completed" (not "failed") as Outcome for assessed-but-failing | "failed" implies the assessment failed; "completed" correctly indicates assessment succeeded but remediation did not fully resolve the issue |
| Check bullet non-emptiness as proxy for failing scores | `BuildComponentBullets` already encodes the score < 1.0 logic; reusing it avoids duplicating the threshold check |
| Only non-nil scores count as failing | Nil score means the component was disabled (e.g., Prometheus off); absence is not evidence of failure |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `BuildVerificationSummary` (all code paths: nil EA, unknown reason, full+passing, full+failing, other reasons)
- **Integration**: N/A -- `BuildVerificationSummary` is pure logic with no I/O; covered at unit tier only
- **E2E**: N/A -- notification text is validated in existing E2E fullpipeline tests

### 5.2 Two-Tier Minimum

This fix is pure logic (no I/O, no K8s client calls, no DB). The function takes an EA struct and returns strings/structs. The unit tier provides complete coverage. Integration and E2E tiers are not applicable for this isolated logic function.

### Tier Skip Rationale

- **Integration**: `BuildVerificationSummary` is a pure function -- no database, no HTTP, no K8s API. There is no wiring to test.
- **E2E**: The E2E fullpipeline test already validates the notification body end-to-end. The specific text variation (full+failing vs full+passing) depends on the demo scenario's actual component scores, which are not controllable in E2E. Unit tests provide deterministic coverage.

### 5.3 Business Outcome Quality Bar

Each test validates what the **operator sees** in the Slack notification and what **routing rules receive** as programmatic context -- not which internal functions are called.

### 5.4 Pass/Fail Criteria

**PASS** -- all of the following must be true:

1. All 4 new P0 tests pass (UT-RO-596-001 through 004)
2. All existing UT-RO-318-* and UT-RO-546-* tests pass unmodified (0 regressions)
3. `go build ./...` and `go vet ./...` succeed
4. Pre-commit hooks pass

**FAIL** -- any of the following:

1. Any P0 test fails
2. Any existing verification summary test regresses
3. Build or vet fails

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/remediationorchestrator/creator/notification.go` | `BuildVerificationSummary` | ~45 (lines 948-993) |
| `pkg/remediationorchestrator/creator/notification.go` | `BuildComponentBullets` | ~25 (lines 1028-1054) |
| `pkg/remediationorchestrator/creator/notification.go` | `verificationMessages` map | ~12 (lines 935-946) |

### 6.2 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `fix/v1.2.0-rc2` HEAD | After Issue #595 commits |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-035 | Completion notification accurately reflects verification result | P0 | Unit | UT-RO-596-001 | Pending |
| BR-ORCH-035 | Affirmative "passed" preserved when all components pass | P0 | Unit | UT-RO-596-002 | Pending |
| BR-ORCH-035 | VerificationContext.Summary matches rendered text | P0 | Unit | UT-RO-596-002 | Pending |
| BR-EM-012 | Disabled components (nil score) don't trigger qualified message | P0 | Unit | UT-RO-596-003 | Pending |
| BR-ORCH-035 | Outcome="completed" enables routing distinction from "passed" | P0 | Unit | UT-RO-596-001 | Pending |
| BR-ORCH-035 | Full reproduction of the crashloop demo scenario | P0 | Unit | UT-RO-596-004 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-RO-596-{SEQUENCE}` (Unit, Remediation Orchestrator, Issue #596)

### Tier 1: Unit Tests

**Testable code scope**: `BuildVerificationSummary` in `pkg/remediationorchestrator/creator/notification.go` -- >=80% coverage of all code paths.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-596-001` | Operator sees qualified message and Outcome="completed" when reason=full but alerts firing and metrics anomaly persists | Pending |
| `UT-RO-596-002` | Operator sees affirmative "passed" message and Outcome="passed" when reason=full and all scores >= 1.0 (existing behavior preserved) | Pending |
| `UT-RO-596-003` | Disabled components (nil AlertScore, nil MetricsScore) with reason=full produce "passed" outcome, not "completed" | Pending |
| `UT-RO-596-004` | Exact crashloop demo reproduction: reason=full, HealthScore=1.0, AlertScore=0.0, MetricsScore=0.0 -- qualified message with both alert and metrics bullets | Pending |

---

## 9. Test Cases

### UT-RO-596-001: Full assessment with failing components produces qualified message

**BR**: BR-ORCH-035
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Preconditions**:
- EA with `AssessmentReason=full`, `HealthScore=1.0`, `AlertScore=0.0`, `MetricsScore=0.5`

**Test Steps**:
1. **Given**: EA with reason=full but AlertScore < 1.0 and MetricsScore < 1.0
2. **When**: `BuildVerificationSummary(ea, nil)` is called
3. **Then**: Summary contains "all checks were performed, but some indicate the remediation was not fully effective"
4. **Then**: Summary does NOT contain "Verification passed"
5. **Then**: `VerificationContext.Outcome` equals `"completed"` (not `"passed"`)
6. **Then**: `VerificationContext.Reason` equals `"full"`
7. **Then**: `VerificationContext.Summary` contains the qualified text (not the affirmative)

**Acceptance Criteria**:
- **Behavior**: Operator sees a non-contradictory notification
- **Correctness**: Outcome is "completed", not "passed"
- **Accuracy**: Summary text matches the qualified wording; reason is still "full"

---

### UT-RO-596-002: Full assessment with all passing scores preserves affirmative message

**BR**: BR-ORCH-035
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Preconditions**:
- EA with `AssessmentReason=full`, all scores = 1.0, hash match

**Test Steps**:
1. **Given**: EA with reason=full and all component scores >= 1.0
2. **When**: `BuildVerificationSummary(ea, nil)` is called
3. **Then**: Summary contains "Verification passed"
4. **Then**: `VerificationContext.Outcome` equals `"passed"`
5. **Then**: `VerificationContext.Summary` contains "Verification passed"

**Note**: This is a regression guard. The existing `UT-RO-318-001` already covers this exact case with all scores at 1.0. This test is therefore redundant with UT-RO-318-001 and should NOT be created as a separate test -- UT-RO-318-001 already provides this coverage.

---

### UT-RO-596-003: Disabled components (nil scores) with full assessment produce "passed"

**BR**: BR-EM-012
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Preconditions**:
- EA with `AssessmentReason=full`, `HealthScore=1.0`, `AlertAssessed=true` but `AlertScore=nil` (AM disabled), `MetricsAssessed=true` but `MetricsScore=nil` (Prom disabled), hash match

**Test Steps**:
1. **Given**: EA with reason=full, disabled alert+metrics (nil scores), passing health+hash
2. **When**: `BuildVerificationSummary(ea, nil)` is called
3. **Then**: Summary contains "Verification passed" (nil is not failure)
4. **Then**: `VerificationContext.Outcome` equals `"passed"`
5. **Then**: Bullets are empty (nil scores are not emitted as failure bullets)

**Acceptance Criteria**:
- **Behavior**: Disabled components don't produce false "not fully effective" messages
- **Correctness**: Nil score is treated as "not assessed for this purpose", not as failure
- **Accuracy**: Operator is not misled by a qualified message when nothing actually failed

---

### UT-RO-596-004: Exact crashloop demo scenario reproduction

**BR**: BR-ORCH-035
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Preconditions**:
- EA with `AssessmentReason=full`, `HealthScore=1.0`, `AlertScore=0.0`, `MetricsScore=0.0`, `HashComputed=true` with matching hashes

**Test Steps**:
1. **Given**: EA reproducing the crashloop demo: health recovered, alerts still firing, metrics anomaly persists
2. **When**: `BuildVerificationSummary(ea, nil)` is called
3. **Then**: Summary contains "all checks were performed, but some indicate the remediation was not fully effective"
4. **Then**: Summary contains "Related alerts: still firing"
5. **Then**: Summary contains "Metrics: anomaly persists"
6. **Then**: Summary does NOT contain "Verification passed"
7. **Then**: `VerificationContext.Outcome` equals `"completed"`

**Acceptance Criteria**:
- **Behavior**: The exact contradictory output from the crashloop demo is resolved
- **Correctness**: Both failing component bullets are present with the qualified summary
- **Accuracy**: Health (passing) does NOT appear in bullets; only failing components shown

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None required -- `BuildVerificationSummary` is a pure function taking EA/RR structs
- **Location**: `test/unit/remediationorchestrator/verification_summary_test.go`
- **Resources**: Minimal (in-memory only)

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.24+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None. All required code and test infrastructure already exists.

### 11.2 Execution Order

1. **Phase 1 (RED)**: Write UT-RO-596-001, UT-RO-596-003, UT-RO-596-004 as failing tests
2. **Phase 2 (GREEN)**: Minimal fix to `BuildVerificationSummary` -- add context-aware override when `reason=full && bullets != ""`
3. **Phase 3 (REFACTOR)**: Update `VerificationContext.Summary` and `Outcome` in the override path; build/vet/lint

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/596/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/remediationorchestrator/verification_summary_test.go` | 3 new Ginkgo BDD tests |

---

## 13. Execution

```bash
# Run all verification summary tests (new + existing)
go test ./test/unit/remediationorchestrator/... -ginkgo.v -ginkgo.focus="596|318|546"

# Run only new tests
go test ./test/unit/remediationorchestrator/... -ginkgo.v -ginkgo.focus="596"

# Full regression
go test ./test/unit/remediationorchestrator/... -ginkgo.v
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-RO-318-001 | `Outcome == "passed"` with all scores 1.0 | **None** -- still correct | All scores pass, so "passed" is the correct outcome |
| UT-RO-546-006 | `Outcome == "passed"` with empty components | **None** -- still correct | No bullets emitted for empty components, so "passed" is correct |
| notification_retry_test.go IT-RO-318-004 | `Outcome == "passed"` with all scores 1.0 | **None** -- still correct | Test uses all-passing scores |

No existing tests require modification.

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
