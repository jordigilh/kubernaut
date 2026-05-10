# Production Deployment Guide

## Prerequisites

- Kubernetes cluster (1.29+) with CRD support
- Helm 3.12+
- OIDC identity provider (Keycloak, Okta, or similar) configured
- Kubernaut Agent (KA) and DataStorage (DS) services deployed
- Prometheus Operator (for PrometheusRule CRD)
- Prometheus instance reachable from AF pods (for severity triage — `/api/v1/alerts`, `/api/v1/rules`, `/api/v1/query`)

## Deployment Model

The API Frontend workload (Deployment, Service, ConfigMap) is managed by the **kubernaut-operator** in production environments. The Helm chart in this repository (`deploy/helm/`) provides **RBAC resources only** (ServiceAccount, ClusterRole, ClusterRoleBinding).

For development or standalone environments without the operator, deploy the workload using raw manifests or a parent chart that includes this chart as a dependency.

## Resource Requirements

### Aspirational (pre-validation)

| Resource | Request | Limit | Notes |
|----------|---------|-------|-------|
| CPU | 100m | 500m | Scale up for >50 concurrent users |
| Memory | 128Mi | 512Mi | Audit buffer + rate limiter maps |
| Replicas | 2 | - | HA with pod anti-affinity |

### Storage

No persistent storage required. All state is held in:
- K8s CRDs (InvestigationSession) — etcd
- In-memory session state (ephemeral per pod)
- DataStorage (external service for audit trail)

## Helm Chart (RBAC Only)

The chart at `deploy/helm/` installs Kubernetes RBAC resources required by the AF service account:

```bash
helm install af-rbac deploy/helm/ \
  --namespace kubernaut \
  --create-namespace
```

This creates:
- **ServiceAccount** — pod identity for AF
- **ClusterRole** — CRD CRUD, TokenReview, SubjectAccessReview
- **ClusterRoleBinding** — binds the role to the service account

The chart does **not** create Deployment, Service, or ConfigMap resources. Those are managed by the kubernaut-operator (see `docs/design/ARCHITECTURE.md`).

## Configuration

Mount a ConfigMap at `/etc/apifrontend/config.yaml`:

```yaml
server:
  port: 8443

agent:
  gcpProject: "<YOUR_PROJECT>"
  gcpRegion: "us-central1"
  kaBaseURL: "http://kubernaut-agent:8080"
  kaMCPEndpoint: "http://kubernaut-agent:8080/api/v1/mcp/"
  dsBaseURL: "http://data-storage:9090"

auth:
  issuerURL: "https://keycloak.example.com/realms/kubernaut"
  audience: "kubernaut-apifrontend"

mcp:
  enabled: true

agentCard:
  url: "https://apifrontend.example.com"

severityTriage:
  enabled: true
  prometheusURL: "http://prometheus-operated:9090"
  cacheTTLSeconds: 30
  maxQueriesPerCall: 10
  maxRulesEvaluated: 100
  llmConfidence: 0.7
  # prometheus:
  #   tlsCaFile: "/etc/pki/tls/certs/prometheus-ca.crt"
  #   bearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token"

logging:
  level: "INFO"

rateLimit:
  ipRequestsPerSec: 100
  userRequestsPerSec: 50
  maxConcurrentSessions: 5
  toolCallsPerMinute: 60

resilience:
  ka:
    connectTimeout: 5s
    requestTimeout: 30s
    cbMaxRequests: 3
    cbInterval: 10s
    cbTimeout: 30s
    cbFailureThreshold: 5
    retryMax: 2
    retryInitBackoff: 500ms
    retryMaxBackoff: 5s
    retryableStatuses: [502, 503, 504]
  ds:
    connectTimeout: 3s
    requestTimeout: 10s
    cbMaxRequests: 3
    cbInterval: 10s
    cbTimeout: 15s
    cbFailureThreshold: 3
    retryMax: 3
    retryInitBackoff: 200ms
    retryMaxBackoff: 3s
    retryableStatuses: [502, 503, 504]
  k8s:
    connectTimeout: 5s
    requestTimeout: 30s
    cbMaxRequests: 3
    cbInterval: 10s
    cbTimeout: 30s
    cbFailureThreshold: 5
```

## Health Probes

| Probe | Path | Expected Response |
|-------|------|-------------------|
| Liveness | `/healthz` | 200 (always, unless process is dead) |
| Readiness | `/readyz` | 200 when all dependencies are reachable |
| Metrics | `/metrics` | Prometheus exposition format |

## Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 8443 | HTTP | Main API (A2A, MCP, Agent Card, health) |

## RBAC

The Helm chart creates:
- **ServiceAccount** — identity for the AF pod
- **ClusterRole** — CRD CRUD, TokenReview, SubjectAccessReview
- **ClusterRoleBinding** — binds role to service account

### Agent Card RBAC (Group-to-Role Mapping)

To enable per-persona skill filtering on the Agent Card endpoint, add the `rbac.groupMapping` section to the ConfigMap. This maps OIDC group names (from JWT `groups` claim) to AF role keys defined in `internal/agent/rbac_roles.yaml`:

```yaml
rbac:
  groupMapping:
    platform-sre-team: sre
    github-actions-bot: cicd
    monitoring-agents: observability
    change-approval-board: remediation-approver
    security-audit-team: l3-audit
```

When configured:
- **Unauthenticated** requests to `/.well-known/agent-card.json` receive a shell card (metadata only, no skills)
- **Authenticated** requests receive a persona-filtered skills list based on the caller's groups

When `rbac.groupMapping` is omitted, the Agent Card still applies RBAC filtering using the embedded role definitions — JWT groups must match role keys directly (e.g., group `sre` maps to role `sre`).

## Hot-Reloadable Configuration

The following fields are dynamically reloaded without restart:
- `logging.level` — via atomic zap level
- `rateLimit.ipRequestsPerSec` — via SetLimit/SetBurst on existing limiters
- `rateLimit.userRequestsPerSec` — via SetLimit/SetBurst
- `rateLimit.toolCallsPerMinute` — via SetLimit/SetBurst

Fields requiring pod restart: auth, resilience CB thresholds, server.port, agent endpoints.

## High Availability

For production HA:
1. Set `replicaCount: 2` (or more)
2. Configure pod anti-affinity (spread across nodes/AZs)
3. Configure PodDisruptionBudget with `minAvailable: 1`
4. Enable HPA based on CPU or custom metrics (SSE connection count)

## Monitoring

Deploy `deploy/prometheus-rules.yaml` as a PrometheusRule CR:
```bash
kubectl apply -f deploy/prometheus-rules.yaml
```

Import Grafana dashboards from `docs/grafana/` (when available).
