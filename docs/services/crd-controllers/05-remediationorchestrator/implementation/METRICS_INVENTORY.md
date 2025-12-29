# Metrics Inventory - Remediation Orchestrator

**Parent Document**: [IMPLEMENTATION_PLAN_V1.1.md](./IMPLEMENTATION_PLAN_V1.1.md)
**Template Source**: SERVICE_IMPLEMENTATION_PLAN_TEMPLATE V3.0
**Last Updated**: 2025-12-04

---

## Metrics Summary

| Category | Count | Cardinality Risk |
|----------|-------|------------------|
| Core Reconciliation | 4 | Low |
| Phase Transitions | 3 | Medium |
| Child CRD Creation | 2 | Low |
| Business Outcomes | 4 | Low |
| Performance | 3 | Low |
| **Total** | **16** | **Medium** |

---

## Core Metrics

### 1. kubernaut_orchestrator_reconciliation_duration_seconds

**Type**: Histogram
**Labels**: `namespace`
**Buckets**: 10ms to ~10s (exponential)
**Cardinality**: ~50

```go
ReconciliationDuration: *prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "kubernaut_orchestrator_reconciliation_duration_seconds",
        Help:    "Duration of reconciliation loops in seconds",
        Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
    },
    []string{"namespace"},
)
```

### 2. kubernaut_orchestrator_reconciliation_errors_total

**Type**: Counter
**Labels**: `namespace`, `error_type`
**Error Types**: `fetch_error`, `child_creation_error`, `status_update_error`, `status_aggregation_error`, `timeout_error`
**Cardinality**: ~300

### 3. kubernaut_orchestrator_active_remediations

**Type**: Gauge
**Labels**: `namespace`, `phase`
**Phases**: `Pending`, `Processing`, `Analyzing`, `AwaitingApproval`, `Executing`, `Completed`, `Failed`, `TimedOut`, `Skipped`
**Cardinality**: ~450

### 4. kubernaut_orchestrator_phase_transitions_total

**Type**: Counter
**Labels**: `namespace`, `phase`
**Cardinality**: ~450

---

## Phase Metrics

### 5. kubernaut_orchestrator_phase_duration_seconds

**Type**: Histogram
**Labels**: `namespace`, `phase`
**Buckets**: 1s to ~68min (exponential)
**Cardinality**: ~450

### 6. kubernaut_orchestrator_phase_timeouts_total

**Type**: Counter
**Labels**: `namespace`, `phase`
**Purpose**: Track BR-ORCH-027, BR-ORCH-028 violations
**Cardinality**: ~450

---

## Child CRD Metrics

### 7. kubernaut_orchestrator_child_crd_creations_total

**Type**: Counter
**Labels**: `namespace`, `crd_type`
**CRD Types**: `SignalProcessing`, `AIAnalysis`, `WorkflowExecution`, `NotificationRequest`
**Cardinality**: ~200

### 8. kubernaut_orchestrator_child_crd_creation_seconds

**Type**: Histogram
**Labels**: `namespace`, `crd_type`
**Buckets**: 1ms to ~1s (exponential)
**Cardinality**: ~200

---

## Business Metrics

### 9. kubernaut_orchestrator_remediations_completed_total

**Type**: Counter
**Labels**: `namespace`
**Cardinality**: ~50

### 10. kubernaut_orchestrator_remediations_failed_total

**Type**: Counter
**Labels**: `namespace`, `failure_phase`
**Cardinality**: ~450

### 11. kubernaut_orchestrator_approvals_required_total

**Type**: Counter
**Labels**: `namespace`
**Purpose**: Track BR-ORCH-001
**Cardinality**: ~50

### 12. kubernaut_orchestrator_duplicates_total

**Type**: Counter
**Labels**: `namespace`, `skip_reason`
**Skip Reasons**: `ResourceBusy`, `RecentlyRemediated`
**Purpose**: Track BR-ORCH-033
**Cardinality**: ~100

---

## Performance Metrics

### 13. kubernaut_orchestrator_remediation_duration_seconds

**Type**: Histogram
**Labels**: `namespace`, `outcome`
**Outcomes**: `Completed`, `Failed`, `TimedOut`, `Skipped`
**Buckets**: 1min to ~17h (exponential)
**Cardinality**: ~200

### 14. kubernaut_orchestrator_status_update_seconds

**Type**: Histogram
**Labels**: `namespace`
**Cardinality**: ~50

### 15. kubernaut_orchestrator_status_update_conflicts_total

**Type**: Counter
**Labels**: `namespace`
**Purpose**: Track optimistic locking conflicts
**Cardinality**: ~50

---

## Cardinality Protection

### Bounded Label Values

```go
var validPhases = map[string]bool{
    "Pending": true, "Processing": true, "Analyzing": true,
    "AwaitingApproval": true, "Executing": true,
    "Completed": true, "Failed": true, "TimedOut": true, "Skipped": true,
}

func sanitizePhase(phase string) string {
    if validPhases[phase] {
        return phase
    }
    return "unknown"
}
```

### Cardinality Limits

| Label | Maximum Values |
|-------|----------------|
| `namespace` | 50 |
| `phase` | 9 |
| `error_type` | 6 |
| `crd_type` | 4 |
| `outcome` | 4 |
| `skip_reason` | 2 |

### Total Cardinality Summary

**Total Maximum Cardinality**: ~3,500 time series
**Risk Assessment**: LOW

---

## Alerting Rules

```yaml
groups:
  - name: remediation-orchestrator
    rules:
      - alert: ROHighErrorRate
        expr: |
          sum(rate(kubernaut_orchestrator_reconciliation_errors_total[5m]))
          / sum(rate(kubernaut_orchestrator_reconciliation_duration_seconds_count[5m]))
          > 0.1
        for: 5m
        labels:
          severity: warning

      - alert: ROPhaseTimeouts
        expr: sum(increase(kubernaut_orchestrator_phase_timeouts_total[1h])) > 10
        labels:
          severity: warning

      - alert: ROHighFailureRate
        expr: |
          sum(rate(kubernaut_orchestrator_remediations_failed_total[1h]))
          / (sum(rate(kubernaut_orchestrator_remediations_completed_total[1h]))
             + sum(rate(kubernaut_orchestrator_remediations_failed_total[1h])))
          > 0.2
        for: 30m
        labels:
          severity: critical
```

---

## Audit Checklist

- [ ] All metrics registered in controller initialization
- [ ] Labels have bounded cardinality
- [ ] Histogram buckets appropriate for expected ranges
- [ ] No high-cardinality labels (names, IDs)
- [ ] Alerting rules defined
- [ ] Grafana dashboard created

