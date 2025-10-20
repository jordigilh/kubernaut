# RemediationOrchestrator - Approval Notification Documentation Plan

**Date**: October 20, 2025
**Status**: 📋 **DOCUMENTATION PLAN** (Not Yet Implemented)
**Related**: ADR-018 (Approval Notification V1.0 Integration)
**Target**: Extend existing `NOTIFICATION_INTEGRATION_PLAN.md` or Implementation Plan v1.0.2

---

## 🎯 **Purpose**

Document how to integrate **BR-ORCH-001** (Create NotificationRequest CRDs for approval requests) into the RemediationOrchestrator Implementation Plan v1.0.2.

**This is a NEW requirement** from ADR-018 that adds approval notification capability to the existing completion/failure notifications.

---

## 📋 **Current Status**

### **Existing Coverage**:
- ✅ **ADR-017**: NotificationRequest CRD creation for completion/failure (existing plan)
- ✅ **NOTIFICATION_INTEGRATION_PLAN.md**: Implementation guide for basic notifications
- ✅ **BR-REM-060**: NotificationRequest CRD creation for escalation (v1.0.2)
- ✅ **Day 11**: Escalation workflow with Notification Service integration

### **Missing Coverage** (NEW from ADR-018):
- ❌ **BR-ORCH-001**: Create NotificationRequest CRDs for approval requests
- ❌ Watch AIAnalysis CRDs for `phase="Approving"`
- ❌ Extract approval context from `AIAnalysis.status.approvalContext`
- ❌ Format approval notification body with rich context
- ❌ Track `status.approvalNotificationSent` to prevent duplicates

---

## 📝 **Proposed Documentation Updates**

### **Update 1: Add to Business Requirements Section**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Business Requirements section

**Add After "BR-REM-060: NotificationRequest CRD creation for escalation"**:

```markdown
### **Approval Notification Creation (BR-ORCH-001) - 1 NEW BR**

**BR-ORCH-001**: RemediationOrchestrator Approval Notification Creation (P0)
- **Requirement**: Create NotificationRequest CRDs when AIAnalysis requires approval
- **Trigger**: AIAnalysis `status.phase = "Approving"` and `status.requiresApproval = true`
- **Context Source**: `AIAnalysis.status.approvalContext` (BR-AI-059)
- **Notification Type**: `escalation` (high priority)
- **Channels**: Slack + Console (V1), Policy-based routing (V2)
- **Idempotency**: Track `status.approvalNotificationSent` to prevent duplicates
- **Owner Reference**: NotificationRequest owned by RemediationRequest (cascade deletion)
- **Metadata**: Include `aiAnalysisName`, `aiApprovalRequestName`, `confidence`, `priority`
- **Validation**: Integration tests verify notification created when approval required

**Why BR-ORCH-001?**
- **Problem**: 40-60% of approval requests timeout due to lack of notifications
- **Impact**: $392K lost per timeout (large enterprise with $7K/min downtime cost)
- **Solution**: Push notifications (Slack/Email) when approval required
- **Value**: 91% MTTR reduction (60 min → 5 min avg)

**Related**: ADR-018 (Approval Notification Integration), BR-AI-059 (Approval Context Capture)
**Reference**: [APPROVAL_NOTIFICATION_BUSINESS_REQUIREMENTS.md](../../../../requirements/APPROVAL_NOTIFICATION_BUSINESS_REQUIREMENTS.md)
```

---

### **Update 2: Add to Day 11 (Escalation Workflow)**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Timeline section, Day 11

**Current** (Line ~141):
```markdown
| **Day 11** | Escalation Workflow | 8h | Notification Service integration, NotificationRequest CRD creation |
```

**Proposed**:
```markdown
| **Day 11** | Escalation & Approval Notifications | 10h | Notification Service integration, NotificationRequest CRD creation for failures + approvals (BR-ORCH-001) |
```

**Effort Impact**: +2 hours (8h → 10h) for approval notification logic

