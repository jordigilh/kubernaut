# DD-FLEET-001: Fleet Hierarchical Scope Checking and Remote Owner Chain Resolution

## Status

Accepted

## Context

Issue #54 P1 introduces multi-cluster federation support where signals from remote
clusters arrive at the Gateway via Thanos-federated Prometheus alerts. Two capabilities
are required:

1. **Hierarchical scope checking**: Before checking whether a resource or namespace is
   managed by Kubernaut, we must first verify that the cluster itself is known. This
   prevents unnecessary remote calls to clusters that Kubernaut does not manage.

2. **Remote owner chain resolution**: The Gateway's PrometheusAdapter resolves Pod
   alerts to their owning Deployment/StatefulSet for consistent fingerprinting. For
   remote cluster signals, this resolution must be performed against the remote
   cluster's K8s API via the MCP Gateway.

Additionally, the `ReaderFactory` pattern (which abstracts local vs. remote
`client.Reader` construction) was duplicated across Signal Processing and FMC. It
needed consolidation into `pkg/fleet/` to support the Gateway's new requirement.

## Decisions

### 1. ReaderFactory Consolidation

The `ReaderFactory` interface and `ReaderFactoryFunc` adapter are defined in
`pkg/fleet/reader_factory.go`. The MCP-backed implementation (`NewMCPReaderFactory`)
lives in `pkg/fleet/mcpclient/reader_factory.go` to avoid import cycles.

Consumers:
- **SP**: `K8sEnricher.SetReaderFactory()` for remote enrichment
- **GW**: `PrometheusAdapter.SetReaderFactory()` for remote owner chain resolution
- **FMC**: `Syncer` uses a structurally compatible local interface

### 2. Hierarchical Scope Check (3-Level)

When fleet mode is enabled, `FederatedScopeChecker.IsManagedResource()` performs a
three-level check:

```
Cluster managed? --> Resource managed? --> Namespace managed?
```

The cluster check uses a `ClusterLookup` interface injected via `WithClusterLookup`
option. The production implementation is `BackendInformerRegistry` (via
`ClusterLookupAdapter`), which watches Backend CRDs with the `kubernaut.ai/managed`
label.

If the cluster is not known, the resource is immediately classified as unmanaged
without making any remote calls.

### 3. Remote Owner Chain Resolution

`PrometheusAdapter` gains an optional `readerFactory` field (set via
`SetReaderFactory`). In `Parse`/`ParseBatch`, when `clusterID` is non-empty:

1. Call `readerFactory.ReaderFor(ctx, clusterID)` to obtain a remote `client.Reader`
2. Construct a `K8sOwnerResolver` backed by the remote reader
3. Use it for `ResolveFingerprintWithCluster` instead of the local resolver

Fallback behavior:
- If `readerFactory` is nil, the local resolver is used (backward compatible)
- If `ReaderFor` returns an error, the local resolver is used with a logged warning

### 4. Cluster Identity Convention

The Thanos `cluster` label value must match the Envoy AI Gateway `Backend` CR name.
This is a deployment prerequisite documented in ADR-068. The `mcpclient` uses the
cluster name as a tool prefix (e.g., `prod-east-1__resources_get`).

### 5. MCP Client Fixes

The `mcpclient` was fixed to:
- Send the mandatory `apiVersion` parameter in all tool calls (K8s MCP Server requires it)
- Support typed objects (e.g., `*corev1.Pod`, `*appsv1.Deployment`) via JSON round-trip
  in `populateObject` and `populateListObject`

### 6. BackendInformerRegistry Rename

`CRDWatcher` was renamed to `BackendInformerRegistry` to better reflect its purpose
as a Kubernetes informer-based registry for Backend CRDs. Files, structs, config
types, and constructors were all updated.

## Configuration

Gateway YAML config:

```yaml
fleet:
  enabled: true
  backend: fmc
  endpoint: "http://fmc.kubernaut.svc:8080"
  mcpGatewayEndpoint: "http://eaigw.kubernaut.svc:8080/v1/sse"
  oauth2:
    enabled: true
    tokenURL: "https://dex.kubernaut.svc/token"
    credentialsSecretRef: "fleet-oauth2-credentials"
```

## Wiring Manifest

| Component | Production Entry Point | Wiring Code Location | IT Test ID |
|-----------|----------------------|---------------------|------------|
| SetReaderFactory | prometheusAdapter.SetReaderFactory() | cmd/gateway/main.go | IT-GW-P1-001 |
| resolverForCluster | Parse/ParseBatch dispatch | pkg/gateway/adapters/prometheus_adapter.go | UT-GW-P1-001..007 |
| ClusterLookup | WithClusterLookup option | pkg/fleet/federated_checker.go | UT-SCOPE-P1-001..003 |
| ClusterLookupAdapter | NewClusterLookupAdapter() | pkg/fleet/registry/scope_adapter.go | UT-SCOPE-P1-001 |
| FleetConfig.MCPGatewayEndpoint | serverCfg.Fleet.MCPGatewayEndpoint | cmd/gateway/main.go | IT-GW-P1-001 |
| FleetResilientClient shutdown | fleetResilientClient.Close() | cmd/gateway/main.go | - |

## Consequences

- Gateway can now resolve owner chains for remote cluster signals via MCP
- Scope checking is more efficient: unknown clusters are rejected without remote calls
- ReaderFactory is shared across services, reducing code duplication
- MCP client correctly sends apiVersion, preventing tool call failures
- The FleetConfig type is extended with MCPGatewayEndpoint and OAuth2, which is a
  backward-compatible addition (zero-value = disabled)

## References

- ADR-053: Federated Scope Checking
- ADR-065: Fleet Control Plane Architecture
- ADR-068: Fleet Federation Architecture
- BR-INTEGRATION-065: Multi-cluster signal ingestion
- Issue #54: Fleet E2E Lane
