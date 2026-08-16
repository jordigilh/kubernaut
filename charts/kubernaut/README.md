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
  --set monitoring.prometheus.enabled=true \
  --set monitoring.prometheus.url=http://kube-prometheus-stack-prometheus.monitoring.svc:9090 \
  --set monitoring.alertManager.enabled=true \
  --set monitoring.alertManager.url=http://kube-prometheus-stack-alertmanager.monitoring.svc:9093 \
  --set gateway.auth.signalSources[0].name=alertmanager \
  --set gateway.auth.signalSources[0].serviceAccount=alertmanager-kube-prometheus-stack-alertmanager \
  --set gateway.auth.signalSources[0].namespace=monitoring
```

Configure AlertManager to send webhooks to the Gateway (requires `gateway.enabled=true`, the
default — see [Enable/Disable Gateway or APIFrontend](#enable-gateway-or-apifrontend) if you've
disabled it):

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

### Optional: Distributed JWT Replay Cache (GAP-08)

APIFrontend detects replayed JWTs via their `jti` claim. By default this uses
an in-memory cache that is per-process: in a multi-replica deployment, a token
replayed against a *different* replica than the one that first observed it is
not detected.

**Left opt-in, disabled by default — reverted from a brief mandatory-by-default
window** ([DD-PLATFORM-006](../../docs/architecture/decisions/DD-PLATFORM-006-helm-chart-configuration-surface-reduction.md)
DA6 addendum): unlike the audit-hash-chain and rate-limiting toggles above,
this control is **jti-uniqueness-based**, not replica-count-aware — it rejects
*any* second presentation of the same token within the cache TTL (10 minutes),
which is exactly how a legitimate client is expected to use an OAuth2 Bearer
token (fetch once, reuse for many requests until it expires). Making it
mandatory broke normal multi-call sessions for every client, not just a
multi-replica edge case. See [BR-SECURITY-1505](../../docs/requirements/BR-SECURITY-1505-distributed-jwt-replay-cache.md)
for the full design; enable it only if your deployment's clients mint a fresh
token per request (e.g. a token-exchange flow), not a reused session token.

Enabling `apifrontend.config.auth.replayCache` shares replay state across all
APIFrontend replicas via the cluster's Valkey instance (the same instance and
Secret already used by DataStorage) — closing the HA gap a per-process
in-memory cache would have. If Valkey is unreachable at runtime, APIFrontend
falls back to the in-memory cache and logs the degradation rather than
disabling replay protection outright.

```bash
helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system \
  --reuse-values \
  --set apifrontend.config.auth.replayCache.enabled=true
```

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
- `gateway.enabled` and `apifrontend.enabled` (both default `true`, Issue #2162) are fully
  independent — Gateway (webhook-driven signal ingestion, e.g. AlertManager) and APIFrontend
  (natural-language-driven investigation) are complementary, not redundant, entry points into
  the same `RemediationRequest` pipeline. Disable either one on its own to run with only the
  ingress point(s) you actually use, or leave both enabled to support both. `console.enabled`
  continues to require `apifrontend.enabled` specifically (Console's UI proxies to APIFrontend
  only) — it has no dependency on `gateway.enabled`.
- The chart fails fast at `helm template`/`helm install` time (before any resources are
  applied) if `console.enabled=true` and `console.auth.secretName`, a resolvable OIDC issuer,
  or `console.ingress.host` is missing.

### Enable/Disable Gateway or APIFrontend (Issue #2162)

Gateway and APIFrontend are Kubernaut's two independent ingress points — both create
`RemediationRequest` CRDs, but from different sources: Gateway ingests webhook-driven signals
(e.g. AlertManager), while APIFrontend serves natural-language-driven investigations (and is
required by Console, see above). Both default to `enabled: true`; disable whichever one you
don't use to remove its Deployment/Service/RBAC/NetworkPolicy footprint entirely:

```bash
# Gateway-only deployment (no natural-language API/Console)
helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system --reuse-values \
  --set apifrontend.enabled=false

# APIFrontend-only deployment (no AlertManager/webhook ingestion)
helm upgrade kubernaut oci://quay.io/kubernaut-ai/charts/kubernaut \
  --namespace kubernaut-system --reuse-values \
  --set gateway.enabled=false
