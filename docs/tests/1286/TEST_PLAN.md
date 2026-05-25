# Test Plan: AF Audit Trail Interpretation Layer

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut
>
> Based on IEEE 829-2008 (Standard for Software and System Test Documentation) with
> Kubernaut-specific extensions for TDD phase tracking, business requirement traceability,
> and per-tier coverage policy.

**Test Plan Identifier**: TP-1286-v1.0
**Feature**: Static event-type-to-description mapping in AF audit trail tool handler
**Version**: 1.0
**Created**: 2026-05-25
**Author**: AI Agent
**Status**: Active
**Branch**: `feat/1286-af-audit-trail-interpretation`

---

## 1. Introduction

### 1.1 Purpose

The `kubernaut_get_audit_trail` MCP tool returns raw DS audit events with opaque `event_type` strings (e.g. `gateway.signal.deduplicated`). The AF LLM cannot interpret these as meaningful remediation lifecycle data, reporting "audit trail came back empty" despite 50+ events being returned. This test plan validates a deterministic interpretation layer that maps event types to human-readable lifecycle phases and descriptions.

### 1.2 Objectives

1. **Enrichment correctness**: All canonical event types map to the correct lifecycle phase and human-readable description
2. **Fallback resilience**: Unknown event types produce a fallback phase/description without errors
3. **Lifecycle summary**: Chronologically ordered, deduplicated phase summaries are generated from event sequences
4. **Backward compatibility**: Existing `AuditEvent` consumers are not broken by the enriched struct

### 1.3 Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Unit test pass rate | 100% | `go test ./pkg/apifrontend/tools/... -ginkgo.focus="UT-AF-1286"` |
| Unit-testable code coverage | >=80% | `go test -coverprofile` on interpretation logic |
| Backward compatibility | 0 regressions | Existing UT-AF-124-001..004 pass without modification |

---

## 2. References

### 2.1 Authority (governing documents)

- Issue #1286: DS: expose MCP tool for interpreted audit trail data
- Accepted approach: Deterministic interpretation layer in AF `HandleGetAuditTrail`

### 2.2 Cross-References

- [Testing Strategy](../../.cursor/rules/03-testing-strategy.mdc)
- Event type sources: `pkg/gateway/server.go`, `internal/kubernautagent/audit/emitter.go`, `pkg/aianalysis/audit/audit.go`, `pkg/datastorage/audit/workflow_discovery_event.go`, `pkg/remediationorchestrator/audit/manager.go`, `pkg/apifrontend/audit/audit.go`, `pkg/workflowexecution/audit/manager.go`, `pkg/notification/audit/manager.go`, `pkg/signalprocessing/audit/client.go`

---

## 3. Risks & Mitigations

| ID | Risk | Impact | Probability | Affected Tests | Mitigation |
|----|------|--------|-------------|----------------|------------|
| R1 | Event type map incomplete — new types added without updating map | LLM sees "Unknown" phase for valid events | Medium | UT-AF-1286-006 | Table-driven test iterates all known constants; fallback ensures no crashes |
| R2 | Lifecycle summary order wrong if timestamps are not sorted | Incorrect narrative for operator | Low | UT-AF-1286-003 | Events arrive from DS in chronological order; test verifies ordering |
| R3 | Enriched struct breaks JSON serialization for existing consumers | Runtime failure in MCP bridge | Low | UT-AF-1286-007 | New fields use `omitempty`; existing fields unchanged |

---

## 4. Scope

### 4.1 Features to be Tested

- **Event interpretation map** (`pkg/apifrontend/tools/ds_tools.go`): Static `event_type` → `(Phase, Description)` lookup table
- **AuditEvent enrichment** (`pkg/apifrontend/tools/ds_tools.go`): `Phase` and `Description` fields added to `AuditEvent` struct
- **Lifecycle summary** (`pkg/apifrontend/tools/ds_tools.go`): `Lifecycle` string in `GetAuditTrailResult`
- **HandleGetAuditTrail** (`pkg/apifrontend/tools/ds_tools.go`): Enrichment wired into handler

### 4.2 Features Not to be Tested

- **DS query layer** (`pkg/apifrontend/ds/ogen_client.go`): No changes to DS communication
- **MCP bridge wiring** (`pkg/apifrontend/handler/mcp_bridge.go`): Existing IT-BRIDGE-007 covers wiring
- **DS-side event emission**: Out of scope; events are produced by other services

### 4.3 Design Decisions

