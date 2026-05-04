# Test Plan: K8s Call Audit Event for Impersonated API Calls

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-898-v1
**Feature**: Emit `aiagent.interactive.k8s_call` audit event for every impersonated K8s API call during interactive sessions
**Version**: 1.0
**Created**: 2026-05-04
**Author**: AI Assistant
**Status**: Draft
**Branch**: `feature/898-k8s-call-audit`

---

## 1. Introduction

### 1.1 Purpose

Validates that every impersonated K8s API call made by the `ImpersonatingRoundTripper` during an interactive MCP session emits a structured audit event to DataStorage, enabling SOC2 CC8.1 compliance and forensic reconstruction of user actions.

### 1.2 Objectives

1. **Audit emission**: Every impersonated HTTP request through `ImpersonatingRoundTripper` produces an `aiagent.interactive.k8s_call` audit event with correct fields
2. **No-op for autonomous**: No audit event is emitted when no impersonation context is set (autonomous mode)
3. **Fire-and-forget**: Audit emission failures do not block or fail the K8s API call
4. **DataStorage integration**: The ogen-generated client correctly serializes the new `AIAgentInteractiveK8sCallPayload`
5. **Decoupling**: The round-tripper depends on a narrow `K8sCallAuditor` interface, not the concrete `AuditStore`

### 1.3 Success Metrics

- Unit test pass rate: 100% (`go test ./test/unit/kubernautagent/transport/...` and `./test/unit/kubernautagent/audit/...`)
- Unit-testable code coverage: >=80% of new code in `pkg/shared/transport/impersonate.go`, `internal/kubernautagent/audit/emitter.go`, `internal/kubernautagent/audit/ds_store.go`
- Integration-testable code coverage: >=80% of wiring paths (`cmd/kubernautagent/main.go` injection)
- Backward compatibility: 0 regressions in existing transport/audit tests

---

## 2. References

### 2.1 Authority (governing documents)

- **BR-INTERACTIVE-003**: Audit attribution — every action during an interactive session must be attributable to the acting user
- **BR-AUDIT-005**: Audit event emission to DataStorage
- Issue #898: K8s call audit event
- Issue #703: Agentic Integration (parent)

### 2.2 Cross-References

- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Testing Guidelines](../../development/business-requirements/TESTING_GUIDELINES.md)
- [DD-AUTH-MCP-001](../../architecture/decisions/DD-AUTH-MCP-001-mcp-endpoint-security.md): `aiagent.interactive.k8s_call` classified as Sensitive
- [DD-INTERACTIVE-002](../../architecture/decisions/DD-INTERACTIVE-002-dynamic-takeover-model.md): Takeover model

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Audit emission blocks K8s API call | Degraded K8s operations during interactive sessions | Low | UT-KA-898-004 | Fire-and-forget pattern with `context.Background()` |
| R2 | K8s URL parsing fails on custom resource paths | Missing audit data for CR operations | Medium | UT-KA-898-003 | Table-driven tests covering standard, CRD, and edge-case URL patterns |
| R3 | `pkg/shared/transport` importing `internal/` | Circular dependency or visibility violation | High | UT-KA-898-001 | Narrow `K8sCallAuditor` interface defined in `pkg/shared/transport/` |
| R4 | OpenAPI spec changes break existing ogen types | Build failure | Low | Build checkpoint | Regenerate and verify build before tests |

### 3.1 Risk-to-Test Traceability

- **R1 (High)**: UT-KA-898-004 (audit failure does not block RoundTrip)
- **R2 (Medium)**: UT-KA-898-003 (table-driven URL parsing)
- **R3 (High)**: UT-KA-898-001 (interface-based mock injection)

---

## 4. Scope

### 4.1 Features to be Tested

- **K8sCallAuditor interface** (`pkg/shared/transport/auditor.go`): Narrow interface for audit emission, decoupling transport from internal audit
- **ImpersonatingRoundTripper audit hook** (`pkg/shared/transport/impersonate.go`): Post-call audit emission with K8s URL parsing
- **Emitter constants** (`internal/kubernautagent/audit/emitter.go`): `EventTypeInteractiveK8sCall` constant and `AllEventTypes` registration
- **DS store case** (`internal/kubernautagent/audit/ds_store.go`): `buildEventData` case for `EventTypeInteractiveK8sCall`
- **OpenAPI payload** (`api/openapi/data-storage-v1.yaml`): `AIAgentInteractiveK8sCallPayload` schema

### 4.2 Features Not to be Tested

