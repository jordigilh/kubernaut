# Test Plan: Deterministic UUIDs + Authwebhook Startup Reconciliation (#548)

**Feature**: PVC-wipe resilience via deterministic workflow UUIDs and authwebhook startup reconciliation of CRDs to DataStorage
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Agent
**Status**: Draft
**Branch**: `development/v1.2`

**Authority**:
- Issue #548: DS: Deterministic UUIDs + authwebhook startup reconciliation for PVC-wipe resilience (consolidated from #548 + #536)
- DD-WORKFLOW-002 v3.0: Workflow identity model (workflow_id as UUID PK)
- BR-WORKFLOW-006: Kubernetes-native workflow registration via CRD + AW bridge
- ADR-058: Inline workflow schema registration (CRD-based)

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [Test Plan #546](../546/TEST_PLAN.md) — prior issue in v1.2

---

## 1. Scope

### In Scope

- **`DeterministicUUID` utility**: New pure function that converts a SHA-256 content hash into a standards-compliant UUIDv5 using a fixed kubernaut namespace UUID. Lives on the DS side.
- **`Repository.Create` deterministic ID**: Modified to supply the pre-computed `workflow_id` in the INSERT instead of relying on `DEFAULT uuid_generate_v4()`.
- **`handleDuplicateWorkflow` integration**: Computes `DeterministicUUID(contentHash)` and sets `workflow.WorkflowID` before calling `Create`.
- **Migration schema cleanup**: Remove `DEFAULT uuid_generate_v4()` from `workflow_id` column in `migrations/001_v1_schema.sql` (explicit-only UUID).
- **Startup reconciler `Runnable`**: New `pkg/authwebhook/startup_reconciler.go` implementing `manager.Runnable` that lists ActionType and RemediationWorkflow CRDs on startup, registers them with DS, and updates CRD `.status` fields.
- **Fail-closed startup**: Authwebhook blocks readiness until the startup reconciler completes (DS available and catalog populated).
- **`cmd/authwebhook/main.go` wiring**: Adds the startup reconciler as a `manager.Runnable`.

### Out of Scope

- **ActionType deterministic IDs**: ActionType already uses a deterministic text PK (`action_type TEXT PRIMARY KEY`). No UUID changes needed.
- **Backward compatibility / migration of existing random UUIDs**: Per user directive — clean-slate approach, no migration strategy.
- **`resource_action_traces` / `audit_events` historical data cleanup**: Existing denormalized references in historical tables are left as-is (read-only, never joined back).
- **E2E tests**: Startup reconciler E2E requires a fresh Kind cluster with PVC wipe simulation, deferred to release pipeline.
- **DS health check endpoint changes**: DS already exposes health endpoints; no modifications needed.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| UUIDv5 proper with kubernaut namespace UUID (not truncated SHA-256) | Standards-compliant (RFC 4122), reproducible, avoids collision risk from truncation. A fixed namespace UUID ensures determinism scoped to kubernaut. |
| UUID computed on DS side (in handler, from content_hash) | Centralizes UUID logic alongside existing idempotency (`handleDuplicateWorkflow`). Authwebhook doesn't need to know about UUID generation. |
| Fail-closed startup (block until DS is available) | Guarantees catalog is populated before serving webhook requests. Prevents scenarios where CRD CREATE events fire before the catalog is ready. |
| CRD `.status` updated after startup reconciliation | Keeps CRD status consistent with DS state. Operators see `catalogStatus: Active` and `workflowId` in `kubectl describe rw` even after PVC wipe. |
| ActionTypes reconciled before Workflows | Workflows have a FK to `action_type_taxonomy.action_type`. Registering ActionTypes first ensures the FK constraint is satisfied. |
| Exponential backoff with configurable timeout | Handles simultaneous startup of authwebhook and DS (common in fresh clusters). Prevents tight-loop retries from overwhelming DS during bootstrap. |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of new and modified unit-testable code. Targets:
  - `pkg/datastorage/uuid/deterministic.go`: `DeterministicUUID` function (100% of new code)
  - `pkg/datastorage/server/workflow_handlers.go`: UUID assignment in handler flow (~80%)
  - `pkg/authwebhook/startup_reconciler.go`: reconciler logic paths (>=80%)
- **Integration**: >=80% of new and modified integration-testable code. Targets:
  - `pkg/datastorage/repository/workflow/crud.go`: `Create` with pre-computed ID (>=80%)
  - `pkg/authwebhook/startup_reconciler.go`: full reconciliation cycle with K8s client (>=80%)
  - `cmd/authwebhook/main.go`: startup wiring (covered by reconciler integration test)

### 2-Tier Minimum

Every business requirement gap is covered by at least 2 tiers:
- **Unit tests**: Validate UUID generation correctness, reconciler decision logic, ordering constraints, error handling
- **Integration tests**: Validate end-to-end workflow creation with deterministic UUID, reconciler with K8s envtest, CRD status updates

### Tier Skip Rationale

- **E2E**: Deferred. PVC-wipe simulation requires wiping the PostgreSQL PV in a running Kind cluster, re-deploying DS, and verifying the authwebhook startup reconciler re-populates the catalog. This is best covered in the release pipeline.

### Business Outcome Quality Bar

Every test validates an observable business outcome:
- "After PVC wipe and re-registration, does the same CRD produce the same workflow_id?"
- "Does the operator see `catalogStatus: Active` on CRDs after startup reconciliation?"
- "Does the authwebhook block readiness until the DS catalog is populated?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/uuid/deterministic.go` (new) | `DeterministicUUID` | ~20 |
| `pkg/datastorage/server/workflow_handlers.go` | `computeContentHash` (existing, unchanged), UUID assignment in `handleDuplicateWorkflow` | ~10 (modified) |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/repository/workflow/crud.go` | `Create` (modified INSERT with explicit workflow_id) | ~15 (modified) |
| `pkg/authwebhook/startup_reconciler.go` (new) | `Start`, `reconcileActionTypes`, `reconcileWorkflows`, `syncWorkflowCRD`, `syncActionTypeCRD` | ~150 |
| `cmd/authwebhook/main.go` | Startup reconciler wiring (`mgr.Add`) | ~10 (modified) |

---

## 4. BR Coverage Matrix

| BR/Issue | Description | Priority | Tier | Test ID | Status |
|----------|-------------|----------|------|---------|--------|
| #548-UUID | DeterministicUUID produces valid UUIDv5 from content hash | P0 | Unit | UT-DS-548-001 | Pending |
| #548-UUID | Same content hash always yields same UUID (idempotent) | P0 | Unit | UT-DS-548-002 | Pending |
| #548-UUID | Different content hashes yield different UUIDs | P0 | Unit | UT-DS-548-003 | Pending |
| #548-UUID | UUID conforms to RFC 4122 v5 format (version=5, variant=10xx) | P1 | Unit | UT-DS-548-004 | Pending |
| #548-UUID | Empty content hash still produces a valid UUID | P2 | Unit | UT-DS-548-005 | Pending |
| #548-HANDLER | Handler assigns deterministic UUID to workflow before Create | P0 | Unit | UT-DS-548-006 | Pending |
| #548-HANDLER | Idempotent re-apply returns workflow with deterministic UUID | P0 | Unit | UT-DS-548-007 | Pending |
| #548-HANDLER | Supersede produces new deterministic UUID from new content | P0 | Unit | UT-DS-548-008 | Pending |
| #548-STARTUP | Startup reconciler registers ActionType CRDs with DS | P0 | Unit | UT-AW-548-001 | Pending |
| #548-STARTUP | Startup reconciler registers RemediationWorkflow CRDs with DS | P0 | Unit | UT-AW-548-002 | Pending |
| #548-STARTUP | ActionTypes registered before Workflows (ordering) | P0 | Unit | UT-AW-548-003 | Pending |
| #548-STATUS | Startup reconciler updates CRD status after registration | P0 | Unit | UT-AW-548-004 | Pending |
| #548-FAILCLOSED | Startup reconciler retries with backoff when DS unavailable | P0 | Unit | UT-AW-548-005 | Pending |
| #548-FAILCLOSED | Startup reconciler returns error (blocking readiness) if DS never responds | P0 | Unit | UT-AW-548-006 | Pending |
| #548-STARTUP | Startup reconciler handles empty CRD lists gracefully | P1 | Unit | UT-AW-548-007 | Pending |
| #548-STARTUP | Startup reconciler is idempotent (re-registration of already-synced CRDs) | P1 | Unit | UT-AW-548-008 | Pending |
| #548-REPO | Repository.Create uses pre-computed workflow_id | P0 | Integration | IT-DS-548-001 | Pending |
| #548-REPO | Re-registration after DB wipe produces same workflow_id | P0 | Integration | IT-DS-548-002 | Pending |
| #548-STARTUP | Full startup reconciliation cycle with K8s envtest | P0 | Integration | IT-AW-548-001 | Pending |
| #548-STATUS | CRD status updated with workflowId and catalogStatus after startup sync | P0 | Integration | IT-AW-548-002 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-548-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `DS` (DataStorage), `AW` (AuthWebhook)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests

**Testable code scope**:
- `pkg/datastorage/uuid/deterministic.go` (`DeterministicUUID`): 100%
- `pkg/datastorage/server/workflow_handlers.go` (UUID assignment in handler): >=80%
- `pkg/authwebhook/startup_reconciler.go` (reconciler logic): >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-548-001` | System produces a valid UUIDv5 from a SHA-256 content hash, ensuring workflow identity is deterministic across PVC wipes | Pending |
| `UT-DS-548-002` | Same CRD content always maps to the same workflow_id, so re-applying a CRD after PVC wipe recovers the original identity | Pending |
| `UT-DS-548-003` | Different CRD content produces different workflow_ids, preventing identity collision between distinct workflows | Pending |
| `UT-DS-548-004` | Generated UUID has version nibble=5 and variant bits=10xx, conforming to RFC 4122 for interoperability | Pending |
| `UT-DS-548-005` | Edge case: empty content hash does not panic and produces a valid UUID | Pending |
| `UT-DS-548-006` | When a new workflow is created via handler, the workflow_id in the DB row matches DeterministicUUID(contentHash) | Pending |
| `UT-DS-548-007` | Idempotent re-apply (same content) returns the existing workflow with the same deterministic UUID (no new row created) | Pending |
| `UT-DS-548-008` | Supersede (different content) produces a new workflow with a new deterministic UUID derived from the new content_hash | Pending |
| `UT-AW-548-001` | Startup reconciler discovers all ActionType CRDs and registers each with DS via CreateActionType | Pending |
| `UT-AW-548-002` | Startup reconciler discovers all RemediationWorkflow CRDs and registers each with DS via CreateWorkflowInline | Pending |
| `UT-AW-548-003` | All ActionTypes are registered before any Workflows, satisfying the FK constraint in DS | Pending |
| `UT-AW-548-004` | After successful DS registration, the CRD `.status.catalogStatus` is set to Active and `.status.workflowId` is populated | Pending |
| `UT-AW-548-005` | When DS is initially unavailable, the reconciler retries with exponential backoff until DS responds | Pending |
| `UT-AW-548-006` | When DS remains unavailable beyond the configured timeout, the reconciler returns an error that blocks authwebhook readiness | Pending |
| `UT-AW-548-007` | When no CRDs exist in the cluster, the reconciler completes successfully without errors | Pending |
| `UT-AW-548-008` | When CRDs are already registered in DS (idempotent re-apply returns 200), the reconciler still updates CRD status and completes | Pending |

### Tier 2: Integration Tests

**Testable code scope**:
- `pkg/datastorage/repository/workflow/crud.go` (`Create` with explicit workflow_id): >=80%
- `pkg/authwebhook/startup_reconciler.go` (full cycle with envtest): >=80%
- CRD status update path: >=80%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-548-001` | Workflow inserted into PostgreSQL has the deterministic UUID as its primary key (not a random UUID) | Pending |
| `IT-DS-548-002` | After wiping the DB and re-inserting the same workflow content, the workflow_id is identical to the original | Pending |
| `IT-AW-548-001` | Startup reconciler lists CRDs from K8s envtest, calls DS, and all CRDs are registered in correct order | Pending |
| `IT-AW-548-002` | After startup reconciliation, `kubectl get rw` shows `catalogStatus: Active` and `workflowId` populated on all CRDs | Pending |

### Tier Skip Rationale

- **E2E**: Deferred. Full PVC-wipe resilience E2E requires Kind cluster with persistent volumes, DS PostgreSQL wipe, authwebhook restart, and verification that the catalog is re-populated with matching UUIDs. This is a release-pipeline scenario.

---

## 6. Test Cases (Detail)

### UT-DS-548-001: DeterministicUUID produces valid UUIDv5

**BR**: #548-UUID
**Type**: Unit
**File**: `test/unit/datastorage/deterministic_uuid_test.go`

**Given**: A known SHA-256 content hash string (e.g., `"a1b2c3d4e5f6..."`)
**When**: `DeterministicUUID(contentHash)` is called
**Then**: The result is a valid UUID string matching `^[0-9a-f]{8}-[0-9a-f]{4}-5[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`

**Acceptance Criteria**:
- Return value parses as a valid UUID (no error from `uuid.Parse`)
- UUID version is 5 (byte 6 high nibble == 0x50)
- UUID variant is RFC 4122 (byte 8 high bits == 10)

---

### UT-DS-548-002: Same content hash yields same UUID (idempotent)

**BR**: #548-UUID
**Type**: Unit
**File**: `test/unit/datastorage/deterministic_uuid_test.go`

**Given**: A content hash `h`
**When**: `DeterministicUUID(h)` is called twice
**Then**: Both calls return the identical UUID string

**Acceptance Criteria**:
- `DeterministicUUID(h) == DeterministicUUID(h)` is true
- Tested across 10 different content hashes in a table-driven test

---

### UT-DS-548-003: Different content hashes yield different UUIDs

**BR**: #548-UUID
**Type**: Unit
**File**: `test/unit/datastorage/deterministic_uuid_test.go`

**Given**: Two distinct content hashes `h1` and `h2`
**When**: `DeterministicUUID(h1)` and `DeterministicUUID(h2)` are called
**Then**: The results are different

**Acceptance Criteria**:
- `DeterministicUUID(h1) != DeterministicUUID(h2)` for all pairs in a table-driven test
- Tested with hashes differing by a single character

---

### UT-DS-548-004: UUID conforms to RFC 4122 v5 format

**BR**: #548-UUID
**Type**: Unit
**File**: `test/unit/datastorage/deterministic_uuid_test.go`

**Given**: Any content hash
**When**: `DeterministicUUID(contentHash)` is called and the result is parsed as UUID bytes
**Then**: Version nibble (byte 6, bits 4-7) == 0x5, variant bits (byte 8, bits 6-7) == 0b10

**Acceptance Criteria**:
- `parsedUUID.Version() == 5`
- `parsedUUID.Variant() == uuid.RFC4122`

---

### UT-DS-548-005: Empty content hash produces valid UUID

**BR**: #548-UUID
**Type**: Unit
**File**: `test/unit/datastorage/deterministic_uuid_test.go`

**Given**: An empty string `""`
**When**: `DeterministicUUID("")` is called
**Then**: The result is a valid UUIDv5 (no panic, no error)

**Acceptance Criteria**:
- No panic occurs
- Return value parses as a valid UUID

---

### UT-DS-548-006: Handler assigns deterministic UUID before Create

**BR**: #548-HANDLER
**Type**: Unit
**File**: `test/unit/datastorage/workflow_deterministic_uuid_test.go`

**Given**: A workflow creation request with content `C`, where `computeContentHash(C)` = `H`
**When**: `handleDuplicateWorkflow` processes the request (no existing workflow in DS)
**Then**: The created workflow's `WorkflowID` equals `DeterministicUUID(H)`

**Acceptance Criteria**:
- `result.workflow.WorkflowID == DeterministicUUID(computeContentHash(C))`
- The workflow is stored with this ID (not a random UUID)

---

### UT-DS-548-007: Idempotent re-apply returns same deterministic UUID

**BR**: #548-HANDLER
**Type**: Unit
**File**: `test/unit/datastorage/workflow_deterministic_uuid_test.go`

**Given**: An active workflow with content hash `H` and `WorkflowID == DeterministicUUID(H)`
**When**: Same content is submitted again (idempotent re-apply)
**Then**: Response status is 200, returned `WorkflowID` matches the original

**Acceptance Criteria**:
- HTTP status is 200 (not 201)
- `result.workflow.WorkflowID == DeterministicUUID(H)`
- No new row created in DB

---

### UT-DS-548-008: Supersede produces new deterministic UUID

**BR**: #548-HANDLER
**Type**: Unit
**File**: `test/unit/datastorage/workflow_deterministic_uuid_test.go`

**Given**: An active workflow with content hash `H1` and ID `DeterministicUUID(H1)`
**When**: New content with hash `H2 != H1` is submitted for the same workflow_name+version
**Then**: Old workflow is superseded, new workflow has `WorkflowID == DeterministicUUID(H2)`

**Acceptance Criteria**:
- HTTP status is 201
- New `WorkflowID == DeterministicUUID(H2)`
- Old workflow status is "Superseded"
- `DeterministicUUID(H1) != DeterministicUUID(H2)`

---

### UT-AW-548-001: Startup reconciler registers ActionType CRDs

**BR**: #548-STARTUP
**Type**: Unit
**File**: `test/unit/authwebhook/startup_reconciler_test.go`

**Given**: 2 ActionType CRDs exist in K8s (fake client), DS mock accepts CreateActionType
**When**: The startup reconciler runs
**Then**: DS mock received exactly 2 CreateActionType calls, one for each CRD's `spec.name`

**Acceptance Criteria**:
- CreateActionType called once per ActionType CRD
- The `registeredBy` parameter is `"system:authwebhook-startup"`
- Both CRDs are processed (count matches)

---

### UT-AW-548-002: Startup reconciler registers RemediationWorkflow CRDs

**BR**: #548-STARTUP
**Type**: Unit
**File**: `test/unit/authwebhook/startup_reconciler_test.go`

**Given**: 3 RemediationWorkflow CRDs exist in K8s (fake client), DS mock accepts CreateWorkflowInline
**When**: The startup reconciler runs
**Then**: DS mock received exactly 3 CreateWorkflowInline calls with clean CRD content

**Acceptance Criteria**:
- CreateWorkflowInline called once per RW CRD
- Content passed is the `marshalCleanCRDContent` output (deterministic, no runtime metadata)
- The `registeredBy` parameter is `"system:authwebhook-startup"`

---

### UT-AW-548-003: ActionTypes registered before Workflows (ordering)

**BR**: #548-STARTUP
**Type**: Unit
**File**: `test/unit/authwebhook/startup_reconciler_test.go`

**Given**: 1 ActionType CRD and 2 RemediationWorkflow CRDs in K8s, DS mock records call order
**When**: The startup reconciler runs
**Then**: All CreateActionType calls precede all CreateWorkflowInline calls in the recorded order

**Acceptance Criteria**:
- Call log shows: `[CreateActionType, CreateWorkflowInline, CreateWorkflowInline]`
- No CreateWorkflowInline call appears before the last CreateActionType call

---

### UT-AW-548-004: CRD status updated after successful registration

**BR**: #548-STATUS
**Type**: Unit
**File**: `test/unit/authwebhook/startup_reconciler_test.go`

**Given**: 1 RemediationWorkflow CRD, DS returns workflowId="abc-123" on CreateWorkflowInline
**When**: The startup reconciler runs
**Then**: The CRD's `.status.workflowId` is "abc-123", `.status.catalogStatus` is "Active", `.status.registeredBy` is "system:authwebhook-startup"

**Acceptance Criteria**:
- `rw.Status.WorkflowID == "abc-123"`
- `rw.Status.CatalogStatus == "Active"`
- `rw.Status.RegisteredBy == "system:authwebhook-startup"`
- `rw.Status.RegisteredAt` is set (non-nil, recent timestamp)

---

### UT-AW-548-005: Retries with backoff when DS unavailable

**BR**: #548-FAILCLOSED
**Type**: Unit
**File**: `test/unit/authwebhook/startup_reconciler_test.go`

**Given**: DS mock returns connection errors for the first 3 attempts, then succeeds on the 4th, 1 ActionType CRD exists
**When**: The startup reconciler runs with a sufficient timeout
**Then**: The reconciler retries until DS becomes available, then completes successfully

**Acceptance Criteria**:
- DS mock received at least 4 connection attempts
- The reconciler did not return an error
- The ActionType CRD was successfully registered
- Time between retries increases (exponential backoff)

---

### UT-AW-548-006: Returns error when DS never responds (blocks readiness)

**BR**: #548-FAILCLOSED
**Type**: Unit
**File**: `test/unit/authwebhook/startup_reconciler_test.go`

**Given**: DS mock always returns connection errors, configured timeout is 5 seconds
**When**: The startup reconciler runs
**Then**: The reconciler returns a non-nil error after the timeout expires

**Acceptance Criteria**:
- `err != nil` (reconciler returned an error)
- Error message indicates DS unavailability
- Reconciler ran for approximately the configured timeout duration (within 1s tolerance)
- This error propagates to the manager, preventing readiness

---

### UT-AW-548-007: Empty CRD lists handled gracefully

**BR**: #548-STARTUP
**Type**: Unit
**File**: `test/unit/authwebhook/startup_reconciler_test.go`

**Given**: No ActionType or RemediationWorkflow CRDs exist in K8s
**When**: The startup reconciler runs
**Then**: The reconciler completes successfully with no DS calls

**Acceptance Criteria**:
- No error returned
- DS mock received zero calls
- Reconciler logged "no CRDs found" (or equivalent)

---

### UT-AW-548-008: Idempotent re-registration of already-synced CRDs

**BR**: #548-STARTUP
**Type**: Unit
**File**: `test/unit/authwebhook/startup_reconciler_test.go`

**Given**: 1 RW CRD already registered in DS (CreateWorkflowInline returns 200 with existing workflowId)
**When**: The startup reconciler runs
**Then**: The reconciler updates the CRD status and completes without error

**Acceptance Criteria**:
- No error returned
- CRD status is still updated (consistent with DS state)
- The 200 (idempotent) response is handled without treating it as an error

---

### IT-DS-548-001: Repository.Create uses pre-computed workflow_id

**BR**: #548-REPO
**Type**: Integration
**File**: `test/integration/datastorage/deterministic_uuid_test.go`

**Given**: A PostgreSQL database with the v1 schema, a workflow model with `WorkflowID` pre-set to `DeterministicUUID(contentHash)`
**When**: `Repository.Create(ctx, workflow)` is called
**Then**: The row in `remediation_workflow_catalog` has `workflow_id` matching the pre-set value

**Acceptance Criteria**:
- `SELECT workflow_id FROM remediation_workflow_catalog WHERE workflow_name = $1` returns the pre-set deterministic UUID
- No fallback to `uuid_generate_v4()` occurred
- `workflow.WorkflowID` after Create matches the pre-set value

---

### IT-DS-548-002: Re-registration after DB wipe produces same workflow_id

**BR**: #548-REPO
**Type**: Integration
**File**: `test/integration/datastorage/deterministic_uuid_test.go`

**Given**: A workflow created with content `C`, producing `workflow_id = DeterministicUUID(computeContentHash(C))`
**When**: The DB is truncated (`DELETE FROM remediation_workflow_catalog`) and the same content `C` is re-registered
**Then**: The new `workflow_id` is identical to the original

**Acceptance Criteria**:
- `id_before_wipe == id_after_wipe`
- Content hash is identical
- This simulates PVC-wipe resilience

---

### IT-AW-548-001: Full startup reconciliation with K8s envtest

**BR**: #548-STARTUP
**Type**: Integration
**File**: `test/integration/authwebhook/startup_reconciler_test.go`

**Given**: K8s envtest cluster with 1 ActionType CRD and 2 RemediationWorkflow CRDs, a real DS test HTTP server
**When**: The startup reconciler `Start(ctx)` is called
**Then**: All 3 CRDs are registered in DS (ActionType first, then Workflows), and CRD statuses are updated

**Acceptance Criteria**:
- DS server received 1 ActionType registration and 2 Workflow registrations
- ActionType registration completed before Workflow registrations
- All 3 CRDs have `.status.catalogStatus == "Active"` in K8s

---

### IT-AW-548-002: CRD status shows workflowId and catalogStatus after startup

**BR**: #548-STATUS
**Type**: Integration
**File**: `test/integration/authwebhook/startup_reconciler_test.go`

**Given**: K8s envtest with 1 RemediationWorkflow CRD, DS test server returns workflowId
**When**: Startup reconciler completes
**Then**: `kubectl get rw` equivalent shows `.status.workflowId` populated and `.status.catalogStatus` is "Active"

**Acceptance Criteria**:
- `rw.Status.WorkflowID` matches the deterministic UUID returned by DS
- `rw.Status.CatalogStatus` is `"Active"`
- `rw.Status.RegisteredBy` is `"system:authwebhook-startup"`
- `rw.Status.RegisteredAt` is set

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**:
  - K8s fake client (`sigs.k8s.io/controller-runtime/pkg/client/fake`) for startup reconciler
  - Mock DS client (interface-based mock implementing `WorkflowCatalogClient` and `ActionTypeCatalogClient`)
  - Mock HTTP server (`net/http/httptest`) for DS handler tests
  - Mock workflow repository (interface-based) for handler unit tests
- **Location**:
  - `test/unit/datastorage/deterministic_uuid_test.go`
  - `test/unit/datastorage/workflow_deterministic_uuid_test.go`
  - `test/unit/authwebhook/startup_reconciler_test.go`
- **Anti-patterns avoided**:
  - No `time.Sleep()` (use `Eventually()` for async, or controlled time in backoff tests)
  - No `Skip()` (all tests must run or not exist)
  - No `Expect(x).ToNot(BeNil())` without follow-up business assertion
  - All assertions validate business outcomes

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](../../development/business-requirements/INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**:
  - PostgreSQL (testcontainers or dedicated test DB) for DS repository tests
  - K8s envtest (controller-runtime test environment) for authwebhook startup reconciler
  - Real DS test HTTP server (or lightweight in-process server)
- **Location**:
  - `test/integration/datastorage/deterministic_uuid_test.go`
  - `test/integration/authwebhook/startup_reconciler_test.go`

---

## 8. Execution

```bash
# All unit tests
make test

# DS deterministic UUID unit tests
go test ./test/unit/datastorage/... -ginkgo.focus="548"

# AW startup reconciler unit tests
go test ./test/unit/authwebhook/... -ginkgo.focus="548"

# DS integration tests (requires PostgreSQL)
make test-integration-datastorage

# AW integration tests (requires envtest)
go test ./test/integration/authwebhook/... -ginkgo.focus="548"

# Specific test by ID
go test ./test/unit/datastorage/... -ginkgo.focus="UT-DS-548-001"
go test ./test/unit/authwebhook/... -ginkgo.focus="UT-AW-548-001"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan: 16 unit tests + 4 integration tests covering DeterministicUUID utility, handler integration, Repository.Create modification, startup reconciler logic, fail-closed behavior, CRD status updates, and ordering constraints |
