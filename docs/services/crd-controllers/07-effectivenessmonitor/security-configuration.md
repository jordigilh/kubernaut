# Effectiveness Monitor Service - Security Configuration

**Version**: v2.0
**Last Updated**: 2026-08-02
**Service Type**: Kubernetes CRD controller — **no HTTP business API to protect**

---

## Changelog

| Version | Date | Changes | Reference |
|---------|------|---------|-----------|
| **v2.0** | 2026-08-02 | **#1806 CORRECTION**: Full rewrite. Removed the entire fictional stateless-HTTP-service security model: a Kubernetes TokenReviewer `AuthMiddleware()` protecting a nonexistent port-8080 REST API, a `HolmesGPTClient` (`pkg/monitor/holmesgpt_client.go`) authenticating to a live `POST /api/v1/postexec/analyze` call, an `effectiveness-monitor-holmesgpt-client`/`kubernaut-agent-postexec-analyzer` cross-service RBAC pair, a direct PostgreSQL connection with an `effectiveness-monitor-db-credentials` Secret, and a 50 req/s rate-limiting middleware. Replaced with the real security posture verified against `charts/kubernaut/templates/effectivenessmonitor/{effectivenessmonitor,networkpolicy}.yaml`, `charts/kubernaut/templates/rbac/datastorage-rbac.yaml`, `internal/controller/effectivenessmonitor/reconciler.go`, and `pkg/effectivenessmonitor/client/{ds_querier,prometheus_http,alertmanager_http}.go`. | [#1806](https://github.com/jordigilh/kubernaut/issues/1806) |
| v1.0 | 2025-10-06 | ⚠️ **STALE (superseded by v2.0)** — Original fictional stateless-HTTP-service security specification (TokenReviewer auth, HolmesGPT API-key-style flow, PostgreSQL credentials). Never matched any implementation. | — |

---

## Overview

