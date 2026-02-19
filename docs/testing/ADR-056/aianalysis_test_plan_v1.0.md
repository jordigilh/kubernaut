# AIAnalysis Test Plan for ADR-056

**Version**: 1.0
**Created**: 2026-02-17
**Status**: Active
**Authority**: [ADR-056](../../architecture/decisions/ADR-056-post-rca-label-computation.md), [DD-HAPI-018](../../architecture/decisions/DD-HAPI-018-detected-labels-detection-specification.md)
**BRs**: BR-AI-056, BR-AI-013, BR-AI-082

---

## Overview

This test plan covers the AIAnalysis service (Go, CRD Controller) implementation of ADR-056: consuming `detected_labels` from HAPI responses and using them for Rego policy evaluation.

### Scope

- **Cycle 3.1**: `PostRCAContext` CRD type -- JSON serialization contract, omitempty behavior
- **Cycle 3.2**: `ResponseProcessor` -- extraction of `detected_labels` from HAPI response into `PostRCAContext`
- **Cycle 3.3**: `AnalyzingHandler` -- reading `PostRCAContext.DetectedLabels` for Rego policy input
- **Phase 4 / Cycle 4.1**: Remove `DetectedLabels` + `OwnerChain` from `EnrichmentResults`
- **Phase 4 / Cycle 4.2**: Remove old propagation paths

### Out of Scope

- HAPI Python-side label detection and session_state (see [hapi_test_plan_v1.0.md](hapi_test_plan_v1.0.md))
- Rego policy logic itself (existing tests, unchanged)
- CEL immutability rules (validated at CRD manifest level, not unit-testable)

---

## Anti-Pattern Compliance

Per [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md):

- All tests use **Ginkgo/Gomega BDD** framework (no standard `testing.T`)
- No `Skip()` / `XIt` -- tests fail or pass
- No struct-shape tests -- compiler validates field existence; tests validate observable behavior (JSON serialization, Rego input content, CRD status updates)
- No backward-compatibility tests -- no release has been made; `PostRCAContext` is the sole source for `DetectedLabels`
- Mock ONLY external dependencies (HAPI client response) -- real `ResponseProcessor`, `AnalyzingHandler`, and `sharedtypes` structs

---

## Code Surface by Tier

| Tier | Files | LOC | Coverage | Target |
|------|-------|-----|----------|--------|
| Unit | `analyzing.go` (~40 lines: `resolveDetectedLabels`, `detectedLabelsToMap`), `response_processor.go` (~30 lines: `populatePostRCAContext`, `extractDetectedLabels`), `aianalysis_types.go` (~15 lines: `PostRCAContext` struct), `enrichment.go` (field removal) | ~85 | 18 tests (all active) | >= 80% |
| Integration | `post_rca_context_integration_test.go` | ~150 | 6 tests | >= 80% Active |
| E2E | `09_detected_labels_e2e_test.go` | ~120 | 3 tests | >= 80% Active |

---

## Test Scenario Naming Convention

**Format**: `UT-AA-056-{SEQ}` (zero-padded 3-digit sequence)

**Usage in Ginkgo**:
```go
Context("UT-AA-056-001: PostRCAContext JSON serialization", func() {
    It("should serialize to correct camelCase JSON", func() {
        // test body
    })
})
```

---

## BR Coverage Matrix

| BR ID | Description | Test IDs | Count |
|-------|-------------|----------|-------|
| BR-AI-056 | PostRCAContext in AIAnalysis CRD | UT-AA-056-001 to 012 | 12 |
| BR-AI-056 | EnrichmentResults cleanup (Phase 4) | UT-AA-056-013 to 017, 019 | 6 |
| BR-AI-013 | Rego policy evaluation with DetectedLabels | UT-AA-056-009 to 012 | 4 |
| BR-AI-082 | SetAt immutability timestamp | UT-AA-056-005 | 1 |

---

## Cycle 3.1: PostRCAContext CRD Type (2 tests)

**File**: `test/unit/aianalysis/post_rca_context_test.go`
**Production code**: `api/aianalysis/v1alpha1/aianalysis_types.go`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-AA-056-001 | JSON camelCase serialization | P1 | API consumers receive correct field names |
| UT-AA-056-002 | omitempty contract | P1 | Nil PostRCAContext omitted from JSON (clean API) |

