# Test Plan: AIAnalysis CRD Validation for CNV Labels

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1400-v1
**Feature**: Fix AIAnalysis CRD schema validation for CNV boolean fields + extraction data loss
**Version**: 1.0
**Created**: 2026-06-11
**Author**: AI Agent
**Status**: Draft
**Branch**: `feat/structured-decision-payload`

---

## 1. Introduction

### 1.1 Purpose

This test plan validates the fix for two related defects in the AIAnalysis CRD CNV label pipeline:
1. **Schema defect**: CNV boolean fields (`virtualMachine`, `liveMigratable`, `cdiManaged`) are marked `required` in the CRD OpenAPI schema, causing validation failures when not explicitly provided in partial updates.
2. **Data loss defect**: `extractDetectedLabels` in the AA ResponseProcessor does not map the 4 CNV fields that KA sends, silently dropping them before CRD persistence.
3. **Rego gap**: `detectedLabelsToMap` does not include CNV fields in Rego policy input.

### 1.2 Objectives

1. **Schema flexibility**: CNV boolean fields are `+optional` in CRD schema; partial status updates without CNV fields pass validation
2. **Data round-trip**: When KA sends CNV fields in `detected_labels`, they are extracted and persisted to `AIAnalysis.status.postRCAContext.detectedLabels`
3. **Rego completeness**: CNV labels are available to Rego approval policies via `input.detected_labels`
4. **Backward compatibility**: Non-CNV clusters (zero-value CNV fields) continue to work identically
5. **No type change ripple**: Fix uses `// +optional` markers without changing `bool` to `*bool`

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/aianalysis/... -run "UT-AA-1400"` |
| Integration test pass rate | 100% | `go test ./test/integration/aianalysis/... -run "IT-AA-1400"` |
| E2E test pass rate | 100% | `make test-e2e-kubernautagent` (CNV label focus) |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on extraction/map logic |
| CRD schema validation | CNV fields NOT in `required` | grep `kubernaut.ai_aianalyses.yaml` |
| Backward compatibility | 0 regressions | All existing `UT-AA-056-*` tests pass |
| Data integrity | 4 CNV fields round-trip | KA response → CRD → Rego input |

---

## 2. References

### 2.1 Authority

- Issue #1400: AIAnalysis CRD validation for CNV labels
- Issue #1378: CNV label detection (completed — KA-side)
- ADR-056: DetectedLabels in AIAnalysis CRD status (PostRCAContext)
- DD-WORKFLOW-001 v2.3: Detected labels serialization

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- [Wiring Verification](../../.cursor/rules/10-wiring-verification.mdc)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — refactoring validation
- Existing test: `pkg/aianalysis/response_processor_post_rca_test.go` (UT-AA-056-003..007)
- Existing E2E: `test/e2e/kubernautagent/cnv_graceful_skip_e2e_test.go` (E2E-KA-1378-001)

### 2.3 FedRAMP Control Objectives

| Control | NIST Intent | Application to This Feature |
|---------|-------------|----------------------------|
| **AU-3** | Audit records contain sufficient detail | CNV labels must persist to CRD for workflow discovery audit trail |
| **SI-10** | Input validation | CRD schema must accept valid CNV payloads; reject invalid enum values |
| **SI-17** | Fail-safe on error | Missing CNV fields default to `false` (not error); non-CNV clusters unaffected |
| **AC-6** | Least privilege | CNV labels feed Rego approval policies — correct data enables correct authorization |

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | `+optional` marker doesn't remove `required` | CRD validation still fails | Low | UT-AA-1400-006 | Verify generated YAML post `make manifests` |
| R2 | `extractDetectedLabels` key mismatch (camelCase) | Silent data loss | Medium | UT-AA-1400-001 | Use exact same keys as `detectedLabelsToResult` |
| R3 | `detectedLabelsToMap` snake_case mismatch with Rego | Policy evaluation incorrect | Low | UT-AA-1400-004 | Follow existing snake_case convention in function |
| R4 | Existing tests break after `+optional` | Regression | Very Low | All UT-AA-056-* | `+optional` doesn't change Go runtime behavior |

---

## 4. Scope

### 4.1 Features to be Tested

- **CRD schema generation** (`pkg/shared/types/enrichment.go`): `+optional` markers on CNV bool fields
- **CNV extraction** (`pkg/aianalysis/handlers/response_processor.go`): 4 new field mappings in `extractDetectedLabels`
- **Rego input mapping** (`pkg/aianalysis/handlers/analyzing.go`): 4 new fields in `detectedLabelsToMap`
- **CRD regeneration** (`config/crd/bases/kubernaut.ai_aianalyses.yaml`): CNV fields removed from `required`

### 4.2 Features Not to be Tested

- **KA-side CNV detection**: Already tested in #1378 (`internal/kubernautagent/enrichment/`)
- **Workflow discovery/scoring SQL**: Uses separate `workflow_labels.go` path, unaffected
- **ogen-client OptBool**: Already models optional booleans for HTTP API layer

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Keep `bool` type (not `*bool`) | Avoids 20+ file ripple effect; zero-value `false` = "not a VM" is semantically correct |
| `// +optional` without `omitempty` | Field serialized as `false` when present; just removes `required` from schema |
| snake_case in Rego map | Matches existing convention (`git_ops_managed`, `pdb_protected`, etc.) |
| Add `StorageBackend` string to extraction | Complete the 4-field gap; already defined on struct |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (extraction logic, map generation)
- **Integration**: >=80% of integration-testable code (full ResponseProcessor pipeline with CNV fields)
- **E2E**: Covered by existing E2E-KA-1378-001 (validates CNV graceful skip) + new assertion

