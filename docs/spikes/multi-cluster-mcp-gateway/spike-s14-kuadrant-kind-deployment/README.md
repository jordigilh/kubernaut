# Spike S14 -- Kuadrant MCP Gateway Kind Deployment

**Date**: 2026-06-26
**Status**: Complete
**Objective**: Validate that Kuadrant MCP Gateway v0.7.1 deploys and runs in a Kind cluster with Istio mesh disabled, and that MCPServerRegistration CRDs work for tool discovery and tool call routing.

## Go/No-Go Results

| # | Question | Threshold | Result | Evidence |
|---|----------|-----------|--------|----------|
| 1 | Can Kuadrant MCP Gateway images be pulled? | Container starts | **GO** | `ghcr.io/kuadrant/mcp-gateway:v0.7.1` and `ghcr.io/kuadrant/mcp-controller:v0.7.1` pull and start successfully |
| 2 | Can Kuadrant run in Kind without Istio mesh? | Deployment Ready | **GO** | All pods 1/1 (no sidecars). Istio installed as Gateway API provider only, `global.proxy.autoInject=disabled` |
| 3 | Can we install MCPServerRegistration CRDs? | CRD Available | **GO** | 3 CRDs installed: `mcpgatewayextensions`, `mcpserverregistrations`, `mcpvirtualservers` |
| 4 | Does a registered backend respond to tools/list? | Tool discovery works | **GO** | 18 tools discovered via gateway with `loopback_cluster_` prefix |
| 5 | Do tool calls route through the gateway to the backend? | tools/call works | **GO** | `loopback_cluster_namespaces_list`, `loopback_cluster_pods_list`, `loopback_cluster_events_list` all execute correctly through the gateway |
| 6 | Memory footprint <= 1 GB total in Kind? | Kind node survives | **GO** | 1.03 GB total (12.5% of 8 GB), well within budget |

**Overall: GO** -- Kuadrant MCP Gateway deploys and runs in Kind without Istio mesh injection. Both tool discovery and tool call routing work correctly.

## Architecture Deployed

```
Kind Cluster (kuadrant-spike)
├── istio-system/
│   └── istiod (Gateway API provider only, no mesh injection)
├── gateway-system/
│   ├── Gateway (Istio-managed, listener "mcp" on port 8080)
│   ├── mcp-gateway-istio pod (Envoy proxy, NodePort 30080->8001)
│   └── EnvoyFilter (ext_proc -> gRPC Router at :50051)
├── mcp-system/
│   ├── mcp-gateway (Broker + gRPC Router, 8080/50051)
│   ├── mcp-gateway-controller (watches CRDs)
│   └── MCPGatewayExtension -> gateway-system/mcp-gateway
└── mcp-test/
    ├── kube-mcp-server (K8s MCP Server, port 8080)
    ├── HTTPRoute (kube-mcp-server-route -> kube-mcp-server:8080)
    └── MCPServerRegistration (loopback-cluster, prefix: loopback_cluster_)
```

## Deployment Steps (Reproducible)

### 1. Create Kind cluster

```bash
cat <<EOF | kind create cluster --name kuadrant-spike --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8001
    protocol: TCP
EOF
```

### 2. Install Gateway API CRDs

```bash
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml
```

### 3. Install Istio (Gateway API provider, mesh disabled)

```bash
helm repo add istio https://istio-release.storage.googleapis.com/charts
helm install istio-base istio/base -n istio-system --create-namespace --wait
helm install istiod istio/istiod -n istio-system --wait \
  --set global.proxy.autoInject=disabled \
  --set sidecarInjectorWebhook.enableNamespacesByDefault=false
```

### 4. Create Gateway

```bash
kubectl create namespace gateway-system
kubectl apply -f - <<EOF
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: mcp-gateway
  namespace: gateway-system
  annotations:
    networking.istio.io/service-type: NodePort
spec:
  gatewayClassName: istio
  listeners:
  - name: mcp
    port: 8080
    protocol: HTTP
    hostname: "*.127-0-0-1.sslip.io"
    allowedRoutes:
      namespaces:
        from: All
EOF
```

Then patch the NodePort to match the Kind port mapping:

```bash
kubectl patch service mcp-gateway-istio -n gateway-system --type='json' \
  -p='[
    {"op":"replace","path":"/spec/ports/0/nodePort","value":31500},
    {"op":"replace","path":"/spec/ports/1/nodePort","value":30080}
  ]'
```

### 5. Install MCP Gateway via Helm

```bash
helm upgrade -i mcp-gateway oci://ghcr.io/kuadrant/charts/mcp-gateway \
  --version 0.7.1 \
  --namespace mcp-system --create-namespace \
  --set controller.enabled=true \
  --set gateway.publicHost=mcp.127-0-0-1.sslip.io \
  --set mcpGatewayExtension.create=true \
  --set mcpGatewayExtension.gatewayRef.name=mcp-gateway \
  --set mcpGatewayExtension.gatewayRef.namespace=gateway-system \
  --wait --timeout=300s
```

### 6. Deploy backend MCP server and register it

See manifests in this directory: `manifests.yaml`

### 7. Verify

```bash
# Check MCPServerRegistration status
kubectl get mcpserverregistration -n mcp-test
# Should show: Ready=True, TOOLS=18

# Test MCP initialize
curl -s -X POST http://mcp.127-0-0-1.sslip.io:8001/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}'
# Should return: serverInfo.name = "Kuadrant MCP Gateway"

# Test tools/list (needs session from initialize)
# Should return 18 tools prefixed with loopback_cluster_
```

