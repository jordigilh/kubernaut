# Test Plan: Phase 1 Tool specHash Fix (#729)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-729-v1.0
**Feature**: Fix `get_namespaced_resource_context` and `get_cluster_resource_context` to compute specHash before calling DS remediation history API
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Agent
**Status**: Draft
**Branch**: `fix/722-em-false-positive-remediated`

---

## 1. Introduction

### 1.1 Purpose

The Phase 1 LLM tools `get_namespaced_resource_context` and `get_cluster_resource_context` pass an empty `specHash` to `GetRemediationHistory`, causing DS to return HTTP 400. The error is silently swallowed, and the LLM receives empty remediation history during Phase 1 investigation. This test plan validates the fix: computing specHash via `K8sClient.GetSpecHash()` before calling DS, and logging errors instead of discarding them.

### 1.2 Objectives

1. **specHash computed**: Both tools call `GetSpecHash` with the resolved rootOwner coordinates before `GetRemediationHistory`
2. **Error visibility**: `GetOwnerChain` and `GetRemediationHistory` failures are logged at warning level
3. **Graceful degradation**: If `GetSpecHash` fails, the tool continues with empty hash (matching `Enricher.Enrich` behavior)
4. **No response shape change**: Tool JSON output format remains unchanged

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on `resource_context.go` |
| Backward compatibility | 0 regressions | Existing tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- ADR-055: LLM-driven context enrichment — tool contract for `get_namespaced_resource_context`
- DD-EM-002: Canonical spec hash — algorithm and usage
- Issue #729: `get_namespaced_resource_context` passes empty specHash to DS

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Reference implementation: `internal/kubernautagent/enrichment/enricher.go` (Enrich method)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | specHash computed for wrong resource (input vs rootOwner) | History miss: DS returns wrong/empty results | Medium | UT-KA-729-001, UT-KA-729-003 | Test verifies hash is computed for rootOwner, not input resource |
| R2 | GetSpecHash failure breaks tool entirely | LLM gets no tool result, investigation degrades | Low | UT-KA-729-004 | Test verifies graceful degradation with empty hash on failure |
| R3 | get_cluster_resource_context not fixed (same bug) | Cluster-scoped tools still broken | High | UT-KA-729-005 | Explicit test for cluster tool |
| R4 | Additional K8s API call impacts latency | Phase 1 investigation slower | Low | N/A (design) | Sequential tool calls — one additional Get is acceptable |
| R5 | Cluster tool struct lacks K8sClient field | BLOCKER: cannot call GetSpecHash on cluster tool | High | UT-KA-729-005 | Add k8s field to clusterResourceContextTool + update constructor + RegisterAll |
| R6 | No logger on tool structs | Errors invisible to operators | Medium | UT-KA-729-006 | Add `*slog.Logger` to tool structs + update constructors + RegisterAll |
| R7 | Existing tests create cluster tool without k8sClient | Compile failure after signature change | High | All existing cluster tests | Update test fakes + constructor calls |

---

## 4. Scope

### 4.1 Features to be Tested

- **`namespacedResourceContextTool.Execute`** (`pkg/kubernautagent/tools/custom/resource_context.go`): specHash computation with rootOwner coordinates, error logging
- **`clusterResourceContextTool.Execute`** (`pkg/kubernautagent/tools/custom/resource_context.go`): same fix for cluster-scoped variant

### 4.2 Features Not to be Tested

- **DS adapter 400 handling** (`ds_adapter.go`): Separate concern — DS silent 400→empty is pre-existing behavior, not changed here
- **Enricher.Enrich**: Reference implementation, already tested, not modified

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Hash rootOwner, not input resource | DS history is keyed by (kind, name, namespace) and specHash must match the same triple |
| Log warnings, don't fail | Matches Enricher.Enrich behavior — partial failure is acceptable |
| Fix both namespaced and cluster tools | Same bug in both; cluster tool passes empty namespace for hash |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `resource_context.go` Execute methods
- **Integration**: Deferred — tool integration is tested via E2E investigator tests
- **E2E**: Covered by existing KA E2E test plan

### 5.2 Pass/Fail Criteria

**PASS**: All unit tests pass, specHash is computed for rootOwner, errors are logged, response shape unchanged.

