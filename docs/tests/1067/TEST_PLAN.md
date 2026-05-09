# Test Plan: Gateway Fingerprint Divergence on Enrichment Fallback (#1067)

## 1. Introduction

### 1.1 Purpose

Validate that the `namespace` Prometheus alert label is excluded from `extractTargetResource` candidate scoring, preventing Gateway fingerprint divergence when pods are replaced during remediation.

### 1.2 Scope

- Add `namespace` to `PrometheusReservedLabels` denylist
- Verify `namespace` label is not used as a Kubernetes resource candidate
- Verify existing denylist entries remain functional
- Verify non-reserved labels (e.g., `deployment`, `pod`) are unaffected

### 1.3 References

- Issue: [#1067](https://github.com/jordigilh/kubernaut/issues/1067)
- Related: [#1045](https://github.com/jordigilh/kubernaut/issues/1045) (original reserved label denylist)
- Business Requirement: BR-GATEWAY-004 (Cross-adapter deduplication)
- Business Requirement: BR-GATEWAY-069 (Deduplication tracking)
- Testing Guidelines: `docs/development/business-requirements/TESTING_GUIDELINES.md`

---

## 2. Test Environment

### 2.1 Framework

- Ginkgo/Gomega BDD
- Fake discovery client (`fakediscovery.FakeDiscovery`)
- `APIResourceRegistry` with standard + Namespace resources

### 2.2 Test Location

- `test/unit/gateway/adapters/prometheus_denylist_test.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Mitigation |
|----|------|--------|-------------|------------|
| R1 | Alert genuinely targeting a Namespace resource | Wrong target | Low | K8s event adapter handles namespace events; Prometheus `namespace` label is always metadata |
| R2 | `exported_namespace` accidentally excluded | Namespace override broken | None | `exported_namespace` has no API resource singular match; handled by `extractNamespace` |
| R3 | Existing tests break from count change | Build failure | Medium | Update `UT-GW-1045-012` from 5 to 6 entries |

---

## 4. Test Scenarios

### 4.1 Unit Tests

| ID | Scenario | Input | Expected | Tier |
|----|----------|-------|----------|------|
| UT-GW-1067-001 | `namespace` label excluded from candidates | Labels: `namespace=demo-alert-storm`, `pod=crashing-pod` + discovery includes Namespace | Resource: Pod/crashing-pod (not Namespace) | Unit |
| UT-GW-1067-002 | `namespace`-only labels resolve to Unknown | Labels: `namespace=production`, `alertname=Test` + discovery includes Namespace | Resource: Unknown/unknown | Unit |
| UT-GW-1067-003 | `namespace` + `deployment` labels — Deployment wins | Labels: `namespace=production`, `deployment=api-server` | Resource: Deployment/api-server | Unit |
| UT-GW-1067-004 | `exported_namespace` not in denylist | Labels: `exported_namespace=prod`, `pod=worker` | Pod resolves (exported_namespace not matched by LabelToKind) | Unit |
| UT-GW-1045-012 | API surface: reserved labels count updated | N/A | `PrometheusReservedLabels` has 6 entries including `namespace` | Unit |

### 4.2 Integration Tests

| ID | Scenario | Input | Expected | Tier |
|----|----------|-------|----------|------|
| IT-GW-1067-001 | Full pipeline: namespace label excluded with production-realistic discovery | Labels: `namespace=<ns>`, `pod=crashing-pod-abc` + registry includes Namespace | Resource: Pod/crashing-pod-abc; fingerprint based on Pod, not Namespace; CRD target is Pod | Integration |
| IT-GW-1067-002 | Deployment wins over excluded namespace label | Labels: `namespace=<ns>`, `deployment=api-server` + registry includes Namespace | Resource: Deployment/api-server; fingerprint based on Deployment | Integration |

---

## 5. TDD Execution Plan

### Phase 1: RED

- Write UT-GW-1067-001 through UT-GW-1067-004
- Extend `standardResources()` with Namespace API resource in test helper
- UT-GW-1067-001 should FAIL (namespace label resolves as Namespace kind)
- UT-GW-1045-012 count assertion should FAIL (expects 6, currently 5)

### Phase 2: GREEN

- Add `"namespace": true` to `PrometheusReservedLabels`
- Update UT-GW-1045-012 count from 5 to 6
- All tests pass

### Phase 3: REFACTOR

- Review doc comments on `PrometheusReservedLabels`
- 100 Go Mistakes validation
- Lint check

---

## 6. Traceability Matrix

| Test ID | Business Requirement | Issue |
|---------|---------------------|-------|
| UT-GW-1067-001 | BR-GATEWAY-004 | #1067 |
| UT-GW-1067-002 | BR-GATEWAY-004 | #1067 |
| UT-GW-1067-003 | BR-GATEWAY-069 | #1067 |
| UT-GW-1067-004 | BR-GATEWAY-004 | #1067 |
| UT-GW-1045-012 | BR-GATEWAY-184 | #1045, #1067 |
| IT-GW-1067-001 | BR-GATEWAY-004 | #1067 |
| IT-GW-1067-002 | BR-GATEWAY-004 | #1067 |

---

## 7. Status

| Test ID | Status |
|---------|--------|
| UT-GW-1067-001 | Pass |
| UT-GW-1067-002 | Pass |
| UT-GW-1067-003 | Pass |
| UT-GW-1067-004 | Pass |
| UT-GW-1045-012 | Pass |
| IT-GW-1067-001 | Pass (compile-verified) |
| IT-GW-1067-002 | Pass (compile-verified) |
