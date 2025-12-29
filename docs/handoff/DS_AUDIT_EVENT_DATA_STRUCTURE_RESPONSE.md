# DS Team Response: Audit Event Data Structure

**Date**: 2025-12-17
**Responded To**: Notification Team (NT)
**Question**: `docs/handoff/QUESTION_DS_AUDIT_EVENT_DATA_STRUCTURE.md`
**Status**: ✅ **COMPLETE - NT TEAM UNBLOCKED**

---

## 📋 Quick Summary

**NT Team Question**: "What is the correct way to structure `event_data` for audit events?"

**DS Team Answer**: **Use Pattern 2 with `audit.StructToMap()` helper**

---

## ✅ Authoritative Pattern

### Recommended Approach (All Services)

```go
// STEP 1: Define structured type (type-safe, compile-time validated)
type MessageSentEventData struct {
    NotificationID string `json:"notification_id"`
    Channel        string `json:"channel"`
    MessageType    string `json:"message_type"`
    RecipientCount int    `json:"recipient_count"`
}

// STEP 2: Use in business logic
payload := MessageSentEventData{
    NotificationID: notification.Name,
    Channel:        notification.Spec.Channel,
    MessageType:    notification.Spec.Type,
    RecipientCount: len(notification.Status.Recipients),
}

// STEP 3: Convert at API boundary using shared helper
eventDataMap, err := audit.StructToMap(payload)
if err != nil {
    return err
}

// STEP 4: Set event data
audit.SetEventData(event, eventDataMap)
```

---

## 🎯 Key Principles

| Principle | Explanation |
|-----------|-------------|
| **Type Safety in Business Logic** | Use structured Go types for all audit event payloads |
| **Boundary Conversion** | Convert to `map[string]interface{}` ONLY at API boundary |
| **Shared Helper** | Use `audit.StructToMap()` - NO custom `ToMap()` methods |
| **CommonEnvelope is Optional** | Only use if you need the outer envelope structure |
| **DD-AUDIT-004 Compliance** | Structured types eliminate `map[string]interface{}` from business logic |

---

## 📊 Pattern Comparison

| Pattern | Type Safety | Coding Standards | Recommended | Notes |
|---------|-------------|------------------|-------------|-------|
| **Pattern 1** (Direct map) | ❌ | ❌ | **NO** | Violates coding standards, no compile-time validation |
| **Pattern 2** (Structs + `ToMap()`) | ✅ | ⚠️ | **PARTIAL** | Good but use `audit.StructToMap()` instead of custom methods |
| **Pattern 2** (Structs + `audit.StructToMap()`) | ✅ | ✅ | **✅ YES** | **AUTHORITATIVE PATTERN** |
| **Pattern 3** (CommonEnvelope) | ⚠️ | ⚠️ | **OPTIONAL** | Use only if you need outer envelope structure |

---

## 🚀 NT Team Action Items

### Immediate (Unblocked)

1. ✅ **Define structured types** for Notification audit events:
   - `MessageSentEventData`
   - `MessageFailedEventData`
   - `ChannelConfiguredEventData`
   - `RetryAttemptedEventData`

2. ✅ **Use `audit.StructToMap()`** for all conversions

3. ✅ **DO NOT** create custom `ToMap()` methods

4. ✅ **DO NOT** use `CommonEnvelope` unless specifically needed

5. ✅ **Reference DD-AUDIT-004** in code comments

---

## 📚 Authority References

| Reference | Purpose | Key Insight |
|-----------|---------|-------------|
| **DD-AUDIT-004** | Structured types mandate | All services must use structured types for audit event payloads |
| **`pkg/audit/helpers.go:127-153`** | `StructToMap()` helper | "This is the recommended approach per DD-AUDIT-004" |
| **ADR-034** | Unified audit table design | Defines `event_data` as JSONB with flexible schema |
| **02-go-coding-standards.mdc** | Coding standards | Avoid `any`/`interface{}` unless absolutely necessary |

---

## 🔧 Migration Guidance

### Services Need Migration

