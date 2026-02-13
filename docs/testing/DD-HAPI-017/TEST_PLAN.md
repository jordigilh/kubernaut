# Test Plan: Three-Step Workflow Discovery Integration (DD-HAPI-017)

**Feature**: Replace `search_workflow_catalog` with three-step discovery tools for incident and recovery flows
**Version**: 1.0
**Created**: 2026-02-05
**Author**: AI Assistant + Jordi Gil
**Status**: Draft
**Branch**: `fix/ci-coverage-reporting-issue-36`

**Authority**:
- DD-HAPI-017: Three-Step Workflow Discovery Integration
- DD-WORKFLOW-016: Action-Type Workflow Catalog Indexing
- DD-WORKFLOW-017: Workflow Lifecycle Component Interactions
- DD-WORKFLOW-014 v3.0: Workflow Selection Audit Trail
- BR-HAPI-017-001 through BR-HAPI-017-006

---

## 1. Scope

### In Scope

**HAPI-side (Python/pytest)**:
- **Three new tool classes** replacing `SearchWorkflowCatalogTool` in `holmesgpt-api/src/toolsets/workflow_catalog.py`
  - `ListAvailableActionsTool` -- action type discovery from taxonomy
  - `ListWorkflowsTool` -- workflow selection within action type
  - `GetWorkflowTool` -- single workflow parameter schema lookup with security gate
- **Prompt builder updates** -- incident and recovery prompt builders rewritten for three-step instructions
- **Validator updates** -- `WorkflowResponseValidator` gains context filter parameters for DS security gate
- **Recovery validation loop** -- parity with incident flow's `MAX_VALIDATION_ATTEMPTS = 3`
- **remediationId propagation** -- all DS discovery calls include `remediationId` query parameter
- **Old tool removal** -- `SearchWorkflowCatalogTool` and `WorkflowCatalogToolset` deleted

**DS-side (Go/Ginkgo)** -- discovery endpoint changes:
- `GET /api/v1/workflows/actions` handler + repository
- `GET /api/v1/workflows/actions/{action_type}` handler + repository
- `GET /api/v1/workflows/{id}` gains context filter query parameters (security gate)
- Four new audit events per DD-WORKFLOW-014 v3.0
- Removal of `POST /api/v1/workflows/search`

### Out of Scope

- DS workflow registration (OCI pullspec) changes -- owned by DD-WORKFLOW-017
- Workflow lifecycle management (disable/enable/deprecate) -- owned by DD-WORKFLOW-017
- OpenAPI spec authoring -- mechanical prerequisite, not a test target
- Client regeneration -- mechanical step after OpenAPI update
- Existing `search_workflow_catalog` backward compatibility -- no migration required (pre-release)

### Design Decisions (Key Testing Implications)

| Decision | Implication for Tests |
|----------|----------------------|
| Pre-release product, no migration | No coexistence tests needed; old tool is simply removed |
| Recovery flow gains validation loop | Recovery integration/E2E tests must verify self-correction behavior |
| DS security gate via context filters | `get_workflow` unit/integration tests must verify 404 on context mismatch |
| Four DS audit events replace one | Audit E2E tests rewritten from single `search_completed` to four step-specific events |
| `remediationId` on all calls | Every tool unit test must verify query parameter propagation |

---

## 2. BR Coverage Matrix

### HAPI-Side Tests

| BR ID | Description | Priority | Test Type | Test ID | Status |
|-------|-------------|----------|-----------|---------|--------|
| BR-HAPI-017-001 | ListAvailableActionsTool happy path -- DS call + response parsing | P0 | Unit | UT-HAPI-017-001-001 | ⏸️ |
| BR-HAPI-017-001 | ListAvailableActionsTool parameter validation -- missing filters | P0 | Unit | UT-HAPI-017-001-002 | ⏸️ |
| BR-HAPI-017-001 | ListAvailableActionsTool pagination -- hasMore forwarded to LLM | P0 | Unit | UT-HAPI-017-001-003 | ⏸️ |
| BR-HAPI-017-001 | ListAvailableActionsTool error handling -- 400/500/502 from DS | P0 | Unit | UT-HAPI-017-001-004 | ⏸️ |
| BR-HAPI-017-001 | ListAvailableActionsTool context filters forwarded to DS | P0 | Unit | UT-HAPI-017-001-005 | ⏸️ |
| BR-HAPI-017-001 | ListAvailableActionsTool returns StructuredToolResult | P1 | Unit | UT-HAPI-017-001-006 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsTool happy path -- DS call with action_type | P0 | Unit | UT-HAPI-017-001-007 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsTool parameter validation -- missing action_type | P0 | Unit | UT-HAPI-017-001-008 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsTool pagination -- hasMore forwarded to LLM | P0 | Unit | UT-HAPI-017-001-009 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsTool error handling -- 400/500 from DS | P0 | Unit | UT-HAPI-017-001-010 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsTool context filters forwarded to DS | P0 | Unit | UT-HAPI-017-001-011 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsTool returns StructuredToolResult | P1 | Unit | UT-HAPI-017-001-012 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflowTool happy path -- DS call with workflow_id | P0 | Unit | UT-HAPI-017-001-013 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflowTool 404 security gate -- context mismatch | P0 | Unit | UT-HAPI-017-001-014 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflowTool error handling -- 400/500/502 from DS | P0 | Unit | UT-HAPI-017-001-015 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflowTool context filters forwarded to DS | P0 | Unit | UT-HAPI-017-001-016 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflowTool returns StructuredToolResult with parameter schema | P0 | Unit | UT-HAPI-017-001-017 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflowTool strips finalScore from LLM output | P1 | Unit | UT-HAPI-017-001-018 | ⏸️ |
| BR-HAPI-017-001 | Toolset registers all three tools | P0 | Unit | UT-HAPI-017-001-019 | ⏸️ |
| BR-HAPI-017-001 | Three tools against real DS -- action type discovery | P0 | Integration | IT-HAPI-017-001-001 | ⏸️ |
| BR-HAPI-017-001 | Three tools against real DS -- workflow listing for action type | P0 | Integration | IT-HAPI-017-001-002 | ⏸️ |
| BR-HAPI-017-001 | Three tools against real DS -- single workflow retrieval | P0 | Integration | IT-HAPI-017-001-003 | ⏸️ |
| BR-HAPI-017-001 | Pagination with real DS -- multiple pages of workflows | P0 | Integration | IT-HAPI-017-001-004 | ⏸️ |
| BR-HAPI-017-001 | Three-step discovery happy path -- full incident flow | P0 | E2E | E2E-HAPI-017-001-001 | ⏸️ |
| BR-HAPI-017-001 | Three-step discovery happy path -- full recovery flow | P0 | E2E | E2E-HAPI-017-001-002 | ⏸️ |
| BR-HAPI-017-002 | Incident prompt contains three-step instructions | P0 | Unit | UT-HAPI-017-002-001 | ⏸️ |
| BR-HAPI-017-002 | Recovery prompt contains three-step instructions | P0 | Unit | UT-HAPI-017-002-002 | ⏸️ |
| BR-HAPI-017-002 | Incident prompt does NOT contain search_workflow_catalog | P0 | Unit | UT-HAPI-017-002-003 | ⏸️ |
| BR-HAPI-017-002 | Recovery prompt does NOT contain search_workflow_catalog | P0 | Unit | UT-HAPI-017-002-004 | ⏸️ |
| BR-HAPI-017-002 | Step 2 instruction includes "review ALL workflows" mandate | P0 | Unit | UT-HAPI-017-002-005 | ⏸️ |
| BR-HAPI-017-003 | Validator passes context filters to get_workflow call | P0 | Unit | UT-HAPI-017-003-001 | ⏸️ |
| BR-HAPI-017-003 | Validator treats 404 from DS as validation failure | P0 | Unit | UT-HAPI-017-003-002 | ⏸️ |
| BR-HAPI-017-003 | Validator error message includes context mismatch detail | P1 | Unit | UT-HAPI-017-003-003 | ⏸️ |
| BR-HAPI-017-003 | Validator happy path -- workflow matches context | P0 | Unit | UT-HAPI-017-003-004 | ⏸️ |
| BR-HAPI-017-003 | Security gate with real DS -- mismatched context returns 404 | P0 | Integration | IT-HAPI-017-003-001 | ⏸️ |
| BR-HAPI-017-003 | Security gate with real DS -- matching context returns workflow | P0 | Integration | IT-HAPI-017-003-002 | ⏸️ |
| BR-HAPI-017-004 | Recovery validation loop executes up to MAX_VALIDATION_ATTEMPTS | P0 | Unit | UT-HAPI-017-004-001 | ⏸️ |
| BR-HAPI-017-004 | Recovery validation loop injects feedback on retry | P0 | Unit | UT-HAPI-017-004-002 | ⏸️ |
| BR-HAPI-017-004 | Recovery sets needs_human_review after exhausting attempts | P0 | Unit | UT-HAPI-017-004-003 | ⏸️ |
| BR-HAPI-017-004 | Recovery validation loop succeeds on retry (2nd attempt) | P0 | Unit | UT-HAPI-017-004-004 | ⏸️ |
| BR-HAPI-017-004 | Recovery validation loop with real DS -- retry on invalid params | P0 | Integration | IT-HAPI-017-004-001 | ⏸️ |
| BR-HAPI-017-004 | Recovery validation loop with real DS -- succeeds after correction | P0 | Integration | IT-HAPI-017-004-002 | ⏸️ |
| BR-HAPI-017-004 | Recovery flow E2E -- validation loop with Mock LLM | P0 | E2E | E2E-HAPI-017-004-001 | ⏸️ |
| BR-HAPI-017-005 | ListAvailableActionsTool passes remediationId as query param | P0 | Unit | UT-HAPI-017-005-001 | ⏸️ |
| BR-HAPI-017-005 | ListWorkflowsTool passes remediationId as query param | P0 | Unit | UT-HAPI-017-005-002 | ⏸️ |
| BR-HAPI-017-005 | GetWorkflowTool passes remediationId as query param | P0 | Unit | UT-HAPI-017-005-003 | ⏸️ |
| BR-HAPI-017-005 | Empty remediationId handled gracefully (discovery proceeds) | P1 | Unit | UT-HAPI-017-005-004 | ⏸️ |
| BR-HAPI-017-006 | SearchWorkflowCatalogTool class no longer exists | P0 | Unit | UT-HAPI-017-006-001 | ⏸️ |
| BR-HAPI-017-006 | No references to search_workflow_catalog in Python source | P0 | Unit | UT-HAPI-017-006-002 | ⏸️ |

