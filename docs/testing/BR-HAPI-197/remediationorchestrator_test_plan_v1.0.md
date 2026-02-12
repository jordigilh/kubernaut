# RemediationOrchestrator Test Plan for BR-HAPI-197

**Business Requirement**: BR-HAPI-197 - Human Review Required Flag
**Service**: RemediationOrchestrator (RO)
**Version**: 1.0
**Date**: January 20, 2026
**Status**: 🔄 DRAFT (for user review)

---

## 📋 **Test Plan Overview**

### **Scope**
This test plan covers RemediationOrchestrator service's handling of AIAnalysis `needsHumanReview` flag:
- **RO Decision Logic**: Check `needsHumanReview` flag before creating WorkflowExecution
- **NotificationRequest Creation**: Create notification when `needsHumanReview=true`
- **Two-Flag Architecture**: Distinguish `needsHumanReview` (HAPI decision) vs `approvalRequired` (Rego decision)

### **Services Under Test**
1. **RemediationOrchestrator**: AIAnalysis handler, routing logic, CRD orchestration
2. **Integration Points**:
   - AIAnalysis status reading (DD-CONTRACT-002)
   - NotificationRequest creation
   - RemediationRequest status updates

### **Out of Scope**
- ❌ AIAnalysis logic for setting `needsHumanReview` (covered by AIAnalysis test plan)
- ❌ HAPI logic for determining `needs_human_review` (covered by HAPI test plan)
- ❌ NotificationRequest delivery (covered by Notification service test plan)

---

## 🎯 **Defense-in-Depth Coverage**

Following `TESTING_GUIDELINES.md` defense-in-depth strategy:

| Tier | BR Coverage | Code Coverage | Focus | Test Count |
|------|-------------|---------------|-------|------------|
| **Unit** | 70%+ | 70%+ | Handler logic, routing decisions, flag precedence | 6 scenarios |
| **Integration** | 50%+ | 50% | CRD orchestration, NotificationRequest creation, audit events | 3 scenarios |
| **E2E** | <10% | 50% | Full remediation flow with human review gate | 2 scenarios |

---

## 📦 **OpenAPI Dependencies & Type Safety**

### **✅ HolmesGPT Go Client - Type-Safe Enum Constants Available**

**Status**: ✅ **READY** - Go client regenerated with BR-HAPI-197 fields

**Generated Types** (`pkg/holmesgpt/client/oas_schemas_gen.go`):
```go
type IncidentResponse struct {
    // ... existing fields ...

    // BR-HAPI-197 fields
    NeedsHumanReview     OptBool                   `json:"needs_human_review"`
    HumanReviewReason    OptNilHumanReviewReason  `json:"human_review_reason"`

    // ... other fields ...
}

// Enum type
type HumanReviewReason string

// Constants (from holmesgpt-api/src/models/incident_models.py)
const (
    HumanReviewReasonWorkflowNotFound          HumanReviewReason = "workflow_not_found"
    HumanReviewReasonImageMismatch             HumanReviewReason = "image_mismatch"
    HumanReviewReasonParameterValidationFailed HumanReviewReason = "parameter_validation_failed"
    HumanReviewReasonNoMatchingWorkflows       HumanReviewReason = "no_matching_workflows"
    HumanReviewReasonLowConfidence             HumanReviewReason = "low_confidence"
    HumanReviewReasonLlmParsingError           HumanReviewReason = "llm_parsing_error"
    HumanReviewReasonInvestigationInconclusive HumanReviewReason = "investigation_inconclusive"
)
```

**Test Import Path**:
```go
import (
    holmesgpt "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)
```

**Test Usage** (Type-safe):
```go
// ✅ CORRECT: Use OpenAPI constants
humanReviewReason := string(holmesgpt.HumanReviewReasonWorkflowNotFound)

// ❌ INCORRECT: Hardcoded strings (typo-prone)
humanReviewReason := "workflow_not_found"
```

**Rationale**: Ensures type safety, prevents typos, and maintains consistency with authoritative Python models.

### **DataStorage Audit Payload Extension Required**

**File**: `api/openapi/data-storage-v1.yaml`

**Update Required**: Extend `RemediationOrchestratorAuditPayload` schema to include:
```yaml
RemediationOrchestratorAuditPayload:
  properties:
    # ... existing fields ...

    # Human Review Routing Fields (NEW - BR-HAPI-197)
    human_review_reason:
      type: string
      enum:
        - workflow_not_found
        - no_matching_workflows
        - low_confidence
        - llm_parsing_error
        - parameter_validation_failed
        - image_mismatch
        - investigation_inconclusive
      description: Structured reason why human review is required (from HAPI)
    route_type:
      type: string
      enum: [human_review, approval, automatic, blocked]
      description: RO routing decision type
```