**FAIL**: Any test fails, specHash still empty, or tool errors on GetSpecHash failure.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/kubernautagent/tools/custom/resource_context.go` | `namespacedResourceContextTool.Execute`, `clusterResourceContextTool.Execute` | ~80 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| ADR-055 | Tool computes specHash before DS call | P0 | Unit | UT-KA-729-001 | Pending |
| ADR-055 | specHash uses rootOwner (not input resource) | P0 | Unit | UT-KA-729-002 | Pending |
| ADR-055 | Cluster tool computes specHash | P0 | Unit | UT-KA-729-005 | Pending |
| DD-EM-002 | Hash matches DD-EM-002 canonical format | P1 | Unit | UT-KA-729-003 | Pending |
| #729 | GetSpecHash failure degrades gracefully | P0 | Unit | UT-KA-729-004 | Pending |
| #729 | GetRemediationHistory error is logged | P1 | Unit | UT-KA-729-006 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `pkg/kubernautagent/tools/custom/resource_context.go` — >=80% coverage

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-729-001` | Namespaced tool passes computed specHash to GetRemediationHistory | Pending |
| `UT-KA-729-002` | specHash is computed for rootOwner (after owner chain resolution), not the input resource | Pending |
| `UT-KA-729-003` | When owner chain is empty, specHash is computed for the input resource itself | Pending |
| `UT-KA-729-004` | When GetSpecHash fails, tool continues with empty hash and returns remediation history | Pending |
| `UT-KA-729-005` | Cluster tool passes computed specHash to GetRemediationHistory | Pending |
| `UT-KA-729-006` | GetRemediationHistory error is visible (not silently swallowed) | Pending |

### Tier Skip Rationale

- **Integration**: Tool wiring tested via `cmd/kubernautagent/main.go` → `RegisterAll`; tool behavior is pure logic with mocked interfaces
- **E2E**: Covered by existing KA E2E investigator tests

---

## 9. Test Cases

### UT-KA-729-001: Namespaced tool computes specHash

**BR**: ADR-055
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/resource_context_test.go`

**Test Steps**:
1. **Given**: Mock K8sClient returning owner chain `[Pod → ReplicaSet → Deployment]` and specHash `"sha256:abc123"` for the Deployment
2. **When**: `Execute(ctx, {"kind":"Pod","name":"web-0","namespace":"default"})`
3. **Then**: Mock DS receives `GetRemediationHistory("Deployment", "web", "default", "sha256:abc123")`

### UT-KA-729-002: specHash uses rootOwner coordinates

**BR**: ADR-055
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/resource_context_test.go`

**Test Steps**:
1. **Given**: Mock K8sClient with owner chain resolving Pod→Deployment. GetSpecHash mock records calls.
2. **When**: `Execute(ctx, {"kind":"Pod","name":"web-0","namespace":"default"})`
3. **Then**: GetSpecHash called with `("Deployment", "web", "default")`, NOT `("Pod", "web-0", "default")`

### UT-KA-729-003: Empty owner chain hashes input resource

**BR**: DD-EM-002
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/resource_context_test.go`

**Test Steps**:
1. **Given**: Mock K8sClient returning empty owner chain. GetSpecHash returns `"sha256:def456"`.
2. **When**: `Execute(ctx, {"kind":"Deployment","name":"api","namespace":"prod"})`
3. **Then**: GetSpecHash called with `("Deployment", "api", "prod")`; DS receives specHash `"sha256:def456"`

### UT-KA-729-004: GetSpecHash failure degrades gracefully

**BR**: #729
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/resource_context_test.go`

**Test Steps**:
1. **Given**: Mock K8sClient where GetSpecHash returns error
2. **When**: `Execute(ctx, {"kind":"Deployment","name":"api","namespace":"prod"})`
3. **Then**: No error returned; DS receives empty specHash `""`; response contains `remediation_history` (empty or populated)

### UT-KA-729-005: Cluster tool computes specHash

**BR**: ADR-055
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/resource_context_test.go`

**Test Steps**:
1. **Given**: Mock K8sClient returning specHash `"sha256:cluster1"` for cluster-scoped resource
2. **When**: `Execute(ctx, {"kind":"Node","name":"worker-1"})`
3. **Then**: GetSpecHash called with `("Node", "worker-1", "")`; DS receives specHash `"sha256:cluster1"`

### UT-KA-729-006: GetRemediationHistory error is logged

**BR**: #729
**Priority**: P1
**Type**: Unit
**File**: `test/unit/kubernautagent/tools/resource_context_test.go`

**Test Steps**:
1. **Given**: Mock DS where GetRemediationHistory returns error
2. **When**: `Execute(ctx, {"kind":"Deployment","name":"api","namespace":"prod"})`
3. **Then**: Error is logged (not silently discarded); tool returns valid JSON with empty remediation_history

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `enrichment.K8sClient` and `enrichment.DataStorageClient` interfaces (external dependencies)
- **Location**: `test/unit/kubernautagent/tools/`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **TDD RED**: Write all 6 tests — they fail because `GetSpecHash` is never called
2. **TDD GREEN**: Add `GetSpecHash` call before `GetRemediationHistory` in both tools; log errors
3. **TDD REFACTOR**: Extract shared logic if namespaced/cluster tools share hash pattern

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/729/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/kubernautagent/tools/resource_context_test.go` | Ginkgo BDD test file |

---

## 13. Execution

```bash
go test ./test/unit/kubernautagent/tools/... -ginkgo.v
go test ./test/unit/kubernautagent/tools/... -ginkgo.focus="UT-KA-729"
go test ./test/unit/kubernautagent/tools/... -coverprofile=coverage.out
```

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
