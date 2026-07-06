# Kubernaut Agent configuration reference

Authoritative mapping of kubernaut-agent configuration to YAML, runtime behavior, environment variables, and Helm values. Derived from:

- `internal/kubernautagent/config/config.go` (structs, defaults, `Validate`)
- `cmd/kubernautagent/main.go` (flags, wiring, transports)
- `cmd/kubernautagent/llm_builder.go` (LLM HTTP client and hot reload)
- `internal/kubernautagent/credentials/resolver.go` (credential file resolution)
- `pkg/kubernautagent/config` (custom header validation)
- `pkg/shared/tls` (TLS defaults and profiles)
- `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`

## 1. Overview

Configuration is split into two files plus optional CLI overrides:

| Layer | Reload | Purpose |
|-------|--------|---------|
| Static YAML (`Config`) | Requires process restart | Runtime, AI (except hot LLM knobs), integrations |
| LLM runtime YAML (`LLMRuntimeConfig`) | File watcher reload | Model, endpoint, API key placeholder, tuning, custom headers |

CLI flags only select paths or override the main HTTP listen socket; they do not replace the YAML surface.

Root YAML keys:

| Key | Go type | Purpose |
|-----|---------|---------|
| `runtime` | `RuntimeConfig` | Logging, servers, sessions, audit buffer |
| `ai` | `AIConfig` | Provider, investigation, summarizer, enrichment, alignment, safety |
| `integrations` | `IntegrationsConfig` | Data Storage, Prometheus tool, MCP (schema only) |

## 2. Configuration Files

### 2.1 Static configuration

| Item | Detail |
|------|--------|
| Flag | `-config` |
| Default path | `/etc/kubernautagent/config.yaml` |
| Format | YAML |
| Reload | Not hot-reloaded |

The Helm chart invokes the binary with `-config /etc/kubernaut-agent/config.yaml` (path differs from the binary default).

### 2.2 LLM runtime configuration

| Item | Detail |
|------|--------|
| Flag | `-llm-runtime` |
| Default path | `/etc/kubernautagent/llm-runtime.yaml` |
| Format | YAML |
| Reload | Hot reload via file watcher on the configured path |

Helm mounts the runtime file at `/etc/kubernaut-agent/llm-runtime/llm-runtime.yaml` and passes that path on the command line.

### 2.3 CLI flags

| Flag | Effect |
|------|--------|
| `-config PATH` | Static YAML path |
| `-llm-runtime PATH` | Hot-reloadable LLM YAML path |
| `-addr ADDRESS` | If non-empty, used as `http.Server.Addr` for the main API server (otherwise `runtime.server.address` + `:` + `runtime.server.port`). Help text refers to port override; the implementation supplies a full listen address string. |

## 3. Runtime configuration

YAML path: `runtime`

### 3.1 `runtime.logging`

Implements shared `internal/config.LoggingConfig`.

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `level` | string | `INFO` | If non-empty, must be `DEBUG`, `INFO`, `WARN`, or `ERROR` (case-insensitive input normalized for zap). Empty string passes validation and falls through to INFO when mapping to zap. | Log level for the process logger. |

### 3.2 `runtime.server`

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `address` | string | `0.0.0.0` | None | Bind address used with `port` when `-addr` is unset. |
| `port` | int | `8080` | `1`–`65535` | Main API port (with `address`) when `-addr` is unset. |
| `healthAddr` | string | `:8081` | None | Listen address for health/OpenAPI/admin endpoints (plain HTTP, not covered by server TLS). |
| `metricsAddr` | string | `:9090` | None | Prometheus metrics listen address. |
| `tls.certDir` | string | (empty) | None | If non-empty and `tls.crt` / `tls.key` exist under this directory, main API server uses TLS with cert hot-reload. Empty disables server TLS. |
| `tls.caFile` | string | (empty) | None | **Server block field**; not used for outbound trust in the agent’s current wiring. Prefer nested TLS under integrations for client trust. |
| `tlsProfile` | string | (empty) | Non-empty values must resolve via `SetDefaultSecurityProfileFromConfig` | Process-wide TLS profile for services using shared TLS helpers: `Old`, `Intermediate`, `Modern`. `Custom` and other unknown values return an error at startup; startup logs a fallback message and default TLS 1.2 behavior applies (profile not applied). |

### 3.3 `runtime.session`

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `ttl` | duration | `30m` | Must be positive | Session store TTL. |
| `maxConcurrentInvestigations` | int | `10` | Must be positive | Cap on concurrent investigations. |