```

`console.enabled=true` still requires `apifrontend.enabled=true` (see above) — disabling
Gateway has no effect on Console's availability.

### OpenShift (OCP)

This chart targets non-OpenShift (vanilla Kubernetes) deployments. For OpenShift, use the
[Kubernaut Operator](https://jordigilh.github.io/kubernaut-docs/operations/operator/)
instead, which provides native OCP integration (service-ca TLS, OLM catalog, SCC
management, automated upgrades). See the
[Helm-to-Operator Migration Guide](https://jordigilh.github.io/kubernaut-docs/operations/helm-to-operator/).

## Configuration Reference

All values are validated against `values.schema.json`. Run `helm lint` to check your overrides.

**Full field-by-field reference**: every parameter's type, description, default, and
required/optional state is auto-generated straight from `values.schema.json` into
[`docs/generated/helm-values-reference.md`](../../docs/generated/helm-values-reference.md)
(`make generate-helm-config-docs`, CI-enforced freshness) — one table per service, always
in sync with what Helm actually validates. This section covers only the narrative parts
(architecture, cross-field dependencies, worked examples) that a flat field table can't
convey.

### Global / Fleet Federation

Every fleet-integration-capable service (`gateway`, `signalprocessing`, `remediationorchestrator`, `effectivenessmonitor`, `apifrontend`, `fleetmetadatacache`) points at the same physical MCP Gateway instance (and, for `gateway`/`remediationorchestrator`, the same scope-check backend), so `global.fleet` is the **sole source of truth** for all of it ([Issue #1707](https://github.com/jordigilh/kubernaut/issues/1707) follow-up). The single per-service exception is `fleet.oauth2.credentialsSecretRef`, which each service can still override individually (see each service's own `fleet.oauth2` block, or `fleetmetadatacache.oauth2` for FMC) — every other field that used to be duplicated per service (`backend`, `endpoint`, `mcpGatewayEndpoint`, `mcpGatewayType`, `tlsCAFile`, `tokenSecretRef`, `oauth2.enabled`/`tokenURL`/`scopes`/`tlsCAFile`) was removed from every per-service schema entirely; setting it there now fails Helm schema validation at render time instead of being silently ignored. `global.fleet.enabled` is the single on/off switch for multi-cluster fleet federation across `gateway`, `remediationorchestrator`, `apifrontend`, and `effectivenessmonitor` — there is no per-service equivalent. `fleetmetadatacache.enabled` (whether to deploy that service at all) remains independent and is not controlled by this global.

`global.fleet.mcpGatewayEndpoint` is required when `global.fleet.enabled` / `fleetmetadatacache.enabled` is `true` for `gateway`, `remediationorchestrator`, and `fleetmetadatacache` — `helm install`/`upgrade` fails fast with a remediation message if it's unset. It's optional for `effectivenessmonitor` and `apifrontend`, where an empty value just means those services fall back to reading local-cluster-only state instead of federating through the MCP Gateway.

**Worked example** (enabling Fleet with the FleetMetadataCache backend and OAuth2 auth; see
`docs/generated/helm-values-reference.md#global` for the complete field list including
`backend: "acm"`'s additional requirements):

```bash
helm install kubernaut charts/kubernaut \
  --set global.fleet.enabled=true \
  --set global.fleet.mcpGatewayEndpoint=http://envoy-ai-gateway.kubernaut-system.svc:8080/mcp \
  --set global.fleet.oauth2.enabled=true \
  --set global.fleet.oauth2.tokenURL=https://dex.example.com/token \
  --set global.fleet.oauth2.credentialsSecretRef=fleet-oauth2-creds \
  --set fleetmetadatacache.enabled=true
```

### LLM Profiles (DD-PLATFORM-007)

LLM provider configuration is defined once as a **named profile** under `global.llmProfiles`
and referenced by name from every consumer — mirroring the Kubernaut Operator's
`spec.llmProfiles`. This replaces the old `kubernautAgent.llm.*` literal block; there is no
backward-compat shim, since the chart is pre-GA.