---

### **Update 3: Add Day 11 Implementation Details**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Day 11: Escalation Workflow section

**Add New Section** (after existing escalation logic):

#### **Day 11.5: Approval Notification Creation (BR-ORCH-001)**

**Goal**: Create NotificationRequest CRDs when AIAnalysis requires approval

**Implementation Steps**:

1. **Watch AIAnalysis for Approval Requirement**:
   ```go
   // Already implemented: RemediationOrchestrator watches AIAnalysis
   // Extend Reconcile to check for approval state

   func (r *RemediationRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
       // ... fetch RemediationRequest ...

       // Check if AIAnalysis exists and requires approval
       if remediation.Status.AIAnalysisRef != nil {
           var aiAnalysis aianalysisv1alpha1.AIAnalysis
           aiAnalysisKey := types.NamespacedName{
               Name:      remediation.Status.AIAnalysisRef.Name,
               Namespace: remediation.Status.AIAnalysisRef.Namespace,
           }

           if err := r.Get(ctx, aiAnalysisKey, &aiAnalysis); err != nil {
               if errors.IsNotFound(err) {
                   log.Info("AIAnalysis not found yet", "aiAnalysisName", aiAnalysisKey.Name)
                   return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
               }
               return ctrl.Result{}, err
           }

           // Check for approval requirement and notification not sent yet
           if aiAnalysis.Status.Phase == "Approving" && !remediation.Status.ApprovalNotificationSent {
               log.Info("AIAnalysis requires approval, creating notification")

               // Create approval notification
               if err := r.createApprovalNotification(ctx, &remediation, &aiAnalysis); err != nil {
                   log.Error(err, "Failed to create approval notification")
                   r.Recorder.Event(&remediation, corev1.EventTypeWarning, "NotificationFailed",
                       fmt.Sprintf("Failed to create approval notification: %v", err))
                   return ctrl.Result{}, err
               }

               // Update status to track notification sent (prevent duplicates)
               remediation.Status.ApprovalNotificationSent = true
               if err := r.Status().Update(ctx, &remediation); err != nil {
                   log.Error(err, "Failed to update RemediationRequest status")
                   return ctrl.Result{}, err
               }

               log.Info("Approval notification created successfully")
               r.Recorder.Event(&remediation, corev1.EventTypeNormal, "NotificationCreated",
                   "Approval notification sent to operators")
           }
       }

       return ctrl.Result{}, nil
   }
   ```

