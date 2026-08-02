## Security Configuration

> **Rewritten** ([#1806](https://github.com/jordigilh/kubernaut/issues/1806)): the previous version
> of this document described KA authentication via a static API key read from
> `HOLMESGPT_API_KEY_PATH`/`HOLMESGPT_ENDPOINT`, a Go client at `pkg/aianalysis/client/holmesgpt.go`,
> and full RBAC ownership of an `aiapprovalrequests` CRD — none of which match the current
> implementation. See [What Changed](#what-changed-in-this-rewrite) at the end of this document.

### ServiceAccount & RBAC Least Privilege

**ServiceAccount and ClusterRole** (from `internal/controller/aianalysis/aianalysis_controller.go`
kubebuilder markers and `charts/kubernaut/templates/aianalysis/aianalysis.yaml`, which is what is
actually deployed):

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
automountServiceAccountToken: true
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aianalysis-controller
rules:
# AIAnalysis CRD permissions (owns these)
- apiGroups: ["kubernaut.ai"]
  resources: ["aianalyses"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["aianalyses/status"]
  verbs: ["get", "update", "patch"]

# InvestigationSession CRD permissions (BR-INTERACTIVE-010: checks for active sessions,
# sets Active/Completed/Failed phase on takeover/completion — full read + status write,
# not the owning controller)
- apiGroups: ["kubernaut.ai"]
  resources: ["investigationsessions"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["investigationsessions/status"]
  verbs: ["get", "update", "patch"]

# Event emission (write-only)
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]

# controller-runtime leader election (#687)
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: aianalysis-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: aianalysis-controller
subjects:
- kind: ServiceAccount
  name: aianalysis-controller
  namespace: kubernaut-system
```

**Two additional client RoleBindings** grant this same ServiceAccount permission to call
external Kubernaut services — not through Kubernetes resource verbs on those services'
CRDs, but through a synthetic-resource SAR pattern (DD-AUTH-014, see
[Authentication](#authentication-to-kubernaut-agent-and-data-storage-dd-auth-014-dd-auth-005)
below):

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: aianalysis-controller-kubernaut-agent-access
  namespace: kubernaut-system
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: kubernaut-agent-client}
subjects:
- {kind: ServiceAccount, name: aianalysis-controller, namespace: kubernaut-system}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: aianalysis-controller-datastorage-access
  namespace: kubernaut-system
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: data-storage-client}
subjects:
- {kind: ServiceAccount, name: aianalysis-controller, namespace: kubernaut-system}
```

**What is *not* in this RBAC** (corrections vs. earlier documentation):

- ❌ **No `aiapprovalrequests` permissions of any kind.** That CRD name was never implemented.
  The real approval CRD is `RemediationApprovalRequest` (`api/remediation/v1alpha1`, ADR-040),
  and it is created/owned by **RemediationOrchestrator**, not AIAnalysis — AIAnalysis only ever
  sets `status.approvalRequired`/`approvalReason`/`approvalContext` on its own CRD.
- ❌ **No `remediationrequests`/`alertremediations` read permission.** AIAnalysis is a
  self-contained CRD (DD-CONTRACT-002) — all data needed for analysis is copied into
  `AIAnalysis.spec` by RO at creation time, so the controller never fetches the parent
  `RemediationRequest` from the API server during reconciliation.
- ❌ **No `configmaps` RBAC rule.** The Rego policy ConfigMap (`aianalysis-policies`) is
  mounted as a projected volume at pod-start (`/etc/aianalysis/policies/approval.rego`) and
  hot-reloaded from the filesystem (`pkg/shared/hotreload`), not read via `client-go` `Get`
  calls at runtime — so no `configmaps` `get`/`list`/`watch` verb is needed.
- ❌ **No cluster-scoped `apiGroups: ["aianalysis.kubernaut.io"]`.** The real API group for
  every Kubernaut CRD, including `aianalyses`, is the unified `kubernaut.ai` (DD-CRD-001) —
  the `aianalysis.kubernaut.io` / `approval.kubernaut.io` / `remediation.kubernaut.io` groups
  in earlier drafts of this document (and in `config/rbac/role.yaml`'s currently-stale
  `remediationrequests` entry) do not match any `+groupName` in `api/*/groupversion_info.go`.

**Least Privilege Principles**:
- ✅ Update/patch access to its own `AIAnalysis` CRD and status subresource only
- ✅ Read + status-write access to `InvestigationSession` (interactive takeover tracking)
- ✅ No Kubernetes Secret or ConfigMap API access — both are filesystem-mounted
- ✅ Data Storage access is real but write-only and indirect: AIAnalysis emits SOC2/FedRAMP
  audit events (BR-AUDIT-005, DD-AUDIT-003) through a buffered async client
  (`sharedaudit.NewBufferedStore`), not ad-hoc reads/writes against a database

---

### Authentication to Kubernaut Agent and Data Storage (DD-AUTH-014, DD-AUTH-005)

AIAnalysis makes outbound calls to exactly two backends — **Kubernaut Agent (KA)** for
investigation/workflow-selection, and **Data Storage** for audit event persistence
(BR-AUDIT-005) — and authenticates to both the same way. Neither uses a static API key. Both
authenticate as the pod's Kubernetes **ServiceAccount**, via the shared in-process Bearer-token
transport (`pkg/shared/auth`) documented for the Data Storage case in
[DD-AUTH-005](../../../architecture/decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md)
(scope: "7 Go Services... AIAnalysis... All DataStorage API Endpoints") and reused verbatim for
the KA client:

```go
// pkg/agentclient/client.go — NewKubernautAgentClient (identical pattern in
// pkg/audit/openapi_client_adapter.go's NewOpenAPIClientAdapter for the Data Storage client)
baseTransport, _ := sharedtls.DefaultBaseTransportWithRetry()      // TLS_CA_FILE-based, retry-wrapped
transport := auth.NewAuthTransport(auth.NewDefaultTokenSource(), baseTransport)
httpClient := &http.Client{Timeout: cfg.Timeout, Transport: transport}
```

1. `auth.NewDefaultTokenSource()` reads the pod's projected ServiceAccount token from
   `/var/run/secrets/kubernetes.io/serviceaccount/token` (standard Kubernetes-issued, auto-rotated
   token — **not** a manually-provisioned "KA API key" Secret).
2. `AuthTransport.RoundTrip` injects `Authorization: Bearer <token>` on every outbound request to
   both KA and Data Storage, cloning the request rather than mutating the caller's.
3. The token is cached for 5 minutes and invalidated automatically on a `401` response from the
   callee, forcing a re-read from disk on the next call — no manual credential rotation logic is
   needed in AIAnalysis.
4. **`403` does *not* invalidate the cache** — a `403` means the token is valid but the
   ServiceAccount lacks permission (RBAC issue), not that the token expired; re-reading the same
   token would not help.

On the receiving side, per [ADR-045](../../../architecture/decisions/ADR-045-aianalysis-ka-service-contract.md)
("Authentication: Bearer token, validated by KA middleware per DD-AUTH-014"), this Bearer token is
validated by
[DD-AUTH-014](../../../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md)
application middleware — **not** an `ose-oauth-proxy` sidecar (DD-AUTH-014 explicitly replaced
that OpenShift-specific sidecar model for KA, DataStorage, Gateway, and AIAnalysis's own inbound
side): Kubernetes `TokenReview` for authentication, then `SubjectAccessReview` (SAR) against a
synthetic resource for authorization. The chart wires this up via two dedicated client
ClusterRoles, each bound with a namespaced RoleBinding (already shown above): `kubernaut-agent-client`
(`services`/`kubernaut-agent`, verbs `create`/`get`) and `data-storage-client`
(`services`/`data-storage-service`, verbs `create`/`get`/`list`/`update`/`delete`). Neither rule
is used for a real Kubernetes API call — each exists purely so the callee's SAR check has a
concrete resource to authorize against (e.g. "can this ServiceAccount `get` the synthetic
resource `services/kubernaut-agent`?"). `401 Unauthorized` means the Bearer token was
missing/invalid/expired; `403 Forbidden` means the token was valid but the corresponding
RoleBinding is missing.

**DD-AUTH-014 changelog note** (v3.0): AIAnalysis's `kubernaut-agent-client` RBAC was itself a
historical bug fix — it was originally granted to Gateway (which has zero KA client code) instead
of AIAnalysis (the actual caller), causing the 21 AIAnalysis E2E auth failures that prompted the
correction alongside adding `automountServiceAccountToken: true`.

**Transport layer**: `sharedtls.DefaultBaseTransportWithRetry()` gives one-way TLS (the client
verifies the callee's server certificate against the CA at `$TLS_CA_FILE`) plus automatic retry on
transient failures. There is **no mutual TLS (mTLS)** by default — `pkg/shared/tls.WithClientCert`
exists in the shared library for services that need it, but neither `NewKubernautAgentClient` nor
the Data Storage adapter use it, so AIAnalysis presents no client certificate to either backend.

**🚨 Secret handling that still applies**:
- ❌ The ServiceAccount token is never logged, written to CRD status, or included in events —
  `AuthTransport` only ever reads it to set the `Authorization` header.
- ❌ Rego policy contents are never logged (may contain sensitive approval rules) — only policy
  size and hash are logged (`GetPolicyHash()`, ADR-050/DD-INFRA-001).
- ✅ KA response bodies stored on `AIAnalysis.status` (RCA summary, warnings, workflow rationale)
  are LLM-generated narrative text, not credentials — but are still subject to the general rule
  that untrusted signal-derived content (e.g. `SignalAnnotations`) is sanitized by KA's prompt
  builder before it ever reaches the LLM, not by AIAnalysis.

---

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: aianalysis-controller
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Health/readiness probes from kubelet
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 8081  # Health/Ready
  # Metrics scraping from Prometheus
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
      podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 9090  # Metrics
  egress:
  # Kubernetes API server
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    ports:
    - protocol: TCP
      port: 443
  # Kubernaut Agent
  - to:
    - podSelector:
        matchLabels:
          app: kubernaut-agent
    ports:
    - protocol: TCP
      port: 8443
  # Data Storage (BR-AUDIT-005 audit event writes, buffered/async)
  - to:
    - podSelector:
        matchLabels:
          app: datastorage
    ports:
    - protocol: TCP
      port: 8080
  # DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
```

**Why These Restrictions**:
- No external network access (all dependencies internal)
- Kubernaut Agent (KA) is an internal service — AIAnalysis makes no direct outbound calls to any
  external AI provider (OpenAI, Anthropic, etc.); that indirection lives entirely inside KA
- Data Storage egress exists but is one-directional and audit-only: AIAnalysis never queries
  Data Storage for reads, it only ships buffered audit events (`sharedaudit.NewBufferedStore`,
  DD-AUDIT-003) — see [Integration Points](./integration-points.md#service-dependencies)

---

### Security Context

**Pod Security Standards** (Restricted Profile), from
`charts/kubernaut/templates/aianalysis/aianalysis.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: aianalysis-controller
  template:
    metadata:
      labels:
        app: aianalysis-controller
    spec:
      serviceAccountName: aianalysis-controller
      securityContext:
        runAsNonRoot: true
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: aianalysis
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          capabilities:
            drop:
            - ALL
        env:
        - name: CONFIG_PATH
          value: /etc/aianalysis/config.yaml
        - name: TLS_CA_FILE
          value: "<inter-service CA bundle path>"   # kubernaut.interServiceTLS.caFile
        volumeMounts:
        - name: config
          mountPath: /etc/aianalysis
          readOnly: true
        - name: rego-policies
          mountPath: /etc/aianalysis/policies
          readOnly: true
        # + shared TLS CA volume mount (kubernaut.tlsCaVolumeMount)
      volumes:
      - name: config
        configMap:
          name: aianalysis-config
      - name: rego-policies
        configMap:
          name: aianalysis-policies
        # + shared TLS CA volume (kubernaut.tlsCaVolume)
```

**Why These Settings**:
- **runAsNonRoot / seccompProfile / drop ALL capabilities**: standard restricted pod security
  profile shared across Kubernaut services
- **readOnlyRootFilesystem**: immutable container filesystem
- **No `HOLMESGPT_API_KEY_PATH`/`HOLMESGPT_ENDPOINT` env vars or Secret volume mount**: KA's URL
  is a plain config value (`agent.url: https://kubernaut-agent:8443` in `aianalysis-config`), not
  a credential — authentication is the ServiceAccount token described above, mounted
  automatically by Kubernetes via `automountServiceAccountToken: true`, not via a
  service-specific projected Secret volume

---

## What Changed in This Rewrite

| Previous claim | Current reality |
|---|---|
| `aiapprovalrequests` full CRUD RBAC under `apiGroups: ["approval.kubernaut.io"]` | No such RBAC exists. `RemediationApprovalRequest` (`kubernaut.ai` group, ADR-040) is owned by RemediationOrchestrator; AIAnalysis has zero RBAC on it |
| `apiGroups: ["aianalysis.kubernaut.io"]` for its own CRD | `apiGroups: ["kubernaut.ai"]` (DD-CRD-001, unified group) |
| Read-only `remediation.kubernaut.io`/`alertremediations` RBAC for "parent reference" | No RemediationRequest RBAC at all — DD-CONTRACT-002 self-contained CRD means RO copies everything into spec at creation, no runtime read-back |
| `configmaps` `get`/`list`/`watch` RBAC for the Rego policy | Not needed — the policy ConfigMap is a projected volume mount, read from disk with hot-reload, not via the K8s API |
| KA credentials as an API key read from `HOLMESGPT_API_KEY_PATH`, sent however `HolmesGPTClient` chose to send it | Kubernetes ServiceAccount Bearer token (`pkg/shared/auth.AuthTransport`), validated by KA via TokenReview + SAR (DD-AUTH-005/DD-AUTH-014-style pattern) |
| `initHolmesGPTClient`, `sanitizeHolmesGPTResponse`, `pkg/aianalysis/client/holmesgpt.go` | Real client is `pkg/agentclient` (ogen-generated); no bespoke KA-response-sanitizing regex layer exists in that package |
| Projected Secret volume `holmesgpt-credentials` mounting `kubernaut-agent-credentials` | No credential Secret/volume for KA — auth token comes from the standard auto-mounted ServiceAccount token |
| No mention of TLS mode | One-way TLS via `TLS_CA_FILE` (`pkg/shared/tls.DefaultBaseTransportWithRetry`); no mTLS by default |

---

## Related Documents

| Document | Purpose |
|----------|---------|
| [Integration Points](./integration-points.md) | KA async API contract |
| [CRD Schema](./crd-schema.md) | Type definitions |
| [DD-AUTH-005](../../../architecture/decisions/DD-AUTH-005-datastorage-client-authentication-pattern.md) | ServiceAccount Bearer-token client authentication pattern |
| [DD-CRD-001](../../../architecture/decisions/DD-CRD-001-api-group-domain-selection.md) | Unified `kubernaut.ai` API group |
| [ADR-040](../../../architecture/decisions/ADR-040-remediation-approval-request-architecture.md) | RemediationApprovalRequest design |

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: August 2026
