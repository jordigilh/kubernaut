# DD-RO-002 ADDENDUM: Blocked Phase Semantics for V1.0 Routing

**Design Decision ID**: DD-RO-002-ADDENDUM-001
**Parent Decision**: DD-RO-002 (Centralized Routing Responsibility)
**Status**: ✅ **APPROVED** - Authoritative
**Date**: December 15, 2025
**Confidence**: 98%

---

## 🎯 **Decision Summary**

**Decision**: Use `Blocked` phase with `BlockReason` enum for ALL temporary blocking scenarios in V1.0 centralized routing.

**Critical Finding**: V1.0 design had architectural flaw where `Skipped` (terminal phase) would cause Gateway deduplication to break, creating RR flood for duplicate signals.

**Solution**: Keep RR in non-terminal `Blocked` phase while waiting for external conditions to clear.

---

## 📋 **Problem Statement**

### Original V1.0 Design Flaw

```yaml
Signal 1 → RR1 (Executing) → WFE1 executing
Signal 2 (30s later) → RO checks → ResourceBusy → RR1=Skipped (TERMINAL!)
Signal 3 (60s later) → Gateway sees RR1=Skipped → Creates RR2 ❌
Signal 4 (90s later) → RO checks → ResourceBusy → RR2=Skipped (TERMINAL!)
... (RR FLOOD!)
```

**Root Cause**:
- `Skipped` is terminal phase (confirmed in `pkg/gateway/processing/phase_checker.go:177`)
- Gateway allows new RRs for terminal phases
- Result: Deduplication breaks for duplicate signals

---

## 🔍 **Analysis**

### Gateway Phase-Based Deduplication

**File**: `pkg/gateway/processing/phase_checker.go`

```go
// IsTerminalPhase checks if a RemediationRequest phase is terminal.
// Terminal phases allow new RR creation for the same signal fingerprint.
//
// TERMINAL (allow new RR creation):
// - Completed, Failed, TimedOut, Skipped, Cancelled
//
// NON-TERMINAL (deduplicate → update status):
// - Pending, Processing, Analyzing, AwaitingApproval, Executing, Blocked
```

**Key Insight**: ANY non-terminal phase prevents Gateway from creating new RRs.

---

## 💡 **Solution Options Evaluated**

### Option 1: Keep Pending Phase
```yaml
Phase: Pending
PendingReason: ResourceBusy
```
**Verdict**: ❌ Semantically misleading - "Pending" implies will execute

---

### Option 2: Use Blocked Phase ✅ SELECTED
```yaml
Phase: Blocked
BlockReason: ResourceBusy
```
**Verdict**: ✅ Semantically correct - "Blocked by external condition"

---

### Option 3: New AwaitingResource Phase
```yaml
Phase: AwaitingResource
```
**Verdict**: ❌ Requires API change, more complex

---

## 🎯 **Approved Solution**

### Semantic Model: "Blocked"

> **"Cannot proceed right now due to an external condition. Will retry when condition clears OR transition to terminal state if condition persists."**

### Five BlockReason Values

| BlockReason | Temporary? | Will Execute? | External Condition | Semantic Fit |
|-------------|------------|---------------|-------------------|--------------|
| **ConsecutiveFailures** | ✅ Yes (1h) | ❌ No (→Failed) | ⏰ Time-based | ⭐⭐⭐⭐⭐ PERFECT |
| **ResourceBusy** | ✅ Yes | ✅ Yes (when available) | 🔄 Resource availability | ⭐⭐⭐⭐⭐ PERFECT |
| **RecentlyRemediated** | ✅ Yes (5m) | ✅ Yes (after cooldown) | ⏰ Time-based | ⭐⭐⭐⭐⭐ PERFECT |
| **ExponentialBackoff** | ✅ Yes | ✅ Yes (after backoff) | ⏰ Time-based | ⭐⭐⭐⭐⭐ PERFECT |
| **DuplicateInProgress** | ✅ Yes | ❌ No (inherits outcome) | 🔄 Original completion | ⭐⭐⭐⭐ GOOD |

