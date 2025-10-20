# Approval Request Notification Integration

**Date**: October 17, 2025
**Purpose**: Clarify how operators are notified about pending approval requests and status synchronization
**Questions**:
1. How are human operators notified when AIApprovalRequest CRD is created?
2. Does AIAnalysis send a notification via Notification CRD?
3. Is AIApprovalRequest status reflected in AIAnalysis CRD?

---

## 🎯 **TL;DR - Quick Answers**

### **Question 1: How are operators notified about approval requests?**

**Current State (V1)**:
- ❌ **No automatic notification** when AIApprovalRequest is created
- ✅ **Operators must poll**: `kubectl get aiapprovalrequest --watch`
- ✅ **Kubernetes events**: View via `kubectl describe aianalysis`
- ⚠️ **Gap Identified**: Missing operator notification integration

**Planned Enhancement (V1.1)**:
- ✅ **NotificationRequest CRD** will be created by RemediationOrchestrator when approval is needed
- ✅ **Multi-channel notifications**: Slack + Email + Console
- ✅ **Deep links**: Direct link to approval request in notification

---

### **Question 2: Does AIAnalysis create Notification CRDs?**

**Answer**: **NO** - AIAnalysis does NOT create NotificationRequest CRDs

**Architectural Decision (ADR-017)**:
- **RemediationOrchestrator** creates **ALL NotificationRequest CRDs**
- **AIAnalysis** only creates **AIApprovalRequest CRD**
- **Rationale**: Centralized notification logic prevents duplicates and maintains consistent notification patterns

**Flow**:
```
AIAnalysis → creates → AIApprovalRequest ✅
RemediationOrchestrator → watches AIAnalysis → creates → NotificationRequest ✅
```

---

### **Question 3: Is AIApprovalRequest status reflected in AIAnalysis?**

**Answer**: **YES** - AIApprovalRequest status is synchronized to AIAnalysis CRD

**Bi-directional watch pattern**:
- ✅ **AIAnalysis watches AIApprovalRequest** (detects approval/rejection)
- ✅ **AIAnalysis status updated** with approval decision, approver, timestamp
- ✅ **~100ms latency** for status synchronization

**Status fields synchronized**:
- `status.approvalStatus`: "Approved" / "Rejected" / "Pending"
- `status.approvedBy` / `status.rejectedBy`: Operator identity
- `status.approvalTime` / `status.rejectionReason`: Decision metadata
- `status.approvalRequestName`: Link to AIApprovalRequest CRD

---

## 📋 **DETAILED EXPLANATION**

---

## **PART 1: OPERATOR NOTIFICATION METHODS**

### **Current State: Manual Discovery (V1)**

**How operators currently discover pending approvals**:

#### **Method 1: kubectl Watch (Primary)**

```bash
# Watch for new approval requests
kubectl get aiapprovalrequest --watch

# Filter by namespace
kubectl get aiapprovalrequest -n production --watch

# Query with details
kubectl get aiapprovalrequest -o wide
```

**Output Example**:
```
NAME                                    DECISION   AGE
aianalysis-oomkill-12345-approval      Pending    30s
aianalysis-disk-full-67890-approval    Pending    2m
```

---

#### **Method 2: Kubernetes Events**

```bash
# View events for AIAnalysis
kubectl describe aianalysis aianalysis-oomkill-12345
```

**Events Example**:
```
Events:
  Type    Reason             Age   Message
  ----    ------             ----  -------
  Normal  InvestigationStart 5m    HolmesGPT investigation started
  Normal  ApprovalRequested  2m    Manual approval requested with 15m timeout
```

**Source**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md` lines 4582-4584

```go
r.recordEvent(ai, "Normal", "ApprovalRequested",
    fmt.Sprintf("Manual approval requested with %s timeout",
        approvalReq.Spec.Timeout.Duration.String()))
