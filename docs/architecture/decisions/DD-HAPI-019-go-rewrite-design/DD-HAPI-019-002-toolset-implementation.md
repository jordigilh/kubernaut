# DD-HAPI-019-002: Toolset Implementation Design

**Status**: ✅ Approved
**Decision Date**: 2026-03-04
**Version**: 1.1
**Confidence**: 85%
**Deciders**: Architecture Team, Kubernaut Agent Team
**Applies To**: Kubernaut Agent

**Related Business Requirements**:
- [BR-HAPI-433-002: Kubernetes Toolset](../../../requirements/BR-HAPI-433-go-language-migration/BR-HAPI-433-002-kubernetes-toolset.md)
- [BR-HAPI-433-003: Prometheus Toolset](../../../requirements/BR-HAPI-433-go-language-migration/BR-HAPI-433-003-prometheus-toolset.md)

---

## Changelog

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.1 | 2026-03-04 | Architecture Team | Added MCP Tool Provider skeleton (Option C), renamed HAPI-Custom Tools to Kubernaut Agent Custom Tools |
| 1.0 | 2026-03-04 | Architecture Team | Initial design: Go bindings for all toolsets, no shell execution |

---

## Context & Problem

### Current State

HolmesGPT implements all Kubernetes tools as `kubectl` subprocess calls and Prometheus tools as Python HTTP calls via `prometrix`. This creates:

1. **Shell injection vector**: `subprocess.run(cmd, shell=True, executable="/bin/bash")` with user-influenced arguments
2. **Image bloat**: kubectl (~50MB), helm, jq, krew plugins bundled in image
3. **No type safety**: String-based kubectl output parsed by the LLM, not structured
4. **Python dependency chain**: prometrix, requests, dateutil for Prometheus

### Problem Statement

Design a toolset implementation that:
- Uses Go bindings exclusively (client-go for K8s, net/http for Prometheus)
- Produces structured output (Go structs → JSON) for better LLM reasoning
- Integrates with the sanitization pipeline (I1, G4)
- Supports the `llm_summarize` transformer for large outputs

---

## Decision Drivers

1. **No shell execution**: Core security requirement of BR-HAPI-433
2. **Structured output**: JSON is more reliable for LLM reasoning than kubectl's text tables
3. **Built-in size control**: client-go's `TailLines` and `LimitBytes` for logs, response size limits for Prometheus
4. **Sanitization integration**: Tool output flows through the sanitization pipeline before reaching the LLM

---

## Decision

### Tool Interface

All tools implement a common interface:

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage  // JSON Schema for LLM
    Execute(ctx context.Context, args json.RawMessage) (ToolResult, error)
}

type ToolResult struct {
    Content string  // Serialized result (JSON or text)
    Error   string  // Error message if execution failed
}
```

### Tool Registry and Scoping

```go
type Registry struct {
    tools map[string]Tool
}

func (r *Registry) ToolsForPhase(phase Phase) []ToolDefinition {
    // Returns LLM-compatible tool definitions for the given investigation phase
    // Phase determines which tools are available (per-phase scoping, I4)
}

func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) (ToolResult, error) {
    // 1. Look up tool by name
    // 2. Execute tool
    // 3. Run result through sanitization pipeline (I1 + G4)
    // 4. If result exceeds threshold, run llm_summarize
    // 5. Return sanitized result
}
```

### Kubernetes Tools (client-go)

All 11 Kubernetes tools use `k8s.io/client-go` dynamic and typed clients.

**Key design choices**:

| Choice | Decision | Rationale |
|---|---|---|
| Client type | Dynamic client (`client.Resource(gvr)`) for describe/get, typed client (`client.CoreV1()`) for logs/events | Dynamic client handles any GVR without code generation. Typed client provides streaming for logs. |
| Output format | Go structs → JSON | More reliable for LLM reasoning than kubectl text tables. The LLM sees structured fields, not parsed output. |
| Log size control | `TailLines` (default 500) + `LimitBytes` (default 256KB) | Built into client-go's `PodLogOptions`. Prevents unbounded log ingestion. |
| Log grep | Server-side `GetLogs()` + Go `strings.Contains` line filter | Server fetches logs, Go filters locally. Reduces data sent to LLM while keeping implementation simple. |
| Describe equivalent | Structured summary from `Get()` result | kubectl's `describe` is a complex pretty-printer. Our structured JSON is actually better for LLM reasoning — fields are explicit, not parsed from formatted text. |

**Example: kubectl_describe equivalent**

```go
func (t *DescribeTool) Execute(ctx context.Context, args json.RawMessage) (ToolResult, error) {
    var params struct {
        Kind      string `json:"kind"`
        Name      string `json:"name"`
        Namespace string `json:"namespace"`
    }
    json.Unmarshal(args, &params)

    gvr := resolveGVR(params.Kind)
    obj, err := t.client.Resource(gvr).Namespace(params.Namespace).Get(ctx, params.Name, metav1.GetOptions{})
    // ... error handling ...

    summary := buildStructuredSummary(obj)  // Extract status, conditions, labels, events
    content, _ := json.Marshal(summary)
    return ToolResult{Content: string(content)}, nil
}
```

### Prometheus Tools (net/http)

All 6 Prometheus tools use a single `PrometheusClient` wrapping `net/http`.

```go
type PrometheusClient struct {
    httpClient  *http.Client
    baseURL     string
    headers     map[string]string
    timeout     time.Duration
    maxTimeout  time.Duration
    sizeLimit   int  // default 30000 chars
}
```

**Provider support via pluggable transport**:

| Provider | Implementation |
|---|---|
| Standard Prometheus | Default `http.Transport` with config headers |
| AWS AMP | `aws-sdk-go-v2` SigV4 signing middleware on `http.Transport` |
| OpenShift | Bearer token from `/var/run/secrets/` injected in headers |
| VictoriaMetrics | Same client (API-compatible) |
| Thanos Query | Same client (API-compatible) |

**Response size handling**: When response exceeds `sizeLimit`, return a truncated summary with a `topk()` suggestion for the LLM to narrow its query — same behavior as current Python implementation.

### Kubernaut Agent Custom Tools

| Tool | Implementation |
|---|---|
| `list_available_actions` | DataStorage OpenAPI client `GET /api/v1/action-types` |
| `list_workflows` | DataStorage OpenAPI client `POST /api/v1/workflows/search` |
| `get_workflow` | DataStorage OpenAPI client `GET /api/v1/workflows/{id}` |
| `get_resource_context` | client-go owner chain resolution + DataStorage remediation history |

These preserve the DD-HAPI-017 three-step discovery protocol exactly.

### Sanitization Integration

Every tool execution flows through the sanitization pipeline:

```
Tool.Execute() → raw result
    ↓
