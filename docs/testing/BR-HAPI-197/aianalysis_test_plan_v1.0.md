# AIAnalysis Test Plan for BR-HAPI-197

**Business Requirement**: BR-HAPI-197 - Human Review Required Flag
**Service**: AIAnalysis
**Version**: 1.0
**Date**: January 20, 2026
**Status**: 🔄 DRAFT (for user review)

---

## 📋 **Test Plan Overview**

### **Scope**
This test plan covers AIAnalysis service's integration with BR-HAPI-197 `needs_human_review` flag:
- **Phase 1** (BR-HAPI-197 Completion): Extract and store `needsHumanReview` + `humanReviewReason` from HAPI response
- **Phase 2** (BR-HAPI-212 Extension): Extract RCA `affectedResource` (covered in separate test plan)

### **Services Under Test**
1. **AIAnalysis Service**: Response processing, CRD status updates, metrics emission
2. **Integration Point**: HAPI → AIAnalysis contract (DD-CONTRACT-002)

### **Out of Scope**
- ❌ HAPI logic for setting `needs_human_review` (covered by HAPI test plan)
- ❌ RO handling of `needs_human_review` (covered by RO test plan)
- ❌ BR-HAPI-212 `affectedResource` extraction (covered in separate test plan)

---

## 🎯 **Defense-in-Depth Coverage**

Following `TESTING_GUIDELINES.md` defense-in-depth strategy:

| Tier | BR Coverage | Code Coverage | Focus | Test Count |
|------|-------------|---------------|-------|------------|
| **Unit** | 70%+ | 70%+ | Response processor logic, field extraction, error handling | 8 scenarios |
| **Integration** | 50%+ | 50% | CRD updates, metrics emission, HAPI client integration | 4 scenarios |
| **E2E** | <10% | 50% | Full flow with mock LLM, NotificationRequest creation | 2 scenarios |

---

## 🧪 **Unit Tests** (BR-HAPI-197 Response Processing)

**Location**: `test/unit/aianalysis/handlers/response_processor_test.go`
**Test Framework**: Ginkgo/Gomega
**Focus**: Business logic correctness, field extraction, error handling

**Note**: Per `TESTING_GUIDELINES.md` and `03-testing-strategy.mdc`, unit tests are located in `test/unit/{service_name}/` with matching subdirectory structure from `pkg/`.

---

### **UT-AA-197-001: Extract needs_human_review from HAPI response**

**Scenario**: When HAPI returns `needs_human_review=true`, AIAnalysis extracts and stores the flag in CRD status.

**Given**:
- HAPI responds with:
  ```json
  {
    "investigation_id": "test-123",
    "needs_human_review": true,
    "human_review_reason": "workflow_not_found",
    "warnings": ["Workflow validation failed"]
  }
  ```

**When**:
- `ResponseProcessor.Process()` processes the HAPI response

**Then**:
- `AIAnalysis.Status.NeedsHumanReview` is set to `true`
- `AIAnalysis.Status.HumanReviewReason` is set to `"workflow_not_found"`
- `AIAnalysis.Status.Phase` is set to `"Failed"`
- `AIAnalysis.Status.Reason` contains warning message

**Validation**:
- ✅ CRD status fields are populated correctly
- ✅ No errors returned from processor
- ✅ Field mapping follows DD-CONTRACT-002 specification

**Implementation Hint**:
```go
import (
    holmesgpt "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

It("should extract needs_human_review=true from HAPI response", func() {
    hapiResp := &holmesgpt.IncidentResponse{
        InvestigationId:    "test-123",
        NeedsHumanReview:   ptr.To(true),
        HumanReviewReason:  ptr.To(holmesgpt.HumanReviewReasonWorkflowNotFound),  // Use OpenAPI client constant
        Warnings:          []string{"Workflow validation failed"},
    }

    processor := NewResponseProcessor(mockMetrics, mockAudit, mockLogger)
    analysis := &aianalysisv1.AIAnalysis{}

    err := processor.Process(ctx, analysis, hapiResp)
    Expect(err).ToNot(HaveOccurred())

    // Validate BR-HAPI-197 fields
    Expect(analysis.Status.NeedsHumanReview).To(BeTrue())
    Expect(analysis.Status.HumanReviewReason).To(Equal(string(holmesgpt.HumanReviewReasonWorkflowNotFound)))  // Convert to string
    Expect(analysis.Status.Phase).To(Equal("Failed"))
})
```

