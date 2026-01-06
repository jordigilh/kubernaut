# Webhook Audit Test Plan Update - TDD Approach

**Date**: 2026-01-05  
**Objective**: Add audit event validation to WorkflowExecution and RemediationApprovalRequest integration tests  
**Related**: DD-WEBHOOK-003 (Webhook-Complete Audit Pattern)

---

## 🎯 **TDD Approach: RED → GREEN**

### **Phase 1: Update Test Plan (Documentation)**
Add audit event validation scenarios to `WEBHOOK_TEST_PLAN.md`

### **Phase 2: Update Test Infrastructure (RED Phase)**
1. Modify `suite_test.go` to pass `mockAuditMgr` to all handlers
2. Update integration tests to assert audit events
3. Run tests → **Expected: FAIL** (audit events not written yet)

### **Phase 3: Implement Audit Events (GREEN Phase)**
1. Add `auditManager` field to webhook handlers
2. Write audit events in `Handle()` methods
3. Run tests → **Expected: PASS** (9/9 passing)

---

## 📋 **Phase 1: Test Plan Updates**

### **File**: `docs/development/SOC2/WEBHOOK_TEST_PLAN.md`

#### **Section to Update**: Integration Test Scenarios

**Add to INT-WE-01** (WorkflowExecution Block Clearance):

```markdown
#### **Audit Event Validation** (NEW)

**Scenario**: Webhook writes complete audit event for block clearance

**Test Steps**:
1. Operator clears workflow execution block
2. Query mock audit manager events
3. Verify audit event exists with:
   - `EventType`: "workflowexecution.block.cleared"
   - `ActorID`: "admin" (from K8s UserInfo)
   - `EventData`: Contains clear_reason, previous_phase, new_phase
   - `EventCategory`: "workflow"
   - `EventOutcome`: "success"

**Expected Result**:
- ✅ Audit event written within 5 seconds
- ✅ Event contains WHO (ActorID) + WHAT (clear_reason) + ACTION (phase change)
- ✅ Event data matches CRD state

**Validation**:
```go
Eventually(func() int {
    return len(mockAuditMgr.events)
}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1))

var clearanceEvent *audit.AuditEvent
for _, e := range mockAuditMgr.events {
    if e.EventType == "workflowexecution.block.cleared" {
        clearanceEvent = &e
        break
    }
}
Expect(clearanceEvent).ToNot(BeNil())
Expect(clearanceEvent.ActorID).To(Equal("admin"))
Expect(clearanceEvent.EventData).To(ContainSubstring("clear_reason"))
```

**Business Value**: SOC2 CC8.1 compliance - complete audit trail for operator actions
```

**Add to INT-RAR-01** (RemediationApprovalRequest Approval):

```markdown
#### **Audit Event Validation** (NEW)

**Scenario**: Webhook writes complete audit event for approval decision

**Test Steps**:
1. Operator approves remediation request
2. Query mock audit manager events
3. Verify audit event exists with:
   - `EventType`: "remediationapproval.decision.made"
   - `ActorID`: "admin" (from K8s UserInfo)
   - `EventData`: Contains decision, decision_message, ai_analysis_ref
   - `EventCategory`: "remediation"
   - `EventOutcome`: "success"

**Expected Result**:
- ✅ Audit event written within 5 seconds
- ✅ Event contains WHO (ActorID) + WHAT (decision) + ACTION (approval details)
- ✅ Decision="Approved" captured in event data

**Validation**:
```go
Eventually(func() int {
    return len(mockAuditMgr.events)
}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1))

var approvalEvent *audit.AuditEvent
for _, e := range mockAuditMgr.events {
    if e.EventType == "remediationapproval.decision.made" {
        approvalEvent = &e
        break
    }
}
Expect(approvalEvent).ToNot(BeNil())
Expect(approvalEvent.ActorID).To(Equal("admin"))
Expect(approvalEvent.EventData).To(ContainSubstring("Approved"))
```

**Business Value**: SOC2 CC8.1 compliance - operator attribution for approval decisions
```

**Add to INT-RAR-02** (RemediationApprovalRequest Rejection):

```markdown
#### **Audit Event Validation** (NEW)

**Scenario**: Webhook writes complete audit event for rejection decision

**Test Steps**:
1. Operator rejects remediation request
2. Query mock audit manager events
3. Verify audit event exists with:
   - `EventType`: "remediationapproval.decision.made"
   - `ActorID`: "admin" (from K8s UserInfo)
   - `EventData`: Contains decision="Rejected", decision_message
   - `EventCategory`: "remediation"
   - `EventOutcome`: "success"

**Expected Result**:
- ✅ Audit event written for rejection
- ✅ Event contains WHO (ActorID) + WHAT (decision) + ACTION (rejection details)
- ✅ Decision="Rejected" captured in event data

**Validation**: Same pattern as INT-RAR-01, verify `decision="Rejected"` in event data

**Business Value**: SOC2 CC8.1 compliance - operator attribution for rejection decisions
```

