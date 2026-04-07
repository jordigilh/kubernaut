# DD-AUTH-015: Outbound LLM Authentication Transport Architecture

**Date**: April 6, 2026
**Status**: ✅ **APPROVED** - Authoritative Pattern
**Builds On**: DD-AUTH-005 (DataStorage Client Authentication Pattern — `http.RoundTripper` transport injection)
**Confidence**: 97%
**Version**: 1.1
**Decision Makers**: Architecture Team
**Affected Services**: Kubernaut Agent (KA)

---

## 📋 Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial design: two-mode outbound LLM authentication (custom headers + OAuth2 client credentials grant) |
| 1.1 | 2026-04-07 | Config separation: oauth2, custom_headers, structured_output moved exclusively to SDK ConfigMap; main config no longer sources transport fields (`LLMConfig` fields tagged `yaml:"-"`) |

---

## 🎯 **DECISION**

**Kubernaut Agent SHALL authenticate outbound LLM API requests using a composable `http.RoundTripper` transport chain. Two complementary authentication modes are supported — static/sidecar-managed custom headers and native OAuth2 client credentials grant — either independently or layered together.**

### Scope

- **Service**: Kubernaut Agent (`cmd/kubernautagent`)
- **Direction**: Outbound only (KA → LLM provider / enterprise gateway)
- **Protocol**: HTTPS (HTTP for testing)
- **Providers**: Any OpenAI-compatible, Anthropic, Bedrock, Mistral, Vertex, Ollama, or custom LLM endpoint behind an enterprise gateway

### Non-Scope