### 3.4 `runtime.audit`

Buffered audit is implemented only when **both** `enabled` is true **and** `integrations.dataStorage.url` is non-empty. Otherwise a no-op store is used.

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `enabled` | bool | `true` | When true, `bufferSize` and `batchSize` must be positive | Master switch; without Data Storage URL, store is still no-op. |
| `endpoint` | string | (empty) | None | **Not referenced** by current kubernaut-agent wiring; audit client base URL is `integrations.dataStorage.url`. |
| `flushIntervalSeconds` | float64 | `0` | None | If `> 0`, passed as flush interval to the buffered store; otherwise library default applies. |
| `bufferSize` | int | `100` | Positive when `enabled` | Async buffer capacity. |
| `batchSize` | int | `10` | Positive when `enabled` | Batch size for writes. |
| `verbosity` | string | `full` | Must be `full`, `standard`, `minimal`, or empty (treated as allowed) | **Parsed and validated only**; kubernaut-agent does not pass this field into investigation or `pkg/audit` stores in the current codebase. |

## 4. AI configuration

YAML path: `ai`

### 4.1 `ai.llm` (static)

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `provider` | string | `openai` | Used with `LLMRuntimeConfig.Validate` | Provider id (e.g. `openai`, `anthropic`, `bedrock`, `vertex`, `vertex_ai`). |
| `azureApiVersion` | string | (empty) | None | Azure OpenAI API version when using Azure-backed LangChain adapter options. |
| `vertexProject` | string | (empty) | None | GCP project for Vertex-related providers. |
| `vertexLocation` | string | (empty) | None | GCP region for Vertex-related providers. |
| `bedrockRegion` | string | (empty) | None | AWS region for Bedrock. |
| `tlsCaFile` | string | (empty) | None | PEM CA for **LLM** HTTPS client trust when building a custom transport chain. Does not enable `TLS_CA_FILE` fallback for the LLM client. |
| `oauth2.enabled` | bool | `false` | If true, `tokenURL` and `credentialsDir` required | Client-credentials OAuth2 for enterprise LLM gateways. |
| `oauth2.tokenURL` | string | (empty) | Required if OAuth2 enabled | Token endpoint. |
| `oauth2.scopes` | []string | `nil` | None | Optional OAuth2 scopes. |
| `oauth2.credentialsDir` | string | (empty) | Required if OAuth2 enabled | Directory containing files `client-id` and `client-secret` (read at startup). |
| `oauth2.clientId` | — | resolved | Not in YAML (`yaml:"-"`) | Populated from `credentialsDir/client-id`. |
| `oauth2.clientSecret` | — | resolved | Not in YAML (`yaml:"-"`) | Populated from `credentialsDir/client-secret`. |
| `circuitBreaker.enabled` | bool | `false` | None | Enable gobreaker circuit breaker for LLM HTTP client. |
| `circuitBreaker.maxRequests` | uint32 | `3` | None | Requests allowed in half-open state before deciding to close or re-open. |
| `circuitBreaker.interval` | duration | `10s` | None | Cyclic period of closed state for clearing internal counts. `0` means never clear. |
| `circuitBreaker.timeout` | duration | `30s` | None | Duration the circuit stays open before transitioning to half-open. |
| `circuitBreaker.failureThreshold` | uint32 | `10` | None | Minimum request count before failure ratio is evaluated. |
| `circuitBreaker.failureRatio` | float64 | `0.5` | `0.0`–`1.0` | Failure ratio that triggers the circuit to open. |

### 4.2 `ai.investigation`

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `maxTurns` | int | `40` | Positive | Maximum investigator turns per session. |

### 4.3 `ai.summarizer`

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `threshold` | int | `8000` | Not validated for positivity | Tool output summarizer activation threshold (character-oriented). **`<= 0` disables the summarizer** at startup (not an error in `Validate()`). |
| `maxToolOutputSize` | int | `100000` | Positive | Hard cap on tool output before LLM context; constant `DefaultMaxToolOutputSize` in code. |

### 4.4 `ai.enrichment`

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `maxRetries` | int | `3` | `>= 0` | Retries for K8s owner-chain enrichment. |
| `baseBackoff` | duration | `1s` | None | Base backoff between enrichment retries. |

### 4.5 `ai.safety.sanitization`

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `injectionPatternsEnabled` | bool | `true` | None | Enables injection-pattern sanitization stage when building the pipeline. |
| `credentialScrubEnabled` | bool | `true` | None | Enables credential scrub stage. |

