# Test Plan: Ansible Execution Engine and EngineConfig Extension

**Feature**: Ansible (AWX/AAP) execution engine with discriminator-based EngineConfig and float parameter type
**Version**: 1.0
**Created**: 2026-03-02
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `feature/v1.0-bugfixes-demos`

**Authority**:
- [BR-WE-015](../../requirements/BR-WE-015-ansible-execution-engine.md): Ansible execution engine backend
- [BR-WE-016](../../requirements/BR-WE-016-engine-config-discriminator.md): EngineConfig discriminator pattern
- [BR-WORKFLOW-005](../../requirements/BR-WORKFLOW-005-float-parameter-type.md): Float parameter type

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)

---

## 1. Scope

### In Scope

- **AnsibleExecutor**: New executor implementing the Executor interface for AWX/AAP REST API integration
- **EngineConfig discriminator**: `engineConfig` field on WorkflowRef with two-phase unmarshal based on `executionEngine`
- **Float parameter type**: Adding `float` to WorkflowParameter supported types
- **Pipeline pass-through**: EngineConfig flowing through DS -> AIAnalysis -> RO -> WFE CRD
- **Schema parser**: Extraction of `engineConfig`, `bundleDigest`, ansible-specific validation
- **Engine enum cleanup**: Remove dead `lambda`/`shell` values, add `job` to schema validator

### Out of Scope

