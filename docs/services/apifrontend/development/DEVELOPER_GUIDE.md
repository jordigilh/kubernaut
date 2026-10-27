# API Frontend — Developer Guide

## Prerequisites

- Go 1.26+
- Docker (for container builds)
- `controller-gen` (for CRD codegen)
- `ginkgo` (for test runner)

Optional:
- `kind` (for local Kubernetes cluster)
- `k6` (for performance test scripts)
- `syft` + `trivy` (for SBOM/scanning)
- `golangci-lint` v2.9.0+ (for linting)

## Quick Start

```bash
# Clone the monorepo
git clone https://github.com/jordigilh/kubernaut.git
cd kubernaut

# Build the apifrontend binary
make build-apifrontend

# Run unit tests
make test-unit-apifrontend

# Run locally (will fail without K8s cluster; use Kind for full workflow)
go run ./cmd/apifrontend/
```

## Project Structure

```
cmd/apifrontend/              — Application entrypoint and wiring
pkg/apifrontend/
  agent/                      — ADK root agent, RBAC roles, tool registration
  audit/                      — Audit event emitter (StoreAdapter → BufferedAuditStore → DS)
  auth/                       — JWT/OIDC validation, TokenReview fallback, SAR checker
  config/                     — YAML config loading, hot-reload FileWatcher
  ds/                         — DataStorage ogen client
  handler/                    — HTTP handlers (MCP bridge, Agent Card, router)
  httputil/                   — RFC 7807, IP extraction
  ka/                         — KA MCP SDK client (REST retained for health check only)
  launcher/                   — A2A JSON-RPC handler (ADK executor)
  logging/                    — logr/zap logger setup
  metrics/                    — Prometheus registry (af_* prefix)
  prometheus/                 — Prometheus HTTP client (alerts, rules, query)
  ratelimit/                  — Per-IP and per-user rate limiters
  requestid/                  — X-Request-ID middleware
  resilience/                 — Circuit breakers, retry transport, K8s CB
  security/                   — Error redaction, input sanitization
  session/                    — CRD session service (InvestigationSession)
  severity/                   — Multi-tier severity triage pipeline
  streaming/                  — SSE connection tracker
  tlswiring/                  — TLS configuration helpers (server + outbound)
  tools/                      — MCP tool implementations (6 AF-native + 14 kubernaut proxy)
  validate/                   — K8s name/namespace/label validation
internal/controller/apifrontend/ — Session TTL controller (controller-runtime)
api/
  apifrontend/v1alpha1/       — CRD Go types (InvestigationSession)
  apifrontend/openapi/        — OpenAPI spec (apifrontend-v1.yaml)
test/fixtures/kubernetes/apifrontend/
  base/                       — Test-owned RBAC fixture
  e2e/                        — E2E-only fixtures (Dex, mock-LLM, TLS, RBAC)
docs/services/apifrontend/    — ADRs, SLOs, runbooks, test plans, guides
hack/                         — Utility scripts
```

## Adding a New Tool

1. Define the tool in `pkg/apifrontend/tools/`:
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

2. Register it in `pkg/apifrontend/agent/root.go`:
   ```go
   tools = append(tools, toolspkg.NewMyTool(deps))
   ```

3. Add RBAC authorization by adding the tool name to the appropriate persona in
   `charts/kubernaut/values.yaml` under `apifrontend.config.rbac.personas`:
   ```yaml
   sre:
     - af_my_tool
   ```

4. Write tests in `pkg/apifrontend/tools/my_tool_test.go`

5. Update the Agent Card skills in `pkg/apifrontend/handler/agentcard.go`

## Local Development with Kind

The API Frontend is deployed as part of the operator-backed Kubernaut installation.
Follow the [operator quickstart](https://github.com/jordigilh/kubernaut-operator/blob/main/docs/installation/00-quickstart.md)
and the repository's [Interactive mode guide](../../../operations/deployment/INTERACTIVE_MODE_SETUP_GUIDE.md).

The files under `test/fixtures/kubernetes/apifrontend/e2e/` are test fixtures only;
they must not be applied as a standalone application deployment.

## Running Tests

```bash
# All unit tests with race detection and coverage
make test-unit-apifrontend

# Specific package
go test ./pkg/apifrontend/auth/ -v

# Integration tests (requires envtest/kubebuilder binaries)
make test-integration-apifrontend

# E2E tests (requires full Kind cluster with deps)
make test-e2e-apifrontend

# All test tiers
make test-all-apifrontend
```

## Makefile Reference

The Makefile uses pattern-based targets — replace `<service>` with `apifrontend`:

| Target | Description |
|--------|-------------|
| `build-apifrontend` | Build binary to `bin/apifrontend` |
| `test-unit-apifrontend` | Run Ginkgo unit tests with coverage |
| `test-integration-apifrontend` | Run integration tests (envtest) |
| `test-e2e-apifrontend` | Run E2E tests (Kind cluster) |
| `test-all-apifrontend` | Run all test tiers |
| `lint` | Run golangci-lint (monorepo-wide) |
| `manifests` | Generate CRDs, RBAC, webhook configs |
| `generate` | Run controller-gen + ogen code generation |
| `gen-diff` | Verify generated code is up-to-date (CI gate) |

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
- Logging: Structured `logr.Logger` (never stdlib `log` package)

## K8s Client Model (ADR-022)

All K8s API calls made by AF use the AF pod ServiceAccount. OIDC-direct mode
and impersonation have been removed. User access control is managed exclusively
at the MCP RBAC level (SAR-based tool authorization).

See [AUTHENTICATION_AND_RBAC.md](../security/AUTHENTICATION_AND_RBAC.md#3-k8s-client-model-unified-af-serviceaccount-adr-022)
for the full security design.

## Known Tech Debt (v1.5)

| Item | Target | Notes |
|------|--------|-------|
| Trivy CI step uses `continue-on-error: true` | v1.5.1 | Promote to required once Go stdlib CVEs are patched upstream |
| System prompt hardening (canary tokens) | v1.6 | Documented in `docs/security/prompt-injection-risk-assessment.md` |
| Output filtering / content safety layer | v1.6+ | Depends on model provider capabilities |
| ClusterRole grants `delete` on InvestigationSessions | v1.5.1 | Validate whether AF needs delete or only the operator does |
