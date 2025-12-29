# WorkflowExecution V1.0 Routing Removal - Triage & Implementation Plan

**Date**: December 15, 2025
**From**: WE Team (Platform AI)
**Status**: ✅ **READY TO IMPLEMENT**
**Handoff Document**: [WE_TEAM_V1.0_ROUTING_HANDOFF.md](./WE_TEAM_V1.0_ROUTING_HANDOFF.md)

---

## 🎯 **Executive Summary**

**Objective**: Remove ALL routing logic from WorkflowExecution controller as RO now handles all routing decisions

**Scope**: Days 6-7 (16 hours / 2 days)
- **Day 6**: Remove routing functions and simplify reconcilePending
- **Day 7**: Update tests and documentation

**Impact**: -170 lines of routing code, WE becomes pure executor

---

## 📋 **Triage Analysis**

### **1. Routing Functions to Remove** ⚠️ **CRITICAL**

| Function | Lines | Used By | Status |
|----------|-------|---------|--------|
| **CheckCooldown** | 637-776 (140 lines) | reconcilePending | ❌ **REMOVE** |
| **FindMostRecentTerminalWFE** | 783-840 (58 lines) | CheckCooldown | ❌ **REMOVE** |
| **CheckResourceLock** | 568-622 (55 lines) | reconcilePending | ❌ **REMOVE** |
| **MarkSkipped** | 994-1061 (68 lines) | reconcilePending | ❌ **REMOVE** |

**Total Lines to Remove**: ~321 lines

---

### **2. reconcilePending Simplification** ⚠️ **CRITICAL**

**Current Flow** (4 steps with routing):
1. Step 0: Validate spec
2. Step 1: CheckResourceLock (❌ REMOVE)
3. Step 2: CheckCooldown (❌ REMOVE)
4. Step 3: Build and create PipelineRun (✅ KEEP)
5. Step 4: Update status to Running (✅ KEEP)

**Simplified Flow** (3 steps, pure execution):
1. Validate spec
2. Build and create PipelineRun
3. Update status to Running

**HandleAlreadyExists**: ✅ **KEEP** (execution-time safety, DD-WE-003 Layer 2)

---

### **3. Metrics to Remove** 📊

**File**: `internal/controller/workflowexecution/metrics.go`

| Metric | Lines | Status |
|--------|-------|--------|
| **WorkflowExecutionSkipTotal** | 71-80 | ❌ **REMOVE** |
| **BackoffSkipTotal** | 86-95 | ❌ **REMOVE** |
| **ConsecutiveFailuresGauge** | 97-106 | ❌ **REMOVE** |
| **RecordWorkflowSkip** | 143-147 | ❌ **REMOVE** |
| **RecordBackoffSkip** | 153-157 | ❌ **REMOVE** |
| **SetConsecutiveFailures** | 159-162 | ❌ **REMOVE** |
| **ResetConsecutiveFailures** | 164-167 | ❌ **REMOVE** |

**Keep**: WorkflowExecutionTotal, WorkflowExecutionDuration, PipelineRunCreationTotal (execution metrics)

---

### **4. Stub Types to Remove** 🗑️

**File**: `internal/controller/workflowexecution/v1_compat_stubs.go`

**Action**: ❌ **DELETE ENTIRE FILE**

**Rationale**: All routing logic removed, stubs no longer needed

**Types in File**:
- `SkipDetails`
- `ConflictingWorkflowRef`
- `RecentRemediationRef`
- `PhaseSkipped` constant
- `SkipReason*` constants

---

### **5. Tests to Remove** 🧪

**File**: `test/unit/workflowexecution/controller_test.go`

**Estimated Routing Tests to Remove**: ~15 tests

**Test Categories to Remove**:
- `Describe("CheckCooldown", ...)`
- `Describe("MarkSkipped", ...)`
- `Describe("Recently Remediated Skip", ...)`
- `Describe("Resource Lock Skip", ...)`
- `Describe("Exhausted Retries Skip", ...)`
- `Describe("Previous Execution Failed Skip", ...)`

**Tests to Keep**:
- `Describe("reconcilePending - CreatePipelineRun", ...)`
- `Describe("reconcilePending - SpecValidation", ...)`
- `Describe("HandleAlreadyExists", ...)`
- `Describe("PipelineRun Monitoring", ...)`
- `Describe("Failure Handling", ...)`

**Expected**: ~50 tests → ~35 tests (-15 routing tests)

---

