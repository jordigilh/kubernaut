# Developer Guide

## Prerequisites

- Go 1.25.6+
- Docker (for container builds)
- `controller-gen` (for CRD codegen)
- `ginkgo` (for test runner)

Optional:
- `helm` (for chart linting)
- `k6` (for performance test scripts)
- `syft` + `trivy` (for SBOM/scanning)
- `golangci-lint` (for linting)

## Quick Start

```bash
# Clone
git clone https://github.com/jordigilh/kubernaut-apifrontend.git
cd kubernaut-apifrontend

# Build
make build

# Run unit tests
make test-unit

# Run with local config (will fail without K8s cluster)
go run ./cmd/apifrontend/ --config deploy/configmap-local.yaml
```

## Project Structure

```
cmd/apifrontend/          — Application entrypoint
internal/
  agent/                  — ADK root agent, RBAC roles, tool registration
  audit/                  — Audit event emitter (buffered → DS)
  auth/                   — JWT validation, middleware, context helpers
  config/                 — YAML config loading, hot-reload FileWatcher
  controller/             — Session TTL controller
  ds/                     — DataStorage ogen client
  handler/                — HTTP handlers (MCP, Agent Card, router)
  httputil/               — RFC 7807, IP extraction
  ka/                     — KA REST + MCP SDK client
  launcher/               — A2A JSON-RPC handler (ADK executor)
  logging/                — Zap + slog logger setup
  metrics/                — Prometheus registry
  ratelimit/              — Per-IP and per-user rate limiters
  requestid/              — X-Request-ID middleware
  resilience/             — Circuit breakers, retry transport
  security/               — Error redaction, input sanitization
  session/                — CRD session service (InvestigationSession)
  streaming/              — SSE connection tracker
  tools/                  — MCP tool implementations
api/
  apifrontend/v1alpha1/   — CRD types (InvestigationSession)
  openapi/                — OpenAPI spec
deploy/
  helm/                   — Helm chart (SA, ClusterRole, CRB)
  configmap.yaml          — Example ConfigMap
  prometheus-rules.yaml   — PrometheusRule CR
docs/                     — ADRs, SLOs, runbooks, guides
hack/                     — Utility scripts
tests/performance/        — k6 performance test scripts
```

## Adding a New Tool

1. Define the tool in `internal/tools/`:
   ```go
   func NewMyTool(deps MyToolDeps) *genai.Tool {
       return &genai.Tool{
           FunctionDeclarations: []*genai.FunctionDeclaration{{
               Name:        "af_my_tool",
               Description: "Does something useful",
               Parameters:  myToolSchema(),
           }},
       }
   }
   ```

2. Register it in `internal/agent/root.go`:
   ```go
   tools = append(tools, toolspkg.NewMyTool(deps))
   ```

3. Add RBAC authorization in `internal/agent/rbac_roles.yaml`:
   ```yaml
   sre:
     - af_my_tool
   ```

4. Write tests in `internal/tools/my_tool_test.go`

5. Update the Agent Card skills in `internal/handler/agentcard.go`

## Running Tests

```bash
# All unit tests with race detection and coverage
make test-unit

# Specific package
go test ./internal/auth/ -v

# Integration tests (requires cluster)
make test-integration

# Performance tests (dry-run, validates syntax)
make test-perf-local
```

## Makefile Reference

| Target | Description |
|--------|-------------|
| `build` | Build binary to `bin/apifrontend` |
| `test-unit` | Run Ginkgo unit tests with coverage |
| `test-integration` | Run integration tests |
| `lint` | Run golangci-lint |
| `coverage-report` | Generate HTML coverage report |
| `coverage-report-json` | Print per-function coverage |
| `test-perf-local` | Dry-run k6 performance scripts |
| `validate-maturity-ci` | Run service maturity checks |
| `validate-openapi` | Lint OpenAPI spec |
| `helm-lint` | Lint Helm chart |
| `sbom` | Generate CycloneDX SBOM |
| `image-scan` | Trivy image vulnerability scan |
| `docker-build` | Build container image |
| `generate` | Run controller-gen for CRD types |
| `verify-generate` | Verify generated code is up to date |

## Configuration Hot-Reload

The service watches its ConfigMap file for changes. When changes are detected:

1. File is re-read and parsed
2. New config is validated
3. Hot-reloadable fields are applied atomically:
   - `logging.level` → `zap.AtomicLevel.SetLevel()`
   - `rateLimit.*` → `limiter.SetLimit()` / `limiter.SetBurst()`
4. Non-reloadable field changes are logged but ignored (restart required)

## Code Conventions

- Metric names: `af_` prefix (namespace in Prometheus registry)
- Error responses: RFC 7807 via `httputil.WriteProblem()`
- Audit events: Always emit via `audit.Emitter` interface
- Context: Always propagate `context.Context` for cancellation
- Testing: Ginkgo/Gomega with `UT-AF-XXX-NNN` test IDs

## Known Tech Debt (v1.5)

| Item | Target | Notes |
|------|--------|-------|
| Trivy CI step uses `continue-on-error: true` | v1.5.1 | Promote to required once Go stdlib CVEs are patched upstream |
| System prompt hardening (canary tokens) | v1.6 | Documented in `docs/security/prompt-injection-risk-assessment.md` |
| Output filtering / content safety layer | v1.6+ | Depends on model provider capabilities |
| ClusterRole grants `delete` on InvestigationSessions | v1.5.1 | Validate whether AF needs delete or only the operator does |
