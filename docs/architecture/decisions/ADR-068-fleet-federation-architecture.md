# ADR-068: Fleet Federation Architecture

**Status**: Implemented (MVP)
**Date**: 2026-06-19
**Deciders**: Architecture Team
**Context**: Multi-cluster federation requires coordinated architecture across GW, KA, RO, WE, and a new FMC Writer service (#54)
**Related**: ADR-064 (MCP Gateway - deferred), ADR-065 (ClusterID on RR), ADR-067 (KA MCP Dynamic Tool Discovery)

## Context

Kubernaut's single-cluster architecture cannot federate remediation across multiple managed clusters. Issue #54 requires:

1. **Signal ingestion** from remote clusters (via Thanos multi-cluster Prometheus)
2. **Scope gating** — verifying remote resources are managed before creating RRs
3. **Investigation** — KA must discover and use remote cluster tools for RCA
4. **Remediation execution** — WE must create Jobs, Tekton PipelineRuns, and Ansible workflows on remote clusters
5. **Deduplication** — same resource on different clusters must not be deduplicated

The key constraints are:
- p95 < 50ms latency for scope checks (GW critical path)
- Zero regression for existing single-cluster deployments
- OAuth2 authentication for all MCP Gateway connections
- Credential rotation without service restart

## Decision

### MCP Gateway Technology

**Supported**: Both Envoy AI Gateway and Kuadrant MCP Gateway are supported via
the MCP Gateway Adapter Pattern (`pkg/fleet/registry/`). The MCP Gateway is
**external infrastructure** — it must be deployed before Kubernaut, similar to
PostgreSQL, Redis, or Prometheus. Kubernaut's Helm chart does not install the
gateway; platform teams choose and deploy their preferred implementation.

| Criterion | Envoy AI Gateway | Kuadrant MCP Gateway |
|-----------|-----------------|---------------------|
| **Istio dependency** | None | Requires `istiod` (sidecar injection disabled) |
| **Components to deploy** | 1 (single Deployment or standalone binary) | 3+ (Kuadrant controller + broker + Istio control plane) |
| **Memory footprint** | ~122 MB | ~388 MB (Istio + Kuadrant + kube-mcp-server) |
| **Tool prefixing** | `{backendName}__{toolName}` | `{toolPrefix}{toolName}` (configurable via `MCPServerRegistration.spec.prefix`) |
| **Tool discovery** | All tools in `tools/list`; cluster prefixes parsed client-side | `discover_tools`/`select_tools` meta-tools with `--discovery-tool-threshold`; server-side session scoping |
| **Auth model** | Built-in OAuth + CEL authorization | Authorino (external) + OPA Rego policies |
| **Standalone mode** | `aigw run` CLI (IT testing without K8s) | Not available (requires K8s + Istio) |
| **kube-mcp-server flags** | `--list-output=yaml` required (Decision #12) | `--list-output=yaml` required (Decision #12) |
| **Maturity** | v1.0 GA (Bloomberg/Tetrate backing) | Technology Preview |
| **CRD model** | `MCPRoute` + `Backend` (Envoy Gateway standard) | `MCPServerRegistration` + `MCPGatewayExtension` |
| **E2E validation** | IT container test (`fleet_eaigw_test.go`) | E2E fleet lane (Kind + Istio + Kuadrant, spike-s14) |

**Design principle: business logic is gateway-agnostic.** Kubernaut services connect
to the MCP Gateway endpoint and discover clusters/tools via the `GatewayDiscoverer`
interface (decision #11). Tool names carry the gateway-specific prefix (e.g.,
`cluster-a__resources_get` for EAIGW, `cluster_a_resources_get` for Kuadrant).
Services use discovered tool names as-is — they never need to know which gateway
implementation is running.

KA pre-scopes the MCP session to the alert's target cluster and registers two
LLM-callable tools (`list_clusters`, `list_tools_for_cluster`) for optional
multi-cluster investigation. This keeps the LLM context lean (~20 tool schemas)
while enabling cross-cluster correlation when the LLM determines it is needed.
Services (SP, WE, FMC) use `GatewayDiscoverer.ToolsForCluster()` programmatically.

The only gateway-aware components are:
- The `GatewayDiscoverer` implementations (`pkg/fleet/mcpclient/discovery/`)
- The cluster registry in FMC (`pkg/fleet/registry/`)

This awareness is isolated by interfaces and does not leak into other services'
business logic.

### Architecture Overview

```
                    ┌───────────────────────────────────────────────────────┐
                    │                   Management Cluster                   │
                    │                                                        │
┌────────┐         │  ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌────┐ ┌─────┐ │
│ Thanos │─alerts──│─>│ GW │ │ KA │ │ RO │ │ SP │ │ AF │ │ EM │ │ FMC │ │
│Querier │         │  └─┬──┘ └─┬──┘ └────┘ └─┬──┘ └─┬──┘ └─┬──┘ └──┬──┘ │
└────────┘         │    │      │              │      │      │       │     │
                    │    │      │  all read    │      │      │       │     │
                    │    ▼      ▼              ▼      ▼      ▼       ▼     │
                    │  ┌──────────────────────────────────────────────┐    │
                    │  │        MCP Gateway (EAIGW or Kuadrant)        │    │
                    │  │      (routes to per-cluster MCP Servers)      │    │
                    │  │      Gateway-native authz                    │    │
                    │  │      mcp-read: GW,KA,RO,SP,AF,EM,FMC        │    │
                    │  │      mcp-write: WE (remediation)             │    │
                    │  └──────────────────────────────────────────────┘    │
                    │                    │                                 │
                    │    ┌───────────────┼───────────────┐                │
                    │    ▼               ▼               ▼                │
                    │  ┌─────┐       ┌─────┐       ┌─────┐              │
                    │  │K8s  │       │K8s  │       │AAP  │              │
                    │  │MCP  │       │MCP  │       │MCP  │              │
                    │  │Srv A│       │Srv B│       │Srv C│              │
                    │  └─────┘       └─────┘       └─────┘              │
                    │    │               │               │                │
                    └────┼───────────────┼───────────────┼────────────────┘
                         ▼               ▼               ▼
                    ┌─────────┐    ┌─────────┐    ┌─────────┐
                    │Cluster A│    │Cluster B│    │Cluster C│
                    │(K8s)    │    │(K8s)    │    │(K8s+AAP)│
                    └─────────┘    └─────────┘    └─────────┘

                    ┌────┐
                    │ WE │──tools/call (read+write)──► MCP Gateway
                    └────┘

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
| **WorkflowExecution (WE)** | Executes remediation on remote clusters via MCP Gateway: creates Jobs, Tekton PipelineRuns, and Ansible workflows. Requires read+write access. | `internal/controller/workflowexecution/`, `pkg/workflowexecution/executor/` |
| **FMC (Fleet Metadata Cache)** | Polls MCP Gateway for `kubernaut.ai/managed=true` resources, caches metadata in Valkey, exposes scope queries via REST API | `cmd/fmc/`, `pkg/fleet/fmc/` |
| **Fleet Metadata Cache (Valkey)** | Low-latency key-existence checks for remote scope validation | `pkg/fleet/scopecache/` |
| **FederatedScopeChecker** | Routes scope checks: local → scope.Manager, remote → backend adapter (scope.ScopeChecker) | `pkg/fleet/federated_checker.go` |
| **ResilientClient** | MCP client with backoff, reconnect, readiness gating | `pkg/fleet/mcpclient/resilience.go` |
| **ReloadableOAuth2Transport** | Hot-reloadable OAuth2 credentials via FileWatcher | `pkg/fleet/mcpclient/reloadable_auth.go` |
| **CRDWatcher** | Discovers clusters from gateway CRDs (`MCPRoute`/`Backend` for EAIGW, `MCPServerRegistration` for Kuadrant) via adapter pattern | `pkg/fleet/registry/` |
| **GatewayDiscoverer** | Two-phase cluster/tool discovery for LLM context efficiency. Gateway-specific implementations for Kuadrant (`discover_tools`/`select_tools`) and EAIGW (`tools/list` prefix scan). Exposed to LLM as `list_clusters` and `list_tools_for_cluster` tools. | `pkg/fleet/mcpclient/discovery/` |
| **KA Fleet Tools** | Dynamic BridgeTool registration from MCP Gateway. Pre-scopes to target cluster, registers `list_clusters` and `list_tools_for_cluster` as LLM-callable tools for multi-cluster investigation. | `cmd/kubernautagent/main.go` |

### Key Design Decisions

1. **Federated Control Plane interface** (adapter pattern): GW and RO depend on a `FederatedControlPlane` interface, not on any specific storage backend. The adapter is selected at startup based on the environment (FMC, ACM, Rancher, Clusterpedia). This decouples Kubernaut from any single fleet management platform and allows swapping backends without code changes in GW/RO.

2. **FMC as default adapter** (for environments without a federated control plane): FMC Writer polls MCP Gateway, writes to Valkey (co-owned with DataStorage), and exposes a scope query API. Achieves p95 < 1ms. Rancher/ACM/Clusterpedia shops skip FMC entirely and use their native APIs.

3. **FMC Writer as dedicated service** (not sidecar): Keeps GW stateless. Separation of concerns between signal processing and metadata sync.

4. **ReloadableOAuth2Transport** (not static secrets): Supports zero-downtime credential rotation without pod restart. Uses existing `hotreload.FileWatcher` pattern.

5. **ResilientClient with backoff** (not fail-fast): MCP Gateway may not be ready at boot. Exponential backoff + readiness gate prevents cascading failures.

6. **ClusterID on fingerprint** (not label): Fingerprints drive deduplication. Including ClusterID ensures same-resource-different-cluster alerts create separate RRs.

7. **KA dynamic tool discovery** (not hard-coded): Fleet tools are registered at startup from `tools/list`. `AppendFleetToolsToRCA` makes them visible to the LLM investigator without code changes per cluster.

8. **Gateway CRDs as source of truth** (not ConfigMap): The gateway's native CRDs (`MCPRoute`/`Backend` for EAIGW, `MCPServerRegistration` for Kuadrant) are the authoritative registry of MCP backends. Kubernaut is a **read-only consumer** — it watches but never creates or modifies these CRDs. The control plane (ACM, Rancher, GitOps) owns their lifecycle.

9. **MCP Gateway as unified chokepoint for all remote cluster access** (not separate auth paths per backend type): All Kubernaut services that interact with remote clusters — GW, KA, RO, SP, AF, EM (read-only), FMC (metadata sync), and WE (remediation execution) — access remote clusters exclusively through the MCP Gateway. The K8s MCP Server and AAP MCP Server are both registered as backends behind the same gateway. This eliminates the need for service-specific credential management (e.g., separate AAP bearer token injection). Auth is enforced at two layers: (a) the gateway validates caller credentials via its native auth model (OAuth + CEL for EAIGW, Authorino + OPA for Kuadrant), and (b) each MCP Server authenticates against its own local APIs using its own ServiceAccount. No per-cluster SA tokens are maintained by Kubernaut services.

10. **Gateway-agnostic business logic** (not per-gateway code paths): Services that need to call MCP tools on a remote cluster use the `GatewayDiscoverer` interface (decision #11) to discover clusters and tools. The only gateway-aware components are the `GatewayDiscoverer` implementations and the cluster registry in FMC (`pkg/fleet/registry/`). All other services — GW, SP, WE, KA, RO, AF, EM — are fully gateway-agnostic.

11. **GatewayDiscoverer: two-phase tool discovery for LLM context efficiency** (not raw `tools/list` scan): At fleet scale (100+ clusters, 1800+ tools), presenting all tools to the LLM wastes context tokens and causes hallucination. The `GatewayDiscoverer` interface provides two operations — `ListClusters()` and `ToolsForCluster(clusterID)` — that map to two KA-registered tools the LLM can call:

    - **`list_clusters`**: Returns cluster names, categories, and hints — no tool names, no schemas. The LLM uses this only when it needs to investigate beyond the pre-scoped target cluster.
    - **`list_tools_for_cluster(cluster_id)`**: Returns the tools available on a specific cluster and makes them callable. Tool names carry the gateway-assigned cluster prefix (e.g., `prod_cluster_01_resources_get`), which disambiguates tools across clusters in the LLM context.

    **LLM context management**: KA pre-scopes the session to the alert's target cluster at investigation start. The LLM begins with only that cluster's ~18 tools plus the two discovery tools (~20 total). If the LLM determines it needs another cluster (e.g., a cross-cluster dependency), it calls `list_clusters` to browse and `list_tools_for_cluster` to add that cluster's tools. In 99% of investigations, discovery is never called — the pre-scoped tools suffice.

    | Moment | Tools in LLM context | Token cost |
    |--------|---------------------|-----------|
    | Start | 18 target-cluster tools + `list_clusters` + `list_tools_for_cluster` | ~20 schemas |
    | After `list_tools_for_cluster("cluster-02")` | 18 + 18 + 2 = 38 | Lean |
    | Never needed another cluster (99% of cases) | 20 | Minimal |

    **Gateway-specific implementations**: The `GatewayDiscoverer` interface has one implementation per supported gateway, selected by `gatewayType` configuration (same config FMC already uses):

    ```go
    type GatewayDiscoverer interface {
        ListClusters(ctx context.Context) ([]ClusterInfo, error)
        ToolsForCluster(ctx context.Context, clusterID string) ([]ToolDefinition, error)
    }
    ```

    | Gateway | `ListClusters()` | `ToolsForCluster()` |
    |---------|-----------------|-------------------|
    | **Kuadrant** | Calls `discover_tools` meta-tool → returns server metadata | Calls `select_tools` → server-side session scoping, returns tool list |
    | **EAIGW** | Calls `tools/list`, parses unique `__` prefixes | Filters `tools/list` by prefix (client-side filtering) |

    Kuadrant's `discover_tools` / `select_tools` are meta-tools controlled by the `--discovery-tool-threshold` flag. At threshold=0 (default), all tools appear in `tools/list` alongside meta-tools. At threshold > 0, `tools/list` returns only meta-tools, forcing the Discover → Select → Work flow. The `KuadrantDiscoverer` handles both modes transparently.

    **Configuration**: The `gatewayType` is a single field on the Kubernaut CRD (`spec.fleet.gatewayType`). The operator propagates it to both KA's and FMC's configs during reconciliation — one knob, two consumers, zero duplication. Services never need to auto-detect the gateway type.

    **Adding a new gateway**: Implement `GatewayDiscoverer` for the new gateway and register it in the factory. Add the new value to the `gatewayType` enum on the CRD. No changes to KA, services, or LLM tooling.

    **Authority**: Spike S15 (2026-06-27) — validated Kuadrant `discover_tools` / `select_tools` response format, threshold behavior, and tool prefix conventions against live Kuadrant MCP Gateway v0.7.0 in Kind.

12. **kube-mcp-server MUST be deployed with `--list-output=yaml`** (not the default `table`): Kubernaut's `mcpclient` consumes `structuredContent` from `CallToolResult` to build `unstructured.Unstructured` objects without fragile text parsing. The `structuredContent` shape depends on this flag:

    - **`--list-output=table` (default)**: `resources_list` returns flat table-row maps (`{"Name":"x","Status":"y","Age":"5m"}`) in `structuredContent`. These lack `metadata.name`/`metadata.namespace` — `unstructured.GetName()` returns `""`, breaking FMC syncer scope cache keys and any consumer using standard K8s object accessors.
    - **`--list-output=yaml`**: `resources_list` returns full K8s objects (`{"apiVersion":"v1","kind":"Pod","metadata":{"name":"x","namespace":"y",...},"spec":{...},"status":{...}}`) in `structuredContent`. All `unstructured.Unstructured` accessors work correctly.

    `resources_get` returns full K8s objects in `structuredContent` regardless of this flag (it always uses YAML output internally). The flag only affects `resources_list`.

    **Trade-off**: YAML output produces larger responses than table output (~220KB vs ~6KB for 20 pods). This is acceptable for programmatic clients (FMC, SP, WE). LLM-facing sessions (KA investigation) should use a separate kube-mcp-server instance with `--list-output=table` if context window size is a concern.

    **Text parsing fallback**: When `structuredContent` is nil (older kube-mcp-server versions without PR #1232), the client falls back to parsing `Content[0].Text` via `parseMultiFormat` (JSON → YAML → table detection). This degraded path is retained for backward compatibility but logs a warning. The primary data path is `structuredContent`.

    **Authority**: Spike S16 (2026-06-29) — validated against live Kind cluster with Kuadrant MCP Gateway. kube-mcp-server PR #1232 (merged 2026-06-29) adds `structuredContent` via `PrintObjStructured` + `NewToolCallResultFull`.

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
- -: Drift risk; manual management; no reconciliation; no standardized contract
- **Rejected**: Envoy AI Gateway `MCPRoute`/`Backend` CRDs are the authoritative and universal contract

### E. Native control-plane adapters (ACM ManagedCluster watcher, Rancher adapter)
- +: Direct integration, potentially lower latency for cluster discovery
- -: Kubernaut couples to specific control plane; N adapters to maintain; same info available via gateway CRDs
- **Rejected**: Envoy AI Gateway CRDs are the generic interface. Control planes own the creation of these CRDs.

### I. Kuadrant MCP Gateway (alongside Envoy AI Gateway)
- +: Mature MCP server registration model, Kuadrant ecosystem integration, configurable tool prefix
- -: Requires Istio as Gateway API provider; 3+ components to deploy; ~388 MB memory footprint; cannot run standalone (requires K8s + Istio)
- **Accepted**: Both gateways are supported via the MCP Gateway Adapter Pattern. Spike-s14 (2026-06-25) validated Kuadrant in Kind with all go/no-go criteria passed. The E2E fleet lane uses Kuadrant; EAIGW is validated via container IT. Platform teams choose based on their infrastructure.

### F. Single hardcoded scope resolution backend (Valkey only)
- +: Simpler implementation
- -: Forces FMC+Valkey deployment even when ACM Search provides the same data natively
- -: Excludes Rancher shops, standalone Karmada users, and lightweight multi-cluster deployments
- **Rejected**: Federated Control Plane interface with adapter pattern avoids unnecessary infrastructure and removes market constraints

### G. GW/RO directly connecting to storage backends (Valkey, ACM Search)
- +: Lower latency (no intermediate service hop for FMC)
- -: GW/RO config leaks storage implementation details (`valkeyAddr`, GraphQL endpoints)
- -: Backend swap requires config changes in every consuming service
- -: Violates adapter pattern — consumers become coupled to providers
- **Rejected**: FMC and other backends expose their own query APIs; GW/RO consume through the `FederatedControlPlane` interface

### H. Separate auth path for AAP MCP Server (direct bearer token injection)
- +: AAP MCP Server auth is self-contained; BackendTLSPolicy + token Secret per AAP backend
- -: WE needs two auth paths: one for K8s MCP (via gateway) and one for AAP (direct token)
- -: Separate credential management and rotation for AAP tokens outside the gateway
- -: AAP MCP backends are treated differently from K8s MCP backends, violating the unified chokepoint principle
- **Deferred**: AAP MCP Server is now registered as a standard `Backend` behind the MCP Gateway, same as K8s MCP Servers. The gateway handles routing to both. WE calls tools through the gateway without knowing whether the backend is K8s-native or AAP. If AAP MCP requires its own auth, the `MCPRoute.backendRefs[].securityPolicy` handles upstream credential injection — Kubernaut services are not involved.

## Consequences

### Positive
- Fleet scope checking achieves p95 < 1ms (Valkey EXISTS)
- Zero regression for single-cluster (fleet disabled by default)
- Credential rotation requires no service restart
- New clusters auto-discovered via CRDWatcher (watches `MCPRoute`/`Backend` CRDs)
- LLM investigator automatically gains remote cluster tools
- Unified chokepoint: all remote access (read and write) flows through MCP Gateway
- WE can execute Jobs, Tekton pipelines, and AAP workflows on any cluster through a single interface
- No per-cluster credentials managed by Kubernaut services

### Negative
- New service to deploy (FMC Writer)
- Valkey dependency (already exists for other Kubernaut features)
- Stale cache window (sync interval, default 30s)
- MCP subscriptions not yet supported (polling fallback)
- K8s MCP Server SA requires write permissions (larger blast radius than read-only); mitigated by gateway-level CEL authorization rules

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
| SP Enrichment | SP remote cluster enrichment via MCP Gateway (BR-INTEGRATION-054) | Complete |
| RO Fleet Scope | RO scope routing via FederatedScopeChecker (BR-FLEET-054) | Complete |
| WE Fleet Routing | WE JobExecutor.IsCompleted ClusterID propagation (BR-FLEET-054) | Complete |
| EM Fleet Routing | EM target-read routing via ReaderFactory (BR-FLEET-054) | Complete |
| AF Fleet Routing | AF kubectl_get/kubectl_list ResourceReader abstraction + list_clusters tool (BR-FLEET-054) | Complete |
| GatewayDiscoverer | Two-phase tool discovery (`list_clusters`/`list_tools_for_cluster`) with Kuadrant and EAIGW implementations | Planned (v1.6) |
| Rancher Adapter | Rancher v3 API adapter for cluster discovery and scope checks | Planned (v1.6) |
| Clusterpedia Adapter | Clusterpedia Aggregated API adapter for scope checks | Planned (v1.6) |

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|---------|
| FederatedScopeChecker | NewServerWithMetrics() | pkg/gateway/server.go | IT-GW-FLEET-010 |
| fleetScopeChecker dispatch | validateScope() | pkg/gateway/server.go:1518 | IT-GW-FLEET-010/011/012 |
| scope.ScopeChecker (remote backend) | FederatedScopeChecker | pkg/fleet/federated_checker.go | UT-FLEET-FC-003/004/005 |
| fleet.NewScopeChecker factory | NewServer() / main() | pkg/fleet/scope_factory.go | UT-FLEET-FAC-001..006 |
| ResilientClient (KA) | registerFleetTools() | cmd/kubernautagent/main.go | IT-KA-FLEET-001 |
| ResilientClient (FMC) | main() | cmd/fmc/main.go | UT-FMC-001 |
| ReloadableOAuth2Transport (KA) | registerFleetTools() | cmd/kubernautagent/main.go | UT-FLEET-RES-001 |
| ReloadableOAuth2Transport (FMC) | main() | cmd/fmc/main.go | UT-FLEET-RES-001 |
| AppendFleetToolsToRCA | registerFleetTools() → main | cmd/kubernautagent/main.go:231 | IT-KA-FLEET-001 |
| CRDWatcher | main() | cmd/fmc/main.go | UT-FMC-004 |
| BuildKey validation | BuildKey() | pkg/fleet/scopecache/client.go | UT-FLEET-SC-006/007/008 |
| fmc securityContext | Helm template | charts/kubernaut/templates/fmc/fmc.yaml | helm template |
| ResilientClient (SP) | main() | cmd/signalprocessing/main.go | IT-SP-054-001 |
| MCPReaderFactory (SP) | main() | cmd/signalprocessing/main.go | IT-SP-054-001 |
| K8sEnricher.SetReaderFactory | main() | cmd/signalprocessing/main.go | IT-SP-054-001/002 |
| enrichRemote | Enrich() | pkg/signalprocessing/enricher/k8s_enricher.go | UT-SP-054-003a/b/c |

## MCP Gateway Access Model

### Unified Chokepoint

The MCP Gateway is the single entry point for all Kubernaut service interactions with remote
clusters. All backend MCP Servers — K8s MCP Servers and AAP MCP Servers — are registered behind
the gateway via its native CRDs (`MCPRoute`/`Backend` for EAIGW, `MCPServerRegistration` for
Kuadrant). Kubernaut services connect to the gateway, not to individual backends.

```
                   ┌──────────────────────────────────┐
                   │    MCP Gateway (EAIGW / Kuadrant) │
                   │  ┌────────────────────────────┐   │
                   │  │   Gateway-native authz      │   │
                   │  │   (OAuth+CEL / Authorino)   │   │
                   │  └────────────────────────────┘   │
                   │            │                       │
                   │  ┌─────────┴──────────┐           │
                   │  │  CEL Auth Rules     │           │
                   │  │  mcp-read:          │           │
                   │  │   GW,KA,RO,SP,      │           │
                   │  │   AF,EM,FMC         │           │
                   │  │  mcp-write:         │           │
                   │  │   WE                │           │
                   │  └────────────────────┘           │
                   └──────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
        K8s MCP Srv     K8s MCP Srv     AAP MCP Srv
        (mcp-operator   (mcp-operator   (AAP-managed
         SA, R+W)        SA, R+W)        identity)
```

### Per-Service Access Levels

| Service | Gateway Role | Operations | Rationale |
|---------|-------------|------------|-----------|
| **GW** | `mcp-read` | Read-only `tools/call` | Scope gating: checks remote resource labels for `kubernaut.ai/managed=true` |
| **KA** | `mcp-read` | `tools/list`, read-only `tools/call` | Investigation/RCA: reads cluster state (pods, logs, events, deployments) |
| **RO** | `mcp-read` | Read-only `tools/call` | Remediation orchestration: reads remote resource state for scope validation |
| **SP** | `mcp-read` | Read-only `tools/call` | Signal processing: reads remote cluster context for signal enrichment |
| **AF** | `mcp-read` | Read-only `tools/call` | API frontend: reads remote cluster state for user-facing queries |
| **EM** | `mcp-read` | Read-only `tools/call` | Effectiveness measurement: reads remote resource state for post-remediation assessment |
| **FMC** | `mcp-read` | `tools/list`, read-only `tools/call` | Metadata sync: polls for `kubernaut.ai/managed=true` labels |
| **WE** | `mcp-read` + `mcp-write` | `tools/list`, read+write `tools/call` | Remediation execution: reads state before acting, then creates/updates/deletes resources |

### WE Remote Cluster Operations

WE requires both read and write access on remote clusters through the MCP Gateway.

**Read operations** (pre-action validation and status polling):

| Resource | Group | Verbs | Purpose |
|----------|-------|-------|---------|
| Deployments, StatefulSets, DaemonSets | `apps` | get, list | Read state before scaling/restarting |
| Pods | `""` | get, list, watch | Target lookup for eviction/restart |
| ReplicaSets | `apps` | get, list, watch | Rollout status checks |
| ConfigMaps, Secrets | `""` | get, list | Dependency validation |
| Nodes | `""` | get, list | Node status for cordon/drain |
| HPAs | `autoscaling` | get, list | Current HPA state before tuning |
| PDBs | `policy` | get, list | Respect PDB during drain |
| Jobs | `batch` | get, list | Status polling for running workflows |
| PipelineRuns, TaskRuns | `tekton.dev` | get, list | Tekton workflow status |

**Write operations** (remediation actions):

| Resource | Group | Verbs | Purpose |
|----------|-------|-------|---------|
| Deployments, StatefulSets, DaemonSets | `apps` | patch, update | Scale, restart, rollback |
| Pods | `""` | delete | Pod restart |
| Pods/eviction | `""` | create | Graceful pod eviction |
| Nodes | `""` | patch, update | Cordon/uncordon |
| ConfigMaps | `""` | create, update, patch | Config remediation |
| Secrets | `""` | create, update, patch, delete | Secret rotation |
| HPAs | `autoscaling` | patch | HPA tuning |
| PDBs | `policy` | patch | PDB adjustment during drain |
| NetworkPolicies | `networking.k8s.io` | create, update, patch, delete | Network isolation |
| Services | `""` | create, update, patch | Service endpoint updates |
| PVCs | `""` | create, update, patch, delete | Storage remediation |
| Jobs | `batch` | create, delete | K8s Job-based remediation workflows |
| PipelineRuns | `tekton.dev` | create, delete | Tekton pipeline-based remediation |

### K8s MCP Server Identity

Each K8s MCP Server runs with its own ServiceAccount on its local cluster. The SA defines the
**ceiling** of what any caller can do through the gateway. The gateway's OPA policy defines
what each caller is **allowed** to do.

For clusters where WE needs write access, the K8s MCP Server SA requires write verbs:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: mcp-operator
  namespace: kubernaut-mcp
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mcp-operator
rules:
  # Read access (KA investigation + FMC sync + WE pre-action validation)
  - apiGroups: [""]
    resources: ["pods", "pods/log", "services", "endpoints", "configmaps",
                "secrets", "events", "namespaces", "nodes",
                "persistentvolumeclaims", "persistentvolumes",
                "replicationcontrollers", "serviceaccounts", "resourcequotas",
                "limitranges"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "daemonsets", "replicasets", "statefulsets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["ingresses", "networkpolicies"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["autoscaling"]
    resources: ["horizontalpodautoscalers"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["policy"]
    resources: ["poddisruptionbudgets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list"]
  - apiGroups: ["metrics.k8s.io"]
    resources: ["pods", "nodes"]
    verbs: ["get", "list"]
  - apiGroups: ["apiextensions.k8s.io"]
    resources: ["customresourcedefinitions"]
    verbs: ["get", "list"]
  # Write access (WE remediation execution)
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["delete"]
  - apiGroups: [""]
    resources: ["pods/eviction"]
    verbs: ["create"]
  - apiGroups: [""]
    resources: ["configmaps", "secrets", "services", "persistentvolumeclaims"]
    verbs: ["create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["patch", "update"]
  - apiGroups: ["apps"]
    resources: ["deployments", "daemonsets", "statefulsets"]
    verbs: ["patch", "update"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["create", "delete"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["create", "update", "patch", "delete"]
  - apiGroups: ["autoscaling"]
    resources: ["horizontalpodautoscalers"]
    verbs: ["patch"]
  - apiGroups: ["policy"]
    resources: ["poddisruptionbudgets"]
    verbs: ["patch"]
  # Tekton (if installed on the cluster)
  - apiGroups: ["tekton.dev"]
    resources: ["pipelineruns", "taskruns"]
    verbs: ["get", "list", "watch", "create", "delete"]
```

The SA name changes from `mcp-viewer` to `mcp-operator` to reflect the expanded permissions.
The `--read-only` flag is removed from the K8s MCP Server deployment args.

### Auth Architecture: Two Independent Boundaries

Auth is split into two independent concerns. No per-cluster SA tokens are maintained by
Kubernaut services.

**Boundary 1: Kubernaut service → MCP Gateway (north-south)**

| Aspect | Detail |
|--------|--------|
| Mechanism | JWT (Keycloak/DEX) validated by Envoy AI Gateway's built-in OAuth |
| Identity | Service identity: GW, KA, RO, SP, AF, EM, FMC, or WE |
| Authorization | CEL authorization rules on `MCPRoute`: JWT scopes `mcp-read` (GW, KA, RO, SP, AF, EM, FMC) or `mcp-write` (WE); per-cluster gating via `request.mcp.backend in request.auth.jwt.claims.kubernaut_allowed_clusters` |
| Credential mgmt | `ReloadableOAuth2Transport` with file-watched client credentials |

**Boundary 2: MCP Gateway → Backend MCP Server (east-west)**

| Aspect | Detail |
|--------|--------|
| In-cluster | Direct Service routing via `Backend` CRD; no auth (same network) |
| Cross-cluster | `Backend` CRD with TLS endpoint + `securityPolicy.apiKey` for upstream auth |
| Backend identity | Each MCP Server uses its own SA (`mcp-operator`) on its local cluster |
| No token delegation | User/service identity stops at the gateway; backends act with fixed SA |

This is architecturally distinct from the AF → KA pattern, where AF mints a JWT carrying
user context (acting_user, acting_groups) for downstream identity delegation. In the MCP
Gateway architecture, authorization is enforced at the edge (Envoy AI Gateway), and backends
trust the gateway's routing decisions.

## Federated Control Plane Interface

### Design Principle: Adapter Pattern

GW and RO are **consumers** of federated scope information — they must not know how or where
that information is stored. The storage backend (Valkey, ACM Search, Rancher proxy, Clusterpedia)
is an implementation detail owned by the adapter, not by GW or RO.

```
GW/RO  ──►  FederatedControlPlane interface
                        │
            ┌───────────┼──────────────────────────┐
            ▼           ▼                          ▼
     FMC adapter    ACM adapter    Rancher / Clusterpedia adapter
            │           │                          │
            ▼           ▼                          ▼
     Valkey         ACM Search                Rancher v3 API /
     (co-owned      GraphQL API               Clusterpedia
      with DS)                                Aggregated API
```

GW/RO config **does not** contain storage addresses (e.g., `valkeyAddr`). It contains only:
- `enabled: bool` — whether federation is active
- `backend: string` — which adapter to instantiate (`"fmc"`, `"acm"`, `"rancher"`, `"clusterpedia"`)
- `endpoint: string` — adapter-specific endpoint (FMC service URL, ACM Search route, etc.)

### Interface Contract

The unified `ScopeChecker` interface replaced the previous multi-interface stack
(`scope.ScopeChecker` (3-string), `scope.FederatedScopeChecker`, `RemoteScopeResolver`, `IsManagedTyped`)
with a single method accepting a `ResourceIdentity` struct. This migration is complete.

```go
// ResourceIdentity uniquely identifies a Kubernetes resource, optionally on a remote cluster.
type ResourceIdentity struct {
    ClusterID string // empty for local/hub cluster
    Group     string // API group (e.g., "apps", "" for core)
    Version   string // API version (e.g., "v1")
    Kind      string // e.g., "Deployment"
    Namespace string // empty for cluster-scoped resources
    Name      string
}

// ScopeChecker validates if a resource is within Kubernaut's management scope.
// A single method handles both local and remote clusters — the implementation
// routes internally based on ResourceIdentity.ClusterID.
//
// GW and RO consume this interface. The factory selects the implementation at
// construction time based on fleet.backend config. Zero changes to consumers
// when swapping backends.
type ScopeChecker interface {
    IsManaged(ctx context.Context, resource ResourceIdentity) (bool, error)
}
```

**What this replaced** (all removed):

| Removed | Problem | Status |
|---------|---------|--------|
| `scope.ScopeChecker.IsManaged(ctx, ns, kind, name)` | No clusterID, no GVK | Deleted |
| `scope.FederatedScopeChecker.IsManagedOnCluster(ctx, clusterID, ns, kind, name)` | Separate method forces conditionals at every call site | Deleted |
| `RemoteScopeResolver.IsManaged(ctx, clusterID, group, version, kind, ns, name)` | 7 positional strings, leaked Valkey abstraction | Deleted |
| `Client.IsManagedTyped(ctx, clusterID, gvk, key)` | Workaround wrapper | Deleted |

**Factory pattern** — the only change is at construction time in `cmd/`:

```go
// cmd/gateway/main.go, cmd/remediationorchestrator/main.go
scopeChecker, err := fleet.NewScopeChecker(cfg.Fleet, logger)
```

```go
// pkg/fleet/scope_factory.go
func NewScopeChecker(cfg FleetConfig, logger logr.Logger) (scope.ScopeChecker, error) {
    switch cfg.Backend {
    case "fmc":
        return fmc.NewClient(cfg.Endpoint, logger)
    case "acm":
        return acm.NewClient(cfg.Endpoint, cfg.Auth, logger)
    case "rancher":
        return rancher.NewClient(cfg.Endpoint, cfg.Auth, logger)
    case "clusterpedia":
        return clusterpedia.NewClient(logger)
    case "", "local":
        return local.NewChecker(logger) // single-cluster, no regression
    default:
        return nil, fmt.Errorf("unknown fleet backend: %q", cfg.Backend)
    }
}
```

Each implementation handles the `ClusterID` routing internally:
- `ClusterID == ""` → local K8s API check (resource/namespace label lookup)
- `ClusterID != ""` → backend-specific remote check (FMC REST, ACM GraphQL, etc.)

Single-cluster deployments use `backend: "local"` (or omit `fleet` config entirely) —
no regression, no FMC/Valkey dependency.

### Backend A: FMC (Fleet Metadata Cache) — Default

**When to use**: GitOps environments, standalone K8s clusters, environments **without** a
federated control plane (no ACM, Rancher, or Clusterpedia deployed).

**Architecture**: FMC is the Scope Service defined in issue #54 — a dedicated HTTP service for
federated `kubernaut.ai/managed` label resolution. It caches metadata from remote clusters via
MCP K8s, stores it in Valkey with TTL, and exposes a REST API that GW/RO query through the
`FederatedControlPlane` interface. GW/RO never touch Valkey directly.

| Aspect | Detail |
|--------|--------|
| **Service** | `cmd/fmc/` — HTTP server + MCP poller + Valkey cache |
| **Storage** | Valkey (co-owned with DataStorage) — internal to FMC, not exposed to consumers |
| **Read API** | REST endpoint for scope checks and cluster listing |
| **Latency** | p95 < 1ms (Valkey EXISTS behind the API) |
| **Staleness** | Bounded by sync interval (default 30s) + TTL (45s) |
| **Dependencies** | MCP Gateway, Valkey, MCP Server per cluster |
| **Config** | `fleet.backend: "fmc"`, `fleet.fmc.endpoint: "http://fmc.kubernaut-system.svc:8080"` |

**FMC REST API contract**:

```
GET /api/v1/scope/check?cluster={clusterID}&namespace={ns}&kind={kind}&name={name}
→ 200 {"managed": true}  or  {"managed": false}

GET /api/v1/clusters
→ 200 {"clusters": [{"id": "prod-east", "name": "Production East"}, ...]}

GET /healthz
→ 200 OK
```

**FMC connectivity requirements**:

| Requirement | Detail |
|------------|--------|
| **FMC service** | FMC must be deployed in the same cluster as GW/RO. Exposes REST API on port 8080. Handles both the write side (MCP polling → Valkey) and the read side (REST API → Valkey EXISTS). |
| **Valkey** | Internal to FMC — GW/RO do not connect to Valkey. FMC manages its own Valkey connection (co-owned with DataStorage). Configured in FMC's own config. |
| **MCP Gateway** | FMC polls MCP Gateway for managed cluster resources. Configured via OAuth2 in FMC's config (ReloadableOAuth2Transport). |
| **Authentication** | GW/RO → FMC: in-cluster Service access (no auth required if same namespace, or mTLS if cross-namespace). FMC → Valkey: password-based or mTLS. **FMC → MCP Gateway: OAuth2 (mandatory)** — there is no unauthenticated fallback. `oauth2.tokenUrl` and `oauth2.credentialsSecretRef` are required when `fmc.enabled=true`. |
| **Network** | GW/RO pods must reach FMC Service on port 8080. FMC must reach Valkey (6379) and MCP Gateway. A `NetworkPolicy` restricts ingress to GW/RO pods and egress to Valkey + MCP Gateway + DNS. |
| **Configuration** | FMC uses structured YAML configuration per ADR-030 (not env vars). The Helm chart mounts the config at `/etc/fmc/config.yaml` and OAuth2 credentials at `/etc/fmc/fleet-oauth2/`. Fields use camelCase per CRD_FIELD_NAMING_CONVENTION.md. See `pkg/fleet/fmc/config/config.go`. |
| **Kubernaut config** | `fleet.backend: "fmc"`, `fleet.fmc.endpoint: "http://fmc.kubernaut-system.svc:8080"`. |

### Backend B: ACM (Advanced Cluster Management)

**When to use**: Red Hat ACM / Open Cluster Management (OCM) environments where ACM Search
is already deployed and indexing resources across managed clusters.

**Architecture**: The ACM adapter queries the ACM Search GraphQL API directly — no FMC Writer
or Valkey needed. ACM Search already indexes all managed cluster resources via the
`klusterlet-addon-search` collector.

| Aspect | Detail |
|--------|--------|
| **Service** | None (adapter library in `pkg/fleet/controlplane/acm/`) |
| **Storage** | ACM Search index (managed by ACM) |
| **Scope query** | GraphQL query to `search-api` service |
| **Latency** | p95 ~10-50ms (higher than Valkey, acceptable) |
| **Staleness** | Near-real-time (ACM Search collector push) |
| **Dependencies** | ACM hub cluster, `klusterlet-addon-search` enabled on managed clusters |
| **Config** | `fleet.backend: "acm"`, `fleet.endpoint: "https://search-search-api.open-cluster-management.svc:4010"` |
| **Auth** | Service account bearer token with scoped RBAC (see setup guide below) |
| **TLS** | Served by `openshift-service-serving-signer` CA; in-cluster CA inject via `service.beta.openshift.io/inject-cabundle: "true"` ConfigMap annotation |
| **Implementation** | `pkg/fleet/acm/client.go` (hand-rolled HTTP+JSON, zero dependencies) |

**ACM Search GraphQL contract**:

```graphql
# Scope check: is a specific resource managed?
query scopeCheck($input: [SearchInput]) {
    searchResult: search(input: $input) {
        count
    }
}

# Variables:
{
    "input": [{
        "filters": [
            {"property": "kind",      "values": ["Deployment"]},
            {"property": "name",      "values": ["nginx"]},
            {"property": "namespace", "values": ["production"]},
            {"property": "cluster",   "values": ["prod-east"]},
            {"property": "label",     "values": ["kubernaut.ai/managed=true"]}
        ],
        "limit": 1
    }]
}
# → count > 0 means managed

# Cluster listing:
query listClusters($input: [SearchInput]) {
    searchResult: search(input: $input) {
        items
    }
}

# Variables:
{
    "input": [{
        "filters": [
            {"property": "kind", "values": ["Cluster"]}
        ]
    }]
}
```

**ACM connectivity requirements**:

| Requirement | Detail |
|------------|--------|
| **ACM version** | 2.7+ (Search v2 with GraphQL API) |
| **Managed cluster addon** | `klusterlet-addon-search` enabled on every managed cluster. Configured via `KlusterletAddonConfig.spec.searchCollector.enabled: true` in each managed cluster's namespace on the hub. |
| **Search API endpoint** | `search-search-api` Service in `open-cluster-management` namespace (port 4010, HTTPS). If Kubernaut runs outside the ACM hub cluster, create a passthrough Route: `oc create route passthrough search-api --service=search-search-api -n open-cluster-management` |
| **TLS** | The `search-search-api` Service uses TLS with certificates issued by `openshift-service-serving-signer`. For in-cluster access, inject the CA bundle via a ConfigMap with `service.beta.openshift.io/inject-cabundle: "true"` annotation and mount it into the Kubernaut pod. For cross-cluster, export the CA and mount it as a Secret. |
| **Authentication** | ACM Search enforces K8s authentication. Kubernaut needs a bearer token from a ServiceAccount on the ACM hub, passed as `Authorization: Bearer <token>`. |
| **Network policy** | Kubernaut pods (GW, RO) must reach the `search-search-api` Service on port 4010. If NetworkPolicies are enforced, add an egress rule from `kubernaut-system` to `open-cluster-management` namespace. |
| **Kubernaut config** | `fleet.backend: "acm"`, `fleet.endpoint: "https://search-search-api.open-cluster-management.svc:4010"`. If cross-cluster: `fleet.acm.tokenSecretRef: "acm-search-token"`, `fleet.acm.caBundle: "/etc/kubernaut/acm-ca/ca.crt"`. |

#### ACM Search Production Setup Guide

> **Validated**: OCP 4.21.5, ACM 2.17.0, 2026-06-22. All steps below were tested
> end-to-end on a live cluster. See `docs/spikes/multi-cluster-mcp-gateway/spike-acm-search/`
> for raw spike data.

Kubernaut's ACM adapter queries the Search GraphQL API for resource metadata (name,
namespace, kind, cluster, labels). ACM Search controls result visibility through two
independent RBAC layers:

1. **K8s RBAC** — standard ClusterRole/ClusterRoleBinding for API access
2. **ACM managed cluster visibility** — RoleBinding in each managed cluster's hub
   namespace (e.g., `local-cluster`, `prod-east`) using the built-in `view` ClusterRole

Both layers are required. K8s RBAC alone returns `count: 0` because the search-api's
fine-grained RBAC checks the `userpermissions` virtual API (served by `ocm-proxyserver`)
to determine which clusters the caller can see. The `userpermissions` API only recognizes
well-known Kubernetes aggregate roles (`admin`, `view`, `edit`) bound in managed cluster
namespaces.

**Step 1: Enable fine-grained RBAC on the ACM hub**

ACM Search defaults to `fine-grained-rbac: false` (a MulticlusterHub component), which
requires `cluster-admin` for any search results. Enable via the MCH component override:

```bash
oc patch mch multiclusterhub -n open-cluster-management --type json \
  -p '[{"op":"replace","path":"/spec/overrides/components","value":[
    ... existing components ...,
    {"name":"fine-grained-rbac","enabled":true,"configOverrides":{}}
  ]}]'
```

Or, if the `fine-grained-rbac` component is already listed in the MCH spec (check with
`oc get mch multiclusterhub -o jsonpath='{.spec.overrides.components}'`), patch just its
`enabled` field:

```bash
# Find the array index of fine-grained-rbac, then:
oc patch mch multiclusterhub -n open-cluster-management --type json \
  -p '[{"op":"replace","path":"/spec/overrides/components/<INDEX>/enabled","value":true}]'
```

Verify the search-api pod restarts and shows `FineGrainedRbac: true` in its startup config:

```bash
oc logs deploy/search-api -n open-cluster-management | grep -A3 Features
# Expected: "FineGrainedRbac": true
```

> **Important**: The Search CR annotation `fine-grained-rbac` and the Search CR spec
> field `FineGrainedRbac` are NOT the correct mechanism. The MCH component override is
> the only mechanism validated to propagate the setting to the search-api pod.

**Step 2: Create the Kubernaut fleet reader ServiceAccount**

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-fleet-reader
  namespace: kubernaut-system
```

**Step 3: Grant search API access**

The `global-search-user` ClusterRole grants access to the Search API endpoint itself:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-search-api-access
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: global-search-user
subjects:
  - kind: ServiceAccount
    name: kubernaut-fleet-reader
    namespace: kubernaut-system
```

**Step 4: Grant userpermissions API access**

The search-api queries the `userpermissions` virtual API (served by `ocm-proxyserver`)
to determine what each caller can see. The SA needs `list` access to this API:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-search-userpermissions
rules:
  - apiGroups: ["clusterview.open-cluster-management.io"]
    resources: ["userpermissions"]
    verbs: ["list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-search-userpermissions
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-search-userpermissions
subjects:
  - kind: ServiceAccount
    name: kubernaut-fleet-reader
    namespace: kubernaut-system
```

Without this, the search-api returns: `"unable to resolve query because of error while
refreshing user permissions"`.

**Step 5: Grant managed cluster visibility**

This is the critical step. The `userpermissions` virtual API determines cluster visibility
by checking RoleBindings in each managed cluster's hub namespace. Only the built-in
Kubernetes aggregate roles (`admin`, `view`, `edit`) are recognized — custom ClusterRoles
are ignored.

Create a `view` RoleBinding in **each managed cluster namespace** the SA needs to query:

```yaml
# Repeat for each managed cluster (e.g., local-cluster, prod-east, staging-west)
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kubernaut-fleet-reader-view
  namespace: local-cluster    # ← managed cluster namespace on the hub
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
  - kind: ServiceAccount
    name: kubernaut-fleet-reader
    namespace: kubernaut-system
```

> **Why `view` and not a custom role?**: The `userpermissions` API (served by
> `ocm-proxyserver`) only maps well-known Kubernetes aggregate roles to managed cluster
> permissions. A custom ClusterRole bound in the managed cluster namespace returns empty
> items from `userpermissions`, resulting in `count: 0` from search queries. This was
> validated by testing both approaches on ACM 2.17.0.

> **Scope**: The `view` RoleBinding in the managed cluster namespace does NOT grant
> K8s API access to resources on the managed cluster. It only tells the search-api
> that the SA has "view" level visibility for search results from that cluster.

For environments with many managed clusters, automate this with a Placement +
PolicySet or a simple script:

```bash
for cluster in $(oc get managedcluster -o jsonpath='{.items[*].metadata.name}'); do
  oc create rolebinding kubernaut-fleet-reader-view \
    --clusterrole=view \
    --serviceaccount=kubernaut-system:kubernaut-fleet-reader \
    -n "$cluster" --dry-run=client -o yaml | oc apply -f -
done
```

**Step 6: Generate a token (cross-cluster deployments only)**

If Kubernaut runs on a different cluster than the ACM hub:

```bash
# On the ACM hub cluster — create a bound token with rotation
oc create token kubernaut-fleet-reader -n kubernaut-system --duration=8760h

# Store the token as a Secret on the Kubernaut cluster
kubectl create secret generic acm-search-token \
  -n kubernaut-system \
  --from-literal=token=<token-value>
```

For production, prefer short-lived tokens with automated rotation over long-lived tokens.

**Step 7: Inject the TLS CA bundle (in-cluster)**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: acm-search-ca
  namespace: kubernaut-system
  annotations:
    service.beta.openshift.io/inject-cabundle: "true"
data: {}
```

Mount this ConfigMap in the Kubernaut pod and configure
`fleet.acm.caBundle: "/etc/kubernaut/acm-ca/service-ca.crt"`.

**Verification**

After completing setup, verify the SA can query ACM Search:

```bash
TOKEN=$(oc create token kubernaut-fleet-reader -n kubernaut-system)
curl -sk -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  https://search-search-api.open-cluster-management.svc:4010/searchapi/graphql \
  -d '{"query":"query($input:[SearchInput]){searchResult:search(input:$input){count}}","variables":{"input":[{"filters":[{"property":"kind","values":["Deployment"]},{"property":"cluster","values":["local-cluster"]}]}]}}'
# Expected: {"data":{"searchResult":[{"count":N}]}} where N > 0
```

If `count: 0` is returned despite managed clusters existing, check:
1. `fine-grained-rbac` component is enabled in the MCH (`oc get mch -o jsonpath='{.items[0].spec.overrides.components}'`)
2. The search-api shows `FineGrainedRbac: true` in startup logs
3. `userpermissions` API access is granted (Step 4)
4. `view` RoleBinding exists in the managed cluster namespace (Step 5)
5. Verify userpermissions returns items: `curl -sk -H "Authorization: Bearer $TOKEN" $(oc whoami --show-server)/apis/clusterview.open-cluster-management.io/v1alpha1/userpermissions`

**Validated RBAC summary** (OCP 4.21.5, ACM 2.17.0):

| RBAC Component | Purpose | Required |
|----------------|---------|----------|
| MCH `fine-grained-rbac` component enabled | Switches search-api from cluster-admin-only to RBAC-based filtering | Yes |
| `global-search-user` ClusterRoleBinding | Grants SA access to the Search API endpoint | Yes |
| `userpermissions` ClusterRole + ClusterRoleBinding | Allows search-api to check SA's permissions | Yes |
| `view` RoleBinding in managed cluster namespace | Makes SA visible to `userpermissions` API for that cluster | Yes (per cluster) |

**ACM Search schema verification**: Filter properties (`kind`, `name`, `namespace`, `cluster`,
`label`) are confirmed in the ACM Search schema (`searchSchema` query returns `allProperties`).
Label values use `key=value` format per the
[Search Query API spec](https://github.com/stolostron/search-v2-operator/wiki/Search-Query-API).

**Cluster listing item shape** (validated by spike, ACM 2.17.0): each item in
`searchResult[0].items` is a flat `map[string]string` containing:
- `name` / `cluster` — cluster identifier (same value in both fields)
- `apiEndpoint` — Kubernetes API URL
- `kubernetesVersion` — cluster K8s version
- `label` — semicolon-delimited label string
- `ManagedClusterConditionAvailable` — `"True"` / `"False"`
- `ManagedClusterJoined` — `"True"` / `"False"`

### Backend C: Rancher

**When to use**: SUSE Rancher environments managing multiple downstream clusters.

**Architecture**: The Rancher adapter uses the Rancher v3 API for cluster listing and the
Kubernetes proxy API (`/k8s/clusters/{clusterID}/...`) for resource-level scope checks.

| Aspect | Detail |
|--------|--------|
| **Service** | None (adapter library in `pkg/fleet/controlplane/rancher/`) |
| **Storage** | Rancher's internal cluster state |
| **Scope query** | K8s proxy: `GET /k8s/clusters/{id}/api/v1/namespaces/{ns}/{resource}/{name}` — check for `kubernaut.ai/managed` label |
| **Latency** | p95 ~20-100ms estimated (K8s API proxy through Rancher; depends on deployment topology — needs spike to validate) |
| **Staleness** | Real-time (direct K8s API call) |
| **Dependencies** | Rancher server, API key with cluster access |
| **Config** | `fleet.backend: "rancher"`, `fleet.endpoint: "https://rancher.example.com"` |
| **Auth** | Rancher API key (bearer token) |

**Rancher API contract**:

```
# Cluster listing (v3 API):
GET /v3/clusters
→ {"data": [{"id": "c-m-abc12345", "name": "production", "state": "active"}, ...]}

# Resource scope check (K8s proxy):
GET /k8s/clusters/{clusterID}/api/v1/namespaces/{ns}/pods/{name}
→ Check metadata.labels for "kubernaut.ai/managed": "true"

# For non-namespaced resources:
GET /k8s/clusters/{clusterID}/api/v1/nodes/{name}
```

**Rancher connectivity requirements**:

| Requirement | Detail |
|------------|--------|
| **Rancher version** | v2.6+ (v3 API + K8s proxy) |
| **API key** | Create a Rancher API key (`Settings → API Keys → Add Key`) scoped to the clusters Kubernaut needs to query. The key produces a bearer token (`token-xxxxx:yyyyyy`). |
| **RBAC (Rancher)** | The API key's user must have at least `Cluster Member` role on target clusters (read access to resources). For least privilege, create a dedicated Rancher user (e.g., `kubernaut-fleet-reader`) with custom `Read-Only` Global Role, then assign `Cluster Member` per cluster. |
| **TLS** | Kubernaut must trust the Rancher server's TLS certificate. Mount the CA bundle if using internal/self-signed certs. |
| **Network** | Kubernaut pods must reach the Rancher server URL (typically port 443). If cross-cluster, ensure DNS resolution and firewall rules allow HTTPS traffic. |
| **Kubernaut config** | `fleet.backend: "rancher"`, `fleet.rancher.endpoint: "https://rancher.example.com"`, `fleet.rancher.apiKeySecretRef: "rancher-api-key"` (Secret with `token` key containing the bearer token). |

### Backend D: Clusterpedia

**When to use**: Lightweight multi-cluster environments using Clusterpedia for cross-cluster
resource synchronization and search, without a full fleet management platform.

**Architecture**: Clusterpedia registers as a Kubernetes Aggregated API on the hub cluster,
providing a standard K8s-compatible API for querying resources across all clusters. The adapter
uses `client-go` with Clusterpedia's API paths — no custom protocol needed.

| Aspect | Detail |
|--------|--------|
| **Service** | None (adapter library in `pkg/fleet/controlplane/clusterpedia/`) |
| **Storage** | Clusterpedia's internal index (MySQL/PostgreSQL backed) |
| **Scope query** | Aggregated API: `GET /apis/clusterpedia.io/v1beta1/resources/clusters/{clusterID}/...` |
| **Latency** | p95 ~5-30ms (aggregated API on hub cluster) |
| **Staleness** | Near-real-time (ClusterSynchroManager watches) |
| **Dependencies** | Clusterpedia deployed on hub cluster, `PediaCluster` CRs for each managed cluster |
| **Config** | `fleet.backend: "clusterpedia"`, `fleet.endpoint: ""` (uses in-cluster kubeconfig) |
| **Auth** | In-cluster service account (Aggregated API is accessed through the hub API server) |

**Clusterpedia API contract**:

```
# Scope check (resource in specific cluster):
GET /apis/clusterpedia.io/v1beta1/resources/clusters/{clusterID}/api/v1/namespaces/{ns}/pods/{name}
→ Check metadata.labels for "kubernaut.ai/managed": "true"

# List managed resources with label filter:
GET /apis/clusterpedia.io/v1beta1/resources/clusters/{clusterID}/api/v1/namespaces/{ns}/pods?labelSelector=kubernaut.ai/managed=true

# Multi-cluster resource search:
GET /apis/clusterpedia.io/v1beta1/resources/api/v1/pods?clusters={clusterID}&labelSelector=kubernaut.ai/managed=true

# Cluster listing (via PediaCluster CRDs):
kubectl get pediaclusters -o jsonpath='{.items[*].metadata.name}'
```

**Clusterpedia connectivity requirements**:

| Requirement | Detail |
|------------|--------|
| **Clusterpedia version** | v0.6+ (Aggregated API with label selector support) |
| **Deployment** | Clusterpedia operator deployed on the hub cluster (same cluster as Kubernaut). Clusterpedia registers as a K8s Aggregated API — no separate endpoint needed. |
| **PediaCluster CRs** | One `PediaCluster` CR per managed cluster, specifying the sync resources (at minimum: Pods, Deployments, StatefulSets, DaemonSets, Nodes, Services). Each `PediaCluster` contains the kubeconfig or token to reach the managed cluster. |
| **Authentication** | In-cluster SA token — Kubernaut's GW/RO ServiceAccount accesses Clusterpedia through the hub API server's Aggregated API path (`/apis/clusterpedia.io/...`). No additional token or credential needed. |
| **RBAC (hub)** | Kubernaut SA needs RBAC to access the Clusterpedia Aggregated API. Grant `get` and `list` on the `clusterpedia.io` API group. |
| **Network** | No cross-cluster networking needed — Clusterpedia's API server runs on the hub cluster. Kubernaut pods reach it through the hub API server (in-cluster). |
| **Kubernaut config** | `fleet.backend: "clusterpedia"`. No endpoint needed (uses in-cluster kubeconfig). |

**RBAC example (hub cluster)**:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-clusterpedia-reader
rules:
  - apiGroups: ["clusterpedia.io"]
    resources: ["*"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-clusterpedia-reader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-clusterpedia-reader
subjects:
  - kind: ServiceAccount
    name: kubernaut-gateway
    namespace: kubernaut-system
  - kind: ServiceAccount
    name: kubernaut-remediationorchestrator
    namespace: kubernaut-system
```

### Backend Comparison Matrix

| Capability | FMC (Valkey) | ACM Search | Rancher | Clusterpedia |
|-----------|-------------|------------|---------|-------------|
| **Scope check latency** | < 1ms | 10-50ms | 20-100ms | 5-30ms |
| **Staleness** | 30-45s (poll + TTL) | Near-real-time | Real-time | Near-real-time |
| **Extra infra required** | FMC Writer + Valkey | None (uses ACM) | None (uses Rancher) | None (uses Clusterpedia) |
| **Protocol** | REST (HTTP) | GraphQL | REST + K8s proxy | K8s Aggregated API |
| **Auth model** | OAuth2 / mTLS | SA token / OAuth2 | API key (bearer) | In-cluster SA |
| **Offline/airgap** | Yes | Yes (if ACM deployed) | Yes (if Rancher deployed) | Yes |
| **Owner chain resolution** | Via MCP Gateway | Via ACM Search relationships | Via K8s proxy | Via Clusterpedia owner search |
| **Best for** | GitOps, no fleet platform | Red Hat ACM shops | SUSE Rancher shops | Lightweight, vendor-neutral |

### Operator Configuration

Helm values determine which backend adapter is instantiated:

```yaml
fleet:
  enabled: true
  backend: "fmc"  # "fmc" | "acm" | "rancher" | "clusterpedia"

  # Backend-specific configuration (only the selected backend's section is used)
  fmc:
    endpoint: "http://fmc.kubernaut-system.svc:8080"

  acm:
    endpoint: "https://search-search-api.open-cluster-management.svc:4010"
    # Auth: uses mounted SA token by default

  rancher:
    endpoint: "https://rancher.example.com"
    apiKeySecretRef: "rancher-api-key"  # Secret with API key

  clusterpedia:
    # Uses in-cluster kubeconfig; no endpoint needed
    # Optionally specify custom resource sync config
```

### Migration Path: Current → Target

GW/RO now use `fleet.NewScopeChecker(scopeMgr, cfg.Fleet, logger)` — the factory selects the
backend adapter based on `FleetConfig.Backend`. `FederatedScopeChecker` accepts any
`scope.ScopeChecker` for both local and remote, with zero knowledge of Valkey or any specific backend.

Migration status:

1. **Phase 1** (complete): Unified `scope.ScopeChecker` interface with `ResourceIdentity`. `FederatedScopeChecker` moved to `pkg/fleet/`, decoupled from Valkey. Factory centralized in `pkg/fleet/scope_factory.go`.
2. **Phase 2** (complete): FMC REST API (`GET /api/v1/scope/check`, `GET /api/v1/clusters`) implemented in `cmd/fmc/`. `fmc.HTTPClient` adapter implementing `scope.ScopeChecker` created. Factory `BackendFMC` path uses `fmc.NewHTTPClient`.
3. **Phase 3** (complete): `BackendValkey` removed from config/factory/Helm. `ValkeyAddr` removed from GW/RO `FleetConfig`. `scopecache` package isolated to FMC internals only. GW/RO use only `backend` + `endpoint`.
4. **Phase 4** (complete): Server-side ClusterID validation in FMC handler via `registry.Get(clusterID)`.
5. **Phase 5** (complete): ACM Search adapter — `pkg/fleet/acm/client.go` implements `scope.ScopeChecker` via GraphQL. Contract validated against live OCP 4.21.5 / ACM 2.17.0. Spike findings in `docs/spikes/multi-cluster-mcp-gateway/spike-acm-search/`.

## Security Considerations

| Control | Implementation |
|---------|---------------|
| Authentication (gateway) | OAuth2 client credentials grant (file-mounted JWT) validated by Envoy AI Gateway's built-in OAuth |
| Authorization (gateway) | CEL authorization rules enforce role-based tool access: `mcp-read` (GW, KA, RO, SP, AF, EM, FMC), `mcp-write` (WE) |
| Credential rotation | ReloadableOAuth2Transport with FileWatcher |
| Token timeout | 10s context deadline on token refresh HTTP calls |
| Least privilege (FMC) | readOnlyRootFilesystem, drop ALL caps, runAsNonRoot |
| Least privilege (K8s MCP Server) | `mcp-operator` SA with scoped RBAC; no cluster-admin |
| Network policy | MCP Gateway accessible only from kubernaut-system namespace |
| RBAC (FMC) | Read-only on MCPRoute / Backend CRDs |
| RBAC (K8s MCP Server) | Read verbs for investigation/sync, write verbs for remediation; gateway OPA gates which callers can invoke write tools |
| No credential leakage | Kubernaut services hold gateway OAuth2 credentials only; no per-cluster SA tokens or API keys |

## FedRAMP Implications

| Control | Impact |
|---------|--------|
| AC-3 (Access enforcement) | Gateway CEL authorization rules enforce `mcp-read` vs `mcp-write` per service identity; GW, KA, RO, SP, AF, EM, FMC get read-only; only WE can invoke write tools |
| AC-6 (Least privilege) | K8s MCP Server SA has scoped RBAC (not cluster-admin); WE write access limited to remediation-relevant resource types; gateway `toolSelector` restricts exposed tools per backend |
| AU-3 (Audit content) | Cluster provenance recorded per-RR; MCP Gateway logs all tool calls with caller identity |
| SI-4 (Monitoring) | Cross-cluster correlation via ClusterID |
| SC-7 (Boundary protection) | MCP Gateway as single chokepoint for all remote cluster access (read and write) |
| IA-5 (Authenticator management) | Hot-reloadable credentials, bounded token lifetime |
| SC-8 (Transmission confidentiality) | OAuth2 + TLS for all MCP connections |

## SP Remote Enrichment (BR-INTEGRATION-054)

### Overview

Signal Processing (SP) enriches signals with Kubernetes context metadata (labels,
annotations, owner chain). When a signal originates from a remote cluster (non-empty
`ClusterID` on `SignalData`), SP reads the resource metadata from the remote cluster
via the MCP Gateway instead of the local K8s API.

### Architecture

```
SignalData.ClusterID != "" ?
    ├── YES → enrichRemote() → ReaderFactory.ReaderFor(clusterID)
    │                              → mcpclient.NewFromSession(session, clusterID)
    │                                  → MCP Gateway → K8s MCP Server → remote K8s API
    │
    └── NO → existing local enrichment (enrichPodSignal, enrichDeploymentSignal, etc.)
```

### SP Fleet Configuration

Add to the SP controller's `config.yaml`:

```yaml
fleet:
  endpoint: "http://envoy-ai-gateway.kubernaut-system.svc:8080"  # MCP Gateway (Envoy AI Gateway) URL
  oauth2:
    enabled: true
    tokenURL: "https://keycloak.example.com/realms/kubernaut/protocol/openid-connect/token"
    credentialsSecretRef: "sp-fleet-oauth2"  # Secret with client_id + client_secret
```

When `fleet.endpoint` is empty (default), SP operates in local-only mode with zero
overhead — no MCP connection is attempted.

### Degraded Mode Behavior

SP gracefully degrades when remote enrichment fails:

| Failure | Behavior |
|---------|----------|
| MCP Gateway unreachable at boot | `fleetErr` logged, remote enrichment disabled, local enrichment works normally |
| ReaderFactory.ReaderFor() fails | `DegradedMode=true` on KubernetesContext, enrichment continues |
| Remote resource not found | `DegradedMode=true`, namespace context still populated if available |
| MCP session drops mid-request | ResilientClient auto-reconnects on next call |

## Per-Cluster Authorization: BYO (Bring Your Own)

### Scope

Per-cluster authorization — controlling which Kubernaut service can access which
managed cluster — is a **deployment-time configuration** responsibility, not a Kubernaut
code concern. Kubernaut authenticates to the MCP Gateway via OAuth2 (Boundary 1). What
happens at the gateway's authorization layer (Boundary 2) is owned by the platform team.

### Configuration Requirements

Platform teams deploying Kubernaut with multi-cluster federation MUST configure:

1. **Keycloak client scope mapper** — Add a `kubernaut_allowed_clusters` claim to the
   Keycloak client used by Kubernaut services. This claim carries the list of clusters
   each service identity is authorized to access.

2. **CEL authorization rules on MCPRoute** — Extend the `MCPRoute.spec.securityPolicy.authorization`
   to gate tool calls by cluster. Envoy AI Gateway exposes `request.mcp.backend` (the backend
   name, which maps to the cluster ID), enabling direct matching without parsing tool name prefixes.

### Example CEL Authorization Policy (Reference Only)

```yaml
# This is a reference example. Kubernaut does NOT ship or manage this policy.
# Platform teams configure it on the MCPRoute's securityPolicy.authorization.
apiVersion: aigateway.envoyproxy.io/v1beta1
kind: MCPRoute
metadata:
  name: kubernaut-fleet
spec:
  securityPolicy:
    oauth:
      issuer: "https://keycloak.example.com/realms/kubernaut"
      audiences:
        - "https://mcp-gateway.kubernaut-system.svc"
    authorization:
      defaultAction: Deny
      rules:
        # WE (mcp-write scope) can call write tools on allowed clusters
        - source:
            jwt:
              scopes: ["mcp-write"]
          cel: request.mcp.backend in request.auth.jwt.claims.kubernaut_allowed_clusters
        # All other services (mcp-read scope) can call read tools on allowed clusters
        - source:
            jwt:
              scopes: ["mcp-read"]
          target:
            tools:
              - tool: "resources_get"
              - tool: "resources_list"
          cel: request.mcp.backend in request.auth.jwt.claims.kubernaut_allowed_clusters
```

### Why BYO

- Authorization policies vary widely between organizations (different IdPs, different
  cluster naming conventions, different CEL/policy rule structures)
- Kubernaut is a read-only consumer of the MCP Gateway — it does not own the gateway's
  authorization configuration
- The Keycloak claim and CEL authorization rules are standard Envoy AI Gateway patterns,
  not Kubernaut-specific code

## Phase 3: WE MCP Executor (Remote Workflow Execution)

### Overview

The WorkflowExecution (WE) controller gains the ability to create Jobs and
Tekton PipelineRuns on remote clusters by routing K8s write operations through
the MCP Gateway. This extends the read-only MCP client (`mcpclient.Client`)
with a separate `WriterClient` type and introduces a `ClientFactory` pattern
for transparent local/remote routing.

### CRD Change: Spec.ClusterID

`WorkflowExecutionSpec` gains an optional `ClusterID` field:

```yaml
spec:
  clusterID: "prod-east-1"    # empty = local hub cluster
  executionEngine: "job"
  workflowRef:
    workflowID: "restart-pod"
    executionBundle: "quay.io/kubernaut/workflow-restart:v1"
```

When `ClusterID` is empty (default), execution runs on the local hub cluster
exactly as before. When set, the executor routes operations to the remote
cluster via MCP Gateway.

### Design: ClientFactory Pattern

The `ClientFactory` interface provides `ExecutorClient` instances based on
cluster ID:

```go
type ClientFactory interface {
    ClientFor(ctx context.Context, clusterID string) (ExecutorClient, error)
}
```

Two implementations exist:

- **localClientFactory**: Always returns the injected controller-runtime client.
  Used in non-fleet deployments.
- **mcpClientFactory**: Returns the local client for empty ClusterID; returns a
  composite `remoteClient` (MCP Reader + MCP WriterClient) for non-empty IDs.

### Design: WriterClient Isolation

Write operations to remote clusters use `mcpclient.WriterClient`, a dedicated
type that is intentionally separate from `mcpclient.Client` (read-only). This
prevents read-only consumers (SP ReaderFactory, FMC) from gaining accidental
write access through interface widening.

`WriterClient` implements a narrow `ResourceWriter` interface (Create, Delete,
Update) rather than the full `client.Writer` interface, since Patch, DeleteAllOf,
and Apply are not supported via MCP tool calls.

### Executor Refactoring

Both `JobExecutor` and `TektonExecutor` were refactored:

- Internal field changed from `client.Client` to `ClientFactory`
- Each method calls `factory.ClientFor(ctx, wfe.Spec.ClusterID)` to get the
  appropriate client before performing K8s operations
- Backward-compatible constructors preserved: `NewJobExecutor(client.Client)`
  wraps the client in a `localClientFactory` internally
- New constructors added: `NewJobExecutorWithFactory(ClientFactory)` and
  `NewTektonExecutorWithFactory(ClientFactory)` for fleet-enabled deployments

### Wiring in cmd/workflowexecution/main.go

```
if cfg.Fleet.Endpoint != "" {
    fleetResilientClient = connect to MCP Gateway
    clientFactory = NewMCPClientFactory(mgr.GetClient(), session)
} else {
    clientFactory = NewLocalClientFactory(mgr.GetClient())
}

executorRegistry.Register("job", NewJobExecutorWithFactory(clientFactory))
executorRegistry.Register("tekton", NewTektonExecutorWithFactory(clientFactory))
```

### MCP Tool Mapping

Tool names use a gateway-specific prefix convention:
- **EAIGW**: `{backendName}__{toolName}` — `backendName` is the `Backend` CR name
- **Kuadrant**: `{prefix}{toolName}` — `prefix` from `MCPServerRegistration.spec.prefix`

The `GatewayDiscoverer` (decision #11) resolves tool names per gateway. Services
never construct prefixed tool names directly — they receive fully qualified names
from `ToolsForCluster()`. Constants for base tool names are defined in
`pkg/fleet/mcpclient/tool_names.go`.

| Operation | MCP Tool Name | Arguments |
|-----------|---------------|-----------|
| Get       | `{clusterID}__resources_get` | `apiVersion`, `kind`, `name`, `namespace` |
| List      | `{clusterID}__resources_list` | `apiVersion`, `kind`, `namespace` |
| Create/Update | `{clusterID}__resources_create_or_update` | `manifest` (JSON string) |
| Delete    | `{clusterID}__resources_delete` | `apiVersion`, `kind`, `name`, `namespace` |

**Note**: `apiVersion` is a mandatory parameter for all K8s MCP Server tool calls. The
`mcpclient` extracts it from the `GroupVersionKind` set on the `client.Object` before
calling. The mock gateway validates its presence.

**Note**: kube-mcp-server MUST be deployed with `--list-output=yaml` so that
`resources_list` returns full K8s objects in `structuredContent`. See Decision #12.

### Backward Compatibility

- Empty `ClusterID` (default) preserves pre-fleet behavior exactly
- Existing tests pass without modification due to backward-compatible constructors
- No changes to the Executor interface signature
- No changes to the reconciler dispatch logic

### P1 Addendum: Remote Owner Chain and Scope Hierarchy (DD-FLEET-001)

**BackendInformerRegistry**: The `CRDWatcher` component was renamed to
`BackendInformerRegistry` to better reflect its role as a Kubernetes informer-based
registry for Backend CRDs. All struct names, constructors, config types, file names,
and references were updated.

**Shared across services**: `BackendInformerRegistry` is used by GW, RO, and FMC (not
just FMC) to provide cluster-level scope checks. The `ClusterLookupAdapter` adapts
`ClusterRegistry` to the `ClusterLookup` interface consumed by `FederatedScopeChecker`.

**Remote owner chain resolution**: The Gateway's `PrometheusAdapter` gains an optional
`readerFactory` for remote owner chain resolution. When a signal carries a non-empty
`clusterID` and a `ReaderFactory` is configured, the adapter constructs a
`K8sOwnerResolver` backed by a remote `client.Reader` from the MCP Gateway. This
enables consistent Pod-to-Deployment fingerprinting for remote cluster signals.

**FleetConfig extension**: `FleetConfig` now includes `MCPGatewayEndpoint` and `OAuth2`
fields for services that need remote K8s reads via the MCP Gateway.

**Updated Wiring Manifest (P1)**:

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| EAIGWRegistry | NewEAIGWRegistry() | pkg/fleet/registry/ | UT-REG-EAIGW-* |
| ClusterLookupAdapter | NewClusterLookupAdapter() | pkg/fleet/registry/scope_adapter.go | UT-SCOPE-P1-001 |
| SetReaderFactory | prometheusAdapter.SetReaderFactory() | cmd/gateway/main.go | IT-GW-P1-001 |
| resolverForCluster | Parse/ParseBatch dispatch | pkg/gateway/adapters/prometheus_adapter.go | UT-GW-P1-001..007 |
| FleetResilientClient (GW) | fleetclient.NewResilient() | cmd/gateway/main.go | IT-GW-P1-001 |
| FleetResilientClient shutdown | fleetResilientClient.Close() | cmd/gateway/main.go | - |

## References

- Issue #54: Multi-cluster federation
- ADR-064: Multi-Cluster MCP Gateway (deferred direct-connect approach)
- ADR-065: Fleet Cluster Identity on RR
- ADR-067: KA MCP Client Dynamic Tool Discovery
- Spike S6: Fleet Metadata Cache validation
- Spike S8: Real K8s MCP Server with envtest
- Spike S9: ScopeChecker interface redesign
- Spike S10: K8s MCP Server CRD support
- Spike S11: Envoy AI Gateway evaluation (2026-06-25) — validated `__` prefix convention, standalone mode, memory footprint, CEL auth
- Spike S15: Kuadrant tool discovery (2026-06-27) — validated `discover_tools`/`select_tools` response format, `--discovery-tool-threshold` behavior, tool prefix conventions. Designed `GatewayDiscoverer` interface for two-phase LLM-efficient tool discovery.
- Spike S16: structuredContent validation (2026-06-29) — validated kube-mcp-server PR #1232 `structuredContent` against live Kind cluster with Kuadrant MCP Gateway. Established `--list-output=yaml` as hard deployment requirement (Decision #12).

### Backend-Specific Documentation

- **Envoy AI Gateway (MCP)**: [MCP Gateway Documentation](https://aigateway.envoyproxy.io/docs/capabilities/mcp/) — MCPRoute configuration, OAuth, CEL authorization, tool filtering, server multiplexing
- **Kuadrant MCP Gateway Tool Discovery**: [Tool Discovery Guide](https://docs.kuadrant.io/dev/mcp-gateway/docs/guides/tool-discovery/) — `discover_tools`/`select_tools` meta-tools, `--discovery-tool-threshold` configuration, category filtering, session scoping
- **Kuadrant MCPServerRegistration**: [MCPServerRegistration Reference](https://docs.kuadrant.io/dev/mcp-gateway/docs/reference/mcpserverregistration/) — CRD spec including `prefix`, `category`, `hint` fields for tool discovery
- **ACM Search API**: [Red Hat ACM Search Documentation (2.16)](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html-single/search/index) — GraphQL-based search across managed clusters via `search-api` service
- **ACM Search Query API wiki**: [stolostron/search-v2-operator Search Query API](https://github.com/stolostron/search-v2-operator/wiki/Search-Query-API)
- **Rancher v3 API Guide**: [Rancher API Reference](https://ranchermanager.docs.rancher.com/api/v3-rancher-api-guide) — REST API for cluster management, K8s proxy for resource access
- **Clusterpedia**: [Clusterpedia Documentation](https://clusterpedia.io/docs/) — Kubernetes Aggregated API for multi-cluster resource search, compatible with kubectl and client-go
- **Clusterpedia Multi-Cluster Search**: [Multi-Cluster Search](https://clusterpedia.io/docs/usage/search/multi-cluster/) — Label-based filtering, cluster selection, owner chain traversal
