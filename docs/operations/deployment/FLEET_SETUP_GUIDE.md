# Fleet Mode Setup Guide (Hub + Spoke)

**Status**: Local/dev walkthrough for a genuine two-cluster fleet topology, condensed
from this repo's own E2E test infrastructure.
**Audience**: Operators and SREs who want to configure a real hub+spoke environment to
try (or validate) Kubernaut's multi-cluster fleet mode, and contributors who want to
understand each moving part instead of running it as a black box.
**Authority**: Issue #54, [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md), `test/infrastructure/fleet_e2e.go` (source of truth for every step below).

---

## What you're building

```
┌─────────────────────────────── hub cluster ───────────────────────────────┐
│                                                                             │
│  Kubernaut (Helm, global.fleet.enabled=true)   Keycloak (kubernaut-fleet) │
│  GW · SP · RO · WE · AA · EM · KA · AF · DS  ──────────▲───────────────── │
│         │ MCP tool calls (Bearer token)                │ OIDC issuer      │
│         ▼                                               │                 │
│  Kuadrant MCP Gateway ──registration "remote-cluster"───┘                 │
│         │ tools/call                                                      │
│         ▼                                                                 │
│  bridge Service (kube-mcp-server-remote) ─────┐                           │
└────────────────────────────────────────────────┼──────────────────────────┘
                                                   │ podman "kind" bridge network
┌──────────────────────────────────────────────── ▼ ────── spoke cluster ───┐
│  kube-mcp-server (passthrough + RFC 8693 token exchange)                  │
│         │                                                                  │
│         ▼                                                                  │
│  Kubernetes API (OIDC-trusts the SAME Keycloak, via a bridged Service)    │
│         │                                                                  │
│         ▼                                                                  │
│  kubernaut-workflows namespace (WorkflowExecution Job dispatch target)    │
└─────────────────────────────────────────────────────────────────────────┘
```

The **hub** runs the full Kubernaut stack and holds every CRD (`RemediationRequest`,
`RemediationWorkflow`, `MCPServerRegistration`, etc.). The **spoke** is a genuinely
separate Kubernetes control plane — no Kubernaut CRDs, just `kube-mcp-server` plus
whatever target workloads and the `kubernaut-workflows` namespace where
`WorkflowExecution`'s Job dispatch actually runs remediation Jobs
(`pkg/workflowexecution/executor/client_factory.go`: an empty `ClusterID` routes to a
local client, a non-empty one routes through the MCP Gateway to the spoke).

Both clusters trust the **same** Keycloak realm (`kubernaut-fleet`), reached from the
spoke via a hand-authored Service+Endpoints bridge over the podman `kind` network — no
SSH tunnel, no separate IdP per cluster.

---

## Prerequisites

- `kind`, `kubectl`, `helm`, `podman` (this repo's E2E harness uses podman as the Kind
  provider — set `KIND_EXPERIMENTAL_PROVIDER=podman`)