---

## 📋 **Phase 2: Test Infrastructure Updates (RED Phase)**

### **File**: `test/integration/authwebhook/suite_test.go`

#### **Change 1: Pass mockAuditMgr to all webhook handlers**

**Current Code** (lines ~120-135):
```go
// Register WorkflowExecution mutating webhook
wfeHandler := webhooks.NewWorkflowExecutionAuthHandler()
_ = wfeHandler.InjectDecoder(decoder)
webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})

// Register RemediationApprovalRequest mutating webhook
rarHandler := webhooks.NewRemediationApprovalRequestAuthHandler()
_ = rarHandler.InjectDecoder(decoder)
webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})

// Register NotificationRequest mutating webhook for DELETE
mockAuditMgr = &mockAuditManager{events: []audit.AuditEvent{}}
nrHandler := webhooks.NewNotificationRequestDeleteHandler(mockAuditMgr)
```

**Updated Code** (RED Phase):
```go
// Initialize mock audit manager ONCE for all handlers
mockAuditMgr = &mockAuditManager{events: []audit.AuditEvent{}}

// Register WorkflowExecution mutating webhook (with audit manager)
wfeHandler := webhooks.NewWorkflowExecutionAuthHandler(mockAuditMgr)
_ = wfeHandler.InjectDecoder(decoder)
webhookServer.Register("/mutate-workflowexecution", &webhook.Admission{Handler: wfeHandler})

// Register RemediationApprovalRequest mutating webhook (with audit manager)
rarHandler := webhooks.NewRemediationApprovalRequestAuthHandler(mockAuditMgr)
_ = rarHandler.InjectDecoder(decoder)
webhookServer.Register("/mutate-remediationapprovalrequest", &webhook.Admission{Handler: rarHandler})

// Register NotificationRequest mutating webhook for DELETE
nrHandler := webhooks.NewNotificationRequestDeleteHandler(mockAuditMgr)
```

---

### **File**: `test/integration/authwebhook/workflowexecution_test.go`

#### **Change 1: Add BeforeEach audit cleanup**

**Add after line 49**:
```go
BeforeEach(func() {
    ctx = context.Background()
    namespace = "default"
    mockAuditMgr.events = []audit.AuditEvent{} // Clear audit events before each test
})
```

#### **Change 2: Add audit event assertions to INT-WE-01**

**Add after line 121** (after status field verification):
```go
By("Verifying webhook recorded complete audit event (side effect)")
// Per DD-WEBHOOK-003: Webhook writes complete audit event (WHO + WHAT + ACTION)
Eventually(func() int {
    return len(mockAuditMgr.events)
}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
    "Webhook should record audit event for block clearance")

// Verify audit event content
var clearanceEvent *audit.AuditEvent
for _, e := range mockAuditMgr.events {
    if e.EventType == "workflowexecution.block.cleared" &&
       e.ActorID == "admin" {
        clearanceEvent = &e
        break
    }
}
Expect(clearanceEvent).ToNot(BeNil(),
    "Audit event for block clearance should exist")
Expect(clearanceEvent.EventCategory).To(Equal(audit.EventCategoryWorkflow),
    "Event category should be 'workflow'")
Expect(clearanceEvent.EventOutcome).To(Equal(audit.OutcomeSuccess),
    "Event outcome should be 'success'")

// Verify event data completeness (WHO + WHAT + ACTION)
eventDataStr := string(clearanceEvent.EventData)
Expect(eventDataStr).To(ContainSubstring(wfe.Name),
    "Event data should contain workflow name")
Expect(eventDataStr).To(ContainSubstring("Integration test clearance"),
    "Event data should contain clear reason")

GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
GinkgoWriter.Printf("✅ INT-WE-01 PASSED: Block Clearance Attribution + Audit\n")
GinkgoWriter.Printf("   • Cleared by: %s\n", wfe.Status.BlockClearance.ClearedBy)
GinkgoWriter.Printf("   • Audit event: %s\n", clearanceEvent.EventType)
GinkgoWriter.Printf("   • Actor: %s\n", clearanceEvent.ActorID)
GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
```

---

### **File**: `test/integration/authwebhook/remediationapprovalrequest_test.go`

#### **Change 1: Add BeforeEach audit cleanup**

**Add after line 49**:
```go
BeforeEach(func() {
    ctx = context.Background()
    namespace = "default"
    mockAuditMgr.events = []audit.AuditEvent{} // Clear audit events before each test
})
```

#### **Change 2: Add audit event assertions to INT-RAR-01 (Approval)**