2. **Create Approval Notification**:
   ```go
   // Create NotificationRequest CRD for operator approval (BR-ORCH-001)
   func (r *RemediationRequestReconciler) createApprovalNotification(
       ctx context.Context,
       remediation *remediationv1alpha1.RemediationRequest,
       aiAnalysis *aianalysisv1alpha1.AIAnalysis,
   ) error {
       log := ctrl.LoggerFrom(ctx)

       // Generate notification name
       notificationName := fmt.Sprintf("approval-notification-%s-%s",
           remediation.Name,
           aiAnalysis.Name)

       // Check if notification already exists (idempotency)
       var existingNotification notificationv1alpha1.NotificationRequest
       notificationKey := types.NamespacedName{
           Name:      notificationName,
           Namespace: remediation.Namespace,
       }

       err := r.Get(ctx, notificationKey, &existingNotification)
       if err == nil {
           log.Info("Notification already exists, skipping creation")
           return nil
       } else if !errors.IsNotFound(err) {
           return fmt.Errorf("failed to check existing notification: %w", err)
       }

       // Format notification body
       body := r.formatApprovalBody(remediation, aiAnalysis)

       // Create NotificationRequest CRD
       notification := &notificationv1alpha1.NotificationRequest{
           ObjectMeta: metav1.ObjectMeta{
               Name:      notificationName,
               Namespace: remediation.Namespace,
               Labels: map[string]string{
                   "kubernaut.io/notification-type": "approval",
                   "kubernaut.io/remediation":       remediation.Name,
                   "kubernaut.io/aianalysis":        aiAnalysis.Name,
               },
               // Set owner reference for automatic cleanup
               OwnerReferences: []metav1.OwnerReference{
                   {
                       APIVersion: remediation.APIVersion,
                       Kind:       remediation.Kind,
                       Name:       remediation.Name,
                       UID:        remediation.UID,
                       Controller: func() *bool { b := true; return &b }(),
                   },
               },
           },
           Spec: notificationv1alpha1.NotificationRequestSpec{
               Type:     notificationv1alpha1.NotificationTypeEscalation,
               Priority: notificationv1alpha1.NotificationPriorityHigh,
               Recipients: []notificationv1alpha1.Recipient{
                   {
                       Slack: "#ops-alerts", // TODO: Make configurable
                   },
               },
               Subject:  fmt.Sprintf("🚨 Approval Required: %s", remediation.Spec.SignalName),
               Body:     body,
               Channels: []notificationv1alpha1.Channel{
                   notificationv1alpha1.ChannelSlack,
                   notificationv1alpha1.ChannelConsole,
               },
               Metadata: map[string]string{
                   "remediationRequest":  remediation.Name,
                   "aiAnalysis":          aiAnalysis.Name,
                   "aiApprovalRequest":   aiAnalysis.Status.ApprovalRequestName,
                   "confidence":          fmt.Sprintf("%.2f", aiAnalysis.Status.Confidence),
                   "signalNamespace":     remediation.Spec.Namespace,
                   "signalSeverity":      remediation.Spec.Severity,
                   "signalPriority":      remediation.Spec.Priority,
                   "approvalRequestedAt": aiAnalysis.Status.ApprovalRequestedAt.Format(time.RFC3339),
               },
           },
       }

       // Create notification
       if err := r.Create(ctx, notification); err != nil {
           return fmt.Errorf("failed to create NotificationRequest: %w", err)
       }

       log.Info("NotificationRequest CRD created",
           "notificationName", notificationName,
           "channels", notification.Spec.Channels)

       return nil
   }
   ```

