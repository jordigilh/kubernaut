# Test Plan: SA Propagation Simplification — Resolve at WE Execution Time

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-650-v1
**Feature**: Remove SA from AA/RO/WFE-spec propagation chain; resolve from DS catalog at WE execution time into WFE status
**Version**: 1.0
**Created**: 2026-04-06
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.2`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates Issue #650: simplifying ServiceAccountName (SA) propagation
by removing it from the upstream chain (AIAnalysis SelectedWorkflow, RO creator,
WorkflowExecution spec) and instead having the WE controller resolve it at runtime
from the Data Storage catalog into `WorkflowExecution.Status.ServiceAccountName`.
Additionally, the four individual `WorkflowQuerier` methods that each call
`GetWorkflowByID` are consolidated into a single `ResolveWorkflowCatalogMetadata`
method, and the duplicate `BuildPipelineRun`/`PipelineRunName` in the reconciler
are removed in favor of the executor's canonical implementations.

### 1.2 Objectives

1. **SA resolved from DS at runtime**: WE controller fetches SA from the DS catalog via
   consolidated `ResolveWorkflowCatalogMetadata` and writes it to `wfe.Status.ServiceAccountName`.
2. **SA removed from upstream types**: `SelectedWorkflow.ServiceAccountName` (AA type) and
   `WorkflowExecutionSpec.ServiceAccountName` are deleted; RO creator and response_processor
   stop propagating SA.
3. **Executors read from status**: All three executors (Tekton, Job, Ansible) read
   `wfe.Status.ServiceAccountName` instead of `wfe.Spec.ServiceAccountName`.
4. **Consolidated DS call**: Single `GetWorkflowByID` call replaces four individual calls;
   old querier methods removed.
5. **Deduplication**: Reconciler's `BuildPipelineRun`, `PipelineRunName`, and `ConvertParameters`
   removed; `HandleAlreadyExists` refactored to accept `resourceName string`.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/workflowexecution/... ./test/unit/remediationorchestrator/... ./test/unit/aianalysis/...` |