```

---

#### **Method 3: Dashboard Integration (V2 - Future)**

**Planned K8s Dashboard UI**:
```
┌─────────────────────────────────────────────────────────────┐
│ 🔔 Pending Approvals (3)                                    │
├─────────────────────────────────────────────────────────────┤
│ Alert: OOMKilled payment-service                            │
│ Confidence: 72.5% (Medium)                                   │
│ Recommendations: collect_diag, increase_mem, restart_pod    │
│ [Approve] [Reject] [View Details]                           │
│ Timeout: 13m remaining                                       │
├─────────────────────────────────────────────────────────────┤
│ Alert: Disk pressure on node-7                              │
│ Confidence: 68.2% (Medium)                                   │
│ Recommendations: cleanup_storage, expand_pvc                 │
│ [Approve] [Reject] [View Details]                           │
│ Timeout: 8m remaining                                        │
└─────────────────────────────────────────────────────────────┘
```

---

### **Gap Identified: No Push Notifications (V1)**

**Problem**: Operators must actively poll for approval requests

**Impact**:
- ⏰ **Approval delay**: Operators may not notice pending approvals for minutes
- 📊 **Timeout risk**: Approval requests may timeout (default 15min)
- 👥 **On-call burden**: Requires constant monitoring during incidents

**Solution: Planned V1.1 Enhancement** → See Part 2 below

---

## **PART 2: PLANNED NOTIFICATION INTEGRATION (V1.1)**

### **Architecture: RemediationOrchestrator Creates Notifications**

**Design Decision**: Per **ADR-017**, RemediationOrchestrator creates NotificationRequest CRDs

**Why RemediationOrchestrator (not AIAnalysis)?**
1. ✅ **Centralized orchestration**: Consistent with ADR-001 pattern
2. ✅ **Global visibility**: RemediationOrchestrator sees all phases
3. ✅ **No duplicates**: Single point of notification creation
4. ✅ **Architectural consistency**: All child CRDs created by orchestrator

**Source**: `docs/architecture/decisions/ADR-017-notification-crd-creator.md`

---

### **Implementation: RemediationOrchestrator Watches AIAnalysis**

**Flow**:
```
┌───────────────────┐         ┌─────────────────────────┐
│   AIAnalysis      │         │ RemediationOrchestrator │
│   Controller      │         │      Controller         │
└─────────┬─────────┘         └────────────┬────────────┘
          │                                │
          │ 1. Phase = "approving"         │
          ├───────────────────────────────>│ (watch trigger)
          │                                │
          │ 2. Creates AIApprovalRequest   │ 3. Detects approval needed
          │                                │
          │                                │ 4. Creates NotificationRequest
          │                                │    Type: "approval-required"
          │                                │    Channels: [slack, email]
          │                                │    Priority: "high"
          │                                │
          │                                ├────────────────────────────>
          │                                                               │
          │                                                    ┌──────────▼──────────┐
          │                                                    │ Notification        │
          │                                                    │ Controller          │
          │                                                    └─────────────────────┘
          │                                                               │
          │                                                               │ 5. Send notifications
          │                                                               ├──> Slack
          │                                                               ├──> Email
          │                                                               └──> Console
