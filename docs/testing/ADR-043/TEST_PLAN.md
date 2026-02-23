# Test Plan: detectedLabels Workflow Schema Field

**Feature**: Add detectedLabels as optional top-level field in workflow-schema.yaml (ADR-043 v1.3)
**Version**: 1.3
**Created**: 2026-02-20
**Author**: AI Assistant + Jordi Gil
**Status**: Ready for Execution
**Branch**: `feat/group-a-schema-credentials`
**Issue**: [#131](https://github.com/jordigilh/kubernaut/issues/131)

**Authority**:
- ADR-043: Workflow Schema Definition Standard
- BR-WORKFLOW-004: Workflow Schema Format Specification
- DD-WORKFLOW-001 v2.0+: DetectedLabels End-to-End Architecture (authoritative field list)

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Plan Template](../TEST_PLAN_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **WorkflowSchema struct** (`pkg/datastorage/models/workflow_schema.go`): Add `DetectedLabels` field + `ValidateDetectedLabels()` method
- **Schema parser** (`pkg/datastorage/schema/parser.go`): Add `ExtractDetectedLabels()` + call validation in `Validate()`
- **OCI workflow handler** (`pkg/datastorage/server/workflow_handlers.go`): Wire `DetectedLabels` into `buildWorkflowFromSchema`
- **Authoritative docs**: ADR-043, BR-WORKFLOW-004

### Out of Scope

- DetectedLabels runtime detection by SignalProcessing (already implemented)
- HAPI workflow discovery matching logic (uses existing `DetectedLabels` in catalog)
- Database schema changes (the `detected_labels` JSONB column already exists in `remediation_workflow_catalog`)
- Demo scenario workflow-schema.yaml files (8 scenarios already declare `detectedLabels`; they need no changes)

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| New `WorkflowSchemaDetectedLabels` struct for YAML parsing | YAML booleans arrive as strings (`"true"`); need explicit type distinct from `models.DetectedLabels` for safe conversion |
| Boolean fields accept only `"true"` | DD-WORKFLOW-001 v1.6: absence means "no requirement"; `"false"` is ambiguous and rejected |
| Unknown fields rejected | Prevents typos from being silently ignored (e.g., `hpaenabled` instead of `hpaEnabled`) |
| `detectedLabels` is optional | Most workflows don't need infrastructure constraints; only specialized workflows declare them |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

- **Unit**: >=80% of unit-testable code in `workflow_schema.go` (new type + validation) and `parser.go` (extraction)
- **Integration**: >=80% of integration-testable code in `workflow_handlers.go` (buildWorkflowFromSchema) and DB round-trip

### 2-Tier Minimum

Both UT and IT tiers are required:
- **Unit**: Catches validation logic errors, type conversion bugs, parsing edge cases
- **Integration**: Catches wiring errors in the HTTP handler, DB JSONB fidelity, search/discovery behavior

### 3-Tier Coverage

All three tiers are exercised:
- **Unit**: Catches validation logic errors, type conversion bugs, parsing edge cases
- **Integration**: Catches wiring errors in the HTTP handler, DB JSONB fidelity, search/discovery behavior
- **E2E**: Validates the full OCI image registration -> DB persistence -> HTTP retrieval chain through the live DataStorage service

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods (new) | Lines (approx) |
|------|------------------------|-----------------|
| `pkg/datastorage/models/workflow_schema.go` | `WorkflowSchemaDetectedLabels` struct, `ValidateDetectedLabels()` | ~60 new |
| `pkg/datastorage/schema/parser.go` | `ExtractDetectedLabels()`, `Validate()` extension | ~40 new |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods (modified) | Lines (approx) |
|------|------------------------------|-----------------|
| `pkg/datastorage/server/workflow_handlers.go` | `buildWorkflowFromSchema` (add DetectedLabels wiring) | ~10 modified |
| DB layer | JSONB write/read of `detected_labels` column | existing |

---

## 4. BR Coverage Matrix

| BR/ADR ID | Description | Priority | Tier | Test ID | Status |
|-----------|-------------|----------|------|---------|--------|
| ADR-043 | Parse boolean detectedLabels from workflow-schema.yaml | P0 | Unit | UT-DS-043-001 | Pending |
| ADR-043 | Parse string wildcard detectedLabels (gitOpsTool: "*") | P0 | Unit | UT-DS-043-002 | Pending |
| ADR-043 | Schema without detectedLabels produces nil (optional) | P0 | Unit | UT-DS-043-003 | Pending |
| ADR-043 | Reject invalid boolean value with actionable error | P0 | Unit | UT-DS-043-004 | Pending |
| ADR-043 | Reject invalid gitOpsTool with actionable error | P0 | Unit | UT-DS-043-005 | Pending |
| ADR-043 | Reject invalid serviceMesh with actionable error | P0 | Unit | UT-DS-043-006 | Pending |
| ADR-043 | All 8 fields survive YAML-to-model conversion (data accuracy) | P0 | Unit | UT-DS-043-007 | Pending |
| ADR-043 | Multi-field combination mirrors real demo schemas | P0 | Unit | UT-DS-043-008 | Pending |
| ADR-043 | OCI extraction pipeline preserves detectedLabels end-to-end | P0 | Unit | UT-DS-043-009 | Pending |
| ADR-043 | Unknown field rejected with error naming the field | P1 | Unit | UT-DS-043-010 | Pending |
| ADR-043 | Empty detectedLabels section produces empty struct (not nil) | P1 | Unit | UT-DS-043-011 | Pending |
| ADR-043 | POST stores detectedLabels accurately in catalog (JSONB round-trip) | P0 | Integration | IT-DS-043-001 | Pending |
| ADR-043 | GET returns exact detectedLabels registered (no field loss) | P0 | Integration | IT-DS-043-002 | Pending |
| ADR-043 | POST without detectedLabels stores empty DetectedLabels | P0 | Integration | IT-DS-043-003 | Pending |
| ADR-043 | POST with invalid detectedLabels returns HTTP 400 with field error | P0 | Unit | UT-DS-043-004/005/006 | Covered (see note 1) |
| ADR-043 | Workflow search filters by detectedLabels (HAPI discovery) | P0 | Integration | IT-DS-043-005 | Pending |
| ADR-043 | Full schema round-trip (all fields) preserves detectedLabels alongside existing fields | P0 | Integration | IT-DS-043-006 | Pending |
| ADR-043 | Version update with changed detectedLabels stores new values | P1 | Integration | IT-DS-043-007 | Pending |
| ADR-043 | OCI registration response includes parsed detectedLabels | P0 | E2E | E2E-DS-043-001 | Pending |
| ADR-043 | GetWorkflowByID returns detectedLabels after DB round-trip | P0 | E2E | E2E-DS-043-002 | Pending |
| ADR-043 | Workflow with detectedLabels appears in discovery flow (Step 2 + Step 3) | P0 | E2E | E2E-DS-043-003 | Pending |
| ADR-043 | HTTP search with detected_labels query parameter returns filtered results | P0 | E2E | E2E-DS-043-004 | Pending |
| ADR-043 | All 8 detectedLabels fields survive full OCI -> DB -> HTTP round-trip | P0 | E2E | E2E-DS-043-005 | Pending |

---

## 5. Test Scenarios

### Tier 1: Unit Tests

**Testable code scope**: `models/workflow_schema.go` (new type + validation ~60 lines), `schema/parser.go` (extraction ~40 lines). Target: >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-043-001` | Workflow requiring HPA-enabled targets is correctly represented after parsing (boolean field accuracy) | RED |
| `UT-DS-043-002` | Workflow requiring "any GitOps tool" is correctly represented after parsing (wildcard string accuracy) | RED |
| `UT-DS-043-003` | Workflow with no infrastructure requirements has nil detectedLabels (absence = no constraint) | RED |
| `UT-DS-043-004` | Workflow author is told exactly which field has an invalid value and what values are accepted (actionable validation error for boolean) | RED |
| `UT-DS-043-005` | Workflow author is told exactly which gitOpsTool values are valid when they provide an unsupported tool | RED |
| `UT-DS-043-006` | Workflow author is told exactly which serviceMesh values are valid when they provide an unsupported mesh | RED |
| `UT-DS-043-007` | All 8 detectedLabels fields survive YAML-to-model conversion with exact values (no silent data loss across the type boundary) | RED |
| `UT-DS-043-008` | Workflow with multiple detectedLabels (e.g., pdbProtected + hpaEnabled + gitOpsTool) preserves all constraints -- mirrors real demo scenario schemas | RED |
| `UT-DS-043-009` | OCI extraction pipeline (mock image -> parse -> validate -> extract) produces accurate DetectedLabels end-to-end | RED |
| `UT-DS-043-010` | Unknown field in detectedLabels section (e.g., "customField: true") is rejected with error naming the invalid field (prevents silent ignore of typos) | RED |
| `UT-DS-043-011` | Empty detectedLabels section (present but no fields) produces empty DetectedLabels, not nil (distinct from absent) | RED |

### Tier 2: Integration Tests

**Testable code scope**: `server/workflow_handlers.go` (`buildWorkflowFromSchema` ~10 modified lines), DB JSONB round-trip. Target: >=80%.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-043-001` | Workflow registered via POST /api/v1/workflows with detectedLabels is stored accurately in the catalog (JSONB round-trip fidelity for all 8 fields) | RED |
| `IT-DS-043-002` | Workflow retrieved via GET /api/v1/workflows returns exact detectedLabels that were registered (no field loss or type coercion on read-back) | RED |
| `IT-DS-043-003` | Workflow registered without detectedLabels has empty DetectedLabels in catalog (not null, not garbage) | RED |
| `IT-DS-043-005` | Workflow search/discovery filters correctly by detectedLabels (the business purpose: HAPI finds the right workflow for an incident's infrastructure characteristics) | RED |
| `IT-DS-043-006` | Full realistic schema (all fields: metadata + labels + detectedLabels + execution + parameters) round-trips through POST -> DB -> GET with zero data loss across all fields (no regression on existing fields when detectedLabels is added) | RED |
| `IT-DS-043-007` | Workflow version update (POST same workflowId, new version) with changed detectedLabels stores the new values and marks the new version as latest | RED |

### Tier 3: E2E Tests

**Testable code scope**: Full OCI image -> DataStorage service -> PostgreSQL -> HTTP API chain. Validates detectedLabels survive the complete production path.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `E2E-DS-043-001` | OCI image containing workflow-schema.yaml with detectedLabels is registered via CreateWorkflow and the response includes the parsed detectedLabels (both set and unset fields verified) | RED |
| `E2E-DS-043-002` | GetWorkflowByID returns detectedLabels that match the original OCI schema after full DB round-trip (JSONB persistence fidelity) | RED |
| `E2E-DS-043-003` | Workflow with detectedLabels appears in three-step discovery flow (ListWorkflowsByActionType -> GetWorkflowByID) and Step 3 response includes correct detectedLabels | RED |
| `E2E-DS-043-004` | HTTP search with `detected_labels` query parameter filters results: matching filter returns the workflow, non-matching filter excludes or deprioritizes it | RED |
| `E2E-DS-043-005` | All 8 detectedLabels fields survive full OCI -> DB -> HTTP round-trip using a dedicated fixture with every field populated | RED |

---

## 6. Test Cases (Detail)

### UT-DS-043-001: Boolean detectedLabels parsing

**BR**: ADR-043
**Type**: Unit
**File**: `test/unit/datastorage/oci_schema_extractor_test.go`

**Given**: workflow-schema.yaml with `detectedLabels: { hpaEnabled: "true", pdbProtected: "true" }`
**When**: Parser.ParseAndValidate is called
**Then**: `schema.DetectedLabels.HPAEnabled == true` and `schema.DetectedLabels.PDBProtected == true`

**Acceptance Criteria**:
- Boolean fields parsed from string "true" to Go bool `true`
- No other fields affected (GitOpsTool, ServiceMesh remain zero-value)

### UT-DS-043-007: All 8 fields data accuracy

**BR**: ADR-043
**Type**: Unit
**File**: `test/unit/datastorage/oci_schema_extractor_test.go`

**Given**: workflow-schema.yaml with all 8 detectedLabels fields set to valid values
**When**: ExtractDetectedLabels converts to `models.DetectedLabels`
**Then**: Every field in the output struct matches the input value exactly

**Acceptance Criteria**:
- `GitOpsManaged == true`, `GitOpsTool == "argocd"`, `PDBProtected == true`, `HPAEnabled == true`
- `Stateful == true`, `HelmManaged == true`, `NetworkIsolated == true`, `ServiceMesh == "istio"`
- No FailedDetections (empty slice)

### IT-DS-043-005: Workflow discovery by detectedLabels

**BR**: ADR-043
**Type**: Integration
**File**: `test/integration/datastorage/workflow_detected_labels_test.go`

**Given**: Two workflows registered: one with `hpaEnabled: true`, one without
**When**: Search for workflows matching `hpaEnabled: true`
**Then**: Only the HPA-enabled workflow is returned

**Acceptance Criteria**:
- Search result contains exactly 1 workflow
- The returned workflow has `DetectedLabels.HPAEnabled == true`
- The non-HPA workflow is excluded from results

### E2E-DS-043-004: HTTP search with detected_labels query parameter

**BR**: ADR-043, BR-STORAGE-013
**Type**: E2E
**File**: `test/e2e/datastorage/25_detected_labels_search_e2e_test.go`

**Given**: The `detected-labels-test-v1` workflow (hpaEnabled=true, gitOpsTool=argocd) is registered
**When**: `ListWorkflowsByActionType` is called with `DetectedLabels: '{"hpaEnabled":true}'`
**Then**: The workflow appears in the response

**When**: Same call with `DetectedLabels: '{"networkIsolated":true}'` (non-matching)
**Then**: The workflow either does not appear or scores lower than with the matching filter

**Acceptance Criteria**:
- Matching `detected_labels` JSON query parameter returns the expected workflow
- Non-matching parameter excludes or deprioritizes the workflow
- Validates the full HTTP layer: ogen client `OptString` encoding -> query string -> `workflow_handlers.go` JSON deserialization -> repository `SearchByLabels` -> HTTP response

### E2E-DS-043-005: All 8 detectedLabels fields full round-trip

**BR**: ADR-043, BR-WORKFLOW-004
**Type**: E2E
**File**: `test/e2e/datastorage/25_detected_labels_search_e2e_test.go`

**Given**: OCI image `detected-labels-all-fields:v1.0.0` with all 8 fields: hpaEnabled=true, pdbProtected=true, stateful=true, helmManaged=true, networkIsolated=true, gitOpsManaged=true, gitOpsTool=flux, serviceMesh=istio
**When**: Image is registered via `CreateWorkflow`, then retrieved via `GetWorkflowByID`
**Then**: All 8 ogen `Opt*` fields have `.Set == true` with the correct values, and `failedDetections` is empty

**Acceptance Criteria**:
- All 6 boolean fields: `.Set == true`, `.Value == true`
- gitOpsTool: `.Set == true`, `.Value == "flux"`
- serviceMesh: `.Set == true`, `.Value == "istio"`
- failedDetections: empty slice
- Fixture: `test/fixtures/workflows/detected-labels-all-fields/workflow-schema.yaml`
- OCI image: `quay.io/kubernaut-cicd/test-workflows/detected-labels-all-fields:v1.0.0`

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: `oci.MockImagePuller` (existing) for OCI extraction tests
- **Location**: `test/unit/datastorage/oci_schema_extractor_test.go` (extend existing)

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO mocks
- **Infrastructure**: PostgreSQL (existing integration test infra), real HTTP server
- **Location**: `test/integration/datastorage/workflow_detected_labels_test.go` (new file)

### E2E Tests

- **Framework**: Ginkgo/Gomega BDD
- **Mocks**: ZERO mocks
- **Infrastructure**: Live DataStorage service, PostgreSQL, OCI registry (quay.io)
- **Location**: `test/e2e/datastorage/25_detected_labels_search_e2e_test.go`
- **Fixtures**:
  - `test/fixtures/workflows/detected-labels-test/workflow-schema.yaml` (2 fields: hpaEnabled, gitOpsTool)
  - `test/fixtures/workflows/detected-labels-all-fields/workflow-schema.yaml` (all 8 fields)
- **OCI Images**:
  - `quay.io/kubernaut-cicd/test-workflows/detected-labels-test:v1.0.0`
  - `quay.io/kubernaut-cicd/test-workflows/detected-labels-all-fields:v1.0.0`

---

## 8. Execution

```bash
# Unit tests
make test

# Specific unit tests for this feature
go test ./test/unit/datastorage/... -ginkgo.focus="ADR-043"

# Integration tests (requires PostgreSQL)
make test-integration-datastorage

# Specific integration tests for this feature
go test ./test/integration/datastorage/... -ginkgo.focus="ADR-043"

# E2E tests (requires live DataStorage service)
make test-e2e-datastorage

# Specific E2E tests for this feature
go test ./test/e2e/datastorage/... -ginkgo.focus="ADR-043"
```

---

## 9. Notes

**Note 1 -- IT-DS-043-004 removed**: Invalid `detectedLabels` validation (HTTP 400 with field-specific error) is covered by unit tests UT-DS-043-004 (boolean), UT-DS-043-005 (gitOpsTool), and UT-DS-043-006 (serviceMesh). The validation logic is pure (no I/O) and lives in `ValidateDetectedLabels()`, making it ideal for UT coverage. The error-to-HTTP-400 propagation path is the same standard pattern already exercised by existing schema validation integration tests (missing actionType, malformed YAML, etc.). Building and maintaining a deliberately-invalid OCI image for a single IT provides near-zero incremental confidence over the existing UT coverage.

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-20 | Initial test plan: 11 UT + 7 IT scenarios |
| 1.1 | 2026-02-21 | Added 3 E2E scenarios (E2E-DS-043-001/002/003); updated coverage policy to 3-tier |
| 1.2 | 2026-02-21 | Removed IT-DS-043-004 (Skip violation); validation covered by UT-DS-043-004/005/006 (see Note 1) |
| 1.3 | 2026-02-21 | Added E2E-DS-043-004 (HTTP search with detected_labels query param) and E2E-DS-043-005 (all 8 fields round-trip); new fixture `detected-labels-all-fields` |
