# Test Plan: Issue #253 — Correct EA Timing Model: Separate Propagation Delay from Stabilization Window

**Feature**: Decouple async propagation delay from stabilization window in the EA lifecycle
**Version**: 1.0
**Created**: 2026-03-03
**Author**: Architecture Team
**Status**: Draft
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- [BR-EM-010](../../requirements/BR-EM-010-async-hash-deferral.md): Deferred hash computation (BR-EM-010.3, .4, .5)
- [BR-RO-103](../../requirements/BR-RO-103-async-target-detection.md): Async target detection (BR-RO-103.3, .4, .5)
- [DD-EM-004 v2.0](../../architecture/decisions/DD-EM-004-async-hash-deferral.md): Corrected timing model

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Issue #251 Test Plan](../ISSUE-251/TEST_PLAN.md) — Foundation tests (detection, gating)

---

## 1. Scope

### In Scope

- **RO config** (`internal/config/remediationorchestrator/config.go`): New `AsyncPropagationConfig` struct with `gitOpsSyncDelay` and `operatorReconcileDelay`; validation; defaults
- **RO propagation delay computation** (`internal/controller/remediationorchestrator/reconciler.go`): Compounding logic; `HashComputeAfter = now + propagationDelay` (not `StabilizationWindow`)
- **EA CRD phase** (`api/effectivenessassessment/v1alpha1/`): `WaitingForPropagation` phase constant; phase transition rules
- **EM timing computation** (`internal/controller/effectivenessmonitor/reconciler.go`): `checkAfter = HashComputeAfter + StabilizationWindow`; validity deadline extension; `WaitingForPropagation → Stabilizing` transition
- **EM phase logic** (`pkg/effectivenessmonitor/phase/`): Updated valid transitions including `WaitingForPropagation`
- **Audit trail** (`pkg/effectivenessmonitor/audit/manager.go`): Propagation delay fields in `assessment.scheduled` event
- **E2E validation** (`test/e2e/fullpipeline/02_async_hash_deferral_test.go`): Corrected timing assertions

### Out of Scope

- Async detection logic (`IsBuiltInGroup`, GitOps label reading) — covered by #251
- Hash gating mechanism (`CheckHashDeferral`) — covered by #251
- Dynamic propagation delay determination (ArgoCD/Flux API introspection) — future V2
- Helm chart templating for `values.yaml` — configuration is injected via ConfigMap; `values.yaml` wiring is optional

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Config-based delays over dynamic detection | Predictable, no extra RBAC, no coupling to ArgoCD/Flux APIs; dynamic is a V2 enhancement |
| Additive compounding | GitOps sync and operator reconciliation are sequential stages (ArgoCD syncs manifest, then operator reconciles) |
| `WaitingForPropagation` as distinct phase | Operators can distinguish "waiting for change to arrive" from "waiting for system to settle" |
| EM anchors timing to `HashComputeAfter` | Clean separation: RO decides when change arrives, EM uses that as stabilization anchor |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (config validation, compounding logic, phase transitions, timing computation)
- **Integration**: >=80% of integration-testable code (EM reconciler phase transitions with real envtest, RO config-driven EA creation)
- **E2E**: Existing E2E-FP-251-001 updated with corrected timing assertions

### 2-Tier Minimum

Every business requirement gap is covered by at least 2 test tiers (UT + IT) to provide defense-in-depth.

### Business Outcome Quality Bar