**Generated Client**: After `make generate`, tests will use typed `datastorage.RemediationOrchestratorAuditPayload` instead of `map[string]interface{}`.

**Example Test Usage**:
```go
// ✅ CORRECT: Typed OpenAPI struct
payload, ok := auditEvent.EventData.(datastorage.RemediationOrchestratorAuditPayload)
Expect(ok).To(BeTrue(), "event_data should be RemediationOrchestratorAuditPayload")
Expect(payload.HumanReviewReason).To(Equal(string(holmesgpt.HumanReviewReasonWorkflowNotFound)))
Expect(payload.RouteType).To(Equal("human_review"))

// ❌ INCORRECT: Unstructured map (anti-pattern)
Expect(auditEvent.EventData).To(HaveKeyWithValue("human_review_reason", "workflow_not_found"))
```

---

## 🧪 **Unit Tests** (BR-HAPI-197 RO Routing Decision Logic)

**Location**: `test/unit/remediationorchestrator/routing/decision_logic_test.go`
**Test Framework**: Ginkgo/Gomega
**Focus**: **Pure decision logic** - no K8s API, no CRD creation, no side effects

**Philosophy** (per `TESTING_GUIDELINES.md` lines 213-260):
- ✅ **Test WHAT decision RO makes** (routing logic)
- ❌ **NOT HOW RO implements it** (CRD creation, K8s API calls)
- ✅ Fast execution (milliseconds)
- ✅ Mock all external dependencies
- ✅ Focus on algorithm correctness and edge cases

**Note**: Unit tests validate **internal algorithms**, not integration with other services. Integration with K8s API, CRD orchestration, and audit events are covered in **Integration Tests** (IT-RO-197-*).

---

### **UT-RO-197-001: Routing decision - needsHumanReview=true**

**Scenario**: RO routing logic determines `needsHumanReview=true` → route to NotificationRequest creation.

**Given**:
- AIAnalysis status with `needsHumanReview=true`, `humanReviewReason="workflow_not_found"`

**When**:
- `RoutingDecision.DetermineRoute(aiAnalysis)` is called

**Then**:
- **Decision output** (struct):
  - `routeType` = `RouteTypeHumanReview`
  - `shouldCreateNotification` = `true`
  - `shouldCreateWorkflowExecution` = `false`
  - `shouldCreateApprovalRequest` = `false`
  - `reason` = `"workflow_not_found"`

**Validation**:
- ✅ Routing decision logic is correct (pure function)
- ✅ No K8s API calls
- ✅ No side effects

**Implementation Hint**:
```go
// test/unit/remediationorchestrator/routing/decision_logic_test.go
Describe("RoutingDecision", func() {
    var decider *RoutingDecider

    BeforeEach(func() {
        // ✅ CORRECT: Mock logger only, no K8s client
        decider = NewRoutingDecider(mockLogger)
    })

    It("should route to NotificationRequest when needsHumanReview=true", func() {
        // ✅ CORRECT: Test input (AIAnalysis status)
        aiAnalysis := &aianalysisv1.AIAnalysis{
            Status: aianalysisv1.AIAnalysisStatus{
                Phase:              "Failed",
                NeedsHumanReview:   true,
                HumanReviewReason:  "workflow_not_found",
            },
        }

        // ✅ CORRECT: Call pure decision logic
        decision := decider.DetermineRoute(aiAnalysis)

        // ✅ CORRECT: Validate DECISION, not side effects
        Expect(decision.RouteType).To(Equal(RouteTypeHumanReview))
        Expect(decision.ShouldCreateNotification).To(BeTrue())
        Expect(decision.ShouldCreateWorkflowExecution).To(BeFalse())
        Expect(decision.ShouldCreateApprovalRequest).To(BeFalse())
        Expect(decision.Reason).To(Equal("workflow_not_found"))

        // ✅ CORRECT: Validate decision metadata
        Expect(decision.NotificationType).To(Equal("human_review_required"))
        Expect(decision.RRStatusPhase).To(Equal("RequiresReview"))
        Expect(decision.RRStatusReason).To(Equal("HumanReviewRequired"))
    })
})
```

**Why This is Correct**:
- ✅ Tests **decision logic** (pure function)
- ✅ No K8s client
- ✅ No CRD creation
- ✅ No side effect verification
- ✅ Fast (milliseconds)

**Anti-Pattern Warnings**:
- ❌ **TESTING SIDE EFFECTS**: Don't verify CRD creation in unit tests
- ✅ **TESTING DECISIONS**: Validate routing decision struct

---

### **UT-RO-197-002: Routing decision - automatic execution path**

**Scenario**: RO routing logic determines `needsHumanReview=false` and `approvalRequired=false` → route to automatic WorkflowExecution.