**Anti-Pattern Warnings**:
- ❌ **NULL-TESTING**: Don't just check `analysis.Status.NeedsHumanReview != nil`
- ❌ **IMPLEMENTATION TESTING**: Don't test internal processor methods directly
- ✅ **BUSINESS OUTCOME**: Validate CRD status reflects HAPI decision correctly

---

### **UT-AA-197-002: Handle needs_human_review=false (happy path)**

**Scenario**: When HAPI returns `needs_human_review=false`, AIAnalysis does NOT set the flag.

**Given**:
- HAPI responds with:
  ```json
  {
    "investigation_id": "test-456",
    "needs_human_review": false,
    "selected_workflow": {
      "workflow_id": "restart-pod-v1"
    },
    "confidence": 0.95
  }
  ```

**When**:
- `ResponseProcessor.Process()` processes the HAPI response

**Then**:
- `AIAnalysis.Status.NeedsHumanReview` remains `false` (default value)
- `AIAnalysis.Status.HumanReviewReason` is empty
- `AIAnalysis.Status.Phase` is set to `"Completed"`
- `AIAnalysis.Status.SelectedWorkflow` is populated

**Validation**:
- ✅ No human review flags set for successful HAPI responses
- ✅ Normal processing continues (workflow selection, Rego evaluation)
- ✅ Phase transitions correctly to "Completed"

---

### **UT-AA-197-003: Map all 6 human_review_reason values**

**Scenario**: AIAnalysis correctly maps all 6 `human_review_reason` enum values from BR-HAPI-197.2.

**Given**:
- HAPI returns each of the 6 reason values in separate test cases using OpenAPI client constants:
  1. `holmesgpt.HumanReviewReasonWorkflowNotFound` (`"workflow_not_found"`)
  2. `holmesgpt.HumanReviewReasonNoMatchingWorkflows` (`"no_matching_workflows"`)
  3. `holmesgpt.HumanReviewReasonLowConfidence` (`"low_confidence"`)
  4. `holmesgpt.HumanReviewReasonLlmParsingError` (`"llm_parsing_error"`)
  5. `holmesgpt.HumanReviewReasonParameterValidationFailed` (`"parameter_validation_failed"`)
  6. `holmesgpt.HumanReviewReasonImageMismatch` (`"image_mismatch"`)

**When**:
- `ResponseProcessor.Process()` processes each response

**Then**:
- `AIAnalysis.Status.HumanReviewReason` matches the HAPI reason exactly
- All enum values are handled without errors
- No string transformations or modifications applied

**Validation**:
- ✅ All BR-HAPI-197.2 scenarios are supported
- ✅ String values are preserved exactly (no camelCase/snake_case conversion)
- ✅ Metrics can be labeled with these reasons

**Implementation Hint**:
```go
import (
    holmesgpt "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

DescribeTable("should map all human_review_reason enum values",
    func(reason holmesgpt.HumanReviewReason, expectedString string) {
        hapiResp := &holmesgpt.IncidentResponse{
            InvestigationId:   "test",
            NeedsHumanReview:  ptr.To(true),
            HumanReviewReason: &reason,  // Use OpenAPI client enum
        }

        processor := NewResponseProcessor(mockMetrics, mockAudit, mockLogger)
        analysis := &aianalysisv1.AIAnalysis{}

        err := processor.Process(ctx, analysis, hapiResp)
        Expect(err).ToNot(HaveOccurred())
        Expect(analysis.Status.HumanReviewReason).To(Equal(expectedString))
    },
    Entry("workflow_not_found",
        holmesgpt.HumanReviewReasonWorkflowNotFound,
        "workflow_not_found"),
    Entry("no_workflows_matched",
        holmesgpt.HumanReviewReasonNoMatchingWorkflows,
        "no_matching_workflows"),
    Entry("low_confidence",
        holmesgpt.HumanReviewReasonLowConfidence,
        "low_confidence"),
    Entry("llm_parsing_error",
        holmesgpt.HumanReviewReasonLlmParsingError,
        "llm_parsing_error"),
    Entry("parameter_validation_failed",
        holmesgpt.HumanReviewReasonParameterValidationFailed,
        "parameter_validation_failed"),
    Entry("image_mismatch",
        holmesgpt.HumanReviewReasonImageMismatch,
        "image_mismatch"),
)
```

