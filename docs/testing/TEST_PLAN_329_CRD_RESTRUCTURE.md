# Test Plan: RemediationWorkflow CRD Restructure (#329)

**Feature**: Rename `spec.metadata` to `spec.description`, remove `workflowName`, promote `version` to `spec.version`
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feat/329-crd-restructure`

**Authority**:
- [#329](https://github.com/jordigilh/kubernaut/issues/329): Restructure RemediationWorkflow CRD
- [DD-WORKFLOW-002](docs/architecture/decisions/ADR-043-workflow-schema-definition-standard.md): Workflow schema definition standard
- [BR-WORKFLOW-006](docs/requirements/BR-WORKFLOW-006-remediation-workflow-crd.md): Workflow content integrity

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](INTEGRATION_E2E_NO_MOCKS_POLICY.md)

---

## 1. Scope

### In Scope

- **CRD type definitions** (`api/remediationworkflow/v1alpha1/`): Rename `RemediationWorkflowMetadata` to `RemediationWorkflowDescription`, remove `WorkflowName` field, move `Version` to spec level
- **Schema parser** (`pkg/datastorage/schema/parser.go`): Parse new YAML structure, derive `workflow_name` from `metadata.name`
- **Schema converter** (`pkg/workflowschema/converter.go`): Map between CRD spec and internal models with new field paths
- **DataStorage handlers** (`pkg/datastorage/server/workflow_handlers.go`): Register workflows using `metadata.name` for `WorkflowName` and `spec.version` for `Version`
- **Schema validation** (`pkg/datastorage/models/workflow_schema.go`): Validate new structure; reject missing `spec.version` or `spec.description.what`
- **All fixture and demo workflow-schema.yaml files**: 35 kubernaut fixtures + 19 demo scenarios

### Out of Scope

- **Database schema**: No migration needed; `workflow_name` and `version` DB columns are unchanged
- **DataStorage repository layer**: `crud.go` functions use model fields (`WorkflowName`, `Version`) which are unchanged
- **HAPI / LLM tools**: API response shape is unchanged; `workflow_name` and `version` still appear in discovery responses
- **Uniqueness constraint**: Remains `workflow_name + version` (partial unique index on `status = 'active'`)
- **Backwards compatibility / old format detection**: This is a clean break. The old `spec.metadata` struct is removed entirely from the Go types, so old-format YAML will fail at the YAML unmarshalling level. No explicit rejection logic or migration tests are needed.

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| `spec.description` not `metadata.annotations` | Description is a structured object consumed by LLM; annotations are flat string maps |
| `spec.version` not a label | Version participates in DB uniqueness constraint (partial unique index); labels aren't suitable for DB-level constraints |
| `metadata.name` as workflow name | Eliminates redundancy; Kubernetes metadata.name is the canonical identity |
| Clean break (no backwards compatibility) | Old `spec.metadata` struct removed entirely; old YAML fails at unmarshalling. All fixtures and demo scenarios updated in the same PR |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (parser, converter, validator, type changes)
- **Integration**: >=80% of integration-testable code (HTTP handlers, DB registration flow, content integrity)
- **E2E**: Covered by existing workflow seeding E2E tests (update fixtures only)

### 2-Tier Minimum

Every change is covered by at least UT + IT:
- **Unit tests** validate parsing, conversion, and validation logic in isolation
- **Integration tests** validate end-to-end registration flow through HTTP handlers with real PostgreSQL

### Business Outcome Quality Bar

Tests validate that:
- Workflows register correctly with the new schema structure
- The LLM receives accurate description fields during discovery
- Content integrity (BR-WORKFLOW-006) works with the new field paths
- Invalid schemas (missing version, missing description.what) are rejected with clear errors

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/schema/parser.go` | `Parse`, `Validate`, `ExtractDescription` | ~400 |
| `pkg/workflowschema/converter.go` | `SpecToSchema`, `SchemaToSpec` | ~140 |
| `pkg/datastorage/models/workflow_schema.go` | `WorkflowSchema`, `ValidateDescription` | ~420 |
| `api/remediationworkflow/v1alpha1/remediationworkflow_types.go` | `RemediationWorkflowSpec`, `RemediationWorkflowDescription` | ~220 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/server/workflow_handlers.go` | `HandleRegisterWorkflow`, `buildWorkflowCommon`, `handleContentIntegrityConflict` | ~1400 |
| `pkg/datastorage/oci/extractor.go` | `ExtractFromImage` | ~110 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #329 | Parse new CRD structure (spec.description, spec.version) | P0 | Unit | UT-DS-329-001 | Pending |
| #329 | Derive workflow_name from metadata.name | P0 | Unit | UT-DS-329-002 | Pending |
| #329 | Reject schema missing spec.version | P0 | Unit | UT-DS-329-003 | Pending |
| #329 | Reject schema missing spec.description.what | P0 | Unit | UT-DS-329-004 | Pending |
| #329 | Convert spec.description to/from internal model | P0 | Unit | UT-DS-329-005 | Pending |
| #329 | Convert spec.version to/from internal model | P0 | Unit | UT-DS-329-006 | Pending |
| #329 | OCI extractor reads metadata.name as workflow name | P0 | Unit | UT-DS-329-007 | Pending |
| #329 | Print column shows spec.version in kubectl output | P1 | Unit | UT-DS-329-008 | Pending |
| #329 | Register workflow via inline YAML with new structure | P0 | Integration | IT-DS-329-001 | Pending |
| #329 | Register workflow via OCI bundle with new structure | P0 | Integration | IT-DS-329-002 | Pending |
| BR-WORKFLOW-006 | Content integrity: same name+version+hash = idempotent | P0 | Integration | IT-DS-329-003 | Pending |
| BR-WORKFLOW-006 | Content integrity: same name+version, different hash = supersede | P0 | Integration | IT-DS-329-004 | Pending |
| #329 | Workflow discovery returns correct description fields | P0 | Integration | IT-DS-329-005 | Pending |
| #329 | Version management (latest flag, version history) | P1 | Integration | IT-DS-329-006 | Pending |

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-DS-329-{SEQUENCE}`

