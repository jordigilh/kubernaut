# Kubernaut Agent - Metrics & SLOs

**Version**: 1.0
**Last Updated**: 2026-08-02
**Status**: ✅ Current

---

## Purpose

Documents KA's Prometheus metrics surface, exposed on port `9090` (`/metrics`). Grounded directly
in the metric-name constants defined in source, not the "6 metrics files" characterization used
during planning — two of those files are unrelated to KA's own Prometheus export (see
[Note on File Naming](#note-on-file-naming) below).

---

## Metric Namespaces

KA exposes two distinct Prometheus namespaces:

| Namespace | Subsystem | Source | Purpose |
|---|---|---|---|
| `aiagent_*` | (none) | `internal/kubernautagent/metrics/metrics.go` | Core service metrics: sessions, HTTP, authz, audit, interactive MCP, LLM retries |
| `aiagent_alignment_*` | `alignment` | `internal/kubernautagent/alignment/metrics.go` | Shadow Agent (prompt-injection guardrail) subsystem — see [ADR-KA-001](../../architecture/decisions/ADR-KA-001-shadow-agent-alignment-check.md) |

Both namespaces were unified onto the `aiagent` prefix (superseding an earlier, inconsistent
`kubernaut_alignment_*` naming for the alignment subsystem).

---

## Core Metrics (`aiagent_*`)

| Metric | Type | Purpose |
|---|---|---|
| `aiagent_sessions_started_total` | Counter | Investigations submitted |
| `aiagent_sessions_completed_total` | Counter | Investigations completed (by outcome) |
| `aiagent_sessions_active` | Gauge | In-flight investigations (admission-control signal) |
| `aiagent_session_duration_seconds` | Histogram | End-to-end investigation duration |
| `aiagent_http_rate_limited_total` | Counter | Requests rejected by rate limiting |
| `aiagent_http_request_duration_seconds` | Histogram | HTTP request latency (excludes `/stream` — SSE connections are long-lived and would skew P99 to minutes) |
| `aiagent_http_requests_in_flight` | Gauge | Concurrent HTTP requests |
| `aiagent_authz_denied_total` | Counter | SAR authorization denials ([DD-AUTH-014](../../architecture/decisions/DD-AUTH-014-middleware-based-sar-authentication.md)) |
| `aiagent_audit_events_emitted_total` | Counter | Audit events successfully forwarded to Data Storage |
| `aiagent_mcp_interactive_sessions_active` | Gauge | Active Interactive MCP Mode sessions |
| `aiagent_mcp_interactive_command_duration_seconds` | Histogram | Interactive command latency |
| `aiagent_mcp_interactive_takeover_total` | Counter | Operator takeover events |
| `aiagent_mcp_interactive_lease_contention_total` | Counter | Kubernetes Lease contention during session ownership negotiation |
| `aiagent_llm_call_retries_total` | Counter | LLM call retries |

## Shadow Agent Metrics (`aiagent_alignment_*`)

| Metric | Type | Purpose |
|---|---|---|
| `aiagent_alignment_verdict_total` | Counter | Shadow Agent verdicts issued (by outcome) |
| `aiagent_alignment_step_total` | Counter | Alignment-check steps executed |
| `aiagent_alignment_canary_total` | Counter | Canary integrity checks |
| `aiagent_alignment_verdict_duration_seconds` | Histogram | Verdict latency |
| `aiagent_alignment_shadow_audit_total` | Counter | Shadow-mode audit events emitted |
| `aiagent_alignment_circuit_breaker_total` | Counter | Circuit breaker trips (halts investigation on suspicious content) |
| `aiagent_alignment_grounding_total` | Counter | Full-context grounding reviews |
| `aiagent_alignment_grounding_duration_seconds` | Histogram | Grounding review latency |

See [shadow-agent-configuration.md](./shadow-agent-configuration.md) for `enforce`/`monitor` mode
semantics and how these metrics map to operator-facing behavior.

---

## Note on File Naming

Two files sometimes grouped with "KA's metrics files" are **not** Prometheus exporters:

- `internal/kubernautagent/investigator/investigation_metrics.go` — an in-memory
  `InvestigationMetrics` accumulator (LLM turn / tool call counts) surfaced in the structured
  decision payload, not exposed via `/metrics`.
- `pkg/kubernautagent/tools/k8s/metrics.go` — implements the `kubectl_top_pods`/`kubectl_top_nodes`
  **tool** (a Kubernetes-toolset tool that queries the cluster's own Metrics API), unrelated to
  KA's own Prometheus surface.

`internal/kubernautagent/server/http_metrics.go` and `internal/kubernautagent/mcp/tools/metrics.go`
are wiring/interface layers (middleware, an interface abstraction) over the `aiagent_*` collector
defined in `internal/kubernautagent/metrics/metrics.go` — they record into it but define no metric
names of their own.

---

## SLIs / SLOs

KA does not currently publish formal, numerically-committed SLOs. The following are the SLIs
available from the metrics above for operators to build their own alerting/dashboards on:

| SLI | Metric | Notes |
|---|---|---|
| Investigation latency | `aiagent_session_duration_seconds` (p50/p95/p99) | Dominated by LLM round-trip time; varies significantly by provider/model |
| HTTP availability | `aiagent_http_request_duration_seconds` + standard error-rate derivation from access logs | |
| Admission pressure | `aiagent_sessions_active` vs. configured `maxConcurrent` | |
| Authz health | `aiagent_authz_denied_total` rate | Spikes indicate RBAC misconfiguration, not necessarily attacks |
| Audit pipeline health | `aiagent_audit_events_emitted_total` vs. expected event volume | Gaps risk SOC2 CC8.1 reconstruction completeness |
| Shadow Agent health | `aiagent_alignment_circuit_breaker_total` rate | Sustained trips indicate either genuine prompt-injection attempts or an overly strict `enforce`-mode threshold |

---

## Related Documentation

- [observability-logging.md](./observability-logging.md) — structured logging and correlation IDs
- [security/AUDIT_EVENT_CATALOG.md](./security/AUDIT_EVENT_CATALOG.md) — audit event catalog
- [shadow-agent-configuration.md](./shadow-agent-configuration.md) — Shadow Agent operational guide
- [testing-strategy.md](./testing-strategy.md) — how these metrics are tested