- SecretRef parameters (v1.1)
- Schema restructuring to apiVersion/kind/metadata/spec (#292)
- AWX Execution Environment configuration (admin concern, not workflow schema)
- Tekton and Job executor changes (no modifications to existing executors)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Engine name `ansible` not `aap` | Covers both AWX (open-source) and AAP (enterprise) — same REST API |
| `json.RawMessage` / `apiextensionsv1.JSON` for engineConfig | Discriminator pattern — no CRD schema changes when adding future engines |
| Mock AWX client in UT, real AWX in IT/E2E | External dependency mocked at unit level; real AWX validates execution tracking fidelity |
| Float min/max as `*float64` | Backward compatible — JSON/YAML int-to-float coercion is automatic |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (pure logic: ParseEngineConfig, buildExtraVars, status mapping, float validation, config validation)
- **Integration**: >=80% of **integration-testable** code (I/O: DS registration/retrieval, RO creator CRD creation, WFE controller dispatch)
- **E2E**: >=80% of full service code (AnsibleExecutor against real AWX, full pipeline engineConfig round-trip)

### 2-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- BR-WE-015: UT (executor logic) + IT (controller dispatch) + E2E (real AWX execution)
- BR-WE-016: UT (ParseEngineConfig, validation) + IT (DS storage/retrieval, RO pass-through)
- BR-WORKFLOW-005: UT (type validation) + IT (DS registration)

### Business Outcome Quality Bar

Tests validate **business outcomes** — behavior, correctness, and data accuracy:
- "Does the Ansible playbook execute successfully via AWX?" not "Is the HTTP client called?"
- "Does engineConfig survive the full pipeline intact?" not "Is the JSON field populated?"
- "Are float parameters validated with decimal bounds?" not "Does the validator function run?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/workflowexecution/executor/ansible.go` (NEW) | `ParseEngineConfig`, `buildExtraVars`, `mapAWXStatusToPhase`, `buildJobLaunchPayload`, `AnsibleEngineConfig` validation | ~150 |
| `pkg/datastorage/models/workflow_schema.go` | `WorkflowExecution` struct (engineConfig, bundleDigest), `WorkflowParameter` (float type) | ~40 |
| `pkg/datastorage/schema/parser.go` | `ExtractEngineConfig`, `ExtractBundleDigest`, `Validate` (ansible validation branch) | ~30 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/server/workflow_handlers.go` | `buildWorkflowFromSchema` (engineConfig population) | ~10 (delta) |
| `pkg/remediationorchestrator/creator/workflowexecution.go` | `Create` (engineConfig pass-through) | ~5 (delta) |
| `pkg/workflowexecution/executor/ansible.go` (NEW) | `Create` (AWX API call), `GetStatus` (AWX polling), `Cleanup` (AWX cancellation) | ~150 |
| WFE controller dispatch | executor registry lookup for `ansible` | ~10 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WE-015 | AnsibleExecutor launches AWX job | P1 | Unit | UT-WE-015-001 | Pending |
| BR-WE-015 | AWX status mapping to WFE phases | P1 | Unit | UT-WE-015-002 | Pending |
| BR-WE-015 | Extra vars conversion from map[string]string | P1 | Unit | UT-WE-015-003 | Pending |
| BR-WE-015 | AWX job launch payload construction | P1 | Unit | UT-WE-015-004 | Pending |
| BR-WE-015 | AWX API error handling (transient vs permanent) | P1 | Unit | UT-WE-015-005 | Pending |
| BR-WE-015 | WFE controller dispatches to AnsibleExecutor | P1 | Integration | IT-WE-015-001 | Pending |
| BR-WE-015 | Full execution lifecycle (Pending->Running->Completed) | P1 | E2E | E2E-WE-015-001 | Pending |
| BR-WE-015 | Failure scenario (playbook fails -> WFE Failed) | P1 | E2E | E2E-WE-015-002 | Pending |
| BR-WE-015 | Full pipeline Ansible test (signal -> WFE Completed) | P1 | E2E | E2E-FP-015-001 | Pending |
| BR-WE-016 | ParseEngineConfig discriminator (ansible) | P1 | Unit | UT-WE-016-001 | Pending |
| BR-WE-016 | ParseEngineConfig discriminator (tekton/job -> nil) | P1 | Unit | UT-WE-016-002 | Pending |
| BR-WE-016 | ParseEngineConfig unknown engine -> error | P1 | Unit | UT-WE-016-003 | Pending |
| BR-WE-016 | AnsibleEngineConfig validation (playbookPath required) | P1 | Unit | UT-WE-016-004 | Pending |
| BR-WE-016 | Schema parser extracts engineConfig from YAML | P1 | Unit | UT-WE-016-005 | Pending |
| BR-WE-016 | Schema parser extracts bundleDigest from YAML | P1 | Unit | UT-WE-016-006 | Pending |
| BR-WE-016 | Schema validation rejects ansible without playbookPath | P1 | Unit | UT-WE-016-007 | Pending |
| BR-WE-016 | DS stores and retrieves engineConfig in catalog | P1 | Integration | IT-WE-016-001 | Pending |
| BR-WE-016 | DS search returns engineConfig in results | P1 | Integration | IT-WE-016-002 | Pending |
| BR-WE-016 | RO creator passes engineConfig from AI to WFE CRD | P1 | Integration | IT-WE-016-003 | Pending |
| BR-WE-016 | EngineConfig round-trip (DS -> AI -> RO -> WFE -> Executor) | P1 | E2E | E2E-WE-016-001 | Pending |
| BR-WORKFLOW-005 | Float parameter type accepted in schema | P2 | Unit | UT-WF-005-001 | Pending |
| BR-WORKFLOW-005 | Float min/max validation (within bounds) | P2 | Unit | UT-WF-005-002 | Pending |
| BR-WORKFLOW-005 | Float min/max validation (out of bounds) | P2 | Unit | UT-WF-005-003 | Pending |
| BR-WORKFLOW-005 | Integer min/max backward compat after type change | P2 | Unit | UT-WF-005-004 | Pending |
| BR-WORKFLOW-005 | DS registers workflow with float parameters | P2 | Integration | IT-WF-005-001 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: WE (WorkflowExecution), WF (Workflow/DS), FP (Full Pipeline)
- **BR_NUMBER**: Business requirement number
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/workflowexecution/executor/ansible.go`, `pkg/datastorage/models/workflow_schema.go`, `pkg/datastorage/schema/parser.go` (~220 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-WE-015-001` | AnsibleExecutor correctly constructs AWX job launch request with extra_vars from workflow parameters | Pending |
| `UT-WE-015-002` | All 7 AWX job states are correctly mapped to WFE phases (pending->Pending, running->Running, successful->Completed, failed/error/canceled->Failed) | Pending |
| `UT-WE-015-003` | String parameters are coerced to typed extra_vars: "3"->3, "true"->true, "[1,2]"->[1,2], "hello"->"hello" | Pending |
| `UT-WE-015-004` | AWX job launch payload contains correct project, playbook path, inventory, and extra_vars | Pending |
| `UT-WE-015-005` | AWX API 401/403 errors produce non-retryable FailureDetails; 5xx/network errors produce retryable errors | Pending |
| `UT-WE-016-001` | ParseEngineConfig with engine="ansible" returns correctly populated AnsibleEngineConfig | Pending |
| `UT-WE-016-002` | ParseEngineConfig with engine="tekton" or "job" returns nil config (no engine-specific config needed) | Pending |
| `UT-WE-016-003` | ParseEngineConfig with unknown engine returns descriptive error | Pending |
| `UT-WE-016-004` | AnsibleEngineConfig validation requires non-empty playbookPath; inventoryName and jobTemplateName are optional | Pending |
| `UT-WE-016-005` | Schema parser extracts engineConfig from workflow-schema.yaml with engine=ansible | Pending |
| `UT-WE-016-006` | Schema parser extracts bundleDigest from explicit field and from inline @sha256: in bundle URL | Pending |
| `UT-WE-016-007` | Schema validation rejects engine=ansible when engineConfig is missing or playbookPath is empty | Pending |
| `UT-WF-005-001` | WorkflowParameter with type="float" is accepted by schema validator | Pending |
| `UT-WF-005-002` | Float parameter with value within minimum/maximum bounds passes validation | Pending |
| `UT-WF-005-003` | Float parameter with value outside minimum/maximum bounds fails validation with descriptive error | Pending |
| `UT-WF-005-004` | Existing workflows with integer minimum/maximum values remain valid after type change to *float64 | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `pkg/datastorage/server/workflow_handlers.go`, `pkg/remediationorchestrator/creator/workflowexecution.go`, WFE controller dispatch (~25 lines delta, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-WE-015-001` | WFE controller recognizes executionEngine="ansible" and dispatches to AnsibleExecutor (envtest) | Pending |
| `IT-WE-016-001` | Workflow registered with engineConfig is stored in DS catalog and can be retrieved with engineConfig intact | Pending |
| `IT-WE-016-002` | DS workflow search returns engineConfig in results alongside executionBundle | Pending |
| `IT-WE-016-003` | RO creator populates WFE spec.workflowRef.engineConfig from AIAnalysis.status.selectedWorkflow.engineConfig | Pending |
| `IT-WF-005-001` | DS accepts and stores workflow with float parameter type, retrievable with correct type and bounds | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full AnsibleExecutor + pipeline (target >=80% of service code)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-WE-015-001` | AnsibleExecutor launches real AWX job, WFE transitions Pending -> Running -> Completed with correct duration | Pending |
| `E2E-WE-015-002` | AWX playbook failure causes WFE to transition to Failed with FailureDetails populated | Pending |
| `E2E-WE-016-001` | EngineConfig survives full round-trip: DS registration -> search -> AI selection -> RO creation -> WFE CRD -> executor reads correct playbookPath | Pending |
| `E2E-FP-015-001` | Full pipeline: signal -> GW -> SP -> RO -> AI selects Ansible workflow -> WFE with engineConfig -> AnsibleExecutor -> AWX job -> Completed -> EA assessment | Pending |

### Tier Skip Rationale

No tiers skipped. All 3 tiers are covered.

---

## 6. Test Cases (Detail)

### UT-WE-015-001: AWX job launch request construction

**BR**: BR-WE-015
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: A WorkflowExecution CRD with executionEngine="ansible", engineConfig containing playbookPath="playbooks/restart.yml", and parameters TARGET_NAMESPACE="default", TARGET_DEPLOYMENT="my-app"
**When**: AnsibleExecutor.Create is called with a mock AWX client
**Then**: The mock receives a job launch request with extra_vars containing {"target_namespace": "default", "target_deployment": "my-app"} and the correct playbook path

**Acceptance Criteria**:
- extra_vars JSON contains all parameters with correct types
- Playbook path from engineConfig is used in the AWX project/template configuration
- AWX job ID is stored for subsequent status polling

### UT-WE-015-002: AWX status mapping completeness

**BR**: BR-WE-015
**Type**: Unit
**File**: `test/unit/workflowexecution/ansible_executor_test.go`

**Given**: Each of the 7 AWX job states (pending, waiting, running, successful, failed, error, canceled)
**When**: mapAWXStatusToPhase is called with each state
**Then**: Correct WFE phase is returned (Pending, Pending, Running, Completed, Failed, Failed, Failed)

**Acceptance Criteria**:
- All 7 AWX states are mapped (no unknown state falls through)
- Unknown AWX states return an error rather than silently mapping

### UT-WE-016-001: ParseEngineConfig discriminator (ansible)

**BR**: BR-WE-016
**Type**: Unit
**File**: `test/unit/workflowexecution/engine_config_test.go`

**Given**: engine="ansible" and raw JSON `{"playbookPath": "playbooks/restart.yml", "inventoryName": "k8s"}`
**When**: ParseEngineConfig is called
**Then**: Returns *AnsibleEngineConfig with PlaybookPath="playbooks/restart.yml" and InventoryName="k8s"

**Acceptance Criteria**:
- Returned type is *AnsibleEngineConfig (not interface{})
- All fields correctly deserialized
- Optional fields (inventoryName, jobTemplateName) are empty strings when absent

### E2E-WE-015-001: Full execution lifecycle with real AWX

**BR**: BR-WE-015
**Type**: E2E
**File**: `test/e2e/workflowexecution/ansible_execution_test.go`

**Given**: AWX is running in the test cluster with a project pointing to the GitHub test playbook repo, and a Job Template configured
**When**: A WorkflowExecution CRD with executionEngine="ansible" is created
**Then**: WFE transitions Pending -> Running -> Completed within timeout, with duration recorded

**Acceptance Criteria**:
- WFE.Status.Phase reaches "Completed"
- WFE.Status.Duration is populated and > 0
- WFE.Status.ExecutionRef references the AWX job
- AWX job status is "successful" when queried directly

### E2E-FP-015-001: Full pipeline Ansible test

**BR**: BR-WE-015
**Type**: E2E (Full Pipeline)
**File**: `test/e2e/fullpipeline/ansible_workflow_test.go`

**Given**: An Ansible workflow is registered in DS with engine=ansible and engineConfig, AWX is running in the FP Kind cluster, mock LLM is configured to select the Ansible workflow
**When**: A Prometheus alert triggers a remediation signal
**Then**: The full pipeline executes: GW -> SP -> RR -> AI -> WFE(ansible) -> AWX job -> Completed -> EA

**Acceptance Criteria**:
- RemediationRequest reaches Completed status
- WorkflowExecution has executionEngine="ansible" and engineConfig populated
- AWX job executed successfully
- EffectivenessAssessment is created

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: AWX REST API client interface (external dependency)
- **Location**: `test/unit/workflowexecution/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**: envtest (K8s API), real PostgreSQL + Redis (DS), real DataStorage service
- **Location**: `test/integration/workflowexecution/`, `test/integration/datastorage/`, `test/integration/remediationorchestrator/`

### E2E WE Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Real AWX (shared PG + Redis from DS), DS service, WE controller
- **AWX setup**: AWX operator deployed via Helm, sharing PostgreSQL (separate `awx` database) and Redis (DB 1)
- **Test playbook**: GitHub repo with pinned commit SHA
- **Location**: `test/e2e/workflowexecution/`

### E2E Full Pipeline Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Full Kubernaut stack + AWX (shared PG + Redis, tight resource limits 128Mi-256Mi)
- **AWX overhead**: ~576Mi within existing 1.5GB buffer
- **Location**: `test/e2e/fullpipeline/`

---

## 8. Execution

```bash
# Unit tests
make test

# Specific unit test by ID
go test ./test/unit/workflowexecution/... -ginkgo.focus="UT-WE-015-001"

# Integration tests (DS)
make test-integration-datastorage

# Integration tests (WE)
make test-integration-workflowexecution

# Integration tests (RO)
make test-integration-remediationorchestrator

# E2E WE tests
make test-e2e-workflowexecution

# E2E Full Pipeline tests
make test-e2e-fullpipeline
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-02 | Initial test plan |
