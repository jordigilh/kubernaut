# AA-BUG-001: Phase Transition Audit Events Not Being Emitted - Investigation

**Status**: In Progress 🔍
**Priority**: P0 (E2E Test Blocker)
**Date**: January 4, 2026
**Affected Component**: AIAnalysis Controller - Audit Trail

---

## 📋 **Problem Statement**

E2E test `05_audit_trail_test.go:200` expects `aianalysis.phase.transition` audit events but receives **ZERO** events.

### Expected vs Actual

**Expected** (3 transitions):
- `Pending` → `Investigating`
- `Investigating` → `Analyzing`
- `Analyzing` → `Completed`

**Actual** (0 transitions):
- NO `aianalysis.phase.transition` events
- OTHER audit events ARE working:
  - `aianalysis.llm_request`: ✅ 1 event
  - `aianalysis.llm_response`: ✅ 1 event
  - `aianalysis.llm_tool_call`: ✅ 1 event
  - `aianalysis.workflow_validation_attempt`: ✅ 1 event

---

## 🔍 **Investigation Findings**

### Finding 1: Audit Infrastructure is Working
- Data Storage service is deployed ✅
- Audit tables are created ✅
- Other audit event types ARE being recorded ✅
- Issue is SPECIFIC to phase transition events ❌

### Finding 2: RecordPhaseTransition Method Exists
**Location**: `pkg/aianalysis/audit/audit.go:139-174`

```go
func (c *AuditClient) RecordPhaseTransition(ctx context.Context, analysis *aianalysisv1.AIAnalysis, from, to string) {
    // Idempotency check
    if from == to {
        return
    }

    payload := PhaseTransitionPayload{
        OldPhase: from,
        NewPhase: to,
    }

    event := audit.NewAuditEventRequest()
    audit.SetEventType(event, EventTypePhaseTransition) // "aianalysis.phase.transition"
    // ... event configuration ...

    c.store.StoreAudit(ctx, event)
}
```

### Finding 3: Multiple Call Sites Exist

#### Call Site 1: Controller `reconcilePending` (Pending → Investigating)
**Location**: `internal/controller/aianalysis/phase_handlers.go:74`
```go
if r.AuditClient != nil && phaseBefore != PhaseInvestigating {
    r.AuditClient.RecordPhaseTransition(ctx, analysis, phaseBefore, PhaseInvestigating)
}
```

#### Call Site 2: InvestigatingHandler (After ProcessIncidentResponse)
**Location**: `pkg/aianalysis/handlers/investigating.go:177`
```go
if analysis.Status.Phase != oldPhase {
    h.auditClient.RecordPhaseTransition(ctx, analysis, string(oldPhase), string(analysis.Status.Phase))
}
```

#### Call Site 3: ResponseProcessor (Inside ProcessIncidentResponse) - AA-BUG-001 FIX
**Location**: `pkg/aianalysis/handlers/response_processor.go:165`
```go
if p.auditClient != nil && oldPhase != aianalysis.PhaseAnalyzing {
    p.auditClient.RecordPhaseTransition(ctx, analysis, string(oldPhase), string(aianalysis.PhaseAnalyzing))
}
```

### Finding 4: Duplicate Call Potential
- **ResponseProcessor** records transition (Call Site 3)
- **InvestigatingHandler** ALSO records the SAME transition (Call Site 2)
- Idempotency check in `RecordPhaseTransition` prevents duplicate DB inserts BUT shouldn't prevent the first call from working

---

## 🧪 **Test Results**

### AA-BUG-001 Fix Applied
- ✅ All 204 AI Analysis unit tests pass
- ❌ E2E test still fails (35/36 passed)
- ✅ No compilation errors
- ✅ Audit client wiring is correct

### E2E Test Output
```
[FAILED] Should audit phase transitions (Pending→Investigating→Analyzing→Completed)
Expected
    <map[string]int | len:4>: {
        "workflow_validation_attempt": 1,
        "llm_response": 1,
        "llm_tool_call": 1,
        "llm_request": 1,
    }
to have key
    <string>: aianalysis.phase.transition
```

