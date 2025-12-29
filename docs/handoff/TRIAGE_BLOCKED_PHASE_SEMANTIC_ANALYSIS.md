# Blocked Phase Semantic Analysis - All Blocking Reasons

**Date**: December 15, 2025
**Purpose**: Evaluate if "Blocked" phase semantically fits all V1.0 routing scenarios

---

## 🔍 **Current Usage (Production)**

### BR-ORCH-042: Consecutive Failures

**Current Implementation**: `pkg/remediationorchestrator/controller/blocking.go`

```yaml
Scenario: Signal fails 3+ times consecutively
Action: Transition to Blocked phase
BlockReason: "3 consecutive failures"
BlockedUntil: Now + 1 hour cooldown
Behavior:
  - Wait for cooldown to expire
  - After expiry → transition to terminal Failed
  - Gateway sees Blocked (non-terminal) → deduplicates
```

**Semantic Fit**: ✅ **PERFECT**
- "Blocked" = Cannot proceed due to repeated failures
- Will wait, then give up (transition to Failed)

---

## 🎯 **V1.0 Additional Blocking Scenarios**

### 1. ResourceBusy (Same Target, Different or Same Workflow)

**Scenario**: Another workflow is actively running on the same target resource

```yaml
Example:
  - RR1 running workflow "restart-pod" on "namespace/pod/my-app"
  - RR2 arrives targeting same "namespace/pod/my-app"

Action: Block RR2
BlockReason: "ResourceBusy"
BlockingWorkflowExecution: "wfe-abc123"
Behavior:
  - Wait for original workflow to complete
  - Check every 30s if resource available
  - When available → proceed to execute
```

**Semantic Analysis**:
- ✅ "Blocked" = Cannot proceed because resource is in use
- ✅ Temporary: Will proceed when resource available
- ✅ External condition: Blocked by another workflow
- ✅ Won't execute: Waits for resource to free up

**Semantic Fit**: ⭐⭐⭐⭐⭐ **PERFECT**

---

### 2. RecentlyRemediated (Same Workflow, Same Target, Cooldown)

**Scenario**: Same workflow was recently executed on same target, cooldown period active

```yaml
Example:
  - RR1 completed "restart-pod" on "namespace/pod/my-app" at 10:00
  - RR2 arrives at 10:02 with same workflow + target
  - Cooldown: 5 minutes (DD-WE-001)

Action: Block RR2
BlockReason: "RecentlyRemediated"
BlockingWorkflowExecution: "wfe-previous-xyz"
CooldownRemaining: "2m58s"
Behavior:
  - Wait for cooldown to expire (5 min from completion)
  - Check periodically if cooldown expired
  - When expired → proceed to execute
```

**Semantic Analysis**:
- ✅ "Blocked" = Cannot proceed due to recent execution
- ✅ Temporary: Will proceed after cooldown
- ✅ Safety mechanism: Prevents remediation storm
- ✅ Won't execute during cooldown

**Semantic Fit**: ⭐⭐⭐⭐⭐ **PERFECT**

---

### 3. ExponentialBackoff (Pre-Execution Failures)

**Scenario**: Workflow failed before execution (e.g., image pull, quota), exponential backoff active

```yaml
Example:
  - RR1 attempt 1: Failed (ImagePullBackOff) at 10:00
  - RR1 attempt 2: Failed (ImagePullBackOff) at 10:01 (base 1m)
  - RR1 attempt 3: Should retry at 10:03 (backoff 2m)

Action: Block RR1
BlockReason: "ExponentialBackoff"
NextAllowedExecution: "2025-12-15T10:03:00Z"
ConsecutivePreExecutionFailures: 2
Behavior:
  - Wait for backoff window to expire
  - Retry when window expires
  - If succeeds → clear failures
  - If fails again → longer backoff
```

**Semantic Analysis**:
- ✅ "Blocked" = Cannot retry yet due to recent failures
- ✅ Temporary: Will retry after backoff window
- ✅ Graduated response: Backoff increases with failures
- ✅ Will eventually execute or exhaust retries

**Semantic Fit**: ⭐⭐⭐⭐⭐ **PERFECT**

---

### 4. DuplicateInProgress (Duplicate RR, Waiting for Original)

**Scenario**: Duplicate signal arrives while original remediation is in progress

```yaml
Example:
  - RR1 (original): Phase=Executing, WFE running
  - Signal 2 arrives: Same fingerprint as RR1
  - Gateway sees RR1 non-terminal → deduplicates → updates status
  - BUT what if RR2 was already created before RO routing?

Action: Block RR2 (if created)
BlockReason: "DuplicateInProgress"
DuplicateOf: "rr-original-abc123"
Behavior:
  - Wait for original RR to complete
  - Inherit outcome from original
  - When original completes → transition to final state
  - NEVER executes itself
```

