# Test Plan: WE Ansible Executor TokenRequest + CRD Schema Migration

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-501-v1
**Feature**: Per-workflow SA token via TokenRequest for Ansible executor + CRD schema migration of serviceAccountName
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `development/v1.2`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the two changes delivered by Issue #501:
(1) migrating `serviceAccountName` from `spec.executionConfig.serviceAccountName` to `spec.serviceAccountName` in the WorkflowExecution CRD, and
(2) replacing the Ansible executor's static in-cluster credential injection with per-workflow short-lived tokens obtained via the Kubernetes TokenRequest API, including TTL validation, WFE status conditions, and K8s events.

### 1.2 Objectives

1. **CRD schema correctness**: All three executors (Tekton, Job, Ansible) read `spec.serviceAccountName` directly; `ResolveServiceAccountName` is deleted and `ExecutionConfig` no longer contains the field.
2. **TokenRequest injection**: When `spec.serviceAccountName` is set, the Ansible executor uses the TokenRequest API to obtain a short-lived token scoped to the workflow SA; when empty, it falls back to the controller's in-cluster credentials.
3. **TTL validation**: The executor detects when the API server shortens the granted token TTL below the WFE execution timeout and surfaces this as a `CreateResult.Warning`.
4. **Warning propagation**: The reconciler processes `CreateResult.Warnings`, sets WFE status conditions (`TokenTTLInsufficient`), and emits K8s warning events (`TokenTTLShortened`).
5. **RO creator correctness**: `buildExecutionConfig` only handles Timeout; `Spec.ServiceAccountName` is set directly at the WFE construction site.
6. **Interface evolution**: `Executor.Create` returns `(*CreateResult, error)` across all implementations.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/workflowexecution/... ./test/unit/remediationorchestrator/...` |
| Integration test pass rate | 100% | `go test ./test/integration/workflowexecution/...` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on unit-testable files |
| Integration-testable code coverage | >=80% | `go test -coverprofile` on integration-testable files |
| Regressions | 0 | All existing tests pass without modification (beyond planned migration) |

---

## 2. References

### 2.1 Authority (governing documents)

- BR-WE-015: Ansible executor launches AWX Job Templates via REST API
- BR-WE-016: Engine-specific configuration via engineConfig
- DD-WE-005 v2.0: Per-workflow ServiceAccount reference
- Issue #501: WE Ansible executor: per-workflow SA token via TokenRequest + CRD schema migration
- Issue #500: v1.1 interim (prerequisite — `injectK8sCredential` fallback)
- Issue #481: Per-workflow ServiceAccount reference (design)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | TokenRequest returns shorter TTL than requested | Mid-playbook 401 failures | Medium | UT-WE-501-007, UT-WE-501-008, UT-WE-501-009 | TTL validation compares granted vs requested; surfaces warning via CreateResult |
| R2 | Executor interface change breaks existing tests | Test compilation failures | Low | UT-WE-501-001, UT-WE-501-002, UT-WE-501-003 | All 3 executors updated atomically; test migration in scope |
| R3 | RBAC missing for serviceaccounts/token create | TokenRequest fails with Forbidden | Low | (Helm chart, not testable in UT) | Helm RBAC rule added in implementation |
| R4 | Fallback to in-cluster creds breaks when SA specified but nonexistent | AWX gets wrong credentials | Medium | UT-WE-501-006 | TokenRequest error propagates as Create failure; no silent fallback |

### 3.1 Risk-to-Test Traceability

- **R1** (TTL shortened): Covered by UT-WE-501-007 (TTL sufficient), UT-WE-501-008 (TTL insufficient warning), UT-WE-501-009 (warning propagation to reconciler)
- **R2** (interface change): Covered by UT-WE-501-001, -002, -003 (all three executors return CreateResult)
- **R4** (fallback correctness): Covered by UT-WE-501-005 (fallback) and UT-WE-501-006 (TokenRequest failure)

---

## 4. Scope

### 4.1 Features to be Tested

- **CRD schema** (`api/workflowexecution/v1alpha1/workflowexecution_types.go`): `ServiceAccountName` at `Spec` level; removed from `ExecutionConfig`
- **Executor interface** (`pkg/workflowexecution/executor/executor.go`): `CreateResult` struct; `Create` return type
- **Tekton executor** (`pkg/workflowexecution/executor/tekton.go`): Inline SA read; `Create` returns `*CreateResult`
- **Job executor** (`pkg/workflowexecution/executor/job.go`): Inline SA read; `Create` returns `*CreateResult`
- **Ansible executor** (`pkg/workflowexecution/executor/ansible.go`): TokenRequest injection, fallback, TTL validation, warnings
- **RO creator** (`pkg/remediationorchestrator/creator/workflowexecution.go`): `buildExecutionConfig` simplified; `Spec.ServiceAccountName` set directly
- **WE reconciler** (`internal/controller/workflowexecution/workflowexecution_controller.go`): Warning processing, condition setting, event emission

### 4.2 Features Not to be Tested

- **AWX REST API calls**: Mocked in unit tests (external dependency)
- **Helm chart RBAC**: Validated by manual review and E2E (separate issue)
- **DD-WE-005 document update**: Documentation, not code
- **Godoc updates**: Comment-only changes in AIAnalysis and RemediationWorkflow CRD types

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Change `Create` return to `*CreateResult` | Cleanest contract for warning propagation; no BC needed |
| Inline `ResolveServiceAccountName` and delete | One-liner after migration; reduces indirection |
| Mock `kubernetes.Interface` for TokenRequest tests | External K8s API; real calls require envtest |
| Test TTL validation via mock TokenRequest response | Cannot control API server `--service-account-max-token-expiration` in unit tests |

---

## 5. Approach

### 5.1 Coverage Policy

**Authority**: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (executor logic, TTL validation, CreateResult construction, SA resolution)
- **Integration**: >=80% of integration-testable code (reconciler warning handling, condition setting, event emission)
- **E2E**: Deferred — requires AWX and real K8s cluster with configurable `--service-account-max-token-expiration`

### 5.2 Two-Tier Minimum

Every business requirement is covered by at least UT + IT:
- **Unit tests**: TokenRequest logic, TTL validation, fallback, schema migration correctness
- **Integration tests**: Reconciler processes warnings, sets conditions, emits events; existing reconciler tests migrated

### 5.3 Business Outcome Quality Bar

Tests validate business outcomes:
- "Operator gets a WFE condition when token TTL is too short for their playbook timeout" (not "condition setter function is called")
- "AWX receives per-workflow SA credentials instead of controller SA credentials" (not "TokenRequest API is called")
- "Job pod runs with the SA specified in the WFE spec" (not "ServiceAccountName field is read")

### 5.4 Pass/Fail Criteria

**PASS** — all of:

1. All P0 tests pass (0 failures)
2. All P1 tests pass
3. Per-tier code coverage meets >=80%
4. No regressions in existing WE, RO test suites
5. `go build ./...` succeeds
6. `make generate && make manifests` produces clean CRD

**FAIL** — any of:

1. Any P0 test fails
2. Per-tier coverage falls below 80%
3. Existing tests regress

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken; prerequisite #500 reverted.
**Resume**: Build fixed; #500 restored.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/workflowexecution/executor/executor.go` | `CreateResult`, `Warning` structs | ~125 |
| `pkg/workflowexecution/executor/ansible.go` | `injectK8sCredential` (TokenRequest path + fallback), TTL validation helper | ~747 |
| `pkg/workflowexecution/executor/tekton.go` | `buildPipelineRun` (SA inlined) | ~313 |
| `pkg/workflowexecution/executor/job.go` | `buildJob` (SA inlined) | ~337 |
| `pkg/remediationorchestrator/creator/workflowexecution.go` | `buildExecutionConfig` (Timeout only), WFE construction | ~228 |
| `api/workflowexecution/v1alpha1/workflowexecution_types.go` | Condition constants | ~524 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `reconcilePending` (warning processing, conditions, events) | ~1808 |
| `cmd/workflowexecution/main.go` | Ansible executor wiring with `directClientset` | ~363 |

