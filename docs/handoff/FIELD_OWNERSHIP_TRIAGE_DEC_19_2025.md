# Field Ownership Triage: ConsecutiveFailures & NextAllowedExecution

**Date**: December 19, 2025
**Question**: Which service should own these fields - RO or WE?
**Answer**: ✅ **RO (RemediationRequest.Status)** - AUTHORITATIVE
**Confidence**: 100%

---

## Executive Summary

**Authoritative Answer**: Based on DD-RO-002, these fields **MUST** belong to **RemediationRequest.Status** (owned by RO), **NOT** WorkflowExecution.Status.

**Current Implementation**: ✅ **CORRECT** - RO tracks in `RR.Status.ConsecutiveFailureCount` and `RR.Status.NextAllowedExecution`

**WFE Fields Status**: ❌ **VESTIGIAL** - `WFE.Status.ConsecutiveFailures` and `WFE.Status.NextAllowedExecution` should be removed in Phase 3

---

## Authoritative Documentation Analysis

### DD-RO-002: Core Principle (Lines 17-22)

```
RO routes. Executors execute.

If created → execute
If not created → routing decision already made
```

**Interpretation**:
- **Routing state** = RO
- **Execution state** = WE

**Question**: Are ConsecutiveFailures/NextAllowedExecution routing or execution state?
**Answer**: **ROUTING STATE** - They determine WHETHER to execute, not HOW to execute.

---

### DD-RO-002: Problem Statement (Lines 32-38)

```markdown
| Decision Type         | Current Owner | Problem |
|-----------------------|---------------|---------|
| Exponential backoff   | **WE**        | ❌ Executor making routing decisions |
| Exhausted retries     | **WE**        | ❌ Executor making routing decisions |
```

**Clear Statement**: Having these in **WE is explicitly identified as THE PROBLEM**.

**Solution** (Line 57):
> "RemediationOrchestrator makes ALL routing decisions BEFORE creating WorkflowExecution."

---

### DD-RO-002: Routing Checks Table (Lines 65-72)

```markdown
| Check                 | Type      | Action                                      |
|-----------------------|-----------|---------------------------------------------|
| 3. Exponential Backoff| TEMPORARY | Set `SkipReason: "ExponentialBackoff"`, `BlockedUntil` |
| 2. Exhausted Retries  | PERMANENT | Set `SkipReason: "ExhaustedRetries"`        |
```

**Key Insight**: These are **RO routing checks**. The data they operate on must be in RR.Status.

---

### DD-RO-002: Phase 3 WE Simplification (Lines 351-360)

```markdown
### Phase 3: WE Simplification (Days 6-7) - ⏳ NOT STARTED

- [ ] Remove `CheckCooldown()` function (~140 lines)
- [ ] Remove routing logic from WE
- [ ] Update WE unit tests for new architecture
```

**Implication**: WE's routing logic (including backoff tracking) is scheduled for REMOVAL.

---

## Field-by-Field Ownership

### Field 1: ConsecutiveFailures / ConsecutiveFailureCount

**Owner**: ✅ **RO (RemediationRequest.Status)**

**Rationale**:
1. **Used for routing decision**: Determines if workflow should be created
2. **DD-RO-002 Line 107-110**: RO checks "if ≥3 consecutive failures for this fingerprint"
3. **Routing state, not execution state**: WE doesn't use this for execution logic

**API Location**: `api/remediation/v1alpha1/remediationrequest_types.go:551`
```go
// ConsecutiveFailureCount tracks how many times this fingerprint has failed consecutively.
// Reset to 0 when remediation succeeds
// Used by routing engine to block after threshold (BR-ORCH-042)
// +optional
ConsecutiveFailureCount int32 `json:"consecutiveFailureCount,omitempty"`
```

**Comment Confirms**: "Used by routing engine" = RO ownership

---

### Field 2: NextAllowedExecution

**Owner**: ✅ **RO (RemediationRequest.Status)**

**Rationale**:
1. **Used for routing decision**: Determines WHEN workflow should be created
2. **DD-RO-002 Line 114-117**: RO checks "if backoffUntil... backoffUntil != nil"
3. **Routing timing, not execution timing**: WE doesn't control when it's created

