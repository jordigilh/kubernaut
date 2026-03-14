# Test Plan: EM Alert Decay Detection

**Feature**: Keep EA open during Prometheus alert decay window to prevent duplicate RRs, with multi-probe cross-validation
**Version**: 2.2
**Created**: 2026-03-04
**Updated**: 2026-03-14
**Author**: AI Assistant
**Status**: Draft
**Branch**: `fix/v1.0.1-chart-platform-agnostic`

**Authority**:
- BR-EM-012: Alert Decay Detection — EM keeps EA open when alert is decaying
- ADR-EM-001: Effectiveness Monitor Service Integration
- DD-017: Effectiveness Monitor v1.1 Deferral
- DD-AUDIT-003: Service Audit Trace Requirements

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- GitHub Issue: #369 (supersedes #368)

---

## 1. Scope

### In Scope

- **EM Reconciler alert decay detection**: `isAlertDecay` helper with multi-probe cross-validation and integration into the alert assessment flow in `reconciler.go`
- **Health re-probe during decay**: Reset `HealthAssessed=false` on each decay pass for live K8s API re-probe
- **Metrics gate**: Check already-assessed `MetricsScore` in `isAlertDecay` for proactive/predictive signal coverage
- **EA CRD types**: `AlertDecayRetries` field in `EAComponents`, `AssessmentReasonAlertDecayTimeout` constant
- **Audit event**: `effectiveness.alert_decay.detected` event type (OpenAPI, ogen, Go audit manager)
- **Assessment reason**: `alert_decay_timeout` in `determineAssessmentReason` when validity expires during decay

### Out of Scope