---

### UT-AA-056-001: PostRCAContext JSON camelCase serialization

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: Kubernetes API server and clients expect camelCase JSON. Incorrect serialization breaks CRD storage and client parsing.

**Given**: A fully populated `PostRCAContext` with `DetectedLabels` (all 8 fields) and `SetAt` timestamp
**When**: `json.Marshal()` is called on the enclosing `AIAnalysisStatus`
**Then**: JSON output contains camelCase keys: `postRCAContext`, `detectedLabels`, `setAt`, `gitOpsManaged`, `pdbProtected`, `hpaEnabled`, `stateful`, `helmManaged`, `networkIsolated`, `serviceMesh`, `gitOpsTool`, `failedDetections`

**Acceptance Criteria**:
- All struct tags produce correct camelCase in JSON output
- No snake_case keys in serialized output

---

### UT-AA-056-002: PostRCAContext omitempty contract

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: Nil PostRCAContext is omitted from JSON, keeping the API response clean when no labels are available.

**Given**: An `AIAnalysisStatus` with `PostRCAContext` set to `nil`
**When**: `json.Marshal()` is called
**Then**: JSON output does NOT contain `postRCAContext` key

**Acceptance Criteria**:
- `omitempty` tag works correctly for pointer-to-struct field

---

## Cycle 3.2: ResponseProcessor PostRCA Extraction (6 tests)

**File**: `test/unit/aianalysis/response_processor_post_rca_test.go`
**Production code**: `pkg/aianalysis/handlers/response_processor.go`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-AA-056-003 | Incident response -> PostRCAContext | P0 | Labels from HAPI stored in CRD status |
| UT-AA-056-004 | Recovery response -> PostRCAContext | P0 | Same for recovery flow |
| UT-AA-056-005 | SetAt timestamp set | P1 | Immutability guard timestamp recorded |
| UT-AA-056-006 | No labels -> PostRCAContext nil | P1 | Clean status when HAPI has no labels |
| UT-AA-056-007 | failedDetections propagated | P1 | Partial detection failures visible in CRD |
| UT-AA-056-008 | Malformed detected_labels | P1 | No panic on bad data from HAPI |

---

### UT-AA-056-003: ProcessIncidentResponse populates PostRCAContext

**Priority**: P0 | **BR**: BR-AI-056
**Business Value**: This is the core ADR-056 contract on the Go side -- HAPI's runtime-computed labels are persisted in the AIAnalysis CRD status for Rego evaluation.

**Given**: An HAPI incident response containing `detected_labels` with `gitOpsManaged=true`, `pdbProtected=true`
**When**: `ResponseProcessor.ProcessIncidentResponse()` is called
**Then**:
- `analysis.Status.PostRCAContext` is not nil
- `analysis.Status.PostRCAContext.DetectedLabels.GitOpsManaged` is `true`
- `analysis.Status.PostRCAContext.DetectedLabels.PdbProtected` is `true`

**Acceptance Criteria**:
- All 8 DetectedLabels fields correctly deserialized from unstructured JSON
- PostRCAContext created with populated DetectedLabels

---

### UT-AA-056-004: ProcessRecoveryResponse populates PostRCAContext

**Priority**: P0 | **BR**: BR-AI-056
**Business Value**: Recovery flow uses the same PostRCAContext population path.

Same contract as UT-AA-056-003 but for `ProcessRecoveryResponse`.

---

### UT-AA-056-005: SetAt timestamp set when detected_labels present

**Priority**: P1 | **BR**: BR-AI-082
**Business Value**: `SetAt` timestamp serves as an immutability guard -- once set, CEL rules prevent PostRCAContext mutation.

**Given**: HAPI response with `detected_labels`
**When**: `ResponseProcessor.ProcessIncidentResponse()` is called
**Then**: `analysis.Status.PostRCAContext.SetAt` is set to a non-zero time close to `time.Now()`

**Acceptance Criteria**:
- `SetAt` is within 5 seconds of current time
- `SetAt` is not zero value

---

