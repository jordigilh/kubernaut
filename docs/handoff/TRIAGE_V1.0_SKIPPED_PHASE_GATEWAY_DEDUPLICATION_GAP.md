# V1.0 Critical Design Gap: Skipped Phase + Gateway Deduplication

**Triage Date**: December 15, 2025
**Severity**: 🔴 **CRITICAL** - Architectural flaw in V1.0 design
**Reporter**: User (Platform Architect)
**Status**: 🚨 **BLOCKS V1.0** - Requires design fix before implementation

---

## 🎯 **Problem Statement**

**User's Concern**:
> "When skipping a RR because there is already a WE instance for the same target and workflow, we will let the gateway create more RRs. In the initial design, the WE was going to hold on to the skipped WEs until the original or first WE completed, to prevent the duplicated WEs and their relative RRs from being considered as finished."

**Translation**: V1.0 routing design has a race condition where:
1. RO decides to skip (ResourceBusy: another WFE running on same target)
2. RO marks RR as `phase=Skipped` (terminal)
3. Gateway sees terminal phase → creates NEW RR for next duplicate signal
4. Result: **Multiple RRs pile up**, all "Skipped", defeating deduplication

---

## 📋 **Authoritative Documentation Triage**

### Finding 1: Gateway Phase-Based Deduplication ✅ VERIFIED

**File**: `pkg/gateway/processing/phase_checker.go`
**Lines**: 151-183

```go
// IsTerminalPhase checks if a RemediationRequest phase is terminal.
// Terminal phases allow new RR creation for the same signal fingerprint.
//
// TERMINAL (allow new RR creation):
// - Completed: Remediation succeeded
// - Failed: Remediation failed
// - TimedOut: Remediation timed out
// - Skipped: Remediation was not needed (per BR-ORCH-032)  ← 🚨 THE PROBLEM
// - Cancelled: Remediation was manually cancelled
//
// NON-TERMINAL (deduplicate → update status):
// - Pending, Processing, Analyzing, AwaitingApproval, Executing
// - Blocked: RO holds for cooldown, Gateway updates dedup status
```

**Verdict**: **Gateway treats `Skipped` as TERMINAL**

**Implication**: New RRs will be created for duplicate signals

---

### Finding 2: RemediationRequest Phase Enum ✅ VERIFIED

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`
**Lines**: 121-123

```go
// PhaseSkipped is the terminal state when remediation was not needed.
// Reference: BR-ORCH-032 (resource lock deduplication)
PhaseSkipped RemediationPhase = "Skipped"
```

**Verdict**: **`Skipped` is explicitly defined as TERMINAL**

---

### Finding 3: Current ResourceBusy Handler Behavior ✅ VERIFIED

**File**: `pkg/remediationorchestrator/handler/skip/resource_busy.go`
**Lines**: 42-44, 87-88, 99

```go
// BEHAVIOR:
// - Marks RR as Skipped (duplicate)
// - Tracks parent RR via DuplicateOf field
// - Requeues after 30 seconds for retry

err := helpers.UpdateRemediationRequestStatus(ctx, h.ctx.Client, rr, func(rr *remediationv1.RemediationRequest) error {
    rr.Status.OverallPhase = remediationv1.PhaseSkipped  // ← 🚨 TERMINAL PHASE
    rr.Status.SkipReason = "ResourceBusy"
    return nil
})

