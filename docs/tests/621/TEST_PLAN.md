# Test Plan: Timeout Notifications Missing Cluster ID

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-621-v1.0
**Feature**: Inject cluster identification into timeout notification bodies
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.2.0-rc4`

---

## 1. Introduction

### 1.1 Purpose

Validates that timeout notifications (global and per-phase) include the cluster name and UUID in their body text, matching the pattern established in #615 for other notification types via `FormatClusterLine`.

### 1.2 Objectives

1. **Cluster line presence**: Both global and phase timeout notification bodies prepend the cluster identification line when `SetClusterIdentity` has been called.
2. **Graceful degradation**: When cluster identity is empty, no "Cluster:" prefix appears in timeout bodies.
3. **No regression**: Existing timeout behavior (phase transition, metrics, audit) is unaffected.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/...` |
| Backward compatibility | 0 regressions | Existing timeout tests pass without modification |

---

## 2. References

### 2.1 Authority

- BR-ORCH-027: Global Timeout Management
- BR-ORCH-028: Per-phase timeout escalation
- Issue #615: Cluster ID in notifications (established pattern)
- Issue #621: Timeout notifications missing cluster ID

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Timeout notification tests assert exact body text | Test breakage | Low | UT-RO-621-* | Existing timeout tests only check phase transitions, not body text |
| R2 | Reconciler struct change breaks compilation | Build failure | Low | All | Minimal change: two string fields + setter update |

---

## 4. Scope

### 4.1 Features to be Tested

- **Reconciler timeout bodies** (`internal/controller/remediationorchestrator/reconciler.go`): `handleGlobalTimeout` and `createPhaseTimeoutNotification` notification `Body` field includes cluster line
- **SetClusterIdentity propagation** (`reconciler.go`): Stores identity on both Reconciler and NotificationCreator

### 4.2 Features Not to be Tested

- **NotificationCreator body builders**: Already covered by #615 tests in `notification_cluster_test.go`
- **Timeout phase transitions**: Covered by existing `reconcile_phases_test.go`

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (Reconciler timeout body construction)
- **Integration**: Skipped (see Tier Skip Rationale)

### 5.2 Pass/Fail Criteria

**PASS**: All P0 tests pass, no regressions in existing timeout tests.
**FAIL**: Any P0 test fails or existing timeout tests regress.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `handleGlobalTimeout`, `createPhaseTimeoutNotification`, `SetClusterIdentity` | ~120 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-027 | Global timeout notification includes cluster ID | P0 | Unit | UT-RO-621-001 | Pending |
| BR-ORCH-028 | Phase timeout notification includes cluster ID | P0 | Unit | UT-RO-621-002 | Pending |
| Issue #615 | Graceful degradation when identity is empty | P0 | Unit | UT-RO-621-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-RO-621-NNN`

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-621-001` | Global timeout notification body includes cluster line when identity is set | Pending |
| `UT-RO-621-002` | Phase timeout notification body includes cluster line when identity is set | Pending |
| `UT-RO-621-003` | Timeout notification body omits cluster line when identity is empty | Pending |

### Tier Skip Rationale

- **Integration**: Timeout reconciler behavior is unit-tested via table-driven phase tests. The existing `timeout_integration_test.go` documents that full timeout integration tests are not practical in envtest.
- **E2E**: Not applicable for notification body content validation.

---

## 9. Test Cases

### UT-RO-621-001: Global timeout body includes cluster line

**BR**: BR-ORCH-027
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_cluster_test.go`

**Test Steps**:
1. **Given**: A Reconciler with cluster identity set to ("ocp-prod", "uuid-123")
2. **When**: `handleGlobalTimeout` creates a timeout NotificationRequest
3. **Then**: The notification `Spec.Body` starts with `**Cluster**: ocp-prod (uuid-123)`

### UT-RO-621-002: Phase timeout body includes cluster line

**BR**: BR-ORCH-028
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_cluster_test.go`

**Test Steps**:
1. **Given**: A Reconciler with cluster identity set to ("ocp-prod", "uuid-123")
2. **When**: `createPhaseTimeoutNotification` creates a phase timeout NotificationRequest
3. **Then**: The notification `Spec.Body` starts with `**Cluster**: ocp-prod (uuid-123)`

### UT-RO-621-003: Timeout body omits cluster line when empty

**BR**: Issue #615 (graceful degradation)
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/notification_cluster_test.go`

**Test Steps**:
1. **Given**: A Reconciler with no cluster identity set
2. **When**: `handleGlobalTimeout` creates a timeout NotificationRequest
3. **Then**: The notification `Spec.Body` does NOT start with `**Cluster**:`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: fake K8s client (`fake.NewClientBuilder()`)
- **Location**: `test/unit/remediationorchestrator/`

---

## 11. Execution

```bash
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-621" -ginkgo.v
```

---

## 12. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | N/A | N/A | Existing timeout tests don't assert on body text |

---

## 13. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