### UT-AA-056-006: PostRCAContext nil when detected_labels absent

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: Clean CRD status when HAPI response has no labels (e.g., detection not configured).

**Given**: HAPI response WITHOUT `detected_labels` field
**When**: `ResponseProcessor.ProcessIncidentResponse()` is called
**Then**: `analysis.Status.PostRCAContext` remains `nil`

**Acceptance Criteria**:
- No PostRCAContext created when no labels in response
- No error returned

---

### UT-AA-056-007: failedDetections propagated to PostRCAContext

**Priority**: P1 | **BR**: BR-SP-103
**Business Value**: Partial detection failures (e.g., RBAC) are visible in the CRD for observability and Rego evaluation.

**Given**: HAPI response with `detected_labels` including `failedDetections: ["pdbProtected", "hpaEnabled"]`
**When**: `ResponseProcessor.ProcessIncidentResponse()` is called
**Then**: `analysis.Status.PostRCAContext.DetectedLabels.FailedDetections` contains `["pdbProtected", "hpaEnabled"]`

**Acceptance Criteria**:
- failedDetections array preserved in PostRCAContext

---

### UT-AA-056-008: Malformed detected_labels handled gracefully (NEW)

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: Bad data from HAPI (e.g., `detected_labels: "not_a_dict"`) does not panic the controller.

**Given**: HAPI response with `detected_labels` set to a non-object value (e.g., string or number)
**When**: `ResponseProcessor.ProcessIncidentResponse()` is called
**Then**:
- No panic
- `analysis.Status.PostRCAContext` remains `nil` (or error logged gracefully)

**Acceptance Criteria**:
- Controller does not crash on malformed HAPI data
- Error is logged, not propagated as fatal

---

## Cycle 3.3: AnalyzingHandler Rego Integration (4 tests)

**File**: `test/unit/aianalysis/analyzing_handler_post_rca_test.go`
**Production code**: `pkg/aianalysis/handlers/analyzing.go`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-AA-056-009 | PostRCAContext labels in Rego input | P0 | Rego policies evaluate against runtime labels |
| UT-AA-056-010 | FailedDetections in Rego input | P1 | Rego can gate on detection failures |
| UT-AA-056-011 | DetectedLabels read exclusively from PostRCAContext | P1 | Single source of truth for Rego (`EnrichmentResults.DetectedLabels` **ADR-056: removed**) |
| UT-AA-056-012 | Nil DetectedLabels -> empty Rego map | P1 | Rego evaluates safely with empty labels |

---

### UT-AA-056-009: DetectedLabels from PostRCAContext populate Rego input

**Priority**: P0 | **BR**: BR-AI-013
**Business Value**: Rego approval policies (e.g., `approval.rego` checking `detected_labels["stateful"]`) evaluate against HAPI's runtime-computed labels, not stale SP-time labels.

**Given**: An `AIAnalysis` with `Status.PostRCAContext.DetectedLabels` populated (`gitOpsManaged=true`, `stateful=true`)
**When**: `AnalyzingHandler.buildPolicyInput()` constructs Rego input
**Then**: Rego input map contains `detected_labels["git_ops_managed"] = true` and `detected_labels["stateful"] = true`

**Acceptance Criteria**:
- Go struct field names (PascalCase) converted to Rego map keys (snake_case)
- Boolean values preserved correctly in Rego input

---

### UT-AA-056-010: FailedDetections from PostRCAContext populate Rego input

**Priority**: P1 | **BR**: BR-AI-013
**Business Value**: Rego policies can check `failed_detections` to decide whether to require human review when detection is incomplete.

**Given**: `PostRCAContext.DetectedLabels.FailedDetections` = `["pdbProtected", "hpaEnabled"]`
**When**: `buildPolicyInput()` constructs Rego input
**Then**: Rego input contains `detected_labels["failed_detections"] = ["pdbProtected", "hpaEnabled"]`

**Acceptance Criteria**:
- failedDetections array passed through to Rego input

---

### UT-AA-056-011: DetectedLabels read exclusively from PostRCAContext

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: After ADR-056, PostRCAContext is the sole source of DetectedLabels for Rego evaluation. `EnrichmentResults.DetectedLabels` (**ADR-056: removed, now in PostRCAContext**) has been removed. This test confirms the single source of truth.

