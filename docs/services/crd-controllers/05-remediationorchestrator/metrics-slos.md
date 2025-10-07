## Enhanced Metrics & SLOs

### SLI/SLO Definitions

**Service Level Indicators (SLIs)**:

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **End-to-End Remediation Success Rate** | `successful_remediations / total_remediations` | ≥97% | Overall system reliability |
| **Alert-to-Resolution Time (P95)** | `histogram_quantile(0.95, alertremediation_end_to_end_duration_seconds)` | <180s | Fast incident resolution |
| **Phase Success Rate** | `successful_phases / total_phases` | ≥99% | Individual phase reliability |
| **Timeout Detection Accuracy** | `timeouts_detected / timeouts_actual` | ≥99% | Proactive escalation |
| **Child CRD Creation Success Rate** | `child_crd_creation_success / child_crd_creation_total` | ≥99.5% | Orchestration reliability |

**Service Level Objectives (SLOs)**:

```yaml
slos:
  - name: "RemediationRequest End-to-End Success Rate"
    sli: "successful_remediations / total_remediations"
    target: 0.97  # 97%
    window: "30d"
    burn_rate_fast: 14.4
    burn_rate_slow: 6

  - name: "RemediationRequest P95 Alert-to-Resolution Time"
    sli: "histogram_quantile(0.95, alertremediation_end_to_end_duration_seconds)"
    target: 180  # 180 seconds (3 minutes)
    window: "30d"

  - name: "Phase Success Rate"
    sli: "successful_phases / total_phases"
    target: 0.99  # 99%
    window: "30d"
```

---

### Service-Specific Metrics