return ctrl.Result{RequeueAfter: config.RequeueResourceBusy}, nil  // ← Requeues every 30s
```

**Current Behavior**:
1. RR goes to `Skipped` (terminal)
2. RO requeues after 30 seconds
3. Gateway sees terminal phase → **creates new RR for next duplicate**

**Verdict**: **Current code has the exact problem user described**

---

### Finding 4: Old Design (WE-Based Routing) ✅ VERIFIED

**File**: `internal/controller/workflowexecution/workflowexecution_controller.go`
**Function**: `CheckCooldown()`, `CheckResourceLock()`

**Old Behavior**:
1. RR stays in `Executing` phase (non-terminal)
2. WE creates WorkflowExecution CRD with `phase=Skipped`
3. WE holds the WFE CRD until original WFE completes
4. Gateway sees `Executing` (non-terminal) → deduplicates (updates status, no new RR)
5. When original WFE completes → RR can transition to final state

**Key**: **RR NEVER went to terminal phase while waiting**

**Verdict**: **Old design prevented RR flood by keeping phase non-terminal**

---

## 🔍 **Root Cause Analysis**

### Architectural Conflict

**V1.0 Design Decision** (DD-RO-002): RO makes routing decisions BEFORE creating WFE

**Consequence**:
- ✅ **Benefit**: WE simplification (-57% complexity)
- ❌ **Problem**: No CRD to "hold" the request while waiting

**Timeline**:
```
OLD (WE-based routing):
Signal → RR (Executing) → WFE created → WFE (Skipped) → WFE held → Original WFE completes → RR (Skipped)
         ^-- NON-TERMINAL (Gateway deduplicates) ------------------------------------^-- TERMINAL (Gateway allows new)

NEW (V1.0 RO-based routing):
Signal → RR (Executing) → RO decides "skip" → RR (Skipped) → NO WFE created
                                               ^-- TERMINAL (Gateway allows new RR immediately!)

Next Signal → Gateway sees RR=Skipped → Creates NEW RR → RO decides "skip" → RR (Skipped)
Next Signal → Gateway sees RR=Skipped → Creates NEW RR → RO decides "skip" → RR (Skipped)
... (RR flood!)
```

---

## 📊 **Impact Assessment**

### Scenario: High-Frequency Alerts

**Setup**:
- Prometheus fires alert every 30 seconds
- Workflow takes 5 minutes to execute
- Same target resource

**Old Design (WE Routing)**:
- Signal 1 (T+0s): RR1 created, phase=Executing, WFE1 created, executing
- Signal 2 (T+30s): Gateway sees RR1=Executing (non-terminal) → deduplicates, updates status
- Signal 3 (T+60s): Gateway sees RR1=Executing (non-terminal) → deduplicates, updates status
- ...
- Signal 10 (T+5m): WFE1 completes, RR1=Completed (terminal), next signal creates RR2

**Result**: **1 RR** for 10 alerts (proper deduplication)

---

**V1.0 Design (RO Routing - BROKEN)**:
- Signal 1 (T+0s): RR1 created, phase=Executing, RO creates WFE1, executing
- Signal 2 (T+30s): Gateway sees RR1=Executing (non-terminal) → deduplicates (OK so far)
- Signal 3 (T+60s): RO decides "ResourceBusy" (WFE1 still running) → RR1=Skipped (TERMINAL!)
- Signal 4 (T+90s): Gateway sees RR1=Skipped (terminal) → **creates RR2**
- Signal 5 (T+120s): RO decides "ResourceBusy" → RR2=Skipped (TERMINAL!)
- Signal 6 (T+150s): Gateway sees RR2=Skipped (terminal) → **creates RR3**
- ...
- Signal 10 (T+5m): **7 RRs created** (RR1-RR7), all Skipped

**Result**: **7 RRs** for 10 alerts (deduplication broken!)

---

### Severity: CRITICAL

**Impact**:
- 🚨 **Resource Exhaustion**: K8s API server flooded with RR CRDs
- 🚨 **Performance Degradation**: Controller watches trigger for every new RR
- 🚨 **Observability Noise**: Metrics/logs polluted with "Skipped" RRs
- 🚨 **Operator Confusion**: Multiple RRs for same signal, all "Skipped"

**Business Risk**: **HIGH**
- Defeats primary purpose of deduplication (BR-GATEWAY-181)
- Violates phase-based deduplication design (DD-GATEWAY-011)
- Makes V1.0 worse than current design

---

## 💡 **Solution Options**

### Option 1: Introduce "Pending" State (Recommended)

**Approach**: RO keeps RR in `Pending` phase while waiting for resource

**Implementation**:
```go
// In RO routing logic (Days 2-5 implementation)
func (r *Reconciler) CheckRoutingDecision(ctx context.Context, rr *RemediationRequest) (skip bool, requeue time.Duration, err error) {
    // Check if resource is busy
    if blockingWFE := r.FindActiveWFEForTarget(ctx, targetResource); blockingWFE != nil {
        // DO NOT change phase - leave in "Pending" or current phase
        // Update status fields for observability
        rr.Status.SkipReason = "ResourceBusy"
        rr.Status.SkipMessage = fmt.Sprintf("Another workflow is running: %s", blockingWFE.Name)
        rr.Status.BlockingWorkflowExecution = blockingWFE.Name

        // Requeue after 30s to check again
        return true, 30*time.Second, nil  // skip=true (don't create WFE), requeue=30s
    }

    // Resource available - proceed with WFE creation
    return false, 0, nil
}
```

**Phase Transition**:
```
Signal → RR (Pending) → RO checks → Resource busy → RR stays (Pending) → Requeue 30s
                                                    ^-- NON-TERMINAL (Gateway deduplicates)

         Requeue → RO checks → Resource available → Create WFE → RR (Executing)
