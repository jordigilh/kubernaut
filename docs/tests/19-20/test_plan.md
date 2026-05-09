# Test Plan: MCP Tool Bridge — Wire Stubs to Real Implementations

**Test Plan Identifier:** TP-AF-019-020-BRIDGE
**Issues:** [#19](https://github.com/jordigilh/kubernaut-apifrontend/issues/19), [#20](https://github.com/jordigilh/kubernaut-apifrontend/issues/20)
**Version:** 1.1
**Date:** 2026-05-09
**Status:** Draft

---

## 1. Introduction

This test plan validates the MCP tool bridge layer that wires 20 stubbed MCP tools to their real `Handle*` implementations. The bridge dispatches typed MCP `tools/call` requests through RBAC enforcement, nil-safety guards, error redaction, per-tool metrics, tool timeouts, and audit event emission.

### 1.1 Scope

- `MCPBridgeConfig` struct and `RegisterTools` function dispatching all 20 tools
- RBAC guard (`checkRBAC`) with audit on denial
- `tools/list` filtering by user groups (AC-6)
- Nil-safety guards for DS and KA REST clients
- Error redaction via `security.RedactError` before returning to MCP client
- Per-tool metrics: `af_tool_calls_total{tool,outcome}`, `af_tool_call_duration_seconds{tool}`, `af_mcp_rbac_denied_total{tool,user}`
- Tool timeout: `context.WithTimeout(ctx, 30s)`
- Per-session concurrency semaphore (max 5 in-flight tool calls)
- Audit events: `EventMCPToolInvoked` (with session_id), `EventMCPToolDenied`, `EventMCPToolFailed`, `EventMCPSessionInit`
- Agent Card alignment after `mcptools.go` deletion
- k6 perf script updates

### 1.2 Out of Scope

- Individual `Handle*` function logic (covered by TP-AF-019 and TP-AF-020)
- MCP SDK internals (covered by SDK tests)
- A2A path (unchanged)
- Token refresh / session expiry (#5)
- Backstage plugin (separate repo)

### 1.3 References

- TP-AF-019: CRD Tools test plan
- TP-AF-020: DS Tools test plan
- TP-AF-042: Protocol Conformance test plan
- MCP SDK v1.6.0 (`github.com/modelcontextprotocol/go-sdk`)
- `internal/agent/rbac_roles.yaml`
- ADR-020 (to be created): MCP Server Bridge
- FedRAMP Controls: AU-12 (audit generation), AC-6 (least privilege), SI-4 (monitoring)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — refactoring checklist

### 1.4 Preflight Findings (Verified)

These SDK capabilities have been verified against `go-sdk v1.6.0` source and inform the implementation:

- **`AddTool[In, Out]` handler signature:** `func(ctx, *CallToolRequest, In) (*CallToolResult, Out, error)` — 3 return values. Return `nil, result, err` (SDK auto-populates `CallToolResult`).
- **No `SetListToolsHandler`:** SDK does not expose a setter for `tools/list`. Use `srv.AddReceivingMiddleware(...)` to intercept method `"tools/list"`, call the inner handler, then filter the result by user groups. This is how UT-AF-B-031..034 are implemented.
- **Session ID available in tool handlers:** `req.Session.ID()` returns the MCP session ID string. This is how UT-AF-B-053 and UT-AF-B-056 capture `session_id` for audit events.
- **Per-session semaphore via `sync.Map`:** `req.Session.ID()` is stable and unique per session. Use `sync.Map` keyed by session ID → `*semaphore.Weighted` to implement per-session concurrency limits (UT-AF-B-064..066, 069). Clean up entries when sessions disconnect.
- **Context propagation:** `notDone{ctx}` in jsonrpc2 preserves values but removes cancellation. `UserIdentityFromContext(ctx)` works in tool handlers. SDK enforces same-user across session.
- **Tool name:** Use `present_decision` (matches `rbac_roles.yaml` and ADK). NOT `kubernaut_present_decision`.

### 1.5 Definitions

| Term | Definition |
|------|-----------|
| Bridge | Dispatch layer between MCP SDK and existing `Handle*` functions |
| RBAC Guard | Pre-dispatch check comparing user groups against `rbac_roles.yaml` |
| Nil-Safety | Explicit check preventing nil-pointer panic on unavailable backends |
| Tool Timeout | `context.WithTimeout` budget per tool invocation (30s) |
| Session Semaphore | Per-session concurrency limiter using `sync.Map` keyed by `req.Session.ID()` |
| Receiving Middleware | SDK `AddReceivingMiddleware` — wraps all incoming JSON-RPC method handlers for interception |

---

## 2. Test Items

| Item | Package | Source |
|------|---------|--------|
| `MCPBridgeConfig` | `internal/handler` | New |
| `RegisterTools` | `internal/handler` | New |
| `checkRBAC` | `internal/handler` | New |
| `filterToolsByGroups` | `internal/handler` | New |
| `MCPBridgeMetrics` | `internal/handler` | New |
| `MCPConfig` (refactored) | `internal/handler` | Modified |
| `NewMCPHandler` | `internal/handler` | Modified |
| `agentcard.go` (tool list source) | `internal/handler` | Modified |
| `cmd/apifrontend/main.go` (wiring) | `cmd/apifrontend` | Modified |
| `internal/audit/audit.go` (new events) | `internal/audit` | Modified |
| `internal/metrics/metrics.go` (new counters) | `internal/metrics` | Modified |
| `test/perf/mcp_tools.js` | `test/perf` | Modified |
| `tests/performance/scripts/mcp-tools-call.js` | `tests/performance` | Modified |

---

## 3. Business Acceptance Criteria

| ID | Criterion | Source | Priority |
|----|-----------|--------|----------|
| BAC-BRIDGE-01 | All 20 MCP tools return real backend responses (not "not yet wired") | Issue #19, #20 | P0 |
| BAC-BRIDGE-02 | RBAC denies unauthorized tool calls with audit trail | `rbac_roles.yaml`, FedRAMP AC-6 | P0 |
| BAC-BRIDGE-03 | `tools/list` returns only tools the user is allowed to call | FedRAMP AC-6 | P0 |
| BAC-BRIDGE-04 | Per-tool latency and error metrics are emitted for dashboards | SLO_DEFINITIONS.md | P0 |
| BAC-BRIDGE-05 | Audit events capture tool invocation, denial, and failure with session_id | FedRAMP AU-12 | P0 |
| BAC-BRIDGE-06 | Nil backends (out-of-cluster K8s, misconfigured DS/KA) return graceful errors, not panics | Production resilience | P0 |
| BAC-BRIDGE-07 | Error messages to clients never contain internal URLs, paths, or stack traces | ProdSec | P0 |
| BAC-BRIDGE-08 | Tool calls exceeding 30s are cancelled and return timeout error | SRE runbooks | P1 |
| BAC-BRIDGE-09 | A single MCP session cannot exhaust server resources by parallelizing > 5 tool calls | Performance | P1 |
| BAC-BRIDGE-10 | Username from JWT flows into `HandleSubmitSignal`, `HandleApprove`, `HandleCreateRR` | Identity propagation | P0 |
| BAC-BRIDGE-11 | MCP session initialization emits audit event | FedRAMP AU-12 | P1 |
| BAC-BRIDGE-12 | k6 performance tests pass against the wired bridge | CI/CD | P1 |

---

## 4. Features by Tier

### Tier 1: Core Dispatch (20 tools wired)

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-B.1 | K8s CRD tools (6) dispatch to correct Handle* | `list_remediations`, `get_remediation`, `cancel_remediation`, `watch`, `submit_signal`, `approve` return real results from fake K8s |
| F-B.2 | K8s CRD tools with username (3) propagate identity | `submit_signal`, `approve`, `create_rr` receive username from context |
| F-B.3 | Impersonated K8s tools (4) use TriageFactory | `list_events`, `get_pods`, `get_workloads`, `resolve_owner` call factory(ctx) |
| F-B.4 | DS tools (4) dispatch to DS client | `list_workflows`, `get_remediation_history`, `get_effectiveness`, `get_audit_trail` |
| F-B.5 | KA REST tools (2) dispatch to KA client | `start_investigation`, `poll_investigation` |
| F-B.6 | KA MCP tool (1) dispatches to MCPClient | `select_workflow` |
| F-B.7 | Pure tool (1) dispatches without context/client | `present_decision` |
| F-B.8 | `create_rr` uses pointer args and username | `HandleCreateRR(ctx, client, &args, username)` |
| F-B.9 | `poll_investigation` receives configurable maxPolls/pollInterval | Bridge passes `cfg.PollMaxAttempts`, `cfg.PollInterval` |

### Tier 2: Security (RBAC, Redaction, Nil-Safety)

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-B.10 | RBAC check before every dispatch | User without matching group → tool error, not execution |
| F-B.11 | RBAC uses `present_decision` name (not `kubernaut_present_decision`) | Name matches `rbac_roles.yaml` |
| F-B.12 | RBAC denial emits `EventMCPToolDenied` audit event | Event has tool, user, groups in detail |
| F-B.13 | `tools/list` returns only RBAC-allowed tools for user (via `AddReceivingMiddleware`) | `cicd` user sees 4 tools, not 20 |
| F-B.14 | Nil `K8sClient` → `ErrK8sUnavailable` (not panic) | All 12 K8s-dependent tools |
| F-B.15 | Nil `DSClient` → `ErrServiceUnavailable` (not panic) | All 4 DS tools |
| F-B.16 | Nil `KAClient` → `ErrServiceUnavailable` (not panic) | `start_investigation`, `poll_investigation` |
| F-B.17 | Nil `MCPClient` → error (not panic) | `select_workflow` |
| F-B.18 | Tool errors redacted via `security.RedactError` | No URLs, paths, stack traces in client-visible text |
| F-B.19 | Error shape: tool errors → `IsError=true` content, not JSON-RPC protocol error | SDK wraps Go error as tool error content |

### Tier 3: Observability (Metrics, Audit)

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-B.20 | `af_tool_calls_total{tool,outcome}` incremented on every call | Outcomes: `success`, `denied`, `error`, `timeout` |
| F-B.21 | `af_tool_call_duration_seconds{tool}` records latency | Histogram observed for each call |
| F-B.22 | `af_mcp_rbac_denied_total{tool,user}` on RBAC failure | Counter incremented on denial |
| F-B.23 | `EventMCPToolInvoked` emitted with `tool`, `session_id`, `duration_ms` | Success path |
| F-B.24 | `EventMCPToolFailed` emitted with `tool`, `error` (redacted) | Error path |
| F-B.25 | `EventMCPSessionInit` emitted on MCP initialize | Session lifecycle |
| F-B.26 | Metrics registry in main.go exposes MCP counters | `/metrics` endpoint returns new counters |

### Tier 4: Resilience (Timeout, Concurrency, Degradation)

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-B.27 | Tool timeout: 30s context deadline per call | Slow handler cancelled, returns timeout error |
| F-B.28 | Session semaphore: max 5 concurrent tools per session | 6th call blocks until one completes |
| F-B.29 | Semaphore release on panic (deferred) | No deadlock from handler panic |
| F-B.30 | Session idle timeout configured | Stale sessions cleaned after inactivity |
| F-B.31 | Degraded tools not visible in `tools/list` when backend nil | If K8s nil, CRD tools excluded from list |

---

## 5. Test Cases

> **Note on ID sequencing:** UT-AF-B-084 through UT-AF-B-086 were added after the initial 083-test allocation to close checkpoint audit gaps (nil auditor, zero-value config defaults). They are placed in their respective tier tables (Tier 3 and Tier 4) rather than at the end to maintain tier locality.

### 5.1 Tier 1: Core Dispatch (25 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-B-001 | `kubernaut_list_remediations` dispatches to HandleListRemediations with K8sClient | BAC-BRIDGE-01 | P0 |
| UT-AF-B-002 | `kubernaut_get_remediation` dispatches to HandleGetRemediation | BAC-BRIDGE-01 | P0 |
| UT-AF-B-003 | `kubernaut_submit_signal` dispatches with username from context | BAC-BRIDGE-10 | P0 |
| UT-AF-B-004 | `kubernaut_approve` dispatches with username from context | BAC-BRIDGE-10 | P0 |
| UT-AF-B-005 | `kubernaut_cancel_remediation` dispatches to HandleCancelRemediation | BAC-BRIDGE-01 | P0 |
| UT-AF-B-006 | `kubernaut_watch` dispatches to HandleWatch | BAC-BRIDGE-01 | P0 |
| UT-AF-B-007 | `af_list_events` uses TriageFactory for impersonated client | BAC-BRIDGE-01 | P0 |
| UT-AF-B-008 | `af_get_pods` uses TriageFactory | BAC-BRIDGE-01 | P0 |
| UT-AF-B-009 | `af_get_workloads` uses TriageFactory | BAC-BRIDGE-01 | P0 |
| UT-AF-B-010 | `af_resolve_owner` uses TriageFactory | BAC-BRIDGE-01 | P0 |
| UT-AF-B-011 | `af_check_existing_rr` dispatches to HandleCheckExistingRR | BAC-BRIDGE-01 | P0 |
| UT-AF-B-012 | `af_create_rr` dispatches with &args and username | BAC-BRIDGE-10 | P0 |
| UT-AF-B-013 | `kubernaut_list_workflows` dispatches to HandleListWorkflows with DSClient | BAC-BRIDGE-01 | P0 |
| UT-AF-B-014 | `kubernaut_get_remediation_history` dispatches with DSClient | BAC-BRIDGE-01 | P0 |
| UT-AF-B-015 | `kubernaut_get_effectiveness` dispatches with DSClient | BAC-BRIDGE-01 | P0 |
| UT-AF-B-016 | `kubernaut_get_audit_trail` dispatches with DSClient | BAC-BRIDGE-01 | P0 |
| UT-AF-B-017 | `kubernaut_start_investigation` dispatches to HandleStartInvestigation with KAClient | BAC-BRIDGE-01 | P0 |
| UT-AF-B-018 | `kubernaut_poll_investigation` dispatches with maxPolls=5, pollInterval=3s | BAC-BRIDGE-01 | P0 |
| UT-AF-B-019 | `kubernaut_select_workflow` dispatches to HandleSelectWorkflow with MCPClient | BAC-BRIDGE-01 | P0 |
| UT-AF-B-020 | `present_decision` dispatches to HandlePresentDecision (no ctx/client) | BAC-BRIDGE-01 | P0 |
| UT-AF-B-021 | Tool result serialized as JSON content in CallToolResult | BAC-BRIDGE-01 | P0 |
| UT-AF-B-022 | Tool names in bridge match rbac_roles.yaml exactly (20 names) | BAC-BRIDGE-02 | P0 |
| UT-AF-B-023 | RegisterTools registers exactly 20 tools on the server | BAC-BRIDGE-01 | P0 |
| UT-AF-B-024 | Agent Card skills list matches registered MCP tool names | BAC-BRIDGE-01 | P0 |
| UT-AF-B-025 | poll_investigation with 1ms interval completes without flakiness | BAC-BRIDGE-01 | P1 |

### 5.2 Tier 2: Security (20 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-B-026 | RBAC denies `cicd` user calling `kubernaut_approve` | BAC-BRIDGE-02 | P0 |
| UT-AF-B-027 | RBAC allows `sre` user calling any tool | BAC-BRIDGE-02 | P0 |
| UT-AF-B-028 | RBAC denies user with no groups | BAC-BRIDGE-02 | P0 |
| UT-AF-B-029 | RBAC denial returns tool error (IsError=true), not protocol error | BAC-BRIDGE-07 | P0 |
| UT-AF-B-030 | RBAC denial message does not reveal allowed groups | BAC-BRIDGE-07 | P0 |
| UT-AF-B-031 | `tools/list` for `cicd` user returns 4 tools only | BAC-BRIDGE-03 | P0 |
| UT-AF-B-032 | `tools/list` for `sre` user returns 20 tools | BAC-BRIDGE-03 | P0 |
| UT-AF-B-033 | `tools/list` for `observability` user returns 8 tools | BAC-BRIDGE-03 | P0 |
| UT-AF-B-034 | `tools/list` for `l3-audit` user returns 6 tools | BAC-BRIDGE-03 | P0 |
| UT-AF-B-035 | Nil K8sClient: `kubernaut_list_remediations` returns ErrK8sUnavailable | BAC-BRIDGE-06 | P0 |
| UT-AF-B-036 | Nil K8sClient: all 12 K8s tools return graceful error (no panic) | BAC-BRIDGE-06 | P0 |
| UT-AF-B-037 | Nil DSClient: `kubernaut_list_workflows` returns ErrServiceUnavailable | BAC-BRIDGE-06 | P0 |
| UT-AF-B-038 | Nil DSClient: all 4 DS tools return graceful error (no panic) | BAC-BRIDGE-06 | P0 |
| UT-AF-B-039 | Nil KAClient: `kubernaut_start_investigation` returns graceful error | BAC-BRIDGE-06 | P0 |
| UT-AF-B-040 | Nil MCPClient: `kubernaut_select_workflow` returns graceful error | BAC-BRIDGE-06 | P0 |
| UT-AF-B-041 | Error from HandleListRemediations redacted (no K8s API URL) | BAC-BRIDGE-07 | P0 |
| UT-AF-B-042 | Error from HandleStartInvestigation redacted (no KA base URL) | BAC-BRIDGE-07 | P0 |
| UT-AF-B-043 | Error with stack trace redacted | BAC-BRIDGE-07 | P0 |
| UT-AF-B-044 | TriageFactory error redacted (no rest.Config details) | BAC-BRIDGE-07 | P0 |
| UT-AF-B-045 | RBAC check with `present_decision` name matches yaml (not kubernaut_ prefix) | BAC-BRIDGE-02 | P0 |

### 5.3 Tier 3: Observability (17 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-B-046 | Success: `af_tool_calls_total{tool="kubernaut_list_remediations",outcome="success"}` incremented | BAC-BRIDGE-04 | P0 |
| UT-AF-B-047 | RBAC denied: `af_tool_calls_total{...,outcome="denied"}` incremented | BAC-BRIDGE-04 | P0 |
| UT-AF-B-048 | Error: `af_tool_calls_total{...,outcome="error"}` incremented | BAC-BRIDGE-04 | P0 |
| UT-AF-B-049 | Timeout: `af_tool_calls_total{...,outcome="timeout"}` incremented | BAC-BRIDGE-04 | P0 |
| UT-AF-B-050 | Duration histogram observed for successful call | BAC-BRIDGE-04 | P0 |
| UT-AF-B-051 | Duration histogram observed for failed call | BAC-BRIDGE-04 | P0 |
| UT-AF-B-052 | `af_mcp_rbac_denied_total{tool,user}` incremented on RBAC denial | BAC-BRIDGE-04 | P0 |
| UT-AF-B-053 | Success emits `EventMCPToolInvoked` with tool, session_id, duration_ms | BAC-BRIDGE-05 | P0 |
| UT-AF-B-054 | Failure emits `EventMCPToolFailed` with tool, redacted error | BAC-BRIDGE-05 | P0 |
| UT-AF-B-055 | RBAC denial emits `EventMCPToolDenied` with tool, user, groups | BAC-BRIDGE-05 | P0 |
| UT-AF-B-056 | MCP initialize emits `EventMCPSessionInit` | BAC-BRIDGE-11 | P1 |
| UT-AF-B-057 | Audit event detail includes request_id from context | BAC-BRIDGE-05 | P0 |
| UT-AF-B-058 | Metrics defined in registry are exposed at /metrics endpoint | BAC-BRIDGE-04 | P0 |
| UT-AF-B-059 | Every defined metric is incremented by at least one production code path | BAC-BRIDGE-04 | P0 |
| UT-AF-B-060 | Audit event UserID populated from auth context | BAC-BRIDGE-05 | P0 |
| UT-AF-B-061 | Audit event SourceIP populated | BAC-BRIDGE-05 | P1 |
| UT-AF-B-084 | Nil `Auditor` in MCPBridgeConfig: dispatch does not panic, audit silently skipped | BAC-BRIDGE-06 | P0 |

### 5.4 Tier 4: Resilience (14 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-B-062 | Tool exceeding 30s is cancelled and returns context.DeadlineExceeded | BAC-BRIDGE-08 | P0 |
| UT-AF-B-063 | Timeout error is redacted and user-friendly | BAC-BRIDGE-08 | P0 |
| UT-AF-B-064 | 5 concurrent tool calls on one session all succeed | BAC-BRIDGE-09 | P0 |
| UT-AF-B-065 | 6th concurrent call blocks until one of first 5 completes | BAC-BRIDGE-09 | P0 |
| UT-AF-B-066 | Semaphore released on handler panic (deferred recovery) | BAC-BRIDGE-09 | P0 |
| UT-AF-B-067 | 10 goroutines calling RegisterTools concurrently under -race | BAC-BRIDGE-09 | P0 |
| UT-AF-B-068 | 10 goroutines calling checkRBAC concurrently under -race | BAC-BRIDGE-09 | P0 |
| UT-AF-B-069 | Session semaphore does not leak after 50 acquire/release cycles | BAC-BRIDGE-09 | P0 |
| UT-AF-B-070 | Metrics map does not grow with unique tool names (bounded labels) | BAC-BRIDGE-04 | P0 |
| UT-AF-B-071 | Triage factory error under concurrent calls does not panic | BAC-BRIDGE-06 | P0 |
| UT-AF-B-072 | Cancelled context propagates to HandlePollInvestigation sleep loop | BAC-BRIDGE-08 | P1 |
| UT-AF-B-073 | Idle session timeout fires after configured duration | BAC-BRIDGE-09 | P1 |
| UT-AF-B-085 | Zero-value `ToolTimeout` in config defaults to 30s (not infinite) | BAC-BRIDGE-08 | P0 |
| UT-AF-B-086 | Zero-value `MaxConcurrentTools` in config defaults to 5 | BAC-BRIDGE-09 | P0 |

### 5.5 Adversarial Inputs (10 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-B-074 | Empty tool name in internal dispatch rejected | BAC-BRIDGE-07 | P0 |
| UT-AF-B-075 | Tool name with path traversal (`../../etc/passwd`) rejected by SDK schema | BAC-BRIDGE-07 | P0 |
| UT-AF-B-076 | Tool args with max-length+1 namespace string (254 chars) rejected | BAC-BRIDGE-07 | P0 |
| UT-AF-B-077 | Tool args with Unicode NUL bytes rejected | BAC-BRIDGE-07 | P0 |
| UT-AF-B-078 | Tool args with empty namespace (where required) returns validation error | BAC-BRIDGE-07 | P0 |
| UT-AF-B-079 | RBAC group name with special characters handled safely | BAC-BRIDGE-02 | P0 |
| UT-AF-B-080 | Username with CR/LF injection in audit event sanitized | BAC-BRIDGE-05 | P0 |
| UT-AF-B-081 | Session ID exceeding 256 chars handled gracefully | BAC-BRIDGE-09 | P1 |
| UT-AF-B-082 | Tool description with HTML/script tags not reflected unsanitized | BAC-BRIDGE-07 | P1 |
| UT-AF-B-083 | Concurrent calls with same singleflight key from MCP do not corrupt state | BAC-BRIDGE-09 | P0 |

---

## 6. Pass/Fail Criteria

### 6.1 Pass

- All 86 tests pass with `ginkgo -race`
- Coverage >= 80% for `internal/handler/mcp_bridge.go`
- Coverage >= 80% for modified `internal/handler/mcp.go`
- `golangci-lint run` reports 0 errors on all modified files
- No exported symbols from production packages used only in `_test.go`
- k6 scripts execute without error against wired bridge
- All 9 checkpoint categories satisfied at each tier boundary

### 6.2 Fail

- Any test fails under `-race`
- Coverage drops below 80% total (CI gate)
- Nil client causes panic (not graceful error)
- Internal URL/path visible in client-facing error text
- Metric defined but never incremented by production code
- Audit event documented in catalog but not emitted by code

---

## 7. Test Environment

- Go 1.25.6
- `k8s.io/client-go/dynamic/fake` for K8s simulation
- `ka.MockMCPClient` for KA MCP simulation
- `httptest.NewServer` + `ka.NewClient` for KA REST simulation
- Mock `ds.Client` interface implementation
- `github.com/prometheus/client_golang/prometheus/testutil` for metrics assertions
- Ginkgo v2 + Gomega (ADR-015)
- `-race` flag mandatory on all tests
- `funcr` logger for audit emission capture

---

## 8. Implementation Phases

### Phase 1: TDD Red — Tier 1 Core Dispatch

**Goal:** Write 25 failing tests (UT-AF-B-001 through UT-AF-B-025) that assert each tool dispatches to its correct `Handle*` function with the right dependencies.

**Test file:** `internal/handler/mcp_bridge_test.go`

**Fakes needed:**
- `fakeDSClient` implementing `ds.Client` with canned returns
- `httptest.Server` for KA REST (canned `/api/v1/incident/analyze`, `/session/{id}`, `/session/{id}/result`)
- `ka.MockMCPClient` with `SelectWorkflowFn`
- `dynamicfake.NewSimpleDynamicClient` with pre-loaded CRDs
- `fakeTriageFactory` returning a fake dynamic client
- `fakeAuditor` implementing `audit.Emitter` capturing events

**Red criteria:** All 25 tests compile but fail (bridge does not exist yet).

---

### Phase 2: TDD Green — Tier 1 Core Dispatch

**Goal:** Implement `mcp_bridge.go` with `RegisterTools` dispatching all 20 tools. Minimal implementation — no RBAC, no metrics, no timeout yet.

**Files created/modified:**
- NEW: `internal/handler/mcp_bridge.go`
- MODIFY: `internal/handler/mcp.go` (remove stubs, call RegisterTools)
- DELETE: `internal/handler/mcptools.go`
- MODIFY: `internal/handler/agentcard.go` (tool list source)
- MODIFY: `cmd/apifrontend/main.go` (pass deps, reorder RBAC load)

**Green criteria:** All 25 Tier 1 tests pass. `go build ./...` succeeds. `-race` passes.

---

### Phase 3: TDD Refactor — Tier 1

**Checklist (100 Go Mistakes):**
- [ ] #1: Unintended variable shadowing in closures (tool registration loop)
- [ ] #9: Being confused about when to use generics (AddTool type params)
- [ ] #26: Slices and memory leaks (tool name slice not retained)
- [ ] #29: Comparing values incorrectly (string comparison for tool names)
- [ ] #41: Not closing resources (semaphore, context cancel)
- [ ] #46: Using defer inside a loop (tool registration is not a loop body with defer)
- [ ] #56: Concurrency safety of shared state (MCPBridgeConfig is read-only after init)
- [ ] #78: Not using -race (already mandatory)
- [ ] #90: Not being aware of the impacts of running tests in parallel

**Refactoring actions:**
- Extract repeated handler wrapper logic into `wrapTool` helper (unexported)
- Ensure no loop variable capture issues in registration closures (Go 1.22+ loop var semantics)
- Verify struct alignment if MCPBridgeConfig is large

---

### CHECKPOINT 1 (after Tier 1)

Before advancing to Tier 2, verify all 9 categories:

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | No production metrics code exists in Tier 1 (metrics introduced in Tier 3). Deferred to Checkpoint 3 where UT-AF-B-046..052, 058, 059 will verify every metric is incremented. |
| 2 | Adversarial inputs | SDK JSON Schema rejects unknown tool names and malformed args at the protocol layer. UT-AF-B-022 (tool names match yaml exactly) provides Tier 1 input validation coverage. Full adversarial coverage deferred to Checkpoint 5 (UT-AF-B-074..083). |
| 3 | Resource bounds | `RegisterTools` called once at startup. No growing structures in Tier 1. |
| 4 | Concurrency | UT-AF-B-067 (10 goroutines calling RegisterTools) verifies registration is goroutine-safe. Bridge struct is immutable after init. |
| 5 | Nil/zero edge cases | No nil-client code paths exist in Tier 1 production code (Tier 1 dispatches with valid deps only). Nil-safety guards introduced in Tier 2 and verified at Checkpoint 2 by UT-AF-B-035..040. |
| 6 | Error-path observability | No error return paths exist in Tier 1 minimal dispatch (happy path only). Error paths are introduced in Tier 2; verified at Checkpoint 2 by UT-AF-B-041..044 (redacted errors include tool name for SRE diagnosis). |
| 7 | Cross-phase integration | Tier 1 establishes bridge; Tiers 2-4 build on it. Verified by test compilation. |
| 8 | Spec compliance | MCP protocol compliance: `AddTool` with Name, Description per MCP spec. tools/list response shape validated by SDK. JSON-RPC 2.0 shape inherited from SDK. |
| 9 | API surface hygiene | No exported test helpers. `MCPBridgeConfig`, `RegisterTools` must be exported (used by main.go). Verify no debug functions exported. |

**Escalation:** None expected. Proceed to Tier 2.

---

### Phase 4: TDD Red — Tier 2 Security

**Goal:** Write 20 failing tests (UT-AF-B-026 through UT-AF-B-045) that assert RBAC, nil-safety, and error redaction.

**Additional fakes:**
- Context with `UserIdentity{Groups: []string{"cicd"}}` for restricted user
- Context with `UserIdentity{Groups: []string{"sre"}}` for full access
- Context with nil `UserIdentity` for edge case

**Red criteria:** Tests compile, assert RBAC denial/nil-safety/redaction, all fail (not yet implemented).

---

### Phase 5: TDD Green — Tier 2 Security

**Goal:** Implement `checkRBAC`, nil-safety guards, `security.RedactError` wrapping, and `tools/list` filtering.

**Implementation:**
- `checkRBAC(ctx, toolName, roles, auditor)` → checks user groups against role map
- `filterToolsByGroups(groups, roles)` → returns allowed tool names
- Implement `tools/list` RBAC filtering via `srv.AddReceivingMiddleware(...)`
- Add nil checks: `if cfg.DSClient == nil { return ..., ErrServiceUnavailable }`
- Wrap all error returns: `return nil, zero, security.RedactError(err)`

**Green criteria:** All 45 tests pass (Tier 1 + Tier 2). `-race` passes.

---

### Phase 6: TDD Refactor — Tier 2

**Checklist (100 Go Mistakes):**
- [ ] #4: Overusing getters/setters (RBAC map access is direct, not wrapped)
- [ ] #12: Not using type assertion properly for error types
- [ ] #30: Not making slice copies when needed (rbac role slices shared?)
- [ ] #53: Not handling defer errors (RBAC logging)
- [ ] #62: Starting goroutine without knowing when to stop (no goroutines in RBAC path)
- [ ] #73: Not using testing utility packages (verify gomega matchers appropriate)

**Refactoring actions:**
- Ensure `checkRBAC` is O(1) lookup (map, not linear scan)
- Verify error wrapping preserves `errors.Is` / `errors.As` semantics
- Move `ErrServiceUnavailable` to shared errors package if reused elsewhere

---

### CHECKPOINT 2 (after Tier 2)

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | No production metrics code exists in Tier 2 (metrics introduced in Tier 3). Audit emit call exists in `checkRBAC` — verified by UT-AF-B-055 (RBAC denial emits `EventMCPToolDenied`). Full metrics coverage deferred to Checkpoint 3 (UT-AF-B-046..052, 058, 059). |
| 2 | Adversarial inputs | RBAC adversarial inputs covered by: UT-AF-B-028 (user with no groups denied), UT-AF-B-030 (denial message does not reveal allowed groups), UT-AF-B-045 (tool name mismatch rejected). Full adversarial suite (UT-AF-B-074..083) written in Phase 13 RED but not yet passing; deferred to Checkpoint 5. |
| 3 | Resource bounds | RBAC map is static (loaded at startup). `filterToolsByGroups` allocates a new slice per call — verify it does not retain references to the full tool list. |
| 4 | Concurrency | UT-AF-B-068: 10 goroutines calling `checkRBAC` concurrently. RBAC map is read-only → safe. |
| 5 | Nil/zero edge cases | UT-AF-B-035..040: nil clients. UT-AF-B-028: user with no groups. All covered. |
| 6 | Error-path observability | UT-AF-B-041..044: redacted errors include enough context (tool name in error message) for SRE diagnosis without revealing internals. Verify error text format: `"tool %q: service unavailable"`. |
| 7 | Cross-phase integration | RBAC guard from Tier 2 integrates with dispatch from Tier 1. UT-AF-B-026 proves: RBAC denial prevents tool execution (Tier 1 Handle* not called). |
| 8 | Spec compliance | N/A new spec requirements in Tier 2. |
| 9 | API surface hygiene | `checkRBAC`, `filterToolsByGroups` should be unexported (only used within `internal/handler`). `ErrServiceUnavailable` exported only if needed by tests in `handler_test` package. |

**Resolved:** `SetListToolsHandler` is not available in SDK v1.6.0. Verified mitigation: use `srv.AddReceivingMiddleware(...)` to intercept `"tools/list"` method, call the inner handler, then filter the result by user RBAC groups. No architectural change required.

---

### Phase 7: TDD Red — Tier 3 Observability

**Goal:** Write 17 failing tests (UT-AF-B-046 through UT-AF-B-061, plus UT-AF-B-084) asserting metrics, audit events, and nil-auditor safety.

**Additional fakes:**
- `prometheus.NewRegistry()` (isolated test registry)
- `testutil.ToFloat64()` for counter assertions
- `fakeAuditor` capturing emitted events with assertion helpers

**Red criteria:** Tests compile, assert metric values and audit events, all fail.

---

### Phase 8: TDD Green — Tier 3 Observability

**Goal:** Implement metrics instrumentation and audit event emission in the bridge dispatch wrapper.

**Implementation:**
- Add `MCPBridgeMetrics` to `internal/metrics/metrics.go`
- Instrument wrapper: record start time, observe duration, increment counters
- Emit audit events at appropriate points in dispatch
- Add `EventMCPToolDenied`, `EventMCPToolFailed`, `EventMCPSessionInit` constants
- Wire session init audit via MCP server callback (if SDK supports)

**Green criteria:** All 62 tests pass (Tier 1 + 2 + 3). `-race` passes.

---

### Phase 9: TDD Refactor — Tier 3

**Checklist (100 Go Mistakes):**
- [ ] #65: Not using notification channels correctly (metrics are sync, no channels needed)
- [ ] #70: Using mutexes inaccurately (prometheus registry handles its own sync)
- [ ] #83: Not enabling the -race flag (already enabled)
- [ ] #89: Writing inaccurate benchmarks (consider adding benchmark for dispatch hot path)
- [ ] #98: Not using the standard HTTP test utilities (metrics exposed via HTTP)

**Refactoring actions:**
- Ensure label values are bounded (tool names are static set of 20, not user input)
- Verify no high-cardinality labels (user label on RBAC denied counter is bounded by unique users, acceptable)
- Extract metrics observation into `defer` for cleaner code

---

### CHECKPOINT 3 (after Tier 3)

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | **CRITICAL**: UT-AF-B-059 verifies every defined metric is incremented by at least one production path. UT-AF-B-058 verifies metrics exposed at `/metrics`. All 3 counters + 1 histogram must have corresponding production increment. |
| 2 | Adversarial inputs | Tier 1: UT-AF-B-022 (tool names match yaml). Tier 2: UT-AF-B-028 (no groups), UT-AF-B-030 (denial message safe), UT-AF-B-045 (name mismatch). Full adversarial suite (UT-AF-B-074..083) deferred to Checkpoint 5. |
| 3 | Resource bounds | UT-AF-B-070: metrics map with bounded labels does not grow. Verify Prometheus `ConstLabels` vs dynamic labels. |
| 4 | Concurrency | Prometheus registry is concurrent-safe. Audit `BufferedEmitter` is concurrent-safe (uses internal mutex). No new concurrency concern. |
| 5 | Nil/zero edge cases | Nil auditor: bridge must not panic if `cfg.Auditor == nil` (skip audit). Verified by UT-AF-B-084 (nil auditor skips emit without panic). Nil clients covered by UT-AF-B-035..040 (Tier 2). |
| 6 | Error-path observability | UT-AF-B-054: failed tool emits audit with tool name, namespace (from args if available), redacted error text. SRE can identify which tool failed, for which user, without reading source. |
| 7 | Cross-phase integration | **KEY**: Metrics defined in Tier 3 (`metrics-wiring` todo) are called from Tier 1/2 dispatch code. UT-AF-B-046..052 prove the wiring. Audit events from `audit-events` todo are emitted by Tier 2 RBAC code — UT-AF-B-055 proves wiring. |
| 8 | Spec compliance | Prometheus metric naming: `af_` prefix, snake_case, unit suffix (`_total`, `_seconds`). Verify compliance with Prometheus naming conventions. |
| 9 | API surface hygiene | `MCPBridgeMetrics` exported (used by `main.go` and test). Internal helpers for metric observation unexported. |

**Escalation:** If nil auditor causes panic, add guard and corresponding test before proceeding.

---

### Phase 10: TDD Red — Tier 4 Resilience

**Goal:** Write 14 failing tests (UT-AF-B-062 through UT-AF-B-073, plus UT-AF-B-085 and UT-AF-B-086) asserting timeout, concurrency limits, resource bounds, and zero-value config defaults.

**Additional fakes:**
- `slowHandler` that sleeps 35s (exceeds 30s timeout)
- Channel-based synchronization for semaphore tests
- Counter for acquire/release cycle verification

**Red criteria:** Tests compile, assert timeout/semaphore behavior, all fail.

---

### Phase 11: TDD Green — Tier 4 Resilience

**Goal:** Implement context timeout wrapper, session semaphore, and idle timeout configuration.

**Implementation:**
- `context.WithTimeout(ctx, cfg.ToolTimeout)` + `defer cancel()` in dispatch wrapper
- Per-session semaphore: `sync.Map` keyed by `req.Session.ID()` → `*semaphore.Weighted(int64(cfg.MaxConcurrentTools))`
- `defer sem.Release(1)` with panic recovery
- Cleanup semaphore entry when session disconnects (SDK session lifecycle hooks or lazy eviction)
- Pass `IdleTimeout` to `StreamableHTTPHandler` options

**Green criteria:** All 76 tests pass (all tiers). `-race` passes.

---

### Phase 12: TDD Refactor — Tier 4

**Checklist (100 Go Mistakes):**
- [ ] #41: Not closing resources (context cancel MUST be deferred)
- [ ] #57: Inaccurate use of sync.WaitGroup (semaphore uses `golang.org/x/sync/semaphore`, not WaitGroup)
- [ ] #58: Forgetting about sync.Cond (not applicable — using semaphore)
- [ ] #61: Propagating an inappropriate context (timeout context must not leak to callers)
- [ ] #63: Not being careful with goroutines and loop variables (no loop + goroutine in Tier 4)
- [ ] #67: Being puzzled about channel direction (semaphore is not channel-based)
- [ ] #69: Not using select with channels properly (timeout uses context, not select)

**Refactoring actions:**
- Ensure timeout context cancel is always called (even on success path)
- Verify semaphore acquisition uses `ctx` to be cancellable if client disconnects
- Confirm panic recovery does not swallow the error (re-emit as tool error)

---

### CHECKPOINT 4 (after Tier 4)

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | Timeout increments `outcome="timeout"` counter. Verified by UT-AF-B-049. |
| 2 | Adversarial inputs | UT-AF-B-081: oversized session ID. UT-AF-B-083: singleflight race from MCP. |
| 3 | Resource bounds | UT-AF-B-069: 50 acquire/release cycles, semaphore count returns to 0. UT-AF-B-070: metrics label set bounded. |
| 4 | Concurrency | UT-AF-B-064..068, 071: concurrent tool calls, RBAC checks, triage factory under race. All pass with `-race`. |
| 5 | Nil/zero edge cases | Tier 2 covered nil clients (UT-AF-B-035..040). Tier 4 zero-value defaults: UT-AF-B-085 (zero `ToolTimeout` defaults to 30s, not infinite), UT-AF-B-086 (zero `MaxConcurrentTools` defaults to 5). |
| 6 | Error-path observability | Timeout error logged with tool name, session ID, elapsed time. UT-AF-B-063 asserts user-friendly timeout message. |
| 7 | Cross-phase integration | Semaphore (Tier 4) wraps dispatch (Tier 1) which includes RBAC (Tier 2) and metrics (Tier 3). Full integration verified by UT-AF-B-064 (5 concurrent calls all succeed with metrics). |
| 8 | Spec compliance | MCP spec: tools/call response must be valid JSON-RPC. Timeout error must still produce valid `CallToolResult` (SDK handles this via error wrapping). |
| 9 | API surface hygiene | Semaphore is unexported internal detail. `MaxConcurrentTools` and `ToolTimeout` on `MCPBridgeConfig` are exported (needed by main.go). No test-only exports. |

**Escalation:** None expected if all tests pass.

---

### Phase 13: TDD Red — Adversarial Inputs

**Goal:** Write 10 failing tests (UT-AF-B-074 through UT-AF-B-083) that assert malformed inputs are rejected at the appropriate layer (SDK schema validation, bridge-level guards, or Handle* validation).

**Test file:** `internal/handler/mcp_bridge_test.go` (adversarial section)

**Fakes needed:**
- Reuse existing fakes from Tiers 1-4
- Add table-driven test data with adversarial payloads

**Red criteria:** All 10 adversarial tests compile but fail (validation guards not yet implemented). Prior 76 tests (Tiers 1-4) still pass.

---

### Phase 14: TDD Green — Adversarial Inputs

**Goal:** Implement input validation guards in bridge dispatch so all 86 tests pass under `-race`.

**Implementation:**
- Add pre-dispatch validation for tool name (non-empty, no path traversal characters)
- Add namespace length validation before passing to Handle* (K8s RFC 1123: max 253 chars)
- Add NUL byte rejection in string args
- Sanitize CR/LF in username before audit event emission
- Add session ID length guard (truncate or reject > 256 chars)
- Ensure singleflight key derivation is deterministic under concurrent access

**Green criteria:** All 86 tests pass. `-race` passes.

---

### Phase 15: TDD Refactor — Adversarial Inputs

**Checklist (100 Go Mistakes):**
- [ ] #11: Not using functional options pattern (validation config could use options, but simple threshold constants are acceptable here)
- [ ] #15: Missing code documentation (each validation guard must have a comment citing the spec or security rationale)
- [ ] #28: Maps and memory leaks (adversarial key growth — verify no user-controlled strings become map keys)
- [ ] #45: Returning a nil receiver (validation functions must not return typed nil interface values)
- [ ] #73: Not using testing utility packages (adversarial tests should be table-driven with descriptive subtest names)
- [ ] #90: Not being aware of the impacts of running tests in parallel (table-driven adversarial tests must not share mutable state)

**Refactoring actions:**
- Convert adversarial tests to table-driven format with `DescribeTable` / `Entry` (Ginkgo)
- Ensure validation error messages do not echo back the malicious input (prevent log injection)
- Verify validation runs before RBAC check (fail fast on malformed input)
- Extract validation into `validateToolInput(toolName, args)` unexported helper

---

### CHECKPOINT 5 (after Adversarial Inputs)

Before advancing to the final phase, verify all 9 categories across all 86 tests:

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | All 3 counters + 1 histogram verified by UT-AF-B-046..052, 058, 059. Adversarial rejection increments `af_tool_calls_total{outcome="error"}` — verified by UT-AF-B-048. |
| 2 | Adversarial inputs | **COMPLETE**: UT-AF-B-074..083 cover empty string, max-length+1, path traversal, NUL bytes, empty required fields, special chars in groups, CR/LF injection, oversized session ID, HTML injection, and singleflight race. |
| 3 | Resource bounds | UT-AF-B-069 (semaphore cycles), UT-AF-B-070 (metric labels bounded). Adversarial inputs do not create new map entries (validation rejects before dispatch). |
| 4 | Concurrency | UT-AF-B-083 (singleflight race under concurrent adversarial calls). Combined with UT-AF-B-064..068, 071 from Tier 4. All pass under `-race`. |
| 5 | Nil/zero edge cases | UT-AF-B-035..040 (nil clients), UT-AF-B-084 (nil auditor), UT-AF-B-085..086 (zero config defaults). Adversarial: UT-AF-B-074 (empty tool name), UT-AF-B-078 (empty namespace). |
| 6 | Error-path observability | Validation errors include tool name and rejection reason in structured log. UT-AF-B-074..078 verify rejection. UT-AF-B-080 verifies sanitized audit output. |
| 7 | Cross-phase integration | Adversarial validation (Phase 14) runs before RBAC (Tier 2), dispatch (Tier 1), and metrics (Tier 3). UT-AF-B-074 proves: invalid input rejected before RBAC check. UT-AF-B-048 proves: rejection still increments error counter. |
| 8 | Spec compliance | Namespace validation per K8s RFC 1123 (max 253 chars, lowercase alphanumeric + hyphens). Session ID per MCP spec (opaque string, no length constraint in spec — we impose 256 char safety limit). |
| 9 | API surface hygiene | `validateToolInput` unexported. No test helpers exported. Table-driven test data in `_test.go` only. |

**Escalation:** None expected.

---

### Phase 16: Final Lint, Coverage, and Conformance Update

**Actions:**
- Run full `golangci-lint` on all modified files
- Verify 80% coverage gate
- Check for any exported symbols only used in tests
- Update conformance tests (`mcp_conformance_test.go`) for new tool count and real responses
- Final 100 Go Mistakes scan across all new code

---

### FINAL CHECKPOINT (pre-PR)

All 9 categories verified across the complete implementation:

| # | Category | Final Verification |
|---|----------|--------------------|
| 1 | Observability wiring | 3 counters + 1 histogram all have production callers. UT-AF-B-058, 059 prove exposure and increment. UT-AF-B-046..052 prove each outcome path increments. |
| 2 | Adversarial inputs | 10 dedicated tests (UT-AF-B-074..083) + SDK schema validation. All input boundaries verified. |
| 3 | Resource bounds | UT-AF-B-069 (50 semaphore acquire/release cycles), UT-AF-B-070 (metric labels bounded to static set of 20). |
| 4 | Concurrency | UT-AF-B-064..068, 071, 083 — all under `-race`. Competing state transitions in UT-AF-B-083 (singleflight race). |
| 5 | Nil/zero edge cases | UT-AF-B-035..040 (nil clients), UT-AF-B-084 (nil auditor), UT-AF-B-085 (zero ToolTimeout → 30s), UT-AF-B-086 (zero MaxConcurrentTools → 5). |
| 6 | Error-path observability | UT-AF-B-041..044, 054, 063 — all error messages include tool name + context for SRE diagnosis without reading source. |
| 7 | Cross-phase integration | UT-AF-B-064 (concurrent dispatch with metrics + RBAC + timeout all active simultaneously). UT-AF-B-048 (adversarial rejection still increments error counter). |
| 8 | Spec compliance | MCP JSON-RPC 2.0, Prometheus naming (`af_` prefix, `_total`/`_seconds` suffixes), K8s RFC 1123 (inherited from Handle*). |
| 9 | API surface hygiene | `go vet`, grep for exported symbols only in tests, no debug functions. `validateToolInput`, `checkRBAC`, `filterToolsByGroups` all unexported. |

---

## 9. Traceability Matrix

| BAC | Test Cases | Count |
|-----|-----------|-------|
| BAC-BRIDGE-01 | UT-AF-B-001 to 025 | 25 |
| BAC-BRIDGE-02 | UT-AF-B-026 to 030, 045, 079 | 7 |
| BAC-BRIDGE-03 | UT-AF-B-031 to 034 | 4 |
| BAC-BRIDGE-04 | UT-AF-B-046 to 052, 058, 059, 070 | 10 |
| BAC-BRIDGE-05 | UT-AF-B-053 to 057, 060, 061, 080 | 8 |
| BAC-BRIDGE-06 | UT-AF-B-035 to 040, 071, 084 | 8 |
| BAC-BRIDGE-07 | UT-AF-B-029, 030, 041-044, 074-078, 082 | 12 |
| BAC-BRIDGE-08 | UT-AF-B-062, 063, 072, 085 | 4 |
| BAC-BRIDGE-09 | UT-AF-B-064-069, 073, 081, 083, 086 | 10 |
| BAC-BRIDGE-10 | UT-AF-B-003, 004, 012 | 3 |
| BAC-BRIDGE-11 | UT-AF-B-056 | 1 |
| BAC-BRIDGE-12 | k6 script execution (manual/CI) | — |

---

## 10. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| Bridge unit tests | `internal/handler/mcp_bridge_test.go` |
| Updated conformance tests | `internal/handler/mcp_conformance_test.go` |
| Updated MCP handler tests | `internal/handler/mcp_test.go` |
| k6 perf script (session flow) | `test/perf/mcp_tools.js` |
| k6 perf script (tools/call) | `tests/performance/scripts/mcp-tools-call.js` |
| This test plan | `docs/tests/19-20/test_plan.md` |

---

## 11. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| SDK `SetListToolsHandler` not available | **Resolved** | N/A | Use `srv.AddReceivingMiddleware(...)` to intercept and filter `tools/list` by RBAC groups. Verified in SDK v1.6.0 source. |
| Coverage drops below 80% due to bridge size | Medium | High | Each tier adds tests before code; monitor coverage incrementally |
| Polling test flakiness in CI | Medium | Low | Use 1ms poll interval; mock KA status response to return immediately |
| k6 script update breaks perf baseline | Low | Low | Update thresholds to account for real backend latency |
| Session idle timeout not natively supported by SDK | Medium | Medium | Implement via goroutine + timer wrapping the transport |