### DS-Side Tests

| BR ID | Description | Priority | Test Type | Test ID | Status |
|-------|-------------|----------|-----------|---------|--------|
| BR-HAPI-017-001 | ListActions handler -- valid request, returns action types | P0 | Unit | UT-DS-017-001-001 | ⏸️ |
| BR-HAPI-017-001 | ListActions handler -- pagination (offset/limit) | P0 | Unit | UT-DS-017-001-002 | ⏸️ |
| BR-HAPI-017-001 | ListActions handler -- context filter parameters parsed | P0 | Unit | UT-DS-017-001-003 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsByActionType handler -- valid action_type path param | P0 | Unit | UT-DS-017-001-004 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsByActionType handler -- unknown action_type returns empty | P1 | Unit | UT-DS-017-001-005 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsByActionType handler -- pagination (offset/limit) | P0 | Unit | UT-DS-017-001-006 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflow handler -- valid workflow_id with matching context | P0 | Unit | UT-DS-017-001-007 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflow handler -- valid workflow_id but context mismatch returns 404 | P0 | Unit | UT-DS-017-001-008 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflow handler -- nonexistent workflow_id returns 404 | P0 | Unit | UT-DS-017-001-009 | ⏸️ |
| BR-HAPI-017-001 | ListActions repository -- SQL filters by active status | P0 | Integration | IT-DS-017-001-001 | ⏸️ |
| BR-HAPI-017-001 | ListActions repository -- pagination returns correct slice | P0 | Integration | IT-DS-017-001-002 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsByActionType repository -- filters by action_type + context | P0 | Integration | IT-DS-017-001-003 | ⏸️ |
| BR-HAPI-017-001 | ListWorkflowsByActionType repository -- excludes disabled workflows | P0 | Integration | IT-DS-017-001-004 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflowWithContextFilters repository -- context match returns workflow | P0 | Integration | IT-DS-017-001-005 | ⏸️ |
| BR-HAPI-017-001 | GetWorkflowWithContextFilters repository -- context mismatch returns nil | P0 | Integration | IT-DS-017-001-006 | ⏸️ |
| BR-HAPI-017-001 | Three-step endpoints E2E happy path | P0 | E2E | E2E-DS-017-001-001 | ⏸️ |
| BR-HAPI-017-001 | Three-step endpoints E2E -- disabled workflow excluded | P0 | E2E | E2E-DS-017-001-002 | ⏸️ |
| BR-HAPI-017-001 | Three-step endpoints E2E -- security gate 404 | P0 | E2E | E2E-DS-017-001-003 | ⏸️ |
| BR-HAPI-017-005 | ListActions handler -- remediationId query param propagated | P0 | Unit | UT-DS-017-005-001 | ⏸️ |
| BR-HAPI-017-005 | Audit events include remediationId correlation | P0 | Integration | IT-DS-017-005-001 | ⏸️ |
| BR-AUDIT-023 | workflow.catalog.actions_listed audit event emitted | P0 | E2E | E2E-DS-017-AUDIT-001 | ⏸️ |
| BR-AUDIT-023 | workflow.catalog.workflows_listed audit event emitted | P0 | E2E | E2E-DS-017-AUDIT-002 | ⏸️ |
| BR-AUDIT-023 | workflow.catalog.workflow_retrieved audit event emitted | P0 | E2E | E2E-DS-017-AUDIT-003 | ⏸️ |
| BR-AUDIT-023 | workflow.catalog.selection_validated audit event emitted | P0 | E2E | E2E-DS-017-AUDIT-004 | ⏸️ |
| BR-HAPI-017-006 | POST /api/v1/workflows/search endpoint removed | P0 | E2E | E2E-DS-017-006-001 | ⏸️ |

---

## 3. Test Cases

### Phase 1: HAPI Tool Classes (`holmesgpt-api/src/toolsets/workflow_catalog.py`)

#### UT-HAPI-017-001-001: ListAvailableActionsTool happy path
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: ListAvailableActionsTool calls DS `GET /api/v1/workflows/actions` and returns parsed action types
**Preconditions**: Mock DS client returning 200 with action type list
**Steps**:
1. Patch DS client `list_available_actions` to return `{"actionTypes": [{"actionType": "scale_up", "description": "...", "workflowCount": 3}], "hasMore": false}`
2. Invoke `ListAvailableActionsTool._invoke(params={"severity": "critical", "component": "web-api"})`
3. Assert `StructuredToolResult` with status SUCCESS
4. Assert DS client was called with severity="critical", component="web-api"
**Expected Result**: StructuredToolResult.SUCCESS with rendered action types (no internal scores)

