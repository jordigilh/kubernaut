# Local Dev Setup: MCP Gateway + kube-mcp-server + Keycloak

**Status**: Pseudo installation guide (developer productivity aid, not a formal runbook)
**Audience**: Contributors who want to stand up Fleet's multi-cluster MCP discovery
stack on a laptop to test Fleet Metadata Cache (FMC) journeys interactively.
**Authority**: Issue #54, [ADR-068](../../architecture/decisions/ADR-068-fleet-federation-architecture.md),
Spike S17/S18 (`docs/spikes/multi-cluster-mcp-gateway/`).

---

## What you're building

```
┌─────────────┐   client_credentials    ┌───────────┐
│     FMC     │────────────────────────▶│ Keycloak  │  OIDC IdP + RFC 8693
│ (metadata   │◀────── access_token ────│ (realm:   │  token exchange
│  cache)     │                         │ kubernaut-│
└──────┬──────┘                         │  fleet)   │
       │ Bearer <token>                 └─────▲─────┘
       ▼                                       │ token
┌─────────────────────┐   tools/list    ┌──────┴──────┐  exchange
│  MCP Gateway         │────────────────▶│ kube-mcp-   │  (passthrough
│  (Kuadrant broker OR │◀───────────────│  server     │   + STS mode)
│  Envoy AI Gateway)   │  tool results   │ (passthrough│
└──────────────────────┘                 │  + STS)     │
                                          └──────┬──────┘
                                                 ▼
                                          Kubernetes API
                                          (this same Kind
                                           cluster, "loopback")
```

Two interchangeable gateway options sit in the same slot:

| Gateway | Status | Broker component | Registration CRD |
|---|---|---|---|
| **Kuadrant MCP Gateway** | Fully automated (`make test-e2e-fleetmetadatacache-kuadrant`) | `mcp-gateway-controller` + `mcp-gateway` (broker) Deployments | `MCPServerRegistration` (`mcp.kuadrant.io/v1alpha1`) |
| **Envoy AI Gateway (EAIGW)** | Fully automated (`make test-e2e-fleetmetadatacache-eaigw`, Spike S18) | None — `MCPRoute.spec.backendRefs` aggregates natively | `Backend` (`gateway.envoyproxy.io/v1alpha1`) + `MCPRoute` (`aigateway.envoyproxy.io/v1beta1`) |

Both variants reuse the **same Keycloak realm** and the **same kube-mcp-server
passthrough+STS token-exchange config** — the RFC 8693 exchange lives inside
kube-mcp-server itself, not in the gateway (see "Key design decision" in the
EAIGW plan doc). Only the edge auth/routing layer differs.

---

## Prerequisites

- `kind`, `kubectl`, `helm`, `podman` (this repo's E2E harness uses podman as
  the Kind provider — set `KIND_EXPERIMENTAL_PROVIDER=podman`)
- `openssl` (for the self-signed Keycloak TLS cert, EAIGW path only)
- `curl` and `python3` (response inspection)
- ~2.5 GB free RAM for the cluster

---

## Path A (recommended): Use the existing automation

Both gateway variants are fully wired into this repo's E2E harness and are
what CI runs. This is the fastest way to get a working stack:

```bash
# Kuadrant variant — runs the full FMC E2E suite AND leaves the Kind cluster running afterward
PRESERVE_E2E_CLUSTER=true make test-e2e-fleetmetadatacache-kuadrant

# Envoy AI Gateway variant — same journeys, EAIGW instead of Kuadrant, separate Kind cluster
PRESERVE_E2E_CLUSTER=true make test-e2e-fleetmetadatacache-eaigw
```

When it finishes (~10 min), the test output prints (Kuadrant example):

```
✅ Preserving cluster for debugging (fmc-e2e)
   To access: export KUBECONFIG=~/.kube/fmc-e2e-config
   To delete: kind delete cluster --name fmc-e2e
```

```bash
export KUBECONFIG=~/.kube/fmc-e2e-config
kubectl get pods -n kubernaut-system
```

You now have a live stack with:

| Endpoint | URL |
|---|---|
| FMC API | `http://localhost:8150` |
| Kuadrant MCP Gateway | `http://localhost:31975/mcp` |
| Keycloak | `https://localhost:30557/realms/kubernaut-fleet` |
| DataStorage | `https://localhost:30081` |

