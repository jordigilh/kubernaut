# Webhook Audit Field Triage Against Authoritative Documentation

**Date**: January 6, 2026
**Status**: 🚨 **CRITICAL MISALIGNMENT DETECTED**
**Confidence**: 100% - Based on DD-WEBHOOK-003 (Approved ADR)

---

## 🚨 **CRITICAL FINDING**

The integration tests are expecting audit `event_data` fields (**"operator"**, **"crd_name"**, **"namespace"**, **"action"**) that **DO NOT EXIST** in the authoritative documentation.

**Authority Document**: [DD-WEBHOOK-003: Webhook-Complete Audit Pattern](../../architecture/decisions/DD-WEBHOOK-003-webhook-complete-audit-pattern.md)

---

## 📋 **Authoritative Audit Event Structure (per DD-WEBHOOK-003)**

### **Audit Table Columns** (ADR-034 v1.4)

Webhooks MUST use these **structured columns** for standard attribution fields:

| Column | Purpose | Webhook Usage | Example |
|--------|---------|---------------|---------|
| `actor_type` | WHO type | Always "user" | `"user"` |
| `actor_id` | WHO username | Authenticated user | `"admin"` or `"user@example.com"` |
| `resource_type` | WHAT type | CRD kind | `"WorkflowExecution"`, `"NotificationRequest"` |
| `resource_id` | WHAT identifier | CRD UID | `"abc-123-def-456"` |
| `resource_name` | WHAT name | CRD name | `"test-wfe-01"` |
| `namespace` | WHERE | CRD namespace | `"default"` |
| `event_category` | Emitter service | Always "webhook" | `"webhook"` |
| `event_action` | HOW | Operation | `"block_cleared"`, `"deleted"`, `"approval_decided"` |
| `correlation_id` | Correlation | CRD name | `"test-wfe-01"` |

**Key Principle**: Standard attribution fields (WHO, WHAT, WHERE, HOW) go in **structured columns**, NOT in `event_data` JSONB.

### **Event Data JSONB** (Domain-Specific Fields Only)

Per DD-WEBHOOK-003, `event_data` contains **business-specific context**, NOT attribution fields:

```go
// ❌ WRONG: Duplicating structured columns in event_data
event_data: {
    "operator":  "admin",         // ❌ DUPLICATE of actor_id column
    "crd_name":  "test-wfe-01",   // ❌ DUPLICATE of resource_name column
    "namespace": "default",       // ❌ DUPLICATE of namespace column
    "action":    "delete",        // ❌ DUPLICATE of event_action column
}

// ✅ CORRECT: Business context only
event_data: {
    "workflow_name":  "rollback-v2",    // Business field
    "clear_reason":   "Reviewed...",    // Business field
    "previous_state": "Blocked",        // Business field
    "new_state":      "Running",        // Business field
}
```

---

## 🔍 **Current Implementation vs. Authority**

### **1. WorkflowExecution Webhook**

#### **Current Implementation** (`pkg/authwebhook/workflowexecution_handler.go:111-117`)
```go
eventData := map[string]interface{}{
    "workflow_name": wfe.Name,           // ✅ CORRECT (business field)
    "clear_reason":  wfe.Status.BlockClearance.ClearReason,  // ✅ CORRECT
    "cleared_by":    wfe.Status.BlockClearance.ClearedBy,    // ⚠️ REDUNDANT (actor_id column)
    "cleared_at":    wfe.Status.BlockClearance.ClearedAt.Time,  // ✅ CORRECT (business timestamp)
    "action":        "block_clearance_approved",  // ⚠️ REDUNDANT (event_action column)
}
```

#### **DD-WEBHOOK-003 Specification** (lines 290-295)
```go
EventData: marshalJSON({
    "workflow_name": wfe.Name,           // ✅ Business field
    "clear_reason":  wfe.Status.BlockClearance.ClearReason,  // ✅ Business field
    "previous_state": oldWFE.Status.Phase,  // ✅ Business field
    "new_state":      wfe.Status.Phase,     // ✅ Business field
})
```

**Verdict**: ⚠️ **PARTIALLY COMPLIANT** (includes redundant "cleared_by" and "action" fields)

---

### **2. RemediationApprovalRequest Webhook**