#### UT-HAPI-017-001-002: ListAvailableActionsTool parameter validation
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Error Handling
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool returns validation error when no context filters provided
**Preconditions**: No mock DS call (validation fails before invocation)
**Steps**:
1. Invoke `ListAvailableActionsTool._invoke(params={})`
**Expected Result**: StructuredToolResult with error detail indicating missing filter parameters

#### UT-HAPI-017-001-003: ListAvailableActionsTool pagination
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool forwards pagination metadata (hasMore, offset, total) to LLM
**Preconditions**: Mock DS client returning `{"actionTypes": [...], "hasMore": true, "offset": 0, "limit": 20, "total": 45}`
**Steps**:
1. Patch DS client to return paginated response with `hasMore=true`
2. Invoke tool with default offset/limit
3. Assert result contains pagination metadata for LLM to request next page
**Expected Result**: StructuredToolResult includes `hasMore: true` and next page offset in rendered output

#### UT-HAPI-017-001-004: ListAvailableActionsTool error handling
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Error Handling
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool handles 400, 500, 502 errors from DS with appropriate messages
**Preconditions**: Mock DS client raising `ApiException` with various status codes
**Steps**:
1. Patch DS client to raise `ApiException(status=400, reason="validation-error")`
2. Invoke tool
3. Assert StructuredToolResult with error message containing "request error"
4. Repeat for 500 ("internal error") and 502 ("service unavailable")
**Expected Result**: Each error code maps to the correct error category per DD-HAPI-017 error table

#### UT-HAPI-017-001-005: ListAvailableActionsTool context filters forwarded
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: All signal context filters (severity, component, environment, priority, custom_labels, detected_labels) are passed to the DS client call
**Preconditions**: Mock DS client capturing call arguments
**Steps**:
1. Invoke tool with full context: `severity="critical"`, `component="web-api"`, `environment="production"`, `priority="P1"`, `custom_labels={"team": ["platform"]}`, `detected_labels={"app": "nginx"}`
2. Assert DS client was called with all six filter parameters matching input
**Expected Result**: DS client receives all filters; none are dropped or defaulted

#### UT-HAPI-017-001-006: ListAvailableActionsTool returns StructuredToolResult
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool returns a well-formed StructuredToolResult with status and rendered content
**Preconditions**: Mock DS client returning valid response
**Steps**:
1. Invoke tool with valid params
2. Assert return type is `StructuredToolResult`
3. Assert `status` is `StructuredToolResultStatus.SUCCESS`
4. Assert `rendered` contains human-readable action type descriptions
**Expected Result**: Correct type, status, and rendered output

#### UT-HAPI-017-001-007: ListWorkflowsTool happy path
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: ListWorkflowsTool calls DS `GET /api/v1/workflows/actions/{action_type}` and returns workflows for the specified action type
**Preconditions**: Mock DS client returning 200 with workflow list
**Steps**:
1. Patch DS client `list_workflows` to return `{"workflows": [{"workflowId": "uuid-1", "name": "scale-conservative", ...}], "hasMore": false}`
2. Invoke `ListWorkflowsTool._invoke(params={"action_type": "scale_up", "severity": "critical"})`
3. Assert StructuredToolResult.SUCCESS with workflow list
4. Assert DS client called with `action_type="scale_up"` as path parameter
**Expected Result**: StructuredToolResult with all workflows for the action type

#### UT-HAPI-017-001-008: ListWorkflowsTool missing action_type
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Error Handling
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool returns validation error when action_type is not provided
**Preconditions**: No mock DS call
**Steps**:
1. Invoke `ListWorkflowsTool._invoke(params={"severity": "critical"})`
**Expected Result**: StructuredToolResult with error indicating missing `action_type` parameter

#### UT-HAPI-017-001-009: ListWorkflowsTool pagination
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool forwards pagination metadata for LLM to request all pages
**Preconditions**: Mock DS client returning paginated response with `hasMore=true`
**Steps**:
1. Patch DS client to return first page with `hasMore=true`
2. Invoke tool
3. Assert result includes pagination metadata AND explicit instruction to fetch remaining pages
**Expected Result**: LLM receives "review ALL workflows" context with page navigation info

#### UT-HAPI-017-001-010: ListWorkflowsTool error handling
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Error Handling
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool handles 400 and 500 errors from DS
**Preconditions**: Mock DS client raising `ApiException`
**Steps**:
1. Patch DS client to raise `ApiException(status=400)`
2. Invoke tool
3. Assert appropriate error StructuredToolResult
4. Repeat for 500
**Expected Result**: Error category matches DD-HAPI-017 error table

#### UT-HAPI-017-001-011: ListWorkflowsTool context filters forwarded
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: All signal context filters forwarded alongside action_type
**Preconditions**: Mock DS client capturing call arguments
**Steps**:
1. Invoke tool with `action_type="scale_up"` and full signal context
2. Assert DS client received action_type as path param AND all context filters as query params
**Expected Result**: Both action_type routing and context filtering applied

#### UT-HAPI-017-001-012: ListWorkflowsTool returns StructuredToolResult
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool returns well-formed StructuredToolResult with workflow details
**Preconditions**: Mock DS client returning valid response
**Steps**:
1. Invoke tool with valid params
2. Assert correct return type and rendered content includes workflow names, descriptions, effectiveness data
**Expected Result**: StructuredToolResult.SUCCESS with clean LLM-rendered workflow list

#### UT-HAPI-017-001-013: GetWorkflowTool happy path
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: GetWorkflowTool calls DS `GET /api/v1/workflows/{workflow_id}` and returns parameter schema
**Preconditions**: Mock DS client returning 200 with full workflow detail including parameter schema
**Steps**:
1. Patch DS client `get_workflow` to return `{"workflowId": "uuid-1", "name": "scale-conservative", "parameters": {"replicas": {"type": "integer", "required": true}}, ...}`
2. Invoke `GetWorkflowTool._invoke(params={"workflow_id": "uuid-1", "severity": "critical"})`
3. Assert StructuredToolResult.SUCCESS with parameter schema
4. Assert DS client called with `workflow_id="uuid-1"` as path parameter
**Expected Result**: StructuredToolResult with full workflow detail and parameter schema

#### UT-HAPI-017-001-014: GetWorkflowTool 404 security gate
**BR**: BR-HAPI-017-001, BR-HAPI-017-003
**Type**: Unit
**Category**: Security
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: GetWorkflowTool returns error when DS returns 404 (workflow exists but context filters exclude it)
**Preconditions**: Mock DS client raising `ApiException(status=404)` (security gate rejection)
**Steps**:
1. Patch DS client to raise 404 ApiException
2. Invoke tool with valid workflow_id but mismatched context filters
3. Assert StructuredToolResult with error message indicating workflow not found for context
**Expected Result**: Tool returns error to LLM (not exception); LLM can retry with different workflow

#### UT-HAPI-017-001-015: GetWorkflowTool error handling
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Error Handling
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool handles 400, 500, 502 from DS
**Preconditions**: Mock DS client raising `ApiException` with various status codes
**Steps**:
1. Test 400 (validation error), 500 (internal), 502 (service unavailable)
2. Assert each maps to correct error category
**Expected Result**: Error handling matches DD-HAPI-017 error table

#### UT-HAPI-017-001-016: GetWorkflowTool context filters forwarded
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Security
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: All signal context filters forwarded for defense-in-depth security gate
**Preconditions**: Mock DS client capturing call arguments
**Steps**:
1. Invoke tool with full context filters
2. Assert DS client received all filters as query parameters alongside workflow_id
**Expected Result**: Security gate context fully propagated