The EAIGW variant is identical except the Kind cluster is named
`fmc-eaigw-e2e` (kubeconfig `~/.kube/fmc-eaigw-e2e-config`) and the gateway
endpoint is `http://localhost:31976/mcp` (NodePort per DD-TEST-001) instead
of the Kuadrant gateway's `31975`.

Skip to [Exercising the stack](#exercising-the-stack) to start poking at it.
To tear it down: `kind delete cluster --name fmc-e2e` (or `fmc-eaigw-e2e`).

This automation lives in `test/infrastructure/fleetmetadatacache_e2e.go`
(`SetupFMCE2EInfrastructure` / `SetupFMCE2EInfrastructureEAIGW`) and
`test/infrastructure/fleet_e2e.go` (`DeployFleetCoreInfra`,
`deployEnvoyAIGatewayInfra`) if you want to read the real source instead of
the condensed steps below.

---

## Path B: Manual walkthrough (Kuadrant variant)

Useful if you want to understand — or tweak — each moving part rather than
running it as a black box.

### B1. Create the Kind cluster

```bash
kind create cluster --name fleet-dev
export KUBECONFIG="$(kind get kubeconfig-path --name=fleet-dev 2>/dev/null || echo ~/.kube/config)"
kubectl create namespace kubernaut-system
```

### B2. Deploy Keycloak with the kubernaut-fleet realm

The realm (`test/infrastructure/keycloak-realm-fleet.json`) pre-declares three
clients — copy it locally or reference the checked-in file:

| Client ID | Secret | Purpose |
|---|---|---|
| `kubernaut-fleet-read` | `e2e-fleet-secret` | FMC's own `client_credentials` identity (the "subject" of the token exchange) |
| `kube-mcp-server` | `e2e-kube-mcp-server-secret` | Requests the RFC 8693 token exchange (`standard.token.exchange.enabled: true`) |
| `k8s-api` | *(bearer-only, no secret)* | The exchange's target audience — what the Kubernetes API server validates |

```bash
kubectl create configmap keycloak-realm-config \
  --from-file=kubernaut-fleet-realm.json=test/infrastructure/keycloak-realm-fleet.json \
  -n kubernaut-system

# Self-signed TLS cert for Keycloak's HTTPS listener
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout /tmp/keycloak-tls.key -out /tmp/keycloak-tls.crt \
  -subj "/CN=keycloak" -addext "subjectAltName=DNS:keycloak"
kubectl create secret tls keycloak-tls \
  --cert=/tmp/keycloak-tls.crt --key=/tmp/keycloak-tls.key \
  -n kubernaut-system

kubectl apply -n kubernaut-system -f - <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: keycloak
spec:
  replicas: 1
  selector:
    matchLabels: {app: keycloak}
  template:
    metadata:
      labels: {app: keycloak}
    spec:
      containers:
      - name: keycloak
        image: quay.io/keycloak/keycloak:26.6.4
        args: ["start-dev", "--import-realm"]
        env:
        - {name: KC_BOOTSTRAP_ADMIN_USERNAME, value: "admin"}
        - {name: KC_BOOTSTRAP_ADMIN_PASSWORD, value: "admin"}
        - {name: KC_HTTPS_CERTIFICATE_FILE, value: /etc/keycloak-tls/tls.crt}
        - {name: KC_HTTPS_CERTIFICATE_KEY_FILE, value: /etc/keycloak-tls/tls.key}
        - {name: KC_HOSTNAME, value: "https://keycloak:8443"}
        - {name: KC_HOSTNAME_STRICT_HTTPS, value: "false"}
        ports: [{name: https, containerPort: 8443}]
        volumeMounts:
        - {name: realm-config, mountPath: /opt/keycloak/data/import, readOnly: true}
        - {name: tls-certs, mountPath: /etc/keycloak-tls, readOnly: true}
        readinessProbe:
          httpGet: {path: /realms/master, port: 8443, scheme: HTTPS}
          initialDelaySeconds: 15
          periodSeconds: 5
          failureThreshold: 24
      volumes:
      - {name: realm-config, configMap: {name: keycloak-realm-config}}
      - {name: tls-certs, secret: {secretName: keycloak-tls}}
---
apiVersion: v1
kind: Service
metadata:
  name: keycloak
spec:
  type: NodePort
  ports: [{name: https, port: 8443, targetPort: 8443, nodePort: 30557}]
  selector: {app: keycloak}
EOF

kubectl rollout status deployment/keycloak -n kubernaut-system --timeout=180s
```

> `start-dev --import-realm` is deliberate: this is throwaway dev infra
> (in-memory H2 storage), so production-mode preflight checks (external DB,
> strict hostname) are skipped for faster startup.

### B3. Enable OIDC on the Kind API server

The Kubernetes API server needs to trust Keycloak-issued (specifically,
**exchanged**, `k8s-api`-audience) tokens:

```bash
NODE=fleet-dev-control-plane

# Copy Keycloak's TLS CA into the node so the API server can verify it during
# OIDC discovery (this walkthrough's self-signed cert IS its own CA — in the
# real E2E harness both Dex and Keycloak share one inter-service CA).
podman cp /tmp/keycloak-tls.crt "$NODE:/etc/kubernetes/pki/oidc-ca.crt"

# Insert OIDC flags into the static API server pod manifest.
podman exec "$NODE" bash -c '
sed -i "/--tls-private-key-file/a\\
    - \"--oidc-username-prefix=keycloak:\"\\
    - --oidc-username-claim=preferred_username\\
    - --oidc-client-id=k8s-api\\
    - --oidc-ca-file=/etc/kubernetes/pki/oidc-ca.crt\\
    - \"--oidc-issuer-url=https://keycloak:8443/realms/kubernaut-fleet\"" \
  /etc/kubernetes/manifests/kube-apiserver.yaml'
```

> **ClientID is `k8s-api`, not `kubernaut-fleet-read`.** The API server
> validates the *exchanged* token's audience (what kube-mcp-server presents
> after RFC 8693 exchange), not the original caller's token audience.
> **UsernameClaim is `preferred_username`**: the `k8s-api-audience`
> client-scope carries a dedicated User Property mapper for it — Keycloak's
> default service-account tokens otherwise carry no `preferred_username`
> claim at all, which is a hard 401 for the API server's OIDC authenticator,
> not merely an authz failure.

