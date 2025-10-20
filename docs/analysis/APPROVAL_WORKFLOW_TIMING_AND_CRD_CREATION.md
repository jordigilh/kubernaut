# Approval Workflow Timing and CRD Creation

**Date**: October 17, 2025
**Purpose**: Clarify WHEN approvals are requested and WHO creates approval/notification CRDs
**Questions**:
1. Are approvals requested BEFORE workflow execution or per-step during execution?
2. Does the workflow create Notification and AIApprovalRequest CRDs?

---

## 🎯 **TL;DR - Quick Answers**

### **Question 1: When are approvals requested?**

**Answer**: **BEFORE workflow execution** (during AIAnalysis phase)

**Workflow-level approval** happens at the **AIAnalysis phase**:
- If AI confidence is 60-79% (medium) → AIAnalysis controller creates AIApprovalRequest CRD
- Human approves/rejects via AIApprovalRequest CRD
- Only after approval does RemediationOrchestrator create WorkflowExecution CRD

**Per-step approval** is NOT currently implemented (V1):
- WorkflowExecution has `spec.executionStrategy.approvalRequired` flag
- But this is workflow-level (wait for annotation before starting execution)
- Individual steps do NOT have separate approval gates in V1

---

### **Question 2: Who creates Notification and AIApprovalRequest CRDs?**

**Answer**:

| CRD Type | Created By | When | Purpose |
|---|---|---|---|
| **AIApprovalRequest** | **AIAnalysis Controller** | During `approving` phase | Request manual approval for medium-confidence recommendations |
| **NotificationRequest** | **RemediationOrchestrator Controller** | During failure/timeout/escalation events | Notify operators of remediation state changes |

**Key Point**: **WorkflowExecution Controller does NOT create approval or notification CRDs**
- It only creates **KubernetesExecution CRDs** (one per workflow step)
- Approval happens BEFORE workflow creation
- Notifications happen at orchestration level (RemediationOrchestrator)

---

## 📋 **DETAILED EXPLANATION**

---

## **PART 1: APPROVAL TIMING - BEFORE WORKFLOW EXECUTION**

### **Approval Flow Architecture**

```
┌─────────────────────────────────────────────────────────────────────┐
│                        APPROVAL HAPPENS HERE                         │
│                    (Before Workflow Execution)                       │
│                                                                       │
│  RemediationRequest → RemediationProcessing → AIAnalysis             │
│                                                 │                     │
│                                          AI Confidence?              │
│                                                 │                     │
│                              ┌──────────────────┼──────────────────┐ │
│                              │                  │                  │ │
│                         High (≥80%)       Medium (60-79%)    Low (<60%) │
│                              │                  │                  │ │
│                          Auto-Approve      Create AIApprovalRequest Block │
│                              │                  │                  │ │
│                              │            ⏸️ WAIT FOR APPROVAL    Escalate │
│                              │                  │                  │ │
│                              │          Approved / Rejected       │ │
│                              │                  │                  │ │
│                              └──────────────────┴──────────────────┘ │
│                                                 │                     │
│                              RemediationOrchestrator                 │
│                                                 │                     │
│                              Creates WorkflowExecution CRD           │
│                                                 │                     │
└─────────────────────────────────────────────────┼─────────────────────┘
                                                  │
┌─────────────────────────────────────────────────┼─────────────────────┐
│                   NO APPROVAL HAPPENS HERE                            │
│                    (Workflow Executes Steps)                          │
│                                                                       │
│  WorkflowExecution → KubernetesExecution (Step 1) → Complete         │
│                   → KubernetesExecution (Step 2) → Complete         │
│                   → KubernetesExecution (Step 3) → Complete         │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
```

---

### **Step-by-Step Flow with Approval**

#### **Phase 1: AIAnalysis Determines Confidence (BR-AI-016 to BR-AI-030)**

**Controller**: AIAnalysis Reconciler
**Phase**: `recommending` → `approving` (if needed)

```yaml
# AIAnalysis CRD after HolmesGPT investigation
apiVersion: aianalysis.kubernaut.ai/v1alpha1
kind: AIAnalysis
metadata:
  name: aianalysis-oomkill-12345
spec:
  alertName: "OOMKilled payment-service"
  targetNamespace: "production"
status:
  phase: "recommending"
  confidenceScore: 72.5  # Medium confidence (60-79%)
  confidenceLevel: "medium"
  recommendations:
    - id: "rec-001"
      action: "collect_diagnostics"
      confidence: 0.98
    - id: "rec-002"
      action: "increase_resources"
      confidence: 0.88
    - id: "rec-003"
      action: "restart_pod"
      confidence: 0.95
```

