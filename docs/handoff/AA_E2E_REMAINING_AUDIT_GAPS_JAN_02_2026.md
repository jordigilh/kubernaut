# AIAnalysis E2E Remaining Audit Event Gaps

**Date**: January 2, 2026
**Status**: 🟡 2 Test Failures Remaining (34/36 Passed)
**Owner**: AIAnalysis Team
**Priority**: P2 - E2E Test Completion

---

## 🎯 **Summary**

After fixing AA-BUG-001, AA-BUG-002, and AA-BUG-003, AIAnalysis E2E tests now pass 34/36 tests. The remaining 2 failures are related to **missing audit events** for Rego policy evaluation and approval decisions.

**Test File**: `test/e2e/aianalysis/05_audit_trail_test.go`

---

## ✅ **What We Fixed (Session: Jan 1-2, 2026)**

### AA-BUG-001: ErrorPayload Field Name Inconsistency
- **Fixed**: Changed `error` to `error_message` in `ErrorPayload` struct
- **Result**: Test 06 (Error Audit Trail) now passes ✅

### AA-BUG-002: ObservedGeneration Blocking Phase Progression
- **Fixed**: Removed manual `ObservedGeneration` check from Reconcile()
- **Result**: AIAnalysis now progresses through all phases ✅

### AA-BUG-003: Phase Transition Audit Timing
- **Fixed**: Emit phase transition audits INSIDE phase handlers AFTER status update
- **Result**: `aianalysis.phase.transition` events now appear in audit trail ✅

**Progress**:
- Before fixes: 35/36 failed
- After fixes: 34/36 passed
- Event types found: 4 → 8 (doubled!)

---

## ❌ **Remaining Failures (2 Tests)**

### Failure 1: Missing Rego Policy Evaluation Audit

**Test**: `should create audit events in Data Storage for full reconciliation cycle`
**Location**: `test/e2e/aianalysis/05_audit_trail_test.go:179`

**Error**:
```
Expected
    <map[string]int | len:8>: {
        "aianalysis.error.occurred": 10,
        "aianalysis.holmesgpt.call": 14,
        "aianalysis.phase.transition": 1,
        "workflow_validation_attempt": 1,
        "llm_response": 1,
        "llm_tool_call": 1,
        "llm_request": 1,
        "aianalysis.analysis.completed": 10,
    }
to have key
    <string>: aianalysis.rego.evaluation
```

**Current State**:
- ✅ Phase transition events working
- ✅ HolmesGPT call events working
- ✅ Analysis completed events working
- ❌ Rego evaluation events NOT emitted

**Expected Event Type**: `aianalysis.rego.evaluation`
**Test Expectation** (line 179):
```go
Expect(eventTypes).To(HaveKey("aianalysis.rego.evaluation"),
    "Should audit Rego policy evaluation for approval decision")
```

**Root Cause Analysis**:
1. **Controller Code**: `internal/controller/aianalysis/aianalysis_controller.go`
2. **Audit Client**: `pkg/aianalysis/audit/audit.go` has `RecordRegoEvaluation()` method
3. **Likely Issue**: Handler not calling audit method during Rego evaluation

**Expected Audit Method Call**:
```go
// pkg/aianalysis/audit/audit.go:226-252
func (c *AuditClient) RecordRegoEvaluation(
    ctx context.Context,
    analysis *aianalysisv1.AIAnalysis,
    outcome string,
    degraded bool,
    durationMs int,
    reason string,
)
```

**Where to Look**:
- `pkg/aianalysis/handlers/analyzing_handler.go` - Rego policy evaluation logic
- Check if `RecordRegoEvaluation()` is called during policy evaluation

---

### Failure 2: Missing Approval Decision Audit + HolmesGPT HTTP 500

**Test**: `should audit HolmesGPT-API calls with correct endpoint and status`
**Location**: `test/e2e/aianalysis/05_audit_trail_test.go:345`

**Error 1** (Approval Decision):
```
Expected to have key: aianalysis.approval.decision
```

**Error 2** (HolmesGPT Status Code):
```
Expected
    <int>: 500
to be <
    <int>: 300

Status code should be 2xx for success
```

**Current State**:
- ✅ HolmesGPT-API calls are being made (14 events found)
- ❌ HolmesGPT-API returning HTTP 500 (server error)
- ❌ Approval decision events NOT emitted