Tests validate **business outcomes** — correct timing, correct phases, correct audit trail — not just code path coverage.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/config/remediationorchestrator/config.go` | `AsyncPropagationConfig` struct, `Validate()` additions | ~30 |
| `api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go` | `PhaseWaitingForPropagation` constant | ~5 |
| `pkg/effectivenessmonitor/phase/types.go` | `ValidTransitions` map, `CanTransition()`, `IsTerminal()` | ~15 |
| `pkg/remediationorchestrator/creator/effectivenessassessment.go` | Propagation delay compounding helper | ~20 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/effectivenessmonitor/reconciler.go` | `Reconcile()` — timing anchor, phase transitions, validity extension | ~40 |
| `internal/controller/remediationorchestrator/reconciler.go` | `emitEffectivenessAssessment()` — config-driven propagation delay | ~25 |
| `pkg/effectivenessmonitor/audit/manager.go` | `RecordAssessmentScheduled()` — propagation delay fields | ~10 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-RO-103.4 | Config loads with correct defaults (`gitOpsSyncDelay=3m`, `operatorReconcileDelay=1m`) | P0 | Unit | UT-RO-253-001 | Pending |
| BR-RO-103.4 | Config rejects negative delay; accepts zero delay | P0 | Unit | UT-RO-253-002, UT-RO-253-003 | Pending |
| BR-RO-103.4 | Config loads explicit custom values correctly | P0 | Unit | UT-RO-253-008 | Pending |
| BR-RO-103.5 | Compounding: GitOps-only, operator-only, both, neither | P0 | Unit | UT-RO-253-004, UT-RO-253-005, UT-RO-253-006, UT-RO-253-007 | Pending |
| BR-RO-103.3 | Config-driven propagation delay in EA spec (envtest) | P0 | Integration | IT-RO-253-001 | Pending |
| BR-RO-103.5 | Compounding for dual-async target (envtest) | P1 | Integration | IT-RO-253-002 | Pending |
| BR-EM-010.3 | `WaitingForPropagation` phase entered and exited correctly | P0 | Unit | UT-EM-253-001, UT-EM-253-002 | Pending |
| BR-EM-010.4 | `checkAfter = HashComputeAfter + StabilizationWindow` | P0 | Unit | UT-EM-253-003 | Pending |
| BR-EM-010.4 | Validity deadline extended for async targets | P0 | Unit | UT-EM-253-004 | Pending |
| BR-EM-010.4 | Sync target: `PrometheusCheckAfter = creation + StabilizationWindow` | P0 | Unit | UT-EM-253-005 | Pending |
| BR-EM-010.3 | `WaitingForPropagation → Stabilizing` transition (envtest) | P0 | Integration | IT-EM-253-001 | Pending |
| BR-EM-010.4 | Health checks deferred until `HashComputeAfter + StabilizationWindow` (envtest) | P0 | Integration | IT-EM-253-002 | Pending |
| BR-EM-010.5 | Audit `assessment.scheduled` includes propagation delay fields | P1 | Integration | IT-EM-253-003 | Pending |
| BR-EM-010.4 | Async target `ValidityDeadline` extended correctly (envtest) | P0 | Integration | IT-EM-253-004 | Pending |
| BR-EM-010.3, BR-EM-010.4 | Full pipeline: corrected timing, phase transitions, audit | P0 | E2E | E2E-FP-253-001 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-253-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: RO (Remediation Orchestrator), EM (Effectiveness Monitor)
- **253**: Issue number
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: RO config validation + parsing (~30 LOC), propagation compounding (~20 LOC), EM phase logic (~15 LOC), EM timing computation (~20 LOC). Target: >=80%.

#### RO — Config Validation and Defaults

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-253-001` | Config loads with defaults: `gitOpsSyncDelay=3m`, `operatorReconcileDelay=1m` when not specified | Pending |
| `UT-RO-253-002` | Config rejects negative `gitOpsSyncDelay` | Pending |
| `UT-RO-253-003` | Config accepts zero delay (disables that stage) | Pending |
| `UT-RO-253-008` | Config loads explicit custom values (`gitOpsSyncDelay=2m`, `operatorReconcileDelay=45s`) correctly | Pending |

**File**: `test/unit/remediationorchestrator/async_propagation_config_test.go`

#### RO — Propagation Delay Compounding

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-253-004` | GitOps-only target → `propagationDelay = gitOpsSyncDelay` | Pending |
| `UT-RO-253-005` | Operator-only target → `propagationDelay = operatorReconcileDelay` | Pending |
| `UT-RO-253-006` | GitOps + operator target → `propagationDelay = gitOpsSyncDelay + operatorReconcileDelay` | Pending |
| `UT-RO-253-007` | Sync target (neither signal) → `propagationDelay = 0`, `HashComputeAfter = nil` | Pending |

