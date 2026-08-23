# Kubernaut Helm Chart Configuration Reference

Auto-generated from `charts/kubernaut/values.schema.json` by `hack/gen-helm-config-docs` (`make generate-helm-config-docs`). Do not edit by hand -- changes will be overwritten on the next generation and CI's drift check (`make generate-helm-config-docs && git diff --exit-code -- docs/generated/helm-values-reference.md`) will fail on a stale, hand-edited copy.

## aianalysis

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `policies.content` | string |  | `""` | No |
| `policies.existingConfigMap` | string |  | `""` | No |
| `rego.confidenceThreshold` | object | BR-AI-076: nil = use Rego default (0.8) | `` | No |
| `rego.lowConfidenceFloor` | object | BR-AI-088.4, Issue #1828: Investigating-phase floor for auto-proceeding with a KA-selected workflow (distinct from confidenceThreshold above, which tunes the later Rego auto-approval gate). nil = use the built-in 70% default. | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## apifrontend

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `autoscaling.cpuTarget` | integer | Target average CPU utilization percentage | `75` | No |
| `autoscaling.enabled` | boolean | Create a HorizontalPodAutoscaler for this service | `false` | No |
| `autoscaling.maxReplicas` | integer |  | `5` | No |
| `autoscaling.memoryTarget` | integer | Target average memory utilization percentage (0 disables the memory metric) | `80` | No |
| `autoscaling.minReplicas` | integer |  | `1` | No |
| `config.auth.allowInsecureIssuers` | boolean | Allow http:// (non-TLS) issuerURL/jwksURL values. Dev/test only -- never set true in production (SC-8). | `false` | No |
| `config.auth.audience` | string | Expected JWT audience claim | `""` | No |
| `config.auth.issuerURL` | string | OIDC issuer URL for JWT validation. When set, OIDC/JWKS auth is used. When empty, K8s TokenReview is auto-detected. | `""` | No |
| `config.auth.jwksURL` | string | JWKS endpoint URL for the legacy single-provider issuerURL (optional, defaults to issuerURL's well-known discovery when empty) | `""` | No |
| `config.auth.jwtProviders` | array of object | Multi-provider JWT configuration (#1436). When set, takes precedence over issuerURL/audience. | `` | No |
| `config.auth.oidcCaFile` | string | Path to a CA bundle used when fetching JWKS from issuerURL over TLS with a private/self-signed CA (e.g. an in-cluster OIDC test double signed by the chart's own inter-service CA). Requires mounting that CA into the pod -- the chart does this automatically when this field is set (or when global.fleet.enabled is true). | `""` | No |
| `config.auth.replayCache.enabled` | boolean |  | `false` | No |
| `config.auth.replayCache.redisDB` | integer | Valkey logical DB index used for replay cache keys | `1` | No |
| `config.auth.replayCache.tls.caFile` | string | CA bundle to verify the Valkey/Redis server certificate. Defaults to the chart's distributed inter-service CA (/etc/tls-ca/ca.crt) when left empty; override only for a BYO Valkey/Redis signed by a different CA. | `""` | No |
| `config.auth.replayCache.tls.certFile` | string | Client certificate file path (mTLS) -- unused against the chart's own Valkey (no client cert required), available for BYO Redis that requires mTLS | `""` | No |
| `config.auth.replayCache.tls.enabled` | boolean | Enable TLS for the replay-cache Valkey/Redis connection | `false` | No |
| `config.auth.replayCache.tls.keyFile` | string | Client key file path (mTLS) | `""` | No |
| `config.auth.replayCache.trustedProxyCIDRs` | array of string | DD-PLATFORM-006 DA18 (#1999): CIDRs of proxies/ingresses trusted to supply a forwarded client-IP header for jti replay-cache source binding. Empty by default -- fail-closed: no proxy header is trusted for this purpose until explicitly configured to match the deployment's real ingress topology (mirrors pkg/gateway's TrustedRealIP / DD-AUTH-003 pattern). | `[]` | No |
| `config.interactive.awaitSessionTimeout` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"3m"` | No |
| `config.interactive.bridgeInactivityTimeout` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"180s"` | No |
| `config.interactive.enabled` | boolean |  | `true` | No |
| `config.mcp.enabled` | boolean |  | `true` | No |
| `config.mcp.sessionIdleTimeout` | string | Issue #2220: was 5m, inconsistent with the 30m the Go-side cmd/apifrontend fallback silently applied whenever this was unset. Consolidated to a single 30m default, now also set explicitly in config.DefaultConfig(). | `"30m"` | No |
| `config.mcp.toolTimeout` | string | Issue #2221: default per-tool MCP call timeout, previously never exposed via Helm despite being wired into the MCP bridge (cmd/apifrontend/mcp_a2a_handlers.go). | `"30s"` | No |
| `config.mcp.toolTimeouts.kubernaut_await_session` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"3m"` | No |
| `config.mcp.toolTimeouts.kubernaut_discover_workflows` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"60s"` | No |
| `config.mcp.toolTimeouts.kubernaut_investigate` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"15m"` | No |
| `config.mcp.toolTimeouts.kubernaut_watch` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"15m"` | No |
| `config.rateLimit.ipRequestsPerSec` | integer |  | `10000` | No |
| `config.rateLimit.maxConcurrentSessions` | integer |  | `50` | No |
| `config.rateLimit.toolCallsPerMinute` | integer |  | `600` | No |
| `config.rateLimit.userRequestsPerSec` | integer |  | `100` | No |
| `config.rbac.consoleAccessAuthorizationCheckEnabled` | boolean | Enables the authorization check on the coarse-grained console gate (kubernaut.ai/console), so apifrontend.config.rbac.consoleAccessGroups takes precedence. Defaults to false (authentication-only) so dev/eval installs get a working console with zero RBAC configuration (#2150); production installs should configure consoleAccessGroups and set this to true to enable the authorization check on the console gate. Per-tool authorization is unaffected either way and remains unconditionally fail-closed. | `false` | No |
| `config.rbac.consoleAccessGroups` | array of string | OIDC groups granted the coarse-grained kubernaut-console-access ClusterRole (kubernaut.ai/console, verb=use), a separate grant from the per-tool personas above (#1919) | `` | No |
| `config.rbac.personas` | map[string]object | Map of persona name to list of allowed tool names for SAR-based authorization | `` | No |
| `config.rbac.sarCacheTTL` | string | TTL for SubjectAccessReview cache entries | `"30s"` | No |
| `config.resilience.prometheus.connectTimeout` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"5s"` | No |
| `config.resilience.prometheus.requestTimeout` | string | Raise this if Prometheus is known to be slower than usual in a given environment (e.g. a resource-constrained or heavily-loaded test cluster) -- #1839 found E2E fullpipeline's Prometheus timing out at the 10s default under that suite's scrape+eval+API load. | `"10s"` | No |
| `config.server.healthPort` | integer |  | `8081` | No |
| `config.server.httpPort` | integer |  | `8443` | No |
| `config.server.metricsPort` | integer |  | `9090` | No |
| `config.session.disconnectTTL` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"10m"` | No |
| `config.session.retentionTTL` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"720h"` | No |
| `config.severityTriage.cacheTTLSeconds` | integer |  | `30` | No |
| `config.severityTriage.llmConfidence` | number |  | `0.7` | No |
| `config.severityTriage.llmEnabled` | boolean | DD-PLATFORM-007: whether LLM-based severity-triage tiers are active. When false, no llm block is rendered, forcing rule-based-only triage -- independent of llmProfileRef and of severity triage as a whole. Matches the Kubernaut Operator's APIFrontendSeverityTriageSpec.LLMEnabled (default true). | `true` | No |
| `config.severityTriage.llmProfileRef` | string | DD-PLATFORM-007: name of an entry in global.llmProfiles for severity-triage's LLM fallback tiers, independent of API Frontend's own apifrontend.llmProfileRef connection. Empty (default) inherits API Frontend's own resolved profile, matching the Kubernaut Operator's APIFrontendSeverityTriageSpec.LLMProfileRef behavior. Newly exposed -- this Go capability (Config.SeverityTriage.LLM) existed but was previously unreachable via Helm. May reference a profile with a different provider/credentialsSecretName than API Frontend's own, including two independent vertex_ai profiles (kubernaut#1861 lifted the earlier kubernaut#1731 restriction requiring both to share the same credentialsSecretName). | `""` | No |
| `config.severityTriage.maxQueriesPerCall` | integer |  | `10` | No |
| `config.severityTriage.maxRulesEvaluated` | integer |  | `100` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `enabled` | boolean |  | `true` | No |
| `fleet.namespace` | string | Restricts the ClusterRegistry CRD watch to a single namespace instead of cluster-wide (BR-RBAC-020, #1686). Empty (default) watches all namespaces and grants cluster-wide RBAC | `""` | No |
| `fleet.oauth2.credentialsSecretRef` | string | K8s Secret with keys: client-id, client-secret. Overrides global.fleet.oauth2.credentialsSecretRef for this service only. | `""` | No |
| `fleet.resilience.connectTimeout` | string | Bounds each individual MCP connect attempt, independent of whether the caller's context carries a deadline (issue #1934). | `"30s"` | No |
| `fleet.resilience.discoverProbeTimeout` | string | Bounds the SEP-2575 server/discover probe independently of connectTimeout, so a gateway that hangs (rather than erroring) on that probe cannot consume connectTimeout's entire budget before the legacy initialize handshake fallback gets a chance to run (issue #2262). | `"5s"` | No |
| `fleet.resilience.initialInterval` | string | Starting backoff interval for startup connect retries. | `"1s"` | No |
| `fleet.resilience.maxElapsedTime` | string | Total time before giving up on startup connection. | `"5m"` | No |
| `fleet.resilience.maxInterval` | string | Maximum backoff interval between connect retries. | `"30s"` | No |
| `fleet.resilience.tokenRefreshTimeout` | string | Bounds OAuth2 token refresh HTTP calls. | `"10s"` | No |
| `image.pullPolicy` | string |  | `"IfNotPresent"` | No |
| `image.repository` | string |  | `"ghcr.io/jordigilh/kubernaut/apifrontend"` | No |
| `image.tag` | string |  | `""` | No |
| `ingress.annotations` | object |  | `{}` | No |
| `ingress.className` | string |  | `""` | No |
| `ingress.enabled` | boolean |  | `false` | No |
| `ingress.host` | string | Hostname for external access. Leaving this empty renders a catch-all Ingress rule. | `""` | No |
| `ingress.tls.secretName` | string | Pre-created Secret with a TLS cert for `host`; omit for HTTP-only or an ingress controller that provides its own default cert | `""` | No |
| `llmProfileRef` | string | DD-PLATFORM-007: name of an entry in global.llmProfiles for API Frontend's own agent-loop LLM connection. Empty (default) defaults to kubernautAgent.llmProfileRef's resolved profile, matching the Kubernaut Operator's AFLLMProfileRef fallback behavior. Newly exposed -- this Go capability (Config.Agent.LLM) existed but was previously unreachable via Helm. | `""` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `service.nodePort` | integer | SPIKE (Issue #1737): pin an explicit NodePort for the service's primary port when type=NodePort (must be in the 30000-32767 range per Kubernetes' default --service-node-port-range). 0 (default) preserves existing behavior -- Kubernetes auto-assigns a port. Intended for test/CI environments that need a deterministic, host-accessible port; production deployments should generally leave this unset and use an Ingress/Gateway instead. | `0` | No |
| `service.type` | string | Kubernetes Service type | `"ClusterIP"` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## authwebhook

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## console

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `auth.secretName` | string | Pre-created Secret with keys: client-id, client-secret, cookie-secret. Required when console.enabled=true. | `""` | No |
| `enabled` | boolean |  | `false` | No |
| `ingress.annotations` | object |  | `{}` | No |
| `ingress.className` | string |  | `""` | No |
| `ingress.enabled` | boolean |  | `false` | No |
| `ingress.host` | string | Hostname for external access. Leaving this empty renders a catch-all Ingress rule. | `""` | No |
| `ingress.tls.secretName` | string | Pre-created Secret with a TLS cert for `host`; omit for HTTP-only or an ingress controller that provides its own default cert | `""` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `oauth2Proxy.image` | string |  | `"quay.io/oauth2-proxy/oauth2-proxy:v7.15.3"` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |

## datastorage

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `autoscaling.cpuTarget` | integer | Target average CPU utilization percentage | `75` | No |
| `autoscaling.enabled` | boolean | Create a HorizontalPodAutoscaler for this service | `false` | No |
| `autoscaling.maxReplicas` | integer |  | `5` | No |
| `autoscaling.memoryTarget` | integer | Target average memory utilization percentage (0 disables the memory metric) | `80` | No |
| `autoscaling.minReplicas` | integer |  | `1` | No |
| `config.auditHashKey.existingSecret` | string | Pre-created Secret name; defaults to datastorage-audit-hmac-key when unset | `""` | No |
| `config.auditHashKey.secretKey` | string | Key name within the secret's audit-hmac-key.yaml file | `"hmacKey"` | No |
| `config.database.connMaxIdleTime` | string | Issue #2218: was a bare template literal while its 3 struct siblings (maxOpenConns/maxIdleConns/connMaxLifetime) were already schema-driven. Maximum connection idle time | `"10m"` | No |
| `config.database.connMaxLifetime` | string | Maximum connection lifetime | `"1h"` | No |
| `config.database.maxIdleConns` | integer | Maximum idle database connections | `20` | No |
| `config.database.maxOpenConns` | integer | Maximum open database connections | `100` | No |
| `config.database.sslMode` | string | PostgreSQL SSL mode | `"disable"` | No |
| `config.redis.dlqMaxLen` | integer | #1048 Phase 5 / AU-11: Redis DLQ MAXLEN bound | `10000` | No |
| `config.redis.tls.caFile` | string | CA bundle to verify Redis server | `""` | No |
| `config.redis.tls.certFile` | string | Client certificate file path (mTLS) | `""` | No |
| `config.redis.tls.keyFile` | string | Client key file path (mTLS) | `""` | No |
| `config.retention.batchSize` | integer | Max rows per DELETE batch | `1000` | No |
| `config.retention.defaultDays` | integer | Application-level default retention in days (ADR-034: 7 years) | `2555` | No |
| `config.retention.enabled` | boolean | Enable periodic purge of expired audit events | `false` | No |
| `config.retention.interval` | string | How often the retention worker runs | `"24h"` | No |
| `config.server.corsAllowedOrigins` | array of string | #1048 Phase 4 / AC-4: CORS allowed origins | `` | No |
| `config.server.maxBodySize` | string | #1048 Phase 4 / SC-5: Maximum request body size in bytes | `"5242880"` | No |
| `config.server.maxConcurrentRequests` | integer | Maximum concurrent request limit | `100` | No |
| `config.server.rateLimit.burst` | integer | Per-IP token bucket burst size | `100` | No |
| `config.server.rateLimit.requestsPerSecond` | number | Sustained per-IP requests/second | `50` | No |
| `config.server.readTimeout` | string | Server read timeout | `"30s"` | No |
| `config.server.signerCertDir` | string | #1048 Phase 5 / AU-9: Directory containing signing cert (tls.crt, tls.key) | `"/etc/certs"` | No |
| `config.server.writeTimeout` | string | Issue #2219: exact peer of readTimeout, previously missing entirely. Server write timeout | `"30s"` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `dbExistingSecret` | string | DEPRECATED: db-secrets.yaml is now in postgresql-secret. Set only for BYO PostgreSQL with a separate DataStorage secret. | `""` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `service.nodePort` | integer | SPIKE (Issue #1737): pin an explicit NodePort for the service's primary port when type=NodePort (must be in the 30000-32767 range per Kubernetes' default --service-node-port-range). 0 (default) preserves existing behavior -- Kubernetes auto-assigns a port. Intended for test/CI environments that need a deterministic, host-accessible port; production deployments should generally leave this unset and use an Ingress/Gateway instead. | `0` | No |
| `service.type` | string | Kubernetes Service type | `"ClusterIP"` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## effectivenessmonitor

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `additionalClusterRoles` | array of string | Issue #1069 (DD-GATEWAY-018): names of pre-existing ClusterRoles to bind to the EffectivenessMonitor ServiceAccount, for ecosystem resource kinds not already covered by the built-in view ClusterRole binding (#545). | `[]` | No |
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `config.assessment.stabilizationWindow` | string | Time to wait after remediation before assessing effectiveness | `"30s"` | No |
| `config.assessment.validityWindow` | string | Time window for assessment validity | `"120s"` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `fleet.namespace` | string | Restricts the ClusterRegistry CRD watch to a single namespace instead of cluster-wide (BR-RBAC-020, #1686). Empty (default) watches all namespaces and grants cluster-wide RBAC | `""` | No |
| `fleet.oauth2.credentialsSecretRef` | string | K8s Secret with keys: client-id, client-secret. Overrides global.fleet.oauth2.credentialsSecretRef for this service only. | `""` | No |
| `fleet.resilience.connectTimeout` | string | Bounds each individual MCP connect attempt, independent of whether the caller's context carries a deadline (issue #1934). | `"30s"` | No |
| `fleet.resilience.discoverProbeTimeout` | string | Bounds the SEP-2575 server/discover probe independently of connectTimeout, so a gateway that hangs (rather than erroring) on that probe cannot consume connectTimeout's entire budget before the legacy initialize handshake fallback gets a chance to run (issue #2262). | `"5s"` | No |
| `fleet.resilience.initialInterval` | string | Starting backoff interval for startup connect retries. | `"1s"` | No |
| `fleet.resilience.maxElapsedTime` | string | Total time before giving up on startup connection. | `"5m"` | No |
| `fleet.resilience.maxInterval` | string | Maximum backoff interval between connect retries. | `"30s"` | No |
| `fleet.resilience.tokenRefreshTimeout` | string | Bounds OAuth2 token refresh HTTP calls. | `"10s"` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## fleetmetadatacache

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `enabled` | boolean | DD-PLATFORM-006 Decision Area 10: when unset, the effective value is derived from global.fleet.enabled + global.fleet.backend (true whenever the fleet backend resolves to "fleetmetadatacache"); set explicitly to override. An explicit false that contradicts a derived-true fails the render. | `false` | No |
| `image.pullPolicy` | string |  | `"IfNotPresent"` | No |
| `image.repository` | string |  | `"quay.io/kubernaut-ai/fleetmetadatacache"` | No |
| `image.tag` | string |  | `"v1.6.0"` | No |
| `keyTTL` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"45s"` | No |
| `namespace` | string |  | `"kubernaut-system"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `oauth2.credentialsSecretRef` | string | K8s Secret with keys: client-id, client-secret. Overrides global.fleet.oauth2.credentialsSecretRef for this service only. | `""` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resilience.connectTimeout` | string | Bounds each individual MCP connect attempt, independent of whether the caller's context carries a deadline (issue #1934). | `"30s"` | No |
| `resilience.discoverProbeTimeout` | string | Bounds the SEP-2575 server/discover probe independently of connectTimeout, so a gateway that hangs (rather than erroring) on that probe cannot consume connectTimeout's entire budget before the legacy initialize handshake fallback gets a chance to run (issue #2262). | `"5s"` | No |
| `resilience.initialInterval` | string | Starting backoff interval for startup connect retries. | `"1s"` | No |
| `resilience.maxElapsedTime` | string | Total time before giving up on startup connection. | `"5m"` | No |
| `resilience.maxInterval` | string | Maximum backoff interval between connect retries. | `"30s"` | No |
| `resilience.tokenRefreshTimeout` | string | Bounds OAuth2 token refresh HTTP calls. | `"10s"` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `syncInterval` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"30s"` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |
| `valkeyAddr` | string |  | `""` | No |
| `valkeyTLS.caFile` | string | CA bundle to verify the Valkey server certificate. Defaults to the chart's distributed inter-service CA (/etc/tls-ca/ca.crt) when left empty; override only for a BYO Valkey/Redis signed by a different CA. | `""` | No |
| `valkeyTLS.certFile` | string | Client certificate file path (mTLS) -- unused against the chart's own Valkey (no client cert required), available for BYO Valkey/Redis that requires mTLS | `""` | No |
| `valkeyTLS.keyFile` | string | Client key file path (mTLS) | `""` | No |

## fullnameOverride

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `fullnameOverride` | string | Override the full resource names | `""` | No |

## gateway

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `additionalClusterRoles` | array of string | Issue #1069 (DD-GATEWAY-018): names of pre-existing ClusterRoles to bind to the Gateway ServiceAccount, for ecosystem resource kinds not already covered by the built-in view ClusterRole binding (e.g. OLM Subscription, ArgoCD Application). | `[]` | No |
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `auth.signalSources` | array of object | External signal sources that need RBAC to call the Gateway | `` | No |
| `config.cors.allowCredentials` | boolean | Whether cross-origin requests may include credentials. | `false` | No |
| `config.cors.allowedOrigins` | array of string | Allowed CORS origins. M2M API — deny by default. | `[                     "https://no-browser-clients.invalid"                   ]` | No |
| `config.cors.maxAge` | integer | Preflight cache duration in seconds. | `300` | No |
| `config.deduplication.cooldownPeriod` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"5m"` | No |
| `config.middleware.trustedProxyCIDRs` | array of string | Issue #673 L-1: CIDRs whose proxy headers are trusted | `[]` | No |
| `config.server.idleTimeout` | string | Issue #2217: was a bare template literal while its 3 struct siblings (maxConcurrentRequests/readTimeout/writeTimeout) were already schema-driven | `"120s"` | No |
| `config.server.k8sRequestTimeout` | string | Per-handler K8s API timeout (BR-GATEWAY-102) | `"15s"` | No |
| `config.server.maxConcurrentRequests` | integer |  | `100` | No |
| `config.server.readTimeout` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"30s"` | No |
| `config.server.writeTimeout` | string | Go time.Duration string (e.g. "30s", "5m", "1h30m") | `"30s"` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `enabled` | boolean | Issue #2162: whether the Gateway component (Deployment, Service, RBAC) is deployed. Independent of apifrontend.enabled -- either, both, or neither ingress point may be enabled. | `true` | No |
| `fleet.oauth2.credentialsSecretRef` | string | K8s Secret with keys: client-id, client-secret. Overrides global.fleet.oauth2.credentialsSecretRef for this service only. | `""` | No |
| `fleet.resilience.connectTimeout` | string | Bounds each individual MCP connect attempt, independent of whether the caller's context carries a deadline (issue #1934). | `"30s"` | No |
| `fleet.resilience.discoverProbeTimeout` | string | Bounds the SEP-2575 server/discover probe independently of connectTimeout, so a gateway that hangs (rather than erroring) on that probe cannot consume connectTimeout's entire budget before the legacy initialize handshake fallback gets a chance to run (issue #2262). | `"5s"` | No |
| `fleet.resilience.initialInterval` | string | Starting backoff interval for startup connect retries. | `"1s"` | No |
| `fleet.resilience.maxElapsedTime` | string | Total time before giving up on startup connection. | `"5m"` | No |
| `fleet.resilience.maxInterval` | string | Maximum backoff interval between connect retries. | `"30s"` | No |
| `fleet.resilience.tokenRefreshTimeout` | string | Bounds OAuth2 token refresh HTTP calls. | `"10s"` | No |
| `ingress.annotations` | object |  | `{}` | No |
| `ingress.className` | string |  | `""` | No |
| `ingress.enabled` | boolean |  | `false` | No |
| `ingress.host` | string | Hostname for external access. Leaving this empty renders a catch-all Ingress rule. | `""` | No |
| `ingress.tls.secretName` | string | Pre-created Secret with a TLS cert for `host`; omit for HTTP-only or an ingress controller that provides its own default cert | `""` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `service.nodePort` | integer | SPIKE (Issue #1737): pin an explicit NodePort for the service's primary port when type=NodePort (must be in the 30000-32767 range per Kubernetes' default --service-node-port-range). 0 (default) preserves existing behavior -- Kubernetes auto-assigns a port. Intended for test/CI environments that need a deterministic, host-accessible port; production deployments should generally leave this unset and use an Ingress/Gateway instead. | `0` | No |
| `service.type` | string | Kubernetes Service type | `"ClusterIP"` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## global

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `additionalClusterRoles` | array of string | Issue #1069 (DD-GATEWAY-018): names of pre-existing ClusterRoles to bind to ALL THREE of Gateway, EffectivenessMonitor, and Kubernaut Agent's ServiceAccounts -- the common case, since all three inspect the same owner-chain/target resources at different pipeline stages and usually need identical ecosystem visibility (e.g. OLM Subscription, ArgoCD Application). Merged with each service's own additionalClusterRoles (deduplicated). Use the per-service field instead of this one when you deliberately want asymmetric access -- e.g. withholding an ecosystem grant from kubernautAgent, the highest-risk, LLM-driven component (BR-PLATFORM-005). | `[]` | No |
| `defaultResources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `defaultResources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `defaultResources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `defaultResources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `fleet.backend` | string | Backend adapter for GW/RO's federated scope-check: fleetmetadatacache (FMC HTTP API) or acm (ACM Search). Defaults to fleetmetadatacache when empty. | `""` | No |
| `fleet.enabled` | boolean | Multi-cluster fleet federation on/off (Issue #1707), consolidated from four independent per-service toggles (gateway/remediationorchestrator/apifrontend/effectivenessmonitor fleet.enabled) | `false` | No |
| `fleet.endpoint` | string | Endpoint for the selected backend. Auto-derived for fleetmetadatacache; required for acm. | `""` | No |
| `fleet.mcpGatewayEndpoint` | string | Shared MCP Gateway endpoint URL, used by every fleet-integration-capable service (Issue #1707 follow-up: global-only, no per-service override). | `""` | No |
| `fleet.mcpGatewayNamespace` | string | Shared fallback restricting the ClusterRegistry CRD watch to a single namespace, used when a service's own fleet.namespace (signalprocessing) or top-level namespace (fleetmetadatacache) is unset (Issue #1730). Empty (default) leaves each service's own default/cluster-wide behavior unchanged. The per-service override still wins when explicitly set. | `""` | No |
| `fleet.mcpGatewayType` | string | Shared MCP Gateway type (eaigw or kuadrant), used by every fleet-integration-capable service (Issue #1707 follow-up: global-only, no per-service override). | `""` | No |
| `fleet.oauth2.credentialsSecretRef` | string | Shared Secret name (keys: client-id, client-secret) for MCP Gateway OAuth2, used as the fallback when a service's own fleet.oauth2.credentialsSecretRef is unset (the one field that remains overridable per service). | `""` | No |
| `fleet.oauth2.enabled` | boolean | OAuth2 client_credentials auth on/off for MCP Gateway authentication (Issue #1707 follow-up: global-only, no per-service override). | `false` | No |
| `fleet.oauth2.scopes` | array of string | Shared OAuth2 scopes for MCP Gateway authentication, used by every fleet-integration-capable service (Issue #1707 follow-up: global-only, no per-service override). | `[]` | No |
| `fleet.oauth2.tlsCAFile` | string | Shared CA bundle path for verifying tokenURL's TLS certificate, used by every fleet-integration-capable service (Issue #1707 follow-up: global-only, no per-service override). | `""` | No |
| `fleet.oauth2.tokenURL` | string | Shared OAuth2 token URL for MCP Gateway authentication, used by every fleet-integration-capable service (Issue #1707 follow-up: global-only, no per-service override). | `""` | No |
| `fleet.resilience.connectTimeout` | string | Bounds each individual MCP connect attempt, independent of whether the caller's context carries a deadline (issue #1934). | `"30s"` | No |
| `fleet.resilience.discoverProbeTimeout` | string | Bounds the SEP-2575 server/discover probe independently of connectTimeout, so a gateway that hangs (rather than erroring) on that probe cannot consume connectTimeout's entire budget before the legacy initialize handshake fallback gets a chance to run (issue #2262). | `"5s"` | No |
| `fleet.resilience.initialInterval` | string | Starting backoff interval for startup connect retries. | `"1s"` | No |
| `fleet.resilience.maxElapsedTime` | string | Total time before giving up on startup connection. | `"5m"` | No |
| `fleet.resilience.maxInterval` | string | Maximum backoff interval between connect retries. | `"30s"` | No |
| `fleet.resilience.tokenRefreshTimeout` | string | Bounds OAuth2 token refresh HTTP calls. | `"10s"` | No |
| `fleet.tlsCAFile` | string | Shared CA bundle path for verifying the MCP Gateway's TLS certificate, used by every fleet-integration-capable service (Issue #1707 follow-up: global-only, no per-service override). | `""` | No |
| `fleet.tokenSecretRef` | string | K8s Secret (key: token) with a bearer token for authenticating to the ACM Search GraphQL API (backend=acm). Mandatory when backend=acm; FleetConfig.Validate() rejects backend=acm without it (#1556). | `""` | No |
| `image.digest` | string | Image digest (e.g. sha256:abc...); when set, overrides tag for immutable references | `""` | No |
| `image.namespace` | string | Image namespace prefix. Joined to service name by separator. | `"kubernaut-ai"` | No |
| `image.pullPolicy` | string | Image pull policy | `"IfNotPresent"` | No |
| `image.registry` | string | Container image registry hostname (e.g. quay.io, harbor.corp, image-registry.openshift-image-registry.svc:5000/ns) | `"quay.io"` | No |
| `image.separator` | string | Character joining namespace to service name. Use '/' for nested registries (Harbor, Artifactory), '-' for flat registries (quay.io, Docker Hub). | `"/"` | No |
| `image.tag` | string | Image tag; empty defaults to Chart.appVersion | `""` | No |
| `imagePullSecrets` | array of object | List of image pull secret names for private registries | `[]` | No |
| `llmProfiles` | map[string]object | Named LLM provider profiles, referenced by name from kubernautAgent.llmProfileRef, kubernautAgent.phaseModels, kubernautAgent.alignmentCheck.llmProfileRef, apifrontend.llmProfileRef, and apifrontend.config.severityTriage.llmProfileRef (DD-PLATFORM-007). Mirrors the Kubernaut Operator's spec.llmProfiles authoring model -- a profile defined once is reusable by any consumer without re-deriving its field set. | `{}` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `podDefaults.pdb.enabled` | boolean |  | `true` | No |
| `podDefaults.pdb.maxUnavailable` | object |  | `` | No |
| `podDefaults.pdb.minAvailable` | object |  | `` | No |
| `tolerations` | array of object | Pod tolerations | `[]` | No |

## hooks

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `tlsCerts.extraSANs` | array of string | PR #1790 round-14 RCA: extra DNS names added to every chart-issued inter-service leaf cert's SAN (gateway/datastorage/kubernaut-agent/fleetmetadatacache/apifrontend/valkey), alongside the standard in-cluster Service DNS names. 'IP:127.0.0.1' is added automatically whenever this is non-empty. Empty by default -- a real cluster is never accessed via localhost, so this has zero effect on production installs. Exists so host-based test/dev clients (e.g. `https://localhost:<nodePort>`) can pass full TLS hostname verification against the chart's own certs without a separate post-install re-sign+restart pass (which previously caused deterministic rollout timeouts under E2E's resource-constrained CI runners -- see resignHostAccessedTLSCertsWithLocalhostSAN's removal in test/infrastructure/fullpipeline_e2e.go). | `[]` | No |
| `tlsCerts.image` | string |  | `"docker.io/bitnami/kubectl:latest@sha256:6e2cdb22d6ab7264ea198c717f555e30536b54029d26c8781b9f25f78951b564"` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |

## kubernautAgent

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `additionalClusterRoles` | array of string | Issue #1069 (DD-GATEWAY-018): names of pre-existing ClusterRoles to bind to the Kubernaut Agent ServiceAccount, generalizing kubernaut-operator's equivalent KA-only mechanism to the Helm chart, for resource kinds not already covered by its investigator ClusterRole. | `[]` | No |
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `ai.safety.anomaly.maxTotalToolCalls` | integer | Maximum total tool calls allowed per investigation phase before the investigation aborts as budget-exhausted (surfaces as MCPError code tool_budget_exhausted for interactive sessions). | `30` | No |
| `alignmentCheck.enabled` | boolean | Enable the shadow alignment agent. | `false` | No |
| `alignmentCheck.llmProfileRef` | string | DD-PLATFORM-007: name of an entry in global.llmProfiles for a dedicated alignment-check shadow-agent LLM. Empty (default) inherits kubernautAgent.llmProfileRef's resolved profile, reusing the investigation LLM. Replaces the old alignmentCheck.llm.* block, whose apiKey field never had any effect (the Kubernaut Agent binary only reads apiKeyFile -- see kubernaut#1726 for the same class of bug). | `""` | No |
| `alignmentCheck.maxStepTokens` | integer | Max evaluator response tokens per step. | `500` | No |
| `alignmentCheck.timeout` | string | Per-step evaluation timeout. | `"10s"` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `fleet.oauth2.credentialsSecretRef` | string | K8s Secret with keys: client-id, client-secret. Overrides global.fleet.oauth2.credentialsSecretRef for this service only. | `""` | No |
| `fleet.resilience.connectTimeout` | string | Bounds each individual MCP connect attempt, independent of whether the caller's context carries a deadline (issue #1934). | `"30s"` | No |
| `fleet.resilience.discoverProbeTimeout` | string | Bounds the SEP-2575 server/discover probe independently of connectTimeout, so a gateway that hangs (rather than erroring) on that probe cannot consume connectTimeout's entire budget before the legacy initialize handshake fallback gets a chance to run (issue #2262). | `"5s"` | No |
| `fleet.resilience.initialInterval` | string | Starting backoff interval for startup connect retries. | `"1s"` | No |
| `fleet.resilience.maxElapsedTime` | string | Total time before giving up on startup connection. | `"5m"` | No |
| `fleet.resilience.maxInterval` | string | Maximum backoff interval between connect retries. | `"30s"` | No |
| `fleet.resilience.tokenRefreshTimeout` | string | Bounds OAuth2 token refresh HTTP calls. | `"10s"` | No |
| `interactive.enabled` | boolean | Enable MCP interactive mode endpoint and Lease-based session management. DD-PLATFORM-006 Decision Area 11: requires apifrontend.enabled=true (APIFrontend's ka.NewSDKMCPClient is the only caller of this endpoint); the render fails otherwise. | `false` | No |
| `interactive.inactivityTimeout` | string | Session timeout after last activity. | `"10m"` | No |
| `interactive.jwtProviders` | array of object | JWT providers for Pattern B authentication (DD-AUTH-MCP-001 v2.0). | `` | No |
| `interactive.maxAnalyzingTimeout` | string | Extended analyzing timeout for RO when an interactive session is active. Prevents RO from timing out a RemediationRequest while an operator is investigating. | `"45m"` | No |
| `interactive.maxConcurrentSessions` | integer | Maximum concurrent interactive sessions per instance. Issue #1737: raised from 5 to 50 -- the old default was too low even for realistic production use and caused full-suite E2E failures purely from Ginkgo's own test parallelism. | `50` | No |
| `interactive.rateLimitPerUser` | integer | Maximum requests per second per authenticated user. Issue #1737: raised from 10 to 20, alongside maxConcurrentSessions. | `20` | No |
| `interactive.sessionTTL` | string | Maximum duration for an interactive session before auto-release. | `"30m"` | No |
| `investigation.inconclusiveConfidenceThreshold` | number | Confidence below which the LLM classifies an investigation outcome as "inconclusive". Must be less than resolvedConfidenceThreshold. | `0.5` | No |
| `investigation.resolvedConfidenceThreshold` | number | Minimum confidence for the LLM to classify an investigation outcome as "resolved" (problem self-resolved, no workflow needed). | `0.7` | No |
| `llmProfileRef` | string | Name of an entry in global.llmProfiles used for KA's investigator LLM calls (DD-PLATFORM-007). When omitted (or set to ""), inferred automatically if global.llmProfiles defines exactly one profile; otherwise (zero or 2+ profiles) an explicit value is required and the render fails with an actionable message (Issue #1987, DD-PLATFORM-006 DA4 Addendum). An explicit non-empty value always wins outright; an unresolvable/undefined profile name still fails the render either way. Replaces the old kubernautAgent.llm.* literal block. | `""` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `phaseModels` | map[string]object | Per-phase LLM profile overrides (DD-PLATFORM-007), newly exposed via Helm. Maps a phase name (rca, workflow_discovery, validation) to a profile name in global.llmProfiles; unlisted phases use kubernautAgent.llmProfileRef's profile unchanged. | `{}` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `service.nodePort` | integer | SPIKE (Issue #1737): pin an explicit NodePort for the service's primary port when type=NodePort (must be in the 30000-32767 range per Kubernetes' default --service-node-port-range). 0 (default) preserves existing behavior -- Kubernetes auto-assigns a port. Intended for test/CI environments that need a deterministic, host-accessible port; production deployments should generally leave this unset and use an Ingress/Gateway instead. | `0` | No |
| `service.type` | string | Kubernetes Service type | `"ClusterIP"` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## monitoring

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `alertManager.enabled` | boolean | Enable AlertManager integration for EM | `false` | No |
| `alertManager.tlsCaFile` | string | Path to PEM CA bundle for HTTPS. | `""` | No |
| `alertManager.url` | string | AlertManager API base URL. | `""` | No |
| `prometheus.enabled` | boolean | Enable Prometheus integration for both EM and KA | `false` | No |
| `prometheus.tlsCaFile` | string | Path to PEM CA bundle for HTTPS. | `""` | No |
| `prometheus.url` | string | Prometheus API base URL. | `""` | No |
| `prometheusRule.additionalLabels` | object | Additional labels for PrometheusRule (e.g. prometheus: kube-prometheus) | `{}` | No |
| `prometheusRule.enabled` | boolean | Create PrometheusRule resources (interactive session SLOs, DataStorage/APIFrontend alerting) | `false` | No |
| `prometheusRule.thresholds.activeSessionsFor` | string | Duration threshold must be exceeded before firing | `"5m"` | No |
| `prometheusRule.thresholds.activeSessionsMax` | integer | Max concurrent interactive sessions before alerting | `10` | No |
| `prometheusRule.thresholds.commandDurationFor` | string | Duration before firing | `"10m"` | No |
| `prometheusRule.thresholds.commandDurationP99Max` | number | Max p99 command duration in seconds | `30` | No |
| `prometheusRule.thresholds.leaseContentionFor` | string | Duration before firing | `"5m"` | No |
| `prometheusRule.thresholds.leaseContentionRateMax` | number | Max Lease contention events/sec (5m rate) | `0.1` | No |
| `prometheusRule.thresholds.takeoverRateFor` | string | Duration before firing | `"15m"` | No |
| `prometheusRule.thresholds.takeoverRateMax` | number | Max takeover events/sec (5m rate) | `0.05` | No |
| `serviceMonitor.enabled` | boolean | Create ServiceMonitor resources for all metrics-emitting services | `false` | No |

## nameOverride

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `nameOverride` | string | Override the chart name in resource names | `""` | No |

## networkPolicies

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `apiServerCIDR` | string | K8s API server real backend endpoint IP as a /32 CIDR (e.g., 10.89.0.2/32) -- NOT the 'kubernetes' Service ClusterIP, which most CNIs ignore for NetworkPolicy egress. Usually left empty: auto-discovered via `lookup` during a real helm install/upgrade. Set explicitly only for helm template/GitOps rendering, restricted-RBAC installers, or to override discovery. Use for single-control-plane clusters; use apiServerCIDRs for HA. | `""` | No |
| `apiServerCIDRs` | array of string | Additional K8s API server backend endpoint IPs as /32 CIDRs, for HA clusters with multiple control-plane nodes. Merged with apiServerCIDR. Usually left empty (see apiServerCIDR). | `[]` | No |
| `apiServerPort` | integer | Port the K8s API server's real backend endpoint(s) listen on (commonly 6443), not necessarily the 'kubernetes' Service's port (443). 0 means auto-discover via `lookup` (falls back to 443 if unavailable). | `0` | No |
| `apifrontend.ingressCIDRs` | array of string | CIDR blocks allowed as ingress sources (ipBlock). Required for traffic not associated with any pod/namespace -- e.g. NodePort-sourced host traffic (SNAT'd to the node's own IP) or a hostNetwork-mode ingress controller/router -- since podSelector/namespaceSelector can never match non-pod-associated source IPs (Issue #1737). | `[]` | No |
| `apifrontend.ingressNamespaceSelectors` | array of object | Raw namespaceSelector label selectors allowed as ingress sources, for cases the simple name-based ingressNamespaces list cannot express. | `[]` | No |
| `apifrontend.ingressNamespaces` | array of string | Namespaces allowed to send ingress to APIFrontend (e.g., an ingress-controller namespace) | `[]` | No |
| `console.ingressCIDRs` | array of string | CIDR blocks allowed as ingress sources (ipBlock). Required for traffic not associated with any pod/namespace -- e.g. NodePort-sourced host traffic (SNAT'd to the node's own IP) or a hostNetwork-mode ingress controller/router -- since podSelector/namespaceSelector can never match non-pod-associated source IPs (Issue #1737). | `[]` | No |
| `console.ingressNamespaceSelectors` | array of object | Raw namespaceSelector label selectors allowed as ingress sources, for cases the simple name-based ingressNamespaces list cannot express. | `[]` | No |
| `console.ingressNamespaces` | array of string | Namespaces allowed to send ingress to the console (e.g., an ingress-controller namespace) | `[]` | No |
| `datastorage.ingressCIDRs` | array of string | CIDR blocks allowed as ingress sources (ipBlock). Required for traffic not associated with any pod/namespace -- e.g. NodePort-sourced host traffic (SNAT'd to the node's own IP) or a hostNetwork-mode ingress controller/router -- since podSelector/namespaceSelector can never match non-pod-associated source IPs (Issue #1737). | `[]` | No |
| `datastorage.ingressNamespaceSelectors` | array of object | Raw namespaceSelector label selectors allowed as ingress sources, for cases the simple name-based ingressNamespaces list cannot express. | `[]` | No |
| `externalRegistry.cidr` | string | CIDR for OCI container registry egress (datastorage bundle validation) | `"0.0.0.0/0"` | No |
| `externalRegistry.port` | integer | Port for OCI container registry | `443` | No |
| `externalWebhooks.cidr` | string | CIDR for external webhook endpoints (Slack, PagerDuty, Teams) | `"0.0.0.0/0"` | No |
| `externalWebhooks.port` | integer | Port for external webhook endpoints | `443` | No |
| `gateway.ingressCIDRs` | array of string | CIDR blocks allowed as ingress sources (ipBlock). Required for traffic not associated with any pod/namespace -- e.g. NodePort-sourced host traffic (SNAT'd to the node's own IP) or a hostNetwork-mode ingress controller/router -- since podSelector/namespaceSelector can never match non-pod-associated source IPs (Issue #1737). | `[]` | No |
| `gateway.ingressNamespaceSelectors` | array of object | Raw namespaceSelector label selectors allowed as ingress sources, for cases the simple name-based ingressNamespaces list cannot express. | `[]` | No |
| `gateway.ingressNamespaces` | array of string | Namespaces allowed to send ingress to Gateway (e.g., monitoring for AlertManager) | `[]` | No |
| `idp.cidr` | string |  | `"0.0.0.0/0"` | No |
| `idp.extraPorts` | array of integer | Additional IdP ports to open egress on against the same cidr, for deployments where one service must reach two different IdPs on two different ports. | `[]` | No |
| `idp.port` | integer |  | `443` | No |
| `kubernautAgent.ingressCIDRs` | array of string | CIDR blocks allowed as ingress sources (ipBlock). Required for traffic not associated with any pod/namespace -- e.g. NodePort-sourced host traffic (SNAT'd to the node's own IP) or a hostNetwork-mode ingress controller/router -- since podSelector/namespaceSelector can never match non-pod-associated source IPs (Issue #1737). | `[]` | No |
| `kubernautAgent.ingressNamespaceSelectors` | array of object | Raw namespaceSelector label selectors allowed as ingress sources, for cases the simple name-based ingressNamespaces list cannot express. | `[]` | No |
| `llm.cidr` | string |  | `"0.0.0.0/0"` | No |
| `llm.port` | integer |  | `443` | No |
| `mcpGateway.cidr` | string |  | `"0.0.0.0/0"` | No |
| `mcpGateway.port` | integer |  | `8080` | No |
| `monitoring.alertManagerPort` | integer | AlertManager port to allow in the NetworkPolicy egress rule | `9093` | No |
| `monitoring.namespace` | string | Namespace where Prometheus scrapes from | `""` | No |
| `monitoring.prometheusPort` | integer | Prometheus port to allow in the NetworkPolicy egress rule | `9090` | No |
| `prometheus.cidr` | string |  | `"0.0.0.0/0"` | No |
| `prometheus.port` | integer |  | `9090` | No |

## notification

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `credentials` | array of object | Per-receiver credentials projected into /etc/notification/credentials/ | `` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `routing.content` | string |  | `""` | No |
| `routing.existingConfigMap` | string |  | `""` | No |
| `slack.channel` | string | Slack channel for notifications. | `"#kubernaut-alerts"` | No |
| `slack.secretKey` | string | Key within the Secret that holds the webhook URL. | `"webhook-url"` | No |
| `slack.secretName` | string | K8s Secret containing Slack webhook URL. Empty = no Slack. | `""` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## postgresql

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `auth.database` | string |  | `"action_history"` | No |
| `auth.existingSecret` | string | Pre-created Secret name with POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB, db-secrets.yaml keys. Empty = expect 'postgresql-secret'. | `""` | No |
| `auth.username` | string |  | `"slm_user"` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `enabled` | boolean | Set false to skip in-chart PostgreSQL and use external host/port instead | `true` | No |
| `host` | string | External PostgreSQL host. Required when enabled=false. | `""` | No |
| `image` | string |  | `"postgres:16-alpine"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `port` | integer |  | `5432` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `storage.size` | string |  | `"10Gi"` | No |
| `storage.storageClassName` | string | Empty = cluster default | `""` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |

## remediationorchestrator

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `config.asyncPropagation.gitOpsSyncDelay` | string | Expected time for GitOps tool (ArgoCD/Flux) to sync changes to cluster | `"3m"` | No |
| `config.asyncPropagation.operatorReconcileDelay` | string | Expected time for operator to reconcile after CR update | `"1m"` | No |
| `config.asyncPropagation.proactiveAlertDelay` | string | Extra delay for proactive alert resolution | `"5m"` | No |
| `config.effectivenessAssessment.stabilizationWindow` | string | Time for system stabilization after remediation before assessment | `"5m"` | No |
| `config.notifications.notifySelfResolved` | boolean | Emit status-update notification when signal self-resolves (BR-ORCH-037 AC-037-08) | `false` | No |
| `config.retention.period` | string | Duration before terminal RemediationRequests are cleaned up | `"24h"` | No |
| `config.routing.consecutiveFailureCooldown` | string | Cooldown after consecutive failures | `"1h"` | No |
| `config.routing.consecutiveFailureThreshold` | integer | Failures before cooldown | `3` | No |
| `config.routing.ineffectiveChainThreshold` | integer | Ineffective workflow chain length before escalation | `3` | No |
| `config.routing.ineffectiveTimeWindow` | string | Time window for ineffectiveness evaluation | `"4h"` | No |
| `config.routing.recentlyRemediatedCooldown` | string | Cooldown for recently remediated signals | `"5m"` | No |
| `config.routing.recurrenceCountThreshold` | integer | Signal recurrence count before escalation | `5` | No |
| `config.timeouts.analyzing` | string | AI analysis phase timeout | `"10m"` | No |
| `config.timeouts.executing` | string | Workflow execution phase timeout | `"30m"` | No |
| `config.timeouts.global` | string | Global remediation timeout | `"1h"` | No |
| `config.timeouts.processing` | string | Signal processing phase timeout | `"5m"` | No |
| `config.timeouts.verifying` | string | Effectiveness verification phase timeout | `"30m"` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `fleet.oauth2.credentialsSecretRef` | string | K8s Secret with keys: client-id, client-secret. Overrides global.fleet.oauth2.credentialsSecretRef for this service only. | `""` | No |
| `fleet.resilience.connectTimeout` | string | Bounds each individual MCP connect attempt, independent of whether the caller's context carries a deadline (issue #1934). | `"30s"` | No |
| `fleet.resilience.discoverProbeTimeout` | string | Bounds the SEP-2575 server/discover probe independently of connectTimeout, so a gateway that hangs (rather than erroring) on that probe cannot consume connectTimeout's entire budget before the legacy initialize handshake fallback gets a chance to run (issue #2262). | `"5s"` | No |
| `fleet.resilience.initialInterval` | string | Starting backoff interval for startup connect retries. | `"1s"` | No |
| `fleet.resilience.maxElapsedTime` | string | Total time before giving up on startup connection. | `"5m"` | No |
| `fleet.resilience.maxInterval` | string | Maximum backoff interval between connect retries. | `"30s"` | No |
| `fleet.resilience.tokenRefreshTimeout` | string | Bounds OAuth2 token refresh HTTP calls. | `"10s"` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## signalprocessing

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `config.classifier.hotReloadInterval` | string | Rego classification policy hot-reload poll interval (ClassifierConfig.HotReloadInterval) | `"30s"` | No |
| `config.enrichment.cacheTtl` | string | Enrichment cache lifetime (EnrichmentConfig.CacheTTL) | `"5m"` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `fleet.namespace` | string | Restricts the ClusterRegistry CRD watch; empty watches all namespaces | `""` | No |
| `fleet.oauth2.credentialsSecretRef` | string | K8s Secret with keys: client-id, client-secret. Overrides global.fleet.oauth2.credentialsSecretRef for this service only. | `""` | No |
| `fleet.resilience.connectTimeout` | string | Bounds each individual MCP connect attempt, independent of whether the caller's context carries a deadline (issue #1934). | `"30s"` | No |
| `fleet.resilience.discoverProbeTimeout` | string | Bounds the SEP-2575 server/discover probe independently of connectTimeout, so a gateway that hangs (rather than erroring) on that probe cannot consume connectTimeout's entire budget before the legacy initialize handshake fallback gets a chance to run (issue #2262). | `"5s"` | No |
| `fleet.resilience.initialInterval` | string | Starting backoff interval for startup connect retries. | `"1s"` | No |
| `fleet.resilience.maxElapsedTime` | string | Total time before giving up on startup connection. | `"5m"` | No |
| `fleet.resilience.maxInterval` | string | Maximum backoff interval between connect retries. | `"30s"` | No |
| `fleet.resilience.tokenRefreshTimeout` | string | Bounds OAuth2 token refresh HTTP calls. | `"10s"` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `policies.content` | string | Raw Rego policy content. Inject via --set-file signalprocessing.policies.content=policy.rego | `""` | No |
| `policies.existingConfigMap` | string | Pre-existing ConfigMap with key 'policy.rego'. Mutually exclusive with 'content'. | `""` | No |
| `proactiveSignalMappings.content` | string |  | `""` | No |
| `proactiveSignalMappings.existingConfigMap` | string |  | `""` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |

## telemetry

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `endpoint` | string | OTLP/HTTP collector endpoint (host:port, no scheme). Empty disables OTLP export. Requires an operator-supplied collector (Jaeger, Tempo, a vendor APM). | `""` | No |
| `logSink` | boolean | Emit one structured log line per completed span via the service's existing logger. No collector needed -- lands in the same log stream captured by must-gather/CI log collection. | `false` | No |
| `tls.caFile` | string | CA certificate for a self-signed/privately-issued collector cert. Optional -- empty trusts the system CA pool. | `""` | No |
| `tls.certFile` | string | Client certificate for mTLS, if the collector requires client authentication. Optional; must be set together with keyFile. | `""` | No |
| `tls.enabled` | boolean | Use HTTPS for the OTLP/HTTP connection. False (default) uses plain HTTP, matching most in-cluster collectors. | `false` | No |
| `tls.keyFile` | string | Client private key for mTLS. Optional; must be set together with certFile. | `""` | No |

## tls

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `certManager.issuerRef.group` | string | API group of the issuer | `"cert-manager.io"` | No |
| `certManager.issuerRef.kind` | string | Kind of the issuer resource | `"ClusterIssuer"` | No |
| `certManager.issuerRef.name` | string | Name of the Issuer or ClusterIssuer. Usually left empty when tls.mode=cert-manager: auto-selected via `lookup` if exactly one exists in the cluster. Set explicitly for helm template/GitOps rendering or when multiple issuers exist. | `""` | No |
| `interService.caFile` | string | Path to the CA certificate for client-side trust | `"/etc/tls-ca/ca.crt"` | No |
| `interService.certDir` | string | Directory containing tls.crt and tls.key for server-side TLS | `"/etc/tls"` | No |
| `mode` | string | TLS mode: 'hook' (self-signed via Helm hooks), 'cert-manager' (production, requires cert-manager), or 'manual' (user-managed certificates) | `"hook"` | No |

## valkey

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `enabled` | boolean | Set false to skip in-chart Valkey and use external host/port instead | `true` | No |
| `existingSecret` | string | Pre-created Secret name with valkey-secrets.yaml key. Empty = expect 'valkey-secret'. | `""` | No |
| `host` | string | External Valkey host. Required when enabled=false. | `""` | No |
| `image` | string |  | `"valkey/valkey:8-alpine"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `port` | integer |  | `6379` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `storage.size` | string |  | `"512Mi"` | No |
| `storage.storageClassName` | string | Empty = cluster default | `""` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |

## workflowexecution

| Parameter | Type | Description | Default | Required |
|-----------|------|--------------|---------|----------|
| `affinity` | object | Kubernetes affinity rules | `` | No |
| `config.ansible.apiURL` | string | AWX/AAP API base URL (required when ansible is enabled) | `` | Yes |
| `config.ansible.caCertSecretRef.key` | string | Key within the Secret containing the PEM CA certificate | `"ca.crt"` | No |
| `config.ansible.caCertSecretRef.name` | string | Secret name | `` | Yes |
| `config.ansible.organizationID` | integer | AWX organization ID | `1` | No |
| `config.ansible.tokenSecretRef.key` | string | Key within the Secret | `"token"` | Yes |
| `config.ansible.tokenSecretRef.name` | string | Secret name | `` | Yes |
| `config.ansible.tokenSecretRef.namespace` | string | Namespace of the Secret (empty = release namespace) | `""` | No |
| `config.execution.cooldownPeriod` | string | Cooldown between workflow executions | `"1m"` | No |
| `config.tekton.enabled` | boolean | true or omit = auto-discover CRDs; false = disable Tekton engine | `` | No |
| `containerSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `fleet.oauth2.credentialsSecretRef` | string | REQUIRED (no fallback to global.fleet.oauth2.credentialsSecretRef) when global.fleet.oauth2.enabled=true. K8s Secret (keys: client-id, client-secret) for a write-scoped OAuth2 client, distinct from the read-only credential shared by gateway/remediationorchestrator/apifrontend/effectivenessmonitor/signalprocessing/fleetmetadatacache. | `""` | No |
| `logging.level` | string | Log level (Issue #875) | `"INFO"` | No |
| `nodeSelector` | map[string]object | Kubernetes node selector | `` | No |
| `pdb.enabled` | boolean |  | `true` | No |
| `pdb.maxUnavailable` | object |  | `` | No |
| `pdb.minAvailable` | object |  | `` | No |
| `podSecurityContext` | object | Kubernetes securityContext (pod or container level) | `` | No |
| `replicas` | integer |  | `1` | No |
| `resources.limits.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.limits.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.cpu` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `resources.requests.memory` | string | Kubernetes resource quantity (e.g., 256Mi, 100m) | `` | No |
| `tolerations` | array of object | Pod tolerations (overrides global.tolerations for this component) | `` | No |
| `topologySpreadConstraints` | array of object | Kubernetes topology spread constraints | `` | No |
| `workflowNamespace` | string | Namespace for Job/PipelineRun execution | `"kubernaut-workflows"` | No |

