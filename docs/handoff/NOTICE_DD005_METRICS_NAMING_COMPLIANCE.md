# NOTICE: DD-005 Metrics Naming Compliance Gap

**Date**: 2025-12-06
**Status**: 🔴 **COMPLIANCE GAP IDENTIFIED**
**Severity**: Medium
**From**: AIAnalysis Team
**To**: WorkflowExecution Team, Notification Team, SignalProcessing Team, RemediationOrchestrator Team
**Authoritative Document**: [DD-005-OBSERVABILITY-STANDARDS.md](../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md)

---

## 📬 **Handoff Routing**

| Field | Value |
|-------|-------|
| **From** | AIAnalysis Team |
| **To** | WorkflowExecution Team, Notification Team, SignalProcessing Team, RemediationOrchestrator Team |
| **CC** | Platform/Observability Team |
| **Action Required** | Triage and acknowledge; schedule remediation |
| **Response Deadline** | Before respective service's next major release |

---

## 📋 **Summary**

The authoritative metrics naming convention in DD-005 is **NOT being followed** by existing CRD controllers. This creates inconsistency and confusion for new implementations.

---

## 🎯 **DD-005 Standard (AUTHORITATIVE)**

**Format**: `{service}_{component}_{metric_name}_{unit}`

**Rules**:
- **Service prefix**: `gateway_`, `aianalysis_`, `workflowexecution_`, etc.
- **Component**: Logical component (e.g., `reconciler_`, `holmesgpt_`, `rego_`)
- **Metric name**: Descriptive name in snake_case
- **Unit suffix**: `_total`, `_seconds`, `_bytes`, `_ratio`

**Examples** (DD-005 compliant):
```
gateway_signals_received_total
gateway_http_request_duration_seconds
aianalysis_reconciler_duration_seconds
aianalysis_holmesgpt_requests_total
aianalysis_rego_evaluations_total
```

---

## ❌ **Non-Compliant Implementations**

### **WorkflowExecution Controller**

| Current (Non-Compliant) | DD-005 Compliant |
|-------------------------|------------------|
| `workflowexecution_total` | `workflowexecution_reconciler_total` |
| `workflowexecution_duration_seconds` | `workflowexecution_reconciler_duration_seconds` |
| `workflowexecution_skip_total` | `workflowexecution_reconciler_skip_total` |
| `workflowexecution_pipelinerun_creation_total` | `workflowexecution_tekton_pipelinerun_creation_total` |

### **Notification Controller**

| Current (Non-Compliant) | DD-005 Compliant |
|-------------------------|------------------|
| `notification_deliveries_total` | `notification_delivery_requests_total` |
| `notification_failure_rate` | `notification_delivery_failure_rate` |
| `notification_stuck_duration_seconds` | `notification_delivery_stuck_duration_seconds` |
| `notification_slack_retry_count` | `notification_slack_retries_total` |

---

## ✅ **Required Actions**

### **Immediate (AIAnalysis - Day 5)**
- [ ] AIAnalysis MUST follow DD-005 naming convention
- [ ] Use component-level granularity:
  - `aianalysis_reconciler_*` - Reconciliation metrics
  - `aianalysis_holmesgpt_*` - HolmesGPT-API call metrics
  - `aianalysis_rego_*` - Rego policy evaluation metrics

### **Follow-up (Existing Controllers)**
- [ ] Create migration plan for WorkflowExecution metrics
- [ ] Create migration plan for Notification metrics
- [ ] Update Prometheus alert rules if any exist
- [ ] Update Grafana dashboards if any exist

---

## 📊 **AIAnalysis DD-005 Compliant Metrics**

```go
// pkg/aianalysis/metrics/metrics.go

var (
    // Reconciler metrics
    ReconcilerReconciliationsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_reconciler_reconciliations_total",
            Help: "Total number of AIAnalysis reconciliations",
        },
        []string{"phase", "result"},
    )

    ReconcilerDurationSeconds = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_reconciler_duration_seconds",
            Help:    "Duration of AIAnalysis reconciliation",
            Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
        },
        []string{"phase"},
    )

    ReconcilerPhaseTransitionsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_reconciler_phase_transitions_total",
            Help: "Total number of phase transitions",
        },
        []string{"from_phase", "to_phase"},
    )

    // HolmesGPT-API metrics
    HolmesGPTRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_holmesgpt_requests_total",
            Help: "Total number of HolmesGPT-API requests",
        },
        []string{"endpoint", "status_code"},
    )

    HolmesGPTLatencySeconds = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_holmesgpt_latency_seconds",
            Help:    "Latency of HolmesGPT-API calls",
            Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60},
        },
        []string{"endpoint"},
    )

    HolmesGPTRetriesTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_holmesgpt_retries_total",
            Help: "Total number of HolmesGPT-API retry attempts",
        },
        []string{"endpoint"},
    )

    // Rego policy metrics
    RegoEvaluationsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_rego_evaluations_total",
            Help: "Total number of Rego policy evaluations",
        },
        []string{"outcome", "degraded"},
    )

    RegoLatencySeconds = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_rego_latency_seconds",
            Help:    "Latency of Rego policy evaluations",
            Buckets: []float64{0.001, 0.01, 0.05, 0.1, 0.5},
        },
        []string{},
    )

    RegoReloadsTotal = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "aianalysis_rego_reloads_total",
            Help: "Total number of Rego policy reloads",
        },
    )

    // Approval metrics
    ApprovalDecisionsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_approval_decisions_total",
            Help: "Total number of approval decisions",
        },
        []string{"decision", "environment"},
    )

    // Confidence metrics
    ConfidenceScoreDistribution = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "aianalysis_confidence_score_distribution",
            Help:    "Distribution of AI confidence scores",
            Buckets: []float64{0.5, 0.6, 0.7, 0.8, 0.9, 0.95, 1.0},
        },
        []string{"signal_type"},
    )

    // DetectedLabels metrics
    DetectedLabelsFailuresTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "aianalysis_detected_labels_failures_total",
            Help: "Total number of failed label detections",
        },
        []string{"field_name"},
    )
)
```

