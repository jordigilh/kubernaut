# Test Plan — Cycle A: Operational Wiring

| Field | Value |
|-------|-------|
| **Identifier** | `TP-CYCLE-A-001` |
| **Version** | 1.0 |
| **Status** | Draft |
| **Created** | 2026-05-13 |
| **Standard** | IEEE 829-2008 |

## 1. References

| Ref | Document |
|-----|----------|
| R-01 | GA Readiness Audit — 43 findings (`.cursor/plans/rc1_ga_remediation_9a04535e.plan.md`) |
| R-02 | `cmd/apifrontend/main.go` — application wiring |
| R-03 | `internal/metrics/metrics.go` — Prometheus metric definitions |
| R-04 | `internal/handler/health.go` — readiness handler |
| R-05 | `internal/handler/router.go` — HTTP router setup |
| R-06 | `internal/handler/mcp_bridge.go` — MCP tool registration |
| R-07 | `internal/streaming/tracker.go` — SSE connection tracker |
| R-08 | `internal/ka/rest_client.go` — KA REST client |
| R-09 | `internal/resilience/circuitbreaker.go` — circuit breaker transport |
| R-10 | `internal/resilience/k8s_dynamic.go` — resilient K8s dynamic client |
| R-11 | `internal/auth/jwt.go` — JWT validator with CB metrics option |
| R-12 | `internal/ratelimit/ratelimit.go` — user rate limiter |
| R-13 | `deploy/kustomize/base/05-prometheusrule.yaml` — alerting rules |
| R-14 | `deploy/kustomize/base/08-servicemonitor.yaml` — scrape config |
| R-15 | `deploy/kustomize/base/rbac_roles.yaml` — RBAC tool roles |
| R-16 | `internal/agent/rbac_roles.yaml` — embedded RBAC roles |

## 2. Introduction

This test plan covers the **operational wiring** layer of kubernaut-apifrontend: the
code in `cmd/apifrontend/main.go` that connects fully-implemented `internal/` packages
to the production binary. The audit identified 12 findings where config values are
validated and internal packages are unit-tested, but `main.go` never connects them.

**Objective:** Prove that the production binary exhibits correct operational behavior —
metrics are emitted with correct labels, readiness probes reflect dependency health,
shutdown is graceful, rate limits are enforced on tools, and alerting rules are syntactically
valid and semantically correct.

**Approach:** TDD Red/Green/Refactor. Tests are written first (Red), then minimal
production code makes them pass (Green), then code quality is reviewed (Refactor).

## 3. Test Items

| Item | Version | Source |
|------|---------|--------|
| `cmd/apifrontend/main.go` | HEAD (main branch) | R-02 |
| `internal/handler/router.go` | HEAD | R-05 |
| `internal/handler/health.go` | HEAD | R-04 |
| `internal/handler/mcp_bridge.go` | HEAD | R-06 |
| `internal/streaming/tracker.go` | HEAD | R-07 |
| `deploy/kustomize/base/05-prometheusrule.yaml` | HEAD | R-13 |
| `deploy/kustomize/base/08-servicemonitor.yaml` | HEAD | R-14 |
| `deploy/kustomize/base/rbac_roles.yaml` | HEAD | R-15 |

## 4. Software Risk Issues

| Risk | Impact | Mitigation |
|------|--------|------------|
| E2E cluster not available for all tests | Some tests require Kind + port-forward | Unit-level wiring tests cover logic; E2E tests gated by `e2e` label |
| Metrics scrape timing | Prometheus counters are eventually consistent | Tests use direct registry query, not HTTP scrape, for unit tests |
| Port-forward flakiness in shutdown test | SSE drain test depends on stable connection | Retry with backoff; skip in CI if flaky, gate on local E2E |
| RBAC tool name change is breaking | Renaming tools affects existing clients | Coordinate with upstream tool catalog; provide migration note |

## 5. Features to be Tested

### 5.1 WIRE-01 — Health Mux Readyz Dependency Awareness

**Current behavior:** `GET /readyz` on `:8081` returns `{"status":"ready"}` even when KA/DS
dependencies are unreachable. Only checks `draining` flag.