**End-to-End Orchestration Metrics**:

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // End-to-end remediation metrics
    remediationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "alertremediation_total",
            Help: "Total number of alert remediations",
        },
        []string{"status", "environment"},  // status: completed|failed, environment: prod|staging|dev
    )

    remediationEndToEndDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "alertremediation_end_to_end_duration_seconds",
            Help:    "End-to-end remediation duration (alert to resolution) in seconds",
            Buckets: prometheus.ExponentialBuckets(10, 2, 10),  // 10s to 10240s
        },
        []string{"environment"},
    )

    remediationActive = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "alertremediation_active_total",
            Help: "Number of currently active remediations",
        },
        []string{"phase", "environment"},
    )

    // Phase-specific metrics
    phaseExecutionTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "alertremediation_phase_execution_total",
            Help: "Total number of phase executions",
        },
        []string{"phase", "status"},  // phase: processing|analyzing|executing, status: success|failed
    )

    phaseExecutionDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "alertremediation_phase_execution_duration_seconds",
            Help:    "Phase execution duration in seconds",
            Buckets: prometheus.ExponentialBuckets(1, 2, 10),  // 1s to 1024s
        },
        []string{"phase"},
    )

    // Child CRD creation metrics
    childCRDCreationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "alertremediation_child_crd_creation_total",
            Help: "Total number of child CRD creations",
        },
        []string{"crd_type", "status"},  // crd_type: alertprocessing|aianalysis|workflowexecution
    )

    childCRDCreationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "alertremediation_child_crd_creation_duration_seconds",
            Help:    "Child CRD creation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 8),  // 10ms to 2.56s
        },
        []string{"crd_type"},
    )

    // Child CRD status polling metrics
    childCRDStatusPollsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "alertremediation_child_crd_status_polls_total",
            Help: "Total number of child CRD status polls",
        },
        []string{"crd_type"},
    )

    // Timeout detection metrics
    timeoutDetectionTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "alertremediation_timeout_detection_total",
            Help: "Total number of timeout detections",
        },
        []string{"phase"},
    )

    timeoutEscalationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "alertremediation_timeout_escalation_total",
            Help: "Total number of timeout escalations to Notification Service",
        },
        []string{"phase"},
    )

    // Escalation metrics
    escalationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "alertremediation_escalation_total",
            Help: "Total number of escalations to Notification Service",
        },
        []string{"reason"},  // reason: timeout|failure|rejection
    )

    escalationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "alertremediation_escalation_duration_seconds",
            Help:    "Escalation notification duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 8),  // 100ms to 25.6s
        },
    )

    // Lifecycle metrics
    lifecycleEventTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "alertremediation_lifecycle_event_total",
            Help: "Total number of lifecycle events",
        },
        []string{"event"},  // event: created|completed|failed|deleted
    )

    // Alert fingerprint metrics
    alertFingerprintUniqueness = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "alertremediation_alert_fingerprint_uniqueness",
            Help: "Number of unique alert fingerprints being processed",
        },
    )
)
```

---

### Grafana Dashboard JSON

**RemediationRequest Controller Dashboard** (key panels):

```json
{
  "dashboard": {
    "title": "RemediationRequest Controller - End-to-End Observability",
    "uid": "alertremediation-controller",
    "tags": ["kubernaut", "alertremediation", "remediation-orchestrator", "end-to-end"],
    "panels": [
      {
        "id": 1,
        "title": "End-to-End Remediation Success Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(alertremediation_total{status='completed'}[5m])) / sum(rate(alertremediation_total[5m]))",
            "legendFormat": "Success Rate"
          }
        ],
        "thresholds": [{"value": 0.97, "colorMode": "critical", "op": "lt"}]
      },
      {
        "id": 2,
        "title": "Alert-to-Resolution Time (P50, P95, P99)",
        "type": "graph",
        "targets": [
          {"expr": "histogram_quantile(0.50, rate(alertremediation_end_to_end_duration_seconds_bucket[5m]))", "legendFormat": "P50"},
          {"expr": "histogram_quantile(0.95, rate(alertremediation_end_to_end_duration_seconds_bucket[5m]))", "legendFormat": "P95 (SLI)"},
          {"expr": "histogram_quantile(0.99, rate(alertremediation_end_to_end_duration_seconds_bucket[5m]))", "legendFormat": "P99"}
        ],
        "thresholds": [{"value": 180, "colorMode": "critical", "op": "gt"}]
      },
      {
        "id": 3,
        "title": "Active Remediations by Phase",
        "type": "graph",
        "targets": [
          {"expr": "sum by (phase) (alertremediation_active_total)"}
        ]
      },
      {
        "id": 4,
        "title": "Phase Success Rate",
        "type": "graph",
        "targets": [
          {"expr": "sum by (phase) (rate(alertremediation_phase_execution_total{status='success'}[5m])) / sum by (phase) (rate(alertremediation_phase_execution_total[5m]))"}
        ]
      },
      {
        "id": 5,
        "title": "Phase Execution Duration Breakdown",
        "type": "heatmap",
        "targets": [
          {"expr": "sum by (phase, le) (rate(alertremediation_phase_execution_duration_seconds_bucket[5m]))"}
        ]
      },
      {
        "id": 6,
        "title": "Child CRD Creation Success Rate",
        "type": "graph",
        "targets": [
          {"expr": "sum by (crd_type) (rate(alertremediation_child_crd_creation_total{status='success'}[5m])) / sum by (crd_type) (rate(alertremediation_child_crd_creation_total[5m]))"}
        ]
      },
      {
        "id": 7,
        "title": "Timeout Detections & Escalations",
        "type": "graph",
        "targets": [
          {"expr": "sum(rate(alertremediation_timeout_detection_total[5m]))", "legendFormat": "Timeouts Detected"},
          {"expr": "sum(rate(alertremediation_timeout_escalation_total[5m]))", "legendFormat": "Escalations"}
        ]
      },
      {
        "id": 8,
        "title": "Escalation Reason Breakdown",
        "type": "piechart",
        "targets": [
          {"expr": "sum by (reason) (alertremediation_escalation_total)"}
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
- name: alertremediation-slos
  interval: 30s
  rules:
  # SLO: End-to-End Remediation Success Rate
  - alert: RemediationRequestSuccessRateSLOBreach
    expr: |
      (
        sum(rate(alertremediation_total{status="completed"}[1h])) /
        sum(rate(alertremediation_total[1h]))
      ) < 0.97
    for: 5m
    labels:
      severity: critical
      slo: remediation_success_rate
    annotations:
      summary: "Alert remediation success rate below SLO"
      description: "Remediation success rate is {{ $value | humanizePercentage }}, below 97% SLO"

  # SLO: Alert-to-Resolution Time
  - alert: RemediationRequestResolutionTimeSLOBreach
    expr: |
      histogram_quantile(0.95,
        rate(alertremediation_end_to_end_duration_seconds_bucket[5m])
      ) > 180
    for: 10m
    labels:
      severity: warning
      slo: alert_to_resolution_time_p95
    annotations:
      summary: "Alert-to-resolution time exceeds SLO"
      description: "P95 alert-to-resolution time is {{ $value }}s, exceeding 180s SLO"

  # SLO: Phase Success Rate
  - alert: RemediationRequestPhaseSuccessRateSLOBreach
    expr: |
      (
        sum(rate(alertremediation_phase_execution_total{status="success"}[1h])) /
        sum(rate(alertremediation_phase_execution_total[1h]))
      ) < 0.99
    for: 5m
    labels:
      severity: critical
      slo: phase_success_rate
    annotations:
      summary: "Phase execution success rate below SLO"
      description: "Phase success rate is {{ $value | humanizePercentage }}, below 99% SLO"

  # Operational: High Timeout Rate
  - alert: RemediationRequestHighTimeoutRate
    expr: |
      (
        sum(rate(alertremediation_timeout_detection_total[5m])) /
        sum(rate(alertremediation_total[5m]))
      ) > 0.05
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "High timeout detection rate"
      description: "{{ $value | humanizePercentage }} of remediations timing out (threshold: 5%)"

  # Operational: Child CRD Creation Failures
  - alert: RemediationRequestChildCRDCreationFailures
    expr: |
      (
        sum by (crd_type) (rate(alertremediation_child_crd_creation_total{status="error"}[5m])) /
        sum by (crd_type) (rate(alertremediation_child_crd_creation_total[5m]))
      ) > 0.01
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "High child CRD creation failure rate for {{ $labels.crd_type }}"
      description: "{{ $value | humanizePercentage }} of {{ $labels.crd_type }} CRD creations failing"

  # Operational: Remediation Stuck
  - alert: RemediationRequestStuck
    expr: |
      time() - alertremediation_start_timestamp > 600
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Alert remediation stuck"
      description: "Remediation {{ $labels.name }} has been running for over 10 minutes"
```

---

### Query Examples

**End-to-End & Orchestration Queries**:

```promql
# 1. End-to-End Remediation Success Rate (SLI)
sum(rate(alertremediation_total{status="completed"}[5m])) /
sum(rate(alertremediation_total[5m]))

# 2. Alert-to-Resolution Time P95 (SLI)
histogram_quantile(0.95,
  rate(alertremediation_end_to_end_duration_seconds_bucket[5m])
)

# 3. Phase Success Rate
sum by (phase) (rate(alertremediation_phase_execution_total{status="success"}[5m])) /
sum by (phase) (rate(alertremediation_phase_execution_total[5m]))

# 4. Active Remediations by Phase
sum by (phase) (alertremediation_active_total)

# 5. Child CRD Creation Success Rate
sum by (crd_type) (rate(alertremediation_child_crd_creation_total{status="success"}[5m])) /
sum by (crd_type) (rate(alertremediation_child_crd_creation_total[5m]))

# 6. Timeout Detection Rate
sum(rate(alertremediation_timeout_detection_total[5m])) /
sum(rate(alertremediation_total[5m]))

# 7. Escalation Rate by Reason
sum by (reason) (rate(alertremediation_escalation_total[5m]))

# 8. Average Alert-to-Resolution Time
rate(alertremediation_end_to_end_duration_seconds_sum[5m]) /
rate(alertremediation_end_to_end_duration_seconds_count[5m])

# 9. Phase Duration Breakdown
histogram_quantile(0.95,
  sum by (phase, le) (rate(alertremediation_phase_execution_duration_seconds_bucket[5m]))
)

# 10. Unique Alert Fingerprints
alertremediation_alert_fingerprint_uniqueness
```

---