**AIAnalysis Controller Logic**:
```go
func (r *AIAnalysisReconciler) handleRecommending(
    ctx context.Context,
    ai *aianalysisv1alpha1.AIAnalysis,
) (ctrl.Result, error) {
    // Evaluate Rego policy based on confidence
    policyDecision, err := r.PolicyEngine.Evaluate(ctx, ai)
    if err != nil {
        return ctrl.Result{}, err
    }

    // Decision matrix based on confidence
    switch {
    case ai.Status.ConfidenceScore >= 80:
        // High confidence → Auto-approve
        ai.Status.Phase = "completed"
        ai.Status.ApprovalStatus = "auto-approved"
        return ctrl.Result{}, r.Status().Update(ctx, ai)

    case ai.Status.ConfidenceScore >= 60 && ai.Status.ConfidenceScore < 80:
        // Medium confidence → Request approval
        ai.Status.Phase = "approving"
        return ctrl.Result{}, r.Status().Update(ctx, ai)

    default:
        // Low confidence → Block
        ai.Status.Phase = "rejected"
        ai.Status.Message = "Confidence too low (<60%), escalate to human"
        return ctrl.Result{}, r.Status().Update(ctx, ai)
    }
}
```

---

#### **Phase 2: AIAnalysis Creates AIApprovalRequest (BR-AI-035)**

**Controller**: AIAnalysis Reconciler
**Phase**: `approving`
**CRD Created**: `AIApprovalRequest`

**Source**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md` lines 547-603

```go
func (r *AIAnalysisReconciler) handleApproving(
    ctx context.Context,
    ai *aianalysisv1alpha1.AIAnalysis,
) (ctrl.Result, error) {
    // Check if AIApprovalRequest already exists
    approvalReq, err := r.ApprovalManager.GetApprovalRequest(ctx, ai)
    if err != nil && client.IgnoreNotFound(err) != nil {
        return ctrl.Result{}, err
    }

    if approvalReq == nil {
        // ✅ CREATE AIApprovalRequest CRD (APPROVAL HAPPENS HERE)
        approvalReq, err = r.ApprovalManager.CreateApprovalRequest(ctx, ai)
        if err != nil {
            log.Error(err, "Failed to create AIApprovalRequest")
            return ctrl.Result{}, err
        }

        log.Info("AIApprovalRequest created", "name", approvalReq.Name)
        // ⏸️ WAIT - Requeue to monitor approval status
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    // Check approval decision
    switch approvalReq.Status.Decision {
    case "Approved":
        ai.Status.Phase = "completed"  // ✅ Proceed to workflow creation
        ai.Status.ApprovalStatus = "Approved"
    case "Rejected":
        ai.Status.Phase = "rejected"   // ❌ Block workflow creation
        ai.Status.ApprovalStatus = "Rejected"
    default:
        // ⏸️ Still pending - keep waiting
        return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
    }

    return ctrl.Result{}, r.Status().Update(ctx, ai)
}
```

**AIApprovalRequest CRD Created**:
```yaml
apiVersion: aianalysis.kubernaut.ai/v1alpha1
kind: AIApprovalRequest
metadata:
  name: aianalysis-oomkill-12345-approval
  namespace: production
  ownerReferences:
    - apiVersion: aianalysis.kubernaut.ai/v1alpha1
      kind: AIAnalysis
      name: aianalysis-oomkill-12345
      controller: true
spec:
  aiAnalysisRef:
    name: aianalysis-oomkill-12345
    namespace: production
  investigation:
    rootCause: "Memory leak in payment processing coroutine (50MB/hr growth)"
    confidenceScore: 72.5
    confidenceLevel: "medium"
    recommendations:
      - id: "rec-001"
        action: "collect_diagnostics"
      - id: "rec-002"
        action: "increase_resources"
      - id: "rec-003"
        action: "restart_pod"
  requestedAt: "2025-10-17T10:30:00Z"
  timeout: 15m
status:
  decision: ""  # ⏸️ WAITING for human operator to set "Approved" or "Rejected"
  decidedBy: ""
  decidedAt: null
```

**Key Point**: **AIAnalysis Controller CREATES AIApprovalRequest CRD**

---

#### **Phase 3: Human Operator Approves/Rejects**

**Action**: Human operator updates AIApprovalRequest CRD

```bash
# Operator reviews the approval request
kubectl get aiapprovalrequest aianalysis-oomkill-12345-approval -o yaml

# Operator approves by updating status
kubectl patch aiapprovalrequest aianalysis-oomkill-12345-approval \
  --type=merge \
  --subresource=status \
  -p '{"status":{"decision":"Approved","decidedBy":"operator@company.com"}}'
```

**Updated AIApprovalRequest**:
```yaml
status:
  decision: "Approved"  # ✅ Human operator approved
  decidedBy: "operator@company.com"
  decidedAt: "2025-10-17T10:35:00Z"
```

---

#### **Phase 4: RemediationOrchestrator Creates WorkflowExecution (ONLY AFTER APPROVAL)**

**Controller**: RemediationOrchestrator Reconciler
**Watches**: AIAnalysis status changes
**CRD Created**: `WorkflowExecution`

**Source**: `docs/services/crd-controllers/03-workflowexecution/integration-points.md` lines 17-48

```go
func (r *RemediationOrchestratorReconciler) handleAIAnalysisCompleted(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    aiAnalysis *aiv1.AIAnalysis,
) error {
    // ✅ ONLY create workflow if AIAnalysis is approved
    if aiAnalysis.Status.Phase == "completed" && remediation.Status.WorkflowExecutionRef == nil {
        workflowExec := &workflowexecutionv1.WorkflowExecution{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-workflow", remediation.Name),
                Namespace: remediation.Namespace,
                OwnerReferences: []metav1.OwnerReference{
                    *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
                },
            },
            Spec: workflowexecutionv1.WorkflowExecutionSpec{
                RemediationRequestRef: workflowexecutionv1.RemediationRequestReference{
                    Name:      remediation.Name,
                    Namespace: remediation.Namespace,
                },
                WorkflowDefinition: buildWorkflowFromRecommendations(aiAnalysis.Status.Recommendations),
                ExecutionStrategy: workflowexecutionv1.ExecutionStrategy{
                    ApprovalRequired: false, // ✅ Already approved at AIAnalysis level
                    DryRunFirst:      true,
                    RollbackStrategy: "automatic",
                },
            },
        }

        return r.Create(ctx, workflowExec)
    }

    return nil
}
```

**WorkflowExecution CRD Created**:
```yaml
apiVersion: workflowexecution.kubernaut.ai/v1alpha1
kind: WorkflowExecution
metadata:
  name: aianalysis-oomkill-12345-workflow
