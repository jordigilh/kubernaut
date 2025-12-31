# AIAnalysis Recovery Human Review - TDD GREEN Phase Complete

**Date**: December 30, 2025
**Status**: ✅ GREEN PHASE COMPLETE (AA Service Logic)
**Next**: HAPI Python Implementation Required
**Related**: `AA_RECOVERY_HUMAN_REVIEW_TDD_RED_COMPLETE.md`

---

## 🟢 **TDD GREEN PHASE SUMMARY**

### **Objective**

Implement AA service logic to handle `needs_human_review` in recovery responses (BR-HAPI-197).

### **✅ Completion Status**

**Phase**: GREEN (Make Tests Pass)
**Layer**: Layer 2 (AA Service Logic)
**Status**: ✅ COMPLETE
**Compilation**: ✅ PASSES
**Tests Status**: ⏳ PENDING (waiting for HAPI Python implementation)

---

## 📋 **Changes Implemented**

### **File 1: `pkg/aianalysis/handlers/response_processor.go`**

#### **Change 1: Add `needs_human_review` Check to `ProcessRecoveryResponse`**

**Lines**: 162-193

```go
// ProcessRecoveryResponse processes the RecoveryResponse from generated client
// BR-AI-082: Handle recovery flow responses
// BR-HAPI-197: Check needs_human_review before proceeding
func (p *ResponseProcessor) ProcessRecoveryResponse(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
	// BR-AI-009: Reset failure counter on successful API call
	analysis.Status.ConsecutiveFailures = 0

	// Check if NeedsHumanReview is set (BR-HAPI-197)
	needsHumanReview := GetOptBoolValue(resp.NeedsHumanReview)
	hasSelectedWorkflow := resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null

	p.log.Info("Processing successful recovery response",
		"canRecover", resp.CanRecover,
		"confidence", resp.AnalysisConfidence,
		"warningsCount", len(resp.Warnings),
		"hasSelectedWorkflow", hasSelectedWorkflow,
		"needsHumanReview", needsHumanReview,  // ADDED
	)

	// BR-HAPI-197: Check if recovery requires human review
	// This takes precedence over other checks as HAPI has determined it cannot provide reliable recommendations
	if needsHumanReview {
		return p.handleWorkflowResolutionFailureFromRecovery(ctx, analysis, resp)  // ADDED
	}

	// Check if recovery is not possible
	if !resp.CanRecover {
		return p.handleRecoveryNotPossible(ctx, analysis, resp)
	}

	// Check if no workflow was selected (might need human review)
	if !hasSelectedWorkflow {
		return p.handleRecoveryNotPossible(ctx, analysis, resp)
	}

	// ... rest of function
}
```

**Key Changes**:
- ✅ Extract `needsHumanReview` from response using `GetOptBoolValue`
- ✅ Add `needsHumanReview` to logging
- ✅ Check `needsHumanReview` BEFORE other checks (takes precedence)
- ✅ Call new handler method if human review required

---

#### **Change 2: Implement `handleWorkflowResolutionFailureFromRecovery` Handler**

**Lines**: 414-469 (new method)