- **DataStorage persistence**: Covered by existing DS integration tests
- **MCP session management**: Covered by existing session tests (issue #703)
- **JWT authentication**: Covered by existing auth tests (issue #1009)

### 4.3 Design Decisions

- K8sCallAuditor interface on consumer side (pkg/shared/transport/) per Go idiom #6 (interface on consumer side)
- Fire-and-forget with `context.Background()` to avoid blocking K8s calls
- URL parsing extracts resource/verb/namespace from standard K8s API patterns

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of unit-testable code (K8sCallAuditor interface, URL parser, emitter constants, buildEventData case)
- **Integration**: >=80% of integration-testable code (wiring in main.go, round-tripper with real HTTP)

### 5.2 Two-Tier Minimum

Every BR is covered by at least Unit + Integration tests.

### 5.3 Business Outcome Quality Bar

Tests validate that audit events contain the correct acting user, resource, verb, namespace, and HTTP status — the data required for SOC2 CC8.1 forensic reconstruction.

### 5.4 Pass/Fail Criteria

**PASS**: All P0 tests pass, >=80% per-tier coverage, no regressions.
**FAIL**: Any P0 test fails, coverage below 80%, or existing tests regress.

### 5.5 Suspension & Resumption Criteria

**Suspend**: Build broken after ogen regeneration.
**Resume**: Build fixed and ogen types validated.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/transport/impersonate.go` | URL parser logic, audit emission decision | ~30 new |
| `internal/kubernautagent/audit/emitter.go` | `EventTypeInteractiveK8sCall` constant | ~5 new |
| `internal/kubernautagent/audit/ds_store.go` | `buildEventData` case | ~20 new |

### 6.2 Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/transport/impersonate.go` | `RoundTrip` with real HTTP delegate | ~10 modified |
| `cmd/kubernautagent/main.go` | `K8sCallAuditor` injection wiring | ~5 new |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-INTERACTIVE-003 | K8s call audit with acting_user, resource, verb, namespace, result | P0 | Unit | UT-KA-898-001 | Pending |
| BR-INTERACTIVE-003 | No audit for autonomous (no impersonation context) | P0 | Unit | UT-KA-898-002 | Pending |
| BR-INTERACTIVE-003 | K8s URL parsing covers standard + CRD paths | P0 | Unit | UT-KA-898-003 | Pending |
| BR-INTERACTIVE-003 | Audit failure does not block K8s call | P0 | Unit | UT-KA-898-004 | Pending |
| BR-INTERACTIVE-003 | HTTP status captured from response | P1 | Unit | UT-KA-898-005 | Pending |
| BR-AUDIT-005 | buildEventData produces correct payload for k8s_call | P0 | Unit | UT-KA-898-006 | Pending |
| BR-INTERACTIVE-003 | Emitter constant registered in AllEventTypes | P1 | Unit | UT-KA-898-007 | Pending |
| BR-INTERACTIVE-003 | Round-tripper wired with auditor in main.go | P0 | Integration | IT-KA-898-001 | Pending |
| BR-INTERACTIVE-003 | End-to-end: impersonated call -> audit event emitted | P0 | Integration | IT-KA-898-002 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-KA-898-{SEQUENCE}` (TIER: UT/IT, SERVICE: KA = Kubernaut Agent)

### Tier 1: Unit Tests

**Testable code scope**: `pkg/shared/transport/impersonate.go` (URL parser, audit decision), `internal/kubernautagent/audit/emitter.go`, `internal/kubernautagent/audit/ds_store.go` — >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-KA-898-001` | Impersonated K8s call emits audit event with correct acting_user, resource, verb, namespace | Pending |
| `UT-KA-898-002` | Non-impersonated K8s call emits no audit event (autonomous mode safety) | Pending |
| `UT-KA-898-003` | K8s URL parser correctly extracts resource/verb/namespace from standard, CRD, and subresource paths | Pending |
| `UT-KA-898-004` | Audit emission failure does not propagate to RoundTrip caller (fire-and-forget) | Pending |
| `UT-KA-898-005` | HTTP response status code captured in audit event | Pending |
| `UT-KA-898-006` | buildEventData produces AIAgentInteractiveK8sCallPayload with all required fields | Pending |
| `UT-KA-898-007` | EventTypeInteractiveK8sCall is registered in AllEventTypes | Pending |

### Tier 2: Integration Tests

**Testable code scope**: Round-tripper with real HTTP delegate, wiring in main.go — >=80% coverage target.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-KA-898-001` | K8sCallAuditor is injected into ImpersonatingRoundTripper during main.go startup | Pending |
| `IT-KA-898-002` | Full round-trip: impersonated HTTP request -> delegate -> audit event captured by mock auditor | Pending |

### Tier Skip Rationale

- **E2E**: Deferred — requires full Kind cluster with DataStorage. K8s call audit is validated at unit + integration level. E2E coverage will be added with #874 E2E suite.

---

## 9. Test Cases

### UT-KA-898-001: Impersonated call emits audit event

**BR**: BR-INTERACTIVE-003
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/impersonate_audit_test.go`

**Preconditions**:
- Mock `K8sCallAuditor` injected into `ImpersonatingRoundTripper`
- Mock HTTP delegate returns 200 OK
- Impersonation context set with `acting_user: "jane@example.com"`, `session_id: "sess-123"`, `correlation_id: "rr-456"`

**Test Steps**:
1. **Given**: ImpersonatingRoundTripper with mock auditor and impersonation context
2. **When**: `RoundTrip` is called with a GET request to `/api/v1/namespaces/default/pods/my-pod`
3. **Then**: Mock auditor receives exactly one call with: `acting_user="jane@example.com"`, `resource="pods"`, `verb="get"`, `namespace="default"`, `resource_name="my-pod"`, `http_status_code=200`, `session_id="sess-123"`, `correlation_id="rr-456"`

**Expected Results**:
1. Audit event emitted with all required fields
2. Original HTTP response returned unchanged to caller

### UT-KA-898-002: No audit for autonomous mode

**BR**: BR-INTERACTIVE-003
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/impersonate_audit_test.go`

**Preconditions**:
- Mock `K8sCallAuditor` injected
- NO impersonation context set (autonomous mode)

**Test Steps**:
1. **Given**: ImpersonatingRoundTripper without impersonation context
2. **When**: `RoundTrip` is called
3. **Then**: Mock auditor receives zero calls

### UT-KA-898-003: K8s URL parsing (table-driven)

**BR**: BR-INTERACTIVE-003
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/k8s_url_parser_test.go`

**Test Steps**: Table-driven test covering:

| Input URL | Expected Resource | Expected Verb | Expected Namespace | Expected Name |
|-----------|------------------|---------------|-------------------|---------------|
| `GET /api/v1/namespaces/default/pods/my-pod` | pods | get | default | my-pod |
| `POST /api/v1/namespaces/kube-system/services` | services | create | kube-system | (empty) |
| `DELETE /apis/apps/v1/namespaces/prod/deployments/nginx` | deployments | delete | prod | nginx |
| `GET /apis/kubernaut.ai/v1alpha1/namespaces/ns1/remediationrequests/rr-1` | remediationrequests | get | ns1 | rr-1 |
| `GET /api/v1/nodes` | nodes | get | (empty) | (empty) |
| `GET /api/v1/namespaces/default/pods/my-pod/log` | pods/log | get | default | my-pod |
| `PATCH /api/v1/namespaces/default/pods/my-pod/status` | pods/status | get | default | my-pod |

### UT-KA-898-004: Audit failure is fire-and-forget

**BR**: BR-INTERACTIVE-003
**Priority**: P0
**Type**: Unit
**File**: `test/unit/shared/transport/impersonate_audit_test.go`

**Preconditions**:
- Mock `K8sCallAuditor` configured to return error
- Impersonation context set

**Test Steps**:
1. **Given**: Auditor that returns `errors.New("ds unavailable")`
2. **When**: `RoundTrip` is called
3. **Then**: Original HTTP response returned without error; no panic

### UT-KA-898-006: buildEventData produces correct payload

**BR**: BR-AUDIT-005
**Priority**: P0
**Type**: Unit
**File**: `test/unit/kubernautagent/audit/ds_store_test.go`

**Test Steps**:
1. **Given**: `AuditEvent` with `EventType=EventTypeInteractiveK8sCall` and populated metadata
2. **When**: `buildEventData` is called
3. **Then**: Returns `AIAgentInteractiveK8sCallPayload` with all fields correctly mapped

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: Mock `K8sCallAuditor` (interface-based), mock `http.RoundTripper` (delegate)
- **Location**: `test/unit/shared/transport/`, `test/unit/kubernautagent/audit/`

### 10.2 Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks — real HTTP via `httptest.NewServer`
- **Location**: `test/integration/kubernautagent/transport/`

### 10.4 Tools & Versions

| Tool | Minimum Version | Purpose |
|------|-----------------|---------|
| Go | 1.22+ | Build and test |
| Ginkgo CLI | v2.x | Test runner |
| ogen | latest | OpenAPI client generation |

---

## 11. Dependencies & Schedule

### 11.1 Blocking Dependencies

| Dependency | Type | Status | Impact if Not Available | Workaround |
|------------|------|--------|-------------------------|------------|
| ogen binary | Tool | Available | Cannot regenerate client | Use pre-existing types |

### 11.2 Execution Order (TDD Phases)

1. **Phase 1 — TDD RED**: Write all failing unit tests (UT-KA-898-001 through 007) + integration test stubs (IT-KA-898-001, 002)
2. **Checkpoint A — RED Gate**: All tests compile and fail for the right reason (missing implementation, not syntax errors). Verify test descriptions map to BRs. Verify no anti-patterns (no Skip, no time.Sleep, no pending tests).
3. **Phase 2 — TDD GREEN**: Minimal implementation to pass all tests. OpenAPI spec + ogen regen. K8sCallAuditor interface. URL parser. Emitter constants. buildEventData case. RoundTrip hook. main.go wiring.
4. **Checkpoint B — GREEN Gate**: All tests pass. `go build ./...` succeeds. `go vet ./...` clean. No lint errors. Coverage >= 80% per tier.
5. **Phase 3 — TDD REFACTOR**: Code quality pass against 100 Go Mistakes checklist (see Section 16). Duplication removal. Naming consistency. Error handling audit.
6. **Checkpoint C — REFACTOR Gate**: All tests still pass. No new lint errors. Go mistakes checklist complete. Build clean.

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/898/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `test/unit/shared/transport/`, `test/unit/kubernautagent/audit/` | Ginkgo BDD test files |
| Integration test suite | `test/integration/kubernautagent/transport/` | Ginkgo BDD test files |

---

## 13. Execution

```bash
# Unit tests
go test ./test/unit/shared/transport/... ./test/unit/kubernautagent/audit/... -ginkgo.v

# Integration tests
go test ./test/integration/kubernautagent/transport/... -ginkgo.v

# Specific test by ID
go test ./test/unit/shared/transport/... -ginkgo.focus="UT-KA-898"

# Coverage
go test ./test/unit/shared/transport/... -coverprofile=coverage-898-ut.out
go tool cover -func=coverage-898-ut.out
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| K8sCallAuditor injection | `cmd/kubernautagent/main.go` | `ImpersonatingRoundTripper.auditor` field | IT-KA-898-001 | Pending |
| Impersonated call -> audit | `RoundTrip()` with impersonation ctx | `K8sCallAuditor.EmitK8sCallAudit()` | IT-KA-898-002 | Pending |

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| `test/unit/shared/transport/impersonate_test.go` | Tests `NewImpersonatingRoundTripper` with current signature | Update constructor call to include optional auditor parameter | Constructor signature changes |

---

## 16. 100 Go Mistakes Refactoring Checklist

The following checks are applied during TDD REFACTOR phase (Phase 3):

| # | Mistake | Check | Applies To |
|---|---------|-------|------------|
| 1 | Variable shadowing | No `err` redeclaration in nested blocks | URL parser, RoundTrip |
| 2 | Unnecessary nesting | Early-return for no-impersonation case | RoundTrip audit decision |
| 5 | Interface pollution | K8sCallAuditor is minimal (1 method) | Interface definition |
| 6 | Interface on producer side | Interface defined in consumer pkg (transport), not producer (audit) | K8sCallAuditor |
| 7 | Returning interfaces | Functions return concrete types | Auditor implementation |
| 8 | `any` says nothing | No `any`/`interface{}` in new code | All new types |
| 21 | Inefficient slice init | N/A (no slice construction in hot path) | — |
| 27 | Inefficient map init | Metadata map pre-sized if known | Audit event metadata |
| 42 | Wrong receiver type | Pointer receiver for types with mutable state | K8sCallAuditor impl |
| 47 | Defer argument eval | No defer with mutable args in RoundTrip | Audit emission |
| 49 | Error wrapping | Use `%w` for wrappable errors, `%v` for opaque | Error returns |
| 52 | Handling error twice | Don't log AND return in same path | Audit emission |
| 53 | Not handling error | Explicit `_ =` for intentionally ignored errors | Fire-and-forget audit |
| 58 | Data races | No concurrent access to audit event fields | Event construction |

---

## 17. Anti-Pattern Compliance

Per TESTING_GUIDELINES.md:

- **No `time.Sleep()`**: All async assertions use `Eventually()` with Gomega
- **No `Skip()`**: All tests either pass or `Fail()` with clear messages
- **No pending tests**: Every test scenario is fully implemented
- **Ginkgo/Gomega BDD**: Mandatory framework, no standard `testing` package
- **Metrics validation**: Initial/final pattern for any counter/gauge assertions
- **Mock only externals**: K8sCallAuditor interface mocked (external dependency boundary); real business logic used

---

## 18. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-04 | Initial test plan |
