## Enhanced Metrics & SLOs

### SLI/SLO Definitions

**Service Level Indicators (SLIs)**:

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **Workflow Success Rate** | `successful_workflows / total_workflows` | ≥98% | Remediation reliability |
| **Workflow Execution Latency (P95)** | `histogram_quantile(0.95, workflowexecution_duration_seconds)` | <120s | Fast remediation completion |
| **Step Success Rate** | `successful_steps / total_steps` | ≥99% | Individual action reliability |
| **Parallel Execution Efficiency** | `parallel_steps / total_steps` | ≥30% | Resource optimization |
| **Dependency Resolution Time (P95)** | `histogram_quantile(0.95, workflowexecution_dependency_resolution_seconds)` | <5s | Fast orchestration startup |

**Service Level Objectives (SLOs)**:

```yaml
slos:
  - name: "WorkflowExecution Success Rate"
    sli: "successful_workflows / total_workflows"
    target: 0.98  # 98%
    window: "30d"
    burn_rate_fast: 14.4
    burn_rate_slow: 6

  - name: "WorkflowExecution P95 Latency"
    sli: "histogram_quantile(0.95, workflowexecution_duration_seconds)"
    target: 120  # 120 seconds
    window: "30d"

  - name: "Step Success Rate"
    sli: "successful_steps / total_steps"
    target: 0.99  # 99%
    window: "30d"
```

---

### Service-Specific Metrics

**Workflow Orchestration Metrics**:

```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Workflow lifecycle metrics
    workflowTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_total",
            Help: "Total number of workflow executions",
        },
        []string{"status"},  // status: completed|failed
    )

    workflowDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_duration_seconds",
            Help:    "Workflow execution duration in seconds",
            Buckets: prometheus.ExponentialBuckets(5, 2, 10),  // 5s to 5120s
        },
    )

    workflowActive = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "workflowexecution_active_total",
            Help: "Number of currently executing workflows",
        },
    )

    // Step execution metrics
    stepExecutionTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_step_execution_total",
            Help: "Total number of step executions",
        },
        []string{"status", "action"},  // status: completed|failed, action: restart-pod|scale-deployment|etc
    )

    stepExecutionDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_step_execution_duration_seconds",
            Help:    "Step execution duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.5, 2, 10),  // 0.5s to 512s
        },
        []string{"action"},
    )

    // Dependency resolution metrics
    dependencyResolutionDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_dependency_resolution_duration_seconds",
            Help:    "Dependency graph resolution duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),  // 10ms to 10s
        },
    )

    dependencyGraphComplexity = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_dependency_graph_complexity",
            Help:    "Number of dependencies in workflow graph",
            Buckets: prometheus.LinearBuckets(0, 2, 10),  // 0 to 20
        },
    )

    // Parallel execution metrics
    parallelStepsExecuted = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_parallel_steps_count",
            Help:    "Number of steps executed in parallel",
            Buckets: prometheus.LinearBuckets(1, 1, 10),  // 1 to 10
        },
    )

    parallelExecutionEfficiency = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "workflowexecution_parallel_execution_efficiency",
            Help: "Ratio of parallel steps to total steps (0-1)",
        },
    )

    // Workflow complexity metrics
    workflowStepCount = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_step_count",
            Help:    "Number of steps in workflow",
            Buckets: prometheus.LinearBuckets(1, 1, 10),  // 1 to 10 steps
        },
    )

    // Child CRD creation metrics
    kubernetesExecutionCreationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_kubernetesexecution_creation_duration_seconds",
            Help:    "KubernetesExecution CRD creation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 8),  // 10ms to 2.56s
        },
    )
)
```

---

### Grafana Dashboard JSON

**WorkflowExecution Controller Dashboard** (key panels):

