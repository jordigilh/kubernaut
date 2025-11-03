## Enhanced Metrics & SLOs

### SLI/SLO Definitions

**Service Level Indicators (SLIs)**:

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **Enrichment Success Rate** | `successful_enrichments / total_enrichments` | ≥99% | HolmesGPT receives quality data |
| **Processing Latency (P95)** | `histogram_quantile(0.95, alertprocessing_duration_seconds)` | <30s | Fast remediation start |
| **Context Service Availability** | `context_service_requests_success / context_service_requests_total` | ≥95% | Historical pattern availability |
| **Degraded Mode Rate** | `alertprocessing_degraded_total / alertprocessing_total` | <5% | Most alerts fully enriched |

**Service Level Objectives (SLOs)**:

```yaml
slos:
  - name: "RemediationProcessing Enrichment Success Rate"
    sli: "successful_enrichments / total_enrichments"
    target: 0.99  # 99%
    window: "30d"
    burn_rate_fast: 14.4  # 1h window
    burn_rate_slow: 6     # 6h window

  - name: "RemediationProcessing P95 Latency"
    sli: "histogram_quantile(0.95, alertprocessing_duration_seconds)"
    target: 30  # 30 seconds
    window: "30d"

  - name: "Context Service Availability"
    sli: "context_service_requests_success / context_service_requests_total"
    target: 0.95  # 95%
    window: "30d"
```

---

### Grafana Dashboard JSON

**RemediationProcessing Controller Dashboard**:

```json
{
  "dashboard": {
    "title": "RemediationProcessing Controller - Observability",
    "uid": "alertprocessing-controller",
    "tags": ["kubernaut", "alertprocessing", "controller"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Enrichment Success Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(alertprocessing_enrichment_success_total[5m])) / sum(rate(alertprocessing_enrichment_total[5m]))",
            "legendFormat": "Success Rate",
            "refId": "A"
          }
        ],
        "yaxes": [
          {"format": "percentunit", "min": 0, "max": 1}
        ],
        "thresholds": [
          {"value": 0.99, "colorMode": "critical", "op": "lt", "line": true}
        ]
      },
      {
        "id": 2,
        "title": "Processing Latency (P50, P95, P99)",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.50, rate(alertprocessing_duration_seconds_bucket[5m]))",
            "legendFormat": "P50",
            "refId": "A"
          },
          {
            "expr": "histogram_quantile(0.95, rate(alertprocessing_duration_seconds_bucket[5m]))",
            "legendFormat": "P95 (SLI)",
            "refId": "B"
          },
          {
            "expr": "histogram_quantile(0.99, rate(alertprocessing_duration_seconds_bucket[5m]))",
            "legendFormat": "P99",
            "refId": "C"
          }
        ],
        "yaxes": [
          {"format": "s", "min": 0}
        ],
        "thresholds": [
          {"value": 30, "colorMode": "critical", "op": "gt", "line": true}
        ]
      },
      {
        "id": 3,
        "title": "Phase Distribution",
        "type": "piechart",
        "targets": [
          {
            "expr": "sum by (phase) (alertprocessing_active_total)",
            "legendFormat": "{{phase}}",
            "refId": "A"
          }
        ]
      },
      {
        "id": 4,
        "title": "Context Service Availability (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(alertprocessing_context_service_requests_success_total[5m])) / sum(rate(alertprocessing_context_service_requests_total[5m]))",
            "legendFormat": "Availability",
            "refId": "A"
          }
        ],
        "yaxes": [
          {"format": "percentunit", "min": 0, "max": 1}
        ],
        "thresholds": [
          {"value": 0.95, "colorMode": "critical", "op": "lt", "line": true}
        ]
      },
      {
        "id": 5,
        "title": "Degraded Mode Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(alertprocessing_degraded_total[5m])) / sum(rate(alertprocessing_total[5m]))",
            "legendFormat": "Degraded Mode %",
            "refId": "A"
          }
        ],
        "yaxes": [
          {"format": "percentunit", "min": 0, "max": 1}
        ],
        "thresholds": [
          {"value": 0.05, "colorMode": "warning", "op": "gt", "line": true}
        ]
      },
      {
        "id": 6,
        "title": "CRD Lifecycle (Created/Completed/Failed)",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(alertprocessing_created_total[5m])",
            "legendFormat": "Created",
            "refId": "A"
          },
          {
            "expr": "rate(alertprocessing_completed_total[5m])",
            "legendFormat": "Completed",
            "refId": "B"
          },
          {
            "expr": "rate(alertprocessing_failed_total[5m])",
            "legendFormat": "Failed",
            "refId": "C"
          }
        ]
      },
      {
        "id": 7,
        "title": "Trace Visualization (Jaeger Link)",
        "type": "text",
        "options": {
          "content": "[Open Jaeger Traces](http://jaeger.monitoring.svc:16686/search?service=alertprocessing-controller)"
        }
      }
    ]
  }
}
```

---

### Alert Rules YAML

**Prometheus Alert Rules**:

```yaml
groups:
- name: alertprocessing-slos
  interval: 30s
  rules:
  # SLO: Enrichment Success Rate
  - alert: RemediationProcessingEnrichmentSLOBreach
    expr: |
      (
        sum(rate(alertprocessing_enrichment_success_total[1h])) /
        sum(rate(alertprocessing_enrichment_total[1h]))
      ) < 0.99
    for: 5m
    labels:
      severity: critical
      slo: enrichment_success_rate
    annotations:
      summary: "RemediationProcessing enrichment success rate below SLO"
      description: "Enrichment success rate is {{ $value | humanizePercentage }}, below 99% SLO threshold"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-enrichment-failure"

  # SLO: Processing Latency P95
  - alert: RemediationProcessingLatencySLOBreach
    expr: |
      histogram_quantile(0.95,
        rate(alertprocessing_duration_seconds_bucket[5m])
      ) > 30
    for: 10m
    labels:
      severity: warning
      slo: processing_latency_p95
    annotations:
      summary: "RemediationProcessing P95 latency exceeds SLO"
      description: "P95 processing latency is {{ $value }}s, exceeding 30s SLO threshold"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-latency"

  # SLO: Context Service Availability
  - alert: ContextServiceAvailabilitySLOBreach
    expr: |
      (
        sum(rate(alertprocessing_context_service_requests_success_total[1h])) /
        sum(rate(alertprocessing_context_service_requests_total[1h]))
      ) < 0.95
    for: 10m
    labels:
      severity: warning
      slo: context_service_availability
    annotations:
      summary: "Context Service availability below SLO"
      description: "Context Service availability is {{ $value | humanizePercentage }}, below 95% SLO"
      runbook: "https://docs.kubernaut.io/runbooks/context-service-down"

  # Operational: High Degraded Mode Rate
  - alert: RemediationProcessingHighDegradedModeRate
    expr: |
      (
        sum(rate(alertprocessing_degraded_total[5m])) /
        sum(rate(alertprocessing_total[5m]))
      ) > 0.05
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "High RemediationProcessing degraded mode rate"
      description: "{{ $value | humanizePercentage }} of alerts processed in degraded mode (threshold: 5%)"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-degraded-mode"

  # Operational: Enrichment Phase Stuck
  - alert: RemediationProcessingEnrichmentStuck
    expr: |
      time() - alertprocessing_phase_start_timestamp{phase="enriching"} > 300
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "RemediationProcessing stuck in enriching phase"
      description: "RemediationProcessing {{ $labels.name }} has been in enriching phase for over 5 minutes"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-stuck"

  # Operational: High Failure Rate
  - alert: RemediationProcessingHighFailureRate
    expr: |
      (
        sum(rate(alertprocessing_failed_total[5m])) /
        sum(rate(alertprocessing_total[5m]))
      ) > 0.05
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "High RemediationProcessing failure rate"
      description: "{{ $value | humanizePercentage }} of RemediationProcessing operations failing (threshold: 5%)"
      runbook: "https://docs.kubernaut.io/runbooks/alertprocessing-failures"

  # Operational: Controller Restart Loops
  - alert: RemediationProcessingControllerRestartLoop
    expr: |
      rate(kube_pod_container_status_restarts_total{
        namespace="kubernaut-system",
        pod=~"alertprocessing-controller-.*"
      }[15m]) > 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "RemediationProcessing controller in restart loop"
      description: "Controller pod {{ $labels.pod }} restarting frequently"
      runbook: "https://docs.kubernaut.io/runbooks/controller-crash-loop"
```

---

### Query Examples

**SLI Queries for Dashboards**:

```promql
# 1. Enrichment Success Rate (SLI)
sum(rate(alertprocessing_enrichment_success_total[5m])) /
sum(rate(alertprocessing_enrichment_total[5m]))

# 2. Processing Latency P95 (SLI)
histogram_quantile(0.95,
  rate(alertprocessing_duration_seconds_bucket[5m])
)

# 3. Context Service Availability (SLI)
sum(rate(alertprocessing_context_service_requests_success_total[5m])) /
sum(rate(alertprocessing_context_service_requests_total[5m]))

# 4. Degraded Mode Rate (SLI)
sum(rate(alertprocessing_degraded_total[5m])) /
sum(rate(alertprocessing_total[5m]))

# 5. Error Budget Remaining (30-day window)
1 - (
  (1 - 0.99) -  # SLO target: 99%
  (
    1 - (
      sum(increase(alertprocessing_enrichment_success_total[30d])) /
      sum(increase(alertprocessing_enrichment_total[30d]))
    )
  )
) / (1 - 0.99)

# 6. Phase Distribution
sum by (phase) (alertprocessing_active_total)

# 7. Enrichment Duration by Phase
histogram_quantile(0.95,
  rate(alertprocessing_phase_duration_seconds_bucket{phase="enriching"}[5m])
)

# 8. Kubernetes API Query Rate
rate(alertprocessing_kubernetes_api_requests_total[5m])

# 9. CRD Creation Rate
rate(alertprocessing_created_total[5m])

# 10. Active SignalProcessing CRDs
alertprocessing_active_total
```

**Troubleshooting Queries**:

```promql
# Find slow enrichments (>30s)
alertprocessing_duration_seconds > 30

# Find RemediationProcessings in degraded mode
alertprocessing_active_total{degraded_mode="true"}

# Find Context Service failures
rate(alertprocessing_context_service_requests_total{status="error"}[5m])

# Find CRDs stuck in enriching phase
time() - alertprocessing_phase_start_timestamp{phase="enriching"} > 300
```

---