```

**Pros**:
- ✅ Gateway deduplication works (Pending is non-terminal)
- ✅ Minimal API changes (no new phase needed)
- ✅ Clear semantics: "Pending" = waiting to execute
- ✅ No CRD flood

**Cons**:
- ⚠️ RR status might show "Pending" for extended time (operator visibility)
- ⚠️ Needs clear status message for why still Pending

**Confidence**: 95%

---

### Option 2: Introduce New "AwaitingResource" Phase

**Approach**: Add explicit phase for resource busy state

**API Change**:
```go
// In api/remediation/v1alpha1/remediationrequest_types.go
// +kubebuilder:validation:Enum=Pending;Processing;Analyzing;AwaitingApproval;Executing;AwaitingResource;Blocked;Completed;Failed;TimedOut;Skipped;Cancelled
type RemediationPhase string

const (
    // ... existing phases ...

    // PhaseAwaitingResource indicates workflow cannot start due to resource conflict
    // This is a NON-terminal phase - RO will retry when resource becomes available
    // Reference: DD-RO-002 (centralized routing), BR-ORCH-032 (resource lock)
    PhaseAwaitingResource RemediationPhase = "AwaitingResource"
)
```

**Gateway Update**:
```go
// In pkg/gateway/processing/phase_checker.go
func IsTerminalPhase(phase remediationv1alpha1.RemediationPhase) bool {
    switch phase {
    case remediationv1alpha1.PhaseCompleted,
        remediationv1alpha1.PhaseFailed,
        remediationv1alpha1.PhaseTimedOut,
        remediationv1alpha1.PhaseSkipped,
        remediationv1alpha1.PhaseCancelled:
        return true
    default:
        return false  // AwaitingResource is non-terminal
    }
}
```

**Pros**:
- ✅ Explicit semantics (clear what's happening)
- ✅ Better observability (distinct phase for resource conflicts)
- ✅ Gateway deduplication works (non-terminal)

**Cons**:
- ❌ API breaking change (new phase enum value)
- ❌ Requires Gateway update
- ❌ More complex than Option 1
- ❌ Documentation updates needed

**Confidence**: 85%

---

### Option 3: Keep "Executing" Phase (Old Behavior)

**Approach**: RO transitions RR to "Executing" even if skipping WFE creation

**Implementation**:
```go
// In RO reconciliation
if shouldSkipDueToResourceBusy {
    // Keep RR in "Executing" phase (non-terminal)
    // Do NOT create WFE
    // Update status fields for why execution is blocked
    rr.Status.SkipReason = "ResourceBusy"
    rr.Status.BlockingWorkflowExecution = blockingWFE.Name
    // Phase stays "Executing"

    return ctrl.Result{RequeueAfter: 30*time.Second}, nil
}
```

**Pros**:
- ✅ No API changes
- ✅ Gateway deduplication works (Executing is non-terminal)
- ✅ Mimics old WE behavior

**Cons**:
- ❌ Misleading semantics ("Executing" but nothing executing)
- ❌ Operator confusion (why is RR "Executing" with no WFE?)
- ❌ Conflates two states (executing workflow vs. waiting for resource)

**Confidence**: 70% (works but semantically wrong)

---

### Option 4: Use "Blocked" Phase (Current Non-Terminal)

**Approach**: Overload "Blocked" phase for both consecutive failures and resource busy

**Current "Blocked" Definition** (api/remediation/v1alpha1/remediationrequest_types.go:106-109):
```go
// PhaseBlocked indicates remediation is in cooldown after consecutive failures.
// This is a NON-terminal phase - RO will transition to Failed after cooldown.
// Reference: BR-ORCH-042 (consecutive failure blocking)
PhaseBlocked RemediationPhase = "Blocked"
```

**Implementation**:
```go
// Expand "Blocked" to cover resource busy
if shouldSkipDueToResourceBusy {
    rr.Status.OverallPhase = remediationv1.PhaseBlocked
    rr.Status.SkipReason = "ResourceBusy"  // Distinguish from consecutive failures
    rr.Status.BlockingWorkflowExecution = blockingWFE.Name

    return ctrl.Result{RequeueAfter: 30*time.Second}, nil
}
```

**Pros**:
- ✅ No API changes (Blocked already exists and is non-terminal)
- ✅ Gateway deduplication works
- ✅ Documented as non-terminal

**Cons**:
- ⚠️ Semantic overload ("Blocked" for two different reasons)
- ⚠️ Existing monitoring/alerts may conflate the two scenarios
- ⚠️ Documentation updates needed to clarify dual usage

**Confidence**: 80%

---

## 🎯 **Recommendation**

### **Recommended: Option 1 (Pending State) OR Option 4 (Blocked Overload)**

**Option 1 is IDEAL if**:
- We want clean semantics
- Operator clarity is priority
- Willing to accept "Pending" showing for extended time

**Option 4 is PRAGMATIC if**:
- We want zero API changes
- Speed of implementation is priority
- Acceptable to overload "Blocked" phase

---

### **Implementation for Option 1**

#### Change 1: RO Routing Logic (Days 2-5)

**File**: TBD (new routing logic file)

```go
// CheckResourceLock checks if target resource has active WFE
func (r *Reconciler) CheckResourceLock(ctx context.Context, rr *RemediationRequest, targetResource string) (blocked bool, requeueAfter time.Duration, err error) {
    // Find active WFE for same target
    activeWFE := r.FindActiveWFEForTarget(ctx, targetResource)
    if activeWFE == nil {
        return false, 0, nil  // Resource available
    }

    // Resource busy - update status but keep phase non-terminal
    err = helpers.UpdateRemediationRequestStatus(ctx, r.Client, rr, func(rr *RemediationRequest) error {
        // DO NOT change OverallPhase - leave in current state (Pending/Analyzing/etc.)
        rr.Status.SkipReason = "ResourceBusy"
        rr.Status.SkipMessage = fmt.Sprintf("Another workflow is running on target %s: %s", targetResource, activeWFE.Name)
        rr.Status.BlockingWorkflowExecution = activeWFE.Name
        return nil
    })

    return true, 30*time.Second, err  // blocked=true, requeue after 30s
}
```

#### Change 2: Gateway Phase Checker (No Change Needed!)

Gateway already treats `Pending` as non-terminal:
```go
// NON-TERMINAL (deduplicate → update status):
// - Pending, Processing, Analyzing, AwaitingApproval, Executing
```

**Result**: Gateway will continue to deduplicate while RR is in Pending phase

---

### **Implementation for Option 4**

#### Change 1: RO Routing Logic (Days 2-5)

```go
// CheckResourceLock checks if target resource has active WFE
func (r *Reconciler) CheckResourceLock(ctx context.Context, rr *RemediationRequest, targetResource string) (blocked bool, requeueAfter time.Duration, err error) {
    activeWFE := r.FindActiveWFEForTarget(ctx, targetResource)
    if activeWFE == nil {
        return false, 0, nil
    }

    // Resource busy - use Blocked phase (non-terminal)
    err = helpers.UpdateRemediationRequestStatus(ctx, r.Client, rr, func(rr *RemediationRequest) error {
        rr.Status.OverallPhase = remediationv1.PhaseBlocked  // NON-TERMINAL
        rr.Status.SkipReason = "ResourceBusy"  // Distinguish from consecutive failures
        rr.Status.SkipMessage = fmt.Sprintf("Another workflow is running on target %s: %s", targetResource, activeWFE.Name)
        rr.Status.BlockingWorkflowExecution = activeWFE.Name
        return nil
    })

    return true, 30*time.Second, err
}
```

#### Change 2: Documentation Update

**File**: `api/remediation/v1alpha1/remediationrequest_types.go`

```go
// PhaseBlocked indicates remediation is blocked and will retry.
// This is a NON-terminal phase - RO will retry after wait period.
// Reasons for blocking:
// - Consecutive failures: RO will transition to Failed after cooldown (BR-ORCH-042)
// - Resource busy: RO will retry when target resource becomes available (DD-RO-002)
PhaseBlocked RemediationPhase = "Blocked"
```

---

## 🚨 **V1.0 Implementation Impact**

### Timeline Impact

**Current V1.0 Plan**:
- **Days 2-5**: RO routing logic implementation

**Required Changes**:
- **Day 2**: Decide on Option 1 vs. Option 4
- **Days 2-5**: Implement chosen option in routing logic
- **Day 2**: Update documentation/tests

**Timeline Risk**: **LOW** (can be done within existing Days 2-5 window)

---

### Code Changes Required

**Option 1 (Pending State)**:
- ✅ RO routing logic: Keep phase non-terminal
- ✅ Status update: Add SkipReason, SkipMessage, BlockingWorkflowExecution
- ❌ No API changes
- ❌ No Gateway changes

**Option 4 (Blocked Overload)**:
- ✅ RO routing logic: Set phase to Blocked
- ✅ Status update: Add SkipReason, SkipMessage, BlockingWorkflowExecution
- ⚠️ Documentation: Update Blocked phase definition
- ❌ No API changes
- ❌ No Gateway changes

---

## 📋 **Action Items**

### Immediate (Before Days 2-5)

- [ ] **User Decision**: Choose Option 1 (Pending) vs. Option 4 (Blocked)
- [ ] **Update V1.0 Implementation Plan**: Add chosen solution to routing logic design
- [ ] **Update DD-RO-002**: Document phase strategy for resource conflicts

### Days 2-5 (RO Routing Logic)

- [ ] **Implement chosen option** in RO routing logic
- [ ] **Add unit tests** for resource busy scenario
- [ ] **Add integration tests** verifying Gateway deduplication works
- [ ] **Update status field population** (SkipReason, SkipMessage, BlockingWorkflowExecution)

### Documentation

- [ ] **Update BR-ORCH-032**: Clarify phase behavior for resource busy
- [ ] **Update DD-GATEWAY-011**: Confirm deduplication works with chosen phase
- [ ] **Create test scenario**: High-frequency alerts with resource busy

---

## 🎯 **Success Criteria**

### Functional

- ✅ **No RR Flood**: Only 1 active RR for same signal fingerprint at a time
- ✅ **Gateway Deduplication**: Duplicate signals update existing RR status, don't create new RR
- ✅ **Resource Wait**: RR waits (non-terminal phase) until target resource available
- ✅ **Eventual Execution**: Once resource available, RR proceeds to execute workflow

### Test Scenario

```yaml
Given: Prometheus fires alert every 30s for same resource
And: Workflow takes 5 minutes to execute
When: 10 alerts fired over 5 minutes
Then: Only 1 RR created
And: RR.Status.Deduplication.OccurrenceCount = 10
And: RR phase transitions: Pending → Analyzing → Executing → Completed
And: NO intermediate Skipped phase
```

---

## 📚 **References**

- **DD-GATEWAY-011**: Phase-based deduplication (Gateway design)
- **BR-GATEWAY-181**: Deduplication tracking in status
- **BR-ORCH-032**: Resource lock handling (RO responsibility)
- **DD-RO-002**: Centralized routing responsibility
- **BR-ORCH-042**: Consecutive failure blocking

---

**Document Version**: 1.0
**Created**: December 15, 2025
**Status**: 🔴 **BLOCKS V1.0 IMPLEMENTATION**
**Required Decision**: Option 1 (Pending) vs. Option 4 (Blocked)
**Timeline**: Must be resolved before Days 2-5 (RO routing logic)




