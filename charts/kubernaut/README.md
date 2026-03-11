# Kubernaut Helm Chart

Kubernaut is an autonomous Kubernetes remediation platform that detects incidents via Prometheus AlertManager and Kubernetes events, performs AI-powered root cause analysis, and executes automated remediation workflows with human-in-the-loop approval gates.

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

## Prerequisites

<!-- --8<-- [start:prerequisites] -->
| Requirement | Version | Notes |
|---|---|---|
| Kubernetes | 1.32+ | selectableFields GA required |
| Helm | 3.12+ | |
| StorageClass | dynamic provisioning | For PostgreSQL and Redis PVCs |
| cert-manager | 1.12+ (production) | Required when `tls.mode=cert-manager`. Optional for dev (`tls.mode=hook` is default). |

**Workflow execution engine** (at least one):

- Kubernetes Jobs (built-in, no extra dependency)
- Tekton Pipelines (optional)
- Ansible Automation Platform (AAP) / AWX (optional)

**External monitoring** (recommended):

- [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack) provides:
  - Alert-based signal ingestion (AlertManager sends alerts to Gateway)
  - Metrics enrichment for effectiveness assessments (Prometheus queries)
  - Alert resolution checks (AlertManager API)
  - Metrics scraping for all Kubernaut services (all pods expose `/metrics`)

If Prometheus and AlertManager are not deployed, set `effectivenessmonitor.external.prometheusEnabled=false` and `effectivenessmonitor.external.alertManagerEnabled=false`.
<!-- --8<-- [end:prerequisites] -->

## Infrastructure Setup

Complete these steps before installing the Kubernaut chart.

<!-- --8<-- [start:infrastructure-setup] -->
### Storage

PostgreSQL and Redis each require a PersistentVolumeClaim for data persistence:

| Component | PVC Name | Default Size | Values |
|---|---|---|---|
| PostgreSQL | `postgresql-data` | `10Gi` | `postgresql.storage.size`, `postgresql.storage.storageClassName` |
| Redis | `redis-data` | `512Mi` | `redis.storage.size`, `redis.storage.storageClassName` |

Both PVCs are annotated with `helm.sh/resource-policy: keep` so data survives `helm uninstall`.

If the cluster has no default StorageClass, set `storageClassName` explicitly:

```yaml
postgresql:
  storage:
    size: 50Gi
    storageClassName: gp3-encrypted
redis:
  storage:
    storageClassName: gp3-encrypted
```

To skip in-chart databases entirely and use external instances, set `postgresql.enabled=false` and/or `redis.enabled=false` and configure `externalPostgresql` / `externalRedis` values.

### Prometheus and AlertManager

Kubernaut integrates with Prometheus and AlertManager at two levels:

**1. EffectivenessMonitor queries** -- EM queries Prometheus for metric-based assessment enrichment and AlertManager for alert resolution checks. The expected service endpoints (configurable):

| Service | Default URL | Override |
|---|---|---|
| Prometheus | `http://kube-prometheus-stack-prometheus.monitoring.svc:9090` | `effectivenessmonitor.external.prometheusUrl` |
| AlertManager | `http://kube-prometheus-stack-alertmanager.monitoring.svc:9093` | `effectivenessmonitor.external.alertManagerUrl` |

**2. AlertManager sends alerts to Gateway** -- AlertManager must be configured with a webhook receiver pointing to the Kubernaut Gateway. Add this to your AlertManager configuration:

```yaml
receivers:
  - name: kubernaut
    webhook_configs:
      - url: "http://gateway.kubernaut-system.svc:9090/api/v1/signals/alertmanager"
        send_resolved: true

route:
  routes:
    - receiver: kubernaut
      matchers:
        - alertname!=""
      continue: true
```

For kube-prometheus-stack, configure this via Helm values:

```yaml
# kube-prometheus-stack values
alertmanager:
  config:
    receivers:
      - name: kubernaut
        webhook_configs:
          - url: "http://gateway.kubernaut-system.svc:9090/api/v1/signals/alertmanager"
            send_resolved: true
    route:
      routes:
        - receiver: kubernaut
          matchers:
            - alertname!=""
          continue: true
```

**3. Gateway RBAC for AlertManager** -- The Gateway requires AlertManager's ServiceAccount to be authorized as a signal source. Configure `gateway.auth.signalSources` in your Kubernaut values:

```yaml
gateway:
  auth:
    signalSources:
      - name: alertmanager
        serviceAccount: alertmanager-kube-prometheus-stack-alertmanager
        namespace: monitoring
```
<!-- --8<-- [end:infrastructure-setup] -->

## Pre-Installation

<!-- --8<-- [start:pre-installation] -->
### 1. Install CRDs