**File**: `test/unit/remediationorchestrator/propagation_delay_test.go`

#### EM — Phase Transitions

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-253-001` | `WaitingForPropagation` is a valid phase; `Pending → WaitingForPropagation` allowed | Pending |
| `UT-EM-253-002` | `WaitingForPropagation → Stabilizing` allowed; `WaitingForPropagation → Assessing` forbidden | Pending |

**File**: `test/unit/effectivenessmonitor/propagation_phase_test.go`

#### EM — Timing Computation

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-253-003` | Async target: `PrometheusCheckAfter = HashComputeAfter + StabilizationWindow` | Pending |
| `UT-EM-253-004` | Async target: `ValidityDeadline = creation + propagationDelay + StabilizationWindow + ValidityWindow` | Pending |
| `UT-EM-253-005` | Sync target (nil `HashComputeAfter`): `PrometheusCheckAfter = creation + StabilizationWindow` | Pending |

**File**: `test/unit/effectivenessmonitor/timing_computation_test.go`

### Tier 2: Integration Tests

**Testable code scope**: EM reconciler timing and phase logic (~40 LOC), EM validity deadline (~5 LOC), RO config-driven EA creation (~25 LOC), audit payload (~10 LOC). Target: >=80%.

#### EM — Phase Transitions and Timing (envtest)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-EM-253-001` | Async EA enters `WaitingForPropagation`; after `HashComputeAfter` elapses, transitions to `Stabilizing` with hash computed | Pending |
| `IT-EM-253-002` | Async EA health checks (`PrometheusCheckAfter`) are `HashComputeAfter + StabilizationWindow`, not `creation + StabilizationWindow` | Pending |
| `IT-EM-253-003` | Audit `assessment.scheduled` event includes `gitops_sync_delay`, `operator_reconcile_delay`, `total_propagation_delay` for async target | Pending |
| `IT-EM-253-004` | Async target `ValidityDeadline` extended to `creation + propagationDelay + StabilizationWindow + ValidityWindow` | Pending |

**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

#### RO — Config-Driven Propagation Delay (envtest)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-253-001` | RO config with `gitOpsSyncDelay=2m`, `operatorReconcileDelay=30s`: CRD target EA gets `HashComputeAfter = now + 30s` (operator-only) | Pending |
| `IT-RO-253-002` | GitOps + CRD target: EA gets `HashComputeAfter = now + 2m30s` (compounded) | Pending |

