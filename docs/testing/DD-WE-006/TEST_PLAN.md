# Test Plan: Schema-Declared Infrastructure Dependencies (DD-WE-006)

**Feature**: Workflow schemas declare infrastructure dependencies (Secrets/ConfigMaps) that are validated at registration and injected at execution
**Version**: 1.0
**Created**: 2026-02-24
**Author**: AI Assistant + Jordi Gil
**Status**: Draft
**Branch**: `main`

**Authority**:
- [DD-WE-006](../../../docs/architecture/decisions/DD-WE-006-schema-declared-dependencies.md): Schema-Declared Infrastructure Dependencies
- [BR-WORKFLOW-004](../../../docs/requirements/BR-WORKFLOW-004-workflow-schema-format.md): Workflow Schema Format Specification
- [BR-WE-014](../../../docs/requirements/BR-WE-014-kubernetes-job-execution-backend.md): Kubernetes Job Execution Backend

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **DS dependency parsing** (`pkg/datastorage/models/`, `pkg/datastorage/schema/`): YAML parsing of `dependencies` section, structural validation (non-empty names, uniqueness within category)
- **DS registration-time K8s validation** (`pkg/datastorage/validation/`, `pkg/datastorage/server/workflow_handlers.go`): Verifying declared Secrets/ConfigMaps exist in `kubernaut-workflows` with non-empty `.data` at workflow registration
- **WE dependency querying** (`pkg/workflowexecution/client/`): `OgenWorkflowQuerier` fetching workflow content from DS, parsing dependencies client-side
- **WE execution-time K8s validation** (`internal/controller/workflowexecution/`): Defense-in-depth re-validation of dependency existence before creating execution resources
- **Job executor dependency injection** (`pkg/workflowexecution/executor/job.go`): Volume mounts at `/run/kubernaut/secrets/<name>` and `/run/kubernaut/configmaps/<name>`
- **Tekton executor dependency injection** (`pkg/workflowexecution/executor/tekton.go`): Workspace bindings `secret-<name>` and `configmap-<name>`
- **Backward compatibility**: Workflows without `dependencies` section continue to work unaffected

### Out of Scope

- **RBAC provisioning**: `dependency-reader-rbac.yaml` Helm chart testing (infrastructure concern, not business logic)
- **Demo scenario validation**: `deploy/demo/scenarios/` shell scripts (manual validation)
- **OpenAPI spec changes**: No DS API changes required (dependencies parsed from existing `Content` field)
- **CRD schema changes**: No WorkflowExecution CRD modifications (dependencies resolved on-demand, not propagated through CRDs)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Volume mounts over env vars | Avoids credential leakage in `kubectl describe`; supports multi-key secrets |
| Dual validation (DS + WFE) | Defense-in-depth: catches post-registration infrastructure changes |
| On-demand DS query (no CRD propagation) | Workflows are immutable (DD-WORKFLOW-012); avoids CRD bloat |
| `ConfigurationError` for failure reason | Reuses existing CRD enum value; avoids CRD schema migration |
| `kubernaut-workflows` namespace scoping | Dependencies co-located with execution resources; RBAC is namespace-scoped |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (pure logic: schema parsing, validators, dependency builders, querier)
- **Integration**: >=80% of **integration-testable** code (I/O: HTTP handler validation step, controller `resolveDependencies`, K8s client wiring)
- **E2E**: >=80% of full service code (when applicable)

### 2-Tier Minimum

Every business requirement gap is covered by at least 2 test tiers (UT + IT):
- **Unit tests** catch logic and correctness errors (parsing, validation, volume/workspace construction)
- **Integration tests** catch wiring, data fidelity, and behavior errors across DS/WFE component boundaries

### Business Outcome Quality Bar

Tests validate **business outcomes** -- behavior, correctness, and data accuracy -- not just code path coverage. Each test scenario answers: "what does the user/operator/system get?" not "what function is called?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/models/workflow_schema.go` | `WorkflowDependencies`, `ResourceDependency`, `ValidateDependencies` | ~55 |
| `pkg/datastorage/schema/parser.go` | `ExtractDependencies`, `ValidateDependencies` call in `Validate` | ~14 |
| `pkg/datastorage/validation/dependency_validator.go` | `DependencyValidator` interface, `K8sDependencyValidator`, `ValidateDependencies` | ~80 |
| `pkg/workflowexecution/executor/executor.go` | `CreateOptions` struct | ~10 |
| `pkg/workflowexecution/executor/job.go` | `buildDependencyVolumes`, `SecretMountBasePath`, `ConfigMapMountBasePath` | ~55 |
| `pkg/workflowexecution/executor/tekton.go` | `buildDependencyWorkspaces` | ~35 |
| `pkg/workflowexecution/client/workflow_querier.go` | `WorkflowQuerier` interface, `OgenWorkflowQuerier`, `GetWorkflowDependencies` | ~118 |

