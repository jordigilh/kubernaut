# BR-EM-009: Derived Timing Computation

**Status**: Draft
**Date**: 2026-02-12
**Category**: EFFECTIVENESS
**Priority**: High
**Related**: ADR-EM-001 v1.3, DD-017 v2.3, BR-EM-006, BR-EM-007

---

## Business Need

The Effectiveness Monitor (EM) must compute and persist all timing-derived assessment fields on first reconciliation. This prevents redundant recomputation on every reconcile loop, provides operator observability into the assessment timeline, and enforces the invariant that `StabilizationWindow < ValidityDeadline`.

**EA Spec Simplification**: The EA CRD `spec.config` (EAConfig) contains only `StabilizationWindow` (set by the RO). `ScoringThreshold`, `PrometheusEnabled`, and `AlertManagerEnabled` are EM operational config only — they are not in the EA spec.

Previously, `ValidityDeadline` was set by the RO in the EA spec. This created a risk where the RO could misconfigure the deadline such that `StabilizationWindow > ValidityDeadline`, causing the EA to expire before assessment could begin.

## Requirements

### BR-EM-009.1: ValidityDeadline Computation

**The EM controller MUST compute `ValidityDeadline` on first reconciliation** (Pending → Assessing transition) as:

```
ValidityDeadline = EA.creationTimestamp + config.ValidityWindow
```

Where `config.ValidityWindow` comes from the EM's `ReconcilerConfig` (default: 30 minutes, configurable via EM service config).

**Acceptance Criteria:**
- ValidityDeadline is stored in `EA.Status.ValidityDeadline`
- ValidityDeadline is NOT in EA spec (RO does not set it)
- ValidityDeadline is computed exactly once (first reconciliation only)
- Subsequent reconciliations use the persisted value, no recomputation

### BR-EM-009.2: PrometheusCheckAfter Computation

**The EM controller MUST compute `PrometheusCheckAfter` on first reconciliation** as:

```
PrometheusCheckAfter = EA.creationTimestamp + StabilizationWindow
```

Where `StabilizationWindow` comes from `EA.Spec.Config.StabilizationWindow`. The EA spec `EAConfig` contains only `StabilizationWindow` (set by the RO); `PrometheusEnabled` and `AlertManagerEnabled` are EM operational config only, not in the EA spec.

**Acceptance Criteria:**
- PrometheusCheckAfter is stored in `EA.Status.PrometheusCheckAfter`
- The reconciler uses this value to determine when Prometheus checks can begin
- Value is computed exactly once

### BR-EM-009.3: AlertManagerCheckAfter Computation

**The EM controller MUST compute `AlertManagerCheckAfter` on first reconciliation** as:

```
AlertManagerCheckAfter = EA.creationTimestamp + StabilizationWindow
```

**Acceptance Criteria:**
- AlertManagerCheckAfter is stored in `EA.Status.AlertManagerCheckAfter`
- The reconciler uses this value to determine when AlertManager checks can begin
- Value is computed exactly once

### BR-EM-009.4: Assessment Scheduled Audit Event

**The EM controller MUST emit an `effectiveness.assessment.scheduled` audit event on first reconciliation** containing all derived timing values. This provides a complete audit trail of when each assessment check was scheduled.

**Payload fields:**
- `validity_deadline`: Computed absolute expiry time
- `prometheus_check_after`: Computed earliest Prometheus check time
- `alertmanager_check_after`: Computed earliest AlertManager check time
- `validity_window`: Duration from EM config (for observability)
- `stabilization_window`: Duration from EA spec (for observability)

**Acceptance Criteria:**
- Event emitted exactly once per EA lifecycle (on first reconciliation)
- Event category: `effectiveness`
- Event type: `effectiveness.assessment.scheduled`
- All five payload fields populated with correct values
- Event stored in DataStorage audit trail

### BR-EM-009.5: ValidityWindow Configuration

**The EM's ValidityWindow MUST be exposed as a configurable parameter** in the EM service configuration.

**Acceptance Criteria:**
- `assessment.validityWindow` in EM config YAML (default: 30m)
- Validated: minimum 5m, maximum 24h
- EM config validation enforces `ValidityWindow > StabilizationWindow`
- Wired into `ReconcilerConfig.ValidityWindow`

## Design Rationale

1. **Kubernetes spec/status convention**: The RO sets desired state (StabilizationWindow in spec), and the EM computes observed/derived state (timing fields in status). This is the standard K8s pattern.

2. **Prevents misconfiguration**: Since EM config enforces `ValidityWindow > StabilizationWindow`, and EM computes `ValidityDeadline`, the invariant `StabilizationWindow < ValidityDeadline` is always satisfied.

3. **Performance**: Computing timing once and persisting avoids redundant time arithmetic on every reconcile loop.

4. **Observability**: Operators can `kubectl get ea -o yaml` and immediately see the complete assessment timeline without knowing the EM's internal configuration.

5. **Audit traceability**: The `assessment.scheduled` event provides a complete record of what timing was computed, enabling post-mortem analysis of assessment behavior.

## Test Coverage

See [EM Comprehensive Test Plan](../../services/crd-controllers/07-effectivenessmonitor/EM_COMPREHENSIVE_TEST_PLAN.md) — Derived Timing (DT) domain:
- 6 unit test scenarios (UT-EM-DT-001 through UT-EM-DT-006)
- 9 integration test scenarios (IT-EM-DT-001 through IT-EM-DT-009)
- 2 E2E test scenarios (E2E-EM-DT-001, E2E-EM-DT-002)

## References

- [ADR-EM-001 v1.3](../../architecture/decisions/ADR-EM-001-effectiveness-monitor-service-integration.md) — Design principles 7, sections 4, 6, 8, 9.2.0, 9.4
- [DD-017 v2.3](../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md) — EM trigger section
- BR-EM-006: Stabilization window (unchanged, set by RO in spec)
- BR-EM-007: Validity window (extended: now computed by EM as status field)