---

### **UT-AA-197-004: Emit human_review_required metric**

**Scenario**: When `needs_human_review=true`, AIAnalysis emits `kubernaut_aianalysis_human_review_required_total` metric with reason label.

**Given**:
- HAPI responds with `needs_human_review=true` and `human_review_reason="workflow_not_found"`

**When**:
- `ResponseProcessor.Process()` processes the response

**Then**:
- Metric `kubernaut_aianalysis_human_review_required_total{reason="workflow_not_found"}` is incremented by 1
- No other metrics are affected

**Validation**:
- ✅ Metric emitted with correct label
- ✅ Reason value matches HAPI response
- ✅ Counter increments exactly once per occurrence

**Implementation Hint**:
```go
import (
    holmesgpt "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

It("should emit human_review_required metric with reason label", func() {
    hapiResp := &holmesgpt.IncidentResponse{
        InvestigationId:   "test",
        NeedsHumanReview:  ptr.To(true),
        HumanReviewReason: ptr.To(holmesgpt.HumanReviewReasonWorkflowNotFound),  // Use OpenAPI client constant
    }

    // Use real metrics from pkg/aianalysis/metrics (not mock)
    testMetrics := metrics.NewMetrics()
    processor := NewResponseProcessor(testMetrics, mockAudit, mockLogger)
    analysis := &aianalysisv1.AIAnalysis{}

    _ = processor.Process(ctx, analysis, hapiResp)

    // Validate metric using getCounterValue helper or direct Prometheus API
    // (See test/unit/aianalysis/investigating_handler_test.go for pattern)
})
```

**Anti-Pattern Warnings**:
- ❌ **DIRECT METRICS CALLS**: Don't call `metrics.HumanReviewRequiredTotal.Inc()` directly in tests
- ✅ **BUSINESS VALIDATION**: Validate metrics through business logic invocation (Process())

---

### **UT-AA-197-006: Handle needs_human_review with selected_workflow (edge case)**

**Scenario**: When HAPI returns both `needs_human_review=true` AND `selected_workflow` (HAPI decided workflow after LLM correction), AIAnalysis prioritizes `needs_human_review`.

**Given**:
- HAPI responds with:
  ```json
  {
    "investigation_id": "test-edge",
    "needs_human_review": true,
    "human_review_reason": "low_confidence",
    "selected_workflow": {
      "workflow_id": "restart-pod-v1"
    },
    "confidence": 0.45
  }
  ```

**When**:
- `ResponseProcessor.Process()` processes the response

**Then**:
- `AIAnalysis.Status.NeedsHumanReview` is `true` (takes precedence)
- `AIAnalysis.Status.SelectedWorkflow` is populated (for operator reference)
- `AIAnalysis.Status.Phase` is `"Failed"` (requires human review)
- RO will create NotificationRequest (not WorkflowExecution)

**Validation**:
- ✅ `needs_human_review` flag is authoritative (blocks automatic execution)
- ✅ Workflow information is preserved for operator decision-making
- ✅ DD-CONTRACT-002 guarantees RO checks `needsHumanReview` before creating WE

---

### **UT-AA-197-007: Handle nil pointer safety for optional fields**

**Scenario**: When HAPI returns `nil` pointers for optional fields, AIAnalysis handles gracefully without panics.