spec:
  remediationRequestRef:
    name: remediation-oomkill-12345
  workflowDefinition:
    steps:
      - stepNumber: 1
        action: "collect_diagnostics"
        dependencies: []
      - stepNumber: 2
        action: "increase_resources"
        dependencies: [1]
      - stepNumber: 3
        action: "restart_pod"
        dependencies: [1,2]
  executionStrategy:
    approvalRequired: false  # ✅ Already approved at AIAnalysis level
    dryRunFirst: true
status:
  phase: "planning"
```

**Key Point**: **WorkflowExecution is ONLY created AFTER approval** (or auto-approval for high confidence)

---

### **Per-Step Approval (V1 Limitation)**

**Current State**: WorkflowExecution has workflow-level approval flag but **NO per-step approval gates**

**WorkflowExecution Approval (Workflow-Level)**:
```yaml
spec:
  executionStrategy:
    approvalRequired: true  # ⏸️ Workflow-level approval (wait for annotation before starting)
```

**How Workflow-Level Approval Works**:

**Source**: `docs/analysis/AI_TO_WORKFLOW_DETAILED_FLOW.md` lines 548-563

```go
func (r *WorkflowExecutionReconciler) handleValidatingPhase(
    ctx context.Context,
    workflow *workflowv1.WorkflowExecution,
) (ctrl.Result, error) {
    // ... safety checks ...

    // Check workflow-level approval (if required)
    if workflow.Spec.ExecutionStrategy.ApprovalRequired {
        approved := r.checkApprovalAnnotation(workflow)
        if !approved {
            // ⏸️ WAIT - Requeue until approval annotation added
            return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
        }
    }

    // Approval received → proceed to executing phase
    workflow.Status.Phase = "executing"
    return ctrl.Result{}, r.Status().Update(ctx, workflow)
}
```

**Operator Approves**:
```bash
# Add approval annotation to WorkflowExecution
kubectl annotate workflowexecution aianalysis-oomkill-12345-workflow \
  "kubernaut.ai/approved-by=operator@company.com"