| Integration test pass rate | 100% | `go test ./test/integration/workflowexecution/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| Regressions | 0 | All existing tests pass (beyond planned migration) |
| No Spec.ServiceAccountName refs | 0 | `rg 'Spec\.ServiceAccountName' --glob '*.go'` returns 0 non-comment hits |

---

## 2. References

### 2.1 Authority (governing documents)

- DD-WE-005 v2.0: Per-workflow ServiceAccount reference
- DD-WE-006: WFE queries DS on demand using the workflow ID
- Issue #650: Simplify SA propagation
- Issue #518: Runtime engine resolution (established pattern)
- Issue #501: SA schema migration to spec top-level (predecessor)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | DS catalog entry has no SA set | Executor receives empty SA, uses K8s default | Low | UT-WE-650-002 | Empty SA is the intended fallback; tested explicitly |
| R2 | Status write timing: SA not persisted before exec.Create | Executor reads stale empty SA | Low | UT-WE-650-001 | SA set in-memory before exec.Create (same pattern as ExecutionEngine) |
| R3 | Existing WFEs in etcd lose spec.serviceAccountName on CRD regen | Silent data loss for in-flight WFEs | Medium | (Operational) | WE controller re-resolves SA from DS; spec field was informational not authoritative |
| R4 | Ansible executor reads SA in 8+ locations | Missed reference causes fallback regression | Medium | UT-WE-650-005 | Exhaustive grep; all 8 locations mapped in preflight |
| R5 | HandleAlreadyExists refactor breaks race condition handling | PipelineRun collision not detected | Low | UT-WE-650-011 | Only `pr.Name` was used; resourceName string is equivalent |

### 3.1 Risk-to-Test Traceability

- **R1** (empty SA fallback): UT-WE-650-002 explicitly tests empty OptString from DS
- **R2** (status timing): UT-WE-650-001 verifies SA in status before exec.Create
- **R4** (Ansible 8 refs): UT-WE-650-005 covers tokenRequest path with status-sourced SA
- **R5** (HandleAlreadyExists): UT-WE-650-011 verifies refactored signature works

---

## 4. Scope

### 4.1 Features to be Tested

- **WFE CRD types** (`api/workflowexecution/v1alpha1/workflowexecution_types.go`): SA removed from spec, added to status
- **AA CRD types** (`api/aianalysis/v1alpha1/aianalysis_types.go`): SA removed from SelectedWorkflow
- **Consolidated querier** (`pkg/workflowexecution/client/workflow_querier.go`): `ResolveWorkflowCatalogMetadata` returns all metadata from single DS call
- **WE controller** (`internal/controller/workflowexecution/workflowexecution_controller.go`): Consolidated resolve + SA-to-status write + dedup cleanup
- **Tekton executor** (`pkg/workflowexecution/executor/tekton.go`): Reads `wfe.Status.ServiceAccountName`
- **Job executor** (`pkg/workflowexecution/executor/job.go`): Reads `wfe.Status.ServiceAccountName`
- **Ansible executor** (`pkg/workflowexecution/executor/ansible.go`): All 8 SA references read from status
- **RO creator** (`pkg/remediationorchestrator/creator/workflowexecution.go`): Stops copying SA to WFE spec
- **AA response processor** (`pkg/aianalysis/handlers/response_processor.go`): Stops extracting SA from HAPI response

### 4.2 Features Not to be Tested

- **KA internal types** (`InvestigationResult.ServiceAccountName`): Kept for HAPI observability, not part of the propagation chain removal
- **DS catalog schema**: SA stays in catalog (source of truth)
- **RemediationWorkflow CRD**: SA stays (source of truth for DS)
- **E2E tests**: Deferred to CI pipeline; UT + IT provide adequate coverage

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| SA in WFE status, not CreateOptions | Operator visibility via `kubectl get wfe -oyaml`; no executor interface change |
| Consolidate all 4 querier calls into 1 | Eliminates 3 redundant `GetWorkflowByID` calls per reconcile |
| Delete reconciler BuildPipelineRun | Exact duplicate of executor's buildPipelineRun; HandleAlreadyExists only needs name |
| Replace PipelineRunName with ExecutionResourceName | Byte-for-byte identical; executor version is canonical |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (querier, executor SA reads, RO creator, response_processor)
- **Integration**: >=80% of integration-testable code (reconciler SA-to-status, consolidated resolve, HandleAlreadyExists)
- **E2E**: Deferred — existing E2E suite validates full workflow execution; SA source change is transparent

### 5.2 Two-Tier Minimum

Every behavioral change is covered by at least 2 tiers (UT + IT):
- **Unit tests**: Querier consolidation, executor SA source, RO creator, AA processor
- **Integration tests**: Reconciler lifecycle with SA in status, HandleAlreadyExists with resourceName

### 5.3 Business Outcome Quality Bar

Tests validate observable behavior: "SA appears in WFE status", "executors use SA from status",
"no SA in WFE spec", "single DS call per reconcile".

### 5.4 Pass/Fail Criteria

**PASS** — all of:
1. All P0 tests pass (0 failures)
2. Per-tier coverage >= 80%
3. No regressions in existing test suites
4. `rg 'wfe\.Spec\.ServiceAccountName' --glob '*.go'` returns 0 hits outside comments/docs

**FAIL** — any of:
1. Any P0 test fails
2. Per-tier coverage below 80%
3. Existing tests regress
4. Any executor still reads `wfe.Spec.ServiceAccountName`

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken; CRD regen produces unexpected diff; DS catalog API changes.
**Resume**: Build green; CRD diff validated; API stable.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/workflowexecution/client/workflow_querier.go` | `ResolveWorkflowCatalogMetadata` | ~60 |
| `pkg/workflowexecution/executor/tekton.go` | `buildPipelineRun` | ~45 |
| `pkg/workflowexecution/executor/job.go` | `Create` (PodSpec SA) | ~5 |
| `pkg/workflowexecution/executor/ansible.go` | `Create`, `injectK8sCredential`, `requestTokenForSA` | ~30 |
| `pkg/remediationorchestrator/creator/workflowexecution.go` | `Create` | ~5 |
| `pkg/aianalysis/handlers/response_processor.go` | `processLLMResponse` (3 locations) | ~15 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `reconcilePending`, `HandleAlreadyExists` | ~80 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.2` HEAD | Post-#236, post-authwebhook fix |
| Dependency: ogen client | Generated | `ogenclient.RemediationWorkflow.ServiceAccountName` is `OptString` |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| DD-WE-005 v2.0 | SA resolved from DS at runtime into WFE status | P0 | Unit | UT-WE-650-001 | Pending |
| DD-WE-005 v2.0 | Empty SA from DS = K8s default SA fallback | P0 | Unit | UT-WE-650-002 | Pending |
| DD-WE-006 | Consolidated querier returns all metadata from single DS call | P0 | Unit | UT-WE-650-003 | Pending |
| DD-WE-006 | Querier handles DS error gracefully | P0 | Unit | UT-WE-650-004 | Pending |
| DD-WE-005 v2.0 | Ansible executor reads SA from status for TokenRequest | P0 | Unit | UT-WE-650-005 | Pending |
| DD-WE-005 v2.0 | Tekton executor reads SA from status | P0 | Unit | UT-WE-650-006 | Pending |
| DD-WE-005 v2.0 | Job executor reads SA from status | P0 | Unit | UT-WE-650-007 | Pending |
| DD-WE-005 v2.0 | RO creator does not set SA on WFE spec | P0 | Unit | UT-WE-650-008 | Pending |
| DD-WE-005 v2.0 | AA response processor does not extract SA | P0 | Unit | UT-WE-650-009 | Pending |
| DD-WE-006 | Consolidated querier extracts dependencies from Content | P1 | Unit | UT-WE-650-010 | Pending |
| DD-WE-003 | HandleAlreadyExists works with resourceName string | P1 | Unit | UT-WE-650-011 | Pending |
| DD-WE-005 v2.0 | Reconciler writes SA to status during pending reconcile | P0 | Integration | IT-WE-650-001 | Pending |
| DD-WE-006 | Integration: single DS call per reconcile | P1 | Integration | IT-WE-650-002 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-WE-650-{SEQUENCE}`

