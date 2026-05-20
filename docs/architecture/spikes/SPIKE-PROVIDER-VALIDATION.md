# Spike: OAS Go SDK Provider Validation

**Date**: May 20, 2026
**Status**: COMPLETED (research phase)
**Duration**: 1 session
**Relates to**: PROPOSAL-EXT-003 Addendum, DG-23

---

## Objective

Validate that the `open-agent-sdk-go` v0.5.0 SDK can work with the four LLM providers in Kubernaut's target matrix: Anthropic (direct), Google Vertex AI, Azure OpenAI, and AWS Bedrock.

## SDK Provider Model

The SDK supports two provider protocols:

1. **Anthropic API** (native): Auto-detected from API key prefix (`sk-ant-`) or model name containing `claude`/`sonnet`/`opus`/`haiku`. Uses the Anthropic Messages API.

2. **OpenAI-compatible API**: Any `BaseURL` that exposes `/v1/chat/completions`. Auto-detected from model names containing `gpt`/`o1`/`o3`, or explicitly via `BaseURL` setting.

The provider is selected at agent creation time via `agent.Options`. No provider-specific code paths exist in the SDK -- all differences are handled by the `BaseURL` + `APIKey` + `Model` tuple.

## Provider Compatibility Assessment

### 1. Anthropic (Direct API)

