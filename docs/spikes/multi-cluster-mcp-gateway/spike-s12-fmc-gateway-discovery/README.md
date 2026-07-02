# Spike S12 — FMC Gateway-Based Cluster Discovery

## Objective

Validate that FMC can discover registered backends by calling `tools/list` on
the MCP Gateway and parsing cluster IDs from the `{clusterID}__tool_name`
prefix convention. Assess whether this can supplement the CRDWatcher as a
consistency/health check, while the CRDWatcher remains the authoritative
source for which clusters Kubernaut manages (via `kubernaut.ai/managed=true`).

## Background

Today FMC discovers remote clusters in two steps:

1. **CRDWatcher** watches `MCPServerRegistration` CRDs labeled
   `kubernaut.ai/managed=true` on the hub K8s API. Each CRD yields a
   `ClusterInfo{ID, Name, MCPEndpoint}`.
2. **Syncer** iterates over those clusters and for each one calls
   `{clusterID}__list_resources` through the MCP Gateway to populate the
   Valkey scope cache.

The CRDWatcher approach requires:
- A K8s client with RBAC to read `MCPServerRegistrations`
- The CRDs to exist on the hub cluster before the MCP Gateway registers
  the backend

In the target architecture (ADR-068), the MCP Gateway is the single source
of truth for which backends are registered. The Gateway's `tools/list` response
already encodes cluster membership — every tool from cluster `prod-east` is
prefixed as `prod_east__get_resource`, `prod_east__list_resources`, etc.

If FMC can parse cluster IDs from `tools/list`, it gains a supplementary
signal for operational health:
- Detect drift between CRD-declared clusters and Gateway-registered backends
- Confirm backend reachability before attempting sync
- Surface configuration errors (CRD exists but backend missing, or vice versa)

The CRDWatcher remains essential because the `MCPServerRegistration` CRD with
`kubernaut.ai/managed=true` gives operators a cluster-level kill switch —
removing the label instantly stops Kubernaut from managing a cluster without
needing to update labels on individual namespaces or workloads.

## Key Questions

| # | Question | Status |
|---|----------|--------|
| Q1 | Does `tools/list` on the Gateway return all cluster-prefixed tools? | |
| Q2 | Can we reliably parse unique cluster IDs from `{clusterID}__` prefixes? | |
| Q3 | What is the `tools/list` payload size at 10 / 50 clusters? | |
| Q4 | Can this feed a `ClusterRegistry` implementation end-to-end? | |
| Q5 | Should Gateway discovery replace or supplement CRDWatcher? | |

## Test Scenarios

### S12-001: tools/list returns all cluster-prefixed tools

Connect to a mock MCP Gateway with 3 registered clusters, each exposing
`get_resource` and `list_resources`. Call `tools/list` via MCP SDK and assert
all 6 tools are present with correct prefix format.

### S12-002: Parse unique cluster IDs from tool names

From the `tools/list` response, extract unique cluster IDs by splitting on
`__`. Assert the resulting set matches the registered clusters exactly.

### S12-003: Scale — tools/list at 50 clusters

Register 50 clusters on the mock gateway (2 tools each = 100 tools). Measure:
- Response parse time
- Unique cluster extraction time
- Memory allocation per response

### S12-004: Full pipeline — tools/list → cluster set → list_resources → scope keys

End-to-end: call `tools/list`, derive clusters, then for each cluster call
`{clusterID}__list_resources` with `labelSelector=kubernaut.ai/managed=true`,
parse the response, build Valkey scope keys. Assert keys match expected format.

### S12-005: Polling-based refresh — detect new and removed clusters

Start with 2 clusters. Add a 3rd cluster (register new tools on gateway).
Re-call `tools/list` and assert the diff. Then remove 1 cluster and assert
the diff reflects the removal.

## Architecture: Gateway Discovery vs CRDWatcher

```
Current (CRDWatcher):
┌──────────┐  Watch MCPServerRegistration  ┌─────┐  tools/call   ┌─────────────┐
│ Hub K8s  │ ──────────────────────────────►│ FMC │ ────────────► │ MCP Gateway │
│ API      │  (informer + label filter)     │     │  per cluster  │             │
└──────────┘                                └─────┘               └─────────────┘

Proposed (Gateway Discovery):
                                            ┌─────┐  tools/list   ┌─────────────┐
                                            │ FMC │ ────────────► │ MCP Gateway │
                                            │     │ ◄──────────── │             │
                                            │     │  tool list    │             │
                                            │     │               │             │
                                            │     │  tools/call   │             │
                                            │     │ ────────────► │             │
                                            └─────┘  per cluster  └─────────────┘
```

With Gateway Discovery, FMC needs only:
- MCP SDK client to the Gateway
- Periodic `tools/list` polling (same interval as sync: 30s)
- Prefix parser to extract cluster IDs

No K8s API dependency. No CRD RBAC. No informer.

## Recommendation Criteria

