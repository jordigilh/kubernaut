# AIAnalysis E2E Root Cause: Missing Audit Event Types

**Date**: December 20, 2025
**Severity**: 🚨 **CRITICAL V1.0 BLOCKER**
**Status**: ✅ **ROOT CAUSE IDENTIFIED**

---

## 🎯 **Root Cause Summary**

**Issue**: AIAnalysis controller is creating ONLY `aianalysis.analysis.completed` events, but E2E tests expect **5 different audit event types** per ADR-032.

**Evidence from PostgreSQL**:
```sql
SELECT COUNT(*) AS total_events, event_category, event_type
FROM audit_events
GROUP BY event_category, event_type;

 total_events | event_category |          event_type
--------------+----------------+-------------------------------
           23 | analysis       | aianalysis.analysis.completed
(1 row)
```

**Translation**: 23 audit events exist, but **ALL** are the same type!

---

## ❌ **Missing Audit Event Types**

### **Expected Event Types (per ADR-032)**

| Event Type | Status | E2E Test File | Line |
|-----------|--------|---------------|------|
| `aianalysis.analysis.completed` | ✅ Created | `05_audit_trail_test.go` | 109 |
| `aianalysis.phase.transition` | ❌ Missing | `05_audit_trail_test.go` | 216 |
| `aianalysis.holmesgpt.call` | ❌ Missing | `05_audit_trail_test.go` | 288 |
| `aianalysis.rego.evaluated` | ❌ Missing | `05_audit_trail_test.go` | 368 |
| `aianalysis.approval.decision` | ❌ Missing | `05_audit_trail_test.go` | 453 |

### **Why E2E Tests Failed**

**All 5 failures** queried Data Storage with filters that don't match any events:

1. **Test 1** (line 109): Query for `event_type = aianalysis.analysis.completed` ✅ **Should pass!** (but queries for specific `resource_id`, which might not match)