| Service | Current Pattern | Action Required | Effort |
|---------|----------------|-----------------|--------|
| **SignalProcessing** | Pattern 1 (Direct map) | Migrate to structured types + `audit.StructToMap()` | 1-2 hours |
| **WorkflowExecution** | Pattern 2 (Custom `ToMap()`) | Replace custom `ToMap()` with `audit.StructToMap()` | 30 min |
| **AIAnalysis** | Pattern 2 (Custom `ToMap()`) | Replace custom `ToMap()` with `audit.StructToMap()` | 30 min |
| **Notification** | ⏳ Not started | Implement Pattern 2 with `audit.StructToMap()` | 1-2 hours |

---

## ❓ FAQ

### Q: Is `CommonEnvelope` mandatory?
**A**: **NO** - `CommonEnvelope` is optional. Use it only if you need the outer envelope structure (version, service, operation, status). Most services don't need it.

### Q: Why does OpenAPI spec say "CommonEnvelope structure"?
**A**: Historical documentation. We'll update the OpenAPI spec to clarify that `CommonEnvelope` is optional.

### Q: How does this comply with DD-AUDIT-004?
**A**: DD-AUDIT-004 mandates structured types in business logic. The `map[string]interface{}` in the API contract is the boundary layer. Type safety is maintained through:
- **Business Logic**: Structured types (type-safe)
- **Boundary**: `audit.StructToMap()` conversion
- **API Contract**: `map[string]interface{}` (required by OpenAPI spec)

### Q: Should I create custom `ToMap()` methods?
**A**: **NO** - Use the shared `audit.StructToMap()` helper instead. This ensures consistency across all services.

### Q: What if I need the outer envelope structure?
**A**: Use `CommonEnvelope` with structured types:
```go
payload := MessageSentEventData{...}
payloadMap, _ := audit.StructToMap(payload)

envelope := audit.NewEventData(
    "notification",
    "message.sent",
    "success",
    payloadMap,
)

audit.SetEventDataFromEnvelope(event, envelope)
```

---

## 📈 Benefits of Recommended Pattern

| Benefit | Impact |
|---------|--------|
| **Type Safety** | Compile-time field validation, no runtime typos |
| **Maintainability** | Refactor-safe, IDE autocomplete support |
| **Consistency** | All services use same pattern (`audit.StructToMap()`) |
| **Testing** | 100% field validation through structured types |
| **Coding Standards** | Eliminates `map[string]interface{}` from business logic |
| **Documentation** | Struct definitions are authoritative schema |

---

## ✅ Resolution Summary

**NT Team Question**: Which pattern should we use for audit event data?

**DS Team Answer**:
- ✅ **Authoritative Pattern**: Structured types + `audit.StructToMap()`
- ✅ **CommonEnvelope**: Optional, not mandatory
- ✅ **Type Safety**: Structured types in business logic
- ✅ **Boundary Conversion**: Use shared `audit.StructToMap()` helper
- ✅ **DD-AUDIT-004 Compliance**: Achieved through structured types

**NT Team Status**: ✅ **UNBLOCKED** - Proceed with structured types + `audit.StructToMap()`

**Documentation Updates**:
- ⏸️ Update OpenAPI spec comment (clarify CommonEnvelope is optional)
- ⏸️ Add migration guide for Pattern 1 services
- ⏸️ Update DD-AUDIT-004 with recommended pattern examples

---

## 🔗 Related Documents

- **Question**: `docs/handoff/QUESTION_DS_AUDIT_EVENT_DATA_STRUCTURE.md` (NT team question with full DS response)
- **DD-AUDIT-004**: `docs/architecture/decisions/DD-AUDIT-004-structured-types-for-audit-event-payloads.md`
- **Helper Implementation**: `pkg/audit/helpers.go:127-153`
- **ADR-034**: `docs/architecture/decisions/ADR-034-unified-audit-table-design.md`

---

**Confidence Assessment**: **100%**
**Justification**:
- Authoritative references (DD-AUDIT-004, pkg/audit/helpers.go)
- Explicit comment in code: "This is the recommended approach per DD-AUDIT-004"
- Consistent with project coding standards
- Clear migration path for existing services

**NT Team Next Steps**: Implement structured types + `audit.StructToMap()` for all Notification audit events