**Given**:
- AIAnalysis status with:
  - `needsHumanReview=false`
  - `approvalRequired=false`
  - `selectedWorkflow.workflowId="restart-pod-v1"`

**When**:
- `RoutingDecision.DetermineRoute(aiAnalysis)` is called

**Then**:
- **Decision output** (struct):
  - `routeType` = `RouteTypeAutomaticExecution`
  - `shouldCreateNotification` = `false`
  - `shouldCreateWorkflowExecution` = `true`
  - `shouldCreateApprovalRequest` = `false`
  - `selectedWorkflowId` = `"restart-pod-v1"`

**Validation**:
- ✅ Normal remediation path decision is correct
- ✅ WorkflowExecution creation decision made
- ✅ No notification or approval decision

**Implementation Hint**:
```go
It("should route to automatic execution when both flags are false", func() {
    aiAnalysis := &aianalysisv1.AIAnalysis{
        Status: aianalysisv1.AIAnalysisStatus{
            Phase:              "Completed",
            NeedsHumanReview:   false,
            ApprovalRequired:   false,
            SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
                WorkflowId: "restart-pod-v1",
            },
        },
    }

    decider := NewRoutingDecider(mockLogger)
    decision := decider.DetermineRoute(aiAnalysis)

    // ✅ CORRECT: Validate routing decision
    Expect(decision.RouteType).To(Equal(RouteTypeAutomaticExecution))
    Expect(decision.ShouldCreateWorkflowExecution).To(BeTrue())
    Expect(decision.ShouldCreateNotification).To(BeFalse())
    Expect(decision.ShouldCreateApprovalRequest).To(BeFalse())
    Expect(decision.SelectedWorkflowId).To(Equal("restart-pod-v1"))

    // ✅ CORRECT: Validate RR status decision
    Expect(decision.RRStatusPhase).To(Equal("InProgress"))
    Expect(decision.RRStatusReason).To(Equal("WorkflowExecuting"))
})
```

**Why This is Correct**:
- ✅ Tests **decision logic** for automatic execution path
- ✅ No K8s API calls
- ✅ Validates workflow ID is passed through correctly

---

### **UT-RO-197-003: Flag precedence logic - needsHumanReview wins**

**Scenario**: RO routing logic gives `needsHumanReview` precedence over `approvalRequired` when both are true.

**Given**:
- AIAnalysis status with BOTH flags true:
  - `needsHumanReview=true`, `humanReviewReason="low_confidence"`
  - `approvalRequired=true`, `approvalReason="high_risk_action"`
  - `selectedWorkflow.workflowId="delete-pvc-v1"`

**When**:
- `RoutingDecision.DetermineRoute(aiAnalysis)` is called

**Then**:
- **Decision output** (struct):
  - `routeType` = `RouteTypeHumanReview` (NOT `RouteTypeApproval`)
  - `shouldCreateNotification` = `true`
  - `shouldCreateApprovalRequest` = `false`
  - `primaryReason` = `"low_confidence"`
  - `secondaryContext` = includes `"high_risk_action"` (for operator context)

**Validation**:
- ✅ `needsHumanReview` takes precedence (AI reliability > policy)
- ✅ Operator gets full context (both concerns)
- ✅ DD-CONTRACT-002 contract enforced

**Implementation Hint**:
```go
It("should prioritize needsHumanReview over approvalRequired", func() {
    aiAnalysis := &aianalysisv1.AIAnalysis{
        Status: aianalysisv1.AIAnalysisStatus{
            Phase:              "Completed",
            NeedsHumanReview:   true,
            HumanReviewReason:  "low_confidence",
            ApprovalRequired:   true,
            ApprovalReason:     "high_risk_action",
            SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
                WorkflowId: "delete-pvc-v1",
            },
        },
    }

    decider := NewRoutingDecider(mockLogger)
    decision := decider.DetermineRoute(aiAnalysis)

    // ✅ CORRECT: Validate precedence logic
    Expect(decision.RouteType).To(Equal(RouteTypeHumanReview),
        "needsHumanReview should take precedence over approvalRequired")
    Expect(decision.ShouldCreateNotification).To(BeTrue())
    Expect(decision.ShouldCreateApprovalRequest).To(BeFalse(),
        "Approval is moot if AI can't produce reliable result")
    Expect(decision.ShouldCreateWorkflowExecution).To(BeFalse())

    // ✅ CORRECT: Validate context includes both concerns
    Expect(decision.PrimaryReason).To(Equal("low_confidence"))
    Expect(decision.SecondaryContext).To(ContainSubstring("high_risk_action"),
        "Operator should see both AI reliability and policy concerns")
    Expect(decision.WorkflowSuggestion).To(Equal("delete-pvc-v1"),
        "Workflow suggestion should be preserved for operator reference")
})
```