#### **Current Implementation** (`pkg/authwebhook/remediationapprovalrequest_handler.go:111-117`)
```go
eventData := map[string]interface{}{
    "approval_request_name": rar.Name,   // ✅ CORRECT (business field)
    "decision":              string(rar.Status.Decision),  // ✅ CORRECT
    "decided_by":            rar.Status.DecidedBy,  // ⚠️ REDUNDANT (actor_id column)
    "decided_at":            rar.Status.DecidedAt.Time,  // ✅ CORRECT
    "action":                "approval_decision_made",  // ⚠️ REDUNDANT (event_action column)
}
```

#### **DD-WEBHOOK-003 Specification** (lines 314-318)
```go
EventData: marshalJSON({
    "request_name":     rar.Name,        // ✅ Business field
    "decision":         rar.Status.Decision,  // ✅ Business field
    "decision_message": rar.Status.DecisionMessage,  // ✅ Business field
    "ai_analysis_ref":  rar.Spec.AIAnalysisRef.Name,  // ✅ Business field
})
```

**Verdict**: ⚠️ **PARTIALLY COMPLIANT** (includes redundant "decided_by" and "action" fields, missing "decision_message" and "ai_analysis_ref")

---

### **3. NotificationRequest Webhook**

#### **Current Implementation** (`pkg/authwebhook/notificationrequest_validator.go:123-136`)
```go
eventData := map[string]interface{}{
    // ❌ Test expectations (NOT in DD-WEBHOOK-003)
    "operator":  authCtx.Username,  // ❌ WRONG: Duplicate of actor_id
    "crd_name":  nr.Name,           // ❌ WRONG: Duplicate of resource_name
    "namespace": nr.Namespace,      // ❌ WRONG: Duplicate of namespace column
    "action":    "delete",          // ❌ WRONG: Duplicate of event_action

    // ✅ Business context fields
    "notification_id": nr.Name,
    "type":            string(nr.Spec.Type),
    "priority":        string(nr.Spec.Priority),
    "user_uid":        authCtx.UID,
    "user_groups":     authCtx.Groups,
}
```

#### **DD-WEBHOOK-003 Specification** (lines 335-340)
```go
EventData: marshalJSON({
    "notification_name": nr.Name,        // ✅ Business field (note: "notification_name", not "crd_name")
    "notification_type": nr.Spec.Type,   // ✅ Business field
    "final_status":      nr.Status.Phase,  // ✅ Business field
    "recipients":        nr.Spec.Recipients,  // ✅ Business field
})
```

**Verdict**: ❌ **NON-COMPLIANT** (includes 4 fields that duplicate structured columns, missing authoritative business fields)

---

## 📊 **Test Expectations vs. Authority**

### **Integration Test Expectations** (`test/integration/authwebhook/notificationrequest_test.go:117-122`)

```go
validateEventData(event, map[string]interface{}{
    "operator":  nil,  // ❌ NOT IN DD-WEBHOOK-003
    "crd_name":  nrName,  // ❌ NOT IN DD-WEBHOOK-003
    "namespace": namespace,  // ❌ NOT IN DD-WEBHOOK-003
    "action":    "delete",  // ❌ NOT IN DD-WEBHOOK-003
})
```

**Problem**: Tests expect fields that **DO NOT EXIST** in DD-WEBHOOK-003.

**Root Cause**: Tests were written without referencing the authoritative ADR.

---

## 🎯 **Authoritative Field Mapping Matrix**

| Audit Information | Structured Column | event_data JSONB | Example |
|-------------------|-------------------|------------------|---------|
| **WHO** (operator username) | `actor_id` | ❌ NEVER | `"admin"` |
| **WHO** (operator type) | `actor_type` | ❌ NEVER | `"user"` |
| **WHAT** (resource type) | `resource_type` | ❌ NEVER | `"WorkflowExecution"` |
| **WHAT** (resource UID) | `resource_id` | ❌ NEVER | `"abc-123"` |
| **WHAT** (resource name) | `resource_name` | ❌ NEVER | `"test-wfe-01"` |
| **WHERE** (namespace) | `namespace` | ❌ NEVER | `"default"` |
| **HOW** (operation) | `event_action` | ❌ NEVER | `"deleted"` |
| **EMITTER** (service) | `event_category` | ❌ NEVER | `"webhook"` |
| **Business Context** | ❌ NEVER | ✅ YES | `{"workflow_name": "...", "clear_reason": "..."}` |

**Key Rule**: **NEVER** duplicate structured columns in `event_data`.

---

## 🔧 **Required Corrections**

### **Priority 1: Fix NotificationRequest Webhook** (BREAKING)

