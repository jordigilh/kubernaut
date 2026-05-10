# Test Plan: Three-Tier Severity Triage for Manual Signals

**Test Plan Identifier:** TP-AF-092-SEVERITY-TRIAGE
**Issue:** [#92](https://github.com/jordigilh/kubernaut-apifrontend/issues/92)
**Version:** 1.0
**Date:** 2026-05-02
**Status:** Draft

---

## 1. Introduction

This test plan validates the three-tier severity triage pipeline that determines severity for manual signals before `RemediationRequest` creation. The pipeline queries Prometheus for firing alerts (Tier 1), pending alerts (Tier 1.5), evaluates rule expressions (Tier 2), falls back to LLM with rule context (Tier 2.5), and finally to pure LLM classification (Tier 3).

### 1.1 Scope

- `internal/prometheus/client.go` — HTTP client wrapping `/api/v1/alerts`, `/api/v1/rules`, `/api/v1/query`
- `internal/prometheus/rules.go` — Rule matching: parse PromQL AST, extract label selectors, filter by target resource
- `internal/severity/triage.go` — Orchestrator: runs Tier 1 -> 1.5 -> 2 -> 2.5 -> 3 pipeline
- `internal/severity/types.go` — `TriageResult`, `Source`, severity ordering
- `internal/severity/llm.go` — LLM triage for Tier 2.5 (rule-informed) and Tier 3 (pure fallback)
- `internal/severity/cache.go` — TTL-based cache for `/api/v1/rules` responses
- Integration with `internal/tools/af_create_rr.go` and `internal/tools/crd_tools.go`
- `internal/config/config.go` — New `SeverityTriage` config section
- `internal/metrics/metrics.go` — New `af_severity_triage_*` metrics
- `internal/audit/audit.go` — New `EventSeverityTriage*` audit events
- `deploy/prometheus-rules.yaml` — New alerting rules for triage health

### 1.2 Out of Scope

- Individual `Handle*` function logic (covered by TP-AF-019-020)
- MCP SDK internals (covered by SDK tests)
- Prometheus server-side configuration (rules deployment)
- LLM model training/tuning
- kubernaut-operator deployment manifests

### 1.3 References

- TP-AF-019-020-BRIDGE: MCP Bridge test plan
- Issue #92: Three-Tier Severity Triage for Manual Signals
- DD-SEVERITY-TRIAGE-001: Design document
- DD-SEVERITY-001 v1.1: Severity Determination Refactoring (kubernaut)
- kubernaut `pkg/effectivenessmonitor/client/prometheus_http.go`: Reference Prometheus client
- kubernaut `pkg/agentclient/oas_schemas_gen.go`: Canonical `Severity` type
- FedRAMP Controls: AU-2/AU-12 (auditable events), AC-6 (least privilege), SI-4 (monitoring), RA-5 (vulnerability scanning)
- NIST 800-53: CA-7 (continuous monitoring), SC-8 (transmission confidentiality)
- [100 Go Mistakes](https://github.com/teivah/100-go-mistakes) — refactoring checklist

### 1.4 Readiness Audit Findings Addressed

This test plan incorporates fixes for all 25 findings from the multi-dimensional FedRAMP readiness audit:

#### Architecture (ARCH)

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| ARCH-01 | FAIL | Plan uses `pkg/` but codebase has no `pkg/` directory; all code is in `internal/` | Move packages to `internal/prometheus/` and `internal/severity/` |
| ARCH-02 | FAIL | `signalLabels` not present in existing `af_create_rr` CRD spec | Add `signalLabels` map to RR unstructured spec in `HandleCreateRR` |
| ARCH-03 | WARN | `severity_source` plan says "recommend yes" as first-class field — needs decision | Implement as `spec.signalLabels["severity_source"]` (map entry, not separate field) to avoid CRD schema change |
| ARCH-04 | WARN | Plan references `google.golang.org/adk` for LLM but existing triage code should use `genai` directly | Use `google.golang.org/genai` client directly for structured LLM calls (consistent with existing `internal/session/reinvoke.go`) |
| ARCH-05 | WARN | No circuit breaker specified for Prometheus dependency | Add Prometheus to `ResilienceConfig` with CB and timeout settings |

#### Security / FedRAMP (SEC)

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| SEC-01 | FAIL | No audit event for severity triage decisions (FedRAMP AU-2 gap) | Add `EventSeverityTriageCompleted` and `EventSeverityTriageFailed` audit events with tier, source, severity, duration |
| SEC-02 | FAIL | Prometheus query strings could contain injected PromQL if constructed from user input | All PromQL comes from `/api/v1/rules` responses (server-controlled); validate that no user input is interpolated into query strings. Add explicit guard. |
| SEC-03 | FAIL | LLM responses not validated against severity enum — could produce arbitrary strings | Validate LLM output against `validSeverities` allowlist; reject invalid responses via `NormalizeSeverity` (returns `"medium"` for invalid LLM strings only, not for pipeline failures) |
| SEC-04 | FAIL | No TLS CA config for Prometheus client (FedRAMP SC-8) | Support `prometheus.tlsCaFile` in config; build `*tls.Config` with custom CA pool |
| SEC-05 | WARN | Prometheus bearer token for ServiceAccount auth not specified | Support `prometheus.bearerTokenFile` (standard `/var/run/secrets/kubernetes.io/serviceaccount/token`) |
| SEC-06 | WARN | LLM prompt could leak K8s resource names to external provider | Document that LLM receives namespace/kind/name by design (required for classification); redact any secrets/configmap data from events context |
| SEC-07 | WARN | Error responses from Prometheus could contain internal cluster URLs | Wrap Prometheus errors through `security.RedactError` before returning to caller or audit |

#### QE / Test Quality (QE)

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| QE-01 | FAIL | No integration test for full Tier 1 -> 1.5 -> 2 -> 2.5 -> 3 fallthrough | Add IT test with mock Prometheus returning empty at each tier to verify fallthrough |
| QE-02 | FAIL | No test for PromQL AST parsing of complex expressions (subqueries, `absent()`, aggregations) | Add table-driven tests covering 15+ expression patterns |
| QE-03 | WARN | No test for Prometheus client timeout behavior | Add test with `httptest.Server` that delays beyond timeout |
| QE-04 | WARN | No test for cache TTL expiry and concurrent access | Add test with short TTL and 10+ goroutines accessing cache under `-race` |

#### Production / SRE (SRE)

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| SRE-01 | FAIL | No metrics for triage pipeline health | Add `af_severity_triage_total{tier,severity}`, `af_severity_triage_duration_seconds{tier}`, `af_severity_triage_errors_total{tier,error_type}` |
| SRE-02 | FAIL | No alerting rule for triage failure rate | Add `ApifrontendSeverityTriageErrorRate` Prometheus alert |
| SRE-03 | FAIL | No rate limiting on Prometheus instant queries per triage | Enforce max 10 queries per triage invocation; configurable |
| SRE-04 | WARN | No runbook for triage failures | Add `RB-AF-010.md` runbook for severity triage troubleshooting |
| SRE-05 | WARN | No graceful degradation when Prometheus is unreachable | Fall through to Tier 3 (LLM) on Prometheus errors; emit audit event |

#### Product / UX (UX)

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| UX-01 | FAIL | `severity` is optional in `CreateRRArgs` but `validSeverities` rejects empty string — triage must populate before validation | Invoke triage before severity validation; if triage fails, propagate error to caller (no silent defaults per ADR-021) |
| UX-02 | WARN | No user-visible indication of which tier determined severity | Return `severity_source` in tool response alongside severity |

#### Supply Chain (SC)

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| SC-01 | WARN | `github.com/prometheus/prometheus` is a large dependency (promql/parser); verify only parser subpackage is used | Import only `github.com/prometheus/prometheus/promql/parser` — already in `go.mod` |
| SC-02 | WARN | No `go-licenses` check for Prometheus parser license compatibility | Verified: Apache-2.0 (compatible) |

### 1.5 Definitions

| Term | Definition |
|------|-----------|
| Triage | The process of determining severity for a manual signal |
| Tier 1 | Check firing alerts from Prometheus `/api/v1/alerts` |
| Tier 1.5 | Check pending alerts from Prometheus `/api/v1/rules` |
| Tier 2 | Evaluate matching rule expressions via `/api/v1/query` |
| Tier 2.5 | LLM classification informed by matched rule definitions |
| Tier 3 | Pure LLM classification without rule context |
| severity_source | Audit label tracking which tier determined the severity |
| signalLabels | Map of triage metadata attached to the RR spec |

---

## 2. Test Items

| Item | Package | Source |
|------|---------|--------|
| `Client` (Prometheus HTTP) | `internal/prometheus` | New |
| `ExtractLabelMatchers` | `internal/prometheus` | New |
| `MatchesResource` | `internal/prometheus` | New |
| `RulesCache` | `internal/severity` | New |
| `Triager` | `internal/severity` | New |
| `TriageResult` / `Source` | `internal/severity` | New |
| `LLMTriager` | `internal/severity` | New |
| `SeverityTriageConfig` | `internal/config` | Modified |
| `HandleCreateRR` | `internal/tools` | Modified |
| `HandleSubmitSignal` | `internal/tools` | Modified |
| `MCPBridgeConfig` | `internal/handler` | Modified |
| `Registry` (metrics) | `internal/metrics` | Modified |
| Audit events | `internal/audit` | Modified |
| Prometheus rules | `deploy/prometheus-rules.yaml` | Modified |
| Runbook | `docs/operations/runbooks/RB-AF-010.md` | New |

---

## 3. Business Acceptance Criteria

| ID | Criterion | Source | Priority |
|----|-----------|--------|----------|
| BAC-T-01 | Manual signals with no user-supplied severity receive triage-derived severity before RR creation | Issue #92 | P0 |
| BAC-T-02 | Firing alerts (Tier 1) take precedence — highest severity inherited | DD-SEVERITY-TRIAGE-001 | P0 |
| BAC-T-03 | Pending alerts (Tier 1.5) inherited when no firing alerts | Issue #92 discussion | P0 |
| BAC-T-04 | Rule evaluation (Tier 2) correctly evaluates PromQL and inherits severity | Issue #92 | P0 |
| BAC-T-05 | LLM with rule context (Tier 2.5) used when rules match but expression yields no data | Issue #92 discussion | P0 |
| BAC-T-06 | Pure LLM (Tier 3) used when no rules cover the target resource | Issue #92 | P0 |
| BAC-T-07 | `severity_source` is recorded in `spec.signalLabels` on the RR CRD | FedRAMP AU-2 | P0 |
| BAC-T-08 | Triage decisions emitted as audit events with tier, severity, duration | FedRAMP AU-12 | P0 |
| BAC-T-09 | Triage metrics (counter, histogram, error counter) emitted per tier | SLO | P0 |
| BAC-T-10 | Prometheus errors degrade gracefully to LLM fallback | Production resilience | P0 |
| BAC-T-11 | PromQL expressions only come from Prometheus server responses, never user input | Security | P0 |
| BAC-T-12 | LLM severity response validated against `validSeverities` allowlist | Security | P0 |
| BAC-T-13 | `/api/v1/rules` response cached with configurable TTL (default 30s) | Performance | P1 |
| BAC-T-14 | Max 10 instant queries per triage invocation (configurable) | SRE | P1 |
| BAC-T-15 | Prometheus client supports TLS CA and bearer token auth | FedRAMP SC-8 | P1 |
| BAC-T-16 | Errors from Prometheus are redacted before audit/client exposure | Security | P1 |
| BAC-T-17 | User-supplied severity on `af_create_rr` bypasses triage entirely | UX | P0 |
| BAC-T-18 | Triage failure returns error to caller (no silent defaults); LLM is mandatory at startup (panic on nil) | Product / ADR-021 | P0 |

---

## 4. Features by Tier

### Tier 1: Prometheus Client and PromQL Parsing

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-T.1 | Prometheus HTTP client for `/api/v1/alerts` | Returns typed alerts with labels, severity, state |
| F-T.2 | Prometheus HTTP client for `/api/v1/rules` | Returns typed rule groups with state, expression, labels |
| F-T.3 | Prometheus HTTP client for `/api/v1/query` | Returns instant query result (vector type) |
| F-T.4 | `ExtractLabelMatchers` from PromQL expression | Extracts VectorSelector label matchers from AST |
| F-T.5 | `MatchesResource` compares matchers against resource labels | Returns true when all matchers satisfied |
| F-T.6 | Handles complex PromQL (subqueries, `absent()`, aggregation) | Does not panic; returns partial or empty matchers |
| F-T.7 | Context propagation and timeout on all HTTP calls | Respects `context.Context` deadline |
| F-T.8 | TLS CA file support | Loads CA from file path, adds to TLS config |
| F-T.9 | Bearer token file support | Reads token from file, adds to Authorization header |
| F-T.10 | Error wrapping with `security.RedactError` | No internal URLs in client-visible errors |

### Tier 2: Triage Orchestrator and Cache

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-T.11 | Tier 1: Query firing alerts, filter by resource labels, pick highest severity | Returns severity + `"firing_alert"` source |
| F-T.12 | Tier 1.5: Query rules, filter pending by label matchers | Returns severity + `"pending_alert"` source |
| F-T.13 | Tier 2: For inactive matched rules, evaluate expression via instant query | Returns severity + `"rule_evaluation"` source |
| F-T.14 | Tier 2 query rate limiting (max N per triage) | Stops evaluating after N queries |
| F-T.15 | Tier 2.5: LLM classification with rule context | Returns severity + `"llm_rule_informed"` source |
| F-T.16 | Tier 3: Pure LLM classification | Returns severity + `"llm_triage"` source |
| F-T.17 | Severity ordering (critical > high > medium > low > info) | Picks highest when multiple alerts/rules match |
| F-T.18 | Rules cache with configurable TTL | Same-TTL requests return cached response |
| F-T.19 | Cache concurrent access safety | 10+ goroutines reading/writing under `-race` |
| F-T.20 | Graceful degradation on Prometheus errors | Falls through to next tier on HTTP/parse errors |

### Tier 3: LLM Integration

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-T.21 | Structured LLM prompt with AlertManager-style severity definitions | Prompt includes resource context and severity enum |
| F-T.22 | LLM response parsed and validated against `validSeverities` | Invalid responses default to `"medium"` |
| F-T.23 | Confidence threshold; below threshold defaults to `"medium"` | Configurable threshold (default 0.7) |
| F-T.24 | LLM token accounting via `af_llm_tokens_total` | Existing metric incremented on triage LLM calls |
| F-T.25 | LLM rate limiting via existing `MaxLLMConcurrency` | Respects global concurrency limit |

### Tier 4: Tool Integration and Observability

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-T.26 | `HandleCreateRR` invokes triage when `Severity == ""` | RR created with triage-derived severity |
| F-T.27 | `HandleCreateRR` populates `signalLabels` in RR spec | `severity_source`, `severity_alert_name`, `severity_rule_name` |
| F-T.28 | `HandleSubmitSignal` invokes triage when `Severity == ""` | SP created with triage-derived severity |
| F-T.29 | User-supplied severity bypasses triage | `Severity != ""` skips triage entirely |
| F-T.30 | `af_severity_triage_total{tier,severity}` counter | Incremented per triage completion |
| F-T.31 | `af_severity_triage_duration_seconds{tier}` histogram | Observed per tier execution |
| F-T.32 | `af_severity_triage_errors_total{tier,error_type}` counter | Incremented on tier errors |
| F-T.33 | `EventSeverityTriageCompleted` audit event | Emitted with tier, severity, source, duration_ms |
| F-T.34 | `EventSeverityTriageFailed` audit event | Emitted with error (redacted), tier where failure occurred |
| F-T.35 | Config: `SeverityTriage` struct with defaults and validation | PrometheusURL, CacheTTL, MaxQueries, LLMConfidence |

### Tier 5: Adversarial and Edge Cases

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-T.36 | Empty resource labels → immediate Tier 3 | No Prometheus queries with empty matchers |
| F-T.37 | Prometheus returns malformed JSON | Parsed error, falls through to next tier |
| F-T.38 | Prometheus returns > 1000 rules | Only first 100 evaluated (bounded) |
| F-T.39 | PromQL expression with Unicode | Parser handles or returns error; no panic |
| F-T.40 | LLM returns severity not in allowlist | Rejected; defaults to `"medium"` |

---

## 5. Test Cases

### 5.1 Tier 1: Prometheus Client (22 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-T-001 | `GetAlerts` returns firing alerts filtered by namespace/pod labels | BAC-T-02 | P0 |
| UT-AF-T-002 | `GetAlerts` returns empty when no alerts match target | BAC-T-02 | P0 |
| UT-AF-T-003 | `GetAlerts` handles HTTP 500 from Prometheus | BAC-T-10 | P0 |
| UT-AF-T-004 | `GetAlerts` respects context cancellation | BAC-T-10 | P0 |
| UT-AF-T-005 | `GetRules` returns rule groups with state (inactive/pending/firing) | BAC-T-03 | P0 |
| UT-AF-T-006 | `GetRules` handles malformed JSON response | BAC-T-10 | P0 |
| UT-AF-T-007 | `InstantQuery` returns vector result for valid expression | BAC-T-04 | P0 |
| UT-AF-T-008 | `InstantQuery` returns empty result for no-data expression | BAC-T-04 | P0 |
| UT-AF-T-009 | `InstantQuery` handles Prometheus HTTP 422 (bad query) | BAC-T-10 | P0 |
| UT-AF-T-010 | Client with TLS CA file successfully connects to TLS server | BAC-T-15 | P1 |
| UT-AF-T-011 | Client with bearer token file sends Authorization header | BAC-T-15 | P1 |
| UT-AF-T-012 | Client timeout: request cancelled after configured duration | BAC-T-10 | P0 |
| UT-AF-T-013 | `ExtractLabelMatchers` from simple `up{job="foo"}` | BAC-T-04 | P0 |
| UT-AF-T-014 | `ExtractLabelMatchers` from `rate(http_requests_total{namespace="prod"}[5m]) > 100` | BAC-T-04 | P0 |
| UT-AF-T-015 | `ExtractLabelMatchers` from `absent(up{job="myapp"})` — returns matchers from inner | BAC-T-04 | P0 |
| UT-AF-T-016 | `ExtractLabelMatchers` from aggregation `sum by (namespace) (rate(...))` | BAC-T-04 | P0 |
| UT-AF-T-017 | `ExtractLabelMatchers` from subquery `metric{foo="bar"}[1h:5m]` | BAC-T-04 | P0 |
| UT-AF-T-018 | `ExtractLabelMatchers` from invalid PromQL returns error (no panic) | BAC-T-04 | P0 |
| UT-AF-T-019 | `MatchesResource` returns true when all matchers match resource labels | BAC-T-04 | P0 |
| UT-AF-T-020 | `MatchesResource` returns false on partial match | BAC-T-04 | P0 |
| UT-AF-T-021 | `MatchesResource` handles regex matchers (`=~`) | BAC-T-04 | P0 |
| UT-AF-T-022 | Prometheus error is redacted (no internal URLs) before returning | BAC-T-16 | P0 |

### 5.2 Tier 2: Triage Orchestrator and Cache (24 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-T-023 | Tier 1 hit: firing alert with severity=critical → returns critical, source=firing_alert | BAC-T-02 | P0 |
| UT-AF-T-024 | Tier 1 hit: multiple firing alerts → returns highest severity | BAC-T-02 | P0 |
| UT-AF-T-025 | Tier 1 miss → falls through to Tier 1.5 | BAC-T-03 | P0 |
| UT-AF-T-026 | Tier 1.5 hit: pending rule with severity=high → returns high, source=pending_alert | BAC-T-03 | P0 |
| UT-AF-T-027 | Tier 1.5 miss → falls through to Tier 2 | BAC-T-04 | P0 |
| UT-AF-T-028 | Tier 2 hit: matching rule expression evaluates with data → returns severity, source=rule_evaluation | BAC-T-04 | P0 |
| UT-AF-T-029 | Tier 2 miss: expression returns empty → falls through to Tier 2.5 | BAC-T-05 | P0 |
| UT-AF-T-030 | Tier 2.5: LLM receives rule context → returns severity, source=llm_rule_informed | BAC-T-05 | P0 |
| UT-AF-T-031 | Tier 2 no matching rules → falls through to Tier 3 (skips 2.5) | BAC-T-06 | P0 |
| UT-AF-T-032 | Tier 3: LLM pure fallback → returns severity, source=llm_triage | BAC-T-06 | P0 |
| UT-AF-T-033 | Full pipeline: Tier 1 miss → 1.5 miss → 2 miss → 2.5 hit | BAC-T-05 | P0 |
| UT-AF-T-034 | Full pipeline: all tiers miss → Tier 3 | BAC-T-06 | P0 |
| UT-AF-T-035 | Severity ordering: critical > high > medium > low > info | BAC-T-02 | P0 |
| UT-AF-T-036 | Rate limit: max 10 instant queries per triage enforced | BAC-T-14 | P1 |
| UT-AF-T-037 | Prometheus error at Tier 1 → graceful fallthrough to Tier 1.5 | BAC-T-10 | P0 |
| UT-AF-T-038 | Prometheus error at all tiers → Tier 3 LLM fallback | BAC-T-10 | P0 |
| UT-AF-T-039 | Rules cache: second call within TTL returns cached response | BAC-T-13 | P1 |
| UT-AF-T-040 | Rules cache: call after TTL expiry fetches fresh response | BAC-T-13 | P1 |
| UT-AF-T-041 | Rules cache: 10 goroutines read/write concurrently under -race | BAC-T-13 | P1 |
| UT-AF-T-042 | Rules cache: eviction after 50 cycles does not grow unbounded | BAC-T-13 | P1 |
| UT-AF-T-043 | Triage with empty resource labels → immediately Tier 3 | BAC-T-06 | P0 |
| UT-AF-T-044 | Triage with Prometheus returning > 100 rules → bounded evaluation | BAC-T-14 | P1 |
| UT-AF-T-045 | Triage context cancellation propagates to all tiers | BAC-T-10 | P0 |
| UT-AF-T-046 | Triage respects feature flag `severity.triage.enabled=false` → skips, returns empty | BAC-T-17 | P1 |

### 5.3 Tier 3: LLM Integration (10 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-T-047 | LLM Tier 2.5 prompt includes rule name, expression, annotations, severity | BAC-T-05 | P0 |
| UT-AF-T-048 | LLM Tier 3 prompt includes resource context (namespace, kind, name, description) | BAC-T-06 | P0 |
| UT-AF-T-049 | LLM returns valid severity → accepted | BAC-T-12 | P0 |
| UT-AF-T-050 | LLM returns invalid severity string → defaults to "medium" | BAC-T-12 | P0 |
| UT-AF-T-051 | LLM returns confidence below threshold → defaults to "medium" | BAC-T-18 | P1 |
| UT-AF-T-052 | LLM call error → returns error, audit emitted | BAC-T-08 | P0 |
| UT-AF-T-053 | LLM token accounting: `af_llm_tokens_total` incremented | BAC-T-09 | P0 |
| UT-AF-T-054 | LLM respects `MaxLLMConcurrency` global limit | BAC-T-14 | P1 |
| UT-AF-T-055 | LLM prompt does not contain secrets/configmap data (only namespace/kind/name) | BAC-T-11 | P0 |
| UT-AF-T-056 | LLM timeout: call cancelled after configured LLM timeout | BAC-T-10 | P0 |

### 5.4 Tier 4: Tool Integration and Observability (18 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-T-057 | `HandleCreateRR` with empty severity invokes triage → RR has triage severity | BAC-T-01 | P0 |
| UT-AF-T-058 | `HandleCreateRR` with user-supplied severity skips triage | BAC-T-17 | P0 |
| UT-AF-T-059 | `HandleCreateRR` populates `spec.signalLabels` with severity_source | BAC-T-07 | P0 |
| UT-AF-T-060 | `HandleCreateRR` populates `spec.signalLabels` with alert/rule name when available | BAC-T-07 | P0 |
| UT-AF-T-061 | `HandleSubmitSignal` with empty severity invokes triage → SP has triage severity | BAC-T-01 | P0 |
| UT-AF-T-062 | `HandleSubmitSignal` with user-supplied severity skips triage | BAC-T-17 | P0 |
| UT-AF-T-063 | Triage failure in `HandleCreateRR` propagates error to caller (no silent default) | BAC-T-18 | P0 |
| UT-AF-T-064 | `af_severity_triage_total{tier="1",severity="critical"}` incremented on Tier 1 hit | BAC-T-09 | P0 |
| UT-AF-T-065 | `af_severity_triage_total{tier="3",severity="medium"}` incremented on Tier 3 hit | BAC-T-09 | P0 |
| UT-AF-T-066 | `af_severity_triage_duration_seconds{tier="1"}` observed on Tier 1 | BAC-T-09 | P0 |
| UT-AF-T-067 | `af_severity_triage_duration_seconds{tier="2"}` observed on Tier 2 | BAC-T-09 | P0 |
| UT-AF-T-068 | `af_severity_triage_errors_total{tier="1",error_type="http"}` incremented on Prometheus error | BAC-T-09 | P0 |
| UT-AF-T-069 | `EventSeverityTriageCompleted` audit event emitted with full detail | BAC-T-08 | P0 |
| UT-AF-T-070 | `EventSeverityTriageFailed` audit event emitted with redacted error | BAC-T-08 | P0 |
| UT-AF-T-071 | Audit event includes `severity_source` and `tier` in detail map | BAC-T-08 | P0 |
| UT-AF-T-072 | Config `SeverityTriage` loaded with defaults (30s cache TTL, 10 max queries) | BAC-T-13 | P1 |
| UT-AF-T-073 | Config validation: PrometheusURL required when triage enabled | BAC-T-15 | P1 |
| UT-AF-T-074 | Config validation: invalid LLM confidence threshold rejected | BAC-T-18 | P1 |

### 5.5 Tier 5: Adversarial Inputs and Edge Cases (12 tests)

| ID | Description | BAC | Priority |
|----|-------------|-----|----------|
| UT-AF-T-075 | Resource labels with empty namespace → Tier 3 (no Prometheus queries) | BAC-T-06 | P0 |
| UT-AF-T-076 | Resource labels with path traversal (`../../etc/passwd`) in kind → validated and rejected | BAC-T-11 | P0 |
| UT-AF-T-077 | Resource labels with max-length+1 name (254 chars) → validated and rejected | BAC-T-11 | P0 |
| UT-AF-T-078 | Resource labels with Unicode NUL bytes → rejected | BAC-T-11 | P0 |
| UT-AF-T-079 | Prometheus returns malformed JSON body → parse error, graceful fallthrough | BAC-T-10 | P0 |
| UT-AF-T-080 | Prometheus returns > 1000 rules → bounded to 100 evaluation | BAC-T-14 | P0 |
| UT-AF-T-081 | PromQL expression with Unicode characters → parser handles or errors (no panic) | BAC-T-04 | P0 |
| UT-AF-T-082 | LLM returns empty string → defaults to "medium" | BAC-T-12 | P0 |
| UT-AF-T-083 | LLM returns "CRITICAL" (wrong case) → normalized to "critical" | BAC-T-12 | P0 |
| UT-AF-T-084 | Concurrent triage calls (10 goroutines) under -race | BAC-T-01 | P0 |
| UT-AF-T-085 | NewTriager panics when LLMTriager is nil (fail-fast at startup per ADR-021) | BAC-T-18 | P0 |
| UT-AF-T-086 | Triage with nil metrics → silently skipped, not panic | BAC-T-09 | P0 |

---

## 6. Pass/Fail Criteria

### 6.1 Pass

- All 86 tests pass with `ginkgo -race`
- Coverage >= 80% for `internal/prometheus/*.go`
- Coverage >= 80% for `internal/severity/*.go`
- Coverage >= 80% for modified `internal/tools/af_create_rr.go`
- Coverage >= 80% for modified `internal/tools/crd_tools.go`
- `golangci-lint run` reports 0 errors on all modified/new files
- No exported symbols from production packages used only in `_test.go`
- All 9 checkpoint categories satisfied at each tier boundary

### 6.2 Fail

- Any test fails under `-race`
- Coverage drops below 80% total (CI gate)
- Nil config/metrics/client causes panic (not graceful error)
- Internal URL/path visible in client-facing error text
- Metric defined but never incremented by production code
- Audit event documented but not emitted by code
- User input interpolated into PromQL query string
- LLM response accepted without validation against severity allowlist

---

## 7. Test Environment

- Go 1.25
- `httptest.NewServer` for mock Prometheus API
- `httptest.NewTLSServer` for TLS testing
- Mock `LLMTriager` interface for deterministic LLM responses
- `dynamicfake.NewSimpleDynamicClient` for K8s CRD simulation
- `github.com/prometheus/client_golang/prometheus/testutil` for metrics assertions
- Ginkgo v2 + Gomega (ADR-015)
- `-race` flag mandatory on all tests
- `logr.Discard()` or `funcr` logger for audit capture in tests

---

## 8. Implementation Phases

### Phase 1: TDD Red — Prometheus Client and PromQL Parsing

**Goal:** Write 22 failing tests (UT-AF-T-001 through UT-AF-T-022) that assert the Prometheus HTTP client and PromQL label extraction.

**Test file:** `internal/prometheus/client_test.go`, `internal/prometheus/rules_test.go`

**Fakes needed:**
- `httptest.NewServer` with canned Prometheus API responses (alerts, rules, query)
- `httptest.NewTLSServer` for TLS CA testing
- Table-driven PromQL expression fixtures (15+ patterns)

**Red criteria:** All 22 tests compile but fail (client/parser packages do not exist yet).

---

### Phase 2: TDD Green — Prometheus Client and PromQL Parsing

**Goal:** Implement `internal/prometheus/client.go` and `internal/prometheus/rules.go`.

**Files created:**
- NEW: `internal/prometheus/client.go` — HTTP client following kubernaut EM pattern
- NEW: `internal/prometheus/types.go` — Alert, Rule, RuleGroup, QueryResult types
- NEW: `internal/prometheus/rules.go` — `ExtractLabelMatchers`, `MatchesResource`

**Design decisions:**
- Follow kubernaut `prometheusHTTPClient` pattern: injected `*http.Client`, `baseURL`, context propagation
- Support TLS CA via `crypto/tls` + `x509.CertPool`
- Support bearer token via `RoundTripper` wrapper
- Wrap errors through `security.RedactError` before returning
- `ExtractLabelMatchers` uses `promql/parser.ParseExpr` + `parser.Inspect` for AST walking

**Green criteria:** All 22 Tier 1 tests pass. `-race` passes.

---

### Phase 3: TDD Refactor — Prometheus Client

**Checklist (100 Go Mistakes):**
- [ ] #1: Unintended variable shadowing in `Inspect` callback
- [ ] #5: Interface pollution — keep `PrometheusQuerier` interface minimal
- [ ] #12: Not using type assertion properly for AST node types
- [ ] #26: Slices and memory leaks — matchers slice not retained after function return
- [ ] #41: Not closing resources — `resp.Body.Close()` in all paths
- [ ] #53: Not handling defer errors — verify `resp.Body.Close()` error handling
- [ ] #73: Not using testing utility packages — verify table-driven tests use descriptive names

**Refactoring actions:**
- Extract HTTP request builder into helper to avoid repeated code
- Ensure `io.ReadAll` has bounded reader (max 10MB response) to prevent OOM
- Verify `ExtractLabelMatchers` handles all `parser.Node` subtypes

---

### CHECKPOINT 1 (after Tier 1)

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | No production metrics in Tier 1. Deferred to Checkpoint 4 (UT-AF-T-064..068). |
| 2 | Adversarial inputs | UT-AF-T-018 (invalid PromQL no panic), UT-AF-T-006 (malformed JSON). Full adversarial deferred to Checkpoint 5. |
| 3 | Resource bounds | HTTP response body bounded by `io.LimitReader` (max 10MB). |
| 4 | Concurrency | Prometheus client is stateless (injected `*http.Client` is concurrent-safe). No shared mutable state. |
| 5 | Nil/zero edge cases | UT-AF-T-003, 006, 009: error responses handled. Client constructor validates non-empty baseURL. |
| 6 | Error-path observability | UT-AF-T-022: errors redacted. Error messages include operation context ("querying alerts", "querying rules"). |
| 7 | Cross-phase integration | Client established; used by orchestrator in Tier 2. Verified by compilation. |
| 8 | Spec compliance | Prometheus HTTP API v1 spec: `/api/v1/alerts`, `/api/v1/rules`, `/api/v1/query` paths. Response `status` field checked. |
| 9 | API surface hygiene | `Client` type exported (used by `internal/severity`). `ExtractLabelMatchers`, `MatchesResource` exported. Internal helpers unexported. |

---

### Phase 4: TDD Red — Triage Orchestrator and Cache

**Goal:** Write 24 failing tests (UT-AF-T-023 through UT-AF-T-046) that assert the triage pipeline, severity ordering, cache, and degradation.

**Test file:** `internal/severity/triage_test.go`, `internal/severity/cache_test.go`

**Fakes needed:**
- Mock `PrometheusClient` interface returning canned alerts/rules/queries per tier scenario
- Mock `LLMTriager` interface returning canned severity/confidence
- Table-driven tier scenarios (T1 hit, T1.5 hit, T2 hit, T2.5 hit, T3 hit, full fallthrough)

**Red criteria:** All 24 tests compile but fail (orchestrator does not exist yet).

---

### Phase 5: TDD Green — Triage Orchestrator and Cache

**Goal:** Implement `internal/severity/triage.go`, `internal/severity/types.go`, `internal/severity/cache.go`.

**Files created:**
- NEW: `internal/severity/triage.go` — `Triager` struct with `Triage(ctx, TriageInput) (TriageResult, error)`
- NEW: `internal/severity/types.go` — `TriageResult`, `TriageInput`, `SeveritySource`, severity ordering
- NEW: `internal/severity/cache.go` — `RulesCache` with `sync.RWMutex`, TTL, max entries

**Design decisions:**
- `Triager` holds `PrometheusClient`, `LLMTriager`, `Config` — all injected
- Severity ordering via `severityRank` map: `{"critical": 5, "high": 4, "medium": 3, "low": 2, "info": 1}`
- Rate limit instant queries with counter per `Triage` call (not global)
- Tier errors logged and swallowed; fallthrough continues
- `RulesCache` uses `sync.RWMutex` (not `sync.Map`) for TTL + bounded size

**Green criteria:** All 46 tests pass (Tier 1 + Tier 2). `-race` passes.

---

### Phase 6: TDD Refactor — Triage Orchestrator

**Checklist (100 Go Mistakes):**
- [ ] #28: Maps and memory leaks — RulesCache bounded by max entries + TTL
- [ ] #29: Comparing values incorrectly — severity comparison via rank map, not string comparison
- [ ] #41: Not closing resources — context cancel in cache refresh
- [ ] #56: Concurrency safety of shared state — RulesCache uses RWMutex
- [ ] #62: Starting goroutine without knowing when to stop — no background goroutines in cache (lazy eviction)
- [ ] #78: Not using -race (mandatory)
- [ ] #90: Not being aware of parallel test impacts — each test creates isolated Triager

**Refactoring actions:**
- Extract tier execution into named methods (`runTier1`, `runTier15`, etc.) for clarity
- Verify no goroutine leaks in cache (lazy TTL, no background refresh)
- Ensure severity rank map is package-level `var` (not recreated per call)

---

### CHECKPOINT 2 (after Tier 2)

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | No production metrics in Tier 2. Deferred to Checkpoint 4. |
| 2 | Adversarial inputs | UT-AF-T-043 (empty labels), UT-AF-T-044 (>100 rules bounded). Full adversarial deferred to Checkpoint 5. |
| 3 | Resource bounds | UT-AF-T-042: 50 cache cycles, no unbounded growth. UT-AF-T-044: rule eval bounded to 100. |
| 4 | Concurrency | UT-AF-T-041: 10 goroutines on cache under `-race`. UT-AF-T-045: context propagation. |
| 5 | Nil/zero edge cases | UT-AF-T-037, 038: Prometheus errors at each tier. UT-AF-T-046: disabled triage. |
| 6 | Error-path observability | UT-AF-T-037, 038: errors logged with tier context. Tier fallthrough logged at INFO. |
| 7 | Cross-phase integration | Prometheus client (Tier 1) used by orchestrator (Tier 2). UT-AF-T-023..034 prove wiring. |
| 8 | Spec compliance | Prometheus API response `status` field validated. Severity values match kubernaut canonical set. |
| 9 | API surface hygiene | `Triager`, `TriageResult`, `TriageInput`, `SeveritySource` exported. `RulesCache` exported (used by config wiring). Internal helpers unexported. |

---

### Phase 7: TDD Red — LLM Integration

**Goal:** Write 10 failing tests (UT-AF-T-047 through UT-AF-T-056) for LLM triage.

**Test file:** `internal/severity/llm_test.go`

**Fakes needed:**
- Mock `LLMClient` interface with configurable response (severity, confidence, error)
- Prompt capture to verify content composition

**Red criteria:** All 10 tests compile but fail.

---

### Phase 8: TDD Green — LLM Integration

**Goal:** Implement `internal/severity/llm.go`.

**Files created:**
- NEW: `internal/severity/llm.go` — `LLMTriager` with `TriageWithRules(ctx, rules, input)` and `TriagePure(ctx, input)`

**Design decisions:**
- Use `google.golang.org/genai` client directly (consistent with existing codebase)
- Structured prompt with AlertManager severity definitions
- Parse response: extract severity string, validate against allowlist, check confidence
- Increment `af_llm_tokens_total{direction,model}` on each call
- Respect `MaxLLMConcurrency` via existing semaphore in ratelimit package

**Green criteria:** All 56 tests pass. `-race` passes.

---

### Phase 9: TDD Refactor — LLM Integration

**Checklist (100 Go Mistakes):**
- [ ] #5: Interface pollution — `LLMTriager` interface should be minimal (2 methods)
- [ ] #15: Missing code documentation — prompt template documented
- [ ] #45: Returning a nil receiver — LLM methods must not return typed nil interface
- [ ] #73: Testing utility packages — mock LLM captures prompt for assertion
- [ ] #90: Parallel test safety — mock LLM per test instance

---

### CHECKPOINT 3 (after LLM Integration)

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | UT-AF-T-053: `af_llm_tokens_total` incremented. Full triage metrics deferred to Checkpoint 4. |
| 2 | Adversarial inputs | UT-AF-T-050 (invalid LLM output), UT-AF-T-055 (no secrets in prompt). |
| 3 | Resource bounds | LLM response bounded by max response tokens (model limit). |
| 4 | Concurrency | UT-AF-T-054: respects `MaxLLMConcurrency`. LLM client is stateless. |
| 5 | Nil/zero edge cases | UT-AF-T-052: LLM error handled. UT-AF-T-051: below-threshold confidence. |
| 6 | Error-path observability | UT-AF-T-052: error logged with context. Redacted before audit emission. |
| 7 | Cross-phase integration | LLM triager wired into orchestrator Tier 2.5 and Tier 3. UT-AF-T-030, 032 prove wiring. |
| 8 | Spec compliance | Severity values validated against canonical allowlist. |
| 9 | API surface hygiene | `LLMTriager` interface exported. Implementation unexported. Prompt template in const (not exported). |

---

### Phase 10: TDD Red — Tool Integration and Observability

**Goal:** Write 18 failing tests (UT-AF-T-057 through UT-AF-T-074) for tool wiring, metrics, audit, and config.

**Test file:** `internal/tools/af_create_rr_test.go` (extended), `internal/tools/kubernaut_submit_signal_test.go` (extended), `internal/severity/metrics_test.go`, `internal/config/config_test.go` (extended)

**Red criteria:** All 18 tests compile but fail.

---

### Phase 11: TDD Green — Tool Integration and Observability

**Goal:** Wire triage into `HandleCreateRR` and `HandleSubmitSignal`. Add metrics, audit events, config.

**Files modified:**
- MODIFY: `internal/tools/af_create_rr.go` — Add triage call before RR creation
- MODIFY: `internal/tools/crd_tools.go` — Add triage call before SP creation
- MODIFY: `internal/metrics/metrics.go` — Add `SeverityTriageTotal`, `SeverityTriageDuration`, `SeverityTriageErrorsTotal`
- MODIFY: `internal/audit/audit.go` — Add `EventSeverityTriageCompleted`, `EventSeverityTriageFailed`
- MODIFY: `internal/config/config.go` — Add `SeverityTriage` config section
- MODIFY: `internal/handler/mcp_bridge.go` — Pass triager to tool handlers
- NEW: `deploy/prometheus-rules.yaml` — Add `ApifrontendSeverityTriageErrorRate` alert
- NEW: `docs/operations/runbooks/RB-AF-010.md` — Triage troubleshooting runbook

**Design decisions:**
- `HandleCreateRR` receives `Triager` as required parameter
- Triage invoked only when `args.Severity == ""` (BAC-T-17)
- On triage failure: propagate error to caller (no silent defaults per ADR-021)
- Metrics use existing registry pattern; new counters added to `Registry`
- Audit events emitted via `audit.EmitFromContext` pattern

**Green criteria:** All 74 tests pass. `-race` passes.

---

### Phase 12: TDD Refactor — Tool Integration

**Checklist (100 Go Mistakes):**
- [ ] #1: Variable shadowing in triage result handling
- [ ] #11: Functional options pattern — Triager as optional dependency
- [ ] #15: Code documentation — triage integration documented
- [ ] #41: Resource closure — triage context respected
- [ ] #78: -race mandatory

**Refactoring actions:**
- Extract triage-then-create logic into helper to avoid duplication between RR and SP
- Verify `signalLabels` map construction does not share references

---

### CHECKPOINT 4 (after Tool Integration)

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | **CRITICAL**: UT-AF-T-064..068 verify all 3 new counters + 1 histogram per tier. UT-AF-T-053 verifies LLM token counter. Every defined metric has production caller. |
| 2 | Adversarial inputs | UT-AF-T-075..078 deferred to Checkpoint 5. Tool-level validation already in `HandleCreateRR` / `HandleSubmitSignal`. |
| 3 | Resource bounds | Cache bounded (UT-AF-T-042). Instant queries bounded (UT-AF-T-036, 044). |
| 4 | Concurrency | UT-AF-T-041 (cache), UT-AF-T-084 (concurrent triage). All under `-race`. |
| 5 | Nil/zero edge cases | UT-AF-T-063 (triage failure default), UT-AF-T-085 (nil triager), UT-AF-T-086 (nil metrics). |
| 6 | Error-path observability | UT-AF-T-069..071: audit events include severity_source, tier, duration. UT-AF-T-070: failed triage audit redacted. |
| 7 | Cross-phase integration | **KEY**: Triage (Tier 2) wired into tool handlers (Tier 4). UT-AF-T-057, 061 prove end-to-end: empty severity → triage → RR/SP with derived severity. Metrics (Tier 4) incremented by orchestrator (Tier 2). |
| 8 | Spec compliance | Severity values in `signalLabels` match canonical kubernaut set. Prometheus naming: `af_severity_triage_*`. |
| 9 | API surface hygiene | No test helpers exported. Config fields exported (needed by main.go). |

---

### Phase 13: TDD Red — Adversarial Inputs

**Goal:** Write 12 failing tests (UT-AF-T-075 through UT-AF-T-086) for edge cases and adversarial inputs.

**Test file:** `internal/severity/triage_test.go` (adversarial section), `internal/tools/af_create_rr_test.go` (extended)

**Red criteria:** All 12 tests compile but fail.

---

### Phase 14: TDD Green — Adversarial Inputs

**Goal:** Add input validation, bounds checking, and nil-safety guards.

**Implementation:**
- Add resource label validation before Prometheus queries (empty namespace → skip)
- Add rule count bound (max 100 rules evaluated)
- Add LLM response normalization (lowercase, trim whitespace)
- Add nil triager / nil metrics guards in tool handlers

**Green criteria:** All 86 tests pass. `-race` passes.

---

### Phase 15: TDD Refactor — Adversarial Inputs

**Checklist (100 Go Mistakes):**
- [ ] #28: Maps and memory leaks — verify no user input as map keys
- [ ] #45: Returning a nil receiver — validation returns concrete errors
- [ ] #73: Testing utility packages — table-driven adversarial tests
- [ ] #90: Parallel test safety — isolated per test

---

### CHECKPOINT 5 (after Adversarial Inputs)

| # | Category | Verification |
|---|----------|-------------|
| 1 | Observability wiring | All counters + histograms verified (Checkpoint 4). Adversarial rejection increments error counter (UT-AF-T-068). |
| 2 | Adversarial inputs | **COMPLETE**: UT-AF-T-075..086 cover empty labels, path traversal, max-length, NUL bytes, malformed JSON, unbounded rules, Unicode PromQL, invalid LLM output, case normalization, concurrent triage, nil triager, nil metrics. |
| 3 | Resource bounds | UT-AF-T-042 (cache cycles), UT-AF-T-044/080 (rule count bounded), HTTP response body bounded. |
| 4 | Concurrency | UT-AF-T-041 (cache), UT-AF-T-084 (10 goroutines triage). All under `-race`. |
| 5 | Nil/zero edge cases | UT-AF-T-085 (nil triager), UT-AF-T-086 (nil metrics), UT-AF-T-082 (empty LLM), UT-AF-T-075 (empty labels). |
| 6 | Error-path observability | All error paths emit audit events with sufficient context. UT-AF-T-070 proves redaction. |
| 7 | Cross-phase integration | Full pipeline verified: empty severity → triage → RR with derived severity + signalLabels. |
| 8 | Spec compliance | Severity values canonical. Prometheus API paths correct. K8s RFC 1123 for resource names (inherited from existing validators). |
| 9 | API surface hygiene | All internal helpers unexported. No debug functions exported. Table-driven test data in `_test.go` only. |

---

### Phase 16: Final Lint, Coverage, and Documentation

**Actions:**
- Run full `golangci-lint` on all modified/new files
- Verify 80% coverage gate per package
- Check for exported symbols only used in tests
- Create runbook `RB-AF-010.md`
- Update `deploy/prometheus-rules.yaml` with triage alert
- Update `CHANGELOG.md`
- Update `docs/design/ARCHITECTURE.md` metrics catalog (section 7)
- Final 100 Go Mistakes scan across all new code

---

### FINAL CHECKPOINT (pre-PR)

| # | Category | Final Verification |
|---|----------|--------------------|
| 1 | Observability wiring | 3 new counters + 1 histogram + existing LLM token counter all have production callers. UT-AF-T-064..068, 053 prove increment per tier/outcome. |
| 2 | Adversarial inputs | 12 dedicated tests (UT-AF-T-075..086) + PromQL parser edge cases (UT-AF-T-015..018). All input boundaries verified. |
| 3 | Resource bounds | Cache bounded (50 cycles), rules bounded (100 max), HTTP body bounded (10MB), queries bounded (10 per triage). |
| 4 | Concurrency | Cache, triage, LLM all tested under `-race` with 10+ goroutines. |
| 5 | Nil/zero edge cases | Nil triager, nil metrics, nil LLM client, empty labels, empty severity, zero config defaults — all tested. |
| 6 | Error-path observability | All error paths include tool name, tier, resource context for SRE diagnosis. Audit events redacted. |
| 7 | Cross-phase integration | UT-AF-T-057, 061 prove full chain: MCP tool call → triage → RR/SP creation → metrics + audit. |
| 8 | Spec compliance | Prometheus HTTP API v1, canonical severity values, `af_` metric prefix, K8s RFC 1123 (inherited). |
| 9 | API surface hygiene | All internal helpers unexported. `go vet` clean. No test-only exports. |

---

## 9. Traceability Matrix

| BAC | Test Cases | Count |
|-----|-----------|-------|
| BAC-T-01 | UT-AF-T-057, 061, 084 | 3 |
| BAC-T-02 | UT-AF-T-001, 002, 023, 024, 035 | 5 |
| BAC-T-03 | UT-AF-T-005, 025, 026, 027 | 4 |
| BAC-T-04 | UT-AF-T-007, 008, 013-021, 028, 081 | 13 |
| BAC-T-05 | UT-AF-T-029, 030, 033, 047 | 4 |
| BAC-T-06 | UT-AF-T-031, 032, 034, 043, 048, 075 | 6 |
| BAC-T-07 | UT-AF-T-059, 060, 071 | 3 |
| BAC-T-08 | UT-AF-T-052, 069, 070 | 3 |
| BAC-T-09 | UT-AF-T-053, 064-068, 086 | 7 |
| BAC-T-10 | UT-AF-T-003, 004, 006, 009, 012, 037, 038, 045, 056, 079, 085 | 11 |
| BAC-T-11 | UT-AF-T-055, 076, 077, 078 | 4 |
| BAC-T-12 | UT-AF-T-049, 050, 082, 083 | 4 |
| BAC-T-13 | UT-AF-T-039, 040, 041, 042, 072 | 5 |
| BAC-T-14 | UT-AF-T-036, 044, 054, 080 | 4 |
| BAC-T-15 | UT-AF-T-010, 011, 073 | 3 |
| BAC-T-16 | UT-AF-T-022 | 1 |
| BAC-T-17 | UT-AF-T-046, 058, 062 | 3 |
| BAC-T-18 | UT-AF-T-051, 063, 074 | 3 |

---

## 10. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| Prometheus client tests | `internal/prometheus/client_test.go` |
| PromQL rules tests | `internal/prometheus/rules_test.go` |
| Triage orchestrator tests | `internal/severity/triage_test.go` |
| Cache tests | `internal/severity/cache_test.go` |
| LLM triage tests | `internal/severity/llm_test.go` |
| Extended af_create_rr tests | `internal/tools/af_create_rr_test.go` |
| Extended submit_signal tests | `internal/tools/kubernaut_submit_signal_test.go` |
| Config tests (extended) | `internal/config/config_test.go` |
| This test plan | `docs/tests/92/test_plan.md` |

---

## 11. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| `promql/parser` AST incompatibility with new PromQL features | Low | Medium | Pin to go.mod version; test 15+ expression patterns |
| LLM response non-determinism in tests | Medium | Low | Mock `LLMTriager` interface; no real LLM calls in UT |
| Prometheus client timeout flakiness in CI | Medium | Low | Use `httptest.Server` with deterministic delays |
| Cache TTL race in fast CI environments | Low | Low | Use time-based assertions with tolerance margins |
| Large `go.mod` change from promoting prometheus/prometheus | Low | Low | Already in go.mod; no new dependency |
| Triage adding latency to RR creation path | Medium | Medium | Triage timeout separate from tool timeout; max 10 queries |
