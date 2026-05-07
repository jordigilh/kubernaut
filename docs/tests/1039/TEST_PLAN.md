# Test Plan: Soften Enrichment for Deleted Resources

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1039-v1
**Feature**: Exempt K8s NotFound errors from enrichment HardFail so deleted-resource scenarios proceed to workflow discovery
**Version**: 1.0
**Created**: 2026-05-05
**Author**: AI Agent (supervised)
**Status**: Approved
**Branch**: `fix/1039`

---

## 1. Introduction

### 1.1 Purpose

Validate that the enrichment pipeline treats K8s `NotFound` errors as non-fatal, allowing investigations targeting deleted resources to proceed to workflow selection instead of aborting with `rca_incomplete`. Confirmed by production audit trace on OCP cluster: `ClusterServiceVersion/etcdoperator.v0.9.4` was deleted mid-investigation, causing enrichment HardFail and pipeline abort despite the LLM producing a valid RCA.

### 1.2 Objectives

1. **NotFound exemption**: `IsNotFoundError` correctly detects wrapped K8s NotFound errors and `HardFail` is not set for them
2. **Retry skip**: `resolveOwnerChainWithRetry` does not retry when the initial error is NotFound
3. **Audit trail**: `EnrichmentResult.TargetResourceDeleted` is set to `true` when the resource is deleted, and a warning is surfaced in the investigation response
4. **Regression safety**: Existing `rca_incomplete` behavior is preserved for genuine API errors (timeout, internal server error)
5. **E2E validation**: Full pipeline proceeds to workflow selection in a Kind cluster when the remediation target does not exist

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/kubernautagent/enrichment/... --race` |
| Integration test pass rate | 100% | `go test ./test/integration/kubernautagent/...` |
| E2E test pass rate | 100% | `make test-e2e-kubernautagent` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on enricher.go, errors.go |
| Backward compatibility | 0 regressions | Existing tests pass (IT-KA-704-001 updated) |

---

## 2. References

### 2.1 Authority

- Issue #1039: Agent: add existence validation gate for RCA remediation target
- BR-HAPI-261 AC#7: Enrichment hard-failure triggers rca_incomplete
- Production incident: `OperatorCSVFailed` on `etcdoperator.v0.9.4` in `demo-operator` (OCP audit 2026-05-06)

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../development/business-requirements/TESTING_GUIDELINES.md)
- Issue #704: Original HardFail implementation

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `IsNotFoundError` false positive on non-NotFound StatusError | Genuine API errors silently bypass HardFail | Low | UT-KA-1039-007 | `errors.As` with `StatusReasonNotFound` check; explicit false-positive test |
| R2 | Existing E2E-KA-433-ADV-016 breaks (expects rca_incomplete for NotFound) | E2E regression | High | E2E-KA-1039-001 | Replace with inverted assertions |
| R3 | IT-KA-704-001 breaks (uses NotFound to test rca_incomplete) | Integration regression | High | IT-KA-704-001 | Change error type to InternalError |
| R4 | Mock LLM multi-turn fails after pipeline continuation | E2E test flake | Low | E2E-KA-1039-001 | Preflight verified: ThreeStepDAG returns workflow |
| R5 | Label detection on deleted resource causes nil panic | Runtime crash | Low | UT-KA-1039-001 | Preflight verified: DetectLabels fails gracefully |

---

## 4. Scope

### 4.1 Features to be Tested

- **IsNotFoundError helper** (`internal/kubernautagent/enrichment/errors.go`): Robust detection of wrapped K8s NotFound errors
- **HardFail exemption** (`internal/kubernautagent/enrichment/enricher.go`): NotFound excluded from HardFail alongside NoMatchError
- **TargetResourceDeleted field** (`internal/kubernautagent/enrichment/enricher.go`): New boolean on EnrichmentResult
- **Retry skip** (`internal/kubernautagent/enrichment/enricher.go`): NotFound exits retry loop immediately
- **Warning surface** (`internal/kubernautagent/investigator/investigator.go`): TargetResourceDeleted produces a warning in InvestigationResult
- **E2E pipeline** (`test/e2e/kubernautagent/`): Full investigation with deleted resource proceeds to workflow selection

### 4.2 Features Not to be Tested

- **Workflow catalog matching for deleted resources**: Catalog query uses signal/RCA fields, not enrichment — separate concern
- **Spec hash calculation**: Empty spec hash for deleted resources is correct and handled by existing code
- **DataStorage bad request on empty spec_hash**: Secondary failure, non-blocking, separate issue

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Use `errors.As` with `*apierrors.StatusError` instead of `apierrors.IsNotFound` | Handles wrapped errors from `K8sAdapter.GetOwnerChain` which uses `fmt.Errorf("k8s adapter: ... %w")` |
| Add `TargetResourceDeleted` to `EnrichmentResult` not API schema | Internal field for audit/observability; surfaced via existing `Warnings` array |
| Update IT-KA-704-001 error type rather than delete it | Preserves rca_incomplete regression coverage for genuine API errors |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of `enricher.go` changes + `errors.go` (new)
- **Integration**: >=80% of enrichment I/O paths (envtest)
- **E2E**: 2 scenarios validating full pipeline behavior in Kind cluster

### 5.2 Pass/Fail Criteria

**PASS**: All 11 tests pass, existing suites show 0 regressions, per-tier coverage >=80%

**FAIL**: Any P0 test fails, existing IT-KA-704-001 or E2E suites regress

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/enrichment/errors.go` (new) | `IsNotFoundError` | ~15 |
| `internal/kubernautagent/enrichment/enricher.go` | `Enrich` (HardFail condition), `resolveOwnerChainWithRetry` (early exit) | ~10 changed |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/kubernautagent/enrichment/enricher.go` | `Enrich` with real K8s adapter (envtest) | ~50 |
| `internal/kubernautagent/investigator/investigator.go` | `Investigate` warning injection | ~5 changed |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-HAPI-261 | NotFound exempt from HardFail | P0 | Unit | UT-KA-1039-001 | Pending |
| BR-HAPI-261 | TargetResourceDeleted set on NotFound | P0 | Unit | UT-KA-1039-002 | Pending |
| BR-HAPI-261 | NotFound skips retries | P0 | Unit | UT-KA-1039-003 | Pending |
| BR-HAPI-261 | Other errors still HardFail | P0 | Unit | UT-KA-1039-004 | Pending |
| BR-HAPI-261 | NoMatchError still exempt | P1 | Unit | UT-KA-1039-005 | Pending |
| BR-HAPI-261 | IsNotFoundError detects wrapped error | P0 | Unit | UT-KA-1039-006 | Pending |
| BR-HAPI-261 | IsNotFoundError rejects non-NotFound | P0 | Unit | UT-KA-1039-007 | Pending |
| BR-HAPI-261 | Enrichment pipeline continues on NotFound | P0 | Integration | IT-KA-1039-001 | Pending |
| BR-HAPI-261 | Deleted resource proceeds to workflow selection | P0 | E2E | E2E-KA-1039-001 | Pending |
| BR-HAPI-261 | Deleted resource warning in response | P1 | E2E | E2E-KA-1039-002 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-KA-1039-001 | NotFound error -> HardFail=false, pipeline does not abort | Pending |
| UT-KA-1039-002 | NotFound error -> TargetResourceDeleted=true for audit trail | Pending |
| UT-KA-1039-003 | NotFound error -> no retries attempted (single call only) | Pending |
| UT-KA-1039-004 | InternalError -> HardFail=true, existing behavior preserved | Pending |
| UT-KA-1039-005 | NoMatchError -> HardFail=false, existing behavior preserved | Pending |
| UT-KA-1039-006 | IsNotFoundError detects NotFound wrapped by K8sAdapter | Pending |
| UT-KA-1039-007 | IsNotFoundError returns false for InternalError, Forbidden | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-KA-1039-001 | Enrichment with deleted resource (envtest) completes with TargetResourceDeleted=true | Pending |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| E2E-KA-1039-001 | Deleted resource investigation proceeds to workflow selection (full pipeline) | Pending |
| E2E-KA-1039-002 | Deleted resource warning surfaced in investigation response | Pending |

---

## 9. Test Cases

### UT-KA-1039-001: NotFound does not trigger HardFail

**BR**: BR-HAPI-261
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/enricher_not_found_test.go`

