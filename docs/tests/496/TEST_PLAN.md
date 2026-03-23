# Test Plan: BR-496 v2 — HAPI-Owned Target Resource Identity

**Feature**: Eliminate affectedResource mismatch by having HAPI own target resource identity via standardized TARGET_RESOURCE_* parameters
**Version**: 1.2
**Created**: 2026-03-04
**Author**: AI Assistant (Cursor)
**Status**: Draft
**Branch**: `fix/496-hapi-owned-target`

**Authority**:
- [BR-496]: affectedResource mismatch detection and prevention
- [DD-HAPI-006 v1.4]: affectedResource in RCA — HAPI-owned via canonical params
- [DD-WORKFLOW-003]: Parameterized Actions — TARGET_RESOURCE_* mandatory convention
- [ADR-043]: Workflow Schema Definition Standard

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [kubernaut-demo-scenarios#170](https://github.com/jordigilh/kubernaut-demo-scenarios/issues/170) — production workflow migration
- [kubernaut-docs#34](https://github.com/jordigilh/kubernaut-docs/issues/34) — authoring guide

---

## 1. Scope

### In Scope

- **HAPI target injection** (`llm_integration.py`): Post-loop injection of TARGET_RESOURCE_* from `session_state["root_owner"]` into workflow parameters, construction of `affectedResource` for Go backward compat, `rca_incomplete` when root_owner missing
- **Schema stripping** (`workflow_discovery.py`): Removal of TARGET_RESOURCE_* from `get_workflow` tool response before LLM sees it
- **Schema validation** (`workflow_response_validator.py`): Step 0 — reject workflows missing canonical params; Step 3 — skip required-check for HAPI-managed params
- **Prompt update** (`prompt_builder.py`): Removal of `affectedResource` instructions from Phase 3b and JSON examples
- **Parser update** (`result_parser.py`): Removal of BR-HAPI-212 `rca_incomplete` check for missing `affectedResource` from LLM output
- **Test fixture migration**: All 33 workflow schemas standardized to TARGET_RESOURCE_NAME / TARGET_RESOURCE_KIND / TARGET_RESOURCE_NAMESPACE
- **Mock LLM update**: Stop emitting TARGET_RESOURCE_* and `affectedResource` in responses

### Out of Scope

- **Go controller behavioral changes**: `affectedResource` remains in HAPI response (HAPI-derived from root_owner); Go reads it unchanged
- **CRD `AffectedResource` struct**: Unchanged — same Kind/Name/Namespace fields
- **Rego policy changes**: Rego input already receives structured `affected_resource` with kind/name/namespace
- **DataStorage enforcement**: TARGET_RESOURCE_* are a convention, not enforced by DS validation
- **Production workflow migration**: Handled by kubernaut-demo-scenarios team (issue #170)
- **Go `mapEnumToSubReason` for `rca_incomplete`**: Pre-existing gap (v1.2 follow-up); `rca_incomplete` is stored in `HumanReviewReason` but has no `SubReason` mapping

### In Scope (CRD Enum Cleanup)

- **Go CRD HumanReviewReason enum**: Remove defunct `affectedResource_mismatch` and `unverified_target_resource` values from `aianalysis_types.go` kubebuilder annotation and regenerate manifests
- **HAPI Python enum**: Remove `AFFECTED_RESOURCE_MISMATCH` and `UNVERIFIED_TARGET_RESOURCE` from `HumanReviewReason` in `incident_models.py`
- **HAPI Pydantic validator**: Remove defunct values from `incident_response_data.py`
- **HAPI OpenAPI**: Remove defunct values from `openapi.json`
- **Rationale**: No production deployment exists with these values (only rc6). Clean removal prevents dead code accumulation.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| HAPI strips canonical params from LLM-visible schema | LLM cannot produce inconsistent target identity if it never sees the target fields |
| HAPI injects from K8s-verified root_owner | root_owner is resolved via owner chain (Pod → ReplicaSet → Deployment); more reliable than LLM interpretation |
| affectedResource still in HAPI response | Zero Go changes; CRD, Rego, RO all continue reading same struct unchanged |
| Validator rejects workflows missing canonical params | Forces migration; LLM self-correction loop can select a different workflow |
| rca_incomplete replaces unverified_target_resource | Cleaner semantics: mandatory TARGET_RESOURCE_NAME/KIND cannot be populated |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` — Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (injection logic, stripping logic, validation logic, prompt construction, parser changes)
- **E2E**: >=80% of full pipeline (alert → HAPI → mock LLM → tool calls → injection → Go controller → CRD status)

### 2-Tier Minimum

- **Unit tests** (Python pytest): Catch logic and correctness errors in injection, stripping, validation, and prompt changes
- **E2E tests** (Go Ginkgo in Kind): Validate the full pipeline produces correct `affectedResource` and `TARGET_RESOURCE_*` in the CRD/WFE

### Business Outcome Quality Bar

Tests validate **business outcomes**:
- Operator sees correct `affectedResource` in CRD status (derived from K8s, not LLM)
- Workflow execution receives correct TARGET_RESOURCE_* parameters
- Invalid workflows (missing canonical params) are rejected with actionable error
- Investigation escalates to human review when target cannot be determined (root_owner missing)

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `holmesgpt-api/src/extensions/incident/llm_integration.py` | `_inject_target_resource` (new), removal of `_check_affected_resource_mismatch`, `_affected_resource_matches` | ~70 new, ~70 removed |
| `holmesgpt-api/src/validation/workflow_response_validator.py` | `_validate_canonical_params` (new Step 0), `_validate_parameters` (HAPI_MANAGED_PARAMS skip) | ~30 new, ~5 modified |
| `holmesgpt-api/src/extensions/incident/prompt_builder.py` | `create_incident_investigation_prompt` (Phase 3b simplification, JSON examples) | ~40 modified |
| `holmesgpt-api/src/extensions/incident/result_parser.py` | `parse_and_validate_investigation_result` (remove rca_target extraction, remove BR-HAPI-212 check) | ~15 removed |

### Integration-Testable Code (I/O, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `holmesgpt-api/src/toolsets/workflow_discovery.py` | `GetWorkflowTool._invoke` (schema stripping before `json.dumps`) | ~15 new |

### E2E-Testable Code (full pipeline)

| Component | What is tested |
|-----------|----------------|
| Full pipeline (Kind cluster) | Alert → Gateway → AA → HAPI → mock LLM → get_resource_context → get_workflow (stripped schema) → LLM response → HAPI injection → Go controller → CRD status with correct affectedResource and TARGET_RESOURCE_* |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-496 | TARGET_RESOURCE_* injected from root_owner into workflow params | P0 | Unit | UT-HAPI-496-001 | Pass |
| BR-496 | affectedResource constructed from root_owner for Go compat | P0 | Unit | UT-HAPI-496-002 | Pass |
| BR-496 | rca_incomplete when root_owner missing | P0 | Unit | UT-HAPI-496-003 | Pass |
| BR-496 | No param injection when no selected_workflow | P1 | Unit | UT-HAPI-496-004 | Pass |
| BR-496 | affectedResource populated even without selected_workflow | P1 | Unit | UT-HAPI-496-005 | Pass |
| BR-496 | Operational params preserved alongside injected canonical params | P0 | Unit | UT-HAPI-496-006 | Pass |
| BR-496 | Validator rejects workflow missing TARGET_RESOURCE_NAME | P0 | Unit | UT-HAPI-496-007 | Pass |
| BR-496 | Validator rejects workflow missing TARGET_RESOURCE_KIND | P0 | Unit | UT-HAPI-496-008 | Pass |
| BR-496 | Validator rejects workflow missing TARGET_RESOURCE_NAMESPACE | P0 | Unit | UT-HAPI-496-009 | Pass |
| BR-496 | Validator passes when all 3 canonical params declared | P0 | Unit | UT-HAPI-496-010 | Pass |
| BR-496 | Required-check skipped for HAPI_MANAGED_PARAMS | P0 | Unit | UT-HAPI-496-011 | Pass |
| BR-496 | Schema stripping removes TARGET_RESOURCE_* from tool response | P0 | Unit | UT-HAPI-496-012 | Pass |
| BR-496 | Schema stripping preserves operational params | P0 | Unit | UT-HAPI-496-013 | Pass |
| BR-496 | Schema stripping handles missing parameters section | P1 | Unit | UT-HAPI-496-014 | Pass |
| BR-496 | Prompt does not contain affectedResource instructions | P1 | Unit | UT-HAPI-496-015 | Pass |
| BR-496 | Prompt instructs LLM to call get_resource_context | P1 | Unit | UT-HAPI-496-016 | Pass |
| BR-496 | Parser does not extract or validate affectedResource from LLM | P1 | Unit | UT-HAPI-496-017 | Pass |
| BR-496 | Full pipeline: CRD affectedResource matches root_owner | P0 | E2E | E2E-FP-496-001 | Pass |
| BR-496 | Full pipeline: WFE params contain injected TARGET_RESOURCE_* | P0 | E2E | E2E-FP-496-002 | Pass |
| BR-496 | Cluster-scoped resource: kind + name only, namespace omitted | P1 | Unit | UT-HAPI-496-018 | Pass |
| BR-496 | LLM-provided affectedResource unconditionally overwritten by root_owner | P1 | Unit | UT-HAPI-496-019 | Pass |
| BR-496 | Schema stripping handles malformed parameters gracefully | P1 | Unit | UT-HAPI-496-020 | Pass |
| BR-496 | HumanReviewReason enum does not contain defunct values | P0 | Unit | UT-HAPI-496-021 | Pass |
| BR-496 | Full pipeline: LLM response lacks affectedResource (HAPI adds it) | P1 | E2E | E2E-FP-496-003 | Pass |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios — TDD Execution Plan

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `E2E` (End-to-End)
- **SERVICE**: `HAPI` (HolmesGPT API), `FP` (Full Pipeline)
- **BR_NUMBER**: 496
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `llm_integration.py` (injection), `workflow_response_validator.py` (validation), `workflow_discovery.py` (stripping), `prompt_builder.py` (prompt), `result_parser.py` (parser), `incident_models.py` (enum cleanup) — target >=80% of new/modified logic

---

#### TDD Group 1: Injection Logic (`_inject_target_resource`)

**File under test**: `holmesgpt-api/src/extensions/incident/llm_integration.py`
**Test file**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-496-001` | When a workflow is selected and root_owner is in session_state, TARGET_RESOURCE_NAME/KIND/NAMESPACE are injected into workflow parameters from the K8s-verified root_owner |
| `UT-HAPI-496-002` | affectedResource is constructed in RCA from root_owner so Go controller, Rego policies, and RO can read the target without code changes |
| `UT-HAPI-496-003` | When root_owner is missing from session_state (LLM never called get_resource_context), investigation is flagged rca_incomplete with needs_human_review=True |
| `UT-HAPI-496-004` | When no workflow is selected (resolved/inconclusive), no TARGET_RESOURCE_* params are injected (no parameters dict to inject into) |
| `UT-HAPI-496-005` | When no workflow is selected but root_owner is available, affectedResource is still populated in RCA for observability |
| `UT-HAPI-496-006` | LLM-provided operational parameters (e.g., MEMORY_LIMIT_NEW) are preserved unchanged when HAPI injects canonical params alongside them |
| `UT-HAPI-496-018` | Cluster-scoped resource (Node): root_owner has kind + name only (no namespace); affectedResource omits namespace field; TARGET_RESOURCE_NAMESPACE not injected |
| `UT-HAPI-496-019` | When LLM still provides affectedResource in its output, HAPI unconditionally overwrites it with root_owner values (injection is authoritative, not additive) |

All 8 tests fail because `_inject_target_resource` does not exist yet.

##### GREEN Phase — Minimal implementation

Implement `_inject_target_resource(result, session_state, remediation_id)` in `llm_integration.py`:
1. Read `root_owner` from `session_state`
2. If `root_owner` is None: set `needs_human_review=True`, `human_review_reason="rca_incomplete"`; return
3. Construct `affectedResource` dict from `root_owner` (kind + name mandatory, namespace if present); set in `result["root_cause_analysis"]["affectedResource"]` — overwrites any LLM value
4. If `selected_workflow` exists: inject `TARGET_RESOURCE_NAME`, `TARGET_RESOURCE_KIND` into `result["selected_workflow"]["parameters"]`; inject `TARGET_RESOURCE_NAMESPACE` only if `root_owner` has namespace

Also: remove `_affected_resource_matches` and `_check_affected_resource_mismatch` functions and their call site (line 569). Replace with call to `_inject_target_resource`.

All 8 tests pass.

##### REFACTOR Phase

- Extract `CANONICAL_PARAMS` constant for reuse across injection and validation
- Add structured logging for injection events
- Ensure `_inject_target_resource` is idempotent (safe if called twice)

---

#### TDD Group 2: Schema Validation (`WorkflowResponseValidator`)

**File under test**: `holmesgpt-api/src/validation/workflow_response_validator.py`
**Test file**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-496-007` | Workflow missing TARGET_RESOURCE_NAME is rejected with actionable error directing LLM to select a different workflow |
| `UT-HAPI-496-008` | Workflow missing TARGET_RESOURCE_KIND is rejected with actionable error |
| `UT-HAPI-496-009` | Workflow missing TARGET_RESOURCE_NAMESPACE is rejected with actionable error |
| `UT-HAPI-496-010` | Workflow declaring all 3 canonical params passes Step 0 validation |
| `UT-HAPI-496-011` | LLM response without TARGET_RESOURCE_* values passes Step 3 required-check (HAPI provides them, not LLM) |

All 5 tests fail because Step 0 does not exist and Step 3 does not skip HAPI-managed params.

##### GREEN Phase — Minimal implementation

1. **Step 0 (new)**: After fetching the workflow schema, check that `TARGET_RESOURCE_NAME`, `TARGET_RESOURCE_KIND`, `TARGET_RESOURCE_NAMESPACE` are declared. If any missing, return `ValidationResult(is_valid=False, errors=[...])`.
2. **Step 3 (modify)**: Define `HAPI_MANAGED_PARAMS = {"TARGET_RESOURCE_NAME", "TARGET_RESOURCE_KIND", "TARGET_RESOURCE_NAMESPACE"}`. Skip `required`-check for params in this set.

All 5 tests pass.

##### REFACTOR Phase

- Share `HAPI_MANAGED_PARAMS` constant with injection logic (import from shared location)
- Ensure validation error message is actionable ("select a different workflow")

---

#### TDD Group 3: Schema Stripping (`GetWorkflowTool`)

**File under test**: `holmesgpt-api/src/toolsets/workflow_discovery.py`
**Test file**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-496-012` | get_workflow tool response does not contain TARGET_RESOURCE_NAME, TARGET_RESOURCE_KIND, or TARGET_RESOURCE_NAMESPACE in the parameters schema shown to the LLM |
| `UT-HAPI-496-013` | get_workflow tool response preserves all operational parameters (e.g., MEMORY_LIMIT_NEW, REPLICA_COUNT) in the schema shown to the LLM |
| `UT-HAPI-496-014` | get_workflow tool response handles workflows with empty or missing parameters section without error |
| `UT-HAPI-496-020` | Schema stripping handles malformed parameters (e.g., parameters is a string, or nested unexpected structure) without error |

All 4 tests fail because `get_workflow` returns the full schema including canonical params.

##### GREEN Phase — Minimal implementation

In `GetWorkflowTool._invoke`: after receiving the DS response and before returning to the LLM, parse the JSON, filter out parameters named `TARGET_RESOURCE_NAME`, `TARGET_RESOURCE_KIND`, `TARGET_RESOURCE_NAMESPACE` from `parameters.schema.parameters`, reserialize. Handle missing/malformed parameters gracefully (return as-is).

All 4 tests pass.

##### REFACTOR Phase

- Extract stripping logic into a helper function for testability
- Add logging when parameters are stripped (count stripped vs retained)

---

#### TDD Group 4: Prompt and Parser Cleanup

**Files under test**: `holmesgpt-api/src/extensions/incident/prompt_builder.py`, `result_parser.py`
**Test file**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-496-015` | Investigation prompt does not instruct the LLM to set affectedResource in its JSON output (comprehensive search across entire prompt) |
| `UT-HAPI-496-016` | Investigation prompt instructs the LLM to call get_resource_context for remediation_history |
| `UT-HAPI-496-017` | LLM response missing affectedResource does not trigger rca_incomplete in the parser (HAPI provides it post-loop) |

UT-015 fails because prompt still contains 15+ `affectedResource` references. UT-017 fails because parser still has BR-HAPI-212 check.

##### GREEN Phase — Minimal implementation

1. **prompt_builder.py**: Remove all `affectedResource` instructions from Phase 3b. Remove `affectedResource` from all JSON example blocks (success, failure, validation error templates). Keep `get_resource_context` instruction for remediation history.
2. **result_parser.py**: Remove `rca_target` extraction (line 271-272). Remove BR-HAPI-212 conditional (lines 409-422) that triggers `rca_incomplete` for missing `affectedResource`.

All 3 tests pass.

##### REFACTOR Phase

- Verify no orphaned references to `affectedResource` remain in prompt templates
- Simplify parser logic now that `rca_target` extraction is removed

---

#### TDD Group 5: Enum Cleanup

**Files under test**: `holmesgpt-api/src/models/incident_models.py`, `incident_response_data.py`, `openapi.json`, `aianalysis_types.go`
**Test file**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

##### RED Phase — Write failing test

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `UT-HAPI-496-021` | HumanReviewReason enum does not contain affectedResource_mismatch or unverified_target_resource (CRD enum cleanup) |

Test fails because enum still contains both defunct values.

##### GREEN Phase — Minimal implementation

1. `incident_models.py`: Remove `AFFECTED_RESOURCE_MISMATCH` and `UNVERIFIED_TARGET_RESOURCE` from `HumanReviewReason`
2. `incident_response_data.py`: Remove from `human_review_reason_validate_enum`
3. `openapi.json`: Remove from `HumanReviewReason` enum
4. `aianalysis_types.go`: Remove from `+kubebuilder:validation:Enum` annotation
5. Run `make manifests` to regenerate CRD yaml files

Test passes.

##### REFACTOR Phase

- Verify no remaining references to defunct values across codebase
- Run `go build ./...` to confirm Go compilation

---

### Tier 3: E2E Tests

**Testable code scope**: Full pipeline in Kind cluster with mock LLM — target >=80% of the injection/stripping integration path

**Prerequisite**: All unit test groups (1-5) must be at GREEN before E2E tests are meaningful.

#### TDD Group 6: Full Pipeline E2E

**Test file**: `test/e2e/fullpipeline/` (extend existing)

##### RED Phase — Write failing tests

| ID | Business Outcome Under Test |
|----|-----------------------------|
| `E2E-FP-496-001` | After full pipeline execution, CRD AIAnalysis.Status.RootCauseAnalysis.AffectedResource matches the root_owner resolved by get_resource_context (K8s-verified, not LLM-provided) |
| `E2E-FP-496-002` | After full pipeline execution, WorkflowExecution.Spec.Parameters contains TARGET_RESOURCE_NAME, TARGET_RESOURCE_KIND, TARGET_RESOURCE_NAMESPACE matching the root_owner |
| `E2E-FP-496-003` | Mock LLM response does not include affectedResource in its JSON, yet the CRD has it populated (proving HAPI injection) |

Tests fail because mock LLM still emits affectedResource and old parameter names.

##### GREEN Phase — Minimal implementation

1. Update mock LLM (`server.py`): Remove `affectedResource` construction, remove `include_affected_resource` flag, remove TARGET_RESOURCE_* from scenario parameters
2. Migrate test fixture workflow schemas (33 files) to standard TARGET_RESOURCE_* names
3. Update Go test infrastructure files to use standard param names
4. Update execution artifacts (Dockerfile, remediate.sh)

All 3 E2E tests pass (along with existing E2E tests, which now implicitly validate injection).

##### REFACTOR Phase

- Verify all existing E2E tests still pass (regression)
- Post-migration grep: confirm no old param names (`DEPLOYMENT_NAME`, `POD_NAME`, `NODE_NAME`, `TARGET_CERTIFICATE`, `target_name`) remain in test fixtures

---

### Tier Skip Rationale

- **Integration (Tier 2)**: Skipped. All modified code is Python (HAPI). Python integration tests are not part of the kubernaut test infrastructure — cross-component behavior is validated by Go E2E tests in Kind. The 2-tier minimum is satisfied by Unit (pytest) + E2E (Ginkgo/Kind).

---

## 6. Test Cases (Detail)

### UT-HAPI-496-001: TARGET_RESOURCE_* injected from root_owner

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A result dict with `selected_workflow` containing `parameters: {"MEMORY_LIMIT_NEW": "512Mi"}`, and `session_state` containing `root_owner: {kind: "Deployment", name: "postgres-emptydir", namespace: "demo"}`
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: `result["selected_workflow"]["parameters"]` contains `TARGET_RESOURCE_NAME: "postgres-emptydir"`, `TARGET_RESOURCE_KIND: "Deployment"`, `TARGET_RESOURCE_NAMESPACE: "demo"` alongside existing `MEMORY_LIMIT_NEW: "512Mi"`

**Acceptance Criteria**:
- All 3 canonical params are present in parameters
- Existing operational params are not modified or removed
- Values match session_state root_owner exactly (case preserved)

### UT-HAPI-496-002: affectedResource constructed for Go backward compat

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A result dict with `root_cause_analysis: {}` and `session_state` containing `root_owner: {kind: "Deployment", name: "api-server", namespace: "production"}`
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: `result["root_cause_analysis"]["affectedResource"]` equals `{kind: "Deployment", name: "api-server", namespace: "production"}`

**Acceptance Criteria**:
- `affectedResource` key uses camelCase (Go reads `rcaMap["affectedResource"]`, not snake_case)
- Contains kind, name, namespace fields
- Values match root_owner

### UT-HAPI-496-003: rca_incomplete when root_owner missing

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A result dict with `selected_workflow` present and `session_state` with no `root_owner` key
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: `result["needs_human_review"]` is True, `result["human_review_reason"]` is `"rca_incomplete"`

**Acceptance Criteria**:
- needs_human_review is set to True
- human_review_reason is "rca_incomplete" (not "unverified_target_resource")
- No TARGET_RESOURCE_* params are injected
- No affectedResource is constructed

### UT-HAPI-496-004: No injection when no selected_workflow

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A result dict with `selected_workflow: None` and `session_state` containing `root_owner`
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: No parameters dict is created or modified; `affectedResource` is still populated in RCA from root_owner

**Acceptance Criteria**:
- result has no "selected_workflow" with parameters
- result["root_cause_analysis"]["affectedResource"] is populated

### UT-HAPI-496-005: affectedResource populated without selected_workflow

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A result dict with `selected_workflow: None`, `root_cause_analysis: {summary: "resolved"}`, and `session_state` containing `root_owner`
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: `result["root_cause_analysis"]["affectedResource"]` is populated from root_owner for observability

**Acceptance Criteria**:
- affectedResource is set even though no workflow is selected
- needs_human_review is NOT set (resolved scenario is valid without params)

### UT-HAPI-496-006: Operational params preserved

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A result dict with `selected_workflow.parameters: {"MEMORY_LIMIT_NEW": "512Mi", "REPLICA_COUNT": "3"}` and `session_state` with root_owner
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: Parameters dict contains all 5 keys: 3 canonical + 2 operational, with operational values unchanged

**Acceptance Criteria**:
- MEMORY_LIMIT_NEW is "512Mi"
- REPLICA_COUNT is "3"
- TARGET_RESOURCE_NAME, TARGET_RESOURCE_KIND, TARGET_RESOURCE_NAMESPACE are present

### UT-HAPI-496-007: Validator rejects missing TARGET_RESOURCE_NAME

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A workflow schema declaring TARGET_RESOURCE_KIND and TARGET_RESOURCE_NAMESPACE but NOT TARGET_RESOURCE_NAME
**When**: `WorkflowResponseValidator.validate(workflow_id, bundle, params)` is called
**Then**: Returns `ValidationResult(is_valid=False)` with error containing "TARGET_RESOURCE_NAME"

**Acceptance Criteria**:
- is_valid is False
- Error message identifies the missing param by name
- Error message suggests selecting a different workflow

### UT-HAPI-496-008: Validator rejects missing TARGET_RESOURCE_KIND

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A workflow schema declaring TARGET_RESOURCE_NAME and TARGET_RESOURCE_NAMESPACE but NOT TARGET_RESOURCE_KIND
**When**: `WorkflowResponseValidator.validate(workflow_id, bundle, params)` is called
**Then**: Returns `ValidationResult(is_valid=False)` with error containing "TARGET_RESOURCE_KIND"

**Acceptance Criteria**:
- is_valid is False
- Error message identifies the missing param

### UT-HAPI-496-009: Validator rejects missing TARGET_RESOURCE_NAMESPACE

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A workflow schema declaring TARGET_RESOURCE_NAME and TARGET_RESOURCE_KIND but NOT TARGET_RESOURCE_NAMESPACE
**When**: `WorkflowResponseValidator.validate(workflow_id, bundle, params)` is called
**Then**: Returns `ValidationResult(is_valid=False)` with error containing "TARGET_RESOURCE_NAMESPACE"

**Acceptance Criteria**:
- is_valid is False
- Error message identifies the missing param

### UT-HAPI-496-010: Validator passes with all canonical params

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A workflow schema declaring all 3 canonical params plus MEMORY_LIMIT_NEW
**When**: `WorkflowResponseValidator.validate(workflow_id, bundle, params)` is called with LLM params `{"MEMORY_LIMIT_NEW": "512Mi"}` (no canonical params from LLM)
**Then**: Returns `ValidationResult(is_valid=True)`

**Acceptance Criteria**:
- Step 0 passes (all canonical params declared)
- Step 3 passes (canonical params not flagged as missing despite not being in LLM response)

### UT-HAPI-496-011: Required-check skipped for HAPI-managed params

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A workflow schema with TARGET_RESOURCE_NAME (required: true), TARGET_RESOURCE_KIND (required: true), TARGET_RESOURCE_NAMESPACE (required: true), MEMORY_LIMIT_NEW (required: true)
**When**: `WorkflowResponseValidator.validate()` is called with LLM params `{"MEMORY_LIMIT_NEW": "512Mi"}` — canonical params intentionally absent
**Then**: Only MEMORY_LIMIT_NEW is validated; no "Missing required parameter" error for canonical params

**Acceptance Criteria**:
- Validation passes (is_valid=True)
- No errors about TARGET_RESOURCE_NAME, TARGET_RESOURCE_KIND, or TARGET_RESOURCE_NAMESPACE

### UT-HAPI-496-012: Schema stripping removes canonical params

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A DataStorage workflow response containing parameters.schema.parameters with 5 entries: TARGET_RESOURCE_NAME, TARGET_RESOURCE_KIND, TARGET_RESOURCE_NAMESPACE, MEMORY_LIMIT_NEW, REPLICA_COUNT
**When**: get_workflow tool processes the response before returning to LLM
**Then**: Tool response contains only MEMORY_LIMIT_NEW and REPLICA_COUNT in parameters.schema.parameters

**Acceptance Criteria**:
- 3 canonical params are absent from the tool response
- 2 operational params are present and unchanged
- Parameter descriptions and types are preserved for operational params

### UT-HAPI-496-013: Schema stripping preserves operational params

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A DataStorage workflow response with only operational params (no canonical params — edge case for legacy workflows)
**When**: get_workflow tool processes the response
**Then**: All operational params are preserved unchanged

**Acceptance Criteria**:
- No params removed (none matched canonical names)
- Tool response is identical to DS response for the parameters section

### UT-HAPI-496-014: Schema stripping handles missing parameters

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A DataStorage workflow response with no `parameters` key or empty parameters
**When**: get_workflow tool processes the response
**Then**: No error; tool response returned as-is

**Acceptance Criteria**:
- No KeyError or TypeError
- Response is returned without modification

### UT-HAPI-496-015: Prompt omits affectedResource instructions

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: Standard request_data for an incident investigation
**When**: `create_incident_investigation_prompt(request_data)` is called
**Then**: The returned prompt string does NOT contain "affectedResource" as an instruction for the LLM to set

**Acceptance Criteria**:
- Comprehensive regex search: `"affectedResource"` does not appear anywhere in the entire prompt string (covers Phase 3b, JSON examples, success/failure templates, validation error feedback)
- Phase 3b section exists but only instructs calling get_resource_context
- No remnant references in any JSON example blocks

### UT-HAPI-496-016: Prompt instructs get_resource_context

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: Standard request_data for an incident investigation
**When**: `create_incident_investigation_prompt(request_data)` is called
**Then**: The returned prompt string contains instructions to call `get_resource_context` for remediation history

**Acceptance Criteria**:
- String "get_resource_context" appears in the prompt
- String "remediation_history" or "remediation history" appears in Phase 3b context

### UT-HAPI-496-017: Parser ignores missing affectedResource

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: An investigation result containing `selected_workflow` but NO `affectedResource` in root_cause_analysis
**When**: `parse_and_validate_investigation_result()` is called
**Then**: Result does NOT have `needs_human_review=True` or `human_review_reason="rca_incomplete"` from the parser (HAPI handles this post-loop)

**Acceptance Criteria**:
- Parser returns result without rca_incomplete flag
- Parser does not set needs_human_review for missing affectedResource

### UT-HAPI-496-018: Cluster-scoped resource (Node)

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A result dict with `selected_workflow` present and `session_state` containing `root_owner: {kind: "Node", name: "worker-1"}` (no namespace key)
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: `affectedResource` contains only `kind: "Node"` and `name: "worker-1"` (no namespace field). `TARGET_RESOURCE_NAME: "worker-1"` and `TARGET_RESOURCE_KIND: "Node"` are injected. `TARGET_RESOURCE_NAMESPACE` is not injected.

**Acceptance Criteria**:
- affectedResource has exactly 2 keys: kind and name
- TARGET_RESOURCE_NAME and TARGET_RESOURCE_KIND are in parameters
- TARGET_RESOURCE_NAMESPACE is NOT in parameters
- No KeyError from missing namespace in root_owner

### UT-HAPI-496-019: LLM-provided affectedResource overwritten

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A result dict where LLM set `root_cause_analysis.affectedResource: {kind: "Pod", name: "api-xyz-123", namespace: "prod"}` and `session_state` with `root_owner: {kind: "Deployment", name: "api", namespace: "prod"}`
**When**: `_inject_target_resource(result, session_state, remediation_id)` is called
**Then**: `affectedResource` is overwritten to `{kind: "Deployment", name: "api", namespace: "prod"}` (root_owner, not LLM value)

**Acceptance Criteria**:
- affectedResource.kind is "Deployment" (not "Pod")
- affectedResource.name is "api" (not "api-xyz-123")
- needs_human_review is NOT set (no mismatch — HAPI is authoritative)

### UT-HAPI-496-020: Schema stripping handles malformed parameters

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: A DataStorage workflow response where `parameters` is a string (malformed) instead of a dict/list
**When**: get_workflow tool processes the response before returning to LLM
**Then**: No error raised; response returned as-is without modification

**Acceptance Criteria**:
- No TypeError, KeyError, or AttributeError
- Response is returned unchanged
- Logging captures the malformed structure for debugging

### UT-HAPI-496-021: HumanReviewReason enum cleaned

**BR**: BR-496
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`

**Given**: The `HumanReviewReason` enum in `incident_models.py`
**When**: Enum members are inspected
**Then**: Neither `affectedResource_mismatch` nor `unverified_target_resource` exist as enum values

**Acceptance Criteria**:
- `HumanReviewReason.AFFECTED_RESOURCE_MISMATCH` raises AttributeError
- `HumanReviewReason.UNVERIFIED_TARGET_RESOURCE` raises AttributeError
- `rca_incomplete` still exists as a valid value

### E2E-FP-496-001: CRD affectedResource matches root_owner

**BR**: BR-496
**Type**: E2E
**File**: `test/e2e/fullpipeline/` (existing or new test file)

**Given**: A Kind cluster with mock LLM configured for the oomkilled scenario, workflow schema with TARGET_RESOURCE_* declared
**When**: An OOMKilled alert triggers the full pipeline (Gateway → AA → HAPI → mock LLM → RO → WFE)
**Then**: The completed AIAnalysis CRD has `Status.RootCauseAnalysis.AffectedResource` with Kind="Deployment", Name matching the root_owner resolved by get_resource_context

**Acceptance Criteria**:
- AffectedResource.Kind matches root_owner kind (case-sensitive)
- AffectedResource.Name matches root_owner name
- AffectedResource.Namespace matches root_owner namespace
- Values are K8s-verified (from owner chain resolution), not mock LLM hardcoded

### E2E-FP-496-002: WFE params contain injected TARGET_RESOURCE_*

**BR**: BR-496
**Type**: E2E
**File**: `test/e2e/fullpipeline/` (existing or new test file)

**Given**: Same as E2E-FP-496-001
**When**: WorkflowExecution is created by the RO
**Then**: WFE.Spec.Parameters contains TARGET_RESOURCE_NAME, TARGET_RESOURCE_KIND, TARGET_RESOURCE_NAMESPACE with correct values

**Acceptance Criteria**:
- All 3 canonical params present in WFE parameters
- Values match the root_owner from get_resource_context
- Operational params (e.g., MEMORY_LIMIT_NEW) also present

### E2E-FP-496-003: Mock LLM response lacks affectedResource

**BR**: BR-496
**Type**: E2E
**File**: `test/e2e/fullpipeline/` (existing or new test file)

**Given**: Mock LLM configured to NOT include affectedResource in its JSON response
**When**: Full pipeline completes
**Then**: CRD AIAnalysis.Status.RootCauseAnalysis.AffectedResource is populated (proving HAPI injection)

**Acceptance Criteria**:
- Mock LLM response does not contain "affectedResource" key
- CRD AffectedResource is non-nil
- CRD AffectedResource matches root_owner

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Python pytest (HAPI Python codebase)
- **Mocks**: DataStorage client (mock), session_state (dict fixture), investigation_result (fixture)
- **Location**: `holmesgpt-api/tests/unit/test_target_resource_injection.py`
- **Anti-patterns avoided**:
  - No `time.Sleep()` — Python tests are synchronous
  - No `Skip()` — all tests implemented or not written
  - No direct audit/metrics infrastructure testing — tests validate business logic outcomes

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: Kind cluster with mock LLM, real DataStorage, real HAPI, real Go controllers
- **Location**: `test/e2e/fullpipeline/`
- **Anti-patterns avoided**:
  - Use `Eventually()` for async CRD status assertions, never `time.Sleep()`
  - No `Skip()` — tests fail if infrastructure unavailable

---

## 8. Execution

```bash
# Unit tests (Python)
cd holmesgpt-api && python -m pytest tests/unit/test_target_resource_injection.py -v

# E2E tests (Go — requires Kind cluster)
make test-e2e-fullpipeline

# Specific E2E test by ID
go test ./test/e2e/fullpipeline/... -ginkgo.focus="E2E-FP-496"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for BR-496 v2: HAPI-owned target resource identity |
| 1.1 | 2026-03-04 | Added 4 risk mitigation tests (018-021): cluster-scoped, LLM overwrite, malformed params, enum cleanup. Added CRD enum cleanup to scope. R1: namespace optional for cluster-scoped resources. Strengthened UT-015 acceptance criteria. |
| 1.2 | 2026-03-04 | Restructured section 5 into 6 TDD execution groups with explicit RED/GREEN/REFACTOR phases per group. Moved risk mitigation tests (018-020) into their respective component groups. |
