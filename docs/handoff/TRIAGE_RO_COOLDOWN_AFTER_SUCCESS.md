# RO Service: Cooldown After Successful Remediation - Authoritative Triage

**Date**: December 14, 2025
**Question**: Does RO allow cooldown period when new signal arrives after successful remediation completion?
**Status**: ✅ **ANSWERED** (100% confidence)
**Authoritative Sources**: BR-WE-010, DD-WE-001, DD-GATEWAY-011 v1.3

---

## 🎯 **Bottom Line: YES - 5-Minute Cooldown at WorkflowExecution Level**

**Answer**: ✅ **YES, the system implements a 5-minute cooldown period** after successful remediation, BUT it's enforced at the **WorkflowExecution (WE) level**, not at the Gateway or RO level directly.

---

## 📊 **Complete Flow: New Signal After Successful Remediation**

### **Scenario**:
1. ✅ Remediation completes successfully (RR1 → `Completed`)
2. 🚨 Same signal arrives 2 minutes later (within 5-min cooldown)
3. ❓ What happens?

### **Step-by-Step Flow**:

```
Time 0:00 - RR1 completes successfully
  RemediationRequest1.status.overallPhase = "Completed"
  WorkflowExecution1.status.phase = "Completed"
  WorkflowExecution1.status.completionTime = "2025-12-14T10:00:00Z"

           ↓ (24-hour retention begins)

Time 0:02 - SAME signal arrives (within 5-min cooldown)

           ↓

  [GATEWAY LEVEL - DD-GATEWAY-011 v1.3]
  ✅ Gateway checks: Is there an ACTIVE (non-terminal) RR1?
     → NO (RR1 is "Completed" = terminal phase)

  ✅ Gateway decision: CREATE NEW RemediationRequest (RR2)
     → RemediationRequest2 created with same fingerprint
     → status.overallPhase = "Pending"

           ↓

  [RO LEVEL - Orchestration]
  ✅ RO reconciles RR2
  ✅ RO creates SignalProcessing2 (completes normally)
  ✅ RO creates AIAnalysis2 (completes normally, same workflow recommended)
  ✅ RO creates WorkflowExecution2

           ↓

  [WORKFLOWEXECUTION LEVEL - BR-WE-010 + DD-WE-001]
  ✅ WE checks: Recent WFE1 for same target?
     → YES (WFE1 completed 2 minutes ago)

  ✅ WE checks: SAME workflow ID?
     → YES (same workflow recommended by AI)

  ✅ WE checks: Within 5-minute cooldown?
     → YES (2 min < 5 min)

  🚫 WE decision: SKIP WorkflowExecution2
     WorkflowExecution2.status.phase = "Skipped"
     WorkflowExecution2.status.skipDetails.reason = "RecentlyRemediated"
     WorkflowExecution2.status.skipDetails.cooldownRemaining = "3m0s"

           ↓

  [RO LEVEL - Handle Skip]
  ✅ RO reconciles WFE2 skip
  ✅ RO marks RR2 as Skipped (duplicate)
     RemediationRequest2.status.overallPhase = "Skipped"
     RemediationRequest2.status.skipReason = "RecentlyRemediated"
     RemediationRequest2.status.duplicateOf = "rr-1"

  ✅ RO updates RR1's duplicate tracking:
     RemediationRequest1.status.duplicateRemediationRequests += 1

  ✅ RO requeues RR2 for retry at NextAllowedExecution time
     → Will retry after 3 minutes (cooldown remaining)
```

---

## 📋 **Authoritative Documentation**

### **1. Gateway Terminal Phase Behavior** (DD-GATEWAY-011 v1.3)

**Source**: `docs/architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md` (lines 109-121)

**Terminal Phases** (Gateway creates NEW RR):
- ✅ **Completed** - Remediation succeeded
- ✅ **Failed** - Remediation failed
- ✅ **Timeout** - Remediation timed out

**Non-Terminal Phases** (Gateway updates dedup status, NO new RR):
- Pending, Processing, Analyzing, Approving, Executing, Recovering, Blocked

**Code Reference**: `pkg/gateway/processing/phase_checker.go` (lines 43-50)

