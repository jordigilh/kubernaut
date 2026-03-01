# Test Plan: Guard EA Creation to Successful WFE Only

**Feature**: Only create EffectivenessAssessment when WorkflowExecution completes successfully
**Version**: 1.0
**Created**: 2026-03-01
**Author**: Kubernaut AI
**Status**: Ready for Execution
**Branch**: `main`

**Authority**:
- Issue #240: Guard EA creation to only fire after successful WorkflowExecution
- ADR-EM-001: Effectiveness Monitor Service Integration
- BR-EM-001: Effectiveness Assessment

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)

---

## 1. Scope

### In Scope

- **RO reconciler terminal transitions**: Verifying EA is created ONLY on `transitionToCompleted`, NOT on `transitionToFailed`, `handleGlobalTimeout`, or `handlePhaseTimeout`
- **EffectivenessAssessmentCreator**: Verifying the creator itself works correctly (unchanged logic, but callers change)

### Out of Scope

- EM behavior when no EA exists (EM already handles missing EAs)
- DataStorage or HAPI workflow resolution logic
- EA spec field correctness (covered by existing passing tests UT-RO-EA-001, IT-RO-EA-001, IT-RO-EA-004)

### Design Decisions

- EA is only meaningful when a remediation was successfully applied. Failed or timed-out WFEs may have partially applied changes, making EA results unreliable.
- Existing positive tests (UT-RO-EA-001, IT-RO-EA-001, IT-RO-EA-004) remain unchanged to guard the success path.
- Tests that previously asserted EA creation on failure/timeout are inverted to assert NO EA creation.

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of the reconciler's EA creation decision logic across all 4 terminal paths
- **Integration**: >=80% of the reconciler's terminal transition paths that interact with EA creation via envtest

### 2-Tier Minimum

Every scenario is covered by both UT (fast, isolated with fake client) and IT (envtest with real controllers).

### Business Outcome Quality Bar

Tests validate: "Does the operator see spurious EAs for failed/timed-out remediations?" (answer must be NO).

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic via fake client)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `transitionToCompleted` (EA call retained), `transitionToFailed` (EA call removed), `handleGlobalTimeout` (EA call removed), `handlePhaseTimeout` (EA call removed) | ~150 |

### Integration-Testable Code (envtest with real reconciler)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | Full reconcile loop driving RR through SP -> AIA -> WFE -> terminal phase | ~200 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #240 | No EA on AIA failure (WorkflowResolutionFailed) | P0 | Unit | UT-RO-EA-012 | Pending |
| #240 | No EA on WFE failure | P0 | Unit | UT-RO-EA-013 | Pending |
| #240 | EA still created on successful WFE (regression guard) | P0 | Unit | UT-RO-EA-014 | Pending |
| #240 | No EA on Failed RR (existing test inverted) | P0 | Unit | UT-RO-EA-002 | Pending |
| #240 | No EA on TimedOut RR (existing test inverted) | P0 | Unit | UT-RO-EA-003 | Pending |
| #240 | No EA on phase timeout (existing test inverted) | P0 | Unit | UT-RO-EA-011 | Pending |
| #240 | No EA on SP failure (existing test inverted) | P0 | Integration | IT-RO-EA-002 | Pending |
| #240 | No EA on global timeout (existing test inverted) | P0 | Integration | IT-RO-EA-003 | Pending |
| #240 | No EA on AIA failure via envtest | P0 | Integration | IT-RO-EA-005 | Pending |
| #240 | No EA on WFE failure via envtest | P0 | Integration | IT-RO-EA-006 | Pending |
| #240 | No EA on SP failure with cluster-scoped Node (inverted) | P1 | Integration | IT-RO-192-001 | Pending |
| #240 | No EA on SP failure (dual-target fallback inverted) | P1 | Integration | IT-RO-188-003c | Pending |

---

## 5. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `reconciler.go` terminal transition methods (~150 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-EA-002` | Operator does NOT see spurious EA when WFE fails and RR transitions to Failed | RED |
| `UT-RO-EA-003` | Operator does NOT see spurious EA when RR times out (global timeout) | RED |
| `UT-RO-EA-011` | Operator does NOT see spurious EA when RR times out (phase timeout) | RED |
| `UT-RO-EA-012` | Operator does NOT see spurious EA when AIA fails (WorkflowResolutionFailed, no WFE) | RED |
| `UT-RO-EA-013` | Operator does NOT see spurious EA when WFE fails (RR in Executing -> Failed) | RED |
| `UT-RO-EA-014` | Operator sees EA created when WFE succeeds (positive regression guard) | RED |