EM is a **Kubernetes CRD controller**, not an HTTP service. There is no port 8080 and no business REST API to authenticate or authorize against — its only two network listeners are `/metrics` (port 9090) and `/healthz`/`/readyz` (port 8081), neither of which requires application-level authentication (see [What Is NOT in This Security Model](#what-is-not-in-this-security-model)). EM's real security surface has four parts:

1. **Least-privilege Kubernetes RBAC** for the controller-runtime watch/read/write operations it actually performs
2. **ServiceAccount Bearer-token authentication** when calling out to DataStorage (its one required dependency)
3. **Best-effort TLS + bearer-token, or plain HTTP,** when calling out to Prometheus/AlertManager (both optional)
4. **A restricted Pod Security Standard** (non-root, read-only rootfs, all capabilities dropped)

EM has **zero AI/LLM dependency in V1.0** — there is no HolmesGPT, HAPI, or Kubernaut Agent (KA) client anywhere in its codebase, so there is no AI-related credential, endpoint, or RBAC to document here.

---

## ServiceAccount & RBAC (Least Privilege)

**ServiceAccount and ClusterRole** (`charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml` — what is actually deployed. The kubebuilder markers in `internal/controller/effectivenessmonitor/reconciler.go` that generate `config/rbac/role.yaml` are slightly broader on a couple of verbs — e.g. `create`/`delete` on `effectivenessassessments` — but the Helm chart below is the ClusterRole that is actually bound to the running ServiceAccount):

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: effectivenessmonitor-controller
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: effectivenessmonitor-controller
rules:
  # Own CRD: watched, status written by the Reconciler
  - apiGroups: ["kubernaut.ai"]
    resources: ["effectivenessassessments"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["kubernaut.ai"]
    resources: ["effectivenessassessments/status"]
    verbs: ["get", "update", "patch"]
  # Read-only: RO-created parent, used for correlation only, never mutated
  - apiGroups: ["kubernaut.ai"]
    resources: ["remediationrequests"]
    verbs: ["get", "list", "watch"]
  # Target-resource reads for health scoring (BR-EM-001) and hash/drift
  # detection (DD-EM-002) across the kinds EM is asked to assess
  - apiGroups: [""]
    resources: ["pods", "nodes", "services", "persistentvolumeclaims", "configmaps"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
    verbs: ["get", "list"]
  - apiGroups: ["autoscaling"]
    resources: ["horizontalpodautoscalers"]
    verbs: ["get", "list"]
  - apiGroups: ["policy"]
    resources: ["poddisruptionbudgets"]
    verbs: ["get", "list"]
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["get", "list"]
  # Istio CRDs (read-only) — some target kinds are Istio-managed (Issue #373)
  - apiGroups: ["security.istio.io"]
    resources: ["authorizationpolicies", "peerauthentications", "requestauthentications"]
    verbs: ["get", "list"]
  - apiGroups: ["networking.istio.io"]
    resources: ["virtualservices", "destinationrules", "gateways", "serviceentries"]
    verbs: ["get", "list"]
  # K8s Event emission (EffectivenessAssessed Normal events on the EA)
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]
  # controller-runtime leader election (Issue #687)
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  # Fleet federation only, cluster-scoped Kuadrant MCP Gateway with no
  # namespace restriction configured (Issue #1686, BR-RBAC-020) — granted
  # instead via a namespace-scoped Role/RoleBinding when a namespace IS set
  - apiGroups: ["mcp.kuadrant.io"]
    resources: ["mcpserverregistrations"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: effectivenessmonitor-controller
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: effectivenessmonitor-controller}
subjects:
  - kind: ServiceAccount
    name: effectivenessmonitor-controller
    namespace: kubernaut-system
---
# Issue #545: bind to the built-in "view" ClusterRole for broad read access
# to CRDs (cert-manager, Istio, etc.) needed to capture a post-remediation
# spec hash across arbitrary target kinds (DD-EM-002) that the ClusterRole
# above does not enumerate individually.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: effectivenessmonitor-view
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: view}
subjects:
  - kind: ServiceAccount
    name: effectivenessmonitor-controller
    namespace: kubernaut-system
```

**Client RoleBinding for DataStorage** (DD-AUTH-014 synthetic-resource SAR pattern; `charts/kubernaut/templates/rbac/datastorage-rbac.yaml`):

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: effectivenessmonitor-data-storage-client
  namespace: kubernaut-system
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: data-storage-client}
subjects:
  - kind: ServiceAccount
    name: effectivenessmonitor-controller
    namespace: kubernaut-system
```

This binding grants no real Kubernetes `Service` permissions — it exists purely so DataStorage's SubjectAccessReview check has a concrete synthetic resource (`services/data-storage-service`) to authorize `effectivenessmonitor-controller` against. See [Authentication to DataStorage](#authentication-to-datastorage-dd-auth-005--dd-auth-014) below. The same `data-storage-client` ClusterRole is bound to every other DataStorage-calling service (Gateway, SignalProcessing, RemediationOrchestrator, AuthWebhook, WorkflowExecution, Kubernaut Agent, Notification, APIFrontend) — EM's binding is one row in that shared table, not a bespoke grant.

**What is *not* in this RBAC** (corrections vs. the fictional v1.0 spec):

- ❌ **No `tokenreviews` `create` permission.** EM has no inbound HTTP auth middleware to back with a TokenReview call.
- ❌ **No `aiapprovalrequests`/`postexecanalyses`-style RBAC of any kind.** Neither CRD exists; EM has no AI dependency to authorize.
- ❌ **No `serviceaccounts/token` "client ClusterRole" for services calling EM.** Nothing calls EM's business API because EM has no business API.
- ❌ **No write access to any Kubernetes resource** other than its own EA status subresource, K8s Events, and Lease objects (leader election).

**Least Privilege Principles**:
- ✅ Update/patch access to its own `EffectivenessAssessment` CRD and status subresource only
- ✅ Read-only on `RemediationRequest` — used purely for correlation, never mutated
- ✅ Read-only on every target-resource kind it health-checks/hashes — never `update`/`delete`/`patch` a customer workload
- ✅ DataStorage access is real but goes through the same buffered-write / typed-query pattern as every other Go service (BR-AUDIT-006) — EM has no direct database connection of its own

---

## Authentication to DataStorage (DD-AUTH-005 / DD-AUTH-014)

EM makes exactly one class of authenticated outbound call: to **DataStorage** — both to write audit events (`pkg/audit.BufferedStore`, wired via `audit.NewOpenAPIClientAdapter` in `cmd/effectivenessmonitor/main.go`) and to run its two read queries (pre-remediation-hash lookup, workflow-started/-completed checks, via `pkg/effectivenessmonitor/client/ds_querier.go`'s `ogenDataStorageQuerier`). Both paths authenticate identically, using the same shared in-process Bearer-token transport documented in [DD-AUTH-005](../../../architecture/decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md):

```go
// pkg/effectivenessmonitor/client/ds_querier.go — NewOgenDataStorageQuerier
// (byte-for-byte the same pattern as pkg/audit/openapi_client_adapter.go's
// NewOpenAPIClientAdapter, used for the write path)
baseTransport, _ := sharedtls.DefaultBaseTransportWithRetry() // TLS_CA_FILE-based, retry-wrapped (Issue #853)
transport := auth.NewAuthTransport(auth.NewDefaultTokenSource(), baseTransport)
httpClient := &http.Client{Timeout: timeout, Transport: transport}
```

1. `auth.NewDefaultTokenSource()` reads the pod's projected ServiceAccount token from `/var/run/secrets/kubernetes.io/serviceaccount/token` — the standard Kubernetes-issued, auto-rotated token, **not** a manually provisioned "DataStorage API key."
2. `AuthTransport.RoundTrip` injects `Authorization: Bearer <token>` on every outbound request to DataStorage, cloning the request rather than mutating the caller's.
3. On the receiving side, per [DD-AUTH-014](../../../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md), DataStorage validates the token via Kubernetes `TokenReview`, then authorizes via `SubjectAccessReview` against the synthetic resource `services/data-storage-service` — exactly what the `effectivenessmonitor-data-storage-client` RoleBinding above exists to authorize. `401` means the token was missing/invalid; `403` means the token was valid but the RoleBinding is missing.
4. There is **no manually provisioned credential Secret** for DataStorage access, and no `HOLMESGPT_API_KEY_PATH`-style env var anywhere in EM.

**Transport layer**: one-way TLS only — the client verifies DataStorage's server certificate against the CA at `$TLS_CA_FILE`. There is no mutual TLS (mTLS) by default.

---

## Authentication to Prometheus and AlertManager

Unlike DataStorage, Prometheus and AlertManager are **optional, best-effort enrichment** dependencies (`external.prometheusEnabled`/`external.alertManagerEnabled`), and their auth story is deployment-dependent. Both share one `*http.Client` built once in `cmd/effectivenessmonitor/main.go`'s `buildExternalHTTPClient` and passed into `pkg/effectivenessmonitor/client/prometheus_http.go`'s `NewPrometheusHTTPClient` and `alertmanager_http.go`'s `NewAlertManagerHTTPClient` — **neither client type constructs its own `http.Client` or reads any credential itself**, so all auth/TLS policy is centralized in one place:

| `external.tlsCaFile` (config) | Resulting behavior |
|---|---|
| **Unset** (default — typical for a plain in-cluster Prometheus/AlertManager `Service`) | Plain `http.Client{Timeout: connectionTimeout}` — no TLS verification, no `Authorization` header |
| **Set** (typical for OpenShift's cluster-monitoring stack / Thanos-querier, which terminates TLS with a service-serving certificate and requires a Bearer token) | A CA-trusting transport built from a hot-reloadable CA bundle at the configured path (`sharedtls.NewCAReloader`, Issue #756), wrapped with the **same** `auth.NewAuthTransport(auth.NewDefaultTokenSource(), ...)` ServiceAccount-token transport used for DataStorage (Issue #452) |

**Why this matters operationally**: on OpenShift, `external.tlsCaFile` typically points at the cluster-monitoring service-serving CA, and EM's ServiceAccount must additionally be authorized (via a `cluster-monitoring-view`-style binding, granted outside this chart) to read Thanos-querier metrics — that authorization is cluster-admin territory, not part of the `effectivenessmonitor-controller` ClusterRole documented above.

---

## Network Policies

`charts/kubernaut/templates/effectivenessmonitor/networkpolicy.yaml` (rendered name: `<kubernaut.fullname>-effectivenessmonitor`, via the shared `kubernaut.np.metadata` helper):

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: <kubernaut.fullname>-effectivenessmonitor
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: effectivenessmonitor-controller
  policyTypes: [Ingress, Egress]
  ingress:
    # Metrics scrape — only rendered when networkPolicies.monitoring.namespace
    # is configured; otherwise there is NO ingress rule at all. Health/readiness
    # probes come from the kubelet on the node, which NetworkPolicy cannot
    # restrict, so no explicit kube-system ingress rule is needed for them.
    - ports: [{port: 9090, protocol: TCP}]
      from:
        - namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: "<monitoring namespace>"}}
  egress:
    # DNS + Kubernetes API server (kubernaut.np.commonEgress)
    - ports: [{port: 53, protocol: UDP}, {port: 53, protocol: TCP}]
      to: [{namespaceSelector: {matchLabels: {kubernetes.io/metadata.name: kube-system}}}]
    - ports: [{port: "<discovered/configured API server port>", protocol: TCP}]
      to: ["<one ipBlock per control-plane API server endpoint>"]
    # DataStorage — audit writes + the 2 read queries (kubernaut.np.datastorageEgress)
    - ports: [{port: 8080, protocol: TCP}]
      to: [{podSelector: {matchLabels: {app: datastorage}}}]
    # Prometheus / AlertManager (ports from values: networkPolicies.monitoring.{prometheusPort,alertManagerPort})
    - ports: [{port: "<prometheusPort>", protocol: TCP}]
    - ports: [{port: "<alertManagerPort>", protocol: TCP}]
    # Fleet federation only (global.fleet.enabled), added to close a gap
    # found in Issue #1755 (DD-TEST-015 RCA): OAuth2 token fetch to the IdP
    # (kubernaut.np.idpEgress) and direct egress to the MCP Gateway itself
    # for fleet.ReaderFactory's remote-cluster reads (kubernaut.np.mcpGatewayEgress)
```

Reciprocally, DataStorage's own NetworkPolicy (`charts/kubernaut/templates/datastorage/networkpolicy.yaml`) explicitly allows ingress from pods labeled `app: effectivenessmonitor-controller`.

**What is *not* in this policy** (corrections vs. the fictional v1.0 spec):

- ❌ **No ingress from a "Context API Service" or "kubernaut-agent-service" pod on port 8080.** EM has no such inbound port — Context API was deprecated ([DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md)) and Kubernaut Agent never called EM.
- ❌ **No egress to an "Infrastructure Monitoring Service" on port 8094.** That service was fictional.
- ✅ Metrics ingress is **conditional** on `networkPolicies.monitoring.namespace` being configured — not unconditionally open to any pod labeled `app: prometheus` in a hardcoded `monitoring` namespace.

---

## Pod & Container Security Context

`charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml` Deployment, via the shared `kubernaut.podSecurityContext`/`kubernaut.containerSecurityContext` helpers (`charts/kubernaut/templates/_helpers.tpl`):

```yaml
spec:
  template:
    spec:
      serviceAccountName: effectivenessmonitor-controller
      securityContext:            # kubernaut.podSecurityContext defaults
        runAsNonRoot: true
        seccompProfile: {type: RuntimeDefault}
      containers:
        - name: controller
          securityContext:        # kubernaut.containerSecurityContext defaults
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities: {drop: [ALL]}
          args: ["--config=/etc/effectivenessmonitor/effectivenessmonitor.yaml"]
          env:
            - name: TLS_CA_FILE
              value: "<inter-service TLS CA bundle path>"   # kubernaut.interServiceTLS.caFile
          ports:
            - {containerPort: 9090, name: metrics}
            - {containerPort: 8081, name: health}
          volumeMounts:
            - {name: config, mountPath: /etc/effectivenessmonitor}
            # + conditional: service-ca (Prometheus TLS CA), oauth2-credentials
            #   (Fleet OAuth2 client secret), shared inter-service TLS CA volume
      volumes:
        - name: config
          configMap: {name: effectivenessmonitor-config}
```

**No database credentials Secret by default** — EM has no direct database connection, so there is no `effectiveness-monitor-db-credentials`-style Secret to mount. The **only** Secret volume EM ever mounts is `oauth2-credentials`, and only when both `global.fleet.enabled` **and** Fleet OAuth2 are configured (`effectivenessmonitor.fleet.oauth2.credentialsSecretRef`, containing `client-id`/`client-secret` for the MCP Gateway token exchange).

**Why these settings**:
- **`runAsNonRoot` / `seccompProfile` / `capabilities.drop: [ALL]`**: the standard restricted Pod Security Standard shared across every Kubernaut service
- **`readOnlyRootFilesystem`**: immutable container filesystem — EM writes nothing to local disk except through its config-file mount and (transiently) the CA-reloader's in-memory cache
- **No `HOLMESGPT_API_KEY_PATH`/`HOLMESGPT_ENDPOINT` env vars or Secret volume**: there is no such credential — DataStorage/Prometheus/AlertManager URLs are plain config values (`internal/config/effectivenessmonitor/config.go`), and authentication (where it exists) is the ServiceAccount token described above

---

## Sensitive Data Handling

EM logs component scores, correlation IDs, and phase transitions (see [Observability & Logging](./observability-logging.md)) but never logs a ServiceAccount token, a CA bundle's contents, or Fleet OAuth2 client credentials — `AuthTransport`/`auth.NewDefaultTokenSource()` only ever read the token to set the `Authorization` header, never to log it. Effectiveness scores and audit-event payloads are not treated as secret; they are the intended, documented output of the service (see the [Audit Event Catalog](./security/AUDIT_EVENT_CATALOG.md)).

---

## What Is NOT in This Security Model

The v1.0 draft of this document described a security model for a service that was never built. None of the following exist anywhere in `pkg/effectivenessmonitor/`, `internal/controller/effectivenessmonitor/`, or the Helm chart:

| Previous claim | Current reality |
|---|---|
| Kubernetes TokenReviewer `AuthMiddleware()` protecting a port-8080 REST API | No port 8080, no HTTP business API, no inbound auth middleware of any kind. `/metrics` (9090) and `/healthz`/`/readyz` (8081) are unauthenticated at the application layer, restricted only by NetworkPolicy |
| `HolmesGPTClient` (`pkg/monitor/holmesgpt_client.go`) authenticating with a ServiceAccount token to `POST /api/v1/postexec/analyze` | Never implemented. That endpoint is ⚠️ **Planned V1.1 — NOT YET IMPLEMENTED** ([DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md) §"V1.1 Scope: Level 2"); EM has zero AI/LLM/Kubernaut Agent client code in V1.0 |
| `effectiveness-monitor-holmesgpt-client` ClusterRole + `kubernaut-agent-postexec-analyzer` cross-service RBAC pair | No such RBAC exists on either side |
| 50 req/s rate-limiting middleware (`RateLimitMiddleware`) | No rate limiting anywhere — EM has no inbound business requests to rate-limit |
| Direct PostgreSQL connection (`ConnectToDataStorage`, `sql.Open("postgres", ...)`) + `effectiveness-monitor-db-credentials` Secret | No database connection anywhere in EM. All persistence goes through DataStorage's audit API, authenticated as [described above](#authentication-to-datastorage-dd-auth-005--dd-auth-014) |
| `BR-INS-Security`/`BR-INS-Performance` requirement tags | ⚠️ **STALE** — no `BR-INS-*` requirement document backs the current implementation (see the sibling STALE-ID note in [overview.md](./overview.md#business-requirements-coverage-v10)); this document does not invent a replacement mapping |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [Overview](./overview.md) | Service purpose, architecture, component scorers |
| [CRD Schema](./crd-schema.md) | `EffectivenessAssessment` spec/status fields + RBAC summary |
| [Integration Points](./integration-points.md) | Upstream/downstream services, audit event contract |
| [Testing Strategy](./testing-strategy.md) | Real unit/integration/E2E test suite |
| [DD-AUTH-005](../../../architecture/decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md) | ServiceAccount Bearer-token client authentication pattern |
| [DD-AUTH-014](../../../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md) | TokenReview + SubjectAccessReview middleware pattern (DataStorage-side) |
| [DD-017](../../../architecture/decisions/DD-017-effectiveness-monitor-v1.1-deferral.md) | V1.0 Level 1 / V1.1 Level 2 scoping — authoritative status of the `postexec/analyze` endpoint |
| [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md) | Context API deprecation (why EM has no such upstream client/ingress rule) |

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: 2026-08-02
