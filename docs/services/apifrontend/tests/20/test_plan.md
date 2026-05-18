# Test Plan: Native DS Tools (Issue #20)

**Test Plan Identifier:** TP-AF-020
**Version:** 1.0
**Date:** 2026-05-04
**Author:** AI Assistant

---

## 1. Introduction

This test plan covers the 4 Data Store-native MCP tools that query the kubernaut Data Store (DS) via its OpenAPI REST API. These tools support the L3 Audit / Forensic Analyst persona and provide historical remediation data, workflow catalog, effectiveness scores, and audit trails.

## 2. Test Items

- `internal/tools/kubernaut_list_workflows.go`
- `internal/tools/kubernaut_get_remediation_history.go`
- `internal/tools/kubernaut_get_effectiveness.go`
- `internal/tools/kubernaut_get_audit_trail.go`
- `internal/ds/client.go` -- DSClient interface + mock

## 3. Features to Be Tested

| Feature | Source | Priority |
|---------|--------|----------|
| List workflow catalog | Issue #20 | P0 |
| Query remediation history with filters | Issue #20 | P0 |
| Get effectiveness scores | Issue #20 | P1 |
| Get audit trail for RR | Issue #20, #25 | P0 |
| DS unavailability handling | ARCHITECTURE.md resilience | P0 |
| Filter parameter passthrough | Issue #20 | P1 |
| Empty result handling | Defensive | P1 |
| Mock/real swap boundary | ADR-009 | P1 |

## 4. Approach

- **Mock DS client:** `DSClient` interface with mock implementation
- **Real DS client deferred to PR6:** ogen-generated client wired later
- **Error handling:** Circuit breaker pattern (same as KA REST client)
- **4 tests per tool:** happy path, empty, unavailable, filter params

## 5. Pass/Fail Criteria

- 16 specs pass with `ginkgo -race`
- Coverage >= 80%
- `DSClient` interface clean (no ogen types leaked)
- Mock implementation sufficient for all test scenarios

## 6. Test Case Matrix

### kubernaut_list_workflows (4 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-121-001 | Returns workflow catalog | {} | Workflows list |
| UT-AF-121-002 | Empty catalog | {} (empty DS) | Empty list, no error |
| UT-AF-121-003 | DS unavailable | DS down | User-friendly error |
| UT-AF-121-004 | Filter by kind | {kind: "Deployment"} | Filtered results |

### kubernaut_get_remediation_history (4 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-122-001 | Returns history | {namespace: "pay"} | Historical RRs |
| UT-AF-122-002 | No history | {namespace: "empty"} | Empty list |
| UT-AF-122-003 | DS unavailable | DS down | User-friendly error |
| UT-AF-122-004 | Filter by date range | {since: "2026-01-01"} | Filtered results |

### kubernaut_get_effectiveness (4 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-123-001 | Returns scores | {workflow_id: "wf-1"} | Effectiveness data |
| UT-AF-123-002 | No data | {workflow_id: "unknown"} | Empty, no error |
| UT-AF-123-003 | DS unavailable | DS down | User-friendly error |
| UT-AF-123-004 | Filter by namespace | {namespace: "pay"} | Scoped results |

### kubernaut_get_audit_trail (4 specs)

| ID | Description | Input | Expected |
|----|-------------|-------|----------|
| UT-AF-124-001 | Returns audit events | {rr_id: "pay/rr-1"} | Audit chain |
| UT-AF-124-002 | No events | {rr_id: "pay/missing"} | Empty list |
| UT-AF-124-003 | DS unavailable | DS down | User-friendly error |
| UT-AF-124-004 | Filter by event type | {event_type: "approval"} | Filtered events |

## 7. Environmental Needs

- Mock `DSClient` interface (no real DS in unit tests)
- `httptest.NewServer` for integration-level DS mock (if needed)

## 8. Interface Contract

```go
type DSClient interface {
    ListWorkflows(ctx context.Context, opts ListWorkflowsOpts) ([]Workflow, error)
    GetRemediationHistory(ctx context.Context, opts HistoryOpts) ([]HistoricalRemediation, error)
    GetEffectiveness(ctx context.Context, opts EffectivenessOpts) (*EffectivenessReport, error)
    GetAuditTrail(ctx context.Context, opts AuditTrailOpts) ([]AuditEvent, error)
}
```

This interface is implemented by:
- `MockDSClient` (PR3, for testing)
- `OgenDSClient` (PR6, for production -- wraps kubernaut's ogen-generated client)