---

## 🔗 **References**

| Document | Purpose |
|----------|---------|
| [DD-005-OBSERVABILITY-STANDARDS.md](../architecture/decisions/DD-005-OBSERVABILITY-STANDARDS.md) | **AUTHORITATIVE** - Metrics naming convention |
| [internal/controller/workflowexecution/metrics.go](../../internal/controller/workflowexecution/metrics.go) | Non-compliant example |
| [internal/controller/notification/metrics.go](../../internal/controller/notification/metrics.go) | Non-compliant example |

---

## ✅ **Resolution Tracking**

| Service | Team | Status | Acknowledged | Remediation PR | Target Date |
|---------|------|--------|--------------|----------------|-------------|
| AIAnalysis | AIAnalysis Team | ✅ Compliant (Day 5) | ✅ 2025-12-06 | - | N/A |
| WorkflowExecution | WorkflowExecution Team | ❌ Non-Compliant | ⏳ Pending | - | - |
| Notification | Notification Team | ❌ Non-Compliant | ✅ 2025-12-06 | See plan below | Day 14 |
| SignalProcessing | SignalProcessing Team | ⏳ Pending Review | ⏳ Pending | - | - |
| RemediationOrchestrator | RO Team | ⏳ Pending Review | ⏳ Pending | - | - |

### **Acknowledgment Instructions**

When your team acknowledges this notice:
1. Update your row in the table above with:
   - **Acknowledged**: ✅ YYYY-MM-DD
   - **Target Date**: When you plan to remediate
2. Add a comment to this file with your team's response

### **Team Responses**

<!--
Template:
#### [Service] Team Response (YYYY-MM-DD)
**Acknowledged By**: [Name/Role]
**Assessment**: [Compliant/Non-Compliant/Needs Review]
**Remediation Plan**: [Brief description or N/A]
**Target Date**: [Date or N/A]
-->

#### AIAnalysis Team Response (2025-12-06)
**Acknowledged By**: AI Assistant
**Assessment**: Implementing DD-005 compliant metrics in Day 5
**Remediation Plan**: All new metrics follow DD-005 format
**Target Date**: Day 5 completion

---

#### Notification Team Response (2025-12-06)
**Acknowledged By**: Notification Team
**Assessment**: ❌ Non-Compliant (2 metrics files need updates)
**Target Date**: Day 14 (Enhancement Day)

**Files to Update**:
1. `internal/controller/notification/metrics.go` (8 metrics)
2. `pkg/notification/metrics/metrics.go` (10 metrics)

**Detailed Remediation Plan**:

| Current Metric | DD-005 Compliant | File |
|----------------|------------------|------|
| `notification_failure_rate` | `notification_delivery_failure_rate` | controller |
| `notification_stuck_duration_seconds` | `notification_delivery_stuck_duration_seconds` | controller |
| `notification_deliveries_total` | `notification_delivery_requests_total` | controller |
| `notification_delivery_duration_seconds` | ✅ Already compliant | controller |
| `notification_phase` | `notification_reconciler_phase` | controller |
| `notification_retry_count` | `notification_delivery_retries_total` | controller |
| `notification_slack_retry_count` | `notification_slack_retries_total` | controller |
| `notification_slack_backoff_duration_seconds` | ✅ Already compliant | controller |
| `notification_requests_total` | `notification_reconciler_requests_total` | pkg |
| `notification_delivery_attempts_total` | ✅ Already compliant | pkg |
| `notification_delivery_duration_seconds` | ✅ Already compliant | pkg |
| `notification_retry_count_total` | `notification_delivery_retries_total` | pkg |
| `notification_circuit_breaker_state` | `notification_channel_circuit_breaker_state` | pkg |
| `notification_reconciliation_duration_seconds` | `notification_reconciler_duration_seconds` | pkg |
| `notification_reconciliation_errors_total` | `notification_reconciler_errors_total` | pkg |
| `notification_active_total` | `notification_reconciler_active_total` | pkg |
| `notification_sanitization_redactions_total` | ✅ Already compliant | pkg |
| `notification_channel_health_score` | ✅ Already compliant | pkg |

**Summary**:
- ✅ 7 metrics already DD-005 compliant
- ❌ 11 metrics need renaming
- **Estimated Effort**: 2 hours

**Implementation Steps (Day 14)**:
1. Update metric names in both files
2. Update helper function names if needed
3. Run all tests to verify no breakage
4. Update Prometheus alert rules (if any)
5. Update Grafana dashboards (if any)

---

**Created By**: AIAnalysis Team during Day 5 triage
**Last Updated**: 2025-12-06