2. **Test 2** (line 216): Query for `event_type = aianalysis.phase.transition` ❌ **Zero results** (type doesn't exist)

3. **Test 3** (line 288): Query for `event_type = aianalysis.holmesgpt.call` ❌ **Zero results** (type doesn't exist)

4. **Test 4** (line 368): Query for `event_type = aianalysis.rego.evaluated` ❌ **Zero results** (type doesn't exist)

5. **Test 5** (line 453): Query for `event_type = aianalysis.approval.decision` ❌ **Zero results** (type doesn't exist)

---

## 🔍 **Detailed Analysis**

### **Integration Tests vs E2E Tests**

**Integration Tests** (20/20 passing):
- Mock the Data Storage API
- Don't validate actual event persistence or retrieval
- Test audit client methods in isolation

**E2E Tests** (0/5 passing):
- Use real Data Storage API
- Validate actual event persistence and retrieval
- **Exposed the gap**: Missing event types

### **Current Implementation**

**File**: `pkg/aianalysis/audit/audit.go`

**Methods**:
- ✅ `RecordAnalysisComplete()` - Creates `aianalysis.analysis.completed` events
- ❌ `RecordPhaseTransition()` - **NOT CALLED** in controller
- ❌ `RecordHolmesGPTCall()` - **NOT CALLED** in controller
- ❌ `RecordRegoEvaluation()` - **NOT CALLED** in controller
- ❌ `RecordApprovalDecision()` - **NOT IMPLEMENTED** at all!

---

## 🚨 **V1.0 Impact**

### **Why This is a Critical Blocker**

1. **ADR-032 Violation**: Requires comprehensive audit trail for ALL significant operations
2. **Compliance Gap**: Missing audit events for phase transitions, API calls, policy evaluations
3. **Observability Gap**: Cannot trace AIAnalysis lifecycle end-to-end
4. **Test Gap**: Integration tests passed because they mock Data Storage (didn't catch this)

### **Why Integration Tests Didn't Catch This**

**Integration Test Pattern**:
```go
// test/integration/aianalysis/audit_integration_test.go
It("should record phase transitions", func() {
    // Uses REAL Data Storage, but...
    // Controller doesn't call RecordPhaseTransition() in production code!
    // Test manually calls RecordPhaseTransition() to validate the method works
    // But this doesn't validate the controller ACTUALLY calls it
})
```

**Gap**: Integration tests validate that audit methods **CAN** write events, but don't validate that the controller **DOES** call them during reconciliation.

---

## 📋 **Required Fixes**

### **Fix 1: Add Phase Transition Audit Events**

**Location**: `internal/controller/aianalysis/aianalysis_controller.go`

**Change**: Call `auditClient.RecordPhaseTransition()` when phase changes

```go
// In reconciliation logic after phase transition
if analysis.Status.Phase != oldPhase {
    auditClient.RecordPhaseTransition(ctx, analysis, oldPhase, analysis.Status.Phase)
}
```

**Estimated Effort**: 15 minutes

---

### **Fix 2: Add HolmesGPT-API Call Audit Events**

**Location**: `pkg/aianalysis/handlers/investigating.go`

**Change**: Call `auditClient.RecordHolmesGPTCall()` after API calls

```go
// After HolmesGPT-API call
resp, err := h.holmesClient.Investigate(ctx, request)
if err != nil {
    auditClient.RecordHolmesGPTCall(ctx, analysis, "/api/v1/investigate", "error", err.Error())
    return err
}
auditClient.RecordHolmesGPTCall(ctx, analysis, "/api/v1/investigate", "success", "")
```

**Estimated Effort**: 20 minutes

---

### **Fix 3: Add Rego Evaluation Audit Events**

**Location**: `pkg/aianalysis/handlers/analyzing.go`

**Change**: Call `auditClient.RecordRegoEvaluation()` after policy evaluation

```go
// After Rego evaluation
result, err := h.regoEvaluator.Evaluate(ctx, input)
if err != nil {
    auditClient.RecordRegoEvaluation(ctx, analysis, input, "error", err.Error())
    return err
}
auditClient.RecordRegoEvaluation(ctx, analysis, input, result.ApprovalRequired, "")
```

**Estimated Effort**: 20 minutes

---

### **Fix 4: Implement Approval Decision Audit Event**

**Location**: `pkg/aianalysis/audit/audit.go`

**Change**: Add new method `RecordApprovalDecision()`

```go
const EventTypeApprovalDecision = "aianalysis.approval.decision"

type ApprovalDecisionPayload struct {
    ApprovalRequired bool   `json:"approval_required"`
    ApprovalReason   string `json:"approval_reason"`
    Confidence       float64 `json:"confidence"`
    Environment      string `json:"environment"`
}

func (c *AuditClient) RecordApprovalDecision(ctx context.Context, analysis *aianalysisv1.AIAnalysis) {
    payload := ApprovalDecisionPayload{
        ApprovalRequired: analysis.Status.ApprovalRequired,
        ApprovalReason:   analysis.Status.ApprovalReason,
        Confidence:       analysis.Status.SelectedWorkflow.Confidence,
        Environment:      analysis.Spec.AnalysisRequest.SignalContext.Environment,
    }

    event := audit.NewAuditEventRequest()
    event.Version = "1.0"
    audit.SetEventType(event, EventTypeApprovalDecision)
    audit.SetEventCategory(event, "analysis")
    audit.SetEventAction(event, "approval_decided")
    audit.SetEventOutcome(event, audit.OutcomeSuccess)
    audit.SetActor(event, "service", "aianalysis-controller")
    audit.SetResource(event, "AIAnalysis", analysis.Name)
    audit.SetCorrelationID(event, analysis.Spec.RemediationID)
    audit.SetNamespace(event, analysis.Namespace)
    audit.SetEventData(event, payload)

    if err := c.store.StoreAudit(ctx, event); err != nil {
        c.logger.Error(err, "Failed to record approval decision audit event")
    }
}
```

**Estimated Effort**: 30 minutes

---

### **Fix 5: Call RecordApprovalDecision() in Controller**

**Location**: `pkg/aianalysis/handlers/analyzing.go`

**Change**: Call `auditClient.RecordApprovalDecision()` after setting `approvalRequired`

```go
// After setting analysis.Status.ApprovalRequired
if analysis.Status.ApprovalRequired {
    auditClient.RecordApprovalDecision(ctx, analysis)
}
```

**Estimated Effort**: 5 minutes

---

## ⏱️ **Total Estimated Effort**

**Implementation**: 90 minutes (1.5 hours)
**Testing**: 30 minutes
**Validation**: 15 minutes

**Total**: **2-2.5 hours**

---

## 🎯 **Validation Plan**

### **Step 1: Implement All Fixes**

Apply all 5 fixes listed above

### **Step 2: Run Integration Tests**

```bash
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make test-integration-aianalysis
```

**Expected**: 53/53 passing (no change, should still pass)

### **Step 3: Run E2E Tests**

```bash
make test-e2e-aianalysis
```

**Expected**: 30/30 passing (all audit trail tests should now pass)

### **Step 4: Verify PostgreSQL Event Types**

```bash
export KUBECONFIG=/Users/jgil/.kube/aianalysis-e2e-config
kubectl exec -n kubernaut-system deployment/postgresql -- \
  psql -U slm_user -d action_history -c \
  "SELECT COUNT(*) AS total, event_type FROM audit_events GROUP BY event_type;"
```

**Expected**: 5 different event types with counts > 0

---

## 📊 **V1.0 Decision Matrix**

### **Option A: Fix Before V1.0 Release** (RECOMMENDED)

**Pros**:
- ✅ Complete audit trail per ADR-032
- ✅ Full E2E validation passing
- ✅ Production-ready observability

**Cons**:
- ⏱️ 2-2.5 hours implementation time

**Recommendation**: **PROCEED** - Audit trail is mandatory, effort is minimal

---

### **Option B: Ship V1.0 With Partial Audit Trail** (NOT RECOMMENDED)

**Pros**:
- ✅ No implementation delay

**Cons**:
- ❌ ADR-032 violation
- ❌ Incomplete observability
- ❌ Compliance gap
- ❌ E2E test failures remain

**Recommendation**: **REJECT** - Violates mandatory audit requirements

---

## 🚦 **Status Update**

| Item | Before | After Fix |
|------|--------|-----------|
| **E2E Tests** | 25/30 passing | 30/30 passing (expected) |
| **Audit Event Types** | 1/5 types created | 5/5 types created |
| **ADR-032 Compliance** | ❌ Partial | ✅ Complete |
| **V1.0 Readiness** | ⚠️ 95% (blocker) | ✅ 100% (ready) |

---

**Prepared By**: AI Assistant (Cursor)
**Root Cause Date**: December 20, 2025
**Estimated Fix Time**: 2-2.5 hours
**Recommended Action**: ✅ **IMPLEMENT FIXES BEFORE V1.0 RELEASE**