Kubernaut uses 9 Custom Resource Definitions. Apply them before installing the chart:

```bash
kubectl apply --server-side --force-conflicts -f charts/kubernaut/crds/
```

Helm installs CRDs on first install but does **not** upgrade them on `helm upgrade`. Always apply CRDs manually before upgrading to a new chart version.

### 2. Create the Namespace

```bash
kubectl create namespace kubernaut-system
```

### 3. Provision Secrets

Create the required secrets in the namespace before installing. The chart references these by name.

**PostgreSQL credentials** (required):

```bash
kubectl create secret generic kubernaut-pg-credentials \
  --from-literal=POSTGRES_USER=slm_user \
  --from-literal=POSTGRES_PASSWORD=<password> \
  --from-literal=POSTGRES_DB=action_history \
  -n kubernaut-system
```

| Chart Value | Secret Name | Required Keys |
|---|---|---|
| `postgresql.auth.existingSecret` | Your secret name | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` |

**DataStorage DB config** (required):

```bash
kubectl create secret generic kubernaut-ds-credentials \
  --from-literal=db-secrets.yaml=$'username: slm_user\npassword: <password>' \
  -n kubernaut-system
```

| Chart Value | Secret Name | Required Keys |
|---|---|---|
| `datastorage.dbExistingSecret` | Your secret name | `db-secrets.yaml` (YAML with `username` and `password`) |

**Redis credentials** (required):

```bash
kubectl create secret generic kubernaut-redis-credentials \
  --from-literal=redis-secrets.yaml=$'password: <password>' \
  -n kubernaut-system
```

| Chart Value | Secret Name | Required Keys |
|---|---|---|
| `redis.existingSecret` | Your secret name | `redis-secrets.yaml` (YAML with `password`) |

**LLM credentials** (required for AI analysis):

```bash
kubectl create secret generic kubernaut-llm-credentials \
  --from-literal=OPENAI_API_KEY=sk-... \
  -n kubernaut-system
```

| Chart Value | Secret Name | Required Keys |
|---|---|---|
| `holmesgptApi.llm.credentialsSecretName` | Your secret name (default: `llm-credentials`) | Provider-specific (e.g., `OPENAI_API_KEY`, `AZURE_API_KEY`, `GOOGLE_APPLICATION_CREDENTIALS`) |

HAPI starts without this secret (`optional: true`) but all LLM calls will fail until it is created.

**Notification credentials** (optional, only for Slack delivery):

```bash
kubectl create secret generic kubernaut-slack-credentials \
  --from-literal=webhook-url=https://hooks.slack.com/services/T.../B.../... \
  -n kubernaut-system
```

| Chart Value | Secret Name | Required Keys |
|---|---|---|
| `notification.credentials[].secretName` | Your secret name | `webhook-url` (or custom key via `secretKey`) |

Only required when `notification.slack.enabled=true`. When Slack is disabled (default), no notification secret is needed.
<!-- --8<-- [end:pre-installation] -->

## Installation

<!-- --8<-- [start:installation] -->
### Production

With namespace and secrets already provisioned:

```bash
helm install kubernaut charts/kubernaut/ \
  --namespace kubernaut-system \
  --set postgresql.auth.existingSecret=kubernaut-pg-credentials \
  --set datastorage.dbExistingSecret=kubernaut-ds-credentials \
  --set redis.existingSecret=kubernaut-redis-credentials \
  --set holmesgptApi.llm.provider=openai \
  --set holmesgptApi.llm.model=gpt-4o \
  --set holmesgptApi.llm.credentialsSecretName=kubernaut-llm-credentials \
  --set gateway.auth.signalSources[0].name=alertmanager \
  --set gateway.auth.signalSources[0].serviceAccount=alertmanager-kube-prometheus-stack-alertmanager \
  --set gateway.auth.signalSources[0].namespace=monitoring
```

### From OCI Registry

```bash
helm install kubernaut oci://ghcr.io/jordigilh/kubernaut/charts/kubernaut \
  --version 1.0.0 \
  --namespace kubernaut-system \
  -f my-values.yaml
```

### Development Quick Start

For local development without external monitoring or pre-created secrets:

```bash
helm install kubernaut charts/kubernaut/ \
  --namespace kubernaut-system --create-namespace \
  --set postgresql.auth.password=devpass \
  --set redis.password=redispass \
  --set effectivenessmonitor.external.prometheusEnabled=false \
  --set effectivenessmonitor.external.alertManagerEnabled=false
```

### Post-Install Verification

```bash
# All 13 pods should be 1/1 Running
kubectl get pods -n kubernaut-system