- Inbound API authentication for KA's own HTTP server (covered by DD-AUTH-014 SAR middleware)
- Inter-service authentication (covered by DD-AUTH-005)
- TLS certificate management (covered by Issue #493)

---

## 📊 **Context & Problem**

### Business Requirements

1. **Enterprise Gateway Authentication**: Production LLM endpoints sit behind API gateways (Azure APIM, Kong, Apigee, AWS API Gateway) or SSO-fronted proxies that require authentication headers.
2. **IdP Integration**: Enterprises use Keycloak, Azure AD, Okta, or other OAuth2 providers for machine-to-machine authentication. KA must acquire JWTs via the client credentials grant without human interaction.
3. **Provider Agnosticism**: The authentication mechanism must work with any LLM provider supported by LangChainGo — it operates at the HTTP transport layer, below the provider-specific SDK.
4. **Credential Safety**: API keys, JWTs, and client secrets must never appear in logs, metrics, error messages, or LLM-bound prompt content (DD-HAPI-019-003, G4).
5. **Zero Overhead When Disabled**: Deployments without enterprise gateways must not incur any authentication overhead.

### Problem Statement

LangChainGo backends (OpenAI, Anthropic, Ollama, etc.) accept an `http.Client` for outbound requests, but they do not support arbitrary header injection or OAuth2 client credentials natively. Enterprise deployments need to:

1. Inject static headers (`X-Api-Key`, `X-Tenant-Id`) provided via Helm values or K8s Secrets
2. Inject sidecar-rotated JWTs read from a file path (Vault Agent, cert-manager)
3. Acquire and refresh JWTs automatically from an enterprise IdP

No single mechanism covers all three. A composable transport chain is required.

---

## 🔍 **Alternatives Considered**

### Alternative A: oauth2-proxy Sidecar ❌ REJECTED

**Approach**: Deploy `oauth2-proxy` as a sidecar container. KA sends requests to localhost; the sidecar injects tokens before forwarding to the LLM gateway.

**Why Rejected**:
- `oauth2-proxy` supports **inbound** user authentication (authorization code flow, PKCE) but does NOT support the **outbound** client credentials grant for service-to-service token injection
- Verified via [oauth2-proxy GitHub issues](https://github.com/oauth2-proxy/oauth2-proxy/issues) — no client credentials support as of April 2026
- Adds operational complexity (sidecar lifecycle, health checks, resource overhead)
- Increases attack surface (localhost network hop)

### Alternative B: Custom JWT Middleware in KA ❌ REJECTED

**Approach**: Implement custom JWT acquisition, caching, and refresh logic in KA.

**Why Rejected**:
- Reinvents token lifecycle management that `golang.org/x/oauth2/clientcredentials` already provides
- Security risk: custom token caching may miss edge cases (clock skew, revocation, race conditions)
- Maintenance burden: 200+ lines of token logic vs. ~20 lines of wrapper code

### Alternative C: Provider SDK Auth ❌ REJECTED

**Approach**: Use each LLM provider's native SDK authentication (e.g., OpenAI client API key parameter).

**Why Rejected**:
- Only works for provider-specific API keys, not enterprise gateway auth
- Cannot inject arbitrary headers (tenant ID, correlation headers)
- Does not support OAuth2 flows
- Couples KA to provider-specific authentication mechanisms

### Alternative D: Composable RoundTripper Chain ✅ SELECTED

**Approach**: Build a chain of `http.RoundTripper` wrappers that compose inside-out. Each layer handles one concern:
1. **OAuth2 transport** (innermost auth): Acquires/refreshes JWTs from IdP
2. **Custom headers transport**: Injects static/sidecar-managed headers
3. **Structured output transport**: Mutates request body for provider-specific structured JSON

**Why Selected**:
- `http.RoundTripper` is the standard Go interface for HTTP client middleware — battle-tested, zero dependencies
- Composes naturally: each layer wraps the previous, standard Russian-doll pattern
- `golang.org/x/oauth2/clientcredentials` is maintained by the Go team, handles all edge cases (thread safety, clock skew, token caching)
- Already proven by DD-AUTH-005 for DataStorage client authentication
- ~20 lines of new wrapper code for OAuth2, ~50 lines for custom headers
- Zero overhead when no layers are configured — `buildTransportChain` returns nil, LangChainGo uses provider defaults

---

## 🎯 **Architecture**

### Transport Chain

```
┌──────────────────────────────────────────────────────────────┐
│                    LangChainGo Provider                       │
│                  (OpenAI, Anthropic, etc.)                    │
│                         │                                    │
│                  http.Client{Transport: chain}               │
│                         │                                    │
│  ┌──────────────────────▼──────────────────────────────────┐ │
│  │          StructuredOutputTransport (optional)           │ │
│  │          Mutates request body for JSON schema           │ │
│  ├─────────────────────────────────────────────────────────┤ │
│  │            AuthHeadersTransport (optional)              │ │
│  │     Injects custom headers from value/secret/file       │ │
│  ├─────────────────────────────────────────────────────────┤ │
│  │     OAuth2ClientCredentialsTransport (optional)         │ │
│  │     Acquires JWT from IdP, injects Authorization        │ │
│  ├─────────────────────────────────────────────────────────┤ │
│  │              http.DefaultTransport                      │ │
│  │              (actual TLS + HTTP/2 call)                 │ │
│  └─────────────────────────────────────────────────────────┘ │
│                         │                                    │
│                    LLM Gateway / Provider                     │
└──────────────────────────────────────────────────────────────┘
```

**Ordering rationale**:
- **OAuth2** is innermost auth layer because it manages the `Authorization` header automatically
- **Custom Headers** sits above OAuth2 so operators can **override** the `Authorization` header if needed (e.g., a gateway requires a different token format) or add supplementary headers (`X-Tenant-Id`, `X-Correlation-Id`)
- **Structured Output** is outermost because it mutates the request body, not headers — it must see the final request before it goes on the wire

### Authentication Mode 1: Static / Sidecar-Managed Custom Headers

| Aspect | Detail |
|--------|--------|
| **Use case** | API key injection, tenant headers, sidecar-rotated JWTs (Vault Agent, cert-manager) |
| **Token lifecycle** | External — KA reads the value; a sidecar or operator manages rotation |
| **Config location** | SDK config YAML (`llm.custom_headers`) |
| **Value sources** | `value` (literal), `secretKeyRef` (env var / K8s Secret), `filePath` (re-read per request) |
| **Implementation** | `pkg/kubernautagent/llm/transport/auth_headers.go` — `AuthHeadersTransport` |
| **Credential safety** | `secretKeyRef` and `filePath` values redacted in logs; `value` visible (non-sensitive by convention) |

**Helm configuration example**:

```yaml
kubernautAgent:
  sdkConfigContent: |
    llm:
      provider: "anthropic"
      model: "claude-sonnet-4-20250514"
      custom_headers:
        - name: "X-Api-Key"
          secretKeyRef: "LLM_API_KEY"     # from K8s Secret mounted as env
        - name: "X-Tenant-Id"
          value: "kubernaut-prod"          # literal, non-sensitive
        - name: "Authorization"
          filePath: "/var/run/secrets/jwt/token"  # sidecar-rotated JWT
```

### Authentication Mode 2: OAuth2 Client Credentials Grant (RFC 6749 s4.4)

| Aspect | Detail |
|--------|--------|
| **Use case** | Enterprise IdP integration (Keycloak, Azure AD, Okta) — machine-to-machine JWT authentication |
| **Token lifecycle** | Internal — KA acquires, caches, and refreshes JWTs via `golang.org/x/oauth2/clientcredentials` |
| **Config location** | SDK config YAML (`llm.oauth2`) + K8s Secret for credentials |
| **Implementation** | `pkg/kubernautagent/llm/transport/oauth2_credentials.go` — thin wrapper (~20 lines) around `oauth2.Transport` |
| **Token flow** | `POST token_url` with `grant_type=client_credentials` → receives `{"access_token":"<jwt>", "expires_in":N}` → injects `Authorization: Bearer <jwt>` |
| **Refresh** | Automatic — `oauth2.ReuseTokenSource` refreshes before expiry, thread-safe |

**Helm configuration example**:

```yaml
kubernautAgent:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    oauth2:
      enabled: true
      tokenURL: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
      credentialsSecretRef: "llm-oauth2-credentials"  # Secret with keys: client-id, client-secret
      scopes: "openid llm-gateway"
```

**K8s Secret**:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: llm-oauth2-credentials
type: Opaque
stringData:
  client-id: "kubernaut-agent"
  client-secret: "s3cret-value-from-idp"
```

### Composability: Both Modes Together

Both modes compose in the transport chain. When both are enabled:

1. OAuth2 transport acquires JWT, injects `Authorization: Bearer <jwt>`
2. Custom headers transport injects additional headers (or overrides `Authorization` if configured)
3. LLM gateway receives all headers

This enables scenarios like:
- OAuth2 for base authentication + `X-Tenant-Id` custom header for multi-tenancy
- OAuth2 for gateway auth + `X-Api-Key` custom header for the underlying LLM provider

---

## 🔒 **Security Properties**

### Credential Storage

| Credential | Storage | Access Method |
|-----------|---------|--------------|
| `client_id` | K8s Secret (`credentialsSecretRef`) | Env var `OAUTH2_CLIENT_ID` projected by Helm |
| `client_secret` | K8s Secret (`credentialsSecretRef`) | Env var `OAUTH2_CLIENT_SECRET` projected by Helm |
| `token_url`, `scopes` | ConfigMap (`kubernaut-agent-sdk-config`) | SDK config YAML (non-sensitive) |
| Custom header `secretKeyRef` | K8s Secret (LLM credentials) | Env var or mounted file |
| Custom header `filePath` | Pod filesystem | Sidecar-written file, re-read per request |

### Credential Scrubbing (DD-HAPI-019-003, G4)

- OAuth2 `client_secret` never appears in config YAML or logs — projected from Secret as env var
- Custom header values from `secretKeyRef` and `filePath` sources are redacted as `[REDACTED]` in structured logs
- Custom header values from `value` source are not redacted (non-sensitive by convention)
- JWTs acquired by OAuth2 transport are managed entirely within `oauth2.Transport` — never logged by KA

### Token Lifecycle Security (OAuth2)

| Property | Mechanism |
|----------|-----------|
| **Acquisition** | `golang.org/x/oauth2/clientcredentials.Config.TokenSource()` — standard POST with `grant_type=client_credentials` |
| **Caching** | `oauth2.ReuseTokenSource` — in-memory, single copy, garbage-collected with the transport |
| **Refresh** | Automatic before expiry — `oauth2.Token.Valid()` checks `Expiry` with 10s buffer |
| **Thread safety** | `oauth2.reuseTokenSource` uses `sync.Mutex` — safe for concurrent goroutines |
| **Clock skew** | 10s expiry buffer built into `golang.org/x/oauth2` |
| **Revocation** | Not handled — KA trusts the IdP-issued token; validation is the gateway's responsibility |

---

## 📁 **Implementation Files**

| File | Purpose | Lines |
|------|---------|-------|
| `pkg/kubernautagent/llm/transport/auth_headers.go` | Custom headers `RoundTripper` — resolves values from 3 sources, injects into request | ~95 |
| `pkg/kubernautagent/llm/transport/oauth2_credentials.go` | OAuth2 client credentials `RoundTripper` — thin wrapper around `oauth2.Transport` | ~57 |
| `pkg/kubernautagent/llm/transport/structured_output.go` | Structured JSON output `RoundTripper` (outermost layer) | ~115 |
| `pkg/kubernautagent/llm/transport/resolver.go` | Header value resolver — `value`, `secretKeyRef`, `filePath` | ~80 |
| `pkg/kubernautagent/llm/transport/scrub.go` | Credential redaction for log output | ~30 |
| `internal/kubernautagent/config/config.go` | `OAuth2Config` struct, `SDKConfig` with transport fields, `MergeSDKConfig()` merge, `Validate()` rules. Transport fields in `LLMConfig` tagged `yaml:"-"` (sourced exclusively from SDK) | ~40 (additions) |
| `pkg/kubernautagent/config/headers.go` | `HeaderDefinition` struct, custom header config parsing | ~60 |
| `cmd/kubernautagent/main.go` | `buildTransportChain()` — composes the chain; env var override for OAuth2 creds | ~35 |

### Transport Chain Assembly (`buildTransportChain`)

```go
func buildTransportChain(cfg *kaconfig.Config) http.RoundTripper {
    var base http.RoundTripper = http.DefaultTransport

    // Layer 1 (innermost auth): OAuth2 client credentials
    if cfg.LLM.OAuth2.Enabled {
        base = llmtransport.NewOAuth2ClientCredentialsTransport(cfg.LLM.OAuth2, base)
    }

    // Layer 2: Static/sidecar custom headers (can override OAuth2 Authorization)
    if len(cfg.LLM.CustomHeaders) > 0 {
        base = llmtransport.NewAuthHeadersTransport(cfg.LLM.CustomHeaders, base)
    }

    // Layer 3 (outermost): Structured output body mutation
    if cfg.LLM.StructuredOutput {
        base = llmtransport.NewStructuredOutputTransport(
            parser.InvestigationResultSchema(), base,
        )
    }

    // nil = no custom transport needed, LangChainGo uses provider defaults
    return base
}
```

---

## 🧪 **Validation**

### Test Plan

Covered by **TP-417 v1.2** (`docs/tests/417/TEST_PLAN.md`):

| Tier | Custom Headers | OAuth2 | Total |
|------|---------------|--------|-------|
| Unit | 17 | 9 | 26 |
| Integration | 6 | 3 | 9 |
| **Total** | **23** | **12** | **35** |

### Key Test Scenarios

| ID | What it validates |
|----|-------------------|
| UT-KA-417-001 | All configured headers injected into outbound request |
| UT-KA-417-006 | `filePath` re-reads on token rotation (no caching) |
| UT-KA-417-009 | Sensitive header values redacted in logs |
| UT-KA-417-016 | Original `*http.Request` not mutated (RoundTripper contract) |
| UT-KA-417-020..025 | OAuth2Config parsing and validation |
| UT-KA-417-026..028 | OAuth2 transport construction and chain ordering |
| IT-KA-417-010 | Full round trip: OAuth2 token acquisition from mock IdP |
| IT-KA-417-011 | Automatic token refresh after 1s expiry |
| IT-KA-417-012 | Actionable error when IdP is unreachable |

### Helm Validation

| Scenario | Expected |
|----------|----------|
| OAuth2 disabled (default) | No OAuth2 config rendered, no env vars, no overhead |
| OAuth2 enabled with all fields | SDK ConfigMap has `token_url`/`scopes` under `llm.oauth2`, container has `OAUTH2_CLIENT_ID`/`OAUTH2_CLIENT_SECRET` env vars from Secret |
| OAuth2 enabled, missing `tokenURL` | `helm template` fails with `"tokenURL is required when oauth2.enabled=true"` |
| OAuth2 enabled, missing `credentialsSecretRef` | `helm template` fails with `"credentialsSecretRef is required when oauth2.enabled=true"` |

---

## 📐 **Helm Configuration Reference**

### Minimal: No Auth (Default)

```yaml
kubernautAgent:
  llm:
    provider: "openai"
    model: "gpt-4o"
```

No transport chain assembled. LangChainGo uses its default `http.Client`. Provider API key comes from `credentialsSecretName`.

### Static API Key via Custom Header

```yaml
kubernautAgent:
  sdkConfigContent: |
    llm:
      provider: "openai"
      model: "gpt-4o"
      custom_headers:
        - name: "X-Api-Key"
          secretKeyRef: "GATEWAY_API_KEY"
```

### Sidecar-Rotated JWT via filePath

```yaml
kubernautAgent:
  sdkConfigContent: |
    llm:
      provider: "anthropic"
      model: "claude-sonnet-4-20250514"
      custom_headers:
        - name: "Authorization"
          filePath: "/var/run/secrets/jwt/token"
```

Sidecar (Vault Agent, cert-manager) writes the token file. KA re-reads on every request.

### OAuth2 Client Credentials (Keycloak / Azure AD / Okta)

```yaml
kubernautAgent:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    oauth2:
      enabled: true
      tokenURL: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
      credentialsSecretRef: "llm-oauth2-credentials"
      scopes: "openid llm-gateway"
```

### OAuth2 + Custom Headers (Multi-Tenancy)

```yaml
kubernautAgent:
  llm:
    provider: "anthropic"
    model: "claude-sonnet-4-20250514"
    oauth2:
      enabled: true
      tokenURL: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
      credentialsSecretRef: "llm-oauth2-credentials"
      scopes: "openid"
  sdkConfigContent: |
    llm:
      provider: "anthropic"
      model: "claude-sonnet-4-20250514"
      oauth2:
        enabled: true
        token_url: "https://keycloak.acme.com/realms/infra/protocol/openid-connect/token"
        scopes:
          - "openid"
      custom_headers:
        - name: "X-Tenant-Id"
          value: "kubernaut-prod"
        - name: "X-Correlation-Id"
          value: "kubernaut-remediation"
```

Note: When using `sdkConfigContent`, OAuth2 config must be included in the SDK YAML (the chart does not inject it). The `kubernautAgent.llm.oauth2` Helm values still control the fail guards and Secret env projection.

OAuth2 injects `Authorization: Bearer <jwt>`. Custom headers add `X-Tenant-Id` and `X-Correlation-Id`. All three reach the LLM gateway.

---

## 🔄 **Design Review: Per-LLM Authentication Scoping — RESOLVED (v1.1)**

**Resolved in v1.1**: All LLM transport fields (`oauth2`, `custom_headers`, `structured_output`) are now sourced exclusively from the SDK config (`kubernaut-agent-sdk-config` ConfigMap). The main config (`kubernaut-agent-config`) no longer carries any `llm` section. This was achieved by tagging the three `LLMConfig` fields with `yaml:"-"` (ignored during main config deserialization) and extending `SDKConfig` + `MergeSDKConfig()` to unconditionally copy them from the SDK config.

**Single-LLM design retained (YAGNI)**: The SDK config currently supports one LLM definition. When multi-provider support is required (e.g., primary + fallback with different IdPs), the `SDKConfig.LLM` struct can evolve to a named map (`llms.primary`, `llms.fallback`) with per-provider authentication context. No structural changes are needed until that requirement arrives.

### Helm Upgrade Migration Note

Operators upgrading from v1.0 to v1.1:

- **Chart-generated SDK config** (`llm.provider` + `llm.model` quickstart path): No action needed. The chart renders `oauth2` and `structured_output` into the SDK ConfigMap automatically.
- **`sdkConfigContent` path** (`--set-file`): Operators must add `llm.oauth2` and `llm.structured_output` to their custom SDK YAML. These fields are no longer read from the main config.
- **`existingSdkConfigMap` path**: Operators must add the fields to their externally managed ConfigMap.

---

## 📚 **Related Documents**

| Document | Relationship |
|----------|-------------|
| [DD-AUTH-005](./DD-AUTH-005-datastorage-client-authentication-pattern.md) | Established the `http.RoundTripper` transport injection pattern — this DD extends it to outbound LLM calls |
| [DD-AUTH-014](./DD-AUTH-014-middleware-based-sar-authentication.md) | Inbound SAR/TokenReview middleware for KA's own HTTP server (orthogonal) |
| [DD-HAPI-019-003](./DD-HAPI-019-go-rewrite-design/DD-HAPI-019-003-security-architecture.md) | Prompt injection security + credential scrubbing (G4) |
| [TP-417 v1.2](../../tests/417/TEST_PLAN.md) | Test plan with 35 tests covering both authentication modes |
| Issue #417 | Feature issue: custom authentication headers for LLM proxy endpoints |
| RFC 6749 s4.4 | OAuth2 client credentials grant specification |

---

## ✅ **Acceptance Criteria**

1. Both authentication modes work independently and composed together
2. OAuth2 token acquisition uses the client credentials grant (verified by IT-KA-417-010)
3. OAuth2 tokens are refreshed automatically before expiry (verified by IT-KA-417-011)
4. OAuth2 credentials (`client_id`, `client_secret`) are stored in K8s Secrets, never in ConfigMaps
5. All sensitive header values are redacted in log output (verified by UT-KA-417-009)
6. Zero overhead when no authentication is configured — `buildTransportChain` returns nil
7. Helm chart validates required fields and fails with actionable errors
8. 35 tests pass (26 unit + 9 integration) at >=80% per-tier coverage
