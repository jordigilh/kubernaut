# DataStorage - testutil.ValidateAuditEvent Refactoring Plan
**Date**: December 20, 2025
**Status**: 🚧 **IN PROGRESS**
**Priority**: P0 - MANDATORY (V1.0 maturity requirement)

---

## 🎯 Objective

Refactor DataStorage integration tests to use standardized `testutil.ValidateAuditEvent()` helper instead of manual `Expect()` calls.

---

## 📋 Files to Refactor

### 1. `test/integration/datastorage/audit_events_repository_integration_test.go`
**Estimated Manual Validations**: ~10
**Complexity**: Medium
**Example Pattern**:
```go
// Before
Expect(dbEventType).To(Equal("gateway.signal.received"))
Expect(dbEventCategory).To(Equal("gateway"))
Expect(dbEventAction).To(Equal("received"))
Expect(dbEventOutcome).To(Equal("success"))
Expect(dbCorrelationID).To(Equal(testEvent.CorrelationID))

// After
testutil.ValidateAuditEvent(retrievedEvent, testutil.ExpectedAuditEvent{
    EventType:     "gateway.signal.received",
    EventCategory: dsgen.AuditEventEventCategoryGateway,
    EventAction:   "received",
    EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
    CorrelationID: testEvent.CorrelationID,
})
```

### 2. `test/integration/datastorage/audit_events_query_api_test.go`
**Estimated Manual Validations**: ~5
**Complexity**: Low
**Example Pattern**:
```go
// Before
Expect(event["event_type"]).To(Equal(targetEventType))

// After
testutil.ValidateAuditEvent(event, testutil.ExpectedAuditEvent{
    EventType: targetEventType,
})
```

### 3. `test/integration/datastorage/audit_events_write_api_test.go`
**Estimated Manual Validations**: ~8
**Complexity**: Medium

---

## 🛠️ Implementation Pattern

### Step 1: Add Import

```go
import (
    testutil "github.com/jordigilh/kubernaut/pkg/testutil"
)
```

### Step 2: Replace Manual Validations

**Pattern A: Full Event Validation**
```go
// Before (7 lines)
Expect(event.EventType).To(Equal("gateway.signal.received"))
Expect(event.EventCategory).To(Equal("gateway"))
Expect(event.EventAction).To(Equal("received"))
Expect(event.EventOutcome).To(Equal("success"))
Expect(event.CorrelationID).To(Equal("test-123"))
Expect(*event.ResourceType).To(Equal("Signal"))
Expect(*event.ResourceID).To(Equal("fp-123"))

// After (1 structured call)
testutil.ValidateAuditEvent(event, testutil.ExpectedAuditEvent{
    EventType:     "gateway.signal.received",
    EventCategory: dsgen.AuditEventEventCategoryGateway,
    EventAction:   "received",
    EventOutcome:  dsgen.AuditEventEventOutcomeSuccess,
    CorrelationID: "test-123",
    ResourceType:  ptr("Signal"),
    ResourceID:    ptr("fp-123"),
})
```

**Pattern B: Partial Validation (Only Required Fields)**
```go
// Before (2 lines)
Expect(event.EventType).To(Equal(targetEventType))
Expect(event.EventCategory).To(Equal("gateway"))

// After (1 structured call, only validates specified fields)
testutil.ValidateAuditEvent(event, testutil.ExpectedAuditEvent{
    EventType:     targetEventType,
    EventCategory: dsgen.AuditEventEventCategoryGateway,
})
```

### Step 3: Add Pointer Helper (if not exists)

```go
// Add to test file if needed
func ptr(s string) *string { return &s }
```

---

## ✅ Benefits

| Aspect | Before | After |
|--------|--------|-------|
| **Lines of Code** | 7-10 per validation | 1 structured call |
| **Maintainability** | Individual Expect() calls | Centralized validation logic |
| **Completeness** | Easy to miss fields | Helper ensures all required fields |
| **Consistency** | Varies by test | Standardized across all services |
| **Type Safety** | String comparisons | Enum types (dsgen.AuditEventEventCategory*) |

---

## 📊 Progress Tracking

- [ ] audit_events_repository_integration_test.go (~10 validations)
- [ ] audit_events_query_api_test.go (~5 validations)
- [ ] audit_events_write_api_test.go (~8 validations)
- [ ] audit_events_schema_test.go (if applicable)
- [ ] dlq_test.go (if has audit event validations)
- [ ] Verify all tests still pass
- [ ] Run maturity validation to confirm

**Total Estimated Effort**: 2-3 hours

---

## 🧪 Verification

After refactoring:
```bash
# Run integration tests
make test-integration-datastorage

# Verify maturity validation passes
make validate-maturity | grep datastorage
```

**Expected Result**:
```
Checking: datastorage (stateless)
  ✅ Prometheus metrics
  ✅ Health endpoint
  ✅ Graceful shutdown
  ✅ Audit integration
  ✅ Audit uses OpenAPI client
  ✅ Audit uses testutil validator  ← RESOLVED
```

---

**Status**: 🚧 **READY TO IMPLEMENT**