**Why This is Correct**:
- ✅ Tests **flag precedence logic** (pure algorithm)
- ✅ Validates decision struct fields
- ✅ No side effects

**Anti-Pattern Warnings**:
- ❌ **WRONG PRECEDENCE**: Checking `approvalRequired` before `needsHumanReview` is architecturally wrong
- ✅ **CORRECT**: AI reliability issues (`needsHumanReview`) must be resolved before policy decisions (`approvalRequired`)

---

### **UT-RO-197-004: Edge case - needsHumanReview with Phase=Completed**

**Scenario**: RO routing logic honors `needsHumanReview` flag even when `phase="Completed"` (HAPI completed but flagged as unreliable).

**Given**:
- AIAnalysis status with edge case combination:
  - `phase="Completed"` (HAPI analysis completed)
  - `needsHumanReview=true` (but result unreliable)
  - `humanReviewReason="low_confidence"`
  - `selectedWorkflow.workflowId="restart-pod-v1"`
  - `confidence=0.45`

**When**:
- `RoutingDecision.DetermineRoute(aiAnalysis)` is called

**Then**:
- **Decision output** (struct):
  - `routeType` = `RouteTypeHumanReview` (flag overrides phase)
  - `shouldCreateNotification` = `true`
  - `shouldCreateWorkflowExecution` = `false`
  - `workflowSuggestion` = `"restart-pod-v1"` (for operator reference)

**Validation**:
- ✅ `needsHumanReview` flag is authoritative (overrides phase)
- ✅ Edge case handled correctly
- ✅ Workflow suggestion preserved for operator

**Implementation Hint**:
```go
It("should route to human review even when phase=Completed if needsHumanReview=true", func() {
    aiAnalysis := &aianalysisv1.AIAnalysis{
        Status: aianalysisv1.AIAnalysisStatus{
            Phase:              "Completed",
            NeedsHumanReview:   true,
            HumanReviewReason:  "low_confidence",
            SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
                WorkflowId: "restart-pod-v1",
            },
            Confidence: 0.45,
        },
    }

    decider := NewRoutingDecider(mockLogger)
    decision := decider.DetermineRoute(aiAnalysis)

    // ✅ CORRECT: Validate flag overrides phase
    Expect(decision.RouteType).To(Equal(RouteTypeHumanReview),
        "needsHumanReview flag should override phase-based routing")
    Expect(decision.ShouldCreateNotification).To(BeTrue())
    Expect(decision.ShouldCreateWorkflowExecution).To(BeFalse(),
        "Low confidence blocks automatic execution")

    // ✅ CORRECT: Workflow suggestion preserved
    Expect(decision.WorkflowSuggestion).To(Equal("restart-pod-v1"),
        "Operator should see workflow suggestion for reference")
    Expect(decision.ConfidenceScore).To(Equal(0.45))
})
```

**Why This is Correct**:
- ✅ Tests **edge case logic** (flag precedence over phase)
- ✅ Validates algorithm correctness
- ✅ No side effects

---

### **UT-RO-197-005: Audit event data structure**

**Scenario**: RO routing logic prepares correct audit event data structure for `needsHumanReview` routing decisions.

**Given**:
- AIAnalysis status with `needsHumanReview=true`, `humanReviewReason="workflow_not_found"`
- RemediationRequest UID `"test-uid-123"`

**When**:
- `AuditEventBuilder.BuildHumanReviewAuditEvent(decision, rr)` is called

**Then**:
- **Audit event data** (struct):
  - `event_type` = `"orchestrator.routing.human_review"`
  - `event_action` = `"notification_created"`
  - `correlation_id` = `"test-uid-123"`
  - `event_data` (typed `RemediationOrchestratorAuditPayload`):
    - `HumanReviewReason` = `string(holmesgpt.HumanReviewReasonWorkflowNotFound)` (OpenAPI constant)
    - `RouteType` = `"human_review"`

**Validation**:
- ✅ Audit event structure is correct
- ✅ All required fields populated
- ✅ Compliance trail data complete

**Implementation Hint**:
```go
// test/unit/remediationorchestrator/audit/event_builder_test.go
Describe("AuditEventBuilder", func() {
    var builder *AuditEventBuilder

    BeforeEach(func() {
        builder = NewAuditEventBuilder()
    })

    It("should build correct audit event for human review routing", func() {
        decision := &RoutingDecision{
            RouteType: RouteTypeHumanReview,
            Reason:    "workflow_not_found",
        }

        rr := &remediationv1.RemediationRequest{
            ObjectMeta: metav1.ObjectMeta{
                UID: "test-uid-123",
            },
        }

        // ✅ CORRECT: Test data structure generation (pure function)
        auditEvent := builder.BuildHumanReviewAuditEvent(decision, rr)

        // ✅ CORRECT: Validate audit event structure (typed OpenAPI struct)
        Expect(auditEvent.EventType).To(Equal("orchestrator.routing.human_review"))
        Expect(auditEvent.EventAction).To(Equal("notification_created"))
        Expect(auditEvent.CorrelationId).To(Equal("test-uid-123"))

        // Cast event_data to typed payload (from OpenAPI schema)
        payload, ok := auditEvent.EventData.(datastorage.RemediationOrchestratorAuditPayload)
        Expect(ok).To(BeTrue(), "event_data should be RemediationOrchestratorAuditPayload")
        Expect(payload.HumanReviewReason).To(Equal(string(holmesgpt.HumanReviewReasonWorkflowNotFound)))
        Expect(payload.RouteType).To(Equal("human_review"))
    })
})
```

