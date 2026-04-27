# Kubernaut Agent — Security Configuration

> **Authoritative reference** for the RBAC, network, and secret posture of the
> Kubernaut Agent (KA / HolmesGPT API wrapper).
>
> Related: ADR-055, ADR-056, DD-AUTH-011, DD-AUTH-012, DD-AUTH-014

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

Grants **read-only** access to Kubernetes resources the KA investigation tools
require. The KA never creates, updates, or deletes workload resources.

| apiGroup | Resources | Verbs | Rationale |
|----------|-----------|-------|-----------|
| `""` (core) | pods, pods/log, events, services, endpoints, configmaps, secrets, nodes, namespaces, replicationcontrollers, persistentvolumeclaims, persistentvolumes, resourcequotas, serviceaccounts | get, list, watch | Core investigation: workload state, logs, events, config, auth |
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

### ClusterRoleBinding: kubernaut-agent-investigator-binding

Binds `kubernaut-agent-investigator` to `kubernaut-agent-sa`.

### Additional bindings (via other ClusterRoles)

| ClusterRole | Grants | Purpose |
|-------------|--------|---------|
| `data-storage-auth-middleware` | tokenreviews (authentication.k8s.io), subjectaccessreviews (authorization.k8s.io) | SAR-based auth middleware for DataStorage calls |
| `cluster-monitoring-view` (OCP only) | Read Prometheus/Thanos metrics via OpenShift routes | OCP monitoring access (deprecated v1.4, removed v1.5) |

### Design principles

- **Least privilege**: Read-only verbs only; no write access to any workload resource
- **Dynamic client awareness**: The LLM may request any resource kind via `kubectl_*` tools — missing RBAC causes degraded RCA quality (silent tool failures)
- **Explicit exclusions**: RBAC enumeration (`rbac.authorization.k8s.io/*`) is intentionally excluded for security; `leases`, `limitranges`, `priorityclasses` excluded for low investigation value

---

## Considered but excluded

| Resource | Why |
|----------|-----|
| `rbac.authorization.k8s.io/*` | Security-sensitive; agent should not enumerate cluster RBAC |
| `scheduling.k8s.io/priorityclasses` | Scheduling issues surface via pod events |
| `coordination.k8s.io/leases` | Internal controller detail; low investigation value |
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
    # API calls from AIAnalysis controller
    - from:
        - podSelector:
            matchLabels:
              app: aianalysis-controller
      ports:
        - protocol: TCP
          port: 8080
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
          port: 8080
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
- Ingress limited to health probes, metrics scraping, and AIAnalysis controller calls
- Egress to K8s API (investigation), LLM provider (external HTTPS), DataStorage (audit), Prometheus (tools), and DNS
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
- HolmesGPT responses are sanitized before storage (regex-based secret pattern removal)
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
| v1.4 | Added storage.k8s.io, discovery.k8s.io, ingresses, persistentvolumes, serviceaccounts | #872 |
| v1.3 | Added metrics.k8s.io (pods, nodes) | #770 |
| v1.2 | Initial ClusterRole with core K8s + third-party CRDs | — |
