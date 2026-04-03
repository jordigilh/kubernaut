# Test Plan: RemediationRequest CRD 24h TTL Enforcement (#265)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-265-v1
**Feature**: Enforce 24h TTL on terminal RemediationRequest CRDs — set `retentionExpiryTime`, add cleanup logic, wire configurable retention period
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.4`

---

## 1. Introduction

### 1.1 Purpose

Validates that the RemediationOrchestrator controller correctly sets `retentionExpiryTime` on terminal RemediationRequests, deletes expired CRDs, and makes the retention period configurable. Also validates the F3 fix (CompletedAt on Failed/GlobalTimeout) and F7 consolidation (IsTerminalPhase removal).

### 1.2 Objectives

1. **RetentionExpiryTime set on terminal**: All 5 terminal phases (Completed, Failed, TimedOut, Cancelled, Skipped) trigger `retentionExpiryTime` assignment within one reconcile cycle.
2. **Cleanup enforcement**: Expired CRDs are deleted after audit trace emission.
3. **Configurable period**: Retention period is configurable via YAML config and Helm, default 24h.
4. **CompletedAt consistency**: `transitionToFailed` and `handleGlobalTimeout` now set `CompletedAt`.
5. **Code consolidation**: Local `IsTerminalPhase` removed; `phase.IsTerminal` used exclusively.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/remediationorchestrator/...` |
| Integration test pass rate | 100% | `go test ./test/integration/remediationorchestrator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on modified files |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority

- Issue #265: RemediationRequest CRD 24h TTL not enforced
- BR-ORCH-027: Global Timeout Management
- DD-AUDIT-003: Audit emission for lifecycle events

### 2.2 Cross-References

- `api/remediation/v1alpha1/remediationrequest_types.go` (RetentionExpiryTime field)
- `pkg/remediationorchestrator/phase/types.go` (IsTerminal)
- `internal/controller/remediationorchestrator/reconciler.go` (Reconcile, terminal housekeeping)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Mitigation |
|----|------|--------|-------------|------------|
| R1 | Cascading deletion of child resources | Owner refs should handle GC, but orphans possible | Low | UT + IT verify children cascade-delete via owner refs |
| R2 | Race between TTL delete and in-flight reconcile | CRD deleted while handler processes it | Low | IsNotFound guard on Delete; terminal housekeeping exits early |
| R3 | Clock skew on multi-node clusters | Premature or delayed cleanup | Low | Use server-side `metav1.Now()` consistently |
| R4 | Existing tests break from CompletedAt addition | Tests asserting nil CompletedAt on Failed/TimedOut | Medium | Audit existing tests, update assertions as needed |

---

## 4. Scope

### 4.1 Features to be Tested

- **Config wiring** (`internal/config/remediationorchestrator/config.go`): New `Retention.Period` field
- **Reconciler retention** (`internal/controller/remediationorchestrator/reconciler.go`): SetRetentionPeriod, terminal housekeeping TTL logic
- **CompletedAt fix** (`reconciler.go`): `transitionToFailed`, `handleGlobalTimeout`
- **Cleanup audit** (`reconciler.go`): `emitRetentionCleanupAudit`
- **IsTerminalPhase consolidation** (`reconciler.go`): Remove local function

### 4.2 Features Not to be Tested

- Audit persistence to PostgreSQL (covered by existing DS tests)
- Owner reference cascade deletion (Kubernetes GC, not our code)
- E2E full cluster lifecycle (deferred — Kind cluster TTL cycle takes 24h+)

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (config parsing, retention logic, CompletedAt fix)
- **Integration**: >=80% of integration-testable code (envtest lifecycle: terminal → TTL set → delete)
- **E2E**: Deferred — 24h TTL is not practical for E2E; covered by UT + IT

### 5.2 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Centralized TTL in terminal housekeeping | Single code change point; handles ALL terminal phases including externally-set Cancelled/Skipped |
| Setter pattern for RetentionPeriod | Consistent with existing `SetRESTMapper`, `SetAsyncPropagation`; avoids breaking `NewReconciler` signature |
| RequeueAfter for TTL wakeup | Standard controller-runtime pattern; informer re-list on restart ensures no missed cleanups |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-ORCH-265 | RetentionExpiryTime set on Completed | P0 | Unit | UT-RO-265-001 | Pending |
| BR-ORCH-265 | RetentionExpiryTime set on Failed | P0 | Unit | UT-RO-265-002 | Pending |
| BR-ORCH-265 | RetentionExpiryTime set on TimedOut | P0 | Unit | UT-RO-265-003 | Pending |
| BR-ORCH-265 | RetentionExpiryTime NOT set on non-terminal | P0 | Unit | UT-RO-265-004 | Pending |
| BR-ORCH-265 | RetentionExpiryTime idempotent (not overwritten) | P1 | Unit | UT-RO-265-005 | Pending |
| BR-ORCH-265 | Cleanup deletes expired CRD | P0 | Unit | UT-RO-265-006 | Pending |
| BR-ORCH-265 | Cleanup requeues when not expired | P0 | Unit | UT-RO-265-007 | Pending |
| BR-ORCH-265 | Audit emitted before deletion | P0 | Unit | UT-RO-265-008 | Pending |
| BR-ORCH-265 | CompletedAt set on transitionToFailed | P0 | Unit | UT-RO-265-009 | Pending |
| BR-ORCH-265 | CompletedAt set on handleGlobalTimeout | P0 | Unit | UT-RO-265-010 | Pending |
| BR-ORCH-265 | Default retention period is 24h | P0 | Unit | UT-RO-265-011 | Pending |
| BR-ORCH-265 | Retention period configurable from YAML | P0 | Unit | UT-RO-265-012 | Pending |
| BR-ORCH-265 | Retention period validated (must be > 0) | P1 | Unit | UT-RO-265-013 | Pending |
| BR-ORCH-265 | Terminal RR lifecycle: phase → TTL → delete | P0 | Integration | IT-RO-265-001 | Pending |
| BR-ORCH-265 | Already-expired RR is deleted on reconcile | P0 | Integration | IT-RO-265-002 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**File**: `test/unit/remediationorchestrator/controller/retention_ttl_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-RO-265-001 | Terminal Completed RR gets RetentionExpiryTime = now + retentionPeriod | Pending |
| UT-RO-265-002 | Terminal Failed RR gets RetentionExpiryTime | Pending |
| UT-RO-265-003 | Terminal TimedOut RR gets RetentionExpiryTime | Pending |
| UT-RO-265-004 | Non-terminal RR (Pending/Processing) does NOT get RetentionExpiryTime | Pending |
| UT-RO-265-005 | RR with existing RetentionExpiryTime is not overwritten | Pending |
| UT-RO-265-006 | RR with expired RetentionExpiryTime is deleted from cluster | Pending |
| UT-RO-265-007 | RR with future RetentionExpiryTime returns RequeueAfter | Pending |
| UT-RO-265-008 | Audit event emitted before CRD deletion | Pending |

