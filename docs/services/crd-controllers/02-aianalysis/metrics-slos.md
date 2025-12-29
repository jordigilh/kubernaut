# Metrics & SLOs

**Version**: v2.0
**Status**: ✅ Complete - V1.0 Aligned
**Last Updated**: 2025-11-30

---

## 📋 Changelog

| Version | Date | Changes |
|---------|------|---------|
| v2.0 | 2025-11-30 | **V1.0 ALIGNMENT**: Updated approval metrics to signaling pattern (no AIApprovalRequest CRD); Added DetectedLabels/CustomLabels metrics; Updated phase names |
| v1.0 | 2025-10-15 | Initial specification |

---

## SLI/SLO Definitions

### Service Level Indicators (SLIs)

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **Investigation Success Rate** | `successful_investigations / total_investigations` | ≥95% | HolmesGPT analysis reliability |
| **Investigation Latency (P95)** | `histogram_quantile(0.95, aianalysis_investigation_duration_seconds)` | <30s | Fast root cause identification |
| **HolmesGPT-API Availability** | `holmesgpt_requests_success / holmesgpt_requests_total` | ≥99% | AI service dependency |
| **Auto-Approval Rate** | `approval_not_required_total / total_analyses` | 40-60% | Rego policy effectiveness |
| **Recommendation Confidence (Avg)** | `avg(aianalysis_workflow_confidence)` | ≥0.80 | High-quality workflow selection |

### Service Level Objectives (SLOs)

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
    target: 30  # 30 seconds
    window: "30d"

  - name: "HolmesGPT-API Availability"
    sli: "holmesgpt_requests_success / holmesgpt_requests_total"
    target: 0.99  # 99%
    window: "30d"
```

---

## Service-Specific Metrics

### AI/ML Metrics

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // HolmesGPT-API request metrics
    holmesgptRequestsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_holmesgpt_requests_total",
            Help: "Total number of requests to HolmesGPT-API",
        },
        []string{"status", "endpoint"},  // status: success|error, endpoint: investigate|recovery
    )

    holmesgptRequestDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_holmesgpt_request_duration_seconds",
            Help:    "HolmesGPT-API request duration in seconds",
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

    // Workflow selection metrics
    workflowConfidence = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "aianalysis_workflow_confidence",
            Help:    "Selected workflow confidence score (0-1)",
            Buckets: prometheus.LinearBuckets(0.5, 0.1, 6),  // 0.5 to 1.0
        },
    )

    workflowSelectionTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_workflow_selection_total",
            Help: "Total number of workflow selections",
        },
        []string{"confidence_tier"},  // high (>0.9) | medium (0.7-0.9) | low (<0.7)
    )

    // V1.0: Approval signaling metrics (not AIApprovalRequest CRD)
    approvalSignalingTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_approval_signaling_total",
            Help: "Total number of approval decisions (V1.0: signaling, not CRD)",
        },
        []string{"approval_required"},  // true | false
    )

    // Rego policy metrics
    regoPolicyEvaluationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "aianalysis_rego_policy_evaluation_duration_seconds",
            Help:    "Rego approval policy evaluation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),  // 1ms to 1s
        },
    )

    regoPolicyEvaluationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_rego_policy_evaluation_total",
            Help: "Total number of Rego policy evaluations",
        },
        []string{"result"},  // auto_approve | manual_approval_required | error
    )

    // DetectedLabels metrics (V1.0)
    detectedLabelsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_detected_labels_total",
            Help: "Count of detected labels used in analysis",
        },
        []string{"label_type"},  // gitops_managed | pdb_protected | stateful | etc.
    )

    // CustomLabels metrics (V1.0)
    customLabelsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_custom_labels_total",
            Help: "Count of custom labels (Rego-extracted) used in analysis",
        },
        []string{"subdomain"},  // constraint | team | region | etc.
    )

    // Recovery metrics
    recoveryAnalysisTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_recovery_analysis_total",
            Help: "Total number of recovery analyses (isRecoveryAttempt=true)",
        },
        []string{"attempt_number", "status"},  // attempt: 1|2|3, status: success|error
    )
)
```

---

## Grafana Dashboard JSON

**AIAnalysis Controller Dashboard** (key panels):

