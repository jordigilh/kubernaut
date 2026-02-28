## Enhanced Metrics & SLOs

> **DEPRECATED**: KubernetesExecution CRD and KubernetesExecutor service were eliminated by ADR-025 and replaced by Tekton TaskRun via WorkflowExecution. This documentation is retained for historical reference only. API types and CRD manifests have been removed from the codebase.

### SLI/SLO Definitions

**Service Level Indicators (SLIs)**:

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **Action Execution Success Rate** | `successful_actions / total_actions` | ≥99% | Kubernetes operation reliability |
| **Job Creation Latency (P95)** | `histogram_quantile(0.95, kubernetesexecution_job_creation_duration_seconds)` | <5s | Fast Job startup |
| **Action Execution Latency (P95)** | `histogram_quantile(0.95, kubernetesexecution_execution_duration_seconds)` | <30s | Fast remediation actions |
| **Dry-Run Validation Success Rate** | `dryrun_success / dryrun_total` | ≥99.5% | Pre-execution validation |
| **Per-Action Success Rate** | `successful_actions_by_type / total_actions_by_type` | ≥98% | Action-specific reliability |

**Service Level Objectives (SLOs)**:

```yaml
slos:
  - name: "KubernetesExecution Action Success Rate"
    sli: "successful_actions / total_actions"
    target: 0.99  # 99%
    window: "30d"
    burn_rate_fast: 14.4
    burn_rate_slow: 6

  - name: "KubernetesExecution P95 Execution Latency"
    sli: "histogram_quantile(0.95, kubernetesexecution_execution_duration_seconds)"
    target: 30  # 30 seconds
    window: "30d"

  - name: "Dry-Run Validation Success Rate"
    sli: "dryrun_success / dryrun_total"
    target: 0.995  # 99.5%
    window: "30d"
```

---

### Service-Specific Metrics