### 6.3 Version Identification

| Item | Version/Commit | Notes |
|------|----------------|-------|
| Code under test | `development/v1.2` HEAD | Current branch |
| Dependency: #500 | Merged | `injectK8sCredential` fallback exists |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WE-015 | Ansible executor uses per-workflow SA token via TokenRequest | P0 | Unit | UT-WE-501-004 | Pending |
| BR-WE-015 | Ansible executor falls back to controller SA when no workflow SA | P0 | Unit | UT-WE-501-005 | Pending |
| BR-WE-015 | TokenRequest failure propagates as Create error | P0 | Unit | UT-WE-501-006 | Pending |
| BR-WE-015 | TTL validation: sufficient TTL produces no warnings | P0 | Unit | UT-WE-501-007 | Pending |
| BR-WE-015 | TTL validation: insufficient TTL produces warning in CreateResult | P0 | Unit | UT-WE-501-008 | Pending |
| DD-WE-005 | Tekton executor reads spec.serviceAccountName directly | P0 | Unit | UT-WE-501-001 | Pending |
| DD-WE-005 | Job executor reads spec.serviceAccountName directly | P0 | Unit | UT-WE-501-002 | Pending |
| DD-WE-005 | Ansible Create returns *CreateResult with ResourceName | P0 | Unit | UT-WE-501-003 | Pending |
| DD-WE-005 | RO creator sets Spec.ServiceAccountName directly; buildExecutionConfig only handles Timeout | P0 | Unit | UT-WE-501-010 | Pending |
| DD-WE-005 | Reconciler sets TokenTTLInsufficient condition from CreateResult warning | P0 | Unit | UT-WE-501-009 | Pending |
| DD-WE-005 | Reconciler emits TokenTTLShortened event from CreateResult warning | P1 | Unit | UT-WE-501-011 | Pending |
| DD-WE-005 | Tekton executor returns *CreateResult (interface compliance) | P0 | Integration | IT-WE-501-001 | Pending |
| DD-WE-005 | Job executor returns *CreateResult (interface compliance) | P0 | Integration | IT-WE-501-002 | Pending |
| BR-WE-015 | Reconciler warning-to-condition full integration path | P0 | Integration | IT-WE-501-003 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `WE` (WorkflowExecution)
- **ISSUE**: 501
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `pkg/workflowexecution/executor/`, `pkg/remediationorchestrator/creator/`, condition constants. >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-WE-501-001` | Tekton PipelineRun runs under the SA specified in the WFE spec (not buried in executionConfig) | Pending |
| `UT-WE-501-002` | Job pod runs under the SA specified in the WFE spec (not buried in executionConfig) | Pending |
| `UT-WE-501-003` | Ansible Create returns a structured CreateResult with the AWX job ID as ResourceName | Pending |
| `UT-WE-501-004` | AWX receives per-workflow SA credentials (not controller SA) when workflow SA is specified | Pending |
| `UT-WE-501-005` | AWX receives controller in-cluster credentials when no workflow SA is specified (backward-compatible fallback) | Pending |
| `UT-WE-501-006` | Workflow execution fails cleanly when TokenRequest returns an error (SA not found, RBAC insufficient) | Pending |
| `UT-WE-501-007` | No TTL warning when granted token TTL meets or exceeds execution timeout | Pending |
| `UT-WE-501-008` | TTL warning surfaced in CreateResult when API server shortens token TTL below execution timeout | Pending |
| `UT-WE-501-009` | Reconciler sets TokenTTLInsufficient=True condition on WFE when CreateResult contains TTL warning | Pending |
| `UT-WE-501-010` | RO creator produces WFE with Spec.ServiceAccountName set and ExecutionConfig containing only Timeout | Pending |
| `UT-WE-501-011` | Reconciler emits TokenTTLShortened K8s warning event on WFE when CreateResult contains TTL warning | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `internal/controller/workflowexecution/`, reconciler wiring. >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-WE-501-001` | Tekton executor Create returns *CreateResult through the full reconciler dispatch path | Pending |
| `IT-WE-501-002` | Job executor Create returns *CreateResult through the full reconciler dispatch path | Pending |
| `IT-WE-501-003` | Reconciler reads TTL warning from CreateResult, sets WFE condition, and emits K8s event in envtest | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. Requires real AWX server and K8s cluster with configurable `--service-account-max-token-expiration`. Covered by manual validation during demo.

