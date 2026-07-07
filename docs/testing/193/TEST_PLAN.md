# Test Plan: EM Cluster-Scoped Resource Assessment (Node, PersistentVolume)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-193-v1.1
**Feature**: `EffectivenessMonitor.assessMetrics()`/`assessAlert()` produce meaningful,
deterministic assessment signal for cluster-scoped `SignalTarget` kinds (`Node`,
`PersistentVolume`) instead of unconditionally reporting "no metric data available."
v1.1 adds: the `effectiveness.metrics.assessed` audit event's `metric_deltas` sub-object is
populated for these same cluster-scoped targets (SOC2 CC8.1 / FedRAMP AU-3 audit-completeness
gap fix), propagated full-pipeline through DataStorage and Kubernaut Agent.
**Version**: 1.1
**Created**: 2026-07-07
**Author**: EffectivenessMonitor Team
**Status**: Active
**Branch**: `feature/issue-193-em-cluster-scoped-fix`

---

## 1. Introduction

### 1.1 Purpose

Today, every `EffectivenessAssessment` whose `SignalTarget.Kind` is cluster-scoped (`Node`,
`PersistentVolume`, i.e. `Namespace == ""`) gets `MetricsAssessed=false` on every single
reconciliation, and alert resolution degrades to alertname-only matching (unable to
distinguish `Node/worker-1` from `Node/worker-2` firing the same alert). This test plan
proves the fix: deterministic, `kube-state-metrics`-backed PromQL query builders for
Node/PersistentVolume metrics, and population of the pre-existing
`alert.AlertContext.AlertLabels` extension point for precise AlertManager matching.

### 1.2 Objectives

1. **Metrics dispatch**: `assessMetrics()` produces `MetricsAssessed=true` with a
   correctly-directioned score for `Kind=Node` and `Kind=PersistentVolume` targets, using
   query builders that are unit-testable in isolation from Prometheus I/O.
2. **Alert precision**: `assessAlert()` populates `AlertContext.AlertLabels` with the
   Kind-appropriate label key/value for cluster-scoped targets, verified end-to-end by
   inspecting the actual AlertManager filter matchers sent over HTTP.