**File**: `test/integration/remediationorchestrator/propagation_delay_integration_test.go`

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-FP-253-001` | Full pipeline with cert-manager CRD: corrected timing (propagation delay from config, not stabilization window); `WaitingForPropagation` phase observed; health checks after `HashComputeAfter + StabilizationWindow`; audit includes propagation delay fields | Pending |

**File**: `test/e2e/fullpipeline/02_async_hash_deferral_test.go` (update existing E2E-FP-251-001 with corrected assertions)

**Note**: E2E-FP-253-001 extends the existing E2E-FP-251-001. The test infrastructure (cert-manager install, Mock LLM `cert_not_ready` scenario, namespace isolation) is already in place from #251. The test assertions are updated to validate the corrected timing model.

### Tier Skip Rationale

All tiers (UT, IT, E2E) are covered. No skips.

---

## 6. Test Cases (Detail)

### UT-RO-253-001: Config defaults

**BR**: BR-RO-103.4
**Type**: Unit
**File**: `test/unit/remediationorchestrator/async_propagation_config_test.go`

**Given**: An RO config YAML that does not specify `asyncPropagation` section
**When**: Config is loaded via `LoadFromFile`
**Then**: `GitOpsSyncDelay` defaults to `3m` and `OperatorReconcileDelay` defaults to `1m`

**Acceptance Criteria**:
- Default values match BR-RO-103.4 specification
- Configs without the new section load successfully

### UT-RO-253-002: Config rejects negative delay

**BR**: BR-RO-103.4
**Type**: Unit
**File**: `test/unit/remediationorchestrator/async_propagation_config_test.go`

**Given**: An RO config YAML with `asyncPropagation.gitOpsSyncDelay: -1m`
**When**: Config is loaded and validated
**Then**: `Validate()` returns an error mentioning "gitOpsSyncDelay"

**Acceptance Criteria**:
- Negative durations rejected at config load time

### UT-RO-253-003: Config accepts zero delay

**BR**: BR-RO-103.4
**Type**: Unit
**File**: `test/unit/remediationorchestrator/async_propagation_config_test.go`

**Given**: An RO config YAML with `asyncPropagation.gitOpsSyncDelay: 0s`
**When**: Config is loaded and validated
**Then**: `Validate()` succeeds; `GitOpsSyncDelay` is `0`

**Acceptance Criteria**:
- Zero disables the respective stage (operators in environments with instant sync can set 0)

### UT-RO-253-008: Config loads explicit custom values

**BR**: BR-RO-103.4
**Type**: Unit
**File**: `test/unit/remediationorchestrator/async_propagation_config_test.go`

**Given**: An RO config YAML with `asyncPropagation.gitOpsSyncDelay: 2m` and `asyncPropagation.operatorReconcileDelay: 45s`
**When**: Config is loaded via `LoadFromFile`
**Then**: `GitOpsSyncDelay` is `2m` and `OperatorReconcileDelay` is `45s`

**Acceptance Criteria**:
- Custom values parsed correctly from YAML
- No silent fallback to defaults when values are explicitly provided

### UT-RO-253-004: GitOps-only compounding

**BR**: BR-RO-103.5
**Type**: Unit
**File**: `test/unit/remediationorchestrator/propagation_delay_test.go`

**Given**: Config `gitOpsSyncDelay=3m`, `operatorReconcileDelay=1m`; target is GitOps-managed, built-in API group (Deployment)
**When**: `ComputePropagationDelay(isGitOps=true, isCRD=false)` is called
**Then**: Returns `3m`

### UT-RO-253-005: Operator-only compounding

**BR**: BR-RO-103.5
**Type**: Unit
**File**: `test/unit/remediationorchestrator/propagation_delay_test.go`

**Given**: Config `gitOpsSyncDelay=3m`, `operatorReconcileDelay=1m`; target is CRD, not GitOps-managed
**When**: `ComputePropagationDelay(isGitOps=false, isCRD=true)` is called
**Then**: Returns `1m`

### UT-RO-253-006: GitOps + operator compounding

**BR**: BR-RO-103.5
**Type**: Unit
**File**: `test/unit/remediationorchestrator/propagation_delay_test.go`

**Given**: Config `gitOpsSyncDelay=3m`, `operatorReconcileDelay=1m`; target is both GitOps-managed AND CRD
**When**: `ComputePropagationDelay(isGitOps=true, isCRD=true)` is called
**Then**: Returns `4m` (3m + 1m)

### UT-RO-253-007: Sync target — no delay

**BR**: BR-RO-103.5
**Type**: Unit
**File**: `test/unit/remediationorchestrator/propagation_delay_test.go`

**Given**: Config `gitOpsSyncDelay=3m`, `operatorReconcileDelay=1m`; target is neither GitOps nor CRD
**When**: `ComputePropagationDelay(isGitOps=false, isCRD=false)` is called
**Then**: Returns `0`

### UT-EM-253-001: WaitingForPropagation is valid phase

**BR**: BR-EM-010.3
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/propagation_phase_test.go`