### 4.6 `ai.safety.anomaly`

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `maxToolCallsPerTool` | int | `10` | Positive | Per-tool call budget for anomaly detection. |
| `maxTotalToolCalls` | int | `30` | Positive | Total tool call budget. |
| `maxRepeatedFailures` | int | `3` | Positive | Repeated failure threshold. |
| `exemptPrefixes` | []string | `["todo_"]` | None | Tool name prefixes exempt from anomaly checks. |

## 5. LLM configuration

### 5.1 Provider-specific notes

| Provider strings | Endpoint required in runtime YAML? | Notes |
|------------------|-----------------------------------|--------|
| `bedrock`, `huggingface`, `anthropic`, `openai`, `vertex`, `vertex_ai` | No | Satisfies `LLMRuntimeConfig.Validate` without `endpoint`. |
| Any other provider | Yes | Validation error if `endpoint` empty. |

`vertex` uses the LangChain / Google AI style integration; `vertex_ai` selects the Anthropic-on-Vertex client path in code (`anthropicfamily`).

### 5.2 API key resolution (runtime)

When `LLMRuntimeConfig.apiKey` is empty after parsing YAML, startup and hot reload call `credentials.ResolveCredentialsFile(provider, "/etc/kubernaut-agent/credentials", logger)`:

| Provider | Preferred secret key file under credentials dir |
|----------|------------------------------------------------|
| `openai` | `OPENAI_API_KEY` |
| `anthropic` | `ANTHROPIC_API_KEY` |
| `mistral` | `MISTRAL_API_KEY` |
| `huggingface` | `HUGGINGFACEHUB_API_TOKEN` |
| `vertex`, `vertex_ai` | `GOOGLE_APPLICATION_CREDENTIALS` (JSON or path indirection; see resolver) |

If the preferred file is missing, the first non-empty file in the directory may be used as fallback.

## 6. LLM runtime configuration (hot-reloadable)

Top-level YAML (not nested under `runtime`/`ai` in file). Mapped by `LLMRuntimeConfig`.

| YAML key | Type | Default | Validation / behavior | Description |
|----------|------|---------|------------------------|-------------|
| `model` | string | (empty) | **Required** | Model id for the provider. |
| `endpoint` | string | (empty) | Required for non-standard providers (see section 5.1) | API base / proxy URL. |
| `apiKey` | string | (empty) | If empty, resolved from credential files (section 5.2) | Inline key material (sensitive). |
| `temperature` | float64 | `0` if omitted | None in `Validate` | Passed to `RuntimeParams`. **Omitted YAML fields decode as Go zero values** (`0`); they are **not** merged with `DefaultLLMRuntime()` in `LoadLLMRuntime`. |
| `maxRetries` | int | `0` if omitted | None in `Validate` | Retry attempts = `maxAttempts = 1 + maxRetries`; if `maxAttempts < 1` it is clamped to `1` in chat helper. |
| `timeoutSeconds` | int | `0` if omitted | None | If `<= 0`, per-chat wrapper does not add `context.WithTimeout` (relies on parent context). LangChain adapter may still use a **120s** default HTTP client timeout when a custom transport stack is built. |
| `customHeaders` | array | `nil` | Each entry validated; see below | Extra outbound headers on LLM HTTP stack. |

### 6.1 `customHeaders[]`

| Field | Type | Sources | Validation | Description |
|-------|------|---------|------------|-------------|
| `name` | string | Required | Must not be reserved (`content-type`, `accept`, `host`, `user-agent`) | Header name. |
| `value` | string | Mutually exclusive with others | Exactly one source | Inline static value (sensitive). |
| `secretKeyRef` | string | Mutually exclusive | Name of env var; must be **non-empty at process startup** when headers are validated | Reads `os.Getenv(secretKeyRef)` during `ValidateHeaderSources`. |
| `filePath` | string | Mutually exclusive | Exists at validation time only for structural checks in `ValidateSource`; file read occurs when transport issues requests | Reads header value from file at use time. |

Header names must be unique case-insensitively.

Reload failures leave the previously swapped client in place; reload rejects empty file content.

## 7. Alignment checker

YAML path: `ai.alignmentCheck`

When `enabled` is true and `llm` is nil, startup logs **error-level** diagnostics that shadow traffic shares the primary instrumented LLM client ( contention risk ). When `enabled` is true and dedicated shadow client creation fails (non-nil `ai.alignmentCheck.llm` configured but client build fails), the **process exits** (fail-closed).