**Test Steps**:
1. **Given**: fakeK8sClient returns `apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "deleted-pod")`; enricher configured with MaxRetries=3
2. **When**: `Enrich(ctx, "Pod", "deleted-pod", "production", "", "inc-001")` is called
3. **Then**: `result.HardFail` is `false`; `result.OwnerChainError` is not nil

### UT-KA-1039-002: NotFound sets TargetResourceDeleted

**BR**: BR-HAPI-261
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/enricher_not_found_test.go`

**Test Steps**:
1. **Given**: fakeK8sClient returns NotFound error; enricher with MaxRetries=3
2. **When**: `Enrich` is called
3. **Then**: `result.TargetResourceDeleted` is `true`

### UT-KA-1039-003: NotFound skips retries

**BR**: BR-HAPI-261
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/enricher_not_found_test.go`

**Test Steps**:
1. **Given**: countingK8sClient returns NotFound on every call; enricher with MaxRetries=3
2. **When**: `Enrich` is called
3. **Then**: `k8s.CallCount()` is `1` (no retries)

### UT-KA-1039-004: Non-NotFound errors still HardFail

**BR**: BR-HAPI-261
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/enricher_not_found_test.go`

**Test Steps**:
1. **Given**: fakeK8sClient returns `apierrors.NewInternalError(fmt.Errorf("etcd timeout"))`; enricher with MaxRetries=3
2. **When**: `Enrich` is called
3. **Then**: `result.HardFail` is `true`; `result.TargetResourceDeleted` is `false`

### UT-KA-1039-005: NoMatchError still exempt from HardFail

**BR**: BR-HAPI-261
**Priority**: P1
**File**: `test/unit/kubernautagent/enrichment/enricher_not_found_test.go`

**Test Steps**:
1. **Given**: fakeK8sClient returns `meta.NoResourceMatchError`; enricher with MaxRetries=3
2. **When**: `Enrich` is called
3. **Then**: `result.HardFail` is `false`; `result.TargetResourceDeleted` is `false`

### UT-KA-1039-006: IsNotFoundError detects wrapped NotFound

**BR**: BR-HAPI-261
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/enricher_not_found_test.go`