---

## 9. Test Cases

### UT-WE-501-001: Tekton PipelineRun uses spec.serviceAccountName

**BR**: DD-WE-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/executor_sa_test.go`

**Test Steps**:
1. **Given**: A WFE with `Spec.ServiceAccountName = "custom-tekton-sa"` and no `ExecutionConfig`
2. **When**: `TektonExecutor.Create()` builds a PipelineRun
3. **Then**: The PipelineRun's `TaskRunTemplate.ServiceAccountName` equals `"custom-tekton-sa"` and the returned `CreateResult.ResourceName` is non-empty

**Acceptance Criteria**:
- **Behavior**: PipelineRun is created with the specified SA
- **Correctness**: `ServiceAccountName` is `"custom-tekton-sa"` (exact match)
- **Accuracy**: No warnings in `CreateResult.Warnings` (empty slice)

### UT-WE-501-002: Job pod uses spec.serviceAccountName

**BR**: DD-WE-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/executor_sa_test.go`

**Test Steps**:
1. **Given**: A WFE with `Spec.ServiceAccountName = "custom-job-sa"` and no `ExecutionConfig`
2. **When**: `JobExecutor.Create()` builds a Job
3. **Then**: The Job's `PodSpec.ServiceAccountName` equals `"custom-job-sa"` and the returned `CreateResult.ResourceName` is non-empty