**Required behavior:** `/readyz` on `:8081` must call `depsReady()` (composite of
`kaClient.Healthy()` and `dsResilientTransport.Healthy()`). When any dependency is unhealthy,
return HTTP 503 with structured response.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-01a | AF running, KA/DS healthy | `GET :8081/readyz` | HTTP 200, body contains dependency status | Unit |
| TC-A-01b | AF running, KA unhealthy (CB open) | `GET :8081/readyz` | HTTP 503, body indicates KA unhealthy | Unit |
| TC-A-01c | AF running, DS unreachable | `GET :8081/readyz` | HTTP 503, body indicates DS unhealthy | Unit |
| TC-A-01d | AF draining (shutdown in progress) | `GET :8081/readyz` | HTTP 503, body indicates draining | Unit |
| TC-A-01e | AF in E2E cluster, deps healthy | `GET :8081/readyz` via port-forward | HTTP 200 | E2E |
| TC-A-01f | `depsReady` is nil (defensive) | `GET :8081/readyz` | HTTP 200 (fail-open, log warning) | Unit |

### 5.2 WIRE-03 — K8s Dynamic Client Resilience Wrapping

**Current behavior:** `buildDynFactory` returns raw `dynamic.Interface` from
`auth.NewImpersonatingDynamicFactory`. No circuit breaker protection.

**Required behavior:** Dynamic client wrapped with `resilience.NewResilientDynamicClient`
using `cfg.Resilience.K8s` circuit breaker settings.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-03a | ResilientDynamicClient constructed with CB config | K8s API returns 5xx N times | CB opens; `af_circuit_breaker_state{dependency="k8s"}` transitions to 2 | Unit |
| TC-A-03b | CB open | K8s List call | Error returned immediately (not sent to API server) | Unit |
| TC-A-03c | `cfg.Resilience.K8s` values from E2E config | Inspect constructed CB | `cbFailureThreshold=3`, `cbTimeout=10s` match config | Unit |

### 5.3 WIRE-04 — MCP Bridge UserLimiter Wiring

**Current behavior:** `MCPBridgeConfig.UserLimiter` is not set in `main.go`. The field is
nil, so `wrapTool` skips per-user tool call rate limiting.

**Required behavior:** `main.go` passes the constructed `userLimiter` to
`MCPBridgeConfig.UserLimiter`. When a user exceeds tool call rate, subsequent calls
receive rate-limit error.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-04a | UserLimiter configured: 5 calls/min | 6 rapid tool calls from same user | 6th call returns rate-limit error | Unit |
| TC-A-04b | UserLimiter is nil (defensive path) | Any tool call | Tool executes normally (no rate limit) | Unit |
| TC-A-04c | Two different users | 5 calls each | Both succeed (per-user, not global) | Unit |
| TC-A-04d | Rate-limited call | Check metrics | `af_rate_limit_rejections_total{reason="tool_call"}` incremented | Unit |

### 5.4 WIRE-05 — KA Client Metrics and Resilience Config

**Current behavior:** `ka.NewClient` called with only `BaseURL` and `BaseTransport`.
`ClientMetrics` variadic arg not passed. `cfg.Resilience.KA` values ignored (KA client
uses its own hardcoded defaults).

**Required behavior:** `ka.NewClient` receives `ClientMetrics` from metrics registry.
`cfg.Resilience.KA` values are mapped into `ka.Config` fields so the E2E config
`resilience.ka` section takes effect.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-05a | KA client constructed with ClientMetrics | Successful KA call | `af_downstream_request_duration_seconds{dependency="ka"}` observed | Unit |
| TC-A-05b | KA client constructed with Resilience.KA config | Inspect CB config | `cbFailureThreshold` matches `cfg.Resilience.KA.CBFailureThreshold` | Unit |
| TC-A-05c | KA returns 503 three times | After threshold | `af_circuit_breaker_state{dependency="ka"}` transitions to open (2) | Unit |

### 5.5 WIRE-06 — DS Transport DependencyName on CircuitBreaker