```

**Limitation**: This is **all-or-nothing** - you approve the ENTIRE workflow, not individual steps.

---

### **Per-Step Approval (V2 - Future Enhancement)**

**Proposed Design** (not implemented in V1):

```yaml
spec:
  workflowDefinition:
    steps:
      - stepNumber: 1
        action: "collect_diagnostics"
        requiresApproval: false  # ✅ Safe action, auto-execute

      - stepNumber: 2
        action: "increase_resources"
        requiresApproval: false  # ✅ Low risk, auto-execute

      - stepNumber: 3
        action: "restart_pod"
        requiresApproval: true   # ⚠️ Production restart, requires approval
        approvalPolicy:
          approverGroups: ["sre-team", "platform-team"]
          timeout: "5m"

      - stepNumber: 4
        action: "update_hpa"
        requiresApproval: false  # ✅ Safe action after restart
```

**How Per-Step Approval Would Work** (V2):

1. WorkflowExecution reaches Step 3
2. WorkflowExecution Controller creates `WorkflowStepApprovalRequest` CRD
3. Human operator reviews and approves/rejects Step 3
4. If approved → create KubernetesExecution for Step 3
5. If rejected → skip Step 3, mark workflow as "partial-completion"

**V2 Benefit**: Granular control - approve/reject individual high-risk steps without blocking the entire workflow.

---

## **PART 2: CRD CREATION RESPONSIBILITIES**

### **Who Creates What CRDs?**

```
┌─────────────────────────────────────────────────────────────────────┐
│                    RemediationOrchestrator                           │
│                  (Central Orchestration Controller)                  │
└─────────────────────────────────────────────────────────────────────┘
                              │
                              ├─── Creates: RemediationProcessing
                              ├─── Creates: AIAnalysis
                              ├─── Creates: WorkflowExecution
                              └─── Creates: NotificationRequest ⭐
                                                  │
┌─────────────────────────────────────────────────┼───────────────────┐
│                  AIAnalysis Controller                                │
└───────────────────────────────────────────────────────────────────────┘
                              │
                              └─── Creates: AIApprovalRequest ⭐
                                                  │
┌─────────────────────────────────────────────────┼───────────────────┐
│              WorkflowExecution Controller                             │
└───────────────────────────────────────────────────────────────────────┘
                              │
                              ├─── Creates: KubernetesExecution (Step 1)
                              ├─── Creates: KubernetesExecution (Step 2)
                              └─── Creates: KubernetesExecution (Step 3)
                                   ❌ Does NOT create approval/notification CRDs
```

---

### **CRD Creation Matrix**

| CRD Type | Created By | When | Owner | Purpose |
|---|---|---|---|---|
| **RemediationRequest** | Gateway Service | Alert ingested | N/A (user-facing entry point) | Start remediation workflow |
| **RemediationProcessing** | RemediationOrchestrator | After RemediationRequest | RemediationRequest | Signal normalization & context enrichment |
| **AIAnalysis** | RemediationOrchestrator | After RemediationProcessing | RemediationRequest | AI-powered root cause analysis |
| **AIApprovalRequest** ⭐ | **AIAnalysis Controller** | During `approving` phase (confidence 60-79%) | AIAnalysis | Request manual approval for medium-confidence recommendations |
| **WorkflowExecution** | RemediationOrchestrator | After AIAnalysis completed | RemediationRequest | Multi-step workflow orchestration |
| **KubernetesExecution** | WorkflowExecution Controller | During `executing` phase (per step) | WorkflowExecution | Individual Kubernetes action execution |
| **NotificationRequest** ⭐ | **RemediationOrchestrator Controller** | During failure/timeout/escalation events | RemediationRequest | Notify operators of state changes |

**Source**: `docs/services/crd-controllers/06-notification/implementation/NOTIFICATION_CRD_CREATOR_CONFIDENCE_ASSESSMENT.md` lines 1-66

---

### **Why RemediationOrchestrator Creates NotificationRequest**

**Rationale** (90% confidence):
- **Centralized orchestration pattern**: RemediationOrchestrator has global visibility into all remediation phases
- **Single source of truth**: Only RemediationOrchestrator knows when to send notifications (failure, timeout, escalation)
- **Architectural consistency**: Follows established pattern from ADR-001 (RemediationOrchestrator creates all child CRDs)

**NotificationRequest Creation Triggers**:

```go
func (r *RemediationOrchestratorReconciler) handlePhaseTransitions(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
) error {
    switch remediation.Status.Phase {
    case "failed":
        // ⚠️ CREATE NotificationRequest for failure
        return r.createNotification(ctx, remediation, "critical", "Remediation failed permanently")

    case "timeout":
        // ⚠️ CREATE NotificationRequest for timeout
        return r.createNotification(ctx, remediation, "high", "Remediation timeout (exceeded 15min)")

    case "completed":
        // ℹ️ CREATE NotificationRequest for success (optional)
        if remediation.Spec.NotifyOnSuccess {
            return r.createNotification(ctx, remediation, "info", "Remediation completed successfully")
        }
    }

    return nil
}

