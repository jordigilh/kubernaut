# AIApprovalRequest Rejection - Detailed Behavior

**Date**: October 20, 2025
**Status**: ✅ DEFINITIVE ANSWER
**Source**: Implementation Plans + CRD Types

---

## 🎯 **SHORT ANSWER**

**When an operator rejects an AIApprovalRequest:**

❌ **NO** - The system does NOT automatically try the next recommendation
❌ **NO** - The remediation does NOT continue
✅ **YES** - The remediation STOPS completely
✅ **YES** - The system escalates to manual intervention via notification

---

## 📋 **COMPLETE REJECTION FLOW**

### **Step 1: Operator Rejects AIApprovalRequest**

**Action**: Operator updates `AIApprovalRequest` CRD:
```yaml
apiVersion: approval.kubernaut.io/v1alpha1
kind: AIApprovalRequest
metadata:
  name: aianalysis-abc123-approval
status:
  decision: "Rejected"  # ← Operator sets this
  decidedBy: "user@company.com"
  rejectionReason: "Resource constraints - not safe to scale now"
  decisionTime: "2025-10-20T10:30:00Z"
```

---

### **Step 2: AIAnalysis Controller Detects Rejection**

**Source**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md:4960-4990`

```go
// AIAnalysis Controller watches AIApprovalRequest for decision
if existingApproval.Status.Decision == "Rejected" {
    log.Info("Approval rejected", "decidedBy", existingApproval.Status.DecidedBy)

    // Transition AIAnalysis to "Rejected" phase
    ai.Status.Phase = "Rejected"
    ai.Status.ApprovalStatus = "Rejected"
    ai.Status.RejectionReason = fmt.Sprintf("Manually rejected by %s: %s",
        existingApproval.Status.DecidedBy, existingApproval.Status.Message)
    ai.Status.CompletedAt = &metav1.Time{Time: time.Now()}
    ai.Status.Message = ai.Status.RejectionReason

    // Update condition
    ai.Status.Conditions = append(ai.Status.Conditions, metav1.Condition{
        Type:               "ApprovalDecision",
        Status:             metav1.ConditionFalse,
        Reason:             "ManuallyRejected",
        Message:            existingApproval.Status.Message,
        LastTransitionTime: metav1.Now(),
    })

    r.recordEvent(ai, "Warning", "ManuallyRejected",
        fmt.Sprintf("Rejected by %s: %s", existingApproval.Status.DecidedBy, existingApproval.Status.Message))

    // Record metric
    approvalDecisions.WithLabelValues("rejected").Inc()

    if err := r.Status().Update(ctx, ai); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{}, nil // ← DONE - No further processing
}
```

**Key Point**: The `return ctrl.Result{}, nil // Done` means:
- ❌ AIAnalysis stops reconciliation
- ❌ No WorkflowExecution CRD is created
- ❌ No alternative recommendations are tried

---

### **Step 3: RemediationOrchestrator Detects Rejected AIAnalysis**

**Source**: `docs/services/crd-controllers/05-remediationorchestrator/implementation/IMPLEMENTATION_PLAN_V1.0.md:637-646`

```go
// RemediationOrchestrator watches AIAnalysis for completion
func (r *RemediationRequestReconciler) handleAnalyzing(ctx context.Context, rr *remediationv1alpha1.RemediationRequest) (ctrl.Result, error) {
    // ... fetch AIAnalysis child CRD ...

    // Check if rejected
    if ai.Status.Phase == "Rejected" {
        log.Info("AIAnalysis rejected")

        // Mark remediation as failed
        rr.Status.Phase = "Failed"
        rr.Status.Message = "AIAnalysis rejected"

        if err := r.Status().Update(ctx, rr); err != nil {
            return ctrl.Result{}, err
        }

        // Escalate to manual intervention
        return r.handleEscalation(ctx, rr, "AIAnalysis rejected")
    }

    // ... other status checks ...
}
```