| YAML key | Type | Default | Validation when enabled | Description |
|----------|------|---------|-------------------------|-------------|
| `enabled` | bool | `false` | — | Enables wrapper + evaluator. |
| `mode` | string | `enforce` | Must be `enforce` or `monitor` | Enforcement vs observability-only behavior. |
| `llm` | object (`LLMOverrideConfig`) | `null` | Shadow client validated at startup when non-nil overrides used | Partial override of primary static + runtime LLM for shadow only. See merge rules in code: `EffectiveLLM`. |
| `timeout` | duration | `10s` | Positive | Evaluator/step timeout budget. |
| `verdictTimeout` | duration | `30s` | Positive | Verdict aggregation timeout used by investigator wrapper. |
| `maxStepTokens` | int | `500` | Positive | Cap for alignment step payloads. |
| `maxRetries` | int | `1` | `>= 0` | Evaluator retries. |
| `canary.forceEscalation` | bool | `true` | None | Passed to investigator wrapper (`CanaryForceEscalation`). |

### 7.1 `ai.alignmentCheck.llm` overrides (`LLMOverrideConfig`)

| YAML key | Type | Default | Description |
|----------|------|---------|-------------|
| `provider` | string | (empty) | Overrides static provider when non-empty. |
| `endpoint` | string | (empty) | Overrides runtime endpoint when non-empty. |
| `model` | string | (empty) | Overrides runtime model when non-empty. |
| `apiKey` | string | (empty) | Overrides runtime apiKey when non-empty (sensitive). Does not bypass separate credential-dir resolution unless set in YAML/runtime merge. |
| `azureApiVersion` | string | (empty) | Overrides static Azure version when non-empty. |
| `vertexProject` | string | (empty) | Overrides static Vertex project when non-empty. |
| `vertexLocation` | string | (empty) | Overrides static Vertex location when non-empty. |
| `bedrockRegion` | string | (empty) | Overrides static Bedrock region when non-empty. |

Overrides do not duplicate `tlsCaFile` here; TLS trust stays on `ai.llm.tlsCaFile` for LangChain transports.

### 7.2 Enforcement modes and circuit breaker

The `mode` field controls how suspicious verdicts affect the investigation:

| Mode | Suspicious verdict behavior | Circuit breaker | Use case |
|------|-----------------------------|-----------------|----------|
| `enforce` | Sets `HumanReviewNeeded=true`, `HumanReviewReason="alignment_check_failed"`, appends warning. Circuit breaker cancels the primary LLM context mid-investigation when the first suspicious step is detected. | Active | Production environments where prompt injection must be blocked. |
| `monitor` | Logs verdict, emits audit events and Prometheus metrics. Investigation proceeds normally. | Inactive | Initial rollout, false-positive tuning, observability-only deployment. |

The alignment circuit breaker is distinct from the Data Storage circuit breaker (§8.1). The DS circuit breaker protects against backend failures using request-count thresholds, while the alignment circuit breaker is a per-investigation safety mechanism that cancels the primary LLM on detected prompt injection. The alignment circuit breaker has no configurable thresholds — it fires on the first suspicious evaluation in `enforce` mode.

For the complete shadow agent operational guide, see [shadow-agent-configuration.md](shadow-agent-configuration.md).

## 8. Integrations

YAML path: `integrations`

### 8.1 `integrations.dataStorage`

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `url` | string | (empty) | None | Data Storage OpenAPI base. **Empty disables** DS clients, workflow catalog fetching, DS-backed custom tools, and buffered audit (no-op audit). |
| `saTokenPath` | string | `/var/run/secrets/kubernetes.io/serviceaccount/token` | None | Bearer token file for DS client; audit client reuses token path logic. |
| `tls.caFile` | string | (empty) | None | If set, DS ogen client uses dedicated TLS transport with this CA. If unset, DS client uses `DefaultBaseTransportWithRetry()` which honors `TLS_CA_FILE`. |
| `circuitBreaker.enabled` | bool | `false` | None | Enable gobreaker circuit breaker for Data Storage HTTP client. |
| `circuitBreaker.maxRequests` | uint32 | `3` | None | Requests allowed in half-open state. |
| `circuitBreaker.interval` | duration | `10s` | None | Cyclic period for clearing closed-state counts. |
| `circuitBreaker.timeout` | duration | `30s` | None | Duration the circuit stays open before half-open probe. |
| `circuitBreaker.failureThreshold` | uint32 | `10` | None | Minimum requests before ratio check. |
| `circuitBreaker.failureRatio` | float64 | `0.5` | `0.0`–`1.0` | Failure ratio threshold. |