- `openssl`, `curl`, `python3`
- ~4 GB free RAM across both clusters
- An API key from an LLM provider, if you intend to install the full Kubernaut chart
  on the hub (Path B's last step) rather than just exercising the gateway/spoke wiring
  on its own

---

## Path A (recommended): use the existing automation

The full two-cluster topology is already wired into this repo's E2E harness and is
exactly what CI runs — the fastest, most reliably-in-sync way to get a working hub+spoke
environment:

```bash
PRESERVE_E2E_CLUSTER=true make test-e2e-fleet
```

This takes ~35 minutes and, on success, leaves **both** clusters running:

```bash
export KUBECONFIG=~/.kube/fleet-e2e-config          # hub
export REMOTE_KUBECONFIG=~/.kube/fleet-e2e-remote-config  # spoke

kubectl get pods -n kubernaut-system                       # hub: full Kubernaut stack
KUBECONFIG=$REMOTE_KUBECONFIG kubectl get ns kubernaut-workflows  # spoke: dispatch target
```

| Endpoint | URL |
|---|---|
| Kuadrant MCP Gateway (hub) | `http://localhost:31975/mcp` |
| Keycloak (hub) | `https://localhost:30557/realms/kubernaut-fleet` |
| Remote cluster identity | `remote-cluster` (every fleet test targets this name) |

To tear down: `kind delete cluster --name fleet-e2e && kind delete cluster --name fleet-e2e-remote`.

This automation lives in `test/infrastructure/fleet_e2e.go`
(`SetupFleetE2EInfrastructure`) and `fleetmetadatacache_remote_cluster.go`
(`SetupRemoteClusterForFMC`) if you want to read the real source instead of the
condensed steps below.

Skip to [Verify hub+spoke routing](#verify-hubspoke-routing) to start poking at it, or
continue reading for the manual walkthrough.

---

## Path B: Manual walkthrough

### B1. Create the hub cluster

```bash
kind create cluster --name fleet-hub
export KUBECONFIG=~/.kube/fleet-hub-config
kind get kubeconfig --name fleet-hub > "$KUBECONFIG"
kubectl create namespace kubernaut-system
```

### B2. Deploy Keycloak with the kubernaut-fleet realm

Same realm three clients used by the single-cluster FMC walkthrough — reused here
unmodified:

| Client ID | Secret | Purpose |
|---|---|---|
| `kubernaut-fleet-read` | `e2e-fleet-secret` | Every fleet-aware service's `client_credentials` identity |
| `kube-mcp-server` | `e2e-kube-mcp-server-secret` | Requests the RFC 8693 token exchange |
| `k8s-api` | *(bearer-only, no secret)* | The exchange's target audience — what **both** clusters' API servers validate |

Follow [`fleet-mcp-gateway-keycloak-local-setup.md`](../../development/getting-started/fleet-mcp-gateway-keycloak-local-setup.md)'s
**B2 (Deploy Keycloak)**, **B3 (Enable OIDC on the Kind API server)**, **B4 (Deploy
kube-mcp-server)**, and **B5 (Deploy the Kuadrant MCP Gateway)** against this hub
cluster — those four steps are identical for hub+spoke and single-cluster loopback; no
changes needed. Come back here once B5 is done and the Kuadrant broker is `Ready`.

At the end of B4/B5, note two things you'll need below:

- **Keycloak's TLS cert** (`/tmp/keycloak-tls.crt`) — the spoke's API server needs to
  trust it too.
- **The hub node's name**: `fleet-hub-control-plane`.

### B3. Create the spoke cluster

```bash
kind create cluster --name fleet-spoke
export SPOKE_KUBECONFIG=~/.kube/fleet-spoke-config
kind get kubeconfig --name fleet-spoke > "$SPOKE_KUBECONFIG"
kubectl --kubeconfig "$SPOKE_KUBECONFIG" create namespace kubernaut-system
```

Kind clusters created by the same podman daemon share the `kind` bridge network — any
node's IP is directly routable from any other node's containers, no host port mapping
required (validated in Spike S19). This is what lets the spoke's API server reach the
hub's Keycloak, and what lets the hub's Kuadrant Gateway reach the spoke's
`kube-mcp-server`, without a tunnel.

### B4. Create the workflow dispatch namespace + RBAC on the spoke

The hub only pre-creates `kubernaut-workflows` on itself. Job-backend workflows (e.g.
`crashloop-config-fix-v1`) dispatch their Job to whichever cluster
`RemediationRequest.ClusterID` names, so the namespace and its executor identity must
also exist on the spoke:

```bash
kubectl --kubeconfig "$SPOKE_KUBECONFIG" apply -f - <<'EOF'
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-workflows
EOF

kubectl --kubeconfig "$SPOKE_KUBECONFIG" apply -f - <<'EOF'
apiVersion: v1
kind: ServiceAccount
metadata:
  name: workflow-job-executor
  namespace: kubernaut-workflows
  labels: {app: workflowexecution, component: job-executor}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: workflow-job-executor
rules:
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets", "daemonsets"]
  verbs: ["get", "list", "patch"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: workflow-job-executor
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: workflow-job-executor}
subjects: [{kind: ServiceAccount, name: workflow-job-executor, namespace: kubernaut-workflows}]
EOF
```

`workflow-job-executor` is the ServiceAccount Job pods run as (e.g.
`oomkill-increase-memory-job`'s `remediate.sh` patches a Deployment's resource limits;
`crashloop-config-fix-v1` patches a ConfigMap back to a known-good value) — see
`execution.serviceAccountName` in the relevant `RemediationWorkflow` manifest.

### B5. Bridge Keycloak into the spoke

The spoke's API server needs to reach the hub's Keycloak by the same hostname
(`keycloak:8443`) the OIDC issuer URL uses — a hand-authored Service+Endpoints pair
does this without a selector, resolvable by plain in-cluster DNS:

```bash
HUB_NODE_IP=$(podman inspect fleet-hub-control-plane \
  --format '{{.NetworkSettings.Networks.kind.IPAddress}}')

kubectl --kubeconfig "$SPOKE_KUBECONFIG" apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: keycloak
  namespace: kubernaut-system
spec:
  ports: [{port: 8443, targetPort: 30557}]
---
apiVersion: v1
kind: Endpoints
metadata:
  name: keycloak
  namespace: kubernaut-system
subsets:
- addresses: [{ip: "$HUB_NODE_IP"}]
  ports: [{port: 30557}]
EOF
```

`targetPort`/the Endpoints port here is the hub's Keycloak **NodePort** (`30557`), while
the Service's own `port` (`8443`) is what in-cluster clients dial — these are
deliberately different numbers. A bridge created with them equal reproduces a
"connection refused" (Spike S19 finding: in-cluster clients dial the well-known port,
not the peer's NodePort).

Copy Keycloak's self-signed TLS cert to wherever your spoke-side OIDC patch step (B6)
expects a CA bundle — same file `/tmp/keycloak-tls.crt` from B2.

### B6. Enable OIDC on the spoke's API server

Identical to the hub's B3 step, run against the spoke instead:

```bash
NODE=fleet-spoke-control-plane
podman cp /tmp/keycloak-tls.crt "$NODE:/etc/kubernetes/pki/oidc-ca.crt"
podman exec "$NODE" bash -c '
sed -i "/--tls-private-key-file/a\\
    - \"--oidc-username-prefix=keycloak:\"\\
    - --oidc-username-claim=preferred_username\\
    - --oidc-client-id=k8s-api\\
    - --oidc-ca-file=/etc/kubernetes/pki/oidc-ca.crt\\
    - \"--oidc-issuer-url=https://keycloak:8443/realms/kubernaut-fleet\"" \
  /etc/kubernetes/manifests/kube-apiserver.yaml'

# Same hostNetwork DNS caveat as the hub (see the referenced doc's B3) --
# patch the node's /etc/hosts with the bridge Service's ClusterIP:
KC_IP=$(kubectl --kubeconfig "$SPOKE_KUBECONFIG" get svc keycloak -n kubernaut-system -o jsonpath='{.spec.clusterIP}')
podman exec "$NODE" bash -c "echo '$KC_IP keycloak' >> /etc/hosts"

until kubectl --kubeconfig "$SPOKE_KUBECONFIG" get --raw /readyz &>/dev/null; do sleep 3; done
```

### B7. Deploy kube-mcp-server on the spoke + grant it RBAC

Deploy the exact same passthrough+STS `kube-mcp-server` as the hub (B4's manifest,
unchanged — same Keycloak realm, same STS client), then grant the **exchanged**
identity two things: read access, and — spoke-only — write access to `batch/jobs` so
`WorkflowExecution` can actually create the remediation Job here:

```bash
# Deploy kube-mcp-server into $SPOKE_KUBECONFIG using the referenced doc's B4 manifest.

kubectl --kubeconfig "$SPOKE_KUBECONFIG" apply -f - <<'EOF'
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: fleet-exchanged-identity-binding
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: view}
subjects:
- {kind: User, name: "keycloak:service-account-kubernaut-fleet-read", apiGroup: rbac.authorization.k8s.io}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: fleet-exchanged-identity-job-write
rules:
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create", "get", "list", "watch", "delete", "patch", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: fleet-exchanged-identity-job-write-binding
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: fleet-exchanged-identity-job-write}
subjects:
- {kind: User, name: "keycloak:service-account-kubernaut-fleet-read", apiGroup: rbac.authorization.k8s.io}
EOF
```

`patch`+`update` (not just `create`) are both required: `kube-mcp-server`'s
create-or-update tool always issues a server-side-apply `patch`, regardless of whether
the Job already exists — a bare `create` grant 403s with "cannot patch resource jobs".

Expose it so the hub can reach it:

```bash
kubectl --kubeconfig "$SPOKE_KUBECONFIG" apply -f - <<'EOF'
apiVersion: v1
kind: Service
metadata:
  name: kube-mcp-server-nodeport
  namespace: kubernaut-system
spec:
  type: NodePort
  selector: {app: kube-mcp-server}
  ports: [{port: 8080, targetPort: 8080, nodePort: 30180}]
EOF

SPOKE_NODE_IP=$(podman inspect fleet-spoke-control-plane \
  --format '{{.NetworkSettings.Networks.kind.IPAddress}}')
```

### B8. Bridge the hub's Kuadrant Gateway to the spoke, register as "remote-cluster"

Back on the hub, bridge a local Service to the spoke's `kube-mcp-server` NodePort, then
register it with Kuadrant under the identity every fleet test (and, once you install
the chart, every fleet-tagged `RemediationRequest`) expects: `remote-cluster`.

```bash
kubectl --kubeconfig "$KUBECONFIG" apply -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: kube-mcp-server-remote
  namespace: kubernaut-system
spec:
  ports: [{port: 8080, targetPort: 30180}]
---
apiVersion: v1
kind: Endpoints
metadata:
  name: kube-mcp-server-remote
  namespace: kubernaut-system
subsets:
- addresses: [{ip: "$SPOKE_NODE_IP"}]
  ports: [{port: 30180}]
EOF
```

Get a broker discovery token (the Kuadrant broker's own upstream connection needs a
static credential whenever `require_oauth = true` — separate from, and never injected
into, per-request `tools/call` proxying):

```bash
BROKER_TOKEN=$(curl -sk -X POST \
  https://localhost:30557/realms/kubernaut-fleet/protocol/openid-connect/token \
  -d grant_type=client_credentials -d client_id=kubernaut-fleet-read \
  -d client_secret=e2e-fleet-secret -d scope=kube-mcp-server-audience \
  | python3 -c 'import json,sys;print(json.load(sys.stdin)["access_token"])')

kubectl --kubeconfig "$KUBECONFIG" apply -n kubernaut-system -f - <<EOF
apiVersion: v1
kind: Secret
metadata: {name: kube-mcp-server-remote-broker-cred, labels: {mcp.kuadrant.io/secret: "true"}}
type: Opaque
stringData: {token: "Bearer ${BROKER_TOKEN}"}
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata: {name: kube-mcp-server-remote-route}
spec:
  hostnames: [kube-mcp-server-remote.127-0-0-1.sslip.io]
  parentRefs: [{name: mcp-gateway, namespace: gateway-system, sectionName: mcp}]
  rules: [{backendRefs: [{name: kube-mcp-server-remote, port: 8080}]}]
---
apiVersion: mcp.kuadrant.io/v1alpha1
kind: MCPServerRegistration
metadata: {name: remote-cluster, labels: {kubernaut.ai/managed: "true"}}
spec:
  prefix: "remote_cluster_"
  credentialRef: {name: kube-mcp-server-remote-broker-cred}
  targetRef: {group: gateway.networking.k8s.io, kind: HTTPRoute, name: kube-mcp-server-remote-route, namespace: kubernaut-system}
EOF
```

Grant the hub the same exchanged-identity `view` binding from B7's first manifest (the
hub's own local loopback registration, if you keep one, needs it too).

### B9. Create the shared fleet OAuth2 Secret, then install Kubernaut

Every fleet-aware service (GW, RO, SP, AF, EM, WE) authenticates its own MCP Gateway
calls with one shared Keycloak client:

```bash
kubectl --kubeconfig "$KUBECONFIG" apply -n kubernaut-system -f - <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: fleet-oauth2-creds
  namespace: kubernaut-system
  labels: {component: fleet}
type: Opaque
stringData:
  client-id: "kubernaut-fleet-read"
  client-secret: "e2e-fleet-secret"
EOF
```

Install the chart on the hub with fleet enabled (see
[charts/kubernaut/README.md](../../../charts/kubernaut/README.md) for the rest of the
Quick Start — namespace, PostgreSQL/Valkey Secrets, Rego policies, and an LLM provider
API key are still required the same as any install):

```bash
helm install kubernaut charts/kubernaut -n kubernaut-system \
  --set global.fleet.enabled=true \
  --set global.fleet.mcpGatewayEndpoint="http://mcp-gateway-istio.gateway-system.svc:8080/mcp" \
  --set global.fleet.mcpGatewayType=kuadrant \
  --set global.fleet.oauth2.enabled=true \
  --set global.fleet.oauth2.tokenURL="https://keycloak:8443/realms/kubernaut-fleet/protocol/openid-connect/token" \
  --set global.fleet.oauth2.credentialsSecretRef=fleet-oauth2-creds \
  --set workflowexecution.fleet.oauth2.credentialsSecretRef=fleet-oauth2-creds \
  # ... plus your usual PostgreSQL/Valkey/LLM/Rego overrides
```

`workflowexecution.fleet.oauth2.credentialsSecretRef` is called out explicitly because
WE is the only fleet-integration-capable service that does **not** fall back to
`global.fleet.oauth2.credentialsSecretRef` — it's the sole service calling MCP *write*
tools (`resources_create_or_update`/`resources_delete`), so it keeps its own,
independently-settable credential for least-privilege.

### Fleet still needs a triggering path: Autonomous or Interactive

`global.fleet.*` only wires the MCP Gateway plumbing — it doesn't put anything in
front of Kubernaut that can start an investigation. Layer one of the other two modes
on top, in the same `helm install`:

- **Autonomous** (alert-driven): `gateway.enabled` defaults to `true` already; add
  `gateway.auth.signalSources` for your AlertManager instance. See
  [AUTONOMOUS_MODE_SETUP_GUIDE.md](./AUTONOMOUS_MODE_SETUP_GUIDE.md).
- **Interactive** (chat-driven): `console.enabled=true` plus `console.auth.secretName`
  and `apifrontend.config.auth.issuerURL`/`jwtProviders` for your OIDC provider. See
  [INTERACTIVE_MODE_SETUP_GUIDE.md](./INTERACTIVE_MODE_SETUP_GUIDE.md).

Both work with fleet mode simultaneously — the full `test-e2e-fleet` suite (Path A)
deploys Gateway *and* Console/AF together specifically to exercise both triggering
paths against the same hub+spoke topology. Whether an individual `RemediationRequest`
resolves to the hub or a spoke is driven entirely by
`RemediationRequest.spec.clusterID` (SignalProcessing's `ClusterClassification`, or an
explicit `cluster_id` in an interactive chat message) — independent of which path
created it.

---

## Verify hub+spoke routing

Send a fleet-scoped alert naming `remote-cluster` and confirm the resulting
`RemediationRequest` actually routes through the spoke, not the hub:

```bash
curl -s http://localhost:31975/mcp \
  -H "Authorization: Bearer $BROKER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | python3 -m json.tool
```

Expect tool names prefixed `remote_cluster_` (e.g. `remote_cluster_resources_get`) —
confirming the spoke's tools are discoverable, distinct from any `loopback_cluster_`
tools you may also have registered. Once you post an alert with
`labels.cluster_id: remote-cluster` to the Gateway's `/api/v1/signals/prometheus`
endpoint, check the target namespace on the **spoke**, not the hub:

```bash
KUBECONFIG=$SPOKE_KUBECONFIG kubectl get jobs -n kubernaut-workflows -w
```

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Spoke's API server: `oidc: authenticator not initialized` / DNS resolution failure | Same hostNetwork `/etc/hosts` gap as the hub (see referenced doc's B3) | Patch `/etc/hosts` on the spoke's node too (B6) |
| Kuadrant broker gets `401` discovering `remote-cluster` | No `credentialRef` on the `MCPServerRegistration`, or a stale/expired broker token | Re-mint `BROKER_TOKEN` and recreate the Secret |
| `tools/call` against `remote_cluster_*` tools returns `403` on Job creation | Spoke RBAC only has the `view` binding, missing `fleet-exchanged-identity-job-write` | Apply B7's second `ClusterRoleBinding` |
| Job never appears in `kubernaut-workflows` on the spoke | `RemediationRequest.ClusterID` wasn't set, or the alert used a different cluster identity than the `MCPServerRegistration`'s name | Confirm the alert payload's `cluster_id` matches `remote-cluster` exactly (case-sensitive) |
| Bridge Service connection refused | `port/targetPort` swapped between the Service (well-known port) and the Endpoints (peer's real NodePort) — see B5's note | Double-check which number is the NodePort vs. the in-cluster dial port |
| Everything above already covered | Single-cluster loopback issues (Keycloak, Kuadrant, kube-mcp-server itself) | See [`fleet-mcp-gateway-keycloak-local-setup.md`](../../development/getting-started/fleet-mcp-gateway-keycloak-local-setup.md)'s own troubleshooting table |

---

## Cleanup

```bash
kind delete cluster --name fleet-hub
kind delete cluster --name fleet-spoke
```

---

## References

- [ADR-068: Fleet Federation Architecture](../../architecture/decisions/ADR-068-fleet-federation-architecture.md)
- [Fleet Federation Guide](../../architecture/fleet-federation-guide.md) — production onboarding, not local dev
- [fleet-mcp-gateway-keycloak-local-setup.md](../../development/getting-started/fleet-mcp-gateway-keycloak-local-setup.md) — single-cluster loopback variant this guide extends; B2-B5 are reused here unmodified
- `test/infrastructure/fleet_e2e.go` (`SetupFleetE2EInfrastructure`, `deployKuadrantRegistrations`) — source of truth for Path A's automation and this guide's hub-side steps
- `test/infrastructure/fleetmetadatacache_remote_cluster.go` (`SetupRemoteClusterForFMC`) — source of truth for this guide's spoke-side steps
- `test/infrastructure/workflowexecution_e2e_hybrid.go` (`createWorkflowJobExecutorRBAC`) — source of truth for B4's RBAC
- [charts/kubernaut/README.md](../../../charts/kubernaut/README.md) — full Helm chart install reference (Secrets, Rego policies, LLM provider config)
- [AUTONOMOUS_MODE_SETUP_GUIDE.md](./AUTONOMOUS_MODE_SETUP_GUIDE.md) / [INTERACTIVE_MODE_SETUP_GUIDE.md](./INTERACTIVE_MODE_SETUP_GUIDE.md) — the triggering path fleet mode layers onto; see "Fleet still needs a triggering path" above
- [Issue #54: Fleet Management](https://github.com/jordigilh/kubernaut/issues/54)