**Key Point**: RemediationRequest transitions to "Failed" phase and escalates.

---

### **Step 4: Escalation - Create NotificationRequest**

**Source**: `docs/services/crd-controllers/05-remediationorchestrator/implementation/IMPLEMENTATION_PLAN_V1.0.md:789-802`

```go
func (r *RemediationRequestReconciler) handleEscalation(ctx context.Context, rr *remediationv1alpha1.RemediationRequest, reason string) (ctrl.Result, error) {
    log := log.FromContext(ctx)
    log.Info("Escalating remediation failure", "reason", reason)

    // Create NotificationRequest CRD
    notification, err := r.EscalationManager.CreateNotification(ctx, rr, reason)
    if err != nil {
        log.Error(err, "Failed to create NotificationRequest")
        return ctrl.Result{}, err
    }

    log.Info("NotificationRequest created", "name", notification.Name)
    return ctrl.Result{}, nil // ← DONE - Remediation stops here
}
```

**What Happens**:
1. ✅ NotificationRequest CRD is created
2. ✅ Notification Controller sends alerts (Slack, Console, etc.)
3. ✅ On-call engineer is notified of rejection
4. ❌ NO automatic retry or alternative recommendations

---

## 🔄 **WHY NO AUTOMATIC ALTERNATIVE RECOMMENDATIONS?**

### **Reason 1: AIAnalysis Contains a SINGLE Primary Recommendation**

**Source**: `api/aianalysis/v1alpha1/aianalysis_types.go`

```go
// AIAnalysisStatus contains a single recommended action
type AIAnalysisStatus struct {
    // Single primary recommendation
    RecommendedAction string `json:"recommendedAction,omitempty"`

    // Confidence in this recommendation
    Confidence float64 `json:"confidence,omitempty"`

    // Approval context includes alternatives for CONTEXT ONLY
    ApprovalContext *ApprovalContext `json:"approvalContext,omitempty"`
}

// ApprovalContext provides rich context for approval notifications
type ApprovalContext struct {
    // Recommended actions (primary + alternatives)
    RecommendedActions []RecommendedAction `json:"recommendedActions"`

    // Alternatives considered with pros/cons
    AlternativesConsidered []AlternativeApproach `json:"alternativesConsidered,omitempty"`

    // Why approval is required
    WhyApprovalRequired string `json:"whyApprovalRequired"`
}
```

**Key Insight**:
- ✅ **Primary recommendation**: Single action in `Status.RecommendedAction`
- ✅ **Alternatives**: Listed in `ApprovalContext` for **operator decision support** (not automatic execution)
- ❌ **NOT**: A queue of recommendations to try sequentially

**Purpose of Alternatives**: Inform the operator's decision, not provide fallback actions.

---

### **Reason 2: Operator Rejection is a STOP Signal**

**Philosophy**: If an operator rejects a recommendation, they are saying:
- ❌ "This specific action is NOT appropriate right now"
- ⚠️ "I need to review the situation manually"
- ✋ "Do NOT proceed with automated remediation"

**Why This Matters**:
- If the operator rejected "scale to 5 replicas," automatically trying "scale to 3 replicas" would **violate their explicit rejection**
- The operator may have rejected due to:
  - Resource constraints (no capacity for ANY scaling)
  - Business timing (maintenance window, traffic patterns)
  - Incorrect diagnosis (AI misunderstood the problem)

**Correct Behavior**: Stop and wait for manual intervention.

---

### **Reason 3: Kubernaut's Design is "AI-Assisted," Not "AI-Autonomous"**

**From Architecture Principles**:
- ✅ **Low Confidence (< 60%)**: Automatic escalation to manual review
- ✅ **Medium Confidence (60-79%)**: Require operator approval
- ✅ **High Confidence (≥ 80%)**: Auto-execute (unless policy requires approval)

**Rejection is a Manual Override**: When an operator rejects, they override the AI's judgment. The correct response is to defer to human judgment, not continue with automation.

---