**Total unit-testable**: ~367 lines

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/server/workflow_handlers.go` | Step 5d: `ExtractDependencies` + `ValidateDependencies` in `HandleCreateWorkflow` | ~20 |
| `pkg/datastorage/server/handler.go` | `dependencyValidator` field, `WithDependencyValidator` option wiring | ~14 |
| `internal/controller/workflowexecution/workflowexecution_controller.go` | `resolveDependencies` method, `WorkflowQuerier`/`DependencyValidator` field wiring | ~52 |
| `cmd/datastorage/main.go` | `NewK8sDependencyValidator` + `WithDependencyValidator` startup wiring | ~15 |
| `cmd/workflowexecution/main.go` | `NewOgenWorkflowQuerierFromConfig` + `NewK8sDependencyValidator` startup wiring | ~25 |

**Total integration-testable**: ~126 lines

---

## 4. BR Coverage Matrix

### Data Storage Tests

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-004 | Parse dependencies section with secrets and configMaps | P0 | Unit | UT-DS-006-001 | Pass |
| BR-WORKFLOW-004 | Backward compat: schema without dependencies accepted | P0 | Unit | UT-DS-006-002 | Pass |
| BR-WORKFLOW-004 | Empty dependencies section accepted | P1 | Unit | UT-DS-006-003 | Pass |
| BR-WORKFLOW-004 | Schema with only secrets, no configMaps | P1 | Unit | UT-DS-006-004 | Pass |
| BR-WORKFLOW-004 | Schema with only configMaps, no secrets | P1 | Unit | UT-DS-006-005 | Pass |
| BR-WORKFLOW-004 | Multiple secrets and configMaps parsed | P0 | Unit | UT-DS-006-006 | Pass |
| BR-WORKFLOW-004 | Reject secret with empty name | P0 | Unit | UT-DS-006-010 | Pass |
| BR-WORKFLOW-004 | Reject configMap with empty name | P0 | Unit | UT-DS-006-011 | Pass |
| BR-WORKFLOW-004 | Reject duplicate secret names | P0 | Unit | UT-DS-006-012 | Pass |
| BR-WORKFLOW-004 | Reject duplicate configMap names | P0 | Unit | UT-DS-006-013 | Pass |
| BR-WORKFLOW-004 | Allow same name in different categories | P1 | Unit | UT-DS-006-014 | Pass |
| DD-WE-006 | Extract dependencies from parsed schema | P0 | Unit | UT-DS-006-020 | Pass |
| DD-WE-006 | ExtractDependencies returns nil for no deps | P0 | Unit | UT-DS-006-021 | Pass |
| DD-WE-006 | K8s validator passes when all secrets exist with data | P0 | Unit | UT-DS-006-030 | Pass |
| DD-WE-006 | K8s validator fails when secret does not exist | P0 | Unit | UT-DS-006-031 | Pass |
| DD-WE-006 | K8s validator fails when secret has empty data | P0 | Unit | UT-DS-006-032 | Pass |
| DD-WE-006 | K8s validator passes when all configMaps exist with data | P0 | Unit | UT-DS-006-033 | Pass |
| DD-WE-006 | K8s validator fails when configMap does not exist | P0 | Unit | UT-DS-006-034 | Pass |
| DD-WE-006 | K8s validator fails when configMap has empty data | P0 | Unit | UT-DS-006-035 | Pass |
| DD-WE-006 | K8s validator passes with nil dependencies | P1 | Unit | UT-DS-006-036 | Pass |
| DD-WE-006 | K8s validator validates both secrets and configMaps together | P0 | Unit | UT-DS-006-037 | Pass |
| DD-WE-006 | K8s validator accepts configMap with binary data only | P1 | Unit | UT-DS-006-038 | Pass |
| DD-WE-006 | K8s validator reports specific failing resource in mixed pass/fail | P0 | Unit | UT-DS-006-039 | Pass |
| DD-WE-006 | Registration succeeds when all deps exist with non-empty data | P0 | Integration | IT-DS-006-001 | Implemented |
| DD-WE-006 | Registration fails when declared Secret missing | P0 | Integration | IT-DS-006-002 | Implemented |
| DD-WE-006 | Registration fails when declared Secret has empty data | P0 | Integration | IT-DS-006-003 | Implemented |
| DD-WE-006 | Registration fails when declared ConfigMap missing | P0 | Integration | IT-DS-006-004 | Implemented |
| DD-WE-006 | Registration fails when declared ConfigMap has empty data | P0 | Integration | IT-DS-006-005 | Implemented |
| BR-WORKFLOW-004 | Registration succeeds for workflow without dependencies | P0 | Integration | IT-DS-006-006 | Implemented |

### WorkflowExecution Tests

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| DD-WE-006 | Querier extracts secret dependencies from workflow content | P0 | Unit | UT-WE-006-001 | Pass |
| DD-WE-006 | Querier extracts both secrets and configMaps | P0 | Unit | UT-WE-006-002 | Pass |
| DD-WE-006 | Querier returns nil when workflow has no dependencies | P0 | Unit | UT-WE-006-003 | Pass |
| DD-WE-006 | Querier returns error for invalid UUID | P1 | Unit | UT-WE-006-004 | Pass |
| DD-WE-006 | Querier returns error when workflow not found (404) | P0 | Unit | UT-WE-006-005 | Pass |
| DD-WE-006 | Querier returns error when DS query fails | P0 | Unit | UT-WE-006-006 | Pass |
| DD-WE-006 | Querier returns nil when content is empty | P1 | Unit | UT-WE-006-007 | Pass |
| DD-WE-006 | Querier returns parse error for malformed YAML content | P1 | Unit | UT-WE-006-008 | Pass |
| BR-WE-014 | Job executor mounts secrets as volumes | P0 | Unit | UT-WE-006-020 | Pass |
| BR-WE-014 | Job executor mounts configMaps as volumes | P0 | Unit | UT-WE-006-021 | Pass |
| BR-WE-014 | Job executor mounts both secrets and configMaps at correct paths | P0 | Unit | UT-WE-006-022 | Pass |
| BR-WE-014 | Job executor creates Job without volumes when no dependencies | P0 | Unit | UT-WE-006-023 | Pass |
| BR-WE-014 | Tekton executor adds secret workspace binding | P0 | Unit | UT-WE-006-024 | Pass |
| BR-WE-014 | Tekton executor adds configMap workspace binding | P0 | Unit | UT-WE-006-025 | Pass |
| BR-WE-014 | Tekton executor creates PipelineRun without workspaces when no deps | P0 | Unit | UT-WE-006-026 | Pass |
| BR-WE-014 | Tekton executor adds both secret and configMap workspaces | P0 | Unit | UT-WE-006-027 | Pass |
| BR-WE-014 | Controller queries DS and creates Job with secret volumes | P0 | Integration | IT-WE-006-001 | Implemented |
| BR-WE-014 | Controller queries DS and creates Job with configMap volumes | P0 | Integration | IT-WE-006-002 | Implemented |
| DD-WE-006 | Controller marks WFE Failed (ConfigurationError) when dep missing | P0 | Integration | IT-WE-006-003 | Implemented |
| DD-WE-006 | Controller marks WFE Failed (ConfigurationError) when dep has empty data | P0 | Integration | IT-WE-006-004 | Implemented |
| BR-WE-014 | Controller creates Job without volumes when no dependencies | P0 | Integration | IT-WE-006-005 | Implemented |
| BR-WE-014 | Full pipeline: Job mounts + post-registration drift causes Failed | P0 | E2E | E2E-WE-006-001/002 | Implemented |
| BR-WE-014 | Full pipeline: workflow without deps creates Job without dep volumes | P0 | E2E | E2E-WE-006-003 | Implemented |
| BR-WE-014 | Full pipeline: Tekton PipelineRun gets workspace binding for deps | P0 | E2E | E2E-WE-006-004 | Implemented |

### Status Legend

- Pending: Specification complete, implementation not started
- Pass: Implemented and unit tests passing
- Implemented: Test code written and compiles; awaiting IT/E2E infrastructure run
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: DS (Data Storage), WE (WorkflowExecution)
- **BR_NUMBER**: 006 (DD-WE-006 is the primary authority)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

**ID Ranges**:
- UT-DS-006-001 to 021: Schema parsing and ExtractDependencies
- UT-DS-006-030 to 039: K8sDependencyValidator
- UT-WE-006-001 to 008: OgenWorkflowQuerier
- UT-WE-006-020 to 023: Job executor dependencies
- UT-WE-006-024 to 027: Tekton executor dependencies
- IT-DS-006-001 to 006: DS registration-time validation
- IT-WE-006-001 to 005: WFE controller dependency resolution
- E2E-WE-006-001 to 003: Full pipeline with dependencies

### Tier 1: Unit Tests

**Testable code scope**: ~367 lines across 7 files, target >=80%

#### Existing Tests (37 passing)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-006-001` | Workflow author declares secrets and configMaps and DS correctly parses them | Pass |
| `UT-DS-006-002` | Existing workflows without dependencies continue to register (backward compat) | Pass |
| `UT-DS-006-003` | Workflow author can declare empty dependencies without error | Pass |
| `UT-DS-006-004` | Workflow author can declare only secrets without configMaps | Pass |
| `UT-DS-006-005` | Workflow author can declare only configMaps without secrets | Pass |
| `UT-DS-006-006` | Workflow author can declare multiple resources of each type | Pass |
| `UT-DS-006-010` | Workflow author gets clear error when secret name is missing | Pass |
| `UT-DS-006-011` | Workflow author gets clear error when configMap name is missing | Pass |
| `UT-DS-006-012` | Workflow author gets clear error identifying the duplicated secret | Pass |
| `UT-DS-006-013` | Workflow author gets clear error identifying the duplicated configMap | Pass |
| `UT-DS-006-014` | Same resource name in different categories does not conflict | Pass |
| `UT-DS-006-020` | WFE can extract dependency list from a parsed schema | Pass |
| `UT-DS-006-021` | No-dependency workflows produce nil (no unnecessary processing) | Pass |
| `UT-DS-006-030` | Registration proceeds when all declared secrets exist with data | Pass |
| `UT-DS-006-031` | Registration is rejected and operator told which secret is missing | Pass |
| `UT-DS-006-032` | Registration is rejected for secret with no data (empty placeholder) | Pass |
| `UT-DS-006-033` | Registration proceeds when all declared configMaps exist with data | Pass |
| `UT-DS-006-034` | Registration is rejected and operator told which configMap is missing | Pass |
| `UT-DS-006-035` | Registration is rejected for configMap with no data | Pass |
| `UT-DS-006-036` | No dependencies means no K8s validation needed (nil passthrough) | Pass |
| `UT-DS-006-037` | Mixed secrets and configMaps validated together in single pass | Pass |
| `UT-DS-006-038` | ConfigMap with only binary data (certs/keys) is accepted as non-empty | Pass |
| `UT-WE-006-001` | WFE discovers secret dependencies from DS workflow content | Pass |
| `UT-WE-006-002` | WFE discovers both secrets and configMaps from DS workflow content | Pass |
| `UT-WE-006-003` | No-dependency workflows produce nil deps (no volumes/workspaces added) | Pass |
| `UT-WE-006-004` | Invalid workflow UUID is rejected early with clear error | Pass |
| `UT-WE-006-005` | Missing workflow produces clear "not found" error | Pass |
| `UT-WE-006-006` | DS connectivity failure is reported with clear error wrapping | Pass |
| `UT-WE-006-007` | Empty workflow content handled gracefully (nil deps, no crash) | Pass |
| `UT-WE-006-020` | Workflow container receives secret data at `/run/kubernaut/secrets/<name>` (read-only) | Pass |
| `UT-WE-006-021` | Workflow container receives configMap data at `/run/kubernaut/configmaps/<name>` (read-only) | Pass |
| `UT-WE-006-022` | Workflow container receives both secret and configMap at their correct paths | Pass (strengthen) |
| `UT-WE-006-023` | Job created without additional volumes when workflow has no dependencies | Pass |
| `UT-WE-006-024` | Tekton pipeline can access secret via workspace `secret-<name>` | Pass |
| `UT-WE-006-025` | Tekton pipeline can access configMap via workspace `configmap-<name>` | Pass |
| `UT-WE-006-026` | PipelineRun created without workspaces when workflow has no dependencies | Pass |