The kubelet detects the manifest change and restarts the API server
automatically. **Known gap**: the static pod runs `hostNetwork: true`, which
shares the node's network *namespace* but not its `/etc/hosts` — the API
server cannot resolve the bare in-cluster Service name `keycloak` via CoreDNS
from a host-network pod. Patch the pod's kubelet-managed hosts file directly:

```bash
KC_IP=$(kubectl get svc keycloak -n kubernaut-system -o jsonpath='{.spec.clusterIP}')
podman exec "$NODE" bash -c "
for d in /var/lib/kubelet/pods/*/etc-hosts; do
  grep -q kube-apiserver <(cat \$(dirname \$d)/../containers/*/*.log 2>/dev/null) 2>/dev/null || true
done
echo '$KC_IP keycloak' >> /etc/hosts
"
```
(In the real harness, `patchAPIServerPodHostsForIssuer` locates the exact
per-pod-UID hosts file; appending to the node's own `/etc/hosts` is a
simpler approximation good enough for a hostNetwork static pod.)

Wait for the API server to come back:

```bash
until kubectl get --raw /readyz &>/dev/null; do sleep 3; done
```

Bind the exchanged identity to a role (kube-mcp-server's exchanged token
carries FMC's *original* identity, not its own):

```bash
kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: fmc-exchanged-identity-binding
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: view}
subjects:
- {kind: User, name: "keycloak:service-account-kubernaut-fleet-read", apiGroup: rbac.authorization.k8s.io}
EOF
```

### B4. Deploy kube-mcp-server (passthrough + token exchange)

```bash
kubectl apply -n kubernaut-system -f - <<'EOF'
apiVersion: v1
kind: ServiceAccount
metadata: {name: kube-mcp-server}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: kube-mcp-server-binding}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: view}
subjects: [{kind: ServiceAccount, name: kube-mcp-server, namespace: kubernaut-system}]
---
apiVersion: v1
kind: ConfigMap
metadata: {name: kube-mcp-server-config}
data:
  config.toml: |
    require_oauth = true
    authorization_url = "https://keycloak:8443/realms/kubernaut-fleet"
    oauth_audience = "kube-mcp-server"
    cluster_auth_mode = "passthrough"
    sts_client_id = "kube-mcp-server"
    sts_client_secret = "e2e-kube-mcp-server-secret"
    sts_audience = "k8s-api"
    sts_scopes = ["k8s-api-audience"]
    certificate_authority = "/etc/tls-ca/ca.crt"
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: kube-mcp-server}
spec:
  replicas: 1
  selector: {matchLabels: {app: kube-mcp-server}}
  template:
    metadata: {labels: {app: kube-mcp-server}}
    spec:
      serviceAccountName: kube-mcp-server
      containers:
      - name: kube-mcp-server
        image: ghcr.io/containers/kubernetes-mcp-server:latest
        args:
        - "--port=8080"
        - "--cluster-provider=in-cluster"
        - "--toolsets=core"
        - "--stateless"
        - "--list-output=yaml"
        - "--config=/etc/kubernetes-mcp-server/config.toml"
        ports: [{name: http, containerPort: 8080}]
        volumeMounts:
        - {name: config, mountPath: /etc/kubernetes-mcp-server, readOnly: true}
        - {name: tls-ca, mountPath: /etc/tls-ca, readOnly: true}
        readinessProbe: {httpGet: {path: /healthz, port: 8080}, initialDelaySeconds: 3, periodSeconds: 5}
      volumes:
      - {name: config, configMap: {name: kube-mcp-server-config}}
      - {name: tls-ca, secret: {secretName: keycloak-tls}}
---
apiVersion: v1
kind: Service
metadata: {name: kube-mcp-server}
spec:
  ports: [{name: http, port: 8080, targetPort: 8080}]
  selector: {app: kube-mcp-server}
EOF

kubectl rollout status deployment/kube-mcp-server -n kubernaut-system --timeout=120s
```

> Skipping `--oidc-username-prefix`-flavored `require_oauth` here would make
> kube-mcp-server accept *any* Bearer token uncritically before attempting
> the exchange — `require_oauth = true` + `oauth_audience = "kube-mcp-server"`
> makes it validate the caller's token first (this is what the gateway's
> registration credential, below, authenticates against for its own
> discovery probe).
>
> `sts_scopes` is required even though `k8s-api-audience` is already a
> default scope of the `kube-mcp-server` client: the underlying exchange
> library always sends a `scope` parameter, and Keycloak rejects an
> explicitly empty one with `invalid_scope`.

