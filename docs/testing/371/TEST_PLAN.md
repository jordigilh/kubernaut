# Test Plan: Enforce Single Active Workflow Per Name

**Feature**: Enforce single active catalog entry per (workflow_name, action_type) — supersede old versions on update
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Active
**Branch**: `fix/v1.0.1-chart-platform-agnostic`

**Authority**:
- BR-WORKFLOW-006: Content hash verification and workflow registration
- BR-STORAGE-012: Workflow catalog persistence
- DD-WORKFLOW-002 v3.0: Workflow versioning and is_latest_version flag
- DD-WORKFLOW-017: Workflow lifecycle component interactions

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- GitHub Issue: #371

---

## 1. Scope

### In Scope

- **DS Handler cross-version supersession**: `handleDuplicateWorkflow` in `workflow_handlers.go` enhanced with cross-version active lookup
- **DS Repository**: `GetActiveByWorkflowName` new query method in `crud.go` (supersession uses existing `UpdateStatus`)
- **AuthWebhook UPDATE handling**: `handleUpdate` method in `remediationworkflow_handler.go` to forward CRD updates to DS
- **WorkflowContentIntegrityRepository interface**: Extended with `GetActiveByWorkflowName`

### Out of Scope

- DELETE behavior (remains `disabled` per user decision — unchanged)
- LLM discovery query logic (already filters by `status='active' AND is_latest_version=true`)
- OpenAPI spec changes for audit event types (UPDATE reuses CREATE audit event)
- Database schema changes (existing `status` column already supports `superseded` value)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Supersession key is `(workflow_name, action_type)` | Matches issue spec; allows same name with different action types as distinct workflows |
| UPDATE reuses CREATE audit event type | Avoids OpenAPI scope creep; DS operation is identical (CreateWorkflowInline) |
| Cross-version check runs after same-version check | Preserves existing idempotency for same-version re-apply; only triggers for genuine version upgrades |
| Cross-version supersession uses existing `UpdateStatus` | Reuses proven repository method; avoids adding untested batch-update query |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (handleDuplicateWorkflow cross-version branch, AW handleUpdate routing)
- **Integration**: >=80% of integration-testable code (full lifecycle with real DB: create v1 -> update v2 -> verify supersession)

### 2-Tier Minimum

All business requirement gaps covered by UT + IT.

### Business Outcome Quality Bar

