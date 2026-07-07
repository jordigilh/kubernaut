# BR-HAPI-199: ConfigMap Hot-Reload for HolmesGPT API

## Status
**⚠️ SUPERSEDED** — see note below
**Created**: 2025-12-06
**Last Updated**: 2026-07-06 (superseded)
**Implemented**: 2025-12-07 (for the Python "HolmesGPT API" PoC described below, not the current Go implementation)

---

> **SUPERSEDED (2026-07-06)**: This BR describes a Python-based "HolmesGPT API"
> service (`kubernaut-agent/poc/hot_reload_poc.py`, `watchdog` dependency,
> `DD-HAPI-004`) that was a pre-Go-rewrite proof of concept. It does not
> describe the current Kubernaut Agent (KA, `cmd/kubernautagent`), which is
> implemented in Go per [DD-HAPI-019](../architecture/decisions/DD-HAPI-019-go-rewrite-design).
>
> Specifically, the "Fields Supporting Hot-Reload" table below claiming
> `llm.model` and `llm.provider` are both hot-reloadable (✅) is **incorrect**
> for the current Go implementation: as of
> [#1599](https://github.com/jordigilh/kubernaut/issues/1599) /
> [DD-LLM-008](../architecture/decisions/DD-LLM-008-restart-required-llm-identity-lock.md),
> LLM identity (`provider` + `model`, at both base and per-phase-override level)
> requires a process restart to change — only `endpoint`, `apiKeyFile`,
> `temperature`, `maxRetries`, `timeoutSeconds`, and `customHeaders` are
> hot-reloadable. `llm.provider` was, in fact, never hot-reloadable in the Go
> implementation even before #1599 (KA's `LLMRuntimeConfig` has no `Provider`
> field at all).
>
> The authoritative source for KA's current hot-reload behavior is
> [`docs/services/kubernaut-agent/configuration-reference.md`](../services/kubernaut-agent/configuration-reference.md).
> This document is retained for historical reference only and MUST NOT be used
> to answer "is X hot-reloadable in KA" — recall/query tools should prefer the
> configuration-reference doc and DD-LLM-008 for that question.

## Business Context

### Problem Statement
HolmesGPT API requires pod restart for configuration changes, causing 2-5 minute downtime. This impacts:
- **LLM Cost Management**: Cannot quickly switch models during cost spikes
- **Operational Agility**: Config changes require full deployment cycle
- **Incident Response**: Cannot enable debug logging without restart

### Business Value
| Benefit | Current State | With Hot-Reload |
|---------|---------------|-----------------|
| Config change latency | 2-5 minutes | ~60 seconds |
| LLM model switching | Full restart | Zero downtime |
| Debug log enablement | Full restart | ~60 seconds |

---

## Requirements

### Functional Requirements

| ID | Requirement | Priority |
|----|-------------|----------|
| **FR-1** | Service SHALL reload configuration from ConfigMap without pod restart | P0 |
| **FR-2** | Service SHALL support hot-reload for LLM configuration (model, provider, endpoint) | P0 |
| **FR-3** | Service SHALL support hot-reload for toolset configuration | P0 |
| **FR-4** | Service SHALL support hot-reload for log level | P1 |
| **FR-5** | Service SHALL gracefully degrade on invalid configuration (keep previous) | P0 |
| **FR-6** | Service SHALL log configuration hash on reload for audit trail | P1 |

### Non-Functional Requirements

| ID | Requirement | Target |
|----|-------------|--------|
| **NFR-1** | Configuration reload latency | < 90 seconds (P99) |
| **NFR-2** | Memory overhead per watcher | < 100KB |
| **NFR-3** | Thread-safe configuration access | Required |

---

## Acceptance Criteria (Gherkin)

### Scenario 1: Successful Config Reload
```gherkin
Given HolmesGPT API is running with model "gpt-4"
When the ConfigMap is updated with model "claude-3-5-sonnet"
Then the service SHALL use "claude-3-5-sonnet" for new requests within 90 seconds
And the service SHALL log "Configuration hot-reloaded successfully"
And the service SHALL NOT restart
```

### Scenario 2: Invalid Config Graceful Degradation
```gherkin
Given HolmesGPT API is running with valid configuration
When the ConfigMap is updated with invalid YAML
Then the service SHALL log "Failed to apply new configuration - keeping previous"
And the service SHALL continue using the previous valid configuration
And the service SHALL NOT crash or restart
```

### Scenario 3: LLM Provider Failover
```gherkin
Given HolmesGPT API is configured with provider "openai"
And OpenAI is experiencing an outage
When the ConfigMap is updated with provider "vertex_ai"
Then the service SHALL use Vertex AI for new requests within 90 seconds
And the service SHALL NOT restart
```

### Scenario 4: Log Level Change
```gherkin
Given HolmesGPT API is running with log_level "INFO"
When the ConfigMap is updated with log_level "DEBUG"
Then the service SHALL emit DEBUG-level logs within 90 seconds
And the service SHALL NOT restart
```

### Scenario 5: Configuration Hash Audit
```gherkin
Given HolmesGPT API is running
When the ConfigMap is successfully reloaded
Then the service SHALL log the new configuration hash
And the metric holmesgpt_config_reload_total SHALL increment by 1
```

---

## Fields Supporting Hot-Reload

| Field Path | Type | Hot-Reload | Business Use Case |
|------------|------|------------|-------------------|
| `llm.model` | string | ✅ | Cost/quality switching |
| `llm.provider` | string | ✅ | Provider failover |
| `llm.endpoint` | string | ✅ | Endpoint switching |
| `llm.max_retries` | int | ✅ | Retry tuning |
| `llm.timeout_seconds` | int | ✅ | Timeout adjustment |
| `llm.temperature` | float | ✅ | Response tuning |
| `llm.max_tokens_per_request` | int | ✅ | Cost control |
| `toolsets.*` | object | ✅ | Toolset configuration |
| `log_level` | string | ✅ | Debug enablement |

---

## Out of Scope (Require Restart)

| Field | Reason |
|-------|--------|
| `api_host`, `api_port` | Server bind address |
| `auth_enabled` | Security-critical |
| `kubernetes.*` | Infrastructure |
| `DATA_STORAGE_URL` | Core dependency |

---

## Design Reference

**Design Decision**: [DD-HAPI-004: ConfigMap Hot-Reload](../../architecture/decisions/DD-HAPI-004-configmap-hotreload.md)

---

## Test Coverage Requirements

| Test Type | Count | Scope |
|-----------|-------|-------|
| Unit Tests | 8-10 | FileWatcher, ConfigManager |
| Integration Tests | 4-6 | Reload scenarios |
| E2E Tests | 2-3 | Kind cluster validation |

---

## Implementation Reference

| Artifact | Location |
|----------|----------|
| Design Decision | `docs/architecture/decisions/DD-HAPI-004-configmap-hotreload.md` |
| PoC Code | `kubernaut-agent/poc/hot_reload_poc.py` |
| Implementation Plan | TBD (pending approval) |

---

## Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| `watchdog` | >=3.0.0,<4.0.0 | File system events |

---

## Stakeholders

| Role | Interest |
|------|----------|
| **Operations** | Fast config changes, incident response |
| **Finance** | LLM cost control via model switching |
| **Development** | Debug log enablement |

---

## Timeline

| Phase | Target |
|-------|--------|
| Spec Approval | Pending |
| Implementation | V1.0 |
| Validation | E2E in Kind cluster |

---

**Maintained By**: HolmesGPT API Team

