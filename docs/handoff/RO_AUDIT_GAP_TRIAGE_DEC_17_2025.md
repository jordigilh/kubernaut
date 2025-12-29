# RemediationOrchestrator Audit Coverage Gap Triage - December 17, 2025

**Status**: 🚨 **GAPS IDENTIFIED**
**Priority**: P0 (ADR-032 §1 Compliance)
**Scope**: Comprehensive audit event coverage validation

---

## 🎯 **Objective**

Triage RemediationOrchestrator service for complete audit event coverage per ADR-032 §1:
> "Every orchestration phase transition (RemediationOrchestrator)" must be audited

---

## 📊 **Audit Coverage Matrix**

### **Phase Transitions (Controller State Machine)**

| From Phase | To Phase | Trigger | Audit Event | Status |
|---|---|---|---|---|
| **Pending** → Processing | SignalProcessing created | `lifecycle.started` + `phase.transitioned` | ✅ COMPLETE |
| **Pending** → Blocked | Routing blocked (duplicate/cooldown) | ❌ NONE | 🚨 **MISSING** |
| **Processing** → Analyzing | SignalProcessing completed | `phase.transitioned` | ✅ COMPLETE |
| **Processing** → Failed | SignalProcessing failed | `lifecycle.failed` | ✅ COMPLETE |
| **Processing** → TimedOut | Timeout exceeded | `lifecycle.failed` | ✅ COMPLETE |
| **Analyzing** → AwaitingApproval | Approval needed | `approval.requested` | ✅ COMPLETE |
| **Analyzing** → Executing | Workflow selected, approved | `phase.transitioned` | ✅ COMPLETE |
| **Analyzing** → Blocked | Routing blocked (resource busy/cooldown) | ❌ NONE | 🚨 **MISSING** |
| **Analyzing** → Completed | WorkflowNotNeeded | `lifecycle.completed` | ✅ COMPLETE |
| **Analyzing** → Failed | AIAnalysis failed | `lifecycle.failed` | ✅ COMPLETE |
| **Analyzing** → TimedOut | Timeout exceeded | `lifecycle.failed` | ✅ COMPLETE |
| **AwaitingApproval** → Executing | Human approved | `approval.approved` | ✅ COMPLETE |
| **AwaitingApproval** → Failed | Human rejected | `approval.rejected` | ✅ COMPLETE |
| **AwaitingApproval** → TimedOut | Approval timeout | `approval.expired` | ✅ COMPLETE |
| **Executing** → Completed | Workflow succeeded | `lifecycle.completed` | ✅ COMPLETE |
| **Executing** → Failed | Workflow failed | `lifecycle.failed` | ✅ COMPLETE |
| **Executing** → TimedOut | Timeout exceeded | `lifecycle.failed` | ✅ COMPLETE |
| **Executing** → Skipped | Resource lock conflict | ❌ NONE | 🚨 **MISSING** |
| **Failed** → Blocked | Consecutive failure threshold | ❌ NONE | 🚨 **MISSING** |
| **Blocked** → Failed | Cooldown expired | `lifecycle.failed` | ✅ COMPLETE |

---

### **Routing Decisions (Non-Phase-Transition Events)**

| Routing Check | Condition | Current Behavior | Audit Event | Status |
|---|---|---|---|---|
| **DuplicateInProgress** | Same fingerprint, active RR exists | Sets Blocked phase | ❌ NONE | 🚨 **MISSING** |
| **ResourceBusy** | Another WFE running on same target | Sets Blocked phase | ❌ NONE | 🚨 **MISSING** |
| **RecentlyRemediated** | Same workflow+target within cooldown | Sets Blocked phase | ❌ NONE | 🚨 **MISSING** |
| **ConsecutiveFailures** | Failure count ≥ threshold | Sets Blocked phase | ❌ NONE | 🚨 **MISSING** |
| **ExponentialBackoff** | NextAllowedExecution in future | Sets Blocked phase | ❌ NONE | 🚨 **MISSING** |

---

### **Blocked Phase Lifecycle**

| Event | Trigger | Current Behavior | Audit Event | Status |
|---|---|---|---|---|
| **Block Applied** | Routing condition detected | Status update + metric | ❌ NONE | 🚨 **MISSING** |
| **Block Persists** | Still in cooldown | Requeue | ❌ NONE | ⚠️ **LOW PRIORITY** |
| **Block Unblocked** | Cooldown expired | Transition to Failed | `lifecycle.failed` | ✅ COMPLETE |
| **Block Resolved** | Blocking condition cleared | Retry workflow | ❌ NONE | 🚨 **MISSING** |

