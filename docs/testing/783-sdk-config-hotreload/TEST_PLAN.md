# Test Plan: #783 SDK Config Hot-Reload

**Issue**: [#783](https://github.com/jordigilh/kubernaut/issues/783)
**Service**: Kubernaut Agent (KA)
**Date**: 2026-04-15
**Status**: Active

## Business Requirements

| BR ID | Description | Hot-Reload Relevance |
|-------|-------------|---------------------|
| BR-CFG-005 | Hot-reload configuration where safe | Primary driver: SDK LLM config must reload without pod restart |
| BR-PERF-008 | Hot-reload must apply within 10 seconds | FileWatcher debounce (200ms) + client build must complete within budget |
| BR-AI-029 | Zero-downtime policy updates | Model/endpoint changes must not interrupt in-flight investigations |

## Scope

### In Scope (Hot-Reloadable via ConfigMap Update)

| Field | Merge Semantic | Risk Level |
|-------|---------------|------------|
| `model` | Gap-fill | Low |
| `endpoint` | Gap-fill | Low |
| `api_key` | Gap-fill | Medium (credential rotation) |
| `vertex_project` | Gap-fill | Low |
| `vertex_location` | Gap-fill | Low |
| `bedrock_region` | Gap-fill | Low |
| `azure_api_version` | Gap-fill | Low |
| `custom_headers` | Override (SDK authoritative) | Medium |
| `oauth2.scopes` | Override (non-credential only) | Low |

### Out of Scope (Require Pod Restart)

| Field | Reason |
|-------|--------|
| `provider` | Changes entire construction path, transport stack, credential model |
| `oauth2.token_url` | Credential redirect attack surface (M5) |
| `oauth2.client_id` | Credential exfiltration risk |
| `oauth2.client_secret` | Credential exfiltration risk |
| `structured_output` | Requires prompt.Builder rebuild (scope expansion) |
| `temperature` | No runtime consumer today |
| `max_retries` | No runtime consumer today |
| `timeout_seconds` | No runtime consumer today |

## Test Scenario IDs

### Unit Tests

| ID | Description | Target File |
|----|-------------|-------------|
| UT-KA-783-SC-001 | SwappableClient.Chat delegates to inner | `swappable_client_test.go` |
| UT-KA-783-SC-002 | SwappableClient.Swap replaces inner atomically | `swappable_client_test.go` |
| UT-KA-783-SC-003 | SwappableClient.Swap calls Close on old client | `swappable_client_test.go` |
| UT-KA-783-SC-004 | SwappableClient.Swap does not block on slow Close | `swappable_client_test.go` |
| UT-KA-783-SC-005 | SwappableClient.Snapshot returns pinned client | `swappable_client_test.go` |
| UT-KA-783-SC-006 | SwappableClient.ModelName returns current model | `swappable_client_test.go` |
| UT-KA-783-SC-007 | SwappableClient.Close closes inner | `swappable_client_test.go` |
| UT-KA-783-SC-008 | SwappableClient.Swap(nil) rejected | `swappable_client_test.go` |
| UT-KA-783-SC-009 | NewSwappableClient(nil, "") rejected | `swappable_client_test.go` |
| UT-KA-783-SC-010 | Concurrent Chat+Swap no data race | `swappable_client_test.go` |
| UT-KA-783-SC-011 | Concurrent Snapshot+Swap returns valid client | `swappable_client_test.go` |
| UT-KA-783-SC-012 | Snapshot unaffected by subsequent Swap | `swappable_client_test.go` |
| UT-KA-783-CL-001 | Adapter.Close calls inner model Close if present | `close_test.go` |
| UT-KA-783-CL-002 | Adapter.Close no-op when model lacks Close | `close_test.go` |
| UT-KA-783-CL-003 | Adapter.Close calls closeFn if set | `close_test.go` |
| UT-KA-783-CL-004 | Adapter.Close idempotent (double close safe) | `close_test.go` |
| UT-KA-783-CL-005 | InstrumentedClient.Close delegates to inner | `close_test.go` |
| UT-KA-783-CL-006 | vertexanthropic.Client.Close is no-op | `close_test.go` |
| UT-KA-783-RC-001 | Reload rejects empty SDK content | `reload_callback_test.go` |
| UT-KA-783-RC-002 | Reload rejects whitespace-only SDK content | `reload_callback_test.go` |
| UT-KA-783-RC-003 | Reload rejects provider change | `reload_callback_test.go` |
| UT-KA-783-RC-004 | Reload rejects OAuth2 token_url change | `reload_callback_test.go` |
| UT-KA-783-RC-005 | Reload rejects OAuth2 client_id change | `reload_callback_test.go` |
| UT-KA-783-RC-006 | Reload rejects OAuth2 client_secret change | `reload_callback_test.go` |
| UT-KA-783-RC-007 | Reload accepts model change | `reload_callback_test.go` |
| UT-KA-783-RC-008 | Reload accepts endpoint change | `reload_callback_test.go` |
| UT-KA-783-RC-009 | Reload accepts api_key change | `reload_callback_test.go` |
| UT-KA-783-RC-010 | Reload accepts OAuth2 scopes change | `reload_callback_test.go` |
| UT-KA-783-RC-011 | Reload uses fresh config copy (never mutates live) | `reload_callback_test.go` |
| UT-KA-783-RC-012 | Reload rejects on Validate failure | `reload_callback_test.go` |
| UT-KA-783-RC-013 | Reload rejects on client build failure | `reload_callback_test.go` |
| UT-KA-783-RC-014 | Reload calls SwappableClient.Swap on success | `reload_callback_test.go` |
| UT-KA-783-RC-015 | Reload validates token_url https scheme | `reload_callback_test.go` |
| UT-KA-783-RC-016 | Reload emits slog on success | `reload_callback_test.go` |
| UT-KA-783-RC-017 | Reload emits slog on failure | `reload_callback_test.go` |
| UT-KA-783-RC-018 | Reload rejects structured_output change | `reload_callback_test.go` |

### Integration Tests

| ID | Description | Target File |
|----|-------------|-------------|
| IT-KA-783-001 | Full reload: file change -> swap -> new model in Chat | `sdk_hotreload_783_it_test.go` |
| IT-KA-783-002 | Concurrent investigation during swap uses pinned client | `sdk_hotreload_783_it_test.go` |
| IT-KA-783-003 | Rejection paths: provider/OAuth2/empty all rejected | `sdk_hotreload_783_it_test.go` |
| IT-KA-783-004 | Rapid successive reloads debounce correctly | `sdk_hotreload_783_it_test.go` |
| IT-KA-783-005 | Audit trail contains reload events | `sdk_hotreload_783_it_test.go` |

## Safety Invariants (Tested by Multiple Scenarios)

1. Live config is NEVER mutated before validation succeeds
2. Provider changes are ALWAYS rejected
3. OAuth2 credential changes are ALWAYS rejected
4. Empty/whitespace SDK content is ALWAYS rejected
5. In-flight investigations complete with pinned client
6. Old clients are explicitly closed after swap
7. Structured reload events are emitted for every attempt
