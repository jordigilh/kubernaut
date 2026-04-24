# Test Plan: Schema CRD Format Migration (#292)

**Feature**: Restructure workflow-schema.yaml from flat format to Kubernetes CRD envelope (apiVersion/kind/metadata/spec)
**Version**: 1.1
**Created**: 2026-03-08
**Author**: AI Assistant
**Status**: Complete
**Branch**: `feature/v1.0-remaining-bugs-demos`

**Authority**:
- BR-WORKFLOW-004: Workflow Schema Format Specification
- BR-WORKFLOW-006: RemediationWorkflow CRD Definition
- DD-WORKFLOW-017: OCI-based Workflow Registration

**Cross-References**:
- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [GitHub Issue #292](https://github.com/jordigilh/kubernaut/issues/292)
- [GitHub Issue #299](https://github.com/jordigilh/kubernaut/issues/299)

---

## 1. Scope

### In Scope

- **Parser (schema/parser.go)**: CRD envelope validation (apiVersion, kind, metadata.name), spec extraction, apiVersion-to-schemaVersion derivation
- **Models (models/workflow_schema.go)**: WorkflowSchemaCRD wrapper struct, WorkflowCRDMetadata, APIVersionToSchemaVersion mapping
- **CRD Types (api/remediationworkflow/v1alpha1/)**: Kubernetes-native type definitions, deepcopy generation, CRD YAML manifest
- **Test Fixtures**: 27 test fixture YAMLs, 18 demo scenario YAMLs, 2 job fixture YAMLs migrated to CRD format
- **Build Scripts**: update_bundle_digest regex in build-demo-workflows.sh

### Out of Scope

- Phase 2 changes (inline schema DS endpoint, AW webhook handler) -- deferred to #299
- OCI extraction pipeline changes -- still functional, only YAML format changed
- Database schema changes -- schemaVersion column unchanged, derived from apiVersion

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Parser returns unwrapped `*WorkflowSchema` (spec) | Minimizes caller changes; all existing field access paths work unchanged |
| schemaVersion derived from apiVersion | Eliminates redundant field; `kubernaut.ai/v1alpha1` maps to `"1.0"` |
| WorkflowSchemaCRD is internal to parser | Callers (handlers, querier) don't need to know about CRD envelope |
| v1alpha1 (not v1) | Pre-GA; RBAC stanza (#186) will require v1alpha2 |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of **unit-testable** code (pure logic: validators, parsers, builders, types)
- **Integration**: Existing integration tests cover OCI pipeline end-to-end (uses migrated fixtures)
- **E2E**: Existing E2E suites will validate via migrated fixture YAMLs

### 2-Tier Minimum

Every business requirement gap is covered by at least 2 test tiers:
- **Unit tests** catch logic and correctness errors in parser and validator logic
- **Integration tests** (existing OCI pipeline tests) catch wiring and data fidelity across component boundaries

### Business Outcome Quality Bar

Tests validate that the CRD format is correctly parsed, validated, and produces identical business outcomes (workflow catalog entries, parameters, labels, description) as the previous flat format.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/schema/parser.go` | `Parse`, `ParseAndValidate`, `validateCRDEnvelope`, `Validate` | ~100 |
| `pkg/datastorage/models/workflow_schema.go` | `WorkflowSchemaCRD`, `WorkflowCRDMetadata`, `APIVersionToSchemaVersion` | ~30 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/datastorage/server/workflow_handlers.go` | `buildWorkflowFromSchema` (uses parser output) | ~90 |
| `pkg/workflowexecution/client/workflow_querier.go` | `GetWorkflowSchemaMetadata` (parses stored content) | ~30 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-WORKFLOW-006 | apiVersion derivation to schemaVersion | P0 | Unit | UT-DS-255-001 | Pass |
| BR-WORKFLOW-006 | Missing apiVersion rejected | P0 | Unit | UT-DS-255-002 | Pass |
| BR-WORKFLOW-006 | Unsupported apiVersion rejected | P0 | Unit | UT-DS-255-003 | Pass |
| BR-WORKFLOW-004 | actionType parsed from spec | P0 | Unit | UT-DS-017-001 | Pass |
| BR-WORKFLOW-004 | Labels extracted with camelCase keys | P0 | Unit | UT-DS-017-002 | Pass |
| BR-WORKFLOW-004 | Structured description parsed | P1 | Unit | UT-DS-017-008 | Pass |
| BR-WORKFLOW-004 | Priority normalized to uppercase | P1 | Unit | UT-DS-017-010 | Pass |
| BR-WORKFLOW-004 | Custom labels separated from mandatory | P1 | Unit | UT-DS-212-001 | Pass |
| BR-WORKFLOW-004 | Custom labels correct format | P1 | Unit | UT-DS-212-002 | Pass |
| BR-WORKFLOW-004 | Digest-only bundle accepted | P0 | Unit | UT-DS-017-011 | Pass |
| BR-WORKFLOW-004 | Tag+digest bundle accepted | P0 | Unit | UT-DS-017-012 | Pass |
| BR-WORKFLOW-004 | Tag-only bundle rejected | P0 | Unit | UT-DS-017-013 | Pass |
| BR-WORKFLOW-004 | Missing execution rejected | P0 | Unit | UT-DS-017-014 | Pass |
| DD-WE-006 | Dependencies parsed | P0 | Unit | UT-DS-006-001 | Pass |
| DD-WE-006 | No dependencies backward compat | P1 | Unit | UT-DS-006-002 | Pass |
| DD-WE-006 | Empty secret name rejected | P0 | Unit | UT-DS-006-010 | Pass |
| DD-WE-006 | Duplicate secrets rejected | P0 | Unit | UT-DS-006-012 | Pass |
| ADR-043 | DetectedLabels parsed | P1 | Unit | UT-DS-043-001 | Pass |
| ADR-043 | All 8 detectedLabels fields | P1 | Unit | UT-DS-043-007 | Pass |
| ADR-043 | Unknown detectedLabels field rejected | P1 | Unit | UT-DS-043-010 | Pass |
| BR-WE-016 | Ansible engineConfig extracted | P0 | Unit | UT-WE-016-005 | Pass |
| BR-WE-016 | Ansible without engineConfig rejected | P0 | Unit | UT-WE-016-007 | Pass |
| BR-WORKFLOW-005 | Float parameter type accepted | P1 | Unit | UT-WF-005-001 | Pass |
| BR-WORKFLOW-005 | Float min/max bounds | P1 | Unit | UT-WF-005-002 | Pass |
| DD-WORKFLOW-017 | Valid OCI registration | P0 | Unit | UT-WF-017-001 | Pass |
| DD-WORKFLOW-017 | Invalid schema rejected | P0 | Unit | UT-WF-017-005 | Pass |
| BR-WORKFLOW-016 | Invalid action_type rejected | P0 | Unit | UT-WF-017-008 | Pass |
| DD-WE-006 | Querier extracts dependencies | P0 | Unit | UT-WE-006-001 | Pass |
| DD-WE-006 | Querier extracts secrets+configMaps | P0 | Unit | UT-WE-006-002 | Pass |
| DD-WE-006 | Querier nil for no dependencies | P1 | Unit | UT-WE-006-003 | Pass |

### Status Legend

- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `DS` (DataStorage), `WE` (WorkflowExecution), `WF` (Workflow cross-service)
- **BR_NUMBER**: Business requirement, ADR, or DD number
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

**Testable code scope**: `parser.go` (~100 lines), `workflow_schema.go` (~30 lines) -- target >=90%

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-DS-255-001 | Operator's workflow with valid `apiVersion: kubernaut.ai/v1alpha1` is accepted and schemaVersion "1.0" is derived for DB storage | Pass |
| UT-DS-255-002 | Operator submitting YAML without `apiVersion` gets a clear validation error naming the missing field | Pass |
| UT-DS-255-003 | Operator submitting YAML with unsupported `apiVersion` gets a clear error with accepted values | Pass |
| UT-DS-017-001 | Workflow catalog correctly indexes actionType from CRD spec for three-step discovery | Pass |
| UT-DS-017-002 | Workflow labels stored in JSONB use camelCase keys for OpenAPI compliance | Pass |
| UT-DS-017-008 | LLM receives structured description fields (what, whenToUse, whenNotToUse, preconditions) | Pass |
| UT-DS-017-010 | Lowercase priority in YAML is normalized to uppercase for OpenAPI enum compliance | Pass |
| UT-DS-212-001 | Custom labels are stored separately from mandatory discovery labels | Pass |
| UT-DS-212-002 | Custom labels are extracted as map[string][]string for the custom_labels column | Pass |
| UT-DS-017-011 | Digest-only OCI bundle reference is accepted (immutable deployment) | Pass |
| UT-DS-017-012 | Tag+digest OCI bundle reference is accepted (human-readable + immutable) | Pass |
| UT-DS-017-013 | Tag-only OCI bundle reference is rejected with field-specific error | Pass |
| UT-DS-017-014 | Workflow without execution section is rejected | Pass |
| UT-DS-006-001 | Workflow dependencies (secrets, configMaps) are correctly extracted for WFE provisioning | Pass |
| UT-DS-006-002 | Workflow without dependencies section is backward-compatible (nil) | Pass |
| UT-DS-006-010 | Secret with empty name is rejected with descriptive error | Pass |
| UT-DS-006-012 | Duplicate secret names are rejected with descriptive error | Pass |
| UT-DS-043-001 | DetectedLabels HPA+PDB requirements are parsed for incident matching | Pass |
| UT-DS-043-007 | All 8 detectedLabels fields survive YAML-to-model conversion | Pass |
| UT-DS-043-010 | Unknown detectedLabels field is rejected with the typo'd field name | Pass |
| UT-WE-016-005 | Ansible engineConfig (playbookPath, jobTemplateName, inventoryName) extracted for AWX | Pass |
| UT-WE-016-007 | Ansible workflow without engineConfig is rejected before reaching AWX | Pass |
| UT-WF-005-001 | Float parameter type is accepted in schema for AWX survey compatibility | Pass |
| UT-WF-005-002 | Float min/max bounds are correctly parsed for parameter validation | Pass |
| UT-WF-017-001 | Valid OCI registration with CRD-format schema succeeds | Pass |
| UT-WF-017-005 | Invalid schema (missing required spec fields) returns 400 validation-error | Pass |
| UT-WF-017-008 | Workflow with action_type not in taxonomy returns 400 with taxonomy error | Pass |
| UT-WE-006-001 | WFE querier extracts secret dependencies from stored CRD-format content | Pass |
| UT-WE-006-002 | WFE querier extracts both secrets and configMaps from stored content | Pass |
| UT-WE-006-003 | WFE querier returns nil for workflow with no dependencies | Pass |

### Tier Skip Rationale

- **Integration**: No new integration tests needed. Existing integration tests (OCI pipeline with mock puller, DS workflow dependency validation) exercise the parser through the full registration flow. The migrated fixture YAMLs ensure the CRD format flows through correctly.
- **E2E**: No new E2E tests needed. Existing E2E suites use the migrated fixture YAMLs and will validate the format change end-to-end when run.

---

## 6. Test Cases (Detail)

### UT-DS-255-001: Valid apiVersion derives schemaVersion

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/datastorage/schema_version_test.go`

**Given**: A valid CRD-format workflow YAML with `apiVersion: kubernaut.ai/v1alpha1`, `kind: RemediationWorkflow`, and all required spec fields
**When**: The parser's `ParseAndValidate()` processes the YAML
**Then**: The returned `WorkflowSchema.SchemaVersion` is `"1.0"`, derived from the apiVersion mapping

**Acceptance Criteria**:
- `parsedSchema.SchemaVersion` equals `"1.0"` exactly
- No error returned from `ParseAndValidate()`
- The YAML does NOT contain a `schemaVersion` field -- it is derived

---

### UT-DS-255-002: Missing apiVersion rejected with actionable error

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/datastorage/schema_version_test.go`

**Given**: A CRD-format workflow YAML that is missing the `apiVersion` field (has `kind`, `metadata`, `spec`)
**When**: The parser's `ParseAndValidate()` processes the YAML
**Then**: A `*models.SchemaValidationError` is returned with `Field` containing `"apiVersion"`

**Acceptance Criteria**:
- Error is of type `*models.SchemaValidationError`
- Error message contains `"apiVersion"` so the operator knows which field to fix
- No partial result returned

---

### UT-DS-255-003: Unsupported apiVersion rejected with valid values

**BR**: BR-WORKFLOW-006
**Type**: Unit
**File**: `test/unit/datastorage/schema_version_test.go`

**Given**: A CRD-format workflow YAML with `apiVersion: kubernaut.ai/v2` (unsupported)
**When**: The parser's `ParseAndValidate()` processes the YAML
**Then**: A `*models.SchemaValidationError` is returned referencing `"apiVersion"`

**Acceptance Criteria**:
- Error is of type `*models.SchemaValidationError`
- Error message contains `"apiVersion"` to identify the field
- Error message contains the invalid value `"kubernaut.ai/v2"` so the operator can compare

---

### UT-DS-017-001: actionType parsed from CRD spec for discovery indexing

**BR**: BR-WORKFLOW-004
**Type**: Unit
**File**: `test/unit/datastorage/oci_schema_extractor_test.go`

**Given**: A valid CRD-format YAML with `spec.actionType: RestartPod`
**When**: `ParseAndValidate()` is called
**Then**: `parsedSchema.ActionType` equals `"RestartPod"`

**Acceptance Criteria**:
- ActionType is extracted from `spec.actionType` (not top-level)
- Value is preserved exactly as written (PascalCase)

---

### UT-DS-017-002: Labels extracted with camelCase keys for JSONB storage

**BR**: BR-WORKFLOW-004
**Type**: Unit
**File**: `test/unit/datastorage/oci_schema_extractor_test.go`

**Given**: A valid CRD-format YAML with labels under `spec.labels`
**When**: `ParseAndValidate()` + `ExtractLabels()` are called
**Then**: Labels JSON uses camelCase keys (`severity`, `environment`, `component`, `priority`)

**Acceptance Criteria**:
- `labels["severity"]` is `["critical"]` (JSONB array)
- `labels["environment"]` is `["production"]` (JSONB array)
- `labels["component"]` is `"pod"` (string)
- `labels["priority"]` is `"P1"` (normalized to uppercase from `p1`)

---

### UT-DS-017-013: Tag-only bundle rejected with field-specific error

**BR**: BR-WORKFLOW-004
**Type**: Unit
**File**: `test/unit/datastorage/execution_bundle_validation_test.go`

**Given**: A CRD-format YAML where `spec.execution.bundle` contains a tag-only reference (e.g., `quay.io/kubernaut/workflows/scale-memory-bundle:v1.0.0` without `@sha256:`)
**When**: `ParseAndValidate()` is called
**Then**: A `*models.SchemaValidationError` is returned with `Field == "execution.bundle"` and message containing `"sha256"`

**Acceptance Criteria**:
- Error type is `*models.SchemaValidationError`
- `schemaErr.Field` is exactly `"execution.bundle"`
- `schemaErr.Message` contains `"sha256"` to guide the operator toward digest-pinning

---

### UT-DS-006-001: Dependencies parsed from CRD spec

**BR**: DD-WE-006
**Type**: Unit
**File**: `test/unit/datastorage/schema_dependencies_test.go`

**Given**: A CRD-format YAML with `spec.dependencies.secrets: [{name: "gitea-repo-creds"}]` and `spec.dependencies.configMaps: [{name: "remediation-config"}]`
**When**: `ParseAndValidate()` is called
**Then**: `parsedSchema.Dependencies.Secrets` has 1 item with `Name == "gitea-repo-creds"` and `parsedSchema.Dependencies.ConfigMaps` has 1 item with `Name == "remediation-config"`

**Acceptance Criteria**:
- Secrets slice length is 1
- ConfigMaps slice length is 1
- Names match exactly

---

### UT-DS-043-007: All 8 detectedLabels fields survive conversion

**BR**: ADR-043
**Type**: Unit
**File**: `test/unit/datastorage/oci_schema_extractor_test.go`

**Given**: A CRD-format YAML with all 8 detectedLabels fields under `spec.detectedLabels` (gitOpsManaged, gitOpsTool, pdbProtected, hpaEnabled, stateful, helmManaged, networkIsolated, serviceMesh)
**When**: `ParseAndValidate()` + `ExtractDetectedLabels()` are called
**Then**: All 8 fields are correctly converted from string to typed values

**Acceptance Criteria**:
- Boolean fields (`gitOpsManaged`, `pdbProtected`, `hpaEnabled`, `stateful`, `helmManaged`, `networkIsolated`): `true`
- String fields: `gitOpsTool == "argocd"`, `serviceMesh == "istio"`
- `PopulatedFields` contains all 8 field names
- `FailedDetections` is empty

---

### UT-WE-016-005: Ansible engineConfig extracted for AWX execution

**BR**: BR-WE-016
**Type**: Unit
**File**: `test/unit/datastorage/engine_config_parser_test.go`

**Given**: A CRD-format YAML with `spec.execution.engine: ansible` and `spec.execution.engineConfig` containing `playbookPath`, `jobTemplateName`, `inventoryName`
**When**: `Parse()` + `ExtractEngineConfig()` + `ParseEngineConfig("ansible", ...)` are called
**Then**: `AnsibleEngineConfig` is returned with correct field values

**Acceptance Criteria**:
- `ansibleCfg.PlaybookPath` equals `"playbooks/restart_pod.yml"`
- `ansibleCfg.JobTemplateName` equals `"restart-pod"`
- `ansibleCfg.InventoryName` equals `"production"`

---

### UT-WE-006-001: WFE querier extracts dependencies from CRD-format stored content

**BR**: DD-WE-006
**Type**: Unit
**File**: `test/unit/workflowexecution/workflow_querier_test.go`

**Given**: A mock DS client returning a `RemediationWorkflow` with CRD-format YAML in the `Content` field, including `spec.dependencies.secrets: [{name: "gitea-repo-creds"}]`
**When**: `GetWorkflowSchemaMetadata()` is called with a valid workflow UUID
**Then**: Dependencies are correctly extracted: 1 secret named `"gitea-repo-creds"`, empty configMaps

**Acceptance Criteria**:
- `deps.Secrets` has length 1
- `deps.Secrets[0].Name` equals `"gitea-repo-creds"`
- `deps.ConfigMaps` is empty
- No error returned

---

### UT-WF-017-001: Valid OCI registration with CRD-format schema

**BR**: DD-WORKFLOW-017
**Type**: Unit
**File**: `test/unit/datastorage/workflow_create_oci_handler_test.go`

**Given**: A mock OCI image puller returning a valid CRD-format `workflow-schema.yaml` and a handler wired with the mock extractor
**When**: `HandleCreateWorkflow` is called with a valid `schemaImage` POST body
**Then**: The handler does NOT return 400, 422, or 502 (extraction and validation succeed; may fail at DB insertion since no DB is wired)

**Acceptance Criteria**:
- Response status is NOT `400` (valid schema)
- Response status is NOT `422` (schema found in image)
- Response status is NOT `502` (image pull succeeded)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `MockImagePuller`, `FailingMockImagePuller`, `MockImagePullerWithFailingExists` (OCI layer); `mockWorkflowCatalogClient` (DS client for querier); `mockActionTypeValidator` (taxonomy)
- **Location**: `test/unit/datastorage/`, `test/unit/workflowexecution/`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks -- uses real OCI pipeline with mock puller (DS integration), real envtest K8s API
- **Infrastructure**: PostgreSQL (via testcontainers), Redis (optional)
- **Location**: `test/integration/datastorage/`

---

## 8. Execution

```bash
# Unit tests (datastorage)
go test ./test/unit/datastorage/ -count=1 -timeout=120s

# Unit tests (workflowexecution)
go test ./test/unit/workflowexecution/ -count=1 -timeout=120s

# All unit tests
go test ./test/unit/... -count=1 -timeout=300s

# Specific test by ID (Ginkgo focus)
go test ./test/unit/datastorage/ --ginkgo.focus="UT-DS-255-001"

# Integration tests (datastorage)
make test-integration-datastorage

# Build validation
go build ./...
```

---

## 9. Coverage Summary

| Tier | Tests | Estimated Coverage | Status |
|------|-------|--------------------|--------|
| Unit (datastorage) | 526 scenarios | >90% of parser.go, workflow_schema.go | All passing |
| Unit (workflowexecution) | All scenarios | >80% of workflow_querier.go | All passing |
| Integration (datastorage) | Fixture YAMLs migrated | Covered by existing OCI pipeline tests | Ready |
| E2E | Fixture + demo YAMLs migrated | Covered by existing E2E suites | Ready |

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-08 | Initial test plan |
| 1.1 | 2026-03-08 | Added IEEE 829-style detailed test cases (Section 6), BR Coverage Matrix (Section 4), execution commands (Section 8), test infrastructure (Section 7) |