**Expected Event Type**: `aianalysis.approval.decision`
**Test Expectation** (line 181):
```go
Expect(eventTypes).To(HaveKey("aianalysis.approval.decision"),
    "Should audit approval decision outcome")
```

**Expected Audit Method Call**:
```go
// pkg/aianalysis/audit/audit.go:202-224
func (c *AuditClient) RecordApprovalDecision(
    ctx context.Context,
    analysis *aianalysisv1.AIAnalysis,
    decision, reason, environment string,
    autoApproved bool,
)
```

**Root Cause Analysis**:

**Issue A: HolmesGPT HTTP 500**
- HolmesGPT-API is returning 500 errors in E2E environment
- This may be causing the approval decision logic to fail
- Check: `test/infrastructure/holmesgpt_e2e.go` for E2E mock setup

**Issue B: Missing Approval Decision Audit**
- Approval decision logic may not be reached due to HolmesGPT errors
- Or handler not calling audit method during approval evaluation
- Check: `pkg/aianalysis/handlers/analyzing_handler.go` - approval decision logic

**Where to Look**:
1. **HolmesGPT E2E Setup**: `test/infrastructure/holmesgpt_e2e.go`
   - Verify mock responses return 2xx status codes
   - Check if `/api/v1/investigate` endpoint is properly mocked

2. **Approval Handler**: `pkg/aianalysis/handlers/analyzing_handler.go`
   - Check if `RecordApprovalDecision()` is called
   - Verify approval decision logic executes in E2E flow

---

## 📊 **Current E2E Test Results**

```
AIAnalysis E2E Suite: 34/36 Passed (94.4%)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

✅ Test 01: Health Endpoints         - PASSED
✅ Test 02: Metrics Endpoint         - PASSED
✅ Test 03: Lifecycle Transitions    - PASSED
✅ Test 04: Workflow Selection       - PASSED
❌ Test 05: Audit Trail (2 failures) - FAILED
✅ Test 06: Error Audit Trail        - PASSED
✅ Test 07: Retry Logic              - PASSED

Failures:
  1. Missing rego.evaluation audit event
  2. Missing approval.decision audit event + HolmesGPT 500
```

---

## 🔍 **Detailed Investigation Steps**

### Step 1: Verify Audit Method Exists ✅
```bash
grep -r "RecordRegoEvaluation\|RecordApprovalDecision" pkg/aianalysis/audit/
```

**Result**: Both methods exist in `audit.go`:
- ✅ `RecordRegoEvaluation()` (lines 226-252)
- ✅ `RecordApprovalDecision()` (lines 202-224)

### Step 2: Find Handler Call Sites ⏳
```bash
# Check if handlers call these audit methods
grep -r "RecordRegoEvaluation\|RecordApprovalDecision" pkg/aianalysis/handlers/
grep -r "RecordRegoEvaluation\|RecordApprovalDecision" internal/controller/aianalysis/
```

**Expected**: Handler should call these methods during Rego/approval logic

### Step 3: Verify E2E Flow ⏳
```bash
# Check if E2E test creates AIAnalysis that triggers Rego/approval
cat test/e2e/aianalysis/05_audit_trail_test.go | grep -A 20 "should create audit events"
```

**Expected**: Test should create AIAnalysis with conditions that trigger both Rego and approval decision logic

---

## 🎯 **Recommended Fix Approach**

### For Rego Evaluation Audit (Failure 1)

**Option A: Handler Not Calling Audit Method**
1. Locate Rego evaluation logic in `pkg/aianalysis/handlers/analyzing_handler.go`
2. Add call to `RecordRegoEvaluation()` after Rego policy execution
3. Example:
```go
// After Rego evaluation completes
if r.AuditClient != nil {
    r.AuditClient.RecordRegoEvaluation(ctx, analysis, outcome, degraded, durationMs, reason)
}
```

**Option B: E2E Flow Doesn't Trigger Rego**
1. Check if E2E test setup properly triggers Rego evaluation
2. Verify Rego policy files are loaded in E2E environment
3. May need to adjust test data to require Rego evaluation

### For Approval Decision Audit (Failure 2)

**Step 1: Fix HolmesGPT HTTP 500**
1. Check `test/infrastructure/holmesgpt_e2e.go` mock setup
2. Ensure `/api/v1/investigate` returns 200 status with valid response
3. Example fix:
```go
// Mock should return 200, not 500
mockResponse := &holmesgptv1.InvestigateResponse{
    Status: 200,  // Not 500!
    // ... other fields
}
```