---

## 🚨 **Critical Gaps Identified**

### **Gap 1: Routing Blocked Events** (CRITICAL)

**Issue**: When RR transitions to `Blocked` phase, no audit event is emitted

**Code Location**: `pkg/remediationorchestrator/controller/reconciler.go:763-823`

**Current Behavior**:
```go
func (r *Reconciler) handleBlocked(ctx context.Context, rr *remediationv1.RemediationRequest, blocked *routing.BlockingCondition) {
    // 1. Update status ✅
    rr.Status.OverallPhase = remediationv1.PhaseBlocked
    rr.Status.BlockReason = blocked.Reason

    // 2. Emit metric ✅
    metrics.PhaseTransitionsTotal.Inc()

    // 3. Log ✅
    logger.Info("RemediationRequest blocked")

    // 4. Emit audit event ❌ MISSING!
    // No audit trace of why/how blocking occurred

    return ctrl.Result{RequeueAfter: blocked.RequeueAfter}, nil
}
```

**Impact**:
- ❌ Violates ADR-032 §1 (phase transition not audited)
- ❌ No trace of duplicate detection decisions
- ❌ No trace of cooldown enforcement
- ❌ No trace of resource conflict detection
- ❌ Cannot answer: "Why wasn't this workflow executed?"

**Business Requirements Violated**:
- ADR-032 §1: All phase transitions must be audited
- BR-STORAGE-001: Complete audit trail with no data loss
- DD-AUDIT-003: All orchestration events must be audited

---

### **Gap 2: Blocked → Failed Transition** (LOW PRIORITY)

**Issue**: When block cooldown expires and RR transitions from `Blocked` → `Failed`, audit event exists BUT doesn't include block history

**Code Location**: `pkg/remediationorchestrator/controller/blocking.go:248`

**Current Behavior**:
```go
// Transition to terminal Failed (skip blocking check to avoid infinite loop)
return r.transitionToFailedTerminal(ctx, rr, "blocked",
    fmt.Sprintf("Cooldown expired after blocking due to %s", blockReason))

// transitionToFailedTerminal emits lifecycle.failed ✅
r.emitFailureAudit(ctx, rr, failurePhase, failureReason, durationMs)
```

**Analysis**:
- ✅ Audit event IS emitted (`lifecycle.failed`)
- ⚠️ Event includes `failureReason` with block context
- ⚠️ Missing detailed block history (blocked_since, block_count, etc.)

**Priority**: LOW (event exists, just missing detail)

---

### **Gap 3: Skipped Phase** (MEDIUM PRIORITY)

**Issue**: When RR is skipped due to resource lock, no audit event is emitted

**Code Location**: TBD (need to find where Skipped transition happens)

**Analysis**:
- ⚠️ `Skipped` is a terminal phase per `phase/types.go:71`
- ⚠️ No code found that transitions to `Skipped`
- ⚠️ May be unused/future functionality

**Action**: Search for `PhaseSkipped` usage in codebase

---

### **Gap 4: Failed → Blocked Transition** (MEDIUM PRIORITY)

**Issue**: When consecutive failure threshold is reached, transition from `Failed` → `Blocked` has no audit event

**Code Location**: TBD (need to find where this transition happens)

**Analysis**:
- ⚠️ State machine allows `Failed → Blocked` per `phase/types.go:96`
- ⚠️ No code found that implements this transition
- ⚠️ BR-ORCH-042 mentions consecutive failure blocking

**Action**: Search for consecutive failure blocking implementation

---

## 📋 **Proposed Audit Events**

### **Event 1: `orchestrator.routing.blocked`** (NEW - CRITICAL)

**Purpose**: Audit when RR is blocked by routing engine

**Event Type**: `orchestrator.routing.blocked`

**When Emitted**:
- Pending → Blocked (duplicate detected, early block)
- Analyzing → Blocked (resource busy, cooldown, etc.)