#### UT-HAPI-017-001-017: GetWorkflowTool returns parameter schema
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool's rendered output includes the workflow parameter schema for LLM to populate
**Preconditions**: Mock DS returning workflow with parameters
**Steps**:
1. Invoke tool
2. Assert rendered output contains parameter names, types, required flags, and descriptions
**Expected Result**: LLM can read parameter schema from rendered output to populate values from RCA

#### UT-HAPI-017-001-018: GetWorkflowTool strips finalScore
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Internal scoring fields (finalScore) are not included in LLM-facing output
**Preconditions**: Mock DS response containing `finalScore` field
**Steps**:
1. Invoke tool with response containing `finalScore: 0.85`
2. Assert `finalScore` does NOT appear in StructuredToolResult.rendered
**Expected Result**: Clean LLM output without internal scoring metadata

#### UT-HAPI-017-001-019: Toolset registers all three tools
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: New toolset class registers ListAvailableActionsTool, ListWorkflowsTool, and GetWorkflowTool
**Preconditions**: Instantiate new toolset class
**Steps**:
1. Create toolset instance
2. Assert `get_tools()` returns exactly 3 tools
3. Assert tool names are `list_available_actions`, `list_workflows`, `get_workflow`
**Expected Result**: Three tools registered with correct names

---

### Phase 2: HAPI Prompt Builder Updates

#### UT-HAPI-017-002-001: Incident prompt contains three-step instructions
**BR**: BR-HAPI-017-002
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_prompt_builder_three_step.py`
**Description**: Incident prompt builder generates instructions referencing all three discovery tools
**Preconditions**: Instantiate incident prompt builder with test data
**Steps**:
1. Call `build_investigation_prompt()` on incident prompt builder
2. Assert prompt contains `list_available_actions`
3. Assert prompt contains `list_workflows`
4. Assert prompt contains `get_workflow`
**Expected Result**: All three tool names present in generated prompt

#### UT-HAPI-017-002-002: Recovery prompt contains three-step instructions
**BR**: BR-HAPI-017-002
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_prompt_builder_three_step.py`
**Description**: Recovery prompt builder generates instructions referencing all three discovery tools
**Preconditions**: Instantiate recovery prompt builder with test data
**Steps**:
1. Call `build_investigation_prompt()` on recovery prompt builder
2. Assert prompt contains `list_available_actions`, `list_workflows`, `get_workflow`
**Expected Result**: Recovery prompt has same three-step instructions as incident

#### UT-HAPI-017-002-003: Incident prompt does NOT contain search_workflow_catalog
**BR**: BR-HAPI-017-002, BR-HAPI-017-006
**Type**: Unit
**Category**: Security
**File**: `holmesgpt-api/tests/unit/test_prompt_builder_three_step.py`
**Description**: No references to the old tool name remain in incident prompt
**Preconditions**: Instantiate incident prompt builder
**Steps**:
1. Call `build_investigation_prompt()` and all other prompt methods
2. Assert `search_workflow_catalog` does NOT appear in any generated prompt string
**Expected Result**: Old tool name fully removed

#### UT-HAPI-017-002-004: Recovery prompt does NOT contain search_workflow_catalog
**BR**: BR-HAPI-017-002, BR-HAPI-017-006
**Type**: Unit
**Category**: Security
**File**: `holmesgpt-api/tests/unit/test_prompt_builder_three_step.py`
**Description**: No references to the old tool name remain in recovery prompt
**Preconditions**: Instantiate recovery prompt builder
**Steps**:
1. Call `build_investigation_prompt()` and all other prompt methods
2. Assert `search_workflow_catalog` does NOT appear in any generated prompt string
**Expected Result**: Old tool name fully removed

#### UT-HAPI-017-002-005: Step 2 includes "review ALL workflows" mandate
**BR**: BR-HAPI-017-002
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_prompt_builder_three_step.py`
**Description**: Step 2 instructions explicitly require LLM to review all workflows before selecting
**Preconditions**: Instantiate prompt builder (both incident and recovery)
**Steps**:
1. Generate prompt
2. Assert prompt contains instruction text requiring review of ALL pages/workflows before selection
3. Assert prompt does NOT contain language suggesting first-page preference
**Expected Result**: LLM instructed to comprehensively review before deciding

---

### Phase 3: HAPI Validator with Context Filters

#### UT-HAPI-017-003-001: Validator passes context filters to get_workflow
**BR**: BR-HAPI-017-003
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_workflow_validation_context_filters.py`
**Description**: WorkflowResponseValidator calls DS `get_workflow` with all signal context filters
**Preconditions**: Mock DS client capturing call arguments
**Steps**:
1. Create validator with mock DS client and context filters (severity, component, environment, priority)
2. Call `validate(workflow_id="uuid-1", container_image="quay.io/kubernaut-ai/test:v1", parameters={})`
3. Assert mock DS client `get_workflow` called with workflow_id AND all context filter parameters
**Expected Result**: DS client receives full context for security gate evaluation

#### UT-HAPI-017-003-002: Validator treats 404 as validation failure
**BR**: BR-HAPI-017-003
**Type**: Unit
**Category**: Security
**File**: `holmesgpt-api/tests/unit/test_workflow_validation_context_filters.py`
**Description**: When DS returns 404 (security gate rejection), validator marks validation as failed
**Preconditions**: Mock DS client raising `ApiException(status=404)`
**Steps**:
1. Configure mock DS client to return 404
2. Call `validate(workflow_id="uuid-1", ...)`
3. Assert `ValidationResult.is_valid` is `False`
4. Assert `ValidationResult.errors` includes context mismatch message
**Expected Result**: 404 treated as "workflow does not match signal context" failure

#### UT-HAPI-017-003-003: Validator error message includes context mismatch detail
**BR**: BR-HAPI-017-003
**Type**: Unit
**Category**: Observability
**File**: `holmesgpt-api/tests/unit/test_workflow_validation_context_filters.py`
**Description**: Validation failure error message is actionable for the LLM self-correction loop
**Preconditions**: Mock DS client returning 404
**Steps**:
1. Call validate with mismatched context
2. Assert error message contains the workflow_id that failed
3. Assert error message hints that the LLM should select a different workflow
**Expected Result**: Error message guides LLM to retry with a different selection

#### UT-HAPI-017-003-004: Validator happy path with matching context
**BR**: BR-HAPI-017-003
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_workflow_validation_context_filters.py`
**Description**: When DS returns 200, validation proceeds with parameter schema checks
**Preconditions**: Mock DS client returning 200 with workflow including parameter schema
**Steps**:
1. Configure mock to return valid workflow with parameters
2. Call validate with correct parameters
3. Assert `ValidationResult.is_valid` is `True`
**Expected Result**: Existing parameter validation logic works with new context-filtered response

---

### Phase 4: HAPI Recovery Validation Loop

#### UT-HAPI-017-004-001: Recovery validation loop executes up to MAX_VALIDATION_ATTEMPTS
**BR**: BR-HAPI-017-004
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_recovery_validation_loop.py`
**Description**: Recovery flow retries LLM investigation up to 3 times on validation failure
**Preconditions**: Mock LLM returning invalid workflow selection on all attempts; mock validator always failing
**Steps**:
1. Patch `investigate_issues` to return response with invalid workflow
2. Patch `WorkflowResponseValidator.validate` to always return `is_valid=False`
3. Execute recovery flow analysis
4. Assert `investigate_issues` was called exactly `MAX_VALIDATION_ATTEMPTS` (3) times
**Expected Result**: 3 attempts executed, then flow terminates