**File**: `pkg/authwebhook/notificationrequest_validator.go:123-136`

**BEFORE** (WRONG):
```go
eventData := map[string]interface{}{
    "operator":  authCtx.Username,  // ❌ Duplicate
    "crd_name":  nr.Name,           // ❌ Duplicate
    "namespace": nr.Namespace,      // ❌ Duplicate
    "action":    "delete",          // ❌ Duplicate
    "notification_id": nr.Name,
    "type":            string(nr.Spec.Type),
    "priority":        string(nr.Spec.Priority),
    "user_uid":        authCtx.UID,
    "user_groups":     authCtx.Groups,
}
```

**AFTER** (CORRECT per DD-WEBHOOK-003 lines 335-340):
```go
eventData := map[string]interface{}{
    // Business context ONLY (per DD-WEBHOOK-003)
    "notification_name": nr.Name,           // ✅ Business field
    "notification_type": string(nr.Spec.Type),  // ✅ Business field
    "priority":          string(nr.Spec.Priority),  // ✅ Business field (not in DD-WEBHOOK-003, but useful)
    "final_status":      nr.Status.Phase,   // ✅ Business field (per DD-WEBHOOK-003)
    "recipients":        nr.Spec.Recipients,  // ✅ Business field (per DD-WEBHOOK-003)
}

// Attribution fields are in structured columns (set via audit helper functions):
// - actor_id: authCtx.Username
// - resource_name: nr.Name
// - namespace: nr.Namespace
// - event_action: "deleted"
```

---

### **Priority 2: Fix Integration Tests** (BREAKING)

**File**: `test/integration/authwebhook/notificationrequest_test.go:117-122`

**BEFORE** (WRONG):
```go
validateEventData(event, map[string]interface{}{
    "operator":  nil,  // ❌ NOT IN DD-WEBHOOK-003
    "crd_name":  nrName,  // ❌ NOT IN DD-WEBHOOK-003
    "namespace": namespace,  // ❌ NOT IN DD-WEBHOOK-003
    "action":    "delete",  // ❌ NOT IN DD-WEBHOOK-003
})
```

**AFTER** (CORRECT per DD-WEBHOOK-003):
```go
// Validate structured columns (per ADR-034)
Expect(event.ActorId).To(Equal("admin"))  // WHO (from actor_id column)
Expect(event.ResourceName).To(Equal(nrName))  // WHAT (from resource_name column)
Expect(event.Namespace).To(Equal(&namespace))  // WHERE (from namespace column)
Expect(event.EventAction).To(Equal("deleted"))  // HOW (from event_action column)

// Validate event_data business context (per DD-WEBHOOK-003 lines 335-340)
validateEventData(event, map[string]interface{}{
    "notification_name": nrName,  // ✅ Business field
    "notification_type": "escalation",  // ✅ Business field
    "final_status":      nil,  // ✅ Business field (may be nil if not set)
    "recipients":        nil,  // ✅ Business field (verify existence)
})
```

---

### **Priority 3: Align WorkflowExecution Webhook** (NON-BREAKING)

**File**: `pkg/authwebhook/workflowexecution_handler.go:111-117`

**Recommended Changes**:
1. **Remove** `"cleared_by"` (redundant with `actor_id` column)
2. **Remove** `"action"` (redundant with `event_action` column)
3. **Add** `"previous_state"` and `"new_state"` (per DD-WEBHOOK-003 lines 293-294)

**AFTER**:
```go
eventData := map[string]interface{}{
    "workflow_name":  wfe.Name,
    "clear_reason":   wfe.Status.BlockClearance.ClearReason,
    "cleared_at":     wfe.Status.BlockClearance.ClearedAt.Time,
    "previous_state": oldWFE.Status.Phase,  // ADD
    "new_state":      wfe.Status.Phase,     // ADD
}
```

---

### **Priority 4: Align RemediationApprovalRequest Webhook** (NON-BREAKING)

**File**: `pkg/authwebhook/remediationapprovalrequest_handler.go:111-117`

**Recommended Changes**:
1. **Remove** `"decided_by"` (redundant with `actor_id` column)
2. **Remove** `"action"` (redundant with `event_action` column)
3. **Add** `"decision_message"` (per DD-WEBHOOK-003 line 316)
4. **Add** `"ai_analysis_ref"` (per DD-WEBHOOK-003 line 317)

