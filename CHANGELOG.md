# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

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