---

## 📊 **Common Characteristics**

All `Blocked` scenarios share:
1. ✅ **Non-terminal**: More retries possible
2. ✅ **External blocker**: Something outside this RR is blocking progress
3. ✅ **Time-based OR event-based**: Will clear after time OR when external condition changes
4. ✅ **Gateway deduplicates**: Prevents RR flood while blocked

---

## 🔧 **API Changes Required**

### File: `api/remediation/v1alpha1/remediationrequest_types.go`

```go
// ========================================
// BLOCKED PHASE TRACKING (DD-RO-002 ADDENDUM-001)
// Unified blocking for all temporary wait states
// ========================================

// BlockReason indicates why this remediation is blocked (non-terminal)
// Valid values:
// - "ConsecutiveFailures": Max consecutive failures reached, in cooldown (BR-ORCH-042)
// - "ResourceBusy": Another workflow is using the target resource
// - "RecentlyRemediated": Target recently remediated, cooldown active
// - "ExponentialBackoff": Pre-execution failures, backoff window active
// - "DuplicateInProgress": Duplicate of an active remediation
// Only set when OverallPhase = "Blocked"
// Reference: DD-RO-002 ADDENDUM-001
BlockReason string `json:"blockReason,omitempty"`

// BlockMessage provides human-readable details about why remediation is blocked
// Examples:
// - "Another workflow is running on target: wfe-abc123"
// - "Recently remediated. Cooldown: 3m15s remaining"
// - "Backoff active. Next retry: 2025-12-15T10:30:00Z"
// - "Duplicate of active remediation rr-original-abc"
// - "3 consecutive failures. Cooldown expires: 2025-12-15T11:00:00Z"
// Only set when OverallPhase = "Blocked"
// +optional
BlockMessage string `json:"blockMessage,omitempty"`

// BlockedUntil is when blocking expires (time-based blocks)
// Set for: ConsecutiveFailures, RecentlyRemediated, ExponentialBackoff
// Nil for: ResourceBusy, DuplicateInProgress (event-based)
// +optional
BlockedUntil *metav1.Time `json:"blockedUntil,omitempty"`

// BlockingWorkflowExecution references blocking WFE (if applicable)
// Set for: ResourceBusy, RecentlyRemediated, ExponentialBackoff
// +optional
BlockingWorkflowExecution string `json:"blockingWorkflowExecution,omitempty"`

// DuplicateOf references original RR (if applicable)
// Set for: DuplicateInProgress
// Only set when OverallPhase = "Blocked" with BlockReason = "DuplicateInProgress"
// +optional
DuplicateOf string `json:"duplicateOf,omitempty"`
```

---

## 📝 **Implementation Pattern**

### Unified Blocking Logic

```go
// CheckBlockingConditions checks all blocking scenarios
func (r *Reconciler) CheckBlockingConditions(ctx context.Context, rr *RemediationRequest) (blocked bool, reason string, requeueAfter time.Duration, err error) {

    // Check 1: Consecutive failures (BR-ORCH-042, already implemented)
    if r.ShouldBlockForConsecutiveFailures(ctx, rr) {
        return true, "ConsecutiveFailures", 1*time.Hour, nil
    }

    // Check 2: Resource busy (V1.0 new)
    if activeWFE := r.FindActiveWFEForTarget(ctx, rr.TargetResource); activeWFE != nil {
        return true, "ResourceBusy", 30*time.Second, nil
    }

    // Check 3: Recently remediated (V1.0 new)
    if recentWFE := r.FindRecentCompletedWFE(ctx, rr.TargetResource, rr.WorkflowID); recentWFE != nil {
        cooldownRemaining := r.CalculateCooldownRemaining(recentWFE)
        if cooldownRemaining > 0 {
            return true, "RecentlyRemediated", cooldownRemaining, nil
        }
    }

    // Check 4: Exponential backoff (V1.0 new)
    if rr.Status.NextAllowedExecution != nil && time.Now().Before(rr.Status.NextAllowedExecution.Time) {
        backoffRemaining := time.Until(rr.Status.NextAllowedExecution.Time)
        return true, "ExponentialBackoff", backoffRemaining, nil
    }

    // Check 5: Duplicate in progress (V1.0 new)
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
        // Set reason-specific fields (BlockedUntil, BlockingWorkflowExecution, DuplicateOf)
        return nil
    })
    return ctrl.Result{RequeueAfter: requeueAfter}, err
}
```

