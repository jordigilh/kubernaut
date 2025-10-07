## Enhanced Metrics & SLOs

### SLI/SLO Definitions

**Service Level Indicators (SLIs)**:

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **Investigation Success Rate** | `successful_investigations / total_investigations` | ≥95% | HolmesGPT analysis reliability |
| **Investigation Latency (P95)** | `histogram_quantile(0.95, aianalysis_investigation_duration_seconds)` | <60s | Fast root cause identification |
| **HolmesGPT Availability** | `holmesgpt_requests_success / holmesgpt_requests_total` | ≥99% | AI service dependency |
| **Auto-Approval Rate** | `auto_approved_total / total_approvals` | 40-60% | Rego policy effectiveness |
| **Recommendation Confidence (Avg)** | `avg(aianalysis_recommendation_confidence)` | ≥0.85 | High-quality AI recommendations |

**Service Level Objectives (SLOs)**:

```yaml
slos:
  - name: "AIAnalysis Investigation Success Rate"
    sli: "successful_investigations / total_investigations"
    target: 0.95  # 95%
    window: "30d"
    burn_rate_fast: 14.4  # 1h window
    burn_rate_slow: 6     # 6h window

  - name: "AIAnalysis P95 Investigation Latency"
    sli: "histogram_quantile(0.95, aianalysis_investigation_duration_seconds)"
    target: 60  # 60 seconds
    window: "30d"

  - name: "HolmesGPT Availability"
    sli: "holmesgpt_requests_success / holmesgpt_requests_total"
    target: 0.99  # 99%
    window: "30d"
```

---

### Service-Specific Metrics