### Tier 2: Integration Tests

**Testable code scope**: Full reconciler loop terminal paths (~200 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-EA-002` | No EA created when SP fails (full pipeline, no WFE ever created) | RED |
| `IT-RO-EA-003` | No EA created when RR times out (global timeout, no successful WFE) | RED |
| `IT-RO-EA-005` | No EA created when AIA fails with WorkflowResolutionFailed (full pipeline) | RED |
| `IT-RO-EA-006` | No EA created when WFE fails (full pipeline through Executing -> Failed) | RED |
| `IT-RO-188-003c` | No EA created on SP failure (dual-target fallback path) | RED |
| `IT-RO-192-001` | No EA created on SP failure for cluster-scoped Node target | RED |

### Tier Skip Rationale

- **E2E**: Deferred. EA creation is fully testable at UT + IT tiers. E2E would add runtime cost (Kind cluster) without additional coverage of this specific guard logic.

---

## 6. Test Cases (Detail)

### UT-RO-EA-012: No EA when AIA fails (WorkflowResolutionFailed)

**BR**: #240
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/ea_creation_test.go`

**Given**: RR in Analyzing phase with AIAnalysis in Failed (WorkflowResolutionFailed), no WFE exists
**When**: Reconciler processes the AIA failure and transitions RR to Failed
**Then**: No EffectivenessAssessment CRD is created; RR.Status.EffectivenessAssessmentRef is nil

**Acceptance Criteria**:
- EA CRD does not exist in the namespace after reconciliation
- RR transitions to Failed with correct failurePhase="ai_analysis"
- No EffectivenessAssessed condition is set on the RR

### UT-RO-EA-013: No EA when WFE fails

**BR**: #240
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/ea_creation_test.go`

**Given**: RR in Executing phase with WorkflowExecution in Failed state
**When**: Reconciler processes WFE failure and transitions RR to Failed
**Then**: No EffectivenessAssessment CRD is created

**Acceptance Criteria**:
- EA CRD does not exist in the namespace after reconciliation
- RR transitions to Failed with failurePhase="workflow_execution"

### UT-RO-EA-014: EA still created on successful WFE (regression guard)

**BR**: #240
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/ea_creation_test.go`

**Given**: RR in Executing phase with WorkflowExecution in Completed state
**When**: Reconciler processes WFE success and transitions RR to Completed
**Then**: EffectivenessAssessment CRD IS created with correct spec

**Acceptance Criteria**:
- EA CRD exists with RemediationRequestPhase="Completed"
- EA.Spec.CorrelationID matches RR.Name
- RR.Status.EffectivenessAssessmentRef is populated

### IT-RO-EA-005: No EA when AIA fails (envtest)

**BR**: #240
**Type**: Integration
**File**: `test/integration/remediationorchestrator/ea_creation_integration_test.go`

**Given**: RR created, SP completed, AIA transitions to Failed (WorkflowResolutionFailed)
**When**: Full reconciler loop processes AIA failure
**Then**: No EA CRD exists after RR reaches Failed phase

**Acceptance Criteria**:
- EA CRD `ea-{rr-name}` does NOT exist (Consistently for 5s)
- RR is in Failed phase with RequiresManualReview=true

### IT-RO-EA-006: No EA when WFE fails (envtest)

**BR**: #240
**Type**: Integration
**File**: `test/integration/remediationorchestrator/ea_creation_integration_test.go`

**Given**: RR created, SP completed, AIA completed with workflow, WFE transitions to Failed
**When**: Full reconciler loop processes WFE failure
**Then**: No EA CRD exists after RR reaches Failed phase

**Acceptance Criteria**:
- EA CRD `ea-{rr-name}` does NOT exist (Consistently for 5s)
- RR is in Failed phase with failurePhase="workflow_execution"

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: fake.NewClientBuilder() for K8s client (standard pattern in ea_creation_test.go)
- **Location**: `test/unit/remediationorchestrator/controller/ea_creation_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (envtest with real RO controller)
- **Infrastructure**: envtest (kube-apiserver + etcd)
- **Location**: `test/integration/remediationorchestrator/ea_creation_integration_test.go`

---

## 8. Execution

```bash
# Unit tests
go test ./test/unit/remediationorchestrator/controller/... -ginkgo.focus="EA Creation"

# Integration tests
go test ./test/integration/remediationorchestrator/... -ginkgo.focus="EA Creation"

# Specific test by ID
go test ./test/unit/remediationorchestrator/controller/... -ginkgo.focus="UT-RO-EA-012"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-01 | Initial test plan for issue #240 |
