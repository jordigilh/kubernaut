# Remediation Orchestrator Metrics (DD-005 Compliant)

**Version**: 1.0
**Date**: 2025-12-07
**Status**: ✅ Implemented
**Reference**: DD-005 Metrics Naming Compliance

---

## Overview

All RO metrics follow the DD-005 naming convention:
```
kubernaut_remediationorchestrator_<metric_name>_{total|seconds|...}
```

**Implementation**: `pkg/remediationorchestrator/metrics/prometheus.go`

---

## Metrics Reference

### Reconciliation Metrics

| Metric | Type | Labels | Description | BR Reference |
|--------|------|--------|-------------|--------------|
| `kubernaut_remediationorchestrator_reconcile_total` | Counter | namespace, phase | Total reconciliation attempts | Standard |
| `kubernaut_remediationorchestrator_reconcile_duration_seconds` | Histogram | namespace, phase | Reconciliation duration | Standard |
| `kubernaut_remediationorchestrator_phase_transitions_total` | Counter | from_phase, to_phase, namespace | Phase transitions | Standard |

### Notification Metrics (BR-ORCH-001, BR-ORCH-036)

| Metric | Type | Labels | Description | BR Reference |
|--------|------|--------|-------------|--------------|
| `kubernaut_remediationorchestrator_approval_notifications_total` | Counter | namespace | Approval notifications created | BR-ORCH-001 |
| `kubernaut_remediationorchestrator_manual_review_notifications_total` | Counter | source, reason, sub_reason, namespace | Manual review notifications | BR-ORCH-036 |

**Label Values for `manual_review_notifications_total`**:

| Label | Values |
|-------|--------|
| `source` | `WorkflowExecution`, `AIAnalysis` |
| `reason` | `ExhaustedRetries`, `PreviousExecutionFailed`, `ExecutionFailure`, `WorkflowResolutionFailed` |
| `sub_reason` | `WorkflowNotFound`, `ImageMismatch`, `ParameterValidationFailed`, `NoMatchingWorkflows`, `LowConfidence`, `LLMParsingError`, `InvestigationInconclusive` |

### No-Action Metrics (BR-ORCH-037, BR-HAPI-200)

| Metric | Type | Labels | Description | BR Reference |
|--------|------|--------|-------------|--------------|
| `kubernaut_remediationorchestrator_no_action_needed_total` | Counter | reason, namespace | Remediations where no action was needed | BR-ORCH-037 |

**Label Values**:
- `reason`: `problem_resolved`, `investigation_inconclusive`

### Child CRD Metrics (BR-ORCH-025)

| Metric | Type | Labels | Description | BR Reference |
|--------|------|--------|-------------|--------------|
| `kubernaut_remediationorchestrator_child_crd_creations_total` | Counter | crd_type, namespace | Child CRD creations | BR-ORCH-025 |

**Label Values**:
- `crd_type`: `SignalProcessing`, `AIAnalysis`, `WorkflowExecution`, `NotificationRequest`

### Deduplication Metrics (BR-ORCH-032, BR-ORCH-033)

| Metric | Type | Labels | Description | BR Reference |
|--------|------|--------|-------------|--------------|
| `kubernaut_remediationorchestrator_duplicates_skipped_total` | Counter | skip_reason, namespace | Duplicate remediations skipped | BR-ORCH-032 |

**Label Values**:
- `skip_reason`: `ResourceBusy`, `RecentlyRemediated`

### Timeout Metrics (BR-ORCH-027, BR-ORCH-028)

| Metric | Type | Labels | Description | BR Reference |
|--------|------|--------|-------------|--------------|
| `kubernaut_remediationorchestrator_timeouts_total` | Counter | phase, namespace | Remediation timeouts | BR-ORCH-027 |

**Label Values**:
- `phase`: `processing`, `analyzing`, `executing`, `global`

---

## Usage Examples

### Prometheus Queries

```promql
# Manual review notifications by source
sum by (source) (
  rate(kubernaut_remediationorchestrator_manual_review_notifications_total[5m])
)

# No-action remediations (problem self-resolved)
sum by (reason) (
  increase(kubernaut_remediationorchestrator_no_action_needed_total[1h])
)

# Reconciliation latency p99
histogram_quantile(0.99,
  rate(kubernaut_remediationorchestrator_reconcile_duration_seconds_bucket[5m])
)

# Timeout rate by phase
sum by (phase) (
  rate(kubernaut_remediationorchestrator_timeouts_total[1h])
)
```

### Grafana Dashboard Panels

**Suggested panels**:
1. **Remediation Outcomes** - Pie chart: Remediated vs NoActionRequired vs ManualReviewRequired
2. **Manual Review by Source** - Time series: WE vs AI failures
3. **Reconciliation Latency** - Heatmap: p50, p90, p99
4. **Timeout Rate** - Time series by phase

---

## Implementation Notes

### Metric Registration

Metrics are registered in `init()` via controller-runtime's metrics registry:

```go
func init() {
    metrics.Registry.MustRegister(
        ReconcileTotal,
        ManualReviewNotificationsTotal,
        // ...
    )
}
```

### Usage in Handlers

```go
import "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"

// In handler code
metrics.ManualReviewNotificationsTotal.WithLabelValues(
    "AIAnalysis",
    "WorkflowResolutionFailed",
    "LowConfidence",
    rr.Namespace,
).Inc()
```

---

## Related Documents

- [DD-005: Metrics Naming Compliance](../../../architecture/decisions/DD-005-metrics-naming-compliance.md)
- [BR-ORCH-036: Manual Review Notification](../../../requirements/BR-ORCH-036-manual-review-notification.md)
- [BR-ORCH-037: WorkflowNotNeeded Handling](../../../requirements/BR-ORCH-037-workflow-not-needed.md)

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2025-12-07 | Initial metrics implementation (DD-005 compliant) |

