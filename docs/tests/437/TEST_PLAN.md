# Test Plan: SP Controller Priority Classification Race Fix

**Feature**: Fix informer cache staleness causing wrong priority classification in reconcileClassifying
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `fix/sp-priority-classification-437`

**Authority**:
- [BR-SP-070-072]: Priority Assignment
- [BR-SP-051-053]: Environment Classification
- Issue #437: SP controller BR-SP-070 priority tests consistently get P3 (unknown environment)

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- `internal/controller/signalprocessing/signalprocessing_controller.go` (`reconcileClassifying`): Add FreshGet to bypass informer cache, defensive guard for nil KubernetesContext, and diagnostic logging
- `test/unit/signalprocessing/controller_reconciliation_test.go`: Unit tests for the defensive guard logic
- `test/e2e/signalprocessing/business_requirements_test.go`: Intermediate assertions for BR-SP-070 priority tests
- `test/infrastructure/datastorage.go`: Enhanced must-gather to capture SP CR YAML

### Out of Scope

- Enricher code changes: the enricher correctly populates namespace labels; the bug is in classification reading stale data
- Rego policy changes: the policy is correct; it receives wrong input due to the race
- K8sEnricher cache TTL tuning: not the root cause

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use FreshGet (APIReader) in reconcileClassifying | Informer cache may not have synced KubernetesContext when enriching completes fast (degraded mode). FreshGet bypasses cache for authoritative data. |
| Use EnrichmentComplete condition as safety valve | Both KubernetesContext and EnrichmentComplete are set in the same AtomicStatusUpdate. If KubernetesContext is nil but EnrichmentComplete=True, it indicates a genuine data inconsistency (not a race). If neither is set, enrichment hasn't propagated yet. |
| StartTime-based timeout (30s) | Prevents infinite requeue if enrichment data never appears. After 30s, falls through to current behavior (classification with defaults). |
| No annotation-based retry counting | Avoids extra API writes and complexity. The condition check + time-based safety valve provides equivalent protection. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (reconcileClassifying defensive guard logic)
- **E2E**: >=80% of full service code (priority classification in Kind cluster)

### 2-Tier Minimum

- **Unit tests**: Validate guard logic (nil KubernetesContext requeue, safety valve fallthrough, normal path)
- **E2E tests**: Validate end-to-end priority classification with correct namespace labels

### Business Outcome Quality Bar

Tests validate that the operations team receives correct priority assignments (P0 for production critical, P1 for staging critical) -- the actual business outcome, not just code path coverage.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/signalprocessing/signalprocessing_controller.go` | `reconcileClassifying` (guard logic only) | ~20 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/signalprocessing/signalprocessing_controller.go` | `reconcileClassifying` (full path) | ~100 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-SP-070 | Priority assignment for production critical | P0 | Unit | UT-SP-437-001 | Pending |
| BR-SP-070 | Priority assignment with nil Namespace | P0 | Unit | UT-SP-437-002 | Pending |
| BR-SP-070 | Safety valve after timeout | P1 | Unit | UT-SP-437-003 | Pending |
| BR-SP-070 | Normal path (no regression) | P0 | Unit | UT-SP-437-004 | Pending |
| BR-SP-070 | Priority assignment E2E (production P0) | P0 | E2E | E2E-SP-437-001 | Pending |
| BR-SP-070 | Priority assignment E2E (staging P1) | P0 | E2E | E2E-SP-437-002 | Pending |

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

- **TIER**: `UT` (Unit), `E2E` (End-to-End)
- **SERVICE**: SP (SignalProcessing)
- **BR_NUMBER**: 437 (Issue number)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `reconcileClassifying` guard logic in `signalprocessing_controller.go`, targeting >=80% of the guard paths.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-SP-437-001` | When enrichment data hasn't propagated (nil KubernetesContext, EnrichmentComplete not set), controller requeues instead of misclassifying as "unknown" | RED |
| `UT-SP-437-002` | When KubernetesContext exists but Namespace is nil, controller requeues instead of panicking or misclassifying | RED |
| `UT-SP-437-003` | Safety valve: after 30s of processing, controller proceeds with classification even if data is incomplete (prevents stuck SPs) | RED |
| `UT-SP-437-004` | Normal path: complete KubernetesContext with namespace labels proceeds to classification without delay (no regression) | RED |

### Tier 2: E2E Tests (enhanced assertion)

**Testable code scope**: Full controller pipeline in Kind cluster, targeting correct priority assignment for production and staging namespaces.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-SP-437-001` | BR-SP-070 P0 production test: intermediate gate verifies KubernetesContext.Namespace has correct labels before checking priority | RED |
| `E2E-SP-437-002` | BR-SP-070 P1 staging test: intermediate gate verifies environment classification before checking priority | RED |