Nested `integrations.dataStorage.tls.certDir` parses but is **unused** by current DS outbound client construction (`buildDSBaseTransport` only reads `TLS.CAFile`).

### 8.2 `integrations.tools.prometheus`

Registration runs only when `url` non-empty.

| YAML key | Type | Default | Validation | Description |
|----------|------|---------|------------|-------------|
| `url` | string | (empty) | None | Prometheus / Thanos querier HTTP base for tools package. |
| `timeout` | duration | Client defaults to **30s** when `<= 0` | None before client | Passed into `promtools`; non-positive overridden inside `promtools.NewClient`. |
| `sizeLimit` | int | **`30000` when `<= 0`** | None before client | Max response slice size handled in client. |
| `tlsCaFile` | string | (empty) | None | When set, builds `NewTLSTransport` plus SA bearer transport for tool calls; when unset, `Transport` stays nil and `http.Client` uses default transport (**not** `TLS_CA_FILE` unless the default chain is patched elsewhere — it is plain `DefaultTransport`). |

### 8.3 `integrations.mcp`

| YAML path | Contents |
|-----------|----------|
| `integrations.mcp.servers` | List of `{ name, url, transport }` (`MCPServerEntry`) |

All fields strings. **Present in config schema only:** no reference to `integrations.mcp` appears in `cmd/kubernautagent` today; MCP tooling stubs live under `pkg/kubernautagent/tools/mcp/` but registration is not driven from agent config in the bundled binary paths found at time of writing.

## 9. Environment variables

| Variable | Effect |
|----------|--------|
| `TLS_CA_FILE` | When set to a PEM file path, `DefaultBaseTransport` / `DefaultBaseTransportWithRetry` load a reloadable CA pool. Used by the **audit** HTTP client base and **Data Storage** client base **when no** `integrations.dataStorage.tls.caFile`. **Not** consulted for LangChain LLM client when `ai.llm.tlsCaFile` empty (that path uses `http.DefaultTransport`). |
| Names referenced by `customHeaders[].secretKeyRef` | Required to be populated at startup for validation to pass. |

Kubernetes additionally injects standard ServiceAccount projection paths independent of YAML.

## 10. Helm mapping

Source: `charts/kubernaut/templates/kubernaut-agent/kubernaut-agent.yaml`.

### 10.1 Rendered ConfigMap (`config.yaml`)

| Helm value(s) | Config field |
|----------------|--------------|
| `kubernautAgent.logging.level` | `runtime.logging.level` |
| `kubernautAgent.llm.provider` | `ai.llm.provider` |
| `kubernautAgent.llm.oauth2.*` (+ secret mount) | `ai.llm.oauth2` (`credentialsDir` forced to `/etc/kubernaut-agent/oauth2`) |
| `kubernautAgent.llm.tlsCaFile` | `ai.llm.tlsCaFile` |
| `kubernautAgent.alignmentCheck.*` subset | Partial `ai.alignmentCheck`: `enabled`, `timeout`, `maxStepTokens`, optional nested `llm` |
| Include `kubernaut.datastorage.url` | `integrations.dataStorage.url` |
| Conditional TLS helpers | `integrations.dataStorage.tls.caFile` |

Helm-rendered snippets also emit fixed or defaulted values for `investigation.maxTurns`, audit buffer tuning, optional Prometheus tool URL (`kubernaut.monitoring.prometheus.url`), and TLS cert dir on the server — see the template for exact literals.

### 10.2 LLM runtime ConfigMap (`llm-runtime.yaml`)

Helm emits `model`, `endpoint`, `temperature`, `maxRetries`, `timeoutSeconds` from `kubernautAgent.llm`. Custom headers remain YAML-only unless the template or operators extend them.

### 10.3 Not exposed via primary Helm knobs (supply via extra ConfigMap patching or forks)

Including but not limited to: alignment `mode`, `maxRetries`, `verdictTimeout`, full `canary`, `runtime.audit.verbosity`, `runtime.audit.endpoint` (unused), `runtime.session.*` (chart hardcodes ports; session TTL/max concurrent not parameterized in template), `circuitBreaker.*` (both LLM and DS), finer `investigation`/ `summarizer`/`enrichment`/`safety`/`mcp` trees, Prometheus `timeout`/`sizeLimit`/`tlsCaFile`.