3. **Format Approval Notification Body**:
   ```go
   // Format notification body with approval context (BR-AI-059)
   func (r *RemediationRequestReconciler) formatApprovalBody(
       remediation *remediationv1alpha1.RemediationRequest,
       aiAnalysis *aianalysisv1alpha1.AIAnalysis,
   ) string {
       var body strings.Builder

       // Header
       body.WriteString(fmt.Sprintf("## 🚨 AI Approval Required\n\n"))
       body.WriteString(fmt.Sprintf("**Signal**: %s\n", remediation.Spec.SignalName))
       body.WriteString(fmt.Sprintf("**Namespace**: `%s`\n", remediation.Spec.Namespace))
       body.WriteString(fmt.Sprintf("**Severity**: %s | **Priority**: %s\n\n",
           remediation.Spec.Severity,
           remediation.Spec.Priority))

       // Approval context (if available)
       if aiAnalysis.Status.ApprovalContext != nil {
           ctx := aiAnalysis.Status.ApprovalContext

           body.WriteString(fmt.Sprintf("### Why Approval Required\n%s\n\n", ctx.WhyApprovalRequired))

           body.WriteString(fmt.Sprintf("### AI Investigation Summary\n"))
           body.WriteString(fmt.Sprintf("**Root Cause**: %s\n", aiAnalysis.Status.RootCause))
           body.WriteString(fmt.Sprintf("**Confidence**: %.1f%% (%s)\n\n",
               ctx.ConfidenceScore*100,
               ctx.ConfidenceLevel))

           if ctx.InvestigationSummary != "" {
               body.WriteString(fmt.Sprintf("%s\n\n", ctx.InvestigationSummary))
           }

           // Recommended actions
           if len(ctx.RecommendedActions) > 0 {
               body.WriteString("### Recommended Actions\n")
               for i, action := range ctx.RecommendedActions {
                   body.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n", i+1, action.Action, action.Rationale))
               }
               body.WriteString("\n")
           }

           // Alternatives considered
           if len(ctx.AlternativesConsidered) > 0 {
               body.WriteString("### Alternatives Considered\n")
               for i, alt := range ctx.AlternativesConsidered {
                   body.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n", i+1, alt.Approach, alt.ProsCons))
               }
               body.WriteString("\n")
           }

           // Evidence
           if len(ctx.EvidenceCollected) > 0 {
               body.WriteString("### Evidence\n")
               for _, evidence := range ctx.EvidenceCollected {
                   body.WriteString(fmt.Sprintf("- %s\n", evidence))
               }
               body.WriteString("\n")
           }
       } else {
           // Fallback if approval context not populated
           body.WriteString(fmt.Sprintf("### AI Analysis\n"))
           body.WriteString(fmt.Sprintf("**Root Cause**: %s\n", aiAnalysis.Status.RootCause))
           body.WriteString(fmt.Sprintf("**Confidence**: %.1f%%\n", aiAnalysis.Status.Confidence*100))
           body.WriteString(fmt.Sprintf("**Recommended Action**: %s\n\n", aiAnalysis.Status.RecommendedAction))
       }

       // Approval instructions
       body.WriteString(fmt.Sprintf("### How to Approve/Reject\n"))
       body.WriteString(fmt.Sprintf("```bash\n"))
       body.WriteString(fmt.Sprintf("# Approve\n"))
       body.WriteString(fmt.Sprintf("kubectl patch aiapprovalrequest %s -n %s --type=merge -p '{\"spec\":{\"decision\":\"Approved\",\"decidedBy\":\"operator@company.com\",\"justification\":\"Approved for execution\"}}'\n\n",
           aiAnalysis.Status.ApprovalRequestName,
           remediation.Namespace))
       body.WriteString(fmt.Sprintf("# Reject\n"))
       body.WriteString(fmt.Sprintf("kubectl patch aiapprovalrequest %s -n %s --type=merge -p '{\"spec\":{\"decision\":\"Rejected\",\"decidedBy\":\"operator@company.com\",\"justification\":\"Risk too high\"}}'\n",
           aiAnalysis.Status.ApprovalRequestName,
           remediation.Namespace))
       body.WriteString(fmt.Sprintf("```\n\n"))

       // Metadata
       body.WriteString(fmt.Sprintf("---\n"))
       body.WriteString(fmt.Sprintf("**Remediation Request**: `%s`\n", remediation.Name))
       body.WriteString(fmt.Sprintf("**AIAnalysis**: `%s`\n", aiAnalysis.Name))
       body.WriteString(fmt.Sprintf("**Approval Request**: `%s`\n", aiAnalysis.Status.ApprovalRequestName))
       body.WriteString(fmt.Sprintf("**Requested At**: %s\n",
           aiAnalysis.Status.ApprovalRequestedAt.Format("2006-01-02 15:04:05 MST")))

       return body.String()
   }
   ```

4. **Test Scenarios**:
   - Unit test: Verify notification created when `phase="Approving"`
   - Unit test: Verify notification not created twice (idempotency)
   - Unit test: Verify notification body formatted correctly
   - Integration test: Verify notification delivered to Slack (mock webhook)
   - Integration test: Verify `approvalNotificationSent` status updated
   - E2E test: Operator approves → AIAnalysis status updated → Workflow proceeds

**Effort**: 2 hours (watch extension + notification logic)

---

### **Update 4: Add to RBAC Section**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - RBAC Configuration section

**Add to existing RBAC markers**:

```go
// Approval notification integration (BR-ORCH-001)
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses,verbs=get;list;watch
// +kubebuilder:rbac:groups=aianalysis.kubernaut.io,resources=aianalyses/status,verbs=get
// +kubebuilder:rbac:groups=notification.kubernaut.io,resources=notificationrequests,verbs=create;get;list;watch
// +kubebuilder:rbac:groups=notification.kubernaut.io,resources=notificationrequests/status,verbs=get
```

---

### **Update 5: Add CRD Schema Update**

**Location**: `api/remediation/v1alpha1/remediationrequest_types.go`

**Add to RemediationRequestStatus**:

```go
type RemediationRequestStatus struct {
    // ... existing fields ...

    // Approval notification tracking (BR-ORCH-001)
    // Prevents duplicate notifications when AIAnalysis requires approval
    ApprovalNotificationSent bool `json:"approvalNotificationSent,omitempty"`
}
```

**Note**: This CRD schema change will require `make generate` and `make manifests` regeneration.

---

### **Update 6: Add to Integration Testing**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Integration Testing section

**Add Test Scenario**:

```markdown
#### **Integration Test 5: Approval Notification Creation**

