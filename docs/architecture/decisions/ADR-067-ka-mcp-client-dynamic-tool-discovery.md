# ADR-067: KA MCP Client with Dynamic Tool Discovery

**Status**: Proposed
**Date**: 2026-06-15
**Deciders**: Architecture Team
**Context**: Fleet remediation (ACM) requires multi-cluster investigation via remote MCP servers
**Related**: ADR-064 (Multi-Cluster MCP Gateway), ADR-065 (Fleet Cluster Identity on RR), Issue #54

---

## Context

Kubernaut Agent (KA) currently investigates using local `client-go` tools bound to the cluster where KA runs. For fleet management with ACM, KA must investigate resources on managed clusters without direct kubeconfig access.

Each managed cluster runs an OCP MCP server (`openshift/openshift-mcp-server`) exposing cluster-specific tools (pods, events, logs, alertmanager, etc.) via the MCP protocol. The tool surface varies per cluster -- different OCP versions, different enabled toolsets, different installed operators.

KA needs to:
1. Discover what tools a target cluster exposes
2. Make those tools available to the LLM for investigation
3. Route tool calls back to the correct cluster
4. Do all of this without hardcoding tool definitions in KA

---

## Decision

### 1. KA as Generic MCP Client

KA connects to external MCP servers via the MCP Go SDK (`StreamableClientTransport`) and bridges discovered tools into its investigation pipeline. KA's MCP client code is protocol-level -- it does not know or care what tools the remote server exposes.

**Components** (in `pkg/kubernautagent/tools/mcp/`):

| Component | Purpose |
|-----------|---------|
| `StreamableProvider` | Connects to an MCP server, discovers tools via `tools/list` |
| `BridgeTool` | Wraps an MCP-discovered tool as a KA `tools.Tool` for LLM consumption |

Both reuse the same MCP SDK already used by the API Frontend's `SDKMCPClient`.

### 2. Programmatic Tool Discovery Before LLM Invocation

Tool discovery and scoping happen in Go code **before** the LLM prompt is built. The LLM never participates in discovery -- it receives a curated tool set and starts investigating immediately.

**Investigation flow:**

```
Signal arrives (e.g., "pod crash-looping on cluster prod-east")
    │
    ▼ Go code (deterministic, no LLM)
    │
    ├── Identify target cluster from signal (ClusterName on RR, per ADR-065)
    ├── Discover tools for that cluster (via MCP)
    ├── Bridge tools to KA format (BridgeTool)
    ├── Build prompt with bridged tools + Thanos + common tools
    │
    ▼ LLM agent (reasoning)
    │
    ├── Calls tools to investigate (pods_list, events_list, ...)
    ├── BridgeTool forwards each call to the MCP server
    │
    ▼ LLM conclusion
    │
    "Root cause: OOMKilled, memory limit 256Mi insufficient"
```

### 3. Prefix Stripping for Single-Cluster Investigations

When an investigation is scoped to a single cluster, KA strips the gateway-applied prefix from tool names before presenting them to the LLM. The LLM sees `pods_list`, not `prod_east_pods_list`. KA reconstructs the prefixed name when forwarding the tool call to the gateway.

```
Gateway tools/list:  prod_east_pods_list, prod_east_events_list, ...
LLM sees:            pods_list, events_list, ...
LLM calls:           pods_list(namespace: "default")
BridgeTool sends:    prod_east_pods_list(namespace: "default") → gateway
```

**Rationale**:
- Prompt portability: same prompt template works for every cluster
- Simpler LLM reasoning: `pods_list` is more natural than `prod_east_pods_list`
- Tool descriptions stay generic
- Future: if multi-cluster investigation is needed, prefixes can be reintroduced to disambiguate

### 4. Tool Surface Defined by Remote MCP Server

KA does not maintain a hardcoded list of expected tools. Whatever the remote MCP server exposes is what the LLM gets. This means:

- Cluster admins control the tool surface via their MCP server configuration
- New tools in future OCP MCP server versions are picked up automatically
- Heterogeneous fleets work: OCP 4.14 and 4.17 clusters expose different tools
- Non-OCP clusters work if they run an MCP-compliant server

KA's only contract is the MCP protocol: connect, discover, bridge, execute.

### 5. Two Deployment Topologies (User's Choice)

