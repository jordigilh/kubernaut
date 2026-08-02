# Metrics & SLOs

**Version**: v3.0
**Status**: ✅ Corrected for [#1806](https://github.com/jordigilh/kubernaut/issues/1806) (HAPI → Kubernaut Agent rename/rewrite)
**Last Updated**: 2026-08-02

---

## 📋 Changelog

| Version | Date | Changes |
|---------|------|---------|
| v3.0 | 2026-08-02 | **#1806 CORRECTION**: Full rewrite. Rebuilt the metrics table, dashboard, alerts, and queries around the 4 metrics that actually exist in [`pkg/aianalysis/metrics/metrics.go`](../../../../pkg/aianalysis/metrics/metrics.go) (`aianalysis_rego_evaluations_total`, `aianalysis_approval_decisions_total`, `aianalysis_confidence_score_distribution`, `aianalysis_failures_total`). Removed all `aianalysis_holmesgpt_*`, `aianalysis_investigation_*`, and `aianalysis_workflow_*` metrics — none of these exist in code (client-side KA call metrics were intentionally removed; see Scope Note below). Replaced the 60-second-sync-call latency SLOs with SLOs grounded in the real async submit/poll/result session model (25-minute wall-clock cap, BR-AA-HAPI-064) |
| v2.0 | 2025-11-30 | V1.0 ALIGNMENT: Updated approval metrics to signaling pattern (no AIApprovalRequest CRD); Added DetectedLabels/CustomLabels metrics; Updated phase names |
| v1.0 | 2025-10-15 | Initial specification |

---

## ⚠️ Scope Note: This Service Does Not Export Latency Metrics

AIAnalysis's own Prometheus metrics are **decision/outcome-oriented, not latency-oriented**. Per the `METRIC SELECTION CRITERIA (v1.13)` comment at the top of `pkg/aianalysis/metrics/metrics.go`:

- Only business-value metrics are exported (policy outcomes, approval decisions, AI confidence, failure taxonomy).
- Operational/debugging metrics (phase-transition timings, latency breakdowns) were **removed**.
- **Client-side Kubernaut Agent (KA) call metrics were removed** — KA tracks its own server-side metrics (e.g. `kubernaut_agent_llm_token_usage_total`). AIAnalysis does not export `aianalysis_holmesgpt_requests_total`, `aianalysis_holmesgpt_request_duration_seconds`, or any similar family — they do not exist.
- There is no `aianalysis_investigation_duration_seconds` metric either.

If you need per-investigation timing, use `AIAnalysis.status.investigationTime` (a CRD status field populated once per completed investigation, not a Prometheus time series) or correlate via `status.investigationId` / `status.investigationSession` against Kubernaut Agent's own metrics.

---

## SLI/SLO Definitions

### Service Level Indicators (SLIs)

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **Rego Policy Reliability** | `1 - (rego_evaluations{degraded="true"} / rego_evaluations_total)` | ≥99% | Policy engine availability (BR-AI-014 graceful degradation) |
| **Auto-Approval Rate** | `approval_decisions{decision="auto_approved"} / approval_decisions_total` | 40-60% | Rego policy effectiveness (BR-AI-059) |
| **AI Confidence (Avg)** | `avg(aianalysis_confidence_score_distribution)` | ≥0.80 | High-quality workflow selection (BR-AI-OBSERVABILITY-004) |
| **Failure Rate** | `sum(rate(aianalysis_failures_total[1h]))` | Trend-monitored (no fleet-wide fixed target — depends on cluster alert volume) | Overall investigation health (BR-KA-197) |
| **TransientError Share** | `failures{reason="TransientError"} / failures_total` | Trend-monitored | Async KA session health — covers both retry-exhaustion and the 25-minute session timeout cap (#1078, BR-AA-HAPI-064) |

> **Note on denominators**: There is no "total investigations started" counter (client-side KA call counters were deliberately removed — see Scope Note). `aianalysis_confidence_score_distribution`'s sample count and `aianalysis_failures_total`'s sum are the closest available proxies for "successful" vs. "failed" investigation volume respectively (see `recordPhaseMetrics` in `internal/controller/aianalysis/metrics_recorder.go`), so ratio SLIs above use each metric family's own total as the denominator rather than a global count.

### Service Level Objectives (SLOs)

```yaml
slos:
  - name: "AIAnalysis Rego Policy Reliability"
    sli: "1 - (sum(rate(aianalysis_rego_evaluations_total{degraded=\"true\"}[1h])) / sum(rate(aianalysis_rego_evaluations_total[1h])))"
    target: 0.99  # 99% non-degraded evaluations
    window: "30d"
    burn_rate_fast: 14.4  # 1h window
    burn_rate_slow: 6     # 6h window

  - name: "AIAnalysis Auto-Approval Rate"
    sli: "sum(rate(aianalysis_approval_decisions_total{decision=\"auto_approved\"}[1h])) / sum(rate(aianalysis_approval_decisions_total[1h]))"
    target_band: [0.40, 0.60]  # Operational band, not a pass/fail SLO
    window: "30d"

  - name: "AIAnalysis Average Confidence"
    sli: "avg(aianalysis_confidence_score_distribution)"
    target: 0.80  # avg >= 80%
    window: "30d"
```

---

## Service-Specific Metrics

**Source of truth**: [`pkg/aianalysis/metrics/metrics.go`](../../../../pkg/aianalysis/metrics/metrics.go) — these are the **only 4** metrics this service exports. Do not add speculative metrics here without updating that file first (DD-METRICS-001: dependency-injected metrics pattern).

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
)

var (
    // RegoEvaluationsTotal tracks Rego policy evaluation outcomes (BR-AI-030)
    RegoEvaluationsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_rego_evaluations_total",
            Help: "Total number of Rego policy evaluations",
        },
        []string{"outcome", "degraded"}, // outcome: auto_approved|requires_approval|error
    )

    // ApprovalDecisionsTotal tracks approval vs. auto-execute decisions (BR-AI-059)
    ApprovalDecisionsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_approval_decisions_total",
            Help: "Total number of approval decisions",
        },
        []string{"decision", "environment"}, // decision: auto_approved|requires_approval
    )

    // ConfidenceScoreDistribution tracks AI confidence score distribution (BR-AI-OBSERVABILITY-004)
    ConfidenceScoreDistribution = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_confidence_score_distribution",
            Help:    "Distribution of AI confidence scores",
            Buckets: []float64{0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1.0},
        },
        []string{"signal_type"}, // e.g. OOMKilled, CrashLoopBackOff, NodeNotReady
    )

    // FailuresTotal tracks AIAnalysis failures by reason and sub-reason (BR-KA-197)
    FailuresTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_failures_total",
            Help: "Total number of AIAnalysis failures",
        },
        []string{"reason", "sub_reason"},
    )
)
```

**Recording call sites** (for building accurate dashboards/alerts):

| Metric | Recorded when | Call site |
|--------|---------------|-----------|
| `aianalysis_rego_evaluations_total` | Every Rego policy evaluation during the Analyzing phase | `pkg/aianalysis/handlers/analyzing.go` |
| `aianalysis_approval_decisions_total` | Every approval decision during the Analyzing phase | `pkg/aianalysis/handlers/analyzing.go` |
| `aianalysis_confidence_score_distribution` | Once per `Completed` analysis that has a `selectedWorkflow` | `internal/controller/aianalysis/metrics_recorder.go` (`recordPhaseMetrics`) |
| `aianalysis_failures_total` | Once per reconcile where `status.phase == Failed` | `internal/controller/aianalysis/metrics_recorder.go` (`recordPhaseMetrics`) |

---

## Grafana Dashboard JSON

**AIAnalysis Controller Dashboard** (key panels):

```json
{
  "dashboard": {
    "title": "AIAnalysis Controller - Observability",
    "uid": "aianalysis-controller",
    "tags": ["kubernaut", "aianalysis", "ai-ml", "controller"],
    "panels": [
      {
        "id": 1,
        "title": "Rego Evaluation Degraded Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(aianalysis_rego_evaluations_total{degraded='true'}[5m])) / sum(rate(aianalysis_rego_evaluations_total[5m]))",
            "legendFormat": "Degraded Rate"
          }
        ],
        "thresholds": [{"value": 0.01, "colorMode": "warning", "op": "gt"}]
      },
      {
        "id": 2,
        "title": "Approval Decisions (Auto vs. Manual)",
        "type": "graph",
        "targets": [
          {"expr": "sum(rate(aianalysis_approval_decisions_total{decision='auto_approved'}[5m])) / sum(rate(aianalysis_approval_decisions_total[5m]))", "legendFormat": "Auto-Approved %"}
        ]
      },
      {
        "id": 3,
        "title": "Confidence Score Distribution",
        "type": "heatmap",
        "targets": [
          {"expr": "sum(rate(aianalysis_confidence_score_distribution_bucket[5m])) by (le)"}
        ]
      },
      {
        "id": 4,
        "title": "Average Confidence by Signal Type",
        "type": "graph",
        "targets": [
          {"expr": "sum(rate(aianalysis_confidence_score_distribution_sum[5m])) by (signal_type) / sum(rate(aianalysis_confidence_score_distribution_count[5m])) by (signal_type)", "legendFormat": "{{signal_type}}"}
        ]
      },
      {
        "id": 5,
        "title": "Failures by Reason",
        "type": "graph",
        "targets": [
          {"expr": "sum(rate(aianalysis_failures_total[5m])) by (reason)", "legendFormat": "{{reason}}"}
        ]
      },
      {
        "id": 6,
        "title": "Failures by Sub-Reason (Top 10)",
        "type": "table",
        "targets": [
          {"expr": "topk(10, sum(rate(aianalysis_failures_total[1h])) by (reason, sub_reason))"}
        ]
      }
    ]
  }
}
```

---

## Alert Rules YAML

```yaml
groups:
- name: aianalysis-slos
  interval: 30s
  rules:
  # SLO: Rego Policy Evaluation Reliability
  - alert: AIAnalysisRegoDegradedRateHigh
    expr: |
      (
        sum(rate(aianalysis_rego_evaluations_total{degraded="true"}[1h])) /
        sum(rate(aianalysis_rego_evaluations_total[1h]))
      ) > 0.01
    for: 15m
    labels:
      severity: warning
      slo: rego_policy_reliability
    annotations:
      summary: "AIAnalysis Rego policy evaluations degrading above SLO"
      description: "{{ $value | humanizePercentage }} of Rego evaluations are falling back to degraded/manual-approval defaults (BR-AI-014), above the 1% SLO"

  # Operational: Low Average Confidence
  - alert: AIAnalysisLowConfidence
    expr: |
      avg(aianalysis_confidence_score_distribution) < 0.80
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "AI workflow selections have low average confidence"
      description: "Average confidence score is {{ $value }}, below the 0.80 threshold"

  # Operational: High Manual Approval Rate
  - alert: AIAnalysisHighManualApprovalRate
    expr: |
      (
        sum(rate(aianalysis_approval_decisions_total{decision="requires_approval"}[1h])) /
        sum(rate(aianalysis_approval_decisions_total[1h]))
      ) > 0.60
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "High manual approval rate detected"
      description: "{{ $value | humanizePercentage }} of analyses require manual approval (expected operational band: 40-60%)"

  # Failure taxonomy: Elevated overall failure rate
  - alert: AIAnalysisElevatedFailureRate
    expr: |
      sum(rate(aianalysis_failures_total[1h])) > 5
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "AIAnalysis failure rate elevated"
      description: "{{ $value }} failures/hour across all reasons — inspect `sum by (reason, sub_reason) (aianalysis_failures_total)` for the dominant cause"

  # Async session health: Transient/timeout failures (#1078, BR-AA-HAPI-064)
  - alert: AIAnalysisTransientFailuresElevated
    expr: |
      sum(rate(aianalysis_failures_total{reason="TransientError"}[1h])) > 2
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "Elevated TransientError failures (KA session issues or 25-minute timeout cap)"
      description: "{{ $value }} TransientError failures/hour — check Kubernaut Agent (KA) availability and whether investigations are hitting the DefaultMaxInvestigationDuration (25m) wall-clock cap"
