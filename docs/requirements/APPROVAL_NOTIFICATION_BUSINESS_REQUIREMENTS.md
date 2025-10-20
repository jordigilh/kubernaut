# Approval Notification Business Requirements

**Date**: October 17, 2025
**Status**: ✅ **APPROVED**
**Related**: ADR-018 (Approval Notification V1.0 Integration)
**Priority**: P0 (Critical for V1.0)

---

## 📋 **Overview**

This document defines business requirements for approval notification integration in Kubernaut V1.0. These requirements address a critical usability gap where operators are not notified when approval is required, leading to timeout failures and poor user experience.

**Business Context**:
- **Problem**: 40-60% of approval requests timeout due to lack of notifications
- **Impact**: $392K lost per timeout (large enterprise with $7K/min downtime cost)
- **Solution**: Push notifications (Slack/Email) when approval required
- **Value**: 91% MTTR reduction (60 min → 5 min avg)

---

## 🎯 **Business Requirements**

### **BR-AI-059: AIAnalysis Approval Context Capture**

**Category**: AI Analysis
**Priority**: P0 (Critical - V1.0 Blocker)
**Service**: AIAnalysis Controller

**Description**:
The AIAnalysis service MUST capture comprehensive approval context to enable rich notifications for informed operator decisions.

**Acceptance Criteria**:

1. **MUST capture investigation summary**:
   - Root cause description
   - Confidence score (0.0-1.0)
   - Confidence level ("low" | "medium" | "high")

2. **MUST capture evidence collected**:
   - List of evidence items that led to conclusion
   - Example: "Linear memory growth 50MB/hour per pod"
   - Minimum 1 evidence item, recommended 3-5

3. **MUST capture recommended actions with rationale**:
   - Action type (from 29 canonical actions)
   - Rationale explaining WHY this action is recommended
   - Example: "increase_resources: Increase memory 2Gi → 3Gi based on growth rate"

4. **MUST capture alternatives considered**:
   - Alternative approach description
   - Pros and cons of alternative
   - Example: "Wait and monitor: Pros: No disruption. Cons: OOM risk in 4 hours"
   - Minimum 1 alternative (if applicable)

5. **MUST explain why approval is required**:
   - Clear justification for human review
   - Example: "Medium confidence (72.5%) requires validation per policy"
   - Reference to policy or historical accuracy

**Technical Implementation**:
```yaml
status:
  approvalContext:
    reason: "Medium confidence (72.5%) - requires human review"
    confidenceScore: 72.5
    confidenceLevel: "medium"
    investigationSummary: "Memory leak in payment processing coroutine (50MB/hr growth)"
    evidenceCollected:
      - "Linear memory growth 50MB/hour per pod"
      - "Similar incident resolved 3 weeks ago (92% success rate)"
      - "No code deployment in last 24h"
    recommendedActions:
      - action: "collect_diagnostics"
        rationale: "Capture heap dump before changes"
      - action: "increase_resources"
        rationale: "Increase memory 2Gi → 3Gi based on growth rate"
    alternativesConsidered:
      - approach: "Wait and monitor"
        prosCons: "Pros: No disruption. Cons: OOM risk in 4 hours"
    whyApprovalRequired: "Medium confidence (72.5%) requires validation per policy"
```

**Validation**:
- Unit tests verify all fields populated when `phase = "approving"`
- Integration tests verify context used in notifications
- E2E tests verify operators can make informed decisions from context

**Related BRs**: BR-AI-016 (Confidence-based approval), BR-ORCH-001 (Notification creation)

---

### **BR-AI-060: AIAnalysis Approval Decision Tracking**

**Category**: AI Analysis
**Priority**: P0 (Critical - V1.0 Blocker)
**Service**: AIAnalysis Controller

**Description**:
The AIAnalysis service MUST track comprehensive approval decision metadata for audit trail and compliance.

**Acceptance Criteria**:

1. **MUST track approval status**:
   - Status: "Approved" | "Rejected" | "Pending"
   - Updated when AIApprovalRequest decision changes

2. **MUST track decision maker**:
   - `approvedBy`: Email/username of approver
   - `rejectedBy`: Email/username of rejecter

3. **MUST track decision timing**:
   - `approvalTime`: Timestamp of approval
   - `approvalDuration`: Time from request to decision (e.g., "2m15s")

4. **MUST track decision method**:
   - Method: "kubectl" | "dashboard" | "slack-button" | "email-link"
   - Enables UX improvement tracking

5. **SHOULD track decision justification**:
   - `approvalJustification`: Optional operator comment
   - Example: "Approved based on similar past incident success (92% rate)"

6. **MUST track rejection reason**:
   - `rejectionReason`: Required when rejected
   - Example: "Insufficient evidence for memory leak hypothesis"

**Technical Implementation**:
```yaml
status:
  approvalStatus: "Approved"
  approvedBy: "operator@company.com"
  approvalTime: "2025-10-17T10:35:00Z"
  approvalMethod: "kubectl"
  approvalJustification: "Approved based on similar past incident success (92% rate)"
  approvalDuration: "2m15s"
```

**Validation**:
- Unit tests verify status synchronization from AIApprovalRequest
- Integration tests verify bi-directional watch pattern (<100ms latency)
- Audit trail tests verify complete decision history

**Related BRs**: BR-AI-035 (AIApprovalRequest creation), BR-AI-042 (Manual review)

---

### **BR-ORCH-001: RemediationOrchestrator Notification Creation**

**Category**: Orchestration
**Priority**: P0 (Critical - V1.0 Blocker)
**Service**: RemediationOrchestrator Controller

**Description**:
The RemediationOrchestrator MUST create NotificationRequest CRDs when AIAnalysis requires approval, enabling push notifications to operators.

**Acceptance Criteria**:

1. **MUST watch AIAnalysis status changes**:
   - Watch for `phase = "approving"`
   - React within 1 second of status change

2. **MUST create NotificationRequest CRD**:
   - Type: "approval-required"
   - Priority: "high"
   - Channels: ["console", "slack"] (V1.0), ["email"] (V1.1+)
   - Owner reference: RemediationRequest (for cascade deletion)

3. **MUST include rich approval context**:
   - Subject: "🔔 Approval Required: {alertName}"
   - Body: Formatted from AIAnalysis.status.approvalContext
   - Include: Root cause, evidence, recommended actions, alternatives
   - Include: kubectl command for approval

4. **MUST ensure idempotency**:
   - Track `approvalNotificationSent` in RemediationRequest status
   - Create notification only once per approval request
   - Skip if notification already sent

5. **MUST handle notification failures gracefully**:
   - Retry with exponential backoff (30s, 1min, 2min)
   - Log error but don't fail remediation workflow
   - Maximum 3 retry attempts

6. **MUST respect notification configuration**:
   - Read global notification config (V1.0)
   - Support Slack webhook URL from environment variable
   - Support email addresses from configuration file

**Technical Implementation**:
```go
func (r *RemediationOrchestratorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Fetch AIAnalysis
    aiAnalysis := &aianalysisv1.AIAnalysis{}
    // ... fetch logic ...

    // Create notification when approval needed
    if aiAnalysis.Status.Phase == "approving" && !remediation.Status.ApprovalNotificationSent {
        notification := &notificationv1.NotificationRequest{
            Spec: notificationv1.NotificationRequestSpec{
                Subject: fmt.Sprintf("🔔 Approval Required: %s", aiAnalysis.Spec.AlertName),
                Body: r.formatApprovalBody(aiAnalysis),
                Type: "approval-required",
                Priority: "high",
                Channels: []string{"console", "slack"},
            },
        }

        if err := r.Create(ctx, notification); err != nil {
            return ctrl.Result{RequeueAfter: 30 * time.Second}, err
        }

        remediation.Status.ApprovalNotificationSent = true
        return ctrl.Result{}, r.Status().Update(ctx, remediation)
    }

    return ctrl.Result{}, nil
}
```

