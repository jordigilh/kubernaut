## Enhanced Metrics & SLOs

> **📋 Changelog**
> | Version | Date | Changes | Reference |
> |---------|------|---------|-----------|
> | v1.3 | 2025-11-30 | Added label detection metrics (OwnerChain, DetectedLabels, CustomLabels, Rego) | [DD-WORKFLOW-001 v1.8](../../../architecture/decisions/DD-WORKFLOW-001-mandatory-label-schema.md) |
> | v1.2 | 2025-11-28 | Metric rename: alertprocessing_* → signalprocessing_*, removed Context Service (deprecated), updated SLIs/SLOs | [ADR-015](../../../architecture/decisions/ADR-015-alert-to-signal-naming-migration.md), [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md) |
> | v1.1 | 2025-11-27 | Service rename: SignalProcessing | [DD-SIGNAL-PROCESSING-001](../../../architecture/decisions/DD-SIGNAL-PROCESSING-001-service-rename.md) |
> | v1.0 | 2025-01-15 | Initial metrics and SLOs | - |

### SLI/SLO Definitions

**Service Level Indicators (SLIs)**:

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **Enrichment Success Rate** | `signalprocessing_enrichment_success_total / signalprocessing_enrichment_total` | ≥99% | HolmesGPT receives quality data |
| **Processing Latency (P95)** | `histogram_quantile(0.95, signalprocessing_duration_seconds)` | <5s | Fast remediation start |
| **Degraded Mode Rate** | `signalprocessing_degraded_total / signalprocessing_total` | <5% | Most signals fully enriched |
| **Categorization Success Rate** | `signalprocessing_categorization_success_total / signalprocessing_categorization_total` | ≥99% | Accurate priority assignment |
| **Audit Write Success Rate** | `signalprocessing_audit_success_total / signalprocessing_audit_total` | ≥99% | Audit trail completeness |
| **Owner Chain Build Success** | `signalprocessing_owner_chain_success_total / signalprocessing_owner_chain_total` | ≥99% | DetectedLabels validation enabled |
| **DetectedLabels Coverage** | `avg(signalprocessing_detected_labels_count)` | ≥3 | Minimum detection types per signal |
| **Rego Evaluation Success** | `signalprocessing_rego_success_total / signalprocessing_rego_total` | ≥99% | CustomLabels extraction works |
| **Rego Evaluation Latency (P95)** | `histogram_quantile(0.95, signalprocessing_rego_duration_seconds)` | <100ms | Fast policy evaluation |

**Note**: Context Service metrics removed per [DD-CONTEXT-006](../../../architecture/decisions/DD-CONTEXT-006-CONTEXT-API-DEPRECATION.md). Recovery context is now embedded by Remediation Orchestrator.

### Label Detection Metrics (DD-WORKFLOW-001 v1.8) ⭐ NEW

**OwnerChain Metrics**:
```yaml
# Owner chain build duration
signalprocessing_owner_chain_duration_seconds:
  type: histogram
  labels: [namespace, status]
  buckets: [0.05, 0.1, 0.25, 0.5, 1.0]
  description: "Time to build K8s ownership chain"

# Owner chain length
signalprocessing_owner_chain_length:
  type: histogram
  labels: [namespace]
  buckets: [1, 2, 3, 4, 5, 10]
  description: "Number of entries in ownership chain"

# Owner chain success/failure
signalprocessing_owner_chain_total:
  type: counter
  labels: [namespace, status]  # status: success, error
  description: "Total owner chain build attempts"
```

**DetectedLabels Metrics**:
```yaml
# Labels detected per signal
signalprocessing_detected_labels_count:
  type: gauge
  labels: [namespace]
  description: "Number of DetectedLabels populated (0-9 types)"

# Detection duration
signalprocessing_detected_labels_duration_seconds:
  type: histogram
  labels: [namespace]
  buckets: [0.05, 0.1, 0.2, 0.5, 1.0]
  description: "Time to detect all label types"

# Detection by type
signalprocessing_detected_label_type:
  type: counter
  labels: [label_type]  # gitops, pdb, hpa, stateful, helm, networkpolicy, pss, servicemesh
  description: "Count of each detection type found"
```

