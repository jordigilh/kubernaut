# Kubernaut Helm Chart

Kubernaut is an autonomous Kubernetes remediation platform that detects incidents via Prometheus AlertManager and Kubernetes events, performs AI-powered root cause analysis, and executes automated remediation workflows with human-in-the-loop approval gates.

## Prerequisites

| Requirement | Version |
|---|---|
| Kubernetes | 1.32+ (selectableFields GA) |
| Helm | 3.12+ |
| kube-prometheus-stack | Recommended for AlertManager integration |

**Supported workflow execution engines** (at least one required):

- Kubernetes Jobs (built-in, no extra dependency)
- Tekton Pipelines
- Ansible Automation Platform (AAP) / AWX

## Quick Start

```bash
helm repo add kubernaut https://jordigilh.github.io/kubernaut
helm repo update

helm install kubernaut kubernaut/kubernaut \
  --namespace kubernaut-system --create-namespace \
  --set postgresql.auth.password=<password> \
  --set holmesgptApi.llm.provider=openai \
  --set holmesgptApi.llm.model=gpt-4o
```

## Installation

### From OCI Registry

```bash
helm install kubernaut oci://ghcr.io/jordigilh/kubernaut/charts/kubernaut \
  --version 1.0.0 \
  --namespace kubernaut-system --create-namespace \
  -f my-values.yaml
```

### From Source

```bash
git clone https://github.com/jordigilh/kubernaut.git
cd kubernaut
helm install kubernaut charts/kubernaut \
  --namespace kubernaut-system --create-namespace \
  -f my-values.yaml
```

## Configuration

All values are validated against `values.schema.json`. Run `helm lint` to check your overrides before installing.

### Global Settings

| Parameter | Description | Default |
|---|---|---|
| `global.image.registry` | Container image registry | `quay.io/kubernaut-ai` |
| `global.image.tag` | Image tag override (defaults to `appVersion`) | `""` |
| `global.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `global.nodeSelector` | Global node selector applied to all pods | `{}` |
| `global.tolerations` | Global tolerations applied to all pods | `[]` |

### Gateway

| Parameter | Description | Default |
|---|---|---|
| `gateway.replicas` | Number of gateway replicas | `1` |
| `gateway.resources` | CPU/memory requests and limits | See `values.yaml` |
| `gateway.service.type` | Kubernetes Service type | `ClusterIP` |
| `gateway.auth.signalSources` | External signal sources requiring RBAC | `[]` |

### DataStorage

| Parameter | Description | Default |
|---|---|---|
| `datastorage.replicas` | Number of datastorage replicas | `1` |
| `datastorage.dbExistingSecret` | Pre-created Secret with `db-secrets.yaml` key | `""` |
| `datastorage.resources` | CPU/memory requests and limits | See `values.yaml` |
| `datastorage.service.type` | Kubernetes Service type | `ClusterIP` |

### HolmesGPT API (LLM Integration)

| Parameter | Description | Default |
|---|---|---|
| `holmesgptApi.replicas` | Number of replicas | `1` |
| `holmesgptApi.llm.provider` | LLM provider (e.g., `openai`, `azure`, `vertexai`) | `""` |
| `holmesgptApi.llm.model` | LLM model name | `""` |
| `holmesgptApi.llm.endpoint` | Custom LLM endpoint URL | `""` |
| `holmesgptApi.llm.maxRetries` | Maximum LLM call retries | `3` |
| `holmesgptApi.llm.timeoutSeconds` | LLM call timeout | `120` |
| `holmesgptApi.llm.temperature` | LLM sampling temperature | `0.7` |

### Notification Controller

| Parameter | Description | Default |
|---|---|---|
| `notification.replicas` | Number of replicas | `1` |
| `notification.slack.enabled` | Enable Slack delivery channel | `false` |
| `notification.slack.channel` | Default Slack channel | `#kubernaut-alerts` |
| `notification.credentials` | Projected volume sources from K8s Secrets | See `values.yaml` |

### Controllers (Common Parameters)

All controllers (`aianalysis`, `signalprocessing`, `remediationorchestrator`, `workflowexecution`, `effectivenessmonitor`, `authwebhook`, `notification`) accept:

| Parameter | Description | Default |
|---|---|---|
| `<controller>.replicas` | Number of replicas | `1` |
| `<controller>.resources` | CPU/memory requests and limits | See `values.yaml` |
| `<controller>.podSecurityContext` | Pod-level security context override | Restricted profile defaults |
| `<controller>.containerSecurityContext` | Container-level security context override | Restricted profile defaults |
| `<controller>.nodeSelector` | Per-component node selector (overrides global) | `{}` |
| `<controller>.tolerations` | Per-component tolerations (overrides global) | `[]` |
| `<controller>.affinity` | Pod affinity/anti-affinity rules | `{}` |
| `<controller>.topologySpreadConstraints` | Topology spread constraints | `[]` |
| `<controller>.pdb.enabled` | Create a PodDisruptionBudget | `false` |
| `<controller>.pdb.minAvailable` | PDB minimum available pods | -- |
| `<controller>.pdb.maxUnavailable` | PDB maximum unavailable pods | -- |

### WorkflowExecution

| Parameter | Description | Default |
|---|---|---|
| `workflowexecution.workflowNamespace` | Namespace for Job/PipelineRun execution | `kubernaut-workflows` |

### EffectivenessMonitor

| Parameter | Description | Default |
|---|---|---|
| `effectivenessmonitor.external.prometheusUrl` | Prometheus URL | `http://kube-prometheus-stack-prometheus.monitoring:9090` |
| `effectivenessmonitor.external.prometheusEnabled` | Enable Prometheus integration | `true` |
| `effectivenessmonitor.external.alertManagerUrl` | AlertManager URL | `http://kube-prometheus-stack-alertmanager.monitoring:9093` |
| `effectivenessmonitor.external.alertManagerEnabled` | Enable AlertManager integration | `true` |

### Event Exporter

| Parameter | Description | Default |
|---|---|---|
| `eventExporter.replicas` | Number of replicas | `1` |
| `eventExporter.image` | Container image | `ghcr.io/resmoio/kubernetes-event-exporter:v1.7` |
| `eventExporter.resources` | CPU/memory requests and limits | See `values.yaml` |

### PostgreSQL

| Parameter | Description | Default |
|---|---|---|
| `postgresql.enabled` | Deploy in-chart PostgreSQL | `true` |
| `postgresql.replicas` | Number of replicas | `1` |
| `postgresql.image` | PostgreSQL container image | `postgres:16-alpine` |
| `postgresql.auth.existingSecret` | Pre-created Secret name | `""` |
| `postgresql.auth.username` | Database username | `slm_user` |
| `postgresql.auth.password` | Database password (required if no `existingSecret`) | `""` |
| `postgresql.auth.database` | Database name | `action_history` |
| `postgresql.storage.size` | PVC size | `10Gi` |
| `postgresql.storage.storageClassName` | StorageClass (empty = cluster default) | `""` |

### External PostgreSQL (BYO)

Set `postgresql.enabled=false` and configure these values to use a pre-existing PostgreSQL instance:

| Parameter | Description | Default |
|---|---|---|
| `externalPostgresql.host` | External PostgreSQL hostname (required) | `""` |
| `externalPostgresql.port` | External PostgreSQL port | `5432` |
| `externalPostgresql.auth.existingSecret` | Pre-created Secret name | `""` |
| `externalPostgresql.auth.username` | Database username | `slm_user` |
| `externalPostgresql.auth.password` | Database password | `""` |
| `externalPostgresql.auth.database` | Database name | `action_history` |

### Redis

| Parameter | Description | Default |
|---|---|---|
| `redis.enabled` | Deploy in-chart Redis | `true` |
| `redis.replicas` | Number of replicas | `1` |
| `redis.image` | Redis container image | `quay.io/jordigilh/redis:7-alpine` |
| `redis.existingSecret` | Pre-created Secret name | `""` |
| `redis.password` | Redis password | `""` |
| `redis.storage.size` | PVC size | `512Mi` |
| `redis.storage.storageClassName` | StorageClass (empty = cluster default) | `""` |

### External Redis (BYO)

Set `redis.enabled=false` and configure these values to use a pre-existing Redis instance:

| Parameter | Description | Default |
|---|---|---|
| `externalRedis.host` | External Redis hostname (required) | `""` |
| `externalRedis.port` | External Redis port | `6379` |
| `externalRedis.existingSecret` | Pre-created Secret name | `""` |
| `externalRedis.password` | Redis password | `""` |

### Hooks

