# Test Plan: KA Remediation Target Resolution Fix

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-693-v1
**Feature**: Fix KA remediation target resolution when owner chain is empty after re-enrichment
**Version**: 1.0
**Created**: 2026-04-10
**Author**: AI Assistant
**Status**: Active
**Branch**: `release/v1.3.0-rc3`

---

## 1. Introduction

### 1.1 Purpose

This test plan ensures that the KA remediation target resolution bug (#693) and two related findings (#694, #695, #696) are fixed correctly with full behavioral assurance across unit and integration tiers.

### 1.2 Objectives

1. **F1 (#694)**: `injectRemediationTarget` uses enrichment source identity (not signal) as root when owner chain is empty — 4 test scenarios
2. **F2 (#695)**: Workflow selection prompt and injection both use the post-RCA resource identity — 3 test scenarios
3. **F3 (#696)**: KA owner chain walks `controller: true` refs only, aligned with GW/SP — 2 new + 4 updated scenarios
4. **Regression**: All existing KA unit and integration tests continue to pass

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/enrichment/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on modified files |
| Backward compatibility | 0 regressions | Existing tests pass without modification to assertions |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #693: v1.3.0-rc2: KA sets remediationTarget to ReplicaSet name instead of Deployment name
- Issue #694: injectRemediationTarget loses root owner after re-enrichment
- Issue #695: Workflow selection prompt shows original Pod name instead of re-enriched target
- Issue #696: KA owner chain uses owners[0] instead of controller:true
- ADR-056: Re-enrichment using RCA-identified remediation target

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Empty owner chain after re-enrichment causes wrong remediation target | WFE job fails, all deployment-targeting workflows broken | High | UT-KA-693-001 | F1 fix: enrichment source identity as root fallback |
| R2 | LLM hallucinates deployment name from Pod name in prompt | Increased probability of wrong target | Medium | UT-KA-693-005 | F2 fix: prompt shows re-enriched target |
| R3 | Non-controller ownerRef followed instead of controller | Divergent chain vs GW/SP | Medium | UT-KA-693-008/009 | F3 fix: controller:true selection |
| R4 | JSON serialization of EnrichmentResult changes | Existing tests break | Low | UT-KA-433-028/132 | Use `omitempty` tags on new fields |
| R5 | Integration test fixtures missing controller:true | Tests fail after F3 change | High | IT-KA-433-ENR-* | Update fixtures in Phase 7 |

---

## 4. Scope

### 4.1 Features to be Tested

- **injectRemediationTarget** (`internal/kubernautagent/investigator/investigator.go:781-807`): Root owner resolution with empty chain, enrichment source identity, cross-type preservation
- **EnrichmentResult identity fields** (`internal/kubernautagent/enrichment/enricher.go:123-128`): New ResourceKind/Name/Namespace fields populated by Enrich()
- **Workflow signal override** (`internal/kubernautagent/investigator/investigator.go:139-177`): Post-RCA signal identity for prompt and injection consistency
- **GetOwnerChain controller selection** (`internal/kubernautagent/enrichment/k8s_adapter.go:69-99`): Controller-ref-only traversal

### 4.2 Features Not to be Tested

- **Prompt template rendering**: Template logic is unchanged; only input data changes
- **Parser validation**: Mitigated by F1 (#698 closed)
- **allLabelDetectionsFailed nil behavior**: By design (#697 closed)
- **E2E tests**: Real K8s controllers always set controller:true; E2E is behavior-preserving

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Export `InjectRemediationTarget` for testing | Function is currently unexported; tests in `_test` package need access |
| Use `ptr.To(true)` from `k8s.io/utils/ptr` | Standard K8s helper, already in go.mod |
| Align with SP `getControllerOwner` pattern | Consistency across subsystems; SP and GW both use controller:true |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of modified functions in `investigator.go`, `enricher.go`, `k8s_adapter.go`
- **Integration**: >=80% of enrichment pipeline with real envtest
- **E2E**: Not in scope (real controllers set controller:true; behavior-preserving)

### 5.2 Two-Tier Minimum

Every fix is covered by at least unit tests. F3 is additionally covered by integration tests (envtest fixtures).

### 5.3 Pass/Fail Criteria

**PASS**: All P0 tests pass, per-tier coverage >=80%, no regressions.
**FAIL**: Any P0 test fails, coverage below 80%, or existing tests break.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/investigator/investigator.go` | `injectRemediationTarget` | ~25 |
| `internal/kubernautagent/enrichment/enricher.go` | `EnrichmentResult` struct, `Enrich()` init | ~5 |

### 6.2 Integration-Testable Code (I/O, wiring)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/enrichment/k8s_adapter.go` | `GetOwnerChain` | ~35 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-KA-693 | Remediation target resolves to correct Deployment name | P0 | Unit | UT-KA-693-001 | Pending |
| BR-KA-693 | Non-empty owner chain still uses last entry | P0 | Unit | UT-KA-693-002 | Pending |
| BR-KA-693 | Nil enrichData falls back to signal | P0 | Unit | UT-KA-693-003 | Pending |
| BR-KA-693 | Enrich() populates resource identity on result | P0 | Unit | UT-KA-693-004 | Pending |
| BR-KA-695 | Post-RCA signal identity in workflow prompt | P0 | Unit | UT-KA-693-005 | Pending |
| BR-KA-695 | No re-enrichment leaves signal unchanged | P0 | Unit | UT-KA-693-006 | Pending |
| BR-KA-695 | Injection receives same signal as prompt | P0 | Unit | UT-KA-693-007 | Pending |
| BR-KA-696 | Controller-only ownerRef traversal | P0 | Unit | UT-KA-693-008 | Pending |
| BR-KA-696 | No controller ref yields empty chain | P0 | Unit | UT-KA-693-009 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-693-001` | Empty chain + enrichment source `Deployment/worker` -> target is `{Deployment, worker}`, not `{Pod, worker-77784c6cf7-l27g4}` | Pending |
| `UT-KA-693-002` | Chain `[RS, Deployment]` -> target uses `Deployment` from chain (existing behavior) | Pending |
| `UT-KA-693-003` | Nil enrichData -> target falls back to signal identity | Pending |
| `UT-KA-693-004` | `Enrich("Deployment", "worker", "ns", ...)` -> `result.ResourceKind == "Deployment"` | Pending |
| `UT-KA-693-005` | After re-enrichment, workflow signal shows `Deployment/worker` | Pending |
| `UT-KA-693-006` | No re-enrichment (same target) -> signal unchanged | Pending |
| `UT-KA-693-007` | Same modified signal used for both prompt and injection | Pending |
| `UT-KA-693-008` | Pod with 2 ownerRefs (non-controller RS, controller RS) -> follows controller | Pending |
| `UT-KA-693-009` | Pod with ownerRef but no controller:true -> empty chain | Pending |

---

## 9. Test Cases

### UT-KA-693-001: Empty chain uses enrichment source as root

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/investigator/inject_target_test.go`

**Test Steps**:
1. **Given**: `enrichData` with empty `OwnerChain` but `ResourceKind="Deployment"`, `ResourceName="worker"`, `ResourceNamespace="demo-crashloop"`; signal is `Pod/worker-77784c6cf7-l27g4`; LLM result has `RemediationTarget.Kind="Deployment"`, `Name="worker-77784c6cf7"` (hallucinated RS name)
2. **When**: `InjectRemediationTarget(result, signal, enrichData)` is called
3. **Then**: `result.RemediationTarget` is `{Deployment, worker, demo-crashloop}` (enrichment source, not LLM hallucination)

### UT-KA-693-004: Enrich populates resource identity

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/enrichment/enrichment_test.go`

**Test Steps**:
1. **Given**: Enricher with mock K8s client
2. **When**: `Enrich(ctx, "Deployment", "worker", "demo-crashloop", "", "inc-1")`
3. **Then**: `result.ResourceKind == "Deployment"`, `result.ResourceName == "worker"`, `result.ResourceNamespace == "demo-crashloop"`

### UT-KA-693-008: Controller-only ownerRef traversal

**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/enrichment/k8s_adapter_test.go`

**Test Steps**:
1. **Given**: Pod with 2 ownerRefs: `{Kind: "ReplicaSet", Name: "other-rs", Controller: nil}`, `{Kind: "ReplicaSet", Name: "real-rs", Controller: ptr.To(true)}`
2. **When**: `GetOwnerChain(ctx, "Pod", "test-pod", "default")`
3. **Then**: Chain follows `real-rs`, not `other-rs`

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `fakeK8sClient` for enrichment
- **Location**: `test/unit/kubernautagent/investigator/`, `test/unit/kubernautagent/enrichment/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest (K8s API server)
- **Location**: `test/integration/kubernautagent/enrichment/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| k8s.io/utils | latest | `ptr.To(true)` helper |

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1-3**: F1 (enrichment source identity) — foundational fix
2. **Phase 4-6**: F2 (signal override) — builds on F1
3. **Phase 7-9**: F3 (controller:true) — independent but benefits from F1
4. **Phase 10**: Final validation

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/693/TEST_PLAN.md` | Strategy and test design |
| Unit test suite (F1/F2) | `test/unit/kubernautagent/investigator/inject_target_test.go` | Injection and signal override tests |
| Unit test suite (F1) | `test/unit/kubernautagent/enrichment/enrichment_test.go` | EnrichmentResult identity tests |
| Unit test suite (F3) | `test/unit/kubernautagent/enrichment/k8s_adapter_test.go` | Controller selection tests |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/kubernautagent/investigator/... -ginkgo.v
go test ./test/unit/kubernautagent/enrichment/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/enrichment/... -ginkgo.v

# Specific test by ID
go test ./test/unit/kubernautagent/investigator/... -ginkgo.focus="UT-KA-693"
go test ./test/unit/kubernautagent/enrichment/... -ginkgo.focus="UT-KA-693"

# Coverage
go test ./test/unit/kubernautagent/investigator/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-10 | Initial test plan for #693, #694, #695, #696 |
