# Test Plan: Gateway Prometheus Reserved Label Denylist

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1045-v1.0
**Feature**: Denylist Prometheus-reserved labels from dynamic kind resolution in the Gateway
**Version**: 1.0
**Created**: 2026-05-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/1045-prometheus-denylist`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates that the Gateway's dynamic `extractTargetResource` function correctly
excludes Prometheus-reserved label keys (`job`, `service`, `instance`, `endpoint`, `container`)
from Kubernetes resource kind resolution. These labels carry Prometheus scrape metadata, not
Kubernetes resource identifiers, and their inclusion causes false-positive kind matches that
drop signals or direct investigation to wrong targets.

### 1.2 Objectives

1. **Denylist enforcement**: All 5 Prometheus-reserved labels are excluded from dynamic kind resolution
2. **Non-reserved labels unaffected**: Legitimate resource labels (`deployment`, `pod`, `statefulset`, etc.) continue to resolve correctly
3. **Regression safety**: Existing test suites pass without modification
4. **OCP scenario unblocking**: Alerts with `job: kube-state-metrics` no longer cause signal drops

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/gateway/adapters/... -race` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `extractTargetResource` |
| Backward compatibility | 0 regressions | All existing Gateway tests pass |
| Denylist labels blocked | 5/5 | `job`, `service`, `instance`, `endpoint`, `container` |

---

## 2. References

### 2.1 Authority (governing documents)

