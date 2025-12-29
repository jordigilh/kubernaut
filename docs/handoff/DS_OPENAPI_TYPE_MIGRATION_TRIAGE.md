# Data Storage: OpenAPI Type Migration Triage

**Date**: 2025-12-13
**Status**: 🚨 **CRITICAL CODE SMELL IDENTIFIED**
**Priority**: HIGH - Blocking Phase 4 refactoring completion

---

## 🚨 **Problem Statement**

**Code Smell Identified**: Extensive use of unstructured `map[string]interface{}` instead of OpenAPI-generated types throughout DataStorage handlers.

**Why This Is Bad**:
- ❌ **Type-unsafe**: No compile-time validation
- ❌ **Error-prone**: Manual type assertions everywhere
- ❌ **Inconsistent**: Not aligned with OpenAPI spec
- ❌ **Hard to maintain**: Changes to spec don't trigger compilation errors
- ❌ **Poor error messages**: Runtime failures instead of compile-time
- ❌ **Violates Go best practices**: Prefer structured types over `interface{}`

**Root Cause**: Handlers were written before OpenAPI spec was complete/mature.

---

## 📊 **Triage Results**

### **Files Using Unstructured Data** (3 files found)

| File | Lines | Usage | Severity |
|------|-------|-------|----------|
| `audit_events_handler.go` | 100 | `var payload map[string]interface{}` | 🔴 CRITICAL |
| `audit_events_batch_handler.go` | 73 | `var payloads []map[string]interface{}` | 🔴 CRITICAL |
| `dlq_retry_worker.go` | 239 | `var data map[string]interface{}` | 🟡 MEDIUM |

### **Helper Files Created (should be deleted/rewritten)**

| File | Lines | Usage | Action |
|------|-------|-------|--------|
| `helpers/parsing.go` | 148 | All functions use `map[string]interface{}` | ❌ DELETE |
| `helpers/validation.go` | 206 | Uses `map[string]interface{}` in params | ⚠️ REVIEW |

---

## 🎯 **Available OpenAPI Types**

### **Generated Types Location**: `pkg/datastorage/client/generated.go`

**Key Types We Should Use**:
```go
// Request types
type AuditEventRequest struct {
    ActorId        *string                `json:"actor_id,omitempty"`
    ActorType      *string                `json:"actor_type,omitempty"`
    CorrelationId  string                 `json:"correlation_id"`
    EventAction    string                 `json:"event_action"`
    EventCategory  string                 `json:"event_category"`
    EventData      map[string]interface{} `json:"event_data"`
    EventOutcome   string                 `json:"event_outcome"`
    EventTimestamp string                 `json:"event_timestamp"`
    EventType      string                 `json:"event_type"`
    ResourceId     *string                `json:"resource_id,omitempty"`
    ResourceType   *string                `json:"resource_type,omitempty"`
    Version        string                 `json:"version"`
}

// Response types
type AuditEventResponse struct {
    EventId        string `json:"event_id"`
    EventTimestamp string `json:"event_timestamp"`
    Message        string `json:"message"`
}

// Batch types
type BatchAuditEventRequest []AuditEventRequest
```

---

## 🔧 **Migration Strategy**

### **Phase 1: Update Handlers to Use OpenAPI Types** [2h]

#### **1.1: audit_events_handler.go** (Critical)
**Current**:
```go
var payload map[string]interface{}
if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
    // error handling
}
eventType, _ := payload["event_type"].(string)
eventCategory, _ := payload["event_category"].(string)
// ... manual extraction everywhere
```

**Target**:
```go
var req client.AuditEventRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    // error handling
}
// Direct access: req.EventType, req.EventCategory, etc.
// No type assertions needed!
```

#### **1.2: audit_events_batch_handler.go** (Critical)
**Current**:
```go
var payloads []map[string]interface{}
if err := json.NewDecoder(r.Body).Decode(&payloads); err != nil {
    // error handling
}
```

**Target**:
```go
var req client.BatchAuditEventRequest // which is []AuditEventRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    // error handling
}
```

#### **1.3: dlq_retry_worker.go** (Medium Priority)
**Current**:
```go
var data map[string]interface{}
if err := json.Unmarshal(event.EventData, &data); err != nil {
    // error handling
}
```