---

## ✅ **Benefits**

### 1. Gateway Deduplication Works
- `Blocked` is non-terminal
- Gateway sees Blocked RR → deduplicates (updates status, no new RR)
- Prevents RR flood for duplicate signals

### 2. Semantic Clarity
- "Blocked" + reason clearly explains state
- Operators understand: "Can't proceed because X"
- Each reason has specific meaning

### 3. Unified Logic
- Single blocking handler
- Reason-specific behavior
- Clean phase semantics

### 4. Minimal API Changes
- Reuses existing `Blocked` phase
- Adds 3-5 new status fields
- No Gateway changes needed

---

## 🚫 **Exception: ExhaustedRetries**

**Not Blocked**: Use terminal `Failed` phase instead

**Rationale**:
- No retry expected (permanent failure)
- "Blocked" implies eventual retry
- Terminal state more accurate

---

## 📊 **Validation Results**

### Test Scenario: High-Frequency Alerts

```yaml
Setup:
  - Prometheus fires alert every 30 seconds
  - Workflow takes 5 minutes to execute
  - Same target resource

Expected Behavior:
  - Signal 1 (T+0s): RR1 created, Phase=Pending → Processing → Executing
  - Signal 2-10 (T+30s - T+5m): Gateway sees RR1 non-terminal → deduplicates
  - Result: 1 RR, OccurrenceCount=10 ✅

V1.0 WITHOUT Fix (Broken):
  - Signal 1: RR1 created, executes
  - Signal 2: RO detects ResourceBusy → RR1=Skipped (terminal)
  - Signal 3: Gateway sees RR1=Skipped → creates RR2
  - Signal 4: RO detects ResourceBusy → RR2=Skipped (terminal)
  - Result: 7 RRs for 10 alerts ❌ BROKEN

V1.0 WITH Fix (Correct):
  - Signal 1: RR1 created, executes
  - Signal 2: RO detects ResourceBusy → RR1=Blocked (non-terminal)
  - Signal 3-10: Gateway sees RR1=Blocked → deduplicates
  - Result: 1 RR, OccurrenceCount=10 ✅ WORKS
```

---

## 🎯 **Success Criteria**

- ✅ No RR flood for duplicate signals
- ✅ Gateway deduplication works with Blocked phase
- ✅ Clear semantic model for all blocking reasons
- ✅ Operators can understand blocking state
- ✅ Non-terminal phase prevents new RR creation

---

## 📚 **References**

- **DD-RO-002**: Centralized Routing Responsibility (parent decision)
- **DD-GATEWAY-011**: Phase-based deduplication
- **BR-GATEWAY-181**: Deduplication tracking in status
- **BR-ORCH-042**: Consecutive failure blocking
- **BR-ORCH-032**: Resource lock handling
- **DD-WE-001**: Resource locking safety (5-minute cooldown)
- **DD-WE-004**: Exponential backoff cooldown

---

## 📋 **Related Documents**

- **Triage**: (internal development reference, removed in v1.0)
- **Semantic Analysis**: (internal development reference, removed in v1.0)
- **Implementation Plan**: `docs/services/crd-controllers/05-remediationorchestrator/implementation/V1.0_BLOCKED_PHASE_ROUTING_EXTENSION_V1.0.md`

---

**Document Version**: 1.0
**Status**: ✅ **AUTHORITATIVE**
**Approved By**: Platform Architect
**Date**: December 15, 2025
**Next Review**: After V1.0 implementation (Days 2-5)