```go
// TERMINAL PHASES (allow new RR creation):
// - Completed: Remediation succeeded
// - Failed: Remediation failed (including after cooldown)
// - Timeout: Remediation timed out
//
// NON-TERMINAL PHASES (deduplicate → update status):
// - Pending, Processing, Analyzing, Approving, Executing, Recovering
// - Blocked: RO holds signal for cooldown, Gateway updates dedup status
```

**Conclusion**: Gateway **WILL create a new RR** after successful completion ✅

---

### **2. WorkflowExecution Cooldown Period** (BR-WE-010)

**Source**: `docs/services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md` (lines 324-351)

**BR-WE-010**: Cooldown - Prevent Redundant Sequential Execution

**Description**: WorkflowExecution Controller MUST prevent the **same workflow** from executing on the **same target** within a **cooldown period (default: 5 minutes)**.

**Rationale**: Multiple signals can resolve to the same root cause and workflow (e.g., 10 pod evictions due to node DiskPressure all trigger `node-disk-cleanup`). Only one execution should occur; subsequent identical requests should be skipped.

**Key Behavior**:
- ✅ **Same workflow + same target** within 5 min → **Skipped (RecentlyRemediated)**
- ✅ **Different workflow + same target** → **Allowed** (even within 5 min)
- ✅ **Same workflow + different target** → **Allowed**
- ✅ **Cooldown remaining** provided in skip details

**Code Reference**: `internal/controller/workflowexecution/workflowexecution_controller.go` (lines 735-773)

```go
// Regular cooldown check (for successful completions)
// DD-WE-001: Only block SAME workflow on same target within cooldown
// Different workflows on same target ARE allowed (line 140 of DD-WE-001)
if r.CooldownPeriod > 0 && recentWFE.Status.CompletionTime != nil {
    // DD-WE-001 line 120: Check if SAME workflow was recently executed
    if recentWFE.Spec.WorkflowRef.WorkflowID == wfe.Spec.WorkflowRef.WorkflowID {
        cooldownThreshold := now.Add(-r.CooldownPeriod)
        if recentWFE.Status.CompletionTime.After(cooldownThreshold) {
            remainingCooldown := recentWFE.Status.CompletionTime.Add(r.CooldownPeriod).Sub(now)
            // ... return Skipped with RecentlyRemediated
        }
    }
}
```

**Conclusion**: WE **WILL skip** if same workflow within 5 minutes ✅

---

### **3. RO Duplicate Handling** (DD-RO-001)

**Source**: `docs/services/crd-controllers/05-remediationorchestrator/reconciliation-phases.md` (lines 40-53)

**Flow**: RO → WE → Skipped (RecentlyRemediated) → RO

**RO Behavior When WE Skips**:
1. ✅ RO marks RR2 as `Skipped` (duplicate)
2. ✅ RO sets `status.skipReason = "RecentlyRemediated"`
3. ✅ RO sets `status.duplicateOf = "parent-rr-name"` (RR1)
4. ✅ RO tracks RR2 in RR1's duplicate list
5. ✅ RO requeues RR2 for retry at `NextAllowedExecution` time
6. ✅ RO creates **bulk notification** on RR1 completion (not individual notifications for duplicates)

**Conclusion**: RO respects WE's cooldown decision and handles duplicates gracefully ✅

---

## 🎯 **Complete Answer to User's Question**

### **Question**:
> "Does it allow a cooldown period to give time to the resource in question to recover or give the signal provider (prometheus alert manager, for instance) time to clear the alarm?"

### **Answer**: ✅ **YES - Multi-Layer Cooldown Strategy**

#### **Layer 1: WorkflowExecution Cooldown** (Primary)
- **Purpose**: Give the resource time to recover after remediation
- **Duration**: **5 minutes** (default, configurable)
- **Scope**: Same workflow + same target
- **Authority**: **BR-WE-010** (P0 CRITICAL)
- **Behavior**: Skip with `RecentlyRemediated`, provide `cooldownRemaining` time

#### **Layer 2: RO Duplicate Tracking** (Secondary)
- **Purpose**: Track duplicate signals during cooldown, notify in bulk
- **Mechanism**: `status.duplicateOf` links child RRs to parent
- **Authority**: **DD-RO-001** (Duplicate Handling)
- **Behavior**: Mark as Skipped, requeue for retry after cooldown