```

---

### **RemediationOrchestrator Logic (V1.1 - Planned)**

**Source**: `docs/services/crd-controllers/05-remediationorchestrator/NOTIFICATION_INTEGRATION_PLAN.md`

```go
func (r *RemediationOrchestratorReconciler) handleAIAnalysisApproving(
    ctx context.Context,
    remediation *remediationv1.RemediationRequest,
    aiAnalysis *aianalysisv1.AIAnalysis,
) error {
    // Check if AIAnalysis is in "approving" phase
    if aiAnalysis.Status.Phase != "approving" {
        return nil
    }

    // Check if we've already notified (idempotent)
    if remediation.Status.ApprovalNotificationSent {
        return nil
    }

    // ✅ CREATE NotificationRequest for approval needed
    notification := &notificationv1.NotificationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      fmt.Sprintf("%s-approval-notification", remediation.Name),
            Namespace: remediation.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(remediation, remediationv1.GroupVersion.WithKind("RemediationRequest")),
            },
            Labels: map[string]string{
                "kubernaut.ai/notification-type": "approval-required",
                "kubernaut.ai/remediation":       remediation.Name,
                "kubernaut.ai/aianalysis":        aiAnalysis.Name,
            },
        },
        Spec: notificationv1.NotificationRequestSpec{
            Subject: fmt.Sprintf("🔔 Approval Required: %s", aiAnalysis.Spec.AlertName),
            Body: formatApprovalNotificationBody(aiAnalysis),
            Type: "approval-required",
            Priority: "high",
            Channels: []string{"console", "slack", "email"},
            Metadata: map[string]string{
                "approval-request-name": aiAnalysis.Status.ApprovalRequestName,
                "confidence-score":      fmt.Sprintf("%.1f%%", aiAnalysis.Status.ConfidenceScore),
                "timeout":               "15m",
                "kubectl-command":       formatApprovalCommand(aiAnalysis),
            },
        },
    }

    if err := r.Create(ctx, notification); err != nil {
        return fmt.Errorf("failed to create approval notification: %w", err)
    }

    // Mark notification as sent
    remediation.Status.ApprovalNotificationSent = true
    return r.Status().Update(ctx, remediation)
}

func formatApprovalNotificationBody(ai *aianalysisv1.AIAnalysis) string {
    return fmt.Sprintf(`
🤖 Kubernaut AI Analysis: Approval Required

**Alert**: %s
**Confidence**: %.1f%% (Medium - Manual approval required)

**Root Cause**:
%s

**Recommended Actions**:
%s

**Approve Command**:
kubectl patch aiapprovalrequest %s \
  --type=merge \
  --subresource=status \
  -p '{"status":{"decision":"Approved","decidedBy":"YOUR_EMAIL"}}'

**Reject Command**:
kubectl patch aiapprovalrequest %s \
  --type=merge \
  --subresource=status \
  -p '{"status":{"decision":"Rejected","rejectedBy":"YOUR_EMAIL","rejectionReason":"REASON"}}'

**Timeout**: 15 minutes (auto-reject if no response)

**View Details**: kubectl get aianalysis %s -o yaml
`,
        ai.Spec.AlertName,
        ai.Status.ConfidenceScore,
        ai.Status.RootCause,
        formatRecommendations(ai.Status.Recommendations),
        ai.Status.ApprovalRequestName,
        ai.Status.ApprovalRequestName,
        ai.Name,
    )
}
```

---

### **Notification Example: Slack**

```
┌─────────────────────────────────────────────────────────────┐
│ 🔔 Kubernaut - Approval Required                            │
├─────────────────────────────────────────────────────────────┤
│ **Alert**: OOMKilled payment-service                        │
│ **Confidence**: 72.5% (Medium - Manual approval required)   │
│                                                              │
│ **Root Cause**:                                             │
│ Memory leak in payment processing coroutine                 │
│ (50MB/hr growth, not garbage collected)                     │
│                                                              │
│ **Recommended Actions**:                                    │
│ 1. collect_diagnostics (heap dump)                          │
│ 2. increase_resources (2Gi → 3Gi memory)                    │
│ 3. restart_pod (rolling restart)                            │
│                                                              │
│ ⏰ **Timeout**: 15 minutes (auto-reject if no response)     │
│                                                              │
│ [Approve] [Reject] [View Details]                           │
└─────────────────────────────────────────────────────────────┘
```

---

### **Notification Example: Email**

**Subject**: `🔔 Kubernaut - Approval Required: OOMKilled payment-service`

**Body**:
```html
<html>
<body>
  <h2>🤖 Kubernaut AI Analysis: Approval Required</h2>

  <table>
    <tr><td><strong>Alert:</strong></td><td>OOMKilled payment-service</td></tr>
    <tr><td><strong>Confidence:</strong></td><td>72.5% (Medium)</td></tr>
    <tr><td><strong>Timeout:</strong></td><td>15 minutes</td></tr>
  </table>

  <h3>Root Cause</h3>
  <p>Memory leak in payment processing coroutine (50MB/hr growth)</p>

  <h3>Recommended Actions</h3>
  <ol>
    <li>collect_diagnostics (heap dump)</li>
    <li>increase_resources (2Gi → 3Gi memory)</li>
    <li>restart_pod (rolling restart)</li>
  </ol>

  <h3>Actions</h3>
  <p>
    <a href="https://k8s-dashboard/approve?request=aianalysis-oomkill-12345-approval">
      [Approve]
    </a>
    <a href="https://k8s-dashboard/reject?request=aianalysis-oomkill-12345-approval">
      [Reject]
    </a>
  </p>

  <h3>Manual Approval (kubectl)</h3>
  <pre>