#### UT-HAPI-017-004-002: Recovery validation loop injects feedback on retry
**BR**: BR-HAPI-017-004
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_recovery_validation_loop.py`
**Description**: On retry, validation errors from previous attempt are injected into the prompt
**Preconditions**: Mock validator returning errors on first attempt, success on second
**Steps**:
1. First call: validator returns `is_valid=False, errors=["unknown workflow_id"]`
2. Assert second call to `investigate_issues` receives prompt with "unknown workflow_id" feedback
3. Second call: validator returns `is_valid=True`
**Expected Result**: Prompt for attempt 2 contains error feedback from attempt 1

#### UT-HAPI-017-004-003: Recovery sets needs_human_review after exhausting attempts
**BR**: BR-HAPI-017-004
**Type**: Unit
**Category**: Error Handling
**File**: `holmesgpt-api/tests/unit/test_recovery_validation_loop.py`
**Description**: After 3 failed validation attempts, recovery marks result as needs_human_review
**Preconditions**: All 3 validation attempts fail
**Steps**:
1. Configure all attempts to fail validation
2. Execute recovery analysis
3. Assert result has `needs_human_review=True`
4. Assert `human_review_reason` contains validation failure details
**Expected Result**: Graceful degradation with human escalation

#### UT-HAPI-017-004-004: Recovery validation loop succeeds on retry
**BR**: BR-HAPI-017-004
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_recovery_validation_loop.py`
**Description**: Recovery flow succeeds when LLM self-corrects on second attempt
**Preconditions**: First attempt fails validation, second succeeds
**Steps**:
1. First call: validator returns `is_valid=False`
2. Second call: validator returns `is_valid=True`
3. Assert final result has `needs_human_review=False`
4. Assert `investigate_issues` called exactly 2 times (not 3)
**Expected Result**: Loop exits early on success; no unnecessary retries

---

### Phase 5: remediationId Propagation

#### UT-HAPI-017-005-001: ListAvailableActionsTool passes remediationId
**BR**: BR-HAPI-017-005
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool includes remediationId as query parameter in DS call
**Preconditions**: Tool constructed with `remediation_id="rem-uuid-123"`; mock DS client capturing args
**Steps**:
1. Create tool with `remediation_id="rem-uuid-123"`
2. Invoke tool
3. Assert DS client call includes `remediation_id="rem-uuid-123"` parameter
**Expected Result**: remediationId propagated for audit correlation

#### UT-HAPI-017-005-002: ListWorkflowsTool passes remediationId
**BR**: BR-HAPI-017-005
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool includes remediationId as query parameter in DS call
**Preconditions**: Tool constructed with `remediation_id="rem-uuid-123"`; mock DS client
**Steps**:
1. Create tool with `remediation_id`
2. Invoke tool
3. Assert DS client call includes `remediation_id` parameter
**Expected Result**: remediationId propagated

#### UT-HAPI-017-005-003: GetWorkflowTool passes remediationId
**BR**: BR-HAPI-017-005
**Type**: Unit
**Category**: Happy Path
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool includes remediationId as query parameter in DS call
**Preconditions**: Tool constructed with `remediation_id="rem-uuid-123"`; mock DS client
**Steps**:
1. Create tool with `remediation_id`
2. Invoke tool
3. Assert DS client call includes `remediation_id` parameter
**Expected Result**: remediationId propagated

#### UT-HAPI-017-005-004: Empty remediationId handled gracefully
**BR**: BR-HAPI-017-005
**Type**: Unit
**Category**: Error Handling
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Tool proceeds normally when remediationId is None or empty
**Preconditions**: Tool constructed with `remediation_id=None`
**Steps**:
1. Create tool without remediation_id
2. Invoke tool
3. Assert tool succeeds
4. Assert DS client call either omits remediation_id or passes empty string
**Expected Result**: Discovery proceeds; no error

---

### Phase 6: Old Tool Removal Verification

#### UT-HAPI-017-006-001: SearchWorkflowCatalogTool class no longer exists
**BR**: BR-HAPI-017-006
**Type**: Unit
**Category**: Security
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: Importing `SearchWorkflowCatalogTool` raises ImportError
**Preconditions**: Old class has been removed
**Steps**:
1. Attempt `from holmesgpt_api.toolsets.workflow_catalog import SearchWorkflowCatalogTool`
2. Assert `ImportError` is raised
**Expected Result**: Class does not exist

#### UT-HAPI-017-006-002: No references to search_workflow_catalog in source
**BR**: BR-HAPI-017-006
**Type**: Unit
**Category**: Security
**File**: `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py`
**Description**: No Python source file in holmesgpt-api/src/ references the old tool name
**Preconditions**: Codebase search
**Steps**:
1. Walk all `.py` files in `holmesgpt-api/src/`
2. Search for string `search_workflow_catalog` (case-insensitive)
3. Assert 0 matches
**Expected Result**: Old tool completely removed from codebase

---

### Phase 7: HAPI Integration Tests

#### IT-HAPI-017-001-001: Action type discovery against real DS
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Happy Path
**File**: `holmesgpt-api/tests/integration/test_three_step_discovery_integration.py`
**Description**: ListAvailableActionsTool returns action types from real DS with bootstrapped taxonomy
**Preconditions**: Real DS with PostgreSQL; action_type_taxonomy table seeded via fixtures
**Steps**:
1. Bootstrap workflow fixtures with `action_type` taxonomy (scale_up, restart_pod, rollback_deployment)
2. Create `ListAvailableActionsTool` with real DS URL
3. Invoke tool with `severity="critical"`, `component="web-api"`
4. Assert result contains at least 1 action type from taxonomy
5. Assert result format matches expected structure
**Expected Result**: Real DS returns action types; tool parses correctly

#### IT-HAPI-017-001-002: Workflow listing for action type against real DS
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Happy Path
**File**: `holmesgpt-api/tests/integration/test_three_step_discovery_integration.py`
**Description**: ListWorkflowsTool returns workflows from real DS for a given action type
**Preconditions**: Real DS with bootstrapped workflows of type `scale_up`
**Steps**:
1. Bootstrap 3+ workflows with `action_type="scale_up"`
2. Invoke `ListWorkflowsTool._invoke(params={"action_type": "scale_up", "severity": "critical"})`
3. Assert result contains bootstrapped workflows
4. Assert each workflow has `workflowId`, `name`, `description`
**Expected Result**: Real DS returns matching workflows

#### IT-HAPI-017-001-003: Single workflow retrieval against real DS
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Happy Path
**File**: `holmesgpt-api/tests/integration/test_three_step_discovery_integration.py`
**Description**: GetWorkflowTool returns full workflow detail from real DS
**Preconditions**: Real DS with bootstrapped workflow; known workflow_id
**Steps**:
1. Bootstrap workflow and capture its `workflowId`
2. Invoke `GetWorkflowTool._invoke(params={"workflow_id": known_id, "severity": "critical"})`
3. Assert result contains parameter schema, container image, action_type
**Expected Result**: Full workflow detail returned

#### IT-HAPI-017-001-004: Pagination with real DS
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Happy Path
**File**: `holmesgpt-api/tests/integration/test_three_step_discovery_integration.py`
**Description**: Pagination works with enough workflows to span multiple pages
**Preconditions**: Real DS with 25+ workflows (page size 20)
**Steps**:
1. Bootstrap 25 workflows across multiple action types
2. Invoke `ListAvailableActionsTool` with limit=10
3. Assert `hasMore=true` and correct offset
4. Invoke again with offset=10
5. Assert second page returns remaining action types
**Expected Result**: Multi-page navigation works end-to-end

