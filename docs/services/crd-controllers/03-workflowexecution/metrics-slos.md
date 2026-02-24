## Metrics & SLOs

**Version**: 4.0
**Last Updated**: 2025-12-07
**CRD API Group**: `kubernaut.ai/v1alpha1`
**Status**: ✅ Aligned with BR-WE-008, DD-WE-004, DD-005

---

## Changelog

### Version 4.0 (2025-12-07)
- ✅ **BREAKING**: Aligned metrics with BR-WE-008 authoritative requirements
- ✅ **Removed**: Step orchestration metrics (Tekton handles steps per ADR-044)
- ✅ **Removed**: Non-required metrics (active_total, phase_transition_total, parallel_efficiency)
- ✅ **Updated**: Grafana dashboard to use only implemented metrics
- ✅ **Updated**: Alert rules to use only implemented metrics
- ✅ **Updated**: Query examples to use only implemented metrics
- ✅ **Reference**: Implementation at `internal/controller/workflowexecution/metrics.go`

### Version 3.1 (2025-12-06)
- ✅ **Added**: `workflowexecution_backoff_skip_total{reason}` metric (DD-WE-004 v1.1)
- ✅ **Added**: `workflowexecution_consecutive_failures{target_resource}` gauge (pre-execution failures only)

### Version 3.0 (2025-12-02)
- ✅ **Removed**: All KubernetesExecution (DEPRECATED - ADR-025) metrics (replaced by Tekton PipelineRun)
- ✅ **Added**: Resource locking metrics (DD-WE-001)

---

## Authoritative Requirements

### BR-WE-008: Prometheus Metrics for Execution Outcomes