**Current behavior:** `buildResilientTransport` sets `Name: name` on CB config but leaves
`DependencyName` as empty string. This means `af_circuit_breaker_state` and
`af_downstream_request_duration_seconds` are emitted with `dependency=""`.

**Required behavior:** `DependencyName: name` set on `CircuitBreakerConfig`. Alert rules
filtering on `dependency="ds"` will match.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-06a | DS transport built with `name="ds"` | DS call succeeds | `af_downstream_request_duration_seconds{dependency="ds"}` has observation | Unit |
| TC-A-06b | DS transport built with `name="ds"` | DS CB opens | `af_circuit_breaker_state{dependency="ds"}` == 2 | Unit |
| TC-A-06c | Transport built with empty name (defensive) | Any call | Metric emitted with `dependency=""` (acceptable, log warning) | Unit |

### 5.6 WIRE-07 — Graceful Shutdown: DrainAll + Configurable Timeout

**Current behavior:** Shutdown sequence sets `draining.Store(true)`, then
`context.WithTimeout(15s)` hardcoded. `routerCfg.SSETracker.DrainAll()` is never called.
Active SSE/MCP connections receive no notification before close.

**Required behavior:** On SIGTERM:
1. Set draining
2. Call `SSETracker.DrainAll(ctx)` to notify connected clients
3. Use `cfg.Shutdown.DrainSeconds` (E2E: 3s) as context timeout

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-07a | E2E config: `shutdown.drainSeconds: 3` | SIGTERM | Shutdown context deadline is ~3s, not 15s | Unit |
| TC-A-07b | Active SSE connection | SIGTERM | Client receives drain/shutdown event before disconnect | Unit/E2E |
| TC-A-07c | No active connections | SIGTERM | Shutdown completes without error | Unit |
| TC-A-07d | DrainAll takes longer than drainSeconds | SIGTERM | Context cancels; remaining connections force-closed | Unit |
| TC-A-07e | `cfg.Shutdown.DrainSeconds` is 0 | SIGTERM | Falls back to sensible default (e.g. 15s) | Unit |

### 5.7 WIRE-08 — JWKS Validator CB Metrics

**Current behavior:** `NewJWTValidator` called without `WithCBMetrics(metricsReg.CircuitBreakerState)`.
JWKS fetch circuit breaker state changes are not reported to Prometheus.

**Required behavior:** `WithCBMetrics` passed so `af_circuit_breaker_state{dependency="jwks_*"}`
is emitted on JWKS CB state transitions.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-08a | Validator with CB metrics | JWKS fetch fails N times (CB opens) | `af_circuit_breaker_state{dependency=~"jwks.*"}` == 2 | Unit |
| TC-A-08b | Validator without CB metrics (nil gauge) | JWKS fetch fails | No panic; CB still functions, no metric emitted | Unit |

### 5.8 WIRE-09/10 — Session, LLM, and Triage Metrics

**Current behavior:** `metricsReg.SessionsActive`, `metricsReg.LLMTokensTotal`,
`metricsReg.SeverityTriageTotal`, `metricsReg.SeverityTriageDuration`,
`metricsReg.SeverityTriageErrorsTotal` are registered but never incremented by any
production code path.

**Required behavior:** Each metric is driven by its corresponding production code path.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-09a | MCP session established | MCP init handshake | `af_sessions_active` gauge incremented | Unit |
| TC-A-09b | MCP session closed | Disconnect/timeout | `af_sessions_active` gauge decremented | Unit |
| TC-A-09c | 10 sessions open, 10 close | Lifecycle loop | Gauge returns to 0 | Unit |
| TC-A-10a | Severity triage succeeds | Triage call with mock triager | `af_severity_triage_total{result="success"}` incremented | Unit |
| TC-A-10b | Severity triage fails | Triage call returns error | `af_severity_triage_errors_total` incremented | Unit |
| TC-A-10c | Severity triage latency | Triage call | `af_severity_triage_duration_seconds` has observation | Unit |

### 5.9 WIRE-11 — PromQL Dependency Latency Aggregation