Add Gateway Discovery as supplementary health check if ALL pass:
- S12-001 PASS: tools/list returns prefixed tools
- S12-002 PASS: cluster ID parsing is reliable
- S12-003 PASS: <10ms parse time at 50 clusters
- S12-004 PASS: end-to-end pipeline works
- S12-005 PASS: add/remove detection works

CRDWatcher remains the authoritative `ClusterRegistry` regardless of results.
The `MCPServerRegistration` CRD provides the operational control plane for
cluster-level opt-in/opt-out.

## Findings

### PASS — All 5 test scenarios validated

| Test | Description | Result |
|------|-------------|--------|
| S12-001 | tools/list returns all cluster-prefixed tools | PASS |
| S12-002 | Parse unique cluster IDs from tool names | PASS |
| S12-003 | Scale — 50 clusters (100 tools) parse performance | PASS |
| S12-004 | Full pipeline: tools/list → clusters → list_resources → scope keys | PASS |
| S12-005 | Polling detects added/removed clusters | PASS |

### Key Metrics (50 clusters, 100 tools)

| Metric | Value |
|--------|-------|
| `tools/list` RPC latency | ~600µs |
| Cluster ID parse time | ~10µs |
| Memory allocation per parse | ~4 KB |
| Payload size (JSON) | ~23 KB |
| Total test time (5 scenarios) | 0.015 seconds |

### Answers to Key Questions

**Q1: Does `tools/list` return all cluster-prefixed tools?**
YES. The MCP Gateway's `tools/list` returns every registered tool from every
backend. Each tool follows the `{clusterID}__tool_name` convention. With 3
clusters × 2 tools each, all 6 tools are returned in a single `tools/list` call.

**Q2: Can we reliably parse cluster IDs from prefixes?**
YES. Splitting on `__` and collecting unique left-hand values produces the
exact set of registered clusters. The parser correctly handles underscores
in cluster names (e.g., `prod_east`).

**Q3: What is the payload size at scale?**
~23 KB for 50 clusters (100 tools). Extrapolating: ~46 KB for 100 clusters.
This is negligible for a 30-second polling interval. Parse time is ~10µs —
three orders of magnitude under the 10ms budget.

**Q4: Does the full pipeline work end-to-end?**
YES. The sequence `tools/list` → `parseClusterIDs` → `{cluster}__list_resources`
→ `scopecache.BuildKey` produces correctly formatted Valkey keys like
`kubernaut:managed:hub_east:/v1/Deployment:default/item-1`.

**Q5: Can polling detect add/remove?**
YES. Diffing cluster sets between consecutive `tools/list` calls correctly
identifies newly added clusters (set difference: new − old) and removed
clusters (set difference: old − new).

### Critical Implementation Details

- **Prefix convention**: `{clusterID}__tool_name` (double underscore)
- **Parser**: `strings.Index(name, "__")` — O(n) per tool, O(total_tools) per poll
- **No K8s API needed**: Cluster discovery is pure MCP — no informer, no CRD RBAC
- **Reconnection**: Each poll creates a new `tools/list` call on the existing
  session; if the session drops, the MCP resilient client reconnects automatically

### Recommendation

**Keep CRDWatcher as primary; use Gateway Discovery as supplementary health check.**

The CRDWatcher provides an essential operational control plane that Gateway
Discovery cannot replace:

- **Cluster-level kill switch**: Operators can remove the
  `kubernaut.ai/managed=true` label from a single `MCPServerRegistration` CRD
  to instantly pause Kubernaut management of an entire cluster — without
  touching any workload or namespace labels on that cluster.
- **Separation of concerns**: The MCP Gateway tracks which backends are
  *reachable*; the CRD tracks which clusters are *managed by Kubernaut*.
  A cluster can be registered on the gateway (serving other consumers) but
  not managed by Kubernaut, and vice versa.
- **Granular opt-in/opt-out**: CRD labels and annotations can carry additional
  policy metadata (maintenance windows, tier, environment) that `tools/list`
  cannot express.

Gateway Discovery (`tools/list` → cluster ID parsing) is still valuable as a
**consistency check**: FMC can compare the CRDWatcher's cluster set against the
Gateway's tool set and surface drift (e.g., a CRD exists but the Gateway has
no matching backend, or a backend is registered but no CRD exists).

**Implementation path**:

1. **CRDWatcher** remains the authoritative `ClusterRegistry` for FMC Syncer.
   Only clusters with `kubernaut.ai/managed=true` on their CRD are synced.
2. **GatewayHealthChecker** (new, optional): Periodically calls `tools/list`,
   parses cluster IDs, and compares against the CRDWatcher set. Emits metrics
   and log warnings for:
   - `gateway_cluster_not_in_registry` — backend registered but no CRD
   - `registry_cluster_not_in_gateway` — CRD exists but no gateway backend
3. No changes to the Syncer loop or scope cache pipeline.