- [BR-GATEWAY-184]: Gateway signal ingestion — resource extraction
- Issue #1045: Gateway: denylist Prometheus-reserved labels from dynamic kind resolution
- Issue #1029: Dynamic API resource registry

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Prometheus Data Model](https://prometheus.io/docs/concepts/data_model/) — `job` and `instance` are reserved

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `service` denylist changes behavior for alerts where `service` IS the target K8s Service | Alerts relying solely on `service` label resolve to `Unknown` | Low | UT-GW-1045-009 | Issue #1045 explicitly specifies this; monitoring alerts should use more specific labels |
| R2 | Future K8s CRDs with kinds matching denylist names | Denylist filters legitimate custom resources | Very Low | UT-GW-1045-012 | Denylist covers only well-known Prometheus spec constants |
| R3 | Static fallback path inconsistency | Static path still maps `service` → `Service` | Low | N/A | Static path is deprecated; documented in code |
| R4 | Denylist bypass via label key casing | Prometheus labels are case-sensitive, always lowercase | Very Low | UT-GW-1045-011 | `LabelToKind` uses exact match; uppercase keys won't match API discovery |

### 3.1 Risk-to-Test Traceability

- **R1**: Covered by UT-GW-1045-009 (only-reserved-labels → Unknown)
- **R2**: Covered by UT-GW-1045-006 through UT-GW-1045-008 (non-reserved labels still work)
- **R3**: Documented in test plan; static path tests unchanged
- **R4**: Covered by UT-GW-1045-011 (adversarial casing)

---

## 4. Scope

### 4.1 Features to be Tested

- **`extractTargetResource` denylist** (`pkg/gateway/adapters/prometheus_adapter.go`): Prometheus-reserved label keys are skipped before `LabelToKind()` lookup in the dynamic resolution path
- **`PrometheusReservedLabels` variable** (`pkg/gateway/adapters/prometheus_adapter.go`): Exported package-level denylist for introspection and testing

### 4.2 Features Not to be Tested

- **`buildSnapshot` / `LabelToKind`** (`resource_registry.go`): Registry remains a faithful mirror of API discovery; no changes needed
- **Static `resourceCandidates` fallback**: Deprecated path; unchanged behavior
- **Owner resolution pipeline**: Not affected by this change; covered by existing #1029 tests

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Denylist in `extractTargetResource()` not `buildSnapshot()` | Registry should mirror K8s API discovery; Prometheus-specific semantics belong in the adapter |
| Package-level `var` not ConfigMap | Prometheus-reserved labels are specification constants; they don't change per deployment |
| 5 labels: `job`, `service`, `instance`, `endpoint`, `container` | Per issue #1045 specification; covers all known Prometheus scrape metadata labels |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `extractTargetResource` dynamic path (pure logic: label filtering, candidate building, tier sorting)
- **Integration**: Deferred — existing integration tests in `test/integration/gateway/` cover the Parse→Fingerprint pipeline; denylist is pure logic
- **E2E**: Deferred to separate issue — requires mock Prometheus alerts with `job` labels in Kind cluster

### 5.2 Two-Tier Minimum

- **Unit tests**: 12 scenarios covering denylist enforcement, non-reserved label passthrough, adversarial inputs, and edge cases
- **Integration tests**: Covered by existing `adapters_integration_test.go`; no new integration tests needed (denylist is pure label-key filtering with no I/O)

### 5.3 Business Outcome Quality Bar

Each test validates: "When a Prometheus alert with reserved metadata labels arrives, the Gateway correctly identifies the actual Kubernetes resource being affected, not the monitoring infrastructure."

### 5.4 Pass/Fail Criteria

**PASS** — all of the following must be true:
1. All 12 P0 unit tests pass (0 failures)
2. All existing Gateway unit/integration tests pass (0 regressions)
3. Per-tier code coverage >=80% on `extractTargetResource`
4. Build succeeds with `go build ./...`

**FAIL** — any of the following:
1. Any P0 test fails
2. Existing tests regress
3. Denylist allows any of the 5 reserved labels through

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken; discovery tests fail on main
**Resume**: Build fixed; main is green

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/gateway/adapters/prometheus_adapter.go` | `extractTargetResource` (dynamic path) | ~55 |

### 6.2 Integration-Testable Code

No new integration-testable code. Denylist is pure label-key filtering (map lookup).

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `origin/main` HEAD | `24806d36b` |
| Dependency: #1029 | Merged | Dynamic API resource registry |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-GATEWAY-184 | `job` label excluded from dynamic resolution | P0 | Unit | UT-GW-1045-001 | Pending |
| BR-GATEWAY-184 | `service` label excluded from dynamic resolution | P0 | Unit | UT-GW-1045-002 | Pending |
| BR-GATEWAY-184 | `instance` label excluded from dynamic resolution | P0 | Unit | UT-GW-1045-003 | Pending |
| BR-GATEWAY-184 | `endpoint` label excluded from dynamic resolution | P0 | Unit | UT-GW-1045-004 | Pending |
| BR-GATEWAY-184 | `container` label excluded from dynamic resolution | P0 | Unit | UT-GW-1045-005 | Pending |
| BR-GATEWAY-184 | Non-reserved labels resolve correctly | P0 | Unit | UT-GW-1045-006 | Pending |
| BR-GATEWAY-184 | Reserved + non-reserved: non-reserved wins | P0 | Unit | UT-GW-1045-007 | Pending |
| BR-GATEWAY-184 | All 5 reserved + `deployment`: Deployment resolves | P0 | Unit | UT-GW-1045-008 | Pending |
| BR-GATEWAY-184 | Only reserved labels → Unknown/unknown | P0 | Unit | UT-GW-1045-009 | Pending |
| BR-GATEWAY-184 | Adversarial: empty/unicode/path-traversal values | P0 | Unit | UT-GW-1045-010 | Pending |
| BR-GATEWAY-184 | Adversarial: casing variations on reserved keys | P0 | Unit | UT-GW-1045-011 | Pending |
| BR-GATEWAY-184 | `PrometheusReservedLabels` contains exactly 5 entries | P0 | Unit | UT-GW-1045-012 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-GW-1045-{SEQUENCE}` (Unit), `IT-GW-1045-{SEQUENCE}` (Integration)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/gateway/adapters/prometheus_adapter.go` → `extractTargetResource` dynamic path (>=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-GW-1045-001` | `job` label (scrape target) does not resolve as K8s Job | Pending |
| `UT-GW-1045-002` | `service` label (ServiceMonitor target) does not resolve as K8s Service | Pending |
| `UT-GW-1045-003` | `instance` label (scrape endpoint) does not generate a candidate | Pending |
| `UT-GW-1045-004` | `endpoint` label (port name) does not generate a candidate | Pending |
| `UT-GW-1045-005` | `container` label does not generate a candidate | Pending |
| `UT-GW-1045-006` | `deployment` label still resolves to Deployment after denylist | Pending |
| `UT-GW-1045-007` | `job` + `poddisruptionbudget`: PDB resolves, not Job | Pending |
| `UT-GW-1045-008` | All 5 reserved + `deployment`: Deployment resolves correctly | Pending |
| `UT-GW-1045-009` | Only reserved labels → resolves to Unknown/unknown | Pending |
| `UT-GW-1045-010` | Adversarial reserved label values: empty, unicode, path traversal | Pending |
| `UT-GW-1045-011` | Reserved key casing: `Job`, `SERVICE`, `Instance` don't bypass | Pending |
| `UT-GW-1045-012` | `PrometheusReservedLabels` exported var contains exactly 5 entries | Pending |

### Tier Skip Rationale

- **Integration**: Denylist is pure label-key filtering (map lookup, no I/O). Existing integration tests cover Parse→Fingerprint pipeline. Adding integration tests would not improve confidence.
- **E2E**: Requires mock Prometheus alerts with `job` labels in Kind cluster. Deferred to follow-up issue.

---

## 9. Test Cases

### UT-GW-1045-001: `job` label excluded from dynamic resolution

**BR**: BR-GATEWAY-184
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/prometheus_denylist_test.go`

**Preconditions**:
- `APIResourceRegistry` constructed with `standardResources()` (includes `batch/v1 Job`)
- `PrometheusAdapter` constructed with registry (dynamic path)

**Test Steps**:
1. **Given**: Alert payload with `job: kube-state-metrics`, `pod: worker-abc`, `namespace: production`
2. **When**: `adapter.Parse(ctx, payload)` is called
3. **Then**: Signal resolves to `Pod/worker-abc`, NOT `Job/kube-state-metrics`

**Expected Results**:
1. `signal.Resource.Kind == "Pod"`
2. `signal.Resource.Name == "worker-abc"`

### UT-GW-1045-007: `job` + `poddisruptionbudget` — PDB resolves

**BR**: BR-GATEWAY-184
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/prometheus_denylist_test.go`

**Preconditions**:
- Registry with standard resources (PDB + Job)
- Dynamic path adapter

**Test Steps**:
1. **Given**: Alert with `job: kube-state-metrics`, `poddisruptionbudget: demo-pdb`, `namespace: production`
2. **When**: `adapter.Parse(ctx, payload)` is called
3. **Then**: Signal resolves to `PodDisruptionBudget/demo-pdb`, NOT `Job/kube-state-metrics`

**Expected Results**:
1. `signal.Resource.Kind == "PodDisruptionBudget"`
2. `signal.Resource.Name == "demo-pdb"`

### UT-GW-1045-008: All 5 reserved + deployment — Deployment resolves

**BR**: BR-GATEWAY-184
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/prometheus_denylist_test.go`

**Test Steps**:
1. **Given**: Alert with all 5 reserved labels + `deployment: api-server`, `namespace: production`
2. **When**: `adapter.Parse(ctx, payload)` is called
3. **Then**: Signal resolves to `Deployment/api-server`

### UT-GW-1045-009: Only reserved labels → Unknown

**BR**: BR-GATEWAY-184
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/prometheus_denylist_test.go`

**Test Steps**:
1. **Given**: Alert with ONLY `job: kube-state-metrics`, `service: kube-prometheus`, `namespace: monitoring`
2. **When**: `adapter.Parse(ctx, payload)` is called
3. **Then**: Signal resolves to `Unknown/unknown`

### UT-GW-1045-012: Denylist variable contains exactly 5 entries

**BR**: BR-GATEWAY-184
**Priority**: P0
**Type**: Unit
**File**: `test/unit/gateway/adapters/prometheus_denylist_test.go`

**Test Steps**:
1. **Given**: `adapters.PrometheusReservedLabels` is accessible
2. **When**: Length and keys are inspected
3. **Then**: Contains exactly `{"job", "service", "instance", "endpoint", "container"}`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Fake discovery client (`fakediscovery.FakeDiscovery`) — external dependency
- **Location**: `test/unit/gateway/adapters/`
- **Race detector**: `-race` flag mandatory

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.25.6 | Build and test |
| Ginkgo CLI | v2.x | Test runner |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #1029 | Code | Merged | Dynamic registry not available | N/A |

### 11.2 Execution Order

1. **TDD RED**: Write all 12 unit tests (failing against current implementation)
2. **Checkpoint 1**: 9-category audit
3. **TDD GREEN**: Add denylist filter in `extractTargetResource`
4. **Checkpoint 2**: 9-category audit
5. **TDD REFACTOR**: Code quality + 100 Go Mistakes validation
6. **Checkpoint 3**: Final 9-category audit

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/1045/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/gateway/adapters/prometheus_denylist_test.go` | Ginkgo BDD tests |
| Coverage report | CI artifact | Per-tier coverage percentages |

---

## 13. Execution

```bash
# Unit tests (denylist only)
go test ./test/unit/gateway/adapters/... -ginkgo.focus="1045" -race -v

# Full Gateway unit tests (regression check)
make test-unit-gateway

# Coverage
go test ./test/unit/gateway/adapters/... -coverprofile=coverage.out -coverpkg=./pkg/gateway/adapters/...
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| None | — | — | Denylist is applied in `extractTargetResource` dynamic path; existing tests use static path or don't use reserved labels as primary candidates |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-06 | Initial test plan |
