# BR-ORCH-046: Policy-Driven Operational Awareness Notification

**Service**: RemediationOrchestrator Controller
**Category**: Notification & Observability
**Priority**: P1 (HIGH)
**Version**: 1.0
**Date**: 2026-02-12
**Status**: Planned
**Related BRs**: BR-ORCH-001 (Approval Notification), BR-ORCH-036 (Manual Review), BR-ORCH-042 (Consecutive Failure Blocking), BR-ORCH-045 (Completion Notification)
**Related DDs**: DD-AIANALYSIS-001 (Rego Policy Loading Strategy)

---

## Overview

RemediationOrchestrator MUST evaluate a Rego-based notification policy after SignalProcessing completes, using normalized signal data, to determine whether operators should be proactively notified that a remediation is underway.

**Business Value**: During early adoption, SREs need real-time awareness of automated remediation activity to build trust in the platform. Beyond early adoption, policy-driven notifications detect operational patterns (remediation loops, critical production incidents) that require human attention even when individual remediations succeed.

**Gap Identified**: RO currently creates notifications only for approval gates (BR-ORCH-001), failures (BR-ORCH-036), completions (BR-ORCH-045), and blocking (BR-ORCH-042). There is no mechanism for policy-driven, context-aware notification at the point where normalized signal data first becomes available. This means a critical production remediation can proceed end-to-end without any operator notification until completion -- potentially 30+ minutes of unobserved automated action.

---

## Architecture

### Evaluation Point

The notification policy is evaluated during the `processing → analyzing` phase transition in RO's reconciliation flow, immediately after SignalProcessing completes and before AIAnalysis is created:

```
GW creates RR → RO (pending) → SP created → SP completes
                                                  ↓
                                          ┌───────────────────────┐
                                          │ BR-ORCH-046:          │
                                          │ Evaluate Rego policy  │
                                          │ with normalized data  │
                                          │ + remediation history │
                                          └───────────┬───────────┘
                                                      ↓
                                             (optional NotificationRequest)
                                                      ↓
                                          AA created → ... (continues normally)
```

### Why This Point

| Evaluation point | Normalized data? | Workflow selection? | Remediation history? | Fit |
|---|---|---|---|---|
| GW (RR creation) | No (raw signal) | No | No | Too early |
| SP completion | Yes | No | No | Data ready but wrong service |
| **RO after SP** | **Yes (from SP status)** | **No (not needed)** | **Yes (field selector)** | **Correct** |
| RO after AA | Yes | Yes | Yes | Too late (AA already ran) |

RO after SP has everything needed for operational awareness decisions. Workflow selection is not relevant -- this notification is about "a remediation is underway for this signal context," not "this specific workflow was chosen." Post-workflow notifications (completion, failure) are covered by BR-ORCH-045 and BR-ORCH-036.

### Data Available at Evaluation

| Data | Source | Available? |
|---|---|---|
| Normalized severity | SP status | Yes |
| Classified environment | SP status | Yes |
| Assigned priority | SP status | Yes |
| Signal fingerprint | RR spec (immutable) | Yes |
| Signal name / type | RR spec | Yes |
| Target resource (namespace/kind/name) | RR spec | Yes |
| Remediation history (count, outcomes) | Field selector on `spec.signalFingerprint` (BR-ORCH-042 pattern) | Yes |
| Remediation frequency (count in time window) | Derived from history + creation timestamps | Yes |

---

## Requirements

### BR-ORCH-046.1: Rego Policy Evaluation at SP Completion

**MUST**: When SignalProcessing completes and RO transitions to the `analyzing` phase, RO SHALL evaluate a configurable Rego policy to determine if an operational awareness notification should be created.

**Policy Input**:
```json
{
  "signal_name": "high-memory-payment-api-abc123",
  "signal_type": "prometheus-alert",
  "normalized_severity": "critical",
  "environment": "production",
  "priority": "P0",
  "component": "payment-api",
  "namespace": "production",
  "resource_kind": "Deployment",
  "resource_name": "payment-api",
  "fingerprint": "sha256:abc123...",
  "remediation_history": {
    "total_count": 5,
    "count_1h": 3,
    "count_24h": 5,
    "last_outcome": "Completed",
    "consecutive_successes": 2,
    "consecutive_failures": 0,
    "first_seen": "2026-02-12T08:00:00Z",
    "last_seen": "2026-02-12T10:30:00Z"
  }
}
```