**Current behavior:** `ApifrontendDependencyLatencyHigh` alert uses:
```promql
histogram_quantile(0.95, sum(rate(..._bucket{...}[5m])) by (le)) > 2
```
Missing `dependency` in `by()` clause. The `{{ $labels.dependency }}` template in the
annotation will always be empty.

**Required behavior:** `by (le, dependency)` so the alert fires per-dependency and the
annotation renders correctly.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-11a | PrometheusRule YAML | `promtool check rules` | Exit 0, no errors | CI |
| TC-A-11b | PromQL text analysis | `ApifrontendDependencyLatencyHigh` expr | Contains `by (le, dependency)` | Unit (YAML parse) |
| TC-A-11c | All `histogram_quantile` rules that reference `{{ $labels.dependency }}` | YAML parse | `dependency` present in `by()` clause | Unit (YAML parse) |

### 5.10 WIRE-02 — ServiceMonitor Job Label

**Current behavior:** ServiceMonitor has no `spec.jobLabel` field. Prometheus assigns
default `job` label based on service name, which may or may not equal `"apifrontend"`.

**Required behavior:** ServiceMonitor must produce `job="apifrontend"` on scraped targets.
All PrometheusRule expressions filter on `job="apifrontend"`.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-02a | ServiceMonitor YAML | Parse spec | `spec.jobLabel` is set OR relabeling produces `job=apifrontend` | Unit (YAML parse) |
| TC-A-02b | PrometheusRule YAML | Grep all `expr` fields | All contain `job="apifrontend"` | Unit (YAML parse) |
| TC-A-02c | ServiceMonitor YAML consistency | Parse metadata.name | Matches `selector.matchLabels` reference | Unit (YAML parse) |

### 5.11 RBAC-01 — Tool Name Alignment

**Current behavior:** `deploy/kustomize/base/rbac_roles.yaml` defines 12 tool names
(`kubernaut_investigate`, `k8s_get_resource`, `ds_query_events`, etc.) that do **not**
match any of the 20 tool names registered in `mcp_bridge.go`
(`kubernaut_list_remediations`, `af_list_events`, `af_get_pods`, etc.).

Additionally, `internal/agent/rbac_roles.yaml` has `present_decision` instead of
`kubernaut_present_decision`.

**Required behavior:** Every tool name in `rbac_roles.yaml` must correspond to an
actually-registered MCP tool. Every registered MCP tool must appear in at least one
RBAC role (or be explicitly documented as unrestricted).

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-RBAC-01a | Parse `rbac_roles.yaml` and `mcp_bridge.go` | Set comparison | Every RBAC tool name exists in bridge registration | Unit |
| TC-A-RBAC-01b | Parse `mcp_bridge.go` tool list | Check RBAC coverage | Every registered tool appears in at least one role OR is documented as public | Unit |
| TC-A-RBAC-01c | `internal/agent/rbac_roles.yaml` | Parse tool names | `kubernaut_present_decision` (not `present_decision`) | Unit |

### 5.12 OPS-PROMTOOL — PrometheusRule CI Validation

**Current behavior:** No CI step validates PrometheusRule YAML with `promtool`.

**Required behavior:** CI runs `promtool check rules` on every PR touching alerting rules.

| TC ID | Preconditions | Input | Expected Result | Type |
|-------|---------------|-------|-----------------|------|
| TC-A-PROM-01a | `promtool` available | `deploy/kustomize/base/05-prometheusrule.yaml` | Exit 0, no errors | CI |
| TC-A-PROM-01b | Introduce syntax error in rule | `promtool check rules` | Exit non-zero, error message identifies line | CI (regression) |

## 6. Features Not to be Tested

