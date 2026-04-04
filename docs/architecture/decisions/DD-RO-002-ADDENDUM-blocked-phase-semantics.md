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

## Issue #190: Execution-Time Dedup (Skipped/Deduplicated Phase)

When a WFE encounters a resource collision at execution time (Job or PipelineRun `AlreadyExists` from another WFE), the WFE is classified as `Failed` with `FailureReason=Deduplicated` and `DeduplicatedBy=<originalWFE>`.

### RO Handler Branching

The `WorkflowExecutionHandler.HandleStatus` detects this reason and, instead of calling `transitionToFailed`, sets `DeduplicatedByWE` on the RR status and requeues.

### Result Propagation (C3/C4)

On subsequent reconciles, `handleExecutingPhase` short-circuits when `DeduplicatedByWE` is set:
- Fetches the original WFE by name
- If **Completed** → `transitionToInheritedCompleted` (Outcome=`InheritedCompleted`, skips Verifying)
- If **Failed** → `transitionToInheritedFailed` (FailurePhase=`Deduplicated`)
- If **Deleted** → `transitionToInheritedFailed` (dangling reference)
- If **Running** → requeue after 10s

### Consecutive Failure Exclusion

`transitionToInheritedFailed` does NOT increment `ConsecutiveFailureCount` or set exponential backoff. Additionally, `countConsecutiveFailures` in `blocking.go` skips RRs where `FailurePhase=Deduplicated`. This ensures inherited failures do not contribute to BR-ORCH-042 blocking.

### K8s Events

Inherited transitions emit `InheritedCompleted` (Normal) or `InheritedFailed` (Warning) events with the original WFE name for operator visibility.

### Audit Trail (ADR-032 §1)

All inherited terminal transitions emit DataStorage audit events for SOC 2 compliance:

- **`transitionToInheritedCompleted`** → `orchestrator.lifecycle.completed` with `outcome="InheritedCompleted"` and duration from `rr.Status.StartTime`. Emitted only when the phase actually changed (idempotency guard: `oldPhase != Completed`).
- **`transitionToInheritedFailed`** → `orchestrator.lifecycle.failed` with `failurePhase=Deduplicated` and the propagated `failureErr` (contains original WFE name for traceability). Emitted only when the phase actually changed (idempotency guard: `oldPhase != Failed`).

Both audit events use `rr.Name` as the correlation ID per DD-AUDIT-CORRELATION-002.

### Completion Notifications (BR-ORCH-045)

`transitionToInheritedCompleted` calls `ensureNotificationsCreated(ctx, rr)` after setting `Outcome="InheritedCompleted"`. This creates:
- A **completion `NotificationRequest`** with the inherited outcome in its body/metadata
- A **bulk-duplicate `NotificationRequest`** if applicable (same idempotent, deterministic-name pattern as standard completions)

`transitionToInheritedFailed` does **NOT** create notifications — consistent with `transitionToFailed`, which also omits notifications on failure paths (Issue #240: EA/notifications only on success).

### Metrics

Both inherited transitions record `PhaseTransitionsTotal` with appropriate labels (`from_phase`, `to_phase`, `namespace`). No additional dedicated metrics are required — the `PhaseTransitionsTotal` counter captures inherited transitions distinctly via the `from_phase=Executing` label combined with the `to_phase` value.

`transitionToInheritedFailed` intentionally does **NOT** increment `ConsecutiveFailureCount` or set `NextAllowedExecution` (exponential backoff). This is enforced both in the transition function and in `countConsecutiveFailures` (skip filter on `FailurePhase=Deduplicated`).

---

## Issue #614: RO-level DuplicateInProgress Outcome Inheritance

Issue #190 addressed WE-level deduplication (execution-time resource collisions). Issue #614 extends this to **RO-level deduplication**: when the routing engine blocks an RR as `DuplicateInProgress` (same signal fingerprint, another RR active), the duplicate should inherit the original RR's outcome instead of re-running the entire pipeline.

### Prior Behavior (pre-#614)

When `recheckDuplicateBlock` detected that the original RR reached a terminal phase, it called `clearEventBasedBlock(ctx, rr, phase.Pending)` to reset the duplicate to `Pending` and re-run the full SP → AI → Approval → WE pipeline.

### New Behavior (#614)

`recheckDuplicateBlock` now inherits the original RR's outcome directly:

- **Original Completed** → `transitionToInheritedCompleted(ctx, rr, rr.Status.DuplicateOf, "RemediationRequest")` — sets `Outcome="InheritedCompleted"`, `CompletedAt`
- **Original Failed/TimedOut/Cancelled/Skipped** → `transitionToInheritedFailed(ctx, rr, err, rr.Status.DuplicateOf, "RemediationRequest")` — sets `FailurePhase=Deduplicated`, `FailureReason` with original RR name
- **Original Deleted** → `transitionToInheritedFailed` (dangling reference)
- **Original Still Active** → requeue after `config.RequeueResourceBusy`
- **Empty `DuplicateOf`** → `clearEventBasedBlock` to `Pending` (safety fallback, unchanged)

### Generalized Transition Methods (Phase 1 Refactor)

The `transitionToInheritedCompleted` and `transitionToInheritedFailed` methods were generalized to accept `sourceRef` and `sourceKind` parameters, making them reusable for both WE-level (#190) and RR-level (#614) inheritance:

```go
func (r *Reconciler) transitionToInheritedCompleted(ctx, rr, sourceRef, sourceKind string) (ctrl.Result, error)
func (r *Reconciler) transitionToInheritedFailed(ctx, rr, failureErr, sourceRef, sourceKind string) (ctrl.Result, error)
```

### Observability

- **K8s Events**: `InheritedCompleted`/`InheritedFailed` events include `sourceKind=RemediationRequest` and the original RR name
- **Audit Events**: Standard `orchestrator.lifecycle.completed`/`failed` audit events (ADR-032 §1) are emitted
- **Metrics**: `CurrentBlockedGauge` decrements after successful transition (F-6: gauge decrement occurs *after* the status update to prevent metric drift on failure); `PhaseTransitionsTotal` records `Blocked→Completed` or `Blocked→Failed`

### Notification Guard (F-3)

`ensureNotificationsCreated` is only called for `sourceKind == "WorkflowExecution"` inheritance. DuplicateInProgress RRs never reached the `AIAnalysis` phase, so no AIAnalysis CRD exists and notification creation would fail with a misleading error log.

### Consecutive Failure Exclusion

Inherited failures from RR-level dedup set `FailurePhase=Deduplicated`, which is excluded by `countConsecutiveFailures` in `blocking.go` (same mechanism as #190).

---

## 📚 **References**

- **DD-RO-002**: Centralized Routing Responsibility (parent decision)
- **DD-GATEWAY-011**: Phase-based deduplication
- **BR-GATEWAY-181**: Deduplication tracking in status
- **BR-ORCH-042**: Consecutive failure blocking
- **BR-ORCH-032**: Resource lock handling
- **DD-WE-001**: Resource locking safety (5-minute cooldown)
- **DD-WE-003**: Resource lock persistence (Issue #190 dedup classification)
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




