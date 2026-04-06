# Test Plan: EM Metrics Score Correction + Notification Wording (#639)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-639-v1.0
**Feature**: Fix EM metrics scoring (CPU counter → rate query) and replace misleading "anomaly persists" notification with graduated scoring messages
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.2.0-rc7`

---

## 1. Introduction

> **IEEE 829 §3** — Purpose, objectives, and measurable success criteria for the test effort.

### 1.1 Purpose

The EffectivenessMonitor's metrics assessment uses a raw cumulative counter query for
CPU (`container_cpu_usage_seconds_total`) which always yields a score of 0.0 since
counters monotonically increase. The notification layer unconditionally reports
"anomaly persists" for any score below 1.0, contradicting the platform's own
"Remediated" outcome. This test plan validates that:
- The CPU query uses `rate()` to produce meaningful improvement scores
- The notification wording is graduated based on score value, removing the misleading
  "anomaly persists" message

### 1.2 Objectives

1. **CPU query correctness**: The EM queries CPU as `sum(rate(container_cpu_usage_seconds_total{...}[5m]))` instead of the raw counter, producing scores that reflect actual CPU usage changes
2. **Notification graduation**: `BuildComponentBullets` produces distinct messages for scores >= 0.5 ("partial improvement"), > 0.0 ("minimal improvement"), and == 0.0 ("no improvement detected"), with the score value included for operator transparency
3. **Backward compatibility**: All existing EM and RO tests pass without regressions

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/effectivenessmonitor/...` and `./test/unit/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `pkg/remediationorchestrator/creator/` |
| Backward compatibility | 0 regressions | Existing tests pass (updated assertions where wording changed) |

---

## 2. References

> **IEEE 829 §2** — Documents that govern or inform this test plan.

### 2.1 Authority (governing documents)

- BR-EM-003: Metrics comparison scoring for effectiveness assessment
- BR-ORCH-037: Remediation outcome determination
- Issue #639: EM metrics score consistently below 1.0 — Slack notification reports 'anomaly persists'

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- EM reconciler: `internal/controller/effectivenessmonitor/reconciler.go`
- Notification creator: `pkg/remediationorchestrator/creator/notification.go`
- Metrics scorer: `pkg/effectivenessmonitor/metrics/scorer.go`

---

## 3. Risks & Mitigations

> **IEEE 829 §5** — Software risk issues that drive test design.

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `rate()` returns NaN/empty when insufficient samples exist | CPU query yields no comparison data | Low | UT-EM-639-001 | Existing `executeMetricQuery` graceful degradation handles < 2 samples |
| R2 | Notification wording changes break existing test assertions | CI regression | High | UT-RO-639-004 through -007 | Explicitly update UT-RO-596 assertions in REFACTOR phase |
| R3 | `rate()` 5m window exceeds stabilization window on short tests | Score still 0 on very short-lived assessments | Medium | N/A (E2E) | 5m rate matches existing HTTP queries; stabilizationWindow is a deployment knob |

---

## 4. Scope

> **IEEE 829 §6/§7** — Features to be tested and features not to be tested.

### 4.1 Features to be Tested

- **CPU metric query** (`internal/controller/effectivenessmonitor/reconciler.go`): Verify the PromQL query string uses `rate()` wrapper
- **BuildComponentBullets** (`pkg/remediationorchestrator/creator/notification.go`): Verify graduated messages for metric scores at 0.0, 0.3, 0.5, 0.75, 1.0

### 4.2 Features Not to be Tested

- **Scorer logic** (`pkg/effectivenessmonitor/metrics/scorer.go`): No changes; existing UT-EM-MC-001 through -008 provide coverage
- **Stabilization window tuning**: Deployment-time configuration, not a code change
- **Platform-aware stabilization**: Deferred to v1.3

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Test CPU query via string assertion on PromQL | The query is a string constant; verifying the string is sufficient and avoids Prometheus dependency in unit tests |
| Graduated messages with score value | Provides operator transparency; more informative than binary pass/fail |

---

## 5. Approach

> **IEEE 829 §8/§9/§10** — Test strategy, pass/fail criteria, and suspension/resumption.

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code in `pkg/remediationorchestrator/creator/` (notification) and `internal/controller/effectivenessmonitor/` (reconciler query string)

### 5.2 Two-Tier Minimum

- **Unit tests**: Validate query string content and notification message formatting
- **Integration tier skip**: CPU query string is a constant; no I/O boundary to test. Notification formatting is pure logic.

### 5.3 Pass/Fail Criteria

**PASS** — all of the following must be true:
1. All P0 tests pass (0 failures)
2. Per-tier code coverage meets >=80% on `pkg/remediationorchestrator/creator/`
3. Existing UT-RO-596 tests updated and passing
4. `go build ./...` clean

**FAIL** — any of the following:
1. Any P0 test fails
2. Existing tests regress without documented update

### Tier Skip Rationale

- **Integration**: Both changes are pure-logic (query string constant, message formatting). No I/O boundaries crossed.
- **E2E**: Existing E2E effectivenessmonitor suite covers the full pipeline; query string change validated implicitly.

---

## 6. Test Items

> **IEEE 829 §4** — Software items to be tested.

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/effectivenessmonitor/reconciler.go` | `assessMetrics` (query definitions) | ~1100-1126 |
| `pkg/remediationorchestrator/creator/notification.go` | `BuildComponentBullets` | ~1115-1141 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-EM-003 | CPU metric query uses rate() for meaningful scores | P0 | Unit | UT-EM-639-001 | Pending |
| BR-EM-003 | Memory metric query unchanged (gauge, valid as-is) | P1 | Unit | UT-EM-639-002 | Pending |
| BR-ORCH-037 | Notification shows "no improvement" for score 0.0 | P0 | Unit | UT-RO-639-004 | Pending |
| BR-ORCH-037 | Notification shows "minimal improvement" for score 0.3 | P0 | Unit | UT-RO-639-005 | Pending |
| BR-ORCH-037 | Notification shows "partial improvement" for score 0.75 | P0 | Unit | UT-RO-639-006 | Pending |
| BR-ORCH-037 | Notification shows no bullet for perfect score 1.0 | P0 | Unit | UT-RO-639-007 | Pending |
| BR-ORCH-037 | Notification omits bullet when metrics not assessed | P1 | Unit | UT-RO-639-008 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Fix A — CPU Query (EM reconciler)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-639-001` | CPU query string contains `rate(` wrapper for meaningful improvement scoring | Pending |
| `UT-EM-639-002` | Memory query string uses raw `sum()` (gauge metric, no rate needed) | Pending |
| `UT-EM-639-003` | All 5 metric queries have consistent `LowerIsBetter` / `!LowerIsBetter` flags | Pending |

**Fix B — Notification Wording (RO notification creator)**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-639-004` | Score 0.0 → "no improvement detected" (not "anomaly persists") | Pending |
| `UT-RO-639-005` | Score 0.3 → "minimal improvement (score: 0.30)" | Pending |
| `UT-RO-639-006` | Score 0.75 → "partial improvement (score: 0.75)" | Pending |
| `UT-RO-639-007` | Score 1.0 → no metrics bullet emitted | Pending |
| `UT-RO-639-008` | MetricsAssessed=false → no metrics bullet emitted | Pending |

---

## 9. Test Cases

### UT-EM-639-001: CPU query uses rate() wrapper

**BR**: BR-EM-003
**Priority**: P0
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/metrics_query_test.go`

**Test Steps**:
1. **Given**: The `assessMetrics` function defines a CPU metric query spec
2. **When**: The query string for `container_cpu_usage_seconds_total` is examined
3. **Then**: The query contains `rate(container_cpu_usage_seconds_total` and `[5m])` wrapper

**Expected Results**:
1. CPU query uses `rate()` function, not raw counter sum
2. Rate window is 5m, matching HTTP metric queries

### UT-RO-639-004: Score 0.0 produces "no improvement detected"

**BR**: BR-ORCH-037
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Test Steps**:
1. **Given**: EA with `MetricsAssessed=true`, `MetricsScore=0.0`, `AssessmentReason=full`
2. **When**: `BuildComponentBullets(ea)` is called
3. **Then**: Result contains "Metrics: no improvement detected", NOT "anomaly persists"

### UT-RO-639-006: Score 0.75 produces "partial improvement (score: 0.75)"

**BR**: BR-ORCH-037
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/verification_summary_test.go`

**Test Steps**:
1. **Given**: EA with `MetricsAssessed=true`, `MetricsScore=0.75`, `AssessmentReason=full`
2. **When**: `BuildComponentBullets(ea)` is called
3. **Then**: Result contains "Metrics: partial improvement (score: 0.75)"

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None required (pure logic tests)
- **Location**: `test/unit/effectivenessmonitor/`, `test/unit/remediationorchestrator/`

### 10.2 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `UT-RO-596-001` (verification_summary_test.go:488) | `ContainSubstring("Metrics: anomaly persists")` | `ContainSubstring("Metrics: partial improvement")` | Wording changed from binary to graduated |
| `UT-RO-596-004` (verification_summary_test.go:546) | `ContainSubstring("Metrics: anomaly persists")` | `ContainSubstring("Metrics: no improvement detected")` | Score 0.0 now says "no improvement" |

---

## 12. Execution

```bash
# Unit tests (EM)
go test ./test/unit/effectivenessmonitor/... -ginkgo.v

# Unit tests (RO)
go test ./test/unit/remediationorchestrator/... -ginkgo.v

# Coverage on notification creator
go test ./test/unit/remediationorchestrator/... -coverpkg=github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