Users choose between direct connections and the MCP Gateway based on their fleet size:

#### Direct Connections (Small Fleet, ≤10 Clusters)

KA connects directly to each cluster's MCP server. Each server is listed in KA's configuration.

```
KA ──▶ OCP MCP Server (cluster-a) ──▶ Cluster A K8s API
KA ──▶ OCP MCP Server (cluster-b) ──▶ Cluster B K8s API
KA ──▶ OCP MCP Server (cluster-c) ──▶ Cluster C K8s API
```

KA manages N connections, applies prefixes via `BridgeTool`, and handles per-server health checks.

#### MCP Gateway (Large Fleet, 10+ Clusters)

KA connects to a single MCP Gateway endpoint. The gateway (Envoy AI Gateway) aggregates per-cluster MCP servers, applies `{backendName}__{toolName}` prefixes, and handles connection management.

```
KA ──▶ MCP Gateway ──▶ OCP MCP Server (cluster-a) ──▶ Cluster A K8s API
                   ──▶ OCP MCP Server (cluster-b) ──▶ Cluster B K8s API
                   ──▶ OCP MCP Server (cluster-c) ──▶ Cluster C K8s API
                   ──▶ ... (x1000)
```

KA maintains 1 connection. The gateway handles auth, rate limiting, observability, and tool federation.

**KA's MCP client code is identical in both topologies.** The difference is an infrastructure deployment decision, not a code change.

### 6. Gateway Tool Discovery via Meta-Tools

When using the MCP Gateway (v0.7.0+), KA uses the gateway's built-in meta-tools to discover and scope tools **programmatically** before building the LLM prompt:

| Meta-Tool | Purpose | Called By |
|-----------|---------|-----------|
| `discover_tools` | Lightweight catalog: server names, categories, hints, tool names | KA Go code |
| `filter_tools_by_tags` | Filter tools by tags on `Backend` CRD (e.g., `cluster:prod-east`) | KA Go code |
| `select_tools` | Scope the MCP session to a chosen subset of tools | KA Go code |

**Per-investigation flow with gateway:**

```go
// 1. Filter tools to the target cluster (from signal's ClusterName)
filter_tools_by_tags({tags: ["cluster:prod-east"]})
// → ["prod_east_pods_list", "prod_east_events_list", ...]

// 2. Scope session to those tools
select_tools({tools: [...]})

// 3. ListTools now returns only the scoped tools with full schemas
tools/list → 20 tools with JSON Schemas

// 4. Bridge, strip prefix, build prompt, invoke LLM
```

**No per-cluster configuration in KA.** The cluster-to-tag mapping lives on the gateway's `Backend` resources:

```yaml
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: Backend
metadata:
  name: prod-east
  labels:
    cluster: prod-east
spec:
  endpoints:
    - fqdn:
        hostname: k8s-mcp-server.prod-east.svc
        port: 8080
```

Adding or removing clusters is a gateway-side operation. KA is untouched.

The gateway's `--discovery-tool-threshold` flag ensures that at scale (1000 clusters x 20 tools = 20,000 tools), `tools/list` returns only the meta-tools. Agents must use `discover_tools` → `select_tools` to access real tools, preventing context window explosion.

### 7. Registration Mechanism Evolution

| Version | Registration | Discovery |
|---------|-------------|-----------|
| **v1.5** | `integrations.mcp.servers[]` (static config in KA ConfigMap) | Startup discovery, fail-fast |
| **v1.6** | `MCPServerBinding` CRD (watched by KA controller) | Dynamic: watch + reconcile on CRD changes |

The `MCPServerBinding` CRD (v1.6) enables dynamic server registration without KA restarts. Users define one CRD per MCP endpoint -- a direct MCP server, or a gateway:

```yaml
apiVersion: kubernaut.ai/v1alpha1
kind: MCPServerBinding
metadata:
  name: fleet-gateway
  namespace: kubernaut-system
spec:
  url: https://mcp-gateway.kubernaut-mcp.svc:8080/mcp
  transport: streamable-http
```

For direct connections, users create one CRD per cluster. For the gateway, one CRD pointing at the gateway endpoint. KA discovers tools from each registered endpoint.

---

## Consequences

### Positive