### Tier 1: Unit Tests

**Testable code scope**: `pkg/workflowexecution/client/`, `pkg/workflowexecution/executor/`,
`pkg/remediationorchestrator/creator/`, `pkg/aianalysis/handlers/`

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-WE-650-001 | Controller writes resolved SA from DS to `wfe.Status.ServiceAccountName` before exec.Create | Pending |
| UT-WE-650-002 | When DS catalog has no SA, status SA is empty (K8s default fallback) | Pending |
| UT-WE-650-003 | `ResolveWorkflowCatalogMetadata` returns engine, bundle, digest, SA, deps, engineConfig from single call | Pending |
| UT-WE-650-004 | `ResolveWorkflowCatalogMetadata` returns error when DS is unreachable | Pending |
| UT-WE-650-005 | Ansible executor `requestTokenForSA` uses `wfe.Status.ServiceAccountName` | Pending |
| UT-WE-650-006 | Tekton `buildPipelineRun` sets `TaskRunTemplate.ServiceAccountName` from `wfe.Status` | Pending |
| UT-WE-650-007 | Job executor sets `PodSpec.ServiceAccountName` from `wfe.Status` | Pending |
| UT-WE-650-008 | RO creator builds WFE with no `ServiceAccountName` in spec | Pending |
| UT-WE-650-009 | AA response processor does not populate `SelectedWorkflow.ServiceAccountName` | Pending |
| UT-WE-650-010 | Consolidated querier extracts dependencies and engineConfig from Content YAML | Pending |
| UT-WE-650-011 | `HandleAlreadyExists` accepts `resourceName string` and handles collision correctly | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `internal/controller/workflowexecution/` (envtest with Tekton CRDs)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-WE-650-001 | Full reconcile: Pending WFE resolves SA from DS, writes to status, creates PipelineRun with correct SA | Pending |
| IT-WE-650-002 | Reconcile calls DS exactly once (consolidated querier) | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — SA source change is transparent to the E2E flow. The existing E2E suite
  validates full workflow execution; the behavioral contract (SA applied to execution resource)
  is unchanged, only the source shifts from spec to status.

---

## 9. Test Cases

### UT-WE-650-001: SA resolved from DS written to WFE status