**Source**: [BUSINESS_REQUIREMENTS.md](./BUSINESS_REQUIREMENTS.md#br-we-008-prometheus-metrics-for-execution-outcomes)

**Required Metrics**:
1. `workflowexecution_total{outcome}` - Counter for execution outcomes
2. `workflowexecution_duration_seconds{outcome}` - Histogram for execution duration
3. `workflowexecution_pipelinerun_creation_total` - Counter for PR creation
4. `workflowexecution_skip_total{reason}` - Counter for skipped executions (DD-WE-001)

### DD-WE-004: Exponential Backoff Cooldown

**Source**: [DD-WE-004](../../../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md)

**Additional Metrics**:
5. `workflowexecution_backoff_skip_total{reason}` - Backoff skip visibility
6. `workflowexecution_consecutive_failures{target_resource}` - Real-time failure state

### DD-005: Observability Standards

**Naming Convention**: `{service}_{component}_{metric_name}_{unit}` per [DD-005](../../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)

---

## SLI/SLO Definitions

**Service Level Indicators (SLIs)**:

| SLI | Measurement | Target | Business Impact |
|-----|-------------|--------|----------------|
| **Workflow Success Rate** | `workflowexecution_total{outcome="Completed"} / workflowexecution_total` | ≥98% | Remediation reliability |
| **Workflow Execution Latency (P95)** | `histogram_quantile(0.95, workflowexecution_duration_seconds)` | <120s | Fast remediation completion |
| **Skip Rate** | `workflowexecution_skip_total / (workflowexecution_total + workflowexecution_skip_total)` | <10% | Resource locking efficiency |

**Note**: Step-level SLIs are provided by Tekton Pipelines metrics:
- `tekton_pipelinerun_duration_seconds`
- `tekton_taskrun_duration_seconds`
- `tekton_taskrun_count`

**Service Level Objectives (SLOs)**:

```yaml
slos:
  - name: "WorkflowExecution Success Rate"
    sli: "sum(workflowexecution_total{outcome='Completed'}) / sum(workflowexecution_total)"
    target: 0.98  # 98%
    window: "30d"
    burn_rate_fast: 14.4
    burn_rate_slow: 6

  - name: "WorkflowExecution P95 Latency"
    sli: "histogram_quantile(0.95, rate(workflowexecution_duration_seconds_bucket[5m]))"
    target: 120  # 120 seconds
    window: "30d"

  - name: "Backoff Exhaustion Rate"
    sli: "sum(workflowexecution_backoff_skip_total{reason='ExhaustedRetries'}) / sum(workflowexecution_total)"
    target: 0.01  # <1% - infrastructure issues rare
    window: "7d"
```

---

## Implemented Metrics (BR-WE-008, DD-WE-004)

**Implementation**: `internal/controller/workflowexecution/metrics.go`

```go
package workflowexecution

import (
    "github.com/prometheus/client_golang/prometheus"
    "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// ========================================
// BR-WE-008: Business-Value Metrics
// 4 metrics per BR-WE-008:
// - workflowexecution_total{outcome}
// - workflowexecution_duration_seconds{outcome}
// - workflowexecution_pipelinerun_creation_total
// - workflowexecution_skip_total{reason}
//
// DD-WE-004 Extension (BR-WE-012):
// - workflowexecution_backoff_skip_total{reason}
// - workflowexecution_consecutive_failures{target_resource}
// ========================================

var (
    // WorkflowExecutionTotal tracks total workflow executions by outcome
    // Labels: outcome (Completed, Failed)
    // Business value: SLO success rate tracking
    WorkflowExecutionTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_total",
            Help: "Total number of workflow executions by outcome",
        },
        []string{"outcome"},
    )

    // WorkflowExecutionDuration tracks workflow execution duration
    // Labels: outcome (Completed, Failed)
    // Business value: SLO P95 latency tracking
    WorkflowExecutionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "workflowexecution_duration_seconds",
            Help: "Workflow execution duration in seconds",
            // Buckets: 5s, 10s, 20s, 40s, 80s, 160s, 320s
            Buckets: prometheus.ExponentialBuckets(5, 2, 7),
        },
        []string{"outcome"},
    )

    // PipelineRunCreationTotal tracks PipelineRun creation attempts
    // Business value: Tracks execution initiation success
    PipelineRunCreationTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "workflowexecution_pipelinerun_creation_total",
            Help: "Total number of PipelineRun creations",
        },
    )

    // WorkflowExecutionSkipTotal tracks skipped executions by reason
    // Labels: reason (ResourceBusy, RecentlyRemediated, ExhaustedRetries, PreviousExecutionFailed)
    // Business value: DD-WE-001 resource locking visibility
    WorkflowExecutionSkipTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_skip_total",
            Help: "Total number of skipped workflow executions by reason",
        },
        []string{"reason"},
    )

    // ========================================
    // DD-WE-004 Extension: Backoff Metrics (BR-WE-012)
    // ========================================

    // BackoffSkipTotal tracks workflows skipped due to exponential backoff
    // Labels: reason (ExhaustedRetries, PreviousExecutionFailed)
    // Business value: Visibility into remediation storm prevention
    BackoffSkipTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "workflowexecution_backoff_skip_total",
            Help: "Total workflows skipped due to backoff (ExhaustedRetries, PreviousExecutionFailed)",
        },
        []string{"reason"},
    )

    // ConsecutiveFailuresGauge tracks current consecutive failure count per target
    // Labels: target_resource (e.g., "default/deployment/payment-api")
    // Business value: Real-time visibility into retry state
    ConsecutiveFailuresGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "workflowexecution_consecutive_failures",
            Help: "Current consecutive failure count per target resource",
        },
        []string{"target_resource"},
    )
)

func init() {
    // Register metrics with controller-runtime metrics registry
    metrics.Registry.MustRegister(
        WorkflowExecutionTotal,
        WorkflowExecutionDuration,
        PipelineRunCreationTotal,
        WorkflowExecutionSkipTotal,
        // DD-WE-004 Extension: Backoff metrics (BR-WE-012)
        BackoffSkipTotal,
        ConsecutiveFailuresGauge,
    )
}
```

---

## Metrics Cardinality Audit

**Status**: ✅ SAFE - Total < 100 unique combinations

| Metric | Labels | Max Cardinality | Status |
|--------|--------|-----------------|--------|
| `workflowexecution_total` | outcome(2) | 2 | ✅ |
| `workflowexecution_duration_seconds` | outcome(2) | 2 | ✅ |
| `workflowexecution_pipelinerun_creation_total` | (none) | 1 | ✅ |
| `workflowexecution_skip_total` | reason(4) | 4 | ✅ |
| `workflowexecution_backoff_skip_total` | reason(2) | 2 | ✅ |
| `workflowexecution_consecutive_failures` | target_resource(~100) | ~100 | ✅ |
| **TOTAL** | | **~111** | ✅ |

**Note**: `target_resource` cardinality is bounded by active remediation targets, typically < 100 in production.

---

## Grafana Dashboard JSON

**WorkflowExecution Controller Dashboard** (aligned with BR-WE-008):

```json
{
  "dashboard": {
    "title": "WorkflowExecution Controller - Business Metrics (BR-WE-008)",
    "uid": "workflowexecution-controller",
    "tags": ["kubernaut", "workflowexecution", "controller", "br-we-008"],
    "panels": [
      {
        "id": 1,
        "title": "Workflow Success Rate (SLI)",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(workflowexecution_total{outcome='Completed'}[5m])) / sum(rate(workflowexecution_total[5m]))",
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
        "title": "Workflow Outcomes",
        "type": "graph",
        "targets": [
          {"expr": "sum(rate(workflowexecution_total{outcome='Completed'}[5m]))", "legendFormat": "Completed"},
          {"expr": "sum(rate(workflowexecution_total{outcome='Failed'}[5m]))", "legendFormat": "Failed"}
        ]
      },
      {
        "id": 4,
        "title": "Skip Reasons (DD-WE-001)",
        "type": "graph",
        "targets": [
          {"expr": "sum by (reason) (rate(workflowexecution_skip_total[5m]))", "legendFormat": "{{reason}}"}
        ]
      },
      {
        "id": 5,
        "title": "PipelineRun Creations",
        "type": "stat",
        "targets": [
          {"expr": "rate(workflowexecution_pipelinerun_creation_total[5m])"}
        ]
      },
      {
        "id": 6,
        "title": "Backoff Skip Rate (DD-WE-004)",
        "type": "graph",
        "targets": [
          {"expr": "sum by (reason) (rate(workflowexecution_backoff_skip_total[5m]))", "legendFormat": "{{reason}}"}
        ]
      },
      {
        "id": 7,
        "title": "Consecutive Failures per Target (Top 10)",
        "type": "table",
        "targets": [
          {"expr": "topk(10, workflowexecution_consecutive_failures)", "legendFormat": "{{target_resource}}"}
        ]
      }
    ]
  }
}
```

---

## Alert Rules YAML

**Aligned with BR-WE-008 implemented metrics**:

```yaml
groups:
- name: workflowexecution-slos
  interval: 30s
  rules:
  # SLO: Workflow Success Rate (BR-WE-008)
  - alert: WorkflowExecutionSuccessRateSLOBreach
    expr: |
      (
        sum(rate(workflowexecution_total{outcome="Completed"}[1h])) /
        sum(rate(workflowexecution_total[1h]))
      ) < 0.98
    for: 5m
    labels:
      severity: critical
      slo: workflow_success_rate
      br: BR-WE-008
    annotations:
      summary: "Workflow execution success rate below SLO"
      description: "Workflow success rate is {{ $value | humanizePercentage }}, below 98% SLO"
      runbook: "docs/operations/runbooks/workflowexecution-runbook.md#success-rate-breach"

  # SLO: Workflow Latency P95 (BR-WE-008)
  - alert: WorkflowExecutionLatencySLOBreach
    expr: |
      histogram_quantile(0.95, rate(workflowexecution_duration_seconds_bucket[5m])) > 120
    for: 5m
    labels:
      severity: warning
      slo: workflow_latency_p95
      br: BR-WE-008
    annotations:
      summary: "Workflow execution P95 latency above SLO"
      description: "P95 latency is {{ $value | humanizeDuration }}, above 120s SLO"
      runbook: "docs/operations/runbooks/workflowexecution-runbook.md#latency-breach"

  # Operational: High Skip Rate (DD-WE-001)
  - alert: WorkflowExecutionHighSkipRate
    expr: |
      sum(rate(workflowexecution_skip_total[5m])) /
      (sum(rate(workflowexecution_total[5m])) + sum(rate(workflowexecution_skip_total[5m]))) > 0.10
    for: 10m
    labels:
      severity: warning
      dd: DD-WE-001
    annotations:
      summary: "High workflow skip rate"
      description: "{{ $value | humanizePercentage }} of workflows skipped (threshold: 10%)"
      runbook: "docs/operations/runbooks/workflowexecution-runbook.md#high-skip-rate"

  # Operational: Backoff Exhaustion (DD-WE-004)
  - alert: WorkflowExecutionBackoffExhausted
    expr: |
      sum(rate(workflowexecution_backoff_skip_total{reason="ExhaustedRetries"}[5m])) > 0
    for: 5m
    labels:
      severity: warning
      dd: DD-WE-004
      br: BR-WE-012
    annotations:
      summary: "Workflows exhausting retry attempts"
      description: "Workflows failing due to exhausted retries - infrastructure issue likely"
      runbook: "docs/operations/runbooks/workflowexecution-runbook.md#backoff-exhausted"

  # Operational: Execution Failure Blocking (DD-WE-004)
  - alert: WorkflowExecutionFailureBlocking
    expr: |
      sum(rate(workflowexecution_backoff_skip_total{reason="PreviousExecutionFailed"}[5m])) > 0
    for: 5m
    labels:
      severity: critical
      dd: DD-WE-004
      br: BR-WE-012
    annotations:
      summary: "Workflows blocked by previous execution failure"
      description: "Workflows blocked due to previous execution failure - manual review required"
      runbook: "docs/operations/runbooks/workflowexecution-runbook.md#execution-failure-blocking"

  # Operational: High Consecutive Failures (DD-WE-004)
  - alert: WorkflowExecutionHighConsecutiveFailures
    expr: |
      workflowexecution_consecutive_failures >= 3
    for: 5m
    labels:
      severity: warning
      dd: DD-WE-004
    annotations:
      summary: "Target resource has high consecutive failures"
      description: "Target {{ $labels.target_resource }} has {{ $value }} consecutive failures"
      runbook: "docs/operations/runbooks/workflowexecution-runbook.md#high-consecutive-failures"
```

---

## Query Examples

**Business Metrics Queries (BR-WE-008)**:

```promql
# 1. Workflow Success Rate (SLI)
sum(rate(workflowexecution_total{outcome="Completed"}[5m])) /
sum(rate(workflowexecution_total[5m]))

# 2. Average Workflow Duration
rate(workflowexecution_duration_seconds_sum[5m]) /
rate(workflowexecution_duration_seconds_count[5m])

# 3. Workflow Duration P95
histogram_quantile(0.95, rate(workflowexecution_duration_seconds_bucket[5m]))

# 4. PipelineRun Creation Rate
rate(workflowexecution_pipelinerun_creation_total[5m])

# 5. Skip Rate by Reason (DD-WE-001)
sum by (reason) (rate(workflowexecution_skip_total[5m]))

# 6. Backoff Skip Rate by Reason (DD-WE-004)
sum by (reason) (rate(workflowexecution_backoff_skip_total[5m]))

# 7. Targets with Consecutive Failures (DD-WE-004)
topk(10, workflowexecution_consecutive_failures > 0)

# 8. Total Workflows (Success + Failed + Skipped)
sum(rate(workflowexecution_total[5m])) + sum(rate(workflowexecution_skip_total[5m]))
```

**Tekton Metrics (Step-Level - Provided by Tekton)**:

```promql
# Step-level metrics are handled by Tekton Pipelines:
# - tekton_pipelinerun_duration_seconds
# - tekton_taskrun_duration_seconds
# - tekton_taskrun_count

# Example: Tekton PipelineRun duration for Kubernaut workflows
histogram_quantile(0.95, rate(tekton_pipelinerun_duration_seconds_bucket{
  pipeline=~"kubernaut-.*"
}[5m]))
```

---

## Step-Level Observability Note

Per **ADR-044** (Workflow Execution Engine Delegation), step orchestration is delegated to Tekton Pipelines. Therefore:

1. **Kubernaut WE Controller** provides:
   - Workflow lifecycle metrics (BR-WE-008)
   - Resource locking visibility (DD-WE-001)
   - Backoff state tracking (DD-WE-004)

2. **Tekton Pipelines** provides:
   - Step execution metrics
   - Task duration metrics
   - Pipeline parallelism metrics

This separation follows the principle of "delegate to specialized engines" and avoids metric duplication.

---

## References

- [BR-WE-008: Prometheus Metrics](./BUSINESS_REQUIREMENTS.md#br-we-008-prometheus-metrics-for-execution-outcomes)
- [DD-WE-004: Exponential Backoff Cooldown](../../../architecture/decisions/DD-WE-004-exponential-backoff-cooldown.md)
- [DD-WE-001: Resource Locking Safety](../../../architecture/decisions/DD-WE-001-resource-locking-safety.md)
- [DD-005: Observability Standards](../../../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)
- [ADR-044: Workflow Execution Engine Delegation](../../../architecture/decisions/ADR-044-workflow-execution-engine-delegation.md)

---
