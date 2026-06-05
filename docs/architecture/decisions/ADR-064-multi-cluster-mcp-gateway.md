# ADR-064: Multi-Cluster Investigation via MCP Gateway

**Status**: Deferred to v1.6+ (v1.5 MVP uses Thanos -- see DD-INT-020 v1.4 Part E)
**Date**: 2026-06-04
**Updated**: 2026-06-05
**Deciders**: Architecture Team
**Context**: ServiceNow signal investigation requires multi-cluster K8s access

## v1.5 Decision

For v1.5 MVP, multi-cluster resource status is achieved via **Thanos Querier** (existing Prometheus tools, zero new code). This provides metrics-based health assessment (up/down, CPU, memory, active alerts) across all clusters that feed into Thanos. The MCP Gateway architecture described below is validated (spike complete, prototype working) and deferred to v1.6+ for full K8s API access (events, logs, resource specs).

## Context

ServiceNow tickets reference resources (nodes, clusters) across multiple workload clusters. Kubernaut's Knowledge Agent (KA) runs in a management cluster and currently only accesses the local cluster via `client-go`. For ServiceNow signals (`TargetType="servicenow"`), KA must investigate resources in the workload cluster where the CMDB CI lives.

Single-cluster investigation is insufficient for a real ServiceNow use case.

## Decision

Adopt **Red Hat's MCP Gateway** (Kuadrant / Connectivity Link) as the multi-cluster investigation architecture:

1. **Per-cluster OCP MCP servers**: Deploy `openshift/openshift-mcp-server` on each workload cluster in read-only mode with investigation-scoped RBAC.

2. **MCP Gateway**: Deploy in the management cluster to aggregate per-cluster MCP servers behind a single endpoint. Each cluster is registered with a unique tool prefix (e.g., `cluster_a_`).

3. **KA as MCP client**: KA connects to the MCP Gateway via `StreamableClientTransport`, discovers tools at startup, and wraps them as KA `tools.Tool` via the `BridgeTool` adapter in `pkg/kubernautagent/tools/mcp/`.

4. **Cluster routing**: CMDB CI -> cluster prefix mapping via `ClusterResolver` (ConfigMap-backed lookup table). KA filters tools by prefix for each ServiceNow investigation.

5. **Dual tool path**: K8s-originated signals continue using local `client-go` tools. ServiceNow signals use MCP Bridge tools for remote cluster access.

## Consequences

### Positive

- Multi-cluster investigation without modifying KA's existing K8s tool surface
- Security: per-cluster ServiceAccounts with least-privilege RBAC
- Failure isolation: one cluster's MCP server failing doesn't affect others
- Scale: adding clusters is declarative (deploy MCP server + register)
- Same MCP SDK (`modelcontextprotocol/go-sdk v1.6.1`) already in use
- OCP MCP server provides bonus tools (nodes_log, alertmanager_alerts) KA doesn't have

### Negative

- Additional infrastructure: MCP Gateway + per-cluster MCP server pods
- Tool name explosion with prefixes (~20 tools per cluster visible to LLM)
- Session-per-call latency overhead (~10-50ms per tool call through gateway)
- Technology Preview: MCP Gateway is not GA (API may change)

### Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| MCP Gateway TP instability | Medium | High | Pin version, monitor upstream, fallback to direct MCP client |
| Tool prefix pollution | Low | Medium | LLM prompt restricts to target cluster prefix |
| Cross-cluster network issues | Medium | Medium | Health checks in MCPServerRegistration, graceful degradation |

## Alternatives Considered

### A: Multi-kubeconfig in KA -- Rejected

KA directly manages kubeconfigs for all workload clusters.

**Rejected because**: No auth/rate-limiting infrastructure, no observability, violates separation of concerns. KA becomes a cluster access point.

### B: Single multi-context OCP MCP server -- Rejected

One OCP MCP server with kubeconfig containing all cluster contexts.

**Rejected because**: Single point of failure, violates least-privilege (one SA for all clusters), kubeconfig rotation requires restart.

### C: KA connects directly to per-cluster MCP servers -- Rejected

No gateway; KA manages individual MCP client connections.

**Rejected because**: Loses gateway benefits (auth, rate limiting, aggregation, observability). More complex session management.

## Validation

Spike evidence in `docs/spikes/multi-cluster-mcp-gateway/`:

| Spike | Status | Key Finding |
|-------|--------|-------------|
| Spike 1: Tool Coverage | GO | OCP MCP server covers 82% of KA's investigation tools |
| Spike 2: Gateway Deployment | GO | Manifests ready; Helm + OLM paths documented |
| Spike 3: KA MCP Client | GO | StreamableProvider + BridgeTool working (14 tests passing) |
| Spike 4: Cluster Routing | GO | Per-cluster prefix with ClusterResolver is clean and scalable |

## References

- DD-INT-020 v1.3: ServiceNow Signal Target Type (Part E)
- ADR-063: ServiceNow Signal Integration Architecture
- `openshift/openshift-mcp-server`: https://github.com/openshift/openshift-mcp-server
- Kuadrant MCP Gateway: https://github.com/Kuadrant/mcp-gateway
- Red Hat Connectivity Link MCP Gateway docs: https://docs.redhat.com/en/documentation/red_hat_connectivity_link/1.3/html/installing_the_mcp_gateway
