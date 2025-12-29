# AIAnalysis E2E Audit Trail Investigation - Dec 21, 2025

## 🎯 **Executive Summary**

AIAnalysis E2E tests are failing with 3 audit trail test failures. The root cause is **NO audit events are being written to Data Storage**, not schema mismatches. All other E2E tests (27/30) pass successfully.

---

## 📊 **Current Test Status**

### **E2E Test Results**
- **Total Specs**: 30 (ran), 4 (skipped)
- **Passed**: 27/30 (90%)
- **Failed**: 3/30 (10%)
- **Execution Time**: ~16 minutes

### **Failing Tests** (All Audit Trail related)
1. `should audit phase transitions with correct old/new phase values`
   - **Error**: `Expected <[]map[string]interface {} | len:0, cap:0>: [] not to be empty`
   - **Location**: `test/e2e/aianalysis/05_audit_trail_test.go:221`

2. `should audit HolmesGPT-API calls with correct endpoint and status`
   - **Error**: `Expected <[]map[string]interface {} | len:0, cap:0>: [] not to be empty`
   - **Location**: `test/e2e/aianalysis/05_audit_trail_test.go:293`

3. `should audit Rego policy evaluations with correct outcome`
   - **Error**: `Expected <[]map[string]interface {} | len:0, cap:0>: [] not to be empty`
   - **Location**: `test/e2e/aianalysis/05_audit_trail_test.go:388`

---

## 🔍 **Root Cause Analysis**

### **Symptom**
All three failing tests query Data Storage's audit API and receive **empty result arrays**. No audit events are being recorded.

### **What This Is NOT**
- ❌ **NOT** a schema mismatch (we fixed those)
- ❌ **NOT** a field naming issue (fixed `status_code` → `http_status_code`, etc.)
- ❌ **NOT** a test infrastructure problem (Data Storage is running and queryable)

### **What This IS**
✅ Audit events are **not being written** to Data Storage during reconciliation

---

## 🛠️ **Schema Fixes Completed (P2 Refactoring)**

### **Audit Event Type Schema Updates**

#### **1. `HolmesGPTCallPayload`** (`pkg/aianalysis/audit/event_types.go:64-68`)
**Changed**:
```go
// BEFORE
StatusCode int `json:"status_code"`

// AFTER
HTTPStatusCode int `json:"http_status_code"`
```

#### **2. `PhaseTransitionPayload`** (`pkg/aianalysis/audit/event_types.go:49-57`)
**Changed**:
```go
// BEFORE
FromPhase string `json:"from_phase"`
ToPhase   string `json:"to_phase"`

// AFTER
OldPhase string `json:"old_phase"`
NewPhase string `json:"new_phase"`
```

#### **3. `RegoEvaluationPayload`** (`pkg/aianalysis/audit/event_types.go:85-94`)
**Added**:
```go
// BEFORE (missing field)
type RegoEvaluationPayload struct {
	Outcome    string `json:"outcome"`
	Degraded   bool   `json:"degraded"`
	DurationMs int    `json:"duration_ms"`
}

// AFTER
type RegoEvaluationPayload struct {
	Outcome    string `json:"outcome"`
	Degraded   bool   `json:"degraded"`
	DurationMs int    `json:"duration_ms"`
	Reason     string `json:"reason"` // ← ADDED
}
```

#### **4. `ApprovalDecisionPayload`** (`pkg/aianalysis/audit/event_types.go:70-83`)
**Added**:
```go
// BEFORE (missing fields)
type ApprovalDecisionPayload struct {
	Decision    string `json:"decision"`
	Reason      string `json:"reason"`
	Environment string `json:"environment"`
	// ...
}

// AFTER
type ApprovalDecisionPayload struct {
	ApprovalRequired bool   `json:"approval_required"` // ← ADDED
	ApprovalReason   string `json:"approval_reason"`   // ← ADDED
	AutoApproved     bool   `json:"auto_approved"`     // ← ADDED
	Decision         string `json:"decision"`
	Reason           string `json:"reason"`
	Environment      string `json:"environment"`
	// ...
}
```

### **Function Signature Updates**

#### **`RecordRegoEvaluation` Signature Change**
**File**: `pkg/aianalysis/audit/audit.go:252` + `pkg/aianalysis/handlers/interfaces.go:69`

**Changed**:
```go
// BEFORE
func (c *AuditClient) RecordRegoEvaluation(
	ctx context.Context,
	analysis *aianalysisv1.AIAnalysis,
	outcome string,
	degraded bool,
	durationMs int,
)

// AFTER
func (c *AuditClient) RecordRegoEvaluation(
	ctx context.Context,
	analysis *aianalysisv1.AIAnalysis,
	outcome string,
	degraded bool,
	durationMs int,
	reason string, // ← ADDED
)
```