**Job Execution & Per-Action Metrics**:

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Action execution metrics
    actionExecutionTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernetesexecution_action_execution_total",
            Help: "Total number of action executions",
        },
        []string{"status", "action"},  // status: completed|failed, action: restart-pod|scale-deployment|etc
    )

    actionExecutionDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kubernetesexecution_action_execution_duration_seconds",
            Help:    "Action execution duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.5, 2, 10),  // 0.5s to 512s
        },
        []string{"action"},
    )

    // Kubernetes Job metrics
    jobCreationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernetesexecution_job_creation_total",
            Help: "Total number of Kubernetes Jobs created",
        },
        []string{"status"},  // status: success|error
    )

    jobCreationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "kubernetesexecution_job_creation_duration_seconds",
            Help:    "Kubernetes Job creation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),  // 10ms to 10s
        },
    )

    jobExecutionDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kubernetesexecution_job_execution_duration_seconds",
            Help:    "Kubernetes Job execution duration (pod runtime) in seconds",
            Buckets: prometheus.ExponentialBuckets(0.5, 2, 10),  // 0.5s to 512s
        },
        []string{"action"},
    )

    jobActive = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "kubernetesexecution_job_active_total",
            Help: "Number of currently active Kubernetes Jobs",
        },
    )

    // Dry-run validation metrics
    dryrunValidationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernetesexecution_dryrun_validation_total",
            Help: "Total number of dry-run validations",
        },
        []string{"status", "action"},  // status: success|error
    )

    dryrunValidationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kubernetesexecution_dryrun_validation_duration_seconds",
            Help:    "Dry-run validation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 8),  // 10ms to 2.56s
        },
        []string{"action"},
    )

    // RBAC creation metrics
    rbacCreationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kubernetesexecution_rbac_creation_duration_seconds",
            Help:    "Per-action RBAC creation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 8),  // 10ms to 2.56s
        },
        []string{"action"},
    )

    serviceAccountCreationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kubernetesexecution_serviceaccount_creation_duration_seconds",
            Help:    "ServiceAccount creation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 8),  // 10ms to 2.56s
        },
        []string{"action"},
    )

    // Per-action success rate metrics
    perActionSuccessRate = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "kubernetesexecution_per_action_success_rate",
            Help: "Success rate per action type (0-1)",
        },
        []string{"action"},
    )

    // kubectl command execution metrics
    kubectlCommandExecutionTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kubernetesexecution_kubectl_command_execution_total",
            Help: "Total number of kubectl commands executed",
        },
        []string{"status", "command"},  // status: success|error, command: delete|patch|scale|etc
    )

    kubectlCommandExecutionDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kubernetesexecution_kubectl_command_execution_duration_seconds",
            Help:    "kubectl command execution duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),  // 100ms to 100s
        },
        []string{"command"},
    )
)
```

---

### Grafana Dashboard JSON

**KubernetesExecution Controller Dashboard** (key panels):

```json
{
  "dashboard": {
    "title": "KubernetesExecution Controller - Job Execution Observability",
    "uid": "kubernetesexecution-controller",
    "tags": ["kubernaut", "kubernetesexecution", "jobs", "controller"],
    "panels": [
      {
        "id": 1,
        "title": "Action Execution Success Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(kubernetesexecution_action_execution_total{status='completed'}[5m])) / sum(rate(kubernetesexecution_action_execution_total[5m]))",
            "legendFormat": "Success Rate"
          }
        ],
        "thresholds": [{"value": 0.99, "colorMode": "critical", "op": "lt"}]
      },
      {
        "id": 2,
        "title": "Per-Action Success Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "sum by (action) (rate(kubernetesexecution_action_execution_total{status='completed'}[5m])) / sum by (action) (rate(kubernetesexecution_action_execution_total[5m]))"
          }
        ],
        "thresholds": [{"value": 0.98, "colorMode": "warning", "op": "lt"}]
      },
      {
        "id": 3,
        "title": "Job Creation Latency (P50, P95, P99)",
        "type": "graph",
        "targets": [
          {"expr": "histogram_quantile(0.50, rate(kubernetesexecution_job_creation_duration_seconds_bucket[5m]))", "legendFormat": "P50"},
          {"expr": "histogram_quantile(0.95, rate(kubernetesexecution_job_creation_duration_seconds_bucket[5m]))", "legendFormat": "P95 (SLI)"},
          {"expr": "histogram_quantile(0.99, rate(kubernetesexecution_job_creation_duration_seconds_bucket[5m]))", "legendFormat": "P99"}
        ],
        "thresholds": [{"value": 5, "colorMode": "critical", "op": "gt"}]
      },
      {
        "id": 4,
        "title": "Action Execution Duration by Action Type",
        "type": "heatmap",
        "targets": [
          {"expr": "sum by (action, le) (rate(kubernetesexecution_action_execution_duration_seconds_bucket[5m]))"}
        ]
      },
      {
        "id": 5,
        "title": "Active Kubernetes Jobs",
        "type": "stat",
        "targets": [
          {"expr": "kubernetesexecution_job_active_total"}
        ]
      },
      {
        "id": 6,
        "title": "Dry-Run Validation Success Rate",
        "type": "graph",
        "targets": [
          {"expr": "sum(rate(kubernetesexecution_dryrun_validation_total{status='success'}[5m])) / sum(rate(kubernetesexecution_dryrun_validation_total[5m]))", "legendFormat": "Validation Success"}
        ]
      },
      {
        "id": 7,
        "title": "RBAC Setup Performance",
        "type": "graph",
        "targets": [
          {"expr": "histogram_quantile(0.95, rate(kubernetesexecution_rbac_creation_duration_seconds_bucket[5m]))", "legendFormat": "P95"}
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
- name: kubernetesexecution-slos
  interval: 30s
  rules:
  # SLO: Action Execution Success Rate
  - alert: KubernetesExecutionActionSuccessRateSLOBreach
    expr: |
      (
        sum(rate(kubernetesexecution_action_execution_total{status="completed"}[1h])) /
        sum(rate(kubernetesexecution_action_execution_total[1h]))
      ) < 0.99
    for: 5m
    labels:
      severity: critical
      slo: action_success_rate
    annotations:
      summary: "Action execution success rate below SLO"
      description: "Action success rate is {{ $value | humanizePercentage }}, below 99% SLO"

  # SLO: Dry-Run Validation Success Rate
  - alert: KubernetesExecutionDryRunValidationSLOBreach
    expr: |
      (
        sum(rate(kubernetesexecution_dryrun_validation_total{status="success"}[1h])) /
        sum(rate(kubernetesexecution_dryrun_validation_total[1h]))
      ) < 0.995
    for: 5m
    labels:
      severity: critical
      slo: dryrun_validation_success_rate
    annotations:
      summary: "Dry-run validation success rate below SLO"
      description: "Dry-run validation success rate is {{ $value | humanizePercentage }}, below 99.5% SLO"

  # Operational: Per-Action High Failure Rate
  - alert: KubernetesExecutionPerActionHighFailureRate
    expr: |
      (
        sum by (action) (rate(kubernetesexecution_action_execution_total{status="failed"}[5m])) /
        sum by (action) (rate(kubernetesexecution_action_execution_total[5m]))
      ) > 0.02
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "High failure rate for action {{ $labels.action }}"
      description: "{{ $value | humanizePercentage }} of {{ $labels.action }} actions failing (threshold: 2%)"

  # Operational: Job Creation Slow
  - alert: KubernetesExecutionJobCreationSlow
    expr: |
      histogram_quantile(0.95, rate(kubernetesexecution_job_creation_duration_seconds_bucket[5m])) > 5
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Kubernetes Job creation is slow"
      description: "P95 Job creation latency is {{ $value }}s, exceeding 5s threshold"

  # Operational: Action Stuck (long execution)
  - alert: KubernetesExecutionActionStuck
    expr: |
      time() - kubernetesexecution_action_start_timestamp > 300
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Action execution stuck"
      description: "Action {{ $labels.name }} has been executing for over 5 minutes"
```

---

### Query Examples

**Job Execution & Per-Action Queries**:

```promql
# 1. Action Success Rate (SLI)
sum(rate(kubernetesexecution_action_execution_total{status="completed"}[5m])) /
sum(rate(kubernetesexecution_action_execution_total[5m]))

# 2. Per-Action Success Rate
sum by (action) (rate(kubernetesexecution_action_execution_total{status="completed"}[5m])) /
sum by (action) (rate(kubernetesexecution_action_execution_total[5m]))

# 3. Job Creation P95 Latency
histogram_quantile(0.95,
  rate(kubernetesexecution_job_creation_duration_seconds_bucket[5m])
)

# 4. Action Execution P95 by Action Type
histogram_quantile(0.95,
  sum by (action, le) (rate(kubernetesexecution_action_execution_duration_seconds_bucket[5m]))
)

# 5. Dry-Run Validation Success Rate
sum(rate(kubernetesexecution_dryrun_validation_total{status="success"}[5m])) /
sum(rate(kubernetesexecution_dryrun_validation_total[5m]))

# 6. Active Jobs
kubernetesexecution_job_active_total

# 7. kubectl Command Success Rate
sum(rate(kubernetesexecution_kubectl_command_execution_total{status="success"}[5m])) /
sum(rate(kubernetesexecution_kubectl_command_execution_total[5m]))

# 8. RBAC Creation P95
histogram_quantile(0.95,
  rate(kubernetesexecution_rbac_creation_duration_seconds_bucket[5m])
)
```

---