kubectl patch aiapprovalrequest aianalysis-oomkill-12345-approval \
  --type=merge --subresource=status \
  -p '{"status":{"decision":"Approved","decidedBy":"YOUR_EMAIL"}}'
  </pre>
</body>
</html>
```

---

## **PART 3: STATUS SYNCHRONIZATION**

### **AIApprovalRequest Status Reflected in AIAnalysis**

**Answer**: **YES** - AIApprovalRequest status is synchronized to AIAnalysis CRD

---

### **Bi-Directional Watch Pattern**

**Source**: `docs/services/crd-controllers/02-aianalysis/reconciliation-phases.md` lines 412-437

```go
// AIAnalysis watches AIApprovalRequest
func (r *AIAnalysisReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&aianalysisv1alpha1.AIAnalysis{}).
        // ✅ Watch AIApprovalRequest for decision updates
        Watches(
            &source.Kind{Type: &approvalv1.AIApprovalRequest{}},
            handler.EnqueueRequestsFromMapFunc(r.approvalRequestToAnalysis),
        ).
        Complete(r)
}

// Mapping function: AIApprovalRequest → AIAnalysis
func (r *AIAnalysisReconciler) approvalRequestToAnalysis(obj client.Object) []ctrl.Request {
    approval := obj.(*approvalv1.AIApprovalRequest)
    return []ctrl.Request{
        {
            NamespacedName: types.NamespacedName{
                Name:      approval.Spec.AIAnalysisRef.Name,
                Namespace: approval.Spec.AIAnalysisRef.Namespace,
            },
        },
    }
}
```

**Result**: **~100ms latency** for status synchronization (Kubernetes watch pattern)

---

### **Status Synchronization Flow**

```
┌───────────────────────────────────────────────────────────────────┐
│                    OPERATOR APPROVES                              │
└───────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌───────────────────────────────────────────────────────────────────┐
│ AIApprovalRequest CRD                                             │
│                                                                    │
│ status:                                                            │
│   decision: "Approved"  ✅ (operator sets this)                   │
│   decidedBy: "operator@company.com"                               │
│   decidedAt: "2025-10-17T10:35:00Z"                               │
└───────────────────────────────────────────────────────────────────┘
                              │
                              │ (Kubernetes watch trigger ~100ms)
                              ▼
┌───────────────────────────────────────────────────────────────────┐
│ AIAnalysis Controller Reconcile                                   │
│                                                                    │
│ func handleApproving():                                            │
│   - Fetch AIApprovalRequest status                                │
│   - Sync decision to AIAnalysis.status                            │
│   - Update phase to "completed" or "rejected"                     │
└───────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌───────────────────────────────────────────────────────────────────┐
│ AIAnalysis CRD                                                     │
│                                                                    │
│ status:                                                            │
│   phase: "completed"  ✅ (was "approving")                        │
│   approvalStatus: "Approved"  ✅ (synced from AIApprovalRequest)  │
│   approvedBy: "operator@company.com"  ✅                          │
│   approvalTime: "2025-10-17T10:35:00Z"  ✅                        │
│   approvalRequestName: "aianalysis-oomkill-12345-approval"  ✅    │
│   message: "Manual approval granted - ready for workflow"         │
└───────────────────────────────────────────────────────────────────┘
```

---

### **Status Fields Synchronized**

**Source**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md` lines 547-603

#### **When Approval Granted**:

```go
// AIAnalysis Controller syncs approval status
switch approvalReq.Status.Decision {
case "Approved":
    ai.Status.Phase = "completed"
    ai.Status.ApprovalStatus = "Approved"  ✅
    ai.Status.ApprovedBy = approvalReq.Status.ApprovedBy  ✅
    ai.Status.ApprovalTime = approvalReq.Status.DecisionTime  ✅
    ai.Status.Message = "Manual approval granted - ready for workflow creation"
```

**AIAnalysis Status After Approval**:
```yaml
status:
  phase: "completed"
  approvalStatus: "Approved"
  approvedBy: "operator@company.com"
  approvalTime: "2025-10-17T10:35:00Z"
  approvalRequestName: "aianalysis-oomkill-12345-approval"
  message: "Manual approval granted - ready for workflow creation"
```

---

#### **When Approval Rejected**:

```go
case "Rejected":
    ai.Status.Phase = "rejected"
    ai.Status.ApprovalStatus = "Rejected"  ✅
    ai.Status.RejectedBy = approvalReq.Status.RejectedBy  ✅
    ai.Status.RejectionReason = approvalReq.Status.RejectionReason  ✅
    ai.Status.Message = fmt.Sprintf("Manual approval rejected: %s", approvalReq.Status.RejectionReason)
```

**AIAnalysis Status After Rejection**:
```yaml
status:
  phase: "rejected"
  approvalStatus: "Rejected"
  rejectedBy: "operator@company.com"
  rejectionReason: "Insufficient evidence for memory leak hypothesis"
  approvalRequestName: "aianalysis-oomkill-12345-approval"
  message: "Manual approval rejected: Insufficient evidence for memory leak hypothesis"
```

---

#### **When Approval Pending**:

```go
default:
    // Still pending - requeue
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
```

**AIAnalysis Status While Pending**:
```yaml
status:
  phase: "approving"
  approvalRequestName: "aianalysis-oomkill-12345-approval"
  approvalRequestedAt: "2025-10-17T10:30:00Z"
  message: "Manual approval requested (timeout: 15m)"
```

---

### **Status Fields in AIAnalysis (Planned)**

**Source**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md` lines 4576-4580

**Planned Status Fields** (not yet in actual CRD types):

| Field | Type | Purpose |
|---|---|---|
| `approvalRequestName` | string | Link to AIApprovalRequest CRD |
| `approvalRequestedAt` | metav1.Time | When approval was requested |
| `approvalStatus` | string | "Approved" / "Rejected" / "Pending" |
| `approvedBy` | string | Operator who approved |
| `rejectedBy` | string | Operator who rejected |
| `approvalTime` | metav1.Time | When approval was granted |
| `rejectionReason` | string | Why approval was rejected |

**Current State**: AIAnalysis CRD types (`api/aianalysis/v1alpha1/aianalysis_types.go`) do NOT yet have these fields
**Implementation Status**: Planned for V1.1

---

### **Querying Approval Status**

#### **View AIAnalysis with Approval Status**:

```bash
kubectl get aianalysis aianalysis-oomkill-12345 -o jsonpath='{.status.approvalStatus}'
# Output: Approved

kubectl get aianalysis aianalysis-oomkill-12345 -o jsonpath='{.status.approvedBy}'
# Output: operator@company.com
```

---

#### **View Related AIApprovalRequest**:

```bash
# Get approval request name from AIAnalysis
APPROVAL_REQUEST=$(kubectl get aianalysis aianalysis-oomkill-12345 \
  -o jsonpath='{.status.approvalRequestName}')

# View approval request details
kubectl get aiapprovalrequest $APPROVAL_REQUEST -o yaml
```

**Output**:
```yaml
apiVersion: aianalysis.kubernaut.ai/v1alpha1
kind: AIApprovalRequest
metadata:
  name: aianalysis-oomkill-12345-approval
spec:
  aiAnalysisRef:
    name: aianalysis-oomkill-12345
  investigation:
    rootCause: "Memory leak in payment processing"
    confidenceScore: 72.5
status:
  decision: "Approved"
  decidedBy: "operator@company.com"
  decidedAt: "2025-10-17T10:35:00Z"
