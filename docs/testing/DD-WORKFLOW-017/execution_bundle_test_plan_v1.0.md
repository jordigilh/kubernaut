# DataStorage Execution Bundle Test Plan (Issue #89)

**Design Decision**: DD-WORKFLOW-017 - Workflow Lifecycle Component Interactions
**Business Requirement**: BR-WORKFLOW-004 - Workflow Schema Format Specification
**Service**: DataStorage
**Version**: 1.0
**Date**: February 17, 2026
**Status**: ACTIVE

---

## Test Plan Overview

### Scope

This test plan covers enforcement of digest-only `execution.bundle` references
during workflow registration in the DataStorage service (Issue #89):

- **Schema validation**: `execution.bundle` is mandatory, must contain `@sha256:` with 64 hex chars
- **Handler validation**: OCI registration pipeline rejects invalid bundles with RFC 7807 errors
- **Storage**: New `execution_bundle` and `execution_bundle_digest` fields in DB and API responses
- **Field rename**: `container_image` -> `schema_image` in DS layer

### Services Under Test

1. **DataStorage**: Schema parser (`pkg/datastorage/schema/parser.go`), workflow handlers (`pkg/datastorage/server/workflow_handlers.go`), repository, API responses
2. **Integration Point**: DataStorage -> HAPI contract (`execution_bundle` in discovery API responses)

### Out of Scope

- HAPI consumption of `execution_bundle` (covered by Phase 2 HAPI test plan)
- Downstream CRD field renames (AA `SelectedWorkflow`, WFE `WorkflowRef`) (covered by Phase 3)
- OCI 1.1 subject/referrers integration (Issue #105)
- DB migration and repository SQL (covered during GREEN phase, not unit-testable)

### Design Decisions

**Decision date**: 2026-02-17
**Context**: The `execution.bundle` field in `workflow-schema.yaml` is extracted during OCI registration
but currently not validated or stored. Tag-only references (e.g., `:v1.0.0`) are mutable and violate
supply-chain integrity requirements.

**Decision**: `execution.bundle` is mandatory and must be pinned by sha256 digest. Only `sha256` algorithm
is accepted. The full 64 hex character digest is required.

**Rationale**:
- Digest-pinned references are immutable, ensuring the same bundle is always executed
- sha256 is the industry standard for OCI content-addressable storage
- Rejecting short or non-sha256 digests prevents accidental truncation or weak algorithms

---

## Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-017-{SEQUENCE}`

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md) and
[V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md):

- `UT-DS-017-0xx` -- DataStorage schema parser unit tests (Go, Ginkgo/Gomega)
- `UT-WF-017-0xx` -- Workflow registration handler unit tests (Go, Ginkgo/Gomega)

Existing scenarios in this series: UT-DS-017-001 through UT-DS-017-010 and UT-WF-017-001 through UT-WF-017-009
cover OCI extraction and action type FK validation. This test plan extends with 011+ and 010+ respectively.

---

## Triage Principles Applied

1. **Business outcome focus**: Every scenario validates an observable outcome (parsed struct fields, error types, HTTP status codes) -- not internal parser implementation details.
2. **Exact error type validation**: All rejection scenarios assert the Go error type is `*models.SchemaValidationError` with specific `Field` and `Message` content -- not generic `error != nil`.
3. **RFC 7807 compliance**: Handler rejection scenarios validate the full Problem Details response structure (`type`, `detail`, `status`) -- not just HTTP status code.
4. **Separation of concerns**: Schema validation tests use `parser.ParseAndValidate()` directly. Handler tests use the full OCI pipeline (`MockImagePuller` -> `SchemaExtractor` -> `Handler`). No overlap.

---

## Defense-in-Depth Coverage

| Tier | BR Coverage | Code Coverage Target | Scenarios | Focus |
|------|-------------|---------------------|-----------|-------|
| **Unit** | 100% of execution.bundle validation logic | 100% of `Validate()` bundle path + handler rejection path + bundle existence check | 12 (8 schema + 4 handler) | Parser validation, handler error propagation, bundle existence |
| **Integration** | DB storage and API responses | Deferred to GREEN phase | TBD | Repository SQL, API field presence |

---

## 1. Unit Tests -- Schema Validation (Go)

**Location**: `test/unit/datastorage/execution_bundle_validation_test.go`
**Framework**: Ginkgo/Gomega BDD
**SUT**: `schema.Parser.ParseAndValidate()` (`pkg/datastorage/schema/parser.go`)
**Error type**: `*models.SchemaValidationError` (`pkg/datastorage/models/workflow_schema.go`)
**Existing pattern reference**: `test/unit/datastorage/oci_schema_extractor_test.go` (schema validation via `parser.ParseAndValidate()`)

**Shared preconditions for all schema validation tests**:
- `schema.NewParser()` instantiated in `BeforeEach`
- Base schema YAML satisfies all BR-WORKFLOW-004 non-execution requirements (metadata, actionType, labels, parameters)
- Only the `execution:` section varies between tests

**Base schema YAML constant** (`baseSchemaPrefix`):
```yaml
metadata:
  workflowId: exec-bundle-test
  version: "v1.0.0"
  description:
    what: Tests execution.bundle validation
    whenToUse: When validating digest enforcement
    whenNotToUse: N/A
    preconditions: None
actionType: RestartPod
labels:
  signalType: OOMKilled
  severity: [critical]
  component: pod
  environment: [production]
  priority: P0
parameters:
  - name: NAMESPACE
    type: string
    description: Target namespace
    required: true
```

### 1.1 Positive Scenarios (Valid Bundles)

---

### UT-DS-017-011: Accept valid digest-only execution.bundle

- **BR**: BR-WORKFLOW-004 (Schema Format), DD-WORKFLOW-017 (Registration Lifecycle)
- **Business Outcome**: DataStorage accepts schemas where `execution.bundle` is pinned solely by digest, the most secure and minimal reference format for immutable execution artifacts
- **Preconditions**:
  - `schema.NewParser()` instantiated
  - Base schema YAML satisfies all non-execution BR-WORKFLOW-004 requirements
- **Given**:
  - workflow-schema.yaml with digest-only execution section:
    ```yaml
    execution:
      engine: tekton
      bundle: quay.io/kubernaut/workflows/scale-memory-bundle@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
    ```
- **When**: `parser.ParseAndValidate(validDigestOnlyBundleSchemaYAML)` is called
- **Then**:
  - `err` is nil -- no validation error returned
  - `parsedSchema.Execution` is not nil -- execution section was parsed
  - `parsedSchema.Execution.Bundle` contains `"@sha256:"` -- digest reference preserved in parsed struct
- **Exit Criteria**:
  - Parser does not reject digest-only references
  - Full bundle string is preserved unmodified in the parsed struct
  - No `SchemaValidationError` is returned
- **Implementation Hint**:
  ```go
  It("UT-DS-017-011: should accept valid digest-only execution.bundle", func() {
      parsedSchema, err := parser.ParseAndValidate(validDigestOnlyBundleSchemaYAML)
      Expect(err).ToNot(HaveOccurred(), "digest-only bundle should be accepted")
      Expect(parsedSchema.Execution).ToNot(BeNil())
      Expect(parsedSchema.Execution.Bundle).To(ContainSubstring("@sha256:"))
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not assert only on `err == nil`; also verify the parsed struct has the bundle populated
  - Do not modify or normalize the bundle string during parsing -- preserve it as-is

---

### UT-DS-017-012: Accept valid tag+digest execution.bundle

- **BR**: BR-WORKFLOW-004 (Schema Format), DD-WORKFLOW-017 (Registration Lifecycle)
- **Business Outcome**: DataStorage accepts schemas where `execution.bundle` includes both a human-readable tag and an immutable digest, allowing operators to use familiar versioning alongside supply-chain integrity
- **Preconditions**:
  - `schema.NewParser()` instantiated
  - Base schema YAML satisfies all non-execution BR-WORKFLOW-004 requirements
- **Given**:
  - workflow-schema.yaml with tag+digest execution section:
    ```yaml
    execution:
      engine: tekton
      bundle: quay.io/kubernaut/workflows/scale-memory-bundle:v1.0.0@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
    ```
- **When**: `parser.ParseAndValidate(validTagDigestBundleSchemaYAML)` is called
- **Then**:
  - `err` is nil -- no validation error returned
  - `parsedSchema.Execution` is not nil -- execution section was parsed
  - `parsedSchema.Execution.Bundle` contains `":v1.0.0@sha256:"` -- both tag and digest preserved
- **Exit Criteria**:
  - Tag portion (`:v1.0.0`) is preserved in the bundle string
  - Digest portion (`@sha256:...64 hex chars`) is present and not stripped
  - No `SchemaValidationError` is returned
- **Implementation Hint**:
  ```go
  It("UT-DS-017-012: should accept valid tag+digest execution.bundle", func() {
      parsedSchema, err := parser.ParseAndValidate(validTagDigestBundleSchemaYAML)
      Expect(err).ToNot(HaveOccurred(), "tag+digest bundle should be accepted")
      Expect(parsedSchema.Execution).ToNot(BeNil())
      Expect(parsedSchema.Execution.Bundle).To(ContainSubstring(":v1.0.0@sha256:"))
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not strip the tag when a digest is present -- both must be preserved
  - Do not split on `@` and discard the left side; the full reference is the bundle identity

---

### 1.2 Negative Scenarios (Invalid Bundles)

---

### UT-DS-017-013: Reject tag-only execution.bundle

- **BR**: BR-WORKFLOW-004 (Schema Format), Issue #89 (Digest-only enforcement)
- **Business Outcome**: DataStorage rejects schemas where `execution.bundle` uses only a mutable tag reference, preventing supply-chain attacks where a tag is re-pointed to a different artifact after registration
- **Preconditions**:
  - `schema.NewParser()` instantiated
  - Base schema YAML satisfies all non-execution BR-WORKFLOW-004 requirements
- **Given**:
  - workflow-schema.yaml with tag-only execution section:
    ```yaml
    execution:
      engine: tekton
      bundle: quay.io/kubernaut/workflows/scale-memory-bundle:v1.0.0
    ```
- **When**: `parser.ParseAndValidate(tagOnlyBundleSchemaYAML)` is called
- **Then**:
  - `err` is not nil -- validation rejects the schema
  - `err` is assignable to `*models.SchemaValidationError` -- typed error, not generic
  - `err.Error()` contains `"execution.bundle"` -- identifies the offending field
  - `err.Error()` contains `"sha256"` -- tells the operator what is required
- **Exit Criteria**:
  - Error type is `*models.SchemaValidationError` (not `fmt.Errorf` or generic error)
  - Error message is actionable: operator can identify the field and the fix (add `@sha256:...`)
  - No parsed schema is returned (first return value is unusable)
- **Implementation Hint**:
  ```go
  It("UT-DS-017-013: should reject tag-only execution.bundle", func() {
      _, err := parser.ParseAndValidate(tagOnlyBundleSchemaYAML)
      Expect(err).To(HaveOccurred(), "tag-only bundle must be rejected")

      var schemaErr *models.SchemaValidationError
      Expect(err).To(BeAssignableToTypeOf(schemaErr),
          "error should be SchemaValidationError")
      Expect(err.Error()).To(ContainSubstring("execution.bundle"),
          "error should reference execution.bundle field")
      Expect(err.Error()).To(ContainSubstring("sha256"),
          "error should mention digest requirement")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not assert only on `err != nil` -- must verify the error type and message content
  - Do not check `err.Error() == "exact string"` -- use `ContainSubstring` for resilience against message wording changes

---

### UT-DS-017-014: Reject schema with missing execution section

- **BR**: BR-WORKFLOW-004 (Schema Format), Issue #89 (Mandatory execution.bundle)
- **Business Outcome**: DataStorage rejects schemas that omit the `execution` section entirely, ensuring every registered workflow declares how it should be executed -- a mandatory field for the remediation lifecycle
- **Preconditions**:
  - `schema.NewParser()` instantiated
  - Base schema YAML satisfies all non-execution BR-WORKFLOW-004 requirements
- **Given**:
  - workflow-schema.yaml with no `execution:` section at all:
    ```yaml
    metadata:
      workflowId: exec-bundle-test
      version: "v1.0.0"
      # ... (base schema fields only, no execution section)
    ```
- **When**: `parser.ParseAndValidate(noExecutionSectionSchemaYAML)` is called
- **Then**:
  - `err` is not nil -- validation rejects the schema
  - `err` is assignable to `*models.SchemaValidationError` -- typed error
  - `err.Error()` contains `"execution"` -- identifies the missing section
- **Exit Criteria**:
  - Error type is `*models.SchemaValidationError`
  - Error message references the `execution` section (not a nil pointer panic)
  - Parser handles nil `Execution` field gracefully
- **Implementation Hint**:
  ```go
  It("UT-DS-017-014: should reject schema with missing execution section", func() {
      _, err := parser.ParseAndValidate(noExecutionSectionSchemaYAML)
      Expect(err).To(HaveOccurred(), "missing execution section must be rejected")

      var schemaErr *models.SchemaValidationError
      Expect(err).To(BeAssignableToTypeOf(schemaErr),
          "error should be SchemaValidationError")
      Expect(err.Error()).To(ContainSubstring("execution"),
          "error should reference execution section")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not let this test pass by panicking on nil `Execution` -- the parser must check for nil before accessing fields
  - Do not assert on `"execution.bundle"` here -- the error should reference the section (`"execution"`), not the field within it

---

### UT-DS-017-015: Reject empty bundle string

- **BR**: BR-WORKFLOW-004 (Schema Format), Issue #89 (Mandatory execution.bundle)
- **Business Outcome**: DataStorage rejects schemas where `execution.bundle` is present but empty, preventing registration of workflows with no declared execution artifact
- **Preconditions**:
  - `schema.NewParser()` instantiated
  - Base schema YAML satisfies all non-execution BR-WORKFLOW-004 requirements
- **Given**:
  - workflow-schema.yaml with empty bundle:
    ```yaml
    execution:
      engine: tekton
      bundle: ""
    ```
- **When**: `parser.ParseAndValidate(emptyBundleSchemaYAML)` is called
- **Then**:
  - `err` is not nil -- validation rejects the schema
  - `err` is assignable to `*models.SchemaValidationError` -- typed error
  - `err.Error()` contains `"execution.bundle"` -- identifies the offending field
- **Exit Criteria**:
  - Error type is `*models.SchemaValidationError`
  - Empty string `""` is not confused with "field not present" -- both are rejected
  - Error message tells operator the field is required
- **Implementation Hint**:
  ```go
  It("UT-DS-017-015: should reject empty bundle string", func() {
      _, err := parser.ParseAndValidate(emptyBundleSchemaYAML)
      Expect(err).To(HaveOccurred(), "empty bundle string must be rejected")

      var schemaErr *models.SchemaValidationError
      Expect(err).To(BeAssignableToTypeOf(schemaErr),
          "error should be SchemaValidationError")
      Expect(err.Error()).To(ContainSubstring("execution.bundle"),
          "error should reference execution.bundle field")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not treat `bundle: ""` differently from `bundle:` (YAML null) -- both should be rejected
  - Do not skip the empty string case assuming "nobody would do that" -- it is a common operator error

---

### UT-DS-017-016: Reject execution section without bundle field

- **BR**: BR-WORKFLOW-004 (Schema Format), Issue #89 (Mandatory execution.bundle)
- **Business Outcome**: DataStorage rejects schemas that declare an execution engine but omit the bundle reference, preventing half-configured workflows that know which engine to use but not what to execute
- **Preconditions**:
  - `schema.NewParser()` instantiated
  - Base schema YAML satisfies all non-execution BR-WORKFLOW-004 requirements
- **Given**:
  - workflow-schema.yaml with execution section but no bundle:
    ```yaml
    execution:
      engine: tekton
    ```
- **When**: `parser.ParseAndValidate(executionNoBundleSchemaYAML)` is called
- **Then**:
  - `err` is not nil -- validation rejects the schema
  - `err` is assignable to `*models.SchemaValidationError` -- typed error
  - `err.Error()` contains `"execution.bundle"` -- identifies the missing field specifically
- **Exit Criteria**:
  - Error type is `*models.SchemaValidationError`
  - Error references `execution.bundle` (the field), not just `execution` (the section)
  - Presence of `engine` alone does not satisfy the validation
- **Implementation Hint**:
  ```go
  It("UT-DS-017-016: should reject execution section without bundle field", func() {
      _, err := parser.ParseAndValidate(executionNoBundleSchemaYAML)
      Expect(err).To(HaveOccurred(), "execution without bundle must be rejected")

      var schemaErr *models.SchemaValidationError
      Expect(err).To(BeAssignableToTypeOf(schemaErr),
          "error should be SchemaValidationError")
      Expect(err.Error()).To(ContainSubstring("execution.bundle"),
          "error should reference execution.bundle field")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not accept schemas with `engine` but no `bundle` -- both are required together
  - Do not assert on `"execution"` alone -- be specific: `"execution.bundle"` to distinguish from UT-DS-017-014

---

### UT-DS-017-017: Reject non-sha256 digest algorithm

- **BR**: BR-WORKFLOW-004 (Schema Format), Issue #89 (sha256-only enforcement)
- **Business Outcome**: DataStorage rejects schemas using non-standard digest algorithms (e.g., md5), ensuring all execution bundles use the OCI-standard sha256 algorithm for content addressing and preventing weak or deprecated hash algorithms in the supply chain
- **Preconditions**:
  - `schema.NewParser()` instantiated
  - Base schema YAML satisfies all non-execution BR-WORKFLOW-004 requirements
- **Given**:
  - workflow-schema.yaml with non-sha256 digest:
    ```yaml
    execution:
      engine: tekton
      bundle: quay.io/kubernaut/test@md5:abc123def456
    ```
- **When**: `parser.ParseAndValidate(wrongAlgorithmBundleSchemaYAML)` is called
- **Then**:
  - `err` is not nil -- validation rejects the schema
  - `err` is assignable to `*models.SchemaValidationError` -- typed error
  - `err.Error()` contains `"sha256"` -- tells operator which algorithm is required
- **Exit Criteria**:
  - Error type is `*models.SchemaValidationError`
  - Presence of `@` in the reference does not automatically pass -- the algorithm after `@` must be `sha256`
  - Error message guides the operator toward the correct algorithm
- **Implementation Hint**:
  ```go
  It("UT-DS-017-017: should reject non-sha256 digest algorithm", func() {
      _, err := parser.ParseAndValidate(wrongAlgorithmBundleSchemaYAML)
      Expect(err).To(HaveOccurred(), "non-sha256 digest must be rejected")

      var schemaErr *models.SchemaValidationError
      Expect(err).To(BeAssignableToTypeOf(schemaErr),
          "error should be SchemaValidationError")
      Expect(err.Error()).To(ContainSubstring("sha256"),
          "error should mention sha256 requirement")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not validate by checking only for `@` presence -- the algorithm matters
  - Do not accept `sha512`, `blake2b`, etc. -- only `sha256` is allowed per OCI convention

---

### UT-DS-017-018: Reject short sha256 digest (not 64 hex chars)

- **BR**: BR-WORKFLOW-004 (Schema Format), Issue #89 (Digest integrity)
- **Business Outcome**: DataStorage rejects schemas where the sha256 digest is truncated, preventing accidental use of abbreviated digests (e.g., from Docker CLI short IDs) that could collide or be ambiguous in a production registry
- **Preconditions**:
  - `schema.NewParser()` instantiated
  - Base schema YAML satisfies all non-execution BR-WORKFLOW-004 requirements
- **Given**:
  - workflow-schema.yaml with truncated sha256 digest (6 chars instead of 64):
    ```yaml
    execution:
      engine: tekton
      bundle: quay.io/kubernaut/test@sha256:abc123
    ```
- **When**: `parser.ParseAndValidate(shortDigestBundleSchemaYAML)` is called
- **Then**:
  - `err` is not nil -- validation rejects the schema
  - `err` is assignable to `*models.SchemaValidationError` -- typed error
  - `err.Error()` contains `"64"` -- tells operator the expected digest length
- **Exit Criteria**:
  - Error type is `*models.SchemaValidationError`
  - The algorithm `sha256` passes but the hex portion fails length validation
  - Error message includes `"64"` to indicate the expected number of hex characters
- **Implementation Hint**:
  ```go
  It("UT-DS-017-018: should reject short sha256 digest (not 64 hex chars)", func() {
      _, err := parser.ParseAndValidate(shortDigestBundleSchemaYAML)
      Expect(err).To(HaveOccurred(), "short digest must be rejected")

      var schemaErr *models.SchemaValidationError
      Expect(err).To(BeAssignableToTypeOf(schemaErr),
          "error should be SchemaValidationError")
      Expect(err.Error()).To(ContainSubstring("64"),
          "error should mention expected digest length")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not accept any sha256 digest shorter than 64 hex characters -- Docker short IDs (12 chars) are not valid
  - Do not accept non-hex characters (e.g., `sha256:xyz...`) -- validate `[0-9a-f]` only

---

## 2. Unit Tests -- Handler Validation (Go)

**Location**: `test/unit/datastorage/execution_bundle_validation_test.go`
**Framework**: Ginkgo/Gomega BDD
**SUT**: `server.Handler.HandleCreateWorkflow()` (`pkg/datastorage/server/workflow_handlers.go`)
**Dependencies**: `oci.MockImagePuller` (`pkg/datastorage/oci/mock_puller.go`), `oci.SchemaExtractor`, `schema.Parser`
**Error format**: RFC 7807 Problem Details JSON (`type`, `title`, `status`, `detail`)
**Existing pattern reference**: `test/unit/datastorage/workflow_create_oci_handler_test.go`

**Shared preconditions for all handler validation tests**:
- Handler constructed via `server.NewHandler(nil, server.WithSchemaExtractor(extractor))` -- no DB wired (nil repository)
- `MockImagePuller` configured with specific schema YAML content
- HTTP request uses `POST /api/v1/workflows` with JSON body `{"container_image": "<schema_image>"}`
- Response captured via `httptest.NewRecorder()`

**Helper functions** (defined in test `Context`):
```go
newHandlerWithMockExtractor := func(puller oci.ImagePuller) *server.Handler {
    parser := schema.NewParser()
    extractor := oci.NewSchemaExtractor(puller, parser)
    return server.NewHandler(nil, server.WithSchemaExtractor(extractor))
}

makeCreateRequest := func(schemaImage string) *http.Request {
    body := map[string]string{"container_image": schemaImage}
    jsonBody, _ := json.Marshal(body)
    return httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
}
```

---

### UT-WF-017-010: Accept OCI registration with valid digest-pinned bundle

- **BR**: BR-WORKFLOW-004 (Schema Format), DD-WORKFLOW-017 (OCI Registration Flow)
- **Business Outcome**: The full OCI registration pipeline (pull image -> extract schema -> parse -> validate) passes through to the DB insertion stage when the schema contains a valid digest-pinned `execution.bundle`, confirming that validation does not false-positive on valid bundles
- **Preconditions**:
  - `MockImagePuller` configured with `validDigestOnlyBundleSchemaYAML` (digest-only bundle)
  - Handler wired with mock extractor, no DB (nil repository)
- **Given**:
  - Mock OCI puller returns a schema image containing:
    ```yaml
    execution:
      engine: tekton
      bundle: quay.io/kubernaut/workflows/scale-memory-bundle@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890
    ```
  - HTTP request: `POST /api/v1/workflows` with body `{"container_image": "quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0"}`
- **When**: `handler.HandleCreateWorkflow(rr, req)` is called
- **Then**:
  - `rr.Code` is NOT `400` (Bad Request) -- validation passed
  - `rr.Code` is NOT `422` (Unprocessable Entity) -- schema was found in image
  - `rr.Code` is NOT `502` (Bad Gateway) -- mock puller did not fail
  - Handler proceeds past validation to DB insertion stage (may fail with 500 due to nil repository -- expected)
- **Exit Criteria**:
  - The handler does not reject valid digest-pinned bundles at the validation stage
  - The OCI pull -> extract -> parse -> validate pipeline completes without error
  - Any failure after validation (e.g., nil DB) is a different error class, not a validation rejection
- **Implementation Hint**:
  ```go
  It("UT-WF-017-010: should accept OCI registration with valid digest-pinned bundle", func() {
      puller := oci.NewMockImagePuller(validDigestOnlyBundleSchemaYAML)
      handler := newHandlerWithMockExtractor(puller)
      req := makeCreateRequest("quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0")
      rr := httptest.NewRecorder()

      handler.HandleCreateWorkflow(rr, req)

      Expect(rr.Code).ToNot(Equal(http.StatusBadRequest),
          "valid digest-pinned bundle should not be rejected as bad request")
      Expect(rr.Code).ToNot(Equal(http.StatusUnprocessableEntity),
          "valid schema should not be reported as missing")
      Expect(rr.Code).ToNot(Equal(http.StatusBadGateway),
          "mock puller should not cause image pull failure")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not assert `rr.Code == 201` -- there is no DB wired, so 201 is not possible; the test validates the validation stage, not the full flow
  - Do not skip this positive test -- it guards against false-positive rejections in the validation logic

---

### UT-WF-017-011: Reject OCI registration when bundle has tag-only reference

- **BR**: BR-WORKFLOW-004 (Schema Format), Issue #89 (Digest-only enforcement), DD-WORKFLOW-017 (Error handling)
- **Business Outcome**: The OCI registration pipeline rejects schemas with tag-only `execution.bundle` at the HTTP handler level, returning an RFC 7807 Problem Details response that operators can use to diagnose and fix the schema before re-submitting
- **Preconditions**:
  - `MockImagePuller` configured with `tagOnlyBundleSchemaYAML` (tag-only, no digest)
  - Handler wired with mock extractor, no DB (nil repository)
- **Given**:
  - Mock OCI puller returns a schema image containing:
    ```yaml
    execution:
      engine: tekton
      bundle: quay.io/kubernaut/workflows/scale-memory-bundle:v1.0.0
    ```
  - HTTP request: `POST /api/v1/workflows` with body `{"container_image": "quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0"}`
- **When**: `handler.HandleCreateWorkflow(rr, req)` is called
- **Then**:
  - `rr.Code` is `400` (Bad Request) -- schema validation failed
  - Response body is RFC 7807 JSON with:
    - `"type"`: `"https://kubernaut.ai/problems/validation-error"`
    - `"detail"`: contains `"execution.bundle"` -- identifies the offending field
- **Exit Criteria**:
  - HTTP 400 is returned (not 422 or 500)
  - RFC 7807 `type` field matches the validation-error problem type URI
  - `detail` field contains enough information for the operator to identify and fix the issue
  - The handler does NOT proceed to DB insertion
- **Implementation Hint**:
  ```go
  It("UT-WF-017-011: should reject OCI registration when bundle has tag-only reference", func() {
      puller := oci.NewMockImagePuller(tagOnlyBundleSchemaYAML)
      handler := newHandlerWithMockExtractor(puller)
      req := makeCreateRequest("quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0")
      rr := httptest.NewRecorder()

      handler.HandleCreateWorkflow(rr, req)

      Expect(rr.Code).To(Equal(http.StatusBadRequest),
          "tag-only execution.bundle must be rejected with 400")

      var problem map[string]interface{}
      Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
      Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"),
          "error type should be validation-error")
      Expect(problem["detail"]).To(ContainSubstring("execution.bundle"),
          "error detail should reference execution.bundle")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not assert on `rr.Code != 200` alone -- must verify it is specifically `400` with correct RFC 7807 body
  - Do not use `Expect(rr.Body.String()).To(ContainSubstring(...))` on raw body -- parse as JSON first to validate structure

---

### UT-WF-017-012: Reject OCI registration when execution section is missing

- **BR**: BR-WORKFLOW-004 (Schema Format), Issue #89 (Mandatory execution section), DD-WORKFLOW-017 (Error handling)
- **Business Outcome**: The OCI registration pipeline rejects schemas that omit the `execution` section entirely, returning an RFC 7807 Problem Details response. This prevents registration of workflow schemas that cannot be executed (no engine, no bundle)
- **Preconditions**:
  - `MockImagePuller` configured with `noExecutionSectionSchemaYAML` (base schema only, no execution section)
  - Handler wired with mock extractor, no DB (nil repository)
- **Given**:
  - Mock OCI puller returns a schema image containing only base fields (metadata, actionType, labels, parameters) with NO `execution:` section
  - HTTP request: `POST /api/v1/workflows` with body `{"container_image": "quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0"}`
- **When**: `handler.HandleCreateWorkflow(rr, req)` is called
- **Then**:
  - `rr.Code` is `400` (Bad Request) -- schema validation failed
  - Response body is RFC 7807 JSON with:
    - `"type"`: `"https://kubernaut.ai/problems/validation-error"`
    - `"detail"`: contains `"execution"` -- identifies the missing section
- **Exit Criteria**:
  - HTTP 400 is returned (not 422 or 500)
  - RFC 7807 `type` field matches the validation-error problem type URI
  - `detail` field references `"execution"` (the section), guiding the operator to add the entire section
  - No nil pointer panic occurs when the `Execution` field is nil in the parsed schema
- **Implementation Hint**:
  ```go
  It("UT-WF-017-012: should reject OCI registration when execution section is missing", func() {
      puller := oci.NewMockImagePuller(noExecutionSectionSchemaYAML)
      handler := newHandlerWithMockExtractor(puller)
      req := makeCreateRequest("quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0")
      rr := httptest.NewRecorder()

      handler.HandleCreateWorkflow(rr, req)

      Expect(rr.Code).To(Equal(http.StatusBadRequest),
          "missing execution section must be rejected with 400")

      var problem map[string]interface{}
      Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
      Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/validation-error"),
          "error type should be validation-error")
      Expect(problem["detail"]).To(ContainSubstring("execution"),
          "error detail should reference execution section")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not confuse "missing execution section" (UT-WF-017-012) with "execution section present but no bundle" (UT-DS-017-016) -- they are different failure modes
  - Do not let the handler panic on nil `Execution` -- the `SchemaValidationError` from the parser should be caught and translated to RFC 7807 before any field access

---

### UT-WF-017-013: Reject OCI registration when execution.bundle image does not exist in registry

- **BR**: BR-WORKFLOW-017-001 (Validation #10), DD-WORKFLOW-017 (Error handling), Issue #89
- **Business Outcome**: The OCI registration pipeline provides early feedback when `execution.bundle` references an image that does not exist in the container registry, preventing workflows from being registered that will fail at execution time with `ImagePullBackOff`
- **Preconditions**:
  - `MockImagePullerWithFailingExists` configured: `Pull()` succeeds (schema extraction works), `Exists()` returns error (bundle image not found)
  - Handler wired with mock extractor, no DB (nil repository)
- **Given**:
  - Mock OCI puller returns a valid schema image (all fields pass validation including `execution.bundle` format)
  - `puller.Exists()` returns an error simulating a missing image in the registry
  - HTTP request: `POST /api/v1/workflows` with body `{"schemaImage": "quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0"}`
- **When**: `handler.HandleCreateWorkflow(rr, req)` is called
- **Then**:
  - `rr.Code` is `400` (Bad Request) -- bundle existence check failed
  - Response body is RFC 7807 JSON with:
    - `"type"`: `"https://kubernaut.ai/problems/bundle-not-found"`
    - `"detail"`: contains `"execution.bundle"` -- identifies the problem as bundle-related
  - `Content-Type` header is `application/problem+json`
- **Exit Criteria**:
  - HTTP 400 is returned (not 500 or 502)
  - RFC 7807 `type` field is `bundle-not-found` (distinct from `validation-error`)
  - `detail` field contains enough information for the operator to identify the unreachable bundle
  - The handler does NOT proceed to DB insertion
- **Mock Requirement**:
  - New mock type `MockImagePullerWithFailingExists` that extends `MockImagePuller` (Pull succeeds) but overrides `Exists()` to return an error. This mock is required because the existing `MockImagePuller.Exists()` always returns nil and `FailingMockImagePuller.Pull()` always fails (preventing schema extraction from succeeding).
- **Implementation Hint**:
  ```go
  It("UT-WF-017-013: should reject OCI registration when execution.bundle image does not exist in registry", func() {
      puller := oci.NewMockImagePullerWithFailingExists(
          validDigestOnlyBundleSchemaYAML,
          fmt.Errorf("MANIFEST_UNKNOWN: manifest unknown"),
      )
      handler := newHandlerWithMockExtractor(puller)
      req := makeCreateRequest("quay.io/kubernaut/schemas/exec-bundle-test:v1.0.0")
      rr := httptest.NewRecorder()

      handler.HandleCreateWorkflow(rr, req)

      Expect(rr.Code).To(Equal(http.StatusBadRequest),
          "non-existent execution.bundle must be rejected with 400")
      Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

      var problem map[string]interface{}
      Expect(json.Unmarshal(rr.Body.Bytes(), &problem)).To(Succeed())
      Expect(problem["type"]).To(Equal("https://kubernaut.ai/problems/bundle-not-found"),
          "RFC 7807 type must be bundle-not-found")
      Expect(problem["detail"]).To(ContainSubstring("execution.bundle"),
          "RFC 7807 detail must reference the bundle")
  })
  ```
- **Anti-Pattern Warnings**:
  - Do not use `FailingMockImagePuller` -- it fails on `Pull()` too, preventing schema extraction from succeeding
  - Do not confuse this with `validation-error` (format validation) -- this is a `bundle-not-found` (existence check)
  - Do not skip RFC 7807 body assertions -- the error type distinguishes this from other 400 errors

---

## Test Scenario ID Summary

| ID | Description | Type | TDD Phase | BR/DD Reference |
|----|-------------|------|-----------|-----------------|
| UT-DS-017-011 | Valid digest-only bundle accepted | Positive | RED (passes) | BR-WORKFLOW-004, DD-WORKFLOW-017 |
| UT-DS-017-012 | Valid tag+digest bundle accepted | Positive | RED (passes) | BR-WORKFLOW-004, DD-WORKFLOW-017 |
| UT-DS-017-013 | Tag-only bundle rejected | Negative | RED (fails) | BR-WORKFLOW-004, Issue #89 |
| UT-DS-017-014 | Missing execution section rejected | Negative | RED (fails) | BR-WORKFLOW-004, Issue #89 |
| UT-DS-017-015 | Empty bundle string rejected | Negative | RED (fails) | BR-WORKFLOW-004, Issue #89 |
| UT-DS-017-016 | Execution without bundle rejected | Negative | RED (fails) | BR-WORKFLOW-004, Issue #89 |
| UT-DS-017-017 | Non-sha256 digest rejected | Negative | RED (fails) | BR-WORKFLOW-004, Issue #89 |
| UT-DS-017-018 | Short digest rejected | Negative | RED (fails) | BR-WORKFLOW-004, Issue #89 |
| UT-WF-017-010 | Valid registration accepted | Positive | RED (passes) | BR-WORKFLOW-004, DD-WORKFLOW-017 |
| UT-WF-017-011 | Tag-only bundle registration rejected | Negative | RED (fails) | BR-WORKFLOW-004, Issue #89 |
| UT-WF-017-012 | Missing execution registration rejected | Negative | RED (fails) | BR-WORKFLOW-004, Issue #89 |
| UT-WF-017-013 | Bundle image not found in registry | Negative | RED (fails) | BR-WORKFLOW-017-001 #10, Issue #89 |

---

## References

- [DD-WORKFLOW-017](../../architecture/decisions/DD-WORKFLOW-017-workflow-lifecycle-component-interactions.md) -- Workflow lifecycle and OCI registration flow
- [BR-WORKFLOW-004](../../requirements/) -- Workflow Schema Format Specification
- [Issue #89](https://github.com/jordigilh/kubernaut/issues/89) -- Enforce digest-only execution.bundle
- [Test Plan Template](../../development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)
- `SchemaValidationError` type: [`pkg/datastorage/models/workflow_schema.go`](../../../pkg/datastorage/models/workflow_schema.go) (lines 239-255)
- `MockImagePuller`: [`pkg/datastorage/oci/mock_puller.go`](../../../pkg/datastorage/oci/mock_puller.go)
