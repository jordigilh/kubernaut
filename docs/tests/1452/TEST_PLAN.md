# IEEE 829 Test Plan — Fix #1452: AF Session ID Forwarding to KA

| Field                | Value                                                                              |
|----------------------|------------------------------------------------------------------------------------|
| **Test Plan ID**     | TP-1452                                                                            |
| **Revision**         | 1.0                                                                                |
| **Author**           | Kubernaut AI Agent                                                                 |
| **Date**             | 2026-06-17                                                                         |
| **Status**           | Active                                                                             |
| **Business Req**     | BR-INTERACTIVE-010 (SC-2, SC-8)                                                   |
| **FedRAMP Controls** | AU-3, SI-4, AC-4, SC-8                                                             |

## 1. Introduction

This test plan covers the verification of the fix for GitHub issue #1452, where
AF's `HandleInvestigationMCPWithRegistry` discards the KA session ID returned
by `HandleAwaitSession` and calls `StartInvestigation` without forwarding it.

Root cause: `HandleAwaitSession` successfully polls the AIA CRD and obtains the
KA session ID (`awaitResult.SessionID`), but the code only checks
`awaitResult.Status == "ready"` for a log message. The session ID is never
captured or passed to `mcpClient.StartInvestigation`. KA receives
`action=start` without a session reference and either:
- Creates a **new** session (Jump-In path: wrong EventLogBridge target)
- Creates a **fallback** session (#1440 path: empty session, no events)

AF listens on the wrong session, receives no investigation events, hits the 60s
inactivity timeout, and reports a false completion with `summary_len=0`.

Fix: Thread the KA session ID from `HandleAwaitSession` through
`StartInvestigationArgs` → `SDKMCPClient` MCP argsMap → KA `InvestigateInput`
→ `handleStart` direct session lookup.

## 2. Scope

### In Scope

| Area                                   | Description                                                         |
|----------------------------------------|---------------------------------------------------------------------|
| AF `HandleInvestigationMCPWithRegistry`| Capture `awaitResult.SessionID`, forward to `StartInvestigation`    |
| AF `StartInvestigationArgs`            | Add `SessionID` field                                               |
| AF `SDKMCPClient.StartInvestigation`   | Include `session_id` in MCP argsMap when present                    |
| KA `InvestigateInput`                  | Add `SessionID` field                                               |
| KA `handleStart`                       | Prefer AF-provided session ID for direct lookup over RRID scan      |

### Out of Scope

- HandleAwaitSession polling logic itself (works correctly, returns the right ID)
- EventLogBridge wiring (verified by spike: works when `InvestigationSessionID` is populated)
- #1440 fallback session creation (separate fix, already merged)
- KA `handleTakeover` action (separate code path, not affected)

## 3. Business Requirements Traceability

| BR                    | SC   | Description                                                        | Violated By |
|-----------------------|------|--------------------------------------------------------------------|-------------|
| BR-INTERACTIVE-010    | SC-2 | KA MCP `action=start` detects a pending session and activates it   | Bug: AF omits session_id from `action=start` |
| BR-INTERACTIVE-010    | SC-8 | AF connects to KA via MCP `action=start` **once session exists**   | Bug: AF discards the session ID it waited for |

## 4. FedRAMP / NIST 800-53 Rev 5 Control Verification

Tests verify **business-level behavior** tied to each control objective.

### AU-3 — Content of Audit Records

**Business behavior**: The audit delegation event emitted after
`StartInvestigation` MUST include the KA session ID that AF connected to. When
AF forwards the correct session ID, the audit event's `ka_correlation_id` matches
the investigation session, enabling end-to-end traceability from AF's interactive
request through KA's investigation lifecycle.

**Tests**: UT-AF-1452-006

### SI-4 — System Monitoring

**Business behavior**: AF's event bridge MUST receive investigation events from
the correct KA session. When `session_id` is forwarded, KA's `handleStart`
locates the correct pending/running session deterministically — the
`EventLogBridge` subscribes to the right session's event channel, and AF's
`bridgeEventsCollectSummary` collects the RCA summary without inactivity
timeout.

**Tests**: UT-AF-1452-001, UT-AF-1452-002, IT-AF-1452-001

### AC-4 — Information Flow Enforcement

**Business behavior**: Investigation events MUST flow through the authenticated
MCP session that AF established for this specific investigation. Forwarding the
session ID prevents events from being routed to a stale or unrelated session,
enforcing the principle that information flows only within the authorized
investigation context.

**Tests**: UT-KA-1452-001, UT-KA-1452-002

### SC-8 — Transmission Confidentiality and Integrity

**Business behavior**: The KA session ID transmitted from AIA CRD through AF to
KA MUST arrive unmodified. The forwarded session ID is the same value that AA
wrote to the AIA CRD after KA accepted the investigation submission. Any
modification or loss of this identifier breaks the session correlation chain.

**Tests**: UT-AF-1452-003, UT-AF-1452-004

## 5. Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| `StartInvestigationArgs.SessionID` | `HandleInvestigationMCPWithRegistry` → `mcpClient.StartInvestigation` | `pkg/apifrontend/tools/ka_investigate_mcp.go` | IT-AF-1452-001 |
| `SDKMCPClient` session_id in argsMap | `StartInvestigation` → MCP `CallTool` | `pkg/apifrontend/ka/mcp_sdk_client.go` | IT-AF-1452-001 |
| `InvestigateInput.SessionID` | `handleStart` session lookup | `internal/kubernautagent/mcp/tools/investigate.go` | IT-KA-1452-001 |

## 6. Test Scenarios

### 6.1 Unit Tests — AF Side (`pkg/apifrontend/`)

Tests colocated with production code per kubernaut convention.

#### UT-AF-1452-001: Session ID forwarded when HandleAwaitSession returns ready

**File**: `pkg/apifrontend/tools/ka_investigate_mcp_test.go`

**Precondition**: `HandleAwaitSession` returns `Status: "ready"`, `SessionID: "ka-sess-1452-001"`

**Action**: Call `HandleInvestigationMCPWithRegistry` with blocking=true, mock K8s client seeded with AIA CRD containing `kaSession.id`

**Assertions**:
- `MockMCPClient.StartInvestigationFn` receives `args.SessionID == "ka-sess-1452-001"`
- Result contains the correct session ID in the response

**FedRAMP**: SI-4 (event bridge targets the correct investigation session)

#### UT-AF-1452-002: Session ID empty when HandleAwaitSession times out

**File**: `pkg/apifrontend/tools/ka_investigate_mcp_test.go`

**Precondition**: `HandleAwaitSession` returns `Status: "timeout"` (no AIA CRD with session ID)

**Action**: Call `HandleInvestigationMCPWithRegistry` with blocking=true, mock K8s client with no AIA CRD

**Assertions**:
- `MockMCPClient.StartInvestigationFn` receives `args.SessionID == ""`
- Function does not error (graceful degradation)

**FedRAMP**: SI-4 (no false session correlation on timeout)

#### UT-AF-1452-003: StartInvestigationArgs.SessionID included in MCP argsMap

**File**: `pkg/apifrontend/ka/start_investigation_test.go`

**Precondition**: `StartInvestigationArgs{RRID: "rr-1452", SessionID: "ka-sess-1452"}`

**Action**: Call `SDKMCPClient.StartInvestigation` against a mock MCP server

**Assertions**:
- MCP `CallTool` arguments contain `"session_id": "ka-sess-1452"`
- MCP `CallTool` arguments contain `"action": "start"`
- MCP `CallTool` arguments contain `"rr_id": "rr-1452"`

**FedRAMP**: SC-8 (session ID transmitted unmodified in MCP payload)

#### UT-AF-1452-004: SessionID omitted from MCP argsMap when empty

**File**: `pkg/apifrontend/ka/start_investigation_test.go`

**Precondition**: `StartInvestigationArgs{RRID: "rr-1452", SessionID: ""}`

**Action**: Call `SDKMCPClient.StartInvestigation` against a mock MCP server

**Assertions**:
- MCP `CallTool` arguments do NOT contain `"session_id"` key
- Behavior matches current production (no regression)

**FedRAMP**: SC-8 (no spurious empty-string session ID in protocol messages)

#### UT-AF-1452-005: Full blocking path captures and forwards session ID

**File**: `pkg/apifrontend/tools/ka_investigate_mcp_test.go`

**Precondition**: Fake K8s client with AIA CRD containing `kaSession.id: "ka-sess-e2e"`. MockMCPClient returns `Events` channel with a `complete` event.

**Action**: Call `HandleInvestigationMCPWithRegistry` with blocking=true

**Assertions**:
- `MockMCPClient.StartInvestigationFn` received `args.SessionID == "ka-sess-e2e"`
- Result `Status == "completed"` (investigation bridged successfully)
- Result `Summary != ""` (events received from correct session)

**FedRAMP**: SI-4 + AU-3 (correct session → correct events → correct summary)

#### UT-AF-1452-006: Audit delegation event references correct KA session

**File**: `pkg/apifrontend/tools/ka_investigate_mcp_test.go`

**Precondition**: Fake K8s client seeded, MockMCPClient with `SessionID: "ka-audit-1452"`, mock auditor

**Action**: Call `HandleInvestigationMCPWithRegistry` with blocking=true

**Assertions**:
- Audit event type == `EventKADelegated`
- Audit detail `ka_correlation_id == "ka-audit-1452"`
- Audit detail `delegation_type == "interactive"`

**FedRAMP**: AU-3 (audit records contain correct session correlation)

### 6.2 Unit Tests — KA Side (`internal/kubernautagent/`)

#### UT-KA-1452-001: handleStart uses AF-provided session_id for pending lookup

**File**: `internal/kubernautagent/mcp/tools/interactive_start_test.go`

**Precondition**: `InvestigateInput{RRID: "rr-ka-1452", Action: "start", SessionID: "pending-sess-1452"}`. AutoMgr has a pending session with ID `"pending-sess-1452"`.

**Action**: Call `InvestigateTool.Handle`

**Assertions**:
- `LaunchDeferredInvestigation` called with `"pending-sess-1452"` (direct, no RRID scan)
- `InvestigationSessionID == "pending-sess-1452"`
- `Status == "started"`

**FedRAMP**: AC-4 (session located by AF-provided ID, not RRID scan — deterministic routing)

#### UT-KA-1452-002: handleStart falls back to RRID scan when session_id not provided

**File**: `internal/kubernautagent/mcp/tools/interactive_start_test.go`

**Precondition**: `InvestigateInput{RRID: "rr-ka-fallback", Action: "start", SessionID: ""}`. AutoMgr has a pending session for RRID `"rr-ka-fallback"` with ID `"found-by-scan"`.

**Action**: Call `InvestigateTool.Handle`

**Assertions**:
- `FindPendingByRemediationID` called with `"rr-ka-fallback"`
- `LaunchDeferredInvestigation` called with `"found-by-scan"`
- `InvestigationSessionID == "found-by-scan"`

**FedRAMP**: AC-4 (RRID scan fallback preserves existing behavior)

#### UT-KA-1452-003: handleStart with session_id pointing to non-pending session (Jump-In)

**File**: `internal/kubernautagent/mcp/tools/interactive_start_test.go`

**Precondition**: `InvestigateInput{SessionID: "running-sess-1452"}`. AutoMgr's `LaunchDeferredInvestigation("running-sess-1452")` fails (session not pending). `FindByRemediationID` returns `"running-sess-1452"`.

**Action**: Call `InvestigateTool.Handle`

**Assertions**:
- Falls through to `UpgradeToInteractive("running-sess-1452")`
- `InvestigationSessionID == "running-sess-1452"`

**FedRAMP**: AC-4 (upgrade path works with AF-provided session ID)

### 6.3 Integration Tests

#### IT-AF-1452-001: Full AF→KA session ID forwarding through MCP protocol

**File**: `test/integration/apifrontend/investigation_session_handoff_test.go`

**Precondition**: Fake K8s client with AIA CRD seeded with `kaSession.id`. MCP test server with `kubernaut_investigate` tool registered.

**Action**: Call `HandleInvestigationMCPWithRegistry` in blocking mode. The mock MCP server verifies that the `session_id` argument arrives in the `CallTool` request.

**Assertions**:
- MCP server receives `action=start` with `session_id` matching the AIA CRD value
- AF receives events from the correct session (bridge functional)
- Investigation completes without inactivity timeout

**FedRAMP**: SI-4 + SC-8 (session ID flows AF → MCP transport → KA tool handler without modification)

#### IT-KA-1452-001: KA handleStart wires EventLogBridge to AF-provided session

**File**: `test/integration/kubernautagent/mcp/golden_path_test.go` (or new file)

**Precondition**: Real KA session Manager with a pending session `"it-pending-1452"`. MCP SDK test server.

**Action**: Send `kubernaut_investigate` with `action=start`, `session_id=it-pending-1452` via MCP SDK client. Investigation emits events.

**Assertions**:
- `EventLogBridge` receives events from session `"it-pending-1452"`
- MCP `LoggingMessage` notifications contain investigation events
- No 60s inactivity timeout

**FedRAMP**: SI-4 + AC-4 (events routed through correct session, bridge functional end-to-end)

#### E2E-FP-1452-001: AIA KASession.ID matches InteractiveSession.SessionID after takeover

**File**: `test/e2e/fullpipeline/05_mcp_interactive_lifecycle_test.go`

**Precondition**: Kind cluster with AF, KA, AA deployed. Mock LLM. Real MCP transport.

**Action**:
1. Create RR + IS CRD (pre-interactive pattern)
2. Wait for AIA CRD to have `KASession.ID` (AA submits to KA, KA creates pending session)
3. Capture `KASession.ID` as the session AF will poll and forward
4. Perform MCP `action=takeover` through AF
5. Wait for `InteractiveSession.SessionID` to appear on AIA CRD

**Assertions**:
- `InteractiveSession.SessionID == KASession.ID` — proves AF forwarded the session ID from the AIA CRD to KA, and KA used it for direct lookup (not RRID scan)

**FedRAMP**: SI-4 + AC-4 (session identity preserved end-to-end through the full deployed pipeline)

**Pyramid Invariant**: E2E proves the journey — UT proves logic, IT proves wiring, E2E proves that a real AF instance polls a real AIA CRD, forwards the session ID over real MCP to a real KA, which performs direct session lookup in a real Kind cluster.

## 7. TDD Execution Order

Follows RED → GREEN → CHECKPOINT W → REFACTOR per kubernaut TDD methodology.

### Cycle 1: Type Definitions (foundation)

| Step | Action |
|------|--------|
| RED  | UT-AF-1452-003, UT-AF-1452-004 — `StartInvestigationArgs.SessionID` field does not exist → compile error |
| GREEN | Add `SessionID` to `StartInvestigationArgs`, add `session_id` to argsMap in `SDKMCPClient.StartInvestigation` |
| CHECKPOINT W | `grep -r SessionID pkg/apifrontend/ka/config.go` confirms field exists |

### Cycle 2: AF forwarding logic

| Step | Action |
|------|--------|
| RED  | UT-AF-1452-001, UT-AF-1452-002, UT-AF-1452-005 — `HandleInvestigationMCPWithRegistry` does not capture `awaitResult.SessionID` → session ID always empty |
| GREEN | Capture `awaitResult.SessionID` and pass to `StartInvestigation` |
| CHECKPOINT W | `grep -r awaitResult.SessionID pkg/apifrontend/tools/ka_investigate_mcp.go` confirms forwarding |

### Cycle 3: KA input + handler

| Step | Action |
|------|--------|
| RED  | UT-KA-1452-001, UT-KA-1452-002, UT-KA-1452-003 — `InvestigateInput.SessionID` does not exist → compile error |
| GREEN | Add `SessionID` to `InvestigateInput`, update `handleStart` to prefer AF-provided session ID |
| CHECKPOINT W | `grep -r 'input.SessionID' internal/kubernautagent/mcp/tools/investigate.go` confirms handler uses field |

### Cycle 4: Integration wiring

| Step | Action |
|------|--------|
| RED  | IT-AF-1452-001, IT-KA-1452-001 — integration tests fail (session ID not in MCP protocol / bridge mismatch) |
| GREEN | All wiring in place from Cycles 1-3 → integration tests pass |
| CHECKPOINT W | Full Wiring Manifest verification (all rows have production callers + passing ITs) |

### Cycle 5: REFACTOR

| Step | Action |
|------|--------|
| REFACTOR | Audit log message improvements, remove any interim scaffolding, verify `go build ./...` clean |

## 8. Exit Criteria

- All UT, IT, and E2E tests in this plan PASS
- `go build ./...` succeeds (no compilation errors)
- `golangci-lint run --timeout=5m` clean (no new warnings)
- CHECKPOINT W: every Wiring Manifest row has production caller + passing IT
- Pyramid Invariant: UT proves logic, IT proves wiring, E2E proves the journey
- E2E-FP-1452-001: `InteractiveSession.SessionID == KASession.ID` in a Kind cluster
- Existing tests in `ka_investigate_mcp_test.go`, `start_investigation_test.go`, `interactive_start_test.go` continue to pass (no regression)