G4: Credential scrubbing (BR-HAPI-211 patterns)
    ↓
I1: Prompt injection stripping (instruction-like patterns in tool output)
    ↓
Size check: if > threshold → llm_summarize (secondary LLM call)
    ↓
Sanitized result → appended to conversation as role:"tool" message
```

### llm_summarize Transformer

Carried forward from HolmesGPT. When tool output exceeds a configurable threshold (default 1000 chars), a secondary LLM call summarizes the output before returning it to the investigation loop.

```go
type Summarizer struct {
    llmClient  llm.Client  // can use a cheaper/faster model
    threshold  int
}

func (s *Summarizer) MaybeSummarize(ctx context.Context, toolName string, result string) (string, error) {
    if len(result) <= s.threshold {
        return result, nil
    }
    // Secondary LLM call with summarization prompt
    // "Summarize the following {toolName} output, preserving key details for incident investigation:"
}
```

### MCP Tool Provider (v1.3 Skeleton, v1.4 Transport)

Kubernaut Agent supports future MCP (Model Context Protocol) tool extensibility. In v1.3, the architecture is wired but the transport is stubbed:

```go
// pkg/kubernautagent/tools/mcp/provider.go
type MCPToolProvider interface {
    DiscoverTools(ctx context.Context) ([]tools.Tool, error)
    Close() error
}
```

```go
// pkg/kubernautagent/tools/mcp/config.go
type MCPServerConfig struct {
    Name      string `yaml:"name"`
    URL       string `yaml:"url"`
    Transport string `yaml:"transport"` // "sse" | "stdio"
}
```

**Registry integration** (`pkg/kubernautagent/tools/mcp/registry_integration.go`): Iterates configured `mcp_servers`, calls `DiscoverTools` on each provider, and registers returned tools in the main `tools.Registry`. Each MCP-discovered tool is wrapped as a standard `tools.Tool`.

**v1.3 stub** (`pkg/kubernautagent/tools/mcp/stub.go`): Logs a warning ("MCP servers configured but transport not implemented until v1.4") and returns an empty tool list. This ensures the config parsing and registration wiring are tested without premature transport complexity.

**v1.4 evolution**: Replace stub with real `MCPClient` (e.g., `mark3labs/mcp-go`) implementing SSE transport. The `MCPToolProvider` interface and registry integration remain unchanged.

---

## Consequences

### Positive Consequences

1. **Zero shell execution**: No subprocess calls, no shell injection vector
2. **No CLI binaries**: ~50MB+ removed from image (kubectl, helm, jq, krew)
3. **Type-safe tool arguments**: Go structs with JSON Schema validation
4. **Structured output**: LLM receives JSON fields, not parsed text tables
5. **Built-in size control**: client-go TailLines/LimitBytes for logs, response size limit for Prometheus

### Negative Consequences

1. **kubectl describe parity**: Our structured summary won't match kubectl's exact output format. LLMs trained on kubectl output may need prompt adjustment.
   - **Mitigation**: Structured JSON is actually better for LLM reasoning. Prompts guide the LLM to expect JSON format.

2. **Dynamic GVR resolution**: Need to handle custom resources without pre-generated clients.
   - **Mitigation**: Use dynamic client (`client.Resource(gvr)`) which handles any GVR.

### Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| LLM confused by JSON vs kubectl text | Low | Medium | Prompt engineering, mock-llm test validation |
| Missing K8s API fields in structured summary | Medium | Low | Iterate based on investigation scenarios |
| Prometheus provider-specific API quirks | Low | Low | VictoriaMetrics and Thanos are API-compatible |

---

## Validation Strategy

1. **Unit tests**: Each tool tested with client-go fakes / httptest servers
2. **Integration tests**: Full tool execution against Kind cluster / Prometheus stub
3. **Mock-llm parity**: Same investigation scenarios produce equivalent workflow selections
4. **Image size**: Verify no CLI binaries in final image (`docker run ... ls /usr/local/bin/`)
5. **Sanitization**: Inject known prompt injection payloads in tool output, verify they are stripped

---

## References

- [#508](https://github.com/jordigilh/kubernaut/issues/508): Kubernetes toolset scope
- [#509](https://github.com/jordigilh/kubernaut/issues/509): Prometheus toolset scope
- [DD-HAPI-017](../DD-HAPI-017-three-step-workflow-discovery-integration.md): Three-step workflow discovery

---

**Document Version**: 1.1
**Last Updated**: 2026-03-04
