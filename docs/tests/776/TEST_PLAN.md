# Test Plan: Full DD-HAPI-018 Parity for KA Label Detector

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-776-v1
**Feature**: Achieve 100% HAPI v1.2.1 parity for the KA Go label detector across all 8 DD-HAPI-018 detections
**Version**: 1.0
**Created**: 2026-04-21
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/776-label-detector-hapi-parity`

---

## 1. Introduction

### 1.1 Purpose

Validate that the KA Go label detector (`internal/kubernautagent/enrichment/label_detector.go`) produces detection results identical to HAPI v1.2.1 (`holmesgpt-api/src/detection/labels.py`) for all 8 DD-HAPI-018 detection categories. This test plan covers the 5 detections being fixed (GitOps, ServiceMesh, HPA, Stateful, ResourceQuota) and regression-guards the 3 already at parity (PDB, Helm, NetworkPolicy).

### 1.2 Objectives

1. **GitOps parity**: All 10 DD-HAPI-018 priority checks fire in correct precedence order with namespace and pod template sources
2. **ServiceMesh parity**: Status annotation keys (`sidecar.istio.io/status`, `linkerd.io/proxy-version`) detected from pod template with legacy fallback
3. **HPA parity**: `scaleTargetRef` matched against full owner chain, not just root owner
4. **Stateful parity**: Owner chain iterated for StatefulSet, not just root kind check
5. **ResourceQuota parity**: Quota summary (`QuotaResourceUsage` per resource) populated and wired through enricher to prompt
6. **Zero regressions**: All existing tests pass without modification (except signature updates)

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/enrichment/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/investigator/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `label_detector.go` |
| Backward compatibility | 0 regressions | Existing `detected_labels_679_test.go` + `detected_labels_test.go` pass |
| DD-HAPI-018 conformance | 100% | All DL-HP-* and DL-MX-* vectors pass |

---

## 2. References

### 2.1 Authority

- DD-HAPI-018 v1.5: Detected Labels Detection Specification
- BR-HAPI-264: Post-RCA label detection via EnrichmentService
- BR-HAPI-265: Labels in workflow discovery context
- Issue #776: Label detector fails to set gitOpsManaged=true despite tracking-id presence

### 2.2 Cross-References

- HAPI v1.2.1 reference: `holmesgpt-api/src/detection/labels.py`
- HAPI v1.2.1 context builder: `holmesgpt-api/src/toolsets/resource_context.py`
- ADR-056: DetectedLabels relocated from SP to HAPI/KA

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Legacy gitOps keys removed | GitOps regression | Low | UT-KA-776-001 | Kept as priorities 11-13 |
| R2 | Namespace GET latency/RBAC | Enrichment slowdown | Medium | UT-KA-776-009..011 | Graceful skip on failure |
| R3 | Pod template vs real Pod | Annotation mismatch | Low | UT-KA-776-012..013 | HAPI uses same approach |
| R4 | Cluster-scoped NS GET | Crash | High | UT-KA-776-011 | Skip when namespace="" |
| R5 | dynamicfake with Namespace | Test failures | Medium | All NS tests | Validated by UT-KA-433-541 |
| R6 | ServiceMesh key change | Detection regression | Low | UT-KA-776-014..015 | Legacy keys as fallback |
| R7 | DetectLabels signature change | Compilation failures | Medium | All callers | Mechanical update |

---

## 4. Scope

### 4.1 Features to be Tested

- **GitOps Detection** (`label_detector.go:detectGitOps`): 10-priority cascade with namespace + pod template sources
- **ServiceMesh Detection** (`label_detector.go:detectServiceMesh`): Status keys from pod template + legacy fallback
- **HPA Detection** (`label_detector.go:detectHPA`): Full owner chain matching
- **Stateful Detection** (`label_detector.go:detectStateful`): Full owner chain iteration
- **ResourceQuota Detection** (`label_detector.go:detectResourceQuota`): Quota summary aggregation
- **Pipeline Wiring** (`enricher.go`, `investigator.go`, `builder.go`): QuotaDetails flow to LLM prompt

### 4.2 Features Not to be Tested

- **PDB Detection**: Already at parity — regression-guarded by existing tests
- **Helm Detection**: Already at parity (KA superior) — regression-guarded
- **NetworkPolicy Detection**: Already at parity — regression-guarded

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `label_detector.go` (detection logic, helpers)
- **Integration**: >=80% of enricher-to-prompt pipeline for quota wiring

### 5.2 Pass/Fail Criteria

**PASS**: All P0 tests pass, all existing tests pass, per-tier coverage >=80%.
**FAIL**: Any P0 test fails, any existing test regresses, coverage below 80%.

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| DD-HAPI-018 Det 1 | ArgoCD v3 tracking-id on root owner | P0 | Unit | UT-KA-776-001 | Pending |
| DD-HAPI-018 Det 1 | ArgoCD v3 tracking-id on deployment annotations | P0 | Unit | UT-KA-776-002 | Pending |
| DD-HAPI-018 Det 1 | ArgoCD v3 tracking-id on namespace annotations | P0 | Unit | UT-KA-776-003 | Pending |
| DD-HAPI-018 Det 1 | ArgoCD v3+v2 coexistence (tracking-id wins) | P0 | Unit | UT-KA-776-004 | Pending |
| DD-HAPI-018 Det 1 | Mutual exclusivity DL-MX-01 | P0 | Unit | UT-KA-776-005 | Pending |
| DD-HAPI-018 Det 1 | Mutual exclusivity DL-MX-02 | P0 | Unit | UT-KA-776-006 | Pending |
| DD-HAPI-018 Det 1 | Mutual exclusivity DL-MX-03 | P0 | Unit | UT-KA-776-007 | Pending |
| DD-HAPI-018 Det 1 | Mutual exclusivity DL-MX-04 | P0 | Unit | UT-KA-776-008 | Pending |
| DD-HAPI-018 Det 1 | Namespace annotation managed -> argocd | P0 | Unit | UT-KA-776-009 | Pending |
| DD-HAPI-018 Det 1 | Namespace annotation sync-status -> flux | P0 | Unit | UT-KA-776-010 | Pending |
| DD-HAPI-018 Det 1 | Cluster-scoped: NS check gracefully skipped | P0 | Unit | UT-KA-776-011 | Pending |
| DD-HAPI-018 Det 7 | Istio status annotation on pod template | P0 | Unit | UT-KA-776-012 | Pending |
| DD-HAPI-018 Det 7 | Linkerd proxy-version on pod template | P0 | Unit | UT-KA-776-013 | Pending |
| DD-HAPI-018 Det 7 | Istio inject legacy fallback | P1 | Unit | UT-KA-776-014 | Pending |
| DD-HAPI-018 Det 7 | Linkerd inject legacy fallback | P1 | Unit | UT-KA-776-015 | Pending |
| DD-HAPI-018 Det 3 | HPA targets root owner (existing behavior) | P0 | Unit | UT-KA-776-016 | Pending |
| DD-HAPI-018 Det 3 | HPA targets intermediate owner in chain | P0 | Unit | UT-KA-776-017 | Pending |
| DD-HAPI-018 Det 3 | HPA targets StatefulSet root | P1 | Unit | UT-KA-776-018 | Pending |
| DD-HAPI-018 Det 4 | Stateful via owner chain (root is SS) | P0 | Unit | UT-KA-776-019 | Pending |
| DD-HAPI-018 Det 4 | Stateful via chain iteration (SS in chain) | P1 | Unit | UT-KA-776-020 | Pending |
| DD-HAPI-018 Det 8 | Single RQ with hard/used summary | P0 | Unit | UT-KA-776-021 | Pending |
| DD-HAPI-018 Det 8 | Multiple RQs, first-wins per key | P0 | Unit | UT-KA-776-022 | Pending |
| DD-HAPI-018 Det 8 | RQ with no status fields | P1 | Unit | UT-KA-776-023 | Pending |
| DD-HAPI-018 Det 8 | No RQs -> nil summary | P0 | Unit | UT-KA-776-024 | Pending |
| DD-HAPI-018 Det 8 | RQ API error -> failedDetections | P0 | Unit | UT-KA-776-025 | Pending |
| BR-HAPI-265 | tracking-id through investigator flow | P0 | Integration | IT-KA-776-001 | Pending |
| BR-HAPI-265 | NS tracking-id through investigator | P1 | Integration | IT-KA-776-002 | Pending |
| BR-HAPI-265 | ServiceMesh status through investigator | P1 | Integration | IT-KA-776-003 | Pending |
| BR-HAPI-265 | QuotaDetails in EnrichmentResult | P0 | Integration | IT-KA-776-004 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**File**: `test/unit/kubernautagent/enrichment/detected_labels_776_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-776-001 | ArgoCD v3 tracking-id on root owner annotations -> gitOpsManaged=true, gitOpsTool=argocd | Pending |
| UT-KA-776-002 | ArgoCD v3 tracking-id on deployment (root) annotations -> gitOpsManaged=true | Pending |
| UT-KA-776-003 | ArgoCD v3 tracking-id on namespace annotations -> gitOpsManaged=true | Pending |
| UT-KA-776-004 | tracking-id annotation + instance label coexist -> tracking-id wins (argocd) | Pending |
| UT-KA-776-005 | Pod tracking-id (v3) + Deploy flux label -> argocd wins (DL-MX-01) | Pending |
| UT-KA-776-006 | Pod instance (v2) + Deploy flux label -> argocd wins (DL-MX-02) | Pending |
| UT-KA-776-007 | Deploy flux label + NS argocd label -> flux wins (DL-MX-03) | Pending |
| UT-KA-776-008 | Pod v3+v2 + Deploy v3+v2 -> argocd (DL-MX-04) | Pending |
| UT-KA-776-009 | Namespace annotation argocd.argoproj.io/managed -> gitOpsManaged=true | Pending |
| UT-KA-776-010 | Namespace annotation fluxcd.io/sync-status -> gitOpsTool=flux | Pending |
| UT-KA-776-011 | Cluster-scoped resource (Node) with tracking-id -> detected, NS check skipped | Pending |
| UT-KA-776-012 | Pod template annotation sidecar.istio.io/status -> serviceMesh=istio | Pending |
| UT-KA-776-013 | Pod template annotation linkerd.io/proxy-version -> serviceMesh=linkerd | Pending |
| UT-KA-776-014 | Root owner annotation sidecar.istio.io/inject=true -> serviceMesh=istio (legacy) | Pending |
| UT-KA-776-015 | Root owner annotation linkerd.io/inject=enabled -> serviceMesh=linkerd (legacy) | Pending |
| UT-KA-776-016 | HPA targets Deployment (root owner) -> hpaEnabled=true | Pending |
| UT-KA-776-017 | HPA targets ReplicaSet (intermediate owner) -> hpaEnabled=true | Pending |
| UT-KA-776-018 | HPA targets StatefulSet (root) -> hpaEnabled=true | Pending |
| UT-KA-776-019 | Owner chain [{Pod}, {StatefulSet}] -> stateful=true | Pending |
| UT-KA-776-020 | Owner chain [{Pod}, {RS}, {StatefulSet}] -> stateful=true | Pending |
| UT-KA-776-021 | Single RQ: cpu hard=4 used=2, memory hard=8Gi used=4Gi -> summary populated | Pending |
| UT-KA-776-022 | Two RQs with overlapping keys -> first-wins per resource | Pending |
| UT-KA-776-023 | RQ with empty status -> constrained=true, empty summary | Pending |
| UT-KA-776-024 | No RQs -> constrained=false, nil summary | Pending |
| UT-KA-776-025 | RQ API error -> failedDetections, nil summary | Pending |