#### **Caller Updates**
**File**: `pkg/aianalysis/handlers/analyzing.go:108,132`

```go
// Success case
h.auditClient.RecordRegoEvaluation(ctx, analysis, outcome, result.Degraded, int(regoDuration), result.Reason)

// Error case
h.auditClient.RecordRegoEvaluation(ctx, analysis, "error", true, int(regoDuration), "Rego evaluation failed unexpectedly")
```

---

## 🚨 **Investigation Required: Why No Events Are Written**

### **Hypotheses**

#### **Hypothesis 1: Audit Client Not Initialized**
**Check**:
```bash
# In E2E cluster logs
kubectl logs -n kubernaut-system deployment/aianalysis-controller | grep -i "audit"
```

**Expected**: Should see audit client initialization or audit write errors

---

#### **Hypothesis 2: BufferedAuditStore Not Flushing**
**Possible causes**:
- Buffer not full (events queued but not flushed)
- Context cancelled before flush
- Background goroutine not running

**Check**: Look for `BufferedAuditStore` flush logs or errors

---

#### **Hypothesis 3: OpenAPI Client Write Failures**
**Possible causes**:
- Data Storage service not reachable from controller pod
- Authentication/authorization issues
- Network policies blocking traffic

**Check**:
```bash
# Test Data Storage connectivity from controller pod
kubectl exec -n kubernaut-system deployment/aianalysis-controller -- curl http://datastorage:8091/health
```

---

#### **Hypothesis 4: Event Correlation ID Mismatch**
**Possible issue**: Tests query by `correlation_id`, but events use different correlation ID than expected

**Check**: Query Data Storage directly without correlation ID filter:
```bash
curl http://localhost:8091/api/v1/audit/events?event_type=aianalysis.phase.transition&limit=10
```

---

#### **Hypothesis 5: Controller Not Reconciling**
**Possible issue**: Controller isn't actually processing the AIAnalysis CRs

**Check**:
```bash
# Check if controller is reconciling
kubectl get aianalyses -A
kubectl describe aianalysis -n kubernaut-system <name>
```

---

## 📋 **Next Steps**

### **Immediate Actions (Priority Order)**

1. **Verify Controller is Reconciling** (2 min)
   ```bash
   export KUBECONFIG=/Users/jgil/.kube/aianalysis-e2e-config
   kubectl get aianalyses -A -o wide
   kubectl logs -n kubernaut-system deployment/aianalysis-controller --tail=100
   ```

2. **Check Audit Client Initialization** (3 min)
   - Search logs for "audit" keyword
   - Verify `BufferedAuditStore` is created
   - Check for audit write errors

3. **Test Data Storage Connectivity** (5 min)
   - Exec into controller pod
   - Curl Data Storage health endpoint
   - Curl audit events API

4. **Query Data Storage Directly** (5 min)
   - Query without correlation ID filter
   - Check if ANY aianalysis events exist
   - Verify event schema matches expectations

5. **Enable Debug Logging** (10 min)
   - Add verbose logging to `BufferedAuditStore`
   - Log every `StoreAudit()` call
   - Rebuild and redeploy

---

## 🎯 **Success Criteria**

### **Definition of Done**
- ✅ All 30/30 E2E specs pass (excluding 4 skipped)
- ✅ Phase transition events recorded in Data Storage
- ✅ HolmesGPT-API call events recorded in Data Storage
- ✅ Rego evaluation events recorded in Data Storage
- ✅ Approval decision events recorded in Data Storage

---

## 📚 **Related Documentation**

- **Audit Specification**: `docs/architecture/decisions/DD-AUDIT-004-structured-audit-payloads.md`
- **OpenAPI Client Migration**: `docs/handoff/NOTICE_DD_API_001_OPENAPI_CLIENT_MANDATORY_DEC_18_2025.md`
- **Testing Guidelines**: `docs/development/business-requirements/TESTING_GUIDELINES.md`
- **Service Maturity**: `docs/development/testing/V1_0_SERVICE_MATURITY_TEST_PLAN_TEMPLATE.md`

---

## 🔄 **Status**

- **Current State**: E2E tests failing (3/30 audit trail tests)
- **Blocker**: No audit events being written to Data Storage
- **Priority**: **P0** (V1.0 Release Blocker)
- **Owner**: AA Team
- **Created**: Dec 21, 2025
- **Last Updated**: Dec 21, 2025 10:30 AM EST

---

## 📝 **Notes**

- Schema fixes are complete and correct
- All unit tests pass (190/193 = 98.4%)
- All integration tests pass (53/53 = 100%)
- Only E2E audit trail tests fail due to missing events
- Controller metrics and business logic tests all pass

**Recommendation**: Focus investigation on audit event write path, not schema validation.