**Validation**:
- Unit tests verify notification creation logic
- Integration tests verify AIAnalysis → Notification flow
- E2E tests verify Slack notification delivery (mock webhook)
- Performance tests verify <1s notification latency

**Related BRs**: BR-NOT-050 (Zero data loss), ADR-017 (Notification CRD creator)

---

### **BR-NOT-059: Policy-Based Notification Routing (V2.0)**

**Category**: Notification
**Priority**: P1 (High Value - V2.0)
**Service**: Notification Controller
**Confidence**: 93% (approved for V2 planning)

**Description**:
The Notification service MUST support policy-based destination routing with fallback to namespace annotations and global configuration.

**Acceptance Criteria** (V2.0):

1. **MUST evaluate Rego policy first**:
   - Input: namespace, alertName, confidence, severity
   - Output: List of notification destinations
   - Example: Production + critical → PagerDuty + Slack

2. **MUST fallback to namespace annotations**:
   - If policy returns no destinations
   - Read annotations: `kubernaut.ai/approval-slack-channel`, `kubernaut.ai/approval-email`

3. **MUST fallback to global configuration**:
   - If namespace has no annotations
   - Use global default channels

4. **MUST support multiple destinations per approval**:
   - Example: Slack channel + PagerDuty + Email
   - Parallel delivery to all channels

5. **MUST track routing decision**:
   - Record which routing method used (policy | namespace | global)
   - Record evaluated policy (if policy-based)

**Technical Implementation** (V2.0):
```go
// pkg/notification/routing/router.go

func (r *Router) ResolveDestinations(ctx context.Context, ai *aianalysisv1.AIAnalysis) ([]NotificationDestination, error) {
    // Priority 1: Rego Policy
    if destinations, err := r.evaluatePolicy(ctx, ai); err == nil && len(destinations) > 0 {
        return destinations, nil
    }

    // Priority 2: Namespace Annotations
    if destinations, err := r.resolveFromNamespace(ctx, ai.Namespace); err == nil && len(destinations) > 0 {
        return destinations, nil
    }

    // Priority 3: Global Configuration
    return r.globalConfig.DefaultDestinations, nil
}
```

**Rego Policy Example**:
```rego
package notification_routing

approval_channels[channel] {
    input.namespace == "production"
    input.confidence < 80
    input.severity == "critical"
    channel := {
        "type": "pagerduty",
        "target": "production-oncall",
        "priority": "high"
    }
}
```

**Validation**:
- Unit tests verify routing priority (policy → namespace → global)
- Integration tests verify Rego policy evaluation
- E2E tests verify multi-channel delivery

**Related BRs**: BR-NOT-001 to BR-NOT-058 (Notification foundation)

---

## 📊 **Business Requirements Summary**

| BR | Description | Service | Priority | Version |
|---|---|---|---|---|
| **BR-AI-059** | Capture comprehensive approval context | AIAnalysis | P0 | V1.0 |
| **BR-AI-060** | Track approval decision metadata | AIAnalysis | P0 | V1.0 |
| **BR-ORCH-001** | Create NotificationRequest when approval needed | RemediationOrchestrator | P0 | V1.0 |
| **BR-NOT-059** | Policy-based notification routing | Notification | P1 | V2.0 |

---

## 🎯 **Success Metrics**

### **Technical Metrics**

| Metric | Target | Measurement |
|---|---|---|
| **Notification Latency** | <1s (Slack) | Prometheus: notification_delivery_duration |
| **Notification Delivery Rate** | >99% | NotificationRequest status.phase=delivered |
| **Approval Miss Rate** | <5% (down from 40-60%) | AIApprovalRequest timeout rate |
| **Context Completeness** | 100% | All approvalContext fields populated |