```

---

## Query Examples

### AI/ML-Specific Queries

```promql
# 1. Rego Policy Evaluation Degraded Rate
sum(rate(aianalysis_rego_evaluations_total{degraded="true"}[5m])) /
sum(rate(aianalysis_rego_evaluations_total[5m]))

# 2. Average AI Confidence Score
avg(aianalysis_confidence_score_distribution)

# 3. Auto-Approval Rate
sum(rate(aianalysis_approval_decisions_total{decision="auto_approved"}[5m])) /
sum(rate(aianalysis_approval_decisions_total[5m]))

# 4. Approval Decisions by Environment
sum by (environment) (rate(aianalysis_approval_decisions_total[5m]))

# 5. Confidence Score P50/P95 by Signal Type
histogram_quantile(0.50, sum(rate(aianalysis_confidence_score_distribution_bucket[5m])) by (le, signal_type))
histogram_quantile(0.95, sum(rate(aianalysis_confidence_score_distribution_bucket[5m])) by (le, signal_type))

# 6. Failure Rate by Reason
sum by (reason) (rate(aianalysis_failures_total[5m]))

# 7. Failure Rate by Reason + SubReason (drill-down)
sum by (reason, sub_reason) (rate(aianalysis_failures_total[5m]))

# 8. TransientError Share of All Failures (async session/timeout health signal)
sum(rate(aianalysis_failures_total{reason="TransientError"}[1h])) /
sum(rate(aianalysis_failures_total[1h]))
```

---

## References

- [Observability & Logging](./observability-logging.md) - Logging patterns
- [pkg/aianalysis/metrics/metrics.go](../../../../pkg/aianalysis/metrics/metrics.go) - Authoritative metric definitions (source of truth)
- [pkg/aianalysis/handlers/constants.go](../../../../pkg/aianalysis/handlers/constants.go) - `DefaultMaxInvestigationDuration` (25-minute session cap), retry/backoff constants
- [REGO_POLICY_EXAMPLES.md](./REGO_POLICY_EXAMPLES.md) - Approval policy input schema
