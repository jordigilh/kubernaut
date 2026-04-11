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

## Quick Start

Infrastructure secrets must be pre-created before installing the chart (#557).
The chart does **not** auto-generate credentials — this prevents password leaks
via rendered Helm templates and avoids silent `lookup` failures on OCP / restricted-RBAC
environments.

The chart validates that required secrets exist at install/upgrade time and fails
with an actionable error if they are missing. This validation is automatically
skipped during `helm template` (no cluster access). **Note:** If the Helm installer
ServiceAccount lacks `get` permission on Namespaces, the validation is also
skipped — operators in restricted-RBAC environments must ensure secrets exist
manually before installing.

```bash
# 1. Create namespace
kubectl create namespace kubernaut-system

# 2. Create PostgreSQL + DataStorage credentials (single consolidated secret)
PG_PASSWORD=$(openssl rand -base64 24)
kubectl create secret generic postgresql-secret \
  --from-literal=POSTGRES_USER=slm_user \
  --from-literal=POSTGRES_PASSWORD="$PG_PASSWORD" \
  --from-literal=POSTGRES_DB=action_history \
  --from-literal=db-secrets.yaml="$(printf 'username: slm_user\npassword: %s' "$PG_PASSWORD")" \
  -n kubernaut-system

# 3. Create Valkey credentials
kubectl create secret generic valkey-secret \
  --from-literal=valkey-secrets.yaml="$(printf 'password: %s' "$(openssl rand -base64 24)")" \
  -n kubernaut-system

# 4. Create LLM credentials
kubectl create secret generic llm-credentials \
  --from-literal=OPENAI_API_KEY=sk-... \
  -n kubernaut-system

# 5. Install
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o
```

This deploys the full platform with:

- Default SignalProcessing Rego policy (environment, severity, priority, custom labels)
- Default AIAnalysis approval policy (production requires approval, non-production auto-approves)
- 25 ActionTypes and 20 RemediationWorkflows for common scenarios
- Console-only notifications (no external integrations required)
- Monitoring integrations disabled (enable when kube-prometheus-stack is installed)

Verify:

```bash
kubectl get pods -n kubernaut-system
```

### Enable Slack Notifications

```bash
# Create a Secret with your Slack webhook URL
kubectl create secret generic slack-webhook \
  --from-literal=webhook-url=https://hooks.slack.com/services/T.../B.../... \
  -n kubernaut-system

# Install with Slack enabled
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o \
  --set notification.slack.secretName=slack-webhook \
  --set notification.slack.channel="#ops-alerts"
```

### Enable Monitoring Integration

Install [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack), then:

```bash
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o \
  --set effectivenessmonitor.external.prometheusEnabled=true \
  --set effectivenessmonitor.external.alertManagerEnabled=true \
  --set gateway.auth.signalSources[0].name=alertmanager \
  --set gateway.auth.signalSources[0].serviceAccount=alertmanager-kube-prometheus-stack-alertmanager \
  --set gateway.auth.signalSources[0].namespace=monitoring
```

Configure AlertManager to send webhooks to the Gateway:

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

## Production Configuration

For production environments, use custom secret names and provide custom policies:

```bash
# 1. Create credential Secrets with custom names
PG_PASSWORD=$(openssl rand -base64 24)
kubectl create secret generic pg-credentials \
  --from-literal=POSTGRES_USER=kubernaut \
  --from-literal=POSTGRES_PASSWORD="$PG_PASSWORD" \
  --from-literal=POSTGRES_DB=kubernaut \
  --from-literal=db-secrets.yaml="$(printf 'username: kubernaut\npassword: %s' "$PG_PASSWORD")" \
  -n kubernaut-system

kubectl create secret generic vk-credentials \
  --from-literal=valkey-secrets.yaml="$(printf 'password: %s' "$(openssl rand -base64 24)")" \
  -n kubernaut-system

kubectl create secret generic llm-credentials \
  --from-literal=OPENAI_API_KEY=sk-... \
  -n kubernaut-system

# 2. Install with production overrides
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set postgresql.auth.existingSecret=pg-credentials \
  --set valkey.existingSecret=vk-credentials \
  --set demoContent.enabled=false \
  --set-file kubernautAgent.sdkConfigContent=my-sdk-config.yaml \
  --set-file signalprocessing.policy=my-policy.rego \
  --set-file aianalysis.policies.content=my-approval.rego
```

### BYO PostgreSQL / Valkey

When using external PostgreSQL, the secret referenced by `existingSecret` must
contain **both** the `POSTGRES_*` env-var keys **and** the `db-secrets.yaml` key
(DataStorage reads credentials from this file). Chart validation is skipped for
BYO infrastructure — ensure secrets exist before installing.

```bash
# BYO PostgreSQL secret — must include db-secrets.yaml for DataStorage
kubectl create secret generic my-pg-credentials \
  --from-literal=POSTGRES_USER=myuser \
  --from-literal=POSTGRES_PASSWORD=mypass \
  --from-literal=POSTGRES_DB=mydb \
  --from-literal=db-secrets.yaml="$(printf 'username: myuser\npassword: mypass')" \
  -n kubernaut-system

# BYO Valkey secret
kubectl create secret generic my-valkey-credentials \
  --from-literal=valkey-secrets.yaml="$(printf 'password: mypass')" \
  -n kubernaut-system
```

```yaml
postgresql:
  enabled: false
  host: "db.example.com"
  auth:
    existingSecret: my-pg-credentials

valkey:
  enabled: false
  host: "redis.example.com"
  existingSecret: my-valkey-credentials
```

### OpenShift (OCP)

```bash
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  -n kubernaut-system \
  -f charts/kubernaut/values-ocp.yaml \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o
```

The OCP overlay switches PostgreSQL and Valkey to Red Hat RHEL10 catalog images and replaces `bitnami/kubectl` with `ose-cli` for hook jobs. No ImageStream prerequisites -- pods pull directly from `registry.redhat.io` using the cluster's global pull secret.

## Configuration Reference

All values are validated against `values.schema.json`. Run `helm lint` to check your overrides.

### Global

| Parameter | Description | Default |
|---|---|---|
| `global.image.registry` | Container image registry | `quay.io` |
| `global.image.namespace` | Image namespace prefix | `kubernaut-ai` |
| `global.image.separator` | Namespace-to-service separator (`/` nested, `-` flat) | `/` |
| `global.image.tag` | Image tag (defaults to `appVersion`) | `""` |
| `global.image.digest` | Image digest (overrides tag when set) | `""` |
| `global.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `global.nodeSelector` | Global node selector | `{}` |
| `global.tolerations` | Global tolerations | `[]` |

### Demo Content

| Parameter | Description | Default |
|---|---|---|
| `demoContent.enabled` | Deploy bundled ActionTypes + RemediationWorkflows | `true` |

### Kubernaut Agent (LLM)

| Parameter | Description | Default |
|---|---|---|
| `kubernautAgent.llm.credentialsSecretName` | Secret with LLM API keys (e.g., `OPENAI_API_KEY`) | `llm-credentials` |
| `kubernautAgent.llm.provider` | LLM provider for quickstart (`openai`, `anthropic`) | `""` |
| `kubernautAgent.llm.model` | LLM model for quickstart (`gpt-4o`, `claude-sonnet-4-20250514`) | `""` |
| `kubernautAgent.sdkConfigContent` | Full SDK config YAML (via `--set-file`; overrides provider/model) | `""` |
| `kubernautAgent.existingSdkConfigMap` | Pre-existing ConfigMap for SDK config (highest priority) | `""` |
| `kubernautAgent.prometheus.enabled` | Enable Prometheus toolset in auto-generated SDK config | `false` |
| `kubernautAgent.prometheus.url` | Prometheus/Thanos URL | `""` |
| `kubernautAgent.prometheus.ocpMonitoringRbac` | Create RBAC for OCP monitoring stack | `false` |
| `kubernautAgent.prometheus.tls.enabled` | Enable TLS CA trust for Prometheus connections | `false` |
| `kubernautAgent.prometheus.tls.caConfigMapName` | ConfigMap with CA PEM (leave empty on OCP) | `""` |

**SDK config precedence**: `existingSdkConfigMap` > `sdkConfigContent` > `llm.provider`+`llm.model` > fail.

For Vertex AI, Azure, or advanced setups (toolsets, MCP servers), use `sdkConfigContent` or `existingSdkConfigMap`. See `examples/sdk-config.yaml`.

### SignalProcessing

| Parameter | Description | Default |
|---|---|---|
| `signalprocessing.policy` | Rego policy content (via `--set-file`) | Embedded default |
| `signalprocessing.existingPolicyConfigMap` | Pre-existing ConfigMap with `policy.rego` key | `""` |
| `signalprocessing.proactiveSignalMappings.content` | Proactive signal mappings YAML | Embedded default |
| `signalprocessing.proactiveSignalMappings.existingConfigMap` | Pre-existing ConfigMap | `""` |

### AIAnalysis

| Parameter | Description | Default |
|---|---|---|
| `aianalysis.policies.content` | Approval policy Rego (via `--set-file`) | Embedded default |
| `aianalysis.policies.existingConfigMap` | Pre-existing ConfigMap with `approval.rego` key | `""` |
| `aianalysis.rego.confidenceThreshold` | Confidence threshold for auto-approval (nil = Rego default 0.8) | `null` |

### Notification

| Parameter | Description | Default |
|---|---|---|
| `notification.slack.secretName` | Secret with Slack webhook URL (enables Slack) | `""` |
| `notification.slack.secretKey` | Key in Secret containing the webhook URL | `webhook-url` |
| `notification.slack.channel` | Slack channel | `#kubernaut-alerts` |
| `notification.routing.content` | Full routing YAML (via `--set-file`; overrides slack shortcut) | `""` |
| `notification.routing.existingConfigMap` | Pre-existing routing ConfigMap (highest priority) | `""` |
| `notification.credentials` | Additional projected volume sources from Secrets | `[]` |

### WorkflowExecution

| Parameter | Description | Default |
|---|---|---|
| `workflowexecution.config.execution.cooldownPeriod` | Cooldown between workflow executions | `1m` |
| `workflowexecution.config.ansible.apiURL` | AWX/AAP API URL (enables Ansible engine) | _(not set)_ |
| `workflowexecution.config.ansible.insecure` | Skip TLS verification for AWX API | `false` |
| `workflowexecution.config.ansible.organizationID` | AWX organization ID | `1` |
| `workflowexecution.config.ansible.tokenSecretRef.name` | Secret containing AWX API token | `""` |
| `workflowexecution.config.ansible.tokenSecretRef.key` | Key within the Secret | `token` |
| `workflowexecution.config.ansible.tokenSecretRef.namespace` | Secret namespace (defaults to release namespace) | _(release ns)_ |

### Gateway

| Parameter | Description | Default |
|---|---|---|
| `gateway.auth.signalSources` | External signal sources needing RBAC | `[]` |
| `gateway.service.type` | Service type | `ClusterIP` |

### EffectivenessMonitor

| Parameter | Description | Default |
|---|---|---|
| `effectivenessmonitor.external.prometheusEnabled` | Enable Prometheus integration | `false` |
| `effectivenessmonitor.external.prometheusUrl` | Prometheus URL | `http://kube-prometheus-stack-prometheus.monitoring.svc:9090` |
| `effectivenessmonitor.external.alertManagerEnabled` | Enable AlertManager integration | `false` |
| `effectivenessmonitor.external.alertManagerUrl` | AlertManager URL | `http://kube-prometheus-stack-alertmanager.monitoring.svc:9093` |

### Infrastructure

| Parameter | Description | Default |
|---|---|---|
| `postgresql.enabled` | Deploy in-chart PostgreSQL | `true` |
| `postgresql.auth.existingSecret` | Pre-created Secret name (empty = expect `postgresql-secret`) | `""` |
| `postgresql.variant` | Image variant: `upstream` or `ocp` | `upstream` |
| `postgresql.host` | External host (when `enabled=false`) | `""` |
| `datastorage.dbExistingSecret` | DEPRECATED: db-secrets.yaml is now in postgresql-secret | `""` |
| `valkey.enabled` | Deploy in-chart Valkey | `true` |
| `valkey.existingSecret` | Pre-created Secret name (empty = expect `valkey-secret`) | `""` |
| `valkey.host` | External host (when `enabled=false`) | `""` |

### TLS

| Parameter | Description | Default |
|---|---|---|
| `tls.mode` | `hook` (self-signed), `cert-manager` (production), or `manual` | `hook` |
| `tls.certManager.issuerRef.name` | Issuer name (required when mode=cert-manager) | `""` |

### NetworkPolicies

| Parameter | Description | Default |
|---|---|---|
| `networkPolicies.enabled` | Enable default-deny NetworkPolicies for all services | `true` |
| `networkPolicies.apiServerCIDR` | K8s API server CIDR (e.g., `10.96.0.1/32`) | `""` |
| `networkPolicies.monitoring.namespace` | Namespace for Prometheus metrics scraping ingress | `""` |
| `networkPolicies.monitoring.prometheusPort` | Prometheus port (9090 vanilla, 9091 OCP) | `9090` |
| `networkPolicies.monitoring.alertManagerPort` | AlertManager port (9093 vanilla, 9094 OCP) | `9093` |
| `networkPolicies.externalWebhooks.cidr` | CIDR for Slack/PagerDuty/Teams webhook egress | `0.0.0.0/0` |
| `networkPolicies.externalRegistry.cidr` | CIDR for OCI registry egress (datastorage bundle validation) | `0.0.0.0/0` |
| `networkPolicies.<service>.enabled` | Per-service toggle (gateway, datastorage, etc.) | `true` |

When enabled, each service gets a NetworkPolicy with:
- **Default-deny ingress** with service-specific allow rules
- **Egress**: most services restrict egress to DNS, K8s API, and known peers; **Kubernaut Agent uses an ingress-only policy** (unrestricted egress) because it must reach arbitrary LLM providers, MCP servers, and tool endpoints
- **Datastorage**: allows egress to PostgreSQL, Valkey, and external container registries (configurable CIDR for OCI bundle validation)

Example:

```bash
helm install kubernaut charts/kubernaut \
  --set networkPolicies.enabled=true \
  --set networkPolicies.apiServerCIDR=10.96.0.1/32 \
  --set networkPolicies.monitoring.namespace=monitoring \
  --set "networkPolicies.gateway.ingressNamespaces[0]=monitoring"
```

On OpenShift, the `values-ocp.yaml` overlay sets monitoring ports to 9091/9094.

### Common Controller Parameters

All controllers accept: `replicas`, `resources`, `pdb.{enabled,minAvailable,maxUnavailable}`, `podSecurityContext`, `containerSecurityContext`, `nodeSelector`, `tolerations`, `affinity`, `topologySpreadConstraints`.

## Disconnected / Air-Gapped Install

The chart OCI artifact is fully self-contained (policies, demo content, credential auto-generation). For airgapped environments, mirror container images and override the registry:

```bash
# Nested registry (Harbor, Artifactory)
helm install kubernaut oci://harbor.corp/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set global.image.registry=harbor.corp \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o

# Flat registry (quay.io, Docker Hub, OCP internal)
helm install kubernaut oci://quay.io/myorg/charts/kubernaut \
  --namespace kubernaut-system \
  --set global.image.registry=quay.io/myorg \
  --set global.image.separator=- \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o
```

See the [Disconnected Install Guide](https://jordigilh.github.io/kubernaut-docs/operations/disconnected-install/) for image mirroring and OCP IDMS instructions.

## Upgrading

Helm does **not** upgrade CRDs on `helm upgrade`. Apply new CRDs first:

```bash
helm pull oci://quay.io/kubernaut-ai/charts/kubernaut \
  --version <new-version> --untar
kubectl apply --server-side --force-conflicts -f kubernaut/crds/

helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --version <new-version> \
  -n kubernaut-system -f my-values.yaml
```

> **Warning — Do not `kubectl patch` Helm-managed ConfigMaps** (#539):
> Using `kubectl patch` on chart-managed ConfigMaps (e.g., `kubernaut-agent-sdk-config`,
> `workflowexecution-config`) transfers field ownership to the `kubectl-patch` field
> manager. Subsequent `helm upgrade` will fail with a server-side apply conflict.
>
> Instead, use Helm values at install/upgrade time:
> - **Prometheus toolset**: `--set kubernautAgent.prometheus.enabled=true --set kubernautAgent.prometheus.url=<url>`
> - **Ansible/AAP engine**: `--set workflowexecution.config.ansible.apiURL=<url>`
>
> If you already have conflicting ConfigMaps, delete them before upgrading — Helm
> will recreate them with the correct values:
> ```bash
> kubectl delete cm kubernaut-agent-sdk-config workflowexecution-config -n kubernaut-system
> helm upgrade kubernaut ... -f my-values.yaml
> ```

## Uninstalling

```bash
helm uninstall kubernaut -n kubernaut-system

# Full cleanup (PVCs, cluster resources, CRDs)
kubectl delete pvc postgresql-data valkey-data -n kubernaut-system
kubectl delete clusterrole kubernaut-hook-role --ignore-not-found
kubectl delete clusterrolebinding kubernaut-hook-rolebinding --ignore-not-found
kubectl delete crd actiontypes.kubernaut.ai aianalyses.kubernaut.ai \
  effectivenessassessments.kubernaut.ai notificationrequests.kubernaut.ai \
  remediationapprovalrequests.kubernaut.ai remediationrequests.kubernaut.ai \
  remediationworkflows.kubernaut.ai signalprocessings.kubernaut.ai \
  workflowexecutions.kubernaut.ai
kubectl delete namespace kubernaut-system
```

## Known Limitations

- **Single installation per cluster**: Cluster-scoped resources use static names.
- **`helm template` and auto-generated credentials**: `lookup` returns nil during `helm template`, so random passwords are generated on each dry-run. Use `helm install` directly or provide `existingSecret` for reproducible output.

## Documentation

- [Installation Guide](https://jordigilh.github.io/kubernaut-docs/getting-started/installation/)
- [Configuration Reference](https://jordigilh.github.io/kubernaut-docs/user-guide/configuration/)
- [Security & RBAC](https://jordigilh.github.io/kubernaut-docs/architecture/security-rbac/)
- [Architecture](https://jordigilh.github.io/kubernaut-docs/architecture/overview/)

## License

See [LICENSE](https://github.com/jordigilh/kubernaut/blob/main/LICENSE) in the project repository.
