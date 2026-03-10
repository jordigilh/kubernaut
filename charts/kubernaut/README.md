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
| `notification.slack.enabled` | Enable Slack delivery channel | `false` |
| `notification.slack.channel` | Default Slack channel | `#kubernaut-alerts` |
| `notification.credentials` | Projected volume sources from K8s Secrets | See `values.yaml` |

### Controllers

All controllers (`aianalysis`, `signalprocessing`, `remediationorchestrator`, `workflowexecution`, `effectivenessmonitor`, `authwebhook`) accept:

| Parameter | Description | Default |
|---|---|---|
| `<controller>.resources.requests.cpu` | CPU request | Varies |
| `<controller>.resources.requests.memory` | Memory request | Varies |
| `<controller>.resources.limits.cpu` | CPU limit | Varies |
| `<controller>.resources.limits.memory` | Memory limit | Varies |

### PostgreSQL

| Parameter | Description | Default |
|---|---|---|
| `postgresql.enabled` | Deploy in-chart PostgreSQL | `true` |
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

## Uninstalling

```bash
helm uninstall kubernaut -n kubernaut-system
```

CRDs and their data are **not** removed by `helm uninstall`. To remove CRDs:

```bash
kubectl delete -f charts/kubernaut/crds/
```

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

## License

See [LICENSE](https://github.com/jordigilh/kubernaut/blob/main/LICENSE) in the project repository.