```json
{
  "dashboard": {
    "title": "AIAnalysis Controller - V1.0 Observability",
    "uid": "aianalysis-controller-v1",
    "tags": ["kubernaut", "aianalysis", "ai-ml", "controller", "v1.0"],
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
        "title": "HolmesGPT-API Latency (P50, P95, P99)",
        "type": "graph",
        "targets": [
          {"expr": "histogram_quantile(0.50, rate(aianalysis_holmesgpt_request_duration_seconds_bucket[5m]))", "legendFormat": "P50"},
          {"expr": "histogram_quantile(0.95, rate(aianalysis_holmesgpt_request_duration_seconds_bucket[5m]))", "legendFormat": "P95 (SLI)"},
          {"expr": "histogram_quantile(0.99, rate(aianalysis_holmesgpt_request_duration_seconds_bucket[5m]))", "legendFormat": "P99"}
        ],
        "thresholds": [{"value": 30, "colorMode": "critical", "op": "gt"}]
      },
      {
        "id": 3,
        "title": "Workflow Confidence Distribution",
        "type": "heatmap",
        "targets": [
          {"expr": "sum(rate(aianalysis_workflow_confidence_bucket[5m])) by (le)"}
        ]
      },
      {
        "id": 4,
        "title": "Approval Signaling (V1.0)",
        "type": "graph",
        "targets": [
          {"expr": "sum(rate(aianalysis_approval_signaling_total{approval_required='false'}[5m])) / sum(rate(aianalysis_approval_signaling_total[5m]))", "legendFormat": "Auto-Approved %"}
        ]
      },
      {
        "id": 5,
        "title": "Rego Policy Evaluation Performance",
        "type": "graph",
        "targets": [
          {"expr": "histogram_quantile(0.95, rate(aianalysis_rego_policy_evaluation_duration_seconds_bucket[5m]))", "legendFormat": "P95"}
        ],
        "thresholds": [{"value": 2, "colorMode": "warning", "op": "gt"}]
      },
      {
        "id": 6,
        "title": "DetectedLabels Usage",
        "type": "piechart",
        "targets": [
          {"expr": "sum by (label_type) (aianalysis_detected_labels_total)"}
        ]
      },
      {
        "id": 7,
        "title": "Recovery Analysis Attempts",
        "type": "graph",
        "targets": [
          {"expr": "sum(rate(aianalysis_recovery_analysis_total{status='success'}[5m])) by (attempt_number)", "legendFormat": "Success (Attempt {{attempt_number}})"},
          {"expr": "sum(rate(aianalysis_recovery_analysis_total{status='error'}[5m])) by (attempt_number)", "legendFormat": "Error (Attempt {{attempt_number}})"}
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

  # SLO: HolmesGPT-API Availability
  - alert: HolmesGPTAPIAvailabilitySLOBreach
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
      summary: "HolmesGPT-API availability below SLO"
      description: "HolmesGPT-API availability is {{ $value | humanizePercentage }}, below 99% SLO"

  # Operational: Low Workflow Confidence
  - alert: AIAnalysisLowWorkflowConfidence
    expr: |
      avg(aianalysis_workflow_confidence) < 0.80
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "AI workflow selections have low average confidence"
      description: "Average workflow confidence is {{ $value }}, below 0.80 threshold"

  # Operational: High Manual Approval Rate (V1.0)
  - alert: AIAnalysisHighManualApprovalRate
    expr: |
      (
        sum(rate(aianalysis_approval_signaling_total{approval_required="true"}[1h])) /
        sum(rate(aianalysis_approval_signaling_total[1h]))
      ) > 0.60
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "High manual approval rate detected (V1.0)"
      description: "{{ $value | humanizePercentage }} of analyses require manual approval (threshold: 60%)"

  # Rego Policy: Slow Evaluation
  - alert: AIAnalysisSlowRegoPolicyEvaluation
    expr: |
      histogram_quantile(0.95, rate(aianalysis_rego_policy_evaluation_duration_seconds_bucket[5m])) > 2
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Rego policy evaluation taking >2s (P95)"
      description: "Rego policy evaluation P95 is {{ $value }}s, exceeding 2s target"

  # Recovery: High Failure Rate
  - alert: AIAnalysisRecoveryHighFailureRate
    expr: |
      (
        sum(rate(aianalysis_recovery_analysis_total{status="error"}[1h])) /
        sum(rate(aianalysis_recovery_analysis_total[1h]))
      ) > 0.30
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "High recovery analysis failure rate"
      description: "{{ $value | humanizePercentage }} of recovery analyses failing (threshold: 30%)"
```

---

## Query Examples

### AI/ML-Specific Queries

```promql
# 1. HolmesGPT-API Request Success Rate
sum(rate(aianalysis_holmesgpt_requests_total{status="success"}[5m])) /
sum(rate(aianalysis_holmesgpt_requests_total[5m]))

# 2. Average Workflow Confidence
avg(aianalysis_workflow_confidence)

# 3. Auto-Approval Rate (V1.0 Signaling)
sum(rate(aianalysis_approval_signaling_total{approval_required="false"}[5m])) /
sum(rate(aianalysis_approval_signaling_total[5m]))

# 4. HolmesGPT-API P95 Latency
histogram_quantile(0.95,
  rate(aianalysis_holmesgpt_request_duration_seconds_bucket[5m])
)

# 5. Rego Policy Evaluation Performance
histogram_quantile(0.95,
  rate(aianalysis_rego_policy_evaluation_duration_seconds_bucket[5m])
)

# 6. Workflow Selection by Confidence Tier
sum by (confidence_tier) (rate(aianalysis_workflow_selection_total[5m]))

# 7. DetectedLabels Usage Distribution
sum by (label_type) (aianalysis_detected_labels_total)

# 8. CustomLabels Usage by Subdomain
sum by (subdomain) (aianalysis_custom_labels_total)

# 9. Recovery Analysis Success Rate by Attempt
sum(rate(aianalysis_recovery_analysis_total{status="success"}[5m])) by (attempt_number) /
sum(rate(aianalysis_recovery_analysis_total[5m])) by (attempt_number)
```

---

## References

- [Observability & Logging](./observability-logging.md) - Logging patterns
- [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) - DetectedLabels/CustomLabels
- [REGO_POLICY_EXAMPLES.md](./REGO_POLICY_EXAMPLES.md) - Approval policy schema