### Tier 2: Integration Tests

**File**: `test/integration/kubernautagent/investigator/detected_labels_it_test.go`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-KA-776-001 | tracking-id on deployment produces gitOpsManaged in investigation result | Pending |
| IT-KA-776-002 | NS-level tracking-id detected when root owner lacks it | Pending |
| IT-KA-776-003 | sidecar.istio.io/status on pod template -> serviceMesh=istio in result | Pending |
| IT-KA-776-004 | QuotaDetails populated in EnrichmentResult when RQs exist | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — label detection is internal to KA enrichment and does not require full cluster validation. Unit + integration provide sufficient coverage for DD-HAPI-018 conformance.

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `dynamicfake.NewSimpleDynamicClient` for K8s API
- **Location**: `test/unit/kubernautagent/enrichment/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: Enricher with real LabelDetector, fake K8s client
- **Location**: `test/integration/kubernautagent/investigator/`

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/776/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/enrichment/detected_labels_776_test.go` | DD-HAPI-018 parity tests |
| Integration test suite | `test/integration/kubernautagent/investigator/detected_labels_it_test.go` | Pipeline integration tests |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/enrichment/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/investigator/... -ginkgo.v -ginkgo.focus="776"

# Specific conformance vector
go test ./test/unit/kubernautagent/enrichment/... -ginkgo.focus="UT-KA-776"

# Coverage
go test ./test/unit/kubernautagent/enrichment/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `detected_labels_679_test.go` `newTestMapper()` | No Namespace kind | Add Namespace as RESTScopeRoot | Needed for NS fetch in tests |
| `detected_labels_679_test.go` all `DetectLabels` calls | 2 return values | 3 return values (add quota summary) | Signature change |
| `enrichment_test.go` UT-KA-433-132 | `QuotaDetails map[string]string` | `QuotaDetails map[string]QuotaResourceUsage` | Type change |
| `detected_labels_test.go` all `DetectLabels` calls | 2 return values | 3 return values | Signature change |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-21 | Initial test plan — 25 unit tests, 4 integration tests |
