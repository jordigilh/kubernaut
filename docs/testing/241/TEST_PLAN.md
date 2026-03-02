# Test Plan: Strip Undeclared Parameters from LLM Workflow Response

**Feature**: HAPI strips parameters not declared in workflow schema before they reach execution
**Version**: 1.1
**Created**: 2026-03-02
**Author**: AI Assistant
**Status**: Complete (TDD RED-GREEN-REFACTOR done)
**Branch**: `feature/v1.0-bugfixes-demos`

**Authority**:
- [BR-WORKFLOW-004]: Workflow parameter contract enforcement
- [DD-WE-006]: Schema-declared dependency injection
- [BR-HAPI-191]: Parameter validation in chat session
- [GitHub Issue #241]: Strip undeclared parameters from LLM workflow response

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Existing validator tests](../../../holmesgpt-api/tests/unit/test_workflow_response_validation.py)

---

## 1. Scope

### In Scope

- `workflow_response_validator.py:_validate_parameters`: Add logic to identify and strip undeclared parameters from the `params` dict in-place
- Logging: Emit a warning for each stripped parameter (observability for LLM hallucination patterns)

### Out of Scope

- Changes to `ValidationResult` (not needed with in-place mutation)
- Changes to `result_parser.py` (not needed — the caller's dict is mutated directly)
- Go-side defense-in-depth filtering in WE controller (tracked as separate follow-up issue)
- Changes to the workflow schema format itself
- Changes to LLM prompt engineering to prevent hallucinated parameters

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| In-place mutation of `params` dict | Python passes dicts by reference. The caller's `selected_workflow["parameters"]` IS the same object passed to `_validate_parameters`. Mutating in-place requires zero changes to `ValidationResult` or `result_parser`. Fewer moving parts, fewer tests. |
| Log warnings for each stripped param | Observability: operators can detect LLM hallucination patterns without failing the request. |
| Strip does NOT cause validation failure | The LLM providing extra params is not an error condition — the fix is silent filtering with logging. Declared params are still validated normally. |
| No schema = strip ALL params | If a workflow has no `parameters` schema, nothing is declared, so nothing should pass through. This prevents hallucinated params from reaching execution for schema-less workflows. |
| Go-side follow-up (WE controller) | Defense-in-depth: the WE controller already has the schema from OCI extraction and can filter `wfe.Spec.Parameters` before `buildEnvVars`/`convertParameters`. Tracked as separate issue. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of the parameter-stripping logic (pure logic: set computation, dict mutation, logging)
- **Integration**: Deferred — see Tier Skip Rationale

### 2-Tier Minimum

The stripping logic is pure (no I/O), so unit tests provide complete coverage. The integration path (`result_parser -> validator -> AIAnalysis`) is tested by existing E2E flows; a dedicated IT is not cost-effective for a ~10-line filter.

### Business Outcome Quality Bar

Tests validate that **undeclared parameters are removed before reaching execution**, preventing LLM-hallucinated credentials or arbitrary env vars from being injected into workflow containers.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `holmesgpt-api/src/validation/workflow_response_validator.py` | `_validate_parameters` (stripping addition at end of method) | ~10 |

### Integration-Testable Code (I/O, wiring, cross-component)

No changes needed in `result_parser.py` or `ValidationResult` — the in-place mutation flows through automatically.

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-004 | Undeclared params stripped from LLM response | P0 | Unit | UT-HAPI-241-001 | Pass |
| BR-WORKFLOW-004 | Declared params preserved after stripping | P0 | Unit | UT-HAPI-241-002 | Pass |
| BR-WORKFLOW-004 | Mixed declared + undeclared params: only declared survive | P0 | Unit | UT-HAPI-241-003 | Pass |
| BR-WORKFLOW-004 | No schema: ALL params stripped | P0 | Unit | UT-HAPI-241-004 | Pass |
| BR-WORKFLOW-004 | Empty params dict: no error | P1 | Unit | UT-HAPI-241-005 | Pass |
| DD-WE-006 | Credential-like params (GIT_PASSWORD) stripped | P0 | Unit | UT-HAPI-241-006 | Pass |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-HAPI-241-{SEQUENCE}`

- **TIER**: `UT` (Unit)
- **SERVICE**: HAPI
- **BR_NUMBER**: 241 (issue number, maps to BR-WORKFLOW-004 / DD-WE-006)
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `_validate_parameters` in-place stripping logic — 100% coverage targeted

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-HAPI-241-001` | LLM-hallucinated parameters (not in schema) are removed from the params dict | Pass |
| `UT-HAPI-241-002` | Schema-declared parameters are preserved exactly as provided by the LLM | Pass |
| `UT-HAPI-241-003` | When LLM sends mix of declared + undeclared, only declared params remain in the dict | Pass |
| `UT-HAPI-241-004` | Workflow with no parameter schema: ALL params stripped (nothing declared = nothing allowed) | Pass |
| `UT-HAPI-241-005` | Empty params dict produces no errors and dict remains empty | Pass |
| `UT-HAPI-241-006` | Credential-like hallucinated params (GIT_PASSWORD, ADMIN_TOKEN) are stripped | Pass |

### Tier Skip Rationale

- **Integration**: The stripping logic is pure computation (set difference + dict key deletion). No I/O boundary is crossed. Existing E2E full-pipeline tests exercise the `result_parser -> validator -> AIAnalysis -> RO -> WFE` chain end-to-end. A dedicated integration test would duplicate unit coverage without adding value.

---

## 6. Test Cases (Detail)

### UT-HAPI-241-001: Undeclared params stripped

**BR**: BR-WORKFLOW-004, DD-WE-006
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_workflow_response_validation.py`

**Given**: A workflow with schema declaring `["TARGET_NAMESPACE", "TARGET_RESOURCE_NAME"]`
**When**: Validator runs with params `{"TARGET_NAMESPACE": "prod", "TARGET_RESOURCE_NAME": "cert", "GIT_PASSWORD": "secret123", "GIT_USERNAME": "admin"}`
**Then**: After validation, params dict contains only `{"TARGET_NAMESPACE": "prod", "TARGET_RESOURCE_NAME": "cert"}`

**Acceptance Criteria**:
- `params` dict has exactly 2 keys after validation
- `result.is_valid` is `True` (extra params do not cause failure)
- `result.errors` is empty

### UT-HAPI-241-002: Declared params preserved

**BR**: BR-WORKFLOW-004
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_workflow_response_validation.py`

**Given**: A workflow with schema declaring `["namespace", "replicas"]`
**When**: Validator runs with params `{"namespace": "default", "replicas": 3}` (no extras)
**Then**: After validation, params dict equals `{"namespace": "default", "replicas": 3}` (unchanged)

**Acceptance Criteria**:
- `params` dict is identical to input
- `result.is_valid` is `True`

### UT-HAPI-241-003: Mixed params filtered

**BR**: BR-WORKFLOW-004
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_workflow_response_validation.py`

**Given**: A workflow with schema declaring `["namespace"]` (required, string)
**When**: Validator runs with params `{"namespace": "default", "extra_param": "val1", "another_extra": 42}`
**Then**: After validation, params dict is `{"namespace": "default"}`

**Acceptance Criteria**:
- `params` dict has exactly 1 key
- Keys `extra_param` and `another_extra` are absent
- `result.is_valid` is `True`

### UT-HAPI-241-004: No schema strips all params

**BR**: BR-WORKFLOW-004
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_workflow_response_validation.py`

**Given**: A workflow with `parameters = None` (no schema)
**When**: Validator runs with params `{"any_param": "value", "another": "thing"}`
**Then**: After validation, params dict is `{}` (all stripped — nothing is declared)

**Acceptance Criteria**:
- `params` dict is empty
- `result.is_valid` is `True`

### UT-HAPI-241-005: Empty params no crash

**BR**: BR-WORKFLOW-004
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_workflow_response_validation.py`

**Given**: A workflow with schema declaring `["namespace"]` (optional)
**When**: Validator runs with params `{}`
**Then**: After validation, params dict is `{}` (empty in, empty out)

**Acceptance Criteria**:
- `params` dict is empty
- `result.is_valid` is `True`
- No exception raised

### UT-HAPI-241-006: Credential hallucination stripped

**BR**: DD-WE-006
**Type**: Unit
**File**: `holmesgpt-api/tests/unit/test_workflow_response_validation.py`

**Given**: A workflow with schema declaring `["TARGET_NAMESPACE"]` only
**When**: Validator runs with params `{"TARGET_NAMESPACE": "demo", "GIT_PASSWORD": "kubernaut-token", "GIT_USERNAME": "kubernaut", "ADMIN_TOKEN": "abc"}`
**Then**: After validation, params dict is `{"TARGET_NAMESPACE": "demo"}`

**Acceptance Criteria**:
- No credential-like keys survive in `params`
- `result.is_valid` is `True` (stripping is silent)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: pytest
- **Mocks**: `unittest.mock.Mock` for DS client and workflow objects
- **Helper**: `create_workflow_with_params(param_defs)` (existing in `TestParameterSchemaValidation`)
- **Location**: `holmesgpt-api/tests/unit/test_workflow_response_validation.py` (extend `TestParameterSchemaValidation` class)
- **Key pattern**: Tests pass a `params` dict to `validator.validate()`, then assert on the same `params` dict object after the call returns (verifying in-place mutation)

---

## 8. Execution

```bash
# All HAPI unit tests
make test-unit-holmesgpt-api

# Just the parameter validation tests
cd holmesgpt-api && python -m pytest tests/unit/test_workflow_response_validation.py::TestParameterSchemaValidation -v

# Specific test by name
cd holmesgpt-api && python -m pytest tests/unit/test_workflow_response_validation.py -k "undeclared" -v
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.2 | 2026-03-02 | All 6 tests passing — updated statuses from Pending to Pass, status to Complete |
| 1.1 | 2026-03-02 | Revised: in-place mutation (no ValidationResult/result_parser changes), no-schema strips all, removed UT-007/008, added Go-side follow-up decision |
| 1.0 | 2026-03-02 | Initial test plan |
