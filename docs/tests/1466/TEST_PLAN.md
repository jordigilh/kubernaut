# Test Plan: Issue #1466 — EA Controller Alert Decay Hot Loop

**IEEE 829 Format** | **Issue**: [#1466](https://github.com/jordigilh/kubernaut/issues/1466)

## 1. Test Plan Identifier

TP-1466 — EffectivenessMonitor Alert Decay Reconciliation Rate Bounding

## 2. References

- **Issue**: #1466 — EA controller stuck in Assessing phase after stabilization window expires
- **Business Requirement**: BR-EM-012 — Alert decay detection and monitoring
- **Architecture**: ADR-EM-001 — Effectiveness Monitor Service Integration
- **Design Decision**: DD-CONTROLLER-001 — Controller watch predicate strategy
- **FedRAMP Control**: SI-4 (Information System Monitoring) — system must not degrade its own monitoring capability through resource exhaustion

## 3. Introduction

This test plan covers the fix for a hot reconciliation loop in the EM controller's alert decay path. The controller performs ~17 reconciliations/second during decay monitoring due to self-triggered watch events from status-only updates. The fix adds `GenerationChangedPredicate` to filter status-only watch events, bounding reconciliation rate to the configured `RequeueAssessmentInProgress` interval (15s).

### 3.1 Root Cause Summary

The EM controller's `For(&eav1.EffectivenessAssessment{})` watch fires on every status update, including the controller's own writes. During alert decay, each reconcile increments `AlertDecayRetries` and resets `HealthAssessed`, triggering an immediate watch-driven re-reconcile that bypasses the intended 15s `RequeueAfter` interval.

### 3.2 Fix Summary

Add `predicate.GenerationChangedPredicate{}` to `SetupWithManager`. This filters status-only updates (which don't increment `metadata.generation` when the CRD has a status subresource), allowing `RequeueAfter` timers to drive reconciliation at the intended rate.

## 4. Test Items

| Item | File | Description |
|------|------|-------------|
| `SetupWithManager` | `internal/controller/effectivenessmonitor/reconciler.go` | Controller registration with watch predicate |
| Alert decay path | `internal/controller/effectivenessmonitor/reconcile_components.go` | `runAlertCheck` decay detection and retry logic |
| Reconcile orchestration | `internal/controller/effectivenessmonitor/reconcile_status.go` | `finalizeReconcile` status update and requeue |

## 5. Features to Be Tested

| Feature | Acceptance Criteria |
|---------|-------------------|
| F1: Bounded decay reconciliation rate | During alert decay, `AlertDecayRetries` increment rate is bounded by `RequeueAssessmentInProgress` (~1 per 15s), not by watch event frequency (~17/s) |
| F2: Decay detection still works | EA remains in Assessing during decay, transitions to Completed when alert resolves |
| F3: Phase progression via RequeueAfter | Pending → Stabilizing → Assessing transitions still occur via RequeueAfter timers |
| F4: Create events still trigger | New EA creation by RO triggers first reconciliation |
| F5: Validity expiration still works | EA completes when validity window expires during decay |

## 6. Features Not Tested

| Feature | Rationale |
|---------|-----------|
| Alert decay detection logic | Unchanged; covered by existing UT-EM-DECAY-001 through 011 |
| Component assessment logic | Unchanged; covered by existing health/hash/alert/metrics unit tests |
| RO ↔ EM handoff | Unchanged; covered by existing RO integration tests |

## 7. Approach

### 7.1 Test Pyramid Allocation

| Tier | Test ID | What It Proves |
|------|---------|---------------|
| **UT** | Existing UT-EM-DECAY-001 through 011 | Alert decay logic correctness (unchanged) |
| **IT** | IT-EM-DECAY-003 (new) | Bounded reconciliation rate under real manager with watch + predicate |
| **IT** | IT-EM-DECAY-001 (existing) | End-to-end decay → resolution lifecycle with real K8s API |

### 7.2 Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `GenerationChangedPredicate` | `SetupWithManager()` | `internal/controller/effectivenessmonitor/reconciler.go:222-223` | IT-EM-DECAY-003 |

## 8. Test Cases

### IT-EM-DECAY-003: Bounded Decay Reconciliation Rate (Issue #1466)

**Objective**: Verify that during alert decay monitoring, the reconciliation rate is bounded by `RequeueAssessmentInProgress` and not by watch event frequency.

**Preconditions**:
- envtest running with EA CRDs installed
- EM controller registered with manager (uses `SetupWithManager`)
- Mock AlertManager returning a firing alert
- Healthy target pod (health score = 1.0)

**Steps**:
1. Create EA with short stabilization window (1s)
2. Wait for decay detection (`AlertDecayRetries > 0`)
3. Record retry count snapshot
4. Wait 5 seconds
5. Read retry count again
6. Assert delta < 10 (bounded rate)

**Expected Result**: `retriesDelta < 10` (with 15s RequeueAfter, expect 0-1 additional reconciles in 5s)

**Failure Without Fix**: `retriesDelta ≈ 85` (17 reconciles/sec × 5s)

**Business Outcome**: System does not exhaust K8s API server and AlertManager resources during normal operation (SI-4 compliance).

## 9. Pass/Fail Criteria

- All existing UT-EM-DECAY tests pass (001-011)
- IT-EM-DECAY-003 passes (bounded reconciliation rate)
- IT-EM-DECAY-001 passes (decay → resolution lifecycle)
- `go build ./...` succeeds
- No new lint errors

## 10. Environmental Needs

| Tier | Environment |
|------|-------------|
| UT | `go test` with fake K8s client |
| IT | envtest (etcd + kube-apiserver) + httptest mocks (Prometheus, AlertManager) + DataStorage containers |

## 11. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| `GenerationChangedPredicate` blocks phase progression | Low | High | EM drives phases via `RequeueAfter`, not watch events. WE controller uses same pattern successfully. |
| Existing IT flaky due to timing | Low | Medium | IT-EM-DECAY-003 uses generous bounds (< 10 retries in 5s) |
| External EA status writer breaks | Very Low | Medium | No external actor writes EA status. RO only reads. |
