# Test Plan: Issue #253 — Correct EA Timing Model: Separate Propagation Delay from Stabilization Window

**Feature**: Decouple async propagation delay from stabilization window in the EA lifecycle
**Version**: 1.4
**Created**: 2026-03-03
**Author**: Architecture Team
**Status**: Draft
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- [BR-EM-010](../../requirements/BR-EM-010-async-hash-deferral.md): Deferred hash computation (BR-EM-010.3, .4, .5)
- [BR-RO-103](../../requirements/BR-RO-103-async-target-detection.md): Async target detection (BR-RO-103.3, .4, .5)
- [DD-EM-004 v2.0](../../architecture/decisions/DD-EM-004-async-hash-deferral.md): Corrected timing model

> **#277 Update**: Issue #277 migrated the EA CRD from an absolute `Spec.HashComputeAfter`
> (`*metav1.Time`) to a relative `Spec.Config.HashComputeDelay` (`*metav1.Duration`).
> `GitOpsSyncDelay` and `OperatorReconcileDelay` were removed from EA spec and are now
> RO-internal config. The EM computes the deferral deadline as
> `creation + HashComputeDelay`. Where this test plan says "HashComputeAfter" in timing
> formulas, it refers to the deadline the EM derives from the duration field.
> See [DD-EM-004 v3.0](../../architecture/decisions/DD-EM-004-async-hash-deferral.md).

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Issue #251 Test Plan](../ISSUE-251/TEST_PLAN.md) — Foundation tests (detection, gating)

---

## 1. Scope

### In Scope