**Semantic Analysis**:
- ✅ "Blocked" = Cannot proceed because duplicate of active remediation
- ✅ Temporary: Waiting for original outcome
- ✅ Won't execute: Will inherit outcome, never run
- ⚠️ Different from others: Won't ever execute, just waits

**Semantic Fit**: ⭐⭐⭐⭐ **GOOD** (slightly different but fits)

**Note**: This one is unique - it will NEVER execute, just waits to inherit outcome. But "Blocked by duplicate" still makes semantic sense.

---

### 5. ExhaustedRetries (Max Failures Reached)

**Scenario**: Pre-execution failures exceeded max retry count (e.g., 5 failures)

```yaml
Example:
  - RR1 failed 5 times (ImagePullBackOff)
  - MaxRetries: 5 (configurable)

Action:
  - Option A: Block with no expiry (manual intervention)
  - Option B: Transition to terminal Failed immediately

Current Plan: Option B (Failed terminal)
```

**Semantic Analysis**:
- ⚠️ "Blocked" could work but implies eventual retry
- ⚠️ Better: Terminal Failed (no retry, manual intervention needed)

**Semantic Fit**: ⭐⭐⭐ **OKAY** but terminal Failed is clearer

**Recommendation**: Use terminal **Failed**, not Blocked (no retry expected)

---

## 📊 **Semantic Compatibility Table**

| BlockReason | Temporary? | Will Execute? | External Condition? | Semantic Fit |
|-------------|------------|---------------|-------------------|--------------|
| **ConsecutiveFailures** | ✅ Yes (1h) | ❌ No (→Failed) | ⏰ Time-based | ⭐⭐⭐⭐⭐ PERFECT |
| **ResourceBusy** | ✅ Yes | ✅ Yes (when available) | 🔄 Resource availability | ⭐⭐⭐⭐⭐ PERFECT |
| **RecentlyRemediated** | ✅ Yes (5m) | ✅ Yes (after cooldown) | ⏰ Time-based | ⭐⭐⭐⭐⭐ PERFECT |
| **ExponentialBackoff** | ✅ Yes | ✅ Yes (after backoff) | ⏰ Time-based | ⭐⭐⭐⭐⭐ PERFECT |
| **DuplicateInProgress** | ✅ Yes | ❌ No (inherits outcome) | 🔄 Original completion | ⭐⭐⭐⭐ GOOD |
| **ExhaustedRetries** | ❌ No | ❌ No (permanent) | - | ⭐⭐⭐ Better as Failed |

---

## 🎯 **Semantic Definition of "Blocked"**

Based on all scenarios, "Blocked" means:

> **"Cannot proceed right now due to an external condition. Will retry when condition clears OR transition to terminal state if condition persists."**

### Common Characteristics:
1. ✅ **Non-terminal**: More retries possible
2. ✅ **External blocker**: Something outside this RR is blocking progress
3. ✅ **Time-based OR event-based**: Will clear after time OR when external condition changes
4. ✅ **Gateway deduplicates**: Prevents RR flood while blocked

### Two Sub-Categories:

**Category A: Will Eventually Execute** (if blocker clears)
- ResourceBusy → Executes when resource available
- RecentlyRemediated → Executes after cooldown
- ExponentialBackoff → Executes after backoff window

**Category B: Will Never Execute** (waiting for outcome)
- ConsecutiveFailures → Waits for cooldown, then transitions to Failed
- DuplicateInProgress → Waits for original, then inherits outcome

---

## ✅ **Conclusion: "Blocked" Fits All Scenarios**

**Semantic Verdict**: ⭐⭐⭐⭐⭐ **"Blocked" is semantically correct for all 5 scenarios**

### Why it works:

1. **ConsecutiveFailures**: "Blocked by repeated failures, waiting for cooldown"
2. **ResourceBusy**: "Blocked by another workflow using the resource"
3. **RecentlyRemediated**: "Blocked by recent execution cooldown"
4. **ExponentialBackoff**: "Blocked by backoff window"
5. **DuplicateInProgress**: "Blocked because this is a duplicate"

All share: **Cannot proceed now, external condition blocking, will retry or resolve later**

---

## 📋 **Recommended BlockReason Values (V1.0)**

```go
// In api/remediation/v1alpha1/remediationrequest_types.go

// BlockReason indicates why this remediation is blocked (non-terminal)
// Valid values:
// - "ConsecutiveFailures": Max consecutive failures reached, in cooldown (BR-ORCH-042)
// - "ResourceBusy": Another workflow is using the target resource
// - "RecentlyRemediated": Target recently remediated, cooldown active
// - "ExponentialBackoff": Pre-execution failures, backoff window active
// - "DuplicateInProgress": Duplicate of an active remediation
// Only set when OverallPhase = "Blocked"
// Reference: DD-RO-002 v1.1
BlockReason string `json:"blockReason,omitempty"`
```