### 5.2 Pyramid Invariant

> UT proves logic (extraction maps correct fields). IT proves wiring (ResponseProcessor persists to CRD). E2E proves the journey (KA investigation → AA CRD).

### 5.3 Pass/Fail Criteria

**PASS**:
1. All P0 tests pass (0 failures)
2. Per-tier code coverage >=80%
3. No regressions in `UT-AA-056-*` test suite
4. Generated CRD YAML does NOT list CNV booleans under `required`
5. CNV fields round-trip: KA map → `extractDetectedLabels` → struct → `detectedLabelsToMap` → Rego

**FAIL**:
1. Any P0 test fails
2. Generated CRD still lists CNV fields as required
3. CNV data lost at any pipeline stage

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/aianalysis/handlers/response_processor.go` | `extractDetectedLabels` (4 new lines) | ~4 |
| `pkg/aianalysis/handlers/analyzing.go` | `detectedLabelsToMap` (4 new lines) | ~4 |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/aianalysis/handlers/response_processor.go` | `ProcessIncidentResponse` → `populatePostRCAContext` → CRD write | ~15 |

---

## 7. BR Coverage Matrix

| BR/Issue | Description | Priority | Tier | Test ID | Status |
|----------|-------------|----------|------|---------|--------|
| #1400 | CNV fields extracted from KA response | P0 | Unit | UT-AA-1400-001 | Pending |
| #1400 | Non-CNV response (zero values) works identically | P0 | Unit | UT-AA-1400-002 | Pending |
| #1400 | StorageBackend string extracted | P1 | Unit | UT-AA-1400-003 | Pending |
| #1400 | detectedLabelsToMap includes CNV fields (snake_case) | P0 | Unit | UT-AA-1400-004 | Pending |
| #1400 | detectedLabelsToMap nil-safe for nil DetectedLabels | P1 | Unit | UT-AA-1400-005 | Pending |
| #1400 | CRD schema: CNV booleans NOT in required list | P0 | Unit | UT-AA-1400-006 | Pending |
| #1400 | ProcessIncidentResponse persists CNV fields to PostRCAContext | P0 | Integration | IT-AA-1400-001 | Pending |
| #1400 | Non-CNV KA response: PostRCAContext has false CNV fields | P0 | Integration | IT-AA-1400-002 | Pending |
| #1400 | E2E: CNV labels reach CRD after full investigation | P1 | E2E | E2E-KA-1400-001 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{ISSUE}-{SEQUENCE}`
- **TIER**: `UT` (Unit), `IT` (Integration), `E2E` (End-to-End)
- **SERVICE**: `AA` (AIAnalysis), `KA` (KubernautAgent)
- **ISSUE**: `1400`
- **SEQUENCE**: Zero-padded 3-digit

### Tier 1: Unit Tests

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `UT-AA-1400-001` | AU-3: `extractDetectedLabels` maps all 4 CNV fields from KA camelCase keys | AU-3 | A |
| `UT-AA-1400-002` | SI-17: Non-CNV input (no CNV keys) produces struct with `false` CNV fields (backward compat) | SI-17 | A |
| `UT-AA-1400-003` | AU-3: `extractDetectedLabels` maps `storageBackend` string field | AU-3 | A |
| `UT-AA-1400-004` | AC-6: `detectedLabelsToMap` includes `virtual_machine`, `live_migratable`, `cdi_managed`, `storage_backend` | AC-6 | A |
| `UT-AA-1400-005` | SI-17: `detectedLabelsToMap` with nil DetectedLabels returns empty map (no panic) | SI-17 | A |
| `UT-AA-1400-006` | SI-10: Generated CRD YAML does not list CNV booleans in `required` array | SI-10 | B |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `IT-AA-1400-001` | AU-3: Full ProcessIncidentResponse with CNV labels persists all 4 fields to PostRCAContext | AU-3 | C |
| `IT-AA-1400-002` | SI-17: ProcessIncidentResponse without CNV keys still populates PostRCAContext (zero values) | SI-17 | C |