**Given**: An `AIAnalysis` with `Status.PostRCAContext.DetectedLabels` populated
**When**: `resolveDetectedLabels()` is called
**Then**: Returns `PostRCAContext.DetectedLabels`

**Acceptance Criteria**:
- PostRCAContext.DetectedLabels returned directly
- No fallback to EnrichmentResults (field removed in Phase 4)
- Also covers intent of former UT-AA-056-018 (merged -- AnalyzingHandler reads only PostRCAContext)

---

### UT-AA-056-012: Nil DetectedLabels produces empty Rego labels map (NEW)

**Priority**: P1 | **BR**: BR-AI-013
**Business Value**: When PostRCAContext is nil (no labels computed), Rego policies evaluate with an empty labels map, resulting in safe default behavior (no label-gated approvals).

**Given**: An `AIAnalysis` with `Status.PostRCAContext` = `nil`
**When**: `detectedLabelsToMap(nil)` is called
**Then**: Returns an empty `map[string]interface{}`

**Acceptance Criteria**:
- No panic on nil input
- Empty map (not nil map) returned for safe Rego evaluation

---

## Phase 4 / Cycle 4.1: Remove DetectedLabels + OwnerChain from EnrichmentResults (4 tests)

**Status**: Active
**File**: `test/unit/aianalysis/enrichment_cleanup_test.go`
**Production code**: `pkg/shared/types/enrichment.go`, `pkg/aianalysis/handlers/request_builder.go`

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-AA-056-013 | EnrichmentResults without DetectedLabels | P1 | Clean type after field removal |
| UT-AA-056-014 | EnrichmentResults without OwnerChain | P1 | Clean type after field removal |
| UT-AA-056-015 | RequestBuilder maps correctly | P1 | HAPI request valid after removal |
| UT-AA-056-016 | HAPI request validates | P1 | No regression in request construction |

---

### UT-AA-056-013: EnrichmentResults constructs with only KubernetesContext and CustomLabels

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: Confirms `DetectedLabels` and `OwnerChain` fields have been cleanly removed from the shared type.

**Given**: `EnrichmentResults` constructed with `KubernetesContext` and `CustomLabels` only
**When**: The instance is created and fields are accessed
**Then**: `KubernetesContext` and `CustomLabels` are populated correctly

**Acceptance Criteria**:
- Build succeeds after field removal
- Remaining fields function correctly

---

### UT-AA-056-014: EnrichmentResults JSON without detectedLabels or ownerChain

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: JSON serialization confirms removed fields do not appear in API contracts.

**Given**: An `EnrichmentResults` with `KubernetesContext` and `CustomLabels` populated
**When**: `json.Marshal()` is called
**Then**: JSON output does NOT contain `detectedLabels` or `ownerChain` keys

**Acceptance Criteria**:
- No `detectedLabels` key in serialized JSON
- No `ownerChain` key in serialized JSON
- `kubernetesContext` and `customLabels` keys present

---

### UT-AA-056-015: RequestBuilder produces valid HAPI request without DetectedLabels

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: HAPI request construction works without the removed fields; `DetectedLabels` is NOT populated in the request (HAPI computes them post-RCA).

**Given**: An `AnalysisRequest` with `EnrichmentResults` containing only `CustomLabels`
**When**: `RequestBuilder.BuildIncidentRequest()` is called
**Then**: HAPI request has `EnrichmentResults.Set = true`, `CustomLabels.Set = true`, `DetectedLabels.Set = false`

**Acceptance Criteria**:
- No nil pointer errors
- CustomLabels forwarded to HAPI
- DetectedLabels NOT set in request (ADR-056)

---

### UT-AA-056-016: Both incident and recovery requests build without removed fields

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: Both request paths (incident and recovery) work correctly after field removal.

**Given**: An `AnalysisRequest` with `EnrichmentResults` containing only `CustomLabels`
**When**: `BuildIncidentRequest()` and `BuildRecoveryRequest()` are called
**Then**: Both return valid requests; neither contains `DetectedLabels` or `OwnerChain`

**Acceptance Criteria**:
- Incident request: `DetectedLabels.Set = false`, `OwnerChain.Set = false`
- Recovery request: builds without error

