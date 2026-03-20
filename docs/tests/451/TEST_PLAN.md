# Test Plan: Gateway Resilient Batch Alert Processing

**Feature**: Skip stale alerts in grouped AlertManager webhooks instead of dropping the entire batch
**Version**: 1.0
**Created**: 2026-03-20
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `development/v1.2`

**Authority**:
- BR-GATEWAY-004: Cross-adapter deduplication
- Issue #451: Gateway drops entire webhook payload when one alert references a deleted pod

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `pkg/gateway/adapters/prometheus_adapter.go`: `Parse()` method — iterate alerts, skip stale ones, return first valid signal
- `pkg/gateway/adapters/prometheus_adapter.go`: helper JSON builder for multi-alert webhooks in tests

### Out of Scope

- `pkg/gateway/types/fingerprint.go`: `ResolveFingerprint()` — intentionally returns error for stale pods (correct dedup-safety behavior, not modified)
- `pkg/gateway/server.go`: `readParseValidateSignal()` — caller is not modified, still receives a single `*NormalizedSignal`
- Other adapters (K8sEventAdapter) — only PrometheusAdapter receives batched alerts from AlertManager

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Iterate alerts, return first valid signal | Preserves existing single-signal-per-webhook contract with `server.go`; minimal change surface |
| Log warning for each skipped stale alert | Operators need visibility into skipped alerts for debugging (not silent) |
| Return error only when ALL alerts fail resolution | Batch is only dropped when no valid alert exists; partial success is preferred |
| Do not change `ResolveFingerprint` behavior | Stale pod detection is correct dedup safety — the fix is in how the adapter handles the error |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of `PrometheusAdapter.Parse()` code paths (pure logic: iteration, skip, return)
- **Integration**: Tier skip — see rationale below

### 2-Tier Minimum

This fix is entirely within pure parsing logic (no I/O, no K8s, no HTTP handlers). The `PrometheusAdapter.Parse()` method is unit-testable code that takes `[]byte` and returns `*NormalizedSignal`. The caller (`readParseValidateSignal`) is already covered by existing integration tests. A single unit-test tier with comprehensive scenario coverage satisfies the business outcome validation.

### Business Outcome Quality Bar

Tests validate **business outcomes**: "when AlertManager sends a batch with stale + valid alerts, the valid alert is processed" — not "the loop iterates correctly."

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/adapters/prometheus_adapter.go` | `Parse()` (lines 149-215) | ~65 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| N/A — `Parse()` is pure logic | — | — |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #451 | Valid alerts processed when batch contains stale alert first | P0 | Unit | UT-GW-451-001 | Pending |
| #451 | Valid alerts processed when stale alert is in the middle | P0 | Unit | UT-GW-451-002 | Pending |
| #451 | All alerts stale → error returned (entire batch dropped) | P0 | Unit | UT-GW-451-003 | Pending |
| #451 | Single valid alert (no stale) → no behavioral regression | P1 | Unit | UT-GW-451-004 | Pending |
| #451 | Single stale alert → error returned (existing behavior preserved) | P1 | Unit | UT-GW-451-005 | Pending |
| #451 | Stale alert skipped with warning log | P1 | Unit | UT-GW-451-006 | Pending |
| #451 | First valid alert's labels/annotations/severity used in signal | P1 | Unit | UT-GW-451-007 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: `GW` (Gateway)
- **BR_NUMBER**: `451`
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `PrometheusAdapter.Parse()` — target >=80% code path coverage (all branches: stale-first, stale-middle, all-stale, single-valid, single-stale, field correctness)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-451-001` | Valid alert is processed when stale alert appears first in batch | Pending |
| `UT-GW-451-002` | Valid alert is processed when stale alert appears in the middle of batch | Pending |
| `UT-GW-451-003` | Batch is dropped with error when ALL alerts reference deleted pods | Pending |
| `UT-GW-451-004` | Single valid alert (no stale) works identically to pre-fix behavior | Pending |
| `UT-GW-451-005` | Single stale alert returns error (preserves existing drop behavior) | Pending |
| `UT-GW-451-006` | Each skipped stale alert produces a warning-level log entry | Pending |
| `UT-GW-451-007` | Returned signal uses labels, annotations, severity, and fingerprint from the first valid alert | Pending |

### Tier Skip Rationale

- **Integration**: `Parse()` is pure logic (takes `[]byte`, returns struct). No I/O, no K8s API, no HTTP. The caller (`readParseValidateSignal` in `server.go`) is not modified and is covered by existing gateway integration tests. Adding an integration tier would test the same code path with no additional wiring coverage.
- **E2E**: Fix is internal parsing logic. The E2E gateway tests exercise the full webhook→signal→RR pipeline; re-running them after this change validates end-to-end behavior without new E2E scenarios.

---

## 6. Test Cases (Detail)

