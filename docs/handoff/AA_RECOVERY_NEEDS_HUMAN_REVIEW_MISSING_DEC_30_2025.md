# AIAnalysis Recovery Response Gap: Missing `needs_human_review` Field

**Date**: December 30, 2025
**Priority**: P1 - Critical Gap in Error Handling
**Business Requirement**: BR-HAPI-197 (Human review flags for uncertain AI decisions)
**Affected Component**: AIAnalysis Controller (`pkg/aianalysis`)
**HAPI Version**: Current production

---

## 🚨 **Executive Summary**

The AIAnalysis controller's recovery flow is missing critical error handling for `needs_human_review` scenarios. While **incident analysis** correctly checks this flag, **recovery analysis** does not, creating an inconsistency that could lead to automated workflow execution in scenarios requiring human judgment.

**Impact**:
- ❌ Recovery scenarios requiring human review are not properly handled
- ❌ AA service may proceed with unreliable workflow execution
- ❌ Go OpenAPI client is out of sync with HAPI Python schema
- ✅ Incident analysis flow works correctly (already implemented)

---

## 📊 **Root Cause Analysis**

### **Three-Layer Problem**

#### **1. Go OpenAPI Client Out of Date**

The Go HAPI client (`pkg/holmesgpt/client/oas_schemas_gen.go`) is missing fields that exist in the Python HAPI schema:

```go
// ❌ CURRENT Go Client (Missing Fields)
type RecoveryResponse struct {
    IncidentID         string
    CanRecover         bool
    Strategies         []RecoveryStrategy
    AnalysisConfidence float64
    Warnings           []string
    SelectedWorkflow   OptNilRecoveryResponseSelectedWorkflow
    RecoveryAnalysis   OptNilRecoveryResponseRecoveryAnalysis
    // MISSING: needs_human_review
    // MISSING: human_review_reason
}
```

```python
# ✅ ACTUAL Python Schema (BR-HAPI-197)
class RecoveryResponse(BaseModel):
    incident_id: str
    can_recover: bool
    strategies: List[RecoveryStrategy]
    analysis_confidence: float
    warnings: List[str]
    selected_workflow: Optional[Dict[str, Any]]
    recovery_analysis: Optional[Dict[str, Any]]
    needs_human_review: bool = Field(default=False)  # ADDED BR-HAPI-197
    human_review_reason: Optional[str] = Field(default=None)  # ADDED BR-HAPI-197
```

**Action Required**: Regenerate Go OpenAPI client from latest HAPI OpenAPI spec.

---

#### **2. AA Service Logic Gap**

The AA service correctly checks `needs_human_review` for **incident responses** but not for **recovery responses**:

```go
// ✅ INCIDENT: Correctly checks needs_human_review
// File: pkg/aianalysis/handlers/response_processor.go:64-82
func (p *ResponseProcessor) ProcessIncidentResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.IncidentResponse) (ctrl.Result, error) {
    needsHumanReview := GetOptBoolValue(resp.NeedsHumanReview)
    if needsHumanReview {
        return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
    }
    // ... proceed with workflow execution
}

// ❌ RECOVERY: Does NOT check needs_human_review
// File: pkg/aianalysis/handlers/response_processor.go:162-221
func (p *ResponseProcessor) ProcessRecoveryResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
    // Only checks CanRecover and hasSelectedWorkflow
    if !resp.CanRecover {
        return p.handleRecoveryNotPossible(ctx, analysis, resp)
    }
    if !hasSelectedWorkflow {
        return p.handleRecoveryNotPossible(ctx, analysis, resp)
    }
    // ❌ MISSING: No check for resp.NeedsHumanReview
    // Proceeds directly to workflow execution setup
}
```

**Action Required**: Add `needs_human_review` check to `ProcessRecoveryResponse` matching incident flow.

---

#### **3. Test Coverage Gap**

AA integration tests do not cover `needs_human_review` scenarios for recovery responses.

**Existing Coverage** (Incident):
- ✅ Tests exist for `needs_human_review=true` in incident analysis
- ✅ Tests verify transition to `RequiresHumanReview` phase

**Missing Coverage** (Recovery):
- ❌ No tests for `needs_human_review=true` in recovery analysis
- ❌ No tests for `human_review_reason` enum values
- ❌ No tests for recovery scenarios requiring human judgment

**Action Required**: Add integration tests for recovery human review scenarios.

---

## 🎯 **Business Scenarios Requiring Human Review (Recovery)**

Per BR-HAPI-197, HAPI should set `needs_human_review=true` for recovery in these scenarios:

| Scenario | `needs_human_review` | `human_review_reason` | `can_recover` | Expected AA Behavior |
|---|---|---|---|---|
| **No recovery workflow found** | `true` | `no_matching_workflows` | `true` | Transition to `RequiresHumanReview` |
| **Low confidence recovery** | `true` | `low_confidence` | `true` | Transition to `RequiresHumanReview` |
| **Signal not reproducible** | `true` | `signal_not_reproducible` | `false` | Transition to `RequiresHumanReview` or `Completed` |
| **LLM parsing failure** | `true` | `llm_parsing_error` | `true` | Transition to `RequiresHumanReview` |
| **Normal recovery** | `false` | `null` | `true` | Proceed to `AwaitingApproval` |

---

## 🔧 **Required Changes**

### **Change 1: Regenerate Go OpenAPI Client**

**File**: `pkg/holmesgpt/client/`
**Action**: Run HAPI OpenAPI client generation

```bash
cd pkg/holmesgpt/client
go generate ./...
```

**Verify**:
```bash
grep -A 5 "type RecoveryResponse struct" pkg/holmesgpt/client/oas_schemas_gen.go | grep -i "needs_human_review"
```

Expected output:
```go
NeedsHumanReview OptBool `json:"needs_human_review"`
HumanReviewReason OptNilHumanReviewReason `json:"human_review_reason"`
```

---

### **Change 2: Update AA Service Logic**

**File**: `pkg/aianalysis/handlers/response_processor.go`

**Add to `ProcessRecoveryResponse` (after line 175)**:

```go
func (p *ResponseProcessor) ProcessRecoveryResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
    analysis.Status.ConsecutiveFailures = 0

    hasSelectedWorkflow := resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null
    needsHumanReview := GetOptBoolValue(resp.NeedsHumanReview)  // ADD THIS

    p.log.Info("Processing successful recovery response",
        "canRecover", resp.CanRecover,
        "confidence", resp.AnalysisConfidence,
        "warningsCount", len(resp.Warnings),
        "hasSelectedWorkflow", hasSelectedWorkflow,
        "needsHumanReview", needsHumanReview,  // ADD THIS
    )

    // BR-HAPI-197: Check if recovery requires human review (ADD THIS BLOCK)
    if needsHumanReview {
        return p.handleWorkflowResolutionFailureFromRecovery(ctx, analysis, resp)
    }

    // Check if recovery is not possible
    if !resp.CanRecover {
        return p.handleRecoveryNotPossible(ctx, analysis, resp)
    }

    // ... rest of function
}
```

**Add new handler method (similar to incident flow)**:

```go
// handleWorkflowResolutionFailureFromRecovery handles when recovery HAPI cannot provide reliable workflow
// BR-HAPI-197: Human review required for uncertain recovery recommendations
func (p *ResponseProcessor) handleWorkflowResolutionFailureFromRecovery(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
    reasonStr := GetOptNilStringValue(resp.HumanReviewReason)
    subReason := mapEnumToSubReason(reasonStr)

    p.log.Info("Recovery workflow resolution failed - human review required",
        "confidence", resp.AnalysisConfidence,
        "reason", reasonStr,
        "mappedSubReason", subReason,
        "warnings", resp.Warnings,
    )

    now := metav1.Now()
    analysis.Status.Phase = aianalysis.PhaseRequiresHumanReview
    analysis.Status.CompletedAt = &now
    analysis.Status.Reason = "WorkflowResolutionFailed"
    analysis.Status.SubReason = subReason

    // BR-HAPI-197: Track human review metrics
    p.metrics.HumanReviewRequiredTotal.WithLabelValues("WorkflowResolutionFailed", subReason).Inc()
    p.metrics.RecordHumanReview("WorkflowResolutionFailed", subReason)

    analysis.Status.InvestigationID = resp.IncidentID
    analysis.Status.Message = fmt.Sprintf("HAPI could not provide reliable recovery workflow recommendation (reason: %s)", reasonStr)
    analysis.Status.Warnings = resp.Warnings

    return ctrl.Result{}, nil
}
```

---

### **Change 3: Add Integration Tests**

**File**: `test/integration/aianalysis/recovery_human_review_test.go` (NEW FILE)

**Test Scenarios Required**:

1. **Recovery - No Workflow Found**
   - Mock HAPI to return `needs_human_review=true`, `human_review_reason="no_matching_workflows"`
   - Verify AIAnalysis transitions to `RequiresHumanReview` phase
   - Verify `SubReason` is `NoMatchingWorkflows`

2. **Recovery - Low Confidence**
   - Mock HAPI to return `needs_human_review=true`, `human_review_reason="low_confidence"`
   - Verify AIAnalysis transitions to `RequiresHumanReview` phase
   - Verify `SubReason` is `LowConfidence`