#### New/Modified Tests (4 new + 1 strengthen)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-006-039` | Operator is told exactly which resource failed when one secret passes but a configMap is missing | Pass |
| `UT-WE-006-008` | WFE gets clear parse error (not panic) when DS returns malformed YAML | Pass |
| `UT-WE-006-022` | (STRENGTHEN) Assert both volume types are present with correct names and mount paths, not just count | Pass |
| `UT-WE-006-027` | Tekton PipelineRun receives both secret and configMap workspace bindings simultaneously | Pass |

**Note on ID renumbering**: Executor dependency tests were renumbered from UT-WE-006-001..004/010..012 to UT-WE-006-020..027 to resolve collision with querier test IDs.

### Tier 2: Integration Tests

**Testable code scope**: ~126 lines across 5 files, target >=80%

#### DS Integration Tests (6 tests)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-006-001` | Operator registers a workflow with dependencies and DS accepts it (real PostgreSQL + real K8s validation) | Pending |
| `IT-DS-006-002` | Operator gets registration rejection naming the missing Secret | Pending |
| `IT-DS-006-003` | Operator gets registration rejection for Secret with empty data | Pending |
| `IT-DS-006-004` | Operator gets registration rejection naming the missing ConfigMap | Pending |
| `IT-DS-006-005` | Operator gets registration rejection for ConfigMap with empty data | Pending |
| `IT-DS-006-006` | Existing workflows without dependencies register normally (backward compat via real HTTP) | Pending |