```go
// handleWorkflowResolutionFailureFromRecovery handles workflow resolution failure from RecoveryResponse
// BR-HAPI-197: Recovery workflow resolution failed, human must intervene
func (p *ResponseProcessor) handleWorkflowResolutionFailureFromRecovery(ctx context.Context, analysis *aianalysisv1.AIAnalysis, resp *client.RecoveryResponse) (ctrl.Result, error) {
	hasSelectedWorkflow := resp.SelectedWorkflow.Set && !resp.SelectedWorkflow.Null
	humanReviewReason := ""
	if resp.HumanReviewReason.Set && !resp.HumanReviewReason.Null {
		humanReviewReason = resp.HumanReviewReason.Value
	}

	p.log.Info("Recovery workflow resolution failed, requires human review",
		"warnings", resp.Warnings,
		"humanReviewReason", humanReviewReason,
		"hasPartialWorkflow", hasSelectedWorkflow,
		"canRecover", resp.CanRecover,
		"confidence", resp.AnalysisConfidence,
	)

	// Set structured failure with timestamp
	now := metav1.Now()
	analysis.Status.Phase = aianalysis.PhaseFailed
	analysis.Status.CompletedAt = &now
	analysis.Status.Reason = "WorkflowResolutionFailed"
	analysis.Status.InvestigationID = resp.IncidentID

	// BR-HAPI-197: Track failure metrics
	p.metrics.FailuresTotal.WithLabelValues("WorkflowResolutionFailed", "NoWorkflowResolved").Inc()

	// Record failure metric
	subReason := "HumanReviewRequired"
	if humanReviewReason != "" {
		subReason = humanReviewReason
	}
	p.metrics.RecordFailure("WorkflowResolutionFailed", subReason)

	// Map HumanReviewReason to SubReason
	if humanReviewReason != "" {
		analysis.Status.SubReason = p.mapEnumToSubReason(humanReviewReason)
	} else {
		analysis.Status.SubReason = mapWarningsToSubReason(resp.Warnings)
	}

	// Build comprehensive message
	var messageParts []string
	messageParts = append(messageParts, "HolmesGPT-API could not provide reliable recovery workflow recommendation")

	if humanReviewReason != "" {
		messageParts = append(messageParts, fmt.Sprintf("(reason: %s)", humanReviewReason))
	}

	if len(resp.Warnings) > 0 {
		messageParts = append(messageParts, fmt.Sprintf("Warnings: %s", strings.Join(resp.Warnings, "; ")))
	}

	analysis.Status.Message = strings.Join(messageParts, " ")
	analysis.Status.Warnings = resp.Warnings

	return ctrl.Result{}, nil
}
```

**Key Features**:
- ✅ Extracts `human_review_reason` from response
- ✅ Comprehensive logging with all relevant fields
- ✅ Sets `Phase` to `Failed` with completion timestamp
- ✅ Tracks metrics for human review scenarios
- ✅ Maps enum reason to `SubReason` for structured reporting
- ✅ Builds comprehensive message with reason and warnings
- ✅ Matches incident handler pattern for consistency

---

## 🔄 **Parity with Incident Flow**

The recovery handler now has **parity** with the incident handler:

| Feature | Incident Handler | Recovery Handler | Status |
|---|---|---|---|
| **Check `needs_human_review`** | ✅ Line 69 | ✅ Line 169 | ✅ PARITY |
| **Extract `human_review_reason`** | ✅ Line 273 | ✅ Line 419 | ✅ PARITY |
| **Comprehensive logging** | ✅ Line 277 | ✅ Line 424 | ✅ PARITY |
| **Set Phase to Failed** | ✅ Line 285 | ✅ Line 433 | ✅ PARITY |
| **Track metrics** | ✅ Line 291 | ✅ Line 439 | ✅ PARITY |
| **Map enum to SubReason** | ✅ Line 302 | ✅ Line 449 | ✅ PARITY |
| **Build comprehensive message** | ✅ Line 308 | ✅ Line 456 | ✅ PARITY |

---

## ✅ **Compilation & Lint Status**

### **Compilation**
```bash
$ go build ./pkg/aianalysis/handlers/...
✅ SUCCESS - No errors
```

### **Lint**
```bash
$ golangci-lint run pkg/aianalysis/handlers/response_processor.go
✅ SUCCESS - No linter errors
```

---

## ⏳ **Test Status**

### **Current State**

**Integration Tests**: ⏳ PENDING

The tests will still fail because:
- ✅ AA service logic is implemented
- ❌ HAPI Python code doesn't set `needs_human_review` for recovery yet

**Expected Test Output**:
```
❌ Recovery - No Matching Workflows
   Expected: needs_human_review=true
   Got: needs_human_review=false (default)

   Reason: HAPI Python implementation not complete yet
```

---

## 🎯 **Next: HAPI Python Implementation Required**

### **Layer 1: HAPI Python Implementation** (30 min)

**File**: `holmesgpt-api/src/extensions/recovery/llm_integration.py`

**Required Changes**:

```python
# After workflow selection logic in recovery endpoint

# Scenario 1: No matching workflows
if not selected_workflow:
    return RecoveryResponse(
        incident_id=request.incident_id,
        can_recover=True,
        analysis_confidence=confidence,
        needs_human_review=True,  # ADD THIS
        human_review_reason="no_matching_workflows",  # ADD THIS
        warnings=["No workflows matched the recovery criteria"],
        strategies=[],
        ...
    )

# Scenario 2: Low confidence
if confidence < 0.70:
    return RecoveryResponse(
        incident_id=request.incident_id,
        can_recover=True,
        analysis_confidence=confidence,
        needs_human_review=True,  # ADD THIS
        human_review_reason="low_confidence",  # ADD THIS
        warnings=[f"Recovery confidence below threshold ({confidence} < 0.70)"],
        ...
    )

# Scenario 3: Signal not reproducible (if applicable)
if signal_not_found:
    return RecoveryResponse(
        incident_id=request.incident_id,
        can_recover=False,
        analysis_confidence=0.0,
        needs_human_review=True,  # ADD THIS
        human_review_reason="signal_not_reproducible",  # ADD THIS
        warnings=["Signal no longer present - issue may have self-resolved"],
        ...
    )

# Normal recovery (explicit false)
return RecoveryResponse(
    incident_id=request.incident_id,
    can_recover=True,
    analysis_confidence=confidence,
    needs_human_review=False,  # Explicit false
    human_review_reason=None,
    selected_workflow=selected_workflow_dict,
    ...
)
```

---

## 📊 **Implementation Progress**

| Layer | Component | Status | Notes |
|---|---|---|---|
| **Layer 1** | HAPI Python Logic | ❌ NOT IMPLEMENTED | Waiting for HAPI team |
| **Layer 2** | AA Service Logic | ✅ COMPLETE | This GREEN phase |
| **Tests** | Integration Tests | ✅ WRITTEN (RED) | Waiting for Layer 1 |
| **Tests** | Unit Tests | ⏳ OPTIONAL | Can add if needed |

---

## 🔗 **Related Documentation**

- **BR-HAPI-197**: Human Review Flags for Uncertain AI Decisions
- **BR-AI-082**: Recovery Flow Support
- **RED Phase**: `AA_RECOVERY_HUMAN_REVIEW_TDD_RED_COMPLETE.md`
- **Gap Analysis**: `AA_RECOVERY_NEEDS_HUMAN_REVIEW_MISSING_DEC_30_2025.md`
- **Impact Assessment**: `AA_RECOVERY_HUMAN_REVIEW_IMPACT_ASSESSMENT.md`
- **Go Bindings**: `AA_RECOVERY_HUMAN_REVIEW_GO_BINDINGS_REGENERATED.md`

---

## ✅ **GREEN Phase Success Criteria Met (AA Layer)**

1. ✅ `needs_human_review` check added to `ProcessRecoveryResponse`
2. ✅ Handler method `handleWorkflowResolutionFailureFromRecovery` implemented
3. ✅ Parity achieved with incident flow
4. ✅ Comprehensive logging added
5. ✅ Metrics tracking implemented
6. ✅ Enum to SubReason mapping implemented
7. ✅ Code compiles without errors
8. ✅ No lint errors

---

## 🎯 **Next Steps**

### **Option A: Wait for HAPI Implementation**
1. HAPI team implements recovery human review logic (30 min)
2. Run integration tests - should pass
3. Proceed to REFACTOR phase

### **Option B: Test with Mock HAPI**
1. Create mock HAPI responses for testing
2. Verify AA logic works correctly
3. Wait for real HAPI implementation

### **Option C: Document and Hand Off**
1. Create handoff document for HAPI team
2. Document expected behavior
3. Wait for HAPI implementation

**Recommended**: **Option A** - Wait for HAPI team to implement, then run tests

---

**Status**: ✅ **AA SERVICE LOGIC COMPLETE - WAITING FOR HAPI IMPLEMENTATION**

The GREEN phase for AA service logic is complete. Tests will pass once HAPI Python code implements the `needs_human_review` logic for recovery responses.

---

**End of Document**