At least one profile is required. If `kubernautAgent.llmProfileRef` is left unset and
`global.llmProfiles` defines exactly one profile, its name is inferred automatically
(Issue #1987) — this is why this README's examples, `quickstart.sh`, and `helm-smoke-test.sh`
can all use a single profile named `"primary"` without ever setting `llmProfileRef` explicitly.
If `global.llmProfiles` defines zero or 2+ profiles, `kubernautAgent.llmProfileRef` must be
set explicitly, and the chart fails fast at render time either naming the missing/ambiguous
profiles or the undefined referenced profile. See `docs/generated/helm-values-reference.md#global`
for the full `llmProfiles.<name>.*` field list (provider, model, credentials, reasoning knobs, etc.).

`global.llmProfiles.<name>.provider` accepts `openai`, `anthropic`, `gemini`, `vertex_ai`, or
`openai_compatible`. `vertex_ai` hosts either Claude or Gemini models depending on `model` —
both KA and AF auto-detect which client to construct from the model name prefix (`claude-*`
vs `gemini-*`, #1778, #1792). Both Anthropic and Kubernaut Agent's Gemini-family client
(BR-AI-087) have no `reasoning.effort` tier above `high`, so `xhigh` is accepted but clamped
down to `high` on those two providers (not an error).

### Kubernaut Agent (LLM)

All LLM configuration is part of the main `kubernaut-agent-config`/`kubernaut-agent-llm-runtime`
ConfigMaps. Credentials (API keys, `vertex_ai` service-account JSON, OAuth2 client secrets) are
always mounted from a Secret as files — never exposed as environment variables or inlined in a
ConfigMap. Distinct `credentialsSecretName`s across `phaseModels`/`alignmentCheck` each get their
own dedicated Secret mount; a phase/alignment-check profile that shares `llmProfileRef`'s own
`credentialsSecretName` reuses that existing mount instead of duplicating it. See
`docs/generated/helm-values-reference.md#kubernautagent` for the full field list
(`llmProfileRef`, `phaseModels.<phase>`, `alignmentCheck.llmProfileRef`, Prometheus toolset,
service/interactive settings).

### API Frontend (LLM)

API Frontend's own agent-loop LLM connection and severity-triage LLM tiers were previously
unreachable via Helm despite being fully implemented in Go — both are now wired. See
`docs/generated/helm-values-reference.md#apifrontend` for the full field list
(`llmProfileRef`, `config.severityTriage.*`, `config.mcp.*`, `config.interactive.*`,
`config.rateLimit.*`, service/ingress settings).

`vertex_ai` for Claude models (and AF's severity-triage Gemini-on-Vertex path) authenticates via
ambient `GOOGLE_APPLICATION_CREDENTIALS` — as of `kubernaut#1801`, `cmd/apifrontend` injects this
env var in-process at startup (`pkg/apifrontend/launcher.InjectAmbientGoogleCredentials`), reading
the same mounted `apiKeyFile` path used by every other provider, rather than declaring it
statically on the Deployment. It remains a single process-wide variable either way, so AF's own
connection and severity-triage's still cannot both be `vertex_ai` while pointing at *different*
Secrets — there's no way to make two different credential files visible to the SDK's ADC lookup at
the same time. The chart fails fast at render time (`kubernaut#1731`) if this combination is
configured; use the same `credentialsSecretName` for both, or a non-`vertex_ai` provider for one of
them. AF's main-agent Gemini-on-Vertex path is unaffected by this constraint — it authenticates
with the `apiKeyFile`'s content passed as explicit credential bytes, never touching the ambient env
var at all (matching Kubernaut Agent's Gemini client).

### SignalProcessing / AIAnalysis / Notification

See `docs/generated/helm-values-reference.md#signalprocessing`,
`docs/generated/helm-values-reference.md#aianalysis`, and
`docs/generated/helm-values-reference.md#notification` for the full field lists (Rego policy
sourcing, proactive signal mappings, Slack/routing/credentials).

### WorkflowExecution

See `docs/generated/helm-values-reference.md#workflowexecution` for the full field list
(`config.execution.*`, `config.tekton.enabled`, `config.ansible.*`,
`fleet.oauth2.credentialsSecretRef`).

WE's remote-execution `fleet.endpoint` and `fleet.oauth2.{enabled,tokenURL,scopes,tlsCAFile}` come
entirely from [`global.fleet.*`](#global--fleet-federation) — there is no per-service override for them, since WE has
no `ClusterRegistry`/CRD-watch capability (no `mcpGatewayType`, no `namespace`; it discovers MCP tool
prefixes dynamically via `tools/list`). Unlike GW/RO/FMC, an empty `global.fleet.mcpGatewayEndpoint`
is a valid, supported state even with `global.fleet.enabled=true` — WE simply stays in local-only
execution (BR-FLEET-054).

Setting `caCertSecretRef` adds a `build-ca-bundle` init container that combines the custom CA with
the inter-service CA into one trust bundle (`TLS_CA_FILE`), mirroring the Kubernaut Operator. When
`workflowexecution.config.ansible` is configured (`apiURL` set), the WorkflowExecution
NetworkPolicy also allows HTTPS (443) egress, since the AWX/AAP endpoint may be outside the
cluster's other known peers.

### Gateway / RemediationOrchestrator / EffectivenessMonitor

See `docs/generated/helm-values-reference.md#gateway`,
`docs/generated/helm-values-reference.md#remediationorchestrator`, and
`docs/generated/helm-values-reference.md#effectivenessmonitor` for the full field lists
(ingress, service type/nodePort, per-service `fleet.oauth2.credentialsSecretRef` override,
external Prometheus/AlertManager for EM).

#### Extending owner-chain-resolution RBAC (Issue #1069, DD-GATEWAY-018)

Gateway and EffectivenessMonitor bind to Kubernetes' built-in `view` ClusterRole so
owner-chain resolution (walking owner references up to the controlling resource) works
for `PodDisruptionBudget` and any ecosystem CRD whose operator publishes
`rbac.authorization.k8s.io/aggregate-to-view: "true"` labels (cert-manager, Istio,
KubeVirt, and other "well-behaved" operators are covered automatically, and only on
clusters where those operators are actually installed).

Some ecosystems don't aggregate to `view` (OLM's `Subscription`/`ClusterServiceVersion`,
ArgoCD's `Application` -- ArgoCD manages its own RBAC internally). For these,
`global.additionalClusterRoleBindings` lets you bind a `ClusterRole` you create and own to
**all three** service accounts at once, without waiting on a Kubernaut release -- this is the
common case, since Gateway/EM/KA inspect the same owner-chain/target resource at different
pipeline stages and usually need identical visibility:

```yaml
# 1. Create and own a ClusterRole scoped to exactly what you want Kubernaut to read.
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-olm-reader
rules:
  - apiGroups: ["operators.coreos.com"]
    resources: ["subscriptions", "clusterserviceversions", "installplans"]
    verbs: ["get", "list", "watch"]
```

```yaml
# 2. Reference it by name once -- applies to gateway, effectivenessmonitor, AND
# kubernautAgent. Kubernaut only binds it, never authors its rules.
global:
  additionalClusterRoleBindings:
    - my-olm-reader
```

If you want asymmetric access instead -- most commonly, granting Gateway/EM an ecosystem
while withholding it from `kubernautAgent`, the highest-risk, LLM-driven component
(BR-PLATFORM-005) -- use the per-service fields instead of (or in addition to) the global
one: `gateway.additionalClusterRoleBindings`, `effectivenessmonitor.additionalClusterRoleBindings`,
`kubernautAgent.additionalClusterRoleBindings`. The same pattern works for ArgoCD
(`apiGroups: ["argoproj.io"]`, `resources: ["applications"]`) or any other resource kind.

### Infrastructure

**PostgreSQL/Valkey are deployed in-chart by default** (`postgresql.enabled`/`valkey.enabled`
both default `true`, hidden from the example `values.yaml` since the in-chart path is the
common case). See [BYO PostgreSQL / Valkey](#byo-postgresql--valkey) above to bring your own,
and `docs/generated/helm-values-reference.md#postgresql` /
`docs/generated/helm-values-reference.md#valkey` for the complete field lists.

See `docs/generated/helm-values-reference.md#datastorage` for DataStorage's own fields,
including the mandatory (GAP-05/09) `config.auditHashKey.existingSecret` (see
[Keyed Audit Hash Chain](#keyed-audit-hash-chain-gap-05)) and
`config.server.rateLimit.*` (see
[Data Storage Per-IP Rate Limiting](#data-storage-per-ip-rate-limiting-gap-09)).
See `docs/generated/helm-values-reference.md#apifrontend` for the optional,
opt-in (GAP-08) `config.auth.replayCache.*` (see
[Distributed JWT Replay Cache](#optional-distributed-jwt-replay-cache-gap-08)).

### Console

See `docs/generated/helm-values-reference.md#console` for the full field list
(`auth.secretName`, `oauth2Proxy.image`, `ingress.*`, `pdb.*`) and
[Optional: Web Console](#optional-web-console-br-platform-006) for the narrative walkthrough.

### TLS

See `docs/generated/helm-values-reference.md#tls` for `tls.mode`/`tls.certManager.issuerRef.name`.

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
toggle; `additionalProperties: false` in the schema rejects both if set. The 5 policies for
optional services (gateway/apifrontend/console/postgresql/valkey) are still a no-op when that
service itself is disabled -- gate on that service's own `enabled` field, not a
NetworkPolicy-specific one (Issue #2162 added gateway to this set).
See `docs/generated/helm-values-reference.md#networkpolicies` for the full field list
(`apiServerCIDR(s)`, `apiServerPort`, `monitoring.*`, `externalWebhooks.cidr`,
`externalRegistry.cidr`, `apifrontend.ingressNamespaces`, `console.ingressNamespaces`).

Each service gets a NetworkPolicy with:
- **Default-deny ingress** with service-specific allow rules
- **Egress**: most services restrict egress to DNS, K8s API, and known peers; **Kubernaut Agent uses an ingress-only policy** (unrestricted egress) because it must reach arbitrary LLM providers, MCP servers, and tool endpoints
- **Datastorage**: allows egress to PostgreSQL, Valkey, and external container registries (configurable CIDR for OCI bundle validation)
- **APIFrontend** (BR-PLATFORM-005, Kubernaut Operator parity — previously the only mesh component without a NetworkPolicy): ingress from same-namespace callers plus `networkPolicies.apifrontend.ingressNamespaces`; egress to DataStorage, Valkey (DD-PLATFORM-006 DA6: the JWT replay-cache is now mandatory, so this egress rule is unconditional), and OIDC/JWKS discovery (only when OIDC is configured)
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
> - **Prometheus toolset**: `--set monitoring.prometheus.enabled=true --set monitoring.prometheus.url=<url>`
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
