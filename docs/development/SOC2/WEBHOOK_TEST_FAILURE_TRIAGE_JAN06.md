# Webhook Integration Test Failure Triage (Jan 6, 2026)

**Status**: ✅ **ISSUE IDENTIFIED** | 🔧 **FIX READY**

---

## 📊 **Test Results Summary**

**Overall**: 7/9 tests passing (78%)

| Webhook Type | Test Cases | Pass | Fail | Status |
|--------------|------------|------|------|--------|
| **WorkflowExecution** (UPDATE) | 4 | 4 | 0 | ✅ **ALL PASS** |
| **RemediationApprovalRequest** (UPDATE) | 3 | 3 | 0 | ✅ **ALL PASS** |
| **NotificationRequest** (DELETE) | 2 | 0 | 2 | ❌ **ALL FAIL** |

---

## ✅ **Passing Tests (7/9)**

### WorkflowExecution Block Clearance Attribution (4 tests)
- ✅ INT-WE-01: Operator clears workflow execution block
- ✅ INT-WE-02: Missing clearance reason validation
- ✅ INT-WE-03: Short clearance reason validation
- ✅ (1 unnamed test passing)

### RemediationApprovalRequest Decision Attribution (3 tests)
- ✅ INT-RAR-01: Operator approves remediation request
- ✅ INT-RAR-02: Operator rejects remediation request
- ✅ INT-RAR-03: Invalid decision validation

---

## ❌ **Failing Tests (2/9)**

### NotificationRequest DELETE Attribution (2 tests)

#### **FAIL 1: INT-NR-01 - Missing 'operator' field**

**Test**: `BR-AUTH-001: NotificationRequest Cancellation Attribution`
**Scenario**: `INT-NR-01: when operator cancels notification via DELETE`
**Expectation**: `should capture operator identity in audit trail via webhook`
**File**: `test/integration/authwebhook/notificationrequest_test.go:59`
**Error Location**: `helpers.go:282`

**Error**:
```
[FAILED] event_data should have field 'operator'
Expected map to have key: operator
```

**Actual event_data**:
```json
{
  "user_groups": ["system:masters", "system:authenticated"],
  "user_uid": "",
  "action": "notification_cancelled",
  "cancelled_by": "admin",         // ❌ Should be "operator"
  "notification_id": "test-nr-cancel-812872af",
  "priority": "high",
  "type": "escalation"
}
```

**Expected event_data** (per test, lines 117-122):
```go
validateEventData(event, map[string]interface{}{
    "operator":  nil,  // ✅ MUST exist
    "crd_name":  nrName,  // ✅ MUST exist
    "namespace": namespace,  // ✅ MUST exist
    "action":    "delete",  // ✅ MUST be "delete"
})
```

---

#### **FAIL 2: INT-NR-03 - Wrong 'action' value**

**Test**: `BR-AUTH-001: NotificationRequest Cancellation Attribution`
**Scenario**: `INT-NR-03: when NotificationRequest is deleted during processing`
**Expectation**: `should capture attribution even if CRD is mid-processing`
**File**: `test/integration/authwebhook/notificationrequest_test.go:192`
**Error Location**: `helpers.go:287`

**Error**:
```
[FAILED] event_data['action'] should equal 'delete'
Expected: notification_cancelled
To equal: delete
```

**Root Cause**: Same as FAIL 1 - webhook implementation uses wrong field names and values.

---

## 🔍 **Root Cause Analysis**

### Webhook Implementation Issue

**File**: `pkg/authwebhook/notificationrequest_validator.go`
**Lines**: 122-130

**Current Implementation** (INCORRECT):
```go
eventData := map[string]interface{}{
    "notification_id": nr.Name,
    "type":            string(nr.Spec.Type),
    "priority":        string(nr.Spec.Priority),
    "cancelled_by":    authCtx.Username,  // ❌ Should be "operator"
    "user_uid":        authCtx.UID,
    "user_groups":     authCtx.Groups,
    "action":          "notification_cancelled",  // ❌ Should be "delete"
}
```

**Expected Implementation** (CORRECT):
```go
eventData := map[string]interface{}{
    "operator":        authCtx.Username,  // ✅ Test expects "operator"
    "crd_name":        nr.Name,           // ✅ Test expects "crd_name"
    "namespace":       nr.Namespace,      // ✅ Test expects "namespace"
    "action":          "delete",          // ✅ Test expects "delete"
    "notification_id": nr.Name,           // Keep for audit completeness
    "type":            string(nr.Spec.Type),
    "priority":        string(nr.Spec.Priority),
    "user_uid":        authCtx.UID,
    "user_groups":     authCtx.Groups,
}
```

### Why This Happened

**Context**: The webhook implementation was created before the integration tests were written. The test expectations follow the standard pattern established by WorkflowExecution and RemediationApprovalRequest webhooks.