- **RO config** (`internal/config/remediationorchestrator/config.go`): New `AsyncPropagationConfig` struct with `gitOpsSyncDelay` and `operatorReconcileDelay`; validation; defaults
- **RO propagation delay computation** (`internal/controller/remediationorchestrator/reconciler.go`): Compounding logic; `HashComputeDelay = propagationDelay` set in EA.Spec.Config (not `StabilizationWindow`)
- **EA CRD phase** (`api/effectivenessassessment/v1alpha1/`): `WaitingForPropagation` phase constant; phase transition rules; kubebuilder enum update → `make generate` + `make manifests`
- **EM timing computation** (`internal/controller/effectivenessmonitor/reconciler.go`): `checkAfter = (creation + HashComputeDelay) + StabilizationWindow`; validity deadline extension; `WaitingForPropagation → Stabilizing` transition; validity checker calls use derived deferral deadline as stabilization anchor for async targets
- **EM validity checker** (`pkg/effectivenessmonitor/validity/validity.go`): Reconciler passes the derived deferral deadline (from `HashComputeDelay`) instead of `CreationTimestamp` to `Check()` and `TimeUntilStabilized()`, preventing premature `Stabilizing → Assessing` transition
- **EM phase logic** (`pkg/effectivenessmonitor/phase/`): Updated valid transitions including `WaitingForPropagation`
- **EA CRD config fields** (`api/effectivenessassessment/v1alpha1/`): `Spec.Config.HashComputeDelay` and `Spec.Config.AlertCheckDelay` set by RO (#277 migrated from old spec-level fields)
- **Audit trail** (`pkg/effectivenessmonitor/audit/manager.go`): `hash_compute_delay`, `alert_check_delay` in `assessment.scheduled` event; propagation breakdown (`gitops_sync_delay`, `operator_reconcile_delay`) moved to RO `orchestrator.ea.created` audit event (#277)
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
| EM anchors timing to deferral deadline (`creation + HashComputeDelay`) | Clean separation: RO decides propagation duration, EM uses derived deadline as stabilization anchor |
| EM validity checker uses deferral deadline as stabilization anchor | Prevents premature `Stabilizing → Assessing` transition; EA stays in `Stabilizing` until `(creation + HashComputeDelay) + StabilizationWindow` |
| Duration-based delays in EA config | RO sets `HashComputeDelay` (and optionally `AlertCheckDelay`) in `EA.Spec.Config`; individual breakdown (`gitOpsSyncDelay`, `operatorReconcileDelay`) emitted in RO `orchestrator.ea.created` audit event (#277). |

### Impact on #251 Tests

The following existing #251 tests will require in-place updates to reflect #253's corrected timing model:

| #251 Test | Required Update |
|-----------|----------------|
| UT tests referencing `HashComputeDelay` (formerly `HashComputeAfter`) | Update to use `Config.HashComputeDelay` duration field |
| IT tests asserting timing based on `StabilizationWindow` as anchor | Re-anchor to `(creation + HashComputeDelay) + StabilizationWindow` |
| E2E `02_async_hash_deferral_test.go` timing assertions | Update expected values for corrected formula |

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
| `pkg/effectivenessmonitor/timing/derived.go` | `ComputeDerivedTiming()` — anchor selection, validity formula (sync vs async) | ~15 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/effectivenessmonitor/reconciler.go` | `Reconcile()` — timing anchor, phase transitions, validity extension, validity checker anchor adjustment | ~50 |
| `internal/controller/remediationorchestrator/reconciler.go` | `emitEffectivenessAssessment()` — config-driven propagation delay | ~25 |
| `pkg/effectivenessmonitor/audit/manager.go` | `RecordAssessmentScheduled()` — propagation delay fields | ~10 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-RO-103.4 | Config loads with correct defaults (`gitOpsSyncDelay=3m`, `operatorReconcileDelay=1m`) | P0 | Unit | UT-RO-253-001 | Pass |
| BR-RO-103.4 | Config rejects negative delay; accepts zero delay | P0 | Unit | UT-RO-253-002, UT-RO-253-003 | Pass |
| BR-RO-103.4 | Config loads explicit custom values correctly | P0 | Unit | UT-RO-253-008 | Pass |
| BR-RO-103.5 | Compounding: GitOps-only, operator-only, both, neither | P0 | Unit | UT-RO-253-004, UT-RO-253-005, UT-RO-253-006, UT-RO-253-007 | Pass |
| BR-RO-103.3 | Config-driven propagation delay in EA spec (envtest) | P0 | Integration | IT-RO-253-001 | Written |
| BR-RO-103.5 | Compounding for dual-async target (envtest) | P1 | Integration | IT-RO-253-002 | Written |
| BR-EM-010.3 | `WaitingForPropagation` phase entered and exited correctly | P0 | Unit | UT-EM-253-001, UT-EM-253-002 | Pass |
| BR-EM-010.4 | `checkAfter = (creation + HashComputeDelay) + StabilizationWindow` | P0 | Unit | UT-EM-253-003 | Pass |
| BR-EM-010.4 | Validity deadline extended for async targets (guard not triggered) | P0 | Unit | UT-EM-253-004 | Pass |
| BR-EM-010.4 | Sync target: `PrometheusCheckAfter = creation + StabilizationWindow` | P0 | Unit | UT-EM-253-005 | Pass |
| BR-EM-010.4 | Async target + runtime guard + propagation delay interaction | P1 | Unit | UT-EM-253-006 | Pass |
| BR-EM-010.4 | Validity checker uses deferral deadline as stabilization anchor; EA stays `Stabilizing` until `(creation + HashComputeDelay) + StabilizationWindow` | P0 | Unit | UT-EM-253-007 | Pass |
| BR-EM-010.3 | `WaitingForPropagation → Stabilizing` transition (envtest) | P0 | Integration | IT-EM-253-001 | Pass |
| BR-EM-010.4 | Health checks deferred until `(creation + HashComputeDelay) + StabilizationWindow` (envtest) | P0 | Integration | IT-EM-253-002 | Pass |
| BR-EM-010.5 | Audit `assessment.scheduled` includes propagation delay fields | P1 | Integration | IT-EM-253-003 | Written |
| BR-EM-010.4 | Async target `ValidityDeadline` extended correctly (envtest) | P0 | Integration | IT-EM-253-004 | Pass |
| BR-EM-010.3, BR-EM-010.4 | Full pipeline: corrected timing, phase transitions, audit | P0 | E2E | E2E-FP-253-001 | Written |

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
| `UT-RO-253-001` | Config loads with defaults: `gitOpsSyncDelay=3m`, `operatorReconcileDelay=1m` when not specified | Pass |
| `UT-RO-253-002` | Config rejects negative delays (`gitOpsSyncDelay` and `operatorReconcileDelay`, table-driven) | Pass |
| `UT-RO-253-003` | Config accepts zero delay (disables that stage) | Pass |
| `UT-RO-253-008` | Config loads explicit custom values (`gitOpsSyncDelay=2m`, `operatorReconcileDelay=45s`) correctly | Pass |

**File**: `test/unit/remediationorchestrator/async_propagation_config_test.go`

#### RO — Propagation Delay Compounding

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-253-004` | GitOps-only target → `propagationDelay = gitOpsSyncDelay` | Pass |
| `UT-RO-253-005` | Operator-only target → `propagationDelay = operatorReconcileDelay` | Pass |
| `UT-RO-253-006` | GitOps + operator target → `propagationDelay = gitOpsSyncDelay + operatorReconcileDelay` | Pass |
| `UT-RO-253-007` | Sync target (neither signal) → `propagationDelay = 0`, `HashComputeDelay = nil` | Pass |

**File**: `test/unit/remediationorchestrator/propagation_delay_test.go`

#### EM — Phase Transitions

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-253-001` | `WaitingForPropagation` is a valid phase: `Validate()` accepts it, `IsTerminal()` returns false, `Pending → WaitingForPropagation` allowed | Pass |
| `UT-EM-253-002` | `WaitingForPropagation → Stabilizing` allowed; `WaitingForPropagation → Assessing` forbidden; `WaitingForPropagation → Failed` allowed | Pass |

**File**: `test/unit/effectivenessmonitor/propagation_phase_test.go`

#### EM — Timing Computation

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-253-003` | Async target: `PrometheusCheckAfter = (creation + HashComputeDelay) + StabilizationWindow` | Pass |
| `UT-EM-253-004` | Async target: `ValidityDeadline = creation + propagationDelay + StabilizationWindow + ValidityWindow` (guard not triggered) | Pass |
| `UT-EM-253-005` | Sync target (nil `HashComputeDelay`): `PrometheusCheckAfter = creation + StabilizationWindow`, `ValidityDeadline = creation + ValidityWindow` (contrast with UT-EM-253-004) | Pass |
| `UT-EM-253-006` | Async target + runtime guard: `StabilizationWindow >= ValidityWindow` extends deadline correctly with propagation delay | Pass |
| `UT-EM-253-007` | Validity checker returns `WindowStabilizing` until `(creation + HashComputeDelay) + StabilizationWindow` (not `creation + StabilizationWindow`) for async targets | Pass |

**File**: `test/unit/effectivenessmonitor/timing_computation_test.go`

### Tier 2: Integration Tests

**Testable code scope**: EM reconciler timing and phase logic (~40 LOC), EM validity deadline (~5 LOC), RO config-driven EA creation (~25 LOC), audit payload (~10 LOC). Target: >=80%.

#### EM — Phase Transitions and Timing (envtest)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-EM-253-001` | Async EA enters `WaitingForPropagation`; after deferral deadline elapses, transitions to `Stabilizing` with hash computed | Pass |
| `IT-EM-253-002` | Async EA health checks deferred until `(creation + HashComputeDelay) + StabilizationWindow`; phase stays `Stabilizing` until then (no premature `Assessing`) | Pass |
| `IT-EM-253-003` | Audit `assessment.scheduled` event includes `hash_compute_delay` for async target (#277: `gitops_sync_delay`/`operator_reconcile_delay` moved to RO audit) | Written |
| `IT-EM-253-004` | Async target `ValidityDeadline` extended to `creation + propagationDelay + StabilizationWindow + ValidityWindow` | Pass |

**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

#### RO — Config-Driven Propagation Delay (envtest)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-RO-253-001` | RO config with `gitOpsSyncDelay=2m`, `operatorReconcileDelay=30s`: CRD target EA gets `Config.HashComputeDelay = 30s` (operator-only) | Written |
| `IT-RO-253-002` | GitOps + CRD target: EA gets `Config.HashComputeDelay = 2m30s` (compounded) | Written |

**File**: `test/integration/remediationorchestrator/propagation_delay_integration_test.go`

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-FP-253-001` | Full pipeline with cert-manager CRD: corrected timing (propagation delay from config, not stabilization window); `WaitingForPropagation` phase observed; health checks after `(creation + HashComputeDelay) + StabilizationWindow`; audit includes `hash_compute_delay` | Written |

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

### UT-RO-253-002: Config rejects negative delays (table-driven)

**BR**: BR-RO-103.4
**Type**: Unit
**File**: `test/unit/remediationorchestrator/async_propagation_config_test.go`

**Given**: RO config YAML with a negative duration for each propagation delay field (table-driven)
**When**: Config is loaded and validated
**Then**: `Validate()` returns an error mentioning the offending field name

**Table entries**:
| Field | Value | Expected error contains |
|-------|-------|------------------------|
| `gitOpsSyncDelay` | `-1m` | `"gitOpsSyncDelay"` |
| `operatorReconcileDelay` | `-30s` | `"operatorReconcileDelay"` |

**Acceptance Criteria**:
- Both fields reject negative durations independently
- Error message identifies which field is invalid

### UT-RO-253-003: Config accepts zero delay; zero disables stage in compounding

**BR**: BR-RO-103.4, BR-RO-103.5
**Type**: Unit
**File**: `test/unit/remediationorchestrator/async_propagation_config_test.go`

**Given**: An RO config YAML with `asyncPropagation.gitOpsSyncDelay: 0s`
**When**: Config is loaded and validated
**Then**: `Validate()` succeeds; `GitOpsSyncDelay` is `0`

**Given**: Config `gitOpsSyncDelay=0s`, `operatorReconcileDelay=1m`; target is both GitOps AND CRD
**When**: `ComputePropagationDelay(isGitOps=true, isCRD=true)` is called
**Then**: Returns `1m` (0 + 1m; zero disables that stage even when detection is true)

**Acceptance Criteria**:
- Zero disables the respective stage (operators in environments with instant sync can set 0)
- Compounding with zero delay degrades gracefully: `0 + operatorReconcileDelay = operatorReconcileDelay`

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
**When**: Validating `WaitingForPropagation` as a phase
**Then**:
- `Validate(PhaseWaitingForPropagation)` returns nil (accepted as valid phase)
- `IsTerminal(PhaseWaitingForPropagation)` returns false (not a terminal state)
- `CanTransition(PhasePending, PhaseWaitingForPropagation)` returns true

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

**Given**: EA with `HashComputeDelay = 4m` (deferral deadline = T+4m), `StabilizationWindow = 5m`
**When**: EM computes derived timing
**Then**: `PrometheusCheckAfter = T+4m + 5m = T+9m`; `AlertManagerCheckAfter = T+9m`

### UT-EM-253-004: Async target validity extension (guard not triggered)

**BR**: BR-EM-010.4
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/timing_computation_test.go`

**Given**: EA created at `T+0`, `HashComputeDelay = 4m` (deferral deadline = T+4m), `StabilizationWindow = 5m`, `ValidityWindow = 10m` (guard NOT triggered: 5m < 10m)
**When**: EM computes validity deadline
**Then**: `ValidityDeadline = T+0 + 4m + 5m + 10m = T+19m`

**Acceptance Criteria**:
- Values chosen so runtime guard does not trigger (`StabilizationWindow < ValidityWindow`)
- Validity accounts for propagation delay + stabilization + validity

### UT-EM-253-005: Sync target timing (formula contrast with UT-EM-253-004)

**BR**: BR-EM-010.4
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/timing_computation_test.go`

**Given**: EA created at `T+0`, `HashComputeDelay = nil`, `StabilizationWindow = 5m`, `ValidityWindow = 10m` (same stab/validity as UT-EM-253-004)
**When**: EM computes derived timing
**Then**:
- `PrometheusCheckAfter = T+0 + 5m = T+5m` (anchor = creation, not shifted)
- `ValidityDeadline = T+0 + 10m = T+10m` (no propagation delay, no guard extension)

**Acceptance Criteria**:
- Same `stab=5m`, `validity=10m` inputs as UT-EM-253-004 → different results demonstrate formula asymmetry
- Sync: `ValidityDeadline = creation + validity = T+10m` (assessment window = 5m)
- Async (UT-EM-253-004): `ValidityDeadline = creation + prop + stab + validity = T+19m` (assessment window = 10m)
- Sync targets (nil `HashComputeDelay`) anchor timing to EA creation, not to a zero timestamp
- Nil `HashComputeDelay` means compute hash immediately (sync workflow, backward compatible)

### UT-EM-253-006: Async target + runtime guard interaction

**BR**: BR-EM-010.4
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/timing_computation_test.go`

**Given**: EA created at `T+0`, `HashComputeDelay = 4m` (deferral deadline = T+4m), `StabilizationWindow = 15m`, `ValidityWindow = 10m` (guard triggered: 15m >= 10m)
**When**: EM computes derived timing
**Then**:
- `PrometheusCheckAfter = T+4m + 15m = T+19m`
- `EffectiveValidity = 15m + 10m = 25m` (guard extends)
- `ValidityDeadline = T+4m + 25m = T+29m` (anchored to deferral deadline)

**Acceptance Criteria**:
- Runtime guard (`StabilizationWindow >= ValidityWindow`) correctly extends effective validity
- Propagation delay (from `HashComputeDelay`) compounds with the extended validity
- This is the "worst case" timing scenario combining both guard and propagation

### UT-EM-253-007: Validity checker stabilization anchor for async targets

**BR**: BR-EM-010.4
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/timing_computation_test.go`

**Given**: EA created at `T+0`, `HashComputeDelay = 4m` (deferral deadline = T+4m), `StabilizationWindow = 5m`, `ValidityDeadline = T+19m`
**When**: Validity checker `Check(anchor, stabilizationWindow, validityDeadline)` is called at various times with `anchor = deferralDeadline` (T+4m)
**Then** (table-driven):

| Time | `Check(T+4m, 5m, T+19m)` | Rationale |
|------|--------------------------|-----------|
| T+3m | `WindowStabilizing` | Before deferral deadline; stabilization hasn't even started |
| T+5m | `WindowStabilizing` | After deferral deadline but before `T+4m + 5m = T+9m` |
| T+8m | `WindowStabilizing` | Still before `T+9m` |
| T+9m | `WindowActive` | Exactly at `T+4m + 5m`; stabilization complete |
| T+10m | `WindowActive` | Within validity window |

**Acceptance Criteria**:
- When the reconciler passes the deferral deadline (`creation + HashComputeDelay`) as the anchor (instead of `CreationTimestamp`), the validity checker correctly gates the `Stabilizing → Assessing` transition until `deferralDeadline + StabilizationWindow`
- Without this fix, the checker would return `WindowActive` at T+5m (using `creation + 5m`), causing premature phase transition
- Contrast: same inputs with `anchor = CreationTimestamp (T+0)` would return `WindowActive` at T+5m — this is the bug being prevented

### IT-EM-253-001: WaitingForPropagation → Stabilizing (envtest)

**BR**: BR-EM-010.3
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

**Given**: EA created with `HashComputeDelay = 8s` in envtest
**When**: EM reconciler processes the EA
**Then**: EA enters `WaitingForPropagation` phase immediately; after ~8s (deferral deadline), transitions to `Stabilizing` with hash computed

**Acceptance Criteria**:
- `Consistently` verifies phase is `WaitingForPropagation` during deferral window
- `Eventually` verifies phase transitions to `Stabilizing` and hash is computed
- K8s event emitted for `WaitingForPropagation` phase entry

### IT-EM-253-002: Health checks anchored to deferral deadline; phase stays Stabilizing (envtest)

**BR**: BR-EM-010.4
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

**Given**: EA created with `HashComputeDelay = 8s`, `StabilizationWindow = 5s`
**When**: EM reconciler processes the EA through WaitingForPropagation → Stabilizing
**Then**:
- `PrometheusCheckAfter ≈ (creation + 8s) + 5s = now + 13s` (not `creation + 5s`)
- `AlertManagerCheckAfter` matches `PrometheusCheckAfter`
- **Phase remains `Stabilizing` until `PrometheusCheckAfter` elapses** (not prematurely `Assessing`)

**Acceptance Criteria**:
- `PrometheusCheckAfter` is approximately `now + 8s + 5s = now + 13s`
- `AlertManagerCheckAfter` matches `PrometheusCheckAfter`
- `Consistently` verifies phase is `Stabilizing` (not `Assessing`) for ~5s after `WaitingForPropagation → Stabilizing` transition (~now+8s to ~now+13s)
- `Eventually` verifies phase transitions to `Assessing` after `PrometheusCheckAfter`
- Tolerance: ±2s for reconciler scheduling

### IT-EM-253-003: Audit includes propagation delay fields (envtest)

**BR**: BR-EM-010.5
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

**Given**: EA created with `Config.HashComputeDelay = 4m` (set by RO for async target)
**When**: EM emits `assessment.scheduled` audit event
**Then**: Audit payload includes:
- `hash_compute_delay = "4m0s"` (from `EA.Spec.Config.HashComputeDelay`)
- `hash_compute_after` = absolute timestamp (derived: `creation + HashComputeDelay`)

> **#277 Update**: Individual breakdown (`gitops_sync_delay`, `operator_reconcile_delay`) moved to RO
> `orchestrator.ea.created` audit event. EM audit emits the aggregate `hash_compute_delay`.

**Acceptance Criteria**:
- `hash_compute_delay` is a parseable duration string
- Field is absent/null for sync targets (nil `HashComputeDelay`)

### IT-EM-253-004: Async target validity deadline (envtest)

**BR**: BR-EM-010.4
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/propagation_timing_integration_test.go`

**Given**: EA created with `HashComputeDelay = 8s`, `StabilizationWindow = 5s`, `ValidityWindow = 30s`
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
**Then**:
- `EA.Spec.Config.HashComputeDelay = 30s` (operator delay only, not stabilization window)
- RO emits `orchestrator.ea.created` audit with `operator_reconcile_delay=30s`, `gitops_sync_delay` absent

### IT-RO-253-002: Config-driven compounded delay (envtest)

**BR**: BR-RO-103.5
**Type**: Integration
**File**: `test/integration/remediationorchestrator/propagation_delay_integration_test.go`

**Given**: RO config with `gitOpsSyncDelay=2m`, `operatorReconcileDelay=30s`; target is both GitOps-managed AND CRD
**When**: RO creates EA
**Then**:
- `EA.Spec.Config.HashComputeDelay = 2m30s` (compounded: gitOpsSyncDelay + operatorReconcileDelay)
- RO emits `orchestrator.ea.created` audit with `gitops_sync_delay=2m`, `operator_reconcile_delay=30s`

### E2E-FP-253-001: Full pipeline with corrected timing (cert-manager)

**BR**: BR-EM-010.3, BR-EM-010.4, BR-RO-103.3, BR-RO-103.5
**Type**: E2E (Full Pipeline)
**File**: `test/e2e/fullpipeline/02_async_hash_deferral_test.go`

**Given**: cert-manager installed in Kind; RO config with `operatorReconcileDelay=1m`; `CertManagerCertNotReady` alert injected; Mock LLM returns `rca_resource_kind: Certificate`
**When**: Full Kubernaut pipeline runs (RR → SP → AA → WFE → Job → NR → EA)
**Then**:
- EA phase transitions include `WaitingForPropagation`
- `EA.Spec.Config.HashComputeDelay = operatorReconcileDelay` (1m, not 5m stabilization)
- `EA.Status.PrometheusCheckAfter ≈ (creation + HashComputeDelay) + StabilizationWindow`
- Audit `assessment.scheduled` includes `hash_compute_delay`
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
| 1.2 | 2026-03-03 | Triage fixes: UT-EM-253-001 expanded (Validate + IsTerminal); UT-EM-253-002 adds Failed transition; UT-RO-253-002 table-driven for both fields; UT-EM-253-004 clarifies guard-not-triggered values; added UT-EM-253-006 (runtime guard + propagation edge case). Total: 14 UT + 6 IT + 1 E2E = 21 scenarios |
| 1.3 | 2026-03-03 | Critical triage: added UT-EM-253-007 (validity checker stabilization anchor for async — prevents premature Stabilizing→Assessing); strengthened IT-EM-253-002 (phase stays Stabilizing until PrometheusCheckAfter); added EA spec fields for individual delays (gitOpsSyncDelay, operatorReconcileDelay) enabling BR-EM-010.5 audit; updated IT-EM-253-003 with individual delay assertions; updated UT-EM-253-005 with explicit ValidityDeadline and formula contrast. Total: 15 UT + 6 IT + 1 E2E = 22 scenarios |
| 1.4 | 2026-03-03 | Final LOW fixes: IT-RO-253-001/002 assert EA spec delay fields; UT-EM-253-005 covers zero-value HashComputeDelay; UT-RO-253-003 covers zero-delay compounding edge case. Total: 15 UT + 6 IT + 1 E2E = 22 scenarios |
| 1.5 | 2026-03-05 | #277 alignment: Updated all references from `HashComputeAfter` (removed spec field) to `HashComputeDelay` (duration in EAConfig); updated IT-EM-253-003 for new EM audit model (`hash_compute_delay`); updated IT-RO-253-001/002 for `Config.HashComputeDelay`; updated E2E-FP-253-001 assertions. |