**AFTER**:
```go
eventData := map[string]interface{}{
    "request_name":     rar.Name,
    "decision":         string(rar.Status.Decision),
    "decided_at":       rar.Status.DecidedAt.Time,
    "decision_message": rar.Status.DecisionMessage,  // ADD
    "ai_analysis_ref":  rar.Spec.AIAnalysisRef.Name,  // ADD
}
```

---

## 📚 **Authoritative References**

### **Primary Authority**
- **DD-WEBHOOK-003**: Webhook-Complete Audit Pattern (Approved 2026-01-05)
  - Lines 290-295: WorkflowExecution event_data specification
  - Lines 314-318: RemediationApprovalRequest event_data specification
  - Lines 335-340: NotificationRequest event_data specification

### **Supporting Authority**
- **ADR-034 v1.4**: Unified Audit Table Design
  - Lines 50-118: Audit table schema with structured columns
  - Lines 133-144: Event category naming convention

---

## 🎯 **Decision Required**

**Question**: Should we align implementations and tests with DD-WEBHOOK-003 (the authoritative ADR)?

### **Option A: Align with DD-WEBHOOK-003** (RECOMMENDED)

**Pros**:
- ✅ Compliance with approved ADR
- ✅ Eliminates redundant fields
- ✅ Consistent with structured column design
- ✅ Matches ADR examples exactly

**Cons**:
- ❌ Breaking change (tests will fail until updated)
- ❌ Requires updating all 3 webhook implementations
- ❌ Requires rewriting integration tests

**Effort**: ~2-3 hours (fix implementations + tests)

---

### **Option B: Update DD-WEBHOOK-003 to Match Current Implementation** (NOT RECOMMENDED)

**Pros**:
- ✅ No code changes required
- ✅ Tests pass as-is

**Cons**:
- ❌ ADR becomes non-authoritative
- ❌ Duplicates structured columns in event_data (anti-pattern)
- ❌ Violates separation of concerns (structured vs. flexible data)
- ❌ Increases storage cost (~200-500 bytes per event)

**Effort**: ~1 hour (update ADR)

---

### **Option C: Create DD-WEBHOOK-004 with New Pattern** (NOT RECOMMENDED)

**Pros**:
- ✅ Allows coexistence of both patterns
- ✅ Documents intentional deviation

**Cons**:
- ❌ Creates confusion (which pattern to use?)
- ❌ Still duplicates structured columns
- ❌ Requires justification for deviation from ADR-034

**Effort**: ~2 hours (new ADR + updates)

---

## 🏆 **Recommendation**

**Proceed with Option A**: Align implementations and tests with DD-WEBHOOK-003.

**Rationale**:
1. **ADR-034 v1.4** explicitly defines structured columns for attribution fields
2. **DD-WEBHOOK-003** (approved ADR) specifies business-context-only in event_data
3. **Separation of concerns**: Structured columns for indexing, JSONB for flexible data
4. **Performance**: Reduces event_data size by ~30-40%
5. **Consistency**: All services follow same pattern

**Breaking Change Notice**: Integration tests will fail after webhook fixes until test expectations are updated to validate structured columns instead of event_data fields.

---

## 📅 **Implementation Plan**

### **Phase 1: Fix NotificationRequest Webhook** (CRITICAL)
1. Update `pkg/authwebhook/notificationrequest_validator.go` event_data per DD-WEBHOOK-003 lines 335-340
2. Remove "operator", "crd_name", "namespace", "action" from event_data
3. Add "notification_name", "notification_type", "final_status", "recipients"

### **Phase 2: Update Integration Tests** (CRITICAL)
1. Update `test/integration/authwebhook/notificationrequest_test.go`
2. Validate structured columns (ActorId, ResourceName, Namespace, EventAction)
3. Validate business-context-only event_data fields

### **Phase 3: Align WorkflowExecution Webhook** (NON-CRITICAL)
1. Remove redundant "cleared_by" and "action" from event_data
2. Add "previous_state" and "new_state" per DD-WEBHOOK-003

### **Phase 4: Align RemediationApprovalRequest Webhook** (NON-CRITICAL)
1. Remove redundant "decided_by" and "action" from event_data
2. Add "decision_message" and "ai_analysis_ref" per DD-WEBHOOK-003

---

**Document Created**: January 6, 2026
**Status**: Awaiting user decision (Option A/B/C)
**Confidence**: 100% - Based on approved ADR-034 v1.4 and DD-WEBHOOK-003
**Impact**: Breaking change for integration tests, ~200-500 bytes per event storage reduction