### UT-GW-451-001: Stale alert first, valid alert second — valid alert processed

**BR**: Issue #451
**Type**: Unit
**File**: `test/unit/gateway/prometheus_batch_alert_test.go`

**Given**: AlertManager webhook with 2 alerts: alert[0] references a deleted pod (owner resolution fails), alert[1] references an existing pod (owner resolution succeeds)
**When**: `PrometheusAdapter.Parse()` is called
**Then**: Returns a valid `NormalizedSignal` with fingerprint from alert[1]'s resolved owner

**Acceptance Criteria**:
- Error is nil
- Signal is non-nil
- Signal.Fingerprint matches the owner-resolved fingerprint of alert[1]'s resource
- Signal.SignalName matches alert[1]'s alertname

---

### UT-GW-451-002: Stale alert in middle, valid alert after — valid alert processed

**BR**: Issue #451
**Type**: Unit
**File**: `test/unit/gateway/prometheus_batch_alert_test.go`

**Given**: AlertManager webhook with 3 alerts: alert[0] valid, alert[1] stale, alert[2] valid
**When**: `PrometheusAdapter.Parse()` is called
**Then**: Returns signal from alert[0] (first valid alert wins)

**Acceptance Criteria**:
- Signal.SignalName matches alert[0]'s alertname
- Signal.Fingerprint matches alert[0]'s resolved owner

---

### UT-GW-451-003: All alerts stale — error returned

**BR**: Issue #451
**Type**: Unit
**File**: `test/unit/gateway/prometheus_batch_alert_test.go`

**Given**: AlertManager webhook with 2 alerts, both referencing deleted pods
**When**: `PrometheusAdapter.Parse()` is called
**Then**: Returns nil signal and error indicating all alerts failed resolution

**Acceptance Criteria**:
- Error is non-nil
- Error message indicates all alerts failed
- Signal is nil

---

### UT-GW-451-004: Single valid alert — no regression

**BR**: Issue #451
**Type**: Unit
**File**: `test/unit/gateway/prometheus_batch_alert_test.go`

**Given**: AlertManager webhook with 1 alert referencing an existing pod
**When**: `PrometheusAdapter.Parse()` is called
**Then**: Returns signal identical to pre-fix behavior

**Acceptance Criteria**:
- Error is nil
- Signal.Fingerprint matches owner-resolved fingerprint
- Signal.SignalName, Severity, Namespace all match the alert's labels

---

### UT-GW-451-005: Single stale alert — error preserved

**BR**: Issue #451
**Type**: Unit
**File**: `test/unit/gateway/prometheus_batch_alert_test.go`

**Given**: AlertManager webhook with 1 alert referencing a deleted pod
**When**: `PrometheusAdapter.Parse()` is called
**Then**: Returns error (same as pre-fix behavior for single stale alerts)

**Acceptance Criteria**:
- Error is non-nil
- Signal is nil

---

### UT-GW-451-006: Skipped stale alert produces warning log

**BR**: Issue #451
**Type**: Unit
**File**: `test/unit/gateway/prometheus_batch_alert_test.go`

**Given**: AlertManager webhook with stale alert[0] and valid alert[1], using a log observer
**When**: `PrometheusAdapter.Parse()` is called
**Then**: A warning-level log entry is emitted for the skipped stale alert

**Acceptance Criteria**:
- Log contains the stale pod's resource identifier
- Log indicates the alert was skipped (not silently dropped)

---

### UT-GW-451-007: Signal fields from first valid alert

**BR**: Issue #451
**Type**: Unit
**File**: `test/unit/gateway/prometheus_batch_alert_test.go`

**Given**: Webhook with stale alert[0] (severity=warning, alertname=StaleAlert) and valid alert[1] (severity=critical, alertname=KubePodCrashLooping)
**When**: `PrometheusAdapter.Parse()` is called
**Then**: Returned signal has fields from alert[1], not alert[0]

**Acceptance Criteria**:
- Signal.SignalName == "KubePodCrashLooping"
- Signal.Severity == "critical"
- Signal.Namespace matches alert[1]'s namespace
- Signal.Labels include alert[1]'s labels merged with commonLabels

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `mockOwnerResolver` (existing in `test/unit/gateway/kubernetes_event_dedup_test.go`) — configurable per-test resolve behavior
- **Log Observer**: `logr/testr` or custom sink to capture warning logs (UT-GW-451-006)
- **Location**: `test/unit/gateway/prometheus_batch_alert_test.go`

---

## 8. Execution

```bash
# All gateway unit tests
make test

# Specific #451 tests
go test ./test/unit/gateway/... -ginkgo.focus="Issue #451"

# Full gateway unit suite
go test ./test/unit/gateway/... -v
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-20 | Initial test plan — 7 unit test scenarios for batch alert resilience |
