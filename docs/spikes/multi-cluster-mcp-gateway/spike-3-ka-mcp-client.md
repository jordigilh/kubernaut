# Spike 3: KA as MCP Client

**Date**: 2026-06-04
**Status**: Complete
**Objective**: Prototype KA connecting through the MCP Gateway to execute tools on a remote cluster.

## Implementation

### Files Created/Modified

| File | Type | Purpose |
|------|------|---------|
| `pkg/kubernautagent/tools/mcp/streamable_provider.go` | NEW | Real MCP client provider using StreamableClientTransport |
| `pkg/kubernautagent/tools/mcp/bridge_tool.go` | NEW | Wraps MCP-discovered tools as KA `tools.Tool` instances |
| `pkg/kubernautagent/tools/mcp/registry_integration.go` | MODIFIED | Added `DiscoverAndBridge()` for registry integration |
| `pkg/kubernautagent/tools/mcp/streamable_provider_test.go` | NEW | Tests for provider discovery, sessions, error handling |
| `pkg/kubernautagent/tools/mcp/bridge_tool_test.go` | NEW | Tests for tool execution, error propagation, interface compliance |

### Architecture

```
KA Investigator
   │
   ├── Local Tools (client-go)     ← K8s-originated signals
   │   └── pkg/kubernautagent/tools/k8s/
   │
   └── Remote Tools (MCP Bridge)   ← ServiceNow signals
       └── pkg/kubernautagent/tools/mcp/
           ├── StreamableProvider.DiscoverTools()  → connects to gateway, lists tools
           ├── StreamableProvider.NewSession()      → creates session for execution
           └── BridgeTool.Execute()                 → session-per-call tool invocation
```

### Key Design Decisions

1. **Session-per-call pattern**: Each `BridgeTool.Execute()` creates a new MCP session, calls the tool, and closes the session. This matches AF's `SDKMCPClient.callTool()` pattern and avoids session lifecycle complexity.

2. **SessionFactory interface**: `BridgeTool` depends on `SessionFactory` (interface), not `StreamableProvider` (concrete). This enables test doubles and future optimizations (pooled sessions).

3. **Schema bridging**: MCP tool `InputSchema` (type `any`) is marshaled to `json.RawMessage` for KA's `tools.Tool.Parameters()`. No schema transformation needed -- both use JSON Schema.

4. **Error propagation**: Remote tool errors (`result.IsError`) are converted to Go errors with server name context. Connection failures include the server name and endpoint for debugging.

5. **Built on existing infrastructure**: Reuses `MCPToolProvider` interface, `ServerConfig`, `Tool` struct from the existing stub provider package. The `StubProvider` remains as a fallback.

### Test Results

```
14 Passed | 0 Failed | 0 Pending | 0 Skipped
```

Test coverage includes:
- Tool discovery from a live MCP server (httptest)
- Tool execution with text result extraction
- Structured result handling (JSON objects)
- Remote tool error propagation
- Connection failure handling
- Multi-tool bridging
- Tool interface compliance (Name, Description, Parameters)

### Integration with KA's Tool Registry

Usage in `cmd/kubernautagent/main.go` (to be implemented during #1338):

```go
if cfg.MCPGateway.URL != "" {
    provider := mcp.NewStreamableProvider(mcp.ServerConfig{
        Name:      cfg.MCPGateway.Name,
        URL:       cfg.MCPGateway.URL,
        Transport: "streamable-http",
    }, httpClient, logger)

    mcpTools, err := mcp.DiscoverAndBridge(ctx, []*mcp.StreamableProvider{provider}, logger)
    if err != nil {
        logger.Error(err, "failed to discover MCP tools")
    } else {
        for _, t := range mcpTools {
            reg.Register(t)
        }
    }
}
```

### Key Questions Answered

| Question | Answer |
|----------|--------|
| Can we use `mcp.NewClient()` from the SDK? | **YES** -- same SDK used by AF for KA communication |
| How do we bridge MCP tool schemas to KA's `tools.Tool`? | **Direct marshal** -- `InputSchema` (any) -> `json.RawMessage`. No transformation needed. |
| How does cluster targeting work? | Through tool prefix (e.g., `cluster_a_pods_list`) set by `MCPServerRegistration.spec.prefix`. KA selects tools by prefix. |
| What happens when gateway is unreachable? | `NewSession()` returns error with server name context. `BridgeTool.Execute()` propagates the error. Investigator can fall back to local tools or report the failure. |

## Decision

**GO**: KA MCP client works. The bridge pattern cleanly maps MCP-discovered tools to KA's registry. Session-per-call latency is acceptable for investigation (tools are called sequentially, not in parallel).