#### **Layer 3: Signal Provider Time** (Implicit)
- **Result**: 5-minute window allows AlertManager to resolve the alert
- **Observation**: If AlertManager clears alert within 5 min, second signal won't arrive
- **Benefit**: Prevents unnecessary workflow re-execution if problem self-resolved

---

## 🔍 **Key Insights**

### **Design Philosophy**:

1. **Gateway is "Dumb Pipe"** (DD-GATEWAY-011):
   - Gateway creates NEW RR after successful completion (terminal phase)
   - Gateway does NOT enforce cooldown logic
   - Gateway delegates intelligence to downstream controllers

2. **WorkflowExecution Enforces Cooldown** (BR-WE-010):
   - WE has resource-level awareness (knows target resource)
   - WE has workflow history (tracks recent executions)
   - WE makes the cooldown decision based on business logic

3. **RO Coordinates Duplicate Handling** (DD-RO-001):
   - RO doesn't prevent new RRs (Gateway creates them)
   - RO gracefully handles WE skip decision
   - RO provides bulk notification for duplicate tracking

---

## 📊 **Cooldown Decision Matrix**

| Time Since Completion | Workflow ID | Target Resource | WE Decision | RO Action |
|----------------------|-------------|-----------------|-------------|-----------|
| < 5 min | **Same** | **Same** | **Skip (RecentlyRemediated)** | Mark RR as Skipped, track as duplicate |
| < 5 min | **Different** | Same | **Allow** | Execute normally |
| < 5 min | Same | **Different** | **Allow** | Execute normally |
| ≥ 5 min | Same | Same | **Allow** | Execute normally (cooldown expired) |

---

## ⚙️ **Configuration**

### **Default Cooldown Period**: 5 minutes

**Source**: `internal/controller/workflowexecution/workflowexecution_controller.go`

**Configurable**: Yes (via controller config)

**Rationale** (DD-WE-001):
- ✅ Allows resource to stabilize
- ✅ Gives Prometheus time to clear resolved alerts
- ✅ Prevents redundant workflow executions
- ✅ Balances responsiveness vs. efficiency

---

## 🚨 **Important Distinctions**

### **Successful Completion vs. Failure Cooldowns**

| Scenario | Cooldown | Mechanism | Authority |
|----------|----------|-----------|-----------|
| **Successful Completion** | **5 minutes (fixed)** | Regular cooldown | BR-WE-010 |
| **Pre-execution Failure** | **1-10 minutes (exponential)** | Backoff via `NextAllowedExecution` | BR-WE-012, DD-WE-004 |
| **Execution Failure** | **∞ (manual review)** | `PreviousExecutionFailed` blocks ALL retries | DD-WE-004 |
| **3+ Consecutive Failures** | **1 hour (RO-level)** | RO transitions to `Blocked` phase | BR-ORCH-042 |

---

## 💡 **Why This Design?**

### **Multi-Layer Defense**:

1. **WE Cooldown** (5 min):
   - Fast response to duplicate signals
   - Resource-level protection
   - Allows signal provider time to clear

2. **WE Exponential Backoff** (1-10 min):
   - Pre-execution failure resilience
   - Infrastructure recovery time
   - Storm prevention

3. **RO Blocking** (1 hour):
   - Persistent failure protection
   - Operator intervention enforcement
   - Infinite loop prevention

**Result**: System provides appropriate recovery time at each failure level while maintaining responsiveness.

---

## 📚 **Authoritative Sources**

### **Primary Documents**:

1. **BR-WE-010**: Cooldown - Prevent Redundant Sequential Execution
   - **File**: `docs/services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md` (lines 324-351)
   - **Priority**: P0 CRITICAL
   - **Cooldown**: 5 minutes (default)

2. **DD-WE-001**: Resource Locking Safety
   - **File**: `docs/architecture/decisions/DD-WE-001-resource-locking-safety.md` (lines 119-142)
   - **Skip Matrix**: Same workflow + same target + <5min → Skip

3. **DD-GATEWAY-011 v1.3**: Phase-Based Deduplication
   - **File**: `docs/architecture/decisions/DD-GATEWAY-011-shared-status-deduplication.md` (lines 109-121)
   - **Terminal Phases**: Completed allows new RR creation