### B5. Deploy the Kuadrant MCP Gateway

```bash
# Gateway API + Istio (data plane) — see fleet_e2e.go Phase 1 for full detail
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml
helm repo add istio https://istio-release.storage.googleapis.com/charts && helm repo update
kubectl create namespace istio-system
helm template istio-base istio/base -n istio-system --version 1.30.2 | kubectl apply -f -
helm template istiod istio/istiod -n istio-system --version 1.30.2 \
  --set global.proxy.autoInject=disabled \
  --set sidecarInjectorWebhook.enableNamespacesByDefault=false | kubectl apply -f -
kubectl rollout status deployment/istiod -n istio-system --timeout=180s

kubectl create namespace gateway-system
kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: mcp-gateway
  namespace: gateway-system
  annotations: {networking.istio.io/service-type: NodePort}
spec:
  gatewayClassName: istio
  listeners:
  - name: mcp
    port: 8080
    protocol: HTTP
    hostname: "*.127-0-0-1.sslip.io"
    allowedRoutes: {namespaces: {from: All}}
EOF

# Kuadrant controller + broker (creates its own mcp-gateway Deployment)
kubectl apply -k "https://github.com/Kuadrant/mcp-gateway/config/crd?ref=v0.7.1"
kubectl apply -k "https://github.com/Kuadrant/mcp-gateway/config/mcp-gateway/overlays/mcp-system?ref=v0.7.1"

kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata: {name: allow-mcp-extension, namespace: gateway-system}
spec:
  from: [{group: mcp.kuadrant.io, kind: MCPGatewayExtension, namespace: mcp-system}]
  to: [{group: gateway.networking.k8s.io, kind: Gateway}]
EOF

kubectl rollout status deployment/mcp-gateway-controller -n mcp-system --timeout=120s
kubectl rollout status deployment/mcp-gateway -n mcp-system --timeout=120s
```

