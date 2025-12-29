# WE Phase 3 Cleanup: Complete Routing Migration to RO

**Date**: December 19, 2025
**Phase**: DD-RO-002 Phase 3 (WE Simplification)
**Status**: 🚀 **IN PROGRESS**
**Priority**: P1 (Complete V1.0 architecture)

---

## Executive Summary

**Goal**: Complete migration of routing logic from WorkflowExecution (WE) to RemediationOrchestrator (RO) per DD-RO-002.

**Current State**: RO Phase 2 complete, WE still has vestigial routing code.

**Action**: Remove routing fields, logic, and tests from WE to become pure executor.

**Estimated Effort**: 2-3 hours
**Risk**: LOW (RO already owns routing, WE fields are unused)

---

## Phase 3 Checklist (Per DD-RO-002 Lines 351-360)

### API Schema Changes

- [ ] **Deprecate WFE routing fields** in `api/workflowexecution/v1alpha1/workflowexecution_types.go`
  - `ConsecutiveFailures int32`
  - `NextAllowedExecution *metav1.Time`
  - Add deprecation comments (remove in V2.0)

### Controller Logic Removal

- [ ] **Remove backoff calculation** (`workflowexecution_controller.go` lines 903-925)
- [ ] **Remove counter reset** (`workflowexecution_controller.go` lines 810-812)
- [ ] **Remove routing comment** (`workflowexecution_controller.go` line 928)
- [ ] **Remove metrics** (consecutive failures gauge already removed per line 951)

### Test Cleanup

- [ ] **Delete unit tests** for routing logic
  - `test/unit/workflowexecution/consecutive_failures_test.go` (entire file)

- [ ] **Remove integration tests** for routing logic
  - `test/integration/workflowexecution/reconciler_test.go` (BR-WE-012 section, lines 1084-1296)

- [ ] **Delete E2E tests** for routing logic
  - `test/e2e/workflowexecution/03_backoff_cooldown_test.go` (entire file)

### Documentation Updates

- [ ] **Update BR-WE-012** to reference RR fields only
- [ ] **Update CRD schema docs** with deprecation notices
- [ ] **Update DD-RO-002** Phase 3 status to COMPLETE

---

## Implementation Steps

### Step 1: Deprecate API Fields ⏸️

**File**: `api/workflowexecution/v1alpha1/workflowexecution_types.go`

**Changes**:
```go
// ConsecutiveFailures tracks consecutive failures for this target resource
// DEPRECATED (V1.1): Routing state moved to RR.Status per DD-RO-002 Phase 3
// Use RR.Status.ConsecutiveFailureCount instead
// This field will be removed in V2.0 and is no longer updated by WE controller
// +optional
ConsecutiveFailures int32 `json:"consecutiveFailures,omitempty"`

// NextAllowedExecution is the earliest timestamp when execution is allowed
// DEPRECATED (V1.1): Routing state moved to RR.Status per DD-RO-002 Phase 3
// Use RR.Status.NextAllowedExecution instead
// This field will be removed in V2.0 and is no longer updated by WE controller
// +optional
NextAllowedExecution *metav1.Time `json:"nextAllowedExecution,omitempty"`
```

**Rationale**: Mark fields as deprecated but don't remove yet (backward compatibility).

---

### Step 2: Remove Controller Logic ✅ IN PROGRESS

#### 2.1: Remove Backoff Calculation

**File**: `internal/controller/workflowexecution/workflowexecution_controller.go`

**Lines to Remove**: 903-925

**Current Code**:
```go
if wfe.Status.FailureDetails != nil && !wfe.Status.FailureDetails.WasExecutionFailure {
    // Pre-execution failure: increment counter and calculate backoff
    wfe.Status.ConsecutiveFailures++

    // Calculate exponential backoff using shared utility
    if r.BaseCooldownPeriod > 0 {
        backoffConfig := backoff.Config{
            BasePeriod:    r.BaseCooldownPeriod,
            MaxPeriod:     r.MaxCooldownPeriod,
            Multiplier:    2.0,
            JitterPercent: 10,
        }
        duration := backoffConfig.Calculate(wfe.Status.ConsecutiveFailures)

        nextAllowed := metav1.NewTime(time.Now().Add(duration))
        wfe.Status.NextAllowedExecution = &nextAllowed

        logger.Info("Calculated exponential backoff",
            "consecutiveFailures", wfe.Status.ConsecutiveFailures,
            "backoff", duration,
            "nextAllowedExecution", nextAllowed.Time,
        )
    }
} else {
    // Execution failure: DO NOT increment counter or set backoff
    // The PreviousExecutionFailed check in CheckCooldown will block ALL retries
    logger.Info("Execution failure detected - not incrementing ConsecutiveFailures",
        "wasExecutionFailure", wfe.Status.FailureDetails != nil && wfe.Status.FailureDetails.WasExecutionFailure,
    )
}
```