### Tier Skip Rationale

- **Integration**: The bug is a K8s API server/informer cache race that cannot be reproduced with envtest (which uses synchronous fake clients). Unit tests cover the guard logic; E2E tests validate the real-world fix in a Kind cluster.

---

## 6. Test Cases (Detail)

### UT-SP-437-001: Nil KubernetesContext triggers requeue

**BR**: BR-SP-070
**Type**: Unit
**File**: `test/unit/signalprocessing/controller_reconciliation_test.go`

**Given**: SP in Classifying phase with nil KubernetesContext; EnrichmentComplete condition NOT set; StartTime < 30s ago
**When**: Reconcile is triggered
**Then**: Controller returns RequeueAfter > 0 (no error), does NOT call EnvClassifier or PriorityAssigner

**Acceptance Criteria**:
- Result has RequeueAfter > 0 (specifically 500ms)
- No error returned
- EnvClassifier.Classify was NOT called
- PriorityAssigner.Assign was NOT called

### UT-SP-437-002: Nil Namespace triggers requeue

**BR**: BR-SP-070
**Type**: Unit
**File**: `test/unit/signalprocessing/controller_reconciliation_test.go`

**Given**: SP in Classifying phase with KubernetesContext={Namespace: nil}; EnrichmentComplete NOT set; StartTime < 30s ago
**When**: Reconcile is triggered
**Then**: Controller returns RequeueAfter > 0 (no error), does NOT call classifiers

**Acceptance Criteria**:
- Result has RequeueAfter > 0
- No error returned
- Classifiers NOT invoked

### UT-SP-437-003: Safety valve after 30s proceeds with defaults

**BR**: BR-SP-070
**Type**: Unit
**File**: `test/unit/signalprocessing/controller_reconciliation_test.go`

**Given**: SP in Classifying phase with nil KubernetesContext; EnrichmentComplete NOT set; StartTime > 30s ago
**When**: Reconcile is triggered
**Then**: Controller proceeds with classification (calls EnvClassifier, PriorityAssigner) using whatever data is available

**Acceptance Criteria**:
- EnvClassifier.Classify IS called
- PriorityAssigner.Assign IS called
- No infinite requeue loop

### UT-SP-437-004: Normal path (no regression)

**BR**: BR-SP-070
**Type**: Unit
**File**: `test/unit/signalprocessing/controller_reconciliation_test.go`

**Given**: SP in Classifying phase with complete KubernetesContext (Namespace with labels)
**When**: Reconcile is triggered
**Then**: Controller proceeds immediately to classification (no requeue delay)

**Acceptance Criteria**:
- Result has RequeueAfter > 0 (normal phase transition)
- No error returned
- EnvClassifier.Classify IS called with correct context
- PriorityAssigner.Assign IS called

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: controller-runtime fake client (for K8s API); mock classifiers (for EnvClassifier, PriorityAssigner)
- **Key technique**: Two fake clients to simulate informer cache vs API server divergence (stale cache has Phase=Classifying but no KubernetesContext; API client has full data)
- **Location**: `test/unit/signalprocessing/`

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (real Kind cluster)
- **Infrastructure**: Kind cluster with SP controller deployed, namespace creation with kubernaut.ai/environment labels
- **Location**: `test/e2e/signalprocessing/`

---

## 8. Execution

```bash
# Unit tests
make test-unit-signalprocessing

# Specific test by ID
go test ./test/unit/signalprocessing/... -ginkgo.focus="UT-SP-437"

# E2E tests (requires Kind cluster)
make test-e2e-signalprocessing
```

---

## 9. Anti-Pattern Compliance

Per `TESTING_GUIDELINES.md`:

| Anti-Pattern | Status | Notes |
|-------------|--------|-------|
| `time.Sleep()` | COMPLIANT | No time.Sleep used; all waits use Eventually() |
| `Skip()` / `XIt` | COMPLIANT | No pending tests |
| Direct audit testing | N/A | No audit assertions in this plan |
| HTTP endpoint testing | N/A | No HTTP testing |

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for Issue #437 |