```json
{
  "dashboard": {
    "title": "WorkflowExecution Controller - Orchestration Observability",
    "uid": "workflowexecution-controller",
    "tags": ["kubernaut", "workflowexecution", "orchestration", "controller"],
    "panels": [
      {
        "id": 1,
        "title": "Workflow Success Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(workflowexecution_total{status='completed'}[5m])) / sum(rate(workflowexecution_total[5m]))",
            "legendFormat": "Success Rate"
          }
        ],
        "thresholds": [{"value": 0.98, "colorMode": "critical", "op": "lt"}]
      },
      {
        "id": 2,
        "title": "Workflow Execution Latency (P50, P95, P99)",
        "type": "graph",
        "targets": [
          {"expr": "histogram_quantile(0.50, rate(workflowexecution_duration_seconds_bucket[5m]))", "legendFormat": "P50"},
          {"expr": "histogram_quantile(0.95, rate(workflowexecution_duration_seconds_bucket[5m]))", "legendFormat": "P95 (SLI)"},
          {"expr": "histogram_quantile(0.99, rate(workflowexecution_duration_seconds_bucket[5m]))", "legendFormat": "P99"}
        ],
        "thresholds": [{"value": 120, "colorMode": "critical", "op": "gt"}]
      },
      {
        "id": 3,
        "title": "Step Success Rate by Action",
        "type": "graph",
        "targets": [
          {"expr": "sum by (action) (rate(workflowexecution_step_execution_total{status='completed'}[5m])) / sum by (action) (rate(workflowexecution_step_execution_total[5m]))"}
        ]
      },
      {
        "id": 4,
        "title": "Parallel Execution Efficiency",
        "type": "graph",
        "targets": [
          {"expr": "workflowexecution_parallel_execution_efficiency", "legendFormat": "Efficiency (parallel/total)"}
        ]
      },
      {
        "id": 5,
        "title": "Active Workflows",
        "type": "stat",
        "targets": [
          {"expr": "workflowexecution_active_total"}
        ]
      },
      {
        "id": 6,
        "title": "Dependency Resolution Performance",
        "type": "graph",
        "targets": [
          {"expr": "histogram_quantile(0.95, rate(workflowexecution_dependency_resolution_duration_seconds_bucket[5m]))", "legendFormat": "P95"}
        ]
      },
      {
        "id": 7,
        "title": "Step Execution Duration by Action",
        "type": "heatmap",
        "targets": [
          {"expr": "sum by (action, le) (rate(workflowexecution_step_execution_duration_seconds_bucket[5m]))"}
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
- name: workflowexecution-slos
  interval: 30s
  rules:
  # SLO: Workflow Success Rate
  - alert: WorkflowExecutionSuccessRateSLOBreach
    expr: |
      (
        sum(rate(workflowexecution_total{status="completed"}[1h])) /
        sum(rate(workflowexecution_total[1h]))
      ) < 0.98
    for: 5m
    labels:
      severity: critical
      slo: workflow_success_rate
    annotations:
      summary: "Workflow execution success rate below SLO"
      description: "Workflow success rate is {{ $value | humanizePercentage }}, below 98% SLO"

  # SLO: Step Success Rate
  - alert: WorkflowStepSuccessRateSLOBreach
    expr: |
      (
        sum(rate(workflowexecution_step_execution_total{status="completed"}[1h])) /
        sum(rate(workflowexecution_step_execution_total[1h]))
      ) < 0.99
    for: 5m
    labels:
      severity: critical
      slo: step_success_rate
    annotations:
      summary: "Workflow step success rate below SLO"
      description: "Step success rate is {{ $value | humanizePercentage }}, below 99% SLO"

  # Operational: Low Parallel Execution Efficiency
  - alert: WorkflowLowParallelEfficiency
    expr: |
      workflowexecution_parallel_execution_efficiency < 0.30
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "Low parallel execution efficiency"
      description: "Only {{ $value | humanizePercentage }} of steps executed in parallel (threshold: 30%)"

  # Operational: High Step Failure Rate for Specific Action
  - alert: WorkflowStepHighFailureRate
    expr: |
      (
        sum by (action) (rate(workflowexecution_step_execution_total{status="failed"}[5m])) /
        sum by (action) (rate(workflowexecution_step_execution_total[5m]))
      ) > 0.05
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "High step failure rate for action {{ $labels.action }}"
      description: "{{ $value | humanizePercentage }} of {{ $labels.action }} steps failing (threshold: 5%)"

  # Operational: Workflow Stuck (long execution time)
  - alert: WorkflowExecutionStuck
    expr: |
      time() - workflowexecution_start_timestamp > 600
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Workflow execution stuck"
      description: "Workflow {{ $labels.name }} has been executing for over 10 minutes"
```

---

### Query Examples

**Orchestration-Specific Queries**:

```promql
# 1. Workflow Success Rate (SLI)
sum(rate(workflowexecution_total{status="completed"}[5m])) /
sum(rate(workflowexecution_total[5m]))

# 2. Step Success Rate by Action
sum by (action) (rate(workflowexecution_step_execution_total{status="completed"}[5m])) /
sum by (action) (rate(workflowexecution_step_execution_total[5m]))

# 3. Parallel Execution Efficiency
workflowexecution_parallel_execution_efficiency

# 4. Average Workflow Duration
rate(workflowexecution_duration_seconds_sum[5m]) /
rate(workflowexecution_duration_seconds_count[5m])

# 5. Dependency Resolution P95
histogram_quantile(0.95,
  rate(workflowexecution_dependency_resolution_duration_seconds_bucket[5m])
)

# 6. Active Workflows by Phase
workflowexecution_active_total

# 7. Step Execution Rate
sum(rate(workflowexecution_step_execution_total[5m]))

# 8. Average Steps per Workflow
rate(workflowexecution_step_count_sum[5m]) /
rate(workflowexecution_step_count_count[5m])
```

---