**Mismatch**:
1. **Field naming**: Webhook used `"cancelled_by"` instead of standard `"operator"`
2. **Action value**: Webhook used domain-specific `"notification_cancelled"` instead of generic `"delete"`
3. **Missing fields**: Webhook didn't include `"crd_name"` and `"namespace"` (standard audit fields)

---

## 🔧 **Fix Required**

### File to Modify
`pkg/authwebhook/notificationrequest_validator.go` lines 122-130

### Changes Needed

**BEFORE**:
```go
eventData := map[string]interface{}{
    "notification_id": nr.Name,
    "type":            string(nr.Spec.Type),
    "priority":        string(nr.Spec.Priority),
    "cancelled_by":    authCtx.Username,
    "user_uid":        authCtx.UID,
    "user_groups":     authCtx.Groups,
    "action":          "notification_cancelled",
}
```

**AFTER**:
```go
eventData := map[string]interface{}{
    // Standard audit fields (per DD-TESTING-001)
    "operator":        authCtx.Username,  // SOC2 CC8.1: WHO cancelled
    "crd_name":        nr.Name,           // WHAT was cancelled
    "namespace":       nr.Namespace,      // WHERE it happened
    "action":          "delete",          // HOW it was cancelled

    // NotificationRequest-specific context
    "notification_id": nr.Name,
    "type":            string(nr.Spec.Type),
    "priority":        string(nr.Spec.Priority),
    "user_uid":        authCtx.UID,
    "user_groups":     authCtx.Groups,
}
```

### Rationale

**Standard Field Pattern** (per WorkflowExecution/RemediationApprovalRequest webhooks):
- `"operator"`: WHO performed the action (SOC2 CC8.1 attribution)
- `"crd_name"`: WHAT resource was affected
- `"namespace"`: WHERE it happened
- `"action"`: WHAT action was taken (generic, not domain-specific)

**Additional Context Fields**: Domain-specific fields provide audit completeness but aren't validated by standard test helpers.

---

## ✅ **Validation Plan**

### After Applying Fix

**Expected Test Results**:
```
✅ INT-NR-01: Operator cancels notification via DELETE
✅ INT-NR-03: NotificationRequest deleted during processing
```

**Validation Commands**:
```bash
# Run only NotificationRequest DELETE tests
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut
make test-integration-authwebhook

# Expected output:
# Ran 9 of 9 Specs
# PASS! - 9 Passed | 0 Failed | 0 Pending | 0 Skipped
```

### Regression Check

**Confirm**:
- ✅ WorkflowExecution tests still pass (no impact)
- ✅ RemediationApprovalRequest tests still pass (no impact)
- ✅ All 9 webhook integration tests pass

---

## 📚 **References**

### Authority Documents
- **DD-WEBHOOK-003**: Webhook-Complete Audit Pattern
- **DD-TESTING-001**: Audit Event Validation Standards
- **ADR-034 v1.4**: Unified Audit Table Design

### Test Expectations
- **File**: `test/integration/authwebhook/notificationrequest_test.go`
- **Lines**: 117-122 (INT-NR-01), 248-253 (INT-NR-03)
- **Helper**: `helpers.go:275-291` (validateEventData)

### Webhook Implementation
- **File**: `pkg/authwebhook/notificationrequest_validator.go`
- **Lines**: 122-130 (event_data payload)
- **Pattern**: Kubebuilder CustomValidator with ValidateDelete()

---

## 🎯 **Success Criteria**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| **Issue Identified** | ✅ COMPLETE | Field naming mismatch documented |
| **Root Cause Understood** | ✅ COMPLETE | Webhook vs test expectations analyzed |
| **Fix Designed** | ✅ COMPLETE | Exact code changes specified |
| **Fix Applied** | ⏳ PENDING | Awaiting user approval |
| **Tests Passing** | ⏳ PENDING | Awaiting fix application |
| **No Regressions** | ⏳ PENDING | Awaiting test execution |

---

## 🏆 **Key Insights**

### Webhook Implementation Working Correctly

**Evidence from Test Logs**:
```
🔍 ValidateDelete invoked: Name=test-nr-complete-17f3db03
✅ Authenticated user: admin (UID: )
📝 Creating audit event for DELETE operation...
✅ Audit event created: type=notification.request.deleted
💾 Storing audit event to Data Storage...
✅ Audit event stored successfully
✅ Allowing DELETE operation for default/test-nr-complete-17f3db03
```

**Conclusion**: The Kubebuilder CustomValidator pattern is **100% functional**. The only issue is **field naming consistency** with test expectations.

### Embedded Spec Issue Resolved

**Evidence**: No `WARNING: Failed to store audit event` errors for "webhook" category in latest test runs.

**Conclusion**: The `make generate` solution successfully synchronized both embedded OpenAPI specs (`pkg/audit/` and `pkg/datastorage/server/middleware/`).

---

**Document Created**: January 6, 2026
**Status**: Fix ready for application
**Confidence**: 100% - Trivial field renaming, no logic changes required
**Next Step**: Apply fix to `notificationrequest_validator.go` lines 122-130

