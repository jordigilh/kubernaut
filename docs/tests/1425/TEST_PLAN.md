# Test Plan: #1425 Preserve Investigation Results Through Takeover

**Version**: 1.0
**Date**: 2026-06-14
**Issue**: [#1425](https://github.com/jordigilh/kubernaut/issues/1425)
**Business Requirements**: BR-INTERACTIVE-010
**Approach**: Root cause fix in session lifecycle (option B)

## Problem Statement

When a user takes over an MCP interactive session before the autonomous investigation
completes, the investigation goroutine's result is lost. Two bugs cause this:

1. `manager.go:198`: Investigation error path passes `nil` to `storePartialResult`
2. `store.go:284`: `SetResult` blocks ALL writes on UserDriving sessions (even first write)

This causes `discover_workflows` to permanently fail with "no conversation context available."

## FedRAMP Control Mapping

| Control | Requirement | Verified By |
|---------|-------------|-------------|
| SC-24 (Fail in Known State) | Investigation result preserved through takeover; session lifecycle does not discard results downstream consumers depend on | UT-KA-1425-001, UT-KA-1425-003, UT-KA-DW-012, IT-KA-1425-001 |
| SI-10 (Information Input Validation) | Workflow discovery uses authoritative enrichment from the preserved investigation result | UT-KA-DW-012, IT-KA-1425-001 |
| SI-13 (Predictable Failure Prevention) | First-write-wins guard handles the takeover race deterministically | UT-KA-1425-002, UT-KA-1425-003 |
| IR-4(1) (Automated Incident Handling) | Takeover does not permanently block workflow discovery; stored RCA found via normal preferred path | UT-KA-DW-012, IT-KA-1425-001, E2E FP-MCP-005 |
| AU-3 (Audit Records) | Normal code path preserves HTTP session_id and rr_id correlation in all audit events | IT-KA-1425-001 |

## Test Scenarios

### Unit Tests -- Session Lifecycle

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| UT-KA-1425-001 | Cancelled investigation preserves result through UserDriving | Investigation returns (result, ctx.Err()) after takeover | Result retrievable via GetLatestRCAResultByRemediationID | SC-24 |
| UT-KA-1425-002 | SetResult first-write-wins on UserDriving with nil result | UserDriving session with nil result + SetResult(non-nil) | Result written successfully | SI-13 |
| UT-KA-1425-003 | SetResult blocks overwrite on UserDriving with existing result | UserDriving session with non-nil result + SetResult(different) | Original result preserved | SC-24, SI-13 |

### Unit Tests -- discover_workflows Integration

| ID | Scenario | Input | Expected | FedRAMP |
|----|----------|-------|----------|---------|
| UT-KA-DW-012 | discover_workflows uses stored RCA from cancelled investigation | autoMgr returns stored RCA, no conversation context | Status = "workflows_discovered" using preferred path | SC-24, IR-4(1) |

### Integration Tests -- Takeover Wiring

| ID | Scenario | Method | Expected | FedRAMP |
|----|----------|--------|----------|---------|
| IT-KA-1425-001 | Takeover preserves result, discover_workflows succeeds | Real Manager + Lease + investigation goroutine + takeover + discover_workflows | Investigation result survives, workflows_discovered returned | SC-24, SI-10, IR-4(1), AU-3 |

### Integration Tests -- SetResult Guard (Updated)

| ID | Scenario | Method | Expected | FedRAMP |
|----|----------|--------|----------|---------|
| IT-KA-1351-STORE | First-write-wins + overwrite guard | UserDriving session: first SetResult accepted, second blocked | First write preserved, second rejected | KA-HIGH-4, #1425 |

### E2E Tests -- Journey (Existing, No Changes)

| ID | Scenario | Method | Expected | FedRAMP |
|----|----------|--------|----------|---------|
| FP-MCP-005 | Takeover -> discover_workflows -> select_workflow | Full Kind cluster, autonomous investigation + takeover | discover_workflows succeeds after takeover | SI-13 |

## Acceptance Criteria

1. All UT-KA-1425 tests pass (session lifecycle root cause fix)
2. UT-KA-DW-012 passes (discover_workflows uses stored RCA)
3. IT-KA-1425-001 passes (wiring proof through real Manager)
4. IT-KA-1351-STORE updated and passes (first-write-wins + overwrite protection)
5. No regressions in session suite (114 specs) or MCP tools suite (173 specs)
6. Full build passes (`go build ./...`)
7. Zero lint issues

## Production Changes

| File | Change | Lines |
|------|--------|-------|
| `internal/kubernautagent/session/manager.go` | Pass `result` instead of `nil` to `storePartialResult` on error path | L198 |
| `internal/kubernautagent/session/store.go` | First-write-wins: `StatusUserDriving && sess.Result != nil` guard | L284-286 |

## Future Option (Documented in Issue)

If edge cases emerge where the investigation goroutine terminates before producing
any result (not even partial), a defense-in-depth `RunFullInvestigation` fallback
can be added to `handleDiscoverWorkflows`. See issue #1425.