**CustomLabels/Rego Metrics**:
```yaml
# Rego policy evaluation duration
signalprocessing_rego_duration_seconds:
  type: histogram
  labels: [namespace, status]
  buckets: [0.01, 0.025, 0.05, 0.1, 0.25]
  description: "Rego policy evaluation time"

# Rego evaluation results
signalprocessing_rego_total:
  type: counter
  labels: [namespace, status]  # status: success, error, empty
  description: "Total Rego evaluations"

# CustomLabels count
signalprocessing_custom_labels_count:
  type: gauge
  labels: [namespace]
  description: "Number of CustomLabels extracted via Rego"

# Security wrapper blocks
signalprocessing_rego_security_blocks_total:
  type: counter
  labels: [blocked_label]  # The mandatory label that was blocked
  description: "Count of customer attempts to override mandatory labels"
```

**Service Level Objectives (SLOs)**:

```yaml
slos:
  - name: "SignalProcessing Enrichment Success Rate"
    sli: "signalprocessing_enrichment_success_total / signalprocessing_enrichment_total"
    target: 0.99  # 99%
    window: "30d"
    burn_rate_fast: 14.4  # 1h window
    burn_rate_slow: 6     # 6h window

  - name: "SignalProcessing P95 Latency"
    sli: "histogram_quantile(0.95, signalprocessing_duration_seconds)"
    target: 5  # 5 seconds (updated from 30s per implementation plan)
    window: "30d"

  - name: "SignalProcessing Categorization Success Rate"
    sli: "signalprocessing_categorization_success_total / signalprocessing_categorization_total"
    target: 0.99  # 99%
    window: "30d"
```

---

### Grafana Dashboard JSON

**SignalProcessing Controller Dashboard**:

```json
{
  "dashboard": {
    "title": "SignalProcessing Controller - Observability",
    "uid": "signalprocessing-controller",
    "tags": ["kubernaut", "signalprocessing", "controller"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Enrichment Success Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(signalprocessing_enrichment_success_total[5m])) / sum(rate(signalprocessing_enrichment_total[5m]))",
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
            "expr": "histogram_quantile(0.50, rate(signalprocessing_duration_seconds_bucket[5m]))",
            "legendFormat": "P50",
            "refId": "A"
          },
          {
            "expr": "histogram_quantile(0.95, rate(signalprocessing_duration_seconds_bucket[5m]))",
            "legendFormat": "P95 (SLI)",
            "refId": "B"
          },
          {
            "expr": "histogram_quantile(0.99, rate(signalprocessing_duration_seconds_bucket[5m]))",
            "legendFormat": "P99",
            "refId": "C"
          }
        ],
        "yaxes": [
          {"format": "s", "min": 0}
        ],
        "thresholds": [
          {"value": 5, "colorMode": "critical", "op": "gt", "line": true}
        ]
      },
      {
        "id": 3,
        "title": "Phase Distribution",
        "type": "piechart",
        "targets": [
          {
            "expr": "sum by (phase) (signalprocessing_active_total)",
            "legendFormat": "{{phase}}",
            "refId": "A"
          }
        ]
      },
      {
        "id": 4,
        "title": "Categorization Success Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(signalprocessing_categorization_success_total[5m])) / sum(rate(signalprocessing_categorization_total[5m]))",
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
        "id": 5,
        "title": "Degraded Mode Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(signalprocessing_degraded_total[5m])) / sum(rate(signalprocessing_total[5m]))",
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
            "expr": "rate(signalprocessing_created_total[5m])",
            "legendFormat": "Created",
            "refId": "A"
          },
          {
            "expr": "rate(signalprocessing_completed_total[5m])",
            "legendFormat": "Completed",
            "refId": "B"
          },
          {
            "expr": "rate(signalprocessing_failed_total[5m])",
            "legendFormat": "Failed",
            "refId": "C"
          }
        ]
      },
      {
        "id": 7,
        "title": "Audit Write Latency",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, rate(signalprocessing_audit_duration_seconds_bucket[5m]))",
            "legendFormat": "P95 Audit Latency",
            "refId": "A"
          }
        ],
        "yaxes": [
          {"format": "s", "min": 0}
        ]
      },
      {
        "id": 8,
        "title": "Trace Visualization (Jaeger Link)",
        "type": "text",
        "options": {
          "content": "[Open Jaeger Traces](http://jaeger.monitoring.svc:16686/search?service=signalprocessing-controller)"
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
- name: signalprocessing-slos
  interval: 30s
  rules:
  # SLO: Enrichment Success Rate
  - alert: SignalProcessingEnrichmentSLOBreach
    expr: |
      (
        sum(rate(signalprocessing_enrichment_success_total[1h])) /
        sum(rate(signalprocessing_enrichment_total[1h]))
      ) < 0.99
    for: 5m
    labels:
      severity: critical
      slo: enrichment_success_rate
    annotations:
      summary: "SignalProcessing enrichment success rate below SLO"
      description: "Enrichment success rate is {{ $value | humanizePercentage }}, below 99% SLO threshold"
      runbook: "https://docs.kubernaut.io/runbooks/signalprocessing-enrichment-failure"

  # SLO: Processing Latency P95
  - alert: SignalProcessingLatencySLOBreach
    expr: |
      histogram_quantile(0.95,
        rate(signalprocessing_duration_seconds_bucket[5m])
      ) > 5
    for: 10m
    labels:
      severity: warning
      slo: processing_latency_p95
    annotations:
      summary: "SignalProcessing P95 latency exceeds SLO"
      description: "P95 processing latency is {{ $value }}s, exceeding 5s SLO threshold"
      runbook: "https://docs.kubernaut.io/runbooks/signalprocessing-latency"

  # SLO: Categorization Success Rate
  - alert: SignalProcessingCategorizationSLOBreach
    expr: |
      (
        sum(rate(signalprocessing_categorization_success_total[1h])) /
        sum(rate(signalprocessing_categorization_total[1h]))
      ) < 0.99
    for: 5m
    labels:
      severity: critical
      slo: categorization_success_rate
    annotations:
      summary: "SignalProcessing categorization success rate below SLO"
      description: "Categorization success rate is {{ $value | humanizePercentage }}, below 99% SLO"
      runbook: "https://docs.kubernaut.io/runbooks/signalprocessing-categorization-failure"

  # Operational: High Degraded Mode Rate
  - alert: SignalProcessingHighDegradedModeRate
    expr: |
      (
        sum(rate(signalprocessing_degraded_total[5m])) /
        sum(rate(signalprocessing_total[5m]))
      ) > 0.05
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "High SignalProcessing degraded mode rate"
      description: "{{ $value | humanizePercentage }} of signals processed in degraded mode (threshold: 5%)"
      runbook: "https://docs.kubernaut.io/runbooks/signalprocessing-degraded-mode"

  # Operational: Enrichment Phase Stuck
  - alert: SignalProcessingEnrichmentStuck
    expr: |
      time() - signalprocessing_phase_start_timestamp{phase="enriching"} > 300
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "SignalProcessing stuck in enriching phase"
      description: "SignalProcessing {{ $labels.name }} has been in enriching phase for over 5 minutes"
      runbook: "https://docs.kubernaut.io/runbooks/signalprocessing-stuck"

  # Operational: High Failure Rate
  - alert: SignalProcessingHighFailureRate
    expr: |
      (
        sum(rate(signalprocessing_failed_total[5m])) /
        sum(rate(signalprocessing_total[5m]))
      ) > 0.05
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "High SignalProcessing failure rate"
      description: "{{ $value | humanizePercentage }} of SignalProcessing operations failing (threshold: 5%)"
      runbook: "https://docs.kubernaut.io/runbooks/signalprocessing-failures"

  # Operational: Controller Restart Loops
  - alert: SignalProcessingControllerRestartLoop
    expr: |
      rate(kube_pod_container_status_restarts_total{
        namespace="kubernaut-system",
        pod=~"signalprocessing-controller-.*"
      }[15m]) > 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "SignalProcessing controller in restart loop"
      description: "Controller pod {{ $labels.pod }} restarting frequently"
      runbook: "https://docs.kubernaut.io/runbooks/controller-crash-loop"

  # Operational: Audit Write Failures
  - alert: SignalProcessingAuditWriteFailures
    expr: |
      (
        sum(rate(signalprocessing_audit_failed_total[5m])) /
        sum(rate(signalprocessing_audit_total[5m]))
      ) > 0.01
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "SignalProcessing audit write failures detected"
      description: "{{ $value | humanizePercentage }} of audit writes failing (threshold: 1%)"
      runbook: "https://docs.kubernaut.io/runbooks/signalprocessing-audit-failures"
```

