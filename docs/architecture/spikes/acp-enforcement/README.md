# Spike 2: ACP Server Enforcement Layer

**Status**: Complete — the budget/shadow/audit enforcement concerns validated here survive, but relocated to the `AuthBridge` sidecar ([#1535](https://github.com/jordigilh/kubernaut/issues/1535), [#1681](https://github.com/jordigilh/kubernaut/issues/1681)) instead of a Kubernaut-run "ACP server." See [#1536](https://github.com/jordigilh/kubernaut/issues/1536).
**Date**: 2026-05-19  
**Confidence**: 95%

## Objective

Prototype the universal ACP server enforcement layer that intercepts tool
calls from any runtime (Goose, OAS/LangGraph, Deep Agents) to enforce
feature parity with KA v1.5:

1. Tool call budgets (mirrors `AnomalyDetector`)
2. Shadow agent feed (mirrors `alignment.SubmitToolStep`)
3. Audit event emission (mirrors `audit.StoreBestEffort`)

## Test Results

| # | Test | Result |
|---|---|---|
| 1 | Tool calls within budget succeed | PASS |
| 2 | Per-tool limit triggers rejection | PASS |
| 3 | Total budget triggers rejection | PASS |
| 4 | Exempt tools bypass budgets (e.g., `todo_*`) | PASS |
| 5 | Audit events emitted for all calls | PASS |
| 6 | Reset clears counters between phases | PASS |
| 7 | Shadow feed receives results in order | PASS |
| 8 | Failing tools record audit failure events | PASS |
| 9 | WrapForRuntime returns all registered handlers | PASS |

## Design

### Interception Pattern

The enforcement layer uses the same decorator pattern as KA's
`alignment.ToolProxy`, but operates at the ACP server level:

```
Runtime (Goose/OAS/DeepAgent)
  └─ tool_registry[tool_name]
       └─ EnforcementLayer.wrapHandler()
            ├─ checkBudget()          — reject if over limit
            ├─ handler(ctx, args)     — delegate to real tool
            ├─ emitAudit()            — fire-and-forget audit event
            └─ shadowFeed()           — feed result to shadow agent
```

### Integration Points

| KA v1.5 Component | ACP Equivalent | Integration |
|---|---|---|
| `AnomalyDetector` | `EnforcementLayer.checkBudget()` | Same thresholds, same semantics |
| `alignment.SubmitToolStep` | `ShadowFeed` callback | ACP server runs shadow evaluator or delegates to KA |
| `audit.StoreBestEffort` | `AuditSink` callback | Routes to Data Storage via OAS client |
| `registry.ToolRegistry` | `WrapForRuntime()` | Returns map[string]ToolHandler for runtime injection |

### Runtime Adapter Pattern

Each runtime adapter calls `WrapForRuntime()` and injects the result:

**OAS/LangGraph** (Python):
```python
# ACP server builds tool_registry from enforcement layer
tool_registry = acp_enforcement.wrap_for_runtime()
loader = AgentSpecLoader(tool_registry=tool_registry)
graph = loader.load_yaml(spec_yaml)
```

**Goose** (Rust CLI):
```
ACP server exposes MCP endpoint → Goose connects as MCP client
Tool calls from Goose → ACP MCP server → enforcement → KA
```

**Deep Agents** (LangGraph):
```python
# Similar to OAS but with LangGraph native tools
tools = [enforcement.wrap(name, handler) for name, handler in ka_tools.items()]
graph = create_react_agent(llm, tools)
```

## Key Findings

### F1: Decorator Pattern Scales to All Runtimes

The `map[string]ToolHandler` interface is universal. Each runtime has
its own mechanism for tool registration, but they all boil down to
"name -> handler function". The enforcement layer wraps at this boundary.

### F2: Budget Config is Portable

`BudgetConfig` mirrors KA's `AnomalyConfig` exactly. The ACP server
reads the same YAML config section and applies identical thresholds.

### F3: Shadow Feed is Pluggable

The `ShadowFeed` callback decouples the enforcement layer from the
shadow agent implementation. Options:
- ACP server runs its own shadow evaluator (independent of KA)
- ACP server delegates to KA's shadow agent via gRPC/MCP
- Both (primary + remote)

### F4: Audit Events Use Existing Schema

The `AuditEvent` structure mirrors KA's `audit.AuditEvent`. The ACP
server introduces one new event type prefix: `aiagent.runtime.*` to
distinguish runtime-mediated tool calls from KA-native ones.

## Files

| File | Purpose |
|---|---|
| `enforcement.go` | Core enforcement layer: budget, shadow, audit |
| `enforcement_test.go` | Test suite validating all interception behaviors |
| `go.mod` | Module definition |

## Architecture Decision: Where to Run Shadow Agent

| Option | Pros | Cons |
|---|---|---|
| ACP server runs its own shadow | Independent failure domain; lower latency | Duplicate shadow LLM costs |
| Delegate to KA's shadow via gRPC | Single shadow instance; cost efficient | Network hop; coupling to KA |
| Hybrid: local canary + remote full | Best of both; canary catches obvious issues fast | Complexity |

Recommendation: **Hybrid** for v1.6 — local canary in ACP, full grounding
review delegated to KA's existing shadow infrastructure.