---

## Phase 4 / Cycle 4.2: Remove Old Propagation Paths (2 tests)

**Status**: Active
**File**: `test/unit/aianalysis/enrichment_cleanup_test.go`
**Production code**: `pkg/aianalysis/handlers/request_builder.go`, `pkg/aianalysis/handlers/analyzing.go`

**Note**: UT-AA-056-018 was merged into UT-AA-056-011 (both test that `resolveDetectedLabels` reads exclusively from PostRCAContext). With `EnrichmentResults.DetectedLabels` (**ADR-056: removed**) removed in Phase 4, these are identical assertions.

### Summary

| ID | Scenario | Priority | Business Outcome |
|----|----------|----------|------------------|
| UT-AA-056-017 | Request built without old labels | P1 | No stale labels in HAPI request |
| ~~UT-AA-056-018~~ | ~~AnalyzingHandler reads only PostRCAContext~~ | - | Merged into UT-AA-056-011 |
| UT-AA-056-019 | EnrichmentResults JSON contains only authorized fields | P1 | Dead code eliminated |

---

### UT-AA-056-017: HAPI request does not contain DetectedLabels from EnrichmentResults

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: Confirms no stale labels are sent in the HAPI request after removing `DetectedLabels` from `EnrichmentResults`.

**Given**: An `AnalysisRequest` with `EnrichmentResults` containing `CustomLabels` only
**When**: `RequestBuilder.BuildIncidentRequest()` is called
**Then**: `DetectedLabels.Set = false` and `OwnerChain.Set = false` in the HAPI request

**Acceptance Criteria**:
- DetectedLabels not populated from EnrichmentResults (**ADR-056: removed**)
- OwnerChain not populated in request (**ADR-055: removed**)

---

### UT-AA-056-019: EnrichmentResults JSON schema contains only authorized fields

**Priority**: P1 | **BR**: BR-AI-056
**Business Value**: Structural verification that only `kubernetesContext` and `customLabels` keys exist in serialized `EnrichmentResults`.

**Given**: An `EnrichmentResults` with both fields populated
**When**: `json.Marshal()` is called and the result is deserialized to a raw map
**Then**: Only `kubernetesContext` and `customLabels` keys exist in the map

**Acceptance Criteria**:
- No unexpected fields in JSON output
- Catches any accidental field additions

---

## ID Cross-Reference Table

| New ID | Old ID | File | Cycle | Status |
|--------|--------|------|-------|--------|
| UT-AA-056-001 | UT-PRC-003 | post_rca_context_test.go | 3.1 | Active |
| UT-AA-056-002 | UT-PRC-004 | post_rca_context_test.go | 3.1 | Active |
| UT-AA-056-003 | UT-RP-PRC-001 | response_processor_post_rca_test.go | 3.2 | Active |
| UT-AA-056-004 | UT-RP-PRC-002 | response_processor_post_rca_test.go | 3.2 | Active |
| UT-AA-056-005 | UT-RP-PRC-003 | response_processor_post_rca_test.go | 3.2 | Active |
| UT-AA-056-006 | UT-RP-PRC-004 | response_processor_post_rca_test.go | 3.2 | Active |
| UT-AA-056-007 | UT-RP-PRC-005 | response_processor_post_rca_test.go | 3.2 | Active |
| UT-AA-056-008 | NEW (Phase 3.7) | response_processor_post_rca_test.go | 3.2 | Active |
| UT-AA-056-009 | UT-AH-PRC-001 | analyzing_handler_post_rca_test.go | 3.3 | Active |
| UT-AA-056-010 | UT-AH-PRC-002 | analyzing_handler_post_rca_test.go | 3.3 | Active |
| UT-AA-056-011 | UT-AH-PRC-004 | analyzing_handler_post_rca_test.go | 3.3 | Active (updated Phase 4; absorbs UT-AA-056-018) |
| UT-AA-056-012 | NEW (Phase 3.7) | analyzing_handler_post_rca_test.go | 3.3 | Active |
| UT-AA-056-013 | NEW (Phase 4) | enrichment_cleanup_test.go | 4.1 | Active |
| UT-AA-056-014 | NEW (Phase 4) | enrichment_cleanup_test.go | 4.1 | Active |
| UT-AA-056-015 | NEW (Phase 4) | enrichment_cleanup_test.go | 4.1 | Active |
| UT-AA-056-016 | NEW (Phase 4) | enrichment_cleanup_test.go | 4.1 | Active |
| UT-AA-056-017 | NEW (Phase 4) | enrichment_cleanup_test.go | 4.2 | Active |
| ~~UT-AA-056-018~~ | - | - | 4.2 | Merged into UT-AA-056-011 |
| UT-AA-056-019 | NEW (Phase 4) | enrichment_cleanup_test.go | 4.2 | Active |