### Tier 3: E2E Tests

| ID | Business Outcome Under Test | FedRAMP | Phase |
|----|----------------------------|---------|-------|
| `E2E-KA-1400-001` | AU-3: CNV labels detected by KA appear on AIAnalysis CRD status.postRCAContext | AU-3 | D |

**Infrastructure**: Existing E2E-KA-1378-001 cluster setup (Kind, no CNV CRDs — verifies graceful skip). New test extends with mock CNV response scenario.

---

## 9. Test Cases

### UT-AA-1400-001: extractDetectedLabels maps all 4 CNV fields

**Priority**: P0
**Type**: Unit
**File**: `pkg/aianalysis/response_processor_post_rca_test.go` (extend existing)

**Test Steps**:
1. **Given**: A `map[string]interface{}` with `"virtualMachine": true`, `"liveMigratable": true`, `"cdiManaged": false`, `"storageBackend": "odf-ceph"`
2. **When**: `extractDetectedLabels(m)` is called
3. **Then**: Returned struct has `VirtualMachine=true`, `LiveMigratable=true`, `CDIManaged=false`, `StorageBackend="odf-ceph"`

**Expected Results**:
1. All 4 CNV fields match input values
2. Non-CNV fields default to zero values (unaffected)

---

### UT-AA-1400-002: Non-CNV input backward compatibility

**Priority**: P0
**Type**: Unit
**File**: `pkg/aianalysis/response_processor_post_rca_test.go`

**Test Steps**:
1. **Given**: A map with only `"gitOpsManaged": true, "stateful": true` (no CNV keys)
2. **When**: `extractDetectedLabels(m)` is called
3. **Then**: CNV fields are all `false`/empty, non-CNV fields match input

**Expected Results**:
1. `VirtualMachine`, `LiveMigratable`, `CDIManaged` all `false`
2. `StorageBackend` is `""`
3. Behavior identical to pre-fix

---

### UT-AA-1400-004: detectedLabelsToMap includes CNV fields

**Priority**: P0
**Type**: Unit
**File**: `pkg/aianalysis/analyzing_handler_post_rca_test.go` (extend existing)

**Test Steps**:
1. **Given**: A `DetectedLabels` struct with `VirtualMachine=true, LiveMigratable=true, CDIManaged=true, StorageBackend="lvms"`
2. **When**: `detectedLabelsToMap(dl)` is called
3. **Then**: Map contains `"virtual_machine": true, "live_migratable": true, "cdi_managed": true, "storage_backend": "lvms"`

**Expected Results**:
1. All 4 fields present with snake_case keys
2. Values match struct field values

---

### IT-AA-1400-001: Full ProcessIncidentResponse with CNV labels

**Priority**: P0
**Type**: Integration
**File**: `pkg/aianalysis/response_processor_post_rca_test.go` (extend)

**Test Steps**:
1. **Given**: An AIAnalysis in Investigating phase; KA response with all detected_labels including CNV fields
2. **When**: `ProcessIncidentResponse(ctx, analysis, kaResp)` is called
3. **Then**: `analysis.Status.PostRCAContext.DetectedLabels` has all CNV fields populated

**Expected Results**:
1. `VirtualMachine`, `LiveMigratable`, `CDIManaged` match KA response
2. `StorageBackend` matches KA response
3. Non-CNV fields also populated (no regression)

---

### E2E-KA-1400-001: CNV labels reach CRD after investigation

**Priority**: P1
**Type**: E2E
**File**: `test/e2e/kubernautagent/cnv_graceful_skip_e2e_test.go` (extend)

**Test Steps**:
1. **Given**: Kind cluster with KA and AA running; mock KA response includes CNV labels
2. **When**: Full investigation pipeline executes
3. **Then**: AIAnalysis CRD `status.postRCAContext.detectedLabels` contains CNV fields

**Note**: This extends the existing E2E-KA-1378-001 by adding a test case where a mock response includes CNV true values (the existing test verifies graceful skip on non-CNV clusters).

---

## 10. Environmental Needs

### 10.1 Unit Tests
- Go 1.22+
- Ginkgo v2 / Gomega
- No external dependencies

### 10.2 Integration Tests
- Go 1.22+
- `pkg/agentclient` ogen types for mock KA responses
- Existing `buildIncidentResponseWithDetectedLabels` helper

