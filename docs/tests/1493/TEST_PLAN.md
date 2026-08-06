# Test Plan: Fix RAR Resource ID Format Mismatch (Issue #1493)

**Service**: apifrontend
**Version**: 1.0
**Created**: 2026-06-24
**Author**: AI Assistant
**Status**: Approved
**Business Requirement**: BR-API-1493

---

## 1. Purpose

Validate that the `kubernaut_get_approval_request` MCP tool accepts bare RAR names
(as emitted by status subscription metadata) and that `BuildPhaseMetadata` emits
`approval_request_name` in `namespace/name` format for defense-in-depth.

## 2. Objectives

- Fix the semantic contract mismatch between metadata producer and tool consumer
- Maintain backward compatibility with `namespace/name` format in `rar_id`
- Ensure no regressions in existing approval request flows

## 3. Success Metrics

- All UT, IT tests pass
- Existing `UT-AF-109-*` tests remain green (backward compatibility)
- Existing `E2E-AF-1398-*` tests remain green (approval event journey)
- `go build ./...` clean, `golangci-lint` clean

## 4. Scope

### In Scope

- `ParseRARID` function: tolerant parser accepting bare names and `namespace/name`
- `HandleGetApprovalRequest`: wiring to use `ParseRARID`
- `BuildPhaseMetadata`: emit `namespace/name` in `approval_request_name`
- Removal of unused `ParseResourceID`

### Out of Scope

- Console-side changes (`kubernaut-console` repo)
- E2E test additions (existing `E2E-AF-1398-001` covers the journey)

## 5. FedRAMP / SOC2 Control Mapping

| Control | Relevance | Verification |
|---------|-----------|--------------|
| AU-3 (Audit Content) | `approval_request_name` metadata used in audit trails | UT-AF-1493-006 verifies namespace prefix for traceability |
| SC-7 (Boundary Protection) | Server-side namespace injection prevents client namespace spoofing | IT-AF-1493-001 proves injected namespace is used |
| SI-4 (System Monitoring) | Status events carry correct resource identifiers | IT-AF-1493-002 proves namespace/name in metadata |

## 6. BR Coverage Matrix

| BR ID | Description | Priority | Test Type | Test ID | Status |
|-------|-------------|----------|-----------|---------|--------|
| BR-API-1493 | ParseRARID: bare name resolution | P0 | Unit | UT-AF-1493-001 | Pending |
| BR-API-1493 | ParseRARID: namespace/name passthrough | P0 | Unit | UT-AF-1493-002 | Pending |
| BR-API-1493 | ParseRARID: fallback to explicit ns+name | P1 | Unit | UT-AF-1493-003 | Pending |
| BR-API-1493 | ParseRARID: error on empty inputs | P1 | Unit | UT-AF-1493-004 | Pending |
| BR-API-1493 | Handler accepts bare rar_id | P0 | Unit | UT-AF-1493-005 | Pending |
| BR-API-1493 | Metadata includes namespace prefix | P0 | Unit | UT-AF-1493-006 | Pending |
| BR-API-1493 | Bare rar_id wired through handler to K8s | P0 | Integration | IT-AF-1493-001 | Pending |
| BR-API-1493 | Namespace prefix in BuildPhaseMetadata | P0 | Integration | IT-AF-1493-002 | Pending |

## 7. Test Scenarios

### 7.1 Happy Path Tests

| Test ID | Description | Input | Expected Output |
|---------|-------------|-------|-----------------|
| UT-AF-1493-001 | Bare RAR name resolves with injected namespace | `rarID="rar-oom-1"`, `ns="payments"` | `("payments", "rar-oom-1", nil)` |
| UT-AF-1493-002 | namespace/name format passes through | `rarID="payments/rar-oom-1"` | `("payments", "rar-oom-1", nil)` |
| UT-AF-1493-003 | Empty rar_id uses explicit fallback | `rarID=""`, `ns="pay"`, `name="rar-1"` | `("pay", "rar-1", nil)` |
| UT-AF-1493-005 | Handler returns full RAR with bare rar_id | `RARID="rar-oom-1"`, `Namespace="payments"` | Full RAR result |
| IT-AF-1493-001 | Bare rar_id dispatches through handler to K8s | Same as UT-AF-1493-005 via production path | Full RAR result |

### 7.2 Error Handling Tests

| Test ID | Description | Input | Expected Output |
|---------|-------------|-------|-----------------|
| UT-AF-1493-004 | Empty rar_id + empty name | `rarID=""`, `name=""` | Error: "name is required" |

### 7.3 Metadata Tests

| Test ID | Description | Input | Expected Output |
|---------|-------------|-------|-----------------|
| UT-AF-1493-006 | AwaitingApproval emits namespace/name | RR with `Namespace="kubernaut-system"` | `"kubernaut-system/rar-<name>"` |
| IT-AF-1493-002 | Namespace prefix flows through status path | Same RR through BuildPhaseMetadata | `"kubernaut-system/rar-<name>"` |

## 8. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `ParseRARID` | `HandleGetApprovalRequest` | `crd_tools.go:341` | IT-AF-1493-001 |
| `BuildPhaseMetadata` ns prefix | SSE status subscription | `status_types.go:116` | IT-AF-1493-002 |

## 9. TDD Execution Order

1. **Cycle 1** (UT): `ParseRARID` logic in `helpers_test.go`
2. **Cycle 2** (IT+Wire): `HandleGetApprovalRequest` wiring in `kubernaut_get_approval_request_test.go`
3. **Cycle 3** (UT+IT): `BuildPhaseMetadata` namespace prefix in `status_types_test.go`
4. **Refactor**: Remove `ParseResourceID`, update `UT-AF-109-008`

## 10. Risks

| Risk | Mitigation |
|------|-----------|
| Console expects bare `approval_request_name` | Option C (tolerant tool) ensures bare names still work even if console hasn't updated |
| Other tools may use `ParseResourceID` in future | Confirmed zero callers; removal prevents future misuse |
| E2E `E2E-AF-1398-001` may assert on bare name | Check test assertions; update if needed |

## 11. Environment

- Unit tests: `go test ./pkg/apifrontend/...`
- Integration tests: `go test ./pkg/apifrontend/...` (same package, uses `fake.NewClientBuilder`)
- E2E: existing `test/e2e/apifrontend/structured_approval_e2e_test.go`