- KA investigates any cluster without kubeconfig access or `client-go` modifications
- Tool surface is cluster-defined, not KA-defined -- heterogeneous fleets supported
- Same MCP client code for direct and gateway topologies
- Programmatic discovery eliminates wasted LLM turns and reduces token usage
- Prefix stripping keeps prompts portable and tool names natural
- Gateway scales to 1000+ clusters with 1 KA connection and 0 per-cluster KA config
- OCP MCP server provides tools KA doesn't have today (alertmanager_alerts, nodes_log, routes_list)

### Negative

- Requires user to deploy OCP MCP servers on managed clusters (operational overhead)
- Gateway adds infrastructure (optional, only at scale)
- Session-per-call latency overhead (~10-50ms per tool call through gateway)
- MCP Gateway is Technology Preview (API may change)
- For direct connections, KA manages N connections with health checks

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| MCP Gateway TP instability | Medium | High | Pin version, monitor upstream, direct connections as fallback |
| OCP MCP server unavailable on target cluster | Medium | Medium | Fail investigation with clear error, don't fall back to wrong cluster |
| Tool schema changes across OCP versions | Low | Low | KA bridges schemas as-is, LLM adapts to different parameter sets |
| Gateway meta-tool API changes | Low | Medium | Abstraction layer in KA's discovery code, pin gateway version |

---

## Alternatives Considered

### A: Multi-kubeconfig in KA

KA directly manages kubeconfigs for all managed clusters.

**Rejected**: No auth/rate-limiting infrastructure, no observability, violates separation of concerns, kubeconfig rotation nightmare at scale.

### B: Hardcoded Tool Definitions in KA

KA defines the expected tool set and maps them to remote MCP calls.

**Rejected**: Breaks heterogeneous fleet support, requires KA release for every new tool, couples KA to OCP MCP server versions.

### C: LLM-Driven Discovery

LLM uses discovery meta-tools as part of the investigation prompt.

**Rejected**: Wastes LLM turns on deterministic work (tag matching), costs more tokens, non-deterministic tool selection.

### D: ClusterResolver ConfigMap in KA

KA maintains a ConfigMap mapping cluster names to MCP prefixes.

**Rejected for gateway topology**: Requires per-cluster config in KA, negating gateway's "zero KA config" benefit. The gateway's labels on `Backend` CRDs serve as the mapping.

**Acceptable for direct topology**: When not using a gateway, the `MCPServerBinding` CRD (v1.6) carries the prefix alongside the connection URL.

---

## Validation

| Evidence | Status | Finding |
|----------|--------|---------|
| Spike 3: KA MCP Client | Complete | `StreamableProvider` + `BridgeTool` working, 14 tests passing |
| Spike 4: Cluster Routing | Complete | Per-cluster prefix with tool filtering is clean |
| AF `SDKMCPClient` | Production | Same MCP SDK, same session-per-call pattern, proven in production |
| MCP Gateway v0.7.0 | Released | `discover_tools`, `select_tools`, `filter_tools_by_tags` meta-tools available |

---

## Supersedes

This ADR supersedes portions of **ADR-064**:
- ADR-064 deferred the MCP Gateway to v1.6+. This ADR retains that timeline but adds the programmatic discovery approach and the `MCPServerBinding` CRD evolution path.
- ADR-064's `ClusterResolver` ConfigMap is replaced by gateway `Backend` CRD labels for the gateway topology and by the `MCPServerBinding` CRD for the direct topology.
- ADR-064's Alternative C (direct connections) is no longer rejected -- it is supported as a valid small-fleet topology alongside the gateway.

---

## References

- ADR-064: Multi-Cluster MCP Gateway
- ADR-065: Fleet Cluster Identity on RR
- Spike 3: KA MCP Client (`docs/spikes/multi-cluster-mcp-gateway/spike-3-ka-mcp-client.md`)
- Spike 4: Cluster Routing (`docs/spikes/multi-cluster-mcp-gateway/spike-4-cluster-routing.md`)
- Envoy AI Gateway MCP: https://aigateway.envoyproxy.io/docs/capabilities/mcp/
- OCP MCP Server: https://github.com/openshift/openshift-mcp-server
- Envoy AI Gateway: https://github.com/envoyproxy/ai-gateway

---

**Document Version**: 1.0
**Last Updated**: 2026-06-15