func (r *RemediationOrchestratorReconciler) createNotification(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    severity string,
    message string,
) error {
    notification := &notificationv1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-notification-%d", remediation.Name, time.Now().Unix()),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
        },
        Spec: notificationv1.NotificationRequestSpec{
            Subject:  fmt.Sprintf("Remediation %s: %s", remediation.Status.Phase, remediation.Name),
            Body:     message,
            Type:     "escalation",
            Priority: severity,
            Channels: []string{"console", "slack"},
        },
    }

    return r.Create(ctx, notification)
}
```

**Example NotificationRequest CRD**:
```yaml
apiVersion: notification.kubernaut.ai/v1alpha1
kind: NotificationRequest
metadata:
  name: remediation-oomkill-12345-notification-1729160400
  namespace: production
  ownerReferences:
    - apiVersion: remediation.kubernaut.ai/v1alpha1
      kind: RemediationRequest
      name: remediation-oomkill-12345
      controller: true
spec:
  subject: "Remediation failed: remediation-oomkill-12345"
  body: "All retry attempts exhausted. Manual intervention required."
  type: "escalation"
  priority: "critical"
  channels:
    - console
    - slack
status:
  phase: "pending"
```

**Key Point**: **RemediationOrchestrator creates NotificationRequest CRDs**, not WorkflowExecution Controller.

---

## **PART 3: APPROVAL WORKFLOW SEQUENCE DIAGRAM**

```
┌────────────┐  ┌────────────────────┐  ┌──────────────┐  ┌────────────────────┐  ┌───────────────────┐
│  Gateway   │  │ RemediationOrch   │  │ AIAnalysis   │  │ Human Operator     │  │ WorkflowExecution │
└─────┬──────┘  └──────┬────────────┘  └──────┬───────┘  └─────────┬──────────┘  └─────────┬─────────┘
      │                │                       │                    │                        │
      │ 1. Create      │                       │                    │                        │
      │ RemediationReq │                       │                    │                        │
      ├───────────────>│                       │                    │                        │
      │                │                       │                    │                        │
      │                │ 2. Create             │                    │                        │
      │                │ AIAnalysis CRD        │                    │                        │
      │                ├──────────────────────>│                    │                        │
      │                │                       │                    │                        │
      │                │                       │ 3. HolmesGPT       │                        │
      │                │                       │ Investigation      │                        │
      │                │                       │ Confidence: 72.5%  │                        │
      │                │                       │ (Medium)           │                        │
      │                │                       │                    │                        │
      │                │                       │ 4. Create          │                        │
      │                │                       │ AIApprovalRequest  │                        │
      │                │                       │ ⏸️ WAIT FOR APPROVAL                        │
      │                │                       ├───────────────────>│                        │
      │                │                       │                    │                        │
      │                │                       │                    │ 5. Review & Approve    │
      │                │                       │                    │ (kubectl patch)        │
      │                │                       │<───────────────────┤                        │
      │                │                       │                    │                        │
      │                │                       │ 6. AIAnalysis      │                        │
      │                │                       │ phase: "completed" │                        │
      │                │<──────────────────────┤                    │                        │
      │                │                       │                    │                        │
      │                │ 7. Create             │                    │                        │
      │                │ WorkflowExecution CRD │                    │                        │
      │                │ (ONLY AFTER APPROVAL) │                    │                        │
      │                ├──────────────────────────────────────────────────────────────────> │
      │                │                       │                    │                        │
      │                │                       │                    │                        │
      │                │                       │                    │ 8. Execute Steps       │
      │                │                       │                    │ (NO further approval)  │
      │                │                       │                    │                        │
      │                │                       │                    │ Step 1: collect_diag   │
      │                │                       │                    │ Step 2: increase_res   │
      │                │                       │                    │ Step 3: restart_pod    │
      │                │                       │                    │                        │
