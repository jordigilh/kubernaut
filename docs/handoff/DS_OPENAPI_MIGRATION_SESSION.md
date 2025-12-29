# Data Storage: OpenAPI Type Migration - Session Document

**Date**: 2025-12-13
**Status**: 🚧 **IN PROGRESS**
**Duration**: ~4 hours (estimated)

---

## 🎯 **Session Goal**

Migrate DataStorage handlers from unstructured `map[string]interface{}` to OpenAPI-generated types for type safety and maintainability.

---

## 📋 **Progress Tracker**

### ✅ **Completed**
- [x] Triage DS code for unstructured data usage
- [x] Identified 3 files using `map[string]interface{}`
- [x] Created migration plan
- [x] Reviewed OpenAPI types structure

### 🚧 **In Progress**
- [ ] Step 1: Create conversion helpers (client.AuditEventRequest → audit.AuditEvent)
- [ ] Step 2: Migrate audit_events_handler.go
- [ ] Step 3: Migrate audit_events_batch_handler.go
- [ ] Step 4: Review dlq_retry_worker.go
- [ ] Step 5: Delete helpers/parsing.go
- [ ] Step 6: Simplify helpers/validation.go
- [ ] Step 7: Compile & test all changes

---

## 🔧 **Implementation Plan**

### **Step 1: Create Conversion Helpers** [30min]

**File**: `pkg/datastorage/server/helpers/openapi_conversion.go`

**Purpose**: Convert between OpenAPI types and internal types

**Functions**:
```go
// ConvertAuditEventRequest converts OpenAPI request to internal audit event
func ConvertAuditEventRequest(req client.AuditEventRequest) (*audit.AuditEvent, error)

// ConvertToAuditEventResponse converts internal type to OpenAPI response
func ConvertToAuditEventResponse(event *repository.AuditEvent) client.AuditEventResponse
```

**Key Conversions**:
- `req.EventTimestamp` (time.Time) → `event.EventTimestamp` (time.Time) ✅ Direct
- `req.EventOutcome` (enum) → `event.EventOutcome` (string)
- `*req.ActorId` (pointer) → `event.ActorID` (string) with defaults
- `req.EventData` (map) → `event.EventData` (json.RawMessage)

---

### **Step 2: Migrate audit_events_handler.go** [1h]

**Current** (lines 98-110):
```go
// 1. Parse request body (JSON payload with all fields)
s.logger.V(1).Info("Parsing request body...")
var payload map[string]interface{}
if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
    s.logger.Info("Invalid JSON in request body", "error", err)
    writeRFC7807Error(w, validation.NewValidationErrorProblem(
        "audit_event",
        map[string]string{"body": "invalid JSON: " + err.Error()},
    ))
    return
}

// 2. Validate required fields...
// ... 400+ lines of manual extraction and validation
```

**Target**:
```go
// 1. Parse request body using OpenAPI type
s.logger.V(1).Info("Parsing request body...")
var req client.AuditEventRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    s.logger.Info("Invalid JSON in request body", "error", err)
    response.WriteRFC7807Error(w, s.logger, http.StatusBadRequest,
        "invalid_request", "Invalid Request", err.Error(), "")
    return
}

// 2. Validate business rules (timestamp bounds, field lengths, etc.)
if err := helpers.ValidateAuditEventRequest(&req); err != nil {
    // ... RFC 7807 error
    return
}

// 3. Convert to internal type
auditEvent, err := helpers.ConvertAuditEventRequest(req)
if err != nil {
    // ... RFC 7807 error
    return
}

// 4. Write to repository (existing logic)
created, err := s.repository.CreateAuditEvent(ctx, auditEvent)
// ... existing success/DLQ logic
```

**Changes**:
- Remove ~300 lines of manual field extraction
- Remove ~100 lines of manual validation (JSON unmarshaling handles required fields)
- Keep ~100 lines of business validation (timestamp bounds, field lengths)
- Keep all DLQ/logging logic as-is

**Expected**: ~990 lines → ~550 lines (44% reduction)

---

### **Step 3: Migrate audit_events_batch_handler.go** [30min]

**Current** (line 73):
```go
var payloads []map[string]interface{}
if err := json.NewDecoder(r.Body).Decode(&payloads); err != nil {
    // error handling
}
```

**Target**:
```go
var req []client.AuditEventRequest  // OpenAPI batch type
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    // error handling
}

// Convert each request to internal type
events := make([]*audit.AuditEvent, 0, len(req))
for i, r := range req {
    event, err := helpers.ConvertAuditEventRequest(r)
    if err != nil {
        // ... validation error for event[i]
        continue
    }
    events = append(events, event)
}
```

**Expected**: Simpler, type-safe batch processing

---

### **Step 4: Review dlq_retry_worker.go** [15min]

**Current** (line 239):
```go
var data map[string]interface{}
if err := json.Unmarshal(event.EventData, &data); err != nil {
    // error handling
}
```