---

## 🔧 **Implementation Pattern**

### Unified Blocking Logic

```go
// In RO routing logic (Days 2-5)
func (r *Reconciler) CheckBlockingConditions(ctx context.Context, rr *RemediationRequest) (blocked bool, reason string, requeueAfter time.Duration, err error) {

    // Check 1: Consecutive failures
    if r.ShouldBlockForConsecutiveFailures(ctx, rr) {
        return true, "ConsecutiveFailures", 1*time.Hour, nil
    }

    // Check 2: Resource busy
    if activeWFE := r.FindActiveWFEForTarget(ctx, rr.TargetResource); activeWFE != nil {
        return true, "ResourceBusy", 30*time.Second, nil
    }

    // Check 3: Recently remediated (cooldown)
    if recentWFE := r.FindRecentCompletedWFE(ctx, rr.TargetResource, rr.WorkflowID); recentWFE != nil {
        cooldownRemaining := r.CalculateCooldownRemaining(recentWFE)
        if cooldownRemaining > 0 {
            return true, "RecentlyRemediated", cooldownRemaining, nil
        }
    }

    // Check 4: Exponential backoff
    if rr.Status.NextAllowedExecution != nil && time.Now().Before(rr.Status.NextAllowedExecution.Time) {
        backoffRemaining := time.Until(rr.Status.NextAllowedExecution.Time)
        return true, "ExponentialBackoff", backoffRemaining, nil
    }

    // Check 5: Duplicate in progress
    if originalRR := r.FindActiveRRForFingerprint(ctx, rr.Spec.SignalFingerprint); originalRR != nil && originalRR.Name != rr.Name {
        return true, "DuplicateInProgress", 30*time.Second, nil
    }

    // No blocking conditions
    return false, "", 0, nil
}

// Apply blocking
if blocked, reason, requeueAfter, err := r.CheckBlockingConditions(ctx, rr); blocked {
    err = helpers.UpdateRemediationRequestStatus(ctx, r.Client, rr, func(rr *RemediationRequest) error {
        rr.Status.OverallPhase = remediationv1.PhaseBlocked
        rr.Status.BlockReason = reason
        rr.Status.BlockMessage = r.FormatBlockMessage(reason, ...) // Human-readable
        // Set blocking-specific fields based on reason
        return nil
    })
    return ctrl.Result{RequeueAfter: requeueAfter}, err
}
```

---

## 📚 **Field Structure (Updated V1.0)**

```go
// RemediationRequestStatus
type RemediationRequestStatus struct {
    // ... other fields ...

    // ========================================
    // BLOCKED PHASE (NON-TERMINAL)
    // Unified blocking for all temporary wait states
    // ========================================

    // BlockReason indicates why blocked
    // Values: ConsecutiveFailures, ResourceBusy, RecentlyRemediated,
    //         ExponentialBackoff, DuplicateInProgress
    BlockReason string `json:"blockReason,omitempty"`

    // BlockMessage is human-readable explanation
    BlockMessage string `json:"blockMessage,omitempty"`

    // BlockedUntil is when blocking expires (time-based blocks)
    // Set for: ConsecutiveFailures, RecentlyRemediated, ExponentialBackoff
    // Nil for: ResourceBusy, DuplicateInProgress (event-based)
    BlockedUntil *metav1.Time `json:"blockedUntil,omitempty"`

    // BlockingWorkflowExecution references blocking WFE (if applicable)
    // Set for: ResourceBusy, RecentlyRemediated, ExponentialBackoff
    BlockingWorkflowExecution string `json:"blockingWorkflowExecution,omitempty"`

    // DuplicateOf references original RR (if applicable)
    // Set for: DuplicateInProgress
    DuplicateOf string `json:"duplicateOf,omitempty"`
}
```

---

## 🎯 **Final Recommendation**

**Use `Blocked` phase with `BlockReason` enum for all 5 scenarios**

### Rationale:
1. ✅ **Semantically consistent**: All represent "cannot proceed now, external blocker"
2. ✅ **Non-terminal**: All need Gateway deduplication to work
3. ✅ **Unified logic**: Single blocking handler, reason-specific behavior
4. ✅ **Clear to operators**: "Blocked" + reason explains what's happening
5. ✅ **No new phases**: Reuses existing phase, minimal API changes

### Exception:
- **ExhaustedRetries**: Use terminal **Failed** (not Blocked) because no retry expected

---

**Document Version**: 1.0
**Created**: December 15, 2025
**Status**: ✅ **SEMANTIC ANALYSIS COMPLETE**
**Decision**: Use Blocked + BlockReason for all 5 scenarios




