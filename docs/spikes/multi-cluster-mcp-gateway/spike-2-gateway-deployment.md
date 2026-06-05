# Spike 2: MCP Gateway Deployment + Registration

**Date**: 2026-06-04
**Status**: Complete (manifests produced; cluster validation deferred to lab environment)
**Objective**: Deploy the MCP Gateway and register an OCP MCP server through it.

## Deployment Architecture

```
Management Cluster (OpenShift)
├── gateway-system/
│   └── Gateway (Istio-based, Gateway API)
│       ├── Listener: http (80)
│       ├── Listener: mcps (8080, hostname: mcp.kubernaut.example.com)
│       └── Listener: https (443, TLS)
├── kubernaut-mcp/
│   ├── MCPGatewayExtension -> Gateway/mcps listener
│   ├── MCP Broker/Router (auto-created by controller)
│   ├── HTTPRoute (cluster-a) -> OCP MCP Server (in-cluster or remote)
│   ├── MCPServerRegistration (cluster-a, prefix: "cluster_a_")
│   ├── HTTPRoute (cluster-b) -> External OCP MCP Server
│   └── MCPServerRegistration (cluster-b, prefix: "cluster_b_")
└── kubernaut-system/
    └── KA (connects to gateway as MCP client)

Workload Cluster A
└── kubernaut-mcp/
    └── OCP MCP Server (port 9090, read-only, stateless, core+config toolsets)

Workload Cluster B
└── kubernaut-mcp/
    └── OCP MCP Server (port 9090, read-only, stateless, core+config toolsets)
```

## Prerequisites

1. **OpenShift 4.x** with Gateway API support (Istio-based)
2. **Red Hat Connectivity Link 1.3** (provides MCP Gateway operator) OR
   upstream **Kuadrant MCP Gateway v0.6.0** (Helm install)
3. **Istio** as Gateway API provider
4. **Network connectivity** between management cluster and workload clusters

## Installation Options

### Option A: Red Hat Connectivity Link (OLM)

For OCP clusters with Connectivity Link subscription:

```bash
oc create ns kubernaut-mcp
oc apply -n kubernaut-mcp -f - <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: mcp-gateway
spec:
  source: redhat-operators
  sourceNamespace: openshift-marketplace
  name: mcp-gateway
  channel: preview
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: mcp-gateway
spec:
  targetNamespaces:
  - kubernaut-mcp
EOF
```

### Option B: Upstream Kuadrant (Helm)

For non-OCP or development environments:

```bash
export MCP_GATEWAY_VERSION=0.6.0
kubectl apply -k "https://github.com/kuadrant/mcp-gateway/config/crd?ref=v${MCP_GATEWAY_VERSION}"

helm upgrade -i mcp-gateway oci://ghcr.io/kuadrant/charts/mcp-gateway \
  --version ${MCP_GATEWAY_VERSION} \
  --namespace kubernaut-mcp \
  --create-namespace \
  --set controller.enabled=true \
  --set gateway.publicHost=mcp.kubernaut.example.com \
  --set mcpGatewayExtension.create=true \
  --set mcpGatewayExtension.gatewayRef.name=mcp-gateway \
  --set mcpGatewayExtension.gatewayRef.namespace=gateway-system
```

### Option C: Kind Quick Start (Local Development)

```bash
export MCP_GATEWAY_VERSION=0.6.0
curl -sSL https://raw.githubusercontent.com/Kuadrant/mcp-gateway/main/scripts/quick-start.sh | bash
```

## Deployment Steps

### Step 1: Deploy OCP MCP Server on each workload cluster

Apply `manifests/01-namespace.yaml` and `manifests/02-ocp-mcp-server.yaml` on each workload cluster.

```bash
kubectl apply -f manifests/01-namespace.yaml
kubectl apply -f manifests/02-ocp-mcp-server.yaml
kubectl -n kubernaut-mcp wait --for=condition=Available deployment/ocp-mcp-server --timeout=60s
```

### Step 2: Register each cluster with the MCP Gateway

On the management cluster, apply `manifests/03-mcp-gateway-registration.yaml` (one per cluster).

For external clusters, use `manifests/04-cross-cluster-example.yaml` pattern with ServiceEntry.

### Step 3: Verify registration

```bash
kubectl -n kubernaut-mcp get mcpserverregistration
kubectl -n kubernaut-mcp get mcpserverregistration cluster-a -o jsonpath='{.status.conditions}'
```

### Step 4: Test tool call through gateway

Connect with MCP Inspector or Go client:

```bash
npx @anthropic-ai/mcp-inspector --url http://mcp.kubernaut.example.com:8080/mcp
```

Expected: tools appear with cluster prefixes (e.g., `cluster_a_pods_list`, `cluster_a_resources_get`).

## Manifests Produced

| File | Purpose |
|------|---------|
| `manifests/01-namespace.yaml` | Namespace for MCP components |
| `manifests/02-ocp-mcp-server.yaml` | OCP MCP Server deployment (per workload cluster) |
| `manifests/03-mcp-gateway-registration.yaml` | In-cluster MCP server registration (management cluster) |
| `manifests/04-cross-cluster-example.yaml` | Cross-cluster MCP server registration (external clusters) |

## Key Findings

### Tool Prefix Strategy

The `MCPServerRegistration.spec.prefix` field is critical for multi-cluster routing:
- Each cluster gets a unique prefix: `cluster_a_`, `cluster_b_`
- Tools appear as `cluster_a_pods_list`, `cluster_b_resources_get`
- KA maps CMDB CI -> cluster prefix to target the correct cluster

### Authentication Options

1. **No auth** (POC): Gateway allows unauthenticated access from within the mesh
2. **API Key** (Simple): AuthPolicy with API key validation per client
3. **OIDC** (Production): AuthPolicy with Authorino for OIDC token validation

For POC, option 1 (no auth) is sufficient since KA runs in the same mesh.

### Known Issues

- **MCP Gateway v0.6.0**: When using OAuth, `spec.httpRouteManagement` must be set to `Disabled` on the MCPGatewayExtension and a custom HTTPRoute must be created manually.
- **Technology Preview**: MCP Gateway is not GA. Expect API changes.

### Latency Expectations

Based on the architecture (Envoy proxy + MCP broker/router):
- **Additional hops**: 2 (Gateway -> Broker -> Backend MCP Server)
- **Expected overhead**: 10-50ms per tool call (Envoy is <1ms, MCP broker adds SSE session management)
- **Acceptable for investigation**: Yes (investigation tool calls are sequential, not latency-critical)

Actual measurements will be taken during lab validation.

## Decision

**GO**: MCP Gateway deployment is straightforward with Helm or OLM. Manifests are ready for lab validation. The tool prefix strategy provides clean cluster routing.

## Next Steps

1. Deploy to lab environment (Kind or OCP dev cluster)
2. Measure actual latency
3. Test with MCP Inspector
4. Proceed to Spike 3 (KA as MCP client)
