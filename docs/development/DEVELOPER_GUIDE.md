# Developer Guide

## Prerequisites

- Go 1.25.6+
- Docker (for container builds)
- `controller-gen` (for CRD codegen)
- `ginkgo` (for test runner)

Optional:
- `kind` (for local Kubernetes cluster)
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

# Run locally (will fail without K8s cluster; use Kind for full workflow)
go run ./cmd/apifrontend/
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
  logging/                — logr/zap logger setup
  metrics/                — Prometheus registry (af_* prefix)
  prometheus/             — Prometheus HTTP client (alerts, rules, query)
  ratelimit/              — Per-IP and per-user rate limiters
  requestid/              — X-Request-ID middleware
  resilience/             — Circuit breakers, retry transport
  security/               — Error redaction, input sanitization
  session/                — CRD session service (InvestigationSession)
  severity/               — Multi-tier severity triage pipeline
  streaming/              — SSE connection tracker
  tlswiring/              — TLS configuration helpers (server + outbound)
  tools/                  — MCP tool implementations (6 AF-native + 14 kubernaut proxy)
  validate/               — K8s name/namespace/label validation
api/
  apifrontend/v1alpha1/   — CRD types (InvestigationSession)
  openapi/                — OpenAPI spec
deploy/
  kustomize/base/         — Kustomize base (Deployment, Service, RBAC, NetworkPolicy, PrometheusRule)
  kustomize/overlays/dev/ — Dev overlay (Kind, self-signed TLS, debug logging)
  kustomize/overlays/ci/  — CI overlay (GitHub Actions)
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

## Local Development with Kind

Deploy the API Frontend in a local Kubernetes cluster using [Kind](https://kind.sigs.k8s.io/):

```bash
# 1. Create the Kind cluster
make kind-create

# 2. Generate self-signed TLS certificates and create K8s secrets
make generate-dev-certs

# 3. Build the container image and load into Kind
make kind-load

# 4. Deploy using the dev overlay (Kustomize)
make deploy-dev

# 5. Verify the pod is running
kubectl get pods -n kubernaut-system

# 6. Port-forward to access locally
kubectl port-forward -n kubernaut-system svc/apifrontend 8443:8443

# Tear down
make undeploy
make kind-delete
```

The dev overlay provides:
- Debug-level logging
- Reduced resource limits (suitable for laptops)
- Self-signed TLS certificates via `generate-certs.sh`
- Kind port mappings (host 8443 → container 30443)

TLS secrets are optional in the dev overlay — the pod will start without them and serve plain HTTP.

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
| `validate-kustomize` | Validate kustomize build for dev/ci overlays |
| `kind-create` | Create a Kind cluster for development |
| `kind-delete` | Delete the Kind cluster |
| `kind-load` | Build and load image into Kind |
| `generate-dev-certs` | Generate self-signed TLS certificates |
| `deploy-dev` | Deploy to Kind using dev overlay |
| `deploy-ci` | Deploy to Kind using CI overlay |
| `undeploy` | Remove kustomize-managed resources |
| `sbom` | Generate CycloneDX SBOM |
| `image-scan` | Trivy image vulnerability scan |
| `image-build` | Build container image |
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