**API Location**: `api/remediation/v1alpha1/remediationrequest_types.go:544`
```go
// NextAllowedExecution indicates when this RR can be retried after exponential backoff.
// Set by: RO controller when marking RR as Failed
// Used by: RO routing engine CheckExponentialBackoff
// +optional
NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`
```

**Comment Confirms**: "Used by: RO routing engine" = RO ownership

---

## Why WFE Fields Exist (Historical Context)

### Timeline

1. **Pre-DD-RO-002**: BR-WE-012 implemented in WE controller
   - WE tracked ConsecutiveFailures
   - WE calculated NextAllowedExecution
   - WE made routing decisions (skip if backoff active)

2. **DD-RO-002 Phase 2** (Dec 15-19, 2025): Routing moved to RO
   - RO added RR.Status.ConsecutiveFailureCount
   - RO added RR.Status.NextAllowedExecution
   - RO routing checks implemented
   - **WE fields NOT removed** (Phase 3 pending)

3. **Current State** (Dec 19, 2025):
   - ✅ RO tracks in RR.Status (CORRECT)
   - ❌ WE still tracks in WFE.Status (VESTIGIAL)
   - Result: Duplicate tracking

---

## Correct Architecture (Per DD-RO-002)

### Data Flow

```
1. WFE fails (pre-execution)
   ↓
2. WE: WFE.Status.Phase = Failed
   WE: WFE.Status.FailureDetails.WasExecutionFailure = false
   ↓
3. RO: Detects WFE failure (via aggregator)
   RO: RR.Status.ConsecutiveFailureCount++  ✅ RO's field
   RO: Calculates backoff
   RO: RR.Status.NextAllowedExecution = T+1min  ✅ RO's field
   ↓
4. New signal arrives → New RR created
   ↓
5. RO Routing: CheckExponentialBackoff()
   RO: Reads RR.Status.NextAllowedExecution  ✅ RO's field
   RO: Decision: Skip creating WFE (backoff active)
   RO: RR.Status.BlockReason = "ExponentialBackoff"
```

**Note**: WFE fields not involved in routing decision.

---

## What WE Should Track (Execution State)

**WE's Legitimate State** (things WE needs for execution):

| Field | Purpose | Why WE Owns It |
|---|---|---|
| `Phase` | Pending/Running/Completed/Failed | WE tracks execution lifecycle |
| `StartTime` | When PipelineRun started | WE has timing information |
| `CompletionTime` | When PipelineRun completed | WE has timing information |
| `Duration` | How long execution took | Derived from WE's timings |
| `PipelineRunRef` | Which PipelineRun was created | WE creates PipelineRuns |
| `FailureDetails.WasExecutionFailure` | Pre-execution vs execution | WE categorizes failure type |
| `FailureDetails.Reason` | Why it failed | WE has PipelineRun details |

**Routing State** (NOT WE's responsibility):
| Field | Belongs To | Why |
|---|---|---|
| `ConsecutiveFailures` | ❌ RO (RR.Status) | Routing decision input |
| `NextAllowedExecution` | ❌ RO (RR.Status) | Routing decision input |

---

## Phase 3 Cleanup Plan

### Step 1: Deprecate WFE Fields

**File**: `api/workflowexecution/v1alpha1/workflowexecution_types.go`

```go
// ConsecutiveFailures tracks consecutive failures
// DEPRECATED (V1.1): Routing state moved to RR per DD-RO-002 Phase 3
// Use RR.Status.ConsecutiveFailureCount instead
// This field will be removed in V2.0
// +optional
ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

// NextAllowedExecution timestamp
// DEPRECATED (V1.1): Routing state moved to RR per DD-RO-002 Phase 3
// Use RR.Status.NextAllowedExecution instead
// This field will be removed in V2.0
// +optional
NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`
```

### Step 2: Remove WE Controller Logic

**File**: `internal/controller/workflowexecution/workflowexecution_controller.go`

**Remove**:
- Lines 903-925: Exponential backoff calculation
- Lines 810-812: Reset counter on success
- Line 928: Comment about PreviousExecutionFailed check

**Rationale**: DD-RO-002 Phase 3 - WE becomes pure executor

### Step 3: Remove Tests

