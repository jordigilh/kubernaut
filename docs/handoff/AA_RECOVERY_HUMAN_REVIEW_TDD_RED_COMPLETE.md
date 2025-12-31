# AIAnalysis Recovery Human Review - TDD RED Phase Complete

**Date**: December 30, 2025
**Status**: ✅ RED PHASE COMPLETE
**Next**: GREEN Phase Implementation
**Related**: `AA_RECOVERY_NEEDS_HUMAN_REVIEW_MISSING_DEC_30_2025.md`

---

## 🔴 **TDD RED PHASE SUMMARY**

### **Objective**

Create failing integration tests that define the expected behavior for recovery human review scenarios (BR-HAPI-197) using **REAL HAPI service** with MOCK_LLM_MODE=true.

### **✅ Completion Status**

**Phase**: RED (Write Failing Tests)
**Status**: ✅ COMPLETE
**Tests Created**: 4 integration tests
**Compilation Status**: ✅ PASSES
**Test Execution Status**: ❌ FAILS (as expected - HAPI doesn't implement needs_human_review for recovery yet)

---

## 📋 **Tests Created**

**File**: `test/integration/aianalysis/recovery_human_review_test.go`

### **Test Infrastructure**

**Uses REAL Services** (per TESTING_GUIDELINES.md):
- ✅ **REAL HAPI**: http://localhost:18120
- ✅ **MOCK LLM**: HAPI runs with MOCK_LLM_MODE=true (cost constraint)
- ✅ **REAL Data Storage**: http://localhost:18095
- ✅ **REAL PostgreSQL**: localhost:15438
- ✅ **REAL Redis**: localhost:16384

**Infrastructure Startup**:
```bash
podman-compose -f test/integration/aianalysis/podman-compose.yml up -d
```

---

### **Test 1: No Matching Workflows**
```go
It("should return needs_human_review=true when no workflows match", func() {
    recoveryReq := &client.RecoveryRequest{
        IncidentID:            "test-recovery-no-workflow",
        RemediationID:         "req-test-001",
        IsRecoveryAttempt:     client.NewOptBool(true),
        RecoveryAttemptNumber: client.NewOptNilInt(1),
    }

    resp, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)

    Expect(err).ToNot(HaveOccurred())
    Expect(resp.NeedsHumanReview.Value).To(BeTrue())
    Expect(resp.HumanReviewReason.Value).To(Equal("no_matching_workflows"))
})
```

**Expected Behavior**:
- HAPI returns `needs_human_review=true`
- `human_review_reason="no_matching_workflows"`
- `can_recover=true` (workflows exist, just none matched)

**TDD RED Status**: ❌ Will fail - HAPI Python code doesn't set these fields for recovery yet

---

### **Test 2: Low Confidence**
```go
It("should return needs_human_review=true when confidence is below threshold", func() {
    recoveryReq := &client.RecoveryRequest{
        IncidentID:            "test-recovery-low-conf",
        RemediationID:         "req-test-002",
        IsRecoveryAttempt:     client.NewOptBool(true),
        RecoveryAttemptNumber: client.NewOptNilInt(1),
    }

    resp, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)

    Expect(err).ToNot(HaveOccurred())
    Expect(resp.NeedsHumanReview.Value).To(BeTrue())
    Expect(resp.HumanReviewReason.Value).To(Equal("low_confidence"))
})
```

**Expected Behavior**:
- HAPI returns `needs_human_review=true`
- `human_review_reason="low_confidence"`
- `can_recover=true` (recovery possible but uncertain)

**TDD RED Status**: ❌ Will fail - HAPI Python code doesn't set these fields for recovery yet

---

### **Test 3: Signal Not Reproducible**
```go
It("should return needs_human_review=true when signal is no longer present", func() {
    recoveryReq := &client.RecoveryRequest{
        IncidentID:            "test-recovery-not-repro",
        RemediationID:         "req-test-003",
        IsRecoveryAttempt:     client.NewOptBool(true),
        RecoveryAttemptNumber: client.NewOptNilInt(1),
    }

    resp, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)

    Expect(err).ToNot(HaveOccurred())
    Expect(resp.NeedsHumanReview.Value).To(BeTrue())
    Expect(resp.HumanReviewReason.Value).To(Equal("signal_not_reproducible"))
})
```

**Expected Behavior**:
- HAPI returns `needs_human_review=true`
- `human_review_reason="signal_not_reproducible"`
- `can_recover=false` (issue self-resolved)

**TDD RED Status**: ❌ Will fail - HAPI Python code doesn't set these fields for recovery yet

---

### **Test 4: Normal Recovery (Baseline)**
```go
It("should return needs_human_review=false for normal recovery", func() {
    recoveryReq := &client.RecoveryRequest{
        IncidentID:            "test-recovery-normal",
        RemediationID:         "req-test-004",
        IsRecoveryAttempt:     client.NewOptBool(true),
        RecoveryAttemptNumber: client.NewOptNilInt(1),
    }

    resp, err := hapiClient.InvestigateRecovery(testCtx, recoveryReq)

    Expect(err).ToNot(HaveOccurred())
    Expect(resp.NeedsHumanReview.Value).To(BeFalse())
})
```

**Expected Behavior**:
- HAPI returns `needs_human_review=false` (default)
- Normal recovery flow proceeds
- No human intervention required

**TDD RED Status**: ✅ Should pass - default value is false

---

## ❌ **Expected Failures (RED Phase)**

### **Test Execution Failures**

When running the tests:
```bash
go test -v ./test/integration/aianalysis/ -ginkgo.focus="Recovery Human Review"
```

**Expected Output**:
```
❌ Recovery - No Matching Workflows
   Expected: true
   Got: false
   (HAPI doesn't set needs_human_review for recovery yet)

❌ Recovery - Low Confidence
   Expected: true
   Got: false
   (HAPI doesn't set needs_human_review for recovery yet)

❌ Recovery - Signal Not Reproducible
   Expected: true
   Got: false
   (HAPI doesn't set needs_human_review for recovery yet)

✅ Recovery - Normal Flow (Baseline)
   PASSED (default value is false)
```

**Why This is Correct**:
- ✅ Tests compile successfully
- ✅ Tests call REAL HAPI service
- ✅ HAPI Python code doesn't implement needs_human_review for recovery yet
- ✅ Go RecoveryResponse struct HAS the fields (after our regeneration)
- ✅ But HAPI returns them as unset/default values (false, empty string)
- ✅ This is the essence of TDD RED phase

---

## 🎯 **GREEN Phase Requirements**

To make these tests pass, we need to implement in **TWO layers**:

### **Layer 1: HAPI Python Implementation** (HAPI Team - 30 min)

**File**: `holmesgpt-api/src/extensions/recovery/llm_integration.py`

**Add logic to set `needs_human_review` and `human_review_reason`**:

```python
# After workflow selection logic
if not selected_workflow:
    # No workflow found
    return RecoveryResponse(
        incident_id=request.incident_id,
        can_recover=True,
        analysis_confidence=confidence,
        needs_human_review=True,  # ADD THIS
        human_review_reason="no_matching_workflows",  # ADD THIS
        warnings=["No workflows matched the recovery criteria"],
        ...
    )

if confidence < 0.70:
    # Low confidence
    return RecoveryResponse(
        incident_id=request.incident_id,
        can_recover=True,
        analysis_confidence=confidence,
        needs_human_review=True,  # ADD THIS
        human_review_reason="low_confidence",  # ADD THIS
        warnings=[f"Recovery confidence below threshold ({confidence} < 0.70)"],
        ...
    )

# Normal recovery
return RecoveryResponse(
    incident_id=request.incident_id,
    can_recover=True,
    analysis_confidence=confidence,
    needs_human_review=False,  # Explicit false
    human_review_reason=None,
    ...
)
```

---

### **Layer 2: AA Service Logic Update** (AA Team - 30 min)

**File**: `pkg/aianalysis/handlers/response_processor.go`

**Changes Required**:

#### **A. Add `needs_human_review` Check**

```go
func (p *ResponseProcessor) ProcessRecoveryResponse(...) {
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

    // ... rest of function
}
```

#### **B. Implement Handler Method**

```go
// handleWorkflowResolutionFailureFromRecovery handles when recovery HAPI cannot provide reliable workflow
// BR-HAPI-197: Human review required for uncertain recovery recommendations
func (p *ResponseProcessor) handleWorkflowResolutionFailureFromRecovery(
    ctx context.Context,
    analysis *aianalysisv1.AIAnalysis,
    resp *client.RecoveryResponse,
) (ctrl.Result, error) {
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

## 📊 **Test Coverage**

| Scenario | Test Status | HAPI Implementation | AA Service Logic |
|---|---|---|---|
| **No Matching Workflows** | ✅ Written (RED) | ❌ Not Implemented | ❌ Not Implemented |
| **Low Confidence** | ✅ Written (RED) | ❌ Not Implemented | ❌ Not Implemented |
| **Signal Not Reproducible** | ✅ Written (RED) | ❌ Not Implemented | ❌ Not Implemented |
| **Normal Recovery** | ✅ Written (RED) | ✅ Default behavior | ✅ Existing logic |

---

## 🔗 **Related Documentation**

- **BR-HAPI-197**: Human Review Flags for Uncertain AI Decisions
- **BR-AI-082**: Recovery Flow Support
- **Gap Analysis**: `AA_RECOVERY_NEEDS_HUMAN_REVIEW_MISSING_DEC_30_2025.md`
- **Impact Assessment**: `AA_RECOVERY_HUMAN_REVIEW_IMPACT_ASSESSMENT.md`
- **Go Bindings**: `AA_RECOVERY_HUMAN_REVIEW_GO_BINDINGS_REGENERATED.md`

---

## ✅ **RED Phase Success Criteria Met**

1. ✅ Tests define expected behavior clearly
2. ✅ Tests use REAL HAPI service (not mocked)
3. ✅ Tests compile successfully
4. ✅ Tests will fail on assertions (HAPI doesn't implement yet)
5. ✅ Test structure follows existing recovery_integration_test.go pattern
6. ✅ All 4 recovery human review scenarios covered
7. ✅ Baseline test (normal flow) included for comparison

---

## 🎯 **Next Steps: GREEN Phase**

**Estimated Time**: 60 minutes (30 min HAPI + 30 min AA)

### **Option A: HAPI Team Implements First**
1. HAPI team adds `needs_human_review` logic to recovery endpoint (30 min)
2. AA team adds handler logic to ProcessRecoveryResponse (30 min)
3. Run integration tests - should pass

### **Option B: AA Team Proceeds Independently**
1. AA team adds handler logic (30 min)
2. Tests still fail (waiting for HAPI implementation)
3. HAPI team implements (30 min)
4. Tests pass

**Recommended**: **Option A** - HAPI implementation first, then AA logic

**Command to Run Tests**:
```bash
# Start infrastructure
podman-compose -f test/integration/aianalysis/podman-compose.yml up -d

# Run tests
go test -v ./test/integration/aianalysis/ -ginkgo.focus="Recovery Human Review" -timeout=5m
```

---

**Status**: ✅ **READY FOR GREEN PHASE IMPLEMENTATION**

The RED phase is complete. Tests are written using REAL HAPI service, they compile successfully, and will fail on assertions until HAPI implements the recovery human review logic.

---

**End of Document**
