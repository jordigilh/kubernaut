# Audit Events Handler - OpenAPI Refactoring Plan

**File**: `pkg/datastorage/server/audit_events_handler.go`
**Function**: `handleCreateAuditEvent` (lines 88-610, 523 lines)
**Goal**: Replace `map[string]interface{}` with `client.AuditEventRequest`
**Expected**: 523 lines → ~250 lines (52% reduction)

---

## 🎯 **Refactoring Strategy**

### **Current Structure** (523 lines)
1. Parse JSON into `map[string]interface{}` (lines 98-116)
2. Validate required fields manually (lines 118-171)
3. Extract fields manually with type assertions (lines 173-193)
4. Validate event_outcome enum (lines 207-229)
5. Parse timestamp manually (lines 231-255)
6. Validate timestamp bounds (lines 269-308)
7. Extract actor fields with defaults (lines 310-334)
8. Validate field lengths manually (lines 336-385)
9. Extract more optional fields (lines 387-449)
10. Create repository.AuditEvent (lines 451-487)
11. Write to database (lines 489-495)
12. Handle DLQ fallback (lines 497-574)
13. Success response (lines 576-610)

### **Target Structure** (~250 lines)
1. Parse JSON into `client.AuditEventRequest` (lines 98-110) - **12 lines**
2. Validate business rules (timestamp, lengths, enum) (lines 112-120) - **8 lines**
3. Convert to internal type (lines 122-130) - **8 lines**
4. Convert to repository type (lines 132-140) - **8 lines**
5. Write to database (lines 142-148) - **6 lines**
6. Handle DLQ fallback (lines 150-214) - **64 lines** (keep as-is)
7. Success response (lines 216-250) - **34 lines** (keep as-is)

**Reduction**: Remove ~273 lines of manual parsing/validation

---

## 📝 **Detailed Changes**

### **Section 1: Parse Request Body** (Lines 98-116)

**BEFORE** (19 lines):
```go
// 1. Parse request body (JSON payload with all fields)
s.logger.V(1).Info("Parsing request body...")
var payload map[string]interface{}
if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
    s.logger.Info("Invalid JSON in request body",
        "error", err,
        "remote_addr", r.RemoteAddr)

    // Record validation failure metric (BR-STORAGE-019)
    if s.metrics != nil && s.metrics.ValidationFailures != nil {
        s.metrics.ValidationFailures.WithLabelValues("body", "invalid_json").Inc()
    }

    writeRFC7807Error(w, validation.NewValidationErrorProblem(
        "audit_event",
        map[string]string{"body": "invalid JSON: " + err.Error()},
    ))
    return
}
```

**AFTER** (12 lines):
```go
// 1. Parse request body using OpenAPI type (type-safe)
s.logger.V(1).Info("Parsing request body...")
var req dsclient.AuditEventRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    s.logger.Info("Invalid JSON in request body", "error", err)

    if s.metrics != nil && s.metrics.ValidationFailures != nil {
        s.metrics.ValidationFailures.WithLabelValues("body", "invalid_json").Inc()
    }

    response.WriteRFC7807Error(w, s.logger, http.StatusBadRequest, "invalid_request", "Invalid Request", err.Error(), "")
    return
}
```

### **Section 2: Validate Required Fields** (Lines 118-171)

**DELETE ENTIRELY** (54 lines)

**Rationale**: OpenAPI JSON unmarshaling automatically validates:
- Required fields are present
- Field types are correct (string vs int vs time.Time)
- Enum values are valid

### **Section 3-8: Manual Field Extraction & Validation** (Lines 173-449)

**DELETE ENTIRELY** (277 lines)

**Replace with** (20 lines):
```go
// 2. Validate business rules (OpenAPI already validated required fields & types)
s.logger.V(1).Info("Validating business rules...")
if err := helpers.ValidateAuditEventRequest(&req); err != nil {
    s.logger.Info("Business validation failed", "error", err)

    if s.metrics != nil && s.metrics.ValidationFailures != nil {
        s.metrics.ValidationFailures.WithLabelValues("business_rules", "validation_failed").Inc()
    }

    response.WriteRFC7807Error(w, s.logger, http.StatusBadRequest, "validation_error", "Validation Error", err.Error(), "")
    return
}

// 3. Convert OpenAPI request to internal audit event
s.logger.V(1).Info("Converting to internal audit event...")
auditEvent, err := helpers.ConvertAuditEventRequest(req)
if err != nil {
    s.logger.Error(err, "Failed to convert audit event request")
    response.WriteRFC7807Error(w, s.logger, http.StatusInternalServerError, "conversion_error", "Conversion Error", err.Error(), "")
    return
}
```