### **6. Documentation to Update** 📝

**Files**:
1. `docs/services/crd-controllers/03-workflowexecution/reconciliation-phases.md`
2. `docs/services/crd-controllers/03-workflowexecution/controller-implementation.md`

**Changes**:
- ❌ Remove sections about cooldown checks
- ❌ Remove sections about skip logic
- ❌ Remove sections about resource locking
- ✅ Add reference to DD-RO-002 (routing moved to RO)
- ✅ Keep sections about execution logic
- ✅ Keep sections about HandleAlreadyExists (execution safety)

---

## 🚀 **Implementation Plan**

### **Day 6: Routing Logic Removal** (8 hours)

#### **Task 1: Remove Routing Functions** (2 hours)

**Changes**:
1. Remove `CheckCooldown()` (lines 624-776)
2. Remove `FindMostRecentTerminalWFE()` (lines 778-840)
3. Remove `CheckResourceLock()` (lines 568-622)
4. Remove `MarkSkipped()` (lines 990-1061)

**Total Removal**: ~321 lines

---

#### **Task 2: Simplify reconcilePending** (1 hour)

**Remove** (lines 204-232):
```go
// ========================================
// Step 1: Check resource lock (DD-WE-001)
// ========================================
blocked, skipDetails, err := r.CheckResourceLock(ctx, wfe)
if err != nil {
    logger.Error(err, "Failed to check resource lock")
    return ctrl.Result{}, err
}
if blocked {
    logger.Info("Resource is locked, skipping execution",
        "reason", skipDetails.Reason,
    )
    return ctrl.Result{}, r.MarkSkipped(ctx, wfe, skipDetails)
}

// ========================================
// Step 2: Check cooldown (DD-WE-001)
// ========================================
blocked, skipDetails, err = r.CheckCooldown(ctx, wfe)
if err != nil {
    logger.Error(err, "Failed to check cooldown")
    return ctrl.Result{}, err
}
if blocked {
    logger.Info("Cooldown active, skipping execution",
        "reason", skipDetails.Reason,
    )
    return ctrl.Result{}, r.MarkSkipped(ctx, wfe, skipDetails)
}
```

**Keep**: HandleAlreadyExists (execution-time safety)

---

#### **Task 3: Remove Skip Metrics** (30 minutes)

**File**: `internal/controller/workflowexecution/metrics.go`

**Remove**:
- Variable declarations (lines 71-106)
- Registration (lines 115-118)
- Helper functions (lines 143-167)

**Keep**:
- WorkflowExecutionTotal
- WorkflowExecutionDuration
- PipelineRunCreationTotal
- RecordWorkflowCompletion
- RecordWorkflowFailure
- RecordPipelineRunCreation

---

#### **Task 4: Delete Stub File** (5 minutes)

**Action**: Delete `internal/controller/workflowexecution/v1_compat_stubs.go`

---

#### **Task 5: Build Verification** (30 minutes)

**Commands**:
```bash
# Build WE controller
make build-workflowexecution

# Expected: ✅ Build succeeds
```

---

### **Day 7: Tests & Documentation** (8 hours)

#### **Task 6: Remove Routing Tests** (3 hours)

**File**: `test/unit/workflowexecution/controller_test.go`

**Search for and remove**:
```bash
grep -n "CheckCooldown\|MarkSkipped\|ResourceLock\|Cooldown\|Skip" test/unit/workflowexecution/controller_test.go
```

**Remove test blocks**:
- CheckCooldown tests
- MarkSkipped tests
- Resource lock tests
- Cooldown tests
- Skip reason tests

---

#### **Task 7: Verify Execution Tests** (2 hours)

**Commands**:
```bash
# Run WE unit tests
make test-unit-workflowexecution

# Expected: ~35 tests passing
```

**If failures**:
- Check for references to removed functions
- Update test expectations
- Fix imports

---

#### **Task 8: Update Documentation** (2 hours)

**Files**:
1. `docs/services/crd-controllers/03-workflowexecution/reconciliation-phases.md`
   - Remove: Cooldown phase descriptions
   - Remove: Skip phase descriptions
   - Add: Reference to DD-RO-002

2. `docs/services/crd-controllers/03-workflowexecution/controller-implementation.md`
   - Remove: Routing logic sections
   - Update: reconcilePending flow diagram
   - Add: "Routing moved to RO" section

---

#### **Task 9: Lint Checks** (1 hour)

