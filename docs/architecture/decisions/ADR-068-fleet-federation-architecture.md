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
| **FMC (Fleet Metadata Cache)** | Polls MCP Gateway for `kubernaut.ai/managed=true` resources, caches metadata in Valkey, exposes scope queries via REST API | `cmd/fmc/`, `pkg/fleet/fmc/` |
| **Fleet Metadata Cache (Valkey)** | Low-latency key-existence checks for remote scope validation | `pkg/fleet/scopecache/` |
| **FederatedScopeChecker** | Routes scope checks: local → K8s API, remote → Valkey cache | `pkg/fleet/scopecache/federated_checker.go` |
| **ResilientClient** | MCP client with backoff, reconnect, readiness gating | `pkg/fleet/mcpclient/resilience.go` |
| **ReloadableOAuth2Transport** | Hot-reloadable OAuth2 credentials via FileWatcher | `pkg/fleet/mcpclient/reloadable_auth.go` |
| **CRDWatcher** | Discovers clusters from `MCPServerRegistration` CRDs | `pkg/fleet/registry/` |
| **KA Fleet Tools** | Dynamic BridgeTool registration from MCP Gateway `tools/list` | `cmd/kubernautagent/main.go` |

### Key Design Decisions

1. **Federated Control Plane interface** (adapter pattern): GW and RO depend on a `FederatedControlPlane` interface, not on any specific storage backend. The adapter is selected at startup based on the environment (FMC, ACM, Rancher, Clusterpedia). This decouples Kubernaut from any single fleet management platform and allows swapping backends without code changes in GW/RO.

2. **FMC as default adapter** (for environments without a federated control plane): FMC Writer polls MCP Gateway, writes to Valkey (co-owned with DataStorage), and exposes a scope query API. Achieves p95 < 1ms. Rancher/ACM/Clusterpedia shops skip FMC entirely and use their native APIs.

3. **FMC Writer as dedicated service** (not sidecar): Keeps GW stateless. Separation of concerns between signal processing and metadata sync.

4. **ReloadableOAuth2Transport** (not static secrets): Supports zero-downtime credential rotation without pod restart. Uses existing `hotreload.FileWatcher` pattern.

5. **ResilientClient with backoff** (not fail-fast): MCP Gateway may not be ready at boot. Exponential backoff + readiness gate prevents cascading failures.

6. **ClusterID on fingerprint** (not label): Fingerprints drive deduplication. Including ClusterID ensures same-resource-different-cluster alerts create separate RRs.

7. **KA dynamic tool discovery** (not hard-coded): Fleet tools are registered at startup from `tools/list`. `AppendFleetToolsToRCA` makes them visible to the LLM investigator without code changes per cluster.

8. **`MCPServerRegistration` as source of truth** (not ConfigMap): Kuadrant CRD is the authoritative registry of MCP backends. Kubernaut is a **read-only consumer** — it watches but never creates or modifies these CRDs. The control plane (ACM, Rancher, GitOps) owns their lifecycle.

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
- **Rejected**: MCPServerRegistration CRD is the authoritative and universal contract

### E. Native control-plane adapters (ACM ManagedCluster watcher, Rancher adapter)
- +: Direct integration, potentially lower latency for cluster discovery
- -: Kubernaut couples to specific control plane; N adapters to maintain; same info available via MCPServerRegistration
- **Rejected**: MCPServerRegistration is the generic interface. Control planes own the creation of these CRDs.

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
| RemoteScopeResolver interface | FederatedScopeChecker | pkg/fleet/scopecache/resolver.go | UT-FLEET-FC-003/004/005 |
| ResilientClient (KA) | registerFleetTools() | cmd/kubernautagent/main.go | IT-KA-FLEET-001 |
| ResilientClient (FMC) | main() | cmd/fmc/main.go | UT-FMC-001 |
| ReloadableOAuth2Transport (KA) | registerFleetTools() | cmd/kubernautagent/main.go | UT-FLEET-RES-001 |
| ReloadableOAuth2Transport (FMC) | main() | cmd/fmc/main.go | UT-FLEET-RES-001 |
| AppendFleetToolsToRCA | registerFleetTools() → main | cmd/kubernautagent/main.go:231 | IT-KA-FLEET-001 |
| CRDWatcher | main() | cmd/fmc/main.go | UT-FMC-004 |
| BuildKey validation | BuildKey() | pkg/fleet/scopecache/client.go | UT-FLEET-SC-006/007/008 |
| fmc securityContext | Helm template | charts/kubernaut/templates/fmc/fmc.yaml | helm template |

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

The unified `ScopeChecker` interface replaces the current multi-interface stack
(`scope.ScopeChecker`, `scope.FederatedScopeChecker`, `RemoteScopeResolver`, `IsManagedTyped`)
with a single method accepting a `ResourceIdentity` struct:

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

**What this replaces:**

