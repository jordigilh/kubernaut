# Test Plan: Inconclusive Backoff and Chain-Counting (#1091)

**Version**: 1.0
**Created**: 2026-05-11
**Status**: Active
**Service**: Remediation Orchestrator (RO) / Gateway (GW)
**Service Type**: [x] CRD Controller
**Issues**: #1091
**Business Requirements**: BR-ORCH-042.6
**Compliance**: NIST SC-5 (DoS protection), NIST AU-2/AU-3 (audit trail preserved)

---

## 1. Scope

This test plan covers the Inconclusive backoff fix — preventing RR flood for persistent alerts by treating `Inconclusive` outcomes as functional failures for backoff and chain-counting purposes.

| Change | File | Description | Risk |
|--------|------|-------------|------|
| Backoff on Inconclusive completion | `effectiveness_tracking.go` | Increment `ConsecutiveFailureCount`, set `NextAllowedExecution` | Medium |
| Chain-counting for Inconclusive | `blocking.go` | `CheckConsecutiveFailures` counts `Completed+Inconclusive` as failure | Medium |

---

## 2. Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- `UT-RO-1091-*` -- RO backoff and chain-counting tests
- `UT-GW-1091-*` -- GW deduplication for Inconclusive+NextAllowedExecution

---

## 3. Features Not to be Tested

- **E2E tests**: Deferred. Requires controlled alert persistence after remediation, EA scoring `alertScore=0`, and second signal arriving during backoff window. Unit tests provide sufficient behavioral assurance.
- **Prometheus alert rules**: Deferred to post-V1.0 (BR-ORCH-044).

---

## 4. Test Scenarios

### 4.1 RO Backoff on Inconclusive Completion

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-RO-1091-001 | Inconclusive EA outcome sets `ConsecutiveFailureCount=1` and `NextAllowedExecution` in future | Unit | RR status has count=1 and NextAllowedExecution > now |
| UT-RO-1091-002 | Inconclusive with pre-existing `ConsecutiveFailureCount=1` increments to 2, backoff is longer | Unit | count=2, NextAllowedExecution = now + 2*time.Minute (mock) |
| UT-RO-1091-003 | Inconclusive when EA phase is `Failed` (not `Completed`) still transitions correctly | Unit | RR transitions to Completed with Inconclusive outcome and backoff |
| UT-RO-1091-004 | Inconclusive at threshold (`ConsecutiveFailureCount >= 3`): count still increments, no `NextAllowedExecution` set | Unit | count=4, NextAllowedExecution remains nil |
| UT-RO-1091-005 | Remediated outcome does NOT set backoff fields (negative/regression test) | Unit | count=0, NextAllowedExecution=nil |
| UT-RO-1091-006 | Idempotency: second reconcile after Inconclusive completion does not re-apply backoff | Unit | count unchanged from first application |

### 4.2 Routing Engine Chain-Counting for Inconclusive

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-RO-1091-007 | `CheckConsecutiveFailures` counts `Completed+Inconclusive` RR as failure in chain | Unit | consecutiveFailures includes Inconclusive RR |
| UT-RO-1091-008 | `CheckConsecutiveFailures` still breaks chain on `Completed+Remediated` (regression) | Unit | Chain broken at Remediated RR |
| UT-RO-1091-009 | 3 consecutive `Completed+Inconclusive` RRs trigger blocking | Unit | Returns non-nil `BlockingCondition` with `BlockReasonConsecutiveFailures` |

### 4.3 GW Deduplication for Inconclusive with NextAllowedExecution

| ID | Description | Type | Expected Result |
|----|-------------|------|-----------------|
| UT-GW-1091-001 | `ShouldDeduplicate` returns true for `Completed+Inconclusive` RR with future `NextAllowedExecution` | Unit | shouldDedup=true, returns the RR with active backoff |

---

## 5. Test Data Requirements

### EA Fixtures
- EA with `alertScore=0.0` (Inconclusive outcome via `DeriveOutcomeFromEA`)
- EA with `alertScore=1.0` (Remediated outcome)

### RR Fixtures
- RR in `Verifying` phase with EA ref
- RR in `Completed` phase with `Outcome=Inconclusive` and pre-set `ConsecutiveFailureCount`
- Multiple RRs with same `SignalFingerprint` for chain-counting

### Mock Dependencies
- `MockRoutingEngine` (existing in `test_helpers.go`): `Config()` returns threshold=3, `CalculateExponentialBackoff()` returns `consecutiveFailures * time.Minute`

---

## 6. Pass/Fail Criteria

- All 10 tests must pass
- No regression in existing `effectiveness_tracking_test.go` tests
- No regression in existing `consecutive_failure_test.go` tests
- No regression in existing GW `phase_checker_business_test.go` tests
- `go build ./...` succeeds
- `golangci-lint run` introduces no new findings

---

## 7. Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| `Outcome` field empty on legacy Completed RRs | `CheckConsecutiveFailures` only counts `Outcome=="Inconclusive"` explicitly; empty/other outcomes fall through to chain-break (safe) |
| Backoff duplication with `transitionToFailed` | DRY assessment in TDD Refactor phase; extract helper if identical |
| No E2E coverage | Deferred; unit tests cover all business logic paths |

---

## 8. Traceability Matrix

| Business Requirement | Test IDs |
|---------------------|----------|
| BR-ORCH-042.6 AC-042-6-1 | UT-RO-1091-001, UT-RO-1091-002 |
| BR-ORCH-042.6 AC-042-6-2 | UT-RO-1091-005 |
| BR-ORCH-042.6 AC-042-6-3 | UT-RO-1091-007 |
| BR-ORCH-042.6 AC-042-6-4 | UT-RO-1091-008 |
| BR-ORCH-042.6 AC-042-6-5 | UT-RO-1091-009 |

---

## 9. Validation Against 100 Go Mistakes

Performed during TDD Refactor phase. Checklist reference: https://github.com/teivah/100-go-mistakes