**Test Steps**:
1. **Given**: `fmt.Errorf("k8s adapter: get Pod/deleted-pod in production: %w", apierrors.NewNotFound(...))`
2. **When**: `IsNotFoundError(err)` is called
3. **Then**: returns `true`

### UT-KA-1039-007: IsNotFoundError rejects non-NotFound

**BR**: BR-HAPI-261
**Priority**: P0
**File**: `test/unit/kubernautagent/enrichment/enricher_not_found_test.go`

**Test Steps**:
1. **Given**: `apierrors.NewForbidden(...)`, `apierrors.NewInternalError(...)`, `fmt.Errorf("generic error")`
2. **When**: `IsNotFoundError(err)` is called for each
3. **Then**: returns `false` for all

### IT-KA-1039-001: Enrichment pipeline continues on deleted resource

**BR**: BR-HAPI-261
**Priority**: P0
**File**: `test/integration/kubernautagent/enrichment/enrichment_real_ds_test.go`

**Test Steps**:
1. **Given**: envtest cluster with enrichment fixtures; target resource `nonexistent-deploy` does NOT exist
2. **When**: `enricher.Enrich(ctx, "Deployment", "nonexistent-deploy", "it-enrichment", "", "it-1039")` is called
3. **Then**: `result.TargetResourceDeleted` is `true`; `result.HardFail` is `false`; `result.OwnerChain` is empty