3. **Recovery - Signal Not Reproducible**
   - Mock HAPI to return `needs_human_review=true`, `human_review_reason="signal_not_reproducible"`
   - Verify AIAnalysis handles gracefully (issue self-resolved)
   - Verify appropriate phase transition

4. **Recovery - LLM Parsing Error**
   - Mock HAPI to return `needs_human_review=true`, `human_review_reason="llm_parsing_error"`
   - Verify AIAnalysis transitions to `RequiresHumanReview` phase
   - Verify `SubReason` is `LLMParsingError`

5. **Recovery - Normal Flow (Baseline)**
   - Mock HAPI to return `needs_human_review=false`
   - Verify AIAnalysis proceeds to `AwaitingApproval` phase

**Test Template**:

```go
var _ = Describe("Recovery Human Review Required", Label("integration", "recovery"), func() {
    It("should transition to RequiresHumanReview when HAPI returns needs_human_review=true (no_matching_workflows)", func(ctx SpecContext) {
        By("Creating AIAnalysis with previous execution context")
        analysis := createTestAIAnalysisWithPreviousExecution()
        Expect(k8sClient.Create(ctx, analysis)).To(Succeed())

        By("Waiting for investigation phase")
        waitForPhase(ctx, analysis, aianalysis.PhaseInvestigating)

        By("Mocking HAPI recovery response with needs_human_review=true")
        mockHAPIRecoveryResponse(hapiMock, hapiMockRecoveryResponse{
            IncidentID:        analysis.Name,
            CanRecover:        true,
            NeedsHumanReview:  true,
            HumanReviewReason: "no_matching_workflows",
            SelectedWorkflow:  nil,
        })

        By("Verifying transition to RequiresHumanReview phase")
        Eventually(func() string {
            Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(analysis), analysis)).To(Succeed())
            return analysis.Status.Phase
        }, "30s", "1s").Should(Equal(aianalysis.PhaseRequiresHumanReview))

        By("Verifying Reason and SubReason")
        Expect(analysis.Status.Reason).To(Equal("WorkflowResolutionFailed"))
        Expect(analysis.Status.SubReason).To(Equal("NoMatchingWorkflows"))

        By("Verifying completion timestamp is set")
        Expect(analysis.Status.CompletedAt).ToNot(BeNil())
    }, SpecTimeout(time.Minute))
})
```

---

## 📋 **Implementation Checklist**

### **Phase 1: Client Generation (5 min)**
- [ ] Regenerate Go OpenAPI client from latest HAPI spec
- [ ] Verify `NeedsHumanReview` and `HumanReviewReason` fields exist in `RecoveryResponse`
- [ ] Run `make test-unit-aianalysis` to ensure no compilation errors

### **Phase 2: Business Logic (30 min)**
- [ ] Add `needsHumanReview` check to `ProcessRecoveryResponse`
- [ ] Implement `handleWorkflowResolutionFailureFromRecovery` handler
- [ ] Update logging to include `needsHumanReview` field
- [ ] Verify metrics are recorded for human review scenarios

### **Phase 3: Integration Tests (45 min)**
- [ ] Create `recovery_human_review_test.go`
- [ ] Implement 4 test scenarios (no workflow, low confidence, not reproducible, LLM error)
- [ ] Add baseline test for normal recovery flow
- [ ] Verify all tests pass: `make test-integration-aianalysis`

### **Phase 4: Validation (15 min)**
- [ ] Run full test suite: `make test-all-aianalysis`
- [ ] Manual testing in dev environment with mock HAPI
- [ ] Update documentation if needed

**Total Estimated Time**: ~1.5-2 hours

---

## 🔗 **Related Documentation**

- **BR-HAPI-197**: Human Review Flags for Uncertain AI Decisions
- **BR-AI-082**: Recovery Flow Support
- **DD-INTEGRATION-001**: Go-Bootstrapped Integration Test Infrastructure
- **HAPI OpenAPI Spec**: `holmesgpt-api/docs/openapi.yaml`

---

## 📞 **Contact**

**HAPI Team**: Available for questions about `needs_human_review` semantics
**AA Team**: This document is your implementation guide

---

## 🎯 **Success Criteria**

1. ✅ Go OpenAPI client includes `needs_human_review` and `human_review_reason` fields
2. ✅ `ProcessRecoveryResponse` checks `needs_human_review` before proceeding
3. ✅ AIAnalysis transitions to `RequiresHumanReview` phase for recovery scenarios
4. ✅ Integration tests cover all 4 human review scenarios for recovery
5. ✅ Parity achieved with incident analysis flow error handling

---

**End of Document**