Tests validate: "Does the system guarantee only one active workflow per (name, actionType)?" and "Does updating a CRD version correctly supersede the old entry?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/server/workflow_handlers.go` | `handleDuplicateWorkflow` (modified: cross-version branch) | ~20 new |
| `pkg/authwebhook/remediationworkflow_handler.go` | `handleUpdate` (new), `Handle` (modified switch) | ~30 new |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/workflow/crud.go` | `GetActiveByWorkflowName` | ~25 new |
| Full handler + repository wiring | End-to-end lifecycle | ~50 covered |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-006 | Cross-version supersession in handleDuplicateWorkflow | P0 | Unit | UT-DS-371-001 | Pass |
| BR-WORKFLOW-006 | Cross-version detection: new version supersedes old active entry | P0 | Unit | UT-DS-371-002 | Pass |
| BR-WORKFLOW-006 | Idempotent re-apply: same name+version+hash returns 200 | P0 | Unit | UT-DS-371-003 | Pass |
| BR-WORKFLOW-006 | AuthWebhook UPDATE triggers DS registration | P0 | Unit | UT-AW-371-001 | Pass |
| BR-WORKFLOW-006 | AuthWebhook UPDATE idempotent (same content) | P1 | Unit | UT-AW-371-002 | Pass |
| BR-WORKFLOW-006 | Full lifecycle: create v1 -> update v2 -> only v2 active | P0 | Integration | IT-DS-371-001 | Pass (compiles, requires infra) |
| BR-WORKFLOW-006 | Delete+recreate: old disabled, new active, one visible | P1 | Integration | IT-DS-371-002 | Pass (compiles, requires infra) |

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-371-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: DS (DataStorage), AW (AuthWebhook)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-371-001` | System supersedes old active entry when a workflow with the same name is registered via a different version — operator sees old entry marked superseded | Pass |
| `UT-DS-371-002` | System correctly detects cross-version active conflict and creates new entry after superseding — LLM only sees newest version | Pass |
| `UT-DS-371-003` | System preserves idempotent behavior for same name+version+hash re-apply — no unnecessary supersession or creation | Pass |
| `UT-AW-371-001` | AuthWebhook forwards CRD UPDATE to DS for registration — spec changes are reflected in catalog | Pass |
| `UT-AW-371-002` | AuthWebhook UPDATE with unchanged content is idempotent — DS returns 200, no new entry | Pass |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-371-001` | End-to-end: create workflow v1.0.0 then v1.0.1 for same name — v1.0.0 becomes superseded, LLM discovery returns only v1.0.1 | Pass (compiles, requires infra) |
| `IT-DS-371-002` | Delete+recreate pattern: old entry disabled, new entry active, discovery returns only the new entry | Pass (compiles, requires infra) |

---

## 6. Test Cases (Detail)

### UT-DS-371-001: System supersedes old active entry on cross-version registration

**BR**: BR-WORKFLOW-006
**Type**: Unit (behavior)
**File**: `test/unit/datastorage/workflow_content_integrity_test.go`

**Given**: An active workflow entry exists with name="git-revert-v1", version="1.0.0", actionType="GitRevertCommit".
**When**: A new workflow with name="git-revert-v1", version="1.0.1", actionType="GitRevertCommit" (different content hash) is registered via handleDuplicateWorkflow.
**Then**: The old v1.0.0 entry is marked as `superseded`, and the new v1.0.1 entry is created as `active`.

**Acceptance Criteria**:
- Old entry UpdateStatus called with status="superseded"
- New entry created with status="active"
- `WorkflowRegistrationResult.Superseded` is true
- `WorkflowRegistrationResult.SupersededID` equals old entry's UUID
- HTTP status code is 201 (Created)

---

### UT-DS-371-002: Cross-version detection creates new entry after superseding

**BR**: BR-WORKFLOW-006
**Type**: Unit (correctness)
**File**: `test/unit/datastorage/workflow_content_integrity_test.go`

**Given**: No active workflow with name="git-revert-v1" version="1.0.1" exists, but an active workflow with name="git-revert-v1" version="1.0.0" exists (different version, returned by GetActiveByWorkflowName).
**When**: handleDuplicateWorkflow is invoked with the v1.0.1 workflow.
**Then**: The v1.0.0 entry is superseded and a new v1.0.1 entry is created.

**Acceptance Criteria**:
- `GetActiveByWorkflowName` is called after `GetActiveByNameAndVersion` returns nil
- Supersede + create flow matches the same-version different-hash path
- Response status is 201

---

### UT-DS-371-003: Idempotent re-apply returns 200 without supersession

**BR**: BR-WORKFLOW-006
**Type**: Unit (correctness)
**File**: `test/unit/datastorage/workflow_content_integrity_test.go`

**Given**: An active workflow with name="git-revert-v1", version="1.0.0", contentHash="abc123" exists.
**When**: The exact same workflow (same name, version, hash) is registered via handleDuplicateWorkflow.
**Then**: The system returns 200 with the existing entry — no supersession, no new entry.

**Acceptance Criteria**:
- No UpdateStatus calls (no supersession)
- No Create calls (no new entry)
- HTTP status code is 200
- Returned workflow matches the existing entry

---

### UT-AW-371-001: AuthWebhook UPDATE triggers DS registration

**BR**: BR-WORKFLOW-006
**Type**: Unit (behavior)
**File**: `test/unit/authwebhook/remediationworkflow_handler_test.go`

**Given**: A RemediationWorkflow CRD UPDATE admission request.
**When**: The AuthWebhook handles the request.
**Then**: The webhook calls `dsClient.CreateWorkflowInline` with the updated CRD content, and returns Allowed.

**Acceptance Criteria**:
- DS `CreateWorkflowInline` is called exactly once
- Response is `Allowed`
- Content passed to DS matches the clean CRD content (no runtime metadata)

---

### UT-AW-371-002: AuthWebhook UPDATE with same content is idempotent

**BR**: BR-WORKFLOW-006
**Type**: Unit (correctness)
**File**: `test/unit/authwebhook/remediationworkflow_handler_test.go`

**Given**: A RemediationWorkflow CRD UPDATE where the spec hasn't changed (DS returns 200 idempotent).
**When**: The AuthWebhook handles the request.
**Then**: The webhook returns Allowed and DS reports the existing entry (no new creation).

**Acceptance Criteria**:
- DS `CreateWorkflowInline` called once (DS handles idempotency)
- Response is `Allowed`
- CRD status update reflects `PreviouslyExisted=true`

---

### IT-DS-371-001: Full lifecycle — create v1.0.0, then v1.0.1, verify supersession

**BR**: BR-WORKFLOW-006
**Type**: Integration (behavior + correctness)
**File**: `test/integration/datastorage/workflow_supersession_test.go`

**Given**: A workflow "git-revert-v1" v1.0.0 is created in the catalog via POST /api/v1/workflows.
**When**: A new version "git-revert-v1" v1.0.1 (different content) is registered via the same endpoint.
**Then**: The v1.0.0 entry is superseded, v1.0.1 is active, and LLM discovery only returns v1.0.1.

**Acceptance Criteria**:
- v1.0.0 entry: status=superseded, is_latest_version=false
- v1.0.1 entry: status=active, is_latest_version=true
- GET /api/v1/workflows/actions/{actionType} returns only v1.0.1
- v1.0.0 is still queryable by UUID for audit trail

---

### IT-DS-371-002: Delete+recreate — old disabled, new active

**BR**: BR-WORKFLOW-006
**Type**: Integration (behavior)
**File**: `test/integration/datastorage/workflow_supersession_test.go`

**Given**: A workflow "git-revert-v1" v1.0.0 exists and is then disabled (simulating CRD DELETE).
**When**: A new "git-revert-v1" v1.0.1 (different content) is registered.
**Then**: The disabled v1.0.0 remains disabled, v1.0.1 is the only active entry.

**Acceptance Criteria**:
- v1.0.0: status=disabled (from delete, unchanged)
- v1.0.1: status=active, is_latest_version=true
- Discovery returns only v1.0.1

---

## Anti-Pattern Compliance (TESTING_GUIDELINES v2.7.0)

| Anti-Pattern | Status | Notes |
|-------------|--------|-------|
| `time.Sleep()` FORBIDDEN | Compliant | Integration tests use `Eventually()` for async operations |
| `Skip()` FORBIDDEN | Compliant | No conditional skips |
| Testing implementation details | Compliant | Tests verify business outcomes (supersession state, discovery visibility) |

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `mockWorkflowIntegrityRepo` (extended with `GetActiveByWorkflowName`), `mockWorkflowCatalogClient`
- **Location**: `test/unit/datastorage/workflow_content_integrity_test.go`, `test/unit/authwebhook/remediationworkflow_handler_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Infrastructure**: PostgreSQL (real DB), DS HTTP server
- **Location**: `test/integration/datastorage/workflow_supersession_test.go`

---

## 8. Execution

```bash
# Unit tests — DS content integrity (includes supersession tests)
go test ./test/unit/datastorage/... --ginkgo.focus="UT-DS-371"

# Unit tests — AW handler
go test ./test/unit/authwebhook/... --ginkgo.focus="UT-AW-371"

# Integration tests — workflow supersession
go test ./test/integration/datastorage/... --ginkgo.focus="IT-DS-371"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan for #371 (Enforce Single Active Workflow Per Name) |
| 1.1 | 2026-03-04 | All unit tests passing; integration tests compile-verified |