**Scenario**: Verify notification created when AIAnalysis requires approval

**Setup**:
- Create RemediationRequest CRD
- Mock HolmesGPT to return medium confidence (65%)
- Wait for AIAnalysis to enter "Approving" phase

**Test Steps**:
1. Verify AIAnalysis `phase = "Approving"` and `requiresApproval = true`
2. Verify NotificationRequest CRD created with name `approval-notification-{rr}-{ai}`
3. Verify notification has:
   - Type: `escalation`
   - Priority: `high`
   - Channels: `[slack, console]`
   - Subject: "🚨 Approval Required: {SignalName}"
   - Body contains: approval context, recommended actions, approval instructions
4. Verify `status.approvalNotificationSent = true` in RemediationRequest
5. Trigger reconcile again → Verify NO duplicate notification created

**Validation**:
```go
It("should create approval notification when AIAnalysis requires approval", func() {
    // Wait for AIAnalysis to require approval
    Eventually(func() string {
        aiAnalysis := &aianalysisv1alpha1.AIAnalysis{}
        k8sClient.Get(ctx, aiAnalysisKey, aiAnalysis)
        return aiAnalysis.Status.Phase
    }, timeout, interval).Should(Equal("Approving"))

    // Verify NotificationRequest created
    notificationKey := types.NamespacedName{
        Name:      fmt.Sprintf("approval-notification-%s-%s", rrName, aiName),
        Namespace: namespace,
    }
    notification := &notificationv1alpha1.NotificationRequest{}
    Eventually(func() error {
        return k8sClient.Get(ctx, notificationKey, notification)
    }, timeout, interval).Should(Succeed())

    // Verify notification details
    Expect(notification.Spec.Type).To(Equal(notificationv1alpha1.NotificationTypeEscalation))
    Expect(notification.Spec.Priority).To(Equal(notificationv1alpha1.NotificationPriorityHigh))
    Expect(notification.Spec.Channels).To(ContainElements(
        notificationv1alpha1.ChannelSlack,
        notificationv1alpha1.ChannelConsole,
    ))
    Expect(notification.Spec.Subject).To(ContainSubstring("Approval Required"))
    Expect(notification.Spec.Body).To(ContainSubstring("AI Investigation Summary"))
    Expect(notification.Spec.Body).To(ContainSubstring("Recommended Actions"))
    Expect(notification.Spec.Body).To(ContainSubstring("How to Approve/Reject"))

    // Verify status updated
    remediation := &remediationv1alpha1.RemediationRequest{}
    Eventually(func() bool {
        k8sClient.Get(ctx, remediationKey, remediation)
        return remediation.Status.ApprovalNotificationSent
    }, timeout, interval).Should(BeTrue())

    // Trigger reconcile again
    remediation.Annotations["test"] = "trigger-reconcile"
    k8sClient.Update(ctx, remediation)

    // Verify no duplicate notification
    notifications := &notificationv1alpha1.NotificationRequestList{}
    k8sClient.List(ctx, notifications, client.InNamespace(namespace))
    approvalNotifications := filterApprovalNotifications(notifications.Items)
    Expect(approvalNotifications).To(HaveLen(1), "Should only have 1 approval notification")
})
```