**Given**:
- HAPI responds with `nil` pointers:
  ```go
  &holmesgpt.IncidentResponse{
      InvestigationId:   "test-nil",
      NeedsHumanReview:  nil,  // Optional field
      HumanReviewReason: nil,  // Optional field
  }
  ```

**When**:
- `ResponseProcessor.Process()` processes the response

**Then**:
- No panic occurs
- `AIAnalysis.Status.NeedsHumanReview` is `false` (default)
- `AIAnalysis.Status.HumanReviewReason` is empty string
- Processing continues without errors

**Validation**:
- ✅ Nil pointer safety for all optional HAPI fields
- ✅ Safe defaults applied when fields are missing
- ✅ No runtime panics under any HAPI response variation

---

### **UT-AA-197-008: Validate Phase transitions based on needs_human_review**

**Scenario**: AIAnalysis sets correct Phase values based on `needs_human_review` flag.

**Given** (test cases):
| needs_human_review | selected_workflow | Expected Phase | Reason |
|-------------------|-------------------|----------------|--------|
| `false` | present | `"Completed"` | Happy path |
| `true` | absent | `"Failed"` | Human review required |
| `true` | present | `"Failed"` | Human review takes precedence |

**When**:
- `ResponseProcessor.Process()` processes each scenario

**Then**:
- `AIAnalysis.Status.Phase` matches expected value
- Phase transitions follow AIAnalysis state machine
- RO can rely on Phase + `needs_human_review` combination for routing

**Validation**:
- ✅ Phase values are deterministic based on HAPI response
- ✅ No ambiguous states (Phase + flags are always consistent)
- ✅ DD-CONTRACT-002 guarantees are maintained

---

## 🔗 **Integration Tests** (BR-HAPI-197 End-to-End Integration)

**Location**: `test/integration/aianalysis/needs_human_review_integration_test.go`
**Test Framework**: Ginkgo/Gomega
**Infrastructure**: envtest + HAPI mock container
**Focus**: CRD updates, metrics emission, audit events

---

### **IT-AA-197-001: Full flow with needs_human_review=true from mock HAPI**

**Scenario**: AIAnalysis processes a HAPI response with `needs_human_review=true` and updates CRD, emits metrics, and creates audit event.

**Given**:
- AIAnalysis CRD exists in envtest with `spec.analysisRequest`
- Mock HAPI container is running and configured to return `needs_human_review=true`

**When**:
- AIAnalysis controller reconciles the CRD
- Controller calls HAPI `/api/v1/incident/analyze` endpoint
- HAPI returns:
  ```json
  {
    "investigation_id": "integ-test-001",
    "needs_human_review": true,
    "human_review_reason": "no_workflows_matched",
    "warnings": ["No matching workflows found for alert type"]
  }
  ```

**Then**:
- **CRD Status Updated**:
  - `status.needsHumanReview` = `true`
  - `status.humanReviewReason` = `"no_workflows_matched"`
  - `status.phase` = `"Failed"`
  - `status.conditions` includes "HumanReviewRequired" condition
- **Metrics Emitted**:
  - `kubernaut_aianalysis_human_review_required_total{reason="no_workflows_matched"}` increments
- **Audit Event Created**:
  - `event_type` = `"aianalysis.human_review_required"`
  - `event_action` = `"review_required"`
  - `correlation_id` matches AIAnalysis UID
  - `details` includes `human_review_reason`

**Validation**:
- ✅ CRD status reflects HAPI decision
- ✅ Metrics observable via Prometheus registry
- ✅ Audit event queryable from DataStorage OpenAPI client
- ✅ No WorkflowExecution CRD created (RO will create NotificationRequest instead)

