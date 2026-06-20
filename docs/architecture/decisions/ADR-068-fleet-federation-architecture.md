# ADR-068: Fleet Federation Architecture

**Status**: Implemented (MVP)
**Date**: 2026-06-19
**Deciders**: Architecture Team
**Context**: Multi-cluster federation requires coordinated architecture across GW, KA, RO, and a new FMC Writer service (#54)
**Related**: ADR-064 (MCP Gateway - deferred), ADR-065 (ClusterID on RR), ADR-067 (KA MCP Dynamic Tool Discovery)

## Context

Kubernaut's single-cluster architecture cannot federate remediation across multiple managed clusters. Issue #54 requires:

1. **Signal ingestion** from remote clusters (via Thanos multi-cluster Prometheus)
2. **Scope gating** — verifying remote resources are managed before creating RRs
3. **Investigation** — KA must discover and use remote cluster tools for RCA
4. **Deduplication** — same resource on different clusters must not be deduplicated

The key constraints are:
- p95 < 50ms latency for scope checks (GW critical path)
- Zero regression for existing single-cluster deployments
- OAuth2 authentication for all MCP Gateway connections
- Credential rotation without service restart

## Decision

### Architecture Overview

```
                    ┌──────────────────────────────────────────────────┐
                    │                Management Cluster                 │
                    │                                                   │
┌────────┐         │  ┌────┐   ┌────┐   ┌────┐   ┌────────────────┐  │
│ Thanos │─alerts──│─>│ GW │   │ KA │   │ RO │   │  FMC Writer    │  │
│Querier │         │  └─┬──┘   └─┬──┘   └────┘   └───────┬────────┘  │
└────────┘         │    │        │                        │           │
                    │    │scope   │tools/list              │sync       │
                    │    ▼        ▼                        ▼           │
                    │  ┌─────────────────────────────────────┐        │
                    │  │         MCP Gateway (Kuadrant)       │        │
                    │  │   (routes to per-cluster MCP Servers)│        │
                    │  └─────────────────────────────────────┘        │
                    │                    │                             │
                    │    ┌───────────────┼───────────────┐            │
                    │    ▼               ▼               ▼            │
                    │  ┌─────┐       ┌─────┐       ┌─────┐          │
                    │  │K8s  │       │K8s  │       │K8s  │          │
                    │  │MCP  │       │MCP  │       │MCP  │          │
                    │  │Srv A│       │Srv B│       │Srv C│          │
                    │  └─────┘       └─────┘       └─────┘          │
                    │    │               │               │            │
                    └────┼───────────────┼───────────────┼────────────┘
                         ▼               ▼               ▼
                    ┌─────────┐    ┌─────────┐    ┌─────────┐
                    │Cluster A│    │Cluster B│    │Cluster C│
                    └─────────┘    └─────────┘    └─────────┘

                    ┌──────────────────────┐
                    │  Valkey (Fleet       │
                    │  Metadata Cache)     │◄── FMC Writer polls & writes
                    └──────────────────────┘
                              ▲
                              │ scope check (p95<50ms)
                    ┌─────────┴─────────┐
                    │GW FederatedScope   │
                    │Checker             │
                    └───────────────────┘
```

### Component Responsibilities

| Component | Role | Package |
|-----------|------|---------|
| **Gateway (GW)** | Extracts `cluster` label from Thanos alerts, computes cluster-aware fingerprints, gates signals via FederatedScopeChecker | `pkg/gateway/` |
| **FMC Writer** | Polls MCP Gateway for `kubernaut.ai/managed=true` resources, writes keys to Valkey with TTL | `cmd/fmcwriter/`, `pkg/fleet/fmcwriter/` |
| **Fleet Metadata Cache (Valkey)** | Low-latency key-existence checks for remote scope validation | `pkg/fleet/scopecache/` |
| **FederatedScopeChecker** | Routes scope checks: local → K8s API, remote → Valkey cache | `pkg/fleet/scopecache/federated_checker.go` |
| **ResilientClient** | MCP client with backoff, reconnect, readiness gating | `pkg/fleet/mcpclient/resilience.go` |
| **ReloadableOAuth2Transport** | Hot-reloadable OAuth2 credentials via FileWatcher | `pkg/fleet/mcpclient/reloadable_auth.go` |
| **CRDWatcher** | Discovers clusters from `MCPServerRegistration` CRDs | `pkg/fleet/registry/` |
| **KA Fleet Tools** | Dynamic BridgeTool registration from MCP Gateway `tools/list` | `cmd/kubernautagent/main.go` |

### Key Design Decisions

1. **Valkey cache for scope checks** (not direct MCP calls): Achieves p95 < 50ms. MCP round-trips (50-200ms) are unacceptable on the GW hot path.

2. **FMC Writer as dedicated service** (not sidecar): Keeps GW stateless. Separation of concerns between signal processing and metadata sync.

3. **ReloadableOAuth2Transport** (not static secrets): Supports zero-downtime credential rotation without pod restart. Uses existing `hotreload.FileWatcher` pattern.

4. **ResilientClient with backoff** (not fail-fast): MCP Gateway may not be ready at boot. Exponential backoff + readiness gate prevents cascading failures.

5. **ClusterID on fingerprint** (not label): Fingerprints drive deduplication. Including ClusterID ensures same-resource-different-cluster alerts create separate RRs.

6. **KA dynamic tool discovery** (not hard-coded): Fleet tools are registered at startup from `tools/list`. `AppendFleetToolsToRCA` makes them visible to the LLM investigator without code changes per cluster.

7. **`MCPServerRegistration` as source of truth** (not ConfigMap): Kuadrant CRD is the authoritative registry of MCP backends. Kubernaut is a consumer, not an owner.

## Alternatives Considered

### A. Direct MCP calls for scope checking
- +: No cache service needed
- -: 50-200ms latency per check; violates GW SLA
- **Rejected**: Performance unacceptable

### B. Sidecar cache in GW pod
- +: Lower network hop
- -: GW becomes stateful; scaling complicates cache coherence
- **Rejected**: Violates stateless GW design

### C. Push-based cache updates (MCP subscriptions)
- +: Near-real-time updates
- -: MCP subscriptions not yet supported by K8s MCP Server
- **Deferred**: Will adopt when MCP spec supports subscriptions

### D. ConfigMap-based cluster registry
- +: Simple, no CRD dependency
- -: Drift risk; manual management; no reconciliation
- **Rejected**: CRDWatcher on MCPServerRegistration is authoritative

## Consequences

### Positive
- Fleet scope checking achieves p95 < 1ms (Valkey EXISTS)
- Zero regression for single-cluster (fleet disabled by default)
- Credential rotation requires no service restart
- New clusters auto-discovered via CRDWatcher
- LLM investigator automatically gains remote cluster tools

### Negative
- New service to deploy (FMC Writer)
- Valkey dependency (already exists for other Kubernaut features)
- Stale cache window (sync interval, default 30s)
- MCP subscriptions not yet supported (polling fallback)

### Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Valkey unavailable | FederatedScopeChecker returns `false` (fail-safe: rejects unmanaged) |
| MCP Gateway down | ResilientClient reconnects with backoff; readiness gate prevents traffic |
| Stale cache (resource label removed) | TTL-based expiry (45s); false positive window bounded |
| OAuth2 IdP unreachable | TokenTimeout (10s) prevents indefinite hangs |

## Implementation Status

| Phase | Description | Status |
|-------|-------------|--------|
| Phase 1 | GW cluster-aware signal ingestion | Complete |
| Phase 2 | FMC Writer service | Complete |
| Phase 3 | Helm config wiring | Complete |
| Phase 4 | Lightweight fleet E2E | Complete |
| Phase 5 | CRD schema (ClusterID on RR) | Complete |
| Phase 6 | MCP client resilience | Complete |
| Remediation | Security, wiring, validation hardening | Complete |

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|---------|
| FederatedScopeChecker | NewServerWithMetrics() | pkg/gateway/server.go | IT-GW-FLEET-010 |
| fleetScopeChecker dispatch | validateScope() | pkg/gateway/server.go:1518 | IT-GW-FLEET-010/011/012 |
| ResilientClient (KA) | registerFleetTools() | cmd/kubernautagent/main.go | IT-KA-FLEET-001 |
| ResilientClient (FMC) | main() | cmd/fmcwriter/main.go | UT-FMC-001 |
| ReloadableOAuth2Transport (KA) | registerFleetTools() | cmd/kubernautagent/main.go | UT-FLEET-RES-001 |
| ReloadableOAuth2Transport (FMC) | main() | cmd/fmcwriter/main.go | UT-FLEET-RES-001 |
| AppendFleetToolsToRCA | registerFleetTools() → main | cmd/kubernautagent/main.go:231 | IT-KA-FLEET-001 |
| CRDWatcher | main() | cmd/fmcwriter/main.go | UT-FMC-004 |
| BuildKey validation | BuildKey() | pkg/fleet/scopecache/client.go | UT-FLEET-SC-006/007/008 |
| fmcwriter securityContext | Helm template | charts/kubernaut/templates/fmcwriter/fmcwriter.yaml | helm template |

## Security Considerations

| Control | Implementation |
|---------|---------------|
| Authentication | OAuth2 client credentials grant (file-mounted JWT) |
| Credential rotation | ReloadableOAuth2Transport with FileWatcher |
| Token timeout | 10s context deadline on token refresh HTTP calls |
| Least privilege | fmcwriter: readOnlyRootFilesystem, drop ALL caps, runAsNonRoot |
| Network policy | MCP Gateway accessible only from kubernaut-system namespace |
| RBAC | fmcwriter: read-only on MCPServerRegistration CRDs |

## FedRAMP Implications

| Control | Impact |
|---------|--------|
| AU-3 (Audit content) | Cluster provenance recorded per-RR |
| SI-4 (Monitoring) | Cross-cluster correlation via ClusterID |
| SC-7 (Boundary protection) | MCP Gateway as chokepoint for all remote access |
| IA-5 (Authenticator management) | Hot-reloadable credentials, bounded token lifetime |
| SC-8 (Transmission confidentiality) | OAuth2 + TLS for all MCP connections |

## References

- Issue #54: Multi-cluster federation
- ADR-064: Multi-Cluster MCP Gateway (deferred direct-connect approach)
- ADR-065: Fleet Cluster Identity on RR
- ADR-067: KA MCP Client Dynamic Tool Discovery
- Spike S6: Fleet Metadata Cache validation
- Spike S8: Real K8s MCP Server with envtest
- Spike S9: ScopeChecker interface redesign
- Spike S10: K8s MCP Server CRD support