**Why This is Correct**:
- ✅ Tests **data structure generation** (pure function)
- ✅ No audit client calls (no side effects)
- ✅ Validates audit event fields

**Anti-Pattern Warnings**:
- ❌ **TESTING AUDIT STORAGE**: Don't test DataStorage persistence in unit tests
- ✅ **TESTING DATA STRUCTURE**: Validate audit event struct fields

---

### **UT-RO-197-006: Map all 6 human_review_reason values in notifications**

**Scenario**: RO creates NotificationRequest with correct message for all 6 `human_review_reason` enum values from BR-HAPI-197.2.

**Given** (table-driven test):
| human_review_reason | Expected Message Content |
|--------------------|-------------------------|
| `workflow_not_found` | "Workflow validation failed: workflow not found" |
| `no_workflows_matched` | "No matching workflows found for alert type" |
| `low_confidence` | "AI confidence below threshold" |
| `llm_parsing_error` | "Failed to parse LLM response" |
| `parameter_validation_failed` | "Workflow parameter validation failed" |
| `container_image_mismatch` | "Container image mismatch detected" |

**When**:
- `AIAnalysisHandler.HandleAIAnalysisStatus()` processes each scenario

**Then**:
- NotificationRequest message includes reason-specific explanation
- Operator gets actionable context for each failure type

**Validation**:
- ✅ All BR-HAPI-197 scenarios handled
- ✅ Notification messages are operator-friendly
- ✅ No generic "human review required" messages (always include specific reason)

**Implementation Hint**:
```go
DescribeTable("should create NotificationRequest with reason-specific message",
    func(reason, expectedMessage string) {
        aiAnalysis := &aianalysisv1.AIAnalysis{
            Status: aianalysisv1.AIAnalysisStatus{
                Phase:              "Failed",
                NeedsHumanReview:   true,
                HumanReviewReason:  reason,
            },
        }

        handler := NewAIAnalysisHandler(k8sClient, mockMetrics, mockAudit, mockLogger)
        _ = handler.HandleAIAnalysisStatus(ctx, rr, aiAnalysis)

        notificationList := &notificationv1.NotificationRequestList{}
        Expect(k8sClient.List(ctx, notificationList)).To(Succeed())
        Expect(notificationList.Items).To(HaveLen(1))

        notification := notificationList.Items[0]
        Expect(notification.Spec.Message).To(ContainSubstring(expectedMessage))
    },
    Entry("workflow_not_found", string(holmesgpt.HumanReviewReasonWorkflowNotFound), "workflow not found"),
    Entry("no_workflows_matched", string(holmesgpt.HumanReviewReasonNoMatchingWorkflows), "No matching workflows"),
    Entry("low_confidence", string(holmesgpt.HumanReviewReasonLowConfidence), "confidence below threshold"),
    Entry("llm_parsing_error", string(holmesgpt.HumanReviewReasonLLMParsingError), "parse LLM response"),
    Entry("parameter_validation_failed", string(holmesgpt.HumanReviewReasonParameterValidationFailed), "parameter validation"),
    Entry("container_image_mismatch", string(holmesgpt.HumanReviewReasonImageMismatch), "image mismatch"),
)
```

---

## 🔗 **Integration Tests** (BR-HAPI-197 RO CRD Orchestration)

**Location**: `test/integration/remediationorchestrator/needs_human_review_integration_test.go`
**Test Framework**: Ginkgo/Gomega
**Infrastructure**: envtest + DataStorage container
**Focus**: CRD orchestration, status updates, audit events

---

### **IT-RO-197-001: Full RO reconciliation with needsHumanReview=true**

**Scenario**: RO reconciles RemediationRequest, reads AIAnalysis with `needsHumanReview=true`, creates NotificationRequest, and updates status.

**Given**:
- envtest K8s API running
- DataStorage container running
- RemediationRequest CRD created
- AIAnalysis CRD created with `status.needsHumanReview=true`

**When**:
- RO controller reconciles RemediationRequest