Register kube-mcp-server as a cluster, plus a static credential the broker
uses for its own upstream discovery connection (separate from per-request
`tools/call` proxying, which forwards the caller's header unmodified):

```bash
BROKER_TOKEN=$(curl -sk -X POST \
  https://localhost:30557/realms/kubernaut-fleet/protocol/openid-connect/token \
  -d grant_type=client_credentials -d client_id=kubernaut-fleet-read \
  -d client_secret=e2e-fleet-secret -d scope=kube-mcp-server-audience \
  | python3 -c 'import json,sys;print(json.load(sys.stdin)["access_token"])')

kubectl apply -n kubernaut-system -f - <<EOF
apiVersion: v1
kind: Secret
metadata: {name: kube-mcp-server-broker-cred, labels: {mcp.kuadrant.io/secret: "true"}}
type: Opaque
stringData: {token: "Bearer ${BROKER_TOKEN}"}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: kube-mcp-server-route}
spec:
  hostnames: [kube-mcp-server.127-0-0-1.sslip.io]
  parentRefs: [{name: mcp-gateway, namespace: gateway-system, sectionName: mcp}]
  rules: [{backendRefs: [{name: kube-mcp-server, port: 8080}]}]
---
apiVersion: mcp.kuadrant.io/v1alpha1
kind: MCPServerRegistration
metadata: {name: loopback-cluster, labels: {kubernaut.ai/managed: "true"}}
spec:
  prefix: "loopback_cluster_"
  credentialRef: {name: kube-mcp-server-broker-cred}
  targetRef: {group: gateway.networking.k8s.io, kind: HTTPRoute, name: kube-mcp-server-route, namespace: kubernaut-system}
EOF
```

You now have the same stack Path A produces, minus FMC/Valkey/DataStorage
themselves (add those from `DeployFleetCoreInfra` Phase 4+ if you need FMC's
own HTTP API too — Path A is faster for that).

---

## Path C: Envoy AI Gateway (EAIGW) variant — manual walkthrough

EAIGW replaces the whole Kuadrant stack (Istio + controller + broker) with
two Helm charts and no separate broker component — `MCPRoute.backendRefs`
aggregates multiple backends natively. This path is **spike-validated
(Spike S18) and fully automated** via `make test-e2e-fleetmetadatacache-eaigw`
(Path A); the manual steps below are for understanding or tweaking each
moving part, mirroring Path B for the Kuadrant variant.

Reuse Keycloak + kube-mcp-server exactly as in Path B (steps B2-B4) — the
RFC 8693 exchange lives inside kube-mcp-server, not the gateway, so nothing
there changes. Replace step B5 with:

```bash
# Two separate charts: the base gateway, then the AI Gateway layer + its CRDs.
# ⚠️ Version pin matters: ai-gateway-helm v1.0.0 requires Envoy Gateway v1.8.1,
#    NOT v1.7.0 (mismatched versions crash-loop the AI Gateway controller with
#    "no matches for aigateway.envoyproxy.io/v1beta1").
helm install eg oci://docker.io/envoyproxy/gateway-helm \
  --version v1.8.1 -n envoy-gateway-system --create-namespace
kubectl rollout status deployment/envoy-gateway -n envoy-gateway-system --timeout=180s

# ⚠️ CRDs are a SEPARATE chart in v1.0.0 (the main chart does not bundle them
#    despite older docs implying otherwise) — install both or the controller
#    crash-loops on missing CRDs.
helm install aieg-crds oci://docker.io/envoyproxy/ai-gateway-crds-helm \
  --version v1.0.0 -n envoy-ai-gateway-system --create-namespace
helm install aieg oci://docker.io/envoyproxy/ai-gateway-helm \
  --version v1.0.0 -n envoy-ai-gateway-system
kubectl rollout status deployment/ai-gateway-controller -n envoy-ai-gateway-system --timeout=180s

# ⚠️ envoy-gateway's own ServiceAccount has no RBAC for MCPRoute by default,
#    even though extensionManager.resources below declares it as a watched
#    resource — without this, the envoy-gateway controller cache-sync fails
#    ("mcproutes.aigateway.envoyproxy.io is forbidden") and the data-plane
#    pod never becomes ready.
kubectl apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: envoy-gateway-mcproute-reader}
rules:
- apiGroups: [aigateway.envoyproxy.io]
  resources: [mcproutes, mcproutes/status]
  verbs: [get, list, watch, patch, update]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: envoy-gateway-mcproute-reader}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: envoy-gateway-mcproute-reader}
subjects: [{kind: ServiceAccount, name: envoy-gateway, namespace: envoy-gateway-system}]
EOF

# ⚠️ Several undocumented config keys are mandatory on envoy-gateway-config,
#    none mentioned in the upstream MCP quickstart:
#      1. extensionApis.enableBackend (+ enableEnvoyPatchPolicy) — otherwise
#         every route reports "ResolvedRefs: Backend is disabled in Envoy
#         Gateway configuration" (500 direct_response).
#      2. extensionManager wired to the AI Gateway controller's own
#         extension-server port (1063), WITH the full hooks.xdsTranslator
#         translation block (listener/route/cluster/secret includeAll) — a
#         partial config (e.g. only hooks.xdsTranslator.post) reproduces the
#         same symptom as a missing extensionManager: the proxy's upstream
#         cluster stays pointed at a literal placeholder IP (192.0.2.42) and
#         every request times out (503 connection_timeout).
kubectl patch configmap envoy-gateway-config -n envoy-gateway-system --type merge -p '
data:
  envoy-gateway.yaml: |
    apiVersion: gateway.envoyproxy.io/v1alpha1
    kind: EnvoyGateway
    provider:
      type: Kubernetes
    gateway:
      controllerName: gateway.envoyproxy.io/gatewayclass-controller
    logging:
      level:
        default: info
    extensionApis:
      enableBackend: true
      enableEnvoyPatchPolicy: true
    extensionManager:
      hooks:
        xdsTranslator:
          post: [Translation, Cluster, Route]
          translation:
            listener: {includeAll: true}
            route: {includeAll: true}
            cluster: {includeAll: true}
            secret: {includeAll: true}
      service:
        fqdn:
          hostname: ai-gateway-controller.envoy-ai-gateway-system.svc.cluster.local
          port: 1063
      resources:
      - group: aigateway.envoyproxy.io
        version: v1beta1
        kind: MCPRoute
'
kubectl rollout restart deployment/envoy-gateway -n envoy-gateway-system

kubectl apply -f - <<'EOF'
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata: {name: envoy-ai-gateway}
spec: {controllerName: gateway.envoyproxy.io/gatewayclass-controller}
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata: {name: mcp-gateway, namespace: kubernaut-system}
spec:
  gatewayClassName: envoy-ai-gateway
  listeners: [{name: mcp, port: 8080, protocol: HTTP}]
EOF
```

The Gateway's Service is dynamically named
(`envoy-<gw-namespace>-<gw-name>-<8-char-hash>`) — find it via its owning-
gateway labels rather than guessing the name:

```bash
kubectl get svc -n envoy-gateway-system \
  -l gateway.envoyproxy.io/owning-gateway-name=mcp-gateway,gateway.envoyproxy.io/owning-gateway-namespace=kubernaut-system \
  -o jsonpath='{.items[0].metadata.name}'
```

⚠️ **JWKS over self-signed TLS needs an explicit `Backend` + `BackendTLSPolicy`**
— `MCPRoute.securityPolicy.oauth.jwks.remoteJWKS.uri` alone only trusts
Envoy's system CA bundle, which rejects Keycloak's self-signed cert:

```bash
kubectl create configmap keycloak-ca --from-file=ca.crt=/tmp/keycloak-tls.crt -n kubernaut-system

kubectl apply -n kubernaut-system -f - <<'EOF'
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: Backend
metadata: {name: keycloak-jwks}
spec:
  endpoints: [{fqdn: {hostname: keycloak.kubernaut-system.svc.cluster.local, port: 8443}}]
---
apiVersion: gateway.networking.k8s.io/v1alpha3
kind: BackendTLSPolicy
metadata: {name: keycloak-jwks-tls}
spec:
  targetRefs: [{group: gateway.envoyproxy.io, kind: Backend, name: keycloak-jwks}]
  validation:
    caCertificateRefs: [{name: keycloak-ca, group: "", kind: ConfigMap}]
    hostname: keycloak
---
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: Backend
metadata: {name: loopback-cluster}
spec:
  # ⚠️ hostname MUST be a fully-qualified name (>= 2 dots) — a bare Service
  #    name like "kube-mcp-server" fails Backend validation ("should be a
  #    domain with at least two segments separated by dots").
  endpoints: [{fqdn: {hostname: kube-mcp-server.kubernaut-system.svc.cluster.local, port: 8080}}]
---
apiVersion: aigateway.envoyproxy.io/v1beta1
kind: MCPRoute
metadata: {name: kube-mcp-server-route}
spec:
  parentRefs: [{name: mcp-gateway}]
  # ⚠️ path is a plain string, NOT {value: ...} — {value: ...} fails schema validation.
  path: /mcp
  backendRefs:
  # ⚠️ kind defaults to Service — MUST set group+kind explicitly to target a
  #    Backend CR, or the route silently targets a nonexistent Service.
  #    No "prefix" or "toolSelector" field exists on backendRefs (neither
  #    validates against the schema); EAIGW auto-prefixes each backend's
  #    tools with "{name}__" for free — add more Backend+backendRefs entries
  #    (e.g. prod-east, prod-west) to aggregate multiple clusters behind one
  #    shared MCPRoute, exactly as Path A's automation does.
  - group: gateway.envoyproxy.io
    kind: Backend
    name: loopback-cluster
  securityPolicy:
    oauth:
      claimToHeaders: []
      jwt:
        providers:
        - name: keycloak
          issuer: "https://keycloak:8443/realms/kubernaut-fleet"
          audiences: ["kube-mcp-server"]
          remoteJWKS:
            uri: "https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/certs"
            backendRefs: [{name: keycloak-jwks}]
EOF
```

(Exact `MCPRoute.securityPolicy` field names may drift slightly between EAIGW
patch releases — see the working manifests captured in
`docs/spikes/multi-cluster-mcp-gateway/spike-s18-envoy-ai-gateway-e2e/` and
the canonical, kept-current version in `test/infrastructure/fleet_e2e.go`'s
`deployEnvoyAIGatewayInfra`/`deployEnvoyAIGatewayRegistrations`.)

**Measured resource footprint** (steady state, `crictl stats`): Envoy Gateway
controller 76 MB + Envoy data-plane pod 153 MB + AI Gateway controller 63 MB
+ kube-mcp-server 24 MB ≈ **316 MB total**, comparable to Kuadrant's
Istio+controller+broker stack.

---

## Exercising the stack

Get a token as FMC would, and call the gateway directly:

```bash
TOKEN=$(curl -sk -X POST \
  https://localhost:30557/realms/kubernaut-fleet/protocol/openid-connect/token \
  -d grant_type=client_credentials -d client_id=kubernaut-fleet-read \
  -d client_secret=e2e-fleet-secret -d scope=kube-mcp-server-audience \
  | python3 -c 'import json,sys;print(json.load(sys.stdin)["access_token"])')

# Kuadrant path
curl -s http://localhost:31975/mcp -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | python3 -m json.tool
```

Expect tool names prefixed by cluster, e.g. `loopback_cluster_resources_get`
(Kuadrant) or `loopback-cluster__resources_get` (EAIGW) — confirming FMC's
per-cluster tool-name disambiguation is working end-to-end.

Sanity-check the negative cases (this is exactly what
`E2E-FMC-054-014`/token-exchange tests assert):

```bash
# No token → 401
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:31975/mcp

# FMC's own (un-exchanged) token presented directly to the K8s API → 401
kubectl --kubeconfig <(kubectl config view --raw --minify | \
  yq '.users[0].user = {"token": "'"$TOKEN"'"}') get pods -A
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `oidc: authenticator not initialized` / `dial tcp: ... no such host` | API server's static pod is `hostNetwork: true` and can't resolve in-cluster Service names via CoreDNS | Patch the pod's hosts file with a static IP entry (see B3) |
| Keycloak token exchange returns `invalid_scope` | `sts_scopes` unset — the exchange library always sends a `scope` param, and Keycloak rejects an explicitly empty one | Set `sts_scopes = ["k8s-api-audience"]` even though it's already a default client scope |
| Exchanged token has no `preferred_username`, API server 401s | Keycloak's default service-account tokens carry no `preferred_username` claim | Use the `k8s-api-audience` scope's dedicated User Property mapper (already in the realm export) |
| Kuadrant broker gets 401 on its own discovery connection | `require_oauth = true` on kube-mcp-server also gates the broker's upstream probe, not just client requests | Give the broker its own static credential via `MCPServerRegistration.credentialRef` |
| EAIGW: `no matches for aigateway.envoyproxy.io/v1beta1`, controller crash-loops | Wrong Envoy Gateway version pin (v1.7.0 vs required v1.8.1), or CRDs chart not installed separately | Pin Envoy Gateway to v1.8.1; install `ai-gateway-crds-helm` explicitly |
| EAIGW: `500`, `ResolvedRefs: Backend is disabled` | `extensionApis.enableBackend` not set | Patch `envoy-gateway-config` (see Path C) |
| EAIGW: `503 upstream_reset_before_response_started{connection_timeout}` | `extensionManager` not wired; proxy cluster stuck at placeholder IP `192.0.2.42` | Patch `extensionManager` to point at the AI Gateway controller's extension-server port 1063 |
| EAIGW: valid token still `401` at the gateway | JWKS fetch over self-signed TLS fails Envoy's default trust bundle | Add a `Backend` + `BackendTLSPolicy` for the Keycloak JWKS endpoint (see Path C) |
| EAIGW: `envoy-gateway` controller cache-sync never completes, data-plane pod stuck not-ready, logs show `mcproutes.aigateway.envoyproxy.io is forbidden` | `envoy-gateway` ServiceAccount has no RBAC for `MCPRoute` even though `extensionManager.resources` declares it as watched | Apply the `ClusterRole`/`ClusterRoleBinding` shown in Path C |
| EAIGW: `Backend` rejected with `hostname ... should be a domain with at least two segments separated by dots` | `Backend.spec.endpoints[].fqdn.hostname` requires a fully-qualified name | Use `<svc>.<namespace>.svc.cluster.local`, not the bare Service name |
| EAIGW: `MCPRoute` rejected with `unknown field "spec.backendRefs[0].prefix"` / `"toolSelector"` / `"spec.path.value"` | Those fields don't exist in the `MCPRoute` schema | Omit `prefix`/`toolSelector` (EAIGW auto-prefixes tools with `{name}__`); use `path: /mcp` (plain string, not `{value: ...}`) |
| `helm upgrade --reuse-values` fails across a major Envoy Gateway version bump | Breaking values-schema change between chart versions | `helm uninstall` + fresh `helm install` instead of upgrading in place |

---

## Cleanup

```bash
kind delete cluster --name fleet-dev   # Path B/C
kind delete cluster --name fmc-e2e     # Path A
```

## References

- [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- [Fleet Federation Guide](../../architecture/fleet-federation-guide.md) (production onboarding, not dev setup)
- `test/infrastructure/fleet_e2e.go`, `fleetmetadatacache_e2e.go`, `keycloak_e2e.go` — source of truth for Path A's automation
- `docs/testing/BR-INTEGRATION-054/TEST_PLAN.md` — E2E scenario catalog (`E2E-FMC-054-01{0..4}`)
- Spike S17 (`docs/spikes/multi-cluster-mcp-gateway/spike-s17-keycloak-token-exchange/`) — Keycloak vs. Dex decision, RFC 8693 exchange design
- Spike S18 (`docs/spikes/multi-cluster-mcp-gateway/spike-s18-envoy-ai-gateway-e2e/`) — Envoy AI Gateway real Kind E2E validation, multi-backend aggregation, dynamic Service resolution
