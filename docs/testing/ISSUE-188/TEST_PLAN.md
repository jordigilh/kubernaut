# Test Plan: EA Dual-Target Assessment + Issue #192 Namespace Validation

**Feature**: Separate signal target and remediation target in EffectivenessAssessment for accurate per-component assessment, plus fix EA creation failure for empty/missing namespace fields
**Version**: 1.1
**Created**: 2026-02-25
**Author**: AI Assistant
**Status**: Implemented
**Branch**: `main`

**Authority**:
- [DD-EM-003]: Dual-Target Effectiveness Assessment (Signal Target + Remediation Target)
- [BR-EM-001]: Health assessment uses signal target
- [BR-EM-002]: Metrics assessment uses signal target namespace
- [BR-EM-003]: Alert resolution uses signal target namespace + signal name
- [BR-EM-004]: Spec hash uses remediation target (DD-EM-002)
- [Issue #188]: EA dual-target: separate signal target and remediation target
- [Issue #192]: EA creation fails with 'Required value' for signalTarget and remediationTarget

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **RO `resolveDualTargets()`**: Pure logic that resolves signal + remediation targets from RR and AA. Validates edge cases: nil AA, nil RCA, empty AffectedResource fields.
- **RO `CreateEffectivenessAssessment()`**: Creates EA with correct dual-target fields from resolved targets.
- **EM reconciler component routing**: Each assessment component uses the correct target field per DD-EM-003 (hash→RemediationTarget, health/alert/metrics→SignalTarget).
- **Issue #192 fix**: `TargetResource.Namespace` changed from `+kubebuilder:validation:Required` to `+optional` with `omitempty` to support cluster-scoped resources (Node, PersistentVolume) that legitimately have no namespace.

### Out of Scope

- **EA CRD schema generation**: CRD YAML regeneration is a build step, not testable business logic.
- **Gateway signal target extraction**: Tested separately under Issue #191.
- **HAPI RCA AffectedResource population**: Tested separately under AI Analysis service tests.
- **EM validity window runtime guard**: Already tested under Issue #188 follow-up (UT-EM-VWG-001/002/003, IT-EM-VWG-001).

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Test `resolveDualTargets` indirectly through reconciler.Reconcile() | The existing UT-RO-188-001/002 already cover divergent and fallback paths; direct unit tests for the unexported function would duplicate coverage |
| Integration tests use envtest with real K8s API for CRD validation | Issue #192 was missed because fake client skips schema validation; envtest catches it |
| Fix is schema-only (no logic change) | Cluster-scoped resources legitimately have empty namespace; making the field optional is the correct CRD semantic |
| No E2E tier | Dual-target routing is fully exercisable through UT + IT; E2E would duplicate IT with higher cost and no additional coverage |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (`resolveDualTargets`, `DualTarget` handling in EA creator, EM component routing logic)
- **Integration**: >=80% of integration-testable code (RO reconciler EA creation flow through envtest, EM reconciler dual-target routing through envtest, K8s CRD validation)

### 2-Tier Minimum

Every business requirement is covered by at least Unit + Integration:
- **Unit tests**: Catch logic errors in target resolution, namespace fallback, validation
- **Integration tests**: Catch wiring errors, K8s API validation rejection, cross-component data flow

### Business Outcome Quality Bar

Tests validate **business outcomes**: "Does the EA correctly track which resource was modified?" and "Does the EM measure the right resource for each assessment component?" — not just "Does the function return the right struct?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `resolveDualTargets` | ~30 |
| `pkg/remediationorchestrator/creator/effectivenessassessment.go` | `DualTarget` handling in `CreateEffectivenessAssessment` | ~25 |
| `api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go` | `TargetResource` struct (Namespace: Required→optional for #192) | ~20 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `createEffectivenessAssessmentIfNeeded` (fetches AA, creates EA) | ~55 |
| `pkg/remediationorchestrator/creator/effectivenessassessment.go` | `CreateEffectivenessAssessment` (K8s API create with validation) | ~80 |
| `internal/controller/effectivenessmonitor/reconciler.go` | `assessHash`, `assessHealth`, `assessAlert`, `assessMetrics` (target routing) | ~120 |

---

## 4. BR Coverage Matrix

| BR / Issue | Description | Priority | Tier | Test ID | Status |
|------------|-------------|----------|------|---------|--------|
| DD-EM-003 | EA spec has SignalTarget field | P0 | Unit | UT-EM-188-001 | Pass |
| DD-EM-003 | EA spec has RemediationTarget field | P0 | Unit | UT-EM-188-002 | Pass |
| DD-EM-003 | Signal and remediation targets can differ | P0 | Unit | UT-EM-188-003 | Pass |
| DD-EM-003 | RO sets SignalTarget from RR, RemediationTarget from AA | P0 | Unit | UT-RO-188-001 | Pass |
| DD-EM-003 | RO falls back to RR for both targets when no AA | P0 | Unit | UT-RO-188-002 | Pass |
| Issue #192 | Reconciler propagates empty namespace for cluster-scoped Node target to EA | P0 | Unit | UT-RO-192-001 | Pass |
| DD-EM-003 | EM hash uses RemediationTarget | P0 | Integration | IT-EM-188-004 | Pass |
| DD-EM-003 | EM health uses SignalTarget | P0 | Integration | IT-EM-188-005 | Pass |
| DD-EM-003 | EM alert uses SignalTarget.Namespace | P0 | Integration | IT-EM-188-006 | Pass |
| DD-EM-003 | EM metrics uses SignalTarget.Namespace | P0 | Integration | IT-EM-188-007 | Pass |
| DD-EM-003 | EM drift guard uses RemediationTarget | P0 | Integration | IT-EM-188-008 | Pass |
| DD-EM-003 | Full reconcile with divergent targets completes | P0 | Integration | IT-EM-188-FULL | Pass |
| DD-EM-003 | RO creates EA with divergent targets through envtest | P0 | Integration | IT-RO-188-003 | Pass |
| DD-EM-003 | RO falls back when AA has empty AffectedResource | P0 | Integration | IT-RO-188-003b | Pass |
| DD-EM-003 | RO falls back when no AIAnalysis exists | P0 | Integration | IT-RO-188-003c | Pass |
| Issue #192 | EA creation succeeds for cluster-scoped Node target with empty namespace through envtest | P0 | Integration | IT-RO-192-001 | Pass |

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

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `RO` (RemediationOrchestrator), `EM` (EffectivenessMonitor)
- **BR_NUMBER**: `188` (dual-target), `192` (namespace validation)
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `resolveDualTargets` (~30 lines, target 100%), `DualTarget` handling in EA creator (~25 lines, target 100%), EA type fields (~20 lines, target 100%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-188-001` | EA spec carries the signal target that triggered the alert | Pass |
| `UT-EM-188-002` | EA spec carries the remediation target the workflow modified | Pass |
| `UT-EM-188-003` | Signal and remediation targets can differ (HPA-maxed scenario) | Pass |
| `UT-RO-188-001` | RO populates SignalTarget from RR and RemediationTarget from AA when AI identifies a different resource | Pass |
| `UT-RO-188-002` | RO uses RR target for both when AI analysis is unavailable | Pass |
| `UT-RO-192-001` | Reconciler creates EA with empty namespace for cluster-scoped Node target (code path validation via fake client) | Pass |

### Tier 2: Integration Tests

**Testable code scope**: RO `createEffectivenessAssessmentIfNeeded` (~55 lines, target 80%), EA creator K8s create path (~80 lines, target 80%), EM reconciler target routing (~120 lines, target 85%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-EM-188-004` | EM computes spec hash from the remediation target resource, not the signal target | Pass |
| `IT-EM-188-005` | EM checks health of the signal target resource, not the remediation target | Pass |
| `IT-EM-188-006` | EM scopes alert resolution to the signal target's namespace | Pass |
| `IT-EM-188-007` | EM scopes Prometheus metric queries to the signal target's namespace | Pass |
| `IT-EM-188-008` | EM drift guard re-hashes the remediation target, not the signal target | Pass |
| `IT-EM-188-FULL` | Full EM reconciliation with divergent targets completes with correct per-component routing | Pass |
| `IT-RO-188-003` | RO creates EA with divergent targets when AA identifies a different affected resource | Pass |
| `IT-RO-188-003b` | RO falls back to RR target when AA has empty AffectedResource | Pass |
| `IT-RO-188-003c` | RO falls back to RR target when no AIAnalysis exists (SP failure path) | Pass |
| `IT-RO-192-001` | EA creation succeeds for cluster-scoped Node target with empty namespace through envtest (catches #192 schema rejection) | Pass |

### Tier Skip Rationale

- **E2E**: Dual-target routing is fully exercisable through UT (pure logic) and IT (envtest with real K8s API validation). E2E would require a full Kind cluster with all controllers deployed and a scenario that produces divergent targets (e.g., hpa-maxed), which adds 10+ minutes of setup time for no additional coverage beyond what IT already provides. The demo team's manual validation of the `demo-taint` scenario serves as ad-hoc E2E verification.

---

## 6. Test Cases (Detail)

### UT-RO-192-001: Reconciler creates EA with empty namespace for cluster-scoped Node target

**BR**: Issue #192
**Type**: Unit
**File**: `test/unit/remediationorchestrator/controller/ea_creation_test.go`

**Given**: An RR in namespace `kubernaut-system` with `TargetResource{Kind: "Node", Name: "worker-1", Namespace: ""}` and a completed WorkflowExecution
**When**: The reconciler processes the RR and creates an EA
**Then**: EA is created with `SignalTarget.Namespace == ""` and `RemediationTarget.Namespace == ""`

**Acceptance Criteria**:
- EA exists with Kind == "Node", Name == "worker-1" on both targets
- Both target namespaces are empty (not filled from RR's ObjectMeta namespace)
- Validates the code path propagates empty namespace correctly (fake client skips schema validation)

---

### IT-RO-192-001: EA creation succeeds for cluster-scoped Node target through envtest

**BR**: Issue #192
**Type**: Integration
**File**: `test/integration/remediationorchestrator/ea_creation_integration_test.go`

**Given**: A running envtest API server with the EA CRD installed, an RR targeting `Node/worker-1` with empty namespace
**When**: SP fails, RR transitions to Failed, and the RO reconciler creates an EA with empty namespace on both targets
**Then**: EA is created successfully (envtest enforces CRD schema validation)

**Acceptance Criteria**:
- EA exists in the cluster with SignalTarget and RemediationTarget both having empty namespace
- This test catches Issue #192: prior to the fix, envtest would reject the EA with "Required value" for namespace
- RemediationRequestPhase == "Failed"

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake K8s client (controller-runtime `fake.NewClientBuilder()`)
- **Location**: `test/unit/remediationorchestrator/controller/ea_creation_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see No-Mocks Policy)
- **Infrastructure**: envtest (real K8s API server with CRD schema validation)
- **Location**: `test/integration/remediationorchestrator/ea_creation_integration_test.go`

---

## 8. Execution

```bash
# Issue #192 unit test
go test ./test/unit/remediationorchestrator/controller/... --ginkgo.focus="UT-RO-192"

# Issue #192 integration test
go test ./test/integration/remediationorchestrator/... --ginkgo.focus="IT-RO-192"

# All #188 + #192 tests (existing + new)
go test ./test/unit/effectivenessmonitor/... --ginkgo.focus="UT-EM-188"
go test ./test/unit/remediationorchestrator/... --ginkgo.focus="UT-RO-188|UT-RO-192"
go test ./test/integration/effectivenessmonitor/... --ginkgo.focus="IT-EM-188"
go test ./test/integration/remediationorchestrator/... --ginkgo.focus="IT-RO-188|IT-RO-192"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-25 | Initial test plan covering DD-EM-003 dual-target + Issue #192 namespace validation |
| 1.1 | 2026-02-25 | Refined #192 scope: schema-only fix (TargetResource.Namespace Required→optional), removed obsolete pending tests (UT-RO-188-003/004/005, UT-RO-192-002/003, IT-RO-192-002). Added UT-RO-192-001 and IT-RO-192-001 as implemented and passing. |