**Replace With**:
```go
// DD-RO-002 Phase 3: Routing logic moved to RO
// WE is now a pure executor - no routing decisions
// RO tracks ConsecutiveFailureCount and NextAllowedExecution in RR.Status
logger.V(1).Info("Workflow execution failed",
    "wasExecutionFailure", wfe.Status.FailureDetails != nil && wfe.Status.FailureDetails.WasExecutionFailure,
)
```

---

#### 2.2: Remove Counter Reset

**File**: `internal/controller/workflowexecution/workflowexecution_controller.go`

**Lines to Remove**: 810-812

**Current Code**:
```go
// Reset failure counter on success
wfe.Status.ConsecutiveFailures = 0
wfe.Status.NextAllowedExecution = nil
```

**Replace With**:
```go
// DD-RO-002 Phase 3: Routing state moved to RO
// RO resets RR.Status.ConsecutiveFailureCount on successful remediation
```

---

### Step 3: Delete Unit Tests ✅ IN PROGRESS

**File to Delete**: `test/unit/workflowexecution/consecutive_failures_test.go`

**Rationale**: Tests routing logic that now belongs to RO.

**Alternative**: Move tests to RO test suite (if needed, but RO already has unit tests).

---

### Step 4: Remove Integration Tests ✅ IN PROGRESS

**File**: `test/integration/workflowexecution/reconciler_test.go`

**Section to Remove**: Lines 1084-1296 (BR-WE-012 context block)

**Tests Being Removed**:
1. "should increment ConsecutiveFailures through multiple pre-execution failures"
2. "should cap NextAllowedExecution at MaxDelay (15 minutes)"
3. "should persist ConsecutiveFailures and NextAllowedExecution across reconciliations"
4. "should clear NextAllowedExecution on successful completion after failures"
5. "should reset ConsecutiveFailures counter to 0 on successful completion" (existing)
6. "should NOT increment ConsecutiveFailures for execution failures" (existing)

**Rationale**: These test routing state tracking, which is now RO's responsibility.

**Note**: Integration tests for WFE *execution* should remain (phase tracking, PipelineRun creation, etc.).

---

### Step 5: Delete E2E Tests ✅ IN PROGRESS

**File to Delete**: `test/e2e/workflowexecution/03_backoff_cooldown_test.go`

**Tests Being Removed**:
1. "should apply exponential backoff for consecutive pre-execution failures"
2. "should mark Skipped with ExhaustedRetries after MaxConsecutiveFailures"

**Rationale**: E2E tests for routing belong in RO E2E suite.

---

### Step 6: Update Documentation 📝

#### 6.1: Update BR-WE-012

**File**: `docs/services/crd-controllers/03-workflowexecution/BUSINESS_REQUIREMENTS.md`

**Change**:
```markdown
### Implementation

**State Tracking** (DEPRECATED - Moved to RO in V1.0):
- ~~WFE.Status.ConsecutiveFailures~~ → Use RR.Status.ConsecutiveFailureCount
- ~~WFE.Status.NextAllowedExecution~~ → Use RR.Status.NextAllowedExecution

**Current Implementation** (Per DD-RO-002 Phase 3):
- RO tracks consecutive failures in RR.Status
- RO calculates exponential backoff
- RO enforces routing decisions before creating WFE
- WE categorizes failure types only (WasExecutionFailure)
```

#### 6.2: Update CRD Schema Docs

**File**: `docs/services/crd-controllers/03-workflowexecution/crd-schema.md`

**Add Deprecation Notice** (lines 161-176):
```markdown
// ========================================
// EXPONENTIAL BACKOFF (DEPRECATED V1.1)
// Fields moved to RR.Status per DD-RO-002 Phase 3
// ========================================

// ConsecutiveFailures: DEPRECATED - Use RR.Status.ConsecutiveFailureCount
// NextAllowedExecution: DEPRECATED - Use RR.Status.NextAllowedExecution
//
// These fields are no longer updated by WE controller as of V1.0.
// RO handles all routing decisions including exponential backoff.
//
// Will be removed in V2.0.
```