**Implementation Hint**:
```go
It("should process needs_human_review=true from HAPI and update CRD", func() {
    // Create AIAnalysis CRD
    analysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-human-review",
            Namespace: testNamespace,
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            AnalysisRequest: aianalysisv1.AnalysisRequest{
                SignalContext: aianalysisv1.SignalContext{
                    AlertType: "PodCrashLoopBackOff",
                },
            },
        },
    }
    Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

    // Wait for AIAnalysis to complete processing
    Eventually(func() string {
        _ = k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)
        return analysis.Status.Phase
    }, timeout, interval).Should(Equal("Failed"))

    // Validate BR-HAPI-197 fields in status
    Expect(analysis.Status.NeedsHumanReview).To(BeTrue())
    Expect(analysis.Status.HumanReviewReason).To(Equal("no_workflows_matched"))

    // Validate metrics (query Prometheus registry)
    metricValue := testutil.GetCounterValue(
        promRegistry,
        "kubernaut_aianalysis_human_review_required_total",
        prometheus.Labels{"reason": "no_workflows_matched"},
    )
    Expect(metricValue).To(BeNumerically(">", 0))

    // Validate audit event (query DataStorage)
    events, _, err := dsClient.AuditAPI.QueryAuditEvents(ctx).
        CorrelationId(string(analysis.UID)).
        EventType("aianalysis.human_review_required").
        Execute()
    Expect(err).ToNot(HaveOccurred())
    Expect(events.Data).To(HaveLen(1))
    Expect(events.Data[0].EventAction).To(Equal("review_required"))
})
```

**Anti-Pattern Warnings**:
- ❌ **DIRECT AUDIT TESTING**: Don't assert on DataStorage's internal audit storage logic
- ✅ **BUSINESS VALIDATION**: Verify audit event exists and contains correct correlation_id
- ❌ **HTTP TESTING**: Don't test AIAnalysis via HTTP endpoints (it's a CRD controller)
- ✅ **CRD INTERACTION**: Test via K8s client CRD operations

---

### **IT-AA-197-002: ~~Verify RO does NOT create WorkflowExecution when needs_human_review=true~~

**STATUS**: ⚠️ **MOVED TO RO TEST PLAN** - This test validates RO controller routing logic, not AIAnalysis controller behavior.

**Original Scenario**: When AIAnalysis has `needsHumanReview=true`, RemediationOrchestrator creates NotificationRequest instead of WorkflowExecution.

**Given**:
- RemediationRequest CRD exists
- AIAnalysis completes with `status.needsHumanReview=true`

**When**:
- RemediationOrchestrator reconciles the RemediationRequest

**Then**:
- **No WorkflowExecution CRD created**
- **NotificationRequest CRD created** with:
  - `spec.notificationType` = `"human_review_required"`
  - `spec.message` includes `human_review_reason`
  - `spec.correlationId` matches RemediationRequest UID
- **RemediationRequest status updated**:
  - `status.phase` = `"RequiresReview"`
  - `status.reason` = `"HumanReviewRequired"`

**Validation**:
- ✅ RO routing logic honors `needsHumanReview` flag
- ✅ No automatic remediation attempted
- ✅ Operator receives notification
- ✅ DD-CONTRACT-002 contract is enforced

**Implementation Hint**:
```go
It("should create NotificationRequest when AIAnalysis has needsHumanReview=true", func() {
    // Create RemediationRequest
    rr := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-rr-human-review",
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

    // Create AIAnalysis with needs_human_review=true
    analysis := &aianalysisv1.AIAnalysis{
        ObjectMeta: metav1.ObjectMeta{
            Name:      rr.Name + "-aianalysis",
            Namespace: testNamespace,
            Labels: map[string]string{
                "remediation-request": rr.Name,
            },
        },
        Spec: aianalysisv1.AIAnalysisSpec{
            AnalysisRequest: aianalysisv1.AnalysisRequest{/* ... */},
        },
        Status: aianalysisv1.AIAnalysisStatus{
            Phase:              "Failed",
            NeedsHumanReview:   true,
            HumanReviewReason:  "workflow_not_found",
        },
    }
    Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

    // Wait for RO to process
    Eventually(func() bool {
        notificationList := &notificationv1.NotificationRequestList{}
        _ = k8sClient.List(ctx, notificationList, client.InNamespace(testNamespace))
        return len(notificationList.Items) > 0
    }, timeout, interval).Should(BeTrue())

    // Verify NotificationRequest created (NOT WorkflowExecution)
    weList := &wev1.WorkflowExecutionList{}
    Expect(k8sClient.List(ctx, weList, client.InNamespace(testNamespace))).To(Succeed())
    Expect(weList.Items).To(HaveLen(0), "No WorkflowExecution should be created")

    notificationList := &notificationv1.NotificationRequestList{}
    Expect(k8sClient.List(ctx, notificationList, client.InNamespace(testNamespace))).To(Succeed())
    Expect(notificationList.Items).To(HaveLen(1), "NotificationRequest should be created")

    notification := notificationList.Items[0]
    Expect(notification.Spec.NotificationType).To(Equal("human_review_required"))
    Expect(notification.Spec.Message).To(ContainSubstring("workflow_not_found"))
})
```