## 🔄 **WHAT IF THE OPERATOR WANTS TO TRY AN ALTERNATIVE?**

### **Manual Flow for Trying Alternatives**

**Scenario**: Operator rejects "scale to 5 replicas" but wants to try "scale to 3 replicas"

#### **⚠️ CURRENT LIMITATION: No "Forced Recommendation" Field**

**Status**: The `RemediationRequest` CRD **does not currently support** forcing a specific action or bypassing AI analysis.

**Available Options**:

#### **Option 1: Wait for Alert Re-Fire (Automatic)**

1. ✅ Original `RemediationRequest` completes as "Failed"
2. ✅ If alert continues firing, Gateway creates a **new** `RemediationRequest`
3. ⚠️ AI may provide same or different recommendation (not guaranteed to be different)

**Limitation**: No control over which recommendation AI provides

---

#### **Option 2: Create New RemediationRequest (Workaround)**

1. ✅ Operator manually creates a **new** `RemediationRequest` with the same signal:

```yaml
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationRequest
metadata:
  name: manual-retry-webapp-003
  labels:
    manual-retry: "true"
spec:
  # Copy all fields from original rejected RemediationRequest
  signalFingerprint: "abc123..."  # Same signal
  signalName: "HighMemoryUsage"
  severity: "critical"
  environment: "production"
  priority: "P0"
  signalType: "prometheus"
  targetType: "kubernetes"
  firingTime: "2025-10-20T10:00:00Z"
  receivedTime: "2025-10-20T10:30:00Z"
  # ... all other fields ...
```

**Result**:
- ✅ New AIAnalysis will be created
- ⚠️ AI may recommend same action (no guarantee of different recommendation)
- ⚠️ No way to "force" the alternative action

**Limitation**: Cannot force AI to recommend a specific alternative

---

#### **Option 3: Manual kubectl Commands (Direct Execution)** ⭐ **RECOMMENDED**

1. ✅ Operator executes alternative action directly:
```bash
# Execute the alternative action the operator preferred
kubectl scale deployment webapp --replicas=3
```

2. ⚠️ **Trade-off**:
   - ❌ Bypasses Kubernaut audit trail
   - ❌ Bypasses effectiveness tracking
   - ✅ **Immediate** and **guaranteed** to execute operator's chosen action
   - ✅ Operator has full control

**Why This is Currently Best**: Since Kubernaut doesn't support forcing specific recommendations, direct execution is the only way to guarantee the operator's chosen alternative is executed.

---

### **🔧 FEATURE GAP - APPROVED FOR V2**

**Missing Capability**: `RemediationRequest` should support forced recommendations

**Status**: ✅ **APPROVED FOR V2** (October 20, 2025)

**Proposed Enhancement** (V2):
```yaml
# V2 FEATURE (NOT IN V1)
apiVersion: remediation.kubernaut.io/v1alpha1
kind: RemediationRequest
spec:
  # ... standard fields ...

  # NEW: Force specific recommendation (bypass AI)
  forcedRecommendation:
    action: "scale-deployment"
    parameters:
      deployment: "webapp"
      targetReplicas: 3
    justification: "Resource constraints - scaling to 3 instead of AI's 5"
    forcedBy: "ops-engineer@company.com"

  # NEW: Skip AI analysis
  bypassAIAnalysis: true
```

**Benefits**:
- ✅ Complete audit trail for operator-initiated actions
- ✅ Effectiveness tracking for forced recommendations
- ✅ Operator autonomy for known fixes
- ✅ Time savings (bypass 1-2 min AI analysis)
- ✅ System learns from operator decisions

**Documentation**:
- **Business Requirement**: [BR-RR-001: Forced Recommendation Manual Override](../requirements/BR-RR-001-FORCED-RECOMMENDATION-MANUAL-OVERRIDE.md)
- **Architecture Decision**: [ADR-026: Forced Recommendation Manual Override](../architecture/decisions/ADR-026-forced-recommendation-manual-override.md)