**BR**: DD-WE-005 v2.0
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Preconditions**: Mock querier returns `WorkflowCatalogMetadata` with `ServiceAccountName: "workflow-sa"`.

**Test Steps**:
1. **Given**: A WFE in Pending phase with `WorkflowRef.WorkflowID` set
2. **When**: Controller reconciles and calls `ResolveWorkflowCatalogMetadata`
3. **Then**: `wfe.Status.ServiceAccountName` equals `"workflow-sa"`

### UT-WE-650-003: Consolidated querier returns all metadata

**BR**: DD-WE-006
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Preconditions**: Mock `WorkflowCatalogClient` returns a `RemediationWorkflow` with all fields populated.

**Test Steps**:
1. **Given**: DS catalog entry with engine=tekton, bundle=ghcr.io/test, SA=workflow-sa, Content with deps
2. **When**: `ResolveWorkflowCatalogMetadata` is called
3. **Then**: Returned `WorkflowCatalogMetadata` has all fields populated from the single response

### UT-WE-650-008: RO creator builds WFE without SA

**BR**: DD-WE-005 v2.0
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/workflowexecution_creator_sa_test.go`

**Preconditions**: AIAnalysis with SelectedWorkflow that has no ServiceAccountName field.

**Test Steps**:
1. **Given**: AA status with selected workflow (no SA field in type)
2. **When**: RO creator builds WorkflowExecution
3. **Then**: `wfe.Spec` has no `ServiceAccountName` field (field does not exist in type)

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `mockWorkflowCatalogClient` (DS client), `fake.NewClientBuilder()` (K8s)
- **Location**: `test/unit/workflowexecution/`, `test/unit/remediationorchestrator/`, `test/unit/aianalysis/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: envtest with Tekton CRDs, PostgreSQL + Redis (shared infra), real DS
- **Location**: `test/integration/workflowexecution/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.23 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-gen | v0.x | CRD generation |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #501 | Code | Merged | SA schema baseline | N/A (already merged) |
| Issue #518 | Code | Merged | Runtime engine resolution pattern | N/A (already merged) |

### 11.2 Execution Order

1. **Phase 1 RED**: Type changes + new/updated failing tests
2. **Phase 2 GREEN**: Implement querier, controller SA-to-status, executor updates, CRD regen
3. **Phase 3 REFACTOR**: Remove old querier methods, dedup BuildPipelineRun/PipelineRunName

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/650/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/workflowexecution/`, `test/unit/remediationorchestrator/`, `test/unit/aianalysis/` | Ginkgo BDD test files |
| Integration test suite | `test/integration/workflowexecution/` | Ginkgo BDD test files |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/workflowexecution/... -ginkgo.v
go test ./test/unit/remediationorchestrator/... -ginkgo.v
go test ./test/unit/aianalysis/... -ginkgo.v

# Integration tests
go test ./test/integration/workflowexecution/... -ginkgo.v

# Specific test by ID
go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-650"

# Coverage
go test ./test/unit/workflowexecution/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `executor_sa_test.go` (5 refs) | `wfe.Spec.ServiceAccountName` | Change to `wfe.Status.ServiceAccountName` | SA moved to status |
| `controller_test.go` (2 refs) | `wfe.Spec.ServiceAccountName` in BuildPipelineRun assertions | Remove/rewrite for status-based SA | BuildPipelineRun deleted |
| `executor_test.go` (2 refs) | `wfe.Spec.ServiceAccountName` | Change to `wfe.Status.ServiceAccountName` | SA moved to status |
| `workflowexecution_creator_sa_test.go` (4 refs) | Sets `SelectedWorkflow.ServiceAccountName` | Remove SA from AA type; assert WFE has no spec SA | SA removed from upstream |
| `reconciler_test.go` (2 refs) | `wfe.Spec.ServiceAccountName` | Change to `wfe.Status.ServiceAccountName` | SA moved to status |
| `job_lifecycle_integration_test.go` (1 ref) | `wfe.Spec.ServiceAccountName` | Change to `wfe.Status.ServiceAccountName` | SA moved to status |
| `response_processor_sa_test.go` | Asserts SA extracted from HAPI response | Rewrite: assert SA is NOT extracted | SA extraction removed |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial test plan |