| Decision | Rationale |
|----------|-----------|
| Static map over prompt-based interpretation | Deterministic, no LLM inference, extensible with one-line additions |
| Fallback to `Phase: "Unknown"` for unrecognized types | Fail-open for display; no crashes on new event types |
| Lifecycle built by deduplicating phases in order | Concise summary without repetition |

---

## 5. Approach

### 5.1 Coverage Policy

- **Unit**: >=80% of interpretation logic (map, enrichment, lifecycle builder)
- **Integration**: Not required — pure logic with no I/O; existing IT-BRIDGE-007 covers wiring

### 5.2 Two-Tier Minimum

Unit tier only. Integration coverage provided by existing IT-BRIDGE-007 (MCP dispatch to `HandleGetAuditTrail`).

### 5.3 Pass/Fail Criteria

**PASS**: All UT-AF-1286-* tests pass, existing UT-AF-124-* tests pass, >=80% coverage on new code.

**FAIL**: Any test failure, regression in existing tests, or coverage below 80%.

---

## 6. Test Items

### 6.1 Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/apifrontend/tools/ds_tools.go` | `eventDescriptions` map, `enrichEvent`, `buildLifecycleSummary`, `HandleGetAuditTrail` (enrichment additions) | ~120 new |

---

## 7. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| #1286 | Known event types map to correct phase/description | P0 | Unit | UT-AF-1286-001 | Pending |
| #1286 | Unknown event types get fallback | P0 | Unit | UT-AF-1286-002 | Pending |
| #1286 | Lifecycle summary from mixed events | P0 | Unit | UT-AF-1286-003 | Pending |
| #1286 | Empty events produce empty lifecycle | P1 | Unit | UT-AF-1286-004 | Pending |
| #1286 | Duplicate phases deduplicated | P1 | Unit | UT-AF-1286-005 | Pending |
| #1286 | All canonical event types covered | P0 | Unit | UT-AF-1286-006 | Pending |
| #1286 | Backward compatibility with enriched struct | P0 | Unit | UT-AF-1286-007 | Pending |

---

## 8. Test Scenarios

### Test ID Naming Convention

Format: `UT-AF-1286-{SEQUENCE}` — Unit, API Frontend, issue 1286.

### Tier 1: Unit Tests

**Testable code scope**: `pkg/apifrontend/tools/ds_tools.go` — interpretation map, enrichment, lifecycle builder.

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-AF-1286-001` | Known event type (e.g. `gateway.signal.received`) maps to correct phase ("Signal Processing") and description | Pending |
| `UT-AF-1286-002` | Unknown event type (e.g. `custom.unknown.event`) maps to phase "Unknown" with event_type as description | Pending |
| `UT-AF-1286-003` | Lifecycle summary from mixed events produces chronological, deduplicated phase string | Pending |
| `UT-AF-1286-004` | Empty events produce empty lifecycle and zero count | Pending |
| `UT-AF-1286-005` | Multiple events with same phase produce only one occurrence in lifecycle string | Pending |
| `UT-AF-1286-006` | Table-driven test: every canonical event type constant has a mapping entry | Pending |
| `UT-AF-1286-007` | Existing UT-AF-124-001..004 tests pass with enriched struct (backward compat) | Pending |

### Tier Skip Rationale

- **Integration**: Not applicable — interpretation is pure logic. Existing IT-BRIDGE-007 validates wiring.
- **E2E**: Not applicable — no new endpoints or infrastructure.

---

## 9. Test Cases

### UT-AF-1286-001: Known event type maps to correct phase and description

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/tools/kubernaut_get_audit_trail_test.go`

**Test Steps**:
1. **Given**: A mock DS client returns an event with `EventType: "gateway.signal.received"`
2. **When**: `HandleGetAuditTrail` is called
3. **Then**: The returned `AuditEvent` has `Phase: "Signal Processing"` and `Description: "Incoming signal received by gateway"`

### UT-AF-1286-002: Unknown event type gets fallback

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/tools/kubernaut_get_audit_trail_test.go`

**Test Steps**:
1. **Given**: A mock DS client returns an event with `EventType: "custom.unknown.event"`
2. **When**: `HandleGetAuditTrail` is called
3. **Then**: The returned `AuditEvent` has `Phase: "Unknown"` and `Description: "custom.unknown.event"`

### UT-AF-1286-003: Lifecycle summary from mixed events

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/tools/kubernaut_get_audit_trail_test.go`

