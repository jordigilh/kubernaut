# TRIAGE: Workflow-Specific Cooldown V1.0 Implementation Gap

**Date**: December 16, 2025
**Status**: 🔧 **IMPLEMENTATION GAP IDENTIFIED**
**Classification**: V1.0 Implementation Gap (NOT V2.0 Feature)
**Priority**: P2 (Correctness)
**Confidence**: 100%

---

## 🎯 **Executive Summary**

The workflow-specific cooldown functionality was **explicitly designed** in DD-RO-002 (Check 4) but was **simplified during implementation** to target-only matching. This is a V1.0 implementation gap, not a V2.0 feature.

---

## 📋 **Finding Details**

### Authoritative Design (DD-RO-002 Check 4)

**Lines 119-125**:
```go
// Check 4: Workflow Cooldown (TEMPORARY SKIP)
// If same workflow executed recently for same target
if recentWFE := r.findRecentWorkflowExecution(ctx, targetResource, workflowId); recentWFE != nil {
    cooldownRemaining := r.calculateCooldownRemaining(recentWFE)
    return r.markTemporarySkip(ctx, rr, "RecentlyRemediated", recentWFE.Name, &cooldownRemaining,
        fmt.Sprintf("Same workflow executed recently. Cooldown: %s remaining", cooldownRemaining.Sub(time.Now())))
}
```

**Key Observation**: The design explicitly takes **BOTH** `targetResource` AND `workflowId` as parameters.

**Intended Behavior**:
- Same workflow + same target → **BLOCKED** (cooldown)
- Different workflow + same target → **NOT BLOCKED** (different remediation approach)

### Current V1.0 Implementation (blocking.go lines 257-264)

```go
// Day 3 GREEN: Simplified version - block ANY recent remediation on same target
// TODO Day 4 REFACTOR: Add workflow ID matching when RR.Spec.WorkflowRef is available
// For now, pass empty workflow ID to match ANY workflow on the target
recentWFE, err := r.FindRecentCompletedWFE(
    ctx,
    targetResourceStr,
    "", // Empty = match ANY workflow (workflow matching in Day 4)
    r.config.RecentlyRemediatedCooldown,
)
```

**Key Issue**: Empty workflow ID (`""`) matches ANY workflow on the target.

**Current Behavior**:
- Same workflow + same target → BLOCKED ✅
- Different workflow + same target → **INCORRECTLY BLOCKED** ❌

---

## 🔍 **Root Cause Analysis**

### Timeline
1. **Day 3 GREEN**: Simplified implementation with target-only matching
2. **Day 4 REFACTOR**: TODO added for workflow ID matching - **NEVER COMPLETED**
3. **Current State**: Simplified behavior remains in production

### Architectural Misconception in TODO Comment

The TODO comment states:
> "Add workflow ID matching when RR.Spec.WorkflowRef is available"

**This is architecturally incorrect because:**
1. `RR.Spec` is **immutable** - cannot be modified after creation
2. `WorkflowRef` is **never in `RR.Spec`** - the workflow is selected by AIAnalysis AFTER RR creation
3. The workflow ID is available from `AIAnalysis.Status.SelectedWorkflow.WorkflowID` at the point where `CheckRecentlyRemediated` is called

### Correct Architecture

In `handleAnalyzingPhase` (reconciler.go), when routing checks are performed:
1. AIAnalysis `ai` object is already fetched (lines 391-403)
2. `ai.Status.SelectedWorkflow.WorkflowID` is available
3. This ID should be passed to `CheckBlockingConditions` → `CheckRecentlyRemediated`

---

## ✅ **Correct Fix (V1.0)**

### 1. Update `CheckRecentlyRemediated` Signature

```go
// Before (V1.0 simplified):
func (r *RoutingEngine) CheckRecentlyRemediated(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
) (*BlockingCondition, error)

// After (V1.0 complete):
func (r *RoutingEngine) CheckRecentlyRemediated(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    workflowID string,
) (*BlockingCondition, error)
```

### 2. Update Reconciler Call Site

```go
// In handleAnalyzingPhase, after AIAnalysis is fetched:
workflowID := ""
if ai.Status.SelectedWorkflow != nil {
    workflowID = ai.Status.SelectedWorkflow.WorkflowID
}
blocked, err := r.routingEngine.CheckBlockingConditions(ctx, rr, workflowID)
```

### 3. Pass Through CheckBlockingConditions

```go
func (r *RoutingEngine) CheckBlockingConditions(
    ctx context.Context,
    rr *remediationv1.RemediationRequest,
    workflowID string,  // NEW: From AIAnalysis.Status.SelectedWorkflow.WorkflowID
) (*BlockingCondition, error) {
    // ... other checks ...

    // Check 4: Recently remediated (with workflow matching)
    blocked, err = r.CheckRecentlyRemediated(ctx, rr, workflowID)
    // ...
}
```

---

## 📊 **Impact Assessment**

| Aspect | Impact |
|--------|--------|
| **Current Behavior** | Blocks different workflows on same target (overly conservative) |
| **Correct Behavior** | Only blocks same workflow on same target |
| **User Impact** | Potential unnecessary delays in remediation diversity |
| **Risk Level** | LOW (current behavior is safe, just overly restrictive) |
| **Breaking Change** | NO (relaxes blocking, doesn't add new blocks) |

---

## 🧪 **TDD Implementation Plan**

### RED Phase
1. Activate `PIt()` test → `It()` with correct architecture comment
2. Test expects `blocked == nil` for different workflow on same target
3. Test will FAIL (current implementation blocks ANY workflow)

### GREEN Phase
1. Add `workflowID` parameter to `CheckRecentlyRemediated`
2. Add `workflowID` parameter to `CheckBlockingConditions`
3. Update reconciler to pass `ai.Status.SelectedWorkflow.WorkflowID`
4. Pass workflow ID through to `FindRecentCompletedWFE`
5. Test will PASS

### REFACTOR Phase
1. Remove incorrect TODO comment
2. Verify all callers updated
3. Run full test suite

---

## 📚 **References**

- **DD-RO-002**: Centralized Routing Responsibility (lines 119-125)
- **BR-WE-010**: Cooldown - Prevent Redundant Sequential Execution
- **blocking.go**: Current implementation (lines 250-287)
- **blocking_test.go**: Test case (lines 582-626)

---

## ✅ **Approval**

**Classification**: V1.0 Implementation Gap
**Action Required**: Complete workflow-specific cooldown per DD-RO-002 design
**Confidence**: 100% (design explicitly specifies workflow ID matching)

---

**Document Version**: 1.0
**Last Updated**: December 16, 2025