#### WE Integration Tests (5 tests)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-WE-006-001` | Controller creates Job with secret volume mounts after querying DS and validating deps | Pending |
| `IT-WE-006-002` | Controller creates Job with configMap volume mounts after querying DS and validating deps | Pending |
| `IT-WE-006-003` | Controller marks WFE as Failed with ConfigurationError when declared dependency is missing | Pending |
| `IT-WE-006-004` | Controller marks WFE as Failed with ConfigurationError when declared dependency has empty data | Pending |
| `IT-WE-006-005` | Controller creates Job without additional volumes when workflow has no dependencies | Pending |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-WE-006-001` | Full remediation pipeline: register workflow with deps, create WFE, Job has correct volume mounts | Pending |
| `E2E-WE-006-002` | Full pipeline failure: missing dependency causes WFE Failed with ConfigurationError, no Job created | Pending |
| `E2E-WE-006-003` | Regression: workflow without dependencies proceeds through full pipeline normally | Pending |

### Tier Skip Rationale

No tiers are skipped. All three tiers are covered.

---

## 6. Test Cases (Detail)

### New/Modified Unit Tests

#### UT-DS-006-039: Validator reports specific failing resource in mixed pass/fail

**BR**: DD-WE-006
**Type**: Unit
**File**: `test/unit/datastorage/dependency_validator_test.go`

**Given**: A `K8sDependencyValidator` with a K8s client where:
  - Secret `gitea-repo-creds` exists with non-empty data
  - ConfigMap `missing-config` does NOT exist
**When**: `ValidateDependencies` is called with dependencies declaring both resources
**Then**: Error is returned that names `missing-config` specifically

**Acceptance Criteria**:
- Error message contains the name of the missing ConfigMap (`missing-config`)
- Error message contains `not found` or `configMap`
- The existing secret does NOT appear in the error message

---

#### UT-WE-006-008: Querier returns parse error for malformed YAML content

**BR**: DD-WE-006
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Given**: A mock DS client that returns a `RemediationWorkflow` with `Content: "{{invalid yaml"`
**When**: `GetWorkflowDependencies` is called
**Then**: An error is returned indicating a parse/YAML failure (not a panic or nil)

**Acceptance Criteria**:
- Error is non-nil
- Error message indicates YAML or parse failure
- No panic occurs

---

#### UT-WE-006-022: (STRENGTHEN) Job mounts both secrets and configMaps at correct paths

**BR**: BR-WE-014
**Type**: Unit
**File**: `test/unit/workflowexecution/executor_test.go`

**Given**: A Job executor with `CreateOptions` containing both a secret (`gitea-repo-creds`) and configMap (`remediation-config`)
**When**: `Create` is called
**Then**: The Job has exactly 2 volumes and 2 volume mounts, with:
  - Volume `secret-gitea-repo-creds` mounted at `/run/kubernaut/secrets/gitea-repo-creds` (read-only)
  - Volume `configmap-remediation-config` mounted at `/run/kubernaut/configmaps/remediation-config` (read-only)

**Acceptance Criteria**:
- Both volume names are asserted explicitly (not just count)
- Both mount paths are asserted explicitly
- Both ReadOnly flags are asserted as true

---

#### UT-WE-006-027: Tekton adds both secret and configMap workspaces

**BR**: BR-WE-014
**Type**: Unit
**File**: `test/unit/workflowexecution/executor_test.go`

**Given**: A Tekton executor with `CreateOptions` containing both a secret (`gitea-repo-creds`) and configMap (`remediation-config`)
**When**: `Create` is called
**Then**: The PipelineRun has exactly 2 workspace bindings:
  - Workspace `secret-gitea-repo-creds` backed by Secret `gitea-repo-creds`
  - Workspace `configmap-remediation-config` backed by ConfigMap `remediation-config`

**Acceptance Criteria**:
- Both workspace names are asserted explicitly
- Both backing resource names are asserted explicitly
- Workspace count is exactly 2

---

### DS Integration Tests

#### IT-DS-006-001: Registration succeeds when all deps exist

**BR**: DD-WE-006, BR-WORKFLOW-004
**Type**: Integration
**File**: `test/integration/datastorage/workflow_dependency_validation_test.go`

**Given**: A running DS server (in-process via `httptest.Server` + `server.NewServer`) with a real `K8sDependencyValidator` backed by envtest, and a Secret `gitea-repo-creds` with data `{"username": "kubernaut"}` in namespace `kubernaut-workflows`
**When**: A workflow registration request is sent with schema declaring `dependencies.secrets: [{name: gitea-repo-creds}]`
**Then**: Registration succeeds (HTTP 201) and the workflow is stored in the catalog

**Acceptance Criteria**:
- HTTP response status is 201
- Workflow can be retrieved by ID from catalog
- No error in response body

---

#### IT-DS-006-002: Registration fails when Secret missing

**BR**: DD-WE-006, BR-WORKFLOW-004
**Type**: Integration
**File**: `test/integration/datastorage/workflow_dependency_validation_test.go`

**Given**: A running DS server with `K8sDependencyValidator`, and NO Secret `missing-secret` in namespace `kubernaut-workflows`
**When**: A workflow registration request is sent with schema declaring `dependencies.secrets: [{name: missing-secret}]`
**Then**: Registration fails (HTTP 400 or 422) with error message naming `missing-secret`

**Acceptance Criteria**:
- HTTP response status indicates client error (4xx)
- Response body error message contains `missing-secret`
- No workflow is stored in catalog

---

#### IT-DS-006-003: Registration fails when Secret has empty data

**BR**: DD-WE-006, BR-WORKFLOW-004
**Type**: Integration
**File**: `test/integration/datastorage/workflow_dependency_validation_test.go`

**Given**: A running DS server with `K8sDependencyValidator`, and a Secret `empty-secret` with empty `Data: {}` in namespace `kubernaut-workflows`
**When**: A workflow registration request is sent with schema declaring `dependencies.secrets: [{name: empty-secret}]`
**Then**: Registration fails with error message indicating empty data

**Acceptance Criteria**:
- HTTP response status indicates client error (4xx)
- Response body error message contains `empty-secret` and `empty`
- No workflow is stored in catalog

---

#### IT-DS-006-004: Registration fails when ConfigMap missing

**BR**: DD-WE-006, BR-WORKFLOW-004
**Type**: Integration
**File**: `test/integration/datastorage/workflow_dependency_validation_test.go`

**Given**: A running DS server with `K8sDependencyValidator`, and NO ConfigMap `missing-cm` in namespace `kubernaut-workflows`
**When**: A workflow registration request is sent with schema declaring `dependencies.configMaps: [{name: missing-cm}]`
**Then**: Registration fails with error message naming `missing-cm`

**Acceptance Criteria**:
- HTTP response status indicates client error (4xx)
- Response body error message contains `missing-cm`

---

#### IT-DS-006-005: Registration fails when ConfigMap has empty data

**BR**: DD-WE-006, BR-WORKFLOW-004
**Type**: Integration
**File**: `test/integration/datastorage/workflow_dependency_validation_test.go`

**Given**: A running DS server with `K8sDependencyValidator`, and a ConfigMap `empty-cm` with empty `Data: {}` and empty `BinaryData: {}` in namespace `kubernaut-workflows`
**When**: A workflow registration request is sent with schema declaring `dependencies.configMaps: [{name: empty-cm}]`
**Then**: Registration fails with error message indicating empty data

**Acceptance Criteria**:
- HTTP response status indicates client error (4xx)
- Response body error message contains `empty-cm` and `empty`

---

#### IT-DS-006-006: Registration succeeds for workflow without dependencies

**BR**: BR-WORKFLOW-004
**Type**: Integration
**File**: `test/integration/datastorage/workflow_dependency_validation_test.go`

**Given**: A running DS server with `K8sDependencyValidator` (no Secrets/ConfigMaps needed)
**When**: A workflow registration request is sent with schema that has NO `dependencies` section
**Then**: Registration succeeds (HTTP 201)

**Acceptance Criteria**:
- HTTP response status is 201
- No dependency validation errors
- Backward compatibility confirmed

---

### WE Integration Tests

#### IT-WE-006-001: Controller creates Job with secret volume mounts

**BR**: BR-WE-014, DD-WE-006
**Type**: Integration
**File**: `test/integration/workflowexecution/dependency_resolution_integration_test.go`

**Given**: EnvTest with WFE controller running, `WorkflowQuerier` returning dependencies `{secrets: [{name: gitea-repo-creds}]}`, `DependencyValidator` passing (Secret exists in envtest), and a Secret `gitea-repo-creds` created in `kubernaut-workflows` namespace
**When**: A WFE with `executionEngine: "job"` is created
**Then**: Controller creates a Job with volume `secret-gitea-repo-creds` mounted at `/run/kubernaut/secrets/gitea-repo-creds`

**Acceptance Criteria**:
- Job is created in `kubernaut-workflows` namespace
- Job has volume `secret-gitea-repo-creds` with Secret source
- Container has volume mount at `/run/kubernaut/secrets/gitea-repo-creds` (read-only)
- WFE transitions to Running phase

---

#### IT-WE-006-002: Controller creates Job with configMap volume mounts

**BR**: BR-WE-014, DD-WE-006
**Type**: Integration
**File**: `test/integration/workflowexecution/dependency_resolution_integration_test.go`

**Given**: EnvTest with WFE controller, `WorkflowQuerier` returning `{configMaps: [{name: remediation-config}]}`, ConfigMap `remediation-config` exists in `kubernaut-workflows`
**When**: A WFE with `executionEngine: "job"` is created
**Then**: Controller creates a Job with volume `configmap-remediation-config` mounted at `/run/kubernaut/configmaps/remediation-config`

**Acceptance Criteria**:
- Job has volume with ConfigMap source
- Container has correct mount path (read-only)
- WFE transitions to Running phase

---

#### IT-WE-006-003: Controller marks WFE Failed when dependency missing

**BR**: DD-WE-006
**Type**: Integration
**File**: `test/integration/workflowexecution/dependency_resolution_integration_test.go`

**Given**: EnvTest with WFE controller, `WorkflowQuerier` returning `{secrets: [{name: missing-secret}]}`, NO Secret `missing-secret` in `kubernaut-workflows`
**When**: A WFE with `executionEngine: "job"` is created
**Then**: WFE is marked as Failed with `FailureDetails.Reason: "ConfigurationError"` and message naming `missing-secret`. No Job is created.

**Acceptance Criteria**:
- WFE phase is `Failed`
- `FailureDetails.Reason` is `ConfigurationError`
- `FailureDetails.Message` contains `missing-secret`
- No Job exists in `kubernaut-workflows` for this WFE

---

#### IT-WE-006-004: Controller marks WFE Failed when dependency has empty data

**BR**: DD-WE-006
**Type**: Integration
**File**: `test/integration/workflowexecution/dependency_resolution_integration_test.go`

**Given**: EnvTest with WFE controller, `WorkflowQuerier` returning `{secrets: [{name: empty-secret}]}`, Secret `empty-secret` exists with `Data: {}` in `kubernaut-workflows`
**When**: A WFE with `executionEngine: "job"` is created
**Then**: WFE is marked as Failed with `FailureDetails.Reason: "ConfigurationError"` and message indicating empty data. No Job is created.

**Acceptance Criteria**:
- WFE phase is `Failed`
- `FailureDetails.Reason` is `ConfigurationError`
- `FailureDetails.Message` contains `empty`
- No Job exists in `kubernaut-workflows` for this WFE

---

#### IT-WE-006-005: Controller creates Job without volumes when no dependencies

**BR**: BR-WE-014
**Type**: Integration
**File**: `test/integration/workflowexecution/dependency_resolution_integration_test.go`

**Given**: EnvTest with WFE controller, `WorkflowQuerier` returning nil dependencies
**When**: A WFE with `executionEngine: "job"` is created
**Then**: Controller creates a Job with no additional volumes (backward compatible)

**Acceptance Criteria**:
- Job is created in `kubernaut-workflows`
- Job has no dependency-related volumes
- WFE transitions to Running phase
- Existing Job creation behavior is unchanged

---

### E2E Tests

#### E2E-WE-006-001: Full pipeline with dependencies

**BR**: BR-WE-014, DD-WE-006
**Type**: E2E
**File**: `test/e2e/workflowexecution/04_dependency_injection_test.go`

**Given**: Kind cluster with full stack deployed (DS, WFE controller). Secret `test-dep-secret` with data `{"token": "abc123"}` created in `kubernaut-workflows`. A workflow schema declaring `dependencies.secrets: [{name: test-dep-secret}]` registered in DS.
**When**: A WFE CR is created referencing the registered workflow
**Then**: A Job is created with volume mount `secret-test-dep-secret` at `/run/kubernaut/secrets/test-dep-secret`

**Acceptance Criteria**:
- Workflow registration succeeds via DS API
- WFE transitions to Running
- Job has correct volume and volume mount
- Job container can theoretically read from `/run/kubernaut/secrets/test-dep-secret/token`

---

#### E2E-WE-006-002: Full pipeline failure with missing dependency

**BR**: DD-WE-006
**Type**: E2E
**File**: `test/e2e/workflowexecution/04_dependency_injection_test.go`

**Given**: Kind cluster with full stack deployed. A workflow registered in DS that declares `dependencies.secrets: [{name: nonexistent-secret}]`. The Secret does NOT exist in `kubernaut-workflows` (deleted after registration or registration validation was bypassed).
**When**: A WFE CR is created referencing the registered workflow
**Then**: WFE is marked as Failed with `ConfigurationError`. No Job is created.

**Acceptance Criteria**:
- WFE phase is `Failed`
- `FailureDetails.Reason` is `ConfigurationError`
- No Job in `kubernaut-workflows` for this WFE
- Defense-in-depth validation caught the post-registration drift

---

#### E2E-WE-006-003: Full pipeline regression without dependencies

**BR**: BR-WE-014
**Type**: E2E
**File**: `test/e2e/workflowexecution/04_dependency_injection_test.go`

**Given**: Kind cluster with full stack deployed. A workflow registered in DS with NO `dependencies` section.
**When**: A WFE CR is created referencing the registered workflow
**Then**: Job is created normally without any dependency-related volumes. Existing behavior is preserved.

**Acceptance Criteria**:
- Workflow registration succeeds
- WFE transitions through normal lifecycle
- Job has no additional volumes from dependency injection
- No errors related to dependency resolution in controller logs

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: External dependencies only: `fake.NewClientBuilder()` for K8s client, `mockWorkflowCatalogClient` for DS OpenAPI client
- **Location**: `test/unit/datastorage/`, `test/unit/workflowexecution/`
- **Real business logic**: All `pkg/` code runs as-is (parsers, validators, builders)

### DS Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**:
  - Podman containers: PostgreSQL 16 (port 15433), Redis 7 (port 16379)
  - In-process `httptest.Server` via `server.NewServer(ServerDeps{...})`
  - envtest API server for `K8sDependencyValidator` (real K8s API for Secret/ConfigMap lookup)
- **Location**: `test/integration/datastorage/workflow_dependency_validation_test.go`

### WE Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks for business logic. `WorkflowQuerier` may use a test-specific implementation returning known dependencies (this is wiring configuration, not mocking business logic)
- **Infrastructure**:
  - EnvTest (etcd + kube-apiserver) with WFE + Tekton CRDs
  - Real controller manager running WFE reconciler
  - DS infrastructure (PostgreSQL + Redis + in-process DS server) via `infrastructure.StartDSBootstrap`
  - `WorkflowQuerier` and `DependencyValidator` wired into reconciler
- **Location**: `test/integration/workflowexecution/dependency_resolution_integration_test.go`

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**:
  - Kind cluster (2 nodes: control-plane + worker)
  - Full Kubernaut stack deployed (DS, WFE controller, AuthWebhook)
  - Real Secrets/ConfigMaps created via `kubectl`
  - Real workflow registration via DS API
- **Location**: `test/e2e/workflowexecution/04_dependency_injection_test.go`

---

## 8. Execution

```bash
# Unit tests (all DD-WE-006)
go test ./test/unit/datastorage/... --ginkgo.focus="DD-WE-006"
go test ./test/unit/workflowexecution/... --ginkgo.focus="DD-WE-006"

