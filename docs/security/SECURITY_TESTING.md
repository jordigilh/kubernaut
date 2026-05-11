# Security Testing Pipeline

**Service:** kubernaut-apifrontend
**NIST Controls:** SA-11 (Developer Testing and Evaluation), SI-2 (Flaw Remediation), RA-5 (Vulnerability Monitoring and Scanning)
**Source of truth:** `Makefile`, `hack/validate-maturity.sh`, test suite, CI configuration
**Last updated:** 2026-05-08

---

## 1. Testing Pyramid

```
                ┌─────────────────┐
                │   Performance   │  k6 scripts (4 profiles)
                │   (deferred)    │  docs/testing/PERFORMANCE_TEST_PLAN.md
                └────────┬────────┘
               ┌─────────┴─────────┐
               │   Integration     │  -tags=integration, cross-package
               │   (test-integration) │  E2E flows with fakes
               └─────────┬─────────┘
        ┌────────────────┴────────────────┐
        │           Unit Tests            │  77 test files, Ginkgo + Gomega
        │          (test-unit)            │  ~354 specs, per-package isolation
        └─────────────────────────────────┘
```

### Test File Distribution

| Package | Test Files | Coverage Focus |
|---------|-----------|---------------|
| `internal/tools/` | 22 | Tool behavior, input validation, K8s client mocking |
| `internal/auth/` | 5 | JWT validation, RBAC, impersonation, delegation |
| `internal/handler/` | 6 | HTTP routing, MCP conformance, agent card |
| `internal/session/` | 6 | Session lifecycle, decorator, concurrency, state machine |
| `internal/resilience/` | 4 | Circuit breaker, retry, K8s dynamic client |
| `internal/launcher/` | 5 | A2A conformance, error handling, model mapping |
| `internal/ka/` | 5 | KA REST client, MCP SDK client |
| `internal/metrics/` | 2 | Prometheus registration, recording rules |
| `internal/audit/` | 2 | Emitter, buffered emitter behavior |
| `internal/config/` | 2 | Config parsing, hot-reload |
| Others | 18 | httputil, logging, requestid, security, streaming, controller, ds |

---

## 2. Static Analysis

All static analysis gates run before tests in the CI pipeline and block merge on failure.

| Tool | Makefile Target | What It Catches |
|------|----------------|-----------------|
| `go vet` | `make vet` | Suspicious constructs, unreachable code, incorrect format strings |
| `go fmt` | `make fmt` | Non-canonical formatting (enforces `gofmt` style) |
| `golangci-lint` | `make lint` | 50+ linters including `errcheck`, `gocritic`, `gosec`, `staticcheck` |
| Race detector | `--race` flag in `make test-unit` | Data races in concurrent code |
| OpenAPI validation | `make validate-openapi` | Schema conformance via `vacuum lint` |
| Kustomize validation | `make validate-kustomize` | Kustomize build errors, missing resources |
| Maturity validation | `make validate-maturity-ci` | Service maturity criteria (8 checks) |

### Maturity Validation Checks (`hack/validate-maturity.sh`)

| # | Check | What It Verifies |
|---|-------|------------------|
| 1 | Prometheus metrics namespace | All metrics use `af_` namespace |
| 2 | Health endpoint `/healthz` | Liveness probe registered |
| 3 | Readiness endpoint `/readyz` | Readiness probe registered |
| 4 | Graceful shutdown | Signal handling present |
| 5 | RFC 7807 errors | Problem responses implemented |
| 6 | Audit trail | `audit.Emitter` / `audit.Event` usage |
| 7 | Structured logging | slog or zap configured |
| 8 | Circuit breaker | `gobreaker` in resilience package |

---

## 3. Dependency Scanning

| Tool | Makefile Target | Output | Severity Gate |
|------|----------------|--------|---------------|
| **Trivy** | `make image-scan` | Console report | Exit code 1 on CRITICAL or HIGH CVEs |
| **Syft** | `make sbom` | `sbom.cdx.json` (CycloneDX) | N/A (artifact generation) |
| **Go module audit** | `go mod verify` | — | Fails on tampered checksums |

### Trivy Configuration

- Scans the container image (`quay.io/kubernaut/apifrontend:latest`)
- Severity filter: `CRITICAL,HIGH` only
- `.trivyignore` file for accepted risk suppressions (each entry requires justification comment)
- Exit code 1 fails the CI pipeline

### SBOM Generation

- Format: CycloneDX JSON (`sbom.cdx.json`)
- Generated from the final container image (includes all transitive deps)
- Artifact published alongside release for supply chain consumers

---

## 4. Coverage Requirements

Coverage is measured using Go's native coverage tooling via Ginkgo's `--coverprofile` flag.