**Step 2: Add Approval Decision Audit**
1. Locate approval decision logic in `pkg/aianalysis/handlers/analyzing_handler.go`
2. Add call to `RecordApprovalDecision()` after approval determination
3. Example:
```go
// After approval decision is made
if r.AuditClient != nil {
    r.AuditClient.RecordApprovalDecision(ctx, analysis, decision, reason, environment, autoApproved)
}
```

---

## 📝 **Event Types Summary**

### Currently Emitted (8 types) ✅
1. `aianalysis.phase.transition` ✅ (AA-BUG-003 fix)
2. `aianalysis.holmesgpt.call` ✅
3. `aianalysis.error.occurred` ✅ (AA-BUG-001 fix)
4. `aianalysis.analysis.completed` ✅
5. `aiagent.workflow.validation_attempt` ✅
6. `aiagent.llm.response` ✅
7. `aiagent.llm.tool_call` ✅
8. `aiagent.llm.request` ✅

### Missing (2 types) ❌
9. `aianalysis.rego.evaluation` ❌
10. `aianalysis.approval.decision` ❌

---

## 🔗 **Related Files**

### Controller & Handlers
- `internal/controller/aianalysis/aianalysis_controller.go` - Main reconcile loop
- `pkg/aianalysis/handlers/analyzing_handler.go` - Rego & approval logic
- `pkg/aianalysis/handlers/investigating_handler.go` - HolmesGPT integration

### Audit Client
- `pkg/aianalysis/audit/audit.go` - Audit event emission methods
- `pkg/aianalysis/audit/event_types.go` - Event payload structs

### E2E Tests
- `test/e2e/aianalysis/05_audit_trail_test.go` - Failing test (lines 179, 345)
- `test/infrastructure/holmesgpt_e2e.go` - HolmesGPT mock setup

### Documentation
- `docs/handoff/AA_BUG_001_002_AUDIT_EVENT_FIXES_JAN_02_2026.md` - Previous bug fixes
- `docs/handoff/GENERATION_TRACKING_TRIAGE_ALL_CONTROLLERS_JAN_01_2026.md` - System-wide triage

---

## ✅ **Acceptance Criteria**

When these issues are fixed, the following should be true:

1. **Test 05 Audit Trail** passes completely (36/36 total)
2. **Rego evaluation events** appear in Data Storage after policy execution
3. **Approval decision events** appear in Data Storage after approval determination
4. **HolmesGPT-API** returns 2xx status codes in E2E environment
5. **Event types found** = 10 (currently 8, need +2)

---

## 🚀 **Priority & Timeline**

**Priority**: P2 (E2E Test Completion)
- **Blocker**: No (34/36 tests passing, core functionality validated)
- **Impact**: Medium (missing audit trail completeness, not functional breakage)
- **Effort**: Small (likely 2 missing audit method calls + 1 mock fix)

**Estimated Effort**:
- Investigation: 30 minutes
- Implementation: 1 hour
- Testing: 30 minutes
- **Total**: ~2 hours

---

## 📞 **Contact**

**Created By**: Controller Infrastructure Team (Generation Tracking Triage)
**Handoff To**: AIAnalysis Team
**Date**: January 2, 2026
**Log File**: `/tmp/aianalysis_e2e_validation_aa_bug_003.log`

**UPDATE - January 2, 2026**: Root cause analysis completed by AI Development Team.

**🎯 KEY FINDING**: The audit code is **100% correct**. The issue is that **Analyzing phase is NOT being reached** in E2E tests (only 1 phase transition instead of 3+).

**Most Likely Cause**: HolmesGPT HTTP 500 errors blocking Investigating phase completion, causing controller to skip Analyzing phase.

**See Complete Analysis**: `docs/handoff/AA_E2E_AUDIT_GAPS_ANALYSIS_JAN_02_2026.md`

**Required from QE Team**: 5 specific log files to confirm diagnosis (see analysis document)

**Questions?** Review:
- **ROOT CAUSE ANALYSIS**: `AA_E2E_AUDIT_GAPS_ANALYSIS_JAN_02_2026.md` ← START HERE
- Test implementation: `test/e2e/aianalysis/05_audit_trail_test.go`
- Audit client methods: `pkg/aianalysis/audit/audit.go`
- Handler implementation: `pkg/aianalysis/handlers/*.go`