#### IT-HAPI-017-003-001: Security gate -- mismatched context returns 404
**BR**: BR-HAPI-017-003
**Type**: Integration
**Category**: Security
**File**: `holmesgpt-api/tests/integration/test_workflow_validation_integration.py`
**Description**: GetWorkflow with context filters that don't match the workflow returns 404
**Preconditions**: Real DS with workflow targeting `environment="production"`, `severity="critical"`
**Steps**:
1. Bootstrap workflow with `environment="production"`, `severity="critical"`
2. Invoke `GetWorkflowTool` with `environment="staging"`, `severity="warning"` (mismatched)
3. Assert tool returns error (404 from DS)
**Expected Result**: Security gate rejects the request

#### IT-HAPI-017-003-002: Security gate -- matching context returns workflow
**BR**: BR-HAPI-017-003
**Type**: Integration
**Category**: Happy Path
**File**: `holmesgpt-api/tests/integration/test_workflow_validation_integration.py`
**Description**: GetWorkflow with matching context returns full workflow detail
**Preconditions**: Real DS with workflow targeting `environment="production"`, `severity="critical"`
**Steps**:
1. Bootstrap workflow
2. Invoke `GetWorkflowTool` with matching context
3. Assert workflow returned with parameter schema
**Expected Result**: Security gate allows the request

#### IT-HAPI-017-004-001: Recovery validation loop with real DS -- retry
**BR**: BR-HAPI-017-004
**Type**: Integration
**Category**: Happy Path
**File**: `holmesgpt-api/tests/integration/test_recovery_validation_integration.py`
**Description**: Recovery validation loop retries when DS returns validation error
**Preconditions**: Real DS; mock LLM returning invalid then valid workflow
**Steps**:
1. First LLM response: non-existent workflow_id
2. Validator calls DS, gets 404
3. Second LLM response: valid workflow_id
4. Validator calls DS, gets 200
5. Assert result is valid after 2 attempts
**Expected Result**: Self-correction works with real DS

#### IT-HAPI-017-004-002: Recovery validation with real DS -- succeeds after correction
**BR**: BR-HAPI-017-004
**Type**: Integration
**Category**: Happy Path
**File**: `holmesgpt-api/tests/integration/test_recovery_validation_integration.py`
**Description**: Recovery flow produces valid result after LLM self-corrects parameters
**Preconditions**: Real DS; mock LLM returning wrong params then correct params
**Steps**:
1. First attempt: valid workflow but wrong parameter types
2. Validator detects parameter schema mismatch
3. Second attempt: correct parameters
4. Assert final result has `needs_human_review=False`
**Expected Result**: Parameter validation works with real DS schema

---

### Phase 8: DS Unit Tests (Go/Ginkgo)

#### UT-DS-017-001-001: ListActions handler -- valid request
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: Handler parses GET request with context filters and returns action types
**Preconditions**: Mock repository returning action type list
**Steps**:
1. Create HTTP GET request to `/api/v1/workflows/actions?severity=critical&component=web-api`
2. Call handler
3. Assert 200 response with action types JSON
4. Assert repository called with parsed filter parameters
**Expected Result**: Handler correctly parses and routes request

#### UT-DS-017-001-002: ListActions handler -- pagination
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: Handler respects offset and limit query parameters
**Preconditions**: Mock repository returning paginated results
**Steps**:
1. Create GET request with `?offset=20&limit=10`
2. Call handler
3. Assert repository called with offset=20, limit=10
4. Assert response includes `hasMore`, `total` fields
**Expected Result**: Pagination parameters forwarded to repository

#### UT-DS-017-001-003: ListActions handler -- context filter parsing
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: All six context filter query params correctly parsed
**Preconditions**: Mock repository
**Steps**:
1. Create GET with `?severity=critical&component=web-api&environment=production&priority=P1&custom_labels=team:platform&detected_labels=app:nginx`
2. Call handler
3. Assert repository receives all six filter parameters
**Expected Result**: Full filter set parsed from query string

#### UT-DS-017-001-004: ListWorkflowsByActionType handler -- valid action_type
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: Handler extracts action_type from URL path and queries repository
**Preconditions**: Mock repository returning workflow list for action type
**Steps**:
1. Create GET request to `/api/v1/workflows/actions/scale_up?severity=critical`
2. Call handler
3. Assert repository called with `action_type="scale_up"` and context filters
**Expected Result**: Path parameter correctly routed

#### UT-DS-017-001-005: ListWorkflowsByActionType handler -- unknown action_type
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Error Handling
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: Unknown action_type returns empty result (not error)
**Preconditions**: Mock repository returning empty list
**Steps**:
1. Create GET request with `action_type="nonexistent_action"`
2. Call handler
3. Assert 200 response with `{"workflows": [], "total": 0, "hasMore": false}`
**Expected Result**: Empty result, not 404 (action_type may be valid but have no workflows for context)

#### UT-DS-017-001-006: ListWorkflowsByActionType handler -- pagination
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: Pagination for workflow listing within action type
**Preconditions**: Mock repository returning paginated workflow results
**Steps**:
1. Create GET with `?offset=0&limit=5`
2. Assert response includes `hasMore: true` when more workflows exist
**Expected Result**: Pagination metadata correct

#### UT-DS-017-001-007: GetWorkflow handler -- context match
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: GetWorkflow returns workflow when context filters match
**Preconditions**: Mock repository returning workflow
**Steps**:
1. Create GET request to `/api/v1/workflows/uuid-1?severity=critical&component=web-api`
2. Call handler
3. Assert 200 response with full workflow detail
**Expected Result**: Workflow returned with parameter schema

#### UT-DS-017-001-008: GetWorkflow handler -- context mismatch (security gate)
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Security
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: GetWorkflow returns 404 when context filters exclude the workflow
**Preconditions**: Mock repository returning nil (workflow exists but excluded by context)
**Steps**:
1. Create GET request with mismatched context
2. Call handler
3. Assert 404 response with RFC 7807 `workflow-not-found` error type
**Expected Result**: Security gate returns 404 without distinguishing "not found" from "filtered out"

#### UT-DS-017-001-009: GetWorkflow handler -- nonexistent workflow_id
**BR**: BR-HAPI-017-001
**Type**: Unit
**Category**: Error Handling
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: Nonexistent workflow_id returns same 404 as context mismatch
**Preconditions**: Mock repository returning nil
**Steps**:
1. Create GET request with random UUID
2. Assert 404 response
**Expected Result**: Same 404 for both cases (no information leakage per DD-WORKFLOW-017)

#### UT-DS-017-005-001: remediationId propagated in handler
**BR**: BR-HAPI-017-005
**Type**: Unit
**Category**: Happy Path
**File**: `test/unit/datastorage/workflow_discovery_handler_test.go`
**Description**: remediationId query parameter parsed and passed to audit context
**Preconditions**: Mock repository and audit recorder
**Steps**:
1. Create GET request with `?remediation_id=rem-uuid-123`
2. Call handler
3. Assert audit event contains `remediationId: "rem-uuid-123"`
**Expected Result**: Audit correlation established

---

### Phase 9: DS Integration Tests (Go/Ginkgo)

#### IT-DS-017-001-001: ListActions repository -- active status filter
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Happy Path
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Description**: Repository returns only action types that have active workflows
**Preconditions**: Real PostgreSQL with active and disabled workflows
**Steps**:
1. Insert workflows: 2 with `action_type="scale_up"` (active), 1 with `action_type="restart_pod"` (disabled)
2. Call `ListActions(ctx, filters)`
3. Assert result includes `scale_up` but not `restart_pod`
**Expected Result**: Disabled workflows' action types excluded