### **Section 9: Create Repository Event** (Lines 451-487)

**BEFORE** (37 lines of manual struct creation):
```go
// Create repository audit event
repositoryEvent := &repository.AuditEvent{
    EventID:           uuid.New(),
    EventTimestamp:    eventTimestamp,
    EventType:         eventType,
    // ... 30+ field assignments
}
```

**AFTER** (8 lines):
```go
// 4. Convert to repository type
s.logger.V(1).Info("Converting to repository type...")
repositoryEvent, err := helpers.ConvertToRepositoryAuditEvent(auditEvent)
if err != nil {
    s.logger.Error(err, "Failed to convert to repository type")
    response.WriteRFC7807Error(w, s.logger, http.StatusInternalServerError, "conversion_error", "Conversion Error", err.Error(), "")
    return
}
```

### **Section 10-13: Database Write + DLQ + Response** (Lines 489-610)

**KEEP AS-IS** (121 lines)

**Minor changes**:
- Update variable references (`eventType` → `req.EventType`, etc.)
- Use `response.WriteJSONResponse()` for success response
- Keep all DLQ fallback logic unchanged
- Keep all metrics/logging unchanged

---

## ⚠️ **Critical Considerations**

### **Backward Compatibility**
**REMOVED**: Legacy field name support (service/operation/outcome aliases)

**Rationale**: User confirmed "we haven't released" so no backward compatibility needed.

**Impact**: Clients MUST use ADR-034 field names:
- `event_category` (not `service`)
- `event_action` (not `operation`)
- `event_outcome` (not `outcome`)

### **Error Messages**
**Improved**: OpenAPI unmarshaling provides better error messages:
- "missing required field 'event_category'"
- "invalid type for 'event_timestamp': expected RFC3339"
- "invalid enum value for 'event_outcome'"

### **Performance**
**Improved**: Single JSON unmarshal vs manual field-by-field extraction

---

## 📊 **Line-by-Line Mapping**

| Old Lines | New Lines | Change | Description |
|-----------|-----------|--------|-------------|
| 88-97 | 88-97 | Keep | Function signature + context |
| 98-116 | 98-110 | Simplify | Parse JSON (19 → 12 lines) |
| 118-171 | **DELETE** | -54 | Required field validation |
| 173-193 | **DELETE** | -21 | Manual field extraction |
| 207-229 | **DELETE** | -23 | Enum validation (moved to helper) |
| 231-255 | **DELETE** | -25 | Timestamp parsing (moved to helper) |
| 269-308 | **DELETE** | -40 | Timestamp bounds (moved to helper) |
| 310-334 | **DELETE** | -25 | Actor field extraction |
| 336-385 | **DELETE** | -50 | Field length validation (moved to helper) |
| 387-449 | **DELETE** | -63 | Optional field extraction |
| **NEW** | 112-140 | +29 | Validate + convert (using helpers) |
| 451-487 | **DELETE** | -37 | Manual repository struct creation |
| 489-610 | 142-264 | Keep | Database + DLQ + response |

**Summary**: 523 lines → ~264 lines (259-line reduction, 49%)

---

## ✅ **Testing Strategy**

### **Unit Tests**
- Conversion helpers already have unit tests
- Validation helpers already have unit tests

### **Integration Tests**
Update to use OpenAPI types:
```go
// Before
payload := map[string]interface{}{
    "event_type": "test.event",
    "event_category": "test",
    // ...
}

// After
req := dsclient.AuditEventRequest{
    EventType:     "test.event",
    EventCategory: "test",
    // ...
}
```

### **E2E Tests**
Same pattern - replace unstructured maps with OpenAPI types

---

## 🚦 **Execution Plan**

1. **Create new version** of `handleCreateAuditEvent` with OpenAPI types
2. **Compile** and fix any errors
3. **Update tests** to use OpenAPI types
4. **Run integration tests** to verify functionality
5. **Commit** when all tests pass

**Estimated Time**: 1 hour

---

## 📝 **Post-Refactoring Checklist**

- [ ] Function compiles without errors
- [ ] All validation logic preserved (enum, timestamp, lengths)
- [ ] DLQ fallback logic unchanged
- [ ] Metrics recording unchanged
- [ ] Logging unchanged
- [ ] Success/error responses correct
- [ ] Integration tests updated
- [ ] E2E tests updated
- [ ] All tests passing

---

**Document Version**: 1.0
**Last Updated**: 2025-12-13
**Status**: ✅ PLAN READY - Awaiting execution approval