| Parameter | Description | Default |
|---|---|---|
| `hooks.tlsCerts.image` | kubectl image for TLS cert generation | `bitnami/kubectl:1.32` |
| `hooks.migrations.image` | PostgreSQL image for migrations | `postgres:16-alpine` |
| `hooks.migrations.gooseVersion` | goose CLI version | `v3.24.1` |

### Network Policies

| Parameter | Description | Default |
|---|---|---|
| `networkPolicies.enabled` | Create NetworkPolicy resources | `false` |

When enabled, NetworkPolicies restrict ingress/egress traffic for gateway, datastorage, and authwebhook. DNS egress (port 53) is always allowed.

## Security Hardening

### Pod Security

All Deployments and hook Jobs run with a restricted security profile by default:

**Pod-level** (`securityContext`):
- `runAsNonRoot: true`
- `runAsUser: 65534` (nobody) for application containers; `999` for postgresql/redis
- `seccompProfile.type: RuntimeDefault`

**Container-level** (`securityContext`):
- `allowPrivilegeEscalation: false`
- `readOnlyRootFilesystem: true` (where supported)
- `capabilities.drop: ["ALL"]`

Override per-component via `<component>.podSecurityContext` and `<component>.containerSecurityContext`:

```yaml
gateway:
  podSecurityContext:
    runAsUser: 1000
  containerSecurityContext:
    readOnlyRootFilesystem: false
```

### Service Accounts

PostgreSQL and Redis run with dedicated ServiceAccounts that have `automountServiceAccountToken: false`, preventing unnecessary API token mounting.

## High Availability

### Replicas

All components default to 1 replica. Scale for production:

```yaml
gateway:
  replicas: 3
datastorage:
  replicas: 2
```

### Pod Disruption Budgets

Enable PDBs for critical components:

```yaml
gateway:
  pdb:
    enabled: true
    minAvailable: 1
datastorage:
  pdb:
    enabled: true
    maxUnavailable: 1
```

### Affinity and Topology Spread

Spread pods across nodes or zones:

```yaml
gateway:
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchExpressions:
                - key: app
                  operator: In
                  values: ["gateway"]
            topologyKey: kubernetes.io/hostname
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfiable: DoNotSchedule
      labelSelector:
        matchLabels:
          app: gateway
```

### Scheduling

Per-component `nodeSelector` and `tolerations` override global settings:

```yaml
gateway:
  nodeSelector:
    node-type: kubernaut
  tolerations:
    - key: "dedicated"
      operator: "Equal"
      value: "kubernaut"
      effect: "NoSchedule"
```

## TLS Certificate Management

The chart manages TLS certificates for the admission webhooks automatically via Helm hooks:

1. **Pre-install/pre-upgrade** (`tls-cert-gen`): Generates a self-signed CA and server certificate, stored as the `authwebhook-tls` Secret and `authwebhook-ca` ConfigMap.
2. **Post-install/post-upgrade** (`tls-cabundle-patch`): Patches the `caBundle` field on MutatingWebhookConfiguration and ValidatingWebhookConfiguration.
3. **Post-delete** (`tls-cleanup`): Removes the `authwebhook-tls` Secret and `authwebhook-ca` ConfigMap.

**Automatic renewal**: On `helm upgrade`, if the certificate expires within 30 days, it is automatically regenerated.

**Upgrade behavior**: On upgrade, the chart uses `lookup` to inject the existing CA bundle into webhook configurations at render time, so webhooks remain functional throughout the upgrade process. The post-install patch provides a redundant update.

**Recovery**: If the `authwebhook-ca` ConfigMap is accidentally deleted while `authwebhook-tls` still exists, delete the `authwebhook-tls` Secret and run `helm upgrade` to regenerate both:

```bash
kubectl delete secret authwebhook-tls -n kubernaut-system
helm upgrade kubernaut kubernaut/kubernaut -n kubernaut-system -f my-values.yaml
```

## Credential Management

The chart supports two credential patterns:

**Option A -- Pre-created Secrets (production)**

Create Kubernetes Secrets before installing the chart and reference them:

```bash
kubectl create secret generic my-pg-secret \
  --from-literal=POSTGRES_USER=slm_user \
  --from-literal=POSTGRES_PASSWORD=<password> \
  --from-literal=POSTGRES_DB=action_history \
  -n kubernaut-system

helm install kubernaut kubernaut/kubernaut \
  --set postgresql.auth.existingSecret=my-pg-secret \
  --set datastorage.dbExistingSecret=my-ds-secret
```

