# AIAnalysis Integration Test Failures - Reassessment

**Date**: February 3, 2026  
**Status**: ✅ **ROOT CAUSE CORRECTED** - Issues are AIAnalysis controller bugs, NOT HAPI bugs  
**Credit**: HAPI team identified the correct root cause by referencing BR-HAPI-197

---

## 🎯 Executive Summary

**Initial Assessment** (INCORRECT): Identified 3 HAPI bugs preventing tests from passing.

**Reassessment** (CORRECT): All 3 failures are **AIAnalysis controller bugs**. HAPI is working as designed per BR-HAPI-197.

**Key Finding**: Per BR-HAPI-197, HAPI returns confidence scores but does NOT enforce thresholds. **AIAnalysis controller** must apply the 70% threshold and detect terminal failures.

---

## 📚 **Authoritative Documentation**

### **BR-HAPI-197 - Acceptance Criteria 4**

**Lines 212-220** ([Permanent Link](https://github.com/jordigilh/kubernaut/blob/accd841ac0aa6d1ac503a207377d53ef176c1cf4/docs/requirements/BR-HAPI-197-needs-human-review-field.md#L212-L233)):

```gherkin
Given an IncidentRequest is submitted
When the API returns a response
Then the "selected_workflow.confidence" field SHALL contain the AI confidence score (0.0-1.0)
And the consuming service (AIAnalysis) SHALL apply its configured threshold
```

**Note**: HAPI returns `confidence` but does NOT enforce thresholds. AIAnalysis owns the threshold logic (V1.0: global 70% default, V1.1: operator-configurable).

**Note**: `needs_human_review` is only set by HAPI for validation failures, not confidence thresholds.

---

## 🔍 **Corrected Root Cause Analysis**

### **Failure #1: Low Confidence Not Triggering Human Review**

**Initial Assessment** ❌: HAPI bug - doesn't set `needs_human_review=true` for confidence < 0.7

**Corrected Assessment** ✅: **AIAnalysis controller bug** - doesn't check confidence < 0.7

**Evidence from AIAnalysis Controller** (`pkg/aianalysis/handlers/response_processor.go`):

```go
// Line 84-90: Check for problem resolved (confidence >= 0.7, no workflow)
if !hasSelectedWorkflow && resp.Confidence >= 0.7 {
    return p.handleProblemResolvedFromIncident(ctx, analysis, resp)
}

// Line 92-96: Check for HAPI validation failures
if needsHumanReview {
    return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
}

// Line 122+: Store selected workflow and CONTINUE
// ❌ MISSING: Check for confidence < 0.7 WITH selected workflow!
if hasSelectedWorkflow {
    // Stores workflow and proceeds to Analyzing phase
    // BUG: Doesn't check if confidence < 0.7
}
```

**The Bug**: AIAnalysis controller never checks `hasSelectedWorkflow && confidence < 0.7`.

**Expected Behavior** (Per BR-HAPI-197):
1. HAPI returns `confidence: 0.35` with `selected_workflow: { workflow_id: "..." }` ✅ (HAPI is correct)
2. HAPI does NOT set `needs_human_review=true` ✅ (HAPI is correct - not a validation failure)
3. AIAnalysis controller checks `confidence < 0.7` ❌ (AIAnalysis MISSING this check!)
4. AIAnalysis transitions to `Failed` phase with `LowConfidence` subreason ❌ (AIAnalysis doesn't do this)

**Actual Behavior**:
1. HAPI returns `confidence: 0.35` with workflow ✅
2. HAPI does NOT set `needs_human_review` ✅
3. AIAnalysis controller skips check ❌ (bug!)
4. AIAnalysis stores workflow and proceeds to `Analyzing` phase ❌ (should fail!)

**Fix Location**: `pkg/aianalysis/handlers/response_processor.go` line ~96

**Required Fix**:
```go
// BR-HAPI-200 Outcome A: Problem confidently resolved, no workflow needed
if !hasSelectedWorkflow && resp.Confidence >= 0.7 {
    return p.handleProblemResolvedFromIncident(ctx, analysis, resp)
}

// BR-HAPI-197: Check if HAPI flagged for human review (validation failures)
if needsHumanReview {
    return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
}

// ✅ ADD THIS: BR-HAPI-197 AC-4: AIAnalysis applies confidence threshold (V1.0: 70%)
if hasSelectedWorkflow && resp.Confidence < 0.7 {
    p.log.Info("Low confidence workflow, requires human review",
        "confidence", resp.Confidence,
        "threshold", 0.7)
    
    // Set structured failure
    now := metav1.Now()
    analysis.Status.Phase = aianalysis.PhaseFailed
    analysis.Status.Reason = "WorkflowResolutionFailed"
    analysis.Status.SubReason = aianalysis.SubReasonLowConfidence
    analysis.Status.CompletedAt = &now
    analysis.Status.NeedsHumanReview = true
    analysis.Status.HumanReviewReason = "low_confidence"
    
    // Emit audit event
    p.auditClient.EmitAnalysisFailedEvent(ctx, analysis, "low_confidence")
    
    return ctrl.Result{}, nil
}

// Store HAPI response metadata (only if checks above passed)
// ...
```

---

### **Failure #2: No Workflow Not Triggering Terminal Failure**

**Initial Assessment** ❌: HAPI bug - doesn't set `needs_human_review=true` when `workflow_id=""`

**Corrected Assessment** ✅: **AIAnalysis controller bug** - doesn't detect terminal failure per BR-AI-050

**Evidence from AIAnalysis Controller** (`pkg/aianalysis/handlers/response_processor.go`):

```go
// Line 84-90: Check for problem resolved (confidence >= 0.7, no workflow)
if !hasSelectedWorkflow && resp.Confidence >= 0.7 {
    return p.handleProblemResolvedFromIncident(ctx, analysis, resp) // SUCCESS
}

// Line 92-96: Check for HAPI validation failures
if needsHumanReview {
    return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
}

// ❌ MISSING: Check for !hasSelectedWorkflow && confidence < 0.7 (terminal failure)
// Line 122+: Proceeds assuming workflow exists
```

**The Bug**: AIAnalysis controller doesn't check `!hasSelectedWorkflow && confidence < 0.7`.

**Expected Behavior** (Per BR-HAPI-197 + BR-AI-050):
1. HAPI returns `selected_workflow: null`, `confidence: 0` ✅ (HAPI is correct)
2. HAPI does NOT set `needs_human_review=true` ✅ (HAPI is correct - LLM legitimately found no workflow)
3. AIAnalysis controller detects `selected_workflow == null` ❌ (AIAnalysis MISSING this check!)
4. AIAnalysis transitions to `Failed` phase with `NoMatchingWorkflows` subreason per BR-AI-050 ❌ (doesn't happen)

**Actual Behavior**:
1. HAPI returns `selected_workflow: null` ✅
2. HAPI does NOT set `needs_human_review` ✅
3. AIAnalysis controller checks `!hasSelectedWorkflow && confidence >= 0.7` → false (confidence is 0)
4. AIAnalysis controller proceeds past all checks ❌ (bug!)
5. AIAnalysis stays in `Analyzing` phase ❌ (should transition to Failed!)

**Fix Location**: `pkg/aianalysis/handlers/response_processor.go` line ~96

**Required Fix**:
```go
// BR-HAPI-200 Outcome A: Problem confidently resolved, no workflow needed
if !hasSelectedWorkflow && resp.Confidence >= 0.7 {
    return p.handleProblemResolvedFromIncident(ctx, analysis, resp)
}

// BR-HAPI-197: Check if HAPI flagged for human review (validation failures)
if needsHumanReview {
    return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
}

// ✅ ADD THIS: BR-AI-050: No workflow found (terminal failure)
if !hasSelectedWorkflow {
    // confidence < 0.7, no workflow → terminal failure
    p.log.Info("No workflow selected, terminal failure",
        "confidence", resp.Confidence)
    
    // Set structured failure
    now := metav1.Now()
    analysis.Status.Phase = aianalysis.PhaseFailed
    analysis.Status.Reason = "WorkflowResolutionFailed"
    analysis.Status.SubReason = aianalysis.SubReasonNoMatchingWorkflows
    analysis.Status.CompletedAt = &now
    analysis.Status.NeedsHumanReview = true
    analysis.Status.HumanReviewReason = "no_matching_workflows"
    
    // BR-AI-050: Emit audit event for terminal failure
    p.auditClient.EmitAnalysisFailedEvent(ctx, analysis, "no_matching_workflows")
    
    return ctrl.Result{}, nil
}

// Store HAPI response metadata (only if checks above passed)
// ...
```

---

### **Failure #3: Alternative Workflows Missing from Audit**

**Initial Assessment** ✅: HAPI bug - doesn't extract `alternative_workflows` from LLM response

**Reassessment Status**: ✅ **CONFIRMED AS HAPI BUG** (per HAPI team triage in Issue #27)

**HAPI Team Findings**:
- **Incident Endpoint**: Field EXISTS in model, parser EXTRACTS it, but test gets `nil` instead of empty array
  - Root Cause: Serialization issue - `response_model_exclude_none=True` may exclude empty lists
  - Fix: Ensure empty list is preserved in serialization
- **Recovery Endpoint**: Field is MISSING entirely - not implemented per ADR-045 v1.2
  - Root Cause: Feature not yet implemented
  - Fix: Add `alternative_workflows` field to RecoveryResponse model and parser

**Action**: Keep Issue #27 open - HAPI team will implement fixes

---

## 🔧 **Required Fixes**

### **Priority 1: AIAnalysis Controller Fixes**

**File**: `pkg/aianalysis/handlers/response_processor.go`

**Current Code** (lines 84-96):
```go
// BR-HAPI-200 Outcome A: Problem confidently resolved, no workflow needed
if !hasSelectedWorkflow && resp.Confidence >= 0.7 {
    return p.handleProblemResolvedFromIncident(ctx, analysis, resp)
}

// BR-HAPI-197: Check if workflow resolution failed
if needsHumanReview {
    return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
}

// Store HAPI response metadata
analysis.Status.Warnings = resp.Warnings
// ... continues to store workflow and proceed
```

**Required Fix** (insert after line 96, before line 98):
```go
// BR-HAPI-200 Outcome A: Problem confidently resolved, no workflow needed
if !hasSelectedWorkflow && resp.Confidence >= 0.7 {
    return p.handleProblemResolvedFromIncident(ctx, analysis, resp)
}

// BR-HAPI-197: Check if workflow resolution failed (validation failures)
if needsHumanReview {
    return p.handleWorkflowResolutionFailureFromIncident(ctx, analysis, resp)
}

// ✅ ADD THIS BLOCK: BR-HAPI-197 AC-4 + BR-AI-050
// AIAnalysis must apply confidence threshold and detect terminal failures

// Check 1: Low confidence (confidence < 0.7 WITH workflow selected)
if hasSelectedWorkflow && resp.Confidence < 0.7 {
    return p.handleLowConfidenceFailure(ctx, analysis, resp)
}

// Check 2: No workflow found (terminal failure per BR-AI-050)
if !hasSelectedWorkflow {
    // Confidence < 0.7, no workflow → terminal failure
    return p.handleNoWorkflowTerminalFailure(ctx, analysis, resp)
}

// All checks passed, store HAPI response metadata
analysis.Status.Warnings = resp.Warnings
// ... continue processing
```

**New Helper Methods Needed**:
1. `handleLowConfidenceFailure()` - Sets `Phase=Failed`, `SubReason=LowConfidence`
2. `handleNoWorkflowTerminalFailure()` - Sets `Phase=Failed`, `SubReason=NoMatchingWorkflows`

---

## 📊 **Architecture Clarity**

### **HAPI Responsibilities** (Per BR-HAPI-197)

✅ **What HAPI DOES**:
- Return `confidence` score (0.0-1.0)
- Return `selected_workflow` or `null`
- Set `needs_human_review=true` for **validation failures**:
  - Workflow ID in response but doesn't exist in catalog
  - LLM parsing errors after max retries
  - Image mismatch validation failed
  - Parameter validation failed

❌ **What HAPI DOES NOT DO**:
- Enforce confidence thresholds (AIAnalysis's job)
- Set `needs_human_review` for low confidence (AIAnalysis's job)
- Set `needs_human_review` when LLM legitimately finds no workflow (AIAnalysis's job per BR-AI-050)

### **AIAnalysis Controller Responsibilities** (Per BR-HAPI-197 AC-4 + BR-AI-050)

✅ **What AIAnalysis MUST DO**:
- Apply confidence threshold (70% in V1.0, configurable in V1.1 per BR-HAPI-198)
- Detect `confidence < 0.7` WITH workflow → transition to `Failed` with `LowConfidence` subreason
- Detect `selected_workflow == null` → transition to `Failed` per BR-AI-050
- Emit audit events for terminal failures

❌ **What AIAnalysis is MISSING** (BUGS):
- Low confidence check (`confidence < 0.7` with workflow)
- No workflow terminal failure check (when confidence < 0.7)

---

## 🎯 **Corrected Issue Ownership**

| Issue | Initial Assessment | Corrected Assessment | Owner | Status |
|-------|-------------------|----------------------|-------|---------|
| #25: Low confidence | HAPI bug | **AIAnalysis controller bug** | AIAnalysis team | Closed (HAPI correct) |
| #26: No workflow | HAPI bug | **AIAnalysis controller bug** | AIAnalysis team | Closed (HAPI correct) |
| #27: Alternative workflows | HAPI bug | **Confirmed HAPI bug** (serialization) | HAPI team | Open |

---

## ✅ **Next Steps**

### **For AIAnalysis Team** (This Session)

1. ✅ Close issues #25 and #26 as "NOT A BUG" - HAPI working as designed
2. 🔧 **Create NEW issue**: AIAnalysis controller missing confidence threshold check
3. 🔧 **Create NEW issue**: AIAnalysis controller missing no-workflow terminal failure check
4. 💻 **Implement fixes** in `pkg/aianalysis/handlers/response_processor.go`
5. 🧪 **Update tests** to match correct architecture (HAPI doesn't set `needs_human_review` for confidence/no-workflow)

### **For HAPI Team**

1. ✅ Issue #27 triaged - confirmed as HAPI serialization bug
2. 🔧 **Phase 1**: Fix incident endpoint serialization (empty list → `nil`)
3. 🔧 **Phase 2**: Implement recovery endpoint `alternative_workflows` support

---

## 🎓 **Lessons Learned**

1. **Always check authoritative docs first**: BR-HAPI-197 clearly states AIAnalysis applies thresholds, not HAPI
2. **Architecture boundaries matter**: HAPI returns data, AIAnalysis enforces policy
3. **Test expectations must match architecture**: Tests were expecting HAPI behavior that violates design
4. **HAPI team review was crucial**: They caught the misunderstanding by referencing authoritative docs

---

**Status**: ✅ **Reassessment Complete**  
**Credit**: HAPI team for catching the architecture violation  
**Next**: Fix AIAnalysis controller bugs and update tests