**Then**:
- **RO reads AIAnalysis status** via K8s API
- **NotificationRequest CRD created** with correct spec
- **RemediationRequest status updated**:
  - `status.phase` = `"RequiresReview"`
  - `status.reason` = `"HumanReviewRequired"`
  - `status.conditions` includes "HumanReviewRequired" condition
- **Audit event sent to DataStorage**:
  - `event_type` = `"orchestrator.routing.human_review"`
  - Queryable via DataStorage OpenAPI client

**Validation**:
- ✅ Full reconciliation loop completes successfully
- ✅ CRD orchestration works end-to-end
- ✅ Status updates observable via K8s API
- ✅ Audit event queryable from DataStorage

**Implementation Hint**:
```go
It("should complete full reconciliation with needsHumanReview=true", func() {
    // Step 1: Create RemediationRequest
    rr := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "integ-test-rr",
            Namespace: testNamespace,
        },
        Spec: remediationv1.RemediationRequestSpec{
            TargetResource: remediationv1.TargetResource{
                Kind: "Pod",
                Name: "crashloop-pod",
            },
        },
    }
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())

    // Step 2: Create AIAnalysis with needsHumanReview=true
    analysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      rr.Name + "-aianalysis",
            Namespace: testNamespace,
            Labels: map[string]string{
                "remediation-request": rr.Name,
            },
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            AnalysisRequest: aianalysisv1.AnalysisRequest{
                SignalContext: aianalysisv1.SignalContext{
                    AlertType: "PodCrashLoopBackOff",
                },
            },
        },
        Status: aianalysisv1.AIAnalysisStatus{
            Phase:              "Failed",
            NeedsHumanReview:   true,
            HumanReviewReason:  "workflow_not_found",
        },
    }
    Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

    // Step 3: Trigger RO reconciliation (update RR status to trigger watch)
    rr.Status.Phase = "Pending"
    Expect(k8sClient.Status().Update(ctx, rr)).To(Succeed())

    // Step 4: Wait for NotificationRequest to be created
    Eventually(func() bool {
        notificationList := &notificationv1.NotificationRequestList{}
        _ = k8sClient.List(ctx, notificationList, client.InNamespace(testNamespace))
        return len(notificationList.Items) > 0
    }, timeout, interval).Should(BeTrue())

    // Validate NotificationRequest
    notificationList := &notificationv1.NotificationRequestList{}
    Expect(k8sClient.List(ctx, notificationList, client.InNamespace(testNamespace))).To(Succeed())
    Expect(notificationList.Items).To(HaveLen(1))

    notification := notificationList.Items[0]
    Expect(notification.Spec.NotificationType).To(Equal("human_review_required"))
    Expect(notification.Spec.Message).To(ContainSubstring("workflow_not_found"))

    // Validate RemediationRequest status
    Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
    Expect(rr.Status.Phase).To(Equal("RequiresReview"))
    Expect(rr.Status.Reason).To(Equal("HumanReviewRequired"))

    // Validate audit event
    events, _, err := dsClient.AuditAPI.QueryAuditEvents(ctx).
        CorrelationId(string(rr.UID)).
        EventType("orchestrator.routing.human_review").
        Execute()
    Expect(err).ToNot(HaveOccurred())
    Expect(events.Data).To(HaveLen(1))
    Expect(events.Data[0].EventAction).To(Equal("notification_created"))
})
```

**Anti-Pattern Warnings**:
- ❌ **HTTP TESTING**: Don't test RO via HTTP endpoints (it's a CRD controller)
- ✅ **CRD ORCHESTRATION**: Test via K8s client CRD creation and status watches

---

### **IT-RO-197-002: Verify NO WorkflowExecution created when needsHumanReview=true**

**Scenario**: Integration test confirms NO WorkflowExecution CRD is created when `needsHumanReview=true`.

**Given**:
- RemediationRequest and AIAnalysis CRDs set up as in IT-RO-197-001

**When**:
- RO reconciles and processes `needsHumanReview=true`

**Then**:
- **NotificationRequest exists** (1 CRD)
- **WorkflowExecution does NOT exist** (0 CRDs)
- **RemediationApprovalRequest does NOT exist** (0 CRDs)
- Only notification path is followed

**Validation**:
- ✅ Confirm negative case (no unwanted CRDs created)
- ✅ Routing logic is exclusive (notification OR execution, not both)

---

### **IT-RO-197-003: Handle concurrent RemediationRequests with different needsHumanReview values**

**Scenario**: Multiple concurrent RemediationRequests with different AIAnalysis results route correctly.

**Given**:
- 3 RemediationRequests created simultaneously:
  1. AIAnalysis with `needsHumanReview=true`, `reason="workflow_not_found"`
  2. AIAnalysis with `needsHumanReview=true`, `reason="low_confidence"`
  3. AIAnalysis with `needsHumanReview=false` (normal execution)