---

### **IT-AA-197-003: Handle multiple AIAnalysis with different human_review_reasons**

**Scenario**: AIAnalysis response processor correctly extracts `needs_human_review` from multiple HAPI responses with different reason values.

**Given**:
- Mock HAPI client configured to return different responses:
  1. `needs_human_review=true`, `reason=holmesgpt.HumanReviewReasonWorkflowNotFound`
  2. `needs_human_review=true`, `reason=holmesgpt.HumanReviewReasonLowConfidence`
  3. `needs_human_review=false` (happy path)

**When**:
- ResponseProcessor processes all 3 responses sequentially

**Then**:
- Each AIAnalysis CRD status correctly populated:
  - CRD #1: `NeedsHumanReview=true`, `HumanReviewReason="workflow_not_found"`
  - CRD #2: `NeedsHumanReview=true`, `HumanReviewReason="low_confidence"`
  - CRD #3: `NeedsHumanReview=false`, `HumanReviewReason=""` (empty)

**Validation**:
- ✅ Response processor correctly maps different reason values
- ✅ Uses OpenAPI client constants for type safety
- ✅ No cross-contamination between different responses

**NOTE**: This test focuses on **AIAnalysis response processor logic**. Tests for RO routing behavior (WorkflowExecution prevention, NotificationRequest creation) are in the **RO test plan**.

---

### **IT-AA-197-004: Verify metric cardinality limits (6 reason values)**

**Scenario**: AIAnalysis metrics maintain acceptable cardinality (6 fixed reason values from BR-HAPI-197.2).

**Given**:
- AIAnalysis processes requests with all 6 `human_review_reason` values

**When**:
- Prometheus scrapes metrics endpoint

**Then**:
- Metric `kubernaut_aianalysis_human_review_required_total` has exactly 6 label values:
  1. `{reason="workflow_not_found"}`
  2. `{reason="no_workflows_matched"}`
  3. `{reason="low_confidence"}`
  4. `{reason="llm_parsing_error"}`
  5. `{reason="parameter_validation_failed"}`
  6. `{reason="container_image_mismatch"}`
- No unbounded cardinality explosion
- Metric cardinality remains constant (not growing over time)

**Validation**:
- ✅ Metric cardinality is bounded (6 fixed values)
- ✅ No risk of Prometheus cardinality explosion
- ✅ Metric remains queryable and performant

---

## 🌐 **E2E Tests** (BR-HAPI-197 Full Stack Validation)

**Location**: `test/e2e/aianalysis/needs_human_review_e2e_test.go`
**Test Framework**: Ginkgo/Gomega
**Infrastructure**: KIND cluster + HolmesGPT-API (with Mock LLM) + DataStorage (PostgreSQL + Redis)
**Focus**: Full remediation flow with human review path

---

### **E2E-AA-197-001: AIAnalysis correctly extracts needs_human_review from HAPI (E2E)**

**Scenario**: AIAnalysis controller correctly processes HAPI response with `needs_human_review=true` in a real cluster environment.