**Decision**: **Keep as-is** (event_data is truly unstructured, varies by event type)

**Rationale**: The DLQ handles arbitrary event_data which doesn't have a fixed schema.

---

### **Step 5: Delete helpers/parsing.go** [5min]

**File**: `pkg/datastorage/server/helpers/parsing.go` (148 lines)

**Action**: DELETE entirely

**Rationale**: All parsing functions extract fields from `map[string]interface{}`. With OpenAPI types, we get direct field access via struct fields.

**Before**:
```go
eventType, eventCategory, eventAction, eventOutcome := helpers.ExtractEventFields(payload)
actorType, actorID := helpers.ExtractActorFields(payload, eventCategory)
```

**After**:
```go
// Direct access!
req.EventType
req.EventCategory
req.EventAction
req.EventOutcome
```

---

### **Step 6: Simplify helpers/validation.go** [30min]

**Current**: 206 lines (5 functions)

**Target**: ~80 lines (3 functions)

**Keep** (business validation):
- `ValidateEventOutcome()` - Enum validation (success/failure/pending)
- `ValidateTimestampBounds()` - Gap 1.2: Reject future/old timestamps
- `ValidateFieldLengths()` - Gap 1.2: Max length enforcement

**Delete** (handled by JSON unmarshaling):
- `ValidateRequiredFields()` - OpenAPI struct tags handle this
- `ParseTimestamp()` - OpenAPI decoder handles time.Time parsing
- `DefaultRequiredFieldsConfig()` - No longer needed

**New function** (replaces all deleted functions):
```go
// ValidateAuditEventRequest validates business rules for OpenAPI request
// JSON unmarshaling already validated required fields and types
func ValidateAuditEventRequest(req *client.AuditEventRequest) *validation.RFC7807Problem {
    // 1. Validate event_outcome enum
    if err := ValidateEventOutcome(string(req.EventOutcome)); err != nil {
        return err
    }

    // 2. Validate timestamp bounds
    if err := ValidateTimestampBounds(req.EventTimestamp); err != nil {
        return err
    }

    // 3. Validate field lengths
    fields := map[string]string{
        "event_type":     req.EventType,
        "event_category": req.EventCategory,
        "event_action":   req.EventAction,
        "correlation_id": req.CorrelationId,
    }
    if req.ActorType != nil {
        fields["actor_type"] = *req.ActorType
    }
    if req.ActorId != nil {
        fields["actor_id"] = *req.ActorId
    }
    if err := ValidateFieldLengths(fields, DefaultFieldLengthConstraints()); err != nil {
        return err
    }

    return nil
}
```

---

### **Step 7: Compile & Test** [1h]

**Actions**:
1. Compile all packages: `go build ./pkg/datastorage/...`
2. Run unit tests: `go test ./pkg/datastorage/...`
3. Update integration tests to use OpenAPI types
4. Verify all tests pass

---

## 📊 **Expected Impact**

### **Code Reduction**

| File | Before | After | Reduction |
|------|--------|-------|-----------|
| `audit_events_handler.go` | 990 lines | ~550 lines | -440 lines (44%) |
| `audit_events_batch_handler.go` | ~150 lines | ~100 lines | -50 lines (33%) |
| `helpers/parsing.go` | 148 lines | **DELETED** | -148 lines (100%) |
| `helpers/validation.go` | 206 lines | ~80 lines | -126 lines (61%) |
| **Total** | 1,494 lines | 730 lines | **-764 lines (51%)** |

### **New Files**

| File | Lines | Purpose |
|------|-------|---------|
| `helpers/openapi_conversion.go` | ~100 lines | Conversion helpers |

**Net Reduction**: -664 lines (44% overall)

---

## ✅ **Benefits**

1. **Type Safety**: Compile-time validation instead of runtime failures
2. **Maintainability**: Spec changes trigger compilation errors
3. **Simplicity**: Direct field access vs manual type assertions
4. **Consistency**: Single source of truth (OpenAPI spec)
5. **Error Messages**: Better error reporting from structured types
6. **Testing**: Easier to create test fixtures with structured types

---

## 🚨 **Risks & Mitigations**

| Risk | Mitigation |
|------|------------|
| Breaking changes to API | OpenAPI spec already matches current behavior |
| Conversion bugs | Unit tests for conversion functions |
| Integration test failures | Update tests to use OpenAPI types |
| Missing validation | Keep all business validation (timestamp, length) |

---

## 📝 **Next Actions**

1. **Create `openapi_conversion.go`** with conversion helpers
2. **Refactor `audit_events_handler.go`** to use OpenAPI types
3. **Refactor `audit_events_batch_handler.go`** similarly
4. **Delete `parsing.go`** entirely
5. **Simplify `validation.go`** to only business rules
6. **Update tests** to use structured types
7. **Compile & validate** all changes

---

**Document Version**: 1.0
**Last Updated**: 2025-12-13
**Status**: 🚧 IN PROGRESS - Step 1 starting