**File**: `test/unit/remediationorchestrator/controller/completed_at_fix_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-RO-265-009 | transitionToFailed sets CompletedAt | Pending |
| UT-RO-265-010 | handleGlobalTimeout sets CompletedAt | Pending |

**File**: `test/unit/remediationorchestrator/config_test.go` (append)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-RO-265-011 | DefaultConfig().Retention.Period == 24h | Pending |
| UT-RO-265-012 | LoadFromFile parses retention.period from YAML | Pending |
| UT-RO-265-013 | Validate rejects retention.period <= 0 | Pending |

### Tier 2: Integration Tests

**File**: `test/integration/remediationorchestrator/retention_ttl_integration_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-RO-265-001 | RR → Failed → RetentionExpiryTime set → after TTL → CRD deleted | Pending |
| IT-RO-265-002 | RR with pre-set expired RetentionExpiryTime is deleted on reconcile | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. 24h TTL is not practical for E2E tests in CI. Covered by unit + integration tiers with time manipulation.

---

## 9. Test Cases (P0 Detail)

### UT-RO-265-001: RetentionExpiryTime set on Completed RR

**BR**: BR-ORCH-265
**Priority**: P0

**Test Steps**:
1. **Given**: RR in PhaseCompleted with RetentionExpiryTime=nil, reconciler with 24h retention
2. **When**: Reconcile is called
3. **Then**: RetentionExpiryTime is set to ~now + 24h; result has RequeueAfter ~24h

### UT-RO-265-006: Cleanup deletes expired CRD

**BR**: BR-ORCH-265
**Priority**: P0

**Test Steps**:
1. **Given**: RR in PhaseCompleted with RetentionExpiryTime = 1 hour ago
2. **When**: Reconcile is called
3. **Then**: RR is deleted from the cluster (client.Get returns NotFound)

### IT-RO-265-001: Full TTL lifecycle with envtest

**BR**: BR-ORCH-265
**Priority**: P0

**Test Steps**:
1. **Given**: RR created with Pending phase, reconciler with 1s retention (for test speed)
2. **When**: SP + AA complete, RR transitions to terminal (Failed via forced failure)
3. **Then**: RetentionExpiryTime is set; after 1s + reconcile, RR is deleted

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `fake.NewClientBuilder()` for K8s client, `MockRoutingEngine`, nil audit store
- **Location**: `test/unit/remediationorchestrator/controller/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest (etcd + kube-apiserver)
- **Location**: `test/integration/remediationorchestrator/`

---

## 11. Execution Order

1. **Phase 1**: Config unit tests (UT-RO-265-011, 012, 013) → config implementation
2. **Phase 2**: CompletedAt unit tests (UT-RO-265-009, 010) → fix implementation
3. **Phase 3**: TTL unit tests (UT-RO-265-001–008) → terminal housekeeping implementation
4. **Phase 4**: Integration tests (IT-RO-265-001, 002) → verify with envtest
5. **Phase 5**: Helm + manifest updates (no new tests needed)

---

## 12. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| Existing reconciler tests asserting `CompletedAt == nil` after Failed | No CompletedAt on Failed RRs | Assert CompletedAt is set | F3: CompletedAt now set on all terminal phases |
| Terminal housekeeping tests returning `Result{}` | No requeue on terminal | May now return RequeueAfter | TTL requeue added |

---

## 13. Execution

```bash
# Unit tests (focused)
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="265" -ginkgo.v

# Integration tests (focused)
go test ./test/integration/remediationorchestrator/... -ginkgo.focus="265" -ginkgo.v

# Full suite regression
go test ./test/unit/remediationorchestrator/... -ginkgo.v
go test ./test/integration/remediationorchestrator/... -ginkgo.v
```

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