#### 6.3: Update DD-RO-002

**File**: `docs/architecture/decisions/DD-RO-002-centralized-routing-responsibility.md`

**Update Phase 3 Status** (line 351):
```markdown
### Phase 3: WE Simplification (Days 6-7) - ✅ COMPLETE (Dec 19, 2025)

- [x] Deprecated WFE routing fields (ConsecutiveFailures, NextAllowedExecution)
- [x] Removed backoff calculation logic (~22 lines)
- [x] Removed counter reset logic (~3 lines)
- [x] Removed routing comment
- [x] Deleted consecutive_failures_test.go unit tests
- [x] Removed BR-WE-012 integration tests (~212 lines)
- [x] Deleted 03_backoff_cooldown_test.go E2E tests
- [x] Updated documentation (BR-WE-012, CRD schema, DD-RO-002)
```

---

## Verification Checklist

### After Implementation

- [ ] **Build succeeds**: `make build`
- [ ] **Unit tests pass**: `make test-unit`
- [ ] **Integration tests pass**: `make test-integration`
- [ ] **E2E tests pass**: `make test-e2e`
- [ ] **Linter passes**: `make lint`
- [ ] **No references to removed fields** in active code:
  ```bash
  grep -r "ConsecutiveFailures\|NextAllowedExecution" internal/controller/workflowexecution/ --include="*.go" | grep -v "DEPRECATED"
  ```

### Documentation Verification

- [ ] **BR-WE-012** references RR fields
- [ ] **CRD schema** has deprecation notices
- [ ] **DD-RO-002** Phase 3 marked COMPLETE
- [ ] **Handoff docs** explain migration

---

## Risk Assessment

### Risk 1: Breaking Existing WFE CRDs

**Scenario**: Existing WFEs have ConsecutiveFailures/NextAllowedExecution set.

**Mitigation**:
- Fields remain in API schema (deprecated, not removed)
- Existing WFEs can still be read
- New WFEs won't populate these fields

**Impact**: LOW - Backward compatible

---

### Risk 2: Test Coverage Gap

**Scenario**: Removing WE tests without corresponding RO tests.

**Mitigation**:
- RO already has 34 passing unit tests for routing
- RO has integration tests for routing checks
- E2E tests can be added to RO suite if needed

**Impact**: LOW - RO tests already exist

---

### Risk 3: Documentation Confusion

**Scenario**: Old docs reference WFE fields, new docs reference RR fields.

**Mitigation**:
- Update all docs to reference RR fields
- Add "DEPRECATED" notices to WFE field docs
- Cross-reference DD-RO-002 Phase 3

**Impact**: LOW - Documentation will be consistent

---

## Timeline

### Estimated Duration: 2-3 hours

| Task | Duration | Status |
|---|---|---|
| API field deprecation | 15 min | ⏸️ Pending |
| Controller logic removal | 30 min | 🚀 In Progress |
| Unit test deletion | 10 min | 🚀 In Progress |
| Integration test removal | 15 min | 🚀 In Progress |
| E2E test deletion | 10 min | 🚀 In Progress |
| Documentation updates | 30 min | ⏸️ Pending |
| Testing & verification | 30 min | ⏸️ Pending |
| **Total** | **2.5 hours** | |

---

## Success Criteria

✅ **Architecture**:
- WE contains zero routing logic
- WE fields marked as deprecated
- RO is sole owner of routing decisions

✅ **Code Quality**:
- All tests pass
- No linter errors
- Build succeeds

✅ **Documentation**:
- DD-RO-002 Phase 3 marked COMPLETE
- All docs reference correct fields (RR.Status)
- Migration rationale documented

---

## Post-Completion

### V1.1 (Future)

**Consider**: Remove deprecated WFE fields entirely in V2.0
- Breaking change (CRD schema update)
- Requires migration guide
- Should coincide with other V2.0 breaking changes

### Lessons Learned

Document in DD-RO-002:
- Phase 3 took X hours
- Main challenges: [list]
- Recommendations for future migrations

---

**Document Version**: 1.0
**Date**: December 19, 2025
**Status**: 🚀 **IN PROGRESS**
**Related**: DD-RO-002, BR-WE-012, FIELD_OWNERSHIP_TRIAGE_DEC_19_2025.md
**Owner**: WE Team

