# Test Plan — #1306: Wire G2 KASessionPool for Persistent MCP Sessions

**IEEE 829 Compliant** | **Issue**: [#1306](https://github.com/jordigilh/kubernaut/issues/1306)

## 1. Test Plan Identifier

TP-1306-PERSISTENT-MCP-SESSION

## 2. Introduction

The AF `SDKMCPClient.callTool()` creates a new MCP HTTP session for every tool
call (Connect → CallTool → Close). On the KA side, session close fires
`SessionClosedHandler` which releases the interactive driver lease. This makes
multi-step MCP interactive flows impossible — `takeover` acquires a lease that
is immediately released when the AF closes the session.

This test plan covers the `PooledMCPClient` implementation that wraps the
existing `KASessionPool` to provide persistent MCP sessions keyed by
`(rr_id, username)`.

## 3. Test Items

| Item | File | Description |
|------|------|-------------|
| `PooledMCPClient` | `pkg/apifrontend/ka/pooled_mcp_client.go` | MCPClient impl using pool Acquire/Release |
| Session factory | `cmd/apifrontend/main.go` | Real SessionFactory wiring |
| Pool release on complete/cancel | `pkg/apifrontend/ka/pooled_mcp_client.go` | Auto-release on terminal actions |
| Background eviction | `cmd/apifrontend/main.go` | Idle session cleanup goroutine |

## 4. Features to Be Tested

- BR-INTERACTIVE-001: MCP sessions persist across tool calls for same (rr_id, user)
- BR-INTERACTIVE-002: Session isolation — different users get different sessions
- BR-INTERACTIVE-003: Terminal actions (complete, cancel) release pooled session
- BR-INTERACTIVE-004: Stale session reconnect on CallTool failure
- BR-SHUTDOWN-001: DrainAll closes all pooled sessions on SIGTERM
- BR-PERF-001: Session reuse avoids reconnect overhead

## 5. Features Not Tested

- KA-side MCP session management (tested by KA's own test suite)
- TLS/auth transport (already tested by existing SDKMCPClient tests)
- Mock LLM tool ordering (covered by #1307)

## 6. Approach

Testing Pyramid Invariant: UT proves logic. IT proves wiring. E2E proves the journey.

### 6.1 Unit Tests (pkg/apifrontend/ka/)

| ID | Scenario | Asserts |
|----|----------|---------|
| UT-AF-1306-001 | PooledMCPClient.InvokeAction acquires session from pool | pool.Acquire called with correct (rr_id, username) |
| UT-AF-1306-002 | PooledMCPClient.InvokeAction reuses existing session | factory called once for two sequential calls |
| UT-AF-1306-003 | PooledMCPClient.InvokeAction(complete) releases session | pool.Release called after successful complete |
| UT-AF-1306-004 | PooledMCPClient.InvokeAction(cancel) releases session | pool.Release called after successful cancel |
| UT-AF-1306-005 | PooledMCPClient.DiscoverWorkflows acquires session | pool.Acquire called; CallTool dispatched to pooled session |
| UT-AF-1306-006 | PooledMCPClient.SelectWorkflow acquires session | pool.Acquire called; CallTool dispatched to pooled session |
| UT-AF-1306-007 | User isolation — different users get different sessions | two users, same rr_id → factory called twice |
| UT-AF-1306-008 | Identity missing returns error | no UserIdentity in ctx → error before pool.Acquire |
| UT-AF-1306-009 | Pool.Acquire error propagated | factory error → user-friendly error returned |
| UT-AF-1306-010 | CallTool error on stale session → evict + retry | session.CallTool fails → pool.Release + pool.Acquire + retry |
| UT-AF-1306-011 | Investigate uses session-per-call (not pooled) | Investigate still uses SDKMCPClient directly |
| UT-AF-1306-012 | Compile-time MCPClient interface check | `var _ MCPClient = (*PooledMCPClient)(nil)` |

### 6.2 Integration Tests (cmd/apifrontend/)

| ID | Scenario | Asserts |
|----|----------|---------|
| IT-AF-1306-001 | main.go wires real SessionFactory | pool.Acquire succeeds (factory creates real connection) |
| IT-AF-1306-002 | PooledMCPClient injected as MCPClient for agent | AgentConfig.MCPClient is PooledMCPClient |
| IT-AF-1306-003 | Background eviction goroutine starts | EvictIdle called on interval |
| IT-AF-1306-004 | DrainAll on shutdown closes active sessions | SIGTERM → pool drained |

### 6.3 E2E Tests (test/e2e/apifrontend/)

| ID | Scenario | Asserts |
|----|----------|---------|
| E2E-AF-1306-001 | takeover → discover_workflows succeeds | No not_driving error; workflows returned |
| E2E-AF-1306-002 | takeover → discover → select → complete | Full interactive lifecycle; session released at end |

## 7. Pass/Fail Criteria

- All UT/IT tests pass with >=80% coverage of `pooled_mcp_client.go`
- E2E tests pass in Kind cluster with real KA
- No regression in existing `session_pool_test.go` or `mcp_sdk_client_test.go`
- Zero data races under `go test -race`

## 8. Suspension / Resumption

Suspend if KA MCP protocol changes (e.g., session lifetime model). Resume after
protocol alignment.

## 9. Environmental Needs

- Go 1.23+, MCP Go SDK v1.6.0
- Kind cluster for E2E (existing CI infrastructure)
- Mock KA MCP server for UT (fake PoolSession)