**Given**:
- KIND cluster with all Kubernaut services deployed
- HolmesGPT-API with Mock LLM configured to return `no_workflows_matched`
- DataStorage service running (for audit events)
- RemediationRequest CRD exists (created by test or RO)

**When**:
1. AIAnalysis CRD created (by test or RemediationOrchestrator)
2. AIAnalysis controller reconciles and calls HAPI
3. HAPI returns `needs_human_review=true`, `reason="no_workflows_matched"`
4. AIAnalysis controller processes HAPI response

**Then**:
- **AIAnalysis CRD status updated**:
  - `status.needsHumanReview` = `true`
  - `status.humanReviewReason` = `"no_workflows_matched"`
  - `status.phase` = `"Failed"`
  - `status.reason` = `"WorkflowResolutionFailed"` (backward compatibility)
- **Audit event created** in DataStorage:
  - `event_type` = `"aianalysis.human_review_required"`
  - `event_action` = `"failed"`
  - Event data includes `human_review_reason` field
- **Metrics emitted**:
  - `kubernaut_aianalysis_human_review_required_total{reason="no_workflows_matched"}` incremented

**Validation**:
- ✅ AIAnalysis correctly extracts and stores HAPI's `needs_human_review` flag
- ✅ Status fields properly populated for downstream services (RO) to consume
- ✅ Audit trail includes human review context
- ✅ Metrics correctly labeled with reason

**NOTE**: This test focuses on **AIAnalysis controller behavior**. RO routing logic (NotificationRequest creation, WorkflowExecution prevention) is validated in the **RO E2E test plan**.

**Implementation Hint**:
```go
It("should complete E2E flow with no_workflows_matched and create notification", func() {
    // Step 1: Create simulated signal (via Gateway or direct RR creation)
    rr := &remediationv1.RemediationRequest{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "e2e-no-workflows",
            Namespace: "default",
        },
        Spec: remediationv1.RemediationRequestSpec{
            TargetResource: remediationv1.TargetResource{
                Kind:      "Pod",
                Name:      "unknown-app-pod",
                Namespace: "default",
            },
            SignalContext: remediationv1.SignalContext{
                AlertType: "UnknownAlertType",
            },
        },
    }
    Expect(k8sClient.Create(ctx, rr)).To(Succeed())

    // Step 2: Wait for AIAnalysis to be created by RO
    Eventually(func() bool {
        analysisList := &aianalysisv1.AIAnalysisList{}
        _ = k8sClient.List(ctx, analysisList, client.InNamespace("default"))
        return len(analysisList.Items) > 0
    }, e2eTimeout, e2eInterval).Should(BeTrue())

    // Step 3: Wait for AIAnalysis to complete with needs_human_review
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

    // Validate AIAnalysis status
    Expect(analysis.Status.NeedsHumanReview).To(BeTrue())
    Expect(analysis.Status.HumanReviewReason).To(Equal("no_workflows_matched"))

    // Step 4: Verify audit event in DataStorage (AIAnalysis audit)
    events, _, err := dsClient.AuditAPI.QueryAuditEvents(ctx).
        CorrelationId(analysis.Spec.RemediationID).
        Execute()
    Expect(err).ToNot(HaveOccurred())

    eventTypes := make(map[string]bool)
    for _, event := range events.Data {
        eventTypes[event.EventType] = true
    }
    Expect(eventTypes).To(HaveKey("aianalysis.human_review_required"))

    // Step 5: Verify metrics emitted (scrape Prometheus endpoint)
    // Use 127.0.0.1 instead of localhost to avoid IPv6 resolution issues in CI/CD
    resp, err := http.Get("http://127.0.0.1:9184/metrics")
    Expect(err).ToNot(HaveOccurred())
    defer resp.Body.Close()

    bodyBytes, _ := io.ReadAll(resp.Body)
    metricsText := string(bodyBytes)
    Expect(metricsText).To(ContainSubstring(
        `kubernaut_aianalysis_human_review_required_total{reason="no_workflows_matched"}`))

    // NOTE: RO routing behavior (NotificationRequest creation, WorkflowExecution prevention)
    // is validated in the RO E2E test plan, not here
})
```