| Attribute | Status |
|---|---|
| SDK support | **Native** -- primary provider |
| Auth | `ANTHROPIC_API_KEY` env var or `Options.APIKey` |
| Streaming | Supported via Anthropic SSE format |
| Tool use | Native content block type (`tool_use`, `tool_result`) |
| Extended thinking | Supported (SDK's `ThinkingConfig`) |
| Kubernaut integration | `inference.local` forwards to Anthropic API with credential injection |

**Configuration:**
```go
agent.New(agent.Options{
    Model:  "sonnet-4-6",
    APIKey: "injected-by-inference-local",
})
```

**Verdict**: Works out of the box. No validation spike needed.

### 2. Google Vertex AI (Gemini)

| Attribute | Status |
|---|---|
| OpenAI-compat endpoint | **Yes** -- `https://{region}-aiplatform.googleapis.com/v1beta1/projects/{project}/locations/{location}/endpoints/openapi/chat/completions` | <!-- pre-commit:allow-sensitive -->
| Auth | OAuth2 bearer token (service account or workload identity) |
| Streaming | Supported via SSE |
| Tool use | Supported via OpenAI-compatible function calling |
| API key format | Bearer token, not static API key |

**Configuration:**
```go
agent.New(agent.Options{
    Model:   "gemini-2.0-flash",
    BaseURL: "https://us-central1-aiplatform.googleapis.com/v1beta1/projects/my-project/locations/us-central1/endpoints/openapi", // pre-commit:allow-sensitive
    APIKey:  "ya29.oauth2-bearer-token",
})
```

**Integration with inference.local**: OpenShell's `inference.local` privacy router would need to:
1. Accept calls to `inference.local` with a dummy/no auth header
2. Map to the Vertex AI endpoint URL
3. Inject a fresh OAuth2 bearer token (from service account key or workload identity)
4. Forward the request

**Risk**: Medium. The OAuth2 token refresh adds complexity to `inference.local` configuration. Static API keys are simpler but not available for Vertex AI.

**Validation needed**: End-to-end test with the SDK pointing `BaseURL` to a Vertex AI OpenAI-compat endpoint and confirming streaming + tool use work correctly.

### 3. Azure OpenAI

| Attribute | Status |
|---|---|
| OpenAI-compat endpoint | **Yes** -- `https://{resource}.openai.azure.com/openai/v1/` | <!-- pre-commit:allow-sensitive -->
| Auth | API key (`api-key` header) or Azure AD bearer token |
| Streaming | Supported via SSE |
| Tool use | Supported via OpenAI-compatible function calling |
| API version | Required query parameter (`api-version=2024-06-01` or later) |

**Configuration:**
```go
agent.New(agent.Options{
    Model:   "gpt-4o",
    BaseURL: "https://my-resource.openai.azure.com/openai/v1", // pre-commit:allow-sensitive
    APIKey:  "azure-api-key",
})
```

**Potential issue**: Azure OpenAI may require the `api-version` query parameter on all requests. The SDK's OpenAI-compatible client may not append this parameter. Two mitigations:
1. The `/openai/v1/` unified endpoint (available since 2024) may not require `api-version`
2. If needed, `inference.local` can inject the `api-version` parameter at the proxy layer

**Integration with inference.local**: Straightforward -- API key injection into `api-key` or `Authorization` header.

**Risk**: Low. The unified `/openai/v1/` endpoint is fully OpenAI-compatible.

**Validation needed**: Confirm the SDK's request format is accepted by Azure's unified endpoint without additional query parameters.

### 4. AWS Bedrock

| Attribute | Status |
|---|---|
| OpenAI-compat endpoint | **No native endpoint** -- Bedrock uses the Converse API (custom format) |
| Auth | AWS SigV4 (IAM credentials) |
| Streaming | Supported via Bedrock ConverseStream API (custom SSE format) |
| Tool use | Supported via Bedrock Converse API tool configuration |
| Proxy available | Yes -- `bedrock-access-gateway` (Go, Apache 2.0) provides OpenAI-compat `/v1/chat/completions` |

**Configuration (with proxy):**
```go
agent.New(agent.Options{
    Model:   "anthropic.claude-sonnet-4-v2:0",
    BaseURL: "http://bedrock-proxy.kubernaut-system.svc:8080",
    APIKey:  "bedrock-proxy-api-key",
})
```

**Integration with inference.local**: Bedrock requires a proxy layer (`bedrock-access-gateway` or custom) between the OpenAI-compatible format and Bedrock's Converse API. The proxy handles AWS SigV4 signing. `inference.local` can route to the proxy.

**Deployment**: The Bedrock proxy runs as a separate service (sidecar or standalone pod) with AWS IAM credentials (via IRSA or instance profile). The OAS Runtime never sees AWS credentials.

**Risk**: Medium. Adds an infrastructure dependency (Bedrock proxy). The proxy is a mature Go project but is an additional component to operate.

**Validation needed**: End-to-end test with `bedrock-access-gateway` proxying to Bedrock Converse API, confirming streaming and tool use work through the proxy.

## Summary

| Provider | OpenAI-Compat | Auth Model | Proxy Needed | Risk | Status |
|---|---|---|---|---|---|
| **Anthropic** | N/A (native) | API key | No | Low | Ready |
| **Vertex AI** | Yes (v1beta1) | OAuth2 bearer | No | Medium | Needs e2e test |
| **Azure OpenAI** | Yes (/openai/v1/) | API key or Azure AD | No | Low | Needs e2e test |
| **Bedrock** | No (needs proxy) | AWS SigV4 via proxy | Yes (`bedrock-access-gateway`) | Medium | Needs proxy + e2e test |

## Recommendations

1. **Prioritize Anthropic + Azure OpenAI** for v1.6 launch -- lowest risk, no proxy needed.
2. **Vertex AI** is viable but needs OAuth2 token refresh in `inference.local` configuration. Include in v1.6 if GCP is a target deployment platform.
3. **Bedrock** requires deploying `bedrock-access-gateway` as an additional service. Defer to v1.6.1 or make it a documented "bring your own proxy" option.
4. **End-to-end validation spikes** (DG-23) should be scheduled before v1.6 GA for each target provider.

## Integration with inference.local

All providers can be served through OpenShell's `inference.local` privacy router:

```
OAS Runtime → inference.local → [Anthropic API | Vertex AI | Azure OpenAI | Bedrock Proxy]
```

The `inference.local` configuration specifies the upstream provider endpoint and credentials. The OAS Runtime always calls `inference.local` with no credentials -- the router handles all provider-specific auth.

For providers requiring dynamic auth (Vertex AI OAuth2, Bedrock SigV4), the credential refresh logic lives in the `inference.local` configuration or in the proxy layer, never in the OAS Runtime.
