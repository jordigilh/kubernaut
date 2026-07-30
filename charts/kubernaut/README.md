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

# 4. Create LLM credentials (key must be "api_key" -- the flat filename
#    Kubernaut mounts and reads via the resolved profile's apiKeyFile;
#    vertex_ai profiles use a "credentials.json" service-account key instead)
kubectl create secret generic llm-credentials \
  --from-literal=api_key=sk-... \
  -n kubernaut-system

# 5. Install (--set-file for Rego policies is mandatory)
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set global.llmProfiles.primary.provider=openai \
  --set global.llmProfiles.primary.model=gpt-4o \
  --set global.llmProfiles.primary.endpoint=https://api.openai.com/v1 \
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials \
  --set kubernautAgent.llmProfileRef=primary \
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
  --set global.llmProfiles.primary.provider=openai \
  --set global.llmProfiles.primary.model=gpt-4o \
  --set global.llmProfiles.primary.endpoint=https://api.openai.com/v1 \
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials \
  --set kubernautAgent.llmProfileRef=primary \
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
  --set global.llmProfiles.primary.provider=openai \
  --set global.llmProfiles.primary.model=gpt-4o \
  --set global.llmProfiles.primary.endpoint=https://api.openai.com/v1 \
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials \
  --set kubernautAgent.llmProfileRef=primary \
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

The chart bundles single-replica PostgreSQL and Valkey Deployments
(`postgresql.enabled=true` / `valkey.enabled=true` by default) purely for
convenience — quick installs, evaluation, and development. Neither has
replication or automated failover, and neither is part of Kubernaut's own
managed footprint — they're infrastructure Kubernaut depends on, not
infrastructure it operates.