| Removed | Problem |
|---------|---------|
| `scope.ScopeChecker.IsManaged(ctx, ns, kind, name)` | No clusterID, no GVK |
| `scope.FederatedScopeChecker.IsManagedOnCluster(ctx, clusterID, ns, kind, name)` | Separate method forces conditionals at every call site |
| `RemoteScopeResolver.IsManaged(ctx, clusterID, group, version, kind, ns, name)` | 7 positional strings |
| `Client.IsManagedTyped(ctx, clusterID, gvk, key)` | Workaround wrapper |

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
| **Authentication** | GW/RO → FMC: in-cluster Service access (no auth required if same namespace, or mTLS if cross-namespace). FMC → Valkey: password-based or mTLS. FMC → MCP Gateway: OAuth2. |
| **Network** | GW/RO pods must reach FMC Service on port 8080. FMC must reach Valkey (6379) and MCP Gateway. If NetworkPolicies are enforced, add egress rules accordingly. |
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
| **Auth** | Service account token or OAuth2 bearer token |

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
| **TLS** | The `search-search-api` Service uses TLS. Kubernaut's HTTP client must trust the serving CA. For in-cluster access, the cluster CA is sufficient. For cross-cluster, mount the ACM hub's CA bundle. |
| **Authentication** | ACM Search enforces K8s authentication. Kubernaut needs a bearer token from a ServiceAccount on the ACM hub. The token is passed as `Authorization: Bearer <token>` header. |
| **ServiceAccount (hub)** | Create a dedicated SA (e.g., `kubernaut-fleet-reader`) in the `kubernaut-system` namespace on the ACM hub. If Kubernaut runs on the same cluster as ACM, the SA token is auto-mounted. If cross-cluster, create a long-lived token and mount it as a Secret. |
| **RBAC (hub)** | ACM Search respects K8s RBAC — it only returns resources the authenticated user can `list`. The SA needs a ClusterRole granting `list` on all resource types that Kubernaut manages (Pods, Deployments, StatefulSets, DaemonSets, Nodes, Services, etc.) across all namespaces and clusters. |
| **Network policy** | Kubernaut pods (GW, RO) must be able to reach the `search-search-api` Service on port 4010. If NetworkPolicies are enforced, add an egress rule from `kubernaut-system` to `open-cluster-management` namespace. |
| **Kubernaut config** | `fleet.backend: "acm"`, `fleet.acm.endpoint: "https://search-search-api.open-cluster-management.svc:4010"`. If cross-cluster: `fleet.acm.tokenSecretRef: "acm-search-token"`, `fleet.acm.caBundle: "/etc/kubernaut/acm-ca/ca.crt"`. |

**RBAC example (ACM hub)**:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-fleet-reader
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kubernaut-fleet-search-reader
rules:
  - apiGroups: [""]
    resources: ["pods", "services", "nodes", "namespaces", "configmaps", "secrets"]
    verbs: ["list"]
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets", "replicasets"]
    verbs: ["list"]
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["list"]
  - apiGroups: ["cluster.open-cluster-management.io"]
    resources: ["managedclusters"]
    verbs: ["list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kubernaut-fleet-search-reader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kubernaut-fleet-search-reader
subjects:
  - kind: ServiceAccount
    name: kubernaut-fleet-reader
    namespace: kubernaut-system
```

**Cross-cluster token (if Kubernaut is not on the ACM hub)**:

```bash
# On the ACM hub cluster:
oc create token kubernaut-fleet-reader -n kubernaut-system --duration=8760h
# Store the token as a Secret on the Kubernaut cluster and reference it in fleet.acm.tokenSecretRef
```

**ACM Search schema verification**: Filter properties (`kind`, `name`, `namespace`, `cluster`,
`label`) are confirmed in the ACM Search schema (`searchSchema` query returns `allProperties`).
Label values use `key=value` format per the
[Search Query API spec](https://github.com/stolostron/search-v2-operator/wiki/Search-Query-API).

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

The current implementation has GW/RO directly constructing `scopecache.NewFederatedScopeCheckerFromAddr(scopeMgr, cfg.Fleet.ValkeyAddr, logger)` — coupling them to Valkey. The migration:

1. **Phase 1** (current): `ValkeyAddr` in GW/RO config → direct Valkey client in-process
2. **Phase 2**: Define `FederatedControlPlane` interface, implement FMC REST API (HTTP server in `cmd/fmc/`), implement FMC client adapter
3. **Phase 3**: GW/RO switch from direct Valkey to `FederatedControlPlane` interface; `ValkeyAddr` removed from GW/RO config; only `backend` + `endpoint` remain
4. **Phase 4**: Implement ACM/Rancher/Clusterpedia adapters as needed per deployment environment

During migration, both paths can coexist via feature flag (`fleet.backend: "valkey-direct"` for legacy, `"fmc"` for new).

## Security Considerations

| Control | Implementation |
|---------|---------------|
| Authentication | OAuth2 client credentials grant (file-mounted JWT) |
| Credential rotation | ReloadableOAuth2Transport with FileWatcher |
| Token timeout | 10s context deadline on token refresh HTTP calls |
| Least privilege | fmc: readOnlyRootFilesystem, drop ALL caps, runAsNonRoot |
| Network policy | MCP Gateway accessible only from kubernaut-system namespace |
| RBAC | fmc: read-only on MCPServerRegistration CRDs |

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

### Backend-Specific Documentation

- **ACM Search API**: [Red Hat ACM Search Documentation (2.16)](https://docs.redhat.com/en/documentation/red_hat_advanced_cluster_management_for_kubernetes/2.16/html-single/search/index) — GraphQL-based search across managed clusters via `search-api` service
- **ACM Search Query API wiki**: [stolostron/search-v2-operator Search Query API](https://github.com/stolostron/search-v2-operator/wiki/Search-Query-API)
- **Rancher v3 API Guide**: [Rancher API Reference](https://ranchermanager.docs.rancher.com/api/v3-rancher-api-guide) — REST API for cluster management, K8s proxy for resource access
- **Clusterpedia**: [Clusterpedia Documentation](https://clusterpedia.io/docs/) — Kubernetes Aggregated API for multi-cluster resource search, compatible with kubectl and client-go
- **Clusterpedia Multi-Cluster Search**: [Multi-Cluster Search](https://clusterpedia.io/docs/usage/search/multi-cluster/) — Label-based filtering, cluster selection, owner chain traversal