## 11. Sensitive fields

Store in Kubernetes **Secrets** (or equivalent vault-backed mounts), never in plain ConfigMaps:

| Location | Sensitive material |
|----------|-------------------|
| `LLMRuntimeConfig.apiKey` | API key inline |
| Alignment `LLMOverrideConfig.apiKey` | Shadow model key override |
| `oauth2.credentialsDir` files `client-id`, `client-secret` | OAuth2 client credentials |
| `customHeaders[].value`, `secretKeyRef` backing env, `filePath` targets | Bearer tokens / static secrets |
| `integrations.dataStorage.saTokenPath` file | ServiceAccount JWT |
| Helm `credentialsSecretName` volume (`/etc/kubernaut-agent/credentials`) | Provider key files referenced in section 5.2 |

## 12. Troubleshooting

| Symptom | Likely cause |
|---------|----------------|
| “DataStorage URL not configured” / no catalog / no DS tools | `integrations.dataStorage.url` empty or K8s clients unavailable (`ctrl.GetConfig()` failed). |
| Audit always no-op despite `audit.enabled: true` | Same as above: **`integrations.dataStorage.url` empty** skips buffered audit wiring. Audit transport also ignores `integrations.dataStorage.tls.caFile` and uses **`TLS_CA_FILE`** only via `DefaultBaseTransport`. Mis-matched DS TLS trust manifests as failed audit batches or TLS errors. |
| Prometheus tools missing | `integrations.tools.prometheus.url` empty. |
| LLM TLS verify failures with private CA | Set **`ai.llm.tlsCaFile`**. Expecting **`TLS_CA_FILE`** alone **does not** fix LLM outbound trust. |
| `vertex` vs `vertex_ai` wrong behavior | Provider selects different stacks (Gemini-focused LangChain path vs Claude-on-Anthropic Vertex SDK Path). Align provider string with the backend you deployed. |
| Summarizer never runs | `ai.summarizer.threshold <= 0` disables it without validation error. |
| Startup exit with alignment enabled | Process exits when `alignmentCheck` is enabled **and** a dedicated shadow LLM fails to construct when `alignmentCheck.llm` is configured; sharing primary client when `llm` nil does **not** exit. |
| Custom header validation fails | `secretKeyRef` env unset at startup or multiple of `value`/`secretKeyRef`/`filePath` set.

## Appendix A: TLS trust precedence (independent stacks)

Clients do **not** stack **`TLS_CA_FILE`** with YAML CA paths; each outbound client selects one path:

| Client | YAML CA respected | Else |
|--------|-------------------|------|
| LLM LangChain HTTP | `ai.llm.tlsCaFile` → custom pool | `http.DefaultTransport` |
| DS ogen (`buildDSBaseTransport`) | `integrations.dataStorage.tls.caFile` → custom pool | `DefaultBaseTransportWithRetry` → **`TLS_CA_FILE`** |
| Audit → DS (`buildAuditStore`) | **`integrations.dataStorage.tls.caFile` NOT used** (`DefaultBaseTransport` only) | **`TLS_CA_FILE`** if env set; else defaults |
| Prometheus tools | `integrations.tools.prometheus.tlsCaFile` → custom stack + SA bearer | `http.Transport` defaults via nil `Transport` |

## Appendix B: Known documentation errata

Cross-check lower-level drafts against this file:

| Source | Incorrect / stale claim | Correct per code |
|--------|-------------------------|------------------|
| `docs/operations/configuration/CONFIG_STANDARDS.md` | Example `session.ttl` default `10m` | Default `30m` (`DefaultConfig`) |
| `docs/operations/configuration/CONFIG_STANDARDS.md` | References `logging.format` | Field does **not exist** (`LoggingConfig` only has `level`) |
| `docs/operations/configuration/CONFIG_STANDARDS.md` | `LLM_API_KEY` environment variable shorthand | Agents resolve **`apiKey`** from YAML or **credential files under `/etc/kubernaut-agent/credentials`** (+ optional inline YAML); **`TLS_CA_FILE` and header `secretKeyRef` envs** documented in section 9 |

---

Treat this reference as authoritative for YAML keys, defaults from `internal/kubernautagent/config/config.go` `DefaultConfig` / validators, agent wiring from `cmd/kubernautagent`, and Helm template mapping.