**For production, we recommend running PostgreSQL and Valkey/Redis
separately in HA mode** — via a dedicated operator (e.g. CloudNativePG,
Valkey Operator) or a managed cloud service — and pointing Kubernaut at
them as BYO infrastructure (`postgresql.enabled=false` / `valkey.enabled=false`
plus `host`; see [BYO PostgreSQL / Valkey](#byo-postgresql--valkey) below).

The example below still uses the bundled, single-replica databases (just
with custom secret names) — it's a starting point for locking down secrets,
not a substitute for the BYO+HA recommendation above.

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
  --from-literal=api_key=sk-... \
  -n kubernaut-system

# 2. Install with production overrides
helm install kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set postgresql.auth.existingSecret=pg-credentials \
  --set valkey.existingSecret=vk-credentials \
  --set global.llmProfiles.primary.provider=openai \
  --set global.llmProfiles.primary.model=gpt-4o \
  --set global.llmProfiles.primary.endpoint=https://api.openai.com/v1 \
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials \
  --set kubernautAgent.llmProfileRef=primary \
  --set-file signalprocessing.policies.content=my-policy.rego \
  --set-file aianalysis.policies.content=my-approval.rego
```

### BYO PostgreSQL / Valkey

**Recommended for production** — run PostgreSQL and Valkey/Redis separately
in HA mode and point Kubernaut at them, instead of the chart's bundled
single-replica Deployments.

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

### Keyed Audit Hash Chain (GAP-05)

DataStorage tamper-evidences audit events with a keyed HMAC-SHA256 hash chain
(mandatory, no toggle — [DD-PLATFORM-006](../../docs/architecture/decisions/DD-PLATFORM-006-helm-chart-configuration-surface-reduction.md)
DA6), using a secret key stored outside the database (a Kubernetes Secret).
Forging a valid hash without that key is computationally infeasible — a
meaningfully stronger guarantee than a plain unkeyed hash, which anyone with
database read/write access could recompute after tampering with a row.

With the default `tls.mode: hook` (see [TLS Certificate Provisioning](#tls-certificate-provisioning)
below), the chart's `pre-install`/`pre-upgrade` hook auto-generates this key
on first install and **never rotates it** afterward — regenerating it would
silently corrupt hash-chain verification for every audit record already
hashed with the old key. The generated Secret is not tracked by Helm and is
intentionally excluded from the hook's own uninstall cleanup, so it also
survives `helm uninstall` (and a subsequent reinstall picks the same key back
up). No action is required to enable this — it is on by default.

If `tls.mode` is `cert-manager` or `manual`, or if you set
`datastorage.config.auditHashKey.existingSecret` to BYO your own key (same
contract as the PostgreSQL/Valkey secrets above), the chart does not
auto-generate anything — pre-create the Secret yourself:

```bash
HMAC_KEY=$(openssl rand -base64 32)
kubectl create secret generic datastorage-audit-hmac-key \
  --from-literal=audit-hmac-key.yaml="$(printf 'hmacKey: %s' "$HMAC_KEY")" \
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

### Distributed JWT Replay Cache (GAP-08)

APIFrontend detects replayed JWTs via their `jti` claim, sharing replay state
across all APIFrontend replicas via the cluster's Valkey instance (the same
instance and Secret already used by DataStorage; mandatory, no toggle —
[DD-PLATFORM-006](../../docs/architecture/decisions/DD-PLATFORM-006-helm-chart-configuration-surface-reduction.md)
DA6). This closes the HA gap a per-process in-memory cache would have: without
it, a token replayed against a different replica than the one that first
observed it would go undetected. If Valkey is unreachable at runtime,
APIFrontend falls back to an in-memory cache and logs the degradation rather
than disabling replay protection outright.

No additional Secret is required beyond the existing Valkey credentials
(`valkey.existingSecret` / the `valkey-secret` created above), since the
replay cache uses that same shared instance and Secret. The connection to
Valkey is TLS-verified by default (`apifrontend.config.auth.replayCache.tls`),
matching Valkey's TLS-only listener — see [BYO PostgreSQL / Valkey](#byo-postgresql--valkey)
for pointing this at an external Valkey/Redis instance.

### Data Storage Per-IP Rate Limiting (GAP-09)

The Data Storage HTTP API applies an in-process, per-IP token-bucket limiter
(SC-5 DoS protection; mandatory, no toggle — [DD-PLATFORM-006](../../docs/architecture/decisions/DD-PLATFORM-006-helm-chart-configuration-surface-reduction.md)
DA6) as a defense-in-depth backstop, on top of whatever an external
ingress/proxy layer may already enforce. Denied requests are self-audited
(`datastorage.ratelimit.denied`, FedRAMP AU-12).

Tune `datastorage.config.server.rateLimit.requestsPerSecond` (default `50`)
and `.burst` (default `100`) to match expected legitimate traffic (e.g.
KubernautAgent/APIFrontend polling volume):

```bash
helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --reuse-values \
  --set datastorage.config.server.rateLimit.requestsPerSecond=200 \
  --set datastorage.config.server.rateLimit.burst=400
```

### Optional: Web Console (BR-PLATFORM-006)

The chart can deploy a standalone web console — a static SPA fronted by an `oauth2-proxy`
sidecar, giving users a browser-based A2A chat UI authenticated against the same OIDC
provider as APIFrontend — ported from the Kubernaut Operator's console feature. Opt-in,
disabled by default.

```bash
# 1. Pre-create the OIDC client Secret (client-id/client-secret from your OIDC provider;
#    cookie-secret is a random 32-byte value used to encrypt oauth2-proxy's session cookie)
kubectl create secret generic console-oauth-creds \
  --from-literal=client-id=kubernaut-console \
  --from-literal=client-secret="$OIDC_CLIENT_SECRET" \
  --from-literal=cookie-secret="$(openssl rand -base64 32 | head -c 32 | base64)" \
  -n kubernaut-system

# 2. Enable the console (requires apifrontend.config.auth.issuerURL or .jwtProviders to
#    already be configured, since the console reuses APIFrontend's OIDC provider) and
#    opt in to its Ingress for browser access
helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --reuse-values \
  --set console.enabled=true \
  --set console.auth.secretName=console-oauth-creds \
  --set console.ingress.host=console.apps.example.com \
  --set console.ingress.enabled=true
```

- `console.ingress.host` is **required** whenever `console.enabled=true` — even if you leave
  `console.ingress.enabled=false` (the default) to front the console with your own
  Ingress/Route — because oauth2-proxy needs the browser-facing hostname for its OIDC
  redirect URL regardless of who creates the Ingress.
- `console.ingress.enabled` is **opt-in, disabled by default** (BR-PLATFORM-009), same as
  `gateway.ingress.enabled`/`apifrontend.ingress.enabled`: Console is optional browser-facing
  UI tooling in front of APIFrontend that users may replace with their own UI or front with
  their own Ingress/Route/mesh gateway instead — not something the chart should expose
  externally without an explicit choice. This is a deliberate deviation from the Kubernaut
  Operator's `ConsoleRouteSpec` (which defaults to enabled=true, opt-out). When enabled, the
  chart creates a `networking.k8s.io/v1` Ingress — the vanilla-Kubernetes equivalent of the
  Operator's OCP Route.
- Gateway and APIFrontend have the same opt-in `<service>.ingress.enabled` knob
  (BR-PLATFORM-009, parity with the Operator's `GatewayRoute`/`APIFrontendRoute`) for their
  own external access, since both are machine-facing pipeline entry points where exposure is
  a deliberate security decision. Unlike Console, neither requires a `host` to be set.
- The chart fails fast at `helm template`/`helm install` time (before any resources are
  applied) if `console.enabled=true` and `console.auth.secretName`, a resolvable OIDC issuer,
  or `console.ingress.host` is missing.

### OpenShift (OCP)

This chart targets non-OpenShift (vanilla Kubernetes) deployments. For OpenShift, use the
[Kubernaut Operator](https://jordigilh.github.io/kubernaut-docs/operations/operator/)
instead, which provides native OCP integration (service-ca TLS, OLM catalog, SCC
management, automated upgrades). See the
[Helm-to-Operator Migration Guide](https://jordigilh.github.io/kubernaut-docs/operations/helm-to-operator/).

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
| `global.fleet.enabled` | Multi-cluster fleet federation on/off (BR-INTEGRATION-065, ADR-068), consolidated from four independent per-service toggles ([Issue #1707](https://github.com/jordigilh/kubernaut/issues/1707)) | `false` |
| `global.fleet.backend` | Federated scope-check backend for GW/RO: `""` (defaults to `fleetmetadatacache`), `fleetmetadatacache`, or `acm`. Enum-validated — `helm template`/`install` fails at render time for any other value ([Issue #1707](https://github.com/jordigilh/kubernaut/issues/1707) follow-up) | `""` |
| `global.fleet.endpoint` | Endpoint for the selected `backend`. Auto-derived for `fleetmetadatacache`; required for `acm` | `""` |
| `global.fleet.mcpGatewayEndpoint` | Shared MCP Gateway endpoint URL, used by every fleet-integration-capable service (global-only, no per-service override) | `""` |
| `global.fleet.mcpGatewayType` | Shared MCP Gateway type: `""`, `eaigw`, or `kuadrant`. Enum-validated — `helm template`/`install` fails at render time for any other value | `""` |
| `global.fleet.tlsCAFile` | Shared CA bundle for verifying the MCP Gateway's TLS cert (global-only, no per-service override) | `""` |
| `global.fleet.tokenSecretRef` | Secret (key `token`) with an ACM Search bearer token. **Mandatory when `backend: "acm"`** — GW/RO fail `FleetConfig.Validate()` at startup without it ([Issue #1556](https://github.com/jordigilh/kubernaut/issues/1556)) | `""` |
| `global.fleet.oauth2.enabled` | OAuth2 `client_credentials` auth on/off for MCP Gateway authentication (global-only, no per-service override) | `false` |
| `global.fleet.oauth2.tokenURL` | Shared MCP Gateway OAuth2 token URL (global-only, no per-service override) | `""` |
| `global.fleet.oauth2.credentialsSecretRef` | Shared MCP Gateway OAuth2 credentials Secret (keys: `client-id`, `client-secret`), used as the default when a service's own `fleet.oauth2.credentialsSecretRef` is unset | `""` |
| `global.fleet.oauth2.scopes` | Shared MCP Gateway OAuth2 scopes (global-only, no per-service override) | `[]` |
| `global.fleet.oauth2.tlsCAFile` | Shared CA bundle for verifying `tokenURL`'s TLS cert (global-only, no per-service override) | `""` |

Every fleet-integration-capable service (`gateway`, `signalprocessing`, `remediationorchestrator`, `effectivenessmonitor`, `apifrontend`, `fleetmetadatacache`) points at the same physical MCP Gateway instance (and, for `gateway`/`remediationorchestrator`, the same scope-check backend), so `global.fleet` is the **sole source of truth** for all of it ([Issue #1707](https://github.com/jordigilh/kubernaut/issues/1707) follow-up). The single per-service exception is `fleet.oauth2.credentialsSecretRef`, which each service can still override individually (see each service's own `fleet.oauth2` block, or `fleetmetadatacache.oauth2` for FMC) — every other field that used to be duplicated per service (`backend`, `endpoint`, `mcpGatewayEndpoint`, `mcpGatewayType`, `tlsCAFile`, `tokenSecretRef`, `oauth2.enabled`/`tokenURL`/`scopes`/`tlsCAFile`) was removed from every per-service schema entirely; setting it there now fails Helm schema validation at render time instead of being silently ignored. `global.fleet.enabled` is the single on/off switch for multi-cluster fleet federation across `gateway`, `remediationorchestrator`, `apifrontend`, and `effectivenessmonitor` — there is no per-service equivalent. `fleetmetadatacache.enabled` (whether to deploy that service at all) remains independent and is not controlled by this global.

`global.fleet.mcpGatewayEndpoint` is required when `global.fleet.enabled` / `fleetmetadatacache.enabled` is `true` for `gateway`, `remediationorchestrator`, and `fleetmetadatacache` — `helm install`/`upgrade` fails fast with a remediation message if it's unset. It's optional for `effectivenessmonitor` and `apifrontend`, where an empty value just means those services fall back to reading local-cluster-only state instead of federating through the MCP Gateway.

### LLM Profiles (DD-PLATFORM-007)

LLM provider configuration is defined once as a **named profile** under `global.llmProfiles`
and referenced by name from every consumer — mirroring the Kubernaut Operator's
`spec.llmProfiles`. This replaces the old `kubernautAgent.llm.*` literal block; there is no
backward-compat shim, since the chart is pre-GA.

At least one profile is required — `kubernautAgent.llmProfileRef` has no default and the
chart fails fast at render time if it's unset or names an undefined profile.

| Parameter | Description | Default |
|---|---|---|
| `global.llmProfiles.<name>.provider` | LLM provider: `openai`, `anthropic`, `vertex_ai`, or `openai_compatible` — **required** | none |
| `global.llmProfiles.<name>.model` | Model name (`gpt-4o`, `claude-sonnet-4-20250514`, `gemini-2.5-pro`, ...) | `""` |
| `global.llmProfiles.<name>.credentialsSecretName` | Secret with the LLM API key (key `api_key`) or, for `vertex_ai`, a service-account JSON key (key `credentials.json`) — **required** | none |
| `global.llmProfiles.<name>.endpoint` | Endpoint URL, functionally required for `openai`/`openai_compatible` on both KA and AF (e.g. `https://api.openai.com/v1` for real OpenAI, or a self-hosted/Azure (`azureApiVersion`) base URL) — neither client defaults it. API Frontend fails fast at startup if it's unset for these providers; Kubernaut Agent does not validate it upfront, but every LLM call fails at request time without it | `""` |
| `global.llmProfiles.<name>.temperature` | Sampling temperature | `0.7` |
| `global.llmProfiles.<name>.maxRetries` | Max retry attempts on transient LLM errors | `3` |
| `global.llmProfiles.<name>.timeoutSeconds` | Per-request timeout | `120` |
| `global.llmProfiles.<name>.vertexProject` | GCP project ID (`vertex_ai` only) | `""` |
| `global.llmProfiles.<name>.vertexLocation` | GCP region (`vertex_ai` only) | `""` |
| `global.llmProfiles.<name>.azureApiVersion` | Switches `openai`/`openai_compatible` into Azure OpenAI mode | `""` |
| `global.llmProfiles.<name>.tlsCaFile` | PEM CA cert path for internal LLM endpoints behind a private CA | `""` |
| `global.llmProfiles.<name>.oauth2.enabled` | Enable OAuth2 client-credentials auth for the LLM gateway | `false` |
| `global.llmProfiles.<name>.oauth2.tokenURL` | OAuth2 token endpoint URL | `""` |
| `global.llmProfiles.<name>.oauth2.credentialsSecretRef` | Secret with `client-id`/`client-secret` keys (mounted as files) | `""` |
| `global.llmProfiles.<name>.reasoning.enabled` | Request model reasoning/thinking output (BR-AI-086). Supported today on the Anthropic-family client (native + Vertex) | `false` |
| `global.llmProfiles.<name>.reasoning.budgetTokens` | Max tokens the model may spend on reasoning/thinking (Anthropic extended thinking budget). `0` lets the client choose a default. Anthropic-only; always wins over `effort` when set | `0` |
| `global.llmProfiles.<name>.reasoning.effort` | Unified, provider-agnostic reasoning-depth knob (#1604): `""` (vendor/provider default), `none`, `minimal`, `low`, `medium`, `high`, or `xhigh`. Supported on Anthropic, real OpenAI/gpt-5/o-series models, and DeepSeek | `""` |
| `global.llmProfiles.<name>.reasoning.capabilityOverride` | Override reasoning-capability auto-detection for `openai_compatible` self-hosted models: `""` (auto), `force_on`, or `force_off` | `""` |

### Kubernaut Agent (LLM)

| Parameter | Description | Default |
|---|---|---|
| `kubernautAgent.llmProfileRef` | Name of an entry in `global.llmProfiles` for KA's main investigation LLM — **required, no default** | none |
| `kubernautAgent.phaseModels.<phase>` | Per-phase LLM override, one of `rca`, `workflow_discovery`, `validation` → a `global.llmProfiles` name. Omitted phases use `llmProfileRef`'s profile unchanged | `{}` |
| `kubernautAgent.alignmentCheck.llmProfileRef` | Name of an entry in `global.llmProfiles` for the plan-alignment-check LLM call. Empty (default) inherits `llmProfileRef`'s resolved profile. Fixes a dead-field bug where the chart previously wrote an `apiKey` the Go binary never read (it expects `apiKeyFile`) | `""` |
| `kubernautAgent.prometheus.enabled` | Enable Prometheus toolset | `false` |
| `kubernautAgent.prometheus.url` | Prometheus/Thanos URL | `""` |
| `kubernautAgent.prometheus.tls.enabled` | Enable TLS CA trust for Prometheus connections | `false` |
| `kubernautAgent.prometheus.tls.caConfigMapName` | ConfigMap with CA PEM | `""` |
| `kubernautAgent.service.type` | Kubernetes Service type (`ClusterIP`, `NodePort`, `LoadBalancer`) | `ClusterIP` |
| `kubernautAgent.service.nodePort` | Explicit NodePort for the https (MCP/API) port when `type=NodePort`. `0` = Kubernetes auto-assigns; see `gateway.service.nodePort` for rationale | `0` |
| `kubernautAgent.interactive.maxConcurrentSessions` | Max concurrent MCP interactive sessions per agent replica. Issue #1737 gap found: raised from a too-low prior default of `5` -- confirmed via full-suite E2E to cause "max_sessions: Maximum concurrent sessions reached" from Ginkgo's own test parallelism alone | `50` |
| `kubernautAgent.interactive.rateLimitPerUser` | Max MCP requests per second per authenticated user | `20` |

All LLM configuration is part of the main `kubernaut-agent-config`/`kubernaut-agent-llm-runtime`
ConfigMaps. Credentials (API keys, `vertex_ai` service-account JSON, OAuth2 client secrets) are
always mounted from a Secret as files — never exposed as environment variables or inlined in a
ConfigMap. Distinct `credentialsSecretName`s across `phaseModels`/`alignmentCheck` each get their
own dedicated Secret mount; a phase/alignment-check profile that shares `llmProfileRef`'s own
`credentialsSecretName` reuses that existing mount instead of duplicating it.

### API Frontend (LLM)

API Frontend's own agent-loop LLM connection and severity-triage LLM tiers were previously
unreachable via Helm despite being fully implemented in Go — both are now wired.

| Parameter | Description | Default |
|---|---|---|
| `apifrontend.llmProfileRef` | Name of an entry in `global.llmProfiles` for AF's own `agent.llm` connection. Empty (default) falls back to `kubernautAgent.llmProfileRef`'s resolved profile | `""` |
| `apifrontend.config.severityTriage.llmProfileRef` | Name of an entry in `global.llmProfiles` for severity-triage's LLM fallback tier, independent of `apifrontend.llmProfileRef`. Empty (default) inherits AF's own resolved profile | `""` |
| `apifrontend.config.severityTriage.llmEnabled` | Whether LLM-based severity-triage tiers are active. `false` forces rule-based-only triage (no `llm` block rendered) | `true` |
| `apifrontend.config.severityTriage.maxQueriesPerCall` | Max Prometheus queries per triage call | `10` |
| `apifrontend.config.severityTriage.maxRulesEvaluated` | Max rule-based triage rules evaluated per call | `100` |
| `apifrontend.config.mcp.enabled` | Enables AF's own `/mcp` protocol endpoint for external MCP clients (distinct from AF-as-a-client-of-KA's-MCP-server, always active). Go's `Config.DefaultConfig()` defaults this `false`, so it must be explicitly enabled here; `false` returns 501 for every `/mcp` request | `true` |
| `apifrontend.config.mcp.sessionIdleTimeout` | Idle timeout for AF's `/mcp` sessions | `5m` |
| `apifrontend.config.interactive.enabled` | Enables session-dependent A2A/MCP tools (`kubernaut_investigate`, `discover_workflows`, `select_workflow`, `message`, `complete`, `cancel`, `status`, `reconnect`, `await_session`). Go's `Config.DefaultConfig()` already defaults this `true`; exposed here for override/documentation clarity | `true` |
| `apifrontend.config.interactive.awaitSessionTimeout` | Timeout for the `kubernaut_await_session` tool | `10s` |
| `apifrontend.config.interactive.bridgeInactivityTimeout` | Inactivity timeout for the KA session bridge | `15s` |
| `apifrontend.config.rateLimit.ipRequestsPerSec` | Per-IP request rate limit | `10000` |
| `apifrontend.config.rateLimit.userRequestsPerSec` | Per-user request rate limit | `100` |
| `apifrontend.config.rateLimit.maxConcurrentSessions` | Max concurrent MCP/interactive sessions per user. Go's package-level fallback (used whenever this is `0`) is only `3` -- confirmed via E2E to cause "Maximum concurrent sessions reached" under realistic concurrent load | `50` |
| `apifrontend.config.rateLimit.toolCallsPerMinute` | Max tool calls per minute per user. Go's package-level fallback (used whenever this is `0`) is only `60` | `600` |
| `apifrontend.service.type` | Kubernetes Service type (`ClusterIP`/`NodePort`/`LoadBalancer`) | `ClusterIP` |
| `apifrontend.service.nodePort` | Explicit NodePort for the https port when `type=NodePort`. `0` = Kubernetes auto-assigns; see `gateway.service.nodePort` for rationale | `0` |
| `apifrontend.ingress.enabled` | Create a `networking.k8s.io/v1` Ingress for external access (BR-PLATFORM-009, Kubernaut Operator `APIFrontendRoute` parity). Opt-in: AF is a machine-facing pipeline entry point, so external exposure is a deliberate choice | `false` |
| `apifrontend.ingress.className` | `spec.ingressClassName` | `""` |
| `apifrontend.ingress.host` | External hostname. Optional -- unlike Console, no internal component depends on it; empty renders a catch-all rule | `""` |
| `apifrontend.ingress.annotations` | Extra Ingress annotations. **Required in practice**: AF serves HTTPS internally with a self-signed cert, so a controller-specific backend-protocol/TLS-passthrough annotation is needed (e.g. ingress-nginx: `nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"` + `nginx.ingress.kubernetes.io/proxy-ssl-verify: "off"`) -- the chart does not hardcode one controller's syntax | `{}` |
| `apifrontend.ingress.tls.secretName` | Pre-created Secret with a TLS cert for `host`; omit for a controller with its own default cert | `""` |

`vertex_ai` authenticates via ambient `GOOGLE_APPLICATION_CREDENTIALS` (set automatically on the
Deployment when a resolved profile is `vertex_ai`), so AF's own connection and severity-triage's
cannot both be `vertex_ai` while pointing at *different* Secrets — there's no way to make two
different credential files visible to the SDK's ADC lookup at the same time. The chart fails fast
at render time (`kubernaut#1731`) if this combination is configured; use the same
`credentialsSecretName` for both, or a non-`vertex_ai` provider for one of them.

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
| `workflowexecution.config.ansible.caCertSecretRef.name` | Secret with a custom/private CA cert for a self-signed AWX/AAP endpoint (BR-PLATFORM-005) | `""` |
| `workflowexecution.config.ansible.caCertSecretRef.key` | Key within the Secret (PEM) | `ca.crt` |
| `workflowexecution.fleet.oauth2.credentialsSecretRef` | K8s Secret (keys: `client-id`, `client-secret`) for a **write-scoped** OAuth2 client. **REQUIRED when `global.fleet.oauth2.enabled=true`** — `helm template`/`install` fails at render time if unset. Does **NOT** fall back to `global.fleet.oauth2.credentialsSecretRef`: WE is the only fleet-integration-capable service that calls MCP write tools (`resources_create_or_update`/`resources_delete`) instead of the read-only tools every other service (`gateway`/`remediationorchestrator`/`apifrontend`/`effectivenessmonitor`/`signalprocessing`/`fleetmetadatacache`) uses, so sharing their credential here would be a least-privilege violation | `""` |

WE's remote-execution `fleet.endpoint` and `fleet.oauth2.{enabled,tokenURL,scopes,tlsCAFile}` come
entirely from [`global.fleet.*`](#global) — there is no per-service override for them, since WE has
no `ClusterRegistry`/CRD-watch capability (no `mcpGatewayType`, no `namespace`; it discovers MCP tool
prefixes dynamically via `tools/list`). Unlike GW/RO/FMC, an empty `global.fleet.mcpGatewayEndpoint`
is a valid, supported state even with `global.fleet.enabled=true` — WE simply stays in local-only
execution (BR-FLEET-054).

Setting `caCertSecretRef` adds a `build-ca-bundle` init container that combines the custom CA with
the inter-service CA into one trust bundle (`TLS_CA_FILE`), mirroring the Kubernaut Operator. When
`workflowexecution.config.ansible` is configured (`apiURL` set), the WorkflowExecution
NetworkPolicy also allows HTTPS (443) egress, since the AWX/AAP endpoint may be outside the
cluster's other known peers.

### Gateway

| Parameter | Description | Default |
|---|---|---|
| `gateway.auth.signalSources` | External signal sources needing RBAC | `[]` |
| `gateway.service.type` | Kubernetes Service type (`ClusterIP`/`NodePort`/`LoadBalancer`) | `ClusterIP` |
| `gateway.service.nodePort` | Explicit NodePort for the http port when `type=NodePort`. `0` = Kubernetes auto-assigns from the 30000-32767 range. Useful for CI/test environments needing a deterministic, host-accessible port (e.g. Kind's `extraPortMappings`, which must know the exact port before cluster creation) or on-prem installs with fixed firewall/LB rules | `0` |
| `gateway.ingress.enabled` | Create a `networking.k8s.io/v1` Ingress for external access (BR-PLATFORM-009, Kubernaut Operator `GatewayRoute` parity). Opt-in: Gateway is a machine-facing pipeline entry point, so external exposure is a deliberate choice | `false` |
| `gateway.ingress.className` | `spec.ingressClassName` | `""` |
| `gateway.ingress.host` | External hostname. Optional -- unlike Console, no internal component depends on it; empty renders a catch-all rule | `""` |
| `gateway.ingress.annotations` | Extra Ingress annotations | `{}` |
| `gateway.ingress.tls.secretName` | Pre-created Secret with a TLS cert for `host`; omit for HTTP-only or a controller with its own default cert | `""` |
| `gateway.fleet.oauth2.credentialsSecretRef` | K8s Secret (keys: `client-id`, `client-secret`) overriding `global.fleet.oauth2.credentialsSecretRef` for Gateway only. All other fleet fields (`backend`, `endpoint`, `tokenSecretRef`, `mcpGatewayEndpoint`, `mcpGatewayType`, `tlsCAFile`, `oauth2.enabled`/`tokenURL`/`scopes`/`tlsCAFile`) moved to [`global.fleet.*`](#global) ([Issue #1707](https://github.com/jordigilh/kubernaut/issues/1707) follow-up) | `""` |

### RemediationOrchestrator

| Parameter | Description | Default |
|---|---|---|
| `remediationorchestrator.fleet.oauth2.credentialsSecretRef` | K8s Secret (keys: `client-id`, `client-secret`) overriding `global.fleet.oauth2.credentialsSecretRef` for RemediationOrchestrator only. All other fleet fields (`backend`, `endpoint`, `tokenSecretRef`, `mcpGatewayEndpoint`, `mcpGatewayType`, `tlsCAFile`, `oauth2.enabled`/`tokenURL`/`scopes`/`tlsCAFile`) moved to [`global.fleet.*`](#global) ([Issue #1707](https://github.com/jordigilh/kubernaut/issues/1707) follow-up) | `""` |

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
| `datastorage.service.type` | Kubernetes Service type (`ClusterIP`/`NodePort`/`LoadBalancer`) | `ClusterIP` |
| `datastorage.service.nodePort` | Explicit NodePort for the http port when `type=NodePort`. `0` = Kubernetes auto-assigns; see `gateway.service.nodePort` for rationale | `0` |
| `valkey.enabled` | Deploy in-chart Valkey | `true` |
| `valkey.existingSecret` | Pre-created Secret name (empty = expect `valkey-secret`) | `""` |
| `valkey.host` | External host (when `enabled=false`) | `""` |
| `datastorage.config.auditHashKey.existingSecret` | Pre-created HMAC key Secret name (empty = chart auto-generates one, GAP-05, mandatory); see [Keyed Audit Hash Chain](#keyed-audit-hash-chain-gap-05) | `""` |
| `apifrontend.config.auth.replayCache.redisDB` | Valkey logical DB index used for the (mandatory, GAP-08) distributed JWT replay cache; see [Distributed JWT Replay Cache](#distributed-jwt-replay-cache-gap-08) | `1` |
| `apifrontend.config.auth.replayCache.tls.caFile` | CA bundle for verifying Valkey's TLS cert (mandatory) | `/etc/tls-ca/ca.crt` |
| `datastorage.config.server.rateLimit.requestsPerSecond` | Sustained per-IP requests/second (mandatory, GAP-09); see [Data Storage Per-IP Rate Limiting](#data-storage-per-ip-rate-limiting-gap-09) | `50` |
| `datastorage.config.server.rateLimit.burst` | Per-IP token bucket burst size | `100` |

### Console

| Parameter | Description | Default |
|---|---|---|
| `console.enabled` | Deploy the standalone web console (BR-PLATFORM-006); see [Optional: Web Console](#optional-web-console-br-platform-006) | `false` |
| `console.replicas` | Replica count | `1` |
| `console.auth.secretName` | Secret with keys: `client-id`, `client-secret`, `cookie-secret`. **Required** when `console.enabled=true` | `""` |
| `console.oauth2Proxy.image` | Third-party oauth2-proxy sidecar image | `quay.io/oauth2-proxy/oauth2-proxy:v7.15.3` |
| `console.ingress.enabled` | Create a `networking.k8s.io/v1` Ingress for browser access (BR-PLATFORM-009). Opt-in: Console is optional UI tooling in front of APIFrontend that users may replace or front with their own Ingress/Route -- a deliberate deviation from the Operator's `ConsoleRouteSpec`, which defaults to opt-out | `false` |
| `console.ingress.className` | `spec.ingressClassName` | `""` |
| `console.ingress.host` | Browser-facing hostname. **Required** when `console.enabled=true` (even if `ingress.enabled=false`) | `""` |
| `console.ingress.tls.secretName` | Pre-created Secret with a TLS cert for `host`; omit for HTTP-only or a controller with its own default cert | `""` |
| `console.pdb.{enabled,minAvailable,maxUnavailable}` | PodDisruptionBudget | `enabled: false` |

### TLS

| Parameter | Description | Default |
|---|---|---|
| `tls.mode` | `hook` (self-signed), `cert-manager` (production), or `manual` | `hook` |
| `tls.certManager.issuerRef.name` | Issuer/ClusterIssuer name. When mode=cert-manager and left empty, auto-selected via `lookup` if exactly one exists in the cluster (real `helm install`/`upgrade` only); required if rendering via `helm template`/GitOps or if multiple issuers exist | `""` |

> **`datastorage-signing-cert` prerequisite** (#334): DataStorage signs audit exports with an
> RSA 2048 key for tamper-evidence (AU-9) and **fails to start with no fallback** if this key is
> missing. In `tls.mode=cert-manager`, the chart auto-provisions it via a `Certificate` resource
> (same as `authwebhook-tls`) — no action needed. In `tls.mode=manual`, you must create it
> yourself before installing:
>
> ```bash
> openssl genrsa -out /tmp/signing-tls.key 2048
> openssl req -new -x509 -days 365 -key /tmp/signing-tls.key \
>   -out /tmp/signing-tls.crt -subj "/CN=datastorage-signing-cert"
> kubectl create secret tls datastorage-signing-cert \
>   --cert=/tmp/signing-tls.crt --key=/tmp/signing-tls.key -n kubernaut-system
> ```

> **Inter-service mTLS prerequisite** (DD-PLATFORM-001): ~11 services encrypt
> pod-to-pod HTTP traffic with mTLS, trusting a shared `inter-service-ca`
> ConfigMap and presenting `gateway-tls`/`datastorage-tls`/`kubernautagent-tls`
> certs. In `tls.mode=cert-manager`, the chart auto-provisions all of this via
> a **dedicated internal CA** — deliberately decoupled from
> `tls.certManager.issuerRef`, so internal service-to-service trust never
> depends on whatever external/public PKI issues your webhook certs — no
> action needed. In `tls.mode=manual`, you must create these yourself:
>
> ```bash
> # 1. Generate a root CA (ECDSA P-256, matching hook-mode's algorithm)
> openssl ecparam -genkey -name prime256v1 -noout -out /tmp/ca.key
> openssl req -new -x509 -days 3650 -key /tmp/ca.key -out /tmp/ca.crt -subj "/CN=kubernaut-interservice-ca"
> kubectl create configmap inter-service-ca --from-file=ca.crt=/tmp/ca.crt -n kubernaut-system
>
> # 2. Issue a leaf cert per service (repeat for gateway-service, data-storage-service, kubernaut-agent)
> for SVC in gateway-service data-storage-service kubernaut-agent; do
>   openssl ecparam -genkey -name prime256v1 -noout -out /tmp/${SVC}.key
>   openssl req -new -key /tmp/${SVC}.key -out /tmp/${SVC}.csr -subj "/CN=${SVC}.kubernaut-system.svc" \
>     -addext "subjectAltName=DNS:${SVC},DNS:${SVC}.kubernaut-system,DNS:${SVC}.kubernaut-system.svc,DNS:${SVC}.kubernaut-system.svc.cluster.local"
>   openssl x509 -req -in /tmp/${SVC}.csr -CA /tmp/ca.crt -CAkey /tmp/ca.key -CAcreateserial -out /tmp/${SVC}.crt -days 365
> done
> kubectl create secret tls gateway-tls --cert=/tmp/gateway-service.crt --key=/tmp/gateway-service.key -n kubernaut-system
> kubectl create secret tls datastorage-tls --cert=/tmp/data-storage-service.crt --key=/tmp/data-storage-service.key -n kubernaut-system
> kubectl create secret tls kubernautagent-tls --cert=/tmp/kubernaut-agent.crt --key=/tmp/kubernaut-agent.key -n kubernaut-system
> ```
>
> Without these, `pkg/shared/tls.ConfigureConditionalTLS`/`DefaultBaseTransport`
> silently fall back to plain HTTP for inter-service traffic (no error, but an
> SC-8 compliance gap) — the mounts are `optional: true` by design so the
> chart degrades gracefully rather than crash-looping.

### NetworkPolicies

NetworkPolicies are unconditionally mandatory for every service (DD-PLATFORM-006 Decision Area
3) -- there is no `networkPolicies.enabled` or per-service `networkPolicies.<service>.enabled`
toggle; `additionalProperties: false` in the schema rejects both if set. The 4 policies for
optional services (apifrontend/console/postgresql/valkey) are still a no-op when that service
itself is disabled -- gate on that service's own `enabled` field, not a NetworkPolicy-specific one.

| Parameter | Description | Default |
|---|---|---|
| `networkPolicies.apiServerCIDR` | K8s API server real backend endpoint CIDR (e.g., `10.89.0.2/32` -- NOT the `kubernetes` Service ClusterIP). Usually left empty: auto-discovered via `lookup` on a real `helm install`/`upgrade`; required if rendering via `helm template`/GitOps | `""` |
| `networkPolicies.apiServerCIDRs` | Additional API server backend endpoint CIDRs for HA (multiple control-plane nodes). Merged with `apiServerCIDR`; usually left empty (see above) | `[]` |
| `networkPolicies.apiServerPort` | API server backend endpoint port (commonly 6443). `0` = auto-discover alongside the CIDR | `0` |
| `networkPolicies.monitoring.namespace` | Namespace for Prometheus metrics scraping ingress | `""` |
| `networkPolicies.monitoring.prometheusPort` | Prometheus port to allow in the NetworkPolicy egress rule | `9090` |
| `networkPolicies.monitoring.alertManagerPort` | AlertManager port to allow in the NetworkPolicy egress rule | `9093` |
| `networkPolicies.externalWebhooks.cidr` | CIDR for Slack/PagerDuty/Teams webhook egress | `0.0.0.0/0` |
| `networkPolicies.externalRegistry.cidr` | CIDR for OCI registry egress (datastorage bundle validation) | `0.0.0.0/0` |
| `networkPolicies.apifrontend.ingressNamespaces` | External namespaces (e.g. an ingress-controller namespace) allowed to reach APIFrontend's https port. Same-namespace traffic is always allowed. No-op unless `apifrontend.enabled=true` | `[]` |
| `networkPolicies.console.ingressNamespaces` | External namespaces (e.g. an ingress-controller namespace) allowed to reach the console's oauth2-proxy port. Same-namespace traffic is always allowed. No-op unless `console.enabled=true` | `[]` |

Each service gets a NetworkPolicy with:
- **Default-deny ingress** with service-specific allow rules
- **Egress**: most services restrict egress to DNS, K8s API, and known peers; **Kubernaut Agent uses an ingress-only policy** (unrestricted egress) because it must reach arbitrary LLM providers, MCP servers, and tool endpoints
- **Datastorage**: allows egress to PostgreSQL, Valkey, and external container registries (configurable CIDR for OCI bundle validation)
- **APIFrontend** (BR-PLATFORM-005, Kubernaut Operator parity — previously the only mesh component without a NetworkPolicy): ingress from same-namespace callers plus `networkPolicies.apifrontend.ingressNamespaces`; egress to DataStorage, Valkey (only when `apifrontend.config.auth.replayCache.enabled=true`), and OIDC/JWKS discovery (only when OIDC is configured)
- **Console** (BR-PLATFORM-006; a no-op unless `console.enabled=true`): ingress from same-namespace callers plus `networkPolicies.console.ingressNamespaces`; egress to APIFrontend and OIDC token/JWKS discovery

Example (API server CIDR/port are auto-discovered here; only set them explicitly for `helm template`/GitOps rendering, see [NetworkPolicy API Server Discovery](#networkpolicy-api-server-discovery)):

```bash
helm install kubernaut charts/kubernaut \
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

**APIFrontend** and **FleetMetadataCache** also accept the full set above (`podSecurityContext`,
`containerSecurityContext`, `affinity`, `topologySpreadConstraints`, `pdb`, `nodeSelector`,
`tolerations`) — previously these two services were missing wiring for the pod-hardening and
scheduling helpers that every other service already used, even though the schema and values
already implied they were supported.

#### Default anti-affinity and PDB (DD-PLATFORM-004)

Every service that renders a Deployment gets a default **soft** (preferred, not required)
pod anti-affinity spreading its replicas across nodes, and `pdb.enabled` defaults to
`true` (`maxUnavailable: 1`) for every service in `templates/pdb.yaml` — matching the
Kubernaut Operator's defaults. Both are safe with the chart's default `replicas: 1`: a
preferred anti-affinity term is a no-op with no peer replica to avoid, and
`maxUnavailable: 1` (never `minAvailable`) always permits the sole pod to be voluntarily
evicted (e.g. `kubectl drain`) rather than blocking it. Override per-service via
`<service>.affinity` (deep-merged with the default: a sibling key like `nodeAffinity`
merges in alongside the default `podAntiAffinity`; to fully replace the default
anti-affinity term itself, provide your own **non-empty**
`podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution` list — an *empty*
list or `null` is treated as unset and the default silently wins back, see
DD-PLATFORM-004) and `<service>.pdb.enabled=false` to opt out of the PDB entirely.
`console`'s and `fleetmetadatacache`'s PDBs still only render when the service itself
is enabled.

### Kubernaut Agent Security Hardening (BR-PLATFORM-005)

KubernautAgent is the highest-risk component in the mesh — it is LLM-driven and carries the
broadest investigative RBAC of any Kubernaut service. Two hardening measures, ported from the
Kubernaut Operator, apply unconditionally (not configurable, no `values.yaml` toggle):

- **Short-TTL ServiceAccount token**: the `kubernaut-agent-sa` ServiceAccount disables the default
  automounted token (`automountServiceAccountToken: false`); the Deployment instead mounts a
  1-hour, audience-scoped (`https://kubernetes.default.svc`) projected token, refreshed
  automatically by the kubelet.
- **KubeVirt / scheduling investigative RBAC**: the `kubernaut-agent-investigator` ClusterRole
  includes read-only (`get`/`list`/`watch`) access to `kubevirt.io` VirtualMachines/VMIs/
  migrations, `cdi.kubevirt.io` DataVolumes, and `scheduling.k8s.io` PriorityClasses, so
  investigations on clusters running VM-backed workloads or priority-based scheduling have the
  same visibility as an Operator-managed deployment.

## Disconnected / Air-Gapped Install

For airgapped environments, mirror container images and override the registry. Rego policies must still be provided via `--set-file`:

```bash
# Nested registry (Harbor, Artifactory)
helm install kubernaut oci://harbor.corp/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --set global.image.registry=harbor.corp \
  --set global.llmProfiles.primary.provider=openai \
  --set global.llmProfiles.primary.model=gpt-4o \
  --set global.llmProfiles.primary.endpoint=https://api.openai.com/v1 \
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials \
  --set kubernautAgent.llmProfileRef=primary \
  --set-file signalprocessing.policies.content=path/to/policy.rego \
  --set-file aianalysis.policies.content=path/to/approval.rego

# Flat registry (quay.io, Docker Hub)
helm install kubernaut oci://quay.io/myorg/charts/kubernaut \
  --namespace kubernaut-system \
  --set global.image.registry=quay.io/myorg \
  --set global.image.separator=- \
  --set global.llmProfiles.primary.provider=openai \
  --set global.llmProfiles.primary.model=gpt-4o \
  --set global.llmProfiles.primary.endpoint=https://api.openai.com/v1 \
  --set global.llmProfiles.primary.credentialsSecretName=llm-credentials \
  --set kubernautAgent.llmProfileRef=primary \
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

- **Single installation per cluster**: Cluster-scoped resources use static names. This is an
  enforced constraint, not just a naming convention — a second `helm install` on a cluster that
  already has a Kubernaut release fails fast with a Kubernaut-specific error naming the existing
  release/namespace and the `helm uninstall` remediation step (BR-PLATFORM-004,
  [DD-018](../../docs/architecture/decisions/DD-018-helm-chart-single-install-per-cluster-guard.md)).
  `helm upgrade` of the same release is unaffected. The guard only runs during `helm
  install`/`upgrade` (requires live cluster access via `lookup`) — it is a no-op under `helm
  template`/`helm lint --strict`.
- **`helm template` and auto-generated credentials**: `lookup` returns nil during `helm template`, so random passwords are generated on each dry-run. Use `helm install` directly or provide `existingSecret` for reproducible output.

## Documentation

- [Installation Guide](https://jordigilh.github.io/kubernaut-docs/getting-started/installation/)
- [Configuration Reference](https://jordigilh.github.io/kubernaut-docs/user-guide/configuration/)
- [Security & RBAC](https://jordigilh.github.io/kubernaut-docs/architecture/security-rbac/)
- [Architecture](https://jordigilh.github.io/kubernaut-docs/architecture/overview/)

## License

See [LICENSE](https://github.com/jordigilh/kubernaut/blob/main/LICENSE) in the project repository.
