# Test Plan: Fix Target Display Format (Issue #1492)

**Service**: apifrontend
**Version**: 1.0
**Created**: 2026-06-25
**Author**: AI Assistant
**Status**: Approved
**Business Requirement**: BR-API-1492

---

## 1. Purpose

Validate that all code paths emitting the `target` field in SSE metadata and
RRContext use `Kind/Name` format (e.g. `"Deployment/worker"`) instead of bare
resource names (e.g. `"worker"`), eliminating the console `namespace/name`
display bug reported in kubernaut-console#22.

## 2. Objectives

- All 4 code paths emit `target` in `Kind/Name` format via `FormatResourceDisplay`
- Backward compatibility: empty kind degrades gracefully to bare name
- No regressions in existing status subscription or event bridge tests

## 3. Success Metrics

- All UT and IT tests pass
- Existing `UT-AF-1423-*` event bridge tests remain green
- `go build ./...` clean

## 4. Scope

### In Scope

- `BuildPhaseMetadata` target format in `status_types.go`
- `RRContext.Target` in `af_investigate_alert.go`, `ka_investigate_mcp.go`, `ka_remediate.go`
- IT test proving `Kind/Name` flows through EventBridge to SSE metadata

### Out of Scope

- Console-side changes (`kubernaut-console` repo)
- `FormatResourceDisplay` helper itself (already tested by `UT-RO-635-*`)

## 5. FedRAMP / SOC2 Control Mapping

| Control | Relevance | Verification |
|---------|-----------|--------------|
| AU-3 (Audit Content) | `target` field in audit events must unambiguously identify the resource | UT-AF-1468-001 (updated) |
| SC-7 (Boundary Protection) | Server-sourced `Kind/Name` prevents client-side guessing | IT-AF-1492-001 |
| SI-4 (System Monitoring) | Status events carry correct resource display for monitoring | UT-AF-1468-003 (updated) |

## 6. BR Coverage Matrix

| BR ID | Description | Priority | Test Type | Test ID | Status |
|-------|-------------|----------|-----------|---------|--------|
| BR-API-1492 | BuildPhaseMetadata target = Kind/Name (namespaced) | P0 | Unit | UT-AF-1468-001 | Pending |
| BR-API-1492 | BuildPhaseMetadata target = Kind/Name (cluster-scoped) | P0 | Unit | UT-AF-1468-002 | Pending |
| BR-API-1492 | BuildPhaseMetadata target coexists with phase fields | P0 | Unit | UT-AF-1468-003 | Pending |
| BR-API-1492 | Kind/Name target flows through RRContext to SSE metadata | P0 | Integration | IT-AF-1492-001 | Pending |

## 7. Pyramid Invariant

> UT proves logic. IT proves wiring. E2E proves the journey.

- **UT**: `BuildPhaseMetadata` produces `Kind/Name` format (3 existing tests updated)
- **IT**: `RRContext` with `Kind/Name` target propagates through `EventBridge` to SSE metadata (IT-AF-1492-001)
- **E2E**: Existing `E2E-AF-1398-001` covers the SSE status journey; no new E2E needed

## 8. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `BuildPhaseMetadata` target | SSE status subscription | `status_types.go:70` | UT-AF-1468-001 |
| `RRContext.Target` Kind/Name | `SetRRContextSafe` -> `EventBridge` | `af_investigate_alert.go:190` | IT-AF-1492-001 |
| `RRContext.Target` Kind/Name | `SetRRContextSafe` -> `EventBridge` | `ka_investigate_mcp.go:238` | IT-AF-1492-001 |
| `RRContext.Target` Kind/Name | `SetRRContextSafe` -> `EventBridge` | `ka_remediate.go:95` | IT-AF-1492-001 |

## 9. TDD Execution Order

1. **Cycle 1** (UT): Update `UT-AF-1468-001/002/003` to expect `Kind/Name`, then fix `BuildPhaseMetadata`
2. **Cycle 2** (IT): Write `IT-AF-1492-001` proving `Kind/Name` flows through `EventBridge`, then update tool handlers
3. **Refactor**: Verify build, run full test suites

## 10. Test Scenarios

### 10.1 Updated Unit Tests

| Test ID | Description | Input | Expected `target` |
|---------|-------------|-------|--------------------|
| UT-AF-1468-001 | Namespaced resource | Kind=Deployment, Name=worker | `"Deployment/worker"` |
| UT-AF-1468-002 | Cluster-scoped resource | Kind=Node, Name=node-1 | `"Node/node-1"` |
| UT-AF-1468-003 | Coexists with phase fields | Kind=Deployment, Name=api-server | `"Deployment/api-server"` |

### 10.2 New Integration Test

| Test ID | Description | Input | Expected Output |
|---------|-------------|-------|-----------------|
| IT-AF-1492-001 | Kind/Name target propagates through RRContext -> EventBridge -> SSE metadata | SetRRContext with Target="Deployment/api-frontend" | SSE metadata["target"] == "Deployment/api-frontend" |

## 11. Risks

| Risk | Mitigation |
|------|-----------|
| Console may parse `target` expecting bare name | Console already reported the bug; `Kind/Name` is the expected format |
| `FormatResourceDisplay` with empty kind returns bare name | Tested by existing UT-RO-635-002a; graceful degradation |

## 12. Environment

- Unit tests: `go test ./pkg/apifrontend/handler/...`
- Integration tests: `go test ./pkg/apifrontend/launcher/...`
- Tool handler build: `go build ./pkg/apifrontend/tools/...`