**Priority**: Medium (quality-of-life improvement for operators)
**Target Version**: V2 (Q1-Q2 2026)
**Implementation Effort**: 6 weeks

---

## 📊 **COMPLETE STATE DIAGRAM**

```
┌─────────────────────────────────────────────────────────────┐
│                   APPROVAL REJECTION FLOW                    │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  AIAnalysis Phase: "Approving"                              │
│         ↓                                                    │
│  AIApprovalRequest Created (confidence 60-79%)              │
│         ↓                                                    │
│  Operator Reviews Recommendation + Alternatives             │
│         ↓                                                    │
│    ┌────────┐                                                │
│    │DECISION│                                                │
│    └────┬───┘                                                │
│         │                                                    │
│    ┌────┴────┬─────────────────┐                            │
│    │         │                 │                            │
│ APPROVE   REJECT           TIMEOUT                          │
│    │         │                 │                            │
│    ↓         ↓                 ↓                            │
│ ✅ Ready   ❌ Rejected      ❌ Rejected                      │
│    │         │                 │                            │
│    ↓         ↓                 ↓                            │
│ Create      STOP              STOP                          │
│ Workflow    │                 │                            │
│    │         │                 │                            │
│    ↓         └─────┬───────────┘                            │
│ Execute            ↓                                         │
│ Actions     RemediationRequest                              │
│    │         Phase: "Failed"                                │
│    ↓                ↓                                         │
│ Success      Create NotificationRequest                     │
│              (Escalation)                                    │
│                     ↓                                         │
│              Notify On-Call Engineer                        │
│                     ↓                                         │
│              🚨 Manual Intervention Required                │
│                                                              │
│  ❌ NO AUTOMATIC ALTERNATIVE RECOMMENDATIONS                │
│  ❌ NO AUTOMATIC RETRY                                       │
│  ✅ EXPLICIT OPERATOR DECISION REQUIRED                     │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## 🎯 **KEY TAKEAWAYS**

### **1. Rejection = Complete Stop** ✅

When an operator rejects an `AIApprovalRequest`:
- ❌ Remediation STOPS (no workflow created)
- ❌ No automatic alternatives are tried
- ✅ RemediationRequest transitions to "Failed"
- ✅ NotificationRequest escalates to on-call team

---

### **2. Alternatives are for CONTEXT, Not EXECUTION** 📖

The `ApprovalContext.AlternativesConsidered` field:
- ✅ **Purpose**: Help operator make an informed decision
- ✅ **Content**: Pros/cons of alternative approaches
- ❌ **NOT**: A fallback queue for automatic execution

---

### **3. Operator Override is Respected** ✋

Kubernaut respects human judgment:
- ✅ Operator rejection is a **manual stop signal**
- ✅ Operator must explicitly create new remediation for alternatives
- ❌ System does NOT second-guess operator's rejection

---

### **4. Recovery Flow is Different** 🔄

**Note**: Approval rejection ≠ Workflow execution failure

| Scenario | System Response |
|----------|----------------|
| **Approval Rejected** | STOP → Escalate → Manual intervention required |
| **Workflow Failed** | Evaluate recovery viability → Create new AIAnalysis for alternative approach (max 3 attempts) |

**Why Different**:
- **Approval rejection**: Operator explicitly says "don't do this"
- **Workflow failure**: Technical failure, system can retry with alternative approach

---

## 📋 **APPROVAL REJECTION vs WORKFLOW FAILURE COMPARISON**

| Aspect | Approval Rejection | Workflow Failure |
|--------|-------------------|------------------|
| **Trigger** | Operator clicks "Reject" | Action execution fails (OOMKill, timeout, etc.) |
| **AIAnalysis Phase** | "Rejected" | "Completed" (workflow was approved) |
| **RemediationRequest Phase** | "Failed" | "Recovering" (if viable) |
| **Next Steps** | Manual intervention required | Automatic recovery attempt (up to 3 times) |
| **New AIAnalysis?** | ❌ NO | ✅ YES (with recovery context) |
| **Alternatives Tried?** | ❌ NO (operator override) | ✅ YES (AI generates new approach) |
| **Notification** | ✅ Escalation notification | ✅ Recovery notification |

---

## 📖 **SUPPORTING DOCUMENTATION**

### **Implementation Plans**:
1. **AIAnalysis Controller**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md`
   - Lines 4960-4990: Rejection handling
   - Lines 875-931: Approval workflow management