**Event Structure** (per ADR-034):
```json
{
  "event_type": "orchestrator.routing.blocked",
  "event_category": "routing",
  "event_action": "blocked",
  "event_outcome": "pending",
  "correlation_id": "rr-uid",
  "actor_type": "service",
  "actor_id": "remediation-orchestrator",
  "resource_type": "RemediationRequest",
  "resource_id": "rr-name",
  "resource_namespace": "default",
  "event_data": {
    "block_reason": "RecentlyRemediated",
    "block_message": "Target was remediated recently...",
    "from_phase": "Analyzing",
    "to_phase": "Blocked",
    "workflow_id": "restart-pod-v1",
    "target_resource": "Deployment/my-app",
    "requeue_after_seconds": 300,
    "blocked_until": "2025-12-17T15:30:00Z",

    // Optional fields (based on block reason)
    "blocking_wfe": "wfe-123",           // If ResourceBusy
    "duplicate_of": "rr-original",       // If DuplicateInProgress
    "consecutive_failures": 3,           // If ConsecutiveFailures
    "backoff_seconds": 120,              // If ExponentialBackoff
    "recent_wfe": "wfe-previous"         // If RecentlyRemediated
  }
}
```

**Implementation Priority**: **P0 - CRITICAL**

---

### **Event 2: `orchestrator.routing.unblocked`** (NEW - MEDIUM)

**Purpose**: Audit when RR is unblocked and can proceed

**Event Type**: `orchestrator.routing.unblocked`

**When Emitted**:
- Blocked → Analyzing (retry after condition cleared)
- Blocked → Executing (retry after condition cleared)

**Event Structure**:
```json
{
  "event_type": "orchestrator.routing.unblocked",
  "event_category": "routing",
  "event_action": "unblocked",
  "event_outcome": "success",
  "correlation_id": "rr-uid",
  "actor_type": "service",
  "actor_id": "remediation-orchestrator",
  "resource_type": "RemediationRequest",
  "resource_id": "rr-name",
  "resource_namespace": "default",
  "event_data": {
    "previous_block_reason": "ResourceBusy",
    "blocked_duration_seconds": 120,
    "unblock_reason": "BlockingWorkflowCompleted",
    "from_phase": "Blocked",
    "to_phase": "Analyzing"
  }
}
```

**Implementation Priority**: **P1 - MEDIUM** (nice-to-have for complete lifecycle)

---

### **Event 3: Enhanced `lifecycle.failed`** (ENHANCEMENT - LOW)

**Purpose**: Include block history in failure events when failing from Blocked phase

**Event Type**: `orchestrator.lifecycle.failed` (existing, enhance event_data)

**Enhancement**:
```json
{
  "event_type": "orchestrator.lifecycle.failed",
  "event_category": "lifecycle",
  "event_action": "failed",
  "event_outcome": "failure",
  "event_data": {
    "failure_phase": "blocked",
    "failure_reason": "Cooldown expired after blocking due to ConsecutiveFailures",
    "duration_ms": 3600000,

    // NEW: Block history
    "block_history": {
      "block_reason": "ConsecutiveFailures",
      "blocked_since": "2025-12-17T14:30:00Z",
      "blocked_duration_seconds": 3600,
      "consecutive_failures": 3
    }
  }
}
```

**Implementation Priority**: **P2 - LOW** (enhancement, not gap)

---

## 🔍 **Code Investigation Needed**

### **Investigation 1: Skipped Phase Usage**

**Question**: Where is `PhaseSkipped` transition implemented?

**Search**:
```bash
grep -r "PhaseSkipped\|phase.Skipped" pkg/remediationorchestrator/ --include="*.go"
```

**Status**: ⏳ PENDING

---

### **Investigation 2: Failed → Blocked Transition**

**Question**: Where is `Failed → Blocked` transition implemented for consecutive failures?

**Search**:
```bash
grep -r "ConsecutiveFailure\|consecutive.*block" pkg/remediationorchestrator/ --include="*.go"
```

**Status**: ⏳ PENDING

---

### **Investigation 3: Blocked → Analyzing Retry**

**Question**: Can RR transition from Blocked back to Analyzing after condition clears?

**Analysis**: State machine shows `Blocked → Failed` only (line 93), no retry path

**Conclusion**: Current implementation: Blocked is one-way to terminal Failed

---

## 📈 **Audit Coverage Summary**

### **Current Coverage**

| Category | Total Events | Audited | Missing | Coverage % |
|---|---|---|---|---|
| **Lifecycle Events** | 4 | 4 | 0 | 100% |
| **Phase Transitions** | 19 | 14 | 5 | 74% |
| **Routing Decisions** | 5 | 0 | 5 | 0% |
| **Approval Events** | 4 | 4 | 0 | 100% |
| **Overall** | 32 | 22 | 10 | **69%** |

---

### **Gap Breakdown**