### E2E-KA-1039-001: Deleted resource proceeds to workflow selection

**BR**: BR-HAPI-261
**Priority**: P0
**File**: `test/e2e/kubernautagent/adversarial_parity_e2e_test.go`

**Test Steps**:
1. **Given**: Kind cluster with mock LLM; `unreachable-pod` does NOT exist in `production` namespace
2. **When**: `sessionClient.Investigate(ctx, buildRequest("1039-001", "mock_rca_incomplete", "critical"))`
3. **Then**: Investigation succeeds; `NeedsHumanReview` is NOT true for `rca_incomplete`; `SelectedWorkflow.Set` is true with `workflow_id`; `Confidence` >= 0.5

### E2E-KA-1039-002: Deleted resource warning in response

**BR**: BR-HAPI-261
**Priority**: P1
**File**: `test/e2e/kubernautagent/adversarial_parity_e2e_test.go`

**Test Steps**:
1. **Given**: Same as E2E-KA-1039-001
2. **When**: Investigation completes
3. **Then**: `Warnings` contains at least one entry with "deleted" substring

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `fakeK8sClient`, `fakeDataStorageClient`, `countingK8sClient`, `recordingAuditStore` (existing test helpers)
- **Location**: `test/unit/kubernautagent/enrichment/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest + real DataStorage (PostgreSQL, Redis)
- **Location**: `test/integration/kubernautagent/enrichment/`

### 10.3 E2E Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: Kind cluster, mock LLM, DataStorage, seeded workflow catalog
- **Location**: `test/e2e/kubernautagent/`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1 (TDD RED)**: Write all failing tests (unit + integration + E2E + IT-KA-704-001 update)
2. **Phase 2 (TDD GREEN)**: Implement `IsNotFoundError`, HardFail exemption, `TargetResourceDeleted`, retry skip, warning surface
3. **Phase 3 (TDD REFACTOR)**: 100-go-mistakes review, 9-category checkpoint audit

---

## 12. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| E2E-KA-433-ADV-016 (`adversarial_parity_e2e_test.go:311`) | `NeedsHumanReview=true`, `HumanReviewReason="rca_incomplete"` | Replace with E2E-KA-1039-001: `NeedsHumanReview` not rca_incomplete, `SelectedWorkflow.Set=true` | NotFound no longer triggers HardFail |
| IT-KA-704-001 (`investigator_test.go:738`) | `apierrors.NewNotFound(...)` -> `rca_incomplete` | Change error to `apierrors.NewInternalError(...)` | NotFound is now exempt; test needs non-exempt error to verify rca_incomplete |

---

## 13. Due Diligence Findings

| ID | Finding | Severity | Resolution |
|----|---------|----------|------------|
| DD-1 | Production error wraps NotFound via `fmt.Errorf("k8s adapter: get %s/%s in %s: %w")` | P0 | `errors.As` with `*apierrors.StatusError` handles wrapping; UT-KA-1039-006 validates |
| DD-2 | `GetSpecHash` also fails for deleted resources (empty spec_hash) | Info | Acceptable: empty spec hash → no remediation history match. Not a regression. |
| DD-3 | `ds adapter: bad request` on empty spec_hash | Info | Secondary failure in DataStorage adapter, non-blocking. Separate issue. |
| DD-4 | `allLabelDetectionsFailed` gracefully handles deleted resource enrichment | Info | Preflight verified: all categories marked failed → initial enrichment preserved |
| DD-5 | IT-KA-704-001 uses NotFound to test rca_incomplete | P0 | Must update error type to InternalError to preserve rca_incomplete coverage |
| DD-6 | Mock LLM `rcaIncompleteConfig` has WorkflowID set, multi-turn works after HardFail removal | Info | Preflight verified: ThreeStepDAG returns `submit_result_with_workflow` |

---

## 14. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-05 | Initial test plan with E2E scenarios, production trace validation |
