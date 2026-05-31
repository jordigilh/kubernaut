# Test Plan — #1310: Investigation Failure Transparency and Recovery

**IEEE 829 Compliant** | **Issue**: [#1310](https://github.com/jordigilh/kubernaut/issues/1310) | **Depends on**: TP-1307-MERGE-INVESTIGATE

## 1. Test Plan Identifier

TP-1310-INVESTIGATION-TRANSPARENCY

## 2. Introduction

After deploying the `kubernaut_investigate` tool merge (#1307) on OCP, the AF agent
completed an investigation but the user perceived a "stall" because: (a) KA tool
errors (`kubectl_describe Node` resolving to `config.openshift.io` instead of core
`v1`) were silently absorbed, (b) the A2A EventBridge emitted zero progressive error
events, (c) `InvestigateResult` carried no error context for the LLM to explain the
degraded outcome, and (d) AF emitted zero info-level log lines between stream open
and close.

This test plan covers the transparency and recovery improvements that surface tool
failures to the user in real-time and provide the LLM with error context.

**Business Requirements**:
- BR-TRANSPARENCY-001: Tool errors during KA investigation MUST be bridged to the user via A2A EventBridge in real-time
- BR-TRANSPARENCY-002: `InvestigateResult` MUST include accumulated tool errors so the LLM can explain degraded outcomes
- BR-TRANSPARENCY-003: Investigation-level error events from KA MUST be bridged to the user (not silently absorbed)
- BR-TRANSPARENCY-004: AF MUST log tool call start/completion at `info` level for operator observability
- BR-TRANSPARENCY-005: AF MUST log investigation stream lifecycle (events bridged, errors encountered) at `info` level

**FedRAMP Control Mapping**:
| Control | Objective | Test Coverage |
|---------|-----------|---------------|
| AU-2    | Auditable events defined | Tool errors are auditable events |
| AU-3    | Audit record content | Error messages contain tool name, error detail, session context |
| AU-12   | Audit generation | Tool errors logged and bridged at every occurrence |
| SI-4    | Information system monitoring | Progressive error surfacing via EventBridge |

## 3. Test Items

| Item | File | Description |
|------|------|-------------|
| `extractToolResult` | `pkg/apifrontend/tools/ka_stream.go` | Extracts tool name, preview, and error status from SSE `tool_result` events |
| `formatToolError` | `pkg/apifrontend/tools/ka_stream.go` | Formats human-readable error message from tool name + error text |
| `streamInvestigation` (enhanced) | `pkg/apifrontend/tools/ka_investigate.go` | Error bridging, ToolErrors accumulation, stream lifecycle logging |
| `InvestigateResult.ToolErrors` | `pkg/apifrontend/tools/ka_investigate.go` | New field carrying accumulated tool errors for LLM |
| `newToolLoggingCallbacks` | `pkg/apifrontend/agent/root.go` | Info-level tool call start/complete logging |

## 4. Features to Be Tested

### 4.1 Tool Result Extraction and Error Detection (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1310-010 | BR-TRANSPARENCY-001 | AU-3 | `extractToolResult` with production KA envelope (error) | `toolName == "kubectl_describe"`, `isErr == true`, preview contains error |
| UT-AF-1310-011 | BR-TRANSPARENCY-001 | — | `extractToolResult` with test flat format (success) | `toolName == ""`, `isErr == false` |
| UT-AF-1310-012 | BR-TRANSPARENCY-001 | — | `extractToolResult` with production KA envelope (success) | `isErr == false`, `toolName` extracted |
| UT-AF-1310-013 | BR-TRANSPARENCY-001 | AU-3 | `formatToolError` produces human-readable, truncated message | Starts with tool name, length <= 200 |
| UT-AF-1310-014 | BR-TRANSPARENCY-001 | — | `formatToolError` extracts error from `toolErrorJSON` format | Message contains human error, not raw JSON |
| UT-AF-1310-015 | BR-TRANSPARENCY-001 | — | `extractToolResult` with flat JSON error string | `isErr == true` when string contains `"status":"error"` |

### 4.2 Error Bridging During Investigation Streaming (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1310-001 | BR-TRANSPARENCY-001 | SI-4 | tool_result with error IS bridged via EventBridge | Bridged texts contain `[Error: kubectl_describe` |
| UT-AF-1310-002 | BR-TRANSPARENCY-001 | SI-4 | tool_result with success is NOT bridged as error | Bridged texts do NOT contain `[Error:` |
| UT-AF-1310-003 | BR-TRANSPARENCY-002 | AU-3 | Multiple tool errors accumulated in ToolErrors | `len(result.ToolErrors) == 2`, both tool names present |
| UT-AF-1310-004 | BR-TRANSPARENCY-002 | — | Zero tool errors means ToolErrors is empty | `result.ToolErrors` is nil/empty |
| UT-AF-1310-005 | BR-TRANSPARENCY-003 | SI-4 | Error event bridged with "[Investigation error: ...]" | Bridged texts contain `[Investigation error:` |
| UT-AF-1310-006 | BR-TRANSPARENCY-002 | AU-12 | ToolErrors populated on disconnected stream | `result.ToolErrors` carries error, `result.Status == "disconnected"` |

### 4.3 Tool Call Logging Callbacks (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1310-020 | BR-TRANSPARENCY-004 | AU-12 | Before callback logs tool name at info | Log output contains tool name |
| UT-AF-1310-021 | BR-TRANSPARENCY-004 | AU-12 | After callback logs tool name, duration, result | Log output contains tool name, duration > 0, result label |

### 4.4 Wiring Tests (WT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| WT-AF-1310-030 | BR-TRANSPARENCY-004 | — | NewRootAgent includes logging callbacks | Agent creation succeeds with logging wired |
| WT-AF-1310-031 | BR-TRANSPARENCY-002 | — | ToolErrors field is JSON-serializable with omitempty | Empty ToolErrors omitted from JSON output |

### 4.5 Integration Tests (IT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-AF-1310-050 | BR-TRANSPARENCY-001 | SI-4 | Full A2A path: investigation with tool error bridges error event | EventBridge queue contains `[Error:` artifact |
| IT-AF-1310-051 | BR-TRANSPARENCY-002 | AU-3 | MCP bridge: investigation with tool error returns ToolErrors | JSON response contains `tool_errors` array |

## 5. Features Not Tested

- LLM actually explaining tool errors to the user (non-deterministic; relies on prompt + ToolErrors in result)
- KA-side validation gate parse failure (separate KA issue)
- kagenti client-side timeout behavior (external client)
- ADK-path per-tool timeout (separate P2 item, not triggered in reproduction)

## 6. Approach

**Testing Pyramid Invariant**: UT proves error detection and bridging logic. WT proves callback wiring. IT proves end-to-end error flow through A2A/MCP stacks.

### 6.1 TDD Red Phase
Write all failing tests before implementing production code. Tests use existing
helpers (`spyEmitter`, `testEventQueue`, `sseText`, `sseObj`) plus new
`sseToolResult` helper that emits production-format KA envelope.

### 6.2 TDD Green Phase
Implement minimal production code to pass all tests:
1. `ka_stream.go` — `extractToolResult`, `formatToolError`
2. `ka_investigate.go` — error tracking, bridging, lifecycle logging
3. `root.go` — `newToolLoggingCallbacks`

### 6.3 TDD Refactor Phase
1. Validate against 100-go-mistakes (shadowing, builder misuse, context handling)
2. Ensure no duplication with KA-side `toolErrorJSON` pattern
3. Build + lint + race validation

## 7. Pass/Fail Criteria

- All UT/WT/IT tests pass with >= 80% coverage of modified files
- `go build ./...` succeeds
- `go test -race ./...` succeeds (zero data races)
- `golangci-lint run --timeout=5m` reports zero new issues
- Existing TP-1307-MERGE tests still pass (backward compatibility)
- EventBridge receives `[Error: ...]` artifacts for tool failures
- InvestigateResult.ToolErrors populated for failed tools, empty for successful ones

## 8. Suspension / Resumption

Suspend if:
- KA SSE wire format changes (tool_result envelope structure)
- ADK callback API changes
- A2A EventBridge API changes

Resume after updating extraction/bridging logic to match new contracts.

## 9. Environmental Needs

- Go 1.23+, ADK v1.2.0
- Mock HTTP server for KA SSE stream simulation (existing `kaInvestigateHandler`)
- Mock `audit.Emitter` (`spyEmitter`)
- Mock `eventqueue.Writer` (`testEventQueue`)
- `logr.Logger` test implementation (`testr.New`)

## 10. Traceability Matrix

| Business Requirement | Test IDs |
|---------------------|----------|
| BR-TRANSPARENCY-001 | UT-AF-1310-001, UT-AF-1310-002, UT-AF-1310-010..015, IT-AF-1310-050 |
| BR-TRANSPARENCY-002 | UT-AF-1310-003, UT-AF-1310-004, UT-AF-1310-006, WT-AF-1310-031, IT-AF-1310-051 |
| BR-TRANSPARENCY-003 | UT-AF-1310-005 |
| BR-TRANSPARENCY-004 | UT-AF-1310-020, UT-AF-1310-021, WT-AF-1310-030 |
| BR-TRANSPARENCY-005 | UT-AF-1310-001 (stream summary logging verified via test logger) |