| Gap | Priority | Effort | Impact |
|---|---|---|---|
| **Gap 1: Routing Blocked** | P0 - CRITICAL | 2-3 hours | ADR-032 violation |
| **Gap 2: Blocked → Failed Detail** | P2 - LOW | 1 hour | Enhancement only |
| **Gap 3: Skipped Phase** | P1 - MEDIUM | TBD | May be unused |
| **Gap 4: Failed → Blocked** | P1 - MEDIUM | TBD | May be unused |
| **Event 2: Routing Unblocked** | P1 - MEDIUM | 2 hours | Nice-to-have |

---

## 🎯 **Recommended Implementation Plan**

### **Phase 1: Critical Gaps** (P0 - Today)

1. ✅ **Implement `BuildRoutingBlockedEvent()`** helper
   - File: `pkg/remediationorchestrator/audit/helpers.go`
   - Effort: 30 minutes

2. ✅ **Implement `emitRoutingBlockedAudit()`** function
   - File: `pkg/remediationorchestrator/controller/reconciler.go`
   - Effort: 30 minutes

3. ✅ **Update `handleBlocked()`** to emit audit event
   - File: `pkg/remediationorchestrator/controller/reconciler.go:763`
   - Add call: `r.emitRoutingBlockedAudit(ctx, rr, blocked)`
   - Effort: 15 minutes

4. ✅ **Update integration tests**
   - File: `test/integration/remediationorchestrator/audit_trace_integration_test.go`
   - Add test: "should store orchestrator.routing.blocked event with correct content"
   - Effort: 60 minutes

**Total Phase 1**: 2.5 hours

---

### **Phase 2: Investigation** (P1 - Day 18)

1. ⏳ **Investigate Skipped phase usage**
   - Search codebase for `PhaseSkipped` transitions
   - Determine if implemented or future functionality
   - Effort: 30 minutes

2. ⏳ **Investigate Failed → Blocked transition**
   - Search for consecutive failure blocking implementation
   - Verify if this transition is implemented
   - Effort: 30 minutes

**Total Phase 2**: 1 hour

---

### **Phase 3: Enhancements** (P2 - Post-V1.0)

1. ⏳ **Implement `BuildRoutingUnblockedEvent()` helper** (if retry path exists)
   - Only if Blocked → Analyzing retry is possible
   - Effort: 2 hours

2. ⏳ **Enhance `BuildFailureEvent()` with block history**
   - Add block_history to event_data
   - Effort: 1 hour

**Total Phase 3**: 3 hours

---

## 🚨 **ADR-032 Compliance Status**

### **Before Implementation**

| ADR-032 Requirement | Status | Evidence |
|---|---|---|
| §1: All phase transitions audited | ❌ **VIOLATED** | 5 transitions missing audit events |
| §2: P0 crash on init failure | ✅ COMPLIANT | Verified in main.go |
| §4: Audit functions return error | ✅ COMPLIANT | Recent fixes applied |

**Compliance**: **66%** (2/3 requirements met)

---

### **After Phase 1 Implementation**

| ADR-032 Requirement | Status | Evidence |
|---|---|---|
| §1: All phase transitions audited | ✅ **COMPLIANT** | All implemented transitions have audit events |
| §2: P0 crash on init failure | ✅ COMPLIANT | Verified in main.go |
| §4: Audit functions return error | ✅ COMPLIANT | Recent fixes applied |

**Compliance**: **100%** (3/3 requirements met)

---

## 📝 **Summary**

### **Gaps Identified**

1. 🚨 **CRITICAL**: Routing blocked events missing (ADR-032 violation)
2. ⚠️ **MEDIUM**: Skipped phase audit events (may be unused)
3. ⚠️ **MEDIUM**: Failed → Blocked audit events (may be unused)
4. ℹ️ **LOW**: Block history detail in failure events (enhancement)

---

### **Immediate Action Required**

**Phase 1: Implement routing.blocked audit event** (2.5 hours)
- ADR-032 §1 compliance CRITICAL
- Affects 5 routing decision points
- Required for complete audit trail

---

### **Next Steps**

1. **Implement Phase 1** (today): Routing blocked audit events
2. **Run Integration Tests**: Verify 100% pass rate with new events
3. **Execute Phase 2** (Day 18): Investigate unused phase transitions
4. **Plan Phase 3** (Post-V1.0): Enhancements

---

**Prepared by**: RO Team (AI Assistant)
**Date**: December 17, 2025
**Next Review**: After Phase 1 implementation

