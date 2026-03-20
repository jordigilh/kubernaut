# Test Plan: DSClientAdapter RFC 7807 Error Handling for Workflow Operations

**Feature**: Surface actionable RFC 7807 error details from Data Storage for CreateWorkflow/DisableWorkflow operations
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `release/v1.1.0-rc2`

**Authority**:
- [BR-WORKFLOW-006](../../requirements/BR-WORKFLOW-006-remediation-workflow-crd.md): Kubernetes-native workflow registration — acceptance criteria #2: "CREATE triggers DS registration; CRD is rejected if DS registration fails" with DS error surfaced
- [DD-004](../../architecture/decisions/DD-004-RFC7807-ERROR-RESPONSES.md): RFC 7807 Error Response Standard — clients must surface problem details
- [ADR-058](../../architecture/decisions/ADR-058-webhook-driven-workflow-registration.md): Webhook-driven workflow registration

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- GitHub Issue: [#446](https://github.com/jordigilh/kubernaut/issues/446)
- Related: [#445](https://github.com/jordigilh/kubernaut/issues/445) (root cause investigation)

---

## 1. Scope

### In Scope

- `pkg/authwebhook/ds_client.go` (`CreateWorkflowInline`): Add RFC 7807 error extraction for 400, 401, 403, 409, 500 response types
- `pkg/authwebhook/ds_client.go` (`DisableWorkflow`): Capture response and add type-switch for 200, 400, 404 response types (fixes silent error swallowing)
- `test/unit/authwebhook/ds_client_workflow_test.go`: Unit tests for all response type mappings

### Out of Scope

- RemediationWorkflow handler logic (`remediationworkflow_handler.go`): Already correctly propagates adapter errors to admission responses; no changes needed
- DataStorage server-side error generation: DS already returns RFC 7807 compliant responses
- Integration/E2E tests: Adapter is a pure mapping layer; full CRD-to-DS pipeline already covered by BR-WORKFLOW-006 E2E suite (#299)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Follow `DisableActionType` pattern for error extraction | Established pattern in same file (lines 229-237) already handles RFC 7807 cast + Title/Detail extraction |
| Use `application/problem+json` in test fixtures | Ogen response decoder requires this content type for error responses; `application/json` causes decode failure (validated in risk assessment) |
| Include all 3 required RFC 7807 fields in test fixtures | Ogen decoder requires `type` (URI), `title` (string), `status` (int32); `detail` is optional but included for assertion |
| DisableWorkflow captures response instead of discarding | Current `_, disableErr :=` silently swallows 400/404 typed responses where `err == nil` |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (adapter response mapping in `CreateWorkflowInline` and `DisableWorkflow`)

### 2-Tier Minimum

This change qualifies for a single-tier exception:
- **Unit tests** cover 100% of the new adapter response mapping logic (pure type-switch with no I/O)
- The handler-to-adapter integration is already tested in `remediationworkflow_handler_test.go` via mock `WorkflowCatalogClient`
- The full CRD-to-DS pipeline is tested in the BR-WORKFLOW-006 E2E suite

### Business Outcome Quality Bar

Tests validate that **operators receive actionable error messages** when workflow registration or disable fails — the actual business outcome surfaced through `kubectl apply` admission denials, not just code path coverage.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/authwebhook/ds_client.go` | `CreateWorkflowInline` (switch cases) | ~25 (new) |
| `pkg/authwebhook/ds_client.go` | `DisableWorkflow` (response capture + switch) | ~20 (new) |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| N/A — adapter is pure mapping | — | — |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-006 | DS 400 Bad Request surfaces RFC 7807 title+detail in admission denial | P0 | Unit | UT-AW-446-001 | Pending |
| BR-WORKFLOW-006 | DS 409 Conflict surfaces "already exists" with RFC 7807 details | P1 | Unit | UT-AW-446-002 | Pending |
| DD-004 | DS 403 Forbidden surfaces RFC 7807 details | P1 | Unit | UT-AW-446-003 | Pending |
| DD-004 | DS 401 Unauthorized surfaces RFC 7807 details | P1 | Unit | UT-AW-446-004 | Pending |
| DD-004 | DS 500 Internal Server Error surfaces RFC 7807 details | P1 | Unit | UT-AW-446-005 | Pending |
| BR-WORKFLOW-006 | DS 201 Created returns WorkflowRegistrationResult (no regression) | P0 | Unit | UT-AW-446-006 | Pending |
| BR-WORKFLOW-006 | DS 200 OK re-enable returns WorkflowRegistrationResult with PreviouslyExisted (no regression) | P0 | Unit | UT-AW-446-007 | Pending |
| BR-WORKFLOW-006 | DS 400 Bad Request on disable surfaces RFC 7807 details | P1 | Unit | UT-AW-446-008 | Pending |
| BR-WORKFLOW-006 | DS 404 Not Found on disable surfaces RFC 7807 details | P1 | Unit | UT-AW-446-009 | Pending |
| BR-WORKFLOW-006 | DS 200 OK on disable returns success (no regression) | P0 | Unit | UT-AW-446-010 | Pending |

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

- **TIER**: `UT` (Unit)
- **SERVICE**: AW (AuthWebhook)
- **BR_NUMBER**: 446 (Issue number)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**: `CreateWorkflowInline` and `DisableWorkflow` response type mapping in `pkg/authwebhook/ds_client.go`, targeting >=80% of all response paths.

**CreateWorkflowInline error handling:**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AW-446-001` | Operator sees "workflow registration rejected: {title} — {detail}" in admission denial when DS returns 400 (e.g., missing bundle image) | RED |
| `UT-AW-446-002` | Operator sees "workflow already exists: {title} — {detail}" when DS returns 409 (duplicate name+version) | RED |
| `UT-AW-446-003` | Operator sees "workflow registration forbidden: {title} — {detail}" when DS returns 403 | RED |
| `UT-AW-446-004` | Operator sees "workflow registration unauthorized: {title} — {detail}" when DS returns 401 | RED |
| `UT-AW-446-005` | Operator sees "workflow registration server error: {title} — {detail}" when DS returns 500 | RED |
| `UT-AW-446-006` | Successful 201 Created returns correct WorkflowRegistrationResult (no regression) | RED |
| `UT-AW-446-007` | Successful 200 OK re-enable returns WorkflowRegistrationResult with PreviouslyExisted=true (no regression) | RED |

**DisableWorkflow error handling:**

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AW-446-008` | Disable returns error with "disable workflow: bad request: {title} — {detail}" when DS returns 400 | RED |
| `UT-AW-446-009` | Disable returns error with "disable workflow: not found: {title} — {detail}" when DS returns 404 | RED |
| `UT-AW-446-010` | Successful 200 OK disable returns nil error (no regression) | RED |

### Tier Skip Rationale

- **Integration**: This change is pure adapter-layer response mapping (no I/O beyond the httptest server). The existing `remediationworkflow_handler_test.go` already tests the handler-to-adapter integration with mock `WorkflowCatalogClient`. Real DS integration is tested in E2E.
- **E2E**: The full CRD-to-DS pipeline is already covered by the existing E2E suite for BR-WORKFLOW-006 (#299).

---

## 6. Test Cases (Detail)

### UT-AW-446-001: CreateWorkflow 400 Bad Request surfaces RFC 7807 details

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 400 with `Content-Type: application/problem+json` and RFC 7807 body containing title "Workflow Validation Failed" and detail "Execution bundle image not found in registry"
**When**: `CreateWorkflowInline` is called with valid content, source, and registeredBy
**Then**: Returns error containing "workflow registration rejected: Workflow Validation Failed" and "Execution bundle image not found in registry"

**Acceptance Criteria**:
- Error is non-nil
- Error message contains the RFC 7807 title
- Error message contains the RFC 7807 detail
- No WorkflowRegistrationResult returned

### UT-AW-446-002: CreateWorkflow 409 Conflict surfaces RFC 7807 details

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 409 with RFC 7807 body containing title "Workflow Conflict" and detail "Workflow crashloop-rollback v1.0.0 already exists"
**When**: `CreateWorkflowInline` is called
**Then**: Returns error containing "workflow already exists: Workflow Conflict" and the detail string

**Acceptance Criteria**:
- Error message includes "already exists" for operator clarity
- RFC 7807 title and detail are both present

### UT-AW-446-003: CreateWorkflow 403 Forbidden surfaces RFC 7807 details

**BR**: DD-004
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 403 with RFC 7807 body
**When**: `CreateWorkflowInline` is called
**Then**: Returns error containing "workflow registration forbidden" with RFC 7807 title and detail

**Acceptance Criteria**:
- Error is non-nil with RFC 7807 details

### UT-AW-446-004: CreateWorkflow 401 Unauthorized surfaces RFC 7807 details

**BR**: DD-004
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 401 with RFC 7807 body
**When**: `CreateWorkflowInline` is called
**Then**: Returns error containing "workflow registration unauthorized" with RFC 7807 title and detail

**Acceptance Criteria**:
- Error is non-nil with RFC 7807 details

### UT-AW-446-005: CreateWorkflow 500 Internal Server Error surfaces RFC 7807 details

**BR**: DD-004
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 500 with RFC 7807 body
**When**: `CreateWorkflowInline` is called
**Then**: Returns error containing "workflow registration server error" with RFC 7807 title and detail

**Acceptance Criteria**:
- Error is non-nil with RFC 7807 details

### UT-AW-446-006: CreateWorkflow 201 Created (no regression)

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 201 with valid RemediationWorkflow JSON
**When**: `CreateWorkflowInline` is called
**Then**: Returns WorkflowRegistrationResult with correct WorkflowID, WorkflowName, Version, Status; PreviouslyExisted=false

**Acceptance Criteria**:
- No error returned
- WorkflowID matches UUID from response
- PreviouslyExisted is false

### UT-AW-446-007: CreateWorkflow 200 OK re-enable (no regression)

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 200 with valid RemediationWorkflow JSON (re-enabled workflow)
**When**: `CreateWorkflowInline` is called
**Then**: Returns WorkflowRegistrationResult with PreviouslyExisted=true

**Acceptance Criteria**:
- No error returned
- PreviouslyExisted is true

### UT-AW-446-008: DisableWorkflow 400 Bad Request surfaces RFC 7807 details

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 400 with `Content-Type: application/problem+json` and RFC 7807 body
**When**: `DisableWorkflow` is called with valid workflowID
**Then**: Returns error containing "disable workflow" and "bad request" with RFC 7807 title and detail

**Acceptance Criteria**:
- Error is non-nil (previously returned nil — this is the silent failure fix)
- Error message contains RFC 7807 details

### UT-AW-446-009: DisableWorkflow 404 Not Found surfaces RFC 7807 details

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 404 with RFC 7807 body
**When**: `DisableWorkflow` is called
**Then**: Returns error containing "disable workflow" and "not found" with RFC 7807 title and detail

**Acceptance Criteria**:
- Error is non-nil
- Error message contains RFC 7807 details

### UT-AW-446-010: DisableWorkflow 200 OK (no regression)

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/authwebhook/ds_client_workflow_test.go`

**Given**: httptest server returns 200 with valid RemediationWorkflow JSON
**When**: `DisableWorkflow` is called
**Then**: Returns nil error

**Acceptance Criteria**:
- Error is nil
- No panic or unexpected behavior

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `httptest.NewServer` simulating Data Storage API responses (external dependency mock — compliant with mock strategy)
- **Key technique**: Build ogen `Client` against httptest server URL, wrap in `DSClientAdapter` via `NewDSClientAdapterFromClient`
- **Content-Type**: Error responses use `application/problem+json`; success responses use `application/json`
- **Location**: `test/unit/authwebhook/`

---

## 8. Execution

```bash
# Unit tests (all authwebhook)
go test ./test/unit/authwebhook/... -v

# Specific test by ID
go test ./test/unit/authwebhook/... -ginkgo.focus="UT-AW-446"

# Just CreateWorkflow tests
go test ./test/unit/authwebhook/... -ginkgo.focus="CreateWorkflowInline"

# Just DisableWorkflow tests
go test ./test/unit/authwebhook/... -ginkgo.focus="DisableWorkflow"
```

---

## 9. Anti-Pattern Compliance

Per `TESTING_GUIDELINES.md`:

| Anti-Pattern | Status | Notes |
|-------------|--------|-------|
| `time.Sleep()` | COMPLIANT | Pure synchronous request/response; no async waits |
| `Skip()` / `XIt` | COMPLIANT | All 10 test scenarios implemented |
| Direct audit testing | N/A | Tests validate adapter error mapping, not audit infrastructure |
| HTTP endpoint testing | COMPLIANT | `httptest` simulates external dependency (DS), not testing HTTP contracts |

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for Issue #446 |