3. **Zero regression**: The existing namespace-scoped metrics path (`buildMetricQuerySpecs`,
   issue #639 golden-string suite) and the existing alert/health/hash test suites are
   provably untouched.
4. **Documented behavior change**: `isAlertDecay`'s currently-unreachable-for-cluster-scoped
   guard clause (`MetricsAssessed` was always `false`) becomes reachable; this is captured as
   an explicit test, not a silent side effect.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./internal/controller/effectivenessmonitor/...` |
| Integration test pass rate | 100% | `ginkgo ./test/integration/effectivenessmonitor/...` |
| Backward compatibility | 0 regressions | #639 golden-string suite + existing EM UT/IT suites pass unmodified |

---

## 2. References

### 2.1 Authority (governing documents)

- `BR-EM-002`: Alert resolution check via AlertManager API — extended by this issue
- `BR-EM-003`: Prometheus metric comparison — extended by this issue
- [DD-EM-005](../../architecture/decisions/DD-EM-005-cluster-scoped-metrics-alert-assessment.md): Cluster-Scoped Metrics and Alert Assessment
- [DD-EM-003](../../architecture/decisions/DD-EM-003-dual-target-assessment.md): Dual-Target Effectiveness Assessment (defines `SignalTarget`/`TargetResource`)
- Issue #193: EM cluster-scoped resource assessment gap (this test plan's origin)
- Issue #639: EM metrics query golden-string tests (regression scope)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [AGENTS.md](../../../AGENTS.md) — TDD workflow, Pyramid Invariant

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `kube_persistentvolume_claim_ref` join label names (`name`/`claim_namespace`) differ on some KSM versions | PV usage-ratio query silently returns no data | Low | UT-EM-193-002, IT-EM-193-002 | Query is one of several independent metrics (graceful per-query degradation via existing `Available=false` path); code comment cites the verified upstream-`STABLE` schema |
| R2 | `isAlertDecay`'s `MetricsAssessed` guard becomes reachable for cluster-scoped targets for the first time, exposing a previously-dormant code path to new inputs | Alert-decay detection could misfire for Node/PV if the guard's assumptions don't hold at cluster scope | Low | UT-EM-193-006, UT-EM-193-007 (decay-test) | Explicit tests document the exact interaction (healthy + stable hash + regressed metrics → decay suppressed; same but improved metrics → decay detected, control case) |
| R3 | Kind-dispatch branch added to `assessMetrics`/`assessAlert` regresses the unchanged namespace-scoped path | Existing EAs for namespaced targets stop being assessed correctly | Low | Regression: full existing EM UT/IT suite, #639 golden-string suite | Dispatch is gated strictly on `Namespace == ""`; namespace-scoped branch is unmodified code, not just unmodified behavior |

### 3.1 Risk-to-Test Traceability

All three risks (R1–R3) have direct test coverage as listed above; none are open gaps.

---

## 4. Scope

### 4.1 Features to be Tested

- **`buildNodeMetricQuerySpecs`** (`internal/controller/effectivenessmonitor/assess_components.go`): Node condition-based PromQL query specs (Ready, MemoryPressure, DiskPressure)
- **`buildPVMetricQuerySpecs`** (same file): PersistentVolume phase + usage-join PromQL query specs
- **`clusterScopedAlertLabelKey`** (same file): Kind → `kube-state-metrics` label key mapping
- **`assessMetrics` Kind-dispatch branch** (same file): routes cluster-scoped targets to the new builders
- **`assessAlert` `AlertLabels` population** (same file): wires the mapping into `alert.AlertContext`
- **`isAlertDecay`** (same file): interaction with now-reachable cluster-scoped `MetricsAssessed`

### 4.2 Features Not to be Tested

- `pkg/effectivenessmonitor/alert/alert.go` `buildMatchers()`: unchanged, already covered by existing #269 test suite; this plan only proves the caller now populates `AlertLabels`
- Namespace-scoped `buildMetricQuerySpecs`: unchanged; covered by existing #639 golden-string suite (verified as regression, not re-specified here)
- Gateway's `SignalTarget.Kind` propagation for Node/PV: pre-existing, already-tested code (confirmed by due-diligence spike, not modified by this issue)

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Hardcoded Go query builders, no ConfigMap | Node/PV metrics are deterministic and vendor-neutral (see DD-EM-005); a config schema would add surface for a non-existent problem |
| Reuse existing `AlertContext.AlertLabels`, no new AlertManager code | Extension point already existed and was already consumed correctly by `buildMatchers()`; only the population was missing |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: 100% of the new pure-logic functions (`buildNodeMetricQuerySpecs`, `buildPVMetricQuerySpecs`, `clusterScopedAlertLabelKey`)
- **Integration**: 100% of the new wiring points (Kind-dispatch in `assessMetrics`, `AlertLabels` population in `assessAlert`) — see Wiring Manifest in the implementation plan

### 5.2 Two-Tier Minimum

Every new code path has both a UT (pure logic, isolated) and an IT (real dispatch path through the reconciler, real envtest CRD, httptest Prometheus/AlertManager mocks).

### 5.3 Business Outcome Quality Bar

Tests assert on business outcomes (`MetricsAssessed`, `MetricsScore` direction, AlertManager filter matcher content) — not internal call counts or implementation details.

### 5.4 Pass/Fail Criteria

**PASS**: All UT/IT scenarios below pass; #639 golden-string suite and full existing EM UT/IT suite pass unmodified.

**FAIL**: Any new scenario fails, or any existing EM test regresses.

### 5.5 Suspension & Resumption Criteria

**Suspend** if `go build ./...` fails. **Resume** once the build is green.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/effectivenessmonitor/assess_components.go` | `buildNodeMetricQuerySpecs`, `buildPVMetricQuerySpecs`, `clusterScopedAlertLabelKey`, `isAlertDecay` (pure logic, no I/O — the interaction with cluster-scoped `MetricsAssessed` is proven by UT, not IT) | ~40 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/effectivenessmonitor/assess_components.go` | `assessMetrics` Kind-dispatch, `assessAlert` `AlertLabels` population | ~20 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `feature/issue-193-em-cluster-scoped-fix` HEAD | Branched from `origin/main` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-EM-003 | Prometheus metric comparison (extended: cluster-scoped) | P1 | Unit | UT-EM-193-001, UT-EM-193-002 | Pending |
| BR-EM-003 | Prometheus metric comparison (extended: cluster-scoped) | P1 | Integration | IT-EM-193-001, IT-EM-193-002 | Pending |
| BR-EM-002 | Alert resolution check (extended: cluster-scoped precision) | P1 | Unit | UT-EM-193-003, UT-EM-193-004, UT-EM-193-005 | Pending |
| BR-EM-002 | Alert resolution check (extended: cluster-scoped precision) | P1 | Integration | IT-EM-193-003 | Pending |
| BR-EM-012 | Alert decay detection (interaction with now-reachable cluster-scoped metrics) | P2 | Unit | UT-EM-193-006, UT-EM-193-007 | Pending |
| BR-AUDIT-005, BR-EM-003 | Audit `metric_deltas` completeness for cluster-scoped targets (SOC2 CC8.1, FedRAMP AU-2/AU-3) | P1 | Unit | UT-EM-193-008..010, UT-EM-AM-013..016, UT-RH-LOGIC-025/026, UT-KA-433W-014 | Done |
| BR-AUDIT-005, BR-EM-003 | Audit `metric_deltas` completeness for cluster-scoped targets (SOC2 CC8.1, FedRAMP AU-2/AU-3) | P1 | Integration | IT-EM-193-001, IT-EM-193-002 (extended) | Done |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-EM-193-{SEQUENCE}` (issue-number-keyed, matching existing EM precedent
e.g. `UT-EM-269-NNN`, `IT-EM-MC-NNN`).

### Tier 1: Unit Tests

**Testable code scope**: `internal/controller/effectivenessmonitor/assess_components.go` new pure functions

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-193-001` | `buildNodeMetricQuerySpecs` produces Ready/MemoryPressure/DiskPressure PromQL specs, all `LowerIsBetter=true` | Pending |
| `UT-EM-193-002` | `buildPVMetricQuerySpecs` produces Failed/Pending phase specs (`LowerIsBetter=true`) and the usage-join ratio spec | Pending |
| `UT-EM-193-003` | `clusterScopedAlertLabelKey("Node")` returns `("node", true)` | Pending |
| `UT-EM-193-004` | `clusterScopedAlertLabelKey("PersistentVolume")` returns `("persistentvolume", true)` | Pending |
| `UT-EM-193-005` | `clusterScopedAlertLabelKey("SomeUnknownKind")` returns `("", false)` | Pending |
| `UT-EM-193-006` | `isAlertDecay`: Node target, healthy + hash-stable + real metrics regression (`MetricsScore<=0`, newly reachable at cluster scope) → returns `false` | Pending |
| `UT-EM-193-007` | `isAlertDecay`: Node target, healthy + hash-stable + metrics improved, alert still firing → returns `true` (decay detected, control case) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `assessMetrics`/`assessAlert` dispatch through the real reconciler, envtest CRD, httptest Prometheus/AlertManager mocks

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-EM-193-001` | EA with `SignalTarget.Kind=Node`, `Namespace=""` → `MetricsAssessed=true` with a correctly-directioned score, using mock Prometheus data. Extended (v1.1): the `effectiveness.metrics.assessed` audit event's `metric_deltas` sub-object carries the Node fields, verified by querying the real audit trail back via `QueryAuditEvents` | Done |
| `IT-EM-193-002` | EA with `SignalTarget.Kind=PersistentVolume`, `Namespace=""` → `MetricsAssessed=true` with a correctly-directioned score. Extended (v1.1): `metric_deltas` carries the PV fields, verified via `QueryAuditEvents` | Done |
| `IT-EM-193-003` | EA with `SignalTarget.Kind=Node`, `Namespace=""` → the AlertManager request's `filter` query params include `node="<name>"` | Pending |

### Tier 1b/1c: Unit Tests — Audit `metric_deltas` Extension (v1.1, DD-EM-005 addendum)

**Testable code scope**: EM producer mapping, EM audit DTO, DataStorage projection, Kubernaut Agent enrichment/prompt

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-193-008` | `populateMetricsAssessResult` maps the 3 Node condition query names into `NodeNotReadyBefore/After`, `NodeMemoryPressureBefore/After`, `NodeDiskPressureBefore/After` | Done |
| `UT-EM-193-009` | `populateMetricsAssessResult` maps the 3 PersistentVolume query names into `PVPhaseFailedBefore/After`, `PVPhasePendingBefore/After`, `PVUsageRatioBefore/After` | Done |
| `UT-EM-193-010` | `populateMetricsAssessResult` leaves cluster-scoped fields nil when the query is unavailable (graceful degradation) | Done |
| `UT-EM-AM-013` | `RecordMetricsAssessed` sets the 3 Node condition `metric_deltas` fields when populated | Done |
| `UT-EM-AM-014` | `RecordMetricsAssessed` sets the 3 PersistentVolume `metric_deltas` fields when populated | Done |
| `UT-EM-AM-015` | `RecordMetricsAssessed` leaves cluster-scoped fields unset for namespace-scoped assessments | Done |
| `UT-EM-AM-016` | `RecordMetricsAssessed` does not clobber Phase A/B fields when cluster-scoped fields are also present | Done |
| `UT-RH-LOGIC-025` | DataStorage `mapMetricDeltas` maps the 6 cluster-scoped fields and backfills `throughput_before_rps`/`after_rps` into `RemediationMetricDeltas` | Done |
| `UT-RH-LOGIC-026` | DataStorage `mapMetricDeltas` leaves cluster-scoped/throughput fields unset when absent (namespace-scoped, Phase A/B only) | Done |
| `UT-KA-433W-014` | Kubernaut Agent `ds_adapter.go mapMetricDeltas` maps throughput + cluster-scoped fields into `enrichment.MetricDeltas`, and leaves them nil when absent | Done |
| `UT-KA-433-HP-003` (extended) | `FormatMetricDeltas` renders throughput and cluster-scoped Node/PV pairs as human-readable LLM prompt text | Done |

### Tier Skip Rationale

- **E2E**: Not added for this issue — the fix is fully exercised by UT+IT against real
  `kube-state-metrics`-shaped PromQL (verified live during due diligence, not re-verified in
  CI) and real AlertManager matcher construction. No new E2E-only behavior is introduced.

---

## 9. Test Cases (P0/P1 detail)

### UT-EM-193-001: buildNodeMetricQuerySpecs

**BR**: BR-EM-003
**Priority**: P1
**Type**: Unit
**File**: `internal/controller/effectivenessmonitor/cluster_scoped_metrics_test.go`

**Test Steps**:
1. **Given**: a Node name `"worker-1"`
2. **When**: `buildNodeMetricQuerySpecs("worker-1")` is called
3. **Then**: the returned specs' `Query` strings contain `kube_node_status_condition` with
   `node="worker-1"` for `Ready`/`MemoryPressure`/`DiskPressure`, and all have
   `LowerIsBetter=true`

**Acceptance Criteria**: PromQL substrings match the verified spike queries; direction is correct (a firing "not ready"/"pressure" condition is 1, so lower is better after remediation).

### UT-EM-193-002: buildPVMetricQuerySpecs

**BR**: BR-EM-003
**Priority**: P1
**Type**: Unit
**File**: `internal/controller/effectivenessmonitor/cluster_scoped_metrics_test.go`

**Test Steps**:
1. **Given**: a PersistentVolume name `"pvc-abc123"`
2. **When**: `buildPVMetricQuerySpecs("pvc-abc123")` is called
3. **Then**: returned specs include `kube_persistentvolume_status_phase{...phase="Failed"...}`
   and `...phase="Pending"...` (both `LowerIsBetter=true`), plus the usage-join ratio query
   containing `kube_persistentvolume_claim_ref{persistentvolume="pvc-abc123"}`

### IT-EM-193-001/002: Cluster-scoped metrics dispatch

**BR**: BR-EM-003
**Priority**: P1
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/cluster_scoped_193_integration_test.go`

**Test Steps**:
1. **Given**: mock Prometheus configured with a 2-sample matrix response (improvement)
2. **When**: an EA is created with `SignalTarget.Kind=Node` (or `PersistentVolume`) and empty `Namespace`
3. **Then**: the EA eventually reaches `Completed` with `Status.Components.MetricsAssessed=true` and a non-nil `MetricsScore`

### IT-EM-193-003: Cluster-scoped alert matcher precision

**BR**: BR-EM-002
**Priority**: P1
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/cluster_scoped_193_integration_test.go`

**Test Steps**:
1. **Given**: mock AlertManager configured with no active alerts
2. **When**: an EA is created with `SignalTarget.Kind=Node`, `Name="worker-1"`, empty `Namespace`
3. **Then**: `mockAM.GetRequestLog()` contains a request to `/api/v2/alerts` whose `filter` query
   values include `node="worker-1"`

**Dependencies**: None beyond the shared EM integration suite fixtures (`mockProm`, `mockAM`, `k8sClient`).

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: None — pure function tests, no external dependencies
- **Location**: `internal/controller/effectivenessmonitor/` (white-box, `package controller`)

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD with envtest
- **Mocks**: httptest Prometheus + AlertManager mocks (per existing suite convention — external services only)
- **Location**: `test/integration/effectivenessmonitor/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | per `go.mod` | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

None — all upstream propagation (`SignalTarget.Kind` for Node/PV) is pre-existing, already-tested code (confirmed by due-diligence spike).

### 11.2 Execution Order

1. **Phase 1 (RED)**: UT-EM-193-001..005, then IT-EM-193-001..004 — all fail (functions/dispatch don't exist yet)
2. **Phase 2 (GREEN)**: Minimal implementation of the 3 new functions + Kind-dispatch wiring in `assessMetrics`/`assessAlert` — CHECKPOINT W
3. **Phase 3 (REFACTOR)**: Dedup shared query-spec construction, flatten dispatch conditional, enrich fallback message
4. **Phase 4 (Post-Refactor Validation)**: `go build`, `go test`, stale-symbol grep
5. **Phase 5**: Regression — #639 golden-string suite + full existing EM UT/IT suite

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|--------------|
| This test plan | `docs/testing/193/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `internal/controller/effectivenessmonitor/cluster_scoped_metrics_test.go` | Ginkgo BDD, UT-EM-193-001..005 |
| Integration test suite | `test/integration/effectivenessmonitor/cluster_scoped_193_integration_test.go` | Ginkgo BDD, IT-EM-193-001..004 |

---

## 13. Execution

```bash
# Unit tests
go test ./internal/controller/effectivenessmonitor/... -run TestEffectivenessMonitor -v

# Integration tests (requires podman/envtest toolchain)
make test-integration-effectivenessmonitor
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| `buildNodeMetricQuerySpecs` | `assessMetrics` Kind-dispatch | `MetricsAssessed`/`MetricsScore` on EA status | IT-EM-193-001 | Pending |
| `buildPVMetricQuerySpecs` | `assessMetrics` Kind-dispatch | `MetricsAssessed`/`MetricsScore` on EA status | IT-EM-193-002 | Pending |
| `clusterScopedAlertLabelKey` | `assessAlert` `AlertContext` population | AlertManager `filter` query params | IT-EM-193-003 | Pending |

---

## 15. Existing Tests Requiring Updates

None. The namespace-scoped path (`buildMetricQuerySpecs`, #639 golden-string suite) and
`pkg/effectivenessmonitor/alert/alert.go` are unmodified.

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-07-07 | Initial test plan for issue #193 |
| 1.1 | 2026-07-07 | Added audit `metric_deltas` extension coverage (DD-EM-005 v1.1 addendum): UT-EM-193-008..010, UT-EM-AM-013..016, UT-RH-LOGIC-025/026, UT-KA-433W-014, extended UT-KA-433-HP-003 and IT-EM-193-001/002. Closes a SOC2 CC8.1 / FedRAMP AU-3 audit-completeness gap for Node/PersistentVolume targets. |