---

### Query Examples

**SLI Queries for Dashboards**:

```promql
# 1. Enrichment Success Rate (SLI)
sum(rate(signalprocessing_enrichment_success_total[5m])) /
sum(rate(signalprocessing_enrichment_total[5m]))

# 2. Processing Latency P95 (SLI)
histogram_quantile(0.95,
  rate(signalprocessing_duration_seconds_bucket[5m])
)

# 3. Categorization Success Rate (SLI)
sum(rate(signalprocessing_categorization_success_total[5m])) /
sum(rate(signalprocessing_categorization_total[5m]))

# 4. Degraded Mode Rate (SLI)
sum(rate(signalprocessing_degraded_total[5m])) /
sum(rate(signalprocessing_total[5m]))

# 5. Error Budget Remaining (30-day window)
1 - (
  (1 - 0.99) -  # SLO target: 99%
  (
    1 - (
      sum(increase(signalprocessing_enrichment_success_total[30d])) /
      sum(increase(signalprocessing_enrichment_total[30d]))
    )
  )
) / (1 - 0.99)

# 6. Phase Distribution
sum by (phase) (signalprocessing_active_total)

# 7. Enrichment Duration by Phase
histogram_quantile(0.95,
  rate(signalprocessing_phase_duration_seconds_bucket{phase="enriching"}[5m])
)

# 8. Kubernetes API Query Rate
rate(signalprocessing_kubernetes_api_requests_total[5m])

# 9. CRD Creation Rate
rate(signalprocessing_created_total[5m])

# 10. Active SignalProcessing CRDs
signalprocessing_active_total

# 11. Audit Write Latency P95
histogram_quantile(0.95,
  rate(signalprocessing_audit_duration_seconds_bucket[5m])
)
```

**Troubleshooting Queries**:

```promql
# Find slow enrichments (>5s)
signalprocessing_duration_seconds > 5

# Find SignalProcessings in degraded mode
signalprocessing_active_total{degraded_mode="true"}

# Find CRDs stuck in enriching phase
time() - signalprocessing_phase_start_timestamp{phase="enriching"} > 300

# Find audit write failures
rate(signalprocessing_audit_failed_total[5m])

# Find categorization failures by source
sum by (source) (rate(signalprocessing_categorization_failed_total[5m]))
```

---

### Cross-References

- **DD-005**: Observability Standards (metrics naming, logging format)
- **ADR-015**: Alert-to-Signal naming migration
- **DD-CONTEXT-006**: Context API deprecated (no Context Service metrics)
- **ADR-038**: Async buffered audit ingestion (audit metrics)
- **Implementation Plan**: [IMPLEMENTATION_PLAN_V1.11.md](./IMPLEMENTATION_PLAN_V1.11.md)

---