**Target**: This might be OK - DLQ handles arbitrary event_data which truly is unstructured. Need to review if this can use a structured type.

---

### **Phase 2: Simplify/Delete Helper Functions** [1h]

#### **2.1: Delete `helpers/parsing.go`** (148 lines)
**Rationale**: All these functions exist to extract fields from unstructured maps. With OpenAPI types, we get direct field access.

**Before**:
```go
eventType, eventCategory, eventAction, eventOutcome := helpers.ExtractEventFields(payload)
```

**After**:
```go
// Direct access to struct fields!
req.EventType
req.EventCategory
req.EventAction
req.EventOutcome
```

#### **2.2: Simplify `helpers/validation.go`** (206 lines)
Some validations still needed:
- ✅ Keep: `ValidateEventOutcome()` - enum validation
- ✅ Keep: `ValidateTimestampBounds()` - business rule validation
- ✅ Keep: `ValidateFieldLengths()` - business rule validation
- ❌ Delete: `ValidateRequiredFields()` - OpenAPI handles this via JSON unmarshaling + required tags
- ❌ Delete: `ParseTimestamp()` - Can use standard time.Parse directly

**Estimated Reduction**: 206 lines → ~80 lines (only business validation)

---

### **Phase 3: Update Tests** [1h]

All tests using `map[string]interface{}` for request bodies should use OpenAPI types:
- `test/integration/datastorage/audit_events_write_api_test.go`
- `test/e2e/datastorage/09_event_type_jsonb_comprehensive_test.go`
- `test/e2e/datastorage/10_malformed_event_rejection_test.go`
- etc.

---

## 📈 **Expected Impact**

### **Before Migration**
- **Handler code**: 990 lines with manual type assertions everywhere
- **Helper code**: 354 lines (parsing + validation)
- **Type safety**: Runtime failures
- **Maintainability**: Low (spec changes don't trigger compile errors)

### **After Migration**
- **Handler code**: ~600 lines (cleaner, direct field access)
- **Helper code**: ~80 lines (only business validation)
- **Type safety**: Compile-time validation
- **Maintainability**: High (spec changes cause compilation errors)

**Net Reduction**:
- **Handlers**: -390 lines (39% reduction)
- **Helpers**: -274 lines (77% reduction)
- **Total**: -664 lines saved + improved type safety

---

## ⏱️ **Time Estimate**

| Phase | Task | Time |
|-------|------|------|
| **1** | Migrate audit_events_handler.go | 1h |
| **1** | Migrate audit_events_batch_handler.go | 0.5h |
| **1** | Review dlq_retry_worker.go | 0.5h |
| **2** | Delete/simplify helpers | 0.5h |
| **3** | Update tests | 1h |
| **4** | Compile & validate | 0.5h |
| **Total** | | **4 hours** |

---

## 🚦 **Decision & Next Steps**

### **Approved Migration Plan**

1. ✅ **Phase 1.1**: Migrate `audit_events_handler.go` to use `client.AuditEventRequest`
2. ✅ **Phase 1.2**: Migrate `audit_events_batch_handler.go` to use `client.BatchAuditEventRequest`
3. ✅ **Phase 1.3**: Review `dlq_retry_worker.go` (may keep unstructured for event_data)
4. ✅ **Phase 2**: Delete `helpers/parsing.go`, simplify `helpers/validation.go`
5. ✅ **Phase 3**: Update integration/E2E tests
6. ✅ **Phase 4**: Compile, validate, run tests

### **Post-Migration: Resume Phase 4 Refactoring**

Once migration is complete:
- Handler will be ~600 lines (down from 990)
- Type-safe, maintainable, aligned with OpenAPI spec
- Can then continue with remaining Phase 4 work (if still needed)

---

## 📝 **Related Documents**

- **OpenAPI Spec**: `api/openapi/data-storage-v1.yaml`
- **Generated Client**: `pkg/datastorage/client/generated.go`
- **Phase 4 Analysis**: `docs/handoff/DS_PHASE4_AUDIT_HANDLER_ANALYSIS.md`
- **Refactoring Roadmap**: `docs/handoff/DS_REFACTORING_FINAL_ROADMAP.md`

---

**Document Version**: 1.0
**Last Updated**: 2025-12-13
**Status**: ✅ TRIAGE COMPLETE - Ready for migration

