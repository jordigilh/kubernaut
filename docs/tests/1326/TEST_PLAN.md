# Test Plan — #1326: Migrate AF-to-KA Communication from REST+MCP Hybrid to MCP-Only

**IEEE 829 Compliant** | **Issue**: [#1326](https://github.com/jordigilh/kubernaut/issues/1326) | **Depends on**: TP-1310-INVESTIGATION-TRANSPARENCY, TP-1311-KIND-DISAMBIGUATION

## 1. Test Plan Identifier

TP-1326-MCP-MIGRATION

## 2. Introduction

The API Frontend (AF) currently communicates with the Kubernaut Agent (KA) using two
protocols simultaneously: MCP for 8/9 tools (interactive investigation) and REST+SSE
for 1/9 tools (autonomous investigation via `kubernaut_investigate`). This dual approach
created fragile code that broke in #1310 and #1311, blocks the LLM turn for up to 15
minutes during autonomous investigations, and provides no real-time transparency during
interactive turns.

This test plan covers the migration to MCP-only communication with unified event
streaming via `ServerSession.Log`, non-blocking autonomous investigation via ADK
`IsLongRunning`, and automatic post-investigation RR CRD lifecycle tracking.

**Business Requirements**:
- BR-MCP-001: AF MUST communicate with KA exclusively via MCP (eliminate REST dependency)
- BR-MCP-002: KA MUST expose `start_autonomous` action on `kubernaut_investigate` MCP tool
- BR-MCP-003: KA MUST stream investigation events (reasoning, actions, errors, completion) via `ServerSession.Log` notifications
- BR-MCP-004: AF MUST receive MCP `LoggingMessage` notifications during pending `CallTool` operations
- BR-MCP-005: AF `kubernaut_investigate` MUST be non-blocking (`IsLongRunning: true`) and return immediately with session_id
- BR-MCP-006: AF MUST bridge KA reasoning and action events to A2A EventBridge in real-time (observer mode)
- BR-MCP-007: AF MUST bridge KA reasoning and action events during interactive `message` turns in real-time
- BR-MCP-008: AF MUST NOT bridge tool output/results to the user (only reasoning + actions + errors)
- BR-MCP-009: AF MUST auto-transition from KA event streaming to RR CRD watching after investigation completes
- BR-MCP-010: AF MUST manage background monitor goroutines with proper lifecycle (cancel on disconnect, drain on shutdown)
- BR-MCP-011: REST client (`ka.Client`), SSE parser, and all REST-specific code MUST be removed

**FedRAMP Control Mapping**:
| Control | Objective | Behavioral Assurance |
|---------|-----------|---------------------|
| AU-2    | Auditable events defined | `ka.delegated` emitted on every investigation start (REST or MCP delegation); `ka.result_received` emitted on every completion/error. Both events carry `delegation_type` distinguishing MCP from legacy REST paths. See: `AUDIT_EVENT_CATALOG.md` §KubernautAgent Delegation |
| AU-3    | Audit record content | Every audit record includes `session_id`, `ka_correlation_id`, `delegation_type`, `timestamp`, `user_id`, `source_ip` per `audit.Event` schema. `tool.executed` records include `execution_duration_ms` and `tool_outcome`. Error records include `error` detail |
| AU-12   | Audit generation | Events generated at every investigation lifecycle point: delegation start (`ka.delegated`), progress streaming (SI-4), completion (`ka.result_received`), tool execution (`tool.executed`), and error paths (`mcp.tool_failed`). No lifecycle transition occurs without a corresponding audit event |
| SI-4    | Information system monitoring | Real-time progressive streaming via MCP `LoggingMessage` notifications: reasoning deltas, tool call starts, errors, and completion events arrive incrementally (not batched). AF bridges these to A2A `EmitReasoningSafe` for user visibility. Observable via `af_circuit_breaker_state{dependency="ka-mcp"}` and standard MCP session metrics |
| SC-7    | Boundary protection | Single protocol (MCP) reduces AF-to-KA attack surface. After migration: no REST client, no `kaBaseURL` config, no SSE parser. AF egress to KA is exclusively MCP over TLS on port 8443. Verified by absence of `ka.NewClient`, `kaBaseURL`, and REST-specific code in production binary |

## 3. Test Items

### 3.1 KA Side (Phase 2a)

| Item | File | Description |
|------|------|-------------|
| `ActionStartAutonomous` | `internal/kubernautagent/mcp/tools/investigate_types.go` | New action constant and input validation |
| `handleStartAutonomous` | `internal/kubernautagent/mcp/tools/investigate.go` | Starts autonomous investigation via session.Manager, returns session_id |
| EventChannel-to-Log bridge | `internal/kubernautagent/mcp/tools/investigate.go` | Goroutine reading EventChannel and calling ServerSession.Log |
| `InvestigateRegistration` (updated) | `internal/kubernautagent/mcp/tools/registration.go` | Updated tool description, event streaming registration |

### 3.2 AF Side (Phase 2b)

| Item | File | Description |
|------|------|-------------|
| `MCPClient.StartAutonomous` | `pkg/apifrontend/ka/mcp_client.go` | New interface method for non-blocking autonomous investigation |
| `SDKMCPClient.StartAutonomous` | `pkg/apifrontend/ka/mcp_sdk_client.go` | Dedicated MCP session with LoggingMessageHandler |
| `LoggingMessageHandler` wiring | `pkg/apifrontend/ka/mcp_sdk_client.go` | Register handler + SetLoggingLevel on MCP client |
| `HandleInvestigation` (rewritten) | `pkg/apifrontend/tools/ka_investigate.go` | Non-blocking, IsLongRunning, spawns background monitor |
| `MonitorRegistry` | `pkg/apifrontend/ka/monitor_registry.go` | Lifecycle management for background investigation monitors |
| `NewInvestigateTool` (updated) | `pkg/apifrontend/tools/ka_investigate.go` | IsLongRunning: true, MCPClient instead of REST Client |

### 3.3 Deletion (Phase 2b-3)

| Item | File | Description |
|------|------|-------------|
| `ka.Client` | `pkg/apifrontend/ka/rest_client.go` | Entire REST client — DELETE |
| `parseSSEStream` | `pkg/apifrontend/ka/sse_parser.go` | SSE parser — DELETE |
| SSE helpers | `pkg/apifrontend/tools/ka_stream.go` | Stream digest helpers — DELETE (protocol-agnostic helpers KEEP) |

## 4. Features to Be Tested

### 4.0 FedRAMP Behavioral Assurance

Tests in this plan verify business-level behavior tied to FedRAMP control objectives.
Each scenario below maps to one or more NIST 800-53 controls and asserts observable
behavior — not implementation details.

#### AU-2: Auditable Events Defined

The system MUST define and emit structured audit events at every investigation lifecycle
transition. These map to `AUDIT_EVENT_CATALOG.md` §KubernautAgent Delegation:

| Lifecycle Point | Required Event | Detail Fields Asserted |
|-----------------|---------------|----------------------|
| Investigation delegated to KA | `ka.delegated` (AU-2, AU-12) | `session_id`, `ka_correlation_id`, `delegation_type=mcp` |
| Investigation result received | `ka.result_received` (AU-2, AU-12) | `session_id`, `ka_correlation_id`, `result_type` |
| Investigation error | `ka.result_received` with `result_type=error` | `session_id`, `error` |
| Tool execution | `tool.executed` (AU-12, AU-2) | `tool_name=kubernaut_investigate`, `tool_outcome`, `execution_duration_ms` |

**Tested by**: UT-AF-1326-043 (delegation), UT-AF-1326-054 (completion), UT-AF-1326-100 (audit trail completeness), IT-AF-1326-096 (end-to-end audit trail)

#### AU-3: Audit Record Content

Every audit record MUST contain fields required by the `audit.Event` schema:
`timestamp`, `type`, `correlation_id`, `request_id`, `user_id`, `source_ip`, and
context-specific `detail` fields. Tests verify that `Detail` maps contain all required
keys and that values are non-empty for mandatory fields.

**Tested by**: UT-AF-1326-101 (delegation record fields), UT-AF-1326-102 (completion record fields), UT-AF-1326-103 (error record fields)

#### AU-12: Audit Generation at Every Lifecycle Point

No investigation lifecycle transition occurs without a corresponding audit event.
Tests verify the audit trail has no gaps by asserting the ordered sequence of events
from delegation through completion (or error).

**Tested by**: UT-AF-1326-100 (no-gap audit trail), IT-AF-1326-096 (end-to-end lifecycle audit)

#### SI-4: Information System Monitoring

Real-time progressive streaming MUST deliver events incrementally — not batched.
The observer receives reasoning deltas, tool call starts, errors, and completion
as they occur. Tests assert temporal ordering and progressive delivery:
- First `reasoning_delta` arrives before `complete`
- Multiple events arrive with monotonically increasing timestamps
- AF bridges events to A2A in real-time (not buffered until completion)

**Tested by**: UT-KA-1326-010..014 (event types streamed), UT-AF-1326-050..055 (bridging), IT-KA-1326-091 (progressive delivery), IT-AF-1326-093 (end-to-end streaming)

#### SC-7: Boundary Protection

After migration, AF communicates with KA exclusively via MCP over TLS. Tests verify:
- No REST client (`ka.Client`) exists in production binary
- No `kaBaseURL` in configuration
- No SSE parser in production binary
- AF egress to KA is single-protocol (MCP on port 8443)

**Tested by**: WT-AF-1326-085 (no ka.Client), WT-AF-1326-086 (no kaBaseURL), IT-AF-1326-095 (single protocol)

### 4.1 KA: start_autonomous Action (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-KA-1326-001 | BR-MCP-002 | — | `start_autonomous` with valid rr_id starts investigation | `output.Status == "autonomous_started"`, `output.SessionID != ""` |
| UT-KA-1326-002 | BR-MCP-002 | — | `start_autonomous` with missing rr_id returns error | Error contains "rr_id" |
| UT-KA-1326-003 | BR-MCP-002 | — | `start_autonomous` with nonexistent RR returns error | Error code `ErrCodeRRNotFound` |
| UT-KA-1326-004 | BR-MCP-002 | — | `start_autonomous` when investigation already running returns existing session_id | `output.SessionID` matches existing, status indicates already running |
| UT-KA-1326-005 | BR-MCP-002 | — | `ValidateInput` accepts `start_autonomous` action | No error returned |
| UT-KA-1326-006 | BR-MCP-002 | — | `dispatch` routes `start_autonomous` to `handleStartAutonomous` | Correct handler invoked |
| UT-KA-1326-007 | BR-MCP-002 | — | `start_autonomous` calls `Subscribe()` to activate LazySink EventChannel | `Subscribe()` called before returning; EventChannel non-nil |

### 4.2 KA: EventChannel to ServerSession.Log Bridge (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-KA-1326-010 | BR-MCP-003 | SI-4 | reasoning_delta event streamed as LoggingMessage | `Log` called with level "info", data contains `event_type: "reasoning_delta"` |
| UT-KA-1326-011 | BR-MCP-003 | SI-4 | tool_call_start event streamed as LoggingMessage | `Log` called with data containing `event_type: "tool_call_start"` |
| UT-KA-1326-012 | BR-MCP-003 | SI-4 | tool_result event streamed as LoggingMessage | `Log` called with data containing `event_type: "tool_result"` |
| UT-KA-1326-013 | BR-MCP-003 | SI-4 | complete event streamed as LoggingMessage | `Log` called with data containing `event_type: "complete"` |
| UT-KA-1326-014 | BR-MCP-003 | SI-4 | error event streamed as LoggingMessage | `Log` called with level "error", data contains `event_type: "error"` |
| UT-KA-1326-015 | BR-MCP-003 | — | Bridge goroutine exits when event channel closes | No goroutine leak, no panic |
| UT-KA-1326-016 | BR-MCP-003 | — | Bridge goroutine exits when context cancelled | Goroutine exits cleanly |
| UT-KA-1326-017 | BR-MCP-003 | — | No events streamed if no subscriber (LazySink nil) | `Log` never called |
| UT-KA-1326-018 | BR-MCP-003 | SI-4 | Events arrive with monotonically increasing sequence numbers | Each LoggingMessage data includes `seq` > previous `seq` |

### 4.3 KA: Interactive Streaming During message Action (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-KA-1326-020 | BR-MCP-007 | SI-4 | During `action=message`, reasoning events streamed via Log | `Log` called with reasoning_delta events |
| UT-KA-1326-021 | BR-MCP-007 | SI-4 | During `action=message`, tool_call_start events streamed | `Log` called with tool_call_start events |
| UT-KA-1326-022 | BR-MCP-007 | — | Final message response returned synchronously in CallTool result | `output.Response` contains LLM response text |

### 4.4 AF: MCPClient.StartAutonomous Interface (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1326-030 | BR-MCP-001 | SC-7 | `StartAutonomous` creates dedicated MCP session (non-pooled) | Session created via ConnectSession, not pool.Acquire; single-protocol |
| UT-AF-1326-031 | BR-MCP-004 | SI-4 | `StartAutonomous` calls `SetLoggingLevel("info")` after connect | SetLoggingLevel called on session — without this, notifications are silently dropped |
| UT-AF-1326-032 | BR-MCP-004 | SI-4 | `LoggingMessageHandler` receives notifications during pending CallTool | Events delivered to channel while CallTool blocks |
| UT-AF-1326-033 | BR-MCP-001 | — | `StartAutonomous` returns session_id and event channel | `sessionID != ""`, `events` channel is readable |
| UT-AF-1326-034 | BR-MCP-001 | — | `StartAutonomous` on MCP connect failure returns error | Error wraps connect failure |
| UT-AF-1326-035 | BR-MCP-001 | — | `MockMCPClient.StartAutonomous` stubbed for tests | Mock returns configured session_id and events |

### 4.5 AF: Non-Blocking HandleInvestigation (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1326-040 | BR-MCP-005 | — | `HandleInvestigation` returns immediately with session_id and status "started" | `result.Status == "started"`, `result.SessionID != ""` |
| UT-AF-1326-041 | BR-MCP-005 | — | `NewInvestigateTool` sets `IsLongRunning: true` | Tool config has IsLongRunning |
| UT-AF-1326-042 | BR-MCP-005 | — | Args validation: namespace+name required for new investigation | Error returned if namespace or name empty |
| UT-AF-1326-043 | BR-MCP-005 | AU-2 | Audit event `ka.delegated` emitted with `delegation_type=mcp` on successful start | Auditor receives EventKADelegated; detail includes `delegation_type`, `session_id`, `ka_correlation_id` |
| UT-AF-1326-044 | BR-MCP-005 | — | MCP `start_autonomous` called with correct args (namespace, kind, name) | MCPClient.StartAutonomous called with matching args |

### 4.6 AF: Observer Event Bridging (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1326-050 | BR-MCP-006 | SI-4 | reasoning_delta from MCP notification bridged to A2A EventBridge | `EmitReasoningSafe` called with reasoning text |
| UT-AF-1326-051 | BR-MCP-006 | SI-4 | tool_call_start from MCP notification bridged as "[Retrieving logs...]" | Bridge text contains action description |
| UT-AF-1326-052 | BR-MCP-008 | — | tool_result (success) from MCP notification NOT bridged | `EmitReasoningSafe` NOT called for successful tool_result |
| UT-AF-1326-053 | BR-MCP-006 | SI-4 | tool_result (error) from MCP notification bridged as "[Error: ...]" | Bridge text contains error detail |
| UT-AF-1326-054 | BR-MCP-006 | AU-2, AU-12 | complete event triggers audit `ka.result_received` with `result_type`, `session_id`, `ka_correlation_id` | Auditor receives EventKAResultReceived with all required detail fields |
| UT-AF-1326-055 | BR-MCP-006 | SI-4 | error event bridged as "[Investigation error: ...]" | Bridge text contains investigation error |

### 4.7 AF: MonitorRegistry Lifecycle (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1326-060 | BR-MCP-010 | — | Monitor started and registered by session_id | Registry.Size() == 1 |
| UT-AF-1326-061 | BR-MCP-010 | — | Monitor stops on context cancel (A2A disconnect) | Monitor goroutine exits, done channel closed |
| UT-AF-1326-062 | BR-MCP-010 | — | Monitor stops on complete event | MCP session closed, registry entry removed |
| UT-AF-1326-063 | BR-MCP-010 | — | `DrainAll` cancels all monitors and waits | All monitors stopped, Size() == 0 |
| UT-AF-1326-064 | BR-MCP-010 | — | Duplicate monitor start for same session_id returns existing | No second goroutine spawned |
| UT-AF-1326-065 | BR-MCP-010 | — | Monitor handles MCP session error gracefully | Error logged, monitor exits, registry cleaned |

### 4.8 AF: Post-Investigation RR CRD Transition (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1326-070 | BR-MCP-009 | — | After complete event, monitor switches to RR CRD watching | K8s Watch called for RR resource |
| UT-AF-1326-071 | BR-MCP-009 | SI-4 | RR phase transitions bridged to A2A | Phase change events emitted via EventBridge |
| UT-AF-1326-072 | BR-MCP-009 | — | RR terminal phase (Completed/Failed/TimedOut/Skipped) stops monitor | Monitor exits on terminal RR phase |
| UT-AF-1326-073 | BR-MCP-009 | — | `IsTerminalPhase` includes TimedOut and Skipped | Both return true |

### 4.9 AF: FedRAMP Audit Trail Completeness (UT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| UT-AF-1326-100 | BR-MCP-005, BR-MCP-006 | AU-2, AU-12 | Full lifecycle emits ordered audit trail: `ka.delegated` → streaming → `ka.result_received` | Auditor captures both events in order; no lifecycle transition without an audit event |
| UT-AF-1326-101 | BR-MCP-005 | AU-3 | `ka.delegated` record contains `session_id`, `ka_correlation_id`, `delegation_type=mcp` | All three detail fields present and non-empty |
| UT-AF-1326-102 | BR-MCP-006 | AU-3 | `ka.result_received` record contains `session_id`, `ka_correlation_id`, `result_type` | All three detail fields present and non-empty |
| UT-AF-1326-103 | BR-MCP-006 | AU-3 | Error path: `ka.result_received` with `result_type=error` contains `error` detail | `error` field present and non-empty |
| UT-AF-1326-104 | BR-MCP-005 | AU-2 | `tool.executed` emitted for `kubernaut_investigate` with `execution_duration_ms` | Auditor captures EventToolExecuted with duration > 0 |

### 4.10 Wiring Tests (WT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| WT-KA-1326-080 | BR-MCP-002 | — | `start_autonomous` wired in `InvestigateRegistration` dispatch | Tool description includes "start_autonomous" |
| WT-KA-1326-081 | BR-MCP-003 | — | EventChannel→Log bridge registered on autonomous_started output | Bridge goroutine starts when output.Status == "autonomous_started" |
| WT-AF-1326-082 | BR-MCP-001 | SC-7 | `kubernaut_investigate` wired to MCPClient (not ka.Client) in root.go | Tool constructor receives MCPClient; single protocol boundary |
| WT-AF-1326-083 | BR-MCP-001 | SC-7 | `kubernaut_investigate` wired to MCPClient in mcp_bridge.go | Bridge handler receives MCPClient; no REST path |
| WT-AF-1326-084 | BR-MCP-010 | — | `MonitorRegistry.DrainAll` called during shutdown | Shutdown ladder includes registry drain |
| WT-AF-1326-085 | BR-MCP-011 | SC-7 | `ka.Client` NOT referenced in main.go after migration | No `ka.NewClient` call, no `KAClient` field |
| WT-AF-1326-086 | BR-MCP-011 | SC-7 | `kaBaseURL` NOT in config after migration | Config validation does not require kaBaseURL |
| WT-AF-1326-087 | BR-MCP-005 | — | `NewInvestigateTool` produces tool with `IsLongRunning: true` | Tool.IsLongRunning() returns true |
| WT-AF-1326-088 | BR-MCP-011 | SC-7 | PrometheusRule alert targets `dependency="ka-mcp"` (not `dependency="ka"`) | Alert expr uses correct label after REST removal |

### 4.11 Integration Tests (IT tier)

| ID | BR | FedRAMP | Scenario | Asserts |
|----|-----|---------|----------|---------|
| IT-KA-1326-090 | BR-MCP-002, BR-MCP-003 | AU-2, SI-4 | MCP client calls `start_autonomous`, receives events via LoggingMessage, sees complete | Full event stream received on MCP session; `ka.delegated` event captured |
| IT-KA-1326-091 | BR-MCP-003 | SI-4 | MCP client connects, starts autonomous, reasoning_delta events arrive before complete | Events are progressive (timestamped), not batched |
| IT-AF-1326-092 | BR-MCP-001, BR-MCP-005 | — | A2A message triggers `kubernaut_investigate`, task enters `input-required`, user sees progress artifacts | TaskState is InputRequired after tool returns |
| IT-AF-1326-093 | BR-MCP-006, BR-MCP-009 | SI-4, AU-12 | Observer bridges events to A2A, then transitions to RR watching after complete | A2A artifacts include both KA events and RR transitions; audit trail complete |
| IT-AF-1326-094 | BR-MCP-010 | — | A2A SSE disconnect stops background monitor | Monitor goroutine exits, MCP session closed |
| IT-AF-1326-095 | BR-MCP-011 | SC-7 | No REST endpoints referenced in AF after migration | Config has no kaBaseURL, main.go has no ka.NewClient; single-protocol boundary |
| IT-AF-1326-096 | BR-MCP-005, BR-MCP-006 | AU-2, AU-3, AU-12 | End-to-end audit trail: delegation → streaming → completion produces ordered audit events with required fields | Captured audit events form complete lifecycle; each contains `session_id`, `correlation_id`; no gaps |

## 5. Features Not to Be Tested

- AA controller (AIAnalysis) — unchanged, continues using REST poll
- KA REST API endpoints — will be removed from AF consumption but remain available for AA
- MCP interactive tools (takeover, message, complete, etc.) — existing tests cover these
- AF tool RBAC/rate limiting — orthogonal to protocol migration

## 6. Approach

### 6.1 TDD Methodology

Each implementation phase follows strict TDD RED-GREEN-REFACTOR:

1. **RED**: Write failing Ginkgo/Gomega tests defining the business contract
2. **GREEN**: Minimal implementation to pass all tests
3. **REFACTOR**: Improve code quality; validate against [100 Go Mistakes](https://github.com/teivah/100-go-mistakes)

### 6.2 Test Pyramid (Pyramid Invariant)

| Tier | Count | Target Coverage | Strategy |
|------|-------|-----------------|----------|
| Unit (UT) | 47 scenarios | >= 80% of unit-testable code | Mocked dependencies, table-driven |
| Wiring (WT) | 9 scenarios | Wiring validation | Verify integration points and FedRAMP SC-7 boundary |
| Integration (IT) | 7 scenarios | >= 80% of integration-testable code | MCP server/client with envtest, end-to-end audit trail |

### 6.3 Checkpoints

Each checkpoint performs a multi-dimensional GA readiness audit. Advancement to the
next phase requires **>=95% confidence** across all dimensions. Actionable findings
are escalated before proceeding.

#### CP-1: After Phase 2a (KA changes)

| Dimension | Gate Criteria | Evidence |
|-----------|--------------|----------|
| Build & CI | `go build ./internal/kubernautagent/...` succeeds; `make generate` produces no diff | CI `lint-go` + `unit-tests(kubernautagent)` green |
| Testing | UT-KA-1326-001..018, UT-KA-1326-020..022, WT-KA-1326-080..081 pass; >=80% coverage of new code | `go test -cover` output |
| Security/FedRAMP | SI-4: progressive event streaming verified (UT-KA-1326-018 sequence numbers); no new attack surface | Code review: no new HTTP listeners |
| Observability | KA `/stream` histogram exclusion unchanged (AA path); new MCP streaming has no metric regressions | `http_metrics.go` untouched |
| Error Handling | Bridge goroutine exits cleanly on cancel and channel close (UT-KA-1326-015..016); LazySink activation verified (UT-KA-1326-007) | Test results |
| Backward Compat | AA controller unaffected; existing KA MCP tools unmodified | `pkg/agentclient/client.go` unchanged |

#### CP-2: After Phase 2b-2 (AF investigate rewrite)

| Dimension | Gate Criteria | Evidence |
|-----------|--------------|----------|
| Build & CI | `go build ./pkg/apifrontend/...` succeeds; `go build ./cmd/apifrontend/...` succeeds | CI `lint-go` + `unit-tests(apifrontend)` green |
| Testing | UT-AF-1326-030..035, UT-AF-1326-040..055, UT-AF-1326-060..073, UT-AF-1326-100..104, WT-AF-1326-082..087 pass; >=80% coverage per tier | `go test -cover` output |
| Security/FedRAMP | AU-2/AU-3/AU-12: audit trail completeness verified (UT-AF-1326-100..104); SI-4: bridging verified (UT-AF-1326-050..055); SetLoggingLevel called (UT-AF-1326-031) | Test results + code review |
| Observability | `af_circuit_breaker_state{dependency="ka-mcp"}` exists; no orphaned metrics | PrometheusRule review |
| Error Handling | MCP session drop handled (UT-AF-1326-065); context cancel stops monitor (UT-AF-1326-061) | Test results |
| API Stability | `MCPClient` interface backward-compatible (existing methods unchanged) | Interface diff |

#### CP-3: After Phase 2b-4 (REST eliminated)

| Dimension | Gate Criteria | Evidence |
|-----------|--------------|----------|
| Build & CI | `go build ./...` succeeds; `make test` passes (all tiers); `make generate` clean | Full CI pipeline green |
| Testing | All TP-1326 scenarios pass; no `ka.Client`, `kaBaseURL`, `sse_parser` in test code | `rg` search results |
| Security/FedRAMP | SC-7: single protocol verified (WT-AF-1326-085..086, IT-AF-1326-095); AU-2/AU-3/AU-12: end-to-end audit trail (IT-AF-1326-096) | Integration test results |
| Observability | `ApifrontendCircuitBreakerOpenKA` retargeted to `dependency="ka-mcp"` (WT-AF-1326-088); RB-AF-005 updated | PrometheusRule diff + runbook review |
| Error Handling | No orphaned goroutines; DrainAll tested (UT-AF-1326-063) | Test results |
| API Stability | OpenAPI spec updated if `/stream`/`/snapshot` removed; ogen regenerated; Helm template has `kaMCPEndpoint`, no `kaBaseURL` | `git diff` on spec + Helm |
| Backward Compat | AA controller unaffected; E2E `fullpipeline` tests pass | CI `e2e-tests(fullpipeline)` green |
| Documentation | ADR-014 superseded; ARCHITECTURE.md, DEVELOPER_GUIDE.md updated | File diffs |

## 7. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/tests/1326/TEST_PLAN.md` |
| KA unit tests | `internal/kubernautagent/mcp/tools/investigate_test.go` |
| KA integration tests | `test/integration/kubernautagent/mcp/investigate_autonomous_test.go` |
| AF MCPClient unit tests | `pkg/apifrontend/ka/mcp_sdk_client_test.go` (extended) |
| AF investigate unit tests | `pkg/apifrontend/tools/ka_investigate_test.go` (refactored) |
| AF monitor registry tests | `pkg/apifrontend/ka/monitor_registry_test.go` |
| AF wiring tests | `cmd/apifrontend/main_wiring_test.go` (extended) |
| AF integration tests | `test/integration/apifrontend/mcp_investigation_test.go` |

## 8. Test Environment

| Component | Tool |
|-----------|------|
| Go version | 1.26+ |
| Test framework | Ginkgo v2 / Gomega |
| MCP SDK | `github.com/modelcontextprotocol/go-sdk` v1.6.0 |
| K8s testing | envtest (kubebuilder) for IT tier |
| CI | GitHub Actions (`ci-pipeline.yml`) |

## 9. Schedule

| Phase | Duration | Tests | Checkpoint |
|-------|----------|-------|------------|
| Phase 2a KA (TDD Red/Green/Refactor) | 4-6 days | UT-KA-1326-001..018, UT-KA-1326-020..022, WT-KA-1326-080..081, IT-KA-1326-090..091 | **CP-1** |
| Phase 2b-1 AF MCPClient (TDD R/G/R) | 2-3 days | UT-AF-1326-030..035 | — |
| Phase 2b-2 AF Investigate (TDD R/G/R) | 3-4 days | UT-AF-1326-040..055, UT-AF-1326-060..073, UT-AF-1326-100..104, WT-AF-1326-082..087 | **CP-2** |
| Phase 2b-3 Delete REST + refactor tests | 1-2 days | WT-AF-1326-085..086, WT-AF-1326-088, IT-AF-1326-095 | — |
| Phase 2b-4 Rewire main.go + alerts + Helm | 1 day | IT-AF-1326-092..094, IT-AF-1326-096 | **CP-3** |
| Phase 2c Cleanup | 0.5 day | Documentation only | — |

## 10. Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation | Test Coverage |
|------|--------|------------|------------|---------------|
| KA EventChannel not activated without REST `/stream` subscriber | Events lost silently | Medium | `start_autonomous` calls `Subscribe()` directly to activate LazySink | UT-KA-1326-007 |
| MCP session drops during long investigation | Lost events | Low | Monitor detects error, exits cleanly, logs for operator | UT-AF-1326-065 |
| ADK `IsLongRunning` completion dispatch fails | LLM never presents results | Low | Spike validated pattern; `kubernaut_present_decision` proves it | UT-AF-1326-040 |
| Test refactoring introduces regressions | Broken tests | Medium | Keep existing test scenarios, swap mock layer only | CP-3 full test gate |
| `SetLoggingLevel` not called, events silently dropped | No streaming | High | Test UT-AF-1326-031 explicitly validates this call | UT-AF-1326-031 |
| PrometheusRule alert goes silent after REST removal | KA health unmonitored | High | Retarget `ApifrontendCircuitBreakerOpenKA` to `dependency="ka-mcp"` | WT-AF-1326-088 |
| Helm chart missing `kaMCPEndpoint` | MCP-only deploys from Helm fail | Medium | Add `kaMCPEndpoint` to Helm template while removing `kaBaseURL` | CP-3 Helm gate |
| OpenAPI spec update required before handler deletion | Compile failure | High | Sequence: spec → `make generate` → handler deletion; CI enforces | CP-1 build gate |
| Audit trail gap during protocol migration | FedRAMP AU-12 violation | Medium | `delegation_type=mcp` in audit events; end-to-end audit trail test | UT-AF-1326-100, IT-AF-1326-096 |

## 11. GA Readiness Audit Findings

Pre-implementation audit performed 2026-05-28. All FAIL items have resolutions mapped
to implementation phases. Advancement past each checkpoint requires re-audit.

### FAIL Items (must address before GA)

| ID | Dimension | Finding | Resolution | Phase |
|----|-----------|---------|------------|-------|
| B1 | Build & CI | OpenAPI spec must be updated before handler deletion — ogen `Handler` interface compile failure | Sequence: spec → `make generate` → handler deletion | 2a |
| T4 | Testing | No test for `SetLoggingLevel` — silent notification drop is highest-risk failure mode | UT-AF-1326-031 | 2b-1 |
| S3 | Security | ADR-014 explicitly rejected MCP-only — must be formally superseded | New ADR or status update | 2c |
| O1 | Observability | `af_circuit_breaker_state{dependency="ka"}` alert goes silent after REST removal | Retarget to `dependency="ka-mcp"` (WT-AF-1326-088) | 2b-4 |
| A3 | API Stability | Helm chart hardcodes `kaBaseURL` but missing `kaMCPEndpoint` | Fix Helm template | 2b-4 |
| E4 | Error Handling | `LazySink` requires explicit `Subscribe()` in MCP path — without it, events lost | `handleStartAutonomous` must call `Subscribe()` (UT-KA-1326-007) | 2a |

### WARN Items (tracked, acceptable risk)

| ID | Dimension | Finding | Status |
|----|-----------|---------|--------|
| B2 | Build & CI | CI enforces `make generate` + `git diff` — any OpenAPI change without committed regen fails | Acknowledged; standard workflow |
| B3 | Build & CI | ~15 KA test files reference `/stream` or `/snapshot` | Planned in Phase 2a cleanup |
| B4 | Build & CI | Cross-cutting change touches both services in 4 CI stages | Atomic merge required |
| T3 | Testing | KA E2E tests for `/snapshot` (5) and `/stream` (3) need removal | Planned |
| S4 | Security | `AUDIT_EVENT_CATALOG.md` references "REST" in `ka.delegated` trigger | Update in Phase 2c |
| S5 | Security | `SERVICE_BOUNDARY.md` documents dual REST+MCP egress | Update in Phase 2c |
| O2 | Observability | `ka-mcp` falls into `ApifrontendCircuitBreakerOpenOther` catch-all | Add explicit alert in Phase 2b-4 |
| O3 | Observability | RB-AF-005 diagnosis assumes REST CB with `dependency="ka"` | Update in Phase 2c |
| O4 | Observability | RB-AF-009 says "KA REST/MCP" in overview | Update in Phase 2c |
| O5 | Observability | External Grafana dashboards may filter on `dependency="ka"` | Document label change |
| A1 | API Stability | Removing `/stream` and `/snapshot` is breaking KA API change | Accepted: pre-GA, no external consumers |
| A2 | API Stability | `ARCHITECTURE.md` documents all 6 KA REST endpoints | Update in Phase 2c |
| A4 | API Stability | ADR-013 references "REST (header) and MCP" | Update in Phase 2c |
| A5 | API Stability | 15 files reference `kaBaseURL` | All tracked in Phase 2b-4 |
| E3 | Error Handling | MCP session drop during long investigation | UT-AF-1326-065 covers |
| C3 | Backward Compat | No feature flag for phased rollout — big-bang removal | Accepted: pre-GA |

### PASS Items

| ID | Dimension | Finding |
|----|-----------|---------|
| S1 | Security | No FedRAMP control maps specifically to KA REST SSE path |
| S2 | Security | SC-7 improves — single protocol reduces attack surface |
| S6 | Security | NetworkPolicy unchanged — REST and MCP share host:port:TLS |
| E1 | Error Handling | Unified `ka-mcp` circuit breaker handles all traffic |
| E2 | Error Handling | `ResilientDynamicClient` bypasses CB for K8s Watch |
| C1 | Backward Compat | AA controller unaffected — uses v1.4 REST endpoints only |
| C2 | Backward Compat | KA REST `/cancel` stays for AA |
| C4 | Backward Compat | `pkg/agentclient` ogen regen — AA wrapper unaffected |

**Overall Confidence**: 91% — All FAIL items have clear resolutions. Residual risk:
test refactoring volume and potential out-of-repo consumers.

## 12. Approvals

| Role | Name | Date |
|------|------|------|
| Author | AI Agent | 2026-05-28 |
| Reviewer | — | — |