# DS Integration tests
make test-integration-datastorage
# Or focused:
go test ./test/integration/datastorage/... --ginkgo.focus="DD-WE-006"

# WE Integration tests
make test-integration-workflowexecution
# Or focused:
go test ./test/integration/workflowexecution/... --ginkgo.focus="DD-WE-006"

# WE E2E tests
make test-e2e-workflowexecution
# Or focused:
go test ./test/e2e/workflowexecution/... --ginkgo.focus="DD-WE-006"

# Specific test by ID
go test ./test/unit/datastorage/... --ginkgo.focus="UT-DS-006-039"
go test ./test/integration/workflowexecution/... --ginkgo.focus="IT-WE-006-003"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-24 | Initial test plan: 37 existing UT + 4 new UT + 1 strengthen + 11 IT + 3 E2E = 56 scenarios |
| 1.1 | 2026-02-24 | Status update: All 41 UT pass. 4 new UT implemented and passing. 11 IT implemented (awaiting infra run). 3 E2E implemented (awaiting Kind cluster run). Server refactored to use ServerDeps struct. |
| 1.2 | 2026-02-24 | Gap fixes: DS IT migrated from fake.NewClientBuilder() to envtest per TESTING_GUIDELINES.md. E2E-WE-006-001/002 merged into single atomic test (no serial ordering). E2E-WE-006-003 uses real workflow UUID. Added E2E-WE-006-004 Tekton workspace binding test. |