**Policy Output**:
```json
{
  "notify": true,
  "reason": "critical_production_remediation",
  "priority": "medium",
  "channels": ["slack"]
}
```

**Acceptance Criteria**:

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-046-1-1 | Rego policy evaluated after SP completion, before AA creation | Unit |
| AC-046-1-2 | Policy input contains all fields from Data Available table | Unit |
| AC-046-1-3 | Remediation history computed via field selector on `spec.signalFingerprint` | Unit |
| AC-046-1-4 | Policy returning `notify: false` skips notification creation | Unit |
| AC-046-1-5 | Policy returning `notify: true` creates NotificationRequest | Unit |
| AC-046-1-6 | Policy evaluation failure does NOT block remediation pipeline | Unit |
| AC-046-1-7 | Policy evaluation latency <500ms (non-blocking) | Integration |

---

### BR-ORCH-046.2: Default Notification Rules

**MUST**: Ship a default Rego policy that covers the following operational awareness scenarios:

#### Rule 1: Critical/High Severity in Production

Any remediation in a production environment with critical or high normalized severity triggers a notification.

```rego
package notification.operational

# Critical production remediation → always notify
notify {
    input.environment == "production"
    input.normalized_severity == "critical"
}

notify {
    input.environment == "production"
    input.normalized_severity == "high"
}
```

**Rationale**: SREs must be aware when the platform takes automated action on critical production workloads, especially during early adoption.

#### Rule 2: Remediation Frequency Escalation

Same signal fingerprint remediated 2+ times within 1 hour triggers a notification, regardless of severity or environment.

```rego
# Repeated remediation → notify (possible deeper issue)
notify {
    input.remediation_history.count_1h >= 2
}
```

**Rationale**: Repeated successful remediations of the same signal indicate a deeper underlying issue that won't be resolved by automated remediation alone. Operator attention is needed even though individual remediations succeed. This complements BR-ORCH-042 (which blocks after 3 consecutive *failures*) by catching repeated *successes* that mask a persistent problem.

#### Rule 3: First-Time Remediation (Early Adoption)

The first time a signal fingerprint is ever remediated, notify for visibility.

```rego
# First remediation for this fingerprint → notify (early adoption visibility)
notify {
    input.remediation_history.total_count == 0
}
```

**Rationale**: During early adoption, operators want to see every new type of incident being handled by the platform for the first time. This rule can be removed or relaxed as confidence grows.

**Acceptance Criteria**:

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-046-2-1 | Default policy covers critical/high + production | Unit |
| AC-046-2-2 | Default policy covers frequency escalation (2+ in 1h) | Unit |
| AC-046-2-3 | Default policy covers first-time remediation | Unit |
| AC-046-2-4 | Default policy is loaded from ConfigMap | Unit |
| AC-046-2-5 | Operators can override default policy via ConfigMap | Integration |

---

### BR-ORCH-046.3: NotificationRequest Creation

**MUST**: When the Rego policy returns `notify: true`, create a NotificationRequest CRD:

- **Name**: `nr-operational-{remediationRequest}` (deterministic, idempotent)
- **Type**: `operational-awareness` (new NotificationType enum value)
- **Priority**: From policy output (default: `medium`)
- **Subject**: `Remediation Underway: {signalName} ({environment}/{severity})`
- **Body**: Includes signal context, target resource, remediation history summary, policy reason
- **Channels**: From policy output (default: `[slack, file]`)
- **Metadata**: `remediationRequest`, `signalFingerprint`, `policyReason`, `remediationCount1h`
- **Spec fields**: `spec.type=operational-awareness`, `spec.remediationRequestRef` (Issue #91: labels removed; ownerRef sufficient for component)
- **OwnerReference**: RemediationRequest (for cascade deletion per BR-ORCH-031)

**Acceptance Criteria**:

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-046-3-1 | NotificationRequest created with type=operational-awareness | Unit |
| AC-046-3-2 | Idempotent: deterministic naming prevents duplicates on reconciliation retries | Unit |
| AC-046-3-3 | OwnerReference set for cascade deletion | Unit |
| AC-046-3-4 | Notification reference tracked in RR status (BR-ORCH-035 pattern) | Unit |
| AC-046-3-5 | End-to-end latency <5 seconds from SP completion to notification | Integration |

---

### BR-ORCH-046.4: Idempotency

**MUST**: Use deterministic naming (`nr-operational-{rr.Name}`) and track `operationalNotificationSent` in RemediationRequest status. Only create the notification once per RR.

```go
if policyResult.Notify && !rr.Status.OperationalNotificationSent {
    createOperationalNotification(ctx, rr, spStatus, policyResult)
    rr.Status.OperationalNotificationSent = true
    r.Status().Update(ctx, rr)
}
```

**Acceptance Criteria**:

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-046-4-1 | `operationalNotificationSent` flag prevents duplicate notifications | Unit |
| AC-046-4-2 | Flag persists across RO restarts | Unit |
| AC-046-4-3 | Multiple reconciliation retries create at most 1 notification | Unit |

---

### BR-ORCH-046.5: Metrics

**MUST**: Expose Prometheus metrics for operational awareness notifications:

```go
var (
    OperationalNotificationsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "remediationorchestrator_operational_notifications_total",
            Help: "Total operational awareness notifications created",
        },
        []string{"namespace", "reason", "severity", "environment"},
    )

    OperationalPolicyEvaluationDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "remediationorchestrator_operational_policy_evaluation_duration_seconds",
            Help:    "Duration of Rego policy evaluation for operational notifications",
            Buckets: prometheus.DefBuckets,
        },
    )
)
```

**Acceptance Criteria**:

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-046-5-1 | Counter incremented on notification creation | Unit |
| AC-046-5-2 | Histogram tracks policy evaluation duration | Unit |
| AC-046-5-3 | Metrics include namespace, reason, severity, environment labels | Unit |

---

### BR-ORCH-046.6: Configuration

**MUST**: Operational awareness notification is configurable via RO's ConfigMap:

```yaml
operationalAwarenessNotification:
  enabled: true                           # Master switch (default: true)
  policyConfigMapName: "ro-notification-policy"  # ConfigMap containing Rego policy
  policyConfigMapKey: "operational.rego"   # Key within ConfigMap
  historyWindow: 1h                        # Time window for frequency counting
  defaultChannels: [slack, file]           # Default channels if policy doesn't specify
  defaultPriority: medium                  # Default priority if policy doesn't specify
```

**Acceptance Criteria**:

| ID | Criterion | Test Coverage |
|----|-----------|---------------|
| AC-046-6-1 | `enabled: false` disables all operational notifications | Unit |
| AC-046-6-2 | Policy loaded from ConfigMap (hot-reloadable) | Integration |
| AC-046-6-3 | Missing ConfigMap falls back to default embedded policy | Unit |
| AC-046-6-4 | History window configurable | Unit |

---

## Notification Content Template

```yaml
spec:
  type: operational-awareness
  priority: "{policyPriority}"
  subject: "Remediation Underway: {signalName} ({environment}/{severity})"
  body: |
    Kubernaut is performing automated remediation.

    **Signal**: {signalName}
    **Severity**: {normalizedSeverity}
    **Environment**: {environment}
    **Priority**: {priority}

    **Target Resource**:
    {namespace}/{resourceKind}/{resourceName}

    **Remediation History** (last 24h):
    - Total remediations: {totalCount24h}
    - Last hour: {count1h}
    - Last outcome: {lastOutcome}

    **Notification Reason**: {policyReason}

    This is an informational notification. The remediation pipeline is proceeding
    automatically. No action is required unless you wish to intervene.

    **To view status**: kubectl get remediationrequest {rrName} -n {namespace}
  channels: ["{policyChannels}"]
  metadata:
    remediationRequest: "{rrName}"
    signalFingerprint: "{fingerprint}"
    policyReason: "{reason}"
    remediationCount1h: "{count1h}"
    normalizedSeverity: "{normalizedSeverity}"
    environment: "{environment}"
```

---

## Relationship to Existing Notifications

| BR | Trigger | Phase | Purpose |
|----|---------|-------|---------|
| **BR-ORCH-046 (this)** | **SP completes + Rego policy** | **processing → analyzing** | **Operational awareness: "a remediation is underway"** |
| BR-ORCH-001 | AA → Approving | analyzing | Approval gate: "human decision needed" |
| BR-ORCH-036 | AA/WE → Failed | analyzing/executing | Escalation: "remediation failed, manual intervention required" |
| BR-ORCH-045 | WE → Completed | executing → completed | Completion: "remediation succeeded" |
| BR-ORCH-034 | Parent RR completes with duplicates | completed | Bulk summary: "N duplicate signals suppressed" |
| BR-ORCH-042.5 | 3+ consecutive failures | failed → blocked | Blocking: "signal fingerprint blocked due to repeated failures" |

**Key distinction**: BR-ORCH-046 is the only notification that fires *before* AIAnalysis, using normalized signal context and remediation history -- not workflow selection or execution outcome. It answers "should an operator know this remediation is happening?" rather than "what happened?"

---

## Test Scenarios

```gherkin
Scenario: Critical production signal triggers operational notification
  Given RemediationRequest "rr-1" with signalName "HighCPU-payment-api"
  And SignalProcessing completed with normalized_severity="critical", environment="production"
  And operational awareness notification policy is enabled
  When RemediationOrchestrator transitions from processing to analyzing
  Then NotificationRequest should be created with:
    | type     | operational-awareness |
    | priority | medium                |
    | subject  | Remediation Underway: HighCPU-payment-api (production/critical) |
  And RemediationRequest "rr-1" should have operationalNotificationSent = true
  And AIAnalysis creation should proceed normally (non-blocking)

Scenario: Repeated remediation triggers frequency escalation notification
  Given RemediationRequest "rr-5" with fingerprint "sha256:abc123"
  And 2 previous RemediationRequests exist for fingerprint "sha256:abc123" in the last hour
  And SignalProcessing completed with normalized_severity="low", environment="staging"
  When RemediationOrchestrator transitions from processing to analyzing
  Then NotificationRequest should be created with reason "remediation_frequency_escalation"
  And notification body should contain "Last hour: 2"

Scenario: Low severity staging signal does not trigger notification
  Given RemediationRequest "rr-1" with no remediation history
  And SignalProcessing completed with normalized_severity="low", environment="staging"
  And this is NOT the first remediation for this fingerprint
  When RemediationOrchestrator transitions from processing to analyzing
  Then NO NotificationRequest should be created
  And AIAnalysis creation should proceed normally

Scenario: Notification policy disabled
  Given operational awareness notification is disabled in ConfigMap
  And SignalProcessing completed with normalized_severity="critical", environment="production"
  When RemediationOrchestrator transitions from processing to analyzing
  Then NO NotificationRequest should be created

Scenario: Idempotency on retry
  Given RemediationRequest "rr-1" has operationalNotificationSent = true
  And SignalProcessing is completed
  When RemediationOrchestrator reconciles "rr-1" again
  Then NO new NotificationRequest should be created

Scenario: Policy evaluation failure does not block pipeline
  Given Rego policy contains a syntax error
  And SignalProcessing completed successfully
  When RemediationOrchestrator transitions from processing to analyzing
  Then policy evaluation error should be logged
  And AIAnalysis creation should proceed normally (non-blocking)
  And metric remediationorchestrator_operational_policy_evaluation_errors_total should increment
```

---

## API Change

### NotificationType Enum Update

```go
// +kubebuilder:validation:Enum=escalation;simple;status-update;approval;manual-review;completion;operational-awareness
type NotificationType string

const (
    // ... existing values ...
    NotificationTypeOperationalAwareness NotificationType = "operational-awareness" // BR-ORCH-046
)
```

### RemediationRequest Status Update

```go
type RemediationRequestStatus struct {
    // ... existing fields ...

    // OperationalNotificationSent indicates whether the operational awareness
    // notification has been sent for this RemediationRequest (idempotency flag).
    // Set to true after successful NotificationRequest creation in BR-ORCH-046.
    // +optional
    OperationalNotificationSent bool `json:"operationalNotificationSent,omitempty"`
}
```

---

## Implementation Notes

- Rego evaluation: Use the same OPA/Rego runtime pattern established in DD-AIANALYSIS-001 (Rego Policy Loading Strategy). RO loads the policy from ConfigMap and evaluates it using `github.com/open-policy-agent/opa/rego`.
- Remediation history query: Reuse the `spec.signalFingerprint` field index from BR-ORCH-042 (already exists in SetupWithManager). Query with `client.MatchingFields` and compute count/frequency from creation timestamps.
- Non-blocking: Notification creation and policy evaluation MUST NOT delay the `processing → analyzing` transition. Use the same async pattern as BR-ORCH-001 (create notification, update status, continue).
- File channel: Include `file` channel in default channels to enable E2E test verification (same pattern as BR-ORCH-045).

---

## Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-12 | Initial BR: Policy-driven operational awareness notification at SP completion |

---

**Document Version**: 1.0
**Last Updated**: February 12, 2026
**Maintained By**: Kubernaut Architecture Team