---

## Integration Tests: PostRCAContext CRD Reconciliation (6 tests)

**File**: `test/integration/aianalysis/post_rca_context_integration_test.go`
**Production code**: `pkg/aianalysis/handlers/response_processor.go`, `pkg/aianalysis/handlers/analyzing.go`

### Infrastructure

- envtest (per-process) + HAPI container + Mock LLM (3-step) + DataStorage
- AIAnalysis CRD creation → controller reconciliation → status verification
- Real HAPI calls with Mock LLM backend

### Summary

| ID | Scenario (BDD) | BR | Priority |
|----|----------------|-------|----------|
| IT-AA-056-001 | Given AIAnalysis CR with CrashLoopBackOff, When controller completes reconciliation through HAPI, Then PostRCAContext is handled correctly | ADR-056, BR-AI-056 | P0 |
| IT-AA-056-002 | Given recovery AIAnalysis CR, When reconciliation completes, Then PostRCAContext handling follows same contract as incident | ADR-056, BR-AI-056 | P0 |
| IT-AA-056-003 | Given PostRCAContext populated, When Analyzing phase runs Rego evaluation, Then Rego input contains detected_labels map | ADR-056, BR-AI-013 | P0 |
| IT-AA-056-004 | Given PostRCAContext with failedDetections, When Rego evaluation runs, Then failedDetections array appears in Rego input | BR-SP-103, BR-AI-013 | P1 |
| IT-AA-056-005 | Given incident analysis completes, Then PostRCAContext.setAt timestamp is set (immutability guard) | BR-AI-082 | P1 |
| IT-AA-056-006 | Given AIAnalysis CR, Then enrichmentResults in HAPI request does NOT contain detectedLabels or ownerChain | ADR-056 Phase 4 | P1 |

### Notes

PostRCAContext population depends on HAPI's ability to reach K8s resources in the shared envtest.
In integration tests, PostRCAContext may be nil if the shared envtest lacks resources matching the
target namespace. E2E tests (Kind cluster) provide the full label detection path.

---

## E2E Tests: PostRCAContext in Kind Cluster (3 tests)

**File**: `test/e2e/aianalysis/09_detected_labels_e2e_test.go`
**Infrastructure**: Kind cluster with full stack (AA controller, HAPI, Mock LLM, DataStorage)

### Summary

| ID | Scenario (BDD) | BR | Priority |
|----|----------------|-------|----------|
| E2E-AA-056-001 | Given AIAnalysis CR for production CrashLoopBackOff, When full 4-phase reconciliation completes, Then PostRCAContext is populated AND approvalRequired is true | ADR-056, BR-AI-056, BR-AI-013 | P0 |
| E2E-AA-056-002 | Given recovery AIAnalysis CR, When reconciliation completes in Kind, Then PostRCAContext is populated in CRD status | ADR-056 | P0 |
| E2E-AA-056-003 | Given Deployment with PDB in Kind, When incident analysis runs, Then PostRCAContext.detectedLabels.pdbProtected is true | BR-SP-101, ADR-056 | P1 |

---

## Updated Code Surface by Tier

| Tier | Files | Tests | Coverage | Target |
|------|-------|-------|----------|--------|
| Unit | `analyzing.go`, `response_processor.go`, `aianalysis_types.go`, `enrichment.go`, `request_builder.go` | 18 | >= 80% | MET |
| Integration | `post_rca_context_integration_test.go` | 6 | >= 80% | Active |
| E2E | `09_detected_labels_e2e_test.go` | 3 | >= 80% | Active |