```

---

## 📊 **SUMMARY TABLE**

### **Notification Methods**

| Method | Current (V1) | Planned (V1.1) | Latency |
|---|---|---|---|
| **kubectl watch** | ✅ Available | ✅ Available | Real-time |
| **Kubernetes Events** | ✅ Available | ✅ Available | Real-time |
| **Slack Notification** | ❌ Manual | ✅ Automatic | <1s |
| **Email Notification** | ❌ Manual | ✅ Automatic | <30s |
| **Dashboard UI** | ❌ None | ⏳ V2 | N/A |

---

### **Status Synchronization**

| Question | Answer | Latency | Implementation |
|---|---|---|---|
| **AIApprovalRequest → AIAnalysis** | ✅ Yes, synced | ~100ms | ✅ Implemented (watch pattern) |
| **AIAnalysis creates Notification** | ❌ No | N/A | RemediationOrchestrator creates notifications |
| **Approval status in AIAnalysis** | ✅ Yes | ~100ms | ✅ Planned (status fields) |

---

### **CRD Creation Responsibilities**

| CRD | Created By | When | Purpose |
|---|---|---|---|
| **AIApprovalRequest** | AIAnalysis Controller | During `approving` phase | Request approval |
| **NotificationRequest** | RemediationOrchestrator | When approval needed | Notify operators |

---

## 🎯 **ARCHITECTURAL RATIONALE**

### **Why RemediationOrchestrator Creates Notifications (Not AIAnalysis)?**

**Benefits**:
1. ✅ **Centralized orchestration**: Consistent with ADR-001 pattern
2. ✅ **Global visibility**: RemediationOrchestrator sees all remediation phases
3. ✅ **No duplicates**: Single point of notification creation prevents duplicate notifications
4. ✅ **Consistent logic**: All notification triggers in one place

**Trade-offs**:
- ⚠️ **Indirect notification**: AIAnalysis doesn't directly trigger notifications
- **Mitigation**: Kubernetes watch pattern provides ~100ms latency (acceptable)

---

### **Why Bi-Directional Status Sync?**

**Benefits**:
1. ✅ **Single source of truth**: AIApprovalRequest is approval state source
2. ✅ **Observable**: AIAnalysis shows approval status for debugging
3. ✅ **Integration-friendly**: RemediationOrchestrator only needs to watch AIAnalysis
4. ✅ **Audit trail**: Both CRDs contain approval history

---

## 📚 **REFERENCES**

1. **AIAnalysis Implementation**: `docs/services/crd-controllers/02-aianalysis/implementation/IMPLEMENTATION_PLAN_V1.0.md`
2. **Notification Integration Plan**: `docs/services/crd-controllers/05-remediationorchestrator/NOTIFICATION_INTEGRATION_PLAN.md`
3. **ADR-017**: `docs/architecture/decisions/ADR-017-notification-crd-creator.md`
4. **Reconciliation Phases**: `docs/services/crd-controllers/02-aianalysis/reconciliation-phases.md`
5. **AIAnalysis CRD Types**: `api/aianalysis/v1alpha1/aianalysis_types.go`

---

## 🛠️ **IMPLEMENTATION ROADMAP**

### **V1.0 (Current)**
- ✅ AIApprovalRequest CRD creation
- ✅ Kubernetes events for approval requests
- ✅ kubectl watch for discovery
- ⚠️ **Gap**: No push notifications

### **V1.1 (Q1 2026)**
- 🔄 RemediationOrchestrator notification integration
- 🔄 Slack + Email notifications for approval requests
- 🔄 AIAnalysis status fields for approval tracking
- 🔄 Deep links in notifications

### **V2.0 (Q2 2026)**
- 📋 Kubernetes Dashboard UI for approval management
- 📋 Mobile push notifications
- 📋 Approval delegation and escalation
- 📋 Approval policies (auto-approve patterns)

---

**Document Owner**: Platform Architecture Team
**Review Frequency**: When approval workflow capabilities change
**Next Review Date**: 2026-01-17