**Acceptance Criteria**:
- **Behavior**: Job pod runs under specified SA
- **Correctness**: `ServiceAccountName` is `"custom-job-sa"` (exact match)
- **Accuracy**: No warnings in `CreateResult.Warnings`

### UT-WE-501-003: Ansible Create returns CreateResult

**BR**: DD-WE-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_token_request_test.go`

**Test Steps**:
1. **Given**: A WFE with Ansible engine, no `Spec.ServiceAccountName`, mock AWX client that returns job ID 42
2. **When**: `AnsibleExecutor.Create()` is called
3. **Then**: `CreateResult.ResourceName` equals `"42"` and `CreateResult.Warnings` is empty

**Acceptance Criteria**:
- **Behavior**: AWX job launched, result wrapped in CreateResult
- **Correctness**: ResourceName matches AWX job ID string
- **Accuracy**: No spurious warnings

### UT-WE-501-004: TokenRequest injects per-workflow SA credentials

**BR**: BR-WE-015
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_token_request_test.go`

**Test Steps**:
1. **Given**: A WFE with `Spec.ServiceAccountName = "workflow-sa"`, mock `kubernetes.Interface` that returns token `"sa-token-xyz"` with sufficient TTL, mock AWX client
2. **When**: `AnsibleExecutor.Create()` is called
3. **Then**: AWX `CreateCredential` receives `BearerToken = "sa-token-xyz"` (not the controller's in-cluster token)

**Acceptance Criteria**:
- **Behavior**: AWX ephemeral credential uses the workflow SA token
- **Correctness**: Token value matches the TokenRequest response exactly
- **Accuracy**: `InClusterCredentialsFn` is NOT called

### UT-WE-501-005: Fallback to in-cluster credentials when no SA specified

**BR**: BR-WE-015
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_token_request_test.go`

**Test Steps**:
1. **Given**: A WFE with empty `Spec.ServiceAccountName`, mock `InClusterCredentialsFn` returning `{Token: "controller-token"}`
2. **When**: `AnsibleExecutor.Create()` is called
3. **Then**: AWX `CreateCredential` receives `BearerToken = "controller-token"`

**Acceptance Criteria**:
- **Behavior**: Falls back to controller SA token
- **Correctness**: Token value matches in-cluster credentials
- **Accuracy**: TokenRequest API is NOT called (mock `kubernetes.Interface` receives zero calls)

### UT-WE-501-006: TokenRequest error propagates as Create failure

**BR**: BR-WE-015
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_token_request_test.go`

**Test Steps**:
1. **Given**: A WFE with `Spec.ServiceAccountName = "nonexistent-sa"`, mock `kubernetes.Interface` that returns `NotFound` error
2. **When**: `AnsibleExecutor.Create()` is called
3. **Then**: Returns `nil` result and error containing "TokenRequest" and "nonexistent-sa"

**Acceptance Criteria**:
- **Behavior**: Create fails with descriptive error
- **Correctness**: Error wraps the original NotFound cause
- **Accuracy**: No AWX job is launched (AWX client receives zero `LaunchJobTemplate` calls)

### UT-WE-501-007: No TTL warning when TTL is sufficient

**BR**: BR-WE-015
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_token_request_test.go`

**Test Steps**:
1. **Given**: WFE with 30m timeout, `Spec.ServiceAccountName = "my-sa"`, mock TokenRequest returns `expirationTimestamp` = now + 3600s
2. **When**: `AnsibleExecutor.Create()` is called
3. **Then**: `CreateResult.Warnings` has length 0

**Acceptance Criteria**:
- **Behavior**: No warning when granted TTL (3600s) >= execution timeout (1800s)
- **Correctness**: Warnings slice is empty (not nil -- length 0)
- **Accuracy**: AWX job launches successfully

### UT-WE-501-008: TTL warning when API server shortens token

**BR**: BR-WE-015
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_token_request_test.go`

**Test Steps**:
1. **Given**: WFE with 60m timeout, `Spec.ServiceAccountName = "my-sa"`, mock TokenRequest returns `expirationTimestamp` = now + 600s (API server minimum)
2. **When**: `AnsibleExecutor.Create()` is called
3. **Then**: `CreateResult.Warnings` has exactly 1 warning with `Type = "TokenTTLInsufficient"`, `Reason = "TokenTTLShortened"`, and `Message` containing "600s" and "3600s"

**Acceptance Criteria**:
- **Behavior**: Warning surfaces TTL mismatch
- **Correctness**: Warning type, reason, and message contain expected values
- **Accuracy**: AWX job still launches (soft warning, not hard failure)

### UT-WE-501-009: Reconciler sets condition from warning

**BR**: DD-WE-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Test Steps**:
1. **Given**: A mock executor that returns `CreateResult{Warnings: [{Type: "TokenTTLInsufficient", Reason: "TokenTTLShortened", Message: "granted 600s < timeout 3600s"}]}`
2. **When**: Reconciler processes the CreateResult in `reconcilePending`
3. **Then**: WFE status has a condition with `Type = "TokenTTLInsufficient"`, `Status = True`, `Reason = "TokenTTLShortened"`

**Acceptance Criteria**:
- **Behavior**: Operator sees condition on WFE indicating token TTL concern
- **Correctness**: Condition type, status, and reason match expected values
- **Accuracy**: Condition message includes the granted and requested TTL values

### UT-WE-501-010: RO creator sets Spec.ServiceAccountName directly

**BR**: DD-WE-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/remediationorchestrator/workflowexecution_creator_sa_test.go`

**Test Steps**:
1. **Given**: A RemediationRequest with 30m timeout and AIAnalysis with `SelectedWorkflow.ServiceAccountName = "workflow-sa"`
2. **When**: `WorkflowExecutionCreator.CreateWorkflowExecution()` is called
3. **Then**: Created WFE has `Spec.ServiceAccountName = "workflow-sa"` and `Spec.ExecutionConfig` only contains `Timeout` (no `ServiceAccountName` field)

**Acceptance Criteria**:
- **Behavior**: WFE produced with SA at top-level spec
- **Correctness**: `Spec.ServiceAccountName` equals `"workflow-sa"`; `ExecutionConfig.ServiceAccountName` does not exist (field removed from struct)
- **Accuracy**: `ExecutionConfig.Timeout` still correctly derived from RemediationRequest

### UT-WE-501-011: Reconciler emits K8s event from warning

**BR**: DD-WE-005
**Priority**: P1
**Type**: Unit
**File**: `test/unit/workflowexecution/controller_test.go`

**Test Steps**:
1. **Given**: A mock executor that returns `CreateResult{Warnings: [{Type: "TokenTTLInsufficient", Reason: "TokenTTLShortened", Message: "granted 600s < timeout 3600s"}]}`
2. **When**: Reconciler processes the CreateResult in `reconcilePending`
3. **Then**: A K8s event of type `Warning` with reason `TokenTTLShortened` is emitted on the WFE resource

**Acceptance Criteria**:
- **Behavior**: Operator sees K8s event warning about TTL
- **Correctness**: Event type is `Warning`, reason is `TokenTTLShortened`
- **Accuracy**: Event message includes granted and requested TTL values

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: AWXClient (interface), kubernetes.Interface (fake clientset), InClusterCredentialsFn (function replacement)
- **Location**: `test/unit/workflowexecution/`, `test/unit/remediationorchestrator/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**: envtest (K8s API server), fake executor registry for warning injection
- **Location**: `test/integration/workflowexecution/`

### 10.3 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22 | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| controller-runtime | v0.19+ | envtest |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| Issue #500 | Code | Merged | `injectK8sCredential` fallback does not exist | N/A — already merged |

### 11.2 Execution Order

1. **Phase 1**: CRD schema migration + Executor interface change (UT-WE-501-001, -002, -003, -010)
2. **Phase 2**: TokenRequest injection + TTL validation (UT-WE-501-004 through -008)
3. **Phase 3**: Reconciler warning processing (UT-WE-501-009, -011)
4. **Phase 4**: Integration tests (IT-WE-501-001 through -003)

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/501/TEST_PLAN.md` | Strategy and test design |
| Unit test suite (executor) | `test/unit/workflowexecution/` | CRD migration + TokenRequest tests |
| Unit test suite (RO creator) | `test/unit/remediationorchestrator/` | buildExecutionConfig migration |
| Integration test suite | `test/integration/workflowexecution/` | Reconciler warning processing |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/workflowexecution/... -ginkgo.v
go test ./test/unit/remediationorchestrator/... -ginkgo.v

# Integration tests
go test ./test/integration/workflowexecution/... -ginkgo.v

# Specific test by ID
go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-501"

# Coverage
go test ./test/unit/workflowexecution/... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

---

## 14. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `executor_sa_test.go:51-54` | `wfe.Spec.ExecutionConfig.ServiceAccountName` | `wfe.Spec.ServiceAccountName` | Field moved to spec level |
| `executor_sa_test.go:59-85` | Tests `ResolveServiceAccountName` function | Rewrite to test SA field read directly by executors | Function deleted |
| `controller_test.go:545-548` | `wfe.Spec.ExecutionConfig = &{ServiceAccountName: "custom-sa"}` | `wfe.Spec.ServiceAccountName = "custom-sa"` | Field moved |
| `controller_test.go:493` | `exec.Create(ctx, wfe, ns, opts)` returns `(string, error)` | Returns `(*CreateResult, error)` | Interface changed |
| `workflowexecution_creator_sa_test.go` | Tests `buildExecutionConfig(rr, saName)` | Test `buildExecutionConfig(rr)` + separate `Spec.ServiceAccountName` | SA param removed |
| `reconciler_test.go` (2 instances) | `Spec.ExecutionConfig.ServiceAccountName` | `Spec.ServiceAccountName` | Field moved |

---

## 15. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