```

**Key Timing**:
- **Approval happens at step 4** (before workflow creation)
- **Workflow executes steps 8** (no further approval needed)
- **Per-step approval NOT implemented** in V1

---

## 📊 **SUMMARY TABLE**

### **Approval Timing**

| Approval Type | When | Who Requests | Who Approves | Implemented |
|---|---|---|---|---|
| **AIAnalysis-Level** | **BEFORE workflow execution** (during AIAnalysis `approving` phase) | AIAnalysis Controller (creates AIApprovalRequest) | Human operator (updates AIApprovalRequest status) | ✅ V1 |
| **Workflow-Level** | BEFORE workflow starts (during WorkflowExecution `validating` phase) | WorkflowExecution waits for annotation | Human operator (adds approval annotation) | ✅ V1 (but redundant with AIAnalysis approval) |
| **Per-Step** | BEFORE each step execution (during WorkflowExecution `executing` phase) | WorkflowExecution Controller (would create WorkflowStepApprovalRequest) | Human operator (would approve/reject each step) | ❌ V2 (planned) |

**V1 Reality**: **Approval happens ONCE, before workflow execution, at the AIAnalysis level.**

---

### **CRD Creation Responsibilities**

| CRD | Creator | Phase | Purpose |
|---|---|---|---|
| **AIApprovalRequest** | **AIAnalysis Controller** | During AIAnalysis `approving` phase | Request manual approval for medium-confidence recommendations |
| **NotificationRequest** | **RemediationOrchestrator Controller** | During RemediationRequest failure/timeout/escalation | Notify operators of state changes |
| **WorkflowExecution** | RemediationOrchestrator Controller | After AIAnalysis `completed` (approved) | Orchestrate multi-step remediation |
| **KubernetesExecution** | WorkflowExecution Controller | During WorkflowExecution `executing` phase (per step) | Execute individual Kubernetes actions |

**Key Insight**: **WorkflowExecution Controller does NOT create approval or notification CRDs** - it only creates KubernetesExecution CRDs (one per workflow step).

---

## 🎯 **ARCHITECTURAL RATIONALE**

### **Why Approve Before Workflow Execution?**

**Benefits**:
1. ✅ **Single approval decision**: Human reviews ALL steps at once, not piecemeal
2. ✅ **Faster execution**: No mid-workflow pauses waiting for approval
3. ✅ **Simpler UX**: Operator sees complete remediation plan upfront
4. ✅ **Atomic workflow**: Either execute all steps or none (all-or-nothing)
5. ✅ **Lower latency**: Workflow executes uninterrupted after approval

**Trade-offs**:
- ⚠️ **All-or-nothing**: Can't approve/reject individual steps (V1 limitation)
- ⚠️ **Less granular control**: Can't conditionally approve based on step 1 outcome

**V2 Enhancement** (per-step approval):
- Would allow "approve Steps 1-2, wait for approval before Step 3"
- More complex UX but more flexible control
- Use case: "Let AI try low-risk fixes, but ask me before restarting production pods"

---

### **Why RemediationOrchestrator Creates Notifications?**

**Benefits**:
1. ✅ **Centralized orchestration**: RemediationOrchestrator has global view of all phases
2. ✅ **Single source of truth**: Only one controller decides when to notify
3. ✅ **Consistent pattern**: Follows ADR-001 (RemediationOrchestrator creates all child CRDs)
4. ✅ **Simplified logic**: WorkflowExecution doesn't need notification logic

**Alternative Rejected**: Each controller creates its own notifications
- ❌ Duplicate notifications (AIAnalysis fails → notify, WorkflowExecution fails → notify again)
- ❌ No coordination (who decides when to escalate?)
- ❌ Inconsistent notification content

---

## 📚 **REFERENCES**

1. **AIAnalysis Approval**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md` lines 547-603
2. **RemediationOrchestrator Integration**: `docs/services/crd-controllers/03-workflowexecution/integration-points.md` lines 17-48
3. **Workflow Approval**: `docs/analysis/AI_TO_WORKFLOW_DETAILED_FLOW.md` lines 548-563
4. **Notification Creation**: `docs/services/crd-controllers/06-notification/implementation/NOTIFICATION_CRD_CREATOR_CONFIDENCE_ASSESSMENT.md` lines 1-66
5. **CRD Schema**: `docs/services/crd-controllers/03-workflowexecution/crd-schema.md` lines 55-92
6. **Design Decisions**: `docs/architecture/DESIGN_DECISIONS.md`

---

**Document Owner**: Platform Architecture Team
**Review Frequency**: When approval workflow capabilities change
**Next Review Date**: 2026-01-17

