# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **#39** Config hot-reload via `FileWatcher` (fsnotify-based, SHA256 dedup, 200ms debounce)
  - Extended `Config` with `Auth`, `Logging`, `RateLimit`, `Shutdown` sections + validators
  - FedRAMP AU-2 audit events emitted on reload success/rejection
  - Bounded file reads via `io.LimitReader` (1 MiB max)
- **#40** OpenAPI 3.1 specification (`api/openapi/apifrontend-v1.yaml`) for all 6 HTTP endpoints
  - CI `validate-openapi` job using pinned `vacuum@v0.14.4`
  - `protocolVersion` field in Agent Card sourced from `a2a.Version` SDK constant
- **#41** SLO definitions (`docs/slo/SLO_DEFINITIONS.md`) with Prometheus alerting rules
  - 7 SLO targets (latency p95/p99, availability, error rate) aligned to `prometheus.DefBuckets`
  - `deploy/prometheus-rules.yaml` with warning (0.5%) and critical (1%) error rate tiers
- **#42** MCP Streamable HTTP protocol conformance tests (tools/list, error codes -32600/-32601/-32700)
- **#43** Performance test plan (`docs/testing/PERFORMANCE_TEST_PLAN.md`) and k6 script skeletons

- HTTP router with 6 endpoints:
  - `GET /healthz` — liveness probe (always 200)
  - `GET /readyz` — readiness probe (checks JWKS validator status)
  - `GET /metrics` — Prometheus metrics endpoint
  - `GET /.well-known/agent-card.json` — A2A agent card discovery
  - `POST /a2a/invoke` — A2A JSON-RPC task execution (authenticated)
  - `POST /mcp` — MCP Streamable HTTP handler (authenticated, feature-gated)
- HTTP metrics middleware: `af_http_requests_total` counter and `af_http_request_duration_seconds` histogram with path normalization to prevent label cardinality explosion
- Prometheus metric path normalization (`normalizePath`) for bounded cardinality
- `http.Flusher` support on `statusRecorder` for MCP streaming compatibility
- Audit event emission for A2A task lifecycle (`EventA2ATaskStarted`, `EventA2ATaskCompleted`, `EventA2ATaskFailed`) and MCP tool invocations (`EventMCPToolInvoked`)
- MCP feature gate: `mcp.enabled` config field controls tool stub exposure (returns 501 when disabled)
- `AllReady()` helper for composing multiple readiness checkers
- ConfigMap-based YAML configuration (`internal/config` package) with `--config` CLI flag
- Sample Kubernetes ConfigMap manifest (`deploy/configmap.yaml`)
- IEEE 829 test plan for configuration validation (`docs/tests/39/test_plan.md`)

### Changed

- **BREAKING:** Configuration is now loaded from a YAML file (ConfigMap mount) instead of environment variables. All `envOr()` calls and `os.Getenv` usage removed. See `docs/design/ARCHITECTURE.md` "Configuration" section for migration.

### Removed

- `envOr()` helper function and all environment variable-based configuration (`GCP_PROJECT`, `GCP_REGION`, `KA_BASE_URL`, `KA_MCP_ENDPOINT`, `DS_BASE_URL`, `PORT`, `ENABLE_MCP`, `AGENT_CARD_URL`)

### Migration Guide

If you previously configured the API Frontend via environment variables, create a config file:

```yaml
# /etc/apifrontend/config.yaml (or specify via --config flag)
server:
  port: 8443              # was: PORT
agent:
  gcpProject: "my-proj"   # was: GCP_PROJECT
  gcpRegion: "us-central1" # was: GCP_REGION
  kaBaseURL: "http://ka:8080" # was: KA_BASE_URL
  kaMCPEndpoint: "http://ka:8080/api/v1/mcp/" # was: KA_MCP_ENDPOINT
  dsBaseURL: "http://ds:9090" # was: DS_BASE_URL
mcp:
  enabled: true            # was: ENABLE_MCP=true
agentCard:
  url: "https://af.example.com" # was: AGENT_CARD_URL
```