**Test Steps**:
1. **Given**: Mock returns events: `gateway.signal.received`, `gateway.crd.created`, `aiagent.session.started`, `aiagent.rca.complete`, `aiagent.session.completed`
2. **When**: `HandleGetAuditTrail` is called
3. **Then**: `Lifecycle` equals `"Signal Processing -> Investigation -> Investigation"` (deduplicated: `"Signal Processing -> Investigation"`)

### UT-AF-1286-004: Empty events produce empty lifecycle

**Priority**: P1
**Type**: Unit
**File**: `pkg/apifrontend/tools/kubernaut_get_audit_trail_test.go`

**Test Steps**:
1. **Given**: Mock returns empty event list
2. **When**: `HandleGetAuditTrail` is called
3. **Then**: `Lifecycle` is empty string, `Count` is 0

### UT-AF-1286-005: Duplicate phases deduplicated in lifecycle

**Priority**: P1
**Type**: Unit
**File**: `pkg/apifrontend/tools/kubernaut_get_audit_trail_test.go`

**Test Steps**:
1. **Given**: Mock returns 3 events all with `aiagent.*` types (same "Investigation" phase)
2. **When**: `HandleGetAuditTrail` is called
3. **Then**: `Lifecycle` contains only one "Investigation" entry

### UT-AF-1286-006: All canonical event types covered

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/tools/kubernaut_get_audit_trail_test.go`

**Test Steps**:
1. **Given**: A table of all canonical event type strings from source constants
2. **When**: Each is looked up in `eventDescriptions`
3. **Then**: Every constant has a non-empty Phase and Description; none fall through to "Unknown"

### UT-AF-1286-007: Backward compatibility

**Priority**: P0
**Type**: Unit
**File**: `pkg/apifrontend/tools/kubernaut_get_audit_trail_test.go`

**Test Steps**:
1. **Given**: Existing UT-AF-124-001..004 test scenarios
2. **When**: Tests run against enriched `AuditEvent` and `GetAuditTrailResult` structs
3. **Then**: All existing assertions pass; new `Phase`, `Description`, `Lifecycle` fields do not break existing behavior

---

## 10. Environmental Needs

### 10.1 Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: `ds.MockClient` (existing, external dependency)
- **Location**: `pkg/apifrontend/tools/kubernaut_get_audit_trail_test.go`

---

## 11. Dependencies & Schedule

### 11.1 Execution Order

1. **Phase 1 (RED)**: Write UT-AF-1286-001..007 failing tests
2. **Phase 2 (GREEN)**: Implement `eventDescriptions` map, enrich `AuditEvent`, add `Lifecycle` to result
3. **Phase 3 (REFACTOR)**: Extract helpers, validate against 100 Go Mistakes
4. **CHECKPOINT A**: Build passes, all tests green, >=80% coverage

---

## 12. Test Deliverables

| Deliverable | Location | Description |
|-------------|----------|-------------|
| This test plan | `docs/tests/1286/TEST_PLAN.md` | Strategy and test design |
| Unit test suite | `pkg/apifrontend/tools/kubernaut_get_audit_trail_test.go` | Ginkgo BDD tests |

---

## 13. Execution

```bash
# Unit tests
go test ./pkg/apifrontend/tools/... -ginkgo.v -ginkgo.focus="UT-AF-1286"

# All audit trail tests (including existing)
go test ./pkg/apifrontend/tools/... -ginkgo.v -ginkgo.focus="kubernaut_get_audit_trail"

# Coverage
go test ./pkg/apifrontend/tools/... -coverprofile=coverage.out -ginkgo.focus="UT-AF-1286"
go tool cover -func=coverage.out
```

---

## 14. Wiring Verification (TDD Phase 4)

| Code Path | Entry Point | Exit Point | Wiring IT | Status |
|-----------|-------------|------------|-----------|--------|
| `HandleGetAuditTrail` enrichment | MCP `kubernaut_get_audit_trail` tool call | JSON response with `Phase`, `Description`, `Lifecycle` | IT-BRIDGE-007 (existing) | Pass (no new wiring) |

---

## 15. Existing Tests Requiring Updates

| Test ID / Location | Current Assertion | Required Change | Reason |
|-------------------|-------------------|-----------------|--------|
| UT-AF-124-001..004 | Assert on `Count`, `err`, `EventType` | No change required | New fields are additive; existing assertions remain valid |

---

## 16. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-05-25 | Initial test plan |