**When**:
- RO reconciles all 3 concurrently

**Then**:
- RR #1: NotificationRequest created with "workflow_not_found"
- RR #2: NotificationRequest created with "low_confidence"
- RR #3: WorkflowExecution created (normal flow)
- NO cross-contamination between CRDs
- All audit events correctly correlated by `correlation_id`

**Validation**:
- ✅ No concurrency issues between CRDs
- ✅ Each RR follows correct path independently
- ✅ Audit trail maintains correct correlations

---

## 🌐 **E2E Tests** (BR-HAPI-197 Full Remediation Flow)

**Location**: `test/e2e/remediationorchestrator/needs_human_review_e2e_test.go`
**Test Framework**: Ginkgo/Gomega
**Infrastructure**: KIND cluster + Mock LLM + DataStorage
**Focus**: Full remediation journey with human review gate

---

### **E2E-RO-197-001: Complete remediation flow blocked by needsHumanReview**

**Scenario**: End-to-end remediation flow where HAPI returns `needs_human_review=true`, blocking automatic remediation.

**Given**:
- KIND cluster with all Kubernaut services deployed
- Mock LLM configured to return `needs_human_review=true`, `reason="no_workflows_matched"`
- Simulated PodCrashLoopBackOff alert

**When**:
1. Signal ingested → RemediationRequest created
2. RO creates AIAnalysis
3. AIAnalysis calls HAPI → `needs_human_review=true`
4. RO reads AIAnalysis status
5. RO creates NotificationRequest (NOT WorkflowExecution)

**Then**:
- **Complete CRD Lifecycle**:
  - ✅ RemediationRequest: `phase="RequiresReview"`
  - ✅ AIAnalysis: `phase="Failed"`, `needsHumanReview=true`
  - ✅ NotificationRequest: Created and delivered
  - ❌ WorkflowExecution: Does NOT exist
  - ❌ RemediationApprovalRequest: Does NOT exist
- **Audit Trail Complete**:
  - `signal.received` (Gateway)
  - `orchestrator.aianalysis.created` (RO)
  - `aianalysis.human_review_required` (AIAnalysis)
  - `orchestrator.routing.human_review` (RO)
  - `orchestrator.notification.created` (RO)
- **Operator Notification Sent** with full context

**Validation**:
- ✅ Full remediation flow stops at human review gate
- ✅ NO automatic remediation attempted
- ✅ Operator receives actionable notification
- ✅ Complete audit trail for compliance

**Implementation Hint**:
```go
It("should complete E2E flow with needsHumanReview and block remediation", func() {
    // Step 1: Simulate signal (create RR directly or via Gateway)
    rr := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "e2e-human-review-test",
            Namespace: "default",
        },
        Spec: remediationv1.RemediationRequestSpec{
            TargetResource: remediationv1.TargetResource{
                Kind:      "Pod",
                Name:      "crashloop-pod",
                Namespace: "default",
            },
            SignalContext: remediationv1.SignalContext{
                AlertType: "PodCrashLoopBackOff",
            },
        },
    }
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())

    // Step 2: Wait for RO to create AIAnalysis
    Eventually(func() bool {
        analysisList := &aianalysisv1.AIAnalysisList{}
        _ = k8sClient.List(ctx, analysisList, client.InNamespace("default"))
        return len(analysisList.Items) > 0
    }, e2eTimeout, e2eInterval).Should(BeTrue())

    // Step 3: Wait for AIAnalysis to complete with needsHumanReview=true
    var analysis aianalysisv1.AIAnalysis
    Eventually(func() bool {
        analysisList := &aianalysisv1.AIAnalysisList{}
        _ = k8sClient.List(ctx, analysisList, client.InNamespace("default"))
        if len(analysisList.Items) == 0 {
            return false
        }
        analysis = analysisList.Items[0]
        return analysis.Status.Phase == "Failed" && analysis.Status.NeedsHumanReview
    }, e2eTimeout, e2eInterval).Should(BeTrue())

    Expect(analysis.Status.NeedsHumanReview).To(BeTrue())
    Expect(analysis.Status.HumanReviewReason).To(Equal("no_workflows_matched"))

    // Step 4: Wait for RO to create NotificationRequest
    Eventually(func() bool {
        notificationList := &notificationv1.NotificationRequestList{}
        _ = k8sClient.List(ctx, notificationList, client.InNamespace("default"))
        return len(notificationList.Items) > 0
    }, e2eTimeout, e2eInterval).Should(BeTrue())

    // Step 5: Verify NO WorkflowExecution or RemediationApprovalRequest
    weList := &wev1.WorkflowExecutionList{}
    Expect(k8sClient.List(ctx, weList, client.InNamespace("default"))).To(Succeed())
    Expect(weList.Items).To(HaveLen(0), "No WorkflowExecution should exist")

    rarList := &remediationv1.RemediationApprovalRequestList{}
    Expect(k8sClient.List(ctx, rarList, client.InNamespace("default"))).To(Succeed())
    Expect(rarList.Items).To(HaveLen(0), "No RemediationApprovalRequest should exist")

    // Step 6: Verify complete audit trail
    events, _, err := dsClient.AuditAPI.QueryAuditEvents(ctx).
        CorrelationId(string(rr.UID)).
        Execute()
    Expect(err).ToNot(HaveOccurred())

    eventTypes := make(map[string]bool)
    for _, event := range events.Data {
        eventTypes[event.EventType] = true
    }
    Expect(eventTypes).To(HaveKey("aianalysis.human_review_required"))
    Expect(eventTypes).To(HaveKey("orchestrator.routing.human_review"))
    Expect(eventTypes).To(HaveKey("orchestrator.notification.created"))

    // Step 7: Verify RemediationRequest status
    Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rr), rr)).To(Succeed())
    Expect(rr.Status.Phase).To(Equal("RequiresReview"))
    Expect(rr.Status.Reason).To(Equal("HumanReviewRequired"))
})
```