**Given**: Phase transition rules loaded
**When**: Checking `CanTransition(PhasePending, PhaseWaitingForPropagation)`
**Then**: Returns `true`

### UT-EM-253-002: WaitingForPropagation transition rules

**BR**: BR-EM-010.3
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/propagation_phase_test.go`

**Given**: Phase transition rules loaded
**When**: Checking transitions from `WaitingForPropagation`
**Then**: `WaitingForPropagation → Stabilizing` is allowed; `WaitingForPropagation → Assessing` is forbidden; `WaitingForPropagation → Failed` is allowed (error path)

### UT-EM-253-003: Async target timing anchor

**BR**: BR-EM-010.4
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/timing_computation_test.go`

**Given**: EA with `HashComputeAfter = T+4m`, `StabilizationWindow = 5m`
**When**: EM computes derived timing
**Then**: `PrometheusCheckAfter = T+4m + 5m = T+9m`; `AlertManagerCheckAfter = T+9m`

### UT-EM-253-004: Async target validity extension

**BR**: BR-EM-010.4
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/timing_computation_test.go`

**Given**: EA created at `T+0`, `HashComputeAfter = T+4m`, `StabilizationWindow = 5m`, `ValidityWindow = 10m`
**When**: EM computes validity deadline
**Then**: `ValidityDeadline >= T+0 + 4m + 5m + 10m = T+19m` (accounts for propagation + stabilization + validity)

### UT-EM-253-005: Sync target timing

**BR**: BR-EM-010.4
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/timing_computation_test.go`

**Given**: EA with `HashComputeAfter = nil`, `StabilizationWindow = 5m`
**When**: EM computes derived timing
**Then**: `PrometheusCheckAfter = EA.creation + 5m`

**Acceptance Criteria**:
- Sync targets (nil `HashComputeAfter`) anchor timing to EA creation, not to a zero timestamp

### IT-EM-253-001: WaitingForPropagation → Stabilizing (envtest)

**BR**: BR-EM-010.3
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

**Given**: EA created with `HashComputeAfter = now + 8s` in envtest
**When**: EM reconciler processes the EA
**Then**: EA enters `WaitingForPropagation` phase immediately; after ~8s, transitions to `Stabilizing` with hash computed

**Acceptance Criteria**:
- `Consistently` verifies phase is `WaitingForPropagation` during deferral window
- `Eventually` verifies phase transitions to `Stabilizing` and hash is computed
- K8s event emitted for `WaitingForPropagation` phase entry

### IT-EM-253-002: Health checks anchored to HashComputeAfter (envtest)

**BR**: BR-EM-010.4
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

**Given**: EA created with `HashComputeAfter = now + 8s`, `StabilizationWindow = 5s`
**When**: EM reconciler computes derived timing
**Then**: `PrometheusCheckAfter ≈ HashComputeAfter + 5s` (not `creation + 5s`)

**Acceptance Criteria**:
- `PrometheusCheckAfter` is approximately `now + 8s + 5s = now + 13s`
- `AlertManagerCheckAfter` matches `PrometheusCheckAfter`
- Tolerance: ±2s for reconciler scheduling

### IT-EM-253-003: Audit includes propagation delay fields (envtest)

**BR**: BR-EM-010.5
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

**Given**: EA created with `HashComputeAfter` set by RO with known propagation delay
**When**: EM emits `assessment.scheduled` audit event
**Then**: Audit payload includes `total_propagation_delay` duration string

**Acceptance Criteria**:
- `total_propagation_delay` is a parseable duration string
- Field is absent/null for sync targets

### IT-EM-253-004: Async target validity deadline (envtest)

**BR**: BR-EM-010.4
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

**Given**: EA created with `HashComputeAfter = now + 8s`, `StabilizationWindow = 5s`, `ValidityWindow = 30s`
**When**: EM reconciler computes derived timing
**Then**: `ValidityDeadline ≈ creation + 8s + 5s + 30s = creation + 43s`