| Feature | Rationale |
|---------|-----------|
| KA/DS actual HTTP responses | Mocked in unit tests; covered by E2E suite Phase 1 |
| LLM integration | Requires real LLM; covered by `test-llm-local` target |
| CRD session lifecycle | Deferred to operator (issue #97) |
| Distributed replay cache | Deferred to operator (issue #98) |
| Production image digest pinning | Operator responsibility (issue #73) |

## 7. Approach

### 7.1 Unit Tests (`internal/` and `cmd/`)

- **Framework:** Ginkgo v2 + Gomega
- **Pattern:** Table-driven test cases within `Describe`/`It` blocks
- **Metrics testing:** Construct fresh `metrics.Registry`, call production code, query
  counter/gauge/histogram directly via `testutil.ToFloat64()` or registry `Gather()`
- **Mocking:** Use existing mocks (`ka.MockClient`, `ds.MockClient`, `dynamicfake`);
  add mock `ConnectionTracker` for DrainAll verification
- **Race detection:** All unit tests run with `-race` (`make test-unit`)

### 7.2 E2E Tests (`test/e2e/`)

- **Framework:** Ginkgo v2 with `phase1` label filter
- **Infrastructure:** Kind cluster with TLS, DEX, NetworkPolicy (`make e2e-setup`)
- **Metrics scrape:** HTTP GET to `:9090/metrics`, parse Prometheus text format
- **Readyz probe:** HTTP GET to `:8081/readyz` via port-forward
- **Shutdown:** `kubectl delete pod` with active SSE connection; assert drain event

### 7.3 Manifest Tests (YAML validation)

- **PromQL:** `promtool check rules` in CI step
- **YAML parsing:** Go tests that `yaml.Unmarshal` the ServiceMonitor and PrometheusRule,
  then assert structural properties (job label, `by()` clauses, tool name alignment)

## 8. Pass/Fail Criteria

### Item-level

A test item **passes** when all its test cases (TC-*) pass with `-race` enabled.

### Plan-level

The plan **passes** when:
1. All test items pass
2. `make test-unit` exits 0 with `-race`
3. `make lint` exits 0
4. `make vet` exits 0
5. `promtool check rules deploy/kustomize/base/05-prometheusrule.yaml` exits 0
6. No new `golangci-lint` warnings in touched files
7. 9-category checkpoint audit (Checkpoint A) is satisfied

### Plan-level failure

The plan **fails** if any test case fails after the Green phase. Failures in the Red
phase are expected (tests are written before implementation).

## 9. Suspension Criteria and Resumption Requirements

| Condition | Action |
|-----------|--------|
| E2E cluster unavailable | Suspend E2E tests; continue unit tests. Resume when cluster is restored. |
| `promtool` binary not available | Install via `go install github.com/prometheus/prometheus/cmd/promtool@latest`. |
| Dependency API change (KA/DS interface) | Escalate. Assess impact on mock contracts. |
| Race condition detected | Fix before advancing to Refactor phase. |

## 10. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| This test plan | `docs/test/cycle-a-operational-wiring/TEST_PLAN.md` |
| Unit test source | `cmd/apifrontend/main_wiring_test.go` |
| Manifest validation tests | `internal/handler/manifest_validation_test.go` |
| E2E operational tests | `test/e2e/operational_contract_test.go` |
| Coverage report | `cover.out` (via `make test-unit`) |
| PromQL validation | CI step output |

## 11. Environmental Needs

| Environment | Purpose | Setup |
|-------------|---------|-------|
| Local Go 1.26+ | Unit tests | Developer workstation |
| Kind cluster | E2E tests | `make e2e-setup` |
| `promtool` binary | PromQL validation | `go install` or OS package manager |
| `golangci-lint` | Lint gate | Already in CI |

## 12. Schedule

| Phase | Activities | Gate |
|-------|-----------|------|
| A.Plan | This document | Reviewed |
| A.Red | Write all TC-* tests; verify they fail | All tests compile and fail for the expected reason |
| A.Green | Implement wiring fixes in `main.go` + manifests | All tests pass |
| A.Refactor | 100-go-mistakes review; `make lint`; `make test-unit` with `-race` | Clean |
| Checkpoint A | 9-category audit | All 9 satisfied |

## 13. Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| RBAC tool name rename breaks existing clients | Medium | High | Verify no external consumers of current `rbac_roles.yaml`; this is pre-GA |
| DrainAll test flaky under CI load | Medium | Low | Use generous timeouts; mark as `[Flaky]` if needed |
| Metrics registry coupling | Low | Medium | Use fresh registry per test; no global state |
| PromQL `by()` fix changes alert semantics | Low | High | Verify with `promtool` and manual PromQL review |