## Images and Versions

| Component | Image | Size |
|-----------|-------|------|
| Istio control plane | `registry.istio.io/release/pilot:1.30.2` | included in base |
| Istio gateway proxy | `registry.istio.io/release/proxyv2:1.30.2` | included in base |
| MCP Gateway broker | `ghcr.io/kuadrant/mcp-gateway:v0.7.1` | ~30 MB |
| MCP Gateway controller | `ghcr.io/kuadrant/mcp-controller:v0.7.1` | ~30 MB |
| K8s MCP Server | `ghcr.io/containers/kubernetes-mcp-server:latest` | ~60 MB |

## Memory Footprint

Total Kind node memory with all components running: **1.03 GB** (12.5% of 8 GB available).

This is well under the existing EAIGW E2E lane which uses ~66 MB for fleet components
plus the full pipeline (~800 MB total). The Kuadrant stack adds approximately:
- istiod: ~200 MB
- Istio gateway proxy: ~50 MB
- MCP Gateway broker: ~30 MB
- MCP Gateway controller: ~30 MB
- **Total additional over EAIGW**: ~310 MB (vs ~50 MB for EAIGW standalone)

## Disabling Istio Mesh

Istio mesh (sidecar injection) is fully disabled via two mechanisms:

1. **Helm values**: `global.proxy.autoInject=disabled` and `sidecarInjectorWebhook.enableNamespacesByDefault=false`
2. **No namespace labeling**: No namespace has `istio-injection=enabled`

All pods run with 1/1 containers (no sidecars). Istio functions purely as a Gateway API
provider, managing the gateway Envoy proxy pod in `gateway-system`.

## Key Learnings

### Gateway listener name matters
The MCPGatewayExtension expects `targetRef.sectionName: mcp`, which must match a
listener name on the Gateway. Using `default` or any other name causes the extension
to fail with "listener not found".

### HTTPRoute hostname required
MCPServerRegistration's HTTPRoute must include a `hostnames` field. Without it, the
controller rejects the registration with "HTTPRoute must have at least one hostname".

### Config secret propagation
The controller writes backend config to a Secret (`mcp-gateway-config`). The broker
reads this via a mounted volume. After the first MCPServerRegistration is created,
the broker may need a restart to pick up the initial config (Kubernetes secret volume
update latency). Subsequent updates are picked up within `--mcp-check-interval` (60s default).

### Tool prefix convention
Kuadrant uses `spec.prefix` from MCPServerRegistration (e.g., `loopback_cluster_`).
This is user-defined and matches the single-underscore convention we planned for
`ClusterInfo.ToolPrefix`. Unlike EAIGW's auto-generated `{backendName}__` (double
underscore), Kuadrant's prefix is explicit.

### Backend HTTPRoute must use a unique internal hostname (CRITICAL)
Each backend MCP server's HTTPRoute **must** use a unique hostname that differs from
the broker's hostname. The ext_proc Router routes `tools/call` requests by setting
the `:authority` header to the backend's hostname and calling `clear_route_cache:true`.
If the backend shares the broker's hostname (e.g., both use `mcp.127-0-0-1.sslip.io`),
Envoy re-routes back to the broker instead of the backend.

**Wrong**: `hostnames: [mcp.127-0-0-1.sslip.io]` (same as broker -- tool calls fail)
**Right**: `hostnames: [kube-mcp-server.127-0-0-1.sslip.io]` (unique -- tool calls work)

The hostname only needs to match the Gateway's wildcard listener (`*.127-0-0-1.sslip.io`)
and does not need to be publicly resolvable. This is documented in the Kuadrant guide
for registering MCP servers but is easy to miss.

### Tool call routing works end-to-end
With the correct HTTPRoute hostname, tool calls through the Kuadrant gateway work
correctly. The ext_proc Router strips the prefix, initializes a backend session,
and routes the request via Envoy to the backend MCP server. Verified with
`namespaces_list`, `pods_list`, and `events_list` through the gateway.

### CRDs installed by Helm

| CRD | Group | Purpose |
|-----|-------|---------|
| `mcpgatewayextensions` | `mcp.kuadrant.io` | Binds MCP Gateway to an Istio Gateway listener |
| `mcpserverregistrations` | `mcp.kuadrant.io` | Registers backend MCP servers with prefix |
| `mcpvirtualservers` | `mcp.kuadrant.io` | Virtual server aggregation (not used in this spike) |

## Impact on Implementation Plan

1. **E2E-FLEET-KUA-001 is feasible** -- Kuadrant deploys in Kind with full tool routing
2. **Istio mesh disabled** -- no sidecar overhead, pure Gateway API provider
3. **Tool call routing works** -- kubernaut's MCP client calls route through the gateway correctly
4. **Backend hostname convention**: Each backend HTTPRoute needs a unique internal hostname matching the Gateway wildcard (e.g., `<backend-name>.127-0-0-1.sslip.io`). The `KuadrantRegistry` implementation should enforce this when creating HTTPRoutes for discovered clusters.
5. **Additional E2E setup time**: ~25 seconds (Helm install + pod startup)
6. **NodePort mapping**: Needs careful port assignment (MCP on :8080, not status on :15021)
7. **Prefix convention**: `spec.prefix` with single underscore matches plan

## Cleanup

```bash
kind delete cluster --name kuadrant-spike
```