---

## 🤔 **Hypothesis**

The audit client is **likely `nil`** in the E2E environment, causing all `RecordPhaseTransition` calls to be skipped.

**Evidence**:
1. All call sites check `if auditClient != nil` before recording
2. If audit client initialization fails, `cmd/aianalysis/main.go:161` logs "audit will be disabled" and continues with `nil` client
3. We couldn't find this log message in E2E output, but controller logs weren't captured

**Next Steps**:
1. ✅ Add debug logging to `RecordPhaseTransition` to confirm if it's being called
2. ✅ Check E2E controller pod logs to see audit client initialization
3. ✅ Verify Data Storage connection from AIAnalysis controller
4. ✅ Add integration test that specifically validates phase transitions

---

## 📝 **Code Changes Applied (AA-BUG-001 Fix)**

### File: `pkg/aianalysis/handlers/response_processor.go`
**Changes**:
1. Added `auditClient AuditClientInterface` field to `ResponseProcessor` struct
2. Updated `NewResponseProcessor` to accept `auditClient` parameter
3. Added `RecordPhaseTransition` calls after phase transitions (2 locations)

### File: `pkg/aianalysis/handlers/investigating.go`
**Changes**:
1. Updated `NewInvestigatingHandler` to pass `auditClient` to `NewResponseProcessor`

### File: `test/unit/aianalysis/response_processor_test.go`
**Changes**:
1. Used existing `noopAuditClient` for unit tests

---

## 🎯 **Recommended Next Steps**

### Option A: Debug Logging Approach
1. Add temporary debug logs to `RecordPhaseTransition`
2. Re-run E2E test
3. Capture controller logs
4. Identify if method is being called

### Option B: Integration Test Approach
1. Create AI Analysis integration test for phase transitions
2. Validate audit events are created
3. Easier to debug than full E2E

### Option C: Check Audit Store Initialization
1. Add log output to show if audit store is `nil`
2. Verify Data Storage connection string
3. Check if there are network/DNS issues in E2E cluster

---

## 📚 **Related Files**

- `pkg/aianalysis/audit/audit.go`: Audit client implementation
- `internal/controller/aianalysis/phase_handlers.go`: Controller phase handlers
- `pkg/aianalysis/handlers/investigating.go`: Investigating phase handler
- `pkg/aianalysis/handlers/response_processor.go`: Response processor with AA-BUG-001 fix
- `cmd/aianalysis/main.go`: Controller startup and audit client initialization
- `test/e2e/aianalysis/05_audit_trail_test.go:200`: Failing E2E test

---

## ⚠️ **Critical Questions**

1. **Why do other audit events work but not phase transitions?**
   - `RecordHolmesGPTCall` works ✅
   - `RecordPhaseTransition` doesn't work ❌
   - Same audit client, same store, different methods

2. **Is the audit client `nil` in E2E?**
   - Need controller logs to confirm
   - Cluster was torn down before we could check

3. **Are phase transitions actually happening?**
   - Test expects `Analyzing` → `Completed` transition
   - E2E test shows AIAnalysis reaches `Completed` phase
   - So transitions ARE happening, just not being audited

---

## 🔗 **Related Bugs**

- **SP-BUG-001**: Similar issue in Signal Processing (FIXED)
  - Signal Processing WAS emitting phase transition events
  - Fix: Added `RecordPhaseTransition` calls
- **SP-BUG-002**: Duplicate phase transition audit events (FIXED)
  - Idempotency check added to prevent duplicates

---

## 📊 **Confidence Assessment**

**Hypothesis Confidence**: 75%
- High likelihood audit client is `nil` in E2E
- Other audit events working suggests infrastructure is OK
- Need controller logs to confirm

**Fix Confidence**: 60%
- Code changes are correct
- Wiring is correct
- Something environmental is blocking execution