**Commands**:
```bash
# Run linter
golangci-lint run ./internal/controller/workflowexecution/... ./test/unit/workflowexecution/...

# Expected: No unused function errors
```

---

## ✅ **Success Criteria**

### **Day 6 Deliverables**

- [ ] CheckCooldown function removed
- [ ] FindMostRecentTerminalWFE removed
- [ ] CheckResourceLock removed
- [ ] MarkSkipped function removed
- [ ] reconcilePending simplified (no routing logic)
- [ ] WE skip metrics removed
- [ ] v1_compat_stubs.go deleted
- [ ] Build succeeds: `make build-workflowexecution`

### **Day 7 Deliverables**

- [ ] 15 routing tests removed
- [ ] ~35 execution tests passing: `make test-unit-workflowexecution`
- [ ] Lint passes: `golangci-lint run ./pkg/workflowexecution/...`
- [ ] Documentation updated (2 files)
- [ ] WE complexity reduced by 57% (-170 lines)

---

## 🎯 **Expected Impact**

### **Code Complexity**

| Metric | Before | After | Change |
|--------|--------|-------|--------|
| **reconcilePending LOC** | ~300 lines | ~130 lines | **-57%** ✅ |
| **Total WE LOC** | ~2,000 lines | ~1,830 lines | **-170 lines** ✅ |
| **Routing functions** | 4 functions | 0 functions | **-100%** ✅ |
| **Unit tests** | ~50 tests | ~35 tests | **-15 tests** ✅ |
| **Metrics** | 7 metrics | 3 metrics | **-4 metrics** ✅ |

### **Architectural Benefits**

| Benefit | Impact |
|---------|--------|
| **Single Source of Truth** | RR.Status for all routing decisions |
| **Clear Separation** | RO routes, WE executes |
| **Reduced Complexity** | WE is now pure executor |
| **Easier Debugging** | Single controller for routing logic |
| **Better Testability** | Routing tests in one place (RO) |

---

## 📚 **Reference Documents**

### **Must Read**

1. ✅ **DD-RO-002**: Centralized Routing Responsibility
2. ✅ **WE_TEAM_V1.0_ROUTING_HANDOFF.md**: RO team handoff
3. ✅ **V1.0_CENTRALIZED_ROUTING_IMPLEMENTATION_PLAN.md**: Full V1.0 plan

### **Optional Context**

1. **DD-WE-003**: Lock Persistence (explains HandleAlreadyExists)
2. **TRIAGE_V1.0_DAYS_2-5_COMPLETE_IMPLEMENTATION.md**: RO implementation status

---

## 🚨 **Key Principles**

### **Core Principle**

> **"If WFE exists, execute it. RO already checked routing."**

WE trusts RO's routing decisions completely. No second-guessing.

### **What to Keep**

- ✅ **Execution logic**: validateSpec, buildPipelineRun, monitoring
- ✅ **Failure handling**: MarkFailedWithReason, failure details tracking
- ✅ **Execution safety**: HandleAlreadyExists (DD-WE-003 Layer 2)
- ✅ **Status management**: phase transitions, completion tracking
- ✅ **Audit events**: workflow.started, workflow.completed, workflow.failed

### **What to Remove**

- ❌ **All routing logic**: CheckCooldown, CheckResourceLock
- ❌ **Skip logic**: MarkSkipped, skip details
- ❌ **Routing queries**: FindMostRecentTerminalWFE
- ❌ **Skip metrics**: WorkflowExecutionSkipTotal, BackoffSkipTotal
- ❌ **Stub types**: v1_compat_stubs.go

---

## 🔍 **Confidence Assessment**

**Handoff Document Quality**: 98% - Comprehensive and authoritative

**Implementation Clarity**: 95% - Clear instructions with code examples

**Risk Assessment**: Low
- RO team completed Days 2-5 (routing implementation)
- CRD changes already applied (Day 1)
- WE changes are simplifications (removing code, not adding)

**Estimated Completion**: 2 days (16 hours) as planned

---

## 📞 **Contact & Support**

**Questions**: Contact RO Team

**Issues**: Create ticket and tag @ro-team

**Clarifications**: Refer to DD-RO-002 and WE_TEAM_V1.0_ROUTING_HANDOFF.md

---

**Prepared By**: WE Team (Platform AI)
**Date**: December 15, 2025
**Status**: ✅ **READY TO IMPLEMENT**
**Next Step**: Begin Day 6 Task 1 (Remove Routing Functions)