**Option B -- Chart-managed Secrets (development)**

Pass passwords directly and let the chart create Secrets:

```bash
helm install kubernaut kubernaut/kubernaut \
  --set postgresql.auth.password=devpass \
  --set redis.password=redispass
```

## CRD Management

Kubernaut installs 9 Custom Resource Definitions in the `crds/` directory:

- `ActionType`, `RemediationWorkflow`, `RemediationRequest`
- `AIAnalysis`, `WorkflowExecution`, `EffectivenessAssessment`
- `NotificationRequest`, `RemediationApprovalRequest`, `SignalProcessing`

### CRD Upgrade Path

Helm does **not** update CRDs automatically on `helm upgrade`. To upgrade CRDs:

```bash
kubectl apply --server-side --force-conflicts -f charts/kubernaut/crds/
```

Run this **before** `helm upgrade` when CRD schemas have changed between versions.

## Upgrading

```bash
# 1. Update CRDs if schema changed
kubectl apply --server-side --force-conflicts -f charts/kubernaut/crds/

# 2. Upgrade the release
helm upgrade kubernaut kubernaut/kubernaut \
  -n kubernaut-system -f my-values.yaml
```

Key upgrade behaviors:

- **TLS certificates** are renewed automatically if expiring within 30 days.
- **Database migrations** run automatically via the post-upgrade hook.
- **PVCs** are not modified (immutable for bound claims).
- **ConfigMaps and Secrets** are updated to reflect new values.

## Uninstalling

```bash
helm uninstall kubernaut -n kubernaut-system
```

### What is retained after uninstall

| Resource | Behavior | Manual cleanup |
|---|---|---|
| PostgreSQL PVC (`postgresql-data`) | **Retained** (`resource-policy: keep`) | `kubectl delete pvc postgresql-data -n kubernaut-system` |
| Redis PVC (`redis-data`) | **Retained** (`resource-policy: keep`) | `kubectl delete pvc redis-data -n kubernaut-system` |
| CRDs (9 definitions) | **Retained** (standard Helm behavior) | `kubectl delete -f charts/kubernaut/crds/` |
| CR instances | **Retained** until CRDs are deleted | Deleted when parent CRD is deleted |
| TLS Secret and CA ConfigMap | **Deleted** by post-delete hook | -- |
| Cluster-scoped RBAC | **Deleted** by Helm | -- |
| `kubernaut-workflows` namespace | **Deleted** by Helm | May get stuck if it contains active Jobs; see below |

If the `kubernaut-workflows` namespace gets stuck in `Terminating` state:

```bash
# Check for remaining resources
kubectl get all -n kubernaut-workflows
# Delete stuck resources
kubectl delete jobs --all -n kubernaut-workflows
```

## Known Limitations

- **Single installation per cluster**: Cluster-scoped resources (ClusterRoles, ClusterRoleBindings, WebhookConfigurations) use static names. Installing multiple releases in different namespaces will cause conflicts.
- **Init container timeouts**: The `wait-for-postgres` init containers in DataStorage and the migration Job have no timeout. If PostgreSQL is unavailable, these containers will block indefinitely.
- **Event Exporter probes**: The event-exporter container does not expose health endpoints, so no liveness or readiness probes are configured.

## Architecture

Kubernaut consists of 9 microservices, each deployed as a Kubernetes Deployment:

- **Gateway**: Ingests signals from AlertManager and Kubernetes events
- **SignalProcessing**: Correlates and deduplicates signals
- **AIAnalysis**: AI-powered root cause analysis via HolmesGPT
- **RemediationOrchestrator**: Routes analysis results to appropriate workflows
- **WorkflowExecution**: Executes remediation via K8s Jobs, Tekton, or Ansible
- **EffectivenessMonitor**: Validates remediation effectiveness
- **Notification**: Delivers notifications to Slack and console
- **AuthWebhook**: Kubernetes admission webhooks for SOC2 attribution
- **DataStorage**: Audit trail persistence with PostgreSQL and Redis

Supporting infrastructure:

- **Event Exporter**: Forwards Kubernetes Warning events to the Gateway
- **PostgreSQL**: Audit trail and workflow state persistence
- **Redis**: Dead letter queue for failed events

## License

See [LICENSE](https://github.com/jordigilh/kubernaut/blob/main/LICENSE) in the project repository.
