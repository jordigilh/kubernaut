# Spike S6: Fleet Metadata Cache (FMC) Validation

## Objective

Validate the technical feasibility and performance characteristics of a Fleet Metadata Cache
backed by the existing Valkey instance. This cache enables sub-5ms scope checks for remote
clusters in the Gateway (GW) and Remediation Orchestrator (RO) services.

## Key Questions

1. **MCP Subscriptions**: Does the K8s MCP Server support `subscriptions/listen` for
   real-time label change notifications? If not, what polling interval is acceptable?
2. **Valkey Latency**: Can we achieve <5ms p95 for `EXISTS kubernaut:managed:{clusterID}:{gvr}:{ns}/{name}`?
3. **Sync Volume**: For a typical fleet (10 clusters, ~1000 managed resources each),
   what is the estimated Valkey memory footprint and sync bandwidth?

## Findings

### 1. MCP Subscriptions Support

The MCP SDK `v1.6.1` includes `subscriptions/subscribe` and `subscriptions/unsubscribe`
as part of the protocol spec (2025-03-26 draft). However, the K8s MCP Server implementation
(`github.com/strowk/mcp-k8s-go`) does NOT yet implement subscriptions.

**Decision**: Use polling-based sync with configurable interval (default: 30s).
The FMC service will call `list_resources` for each cluster at the configured interval
and sync ObjectMeta (labels) to Valkey with a TTL slightly longer than the interval (45s).

### 2. Valkey Latency Benchmark

Using the existing Valkey instance (valkey:8-alpine), benchmark results for key operations:

| Operation | p50 | p95 | p99 |
|----|----|----|----|
| EXISTS (single key) | 0.08ms | 0.15ms | 0.3ms |
| GET (single key, 256 bytes) | 0.10ms | 0.18ms | 0.4ms |
| HGET (hash field) | 0.09ms | 0.16ms | 0.35ms |
| Pipeline (10 ops) | 0.5ms | 0.9ms | 1.5ms |

**Conclusion**: <5ms p95 SLA is easily achievable even with batch lookups.

### 3. Sync Volume Estimation

Assumptions:
- 10 managed clusters
- ~1000 resources per cluster with `kubernaut.ai/managed=true` label
- Key format: `kubernaut:managed:{clusterID}:{gvr}:{ns}/{name}` (~120 bytes average)
- Value: JSON ObjectMeta labels subset (~200 bytes average)

| Metric | Value |
|----|----|
| Total keys | ~10,000 |
| Memory per key (with overhead) | ~400 bytes |
| Total Valkey memory | ~4 MB |
| Sync bandwidth per interval (30s) | ~2 MB per full sync |
| TTL strategy | 45s (1.5x sync interval) |

**Conclusion**: Negligible impact on existing Valkey instance (currently used for DLQ at ~50MB).

## Architecture

```
┌──────────┐     tools/list      ┌─────────────┐     SET/TTL     ┌────────┐
│ K8s MCP  │ ◄──────────────────  │     FMC     │ ──────────────► │ Valkey │
│ Servers  │     (polling 30s)   │   Service   │                 │        │
└──────────┘                     └─────────────┘                 └────────┘
                                                                      ▲
                                                         EXISTS/GET   │
                                                                      │
                                                   ┌──────────────────┘
                                                   │
                                         ┌─────────┴──────────┐
                                         │  GW / RO services  │
                                         │  (scope checking)   │
                                         └────────────────────┘
```

## Key Schema

```
kubernaut:managed:{clusterID}:{group}/{version}/{kind}:{namespace}/{name}
```

Example:
```
kubernaut:managed:prod-east:apps/v1/Deployment:default/nginx
```

Value: `{"managed":true,"labels":{"team":"platform"}}`

## Fallback Strategy

If Valkey returns a miss (key not found or expired):
1. Direct MCP call to the cluster's K8s MCP Server: `get_resource` with label check
2. On success, populate Valkey key with standard TTL (backfill)
3. If MCP call also fails, reject with "scope check unavailable" (fail-closed)

## Spike Validation Code

See `valkey_bench_test.go` in this directory for the Valkey latency benchmark.

## Recommendation

**Proceed with FMC implementation** in Phase 6B using:
- Polling sync (30s interval)
- Valkey with 45s TTL
- Fallback to direct MCP on miss
- Shared `pkg/fleet/scopecache/` client library for GW/RO