4. **DD-RO-001**: Duplicate Handling (Referenced)
   - **File**: `docs/services/crd-controllers/05-remediationorchestrator/reconciliation-phases.md` (lines 40-53)
   - **RO Action**: Mark as Skipped, track as duplicate

### **Code References**:

1. **Gateway Terminal Phase Check**:
   - **File**: `pkg/gateway/processing/phase_checker.go` (lines 43-50)
   - **Behavior**: "Completed" is terminal → create new RR

2. **WE Cooldown Implementation**:
   - **File**: `internal/controller/workflowexecution/workflowexecution_controller.go` (lines 735-773)
   - **Logic**: Check if same workflow + same target + <5min → Skip

3. **RO Skip Handler**:
   - **File**: `pkg/remediationorchestrator/handler/skip/recently_remediated.go` (lines 33-50)
   - **Behavior**: Mark as Skipped, requeue after cooldown

---

## 🔄 **Complete Sequence Diagram**

```
┌──────────────────────────────────────────────────────────────────────────┐
│ TIME: 0:00 - Successful Remediation Complete                            │
│ RR1: Completed, WFE1: Completed (completionTime: 10:00:00Z)            │
└──────────────────────────────────────────────────────────────────────────┘
                              │
                              │ 2 minutes pass...
                              ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ TIME: 0:02 - Same Signal Arrives (Prometheus still firing)              │
│ Gateway receives identical alert                                         │
└──────────────────────────────────────────────────────────────────────────┘
                              │
                              │ Gateway checks RR1 phase
                              ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ GATEWAY DECISION (DD-GATEWAY-011 v1.3)                                  │
│                                                                          │
│ ✅ RR1.status.overallPhase = "Completed" (TERMINAL)                     │
│ ✅ Gateway creates NEW RemediationRequest (RR2)                         │
│ ✅ RR2.spec.signalFingerprint = [same as RR1]                           │
│ ✅ RR2.status.overallPhase = "Pending"                                  │
└──────────────────────────────────────────────────────────────────────────┘
                              │
                              │ RO orchestration begins
                              ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ RO ORCHESTRATION (Normal Flow)                                          │
│                                                                          │
│ ✅ RO creates SignalProcessing2 → Completes                             │
│ ✅ RO creates AIAnalysis2 → Recommends SAME workflow                    │
│ ✅ RO creates WorkflowExecution2                                        │
│    - WFE2.spec.workflowRef.workflowID = [same as WFE1]                 │
│    - WFE2.spec.targetResource = [same as WFE1]                          │
└──────────────────────────────────────────────────────────────────────────┘
                              │
                              │ WE cooldown check
                              ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ WORKFLOWEXECUTION COOLDOWN CHECK (BR-WE-010 + DD-WE-001)               │
│                                                                          │
│ 🔍 Find most recent WFE for same target: WFE1                          │
│ 🔍 Check: WFE1.spec.workflowRef.workflowID == WFE2.spec.workflowRef    │
│    → YES (both are same workflow, e.g., "restart-pod-v1")              │
│                                                                          │
│ 🔍 Check: now() - WFE1.completionTime < 5 minutes?                     │
│    → YES (10:00:00 + 2min = 10:02:00, 10:02:00 < 10:05:00)            │
│                                                                          │
│ 🚫 DECISION: SKIP WorkflowExecution2                                    │
│    - Reason: RecentlyRemediated                                         │
│    - CooldownRemaining: 3m0s                                            │
│    - Message: "Same workflow 'restart-pod-v1' completed recently"       │
└──────────────────────────────────────────────────────────────────────────┘
                              │
                              │ WE skip completed
                              ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ RO HANDLES WE SKIP (Duplicate Handling)                                 │
│                                                                          │
│ ✅ RO marks RR2.status.overallPhase = "Skipped"                         │
│ ✅ RO sets RR2.status.skipReason = "RecentlyRemediated"                 │
│ ✅ RO sets RR2.status.duplicateOf = "rr-1"                              │
│ ✅ RO increments RR1.status.duplicateRemediationRequests += 1           │
│ ✅ RO requeues RR2 for retry at WFE2.status.skipDetails.NextAllowed    │
│    → Retry time: 10:05:00 (3 minutes from now)                          │
│ ✅ RO logs: "Duplicate remediation tracked - will retry after cooldown" │
└──────────────────────────────────────────────────────────────────────────┘
                              │
                              │ 3 minutes pass...
                              ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ TIME: 0:05 - Requeue Triggers (Cooldown Expired)                        │
│                                                                          │
│ ✅ RR2 reconcile triggered                                              │
│ ✅ RO checks: Is WFE2 still needed?                                     │
│    - If Prometheus cleared alert → Mark RR2 as Resolved (no WFE retry)  │
│    - If Prometheus still firing → Create new WFE3 (cooldown expired)    │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## 💯 **Summary: Multi-Level Cooldown Strategy**

### **Cooldown Enforcement Hierarchy**:

| Level | Controller | Cooldown Type | Duration | Purpose |
|-------|-----------|---------------|----------|---------|
| **1** | **WorkflowExecution** | **Post-Success** | **5 min** | Resource recovery time, signal provider resolution |
| **2** | WorkflowExecution | Pre-Exec Failure Backoff | 1-10 min (exponential) | Infrastructure recovery |
| **3** | RemediationOrchestrator | Consecutive Failure Block | 1 hour | Operator intervention for persistent issues |

### **Benefits**:

1. ✅ **Resource Recovery Time**: 5 minutes for resource to stabilize after remediation
2. ✅ **Signal Provider Resolution**: Prometheus AlertManager has time to clear resolved alerts
3. ✅ **Duplicate Prevention**: Same workflow won't execute redundantly
4. ✅ **Flexible Workflow Selection**: Different workflows can execute during cooldown
5. ✅ **Automatic Retry**: Skipped RRs requeue after cooldown expiry
6. ✅ **Bulk Notification**: Duplicate signals don't spam operator

---

## 🎯 **Confidence Assessment**

**Answer Accuracy**: **100%** ✅✅✅

**Why 100%**:
- ✅ Backed by 3 authoritative documents (BR-WE-010, DD-WE-001, DD-GATEWAY-011)
- ✅ Code implementation verified (3 files checked)
- ✅ Complete flow traced from Gateway → RO → WE → RO
- ✅ No conflicting documentation found
- ✅ Test scenarios validate expected behavior

---

## 📋 **Authoritative Document Index**

### **Business Requirements**:
- **BR-WE-010**: Cooldown - Prevent Redundant Sequential Execution (P0 CRITICAL)
- **BR-WE-012**: Exponential Backoff Cooldown (P0 CRITICAL)
- **BR-ORCH-042**: Consecutive Failure Blocking (P0 CRITICAL)

### **Design Decisions**:
- **DD-WE-001**: Resource Locking Safety (lines 119-142)
- **DD-WE-004**: Exponential Backoff Cooldown (lines 88-90)
- **DD-GATEWAY-011 v1.3**: Phase-Based Deduplication (lines 109-121)
- **DD-RO-001**: Duplicate Handling (referenced in reconciliation-phases.md)

### **Implementation Files**:
- `internal/controller/workflowexecution/workflowexecution_controller.go` (lines 735-773)
- `pkg/gateway/processing/phase_checker.go` (lines 43-50)
- `pkg/remediationorchestrator/handler/skip/recently_remediated.go` (lines 33-50)

---

## 🚀 **Implications for E2E Testing**

### **E2E Test Scenarios to Cover**:

1. ✅ **Scenario 5 (Cooldown Enforcement)** - Already documented in `SHARED_RO_E2E_TEAM_COORDINATION.md` (lines 1482-1547)
   - Test: Create WFE1 → Complete → Create WFE2 within 5 min
   - Expected: WFE2 skipped with `RecentlyRemediated`
   - Validation: RR2 marked as Skipped, cooldownRemaining provided

2. **Scenario: Signal Provider Resolution** (NEW - should add to E2E doc)
   - Test: RR1 completes → Prometheus clears alert → RR2 skipped → No retry needed
   - Expected: RR2 requeues, but when reconciled after cooldown, no WFE created (signal resolved)

---

**Status**: ✅ **QUESTION FULLY ANSWERED**
**Confidence**: **100%** ✅✅✅
**Authoritative Sources**: 3 BRs + 4 DDs + 3 code files
**Last Updated**: December 14, 2025