---

### **E2E-RO-197-002: Verify remediation proceeds when needsHumanReview=false**

**Scenario**: Confirm normal remediation flow proceeds when `needsHumanReview=false` (control test).

**Given**:
- KIND cluster with all services
- Mock LLM configured to return successful workflow selection (`needs_human_review=false`)

**When**:
- Full remediation flow executes

**Then**:
- ✅ WorkflowExecution CRD created
- ❌ NotificationRequest NOT created (unless workflow fails later)
- ✅ RemediationRequest progresses to "InProgress" → "Completed"
- ✅ Normal remediation completes successfully

**Validation**:
- ✅ Control case confirms routing logic works both directions
- ✅ No false positives (notification when not needed)
- ✅ Normal flow unaffected by BR-HAPI-197 implementation

---

## 📊 **Test Coverage Summary**

| Test Tier | Scenarios | BR Coverage | Code Coverage Target | Status |
|-----------|-----------|-------------|----------------------|--------|
| **Unit** | 6 | 70%+ | 70%+ | 🔄 DRAFT |
| **Integration** | 3 | 50%+ | 50% | 🔄 DRAFT |
| **E2E** | 2 | <10% | 50% | 🔄 DRAFT |
| **Total** | 11 | - | - | 🔄 DRAFT |

---

## ✅ **Acceptance Criteria**

**Definition of Done**:
- [ ] All 6 unit tests implemented and passing
- [ ] All 3 integration tests implemented and passing
- [ ] All 2 E2E tests implemented and passing
- [ ] Code coverage meets targets (70% unit, 50% integration, 50% E2E)
- [ ] No linter errors introduced
- [ ] Anti-patterns avoided (NULL-TESTING, IMPLEMENTATION TESTING, DIRECT AUDIT)
- [ ] Test execution time within SLA:
  - Unit tests: <5 seconds total
  - Integration tests: <2 minutes total
  - E2E tests: <10 minutes total
- [ ] All tests follow TESTING_GUIDELINES.md patterns
- [ ] DD-CONTRACT-002 two-flag architecture enforced
- [ ] Test plan reviewed and approved by user

---

## 🔗 **Related Documentation**

- [BR-HAPI-197: Human Review Required Flag](../../requirements/BR-HAPI-197-needs-human-review-field.md)
- [BR-HAPI-197 Completion Plan](../../handoff/BR-HAPI-197-COMPLETION-PLAN-JAN20-2026.md)
- [DD-CONTRACT-002: Service Integration Contracts](../../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md)
- [AIAnalysis Test Plan (BR-HAPI-197)](aianalysis_test_plan_v1.0.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)

---

**Confidence Assessment**: 95%
- ✅ Test scenarios cover all RO routing logic for BR-HAPI-197
- ✅ Two-flag architecture (needsHumanReview vs approvalRequired) tested explicitly
- ✅ Defense-in-depth strategy followed (70/50/10 BR coverage)
- ✅ Anti-patterns explicitly warned against
- ✅ Integration with AIAnalysis test plan ensures complete coverage
- ⚠️ 5% risk: Notification service integration may need adjustment based on actual notification delivery mechanism

---

**Next Steps**:
1. User reviews test plan
2. If approved, implement tests following TDD RED-GREEN-REFACTOR
3. Start with Unit tests (UT-RO-197-001 through UT-RO-197-006)
4. Then Integration tests (IT-RO-197-001 through IT-RO-197-003)
5. Finally E2E tests (E2E-RO-197-001, E2E-RO-197-002)
6. Coordinate with AIAnalysis test implementation for end-to-end validation