### 10.3 E2E Tests
- Kind cluster (`kubernautagent-e2e`)
- KA + AA binaries
- Mock response fixtures

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Status | Impact if Missing |
|------------|--------|-------------------|
| `DetectedLabels` struct (from #1378) | Complete | CNV fields exist on struct |
| `detectedLabelsToResult` (KA) | Complete | KA sends CNV fields in HTTP response |
| `buildIncidentResponseWithDetectedLabels` test helper | Exists | Reuse in new tests |
| `make manifests` toolchain | Available | CRD regeneration |

### 11.2 TDD Execution Order (Phased)

```
Phase A (RED -> GREEN -> REFACTOR): Extraction + Map Logic
  ├── UT-AA-1400-001..005
  └── CHECKPOINT A

Phase B (RED -> GREEN -> REFACTOR): CRD Schema
  ├── UT-AA-1400-006
  ├── make manifests
  └── CHECKPOINT B

Phase C (RED -> GREEN -> REFACTOR): Integration Wiring
  ├── IT-AA-1400-001..002
  └── CHECKPOINT W (wiring verification)

Phase D (RED -> GREEN -> REFACTOR): E2E Journey
  ├── E2E-KA-1400-001
  └── CHECKPOINT FINAL (Pyramid Invariant)
```

---

## 12. Test Deliverables

| Deliverable | Location | Format |
|-------------|----------|--------|
| Test plan | `docs/tests/1400/TEST_PLAN.md` | IEEE 829 hybrid |
| Unit tests (extraction) | `pkg/aianalysis/response_processor_post_rca_test.go` | Ginkgo/Gomega |
| Unit tests (Rego map) | `pkg/aianalysis/analyzing_handler_post_rca_test.go` | Ginkgo/Gomega |
| Integration tests | `pkg/aianalysis/response_processor_post_rca_test.go` | Ginkgo/Gomega |
| E2E tests | `test/e2e/kubernautagent/cnv_graceful_skip_e2e_test.go` | Ginkgo/Gomega |

---

## 13. Execution

```bash
# Unit tests (Phase A + B)
go test ./pkg/aianalysis/... -run "UT-AA-1400" -v

# Integration tests (Phase C)
go test ./pkg/aianalysis/... -run "IT-AA-1400" -v

# E2E tests (Phase D)
make test-e2e-kubernautagent FOCUS="E2E-KA-1400"

# Coverage
go test ./pkg/aianalysis/... -coverprofile=coverage-aa.out

# Regression check
go test ./pkg/aianalysis/... -run "UT-AA-056" -v
```

---

## 14. Go Anti-Pattern Validation

| # | Mistake | Applicable? | Validation |
|---|---------|-------------|------------|
| 4 | Overusing getters | No | Direct field access on structs |
| 10 | Not being aware of type embedding pitfalls | No | No embedding changes |
| 28 | Maps and memory leaks | Yes | `detectedLabelsToMap` creates fresh map each call; no accumulation |
| 36 | Unnecessary type conversions | Yes | `GetBoolFromMap` handles type assertion correctly |
| 54 | Not using testing utility packages | Yes | Reuse `buildIncidentResponseWithDetectedLabels` helper |
| 60 | Not using table-driven tests | Yes | CNV field assertions use table-driven Entry pattern |
| 78 | JSON marshaling considerations | No | No JSON marshaling in extraction (works on `map[string]interface{}`) |
| 89 | Not closing resources | No | Pure functions, no resources opened |
| 97 | Not using context correctly | No | No context changes in extraction logic |

---

## 15. Checkpoint Protocol

At each checkpoint (A, B, W, FINAL), perform the following GA readiness audit:

1. **Build validation**: `go build ./...` — zero errors
2. **Test pass rate**: All tests in affected packages pass (100%)
3. **Lint compliance**: `golangci-lint run --timeout=5m` — zero new warnings on changed files
4. **Per-tier coverage**: >=80% on tier-specific code subset
5. **Regression guard**: Existing `UT-AA-056-*` and `E2E-KA-1378-001` tests pass
6. **CRD schema validation**: CNV booleans not in `required` (for phases B+)
7. **100-go-mistakes**: Validate against applicable patterns listed in section 14
8. **Escalation gate**: Confidence >=95% to proceed; <95% escalate with actionable findings

---

## 16. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `extractDetectedLabels` CNV fields | `populatePostRCAContext()` | `pkg/aianalysis/handlers/response_processor.go:284` | IT-AA-1400-001 |
| `detectedLabelsToMap` CNV fields | `resolveRegoInput()` → Rego evaluator | `pkg/aianalysis/handlers/analyzing.go:462` | IT-AA-1400-001 |
| `+optional` CRD markers | Kubernetes API server CRD validation | `pkg/shared/types/enrichment.go:197-203` | UT-AA-1400-006 |