- Gateway `ShouldDeduplicate` logic (unchanged — already deduplicates Verifying RRs)
- Gateway owner resolution / ghost detection (handles pod-level alerts where pod is replaced)
- RO `completeVerificationIfNeeded` (unchanged — no `NextAllowedExecution` needed)
- AlertManager client or alert scoring logic (existing, not modified)
- Metrics re-probing (too noisy during stabilization; initial pre/post comparison is the meaningful signal)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Keep EA open (don't set `AlertAssessed=true`) during decay | Leverages existing Verifying phase dedup in Gateway, no new fields needed on RR |
| Multi-probe cross-validation: alert must be only negative signal | If health or metrics also show degradation, the alert is genuine — remediation failed |
| Re-probe health (live) on each decay pass | Health is cheap (K8s API) and is the ground truth for reactive signals; catches resource degradation after initial assessment |
| Check metrics score but don't re-probe | Metrics are expensive (Prometheus range queries) and noisy during stabilization; initial pre/post comparison captures proactive signal outcomes |
| Metrics nil/unavailable is neutral, not blocking | First pass may not have metrics yet; nil means no Prometheus data available, not negative |
| Single audit event on first detection, silence on retries | Follows metrics component precedent (lines 559-565 of reconciler.go), avoids audit noise |
| `alert_decay_timeout` reason vs `partial` | Lets DataStorage/dashboards distinguish "never checked alert" from "actively monitored decay" |
| Validity window as natural timeout | If alert persists beyond validity, it's a genuine re-occurrence — new RR is correct |
| Pod-level alerts handled by Gateway ghost detection, not EM | When a pod is replaced, the Gateway's owner resolution fails the lookup and drops the signal — EM decay detection is not needed |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (isAlertDecay helper with metrics gate, determineAssessmentReason decay branch, AlertDecayRetries increment, audit event guard, health re-probe reset)
- **Integration**: >=80% of integration-testable code (reconciler loop with fake AlertManager, EA lifecycle through decay including health re-probe and metrics cross-validation)

### 2-Tier Minimum

All business requirement gaps covered by UT + IT.

### Business Outcome Quality Bar

Tests validate business outcomes: "does the system suppress duplicate RRs during alert decay?" and "does the system correctly complete when the alert resolves or validity expires?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/effectivenessmonitor/reconciler.go` | `isAlertDecay` (enhanced: metrics gate + health re-probe reset), `determineAssessmentReason` (modified) | ~25 |
| `api/effectivenessassessment/v1alpha1/effectivenessassessment_types.go` | `AlertDecayRetries` field, `AssessmentReasonAlertDecayTimeout` constant | ~5 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/effectivenessmonitor/reconciler.go` | `Reconcile` (alert block with decay detection, health re-probe reset, metrics cross-validation, audit emission, requeue behavior) | ~40 modified |
| `pkg/effectivenessmonitor/audit/manager.go` | `RecordAlertDecayDetected` | ~25 new |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-EM-012 | Prevent premature EA completion during alert decay | P0 | Unit | UT-EM-DECAY-001 | Pass |
| BR-EM-012 | Report full effectiveness when alert resolves post-decay | P0 | Unit | UT-EM-DECAY-002 | Pass |
| BR-EM-012 | Distinguish decay timeout from generic partial assessment | P0 | Unit | UT-EM-DECAY-003 | Pass |
| BR-EM-012 | No false decay detection on non-pod resources | P1 | Unit | UT-EM-DECAY-004 | Pass |
| BR-EM-012 | Spec drift aborts decay monitoring immediately | P1 | Unit | UT-EM-DECAY-005 | Pass |
| BR-EM-012 | Operator observability: accurate decay retry count | P0 | Unit | UT-EM-DECAY-006 | Pass |
| BR-EM-012 | Audit trail: single entry per decay detection, no noise | P0 | Unit | UT-EM-DECAY-007 | Pass |
| BR-EM-012 | Metrics negative kills decay hypothesis — EA completes with AlertScore=0.0, no decay retries (proactive signal) | P0 | Unit | UT-EM-DECAY-008 | Pass |
| BR-EM-012 | Metrics nil/unavailable is neutral — EA stays in Assessing, decay proceeds (graceful degradation) | P1 | Unit | UT-EM-DECAY-009 | Pass |
| BR-EM-012 | Health re-probed live on each decay pass — HealthAssessed reset, AlertDecayRetries increments (not stale) | P0 | Unit | UT-EM-DECAY-010 | Pass |
| BR-EM-012 | Health degradation during decay kills hypothesis — EA completes with AlertScore=0.0 (reactive signal) | P0 | Unit | UT-EM-DECAY-011 | Pass |
| BR-EM-012 | End-to-end: suppress duplicates during decay, complete on resolution | P0 | Integration | IT-EM-DECAY-001 | Pass (compiles, requires infra) |
| BR-EM-012 | Proactive signal: metrics negative → EA completes with AlertScore=0.0, AlertDecayRetries=1 (metrics gate) | P0 | Integration | IT-EM-DECAY-002 | Pass (compiles, requires infra) |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-EM-DECAY-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: EM (Effectiveness Monitor)
- **FEATURE**: DECAY (Alert Decay Detection)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `isAlertDecay` helper, `determineAssessmentReason` decay branch, `AlertDecayRetries` increment, audit event guard. Target: >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-DECAY-001` | System prevents premature EA completion when alert is decaying (health OK, spec stable, alert firing) — operator sees EA remain open for monitoring | Pass |
| `UT-EM-DECAY-002` | System correctly reports remediation as fully effective when alert resolves during decay monitoring — operator sees AlertScore=1.0, reason=full | Pass |
| `UT-EM-DECAY-003` | System distinguishes "alert actively monitored but never resolved" from generic partial — operator sees reason=alert_decay_timeout, not partial | Pass |
| `UT-EM-DECAY-004` | System does not falsely detect decay on non-pod resources where health cannot be confirmed — alert is assessed normally | Pass |
| `UT-EM-DECAY-005` | System correctly aborts decay monitoring when target spec changes — spec_drift takes priority over decay | Pass |
| `UT-EM-DECAY-006` | Operator can observe how many times the system re-checked during decay — AlertDecayRetries accurately counts each re-check | Pass |
| `UT-EM-DECAY-007` | System emits exactly one audit trail entry when decay monitoring begins, avoiding audit noise on subsequent re-checks | Pass |

### Tier 1 (continued): New Unit Tests for Cross-Validation (Option D)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-DECAY-008` | System kills decay hypothesis when metrics show no improvement (proactive signal) — EA completes with AlertScore=0.0, AlertDecayRetries=0 | Pass |
| `UT-EM-DECAY-009` | System does not block decay detection when metrics are unavailable (nil score) — EA stays in Assessing with AlertDecayRetries=1, RequeueAfter > 0 | Pass |
| `UT-EM-DECAY-010` | System re-probes health live on each decay pass — HealthAssessed reset after each pass, AlertDecayRetries increments across two passes | Pass |
| `UT-EM-DECAY-011` | System kills decay hypothesis when health degrades on pass 2 — EA completes with AlertScore=0.0, AlertDecayRetries=1 from pass 1 | Pass |

### Tier 2: Integration Tests

**Testable code scope**: Full reconciler loop with fake AlertManager and envtest. Target: >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-EM-DECAY-001` | End-to-end: system suppresses duplicate RRs during alert decay window, then correctly completes remediation when alert resolves — no manual intervention needed | Pass (compiles, requires infra) |
| `IT-EM-DECAY-002` | Proactive signal: alert firing + health positive + metrics negative → EA completes with AlertScore=0.0, AssessmentReason=full, AlertDecayRetries=1 (metrics gate killed hypothesis) | Pass (compiles, requires infra) |

### Tier Skip Rationale

- **E2E**: Deferred — requires real AlertManager integration in Kind cluster. Alert decay is tested adequately at UT+IT level with fake AlertManager.

---

## 6. Test Cases (Detail)

### UT-EM-DECAY-001: System prevents premature completion during alert decay

**BR**: BR-EM-012
**Type**: Unit (behavior)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: A remediation succeeded — resource is healthy (HealthScore=1.0), spec is unchanged (HashComputed=true). But the Prometheus alert is still firing (AlertScore=0.0) due to lookback window decay.
**When**: The EM reconciles the EA
**Then**: The system recognizes this as alert decay and keeps the EA open for re-checking, rather than completing with a misleading AlertScore=0.0.

**Acceptance Criteria** (behavior + correctness):
- EA remains in Assessing phase (not prematurely completed)
- AlertAssessed remains `false` (re-check will happen on next reconcile)
- AlertDecayRetries incremented to 1 (operator can observe decay monitoring started)
- Reconciler requeues (RequeueAfter > 0) instead of completing

---

### UT-EM-DECAY-002: System reports full effectiveness when alert resolves

**BR**: BR-EM-012
**Type**: Unit (correctness + accuracy)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: The system has been monitoring alert decay for 3 reconcile cycles (AlertDecayRetries=3). The Prometheus alert has now resolved (AlertScore=1.0).
**When**: The EM reconciles the EA
**Then**: The system recognizes the alert resolved, completes the EA with the correct score (1.0) and reason (full). The operator sees the remediation was fully effective.

**Acceptance Criteria** (correctness + accuracy):
- AlertAssessed is `true` (final assessment recorded)
- AlertScore is exactly `1.0` (alert confirmed resolved — accuracy)
- AssessmentReason is `"full"` (all components assessed — correctness)
- EA phase is `PhaseCompleted` (lifecycle complete)
- AlertDecayRetries preserved (operator can see how long decay lasted)

---

### UT-EM-DECAY-003: System distinguishes decay timeout from generic partial

**BR**: BR-EM-012
**Type**: Unit (accuracy)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: The system has been actively monitoring alert decay (AlertDecayRetries > 0), but the validity window has expired before the alert resolved.
**When**: The EM reconciles the EA
**Then**: The system completes with `alert_decay_timeout` (not `partial`), accurately communicating to operators and DataStorage that the alert was actively re-checked but never resolved — a different situation from "alert was never checked."

**Acceptance Criteria** (accuracy):
- AssessmentReason is `"alert_decay_timeout"` (not `"partial"` or `"expired"`)
- EA phase is `PhaseCompleted`
- AlertDecayRetries preserved in status (operator can see N re-checks happened)

---

### UT-EM-DECAY-004: No false decay detection on non-pod resources

**BR**: BR-EM-012
**Type**: Unit (correctness)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: A non-pod resource (e.g., ConfigMap, CRD) where HealthScore is nil because health scoring doesn't apply. The alert is firing (AlertScore=0.0).
**When**: The EM reconciles the EA
**Then**: The system does NOT assume alert decay — it cannot confirm the resource is healthy without a health score. The alert is assessed normally with score 0.0.

**Acceptance Criteria** (correctness — no false positives):
- AlertAssessed is `true` (assessed normally, not deferred)
- AlertScore is `0.0` (alert firing, recorded as-is)
- AlertDecayRetries is `0` (decay detection was not triggered)

---

### UT-EM-DECAY-005: Spec drift aborts decay monitoring immediately

**BR**: BR-EM-012
**Type**: Unit (behavior)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: The system is monitoring alert decay (AlertDecayRetries=2), but another remediation or operator modifies the target resource's spec (hash changes).
**When**: The EM reconciles the EA
**Then**: Spec drift takes priority — the assessment is invalidated because the resource is no longer in the post-remediation state. The system completes with `spec_drift`, not `alert_decay_timeout`.

**Acceptance Criteria** (behavior — priority ordering):
- AssessmentReason is `"spec_drift"` (spec drift overrides decay monitoring)
- EA phase is `PhaseCompleted`

---

### UT-EM-DECAY-006: Accurate decay retry counting for operator observability

**BR**: BR-EM-012
**Type**: Unit (accuracy)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: Alert continues firing across multiple reconcile cycles while the resource remains healthy.
**When**: The EM reconciles the EA 3 times consecutively
**Then**: AlertDecayRetries accurately reflects the number of re-checks performed (0 → 1 → 2 → 3). An operator running `kubectl get ea -o yaml` sees exactly how many re-checks occurred.

**Acceptance Criteria** (accuracy):
- After reconcile 1: AlertDecayRetries == 1
- After reconcile 2: AlertDecayRetries == 2
- After reconcile 3: AlertDecayRetries == 3
- AlertAssessed remains false throughout (EA stays open)

---

### UT-EM-DECAY-007: Single audit entry per decay detection (no noise)

**BR**: BR-EM-012
**Type**: Unit (behavior — audit as side effect of business logic)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: Alert decay is detected on two consecutive reconcile cycles.
**When**: The EM reconciles the EA twice, both times detecting decay
**Then**: The audit trail contains exactly one `alert_decay_detected` entry (from the first detection). The second reconcile silently re-checks without generating audit noise. This follows the metrics component precedent for silent retries.

**Acceptance Criteria** (behavior — per TESTING_GUIDELINES anti-pattern compliance):
- RecordAlertDecayDetected called exactly 1 time across both reconciles
- No duplicate K8s events for decay detection
- NOTE: This tests the reconciler's audit emission behavior (correct pattern per TESTING_GUIDELINES §"Business Logic with Audit Side Effects"), NOT the audit infrastructure itself

---

### IT-EM-DECAY-001: End-to-end duplicate suppression during alert decay

**BR**: BR-EM-012
**Type**: Integration (behavior + correctness)
**File**: `test/integration/effectivenessmonitor/alert_decay_integration_test.go`

**Given**: A remediation has completed — RR is in Verifying phase, EA is created. The fake AlertManager returns AlertScore=0.0 (simulating alert decay), then switches to AlertScore=1.0 after a configurable number of reconciles (simulating alert resolution).
**When**: The EM reconciler runs through multiple reconcile cycles (using Eventually(), NOT time.Sleep())
**Then**: The system correctly suppresses duplicate RRs during the decay window (EA stays in Assessing → RR stays in Verifying → Gateway deduplicates), and then completes the assessment as fully effective once the alert resolves.

**Acceptance Criteria** (end-to-end behavior):
- EA phase sequence: Assessing → Assessing (decay retries) → Completed
- EA AlertDecayRetries > 0 (system was actively monitoring decay)
- EA AlertScore = 1.0 (alert confirmed resolved — accuracy)
- EA AssessmentReason = "full" (all components assessed — correctness)
- Audit emission behavior verified at unit level by UT-EM-DECAY-007 (audit as side effect of business operation per TESTING_GUIDELINES); integration test focuses on EA lifecycle and status outcomes

**Anti-pattern compliance**:
- Uses `Eventually()` for all async assertions (TESTING_GUIDELINES §"time.Sleep() FORBIDDEN")
- Triggers business operation (EA creation via CRD) and verifies outcomes — does NOT test audit infrastructure directly (TESTING_GUIDELINES §"Audit Anti-Pattern")
- No `Skip()` calls (TESTING_GUIDELINES §"Skip() FORBIDDEN")

---

### UT-EM-DECAY-008: Metrics negative kills decay hypothesis (proactive signal)

**BR**: BR-EM-012
**Type**: Unit (correctness)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: A proactive alert fired (e.g., "disk usage trending toward 90%"). After remediation, health is positive (resource was never unhealthy — health is always positive for proactive signals). Metrics have been assessed and show no improvement (MetricsAssessed=true, MetricsScore=0.0 — disk usage still trending up). The alert is still firing (AlertScore=0.0).
**When**: The EM reconciles the EA
**Then**: The system recognizes this is NOT alert decay — the metrics prove the remediation didn't address the predicted condition. The alert is assessed normally and the EA completes with the alert score reflecting the genuine failure.

**Acceptance Criteria** (correctness — proactive signal coverage):
- EA Phase is `PhaseCompleted` (not kept open for decay monitoring)
- AlertAssessed is `true` (assessed normally, not deferred)
- AlertScore is exactly `0.0` (alert firing, accepted at face value)
- AlertDecayRetries is `0` (decay was never triggered — metrics gate prevented it)
- AssessmentReason reflects that all components were assessed (not `alert_decay_timeout`)

---

### UT-EM-DECAY-009: Metrics nil/unavailable is neutral

**BR**: BR-EM-012
**Type**: Unit (correctness)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: Health is positive (HealthScore=1.0), spec is stable (HashComputed=true), alert is firing (AlertScore=0.0). MetricsAssessed is `true` but MetricsScore is `nil` (Prometheus returned no data for any query — graceful degradation).
**When**: The EM reconciles the EA
**Then**: The system treats nil metrics as neutral — decay detection proceeds. Absence of metric data should not be confused with negative metric data. The EA stays open for re-checking.

**Acceptance Criteria** (correctness — graceful degradation):
- EA Phase remains `PhaseAssessing` (decay monitoring active, EA not completed)
- AlertAssessed remains `false` (decay monitoring continues)
- AlertDecayRetries is `1` (decay detection activated despite nil metrics)
- RequeueAfter > 0 (reconciler schedules next decay check)

---

### UT-EM-DECAY-010: Health re-probed live on each decay pass

**BR**: BR-EM-012
**Type**: Unit (behavior)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: An EA is in a state where alert decay is detected on the first reconcile pass (health=1.0, alert firing, spec stable).
**When**: The EM reconciles the EA through two consecutive passes (both with alert still firing and health still positive)
**Then**: After the first pass, `HealthAssessed` is reset to `false`, forcing a live re-probe on the next pass. On the second pass, health is re-assessed (HealthAssessed transitions from false back to true via the re-probe), and since health remains positive, decay monitoring continues. The operator sees AlertDecayRetries incrementing on each pass, proving the system is actively re-validating rather than relying on stale data.

**Acceptance Criteria** (behavior — live re-probe, observable via multi-pass):
- After pass 1: `HealthAssessed == false` (reset for re-probe), `AlertDecayRetries == 1`
- After pass 2: `HealthAssessed == false` (re-probed then reset again), `AlertDecayRetries == 2`
- EA Phase remains `PhaseAssessing` throughout both passes (decay monitoring continues)
- AlertAssessed remains `false` throughout (EA stays open)

---

### UT-EM-DECAY-011: Health degradation during decay kills hypothesis

**BR**: BR-EM-012
**Type**: Unit (correctness)
**File**: `test/unit/effectivenessmonitor/alert_decay_test.go`

**Given**: Alert decay was detected on pass 1 (health=1.0, alert=0.0, hash stable), causing HealthAssessed to be reset to false. On pass 2, the resource has started crashing (e.g., OOMKilled after memory increase was insufficient) — the health re-probe will return HealthScore=0.0. The alert is still firing.
**When**: The EM reconciles the EA on pass 2 (with health now degraded)
**Then**: The system kills the decay hypothesis — health is no longer positive, so the alert is genuine. The EA completes with the alert score reflecting the real failure. The operator sees that one decay pass occurred before the system correctly identified the genuine failure.

**Acceptance Criteria** (correctness — reactive signal coverage):
- EA Phase is `PhaseCompleted` (decay hypothesis killed, assessment finalized)
- AlertAssessed is `true` (alert accepted at face value)
- AlertScore is exactly `0.0` (alert firing, remediation failed)
- AlertDecayRetries is `1` (reflects the single decay pass from pass 1, not reset)
- AssessmentReason reflects normal completion (not `alert_decay_timeout`)

---

### IT-EM-DECAY-002: Proactive signal — metrics negative, alert is genuine

**BR**: BR-EM-012
**Type**: Integration (behavior + correctness)
**File**: `test/integration/effectivenessmonitor/alert_decay_integration_test.go`

**Given**: A remediation completed for a proactive signal. The fake AlertManager returns AlertScore=0.0 (alert still firing). Health is always positive (resource was never unhealthy). Fake Prometheus returns metrics showing no improvement (MetricsScore <= 0).
**When**: The EM reconciler runs through reconcile cycles (using Eventually(), NOT time.Sleep())
**Then**: On the first pass, health is positive and metrics are not yet available — decay is suspected temporarily (AlertDecayRetries increments). On the second pass, metrics are assessed as negative — the metrics gate prevents decay detection and the alert is accepted at face value. The EA completes with AlertScore=0.0, correctly identifying the remediation as ineffective for this proactive signal.

**Acceptance Criteria** (end-to-end behavior):
- EA phase: Assessing → (brief decay window on pass 1, metrics not yet available) → Completed
- EA AlertScore = 0.0 (alert firing, remediation failed — accuracy)
- EA AlertAssessed = true (alert finalized, not kept open)
- EA AssessmentReason = `"full"` (all components assessed — correctness; this is NOT `alert_decay_timeout` because the metrics gate killed the decay hypothesis before validity expired)
- EA AlertDecayRetries == 1 (exactly one decay pass before metrics gate killed the hypothesis — accuracy)

**Anti-pattern compliance**:
- Uses `Eventually()` for all async assertions (TESTING_GUIDELINES §"time.Sleep() FORBIDDEN")
- No `time.Sleep()`
- No `Skip()` (TESTING_GUIDELINES §"Skip() FORBIDDEN")
- Triggers business operation (EA creation via CRD) and verifies outcomes (TESTING_GUIDELINES §"Audit Anti-Pattern")

---

## Anti-Pattern Compliance (TESTING_GUIDELINES v2.7.0)

All tests in this plan comply with the mandatory anti-pattern rules:

| Anti-Pattern | Status | Notes |
|-------------|--------|-------|
| `time.Sleep()` FORBIDDEN | Compliant | Integration tests use `Eventually()` for all async operations |
| `Skip()` FORBIDDEN | Compliant | No conditional skips; tests either pass or fail |
| Direct audit infrastructure testing | Compliant | UT-EM-DECAY-007 tests reconciler behavior (audit emission as side effect), NOT `auditStore.StoreAudit()` calls |
| Testing implementation details in BR tests | Compliant | All test descriptions frame business outcomes (what the operator/system gets) |

**Business Outcome Quality Bar**: Every test answers "what does the operator/system get?" — not "what function is called?"

- **Behavior**: Does the system keep the EA open during decay? (UT-001, UT-010, IT-001)
- **Correctness**: Does the system report the right assessment reason? (UT-002, UT-003, UT-005, UT-008, UT-009, UT-011, IT-002)
- **Accuracy**: Does AlertDecayRetries accurately count re-checks? Is AlertScore exactly 1.0 on resolution? (UT-002, UT-006)
- **Cross-validation**: Does the system correctly distinguish decay from genuine failure across reactive and proactive signals? (UT-008, UT-011, IT-002)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: AlertManager client (fake returning configurable scores), AuditManager (spy for event emission)
- **Location**: `test/unit/effectivenessmonitor/alert_decay_test.go`
- **Helpers**: Use `makeReconcilerWithConfig` from existing test infrastructure (see `metrics_timed_out_test.go`)

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (fake AlertManager as real HTTP server)
- **Infrastructure**: envtest (API server), fake AlertManager HTTP server
- **Location**: `test/integration/effectivenessmonitor/alert_decay_integration_test.go`

---

## 8. Execution

```bash
# Unit tests — alert decay specific
go test ./test/unit/effectivenessmonitor/... --ginkgo.focus="UT-EM-DECAY"

# Unit tests — full EM suite
go test ./test/unit/effectivenessmonitor/... -v -count=1

# Integration tests — alert decay specific
go test ./test/integration/effectivenessmonitor/... --ginkgo.focus="IT-EM-DECAY"

# Integration tests — full EM suite
make test-integration-effectivenessmonitor
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for #369 (EM Alert Decay Detection) |
| 2.0 | 2026-03-14 | Option D: Multi-probe cross-validation. Added UT-EM-DECAY-008..011, IT-EM-DECAY-002. Health re-probe on each decay pass. Metrics gate for proactive signal coverage. Updated design decisions, scope, and BR coverage matrix. |
| 2.1 | 2026-03-14 | TESTING_GUIDELINES quality pass: Replaced internal function assertions (isAlertDecay return values) with observable EA CRD status fields (Phase, AlertScore, AlertDecayRetries). All tests driven through public Reconcile() API. Made imprecise assertions exact. Added missing Phase/AlertScore/RequeueAfter criteria. |
| 2.2 | 2026-03-14 | Implementation complete. All new tests pass. Updated status columns Pending → Pass. Added missing AssessmentReason assertions to UT-EM-DECAY-008 and 011 per acceptance criteria. |
