# ADR-064: Multi-Cluster Investigation via OCP MCP Server

**Status**: Deferred to v1.6+ (v1.5 MVP uses Thanos -- see DD-INT-020 v1.5 Part E)
**Date**: 2026-06-04
**Updated**: 2026-06-13
**Deciders**: Architecture Team
**Context**: ServiceNow signal investigation requires multi-cluster K8s access

## v1.5 Decision

For v1.5 MVP, multi-cluster resource status is achieved via **Thanos Querier** (existing Prometheus tools, zero new code). The direct KA→OCP MCP architecture described below is validated (spike complete, prototype working) and deferred to v1.6+ for full K8s API access (events, logs, resource specs).

## Context

ServiceNow tickets reference resources (nodes, clusters) across multiple workload clusters. Kubernaut's Knowledge Agent (KA) runs in a management cluster and currently only accesses the local cluster via `client-go`. For ServiceNow signals (`TargetType="servicenow"`), KA must investigate resources in the workload cluster where the CMDB CI lives.

Single-cluster investigation is insufficient for a real ServiceNow use case.

## Decision

KA connects **directly** to per-cluster OCP MCP servers, authenticating with a short-lived JWT per cluster. No MCP Gateway.

```
Management Cluster
└── KA ──JWT──> OCP MCP Server (Workload Cluster A, read-only)
    ──JWT──> OCP MCP Server (Workload Cluster B, read-only)
```

1. **Per-cluster OCP MCP servers**: Deploy `openshift/openshift-mcp-server` on each workload cluster in read-only mode (`--read-only --stateless`) with investigation-scoped RBAC.

2. **KA as direct MCP client**: KA connects to each OCP MCP server via `StreamableClientTransport`. A cluster registry (ConfigMap or Secret) maps cluster names to MCP server endpoints + auth credentials.

3. **Per-cluster JWT authentication**: KA mints or obtains a short-lived token per cluster. Options (in order of preference):
   - **Projected SA token with audience**: `TokenRequest` API targeting the OCP MCP server's expected audience per cluster
   - **ACM managed cluster credentials**: Leverage existing hub-spoke auth (kubeconfig secrets in management cluster)
   - **OIDC federation**: Management cluster's SA token issuer trusted by workload clusters

4. **Cluster routing**: CMDB CI name → cluster endpoint lookup from the cluster registry. KA connects to exactly one MCP server per investigation -- no tool aggregation needed.

5. **Dual tool path**: K8s-originated signals continue using local `client-go` tools. ServiceNow signals use MCP Bridge tools for remote cluster access.

## Why Not MCP Gateway

The MCP Gateway (Kuadrant / Connectivity Link) was evaluated in the spike (4 spikes, all GO). It was **removed** from the architecture because:

| Gateway Capability | Does KA need it? | Reason |
|-------------------|-----------------|--------|
| Tool prefix routing (aggregate N servers) | **No** | KA knows the target cluster from `ProviderData.cmdb_ci` -- connects to exactly one server per investigation |
| Centralized auth | **No** | KA authenticates per-connection with JWT |
| Rate limiting | **No** | KA self-limits; investigation tool calls are sequential |
| Observability | **No** | KA instruments its own MCP calls |

**Net effect of removing the gateway:**
- Eliminates Istio + Gateway API + MCP Gateway operator dependency
- Eliminates Technology Preview risk (MCP Gateway is not GA)
- ~10-50ms lower latency per tool call (no gateway hop)
- Simpler deployment: OCP MCP server per cluster + cluster registry Secret

## Consequences

### Positive

- Multi-cluster investigation without modifying KA's existing K8s tool surface
- Security: per-cluster ServiceAccounts with least-privilege RBAC
- Failure isolation: one cluster's MCP server failing doesn't affect others
- No Technology Preview dependencies
- Lower latency than gateway architecture
- Same MCP SDK (`modelcontextprotocol/go-sdk v1.6.1`) already in use
- OCP MCP server provides bonus tools (nodes_log, alertmanager_alerts) KA doesn't have
- `StreamableProvider` prototype already supports direct connections

### Negative

- KA manages N MCP client connections (one per cluster) instead of 1
- Cluster registry (endpoint + credential per cluster) must be maintained
- No Envoy-level observability on MCP traffic (KA instruments its own calls)

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Network connectivity to workload clusters | Medium | High | Leverage ACM hub-spoke connectivity; OCP MCP server exposed via Route |
| Credential rotation across N clusters | Low | Medium | Short-lived projected SA tokens; no long-lived secrets |
| Cross-cluster latency | Low | Low | Investigation tools are sequential, not latency-critical |

## Alternatives Considered

### A: MCP Gateway (Kuadrant) -- Rejected

**Approach**: Deploy MCP Gateway in management cluster, register per-cluster OCP MCP servers with tool prefixes.

**Rejected because**: Gateway adds infrastructure complexity (Istio + Gateway API + operator) without value -- KA already knows the target cluster and doesn't need tool aggregation. Technology Preview risk. Spike validated it works but the simpler direct approach is preferred.

### B: Multi-kubeconfig in KA -- Rejected

KA directly manages kubeconfigs for all workload clusters via `client-go`.

**Rejected because**: Doesn't leverage OCP MCP server's toolset. Requires KA to reimplement tool logic for remote clusters.

### C: Single multi-context OCP MCP server -- Rejected

One OCP MCP server with kubeconfig containing all cluster contexts.

**Rejected because**: Single point of failure, violates least-privilege (one SA for all clusters), kubeconfig rotation requires restart.

## Validation

Spike evidence in `docs/spikes/multi-cluster-mcp-gateway/`:

| Spike | Status | Key Finding |
|-------|--------|-------------|
| Spike 1: Tool Coverage | GO | OCP MCP server covers 82% of KA's investigation tools |
| Spike 2: Gateway Deployment | GO (architecture validated, gateway removed from design) | Manifests documented but gateway deemed unnecessary |
| Spike 3: KA MCP Client | GO | `StreamableProvider` + `BridgeTool` working (14 tests). Works for direct connections -- no gateway needed. |
| Spike 4: Cluster Routing | GO | Direct cluster endpoint lookup replaces prefix routing |

## References

- DD-INT-020 v1.5: ServiceNow Signal Target Type (Part E)
- ADR-063: ServiceNow Signal Integration Architecture
- `openshift/openshift-mcp-server`: https://github.com/openshift/openshift-mcp-server
