# Test Plan: ActionType CRD Migration (#300)

**Feature**: Migrate ActionType provisioning from SQL seeds to Kubernetes CRD with Admission Webhook lifecycle management
**Version**: 1.0
**Created**: 2026-03-09
**Author**: AI Assistant
**Status**: Active
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- BR-WORKFLOW-007: ActionType CRD Lifecycle Management
- ADR-059: ActionType CRD Lifecycle via Admission Webhook
- DD-ACTIONTYPE-001: ActionType CRD Lifecycle Design

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Phase 2 Test Plan (#299)](../299/TEST_PLAN.md) -- RemediationWorkflow CRD pattern (blueprint)
- [GitHub Issue #300](https://github.com/jordigilh/kubernaut/issues/300)

---

## 1. Scope

### In Scope

- **CRD Types**: `ActionType`, `ActionTypeList`, spec/status fields, DeepCopy, scheme registration
- **DS Repository**: CRUD operations on `action_type_taxonomy` with lifecycle columns
- **DS HTTP Handlers**: POST (create/re-enable), PATCH (update description), PATCH (disable with dependency guard)
- **DS Client Adapter**: `ActionTypeCatalogClient` interface for AW to call DS
- **AW Webhook Handler**: CREATE/UPDATE/DELETE admission for `actiontypes.kubernaut.ai`
- **RW Cross-Update**: Async `activeWorkflowCount` refresh on RW CREATE/DELETE
- **Audit Events**: 5 DS events + 4 AW events with typed payloads
- **Helm Charts**: Webhook config, RBAC, CRD manifest
- **Seed Data Migration**: 24 ActionType CRD YAMLs, demo scripts, test fixture migration

### Out of Scope

- RemediationWorkflow CRD lifecycle (covered by [#299 Test Plan](../299/TEST_PLAN.md))
- DS repository-level unit tests that test raw SQL (keep existing SQL approach)
- ActionType versioning (action types are unversioned taxonomy entries)
- Hard-delete of action types

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| ValidatingWebhookConfiguration (not Mutating) | Consistent with RW pattern; status subresource prevents mutating webhook from patching `.status` |
| Soft-delete only | Preserves referential integrity, audit continuity, enables re-enablement |
| 409 Conflict for dependent deletion | Operators need explicit feedback before disabling an in-use action type |
| Async `activeWorkflowCount` | Non-blocking; eventual consistency acceptable for display purposes |
| Description-only mutability | `spec.name` is the identity; changing it would break all workflow references |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (CRD types, validators, audit constructors, handler logic)
- **Integration**: >=80% of integration-testable code (DS CRUD against real PostgreSQL, handler HTTP tests)
- **E2E**: >=80% of full lifecycle code (Kind cluster, real webhook, real DS)

### 2-Tier Minimum

Every business requirement is covered by at least 2 test tiers:
- **Unit tests** validate logic, type safety, audit payload correctness
- **Integration/E2E tests** validate wiring, DB operations, webhook admission

### Business Outcome Quality Bar

Tests validate business outcomes: "Can operators manage action types via kubectl?" not "Is the handler function called?"

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `api/actiontype/v1alpha1/actiontype_types.go` | Types, DeepCopy, scheme registration | ~120 |
| `pkg/datastorage/audit/actiontype_events.go` | Audit constructors | ~150 |
| `pkg/authwebhook/actiontype_audit.go` | AW audit emission | ~80 |
| `pkg/authwebhook/actiontype_handler.go` | Handler logic (with mocked DS client) | ~200 |
| `pkg/authwebhook/ds_client.go` | Client adapter methods | ~100 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|----------------|
| `pkg/datastorage/repository/actiontype/crud.go` | DB CRUD operations | ~200 |
| `pkg/datastorage/server/actiontype_handlers.go` | HTTP handlers | ~300 |
| `pkg/datastorage/repository/workflow/discovery.go` | Discovery filtering by active status | ~50 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-007.1 | CREATE: new action type registered | P0 | Unit | UT-AT-300-001 | Pending |
| BR-WORKFLOW-007.1 | CREATE: idempotent (already active) | P0 | Unit | UT-AT-300-002 | Pending |
| BR-WORKFLOW-007.1 | CREATE: re-enable disabled | P0 | Unit | UT-AT-300-003 | Pending |
| BR-WORKFLOW-007.2 | UPDATE: description change with audit | P0 | Unit | UT-AT-300-004 | Pending |
| BR-WORKFLOW-007.2 | UPDATE: spec.name change denied | P0 | Unit | UT-AT-300-005 | Pending |
| BR-WORKFLOW-007.3 | DELETE: soft-disable (no deps) | P0 | Unit | UT-AT-300-006 | Pending |
| BR-WORKFLOW-007.3 | DELETE: denied with dependency count | P0 | Unit | UT-AT-300-007 | Pending |
| BR-WORKFLOW-007.4 | Audit: CREATE event payload | P1 | Unit | UT-AT-300-008 | Pending |
| BR-WORKFLOW-007.4 | Audit: UPDATE event with old+new | P1 | Unit | UT-AT-300-009 | Pending |
| BR-WORKFLOW-007.4 | Audit: disable_denied event | P1 | Unit | UT-AT-300-010 | Pending |
| BR-WORKFLOW-007.5 | Cross-update: activeWorkflowCount | P1 | Unit | UT-AT-300-011 | Pending |
| BR-WORKFLOW-007.1 | CREATE: DS CRUD against PostgreSQL | P0 | Integration | IT-AT-300-001 | Pending |
| BR-WORKFLOW-007.1 | CREATE: idempotency matrix | P0 | Integration | IT-AT-300-002 | Pending |
| BR-WORKFLOW-007.3 | DELETE: dependency guard in DB | P0 | Integration | IT-AT-300-003 | Pending |
| BR-WORKFLOW-007.1 | CREATE: full kubectl lifecycle | P0 | E2E | E2E-AT-300-001 | Pending |
| BR-WORKFLOW-007.2 | UPDATE: description via kubectl edit | P0 | E2E | E2E-AT-300-002 | Pending |
| BR-WORKFLOW-007.3 | DELETE: denied then allowed | P0 | E2E | E2E-AT-300-003 | Pending |
| BR-WORKFLOW-007.5 | Cross-update: printer columns | P1 | E2E | E2E-AT-300-004 | Pending |

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-AT-300-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **AT**: ActionType service abbreviation
- **300**: Issue number
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: CRD types, AW handler logic, DS client adapter, audit constructors

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AT-300-001` | New ActionType CRD CREATE registers in DS and populates status | Pending |
| `UT-AT-300-002` | Idempotent CREATE for already-active action type returns NOOP | Pending |
| `UT-AT-300-003` | CREATE for disabled action type re-enables and sets previouslyExisted | Pending |
| `UT-AT-300-004` | Description UPDATE generates audit with old+new values | Pending |
| `UT-AT-300-005` | Spec.name change in UPDATE is denied by webhook | Pending |
| `UT-AT-300-006` | DELETE with no dependent workflows soft-disables successfully | Pending |
| `UT-AT-300-007` | DELETE with N dependent workflows returns denial with count+names | Pending |
| `UT-AT-300-008` | CREATE audit event payload contains all required fields | Pending |
| `UT-AT-300-009` | UPDATE audit event contains oldDescription and newDescription structs | Pending |
| `UT-AT-300-010` | Disable denied audit contains dependentWorkflows as []string | Pending |
| `UT-AT-300-011` | RW CREATE/DELETE triggers async activeWorkflowCount update | Pending |
| `UT-AT-300-012` | DS client adapter Create/Update/Disable map correctly to DS API | Pending |
| `UT-AT-300-013` | CRD types: YAML unmarshal, DeepCopy, scheme registration | Complete |

### Tier 2: Integration Tests

**Testable code scope**: DS repository CRUD, HTTP handlers, discovery filtering

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-AT-300-001` | Create action type in real PostgreSQL, verify row exists | Pending |
| `IT-AT-300-002` | Idempotency matrix: create/re-enable/NOOP against real DB | Pending |
| `IT-AT-300-003` | Disable with dependency guard: count active workflows in DB | Pending |
| `IT-AT-300-004` | Update description: verify old+new values captured | Pending |
| `IT-AT-300-005` | Discovery filtering: disabled action types excluded from ListActions | Pending |
| `IT-AT-300-006` | Audit events written to audit table with correct payloads | Pending |

### Tier 3: E2E Tests

**Testable code scope**: Full kubectl lifecycle in Kind cluster

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-AT-300-001` | kubectl apply creates ActionType, status populated | Pending |
| `E2E-AT-300-002` | kubectl edit updates description, audit trail generated | Pending |
| `E2E-AT-300-003` | kubectl delete denied with dependent workflows, allowed after removal | Pending |
| `E2E-AT-300-004` | Printer columns show correct values (ACTION TYPE, WORKFLOWS, REGISTERED, AGE) | Pending |
| `E2E-AT-300-005` | Wide output shows DESCRIPTION column | Pending |
| `E2E-AT-300-006` | RW CREATE/DELETE updates ActionType activeWorkflowCount | Pending |
| `E2E-AT-300-007` | Re-applying deleted ActionType re-enables with previouslyExisted=true | Pending |

---

## 6. Test Cases (Detail)

### UT-AT-300-001: CREATE registers new ActionType

**BR**: BR-WORKFLOW-007.1
**Type**: Unit
**File**: `test/unit/authwebhook/actiontype_handler_test.go`

**Given**: No action type named "RestartPod" exists in DS
**When**: ActionType CRD CREATE admission request is received
**Then**: DS CreateActionType is called, CRD status is patched with registered=true

### UT-AT-300-007: DELETE denied with dependent workflows

**BR**: BR-WORKFLOW-007.3
**Type**: Unit
**File**: `test/unit/authwebhook/actiontype_handler_test.go`

**Given**: ActionType "RestartPod" has 3 active RemediationWorkflows
**When**: ActionType CRD DELETE admission request is received
**Then**: Admission is denied with message containing count (3) and workflow names

### IT-AT-300-002: Idempotency matrix

**BR**: BR-WORKFLOW-007.1
**Type**: Integration
**File**: `test/integration/datastorage/actiontype_lifecycle_test.go`

**Given**: Real PostgreSQL with action_type_taxonomy table
**When**: CREATE same action type 3 times (new, active, disabled states)
**Then**: First creates, second NOOPs, third re-enables (after manual disable)

### E2E-AT-300-003: DELETE denied then allowed

**BR**: BR-WORKFLOW-007.3
**Type**: E2E
**File**: `test/e2e/authwebhook/actiontype_lifecycle_test.go`

**Given**: ActionType CRD applied, RemediationWorkflow CRD referencing it applied
**When**: kubectl delete actiontype, then delete the workflow, then delete actiontype again
**Then**: First delete denied (409), second delete allowed after workflow removal

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: DS client (mock HTTP), K8s client (fake.NewClientBuilder)
- **Location**: `test/unit/actiontype/`, `test/unit/authwebhook/`, `test/unit/datastorage/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**: Real PostgreSQL (testcontainers or CI database)
- **Location**: `test/integration/datastorage/`

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**: Kind cluster with CRDs, webhooks, DS, PostgreSQL
- **Location**: `test/e2e/authwebhook/`

---

## 8. Execution

```bash
# Unit tests (all ActionType)
go test ./test/unit/actiontype/... -v
go test ./test/unit/authwebhook/... -ginkgo.focus="ActionType" -v
go test ./test/unit/datastorage/... -ginkgo.focus="ActionType" -v

# Integration tests
make test-integration-datastorage GINKGO_LABEL="actiontype"

# E2E tests
make test-e2e-authwebhook GINKGO_LABEL="actiontype"

# Specific test by ID
go test ./test/unit/authwebhook/... -ginkgo.focus="UT-AT-300-007"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-09 | Initial test plan for ActionType CRD migration (#300) |