---

### **Update 7: Version Bump & Changelog**

**Location**: `IMPLEMENTATION_PLAN_V1.0.md` - Version History section (top of file)

**Proposed Version**: **v1.0.3** - Approval Notification Integration Documentation

**Add to Version History**:

```markdown
- **v1.0.3** (2025-10-20): 📋 **Approval Notification Integration Documented**
  - **BR-ORCH-001**: RemediationOrchestrator approval notification creation
    - Watch AIAnalysis for `phase="Approving"` and create NotificationRequest
    - Extract approval context from `AIAnalysis.status.approvalContext` (BR-AI-059)
    - Format rich notification body with investigation summary, evidence, alternatives
    - Track `status.approvalNotificationSent` to prevent duplicate notifications
    - Apply to Day 11.5 (new: Approval Notification Creation, +2h)
  - **CRD Schema Update**: Add `approvalNotificationSent` bool to RemediationRequestStatus
  - **RBAC Update**: Add AIAnalysis read permissions for approval monitoring
  - **Documentation**: [APPROVAL_NOTIFICATION_DOCUMENTATION_PLAN.md](./APPROVAL_NOTIFICATION_DOCUMENTATION_PLAN.md)
  - **Timeline**: Day 11 extended from 8h → 10h (total: 16-18 days)
  - **Confidence**: 95% (no change - straightforward extension)
  - **Expected Impact**: Approval timeout rate -50% (push notifications enable faster decisions)
```

---

## 📊 **Impact Summary**

### **Timeline Impact**:
- **Day 11 Extended**: 8h → 10h (+2 hours for approval notification logic)
- **Total Timeline**: 14-16 days → **16-18 days** (includes all extensions)

### **Effort Breakdown**:
| Task | Effort | Notes |
|---|---|---|
| Day 11.5: Watch AIAnalysis for Approvals | 0.5h | Extend existing watch logic |
| Day 11.5: Create Approval Notification | 1h | Notification CRD creation |
| Day 11.5: Format Approval Body | 0.5h | Template with rich context |
| Integration Testing Updates | 0.5h | Add approval notification test |
| CRD Schema Update | Minimal | Add 1 bool field |
| **Total** | **2.5h** | Rounded to 2h in timeline |

### **Confidence**:
- **95%** - Straightforward extension of existing notification logic
- Reuses existing NotificationRequest CRD (no new infrastructure)
- Leverages existing AIAnalysis watch patterns
- Similar to existing escalation notification logic

---

## ✅ **Next Steps (When Ready for Implementation)**

1. **Review** this documentation plan
2. **Update** `IMPLEMENTATION_PLAN_V1.0.md` with above changes
3. **Bump version** to v1.0.3 with changelog
4. **Update** `api/remediation/v1alpha1/remediationrequest_types.go` (add `approvalNotificationSent` field)
5. **Regenerate CRDs** with `make generate` and `make manifests`
6. **Implement** when RemediationOrchestrator controller development begins

---

## 📚 **References**

- [ADR-018: Approval Notification V1.0 Integration](../../../../architecture/decisions/ADR-018-approval-notification-v1-integration.md)
- [BR-ORCH-001: RemediationOrchestrator Notification Creation](../../../../requirements/APPROVAL_NOTIFICATION_BUSINESS_REQUIREMENTS.md)
- [AIAnalysis Approval Context Plan](../../02-aianalysis/implementation/APPROVAL_CONTEXT_DOCUMENTATION_PLAN.md) - Related planning
- [RemediationOrchestrator Implementation Plan v1.0.2](./IMPLEMENTATION_PLAN_V1.0.md) - Base plan to extend
- [NOTIFICATION_INTEGRATION_PLAN.md](../NOTIFICATION_INTEGRATION_PLAN.md) - Existing notification integration (completion/failure)