# Verify LLM connectivity
kubectl port-forward -n kubernaut-system svc/holmesgpt-api 8080:8080
curl -s http://localhost:8080/health | jq '.'

# Verify workflow catalog
kubectl port-forward -n kubernaut-system svc/data-storage-service 8080:8080
curl -s http://localhost:8080/api/v1/workflows | jq '.'
```
<!-- --8<-- [end:installation] -->

## Post-Installation

<!-- --8<-- [start:post-installation] -->
### Action Types

Kubernaut ships with 24 ActionType definitions in `deploy/action-types/` that define the remediation catalog (e.g., `delete-pod`, `restart-deployment`, `scale-replicas`). Load them per your operational workflow:

```bash
kubectl apply -f deploy/action-types/ -n kubernaut-system
```

### Remediation Workflows

Create RemediationWorkflow CRs to define end-to-end remediation flows that reference ActionTypes. See the project documentation for workflow authoring guidelines.
<!-- --8<-- [end:post-installation] -->

## Configuration Reference

All values are validated against `values.schema.json`. Run `helm lint` to check your overrides before installing.

<!-- --8<-- [start:helm-values] -->
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
| `holmesgptApi.llm.credentialsSecretName` | Name of pre-existing Secret with LLM API keys | `llm-credentials` |

### Notification Controller

| Parameter | Description | Default |
|---|---|---|
| `notification.replicas` | Number of replicas | `1` |
| `notification.slack.enabled` | Enable Slack delivery channel | `false` |
| `notification.slack.channel` | Default Slack channel | `#kubernaut-alerts` |
| `notification.credentials` | Projected volume sources from K8s Secrets | `[]` |

When `slack.enabled` is `true`, add credentials entries pointing to your pre-existing secrets:

```yaml
notification:
  slack:
    enabled: true
    channel: "#kubernaut-alerts"
  credentials:
    - name: slack-webhook
      secretName: kubernaut-slack-credentials
      secretKey: webhook-url
```

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
| `effectivenessmonitor.external.prometheusUrl` | Prometheus URL | `http://kube-prometheus-stack-prometheus.monitoring.svc:9090` |
| `effectivenessmonitor.external.prometheusEnabled` | Enable Prometheus integration | `true` |
| `effectivenessmonitor.external.alertManagerUrl` | AlertManager URL | `http://kube-prometheus-stack-alertmanager.monitoring.svc:9093` |
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

### TLS

| Parameter | Description | Default |
|---|---|---|
| `tls.mode` | TLS mode: `hook` (self-signed via Helm hooks) or `cert-manager` (production) | `hook` |
| `tls.certManager.issuerRef.name` | Issuer/ClusterIssuer name (required when `tls.mode=cert-manager`) | `""` |
| `tls.certManager.issuerRef.kind` | Issuer kind | `ClusterIssuer` |
| `tls.certManager.issuerRef.group` | Issuer API group | `cert-manager.io` |

### Hooks

| Parameter | Description | Default |
|---|---|---|
| `hooks.tlsCerts.image` | kubectl image for TLS cert generation (hook mode only) | `bitnami/kubectl:latest` |
| `hooks.migrations.image` | PostgreSQL image for migrations | `postgres:16-alpine` |
| `hooks.migrations.gooseVersion` | goose CLI version | `v3.24.1` |

### Network Policies

| Parameter | Description | Default |
|---|---|---|
| `networkPolicies.enabled` | Create NetworkPolicy resources | `false` |

When enabled, NetworkPolicies restrict ingress/egress traffic for gateway, datastorage, and authwebhook. DNS egress (port 53) is always allowed.
<!-- --8<-- [end:helm-values] -->

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

<!-- --8<-- [start:tls-management] -->
The chart supports two modes for managing TLS certificates used by the admission webhooks, controlled by `tls.mode`:

### Hook Mode (`tls.mode: hook`) -- Default

Self-signed certificates are generated and managed by Helm hooks. No external dependencies required. Suitable for development, testing, and CI environments.

**How it works:**

1. **Pre-install/pre-upgrade** (`tls-cert-gen`): Generates a self-signed CA and server certificate, stored as the `authwebhook-tls` Secret and `authwebhook-ca` ConfigMap.
2. **Post-install/post-upgrade** (`tls-cabundle-patch`): Patches the `caBundle` field on the webhook configurations.
3. **Post-delete** (`tls-cleanup`): Removes the `authwebhook-tls` Secret and `authwebhook-ca` ConfigMap.

**Automatic renewal**: On `helm upgrade`, if the certificate expires within 30 days, it is automatically regenerated.

**Recovery**: If the `authwebhook-ca` ConfigMap is accidentally deleted while `authwebhook-tls` still exists, delete the `authwebhook-tls` Secret and run `helm upgrade` to regenerate both:

```bash
kubectl delete secret authwebhook-tls -n kubernaut-system
helm upgrade kubernaut kubernaut/kubernaut -n kubernaut-system -f my-values.yaml
```

> **Note**: `helm template` output will not show `caBundle` on webhook configurations. This is expected -- the hook injects it at runtime after the webhook resources are created.

### cert-manager Mode (`tls.mode: cert-manager`) -- Production

Certificates are managed by [cert-manager](https://cert-manager.io/). Recommended for production environments. cert-manager handles issuance, renewal, and `caBundle` injection automatically.

**Prerequisites:**

1. Install cert-manager (v1.12+):

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s
```

2. Create an Issuer or ClusterIssuer. For development with cert-manager, a self-signed issuer works:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
```

For production, use your organization's CA or an ACME issuer (e.g., Let's Encrypt).

3. Install the chart with cert-manager mode:

```bash
helm install kubernaut kubernaut/kubernaut \
  --namespace kubernaut-system \
  --set tls.mode=cert-manager \
  --set tls.certManager.issuerRef.name=selfsigned-issuer \
  -f my-values.yaml
```

The chart creates a `Certificate` resource (`authwebhook-cert`) that provisions the `authwebhook-tls` Secret. cert-manager's `cainjector` automatically writes the `caBundle` into the webhook configurations via the `cert-manager.io/inject-ca-from` annotation.

**No TLS hook jobs** are created in this mode -- cert-manager handles the full lifecycle including renewal.
<!-- --8<-- [end:tls-management] -->

## CRD Management

Kubernaut installs 9 Custom Resource Definitions in the `crds/` directory:

- `ActionType`, `RemediationWorkflow`, `RemediationRequest`
- `AIAnalysis`, `WorkflowExecution`, `EffectivenessAssessment`
- `NotificationRequest`, `RemediationApprovalRequest`, `SignalProcessing`

Helm does **not** update CRDs automatically on `helm upgrade`. To upgrade CRDs:

```bash
kubectl apply --server-side --force-conflicts -f charts/kubernaut/crds/
```

Run this **before** `helm upgrade` when CRD schemas have changed between versions.

## Upgrading

<!-- --8<-- [start:upgrading] -->
```bash
# 1. Update CRDs if schema changed
kubectl apply --server-side --force-conflicts -f charts/kubernaut/crds/

# 2. Upgrade the release
helm upgrade kubernaut kubernaut/kubernaut \
  -n kubernaut-system -f my-values.yaml
```

Key upgrade behaviors:

- **TLS certificates** (`tls.mode: hook`): Renewed automatically if expiring within 30 days. In `cert-manager` mode, cert-manager handles renewal.
- **Database migrations** run automatically via the post-upgrade hook.
- **PVCs** are not modified (immutable for bound claims).
- **ConfigMaps and Secrets** are updated to reflect new values.
<!-- --8<-- [end:upgrading] -->

## Uninstalling

<!-- --8<-- [start:uninstalling] -->
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
| TLS Secret and CA ConfigMap | **Deleted** by post-delete hook (`hook` mode) or by cert-manager (`cert-manager` mode) | -- |
| Cluster-scoped RBAC | **Deleted** by Helm | -- |
| `kubernaut-workflows` namespace | **Deleted** by Helm | May get stuck if it contains active Jobs; see below |

If the `kubernaut-workflows` namespace gets stuck in `Terminating` state:

```bash
kubectl get all -n kubernaut-workflows
kubectl delete jobs --all -n kubernaut-workflows
```

### Full cleanup

To remove everything including persistent data:

```bash
helm uninstall kubernaut -n kubernaut-system
kubectl delete pvc postgresql-data redis-data -n kubernaut-system
kubectl delete -f charts/kubernaut/crds/
kubectl delete namespace kubernaut-system
```
<!-- --8<-- [end:uninstalling] -->

## Known Limitations

<!-- --8<-- [start:known-limitations] -->
- **Single installation per cluster**: Cluster-scoped resources (ClusterRoles, ClusterRoleBindings, WebhookConfigurations) use static names. Installing multiple releases in different namespaces will cause conflicts.
- **Init container timeouts**: The `wait-for-postgres` init containers in DataStorage and the migration Job have no timeout. If PostgreSQL is unavailable, these containers will block indefinitely.
- **Event Exporter probes**: The event-exporter container does not expose health endpoints, so no liveness or readiness probes are configured.
<!-- --8<-- [end:known-limitations] -->

## License

See [LICENSE](https://github.com/jordigilh/kubernaut/blob/main/LICENSE) in the project repository.