**Files to Update**:
- `test/unit/workflowexecution/consecutive_failures_test.go` - DELETE
- `test/integration/workflowexecution/reconciler_test.go` - Remove BR-WE-012 tests (lines 1084-1296)
- `test/e2e/workflowexecution/03_backoff_cooldown_test.go` - DELETE

**Rationale**: Tests for routing logic belong in RO test suite, not WE

### Step 4: Update Documentation

**Files**:
- BR-WE-012: Update to reference RR fields only
- CRD schema docs: Mark WFE fields as deprecated
- DD-RO-002: Update Phase 3 status to COMPLETE

---

## Answers to Specific Questions

### Q1: "In which service should these fields belong to?"

**Answer**: ✅ **RemediationOrchestrator (RO)** - Specifically in `RemediationRequest.Status`

**Authoritative Source**: DD-RO-002 lines 32-38, 57, 351-360

**Confidence**: 100%

---

### Q2: "RO's status or WE's status?"

**Answer**: ✅ **RR.Status (owned by RO)**

**Current API**:
- ✅ `RR.Status.ConsecutiveFailureCount` - CORRECT
- ✅ `RR.Status.NextAllowedExecution` - CORRECT
- ❌ `WFE.Status.ConsecutiveFailures` - VESTIGIAL (remove in Phase 3)
- ❌ `WFE.Status.NextAllowedExecution` - VESTIGIAL (remove in Phase 3)

---

### Q3: "Why does duplicate tracking exist?"

**Answer**: Historical evolution - fields added to RR in Phase 2, but not removed from WFE yet (Phase 3 pending).

**Not a design decision** - it's incomplete migration per DD-RO-002 roadmap.

---

## Correction to Discovery Document

### Original Statement (INCORRECT)

From `STATE_PROPAGATION_DUPLICATE_TRACKING_DISCOVERY_DEC_19_2025.md` lines 16-18:

```
**Problem**: RO does **NOT** read WFE status. RO maintains its own counter and calculates its own backoff.

**Risk**: Two independent tracking systems can diverge, leading to inconsistent routing decisions.
```

### Corrected Statement (CORRECT)

```
**Architecture**: Per DD-RO-002, RO SHOULD maintain its own counter and backoff in RR.Status.

**Current Implementation**: ✅ CORRECT - RO tracks in RR.Status as specified in DD-RO-002.

**Issue**: WFE still has vestigial fields from pre-DD-RO-002 implementation (Phase 3 cleanup pending).

**Action**: Remove WFE fields in Phase 3, not add sync mechanism.
```

---

## Recommendation

### Immediate (V1.0)

✅ **Accept current RO implementation as correct** - RO tracking in RR.Status is per DD-RO-002 spec

⚠️ **Document WFE fields as deprecated** - Add deprecation notices

### Phase 3 (Post-V1.0)

❌ **Remove WFE routing fields** completely:
- `WFE.Status.ConsecutiveFailures`
- `WFE.Status.NextAllowedExecution`
- Related controller logic
- Related tests

✅ **Keep only RR fields**:
- `RR.Status.ConsecutiveFailureCount`
- `RR.Status.NextAllowedExecution`

---

## Summary

**Question**: Which service owns ConsecutiveFailures and NextAllowedExecution?

**Authoritative Answer**: ✅ **RO (RemediationRequest.Status)**

**Evidence**:
1. DD-RO-002 Line 13: "RO owns ALL routing decisions"
2. DD-RO-002 Lines 36-37: WE ownership is listed as "❌ Problem"
3. DD-RO-002 Line 57: Routing decisions made "BEFORE creating WorkflowExecution"
4. DD-RO-002 Lines 351-360: Phase 3 removes routing from WE
5. API comments: "Used by routing engine" = RO

**Current State**:
- ✅ RO implementation CORRECT (RR.Status fields)
- ❌ WE implementation VESTIGIAL (WFE.Status fields - remove in Phase 3)

**Confidence**: 100% - Based on multiple authoritative sources (DD-RO-002, API comments, implementation plan)

---

**Document Version**: 1.0
**Date**: December 19, 2025
**Status**: ✅ **AUTHORITATIVE ANSWER PROVIDED**
**Source**: DD-RO-002 (Centralized Routing Responsibility)
**Action**: Update STATE_PROPAGATION_DUPLICATE_TRACKING_DISCOVERY document to reflect this

