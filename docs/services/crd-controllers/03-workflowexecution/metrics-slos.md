## Enhanced Metrics & SLOs

**Version**: 3.1
**Last Updated**: 2025-12-06
**CRD API Group**: `workflowexecution.kubernaut.ai/v1alpha1`
**Status**: ✅ Updated for Tekton Architecture, Exponential Backoff (DD-WE-004)

---

## Changelog

### Version 3.1 (2025-12-06)
- ✅ **Added**: `workflowexecution_backoff_skip_total{reason}` metric (DD-WE-004 v1.1)
- ✅ **Added**: `workflowexecution_consecutive_failures{target_resource}` gauge (pre-execution failures only)
- ✅ **Clarified**: `PreviousExecutionFailed` reason added to backoff_skip_total for execution failure blocking

### Version 3.0 (2025-12-02)
- ✅ **Removed**: All KubernetesExecution metrics (replaced by Tekton PipelineRun)
- ✅ **Added**: `pipelinerun_creation_duration` metric
- ✅ **Added**: Resource locking metrics (DD-WE-001)

---

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

    // NOTE: Step execution metrics are provided by Tekton Pipelines
    // - tekton_pipelinerun_duration_seconds
    // - tekton_taskrun_duration_seconds
    // - tekton_taskrun_count
    // See: Tekton Metrics documentation

    // Phase transition metrics
    phaseTransitionTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_phase_transition_total",
            Help: "Total phase transitions",
        },
        []string{"from_phase", "to_phase"},
    )

    // Skip reason metrics (DD-WE-001 resource locking)
    skipTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_skip_total",
            Help: "Total workflows skipped",
        },
        []string{"reason"},  // ResourceBusy, RecentlyRemediated
    )

    // Outcome metrics
    outcomeTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_outcome_total",
            Help: "Total workflow outcomes",
        },
        []string{"outcome", "workflow_id"},  // outcome: Success, Failed
    )

    // PipelineRun creation metrics
    pipelineRunCreationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_pipelinerun_creation_duration_seconds",
            Help:    "Tekton PipelineRun creation duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.01, 2, 8),  // 10ms to 2.56s
        },
    )

    // Resource locking metrics (DD-WE-001)
    resourceLockCheckDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "workflowexecution_resource_lock_check_duration_seconds",
            Help:    "Resource lock check duration in seconds",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 8),  // 1ms to 256ms
        },
    )

    resourceLockSkipped = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_resource_lock_skipped_total",
            Help: "Total workflows skipped due to resource lock",
        },
        []string{"reason"},  // reason: ResourceBusy|RecentlyRemediated
    )

    // Exponential backoff metrics (DD-WE-004 v1.1, BR-WE-012)
    // NOTE: These metrics ONLY track pre-execution failures (wasExecutionFailure: false)
    // Execution failures (wasExecutionFailure: true) are tracked via skip_total{reason="PreviousExecutionFailed"}
    backoffSkipTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_backoff_skip_total",
            Help: "Total workflow executions skipped due to exponential backoff or execution failure blocking",
        },
        []string{"reason"},  // reason: RecentlyRemediated|ExhaustedRetries|PreviousExecutionFailed
    )

    consecutiveFailures = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "workflowexecution_consecutive_failures",
            Help: "Current consecutive PRE-EXECUTION failure count per target resource (execution failures not counted)",
        },
        []string{"target_resource"},
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