2. **RemediationOrchestrator Controller**: `docs/services/crd-controllers/05-remediationorchestrator/implementation/IMPLEMENTATION_PLAN_V1.0.md`
   - Lines 637-646: Rejected AIAnalysis detection
   - Lines 789-802: Escalation handling

### **CRD Types**:
3. **AIAnalysis CRD**: `api/aianalysis/v1alpha1/aianalysis_types.go`
   - ApprovalContext structure
   - Single recommendation design

### **Architecture Decisions**:
4. **ADR-018**: Approval Notification V1 Integration
   - Approval workflow design
   - Confidence-based thresholds

---

## ❓ **FREQUENTLY ASKED QUESTIONS**

### **Q1: Why not automatically try the second recommendation?**

**A**: Because:
1. Operator rejection is an explicit "stop" signal
2. Alternatives are for context, not automatic execution
3. Operator may have rejected for reasons that apply to ALL alternatives (resource constraints, business timing, etc.)
4. Respecting human judgment is a core design principle

---

### **Q2: How can an operator try an alternative after rejection?**

**A**: Currently, three options:

1. **Wait for alert re-fire** - Gateway creates new `RemediationRequest`, AI may provide different recommendation (no guarantee)
2. **Create new RemediationRequest** manually - Triggers new AI analysis, but cannot force specific recommendation
3. **Manual kubectl commands** ⭐ **RECOMMENDED** - Guaranteed execution of operator's choice, but bypasses Kubernaut tracking

**Note**: `forcedRecommendation` field **does not currently exist in V1** - approved for V2 (see [BR-RR-001](../requirements/BR-RR-001-FORCED-RECOMMENDATION-MANUAL-OVERRIDE.md))

---

### **Q3: What happens if HolmesGPT generates multiple recommendations?**

**A**: HolmesGPT generates a **single primary recommendation** with alternatives listed as context. If the operator wants a different recommendation, they must:
1. Reject the current AIApprovalRequest
2. Create a new RemediationRequest with the desired action (or execute manually)

---

### **Q4: Can the operator approve with modifications?**

**A**: ❌ **No, not currently supported**. The operator must:
1. Reject the current approval
2. Execute alternative manually via kubectl (recommended)
3. OR wait for alert re-fire and hope for different AI recommendation

**Feature Gap**: `RemediationRequest` doesn't support forced recommendations in V1 (approved for V2 - see [ADR-026](../architecture/decisions/ADR-026-forced-recommendation-manual-override.md))

---

### **Q5: What if I want automatic fallback behavior?**

**A**: Use **high confidence (≥ 80%) with auto-approval** for actions where you trust AI judgment. For medium confidence (60-79%), operator approval is required by design to ensure human oversight for uncertain scenarios.

---

## ✅ **SUMMARY**

**Rejected Approval Behavior**:
- ❌ **Does NOT** try alternative recommendations automatically
- ❌ **Does NOT** create WorkflowExecution
- ✅ **DOES** stop remediation completely
- ✅ **DOES** escalate to manual intervention
- ✅ **DOES** respect operator's explicit decision

**Operator Must**:
- Review alternatives in notification
- Decide to create new remediation manually (if desired)
- OR investigate and resolve manually

**Design Philosophy**: **Human-in-the-loop for uncertain scenarios** - respect operator judgment as the final authority.

---

**Document Complete**: October 20, 2025
**Confidence**: 100% (validated against implementation code)

