# Test Plan: Propagate apiVersion Through Remediation Target Pipeline

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1040-v1
**Feature**: Propagate `apiVersion` from LLM remediation target through the pipeline to EA resource fetching, resolving ambiguous kind resolution on OCP with Knative
**Version**: 1.0
**Created**: 2026-05-05
**Author**: AI Agent (supervised)
**Status**: Approved
**Branch**: `fix/1040`

---

## 1. Introduction

### 1.1 Purpose

Validate that the `api_version` field is correctly parsed from LLM responses, propagated through all target resource types (AIAnalysis, RR, EA CRDs), and used by the EA controller for unambiguous GVR resolution. Confirms backwards compatibility when `api_version` is absent.

### 1.2 Objectives

1. **Parser extraction**: `api_version` from LLM JSON is captured in `RemediationTarget`
2. **Type propagation**: `APIVersion` flows through AIAnalysis → RR → EA without loss
3. **GVR resolution**: EA uses `apiVersion` directly instead of `ResolveGVKForKind` when present
4. **Enrichment disambiguation**: `K8sAdapter` uses `apiVersion` for initial resource resolution
5. **Owner chain capture**: `OwnerChainEntry` retains `APIVersion` from `OwnerReference`
6. **InjectRemediationTarget**: `APIVersion` is preserved through target injection logic
7. **Backwards compatibility**: Absent `api_version` falls back to existing kind-only resolution
8. **Validation**: Invalid `apiVersion` from LLM is rejected with warning, falls back gracefully

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test --race` on affected packages |
| Integration test pass rate | 100% | `go test` on integration packages |
| Backward compatibility | 0 regressions | All existing tests pass |
| CRD generation | Clean | `make generate && make manifests` succeeds |

---

## 2. References

### 2.1 Authority

- Issue #1040: EffectivenessMonitor: ambiguous Route kind resolution on OCP with Knative
- Issue #1029: Gateway Dynamic Owner Resolution for OpenShift CRDs (related pattern)
- Production incident: `route-misconfiguration` scenario on OCP with Knative installed

### 2.2 Cross-References

- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | K8sClient interface change breaks all fakes | Build failure in tests | High | All enrichment/investigator tests | Update all fakes to accept `apiVersion` parameter |
| R2 | LLM hallucinates wrong apiVersion | Wrong resource fetched by EA | Medium | UT-KA-1040-008 | Validate against REST mapper; fall back on failure |
| R3 | CRD schema change not regenerated | Runtime field ignored | Low | Build check | `make generate && make manifests` in GREEN phase |
| R4 | InjectRemediationTarget loses apiVersion during override | EA receives empty apiVersion | Medium | UT-KA-1040-005/006 | Explicit propagation logic with tests |
| R5 | Backwards incompatibility for absent apiVersion | Existing investigations break | Low | UT-KA-1040-007 | All code paths have `""` fallback |

---

## 4. Scope

### 4.1 Features to be Tested

- **Parser** (`internal/kubernautagent/parser/`): `api_version` extraction from LLM JSON
- **Type propagation** (`pkg/kubernautagent/types/`, `api/*/v1alpha1/`): `APIVersion` field on all target types
- **ResolveGVKWithAPIVersion** (`pkg/shared/k8s/gvk.go`): New helper for apiVersion-aware GVR resolution
- **InjectRemediationTarget** (`internal/kubernautagent/investigator/`): APIVersion propagation through injection
- **EA consumer** (`internal/controller/effectivenessmonitor/`): apiVersion-aware resource fetching
- **K8sAdapter** (`internal/kubernautagent/enrichment/`): apiVersion-aware initial resolution and owner chain capture
- **Serialization** (`pkg/aianalysis/handlers/`): `api_version` deserialization from RCA map
- **RO propagation** (`internal/controller/remediationorchestrator/`): APIVersion in WFE/EA creation

### 4.2 Features Not to be Tested

- **Signal context apiVersion**: Gateway-to-agent path; separate issue scope
- **Route CRD E2E in Kind**: Requires Knative + OCP CRDs; deferred to OCP-specific test suite

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of changed logic (parser, helper, injection, resolution)
- **Integration**: >=80% of I/O paths (EA resource fetch, enrichment)

### 5.2 Pass/Fail Criteria

**PASS**: All tests pass, `make generate && make manifests` clean, 0 regressions
**FAIL**: Any P0 test fails, CRD regeneration fails, existing tests regress

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-EA-1040 | Parser extracts api_version from LLM JSON | P0 | Unit | UT-KA-1040-001 | Pending |
| BR-EA-1040 | Parser handles missing api_version (backwards compat) | P0 | Unit | UT-KA-1040-002 | Pending |
| BR-EA-1040 | ResolveGVKWithAPIVersion uses apiVersion when present | P0 | Unit | UT-KA-1040-003 | Pending |
| BR-EA-1040 | ResolveGVKWithAPIVersion falls back when apiVersion empty | P0 | Unit | UT-KA-1040-004 | Pending |
| BR-EA-1040 | InjectRemediationTarget preserves apiVersion (same kind) | P0 | Unit | UT-KA-1040-005 | Pending |
| BR-EA-1040 | InjectRemediationTarget preserves apiVersion (cross-type) | P0 | Unit | UT-KA-1040-006 | Pending |
| BR-EA-1040 | InjectRemediationTarget with empty apiVersion (compat) | P0 | Unit | UT-KA-1040-007 | Pending |
| BR-EA-1040 | ResolveGVKWithAPIVersion rejects invalid apiVersion | P1 | Unit | UT-KA-1040-008 | Pending |
| BR-EA-1040 | OwnerChainEntry captures APIVersion from OwnerReference | P0 | Unit | UT-KA-1040-009 | Pending |
| BR-EA-1040 | K8sAdapter uses apiVersion for initial resolution | P0 | Unit | UT-KA-1040-010 | Pending |
| BR-EA-1040 | ExtractRootCauseAnalysis reads api_version | P0 | Unit | UT-KA-1040-011 | Pending |
| BR-EA-1040 | EA getTargetFunctionalState uses apiVersion | P0 | Integration | IT-KA-1040-001 | Pending |
| BR-EA-1040 | Mock LLM emits api_version in remediation_target | P1 | Unit | UT-KA-1040-012 | Pending |
| BR-EA-1040 | injectTargetResourceParameters includes API_VERSION | P1 | Unit | UT-KA-1040-013 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-1040-001 | LLM JSON with api_version is parsed into RemediationTarget.APIVersion | Pending |
| UT-KA-1040-002 | LLM JSON without api_version preserves empty APIVersion (backwards compat) | Pending |
| UT-KA-1040-003 | ResolveGVKWithAPIVersion("Route", "route.openshift.io/v1") returns correct GVK | Pending |
| UT-KA-1040-004 | ResolveGVKWithAPIVersion("Deployment", "") falls back to static table | Pending |
| UT-KA-1040-005 | InjectRemediationTarget preserves LLM apiVersion when kind matches root | Pending |
| UT-KA-1040-006 | InjectRemediationTarget preserves LLM apiVersion for cross-type target | Pending |
| UT-KA-1040-007 | InjectRemediationTarget with empty apiVersion works (backwards compat) | Pending |
| UT-KA-1040-008 | ResolveGVKWithAPIVersion rejects malformed apiVersion gracefully | Pending |
| UT-KA-1040-009 | K8sAdapter.GetOwnerChain captures APIVersion in OwnerChainEntry | Pending |
| UT-KA-1040-010 | K8sAdapter uses apiVersion parameter for initial resolveMapping | Pending |
| UT-KA-1040-011 | ExtractRootCauseAnalysis captures api_version from RCA JSON map | Pending |
| UT-KA-1040-012 | Mock LLM analysisJSON includes api_version when config has APIVersion | Pending |
| UT-KA-1040-013 | injectTargetResourceParameters includes TARGET_RESOURCE_API_VERSION | Pending |

---

## 9. Test Cases

### UT-KA-1040-001: Parser extracts api_version

**Priority**: P0
**File**: `test/unit/kubernautagent/parser/parser_apiversion_test.go`

**Test Steps**:
1. **Given**: JSON string `{"root_cause_analysis":{"summary":"...","remediation_target":{"kind":"Route","name":"storefront","namespace":"demo-route","api_version":"route.openshift.io/v1"}}}`
2. **When**: `Parse(jsonStr, logger)` is called
3. **Then**: `result.RemediationTarget.APIVersion` equals `"route.openshift.io/v1"`

### UT-KA-1040-003: ResolveGVKWithAPIVersion with apiVersion

**Priority**: P0
**File**: `test/unit/shared/k8s/gvk_apiversion_test.go`

**Test Steps**:
1. **Given**: REST mapper with `route.openshift.io/v1/Route` registered
2. **When**: `ResolveGVKWithAPIVersion(mapper, "Route", "route.openshift.io/v1")`
3. **Then**: Returns GVK `{Group: "route.openshift.io", Version: "v1", Kind: "Route"}`

### UT-KA-1040-005: InjectRemediationTarget preserves apiVersion (same kind)

**Priority**: P0
**File**: `test/unit/kubernautagent/investigator/investigator_phases_apiversion_test.go`

**Test Steps**:
1. **Given**: LLM result with `RemediationTarget{Kind: "Route", Name: "storefront", Namespace: "demo-route", APIVersion: "route.openshift.io/v1"}`; enrichment with empty owner chain but `ResourceKind: "Route"`
2. **When**: `InjectRemediationTarget(result, signal, enrichData)`
3. **Then**: `result.RemediationTarget.APIVersion` equals `"route.openshift.io/v1"` (preserved because kind matches)

---

## 10. Environmental Needs

### 10.1 Unit Tests
- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `fakeK8sClient` with updated interface, `fake.NewSimpleClientset` for REST mapper tests
- **Location**: `test/unit/kubernautagent/`, `test/unit/shared/k8s/`

### 10.2 Integration Tests
- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest for REST mapper with real CRDs
- **Location**: `test/integration/`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1 (TDD RED)**: Write failing tests for parser, GVK helper, InjectRemediationTarget, EA resolution
2. **Phase 2 (TDD GREEN L1-3)**: LLM schema + prompt + parser + internal types + CRD types + make generate/manifests
3. **Phase 3 (TDD GREEN L4-7)**: Serialization, RO propagation, InjectRemediationTarget, EA consumer
4. **Phase 4 (TDD GREEN L8-10)**: Enrichment, validation helper, mock LLM
5. **Phase 5 (TDD REFACTOR)**: 100-go-mistakes review, 9-category checkpoint audit

---

## 12. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `enrichment_test.go` fakeK8sClient | `GetOwnerChain(_, _, _, _)` | Add `apiVersion` parameter | K8sClient interface change |
| `enricher_retry_test.go` countingK8sClient | `GetOwnerChain(_, _, _, _)` | Add `apiVersion` parameter | K8sClient interface change |
| `enricher_not_found_test.go` fakeK8sClient | `GetOwnerChain(_, _, _, _)` | Add `apiVersion` parameter | K8sClient interface change |
| `investigator_test.go` fakeK8sClient | `GetOwnerChain(_, _, _, _)` | Add `apiVersion` parameter | K8sClient interface change |
| `resource_context_test.go` | May use K8sClient mock | Add `apiVersion` parameter | K8sClient interface change |

---

## 13. Due Diligence Findings

| ID | Finding | Severity | Resolution |
|----|---------|----------|------------|
| DD-1 | K8sClient interface change breaks 4 fake implementations + ~15 test call sites | P0 | Update all fakes; add `_` parameter for apiVersion |
| DD-2 | `resolveMapping(kind)` uses heuristic plural — ambiguous for multi-group kinds | P0 | New `resolveMappingWithAPIVersion` path when apiVersion non-empty |
| DD-3 | `OwnerReference.APIVersion` is already parsed in `resolveOwnerMapping` but discarded | P0 | Capture in `OwnerChainEntry.APIVersion` |
| DD-4 | `resource_context.go` tool calls `GetOwnerChain`/`GetSpecHash` without apiVersion context | Info | Pass `""` — tool works on arbitrary resources from LLM tool calls |
| DD-5 | Mock LLM `MockScenarioConfig.APIVersion` field exists but unused in JSON builders | P1 | Wire into `analysisJSON` when non-empty |
| DD-6 | CRD regeneration required after type changes | P0 | `make generate && make manifests` in GREEN phase |

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-05 | Initial test plan |