| Tier | Packages | Target | Rationale |
|------|----------|--------|-----------|
| Security-critical | `internal/auth/`, `internal/security/` | 85%+ | Authentication, sanitization, RBAC |
| Session lifecycle | `internal/session/` | 90%+ | State machine correctness |
| Tools | `internal/tools/` | 77%+ | Tool behavior under all input conditions |
| Metrics | `internal/metrics/` | 100% | Registration correctness, no silent failures |
| Resilience | `internal/resilience/` | 80%+ | Circuit breaker state transitions |

### Coverage Packages (from Makefile `COVERPKGS`)

All `internal/` packages are included in coverage measurement:
`auth`, `ratelimit`, `security`, `httputil`, `logging`, `requestid`, `audit`, `metrics`, `agent`, `tools`, `ka`, `ds`, `session`, `config`, `handler`, `launcher`, `resilience`, `streaming`, `controller`

---

## 5. CI Enforcement Gates

The following gates must pass before a PR can be merged:

| Gate | Command | Blocks Merge |
|------|---------|-------------|
| Format check | `go fmt ./...` (diff check) | Yes |
| Vet | `go vet ./...` | Yes |
| Lint | `golangci-lint run ./...` | Yes |
| Unit tests + race detector | `ginkgo -v --race --coverpkg=... ./internal/...` | Yes |
| Maturity validation | `bash hack/validate-maturity.sh` | Yes |
| Image vulnerability scan | `trivy image --severity CRITICAL,HIGH --exit-code 1` | Yes |
| OpenAPI schema validation | `vacuum lint api/openapi/apifrontend-v1.yaml` | Yes |
| Kustomize validation | `make validate-kustomize` | Yes |
| Coverage threshold | Enforced per-tier (see above) | Advisory (warning) |

### Test Execution Flags

```bash
ginkgo -v \
  --race \
  --coverpkg=./internal/auth/...,./internal/tools/...,... \
  --coverprofile=cover.out \
  ./internal/...
```

Key flags:
- `--race`: Enables Go's race detector for all test executions
- `--coverpkg`: Explicit package list ensures cross-package coverage is captured
- `-v`: Verbose output for CI log traceability

---

## 6. Performance Testing

Performance tests are defined but execution is deferred until proper hardware is available.

| Script | Profile | SLO Validated |
|--------|---------|---------------|
| `health-baseline.js` | Baseline (10 users, 5m) | Idle resource usage |
| `mcp-tools-call.js` | Normal/Peak | Tool call latency p95 < 500ms |
| `sse-streams.js` | Normal/Peak | SSE connection handling |
| `mixed-workload.js` | Peak/Stress | Combined load behavior |

Dry-run validation: `make test-perf-local` (validates scripts parse correctly)

See `docs/testing/PERFORMANCE_TEST_PLAN.md` for full profile definitions and SLO mappings.

---

## 7. Known Gaps and Roadmap

| Gap | Status | Timeline | Mitigation |
|-----|--------|----------|------------|
| DAST (Dynamic Application Security Testing) | Not yet applicable | Post-v1.5 | Compensated by comprehensive unit/integration tests + race detector |
| Penetration testing | Not yet applicable | Post-GA | External audit planned for GA milestone |
| Fuzzing (`go test -fuzz`) | Backlog | v1.6 | Input validation via `internal/validate` package provides defense-in-depth |
| Dependency license scanning | Not automated | v1.6 | Manual review during dependency updates |
| Signed container images (cosign) | Backlog | Pre-GA | SBOM generation is already in place |

---

## 8. Security-Specific Test Categories

| Category | Example Tests | Validates |
|----------|--------------|-----------|
| Auth bypass attempts | `jwt_test.go` — expired, malformed, wrong audience | IA-2, IA-8 |
| RBAC enforcement | `rbac_tools_test.go` — role/tool matrix | AC-3, AC-6 |
| Input validation | `af_*_test.go` — invalid namespaces, SQL injection | SI-10 |
| Rate limit behavior | `ratelimit_test.go` — burst, eviction | SC-5 |
| Circuit breaker state | `k8s_cb_test.go`, `integration_test.go` | SC-7 (availability) |
| Audit event emission | `audit_test.go`, `buffered_test.go` | AU-2, AU-12 |
| Header sanitization | `sanitize_test.go` — control chars, injection | SI-10 |
| Impersonation isolation | `impersonation_test.go` — no identity leakage | AC-6 |

---

*Source files: `Makefile`, `hack/validate-maturity.sh`, `docs/testing/PERFORMANCE_TEST_PLAN.md`, 77 `*_test.go` files across `internal/`*