**Add after line 93** (after status field verification):
```go
By("Verifying webhook recorded complete audit event (side effect)")
// Per DD-WEBHOOK-003: Webhook writes complete audit event (WHO + WHAT + ACTION)
Eventually(func() int {
    return len(mockAuditMgr.events)
}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
    "Webhook should record audit event for approval decision")

// Verify audit event content
var approvalEvent *audit.AuditEvent
for _, e := range mockAuditMgr.events {
    if e.EventType == "remediationapproval.decision.made" &&
       e.ActorID == "admin" {
        approvalEvent = &e
        break
    }
}
Expect(approvalEvent).ToNot(BeNil(),
    "Audit event for approval decision should exist")
Expect(approvalEvent.EventCategory).To(Equal(audit.EventCategoryRemediation),
    "Event category should be 'remediation'")
Expect(approvalEvent.EventOutcome).To(Equal(audit.OutcomeSuccess),
    "Event outcome should be 'success'")

// Verify event data completeness (WHO + WHAT + ACTION)
eventDataStr := string(approvalEvent.EventData)
Expect(eventDataStr).To(ContainSubstring("Approved"),
    "Event data should contain decision 'Approved'")
Expect(eventDataStr).To(ContainSubstring(rar.Name),
    "Event data should contain request name")

GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
GinkgoWriter.Printf("✅ INT-RAR-01 PASSED: Approval Attribution + Audit\n")
GinkgoWriter.Printf("   • Decided by: %s\n", rar.Status.DecidedBy)
GinkgoWriter.Printf("   • Audit event: %s\n", approvalEvent.EventType)
GinkgoWriter.Printf("   • Decision: Approved\n")
GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
```

#### **Change 3: Add audit event assertions to INT-RAR-02 (Rejection)**

**Add after line 161** (after status field verification):
```go
By("Verifying webhook recorded complete audit event for rejection")
Eventually(func() int {
    return len(mockAuditMgr.events)
}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 1),
    "Webhook should record audit event for rejection decision")

// Verify audit event content for rejection
var rejectionEvent *audit.AuditEvent
for _, e := range mockAuditMgr.events {
    if e.EventType == "remediationapproval.decision.made" &&
       e.ActorID == "admin" {
        rejectionEvent = &e
        break
    }
}
Expect(rejectionEvent).ToNot(BeNil(),
    "Audit event for rejection decision should exist")

// Verify event data contains rejection
eventDataStr := string(rejectionEvent.EventData)
Expect(eventDataStr).To(ContainSubstring("Rejected"),
    "Event data should contain decision 'Rejected'")

GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
GinkgoWriter.Printf("✅ INT-RAR-02 PASSED: Rejection Attribution + Audit\n")
GinkgoWriter.Printf("   • Decided by: %s\n", rar.Status.DecidedBy)
GinkgoWriter.Printf("   • Audit event: %s\n", rejectionEvent.EventType)
GinkgoWriter.Printf("   • Decision: Rejected\n")
GinkgoWriter.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
```

---

## 📋 **Phase 3: Webhook Implementation (GREEN Phase)**

### **File**: `pkg/webhooks/workflowexecution_handler.go`

#### **Change 1: Add auditManager field**

```go
type WorkflowExecutionAuthHandler struct {
    authenticator *authwebhook.Authenticator
    decoder       admission.Decoder
    auditManager  audit.Manager  // NEW: Inject audit manager
}

func NewWorkflowExecutionAuthHandler(auditManager audit.Manager) *WorkflowExecutionAuthHandler {
    return &WorkflowExecutionAuthHandler{
        authenticator: authwebhook.NewAuthenticator(),
        auditManager:  auditManager,  // NEW
    }
}
```

#### **Change 2: Write audit event in Handle() method**

**Add after extracting authCtx (line ~60)**:
```go
// Decode OldObject to compare previous state
oldWFE := &workflowexecutionv1.WorkflowExecution{}
if len(req.OldObject.Raw) > 0 {
    _ = json.Unmarshal(req.OldObject.Raw, oldWFE)
}

// Write complete audit event (WHO + WHAT + ACTION)
eventData := map[string]interface{}{
    "workflow_name":    wfe.Name,
    "namespace":        wfe.Namespace,
    "clear_reason":     wfe.Status.BlockClearance.ClearReason,
    "clear_method":     wfe.Status.BlockClearance.ClearMethod,
    "previous_phase":   oldWFE.Status.Phase,
    "new_phase":        wfe.Status.Phase,
    "correlation_id":   wfe.Spec.RemediationRequestRef.Name,
}

eventDataJSON, _ := json.Marshal(eventData)
h.auditManager.RecordEvent(ctx, audit.Event{
    EventID:           audit.NewEventID(),
    EventVersion:      "1.0",
    EventTimestamp:    time.Now().UTC(),
    EventType:         "workflowexecution.block.cleared",
    EventCategory:     audit.EventCategoryWorkflow,
    EventAction:       "update",
    EventOutcome:      audit.OutcomeSuccess,
    ActorID:           authCtx.Username,
    ActorType:         audit.ActorTypeUser,
    CorrelationID:     wfe.Spec.RemediationRequestRef.Name,
    ResourceID:        wfe.Name,
    ResourceType:      "WorkflowExecution",
    ResourceName:      wfe.Name,
    ResourceNamespace: wfe.Namespace,
    EventData:         eventDataJSON,
})
```