#### IT-DS-017-001-002: ListActions repository -- pagination
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Happy Path
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Description**: Pagination returns correct slice and hasMore metadata
**Preconditions**: Real PostgreSQL with 5 distinct action types
**Steps**:
1. Insert workflows spanning 5 action types
2. Call `ListActions(ctx, filters, offset=0, limit=3)`
3. Assert 3 results returned, `hasMore=true`, `total=5`
4. Call with offset=3, limit=3
5. Assert 2 results returned, `hasMore=false`
**Expected Result**: Correct pagination behavior

#### IT-DS-017-001-003: ListWorkflowsByActionType repository -- filters
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Happy Path
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Description**: Repository filters workflows by action_type AND signal context
**Preconditions**: Real PostgreSQL with workflows of different action types and contexts
**Steps**:
1. Insert: `scale-conservative` (action_type=scale_up, severity=critical), `scale-aggressive` (action_type=scale_up, severity=warning), `restart-simple` (action_type=restart_pod, severity=critical)
2. Call `ListWorkflowsByActionType(ctx, "scale_up", filters={severity: "critical"})`
3. Assert only `scale-conservative` returned (matches both action_type and severity)
**Expected Result**: Dual filtering (action_type + context) works

#### IT-DS-017-001-004: ListWorkflowsByActionType -- excludes disabled
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Security
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Description**: Disabled workflows excluded from listing
**Preconditions**: Real PostgreSQL with active and disabled workflows of same action_type
**Steps**:
1. Insert active and disabled `scale_up` workflows
2. Call `ListWorkflowsByActionType(ctx, "scale_up", filters)`
3. Assert disabled workflow not in results
**Expected Result**: Only active workflows returned

#### IT-DS-017-001-005: GetWorkflowWithContextFilters -- context match
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Happy Path
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Description**: Repository returns workflow when context filters match
**Preconditions**: Real PostgreSQL with workflow matching given context
**Steps**:
1. Insert workflow with severity=critical, component=web-api, environment=production
2. Call `GetWorkflowWithContextFilters(ctx, "uuid-1", {severity: "critical", component: "web-api", environment: "production"})`
3. Assert workflow returned with all fields
**Expected Result**: Full workflow returned

#### IT-DS-017-001-006: GetWorkflowWithContextFilters -- context mismatch
**BR**: BR-HAPI-017-001
**Type**: Integration
**Category**: Security
**File**: `test/integration/datastorage/workflow_discovery_repository_test.go`
**Description**: Repository returns nil when context filters exclude the workflow
**Preconditions**: Real PostgreSQL with workflow NOT matching given context
**Steps**:
1. Insert workflow with severity=critical, environment=production
2. Call `GetWorkflowWithContextFilters(ctx, "uuid-1", {severity: "warning", environment: "staging"})`
3. Assert result is nil
**Expected Result**: No workflow returned (security gate)

#### IT-DS-017-005-001: Audit events include remediationId
**BR**: BR-HAPI-017-005
**Type**: Integration
**Category**: Happy Path
**File**: `test/integration/datastorage/workflow_discovery_audit_test.go`
**Description**: Discovery audit events include remediationId for correlation
**Preconditions**: Real PostgreSQL with audit table
**Steps**:
1. Call ListActions endpoint with `remediation_id=rem-uuid-123`
2. Query audit_events table for `workflow.catalog.actions_listed`
3. Assert event payload contains `remediationId: "rem-uuid-123"`
**Expected Result**: Audit correlation established in database

---

### Phase 10: DS E2E Tests (Go/Ginkgo)

#### E2E-DS-017-001-001: Three-step endpoints happy path
**BR**: BR-HAPI-017-001
**Type**: E2E
**Category**: Happy Path
**File**: `test/e2e/datastorage/04_workflow_discovery_test.go` (replaces `04_workflow_search_test.go`)
**Description**: Full three-step discovery via HTTP against deployed DS
**Preconditions**: Kind cluster with DS, PostgreSQL, Redis; seeded workflows
**Steps**:
1. Call `GET /api/v1/workflows/actions?severity=critical` via ogen client
2. Assert response contains action types
3. Select first action_type, call `GET /api/v1/workflows/actions/{action_type}?severity=critical`
4. Assert response contains workflows for that action type
5. Select first workflow_id, call `GET /api/v1/workflows/{workflow_id}?severity=critical`
6. Assert response contains full workflow with parameter schema
**Expected Result**: Complete three-step flow works end-to-end

#### E2E-DS-017-001-002: Disabled workflow excluded from discovery
**BR**: BR-HAPI-017-001
**Type**: E2E
**Category**: Security
**File**: `test/e2e/datastorage/04_workflow_discovery_test.go`
**Description**: Disabled workflows do not appear in step 1 or step 2 results
**Preconditions**: Kind cluster; one active and one disabled workflow of same action type
**Steps**:
1. Create active and disabled workflows
2. Call step 1 and step 2 endpoints
3. Assert disabled workflow absent from all results
**Expected Result**: Discovery only surfaces active workflows

#### E2E-DS-017-001-003: Security gate 404 via E2E
**BR**: BR-HAPI-017-001, BR-HAPI-017-003
**Type**: E2E
**Category**: Security
**File**: `test/e2e/datastorage/04_workflow_discovery_test.go`
**Description**: GetWorkflow with mismatched context returns 404 in deployed DS
**Preconditions**: Kind cluster; workflow with specific context
**Steps**:
1. Create workflow for severity=critical, environment=production
2. Call `GET /api/v1/workflows/{id}?severity=warning&environment=staging`
3. Assert 404 response with RFC 7807 body
**Expected Result**: Security gate enforced in production-like environment

#### E2E-DS-017-AUDIT-001: workflow.catalog.actions_listed audit event
**BR**: BR-AUDIT-023
**Type**: E2E
**Category**: Observability
**File**: `test/e2e/datastorage/06_workflow_discovery_audit_test.go` (replaces `06_workflow_search_audit_test.go`)
**Description**: DS emits `workflow.catalog.actions_listed` audit event after step 1 call
**Preconditions**: Kind cluster with DS
**Steps**:
1. Call `GET /api/v1/workflows/actions?severity=critical&remediation_id=rem-123`
2. Query audit events via DS audit API
3. Assert event with `eventType="workflow.catalog.actions_listed"` exists
4. Assert payload contains `remediationId`, `filtersApplied`, `resultCount`
**Expected Result**: Audit event emitted with correct payload per DD-WORKFLOW-014 v3.0

#### E2E-DS-017-AUDIT-002: workflow.catalog.workflows_listed audit event
**BR**: BR-AUDIT-023
**Type**: E2E
**Category**: Observability
**File**: `test/e2e/datastorage/06_workflow_discovery_audit_test.go`
**Description**: DS emits `workflow.catalog.workflows_listed` audit event after step 2 call
**Preconditions**: Kind cluster with DS
**Steps**:
1. Call `GET /api/v1/workflows/actions/scale_up?severity=critical&remediation_id=rem-123`
2. Query audit events
3. Assert event with `eventType="workflow.catalog.workflows_listed"` exists
4. Assert payload contains `actionType`, `remediationId`, `workflowCount`
**Expected Result**: Step-specific audit event emitted

#### E2E-DS-017-AUDIT-003: workflow.catalog.workflow_retrieved audit event
**BR**: BR-AUDIT-023
**Type**: E2E
**Category**: Observability
**File**: `test/e2e/datastorage/06_workflow_discovery_audit_test.go`
**Description**: DS emits `workflow.catalog.workflow_retrieved` audit event after step 3 call
**Preconditions**: Kind cluster with DS; known workflow_id
**Steps**:
1. Call `GET /api/v1/workflows/{id}?severity=critical&remediation_id=rem-123`
2. Query audit events
3. Assert event with `eventType="workflow.catalog.workflow_retrieved"` exists
4. Assert payload contains `workflowId`, `remediationId`
**Expected Result**: Step 3 audit event emitted