**Acceptance Criteria**:
- `ValidityDeadline` accounts for propagation delay + stabilization + validity window
- Premature EA expiration does not occur during the propagation wait
- Tolerance: ±2s for reconciler scheduling

### IT-RO-253-001: Config-driven operator-only delay (envtest)

**BR**: BR-RO-103.3, BR-RO-103.4
**Type**: Integration
**File**: `test/integration/remediationorchestrator/propagation_delay_integration_test.go`

**Given**: RO config with `gitOpsSyncDelay=2m`, `operatorReconcileDelay=30s`; CRD target (e.g., `Certificate`)
**When**: RO creates EA after successful remediation
**Then**: `EA.Spec.HashComputeAfter ≈ now + 30s` (operator delay only, not stabilization window)

### IT-RO-253-002: Config-driven compounded delay (envtest)

**BR**: BR-RO-103.5
**Type**: Integration
**File**: `test/integration/remediationorchestrator/propagation_delay_integration_test.go`

**Given**: RO config with `gitOpsSyncDelay=2m`, `operatorReconcileDelay=30s`; target is both GitOps-managed AND CRD
**When**: RO creates EA
**Then**: `EA.Spec.HashComputeAfter ≈ now + 2m30s`

### E2E-FP-253-001: Full pipeline with corrected timing (cert-manager)

**BR**: BR-EM-010.3, BR-EM-010.4, BR-RO-103.3, BR-RO-103.5
**Type**: E2E (Full Pipeline)
**File**: `test/e2e/fullpipeline/02_async_hash_deferral_test.go`

**Given**: cert-manager installed in Kind; RO config with `operatorReconcileDelay=1m`; `CertManagerCertNotReady` alert injected; Mock LLM returns `rca_resource_kind: Certificate`
**When**: Full Kubernaut pipeline runs (RR → SP → AA → WFE → Job → NR → EA)
**Then**:
- EA phase transitions include `WaitingForPropagation`
- `EA.Spec.HashComputeAfter ≈ RR.completionTime + operatorReconcileDelay` (1m, not 5m stabilization)
- `EA.Status.PrometheusCheckAfter ≈ HashComputeAfter + StabilizationWindow`
- Audit `assessment.scheduled` includes `total_propagation_delay`
- EA reaches terminal phase

**Acceptance Criteria**:
- Timing assertions validate the corrected model (propagation ≠ stabilization)
- No false-positive pass due to conflated timing

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: None for pure logic; `fake.NewClientBuilder()` for K8s type tests if needed
- **Location**: `test/unit/remediationorchestrator/`, `test/unit/effectivenessmonitor/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**: envtest (real K8s control plane), CRD registration, real EM/RO reconcilers
- **Location**: `test/integration/effectivenessmonitor/`, `test/integration/remediationorchestrator/`

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster with all Kubernaut services, cert-manager, Mock LLM, AlertManager, Prometheus
- **Location**: `test/e2e/fullpipeline/`
- **Pre-existing**: cert-manager installed in `BeforeAll`, Mock LLM `cert_not_ready` scenario already configured (#251)

---

## 8. Execution

```bash
# Unit tests (all)
make test

# Unit tests (RO propagation delay only)
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-253"

# Unit tests (EM timing/phase only)
go test ./test/unit/effectivenessmonitor/... -ginkgo.focus="UT-EM-253"

# Integration tests (EM)
make test-integration-effectivenessmonitor

# Integration tests (RO)
make test-integration-remediationorchestrator

# E2E Full Pipeline
make test-e2e-fullpipeline
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-03 | Initial test plan: 12 UT + 5 IT + 1 E2E |
| 1.1 | 2026-03-03 | Coverage review: added UT-RO-253-008 (config custom values), IT-EM-253-004 (validity deadline); removed backward compat framing. Total: 13 UT + 6 IT + 1 E2E = 20 scenarios |
