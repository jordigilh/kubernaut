# Test Plan: KA Parameter Validation (#1170)

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-1170-v1
**Feature**: Comprehensive workflow parameter validation in Kubernaut Agent with structured LLM self-correction feedback
**Version**: 1.0
**Created**: 2026-05-18
**Author**: AI Assistant
**Status**: Complete
**Branch**: `feat/1170-ka-parameter-validation`

---

## 1. Introduction

### 1.1 Purpose

Issue #1170 is a regression from the Python-to-Go migration: the KA lost comprehensive parameter validation that Python HAPI v1.2 provided. This test plan validates the reimplementation of 8 parameter constraints, undeclared parameter stripping, structured multi-error LLM feedback with schema hints, and the self-correction loop integration.

### 1.2 Objectives

1. **Parameter constraint validation**: All 8 constraint types produce correct errors
2. **KA-managed parameter exclusion**: 4 TARGET_RESOURCE_* params are never validated or stripped
3. **Undeclared parameter stripping**: In-place mutation removes undeclared params (fail-closed)
4. **Schema hint generation**: Formatted schema provided to LLM for self-correction
5. **Self-correction loop**: Multi-error ValidationResult integrates with SelfCorrect + template rendering
6. **Schema population**: FetchValidator extracts parameter schemas from workflow Content
7. **E2E self-correction**: Full stack validates first-fail → correction → pass flow

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./internal/kubernautagent/parser/... -ginkgo.focus="UT-KA-1170"` |
| Integration test pass rate | 100% | `make test-integration-kubernautagent` with focus filter |
| E2E test pass rate | 100% | `make test-e2e-kubernautagent` with focus filter |
| Unit coverage (validator.go) | >=80% | `-coverprofile` on parser package |
| Integration coverage | >=80% | `-coverpkg` on KA packages |
| Backward compatibility | 0 regressions | All existing 453+ specs pass |

---

## 2. References

### 2.1 Business Requirements

| BR ID | Description | Relevance |
|-------|-------------|-----------|
| BR-HAPI-191 | Workflow parameter validation in chat session | Primary: parameter schema validation + LLM self-correction |
| BR-AI-023 | Hallucination detection | Undeclared parameter stripping prevents hallucinated params |
| BR-HAPI-196 | Execution bundle consistency | Bundle validation in Validate() |
| BR-HAPI-197 | needs_human_review field | Exhausted self-correction sets human review |

### 2.2 Design Documents

| DD ID | Description |
|-------|-------------|
| DD-HAPI-002 v1.2 | Workflow Response Validation Architecture (KA as sole validator) |
| DD-HAPI-002 v1.3 | Undeclared parameter stripping |

### 2.3 Source References

- Python HAPI v1.2: `holmesgpt-api/src/validation/workflow_response_validator.py`
- Go validator: `internal/kubernautagent/parser/validator.go`
- Schema model: `pkg/datastorage/models/workflow_schema.go` (`WorkflowParameter` struct)
- Template: `internal/kubernautagent/prompt/templates/validation_error.tmpl`
- WFE defense-in-depth: `pkg/workflowexecution/executor/filter.go` (#243 fix)

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Mitigation |
|----|------|--------|------------|
| R1 | Type coercion: JSON numbers vs Go int/float | False type errors | Accept both int and float64 for integer type; JSON numbers unmarshal as float64 |
| R2 | Regex pattern compilation failure | Panic or silent skip | Compile once at schema load; skip validation with warning on invalid pattern |
| R3 | In-place map mutation during iteration | Undefined behavior | Collect keys to delete first, then delete in separate loop |
| R4 | SelfCorrect signature change breaks callers | Compilation failure | Update all callers in same commit; tests verify interface |
| R5 | Empty Content from DS (contract violation) | All params stripped | Fail-closed: strip all LLM params, log warning |
| R6 | DependsOn circular references | Infinite loop | Validate presence only (not ordering); no recursion |

---

## 4. Scope

### 4.1 In Scope

- 8 parameter validation constraints (required, type, min, max, enum, pattern, dependsOn, undeclared stripping)
- KA-managed parameter exclusion (4 params: TARGET_RESOURCE_NAME/KIND/NAMESPACE/API_VERSION)
- ValidationResult multi-error struct with SchemaHint
- formatSchemaHint() output for LLM feedback
- SelfCorrect loop update for ValidationResult
- validation_error.tmpl wiring into correctionFn
- FetchValidator Content parsing for schema population
- Integration test with real DS + mock LLM
- E2E test with mock LLM self-correction scenario

### 4.2 Out of Scope (deferred to v1.6)

- `type: object` as a parameter type
- Inline JSON Schema (`Items` field) for array element validation
- Object element validation within arrays

---

## 5. Test Scenarios

### Group A: Parameter Constraint Validation (Unit)

| ID | Constraint | Business Outcome Under Test | Status |
|----|-----------|----------------------------|--------|
| `UT-KA-1170-001` | Required | Missing required param produces error | Pending |
| `UT-KA-1170-002` | Required | Optional param absent is valid | Pending |
| `UT-KA-1170-003` | Type/string | String value passes string type check | Pending |
| `UT-KA-1170-004` | Type/integer | Integer value (float64 without fraction) passes | Pending |
| `UT-KA-1170-005` | Type/integer | Non-numeric value fails integer check | Pending |
| `UT-KA-1170-006` | Type/integer | Bool value rejected for integer type | Pending |
| `UT-KA-1170-007` | Type/boolean | Bool value passes boolean check | Pending |
| `UT-KA-1170-008` | Type/float | Numeric value passes float check | Pending |
| `UT-KA-1170-009` | Type/array | Slice value passes array check | Pending |
| `UT-KA-1170-010` | Type/array | Non-slice value fails array check | Pending |
| `UT-KA-1170-011` | Minimum | Value below minimum produces error | Pending |
| `UT-KA-1170-012` | Minimum | Value at minimum is valid | Pending |
| `UT-KA-1170-013` | Maximum | Value above maximum produces error | Pending |
| `UT-KA-1170-014` | Maximum | Value at maximum is valid | Pending |
| `UT-KA-1170-015` | Enum | Value in enum set is valid | Pending |
| `UT-KA-1170-016` | Enum | Value not in enum set produces error | Pending |
| `UT-KA-1170-017` | Pattern | Value matching regex is valid | Pending |
| `UT-KA-1170-018` | Pattern | Value not matching regex produces error | Pending |
| `UT-KA-1170-019` | Pattern | Invalid regex pattern is skipped with warning | Pending |
| `UT-KA-1170-020` | DependsOn | Param present when dependency is present is valid | Pending |
| `UT-KA-1170-021` | DependsOn | Param present when dependency is absent produces error | Pending |

### Group B: Undeclared Parameter Stripping (Unit)

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `UT-KA-1170-030` | Undeclared params are removed in-place | Pending |
| `UT-KA-1170-031` | Declared params are preserved | Pending |
| `UT-KA-1170-032` | KA-managed params (TARGET_RESOURCE_*) are never stripped | Pending |
| `UT-KA-1170-033` | No schema (nil Parameters) strips ALL LLM params | Pending |
| `UT-KA-1170-034` | Empty schema (0 params declared) strips ALL LLM params | Pending |
| `UT-KA-1170-035` | KA-managed params skipped during validation (no required check) | Pending |

### Group C: Schema Hint Formatting (Unit)

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `UT-KA-1170-040` | Schema hint includes param names and types | Pending |
| `UT-KA-1170-041` | Required params marked as "(required)" | Pending |
| `UT-KA-1170-042` | Constraints (min, max, enum) included in hint | Pending |
| `UT-KA-1170-043` | KA-managed params excluded from hint | Pending |
| `UT-KA-1170-044` | Empty schema returns "No parameter schema available." | Pending |

### Group D: Multi-Error ValidationResult + SelfCorrect (Unit)

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `UT-KA-1170-050` | Multiple constraint violations produce multiple errors | Pending |
| `UT-KA-1170-051` | ValidationResult.IsValid=true when no errors | Pending |
| `UT-KA-1170-052` | ValidationResult.SchemaHint populated on failure | Pending |
| `UT-KA-1170-053` | SelfCorrect records multi-error in ValidationAttemptsHistory | Pending |
| `UT-KA-1170-054` | SelfCorrect passes ValidationResult to correctionFn | Pending |
| `UT-KA-1170-055` | Template renders errors + schema hint correctly | Pending |
| `UT-KA-1170-056` | Existing allowlist/confidence checks still work | Pending |
| `UT-KA-1170-057` | HumanReviewNeeded short-circuits validation | Pending |

### Group E: FetchValidator Schema Population (Unit)

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `UT-KA-1170-060` | WorkflowMeta.Parameters populated from Content YAML | Pending |
| `UT-KA-1170-061` | Parse failure leaves Parameters nil (fail-closed) | Pending |
| `UT-KA-1170-062` | Empty Content leaves Parameters nil | Pending |

### Group F: Integration Tests (Real DS + Mock LLM)

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `IT-KA-1170-001` | Invalid params trigger self-correction with schema hint | Pending |
| `IT-KA-1170-002` | Corrected params pass validation on retry | Pending |
| `IT-KA-1170-003` | Undeclared params stripped in final result | Pending |
| `IT-KA-1170-004` | validation_attempts_history shows errors and schema hint | Pending |

### Group G: E2E Tests (Kind + Real DS + Mock LLM)

| ID | Business Outcome Under Test | Status |
|----|----------------------------|--------|
| `E2E-KA-1170-001` | Param validation self-correction: first fail, second pass | Pending |
| `E2E-KA-1170-002` | Final response has valid params, undeclared stripped | Pending |
| `E2E-KA-1170-003` | validation_attempts_history present with attempt 1 failed, attempt 2 passed | Pending |

---

## 6. BR Coverage Matrix

| BR ID | Test IDs Covering It | Status |
|-------|---------------------|--------|
| BR-HAPI-191 | UT-KA-1170-001..021, 030..035, 040..044, IT-KA-1170-001..004, E2E-KA-1170-001..003 | Pending |
| BR-AI-023 | UT-KA-1170-030..034, E2E-KA-1170-002 | Pending |
| BR-HAPI-196 | UT-KA-1170-056 (allowlist preserved) | Pending |
| BR-HAPI-197 | UT-KA-1170-053, UT-KA-1170-054 (exhaustion → human review) | Pending |
| DD-HAPI-002 | All groups | Pending |

---

## 7. Environmental Needs

### 7.1 Unit Tests
- Go 1.23+, Ginkgo v2, Gomega
- No external dependencies

### 7.2 Integration Tests
- Podman (PostgreSQL, Redis, DataStorage containers)
- envtest (K8s API server)
- In-process mock LLM client (response queue)

### 7.3 E2E Tests
- Kind cluster
- Real DataStorage (Podman in Kind)
- Mock LLM container with `param_validation_self_correct` scenario
- Real KA binary (coverage-instrumented)

---

## 8. Anti-Pattern Checklist (100 Go Mistakes)

| # | Mistake | Validation | Phase |
|---|---------|-----------|-------|
| #1 | Unintended variable shadowing | No `:=` inside if that shadows outer | REFACTOR |
| #9 | Being confused about when to use generics | No generics needed here | REFACTOR |
| #12 | Not knowing which type of receiver to use | Pointer receiver for Validator (mutates state) | REFACTOR |
| #28 | Maps and memory leaks | No growing maps without bounds | REFACTOR |
| #53 | Not handling defer errors | No defers with unchecked errors | REFACTOR |
| #54 | Not handling an error | All errors wrapped with context | REFACTOR |
| #56 | Using filename as function input | Pass []byte or io.Reader, not filenames | REFACTOR |
| #73 | Not using testing utility packages | Use Gomega matchers, not manual asserts | REFACTOR |
| #77 | Not closing a resource | regex.Compile: no Close needed | REFACTOR |
| #89 | Writing inaccurate benchmarks | N/A (no benchmarks in this change) | REFACTOR |

---

## 9. GA Readiness Checkpoints

### Checkpoint 1: After TDD RED

- [ ] All test scenarios compile and fail for the right reason
- [ ] No changes to production code
- [ ] Test scenario IDs match this plan

### Checkpoint 2: After TDD GREEN

- [ ] All unit tests pass
- [ ] `go build ./...` succeeds
- [ ] No lint errors in changed files
- [ ] Existing 453+ specs still pass (backward compat)

### Checkpoint 3: After TDD REFACTOR

- [ ] 100 Go Mistakes checklist validated
- [ ] `golangci-lint run --timeout=5m` clean
- [ ] Coverage >=80% for validator.go
- [ ] No code duplication with ValidateParameterValue

### Checkpoint 4: After E2E

- [ ] E2E test passes in Kind cluster
- [ ] Full test suite green
- [ ] Security: no credential exposure in schema hints
- [ ] Observability: audit events emitted for validation attempts
- [ ] API Contract: ValidationAttemptsHistory schema unchanged

---

## 10. Execution

```bash
# Unit tests
go test ./internal/kubernautagent/parser/... -ginkgo.focus="UT-KA-1170" -v

# Integration tests
make test-integration-kubernautagent GINKGO_FOCUS="IT-KA-1170"

# E2E tests
make test-e2e-kubernautagent GINKGO_FOCUS="E2E-KA-1170"

# Coverage
go test ./internal/kubernautagent/parser/... -coverprofile=coverage.out -coverpkg=./internal/kubernautagent/parser/...
go tool cover -func=coverage.out | grep validator
```

---

## 11. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-18 | Initial test plan |