**Anti-Pattern Warnings**:
- ❌ **HTTP TESTING**: Don't test services via HTTP in E2E (use CRD interactions)
- ✅ **BUSINESS JOURNEY**: Test complete user/operator journey through CRD lifecycle
- ❌ **SLEEP FOR TIMING**: Don't use `time.Sleep()` - use `Eventually()` with appropriate timeouts

---

### **E2E-AA-197-002: Verify metrics observable in Prometheus for E2E flow**

**Scenario**: Prometheus metrics for `needs_human_review` are observable and queryable in E2E environment.

**Given**:
- KIND cluster with Prometheus deployed
- ServiceMonitor configured for AIAnalysis metrics endpoint

**When**:
- E2E flow executes with `needs_human_review=true`

**Then**:
- Prometheus query returns metric:
  ```promql
  kubernaut_aianalysis_human_review_required_total{reason="no_workflows_matched"} > 0
  ```
- Metric is scrapeable and queryable
- Metric labels match BR-HAPI-197 enum values

**Validation**:
- ✅ Metrics integration works end-to-end
- ✅ Prometheus scraping configured correctly
- ✅ Operators can create alerts based on this metric

---

## 📊 **Test Coverage Summary**

| Test Tier | Scenarios | BR Coverage | Code Coverage Target | Status |
|-----------|-----------|-------------|----------------------|--------|
| **Unit** | 8 | 70%+ | 70%+ | 🔄 DRAFT |
| **Integration** | 4 | 50%+ | 50% | 🔄 DRAFT |
| **E2E** | 2 | <10% | 50% | 🔄 DRAFT |
| **Total** | 14 | - | - | 🔄 DRAFT |

---

## ✅ **Acceptance Criteria**

**Definition of Done**:
- [ ] All 8 unit tests implemented and passing
- [ ] All 4 integration tests implemented and passing
- [ ] All 2 E2E tests implemented and passing
- [ ] Code coverage meets targets (70% unit, 50% integration, 50% E2E)
- [ ] No linter errors introduced
- [ ] Anti-patterns avoided (NULL-TESTING, IMPLEMENTATION TESTING, DIRECT AUDIT/METRICS)
- [ ] Test execution time within SLA:
  - Unit tests: <5 seconds total
  - Integration tests: <2 minutes total
  - E2E tests: <10 minutes total
- [ ] All tests follow TESTING_GUIDELINES.md patterns
- [ ] Test plan reviewed and approved by user

---

## 🔗 **Related Documentation**

- [BR-HAPI-197: Human Review Required Flag](../../requirements/BR-HAPI-197-needs-human-review-field.md)
- [BR-HAPI-197 Completion Plan](../../handoff/BR-HAPI-197-COMPLETION-PLAN-JAN20-2026.md)
- [DD-CONTRACT-002: Service Integration Contracts](../../architecture/decisions/DD-CONTRACT-002-service-integration-contracts.md)
- [TESTING_GUIDELINES.md](../../development/business-requirements/TESTING_GUIDELINES.md)
- [Test Plan Best Practices](../../development/testing/TEST_PLAN_BEST_PRACTICES.md)

---

**Confidence Assessment**: 95%
- ✅ Test scenarios cover all BR-HAPI-197 requirements
- ✅ Defense-in-depth strategy followed (70/50/10 BR coverage)
- ✅ Anti-patterns explicitly warned against
- ✅ Implementation hints provided for guidance
- ⚠️ 5% risk: Mock LLM configuration for E2E may need adjustment based on actual holmesgpt-api mock implementation

---

**Next Steps**:
1. User reviews test plan
2. If approved, implement tests following TDD RED-GREEN-REFACTOR
3. Start with Unit tests (UT-AA-197-001 through UT-AA-197-008)
4. Then Integration tests (IT-AA-197-001 through IT-AA-197-004)
5. Finally E2E tests (E2E-AA-197-001, E2E-AA-197-002)
