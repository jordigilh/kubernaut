# Kubernaut Agent — Security Configuration

> **Authoritative reference** for the RBAC, network, and secret posture of the
> Kubernaut Agent (KA).
>
> Related: ADR-055, ADR-056, DD-AUTH-011, DD-AUTH-012, DD-AUTH-014, [DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md)

---

## ServiceAccount & RBAC

### ServiceAccount

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kubernaut-agent-sa
  namespace: kubernaut-system
```

### ClusterRole: kubernaut-agent-investigator

Grants **read-only** access to most Kubernetes resources the KA investigation
tools require. The KA never creates, updates, or deletes workload resources,
with the sole exception of `events` (used for emitting K8s events during
investigation lifecycle tracking).

| apiGroup | Resources | Verbs | Rationale |
|----------|-----------|-------|-----------|
| `""` (core) | pods, pods/log, services, endpoints, configmaps, secrets, nodes, namespaces, replicationcontrollers, persistentvolumeclaims, persistentvolumes, resourcequotas, serviceaccounts | get, list, watch | Core investigation: workload state, logs, config, auth |
| `""` (core) | events | get, list, watch, create, patch | Investigation lifecycle events |
| `apps` | deployments, replicasets, statefulsets, daemonsets | get, list, watch | Owner-chain traversal, rollout status |
| `batch` | jobs, cronjobs | get, list, watch | Batch workload investigation |
| `events.k8s.io` | events | get, list, watch | Structured event API |
| `policy` | poddisruptionbudgets | get, list, watch | PDB-deadlock detection (ADR-056) |
| `networking.k8s.io` | networkpolicies, ingresses | get, list, watch | Network connectivity investigation |
| `autoscaling` | horizontalpodautoscalers | get, list, watch | HPA target/status (ADR-056) |
| `storage.k8s.io` | storageclasses, csidrivers, csinodes, volumeattachments | get, list, watch | PV/CSI troubleshooting (#872) |
| `discovery.k8s.io` | endpointslices | get, list, watch | Modern service routing (K8s 1.21+) |
| `metrics.k8s.io` | pods, nodes | get, list | Real-time resource usage (#770) |
| `cert-manager.io` | certificates, clusterissuers, certificaterequests | get, list, watch | TLS/cert investigation |
| `argoproj.io` | applications | get, list, watch | GitOps sync status |
| `policy.linkerd.io` | servers, authorizationpolicies, meshtlsauthentications | get, list, watch | Service mesh policy |
| `security.istio.io` | authorizationpolicies, peerauthentications, requestauthentications | get, list, watch | Istio security |
| `networking.istio.io` | virtualservices, destinationrules, gateways, serviceentries | get, list, watch | Istio routing |
| `monitoring.coreos.com` | prometheusrules, servicemonitors, podmonitors, probes | get, list, watch | Prometheus Operator CRDs |
| `kubernaut.ai` | `agentsessions` | get, list, watch, update, patch | **AA↔KA dispatch channel** ([DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md)): the `Dispatcher` Reconciler watches for Create/Update and metadata-patches the `dispatchCleanupFinalizer` on the base resource. KA never writes `Spec` (immutable after Create) |
| `kubernaut.ai` | `agentsessions/status` | get, update, patch | KA is the **sole writer** of `Status` (BR-AA-KA-065.9) — AA and AF only ever watch it |
| `kubernaut.ai` | `investigationsessions` | get, list, watch | Read-only, dispatch-time existence check ONLY ([DD-AA-KA-001 Amendment "Gap 1"](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md)) — KA never writes `InvestigationSession`, and uses it for nothing but deciding autonomous-vs-interactive dispatch (BR-AA-KA-065.5) |

### ClusterRoleBinding: kubernaut-agent-investigator-binding

Binds `kubernaut-agent-investigator` to `kubernaut-agent-sa`.

### Role (namespace-scoped): kubernaut-agent-leases

Distinct from the cluster-scoped investigator role above — least-privilege, namespace-scoped
(HELM-03, #703). Used by two independent Lease consumers, both unconditional (not gated on
`interactive.enabled`) as of DD-AA-KA-001:

| apiGroup | Resources | Verbs | Rationale |
|----------|-----------|-------|-----------|
| `coordination.k8s.io` | `leases` | get, list, create, update, delete | (1) the `AgentSession` dispatch Lease (`dispatch-<agentsession-name>`), used by every investigation, autonomous or interactive ([DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md)); (2) the pre-existing interactive-driver Lease (per-`RemediationRequest`, one human driver at a time). `list` is required by both the dispatch Lease's stale-Lease reclaim check and the interactive-driver Lease's startup `ReconcileOrphanedLeases` loop |

### Additional bindings (via other ClusterRoles)

| ClusterRole | Grants | Purpose |
|-------------|--------|---------|
| `data-storage-auth-middleware` | tokenreviews (authentication.k8s.io), subjectaccessreviews (authorization.k8s.io) | SAR-based auth middleware for DataStorage calls |
| `cluster-monitoring-view` (OCP only) | Read Prometheus/Thanos metrics via OpenShift routes | OCP monitoring access (deprecated v1.4, removed v1.5) |

### Design principles

- **Least privilege**: Read-only verbs for workload resources; write access limited to `events` for investigation lifecycle tracking
- **Dynamic client awareness**: The LLM may request any resource kind via `kubectl_*` tools — missing RBAC causes degraded RCA quality (silent tool failures)
- **Explicit exclusions**: RBAC enumeration (`rbac.authorization.k8s.io/*`) is intentionally excluded for security; `leases`, `limitranges`, `priorityclasses` excluded for low investigation value
- **Secrets — detective control, not RBAC narrowing** (BR-AUDIT-011, GAP-13, Issue #1505): the broad `secrets` grant above is deliberately *not* narrowed, since a missing permission degrades RCA quality silently. Instead, every Get/List that resolves to the core `Secret` resource emits a dedicated `aiagent.secret.accessed` audit event (verb, namespace, secret name, outcome), independent of the generic `aiagent.llm.tool_call` event already emitted for every tool call — see [BR-AUDIT-011](../../requirements/BR-AUDIT-011-kubernautagent-secret-read-audit.md).

---

## Considered but excluded

| Resource | Why |
|----------|-----|
| `rbac.authorization.k8s.io/*` | Security-sensitive; agent should not enumerate cluster RBAC |
| `scheduling.k8s.io/priorityclasses` | Scheduling issues surface via pod events |
| `coordination.k8s.io/leases` (cluster-scoped investigator role) | Internal controller detail; low investigation value. **Note**: KA *is* granted `leases` get/list/create/update/delete, but only via the separate namespace-scoped `kubernaut-agent-leases` Role for its own dispatch/interactive-driver Leases — see [Role: kubernaut-agent-leases](#role-namespace-scoped-kubernaut-agent-leases) above, not for investigating *other* workloads' Leases |
| `limitranges` | Resource constraints surface via pod events/conditions |

---

## Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: kubernaut-agent
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: kubernaut-agent
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
          port: 8081
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
          port: 9090
    # MCP calls from apifrontend (separate channel, unaffected by
    # DD-AA-KA-001 -- AIAnalysis no longer calls KA over HTTP/TCP at all;
    # it dispatches via the AgentSession CRD through the Kubernetes API
    # server, which is not subject to this pod-level NetworkPolicy)
    - from:
        - podSelector:
            matchLabels:
              app: apifrontend
      ports:
        - protocol: TCP
          port: 8443
  egress:
    # Kubernetes API server
    - to:
        - namespaceSelector:
            matchLabels:
              name: kube-system
      ports:
        - protocol: TCP
          port: 443
    # LLM provider (external)
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
            except:
              - 10.0.0.0/8
              - 172.16.0.0/12
              - 192.168.0.0/16
      ports:
        - protocol: TCP
          port: 443
    # Data Storage Service (audit)
    - to:
        - podSelector:
            matchLabels:
              app: data-storage-service
      ports:
        - protocol: TCP
          port: 8443
    # Prometheus (tools)
    - to:
        - namespaceSelector:
            matchLabels:
              name: monitoring
          podSelector:
            matchLabels:
              app: prometheus
      ports:
        - protocol: TCP
          port: 9090
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

**Why these restrictions:**
- Ingress limited to health probes, metrics scraping, and apifrontend's MCP calls. AIAnalysis
  dispatches KA via the `AgentSession` CRD ([DD-AA-KA-001](../../architecture/decisions/DD-AA-KA-001-agentsession-crd-http-removal.md)) through the Kubernetes API server —
  not a direct pod-to-pod connection — so it does not appear as an ingress rule here
- Egress to K8s API (investigation + `AgentSession`/`InvestigationSession` watch and Status writes), LLM provider (external HTTPS), DataStorage (audit), Prometheus (tools), and DNS
- No arbitrary internal traffic

---

## Secret Management

**LLM credentials** are stored in a Kubernetes Secret and mounted as a projected volume:

```yaml
volumes:
  - name: llm-credentials-file
    secret:
      secretName: <kubernautAgent.llm.credentialsSecretName>
```

**Secret handling rules:**
- LLM API keys are NEVER logged, stored in CRD status, or emitted in events
- Only connection status (success/failure) is logged
- KA responses are sanitized before storage (regex-based secret pattern removal)
- OAuth2 credentials (when enabled) sourced from Secret via `valueFrom.secretKeyRef`

---

## Security Context

The KA deployment follows the **Restricted** Pod Security Standard:

- `runAsNonRoot: true`
- `readOnlyRootFilesystem: true`
- `allowPrivilegeEscalation: false`
- `capabilities.drop: [ALL]`
- `seccompProfile.type: RuntimeDefault`

---

## Changelog

| Version | Change | Issue |
|---------|--------|-------|
| v1.5 | Added `agentsessions`/`agentsessions/status`/`investigationsessions` RBAC (AgentSession CRD dispatch) and the namespace-scoped `kubernaut-agent-leases` Role; replaced stale AIAnalysis-controller HTTP ingress with apifrontend's MCP ingress | #2260, DD-AA-KA-001 |
| v1.4 | Added storage.k8s.io, discovery.k8s.io, ingresses, persistentvolumes, serviceaccounts | #872 |
| v1.3 | Added metrics.k8s.io (pods, nodes) | #770 |
| v1.2 | Initial ClusterRole with core K8s + third-party CRDs | — |