---

### **File**: `pkg/webhooks/remediationapprovalrequest_handler.go`

#### **Change 1: Add auditManager field**

```go
type RemediationApprovalRequestAuthHandler struct {
    authenticator *authwebhook.Authenticator
    decoder       admission.Decoder
    auditManager  audit.Manager  // NEW: Inject audit manager
}

func NewRemediationApprovalRequestAuthHandler(auditManager audit.Manager) *RemediationApprovalRequestAuthHandler {
    return &RemediationApprovalRequestAuthHandler{
        authenticator: authwebhook.NewAuthenticator(),
        auditManager:  auditManager,  // NEW
    }
}
```

#### **Change 2: Write audit event in Handle() method**

**Add after extracting authCtx (line ~60)**:
```go
// Write complete audit event (WHO + WHAT + ACTION)
eventData := map[string]interface{}{
    "request_name":      rar.Name,
    "namespace":         rar.Namespace,
    "decision":          string(rar.Status.Decision),
    "decision_message":  rar.Status.DecisionMessage,
    "ai_analysis_ref":   rar.Spec.AIAnalysisRef.Name,
    "confidence":        rar.Spec.Confidence,
    "correlation_id":    rar.Spec.RemediationRequestRef.Name,
}

eventDataJSON, _ := json.Marshal(eventData)
h.auditManager.RecordEvent(ctx, audit.Event{
    EventID:           audit.NewEventID(),
    EventVersion:      "1.0",
    EventTimestamp:    time.Now().UTC(),
    EventType:         "remediationapproval.decision.made",
    EventCategory:     audit.EventCategoryRemediation,
    EventAction:       "update",
    EventOutcome:      audit.OutcomeSuccess,
    ActorID:           authCtx.Username,
    ActorType:         audit.ActorTypeUser,
    CorrelationID:     rar.Spec.RemediationRequestRef.Name,
    ResourceID:        rar.Name,
    ResourceType:      "RemediationApprovalRequest",
    ResourceName:      rar.Name,
    ResourceNamespace: rar.Namespace,
    EventData:         eventDataJSON,
})
```

---

## ✅ **Validation**

### **After Phase 2 (RED Phase)**
```bash
make test-integration-authwebhook
# Expected: 6/9 FAIL (audit assertions fail)
# - INT-WE-01: FAIL (no audit event)
# - INT-WE-02: PASS (validation only)
# - INT-WE-03: PASS (validation only)
# - INT-RAR-01: FAIL (no audit event)
# - INT-RAR-02: FAIL (no audit event)
# - INT-RAR-03: PASS (validation only)
# - INT-NR-01: PASS (already implemented)
# - INT-NR-02: PASS (no audit expected)
# - INT-NR-03: PASS (already implemented)
```

### **After Phase 3 (GREEN Phase)**
```bash
make test-integration-authwebhook
# Expected: 9/9 PASS
# All integration tests passing with audit validation
```

---

## 📊 **Success Metrics**

- ✅ Test plan updated with audit validation scenarios
- ✅ All 3 webhook handlers receive audit manager
- ✅ All UPDATE operations validate audit events
- ✅ 9/9 integration tests passing
- ✅ Audit events contain WHO + WHAT + ACTION
- ✅ Status fields still populated (MANDATORY)

---

## 🎯 **Implementation Order**

1. **Update WEBHOOK_TEST_PLAN.md** (documentation)
2. **Update suite_test.go** (infrastructure - pass mockAuditMgr)
3. **Update workflowexecution_test.go** (TDD RED - add assertions)
4. **Update remediationapprovalrequest_test.go** (TDD RED - add assertions)
5. **Run tests** → Expect 6/9 FAIL
6. **Update workflowexecution_handler.go** (TDD GREEN - write audit events)
7. **Update remediationapprovalrequest_handler.go** (TDD GREEN - write audit events)
8. **Run tests** → Expect 9/9 PASS

---

**Estimated Time**: 4 hours  
**Priority**: HIGH (SOC2 compliance requirement)  
**Related**: DD-WEBHOOK-003, BR-AUTH-001, webhook-audit-next-steps.md  