**AI/ML Metrics**:

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // HolmesGPT request metrics
    holmesgptRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_holmesgpt_requests_total",
            Help: "Total number of requests to HolmesGPT API",
        },
        []string{"status", "endpoint"},  // status: success|error, endpoint: investigate|analyze
    )

    holmesgptRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_holmesgpt_request_duration_seconds",
            Help:    "HolmesGPT API request duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.5, 2, 8),  // 0.5s to 128s
        },
        []string{"endpoint"},
    )

    // Investigation metrics
    investigationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_investigation_total",
            Help: "Total number of AI investigations",
        },
        []string{"status"},  // status: success|error
    )

    investigationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "aianalysis_investigation_duration_seconds",
            Help:    "AI investigation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(1, 2, 10),  // 1s to 1024s
        },
    )

    // Recommendation metrics
    recommendationConfidence = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "aianalysis_recommendation_confidence",
            Help:    "AI recommendation confidence score (0-1)",
            Buckets: prometheus.LinearBuckets(0.7, 0.05, 7),  // 0.7 to 1.0
        },
    )

    recommendationCount = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_recommendation_count",
            Help:    "Number of recommendations generated per analysis",
            Buckets: prometheus.LinearBuckets(1, 1, 10),  // 1 to 10
        },
        []string{"confidence_tier"},  // high|medium|low
    )

    // Approval metrics
    approvalTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_approval_total",
            Help: "Total number of approval decisions",
        },
        []string{"decision", "type"},  // decision: approved|rejected, type: auto|manual
    )

    approvalDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_approval_duration_seconds",
            Help:    "Approval decision duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 8),  // 0.1s to 25.6s
        },
        []string{"type"},  // auto|manual
    )

    // Rego policy metrics
    regoPolicyEvaluationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "aianalysis_rego_policy_evaluation_duration_seconds",
            Help:    "Rego policy evaluation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),  // 1ms to 1s
        },
    )

    regoPolicyEvaluationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_rego_policy_evaluation_total",
            Help: "Total number of Rego policy evaluations",
        },
        []string{"result"},  // auto_approve|manual_approval_required|reject
    )
)
```

---

### Grafana Dashboard JSON

**AIAnalysis Controller Dashboard** (key panels):

```json
{
  "dashboard": {
    "title": "AIAnalysis Controller - AI/ML Observability",
    "uid": "aianalysis-controller",
    "tags": ["kubernaut", "aianalysis", "ai-ml", "controller"],
    "panels": [
      {
        "id": 1,
        "title": "Investigation Success Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(aianalysis_investigation_total{status='success'}[5m])) / sum(rate(aianalysis_investigation_total[5m]))",
            "legendFormat": "Success Rate"
          }
        ],
        "thresholds": [{"value": 0.95, "colorMode": "critical", "op": "lt"}]
      },
      {
        "id": 2,
        "title": "HolmesGPT Latency (P50, P95, P99)",
        "type": "graph",
        "targets": [
          {"expr": "histogram_quantile(0.50, rate(aianalysis_holmesgpt_request_duration_seconds_bucket[5m]))", "legendFormat": "P50"},
          {"expr": "histogram_quantile(0.95, rate(aianalysis_holmesgpt_request_duration_seconds_bucket[5m]))", "legendFormat": "P95 (SLI)"},
          {"expr": "histogram_quantile(0.99, rate(aianalysis_holmesgpt_request_duration_seconds_bucket[5m]))", "legendFormat": "P99"}
        ],
        "thresholds": [{"value": 60, "colorMode": "critical", "op": "gt"}]
      },
      {
        "id": 3,
        "title": "Recommendation Confidence Distribution",
        "type": "heatmap",
        "targets": [
          {"expr": "sum(rate(aianalysis_recommendation_confidence_bucket[5m])) by (le)"}
        ]
      },
      {
        "id": 4,
        "title": "Auto-Approval Rate",
        "type": "graph",
        "targets": [
          {"expr": "sum(rate(aianalysis_approval_total{type='auto'}[5m])) / sum(rate(aianalysis_approval_total[5m]))", "legendFormat": "Auto-Approval %"}
        ]
      },
      {
        "id": 5,
        "title": "Approval Decision Breakdown",
        "type": "piechart",
        "targets": [
          {"expr": "sum by (decision, type) (aianalysis_approval_total)"}
        ]
      },
      {
        "id": 6,
        "title": "Rego Policy Evaluation Performance",
        "type": "graph",
        "targets": [
          {"expr": "histogram_quantile(0.95, rate(aianalysis_rego_policy_evaluation_duration_seconds_bucket[5m]))", "legendFormat": "P95"}
        ]
      }
    ]
  }
}
```

---

### Alert Rules YAML

```yaml
groups:
- name: aianalysis-slos
  interval: 30s
  rules:
  # SLO: Investigation Success Rate
  - alert: AIAnalysisInvestigationSLOBreach
    expr: |
      (
        sum(rate(aianalysis_investigation_total{status="success"}[1h])) /
        sum(rate(aianalysis_investigation_total[1h]))
      ) < 0.95
    for: 5m
    labels:
      severity: critical
      slo: investigation_success_rate
    annotations:
      summary: "AIAnalysis investigation success rate below SLO"
      description: "Investigation success rate is {{ $value | humanizePercentage }}, below 95% SLO"

  # SLO: HolmesGPT Availability
  - alert: HolmesGPTAvailabilitySLOBreach
    expr: |
      (
        sum(rate(aianalysis_holmesgpt_requests_total{status="success"}[1h])) /
        sum(rate(aianalysis_holmesgpt_requests_total[1h]))
      ) < 0.99
    for: 5m
    labels:
      severity: critical
      slo: holmesgpt_availability
    annotations:
      summary: "HolmesGPT availability below SLO"
      description: "HolmesGPT availability is {{ $value | humanizePercentage }}, below 99% SLO"

  # Operational: Low Recommendation Confidence
  - alert: AIAnalysisLowConfidenceRecommendations
    expr: |
      avg(aianalysis_recommendation_confidence) < 0.85
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "AI recommendations have low average confidence"
      description: "Average recommendation confidence is {{ $value }}, below 0.85 threshold"

  # Operational: High Manual Approval Rate
  - alert: AIAnalysisHighManualApprovalRate
    expr: |
      (
        sum(rate(aianalysis_approval_total{type="manual"}[1h])) /
        sum(rate(aianalysis_approval_total[1h]))
      ) > 0.60
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "High manual approval rate detected"
      description: "{{ $value | humanizePercentage }} of approvals require manual review (threshold: 60%)"
```

---

### Query Examples

**AI/ML-Specific Queries**:

```promql
# 1. HolmesGPT Request Success Rate
sum(rate(aianalysis_holmesgpt_requests_total{status="success"}[5m])) /
sum(rate(aianalysis_holmesgpt_requests_total[5m]))

# 2. Average Recommendation Confidence
avg(aianalysis_recommendation_confidence)

# 3. Auto-Approval Rate
sum(rate(aianalysis_approval_total{type="auto"}[5m])) /
sum(rate(aianalysis_approval_total[5m]))

# 4. HolmesGPT P95 Latency
histogram_quantile(0.95,
  rate(aianalysis_holmesgpt_request_duration_seconds_bucket[5m])
)

# 5. Rego Policy Evaluation Performance
histogram_quantile(0.95,
  rate(aianalysis_rego_policy_evaluation_duration_seconds_bucket[5m])
)

# 6. Recommendations by Confidence Tier
sum by (confidence_tier) (rate(aianalysis_recommendation_count[5m]))

# 7. Approval Decision Distribution
sum by (decision, type) (rate(aianalysis_approval_total[5m]))
```

---
