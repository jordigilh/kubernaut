# Test Plan: Issue #1468 — status/subscribe RR Context Metadata

## 1. Test Plan Identifier

TP-AF-1468

## 2. References

- **Issue**: [#1468](https://github.com/jordigilh/kubernaut/issues/1468)
- **Business Requirement**: BR-API-1468
- **Design Decision**: DD-AF-008 (status/subscribe SSE protocol)
- **FedRAMP Controls**: AU-3, SI-4, SC-7, SI-10

## 3. Introduction

This test plan validates that the `status/subscribe` SSE endpoint includes RR
identity context metadata (namespace, target, kind, alert_name) in every
`status/update` event. This enables the console banner to populate on reconnect
to an in-progress investigation without requiring the A2A EventBridge path.

### Root Cause

`BuildPhaseMetadata()` in `pkg/apifrontend/handler/status_types.go` only reads
from `rr.Status.*` fields. The RR spec fields (`TargetResource.Namespace`,
`TargetResource.Name`, `TargetResource.Kind`, `SignalName`) are available in the
function's parameter but are never included in the metadata map.

### Fix

Add RR spec context fields as base fields in `BuildPhaseMetadata()`, before the
phase-specific switch block. Field names match the existing `RRContext` struct in
`event_bridge.go` for cross-path consistency.

## 4. Test Items

- `BuildPhaseMetadata()` in `pkg/apifrontend/handler/status_types.go`
- `handleSubscribe()` initial event in `pkg/apifrontend/handler/status_handler.go`
- `handleSubscribe()` watch loop events in `pkg/apifrontend/handler/status_handler.go`

## 5. Features to Be Tested

| Feature | FedRAMP Control | Description |
|---|---|---|
| RR spec context in metadata | AU-3 | Investigation identity fields present in every SSE event |
| Empty field omission | SI-10 | Empty spec fields not emitted as empty strings |
| Context + phase coexistence | AU-3 | Spec context and phase-specific fields coexist |
| Server-sourced context on reconnect | SC-7, SI-4 | Initial event delivers context from K8s API |
| Context across phase transitions | AU-3, SI-4 | Phase transition events preserve identity context |

## 6. Features Not to Be Tested

- `cluster` field: Not available in the RR CRD spec (out of scope)
- A2A EventBridge path: Already working via `RRContext`/`mergeRRContext()`
- Existing per-phase metadata: Covered by TP-AF-1460

## 7. Approach

### Pyramid Invariant Allocation

| Tier | What it proves | Tests |
|---|---|---|
| UT | `BuildPhaseMetadata()` logic: spec fields added, empty fields omitted, coexistence with phase fields | UT-AF-1468-001, UT-AF-1468-002, UT-AF-1468-003 |
| IT | Wiring: SSE events emitted by `handleSubscribe()` contain context from K8s CRD | IT-AF-1468-001, IT-AF-1468-002 |

### Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| BuildPhaseMetadata (spec context) | handleSubscribe() initial event | status_types.go:59 | IT-AF-1468-001 |
| BuildPhaseMetadata (spec context) | handleSubscribe() watch loop events | status_handler.go:209 | IT-AF-1468-002 |

## 8. Test Cases

### Unit Tests (pkg/apifrontend/handler/status_types_test.go)

#### UT-AF-1468-001: Metadata contains investigation identity fields (AU-3)

- **Objective**: Verify `BuildPhaseMetadata()` includes `namespace`, `target`, `kind`, `alert_name` sourced from RR spec
- **Input**: RR with populated `Spec.TargetResource` and `Spec.SignalName`, any non-terminal phase
- **Expected**: metadata map contains all 4 fields with correct values
- **FedRAMP**: AU-3 — audit record completeness

#### UT-AF-1468-002: Empty spec fields are omitted (SI-10)

- **Objective**: Verify empty spec fields are not emitted as empty strings
- **Input**: RR with empty `TargetResource.Namespace` (cluster-scoped resource)
- **Expected**: `namespace` key absent from metadata, other populated fields present
- **FedRAMP**: SI-10 — input validation hygiene at the boundary

#### UT-AF-1468-003: Spec context coexists with phase-specific fields (AU-3)

- **Objective**: Verify spec context fields do not interfere with phase-specific metadata
- **Input**: RR in Executing phase with workflow ref, populated spec
- **Expected**: metadata contains both spec context fields AND phase fields (`workflow_id`, `started_at`)
- **FedRAMP**: AU-3 — audit records carry both identity and state

### Integration Tests (test/integration/apifrontend/status_subscribe_test.go)

#### IT-AF-1468-001: Initial SSE event includes server-sourced context (SC-7, SI-4)

- **Objective**: Verify the first `status/update` event after `status/subscribe` includes RR spec context from K8s API
- **Setup**: Create RR with populated spec, connect to `status/subscribe`
- **Expected**: First SSE event metadata contains `namespace`, `target`, `kind`, `alert_name` matching the RR spec
- **FedRAMP**: SC-7 (server-sourced, not client-supplied), SI-4 (monitoring continuity)

#### IT-AF-1468-002: Phase transition events preserve context (AU-3, SI-4)

- **Objective**: Verify phase transition events include spec context alongside phase-specific metadata
- **Setup**: Create RR, subscribe, transition from Processing to Executing
- **Expected**: Executing event metadata contains both spec context AND `workflow_id`
- **FedRAMP**: AU-3 (traceability), SI-4 (continuous monitoring)

## 9. Pass/Fail Criteria

- All 5 test cases must pass
- No regressions in existing TP-AF-1460 tests
- `go build ./...` succeeds
- `golangci-lint run --timeout=5m` succeeds

## 10. Environmental Needs

- **UT**: Standard Go test environment with Ginkgo/Gomega
- **IT**: envtest with controller-runtime providing a real K8s API server

## 11. Risks and Contingencies

| Risk | Likelihood | Mitigation |
|---|---|---|
| Field name mismatch with console contract | Low | Using same names as `RRContext` in `event_bridge.go` |
| Backward incompatibility | None | `metadata` is `map[string]any`, additive keys are non-breaking |
| Existing test interference | Low | New fields are additive; existing assertions use `HaveKey` not `HaveLen` |
