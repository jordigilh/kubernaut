# Test Plan: Fix #1451 bridgeEventsCollectSummary Reports "completed" on Inactivity Timeout

**Issue**: [#1451](https://github.com/jordigilh/kubernaut/issues/1451)
**Service Type**: [x] Stateless HTTP API (API Frontend)
**Date**: 2026-06-26
**Status**: Active

---

## Business Requirements

| BR ID | Description |
|---|---|
| BR-AF-MCP-003 | Event bridge goroutine must accurately report investigation outcome |

## FedRAMP Control Objectives

| Control | Objective | How This Fix Maps |
|---------|-----------|-------------------|
| SI-4 (System Monitoring) | Status must accurately reflect investigation state | "timed_out" vs "completed" distinction prevents LLM confusion loops |
| AU-3 (Content of Audit Records) | Log entries must report correct status | Audit trail must not claim "completed" when investigation was abandoned due to inactivity |

---

## Test Scenario Naming Convention

**Format**: `{TIER}-{SERVICE}-1451-{SEQUENCE}`

- `AF` = API Frontend

---

## Component 1: bridgeEventsCollectSummary (Exit Reason)

### Unit Tests — Exit Path Accuracy (SI-4)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-AF-1451-001 | No events arrive within inactivity timeout | Third return value == `"inactivity_timeout"` — investigation hung, not completed |
| UT-AF-1451-002 | Events channel closes normally (KA session ended) | Third return value == `"channel_closed"` — natural completion |
| UT-AF-1451-003 | Context cancelled externally (SIGTERM / caller timeout) | Third return value == `"ctx_cancelled"` — external termination |

### Unit Tests — Timer Behavior (SI-4)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-AF-1451-007 | Default `BridgeInactivityTimeout` value | `BridgeInactivityTimeout == 180 * time.Second` |
| UT-AF-1451-008 | Inactivity timer resets on each received event | Event at T+170s resets timer; channel close at T+340s → exit reason `"channel_closed"` (not timeout) |

### Unit Tests — Data Preservation on Timeout (AU-3)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-AF-1451-009 | Summary text accumulated before inactivity timeout is preserved | Partial summary string returned; non-empty even on timeout |
| UT-AF-1451-010 | RCA extracted from events before inactivity timeout is preserved | `*InvestigateRCA` non-nil when RCA event arrived before timeout |

---

## Component 2: HandleInvestigationMCPWithRegistry Caller (Status Mapping)

### Unit Tests — Status Mapping (AU-3)

| ID | Scenario | Expected Outcome |
|---|---|---|
| UT-AF-1451-004 | Exit reason `"inactivity_timeout"` → status `"timed_out"` | `InvestigateMCPResult.Status == "timed_out"` — LLM told investigation is hung |
| UT-AF-1451-005 | Exit reason `"channel_closed"` → status `"completed"` | `InvestigateMCPResult.Status == "completed"` — only natural close reports completed |
| UT-AF-1451-006 | Exit reason `"ctx_cancelled"` → status `"timeout"` | `InvestigateMCPResult.Status == "timeout"` — external cancellation |

---

## Existing Tests to Update

The following tests call `BridgeEventsCollectSummary` and must handle the new third return value:

| ID | File | Required Change |
|---|---|---|
| IT-AF-1420-001 | ka_investigate_verdict_1420_test.go | Capture third return value (exit reason) |
| UT-AF-1438-010 | terminal_event_1438_test.go | Capture third return value |
| UT-AF-1438-027 | terminal_event_1438_test.go | Capture third return value |
| UT-AF-1396-020 | ka_investigate_rca_test.go | Capture third return value |
| UT-AF-1396-021 | ka_investigate_rca_test.go | Capture third return value |

---

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|---|---|---|---|
| Exit reason return value | HandleInvestigationMCPWithRegistry blocking path | pkg/apifrontend/tools/ka_investigate_mcp.go:445 | UT-AF-1451-004 |

Note: Signature change to an existing wired function. Existing IT tests (IT-AF-1420-001) already prove the wiring; new tests prove the logic change (exit reason to status mapping).

---

## Test Execution Summary

| Test Category | Tests | Status |
|---|---|---|
| AF Unit Tests — Exit Paths (UT-AF-1451-001..003) | 3 | Pending |
| AF Unit Tests — Timer Behavior (UT-AF-1451-007..008) | 2 | Pending |
| AF Unit Tests — Data Preservation (UT-AF-1451-009..010) | 2 | Pending |
| AF Unit Tests — Status Mapping (UT-AF-1451-004..006) | 3 | Pending |
| Existing Test Updates | 5 | Pending |
| **Total** | **15** | **Pending** |