#### E2E-DS-017-AUDIT-004: workflow.catalog.selection_validated audit event
**BR**: BR-AUDIT-023
**Type**: E2E
**Category**: Observability
**File**: `test/e2e/datastorage/06_workflow_discovery_audit_test.go`
**Description**: DS emits `workflow.catalog.selection_validated` audit event after post-selection validation call
**Preconditions**: Kind cluster with DS
**Steps**:
1. Call `GET /api/v1/workflows/{id}?severity=critical&remediation_id=rem-123&validation=true`
2. Query audit events
3. Assert event with `eventType="workflow.catalog.selection_validated"` exists
4. Assert payload contains `workflowId`, `remediationId`, `validationResult`
**Expected Result**: Validation-specific audit event emitted

#### E2E-DS-017-006-001: Old search endpoint removed
**BR**: BR-HAPI-017-006
**Type**: E2E
**Category**: Security
**File**: `test/e2e/datastorage/04_workflow_discovery_test.go`
**Description**: `POST /api/v1/workflows/search` returns 404 or 405 (endpoint removed)
**Preconditions**: Kind cluster with DS
**Steps**:
1. Call `POST /api/v1/workflows/search` with valid body
2. Assert 404 (Not Found) or 405 (Method Not Allowed)
**Expected Result**: Old endpoint no longer exists

---

### Phase 11: HAPI E2E Tests

#### E2E-HAPI-017-001-001: Incident flow three-step discovery
**BR**: BR-HAPI-017-001
**Type**: E2E
**Category**: Happy Path
**File**: `holmesgpt-api/tests/e2e/test_three_step_discovery_e2e.py`
**Description**: Full incident analysis uses three-step discovery with Mock LLM
**Preconditions**: Kind cluster with HAPI, DS, Mock LLM; seeded workflows with action_type taxonomy
**Steps**:
1. Call HAPI incident analyze endpoint via OpenAPI client
2. Mock LLM programmed to call `list_available_actions`, then `list_workflows`, then `get_workflow`
3. Assert HAPI returns valid investigation result with selected workflow
4. Query DS audit events for all four discovery audit events
**Expected Result**: Full three-step flow executes; audit trail complete

#### E2E-HAPI-017-001-002: Recovery flow three-step discovery
**BR**: BR-HAPI-017-001
**Type**: E2E
**Category**: Happy Path
**File**: `holmesgpt-api/tests/e2e/test_recovery_three_step_e2e.py`
**Description**: Full recovery analysis uses three-step discovery with Mock LLM
**Preconditions**: Kind cluster with HAPI, DS, Mock LLM; seeded workflows
**Steps**:
1. Call HAPI recovery analyze endpoint
2. Mock LLM follows three-step protocol
3. Assert valid recovery result with selected workflow
**Expected Result**: Recovery flow uses same three-step protocol as incident

#### E2E-HAPI-017-004-001: Recovery flow validation loop E2E
**BR**: BR-HAPI-017-004
**Type**: E2E
**Category**: Happy Path
**File**: `holmesgpt-api/tests/e2e/test_recovery_three_step_e2e.py`
**Description**: Recovery validation loop retries and self-corrects with Mock LLM
**Preconditions**: Kind cluster; Mock LLM programmed to return invalid workflow on first attempt, valid on second
**Steps**:
1. Configure Mock LLM scenario: first response has invalid workflow_id, second has valid
2. Call HAPI recovery analyze endpoint
3. Assert final result is valid (self-corrected)
4. Query DS audit events for `aiagent.workflow.validation_attempt` events
5. Assert 2 validation attempts recorded
**Expected Result**: Self-correction works end-to-end in recovery flow

---

## 4. Fixture Updates Required

### HAPI Fixtures (`holmesgpt-api/tests/fixtures/workflow_fixtures.py`)

The existing `WorkflowFixture` and `bootstrap_workflows()` must be updated for the three-step protocol:

| Change | Detail |
|--------|--------|
| Add `action_type` field to `WorkflowFixture` | New mandatory field per DD-WORKFLOW-016 taxonomy |
| Update `TEST_WORKFLOWS` list | Each fixture gets an `action_type` value (e.g., `scale_up`, `restart_pod`, `rollback_deployment`) |
| Add `bootstrap_action_type_taxonomy()` | Seed action types in DS before workflow creation |
| Update `bootstrap_workflows()` | Use new DS `POST /api/v1/workflows` payload (OCI pullspec per DD-WORKFLOW-017 -- but may still use direct create for test seeding) |
| Add pagination fixtures | 25+ workflows for pagination testing |
| Add `get_workflows_by_action_type()` helper | Filter test fixtures by action_type |

### DS Fixtures (`test/infrastructure/`)

| Change | Detail |
|--------|--------|
| Update `workflow_seeding.go` | Seed workflows with `action_type` field |
| Update `workflow_bundles.go` | Include action_type in bundle metadata |
| Add action_type taxonomy seeding | Insert taxonomy entries for test action types |
| Add context-specific workflows | Workflows with specific severity/environment combinations for security gate tests |

---

## 5. Coverage Targets

Per TESTING_GUIDELINES v2.7.0 (>=80% per tier, measured against tier-specific testable code):

| Tier | Code Subset | Target | Actual |
|------|-------------|--------|--------|
| HAPI Unit | Tool classes, prompt builders, validator, recovery loop | >=80% | ⏸️ |
| HAPI Integration | Tool -> DS client round-trip, validation loop | >=80% | ⏸️ |
| DS Unit | Handler parsing, repository query building, audit emission | >=80% | ⏸️ |
| DS Integration | Repository queries against real PostgreSQL | >=80% | ⏸️ |
| E2E (both) | Full stack discovery flow | >=80% | ⏸️ |
| BR Coverage | 100% of BR-HAPI-017-001 through 006 | 100% | ⏸️ |
| Critical Path (P0) | All P0 test cases pass | 100% | ⏸️ |

---

## 6. Test File Locations

| Component | Unit | Integration | E2E |
|-----------|------|-------------|-----|
| HAPI tools | `holmesgpt-api/tests/unit/test_three_step_discovery_tools.py` | `holmesgpt-api/tests/integration/test_three_step_discovery_integration.py` | `holmesgpt-api/tests/e2e/test_three_step_discovery_e2e.py` |
| HAPI prompts | `holmesgpt-api/tests/unit/test_prompt_builder_three_step.py` | -- | -- |
| HAPI validator | `holmesgpt-api/tests/unit/test_workflow_validation_context_filters.py` | `holmesgpt-api/tests/integration/test_workflow_validation_integration.py` | -- |
| HAPI recovery loop | `holmesgpt-api/tests/unit/test_recovery_validation_loop.py` | `holmesgpt-api/tests/integration/test_recovery_validation_integration.py` | `holmesgpt-api/tests/e2e/test_recovery_three_step_e2e.py` |
| DS handlers | `test/unit/datastorage/workflow_discovery_handler_test.go` | -- | -- |
| DS repository | -- | `test/integration/datastorage/workflow_discovery_repository_test.go` | -- |
| DS audit | -- | `test/integration/datastorage/workflow_discovery_audit_test.go` | `test/e2e/datastorage/06_workflow_discovery_audit_test.go` |
| DS E2E | -- | -- | `test/e2e/datastorage/04_workflow_discovery_test.go` |

---

## 7. Sign-off

| Role | Name | Date | Signature |
|------|------|------|-----------|
| Author | AI Assistant | 2026-02-05 | ⏸️ |
| Reviewer | Jordi Gil | | ⏸️ |
| Approver | | | ⏸️ |