### **Business Metrics**

| Metric | Current (V1.0 without notifications) | Target (V1.0 with notifications) | Impact |
|---|---|---|---|
| **Approval Timeout Rate** | 30-40% | <5% | **-35%** |
| **MTTR (approval scenarios)** | 60+ min | 4-5 min | **91% reduction** |
| **Operator Experience** | 4/10 (polling required) | 8/10 (push notifications) | **+4 points** |
| **Business Value per Incident** | $420K loss | $28K | **$392K saved** |

---

## 📚 **Implementation References**

### **AIAnalysis CRD Updates**

**File**: `api/aianalysis/v1alpha1/aianalysis_types.go`

**New Types**:
```go
type ApprovalContext struct {
    Reason                  string                    `json:"reason"`
    ConfidenceScore         float64                   `json:"confidenceScore"`
    ConfidenceLevel         string                    `json:"confidenceLevel"`
    InvestigationSummary    string                    `json:"investigationSummary"`
    EvidenceCollected       []string                  `json:"evidenceCollected,omitempty"`
    RecommendedActions      []RecommendedAction       `json:"recommendedActions"`
    AlternativesConsidered  []AlternativeApproach     `json:"alternativesConsidered,omitempty"`
    WhyApprovalRequired     string                    `json:"whyApprovalRequired"`
}

type RecommendedAction struct {
    Action    string `json:"action"`
    Rationale string `json:"rationale"`
}

type AlternativeApproach struct {
    Approach string `json:"approach"`
    ProsCons string `json:"prosCons"`
}
```

---

### **RemediationOrchestrator Notification Logic**

**File**: `internal/controller/remediationorchestrator/remediationorchestrator_controller.go`

**New Methods**:
```go
func (r *RemediationOrchestratorReconciler) createApprovalNotification(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    aiAnalysis *aianalysisv1.AIAnalysis,
) error

func (r *RemediationOrchestratorReconciler) formatApprovalBody(
    ai *aianalysisv1.AIAnalysis,
) string
```

---

### **Notification Templates**

**File**: `pkg/notification/templates/approval_notification.go`

**V1.0 Hardcoded Template**:
```go
const ApprovalNotificationTemplate = `
🔔 Kubernaut AI Analysis: Approval Required

**Alert**: {{.AlertName}}
**Confidence**: {{.ConfidenceScore}}% ({{.ConfidenceLevel}})

**Root Cause**:
{{.RootCause}}

**Evidence Collected**:
{{range .Evidence}}
• {{.}}
{{end}}

**Recommended Actions**:
{{range $i, $action := .RecommendedActions}}
{{add $i 1}}. **{{$action.Action}}**: {{$action.Rationale}}
{{end}}

**Alternatives Considered**:
{{range .Alternatives}}
• {{.Approach}}: {{.ProsCons}}
{{end}}

**Why Approval Required**:
{{.ApprovalReason}}

**Timeout**: {{.Timeout}}
**Approve**: kubectl patch aiapprovalrequest {{.ApprovalRequestName}} --type=merge --subresource=status -p '{"status":{"decision":"Approved","decidedBy":"YOUR_EMAIL"}}'
`
```

---

## 🔗 **Related Documentation**

1. **ADR-018**: Approval Notification V1.0 Integration Decision
2. **Multi-Step Workflow Examples**: `docs/analysis/MULTI_STEP_WORKFLOW_EXAMPLES.md`
3. **AIAnalysis Implementation Plan**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md`
4. **RemediationOrchestrator Integration**: `docs/services/crd-controllers/05-remediationorchestrator/NOTIFICATION_INTEGRATION_PLAN.md`
5. **Notification Controller**: `docs/services/crd-controllers/06-notification/README.md`

---

**Document Owner**: Platform Architecture Team
**Review Frequency**: After V1.0 deployment
**Next Review Date**: 2026-01-17

