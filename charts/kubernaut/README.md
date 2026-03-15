# Kubernaut Helm Chart

Kubernaut is an autonomous Kubernetes remediation platform that detects incidents via Prometheus AlertManager and Kubernetes events, performs AI-powered root cause analysis, and executes automated remediation workflows with human-in-the-loop approval gates.

> **Full documentation**: [jordigilh.github.io/kubernaut-docs](https://jordigilh.github.io/kubernaut-docs/)

## Prerequisites

| Requirement | Version | Notes |
|---|---|---|
| Kubernetes | 1.31+ | selectableFields (beta in 1.31, GA in 1.32) |
| Helm | 3.12+ | |
| StorageClass | dynamic provisioning | For PostgreSQL and Valkey PVCs |
| cert-manager | 1.12+ (production) | Required when `tls.mode=cert-manager`. Not needed for `tls.mode=hook` (default) or `tls.mode=manual`. |

**Workflow execution engine** (at least one):

- Kubernetes Jobs (built-in, no extra dependency)
- Tekton Pipelines (optional)
- Ansible Automation Platform (AAP) / AWX (optional)

**External monitoring** (recommended): [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack) for alert-based signal ingestion and metrics enrichment.

If Prometheus and AlertManager are not deployed, set `effectivenessmonitor.external.prometheusEnabled=false` and `effectivenessmonitor.external.alertManagerEnabled=false`.

### OpenShift (OCP)

The OCP overlay (`values-ocp.yaml`) switches PostgreSQL and Valkey to Red Hat RHEL10 catalog images (direct pull from `registry.redhat.io`), replaces `bitnami/kubectl` with `ose-cli` for hook jobs, and disables the event exporter (no Red Hat-supported equivalent; users should provide their own Kubernetes event forwarding if needed).

No ImageStream prerequisites are required — pods pull directly from `registry.redhat.io` using the cluster's global pull secret.

```bash
helm install kubernaut charts/kubernaut/ \
  -n kubernaut-system \
  -f charts/kubernaut/values-demo.yaml \
  -f charts/kubernaut/values-ocp.yaml \
  --set holmesgptApi.llm.provider=openai \
  --set holmesgptApi.llm.model=gpt-4o
```

See `values-ocp.yaml` for the full set of OCP-specific overrides.

## Quick Start

```bash
# 1. Create namespace and secrets
kubectl create namespace kubernaut-system
kubectl create secret generic kubernaut-pg-credentials \
  --from-literal=POSTGRES_USER=slm_user \
  --from-literal=POSTGRES_PASSWORD=<password> \
  --from-literal=POSTGRES_DB=action_history \
  -n kubernaut-system
kubectl create secret generic kubernaut-ds-db-credentials \
  --from-literal=db-secrets.yaml=$'username: slm_user\npassword: <password>' \
  -n kubernaut-system
kubectl create secret generic kubernaut-valkey-credentials \
  --from-literal=valkey-secrets.yaml=$'password: <password>' \
  -n kubernaut-system
kubectl create secret generic llm-credentials \
  --from-literal=OPENAI_API_KEY=sk-... \
  -n kubernaut-system

# 2. Install (Helm installs CRDs automatically on first install)
helm install kubernaut charts/kubernaut/ \
  --namespace kubernaut-system \
  -f charts/kubernaut/values-demo.yaml \
  --set holmesgptApi.llm.provider=openai \
  --set holmesgptApi.llm.model=gpt-4o

# 3. Verify
kubectl get pods -n kubernaut-system
```

See the [Installation Guide](https://jordigilh.github.io/kubernaut-docs/getting-started/installation/) for the full walkthrough including secret provisioning, AlertManager integration, and post-install steps.

## AlertManager Integration

Configure AlertManager to send webhooks to the Gateway with bearer token authentication:

```yaml
receivers:
  - name: kubernaut
    webhook_configs:
      - url: "http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/signals/prometheus"
        send_resolved: true
        http_config:
          bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token

route:
  routes:
    - receiver: kubernaut
      matchers:
        - alertname!=""
      continue: true
```

Then register AlertManager's ServiceAccount as an authorized signal source:

```yaml
# kubernaut values.yaml
gateway:
  auth:
    signalSources:
      - name: alertmanager
        serviceAccount: alertmanager-kube-prometheus-stack-alertmanager
        namespace: monitoring
```

> **Important**: Without `http_config.bearer_token_file`, the Gateway rejects requests with `401 Unauthorized`. Without the `signalSources` entry, SAR denies access with `403 Forbidden`.

See [Signal Source Authentication](https://jordigilh.github.io/kubernaut-docs/architecture/security-rbac/#signal-ingestion) for the full TokenReview + SAR flow and RBAC details.

## Configuration Reference

All values are validated against `values.schema.json`. Run `helm lint` to check your overrides before installing.

### Global Settings

| Parameter | Description | Default |
|---|---|---|
| `global.image.registry` | Container image registry hostname | `quay.io` |
| `global.image.namespace` | Image namespace prefix (joined to service name by `separator`) | `kubernaut-ai` |
| `global.image.separator` | Character joining namespace to service name: `/` for nested registries, `-` for flat (quay.io, Docker Hub) | `/` |
| `global.image.tag` | Image tag override (defaults to `appVersion`) | `""` |
| `global.image.digest` | Image digest (e.g. `sha256:abc...`); when set, overrides tag | `""` |
| `global.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `global.nodeSelector` | Global node selector applied to all pods | `{}` |
| `global.tolerations` | Global tolerations applied to all pods | `[]` |

### Gateway

| Parameter | Description | Default |
|---|---|---|
| `gateway.replicas` | Number of gateway replicas | `1` |
| `gateway.resources` | CPU/memory requests and limits | See `values.yaml` |
| `gateway.service.type` | Kubernetes Service type | `ClusterIP` |
| `gateway.auth.signalSources` | External signal sources requiring RBAC | AlertManager for kube-prometheus-stack (see `values.yaml`) |

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
| `holmesgptApi.llm.provider` | LLM provider (e.g., `openai`, `azure`, `vertex_ai`) | `""` |
| `holmesgptApi.llm.model` | LLM model name | `""` |
| `holmesgptApi.llm.endpoint` | Custom LLM endpoint URL | `""` |
| `holmesgptApi.llm.gcpProjectId` | GCP project ID (for `vertex_ai` provider) | `""` |
| `holmesgptApi.llm.gcpRegion` | GCP region (for `vertex_ai` provider) | `""` |
| `holmesgptApi.llm.maxRetries` | Maximum LLM call retries | `3` |
| `holmesgptApi.llm.timeoutSeconds` | LLM call timeout | `120` |
| `holmesgptApi.llm.temperature` | LLM sampling temperature | `0.7` |
| `holmesgptApi.llm.credentialsSecretName` | Name of pre-existing Secret with LLM API keys (K8s resource ref, not in SDK ConfigMap) | `llm-credentials` |
| `holmesgptApi.toolsets` | HolmesGPT SDK toolset configuration (see [upstream docs](https://holmesgpt.dev/data-sources/builtin-toolsets/)) | `{}` |
| `holmesgptApi.mcpServers` | MCP server configuration for the HolmesGPT SDK | `{}` |
| `holmesgptApi.existingSdkConfigMap` | Use a pre-existing ConfigMap for SDK config instead of chart-generated `holmesgpt-sdk-config` | `""` |

#### Enabling Prometheus for AI Analysis

To give the LLM access to Prometheus metrics during incident analysis:

```yaml
holmesgptApi:
  toolsets:
    prometheus/metrics:
      enabled: true
      config:
        prometheus_url: "http://kube-prometheus-stack-prometheus.monitoring.svc:9090"
```

This adds the `prometheus/metrics` toolset to the HolmesGPT SDK config, giving the LLM tools
for PromQL queries, alerting rule inspection, and metric discovery. See the
[HolmesGPT Prometheus toolset documentation](https://holmesgpt.dev/data-sources/builtin-toolsets/prometheus/)
for all available configuration options.

Available built-in toolsets (configured via `holmesgptApi.toolsets`):

| Toolset | Description | Default |
|---|---|---|
| `kubernetes/core` | Pod inspection, events, resource status | Enabled (code default) |
| `kubernetes/logs` | Container log retrieval | Enabled (code default) |
| `kubernetes/live-metrics` | `kubectl top` metrics | Enabled (code default) |
| `prometheus/metrics` | PromQL queries, alerting rules, metric discovery | Disabled (no URL) |

For additional toolsets and advanced configuration, refer to the
[HolmesGPT data sources documentation](https://holmesgpt.dev/data-sources/builtin-toolsets/).

#### Using a Custom SDK ConfigMap

For full control over the SDK configuration, create your own ConfigMap and reference it:

```yaml
holmesgptApi:
  existingSdkConfigMap: "my-custom-sdk-config"
```

The ConfigMap must contain a `sdk-config.yaml` key following the
[HolmesGPT configuration format](https://holmesgpt.dev/).

### Notification Controller

| Parameter | Description | Default |
|---|---|---|
| `notification.replicas` | Number of replicas | `1` |
| `notification.slack.enabled` | Enable Slack delivery channel | `false` |
| `notification.slack.channel` | Default Slack channel | `#kubernaut-alerts` |
| `notification.credentials` | Projected volume sources from K8s Secrets | `[]` |

### Controllers (Common Parameters)

All controllers (`aianalysis`, `signalprocessing`, `remediationorchestrator`, `workflowexecution`, `effectivenessmonitor`, `authwebhook`, `notification`) accept:

| Parameter | Description | Default |
|---|---|---|
| `<controller>.replicas` | Number of replicas | `1` |
| `<controller>.resources` | CPU/memory requests and limits | See `values.yaml` |
| `<controller>.podSecurityContext` | Pod-level security context override | `runAsNonRoot: true` + `seccompProfile: RuntimeDefault` (Tier 1); `seccompProfile: RuntimeDefault` only (Tier 2: postgresql, valkey) |
| `<controller>.containerSecurityContext` | Container-level security context override | `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]` |
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

Optional component that forwards Kubernetes Warning events to the Gateway. Disable when K8s event ingestion is not needed or when no supported image is available (e.g., on OCP where there is no Red Hat equivalent).

| Parameter | Description | Default |
|---|---|---|
| `eventExporter.enabled` | Deploy the in-chart event exporter | `true` |
| `eventExporter.replicas` | Number of replicas | `1` |
| `eventExporter.image` | Container image | `ghcr.io/resmoio/kubernetes-event-exporter:v1.7` |
| `eventExporter.resources` | CPU/memory requests and limits | See `values.yaml` |

### PostgreSQL

| Parameter | Description | Default |
|---|---|---|
| `postgresql.enabled` | Deploy in-chart PostgreSQL | `true` |
| `postgresql.variant` | Image variant: `upstream` (postgres:16-alpine) or `ocp` (Red Hat RHEL10 image) | `upstream` |
| `postgresql.replicas` | Number of replicas | `1` |
| `postgresql.image` | PostgreSQL container image | `postgres:16-alpine` |
| `postgresql.auth.existingSecret` | Pre-created Secret with `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` keys (required) | `""` |
| `postgresql.auth.username` | Database username (used in readiness probes and config) | `slm_user` |
| `postgresql.auth.database` | Database name (used in readiness probes and config) | `action_history` |
| `postgresql.storage.size` | PVC size | `10Gi` |
| `postgresql.storage.storageClassName` | StorageClass (empty = cluster default) | `""` |

**OCP variant**: Set `postgresql.variant=ocp` and `postgresql.image` to the Red Hat RHEL10 image (e.g., `registry.redhat.io/rhel10/postgresql-16`). The chart maps the uniform Secret keys (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`) to the OCP-expected env var names (`POSTGRESQL_USER`, `POSTGRESQL_PASSWORD`, `POSTGRESQL_DATABASE`) and adjusts the data directory mount path automatically. The `values-ocp.yaml` overlay sets both `variant` and `image` for you.

### External PostgreSQL (BYO)

Set `postgresql.enabled=false` and configure these values to use a pre-existing PostgreSQL instance:

| Parameter | Description | Default |
|---|---|---|
| `externalPostgresql.host` | External PostgreSQL hostname (required) | `""` |
| `externalPostgresql.port` | External PostgreSQL port | `5432` |
| `externalPostgresql.auth.existingSecret` | Pre-created Secret with `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` keys (required) | `""` |
| `externalPostgresql.auth.username` | Database username (used in config) | `slm_user` |
| `externalPostgresql.auth.database` | Database name (used in config) | `action_history` |

### Valkey

[Valkey](https://valkey.io/) is a BSD-licensed, Redis-compatible in-memory data store used for the DataStorage dead-letter queue.

| Parameter | Description | Default |
|---|---|---|
| `valkey.enabled` | Deploy in-chart Valkey | `true` |
| `valkey.replicas` | Number of replicas | `1` |
| `valkey.image` | Valkey container image | `valkey/valkey:8-alpine` |
| `valkey.existingSecret` | Pre-created Secret with `valkey-secrets.yaml` key (required) | `""` |
| `valkey.storage.size` | PVC size | `512Mi` |
| `valkey.storage.storageClassName` | StorageClass (empty = cluster default) | `""` |

### External Valkey/Redis (BYO)

Set `valkey.enabled=false` and configure these values to use a pre-existing Valkey or Redis instance:

| Parameter | Description | Default |
|---|---|---|
| `externalValkey.host` | External Valkey hostname (required) | `""` |
| `externalValkey.port` | External Valkey port | `6379` |
| `externalValkey.existingSecret` | Pre-created Secret with `valkey-secrets.yaml` key (required) | `""` |

### TLS

| Parameter | Description | Default |
|---|---|---|
| `tls.mode` | TLS mode: `hook` (self-signed via Helm hooks), `cert-manager` (production), or `manual` (user-managed) | `hook` |
| `tls.certManager.issuerRef.name` | Issuer/ClusterIssuer name (required when `tls.mode=cert-manager`) | `""` |
| `tls.certManager.issuerRef.kind` | Issuer kind | `ClusterIssuer` |
| `tls.certManager.issuerRef.group` | Issuer API group | `cert-manager.io` |

See [TLS and Certificate Management](https://jordigilh.github.io/kubernaut-docs/user-guide/configuration/#tls-and-certificate-management) for details on `hook`, `cert-manager`, and `manual` modes.

### Hooks

| Parameter | Description | Default |
|---|---|---|
| `hooks.tlsCerts.image` | kubectl image for TLS cert generation (`hook` mode only; unused in `manual` and `cert-manager` modes; must include shell + openssl) | `docker.io/bitnami/kubectl:latest` (pinned by digest) |
| `hooks.migrations.image` | UBI9-minimal image with goose + psql for database migrations | `quay.io/kubernaut-ai/db-migrate:v1.1.0-rc0` |

### Network Policies

| Parameter | Description | Default |
|---|---|---|
| `networkPolicies.enabled` | Create NetworkPolicy resources | `false` |

When enabled, NetworkPolicies restrict ingress/egress traffic for gateway, datastorage, and authwebhook. DNS egress (port 53) is always allowed.

## Disconnected / Air-Gapped Install

Kubernaut supports installation in disconnected OCP environments where nodes have no internet access. All chart images must be mirrored to an internal registry first.

### 1. Generate the image inventory

```bash
./hack/airgap/generate-image-list.sh --set global.image.tag=1.0.0
```

This outputs every container image the chart will pull, one per line.

### 2. Mirror images with `oc mirror`

Use the template `ImageSetConfiguration` at `hack/airgap/imageset-config.yaml.tmpl`:

```bash
# Edit the template: replace <VERSION> with your release version
cp hack/airgap/imageset-config.yaml.tmpl imageset-config.yaml
sed -i 's/<VERSION>/1.0.0/g' imageset-config.yaml

# Mirror to your internal registry
oc mirror --config=imageset-config.yaml docker://<mirror-registry>
```

Alternatively, use `skopeo copy` for individual images.

### 3. Configure the chart

Layer the `values-airgap.yaml` overlay (or create your own) to point all images to the mirror.

**Nested registries** (Harbor, Artifactory, generic Docker v2) — images stored as `<mirror>/kubernaut-ai/gateway:tag`:

```bash
helm install kubernaut charts/kubernaut/ \
  -n kubernaut-system \
  -f charts/kubernaut/values-demo.yaml \
  -f charts/kubernaut/values-airgap.yaml \
  --set global.image.registry=harbor.corp
```

**Flat registries** (quay.io, Docker Hub, OCP internal) — images stored as `<mirror>/kubernaut-ai-gateway:tag`:

```bash
helm install kubernaut charts/kubernaut/ \
  -n kubernaut-system \
  -f charts/kubernaut/values-demo.yaml \
  -f charts/kubernaut/values-airgap.yaml \
  --set global.image.registry=quay.io/myorg \
  --set global.image.separator=-
```

See `charts/kubernaut/values-airgap.yaml` for the complete list of image overrides.

### 4. OCP: ImageDigestMirrorSet (IDMS)

On OCP 4.13+, create an `ImageDigestMirrorSet` to redirect image pulls from source registries to the mirror without changing `values.yaml`:

```yaml
apiVersion: config.openshift.io/v1
kind: ImageDigestMirrorSet
metadata:
  name: kubernaut-mirror
spec:
  imageDigestMirrors:
    - source: quay.io/kubernaut-ai
      mirrors:
        - <mirror-registry>/kubernaut-ai   # nested; or <mirror-registry> for flat naming
    - source: registry.redhat.io
      mirrors:
        - <mirror-registry>
```

For OCP < 4.13, use the deprecated `ImageContentSourcePolicy` (ICSP) with the same mirror mappings.

### 5. OCP: Disconnected install

The `values-airgap.yaml` overlay overrides the `registry.redhat.io` image references from `values-ocp.yaml` with pulls from your mirror registry.

**Nested registry** (Harbor, Artifactory):

```bash
helm install kubernaut charts/kubernaut/ \
  -n kubernaut-system \
  -f charts/kubernaut/values-demo.yaml \
  -f charts/kubernaut/values-ocp.yaml \
  -f charts/kubernaut/values-airgap.yaml \
  --set global.image.registry=harbor.corp
```

**Flat registry** (quay.io, OCP internal):

```bash
helm install kubernaut charts/kubernaut/ \
  -n kubernaut-system \
  -f charts/kubernaut/values-demo.yaml \
  -f charts/kubernaut/values-ocp.yaml \
  -f charts/kubernaut/values-airgap.yaml \
  --set global.image.registry=quay.io/myorg \
  --set global.image.separator=-
```

> **Note**: `values-airgap.yaml` must be layered **after** `values-ocp.yaml` so it overrides the `registry.redhat.io` image references with your mirror. The `postgresql.variant: ocp` setting from `values-ocp.yaml` is preserved, ensuring correct env var names (`POSTGRESQL_*`) and data directory paths.

## Upgrading

Helm does **not** upgrade CRDs on `helm upgrade`. When upgrading to a chart version with CRD schema changes, extract and apply the new CRDs first:

```bash
# 1. Pull the new chart version and extract CRDs
helm pull oci://quay.io/kubernaut-ai/charts/kubernaut \
  --version <new-version> --untar
kubectl apply --server-side --force-conflicts -f kubernaut/crds/

# 2. Upgrade the release
helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --version <new-version> \
  -n kubernaut-system -f my-values.yaml
```

## Uninstalling

```bash
helm uninstall kubernaut -n kubernaut-system
```

### Full cleanup

```bash
helm uninstall kubernaut -n kubernaut-system

# Remove PVCs retained by resource policy
kubectl delete pvc postgresql-data valkey-data -n kubernaut-system

# Remove hook-created cluster resources (not tracked by Helm)
kubectl delete clusterrole kubernaut-hook-role --ignore-not-found
kubectl delete clusterrolebinding kubernaut-hook-rolebinding --ignore-not-found

# Remove CRDs and all CR instances
kubectl delete crd actiontypes.kubernaut.ai aianalyses.kubernaut.ai \
  effectivenessassessments.kubernaut.ai notificationrequests.kubernaut.ai \
  remediationapprovalrequests.kubernaut.ai remediationrequests.kubernaut.ai \
  remediationworkflows.kubernaut.ai signalprocessings.kubernaut.ai \
  workflowexecutions.kubernaut.ai

kubectl delete namespace kubernaut-system
```

## Known Limitations

- **Single installation per cluster**: Cluster-scoped resources (ClusterRoles, ClusterRoleBindings, WebhookConfigurations) use static names.
- **Init container timeouts**: `wait-for-postgres` init containers have no timeout.
- **Event Exporter probes**: No liveness or readiness probes (no health endpoint).

## Documentation

- [Installation Guide](https://jordigilh.github.io/kubernaut-docs/getting-started/installation/) -- Full walkthrough
- [Configuration Reference](https://jordigilh.github.io/kubernaut-docs/user-guide/configuration/) -- All settings and tuning
- [Security & RBAC](https://jordigilh.github.io/kubernaut-docs/architecture/security-rbac/) -- RBAC model and signal source authentication
- [Architecture](https://jordigilh.github.io/kubernaut-docs/architecture/overview/) -- System design

## License

See [LICENSE](https://github.com/jordigilh/kubernaut/blob/main/LICENSE) in the project repository.
