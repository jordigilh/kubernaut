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

- Kubernetes Jobs (built-in, always available)
- Tekton Pipelines (auto-discovered via CRDs; [Issue #868](https://github.com/jordigilh/kubernaut/issues/868))
- Ansible Automation Platform (AAP) / AWX (config-gated)

## Quick Start

Infrastructure secrets must be pre-created before installing the chart (#557).
The chart does **not** auto-generate credentials — this prevents password leaks
via rendered Helm templates and avoids silent `lookup` failures on restricted-RBAC
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

# 5. Install (--set-file for Rego policies is mandatory)
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o \
  --set-file signalprocessing.policies.content=path/to/policy.rego \
  --set-file aianalysis.policies.content=path/to/approval.rego
```

This deploys the full platform with:

- User-provided SignalProcessing Rego policy (via `--set-file signalprocessing.policies.content=`)
- User-provided AIAnalysis approval policy (via `--set-file aianalysis.policies.content=`)
- Console-only notifications (no external integrations required)
- Monitoring integrations disabled (enable when kube-prometheus-stack is installed)

> **Note**: The chart does not bundle default Rego policies. You must provide your own
> via `--set-file` or by specifying an `existingConfigMap`. For reference policies, see
> the [kubernaut-demo-scenarios](https://github.com/jordigilh/kubernaut-demo-scenarios) repository.

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
  --set-file signalprocessing.policies.content=path/to/policy.rego \
  --set-file aianalysis.policies.content=path/to/approval.rego \
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
  --set-file signalprocessing.policies.content=path/to/policy.rego \
  --set-file aianalysis.policies.content=path/to/approval.rego \
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

#### ServiceMonitor, PrometheusRule, and Autoscaling (BR-PLATFORM-003)

If the Prometheus Operator CRDs (`monitoring.coreos.com/v1`) are installed
(e.g. via `kube-prometheus-stack`), the chart can generate `ServiceMonitor` and
`PrometheusRule` resources for observability parity with the Kubernaut Operator:

```bash
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set monitoring.serviceMonitor.enabled=true \
  --set monitoring.prometheusRule.enabled=true \
  ...
```

- `monitoring.serviceMonitor.enabled=true` creates a `ServiceMonitor` (scraping `/metrics` every
  15s) for every metrics-emitting service. Not created for `authwebhook`, which intentionally
  does not expose metrics.
- `monitoring.prometheusRule.enabled=true` creates alerting rules for DataStorage and
  APIFrontend (availability, latency, error rate, circuit breakers), ported from the Kubernaut
  Operator's `internal/resources/monitoring.go`. It also controls the pre-existing Kubernaut
  Agent interactive-session SLO rules (Issue #1005).
- Both are a no-op — render nothing — when the `monitoring.coreos.com/v1` CRD is not present on
  the cluster, even if `enabled=true`. Safe to set unconditionally in a values file shared
  across clusters with and without the Prometheus Operator installed.

DataStorage and APIFrontend can additionally scale via a `HorizontalPodAutoscaler`
(`autoscaling/v2`, a stable core API — no CRD required):

```bash
--set datastorage.autoscaling.enabled=true \
--set apifrontend.autoscaling.enabled=true
```

Defaults: `minReplicas: 1`, `maxReplicas: 5`, CPU target `75%`, memory target `80%`
(`datastorage.autoscaling.*` / `apifrontend.autoscaling.*`).

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
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o \
  --set-file signalprocessing.policies.content=my-policy.rego \
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

### Optional: Keyed Audit Hash Chain (GAP-05)

DataStorage tamper-evidences audit events with a hash chain. By default this
uses a plain (unkeyed) SHA256 hash — sufficient to detect accidental
corruption, but recomputable by anyone with database read/write access, so an
attacker with DB privileges could tamper with a row and recompute a
self-consistent chain.

Enabling `datastorage.config.auditHashKey` switches **newly written** events to
a keyed HMAC-SHA256 hash chain, using a secret key stored outside the database
(a Kubernetes Secret). Forging a valid hash without that key is computationally
infeasible. This is opt-in and backward compatible: existing events keep
verifying under the legacy algorithm, and disabling it later simply reverts new
writes to the unkeyed algorithm.

```bash
# Pre-create the HMAC key Secret (chart does not auto-generate it, same as
# the PostgreSQL/Valkey secrets above)
HMAC_KEY=$(openssl rand -base64 32)
kubectl create secret generic datastorage-audit-hmac-key \
  --from-literal=audit-hmac-key.yaml="$(printf 'hmacKey: %s' "$HMAC_KEY")" \
  -n kubernaut-system

helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --reuse-values \
  --set datastorage.config.auditHashKey.enabled=true
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

### Optional: Distributed JWT Replay Cache (GAP-08)

APIFrontend detects replayed JWTs via their `jti` claim. By default this uses
an in-memory cache that is per-process: in a multi-replica deployment, a token
replayed against a different replica than the one that first observed it is
not detected.

Enabling `apifrontend.config.auth.replayCache` shares replay state across all
APIFrontend replicas via the cluster's Valkey instance (the same instance and
Secret already used by DataStorage) — closing this HA gap. If Valkey is
unreachable at runtime, APIFrontend falls back to the in-memory cache and logs
the degradation rather than disabling replay protection outright.

```bash
helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --reuse-values \
  --set apifrontend.config.auth.replayCache.enabled=true
```

No additional Secret is required beyond the existing Valkey credentials
(`valkey.existingSecret` / the `valkey-secret` created above), since the
replay cache uses that same shared instance and Secret.

### Optional: Data Storage Per-IP Rate Limiting (GAP-09)

The Data Storage HTTP API does not rate limit requests by default — existing
deployments may already enforce this at an external ingress/proxy layer.
Enabling `datastorage.config.server.rateLimit` adds an in-process, per-IP
token-bucket limiter (SC-5 DoS protection) as a defense-in-depth backstop.
Denied requests are self-audited (`datastorage.ratelimit.denied`, FedRAMP
AU-12).

```bash
helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --reuse-values \
  --set datastorage.config.server.rateLimit.enabled=true
```

Tune `datastorage.config.server.rateLimit.requestsPerSecond` (default `50`)
and `.burst` (default `100`) to match expected legitimate traffic (e.g.
KubernautAgent/APIFrontend polling volume) before enabling in production.

### OpenShift (OCP)

> **DEPRECATED v1.4 (Issue #848)**: The OCP Helm chart overlay is deprecated and will be
> **removed in v1.5**. For OpenShift deployments, use the
> [Kubernaut Operator](https://jordigilh.github.io/kubernaut-docs/operations/operator/)
> which provides native OCP integration (service-ca TLS, OLM catalog, SCC management,
> automated upgrades). See the
> [Helm-to-Operator Migration Guide](https://jordigilh.github.io/kubernaut-docs/operations/helm-to-operator/).

```bash
# DEPRECATED — use the Kubernaut Operator instead (available since v1.3)
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  -n kubernaut-system \
  -f charts/kubernaut/values-ocp.yaml \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o \
  --set-file signalprocessing.policies.content=path/to/policy.rego \
  --set-file aianalysis.policies.content=path/to/approval.rego
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
| `global.fleet.mcpGatewayEndpoint` | Shared MCP Gateway endpoint URL, used as fallback when a service's own `fleet.mcpGatewayEndpoint` is unset | `""` |
| `global.fleet.mcpGatewayType` | Shared MCP Gateway type (`eaigw` or `kuadrant`), used as fallback when a service's own `fleet.mcpGatewayType` is unset | `""` |
| `global.fleet.tlsCAFile` | Shared CA bundle for verifying the MCP Gateway's TLS cert, used as fallback when a service's own `fleet.tlsCAFile` is unset | `""` |
| `global.fleet.oauth2.tokenURL` | Shared MCP Gateway OAuth2 token URL, used as fallback when a service's own `fleet.oauth2.tokenURL` is unset | `""` |
| `global.fleet.oauth2.credentialsSecretRef` | Shared MCP Gateway OAuth2 credentials Secret (keys: `client-id`, `client-secret`), used as fallback | `""` |
| `global.fleet.oauth2.scopes` | Shared MCP Gateway OAuth2 scopes, used as fallback | `[]` |
| `global.fleet.oauth2.tlsCAFile` | Shared CA bundle for verifying `tokenURL`'s TLS cert, used as fallback | `""` |

Every fleet-integration-capable service (`gateway`, `signalprocessing`, `remediationorchestrator`, `effectivenessmonitor`, `apifrontend`, `fleetmetadatacache`) points at the same physical MCP Gateway instance, so set its endpoint, type, CA bundle, and OAuth2 credentials once here instead of duplicating them per service. Each service's own `fleet.mcpGatewayEndpoint` / `fleet.mcpGatewayType` / `fleet.tlsCAFile` / `fleet.oauth2.*` (or, for `fleetmetadatacache`, top-level equivalents) still takes precedence when set. Per-service `fleet.enabled` / `fleet.oauth2.enabled` (fleet integration on/off) remains independent per service and is not controlled by these globals.

`gateway.fleet.mcpGatewayEndpoint`, `remediationorchestrator.fleet.mcpGatewayEndpoint`, and `fleetmetadatacache.mcpGatewayEndpoint` are required (directly or via the global fallback) when their respective `fleet.enabled` / `fleetmetadatacache.enabled` is `true` — `helm install`/`upgrade` fails fast with a remediation message if neither is set. It's optional for `effectivenessmonitor` and `apifrontend`, where an empty value just means those services fall back to reading local-cluster-only state instead of federating through the MCP Gateway.

### Kubernaut Agent (LLM)

| Parameter | Description | Default |
|---|---|---|
| `kubernautAgent.llm.credentialsSecretName` | Secret with LLM API keys (e.g., `OPENAI_API_KEY`) | `llm-credentials` |
| `kubernautAgent.llm.provider` | LLM provider for quickstart (`openai`, `anthropic`) | `""` |
| `kubernautAgent.llm.tlsCaFile` | PEM CA cert path for internal LLM endpoints behind private CA | `""` |
| `kubernautAgent.llm.model` | LLM model for quickstart (`gpt-4o`, `claude-sonnet-4-20250514`) | `""` |
| `kubernautAgent.llm.oauth2.enabled` | Enable OAuth2 client credentials grant for LLM gateway | `false` |
| `kubernautAgent.llm.oauth2.tokenURL` | OAuth2 token endpoint URL | `""` |
| `kubernautAgent.llm.oauth2.credentialsSecretRef` | Secret with `client-id` and `client-secret` keys (mounted as files) | `""` |
| `kubernautAgent.prometheus.enabled` | Enable Prometheus toolset | `false` |
| `kubernautAgent.prometheus.url` | Prometheus/Thanos URL | `""` |
| `kubernautAgent.prometheus.tls.enabled` | Enable TLS CA trust for Prometheus connections | `false` |
| `kubernautAgent.prometheus.tls.caConfigMapName` | ConfigMap with CA PEM | `""` |

All LLM configuration is now part of the main `kubernaut-agent-config` ConfigMap. OAuth2 credentials are mounted from a Secret as files (never exposed as environment variables).

### SignalProcessing

| Parameter | Description | Default |
|---|---|---|
| `signalprocessing.policies.content` | Rego policy content (via `--set-file`) — **required** | `""` |
| `signalprocessing.policies.existingConfigMap` | Pre-existing ConfigMap with `policy.rego` key | `""` |
| `signalprocessing.proactiveSignalMappings.content` | Proactive signal mappings YAML (via `--set-file`) | `""` |
| `signalprocessing.proactiveSignalMappings.existingConfigMap` | Pre-existing ConfigMap | `""` |

### AIAnalysis

| Parameter | Description | Default |
|---|---|---|
| `aianalysis.policies.content` | Approval policy Rego (via `--set-file`) — **required** | `""` |
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
| `workflowexecution.config.tekton.enabled` | `true`/omit = auto-discover Tekton CRDs; `false` = disable (#868) | _(auto-discover)_ |
| `workflowexecution.config.ansible.apiURL` | AWX/AAP API URL (enables Ansible engine) | _(not set)_ |
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
| `postgresql.host` | External host (when `enabled=false`) | `""` |
| `datastorage.dbExistingSecret` | DEPRECATED: db-secrets.yaml is now in postgresql-secret | `""` |
| `valkey.enabled` | Deploy in-chart Valkey | `true` |
| `valkey.existingSecret` | Pre-created Secret name (empty = expect `valkey-secret`) | `""` |
| `valkey.host` | External host (when `enabled=false`) | `""` |
| `datastorage.config.auditHashKey.enabled` | Enable keyed HMAC-SHA256 audit hash chain (GAP-05); see [Optional: Keyed Audit Hash Chain](#optional-keyed-audit-hash-chain-gap-05) | `false` |
| `apifrontend.config.auth.replayCache.enabled` | Enable distributed (Valkey-backed) JWT replay cache (GAP-08); see [Optional: Distributed JWT Replay Cache](#optional-distributed-jwt-replay-cache-gap-08) | `false` |
| `apifrontend.config.auth.replayCache.redisDB` | Valkey logical DB index used for replay cache keys | `1` |
| `datastorage.config.server.rateLimit.enabled` | Enable per-IP rate limiting on the DS HTTP API (GAP-09); see [Optional: Data Storage Per-IP Rate Limiting](#optional-data-storage-per-ip-rate-limiting-gap-09) | `false` |
| `datastorage.config.server.rateLimit.requestsPerSecond` | Sustained per-IP requests/second | `50` |
| `datastorage.config.server.rateLimit.burst` | Per-IP token bucket burst size | `100` |
| `datastorage.config.auditHashKey.existingSecret` | Pre-created Secret name (empty = expect `datastorage-audit-hmac-key`) | `""` |

### TLS

| Parameter | Description | Default |
|---|---|---|
| `tls.mode` | `hook` (self-signed), `cert-manager` (production), or `manual` | `hook` |
| `tls.certManager.issuerRef.name` | Issuer/ClusterIssuer name. When mode=cert-manager and left empty, auto-selected via `lookup` if exactly one exists in the cluster (real `helm install`/`upgrade` only); required if rendering via `helm template`/GitOps or if multiple issuers exist | `""` |

### NetworkPolicies

| Parameter | Description | Default |
|---|---|---|
| `networkPolicies.enabled` | Enable default-deny NetworkPolicies for all services | `true` |
| `networkPolicies.apiServerCIDR` | K8s API server real backend endpoint CIDR (e.g., `10.89.0.2/32` -- NOT the `kubernetes` Service ClusterIP). Usually left empty: auto-discovered via `lookup` on a real `helm install`/`upgrade`; required if rendering via `helm template`/GitOps | `""` |
| `networkPolicies.apiServerCIDRs` | Additional API server backend endpoint CIDRs for HA (multiple control-plane nodes). Merged with `apiServerCIDR`; usually left empty (see above) | `[]` |
| `networkPolicies.apiServerPort` | API server backend endpoint port (commonly 6443). `0` = auto-discover alongside the CIDR | `0` |
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

Example (API server CIDR/port are auto-discovered here; only set them explicitly for `helm template`/GitOps rendering, see [NetworkPolicy API Server Discovery](#networkpolicy-api-server-discovery)):

```bash
helm install kubernaut charts/kubernaut \
  --set networkPolicies.enabled=true \
  --set networkPolicies.monitoring.namespace=monitoring \
  --set "networkPolicies.gateway.ingressNamespaces[0]=monitoring"
```

#### NetworkPolicy API Server Discovery

During a real `helm install`/`upgrade`, the chart auto-discovers the kube-apiserver's real backend endpoint IP(s) and port via `lookup` against the live `kubernetes` Endpoints object, so `networkPolicies.apiServerCIDR(s)`/`apiServerPort` usually don't need to be set. Set them explicitly if:

- **You render via `helm template` for GitOps (ArgoCD, Flux).** These tools render manifests via `helm template` internally, not a live install/upgrade, so `lookup` always returns empty -- auto-discovery never applies.
- **The Helm installer ServiceAccount lacks permission to read `Endpoints` in the `default` namespace.** The chart fails with a clear error in this case rather than silently omitting the rule.
- **Auto-discovery picks the wrong address**, or you want to pin a specific one.

If neither an override nor discovery succeeds, `helm install`/`upgrade` fails with remediation instructions (see `kubectl get endpoints kubernetes -o wide` to find the real address manually). Use `apiServerCIDR` for single-control-plane clusters; use `apiServerCIDRs` (a list) for HA clusters with multiple control-plane nodes -- auto-discovery already collects every backend address automatically.

### Common Controller Parameters

All controllers accept: `replicas`, `resources`, `pdb.{enabled,minAvailable,maxUnavailable}`, `podSecurityContext`, `containerSecurityContext`, `nodeSelector`, `tolerations`, `affinity`, `topologySpreadConstraints`.

## Disconnected / Air-Gapped Install

For airgapped environments, mirror container images and override the registry. Rego policies must still be provided via `--set-file`:

```bash
# Nested registry (Harbor, Artifactory)
helm install kubernaut oci://harbor.corp/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set global.image.registry=harbor.corp \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o \
  --set-file signalprocessing.policies.content=path/to/policy.rego \
  --set-file aianalysis.policies.content=path/to/approval.rego

# Flat registry (quay.io, Docker Hub)
helm install kubernaut oci://quay.io/myorg/charts/kubernaut \
  --namespace kubernaut-system \
  --set global.image.registry=quay.io/myorg \
  --set global.image.separator=- \
  --set kubernautAgent.llm.provider=openai \
  --set kubernautAgent.llm.model=gpt-4o \
  --set-file signalprocessing.policies.content=path/to/policy.rego \
  --set-file aianalysis.policies.content=path/to/approval.rego
```

See the [Disconnected Install Guide](https://jordigilh.github.io/kubernaut-docs/operations/disconnected-install/) for image mirroring instructions.

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
> Using `kubectl patch` on chart-managed ConfigMaps (e.g., `kubernaut-agent-config`,
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
> kubectl delete cm kubernaut-agent-config workflowexecution-config -n kubernaut-system
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