- **DS**: DataStorage service (primary affected service)
- **329**: Issue number

### Tier 1: Unit Tests

**Testable code scope**: `parser.go`, `converter.go`, `workflow_schema.go`, `remediationworkflow_types.go` — target >=80% coverage

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-DS-329-001` | Parser correctly extracts spec.description and spec.version from new YAML structure | Pending |
| `UT-DS-329-002` | Parser uses metadata.name as workflow_name | Pending |
| `UT-DS-329-003` | Validator rejects schema missing spec.version with descriptive error | Pending |
| `UT-DS-329-004` | Validator rejects schema missing spec.description.what with descriptive error | Pending |
| `UT-DS-329-005` | Converter maps spec.description fields bidirectionally (SpecToSchema / SchemaToSpec) | Pending |
| `UT-DS-329-006` | Converter maps spec.version bidirectionally (SpecToSchema / SchemaToSpec) | Pending |
| `UT-DS-329-007` | OCI extractor derives workflow name from CRD envelope metadata.name | Pending |
| `UT-DS-329-008` | CRD print column annotation references spec.version (kubectl get output) | Pending |

### Tier 2: Integration Tests

**Testable code scope**: `workflow_handlers.go`, OCI extractor, content integrity flow — target >=80% coverage

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-DS-329-001` | Inline workflow registration succeeds with new YAML structure and stores correct workflow_name + version | Pending |
| `IT-DS-329-002` | OCI-based workflow registration succeeds with new YAML structure | Pending |
| `IT-DS-329-003` | Re-registering same name+version+hash is idempotent (no duplicate, no error) | Pending |
| `IT-DS-329-004` | Re-registering same name+version with different hash supersedes previous workflow | Pending |
| `IT-DS-329-005` | Workflow discovery API returns description.what, description.whenToUse in response | Pending |
| `IT-DS-329-006` | Registering v2.0 of existing workflow correctly sets is_latest_version flags | Pending |

### Tier Skip Rationale

- **E2E**: No new E2E tests. Existing E2E workflow seeding tests will be updated with new fixture format. The registration and discovery flows are fully covered by IT-DS-329-001 through IT-DS-329-006. E2E adds Kind cluster overhead without additional coverage for this change.

---

## 6. Test Cases (Detail)

### UT-DS-329-001: Parse new CRD structure

**BR**: #329
**Type**: Unit
**File**: `test/unit/datastorage/schema_parser_test.go`

**Given**: A workflow-schema.yaml with `spec.version: "1.0.0"` and `spec.description.what: "Rolls back deployment"`
**When**: Parser.Parse() is called
**Then**: Returned schema has `Version == "1.0.0"` and `Description.What == "Rolls back deployment"`

**Acceptance Criteria**:
- `spec.description.what`, `whenToUse`, `whenNotToUse`, `preconditions` all parsed correctly
- `spec.version` parsed at the correct level (not nested under metadata)
- No reference to `spec.metadata` in the parsed output

---

### UT-DS-329-002: Derive workflow name from metadata.name

**BR**: #329
**Type**: Unit
**File**: `test/unit/datastorage/schema_parser_test.go`

**Given**: A workflow-schema.yaml with `metadata.name: "rollback-deployment-v1"`
**When**: Parser parses the CRD envelope
**Then**: The derived workflow_name is `"rollback-deployment-v1"`

**Acceptance Criteria**:
- workflow_name matches `metadata.name` exactly
- No `workflowName` field present in the parsed schema

---

### UT-DS-329-003: Reject missing spec.version

