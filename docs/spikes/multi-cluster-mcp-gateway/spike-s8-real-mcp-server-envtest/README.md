# Spike S8 — Real kubernetes-mcp-server against envtest

## Objective

Validate that the real `kubernetes-mcp-server` binary can run against envtest
and serve MCP tool calls, proving the IT test pattern for fleet scope checking.

## Findings

### PASS — All 4 test scenarios validated

| Test | Description | Result |
|------|-------------|--------|
| S8-001 | Connect and list tools | PASS |
| S8-002 | `resources_list` with labelSelector | PASS |
| S8-003 | `resources_get` with labels | PASS |
| S8-004 | Full scope check pattern (namespace + resource) | PASS |

### Key Metrics

- **Startup time**: ~1 second (K8s MCP Server ready to serve)
- **Total test time**: 3.4 seconds (including envtest startup)
- **Memory**: Minimal (envtest + single binary process)

### IT Pattern Validated

```go
// 1. Start envtest -> get rest.Config
// 2. Write kubeconfig from rest.Config
// 3. Start kubernetes-mcp-server --kubeconfig <path> --port <free> --stateless --read-only
// 4. Connect via mcp.StreamableClientTransport{Endpoint: "http://127.0.0.1:<port>/mcp"}
// 5. Call tools (resources_list, resources_get) to verify scope
```

### Critical Implementation Details

- Endpoint format: `http://host:port/mcp` (NOT just `http://host:port`)
- Use `--stateless` flag (required for test clients without session persistence)
- Use `--read-only` for scope-check-only scenarios
- Use `--disable-multi-cluster` for single-cluster envtest
- Use `--list-output yaml` for parseable responses
- `labelSelector` parameter is supported by `resources_list`

### Implications for Fleet IT Tests

1. **No mock needed**: IT tests can use the real K8s MCP Server binary
2. **Fast execution**: Total test time under 5 seconds
3. **Real wiring**: Proves actual MCP protocol flow (client -> server -> K8s API)
4. **Scope validation**: `labelSelector=kubernaut.ai/managed=true` works for filtering

### Recommendation

Adopt this pattern for all fleet IT tests:
- Each IT test suite starts envtest + K8s MCP Server in `BeforeSuite`
- Tests call tools via MCP SDK `ClientSession`
- No mock MCP Gateway needed for scope checking ITs
- For multi-cluster simulation: start 2 MCP Server instances on different ports,
  each with a different envtest (separate kubeconfigs)
