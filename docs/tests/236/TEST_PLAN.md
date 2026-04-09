# Test Plan: EM DataStorageQuerier ogen Migration

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-236-v1
**Feature**: Migrate EffectivenessMonitor DataStorageQuerier from raw HTTP to ogen OpenAPI client (DD-API-001)
**Version**: 1.0
**Created**: 2026-04-09
**Author**: AI Assistant
**Status**: Draft
**Branch**: `development/v1.3_part2`

---

## 1. Introduction

### 1.1 Purpose

Validate that the EM DataStorageQuerier migration from hand-written `net/http` to the ogen-generated OpenAPI client preserves identical business behavior while achieving DD-API-001 compliance.

### 1.2 Objectives

1. **Functional parity**: All 3 `DataStorageQuerier` methods (`QueryPreRemediationHash`, `HasWorkflowStarted`, `HasWorkflowCompleted`) produce identical results before and after migration.
2. **DD-API-001 compliance**: No raw `net/http` calls remain in `ds_querier.go`; all DS communication uses the ogen-generated client.
3. **DD-AUTH-014 compliance**: ServiceAccount transport is used for authentication, aligned with the established ogen adapter pattern.

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./test/unit/effectivenessmonitor/... -ginkgo.focus="DataStorageQuerier"` |
| Integration test pass rate | 100% | `go test ./test/integration/effectivenessmonitor/... -ginkgo.focus="G4"` |
| Backward compatibility | 0 regressions | All existing EM tests pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- DD-API-001: OpenAPI Generated Client MANDATORY
- DD-AUTH-014: ServiceAccount transport authentication pattern
- DD-EM-002: Pre-remediation spec hash lookup
- ADR-EM-001: EffectivenessMonitor assessment scope detection
- Issue #236: Migrate EffectivenessMonitor DataStorageQuerier to ogen OpenAPI client

### 2.2 Cross-References

- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- Pattern reference: `pkg/audit/openapi_client_adapter.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Mitigation |
|----|------|--------|-------------|------------|
| R1 | ogen sum type discriminator requires `event_type` inside `event_data` JSON | Decode error on mock responses | High (verified) | All mock JSON includes discriminator field. Preflight verified exact mapping in `oas_json_gen.go` lines 7896, 7923, 7929. |
| R2 | `AuditEvent` has 8 required fields; current mocks provide 2 | Decode error `validate.ErrFieldRequired` | High (verified) | Create `testAuditEvent` helper populating all 8 required fields with valid defaults. |
| R3 | `WorkflowExecutionAuditPayload` has 7 required non-optional fields | Decode error on HasWorkflowStarted/Completed mocks | Medium | Mock helper includes valid defaults for `workflow_id`, `workflow_version`, `target_resource`, `phase`, `container_image`, `execution_name`. |

---

## 4. Scope

### 4.1 Features to be Tested

- **DataStorageQuerier adapter** (`pkg/effectivenessmonitor/client/ds_querier.go`): ogen-backed implementation of 3 query methods
- **Constructor wiring** (`cmd/effectivenessmonitor/main.go`): New constructor replaces old

### 4.2 Features Not to be Tested

- **ogen-generated client internals** (`pkg/datastorage/ogen-client/`): Generated code, not project-owned
- **Reconciler logic**: Interface unchanged, reconciler not modified
- **Audit write path**: Separate adapter, already ogen-compliant

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Keep `DataStorageQuerier` interface unchanged | Zero blast radius on reconciler and all dependent code |
| Use httptest + real ogen client in tests | Matches established pattern in `openapi_client_adapter_test.go` |
| Schema-compliant mock JSON | Required by ogen decoder's strict field validation |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: Existing 10 UT-EM-DSQ tests adapted to ogen constructor and schema-compliant mocks. No new test scenarios needed (behavior unchanged).
- **Integration**: Existing IT-EM-573-013/014 adapted to new constructor and compliant mocks.
- **E2E**: Tier skip — no runtime behavior change; E2E validates controller reconciliation, not DS client internals.

### 5.2 Business Outcome Quality Bar

Tests validate business outcomes: correct hash extraction, correct boolean responses for workflow lifecycle queries, and correct error propagation. Not just "ogen client is called."

### 5.4 Pass/Fail Criteria

**PASS**: All existing UT-EM-DSQ and IT-EM-573 tests pass with identical assertions after migration.

**FAIL**: Any existing test fails, any regression in other EM test suites, or build failure.

---

## 6. Test Items

### 6.1 Unit-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/effectivenessmonitor/client/ds_querier.go` | `QueryPreRemediationHash`, `HasWorkflowStarted`, `HasWorkflowCompleted`, constructors | ~100 |

### 6.2 Integration-Testable Code

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `cmd/effectivenessmonitor/main.go` | DS querier wiring (lines 332-337) | ~6 |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| DD-API-001 | ogen client mandatory | P0 | Unit | UT-EM-DSQ-001..009, UT-EM-573-009 | Pending |
| DD-EM-002 | Pre-remediation hash lookup | P0 | Unit | UT-EM-DSQ-001..005 | Pending |
| ADR-EM-001 §5 | Workflow lifecycle detection | P0 | Unit | UT-EM-DSQ-006..009, UT-EM-573-009 | Pending |
| ADR-EM-001 §5 | Assessment path differentiation | P0 | Integration | IT-EM-573-013, IT-EM-573-014 | Pending |

---

## 8. Test Scenarios

### Tier 1: Unit Tests

Existing tests adapted (no new scenarios — behavior unchanged):

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| UT-EM-DSQ-001 | Pre-remediation hash retrieved from workflow_created event | Pending |
| UT-EM-DSQ-002 | Empty string returned when no events found | Pending |
| UT-EM-DSQ-003 | Empty string returned when event has no hash field | Pending |
| UT-EM-DSQ-004 | Error propagated on HTTP 500 | Pending |
| UT-EM-DSQ-005 | Error propagated when DS unreachable | Pending |
| UT-EM-DSQ-006 | True returned when execution.started event exists | Pending |
| UT-EM-DSQ-007 | False returned when no execution.started event | Pending |
| UT-EM-DSQ-008 | Error propagated on HTTP 500 (HasWorkflowStarted) | Pending |
| UT-EM-DSQ-009 | Error propagated when DS unreachable (HasWorkflowStarted) | Pending |
| UT-EM-573-009 | HasWorkflowCompleted returns true/false correctly | Pending |

### Tier 2: Integration Tests

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| IT-EM-573-013 | No-execution path detected when no started event | Pending |
| IT-EM-573-014 | Partial path detected when started but not completed | Pending |

### Tier Skip Rationale

- **E2E**: No runtime behavior change. E2E validates full reconciliation, not DS client library choice.

---

## 9. Environmental Needs

### 9.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: `httptest.NewServer` serving ogen-compliant JSON
- **Location**: `test/unit/effectivenessmonitor/ds_querier_test.go`

### 9.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD
- **Infrastructure**: `httptest.NewServer` + `envtest` (existing suite)
- **Location**: `test/integration/effectivenessmonitor/issue573_integration_test.go`

---

## 10. Execution

```bash
# Unit tests
go test ./test/unit/effectivenessmonitor/... -ginkgo.focus="DataStorageQuerier" -ginkgo.v

# Integration tests
go test ./test/integration/effectivenessmonitor/... -ginkgo.focus="G4" -ginkgo.v

# Full EM test suite (regression check)
go test ./test/unit/effectivenessmonitor/... -count=1
```

---

## 11. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-09 | Initial test plan. Adapted from existing UT-EM-DSQ and IT-EM-573 test suites for ogen migration. |