**BR**: #329
**Type**: Unit
**File**: `test/unit/datastorage/schema_parser_test.go`

**Given**: A workflow-schema.yaml with no `spec.version` field
**When**: Parser.Validate() is called
**Then**: Validation fails with error containing "spec.version is required"

**Acceptance Criteria**:
- Clear error message identifying the missing field
- Does not fall back to any default version

---

### UT-DS-329-004: Reject missing spec.description.what

**BR**: #329
**Type**: Unit
**File**: `test/unit/datastorage/schema_parser_test.go`

**Given**: A workflow-schema.yaml with `spec.description` present but `what` field empty or missing
**When**: Parser.ValidateDescription() is called
**Then**: Validation fails with error containing "spec.description.what is required"

**Acceptance Criteria**:
- `what` is mandatory (it's the primary field the LLM uses for workflow selection)
- `whenToUse`, `whenNotToUse`, `preconditions` remain optional

---

### UT-DS-329-005: Converter maps description bidirectionally

**BR**: #329
**Type**: Unit
**File**: `test/unit/workflowschema/converter_test.go`

**Given**: A CRD spec with `spec.description.what`, `whenToUse`, `whenNotToUse`, `preconditions`
**When**: SpecToSchema() and then SchemaToSpec() are called (round-trip)
**Then**: All description fields survive the round-trip without data loss

**Acceptance Criteria**:
- All 4 description fields preserved
- No reference to old `Metadata.Description` path

---

### UT-DS-329-006: Converter maps version bidirectionally

**BR**: #329
**Type**: Unit
**File**: `test/unit/workflowschema/converter_test.go`

**Given**: A CRD spec with `spec.version: "2.1.0"`
**When**: SpecToSchema() then SchemaToSpec() round-trip
**Then**: Version is `"2.1.0"` in both directions

**Acceptance Criteria**:
- Version comes from `spec.version`, not `spec.metadata.version`

---

### UT-DS-329-007: OCI extractor uses metadata.name

**BR**: #329
**Type**: Unit
**File**: `test/unit/datastorage/oci_schema_extractor_test.go`

**Given**: An OCI bundle containing a workflow-schema.yaml with `metadata.name: "patch-hpa-v1"`
**When**: ExtractFromImage() is called
**Then**: The returned schema has `WorkflowName == "patch-hpa-v1"`

**Acceptance Criteria**:
- workflow_name derived from CRD envelope `metadata.name`
- Not from any field inside `spec`

---

### IT-DS-329-001: Inline registration with new structure

**BR**: #329
**Type**: Integration
**File**: `test/integration/datastorage/workflow_registration_329_test.go`

**Given**: DataStorage is running with PostgreSQL; a valid workflow-schema.yaml using the new structure
**When**: POST /api/v1/workflows/register is called with the YAML body
**Then**: Workflow is stored in DB with correct `workflow_name` (from metadata.name), `version` (from spec.version), and `description` (from spec.description.what)

**Acceptance Criteria**:
- DB record `workflow_name` matches `metadata.name`
- DB record `version` matches `spec.version`
- DB record `description` contains `spec.description.what`
- HTTP 201 response

---

### IT-DS-329-003: Content integrity -- idempotent re-apply

**BR**: BR-WORKFLOW-006
**Type**: Integration
**File**: `test/integration/datastorage/workflow_content_integrity_test.go`

**Given**: A workflow already registered with name "rollback-v1", version "1.0.0", hash X
**When**: The exact same YAML is submitted again
**Then**: No error, no duplicate record; existing record is returned

**Acceptance Criteria**:
- HTTP 200 (not 201)
- DB still has exactly one active record for this name+version
- Content hash unchanged

---

### IT-DS-329-005: Discovery returns description fields

**BR**: #329
**Type**: Integration
**File**: `test/integration/datastorage/workflow_discovery_329_test.go`

**Given**: A workflow registered with `spec.description.what: "Rolls back"`, `whenToUse: "On crash loop"`
**When**: GET /api/v1/workflows/by-action-type/{actionType} is called
**Then**: Response includes `description` with `what` and `whenToUse` fields populated

**Acceptance Criteria**:
- Discovery response includes description for LLM consumption
- Field names in API response match what HAPI/LLM expects

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Filesystem (for OCI extractor), HTTP client (for parser tests with YAML strings)
- **Location**: `test/unit/datastorage/`, `test/unit/workflowschema/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks
- **Infrastructure**: PostgreSQL (via testcontainers or shared test DB), DataStorage HTTP server
- **Location**: `test/integration/datastorage/`

---

## 8. Execution

```bash
# Unit tests
make test

# Integration tests
make test-integration-datastorage

# Specific test by ID
go test ./test/unit/datastorage/... -ginkgo.focus="UT-DS-329"
go test ./test/integration/datastorage/... -ginkgo.focus="IT-DS-329"
```

---

## 9. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan |
