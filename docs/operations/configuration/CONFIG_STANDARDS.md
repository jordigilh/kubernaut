# Kubernaut Configuration Standards

**Version**: 1.3
**Last Updated**: 2026-04-03
**Status**: ✅ Authoritative Reference

---

## Overview

This document provides centralized visibility into all configuration options across Kubernaut services. Each service follows a consistent pattern:
- **Sane defaults** - Services run with minimal configuration
- **ConfigMap-based** - YAML structure in Kubernetes ConfigMap
- **Crash-if-missing** - Services crash at startup if required config/dependencies unavailable
- **Environment override** - Environment variables can override ConfigMap values

---

## Configuration Pattern

### Loading Priority

```
Environment Variables > ConfigMap > Defaults
```

### Standard ConfigMap Structure

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: <service>-config
  namespace: kubernaut-system
data:
  config.yaml: |
    # Service-specific configuration
```

### Startup Behavior

Services MUST crash at startup if:
- Required ConfigMap is missing (unless all values have defaults)
- Required dependencies are unavailable (e.g., Tekton, PostgreSQL, Redis)
- Configuration validation fails

---

## Service Configuration Matrix

### CRD Controllers

| Service | ConfigMap Name | Required Dependencies | Crash-if-Missing |
|---------|----------------|----------------------|------------------|
| **SignalProcessing** | `signalprocessing-config` | None | No (uses defaults) |
| **AIAnalysis** | `aianalysis-config` | Kubernaut Agent | Yes |
| **WorkflowExecution** | `workflowexecution-config` | Tekton Pipelines | Yes |
| **RemediationOrchestrator** | `remediationorchestrator-config` | None | No (uses defaults) |
| **Notification** | `notification-config` | None | No (uses defaults) |

### Stateless Services

| Service | ConfigMap Name | Required Dependencies | Crash-if-Missing |
|---------|----------------|----------------------|------------------|
| **Gateway** | `gateway-config` | Redis | Yes |
| **Kubernaut Agent** | `kubernaut-agent-config` | LLM Provider | Yes |
| **Data Storage** | `data-storage-config` | PostgreSQL | Yes |
| **Dynamic Toolset** | `dynamic-toolset-config` | None | No (uses defaults) |
| **Effectiveness Monitor** | `effectiveness-monitor-config` | Data Storage | Yes |

---

## Detailed Configuration by Service

### 1. Gateway Service

**ConfigMap**: `gateway-config`

```yaml
server:
  listen_addr: ":8080"              # Default: ":8080"
  read_timeout: 30s                 # Default: 30s
  write_timeout: 30s                # Default: 30s
  graceful_shutdown_timeout: 30s    # Default: 30s

redis:
  addr: "redis:6379"                # REQUIRED - no default
  password: ""                      # Use env var REDIS_PASSWORD
  db: 0                             # Default: 0
  pool_size: 100                    # Default: 100
  dial_timeout: 5s                  # Default: 5s

deduplication:
  ttl: 5m                           # Default: 5m

storm_detection:
  rate_threshold: 10                # Default: 10 alerts/minute
  pattern_threshold: 5              # Default: 5 similar alerts
  aggregation_window: 1m            # Default: 1m

logging:
  level: "info"                     # Default: "info"
  format: "json"                    # Default: "json"

metrics:
  enabled: true                     # Default: true
  listen_addr: ":9090"              # Default: ":9090"

health:
  listen_addr: ":8081"              # Default: ":8081"
```

---

### 2. Data Storage Service

**ConfigMap**: `data-storage-config`

```yaml
database:
  host: "postgres-service"          # REQUIRED - no default
  port: 5432                        # Default: 5432
  user: "kubernaut"                 # REQUIRED - no default
  password: ""                      # Use env var DB_PASSWORD
  name: "kubernaut"                 # Default: "kubernaut"
  max_connections: 50               # Default: 50
  ssl_mode: "disable"               # Default: "disable"

timeouts:
  query: 30s                        # Default: 30s
  write: 10s                        # Default: 10s
  context: 60s                      # Default: 60s

logging:
  level: "info"                     # Default: "info"
  format: "json"                    # Default: "json"

health:
  listen_addr: ":8081"              # Default: ":8081"

metrics:
  listen_addr: ":9090"              # Default: ":9090"
```

---

### 3. Kubernaut Agent Service

**ConfigMap**: `kubernaut-agent-config`

> **Note**: This service was renamed from HolmesGPT-API as part of #433 (Go rewrite).
> The Python HolmesGPT SDK dependency has been eliminated.

```yaml
llm:
  provider: "openai"                # REQUIRED: openai, ollama, azure, vertex, anthropic, bedrock, huggingface, mistral
  endpoint: "https://api.openai.com" # Provider-specific base URL (required for openai, ollama, azure, vertex, mistral; optional for anthropic; unused by bedrock, huggingface)
  model: "gpt-4"                    # Default: "gpt-4"
  api_key: ""                       # Use env var LLM_API_KEY (required for openai, azure, anthropic, huggingface, mistral; unused by bedrock, vertex, ollama)
  azure_api_version: ""             # Required when provider=azure (e.g. "2024-02-15-preview")
  vertex_project: ""                # Required when provider=vertex (GCP project ID)
  vertex_location: "us-central1"    # Default: "us-central1" (for provider=vertex)
  bedrock_region: ""                # Optional when provider=bedrock (overrides AWS_REGION env / SDK default credential chain)

data_storage:
  url: "http://data-storage:8080"   # REQUIRED - no default

investigator:
  max_turns: 15                     # Default: 15 — max LLM tool-call turns per investigation

sanitization:
  g4_credentials: true              # Default: true — G4 credential scrubbing
  i1_injection: true                # Default: true — I1 injection stripping

anomaly:
  max_tool_calls_per_tool: 10       # Default: 10 — I7 per-tool call limit (raised from 5, #860; pagination calls exempt)
  max_total_tool_calls: 30          # Default: 30 — I7 total tool call limit
  max_repeated_failures: 3          # Default: 3 — I7 consecutive failure limit

summarizer:
  threshold: 8000                   # Default: 8000 chars — above this, tool output is LLM-summarized

audit:
  enabled: true                     # Default: true — sends audit events to DataStorage

session:
  ttl: 10m                          # Default: 10m — async session expiry

logging:
  level: "info"                     # Default: "info"
  format: "json"                    # Default: "json"

health:
  listen_addr: ":8081"              # Default: ":8081"

metrics:
  listen_addr: ":9090"              # Default: ":9090"
```

#### Air-gapped / On-prem LLM Guidance

For environments without internet access (air-gapped, on-prem, classified networks), the recommended approach is:

- **Ollama** (`provider: "ollama"`): Run any GGUF model locally. Set `endpoint` to the Ollama server URL.
- **OpenAI-compatible** (`provider: "openai"`): Many on-prem inference servers (vLLM, LocalAI, TGI) expose an OpenAI-compatible API. Set `endpoint` to the server URL.

The LangChainGo `local` provider (subprocess execution) is **not supported** — it violates BR-HAPI-433 security requirements (no shell execution, no subprocess calls).

---

### 4. WorkflowExecution Controller

**ConfigMap**: `workflowexecution-config`

```yaml
tekton:
  # No config needed - uses cluster's Tekton installation
  # Controller crashes at startup if Tekton CRDs not found

resource_locking:
  cooldown_period: 5m               # Default: 5m

# DD-WE-005 v2.0: PipelineRun/Job ServiceAccount is NOT configured here.
# Operators pre-create per-workflow SAs and set serviceAccountName on the workflow schema
# (RemediationWorkflow → catalog → WorkflowExecution.spec.executionConfig).
# If unset, Kubernetes uses the execution namespace default ServiceAccount.

verification:
  enabled: false                    # Default: false (v1.0)
  policy_name: "require-signed-bundles"  # Default (if enabled)

logging:
  level: "info"                     # Default: "info"
  format: "json"                    # Default: "json"

health:
  listen_addr: ":8081"              # Default: ":8081"

metrics:
  listen_addr: ":9090"              # Default: ":9090"
```

---

### 5. AIAnalysis Controller

**ConfigMap**: `aianalysis-config`

```yaml
kubernaut_agent:
  url: "http://kubernaut-agent:8080"  # REQUIRED - no default
  timeout: 120s                     # Default: 120s

timeouts:
  analysis: 5m                      # Default: 5m
  approval: 24h                     # Default: 24h (if approval required)

logging:
  level: "info"                     # Default: "info"
  format: "json"                    # Default: "json"

health:
  listen_addr: ":8081"              # Default: ":8081"

metrics:
  listen_addr: ":9090"              # Default: ":9090"
```

---

### 6. RemediationOrchestrator Controller

**ConfigMap**: `remediationorchestrator-config`

```yaml
timeouts:
  overall_remediation: 60m          # Default: 60m
  phase_timeout: 15m                # Default: 15m per phase

retention:
  completed_crd: 24h                # Default: 24h
  failed_crd: 72h                   # Default: 72h

logging:
  level: "info"                     # Default: "info"
  format: "json"                    # Default: "json"

health:
  listen_addr: ":8081"              # Default: ":8081"

metrics:
  listen_addr: ":9090"              # Default: ":9090"
```

---

### 7. SignalProcessing Controller

**ConfigMap**: `signalprocessing-config`

```yaml
rego:
  policy_path: "/etc/kubernaut/policies/"  # Default

processing:
  label_extraction_timeout: 5s      # Default: 5s

logging:
  level: "info"                     # Default: "info"
  format: "json"                    # Default: "json"

health:
  listen_addr: ":8081"              # Default: ":8081"

metrics:
  listen_addr: ":9090"              # Default: ":9090"
```

---

### 8. Notification Controller

**ConfigMap**: `notification-config`

```yaml
channels:
  slack:
    enabled: false                  # Default: false
    webhook_url: ""                 # Use env var SLACK_WEBHOOK_URL
  pagerduty:
    enabled: false                  # Default: false
    api_key: ""                     # Use env var PAGERDUTY_API_KEY
  email:
    enabled: false                  # Default: false
    smtp_host: ""                   # Required if enabled

templates:
  path: "/etc/kubernaut/templates/" # Default

logging:
  level: "info"                     # Default: "info"
  format: "json"                    # Default: "json"

health:
  listen_addr: ":8081"              # Default: ":8081"

metrics:
  listen_addr: ":9090"              # Default: ":9090"
```

---

### 9. Dynamic Toolset Service

**ConfigMap**: `dynamic-toolset-config`

```yaml
discovery:
  interval: 30s                     # Default: 30s
  namespaces: []                    # Default: all namespaces

detectors:
  prometheus:
    enabled: true                   # Default: true
  grafana:
    enabled: false                  # Default: false

logging:
  level: "info"                     # Default: "info"
  format: "json"                    # Default: "json"

health:
  listen_addr: ":8081"              # Default: ":8081"

metrics:
  listen_addr: ":9090"              # Default: ":9090"
```

---

## Common Configuration Patterns

### Port Allocation (DD-TEST-001)

| Port | Purpose | Notes |
|------|---------|-------|
| **8080** | HTTP API (stateless services) | Main service port |
| **8081** | Health probes | `/healthz`, `/readyz` |
| **9090** | Metrics | Prometheus scraping |

### Logging Configuration

All services use the same logging structure:
```yaml
logging:
  level: "info"     # trace, debug, info, warn, error
  format: "json"    # json, text
```

### Health Check Configuration

All services expose health probes on port 8081:
```yaml
health:
  listen_addr: ":8081"
  # Endpoints: /healthz (liveness), /readyz (readiness)
```

---

## Environment Variable Overrides

| Pattern | Example |
|---------|---------|
| Flat structure | `LOG_LEVEL=debug` |
| Nested structure | `REDIS_ADDR=redis:6379` |
| Secrets | Use `valueFrom.secretKeyRef` in Deployment |

**Example**:
```yaml
env:
- name: LOG_LEVEL
  value: "debug"
- name: DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: postgres-credentials
      key: password
```

---

## Validation at Startup

Services MUST validate configuration at startup:

```go
func validateConfig(cfg *Config) error {
    // Required fields
    if cfg.Database.Host == "" {
        return fmt.Errorf("database.host is required")
    }

    // Dependency checks
    if !tektonCRDsExist() {
        return fmt.Errorf("Tekton Pipelines not installed")
    }

    return nil
}
```

---

## Document Maintenance

**Last Updated**: 2026-03-21
**Owner**: Platform Team

Update this document when:
- Adding new service configuration
- Changing defaults
- Adding new configuration options

---

